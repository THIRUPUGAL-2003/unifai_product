import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage } from "@/lib/store";
import {
	useCreateFeatureFlagMutation,
	useDeleteFeatureFlagMutation,
	useListFeatureFlagsQuery,
	useUpdateFeatureFlagMutation,
} from "@/lib/store/apis/featureFlagsApi";
import type { FeatureFlagStatus } from "@/lib/types/featureFlag";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { Crown, Loader2, Lock, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { DataTablePagination } from "@/components/table/dataTablePagination";

const FLAG_ID_REGEX = /^[a-z0-9]+([._-][a-z0-9]+)*$/;

export default function FeatureFlagsView() {
	const hasUpdateAccess = useRbac(RbacResource.FeatureFlags, RbacOperation.Update);
	const { data, isLoading, isError, error } = useListFeatureFlagsQuery();
	const [updateFeatureFlag] = useUpdateFeatureFlagMutation();
	const [createFeatureFlag, { isLoading: isCreating }] = useCreateFeatureFlagMutation();
	const [deleteFeatureFlag, { isLoading: isDeleting }] = useDeleteFeatureFlagMutation();
	const [offset, setOffset] = useState(0);
	const [limit, setLimit] = useState(10);

	// Add Flag Dialog State
	const [isAddDialogOpen, setIsAddDialogOpen] = useState(false);
	const [newFlagId, setNewFlagId] = useState("");
	const [newFlagDisplayName, setNewFlagDisplayName] = useState("");
	const [newFlagDescription, setNewFlagDescription] = useState("");
	const [newFlagEnabled, setNewFlagEnabled] = useState(true);
	const [idError, setIdError] = useState("");

	const flags = data?.flags ?? [];
	const pagedFlags = flags.slice(offset, offset + limit);

	async function handleToggle(flag: FeatureFlagStatus, checked: boolean) {
		try {
			await updateFeatureFlag({ id: flag.id, enabled: checked }).unwrap();
			toast.success(`${flag.display_name || flag.id} ${checked ? "enabled" : "disabled"}`);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	}

	async function handleDelete(flag: FeatureFlagStatus) {
		try {
			await deleteFeatureFlag(flag.id).unwrap();
			toast.success(`Feature flag "${flag.display_name || flag.id}" deleted`);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	}

	function handleFlagIdChange(val: string) {
		const lower = val.toLowerCase().trim();
		setNewFlagId(lower);
		if (!lower) {
			setIdError("Flag ID is required");
		} else if (!FLAG_ID_REGEX.test(lower)) {
			setIdError("Must use lowercase letters, numbers, and .-_ separators (e.g. custom.my_feature)");
		} else {
			setIdError("");
		}
	}

	async function handleCreateFlag(e: React.FormEvent) {
		e.preventDefault();
		const trimmedId = newFlagId.trim().toLowerCase();
		if (!trimmedId) {
			setIdError("Flag ID is required");
			return;
		}
		if (!FLAG_ID_REGEX.test(trimmedId)) {
			setIdError("Invalid flag ID format");
			return;
		}

		try {
			await createFeatureFlag({
				id: trimmedId,
				display_name: newFlagDisplayName.trim() || trimmedId,
				description: newFlagDescription.trim(),
				default: false,
				enabled: newFlagEnabled,
			}).unwrap();

			toast.success(`Feature flag "${newFlagDisplayName || trimmedId}" created successfully`);
			setIsAddDialogOpen(false);
			setNewFlagId("");
			setNewFlagDisplayName("");
			setNewFlagDescription("");
			setNewFlagEnabled(true);
			setIdError("");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	}

	return (
		<div className="w-full space-y-4">
			<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
				<div>
					<h2 className="text-lg font-semibold tracking-tight">Feature Flags</h2>
					<p className="text-muted-foreground text-sm">
						These switches change the running product on the next request. Section flags hide that menu and
						reject its APIs. Behavior flags turn off prompt guard, file scan, or the response cache without
						deleting the saved settings. Values pinned in <code className="text-xs">config.json</code> or Helm
						show as locked.
					</p>
				</div>
				<div>
					<Dialog open={isAddDialogOpen} onOpenChange={setIsAddDialogOpen}>
						<DialogTrigger asChild>
							<Button size="sm" disabled={!hasUpdateAccess} className="whitespace-nowrap">
								<Plus className="mr-2 size-4" />
								Add Flag
							</Button>
						</DialogTrigger>
						<DialogContent className="sm:max-w-md">
							<form onSubmit={handleCreateFlag} className="space-y-4">
								<DialogHeader>
									<DialogTitle>Add Feature Flag</DialogTitle>
									<DialogDescription>
										Register a new custom feature flag for this workspace.
									</DialogDescription>
								</DialogHeader>

								<div className="space-y-3">
									<div className="space-y-1">
										<Label htmlFor="flag-id" className="text-xs font-semibold">
											Flag ID <span className="text-destructive">*</span>
										</Label>
										<Input
											id="flag-id"
											placeholder="e.g. custom.experimental_feature"
											value={newFlagId}
											onChange={(e) => handleFlagIdChange(e.target.value)}
											className="font-mono text-sm"
										/>
										{idError ? (
											<p className="text-destructive text-xs">{idError}</p>
										) : (
											<p className="text-muted-foreground text-xs">
												Lowercase letters, digits, and [._-] separators.
											</p>
										)}
									</div>

									<div className="space-y-1">
										<Label htmlFor="flag-display-name" className="text-xs font-semibold">
											Display Name
										</Label>
										<Input
											id="flag-display-name"
											placeholder="e.g. Experimental Feature"
											value={newFlagDisplayName}
											onChange={(e) => setNewFlagDisplayName(e.target.value)}
											className="text-sm"
										/>
									</div>

									<div className="space-y-1">
										<Label htmlFor="flag-description" className="text-xs font-semibold">
											Description
										</Label>
										<Textarea
											id="flag-description"
											placeholder="Explain what this feature flag gates or controls..."
											value={newFlagDescription}
											onChange={(e) => setNewFlagDescription(e.target.value)}
											className="text-sm min-h-[70px]"
										/>
									</div>

									<div className="flex items-center justify-between rounded-sm border p-3">
										<div className="space-y-0.5">
											<Label htmlFor="flag-enabled" className="text-xs font-semibold">
												Initial State
											</Label>
											<p className="text-muted-foreground text-xs">
												Enable this feature immediately upon creation.
											</p>
										</div>
										<Switch
											id="flag-enabled"
											checked={newFlagEnabled}
											onCheckedChange={setNewFlagEnabled}
										/>
									</div>
								</div>

								<DialogFooter className="gap-2 sm:gap-0">
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => setIsAddDialogOpen(false)}
										disabled={isCreating}
									>
										Cancel
									</Button>
									<Button
										type="submit"
										size="sm"
										disabled={isCreating || !newFlagId.trim() || Boolean(idError)}
									>
										{isCreating ? <Loader2 className="mr-2 size-4 animate-spin" /> : null}
										Create Flag
									</Button>
								</DialogFooter>
							</form>
						</DialogContent>
					</Dialog>
				</div>
			</div>

			{isLoading && <p className="text-muted-foreground text-sm">Loading feature flags...</p>}
			{isError && <p className="text-sm text-red-500">Failed to load feature flags: {getErrorMessage(error)}</p>}

			{!isLoading && !isError && (
				<div className="overflow-auto rounded-sm border">
					<Table data-testid="feature-flags-table">
						<TableHeader>
							<TableRow className="bg-muted/50">
								<TableHead className="font-semibold">Flag</TableHead>
								<TableHead className="w-px text-right font-semibold">Enabled</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{flags.length === 0 ? (
								<TableRow data-testid="feature-flags-table-empty-state">
									<TableCell colSpan={2} className="h-24 text-center">
										<span className="text-muted-foreground text-sm">
											No feature flags found. Flags are declared in code via <code className="text-xs">featureflags.Register(...)</code>.
										</span>
									</TableCell>
								</TableRow>
							) : (
								pagedFlags.map((flag) => (
									<FeatureFlagRow
										key={flag.id}
										flag={flag}
										canUpdate={hasUpdateAccess}
										onToggle={handleToggle}
										onDelete={handleDelete}
										isDeleting={isDeleting}
									/>
								))
							)}
						</TableBody>
					</Table>
					<DataTablePagination
						offset={offset}
						limit={limit}
						totalCount={flags.length}
						onOffsetChange={setOffset}
						onLimitChange={(newLimit) => {
							setLimit(newLimit);
							setOffset(0);
						}}
						itemLabel="feature flags"
						perPageLabel="Flags per page"
						dataTestId="feature-flags-pagination"
					/>
				</div>
			)}
		</div>
	);
}

interface FeatureFlagRowProps {
	flag: FeatureFlagStatus;
	canUpdate: boolean;
	onToggle: (flag: FeatureFlagStatus, checked: boolean) => Promise<void>;
	onDelete: (flag: FeatureFlagStatus) => Promise<void>;
	isDeleting: boolean;
}

function FeatureFlagRow({ flag, canUpdate, onToggle, onDelete, isDeleting }: FeatureFlagRowProps) {
	const disabled = flag.locked || !flag.registered || !canUpdate;
	const isCustomOrUnregistered = !flag.locked && (!flag.registered || flag.source === "db");
	// Fall back to id when display_name is empty so unregistered orphans
	// still render something readable in the primary slot.
	const primaryLabel = flag.display_name || flag.id;

	return (
		<TableRow className="group hover:bg-muted/50 transition-colors">
			<TableCell className="align-top">
				<div className="flex flex-col gap-1">
					<div className="flex flex-wrap items-center gap-2">
						<span className="text-sm font-medium">{primaryLabel}</span>
						{flag.display_name && <span className="text-muted-foreground font-mono text-xs">{flag.id}</span>}
						<SourceBadge source={flag.source} />
						{flag.enterprise_only && <EnterpriseBadge />}
						{flag.locked && !flag.enterprise_only && <LockedBadge />}
						{!flag.registered && <UnregisteredBadge />}
					</div>
					{flag.description && <p className="text-muted-foreground text-sm">{flag.description}</p>}
					{!flag.registered && (
						<p className="text-muted-foreground text-xs">
							No code currently reads this flag. The override is stored but inert until a Register() call is added.
						</p>
					)}
				</div>
			</TableCell>
			<TableCell className="w-px text-right align-top">
				<div className="flex items-center justify-end gap-2">
					<Switch
						data-testid={`feature-flag-toggle-${flag.id}`}
						size="md"
						checked={flag.enabled}
						disabled={disabled}
						onAsyncCheckedChange={(checked) => onToggle(flag, checked)}
					/>
					{isCustomOrUnregistered && canUpdate && (
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									className="size-7 text-muted-foreground hover:text-destructive"
									disabled={isDeleting}
									onClick={() => void onDelete(flag)}
								>
									<Trash2 className="size-3.5" />
								</Button>
							</TooltipTrigger>
							<TooltipContent>Delete custom flag or override</TooltipContent>
						</Tooltip>
					)}
				</div>
			</TableCell>
		</TableRow>
	);
}


function SourceBadge({ source }: { source: FeatureFlagStatus["source"] }) {
	return (
		<Badge variant="outline" className="text-xs capitalize">
			{source}
		</Badge>
	);
}

function LockedBadge() {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge variant="secondary" className="flex items-center gap-1 text-xs">
					<Lock className="size-3" />
					Locked
				</Badge>
			</TooltipTrigger>
			<TooltipContent>Value is pinned by config.json or Helm; edit your config to change it.</TooltipContent>
		</Tooltip>
	);
}

function EnterpriseBadge() {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge variant="secondary" className="flex items-center gap-1 text-xs">
					<Crown className="size-3" />
					Enterprise
				</Badge>
			</TooltipTrigger>
			<TooltipContent>This flag gates an enterprise-only feature. Upgrade to enable it.</TooltipContent>
		</Tooltip>
	);
}

function UnregisteredBadge() {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge variant="destructive" className="text-xs">
					Unregistered
				</Badge>
			</TooltipTrigger>
			<TooltipContent>This id has no code registration. Restore the Register() call or clean up the stale value.</TooltipContent>
		</Tooltip>
	);
}