import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multiSelect";
import { ScrollArea } from "@/components/ui/scrollArea";
import {
	getErrorMessage,
	useGetCustomersQuery,
	useGetSessionUsersQuery,
	useGetTeamsQuery,
} from "@/lib/store";
import {
	useGetPromptAccessQuery,
	useUpdatePromptAccessMutation,
} from "@/lib/store/apis/promptsApi";
import {
	Building2,
	CheckCircle2,
	Globe,
	Info,
	Loader2,
	Lock,
	ShieldCheck,
	UserCheck,
	Users,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { usePromptContext } from "../context";

export function PromptAccessDialog() {
	const { promptAccessDialog, setPromptAccessDialog } = usePromptContext();
	const prompt = promptAccessDialog.prompt;
	const isOpen = promptAccessDialog.open && !!prompt;

	const promptId = prompt?.id ?? "";

	// Fetch current prompt access configuration
	const {
		data: accessData,
		isLoading: isAccessLoading,
		isError: accessFailed,
		error: accessError,
		refetch: refetchAccess,
	} = useGetPromptAccessQuery(promptId, {
		skip: !isOpen || !promptId,
	});

	// Fetch governance entities
	const { data: customersData, isLoading: isCustomersLoading, isError: customersFailed, error: customersError } = useGetCustomersQuery(
		undefined,
		{ skip: !isOpen },
	);
	const { data: teamsData, isLoading: isTeamsLoading, isError: teamsFailed, error: teamsError } = useGetTeamsQuery(undefined, {
		skip: !isOpen,
	});
	const { data: sessionUsersData, isLoading: isUsersLoading, isError: usersFailed, error: usersError } = useGetSessionUsersQuery(
		undefined,
		{ skip: !isOpen },
	);
	const accessQueryFailed = accessFailed || customersFailed || teamsFailed || usersFailed;

	const [updatePromptAccess, { isLoading: isUpdating }] = useUpdatePromptAccessMutation();

	// Local selection state
	const [selectedCustomerIds, setSelectedCustomerIds] = useState<string[]>([]);
	const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>([]);
	const [selectedUserIds, setSelectedUserIds] = useState<string[]>([]);

	// Initialize local selections from API response whenever accessData loads or changes
	useEffect(() => {
		if (accessData) {
			setSelectedCustomerIds(accessData.customer_ids || []);
			setSelectedTeamIds(accessData.team_ids || []);
			setSelectedUserIds(accessData.user_ids || []);
		} else {
			setSelectedCustomerIds([]);
			setSelectedTeamIds([]);
			setSelectedUserIds([]);
		}
	}, [accessData, isOpen]);

	// Options for Customer MultiSelect
	const customerOptions = useMemo<MultiSelectOption[]>(() => {
		return (customersData?.customers || []).map((c) => ({
			value: c.id,
			label: c.name,
		}));
	}, [customersData]);

	// Options for Team MultiSelect
	const teamOptions = useMemo<MultiSelectOption[]>(() => {
		const teams = teamsData?.teams || [];
		const customerMap = new Map<string, string>();
		for (const c of customersData?.customers || []) {
			customerMap.set(c.id, c.name);
		}

		return teams.map((t) => {
			const customerName = t.customer?.name || (t.customer_id ? customerMap.get(t.customer_id) : undefined);
			return {
				value: t.id,
				label: customerName ? `${t.name} (${customerName})` : t.name,
			};
		});
	}, [teamsData, customersData]);

	// Options for User MultiSelect
	const userOptions = useMemo<MultiSelectOption[]>(() => {
		const users = sessionUsersData || [];
		return users
			.filter((u) => (u.status || "approved") === "approved")
			.map((u) => ({
				value: u.id,
				label: u.username ? (u.email ? `${u.username} (${u.email})` : u.username) : u.email || u.id,
			}));
	}, [sessionUsersData]);

	// Compute effective users preview locally based on current selections
	const previewUsers = useMemo(() => {
		const customerSet = new Set(selectedCustomerIds);
		const teamSet = new Set(selectedTeamIds);
		const userSet = new Set(selectedUserIds);

		const allTeams = teamsData?.teams || [];
		const allUsers = sessionUsersData || [];

		// Map of user id -> { user, origin }
		const effectiveMap = new Map<
			string,
			{ id: string; username: string; email: string; origin: string }
		>();

		// If owner exists, include them
		if (accessData?.owner_user_id) {
			const owner = allUsers.find((u) => u.id === accessData.owner_user_id);
			if (owner) {
				effectiveMap.set(owner.id, {
					id: owner.id,
					username: owner.username,
					email: owner.email || "",
					origin: "Owner",
				});
			}
		}

		// 1. Direct user assignment
		for (const uid of userSet) {
			const u = allUsers.find((user) => user.id === uid);
			if (u && !effectiveMap.has(u.id)) {
				effectiveMap.set(u.id, {
					id: u.id,
					username: u.username,
					email: u.email || "",
					origin: "Direct User",
				});
			}
		}

		// 2. Team assignment
		for (const tid of teamSet) {
			const team = allTeams.find((t) => t.id === tid);
			const teamName = team?.name || tid;
			// Find team members if populated or fallback
			const members = team?.budgets ? [] : []; // if members exist on team
			// Also check accessData effective_users if matching
			if (accessData?.effective_users) {
				for (const eu of accessData.effective_users) {
					if (eu.origin === "team" && !effectiveMap.has(eu.user_id)) {
						effectiveMap.set(eu.user_id, {
							id: eu.user_id,
							username: eu.username,
							email: eu.email,
							origin: `Team: ${teamName}`,
						});
					}
				}
			}
		}

		// 3. Customer assignment
		for (const cid of customerSet) {
			const customer = (customersData?.customers || []).find((c) => c.id === cid);
			const customerName = customer?.name || cid;
			if (accessData?.effective_users) {
				for (const eu of accessData.effective_users) {
					if (eu.origin === "customer" && !effectiveMap.has(eu.user_id)) {
						effectiveMap.set(eu.user_id, {
							id: eu.user_id,
							username: eu.username,
							email: eu.email,
							origin: `Customer: ${customerName}`,
						});
					}
				}
			}
		}

		// If accessData has effective_users and no local overrides differ yet, use them as baseline
		if (accessData?.effective_users && effectiveMap.size === 0) {
			for (const eu of accessData.effective_users) {
				effectiveMap.set(eu.user_id, {
					id: eu.user_id,
					username: eu.username,
					email: eu.email,
					origin: eu.origin === "direct" ? "Direct User" : eu.origin === "team" ? "Team" : "Customer",
				});
			}
		}

		return Array.from(effectiveMap.values());
	}, [
		selectedCustomerIds,
		selectedTeamIds,
		selectedUserIds,
		accessData,
		teamsData,
		customersData,
		sessionUsersData,
	]);

	const isScopingApplied =
		selectedCustomerIds.length > 0 || selectedTeamIds.length > 0 || selectedUserIds.length > 0;

	const handleClose = () => {
		setPromptAccessDialog({ open: false, prompt: undefined });
	};

	const handleClearAll = () => {
		setSelectedCustomerIds([]);
		setSelectedTeamIds([]);
		setSelectedUserIds([]);
	};

	const handleSave = async () => {
		if (!promptId) return;

		try {
			await updatePromptAccess({
				promptId,
				data: {
					customer_ids: selectedCustomerIds,
					team_ids: selectedTeamIds,
					user_ids: selectedUserIds,
				},
			}).unwrap();

			toast.success("Prompt access permissions updated successfully");
			refetchAccess();
			handleClose();
		} catch (error) {
			toast.error(getErrorMessage(error) || "Failed to update prompt access");
		}
	};

	const isLoadingInitial =
		isAccessLoading || isCustomersLoading || isTeamsLoading || isUsersLoading;

	return (
		<Dialog open={isOpen} onOpenChange={(open) => !open && handleClose()}>
			<DialogContent
				className="sm:max-w-2xl max-h-[85vh] flex flex-col p-6 gap-5"
				data-testid="prompt-access-dialog"
			>
				<DialogHeader className="space-y-1.5 pb-2 border-b">
					<div className="flex items-center gap-2">
						<div className="p-2 rounded-md bg-primary/10 text-primary">
							<ShieldCheck className="h-5 w-5" />
						</div>
						<div>
							<DialogTitle className="text-lg font-semibold flex items-center gap-2">
								Manage Prompt Access
								<Badge variant="outline" className="font-mono text-xs">
									{prompt?.name}
								</Badge>
							</DialogTitle>
							<DialogDescription className="text-xs text-muted-foreground">
								Configure multi-tenant access scoping by assigning customers, teams, or individual users.
							</DialogDescription>
						</div>
					</div>
				</DialogHeader>

				{accessQueryFailed ? (
					<div className="py-4">
						<QueryErrorBanner
							testId="prompt-access-query-error"
							message={
								getErrorMessage(accessError || customersError || teamsError || usersError) ||
								"Failed to load access settings."
							}
						/>
					</div>
				) : null}

				{isLoadingInitial ? (
					<div className="py-12 flex flex-col items-center justify-center gap-2 text-muted-foreground">
						<Loader2 className="h-6 w-6 animate-spin text-primary" />
						<span className="text-sm">Loading access permissions...</span>
					</div>
				) : (
					<div className="flex-1 overflow-y-auto space-y-5 pr-1">
						{/* Status Banner */}
						<div
							className={`rounded-lg border p-3 flex items-start gap-3 text-xs leading-relaxed transition-colors ${
								isScopingApplied
									? "bg-amber-500/10 border-amber-500/30 text-amber-900 dark:text-amber-200"
									: "bg-muted/40 border-border/60 text-muted-foreground"
							}`}
						>
							{isScopingApplied ? (
								<Lock className="h-4 w-4 shrink-0 text-amber-500 mt-0.5" />
							) : (
								<Globe className="h-4 w-4 shrink-0 text-muted-foreground mt-0.5" />
							)}
							<div>
								<span className="font-semibold block mb-0.5">
									{isScopingApplied ? "Restricted Access Scope" : "Open Access Scope"}
								</span>
								{isScopingApplied ? (
									<span>
										This prompt is restricted. Only users within the assigned customers, teams, or direct user accounts can view and execute it.
									</span>
								) : (
									<span>
										No restrictions applied. All workspace users with Prompt Repository permissions can view and run this prompt.
									</span>
								)}
							</div>
						</div>

						{/* Multi-Select Selectors */}
						<div className="space-y-4">
							{/* Customers Select */}
							<div className="space-y-1.5">
								<div className="flex items-center justify-between">
									<Label className="text-xs font-medium flex items-center gap-1.5">
										<Building2 className="h-3.5 w-3.5 text-muted-foreground" />
										Customers
										{selectedCustomerIds.length > 0 && (
											<Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
												{selectedCustomerIds.length}
											</Badge>
										)}
									</Label>
									<span className="text-[11px] text-muted-foreground">
										Assign to all teams under selected customers
									</span>
								</div>
								<MultiSelect
									options={customerOptions}
									defaultValue={selectedCustomerIds}
									resetOnDefaultValueChange
									onValueChange={setSelectedCustomerIds}
									placeholder="Select customers..."
									emptyIndicator="No customers available."
									className="min-h-9 border-input bg-transparent text-xs"
									data-testid="prompt-customers-multiselect"
								/>
							</div>

							{/* Teams Select */}
							<div className="space-y-1.5">
								<div className="flex items-center justify-between">
									<Label className="text-xs font-medium flex items-center gap-1.5">
										<Users className="h-3.5 w-3.5 text-muted-foreground" />
										Teams
										{selectedTeamIds.length > 0 && (
											<Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
												{selectedTeamIds.length}
											</Badge>
										)}
									</Label>
									<span className="text-[11px] text-muted-foreground">
										Assign to all members in selected teams
									</span>
								</div>
								<MultiSelect
									options={teamOptions}
									defaultValue={selectedTeamIds}
									resetOnDefaultValueChange
									onValueChange={setSelectedTeamIds}
									placeholder="Select teams..."
									emptyIndicator="No teams available."
									className="min-h-9 border-input bg-transparent text-xs"
									data-testid="prompt-teams-multiselect"
								/>
							</div>

							{/* Direct Users Select */}
							<div className="space-y-1.5">
								<div className="flex items-center justify-between">
									<Label className="text-xs font-medium flex items-center gap-1.5">
										<UserCheck className="h-3.5 w-3.5 text-muted-foreground" />
										Specific Users
										{selectedUserIds.length > 0 && (
											<Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
												{selectedUserIds.length}
											</Badge>
										)}
									</Label>
									<span className="text-[11px] text-muted-foreground">
										Explicit user-level access override
									</span>
								</div>
								<MultiSelect
									options={userOptions}
									defaultValue={selectedUserIds}
									resetOnDefaultValueChange
									onValueChange={setSelectedUserIds}
									placeholder="Select individual users..."
									emptyIndicator="No approved users found."
									className="min-h-9 border-input bg-transparent text-xs"
									data-testid="prompt-users-multiselect"
								/>
							</div>
						</div>

						{/* Effective Access Preview */}
						<div className="border rounded-md p-3.5 space-y-2.5 bg-muted/20">
							<div className="flex items-center justify-between">
								<div className="flex items-center gap-2">
									<CheckCircle2 className="h-4 w-4 text-primary" />
									<span className="text-xs font-semibold">
										Effective Access Members ({previewUsers.length})
									</span>
								</div>
								{isScopingApplied && (
									<Button
										variant="ghost"
										size="sm"
										onClick={handleClearAll}
										className="h-6 text-[11px] px-2 text-muted-foreground hover:text-destructive"
									>
										Clear All Filters
									</Button>
								)}
							</div>

							{previewUsers.length === 0 ? (
								<div className="py-4 text-center text-xs text-muted-foreground border border-dashed rounded bg-background/50">
									{!isScopingApplied
										? "All workspace users have access (Unrestricted)."
										: "No users currently matched by the selected scopes."}
								</div>
							) : (
								<ScrollArea className="max-h-[140px] pr-2">
									<div className="space-y-1.5">
										{previewUsers.map((user) => (
											<div
												key={user.id}
												className="flex items-center justify-between p-2 rounded-md bg-background border text-xs"
											>
												<div className="flex flex-col min-w-0">
													<span className="font-medium truncate">{user.username}</span>
													{user.email && (
														<span className="text-[10px] text-muted-foreground truncate">
															{user.email}
														</span>
													)}
												</div>
												<Badge
													variant={
														user.origin === "Owner"
															? "default"
															: user.origin.startsWith("Direct")
															? "secondary"
															: "outline"
													}
													className="text-[10px] shrink-0"
												>
													{user.origin}
												</Badge>
											</div>
										))}
									</div>
								</ScrollArea>
							)}
						</div>
					</div>
				)}

				<DialogFooter className="border-t pt-3 flex items-center justify-between sm:justify-between">
					<div className="text-[11px] text-muted-foreground flex items-center gap-1">
						<Info className="h-3 w-3" />
						Changes take effect immediately on next query.
					</div>
					<div className="flex items-center gap-2">
						<Button
							variant="outline"
							size="sm"
							onClick={handleClose}
							disabled={isUpdating}
							data-testid="prompt-access-cancel"
						>
							Cancel
						</Button>
						<Button
							size="sm"
							onClick={handleSave}
							disabled={isUpdating || isLoadingInitial}
							data-testid="prompt-access-save"
						>
							{isUpdating ? (
								<>
									<Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
									Saving...
								</>
							) : (
								"Save Access"
							)}
						</Button>
					</div>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
export default PromptAccessDialog;
