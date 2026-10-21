import { DataTablePagination } from "@/components/table/dataTablePagination";
import { PIN_SHADOW_RIGHT } from "@/components/table/columnPinning";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alertDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { resetDurationLabels } from "@/lib/constants/governance";
import { getErrorMessage, useDeleteTeamMutation } from "@/lib/store";
import { Customer, Team, VirtualKey } from "@/lib/types/governance";
import { cn } from "@/lib/utils";
import { describeTeamVirtualKey, formatCurrency, virtualKeysForTeam } from "@/lib/utils/governance";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { Link } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, Edit, MoreHorizontal, Plus, ScrollText, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Checkbox } from "@/components/ui/checkbox";
import TeamSheet from "./teamSheet";
import { TeamsEmptyState } from "./teamsEmptyState";

// Helper to format reset duration for display
const formatResetDuration = (duration: string) => {
	return resetDurationLabels[duration] || duration;
};

function TeamActionsMenu({
	team,
	hasUpdateAccess,
	hasDeleteAccess,
	isDeleting,
	onEdit,
	onDelete,
}: {
	team: Team;
	hasUpdateAccess: boolean;
	hasDeleteAccess: boolean;
	isDeleting: boolean;
	onEdit: (team: Team) => void;
	onDelete: (teamId: string) => void;
}) {
	const [isOpen, setIsOpen] = useState(false);
	const [deleteOpen, setDeleteOpen] = useState(false);

	return (
		<>
			<DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
				<DropdownMenuTrigger asChild>
					<Button
						variant="ghost"
						size="icon"
						className="h-8 w-8"
						aria-label={`Team actions for ${team.name}`}
						data-testid={`team-actions-btn-${team.name}`}
					>
						<MoreHorizontal className="h-4 w-4" />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem
						className="cursor-pointer"
						disabled={!hasUpdateAccess}
						title={!hasUpdateAccess ? "View-only access: editing is disabled" : undefined}
						data-testid={`team-edit-btn-${team.name}`}
						onSelect={(e) => {
							e.preventDefault();
							onEdit(team);
							setIsOpen(false);
						}}
					>
						<Edit className="h-4 w-4" />
						Edit
					</DropdownMenuItem>
					<DropdownMenuItem asChild className="cursor-pointer" data-testid={`team-view-logs-btn-${team.name}`}>
						<Link to="/workspace/logs" search={{ team_ids: [team.id] }} onClick={() => setIsOpen(false)}>
							<ScrollText className="h-4 w-4" />
							View logs
						</Link>
					</DropdownMenuItem>
					<DropdownMenuItem
						variant="destructive"
						className="cursor-pointer"
						disabled={!hasDeleteAccess}
						title={!hasDeleteAccess ? "Delete permission is required" : undefined}
						data-testid={`team-delete-btn-${team.name}`}
						onSelect={(e) => {
							e.preventDefault();
							setDeleteOpen(true);
							setIsOpen(false);
						}}
					>
						<Trash2 className="h-4 w-4" />
						Delete
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
			<AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete Team</AlertDialogTitle>
						<AlertDialogDescription>
							Are you sure you want to delete &quot;{team.name}&quot;? This will also unassign any virtual keys from this team. This action
							cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction onClick={() => onDelete(team.id)} disabled={isDeleting} className="bg-red-600 hover:bg-red-700">
							{isDeleting ? "Deleting..." : "Delete"}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	);
}

interface TeamsTableProps {
	teams: Team[];
	totalCount: number;
	customers: Customer[];
	virtualKeys: VirtualKey[];
	search: string;
	debouncedSearch: string;
	onSearchChange: (value: string) => void;
	offset: number;
	limit: number;
	onOffsetChange: (offset: number) => void;
	onLimitChange?: (limit: number) => void;
	selectedTeamId: string | null;
	onTeamAdd: () => void;
	onTeamSelect: (team: Team | null) => void;
	onDialogClose: () => void;
	isLoading?: boolean;
}

export default function TeamsTable({
	teams,
	totalCount,
	customers,
	virtualKeys,
	search,
	debouncedSearch,
	onSearchChange,
	offset,
	limit,
	onOffsetChange,
	onLimitChange,
	selectedTeamId,
	onTeamAdd,
	onTeamSelect,
	onDialogClose,
	isLoading,
}: TeamsTableProps) {
	const showTeamSheet = selectedTeamId !== null && selectedTeamId !== "";
	const editingTeam = selectedTeamId && selectedTeamId !== "new" ? (teams.find((t) => t.id === selectedTeamId) ?? null) : null;

	// If a team ID is in the URL but can't be resolved (deleted or filtered out),
	// clear it so we don't silently open the dialog in "create" mode.
	useEffect(() => {
		if (selectedTeamId && selectedTeamId !== "new" && !editingTeam) {
			onDialogClose();
		}
	}, [selectedTeamId, editingTeam, onDialogClose]);

	const hasCreateAccess = useRbac(RbacResource.Teams, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.Teams, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Teams, RbacOperation.Delete);

	const [deleteTeam, { isLoading: isDeleting }] = useDeleteTeamMutation();

	const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
	const [showBulkDeleteDialog, setShowBulkDeleteDialog] = useState(false);
	const [isBulkDeleting, setIsBulkDeleting] = useState(false);

	const visibleIds = useMemo(() => teams.map((t) => t.id), [teams]);
	const selectedVisibleIds = useMemo(() => visibleIds.filter((id) => selectedIds.has(id)), [selectedIds, visibleIds]);
	const selectedCount = selectedIds.size;
	const allVisibleSelected = visibleIds.length > 0 && selectedVisibleIds.length === visibleIds.length;
	const someVisibleSelected = selectedVisibleIds.length > 0 && selectedVisibleIds.length < visibleIds.length;

	const toggleSelectAllVisible = (checked: boolean) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			for (const id of visibleIds) {
				if (checked) {
					next.add(id);
				} else {
					next.delete(id);
				}
			}
			return next;
		});
	};

	const toggleSelectTeam = (teamId: string, checked: boolean) => {
		setSelectedIds((prev) => {
			const next = new Set(prev);
			if (checked) {
				next.add(teamId);
			} else {
				next.delete(teamId);
			}
			return next;
		});
	};

	const handleBulkDelete = async () => {
		if (selectedCount === 0) return;
		setIsBulkDeleting(true);
		let successCount = 0;
		let failCount = 0;
		const ids = Array.from(selectedIds);
		for (const id of ids) {
			try {
				await deleteTeam(id).unwrap();
				successCount++;
			} catch {
				failCount++;
			}
		}
		setIsBulkDeleting(false);
		setShowBulkDeleteDialog(false);
		setSelectedIds(new Set());
		if (successCount > 0) {
			toast.success(`Successfully deleted ${successCount} team${successCount > 1 ? "s" : ""}`);
		}
		if (failCount > 0) {
			toast.error(`Failed to delete ${failCount} team${failCount > 1 ? "s" : ""}`);
		}
	};

	const handleDelete = async (teamId: string) => {
		try {
			await deleteTeam(teamId).unwrap();
			setSelectedIds((prev) => {
				const next = new Set(prev);
				next.delete(teamId);
				return next;
			});
			toast.success("Team deleted successfully");
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const handleAddTeam = () => {
		onTeamAdd();
	};

	const handleEditTeam = (team: Team) => {
		onTeamSelect(team);
	};

	const handleTeamSaved = () => {
		onDialogClose();
	};

	const getVirtualKeysForTeam = (team: Team) => virtualKeysForTeam(virtualKeys, team);

	const getCustomerName = (customerId?: string) => {
		if (!customerId) return "-";
		const customer = customers.find((c) => c.id === customerId);
		return customer ? customer.name : "Unknown Customer";
	};

	const hasActiveFilters = debouncedSearch;

	// True empty state: no teams at all (not just filtered to zero)
	if (totalCount === 0 && !hasActiveFilters) {
		return (
			<>
				<TooltipProvider>
					{showTeamSheet && <TeamSheet team={editingTeam} customers={customers} onSave={handleTeamSaved} onCancel={onDialogClose} />}
					<TeamsEmptyState onAddClick={handleAddTeam} canCreate={hasCreateAccess} />
				</TooltipProvider>
			</>
		);
	}

	return (
		<>
			<TooltipProvider>
				{showTeamSheet && <TeamSheet team={editingTeam} customers={customers} onSave={handleTeamSaved} onCancel={onDialogClose} />}

				<AlertDialog open={showBulkDeleteDialog} onOpenChange={setShowBulkDeleteDialog}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>Delete Selected Teams</AlertDialogTitle>
							<AlertDialogDescription>
								Are you sure you want to delete {selectedCount} selected team{selectedCount > 1 ? "s" : ""}? This will also unassign any virtual keys from these teams. This action cannot be undone.
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel disabled={isBulkDeleting}>Cancel</AlertDialogCancel>
							<AlertDialogAction
								onClick={handleBulkDelete}
								disabled={isBulkDeleting || selectedCount === 0}
								className="bg-red-600 hover:bg-red-700"
								data-testid="teams-bulk-delete-confirm-btn"
							>
								{isBulkDeleting ? "Deleting..." : "Delete Selected"}
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>

				<div className="flex grow flex-col overflow-y-auto">
					<div className="mb-4 flex items-center justify-between">
						<div>
							<div className="flex items-center gap-2">
								<h2 className="text-lg font-semibold">Teams</h2>
								{!hasUpdateAccess && (
									<Badge variant="outline" className="text-amber-500 border-amber-500/30 bg-amber-500/10 text-xs">
										View Only
									</Badge>
								)}
							</div>
							<p className="text-muted-foreground text-sm">Organize users into teams with shared budgets and access controls.</p>
						</div>
						<div className="flex items-center gap-2">
							{selectedCount > 0 && (
								<Button
									variant="destructive"
									data-testid="teams-bulk-delete-btn"
									onClick={() => setShowBulkDeleteDialog(true)}
									disabled={!hasDeleteAccess || isBulkDeleting}
								>
									<Trash2 className="h-4 w-4 mr-1" />
									Delete Selected ({selectedCount})
								</Button>
							)}
							<Button data-testid="create-team-btn" onClick={handleAddTeam} disabled={!hasCreateAccess}>
								<Plus className="h-4 w-4" />
								Add Team
							</Button>
						</div>
					</div>

					<div className="mb-4 flex items-center gap-3">
						<div className="relative max-w-sm flex-1">
							<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
							<Input
								aria-label="Search teams by name"
								placeholder="Search by name..."
								value={search}
								onChange={(e) => onSearchChange(e.target.value)}
								className="pl-9"
								data-testid="teams-search-input"
							/>
						</div>
					</div>

					<div className="mb-2 grow overflow-auto rounded-sm border" data-testid="teams-table">
						<Table className="min-w-[1100px]" containerClassName="h-full">
							<TableHeader className="bg-background sticky top-0">
								<TableRow>
									<TableHead className="w-[48px]">
										<Checkbox
											checked={allVisibleSelected || (someVisibleSelected ? "indeterminate" : false)}
											onCheckedChange={(checked) => toggleSelectAllVisible(checked === true)}
											aria-label="Select all teams on this page"
											data-testid="teams-select-all-checkbox"
										/>
									</TableHead>
									<TableHead>Name</TableHead>
									<TableHead>Customer</TableHead>
									<TableHead>Budget</TableHead>
									<TableHead>Rate Limit</TableHead>
									<TableHead>Virtual Keys</TableHead>
									<TableHead className={`bg-muted sticky right-0 z-10 w-[56px] text-right ${PIN_SHADOW_RIGHT}`}></TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{teams.length === 0 ? (
									<TableRow>
										<TableCell colSpan={7} className="h-24 text-center">
											<span className="text-muted-foreground text-sm">No matching teams found.</span>
										</TableCell>
									</TableRow>
								) : (
									teams.map((team) => {
										const vks = getVirtualKeysForTeam(team);
										const customerName = getCustomerName(team.customer_id);

										// Budget calculations — any of the team's budgets exhausted
										const teamBudgets = team.budgets ?? [];
										const isBudgetExhausted = teamBudgets.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit);

										// Rate limit calculations
										const isTokenLimitExhausted =
											team.rate_limit?.token_max_limit &&
											team.rate_limit.token_max_limit > 0 &&
											(team.rate_limit.token_current_usage ?? 0) >= team.rate_limit.token_max_limit;
										const isRequestLimitExhausted =
											team.rate_limit?.request_max_limit &&
											team.rate_limit.request_max_limit > 0 &&
											(team.rate_limit.request_current_usage ?? 0) >= team.rate_limit.request_max_limit;
										const isRateLimitExhausted = isTokenLimitExhausted || isRequestLimitExhausted;
										const tokenPercentage =
											team.rate_limit?.token_max_limit && team.rate_limit.token_max_limit > 0
												? Math.min(((team.rate_limit.token_current_usage ?? 0) / team.rate_limit.token_max_limit) * 100, 100)
												: 0;
										const requestPercentage =
											team.rate_limit?.request_max_limit && team.rate_limit.request_max_limit > 0
												? Math.min(((team.rate_limit.request_current_usage ?? 0) / team.rate_limit.request_max_limit) * 100, 100)
												: 0;

										const isExhausted = isBudgetExhausted || isRateLimitExhausted;

										return (
											<TableRow
												key={team.id}
												data-testid={`team-row-${team.name}`}
												className={cn(
													"group transition-colors",
													isExhausted && "bg-red-500/10 border-l-4 border-l-red-500 hover:bg-red-500/15 dark:bg-red-950/20",
												)}
											>
												<TableCell className="w-[48px]">
													<Checkbox
														checked={selectedIds.has(team.id)}
														onCheckedChange={(checked) => toggleSelectTeam(team.id, checked === true)}
														aria-label={`Select ${team.name}`}
														data-testid={`team-select-checkbox-${team.id}`}
													/>
												</TableCell>
												<TableCell className="max-w-[200px] py-4">
													<div className="flex flex-col gap-2">
														<span className="truncate font-medium">{team.name}</span>
														{isBudgetExhausted ? (
															<Badge variant="destructive" className="w-fit text-xs bg-red-600 hover:bg-red-700 text-white font-medium">
																Budget Exhausted
															</Badge>
														) : isExhausted ? (
															<Badge variant="destructive" className="w-fit text-xs">
																Rate Limit Reached
															</Badge>
														) : null}
													</div>
												</TableCell>
												<TableCell data-testid={`team-row-${team.name}-customer`}>
													<div className="flex items-center gap-2">
														<Badge variant={team.customer_id ? "secondary" : "outline"}>{customerName}</Badge>
													</div>
												</TableCell>
												<TableCell className="min-w-[180px]">
													{teamBudgets.length > 0 ? (
														<div className="space-y-2.5">
															{teamBudgets.map((b) => {
																const usage = b.current_usage ?? 0;
																const budgetPercentage = b.max_limit > 0 ? Math.min((usage / b.max_limit) * 100, 100) : 0;
																const isExhausted = b.max_limit > 0 && usage >= b.max_limit;
																return (
																	<Tooltip key={b.id}>
																		<TooltipTrigger asChild>
																			<div className="space-y-1.5">
																				<div className="flex items-center justify-between gap-4">
																					<span className="font-medium">{formatCurrency(b.max_limit)}</span>
																					<span className="text-muted-foreground text-xs">{formatResetDuration(b.reset_duration)}</span>
																				</div>
																				<Progress
																					value={budgetPercentage}
																					className={cn(
																						"bg-muted/70 dark:bg-muted/30 h-1.5",
																						isExhausted
																							? "[&>div]:bg-red-500/70"
																							: budgetPercentage > 80
																								? "[&>div]:bg-amber-500/70"
																								: "[&>div]:bg-emerald-500/70",
																					)}
																				/>
																			</div>
																		</TooltipTrigger>
																		<TooltipContent>
																			<p className="font-medium">
																				{formatCurrency(b.current_usage)} / {formatCurrency(b.max_limit)}
																			</p>
																			<p className="text-primary-foreground/80 text-xs">Resets {formatResetDuration(b.reset_duration)}</p>
																		</TooltipContent>
																	</Tooltip>
																);
															})}
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell className="min-w-[180px]">
													{team.rate_limit ? (
														<div className="space-y-2.5">
															{team.rate_limit.token_max_limit && (
																<Tooltip>
																	<TooltipTrigger asChild>
																		<div className="space-y-1.5">
																			<div className="flex items-center justify-between gap-4 text-xs">
																				<span className="font-medium">{team.rate_limit.token_max_limit.toLocaleString()} tokens</span>
																				<span className="text-muted-foreground">
																					{formatResetDuration(team.rate_limit.token_reset_duration || "1h")}
																				</span>
																			</div>
																			<Progress
																				value={tokenPercentage}
																				className={cn(
																					"bg-muted/70 dark:bg-muted/30 h-1",
																					isTokenLimitExhausted
																						? "[&>div]:bg-red-500/70"
																						: tokenPercentage > 80
																							? "[&>div]:bg-amber-500/70"
																							: "[&>div]:bg-emerald-500/70",
																				)}
																			/>
																		</div>
																	</TooltipTrigger>
																	<TooltipContent>
																		<p className="font-medium">
																			{(team.rate_limit.token_current_usage ?? 0).toLocaleString()} /{" "}
																			{(team.rate_limit.token_max_limit ?? 0).toLocaleString()} tokens
																		</p>
																		<p className="text-primary-foreground/80 text-xs">
																			Resets {formatResetDuration(team.rate_limit.token_reset_duration || "1h")}
																		</p>
																	</TooltipContent>
																</Tooltip>
															)}
															{team.rate_limit.request_max_limit && (
																<Tooltip>
																	<TooltipTrigger asChild>
																		<div className="space-y-1.5">
																			<div className="flex items-center justify-between gap-4 text-xs">
																				<span className="font-medium">{team.rate_limit.request_max_limit.toLocaleString()} req</span>
																				<span className="text-muted-foreground">
																					{formatResetDuration(team.rate_limit.request_reset_duration || "1h")}
																				</span>
																			</div>
																			<Progress
																				value={requestPercentage}
																				className={cn(
																					"bg-muted/70 dark:bg-muted/30 h-1",
																					isRequestLimitExhausted
																						? "[&>div]:bg-red-500/70"
																						: requestPercentage > 80
																							? "[&>div]:bg-amber-500/70"
																							: "[&>div]:bg-emerald-500/70",
																				)}
																			/>
																		</div>
																	</TooltipTrigger>
																	<TooltipContent>
																		<p className="font-medium">
																			{(team.rate_limit.request_current_usage ?? 0).toLocaleString()} /{" "}
																			{(team.rate_limit.request_max_limit ?? 0).toLocaleString()} requests
																		</p>
																		<p className="text-primary-foreground/80 text-xs">
																			Resets {formatResetDuration(team.rate_limit.request_reset_duration || "1h")}
																		</p>
																	</TooltipContent>
																</Tooltip>
															)}
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell>
													{vks.length > 0 ? (
														<div className="flex items-center gap-2">
															<Tooltip>
																<TooltipTrigger>
																	<Badge variant="outline" className="text-xs">
																		{vks.length} {vks.length === 1 ? "key" : "keys"}
																	</Badge>
																</TooltipTrigger>
																<TooltipContent>{vks.map(describeTeamVirtualKey).join(", ")}</TooltipContent>
															</Tooltip>
														</div>
													) : (
														<span className="text-muted-foreground text-sm">-</span>
													)}
												</TableCell>
												<TableCell
													className={`group-hover:bg-muted dark:bg-card dark:group-hover:bg-muted sticky right-0 z-10 bg-white text-right ${PIN_SHADOW_RIGHT}`}
												>
													<TeamActionsMenu
														team={team}
														hasUpdateAccess={hasUpdateAccess}
														hasDeleteAccess={hasDeleteAccess}
														isDeleting={isDeleting}
														onEdit={handleEditTeam}
														onDelete={handleDelete}
													/>
												</TableCell>
											</TableRow>
										);
									})
								)}
							</TableBody>
						</Table>
					</div>

					{/* Standard DataTable Pagination Footer */}
					<DataTablePagination
						offset={offset}
						limit={limit}
						totalCount={totalCount}
						onOffsetChange={onOffsetChange}
						onLimitChange={onLimitChange}
						itemLabel="teams"
						perPageLabel="Teams per page"
						dataTestId="teams-pagination"
					/>
				</div>
			</TooltipProvider>
		</>
	);
}