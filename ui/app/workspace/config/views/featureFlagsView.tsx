import { QueryErrorBanner } from "@/components/queryErrorBanner";
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
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
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
import { AlertTriangle, Crown, Loader2, Lock, Pencil, Plus, Sparkles, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { DataTablePagination } from "@/components/table/dataTablePagination";
import { WORKSPACE_SECTIONS } from "@/lib/constants/workspaceSections";

const FLAG_ID_REGEX = /^[a-z0-9]+([._-][a-z0-9]+)*$/;

const FEATURE_CATEGORIES = [
	"Sections & Menus",
	"Sub-Features & Pages",
	"Security & Guardrails",
	"Performance & Caching",
	"Integrations & Tools",
] as const;

interface FeatureFlagPreset {
	id: string;
	displayName: string;
	description: string;
	category: (typeof FEATURE_CATEGORIES)[number];
}

const DYNAMIC_SECTION_PRESETS: FeatureFlagPreset[] = WORKSPACE_SECTIONS.flatMap((sec) => {
	const list: FeatureFlagPreset[] = [
		{
			id: `section.${sec.key}`,
			displayName: `${sec.label} (Full Module)`,
			description: `Enable or gate the entire ${sec.label} section and its APIs.`,
			category: "Sections & Menus",
		},
	];
	if (sec.items) {
		for (const item of sec.items) {
			list.push({
				id: `section.${sec.key}.${item.key}`,
				displayName: `${sec.label}: ${item.label}`,
				description: `Enable or gate ${item.label} under ${sec.label} (${item.path}).`,
				category: "Sub-Features & Pages",
			});
		}
	}
	return list;
});

const PLATFORM_PRESETS: FeatureFlagPreset[] = [
	// Security & Guardrails
	{ id: "browser.prompt_guard", displayName: "Browser prompt guard", description: "Evaluate prompts from Guard agents. Off: prompts are forwarded and not stored in Prompt Logs.", category: "Security & Guardrails" },
	{ id: "browser.file_scan", displayName: "Browser file scan", description: "Scan file and voice uploads from Guard agents. Off: uploads are allowed and not stored.", category: "Security & Guardrails" },
	{ id: "guardrails.pii_masking", displayName: "PII & Secret Redaction", description: "Automatically detect and redact sensitive credentials, API keys, and personal identifiable information.", category: "Security & Guardrails" },
	{ id: "guardrails.prompt_injection", displayName: "Jailbreak & Prompt Injection Defense", description: "Block adversarial jailbreak attempts and system prompt extraction attacks in real-time.", category: "Security & Guardrails" },
	{ id: "guardrails.toxicity", displayName: "Toxicity & Hazard Screening", description: "Screen user inputs and model outputs for harassment, hate speech, and harmful guidance.", category: "Security & Guardrails" },
	{ id: "guardrails.hallucination", displayName: "Factuality & Grounding Validation", description: "Verify model outputs against retrieved context documents to prevent hallucinations.", category: "Security & Guardrails" },

	// Performance & Caching
	{ id: "cache.responses", displayName: "Response cache", description: "Use the Caching settings (direct and semantic) on inference requests. Off: every request goes to the provider.", category: "Performance & Caching" },
	{ id: "routing.complexity", displayName: "Dynamic Complexity Router", description: "Automatically classify prompts by complexity and route to cost-efficient smaller models when suitable.", category: "Performance & Caching" },
	{ id: "routing.fallback_cascade", displayName: "Automated Fallback Cascade", description: "Fail over to secondary LLM providers immediately upon HTTP 429, 500, or network timeouts.", category: "Performance & Caching" },
	{ id: "routing.circuit_breaker", displayName: "Provider Circuit Breaker", description: "Temporarily pause traffic to failing upstream model providers to protect latency and quotas.", category: "Performance & Caching" },

	// Integrations & Tools
	{ id: "mcp.tool_execution", displayName: "MCP Tool Invocation", description: "Enable live MCP tool execution from configured Model Context Protocol servers.", category: "Integrations & Tools" },
	{ id: "mcp.oauth_grants", displayName: "MCP OAuth2 User Consent", description: "Require user consent screens before MCP servers access external OAuth services.", category: "Integrations & Tools" },
	{ id: "observability.metrics_export", displayName: "Prometheus & OpenTelemetry Export", description: "Expose real-time token metrics, latency percentiles, and request counters for monitoring.", category: "Integrations & Tools" },
];

const FEATURE_FLAG_PRESETS: FeatureFlagPreset[] = [...DYNAMIC_SECTION_PRESETS, ...PLATFORM_PRESETS];

export default function FeatureFlagsView() {
	const hasUpdateAccess = useRbac(RbacResource.FeatureFlags, RbacOperation.Update);
	const { data, isLoading, isError, error, refetch } = useListFeatureFlagsQuery();
	const [updateFeatureFlag] = useUpdateFeatureFlagMutation();
	const [createFeatureFlag, { isLoading: isCreating }] = useCreateFeatureFlagMutation();
	const [deleteFeatureFlag, { isLoading: isDeleting }] = useDeleteFeatureFlagMutation();
	const [offset, setOffset] = useState(0);
	const [limit, setLimit] = useState(10);

	// Add Flag Dialog State
	const [isAddDialogOpen, setIsAddDialogOpen] = useState(false);
	const [selectedPreset, setSelectedPreset] = useState("custom");
	const [newFlagId, setNewFlagId] = useState("");
	const [newFlagDisplayName, setNewFlagDisplayName] = useState("");
	const [newFlagDescription, setNewFlagDescription] = useState("");
	const [newFlagEnabled, setNewFlagEnabled] = useState(true);
	const [idError, setIdError] = useState("");
	const [editing, setEditing] = useState<FeatureFlagStatus | null>(null);
	const [editName, setEditName] = useState("");
	const [editDescription, setEditDescription] = useState("");
	const [editEnabled, setEditEnabled] = useState(false);
	const [isSavingEdit, setIsSavingEdit] = useState(false);
	const [flagToDelete, setFlagToDelete] = useState<FeatureFlagStatus | null>(null);

	const flags = data?.flags ?? [];
	const pagedFlags = flags.slice(offset, offset + limit);

	const unaddedPresetsCount = useMemo(() => {
		const existingIds = new Set(flags.map((f) => f.id));
		return FEATURE_FLAG_PRESETS.filter((p) => !existingIds.has(p.id)).length;
	}, [flags]);

	function handleSelectPreset(val: string) {
		setSelectedPreset(val);
		if (val === "custom") {
			setNewFlagId("");
			setNewFlagDisplayName("");
			setNewFlagDescription("");
			setIdError("");
			return;
		}
		const preset = FEATURE_FLAG_PRESETS.find((p) => p.id === val);
		if (preset) {
			setNewFlagId(preset.id);
			setNewFlagDisplayName(preset.displayName);
			setNewFlagDescription(preset.description);
			setIdError("");
		}
	}

	async function handleToggle(flag: FeatureFlagStatus, checked: boolean) {
		try {
			await updateFeatureFlag({ id: flag.id, enabled: checked }).unwrap();
			toast.success(`${flag.display_name || flag.id} ${checked ? "enabled" : "disabled"}`);
		} catch (err) {
			toast.error(getErrorMessage(err) || "Failed to update feature flag.");
		}
	}

	function openEdit(flag: FeatureFlagStatus) {
		setEditing(flag);
		setEditName(flag.display_name || flag.id);
		setEditDescription(flag.description || "");
		setEditEnabled(flag.enabled);
	}

	async function handleSaveEdit(e: React.FormEvent) {
		e.preventDefault();
		if (!editing) return;
		const name = editName.trim();
		if (!name) {
			toast.error("Display name is required.");
			return;
		}
		setIsSavingEdit(true);
		try {
			await updateFeatureFlag({
				id: editing.id,
				enabled: editEnabled,
				display_name: name,
				description: editDescription.trim(),
			}).unwrap();
			toast.success(`Updated "${name}" successfully`);
			setEditing(null);
		} catch (err) {
			toast.error(getErrorMessage(err) || "Failed to update feature flag.");
		} finally {
			setIsSavingEdit(false);
		}
	}

	function onRequestDelete(flag: FeatureFlagStatus) {
		setFlagToDelete(flag);
	}

	async function handleConfirmDelete() {
		if (!flagToDelete) return;
		try {
			await deleteFeatureFlag(flagToDelete.id).unwrap();
			toast.success(`Feature flag "${flagToDelete.display_name || flagToDelete.id}" deleted successfully`);
			setFlagToDelete(null);
		} catch (err) {
			toast.error(getErrorMessage(err) || "Failed to delete feature flag.");
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
			toast.error(getErrorMessage(err) || "Failed to create feature flag.");
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
						<DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
							<form onSubmit={handleCreateFlag} className="flex min-h-0 flex-1 flex-col">
								<DialogHeader className="border-b px-6 py-4">
									<DialogTitle>Add Feature Flag</DialogTitle>
									<DialogDescription>
										Register a new custom feature flag for this workspace.
									</DialogDescription>
								</DialogHeader>

								<div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-6 py-4">
									<div className="space-y-1.5">
										<div className="flex items-center justify-between gap-3">
											<Label htmlFor="feature-preset" className="text-xs font-semibold flex items-center gap-1.5">
												<Sparkles className="size-3.5 text-teal-500" />
												Feature / Section Template
											</Label>
											{unaddedPresetsCount > 0 && (
												<span className="shrink-0 text-[11px] text-teal-600 dark:text-teal-400 font-medium">
													{unaddedPresetsCount} available to add
												</span>
											)}
										</div>
										<Select value={selectedPreset} onValueChange={handleSelectPreset}>
											<SelectTrigger id="feature-preset" className="h-10 w-full text-sm">
												<SelectValue placeholder="Choose a known feature or Custom..." />
											</SelectTrigger>
											<SelectContent className="max-h-72 w-[var(--radix-select-trigger-width)]" position="popper">
												<SelectItem value="custom">
													<span className="font-medium text-muted-foreground">Custom / Enter manually</span>
												</SelectItem>
												{FEATURE_CATEGORIES.map((cat) => {
													const items = FEATURE_FLAG_PRESETS.filter((p) => p.category === cat);
													if (items.length === 0) return null;
													return (
														<SelectGroup key={cat}>
															<SelectLabel className="text-xs font-semibold text-muted-foreground">
																{cat}
															</SelectLabel>
															{items.map((preset) => {
																const isAlreadyAdded = flags.some((f) => f.id === preset.id);
																return (
																	<SelectItem key={preset.id} value={preset.id} className="py-2">
																		<span className="flex min-w-0 items-center gap-2">
																			<span className="truncate">{preset.displayName}</span>
																			<span className="text-muted-foreground shrink-0 font-mono text-[11px]">{preset.id}</span>
																			{isAlreadyAdded ? (
																				<span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[10px]">
																					Added
																				</span>
																			) : null}
																		</span>
																	</SelectItem>
																);
															})}
														</SelectGroup>
													);
												})}
											</SelectContent>
										</Select>
										<p className="text-muted-foreground text-xs">
											Choose a feature from the dropdown to auto-fill its ID, name, and description.
										</p>
									</div>

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

								<DialogFooter className="gap-2 border-t px-6 py-4 sm:gap-2">
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
					<Dialog open={editing !== null} onOpenChange={(open) => { if (!open) setEditing(null); }}>
						<DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
							<form onSubmit={handleSaveEdit} className="flex min-h-0 flex-1 flex-col">
								<DialogHeader className="border-b px-6 py-4">
									<DialogTitle>Edit Feature Flag</DialogTitle>
									<DialogDescription>Update the label, description, and enabled state. The id stays the same.</DialogDescription>
								</DialogHeader>
								<div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-6 py-4">
									<div className="space-y-1">
										<Label htmlFor="edit-flag-id" className="text-xs font-semibold">Flag ID</Label>
										<Input id="edit-flag-id" value={editing?.id ?? ""} disabled className="font-mono text-sm" />
									</div>
									<div className="space-y-1">
										<Label htmlFor="edit-flag-name" className="text-xs font-semibold">Display Name</Label>
										<Input id="edit-flag-name" value={editName} onChange={(e) => setEditName(e.target.value)} className="text-sm" />
									</div>
									<div className="space-y-1">
										<Label htmlFor="edit-flag-description" className="text-xs font-semibold">Description</Label>
										<Textarea
											id="edit-flag-description"
											value={editDescription}
											onChange={(e) => setEditDescription(e.target.value)}
											className="min-h-[70px] text-sm"
										/>
									</div>
									<div className="flex items-center justify-between rounded-sm border p-3">
										<div className="space-y-0.5">
											<Label htmlFor="edit-flag-enabled" className="text-xs font-semibold">Enabled</Label>
											<p className="text-muted-foreground text-xs">Applies on the next request after you save.</p>
										</div>
										<Switch id="edit-flag-enabled" checked={editEnabled} onCheckedChange={setEditEnabled} />
									</div>
								</div>
								<DialogFooter className="gap-2 border-t px-6 py-4 sm:gap-2">
									<Button type="button" variant="outline" size="sm" onClick={() => setEditing(null)} disabled={isSavingEdit}>
										Cancel
									</Button>
									<Button type="submit" size="sm" disabled={isSavingEdit || !editName.trim()}>
										{isSavingEdit ? <Loader2 className="mr-2 size-4 animate-spin" /> : null}
										Save
									</Button>
								</DialogFooter>
							</form>
						</DialogContent>
					</Dialog>
					<AlertDialog open={flagToDelete !== null} onOpenChange={(open) => { if (!open) setFlagToDelete(null); }}>
						<AlertDialogContent>
							<AlertDialogHeader>
								<AlertDialogTitle className="flex items-center gap-2 text-destructive">
									<AlertTriangle className="size-5 shrink-0" />
									Delete Feature Flag
								</AlertDialogTitle>
								<AlertDialogDescription asChild>
									<div className="space-y-3 pt-1 text-sm text-muted-foreground">
										<p>
											Are you sure you want to permanently delete{" "}
											<strong className="text-foreground">{flagToDelete?.display_name || flagToDelete?.id}</strong>{" "}
											(<code className="font-mono text-xs">{flagToDelete?.id}</code>)?
										</p>
										<div className="rounded-md border border-amber-500/20 bg-amber-500/10 p-2.5 text-xs text-amber-700 dark:text-amber-400">
											⚠️ <strong>Warning:</strong> This will permanently delete the flag from the PostgreSQL database and runtime memory. Any services, APIs, or UI routes relying on this flag will immediately fall back to their default code state.
										</div>
									</div>
								</AlertDialogDescription>
							</AlertDialogHeader>
							<AlertDialogFooter className="gap-2 sm:gap-2">
								<AlertDialogCancel disabled={isDeleting} onClick={() => setFlagToDelete(null)}>
									Cancel
								</AlertDialogCancel>
								<AlertDialogAction
									disabled={isDeleting}
									onClick={(e) => {
										e.preventDefault();
										void handleConfirmDelete();
									}}
									className="bg-destructive hover:bg-destructive/90 text-destructive-foreground"
								>
									{isDeleting ? <Loader2 className="mr-2 size-4 animate-spin" /> : <Trash2 className="mr-2 size-4" />}
									Yes, Delete Flag
								</AlertDialogAction>
							</AlertDialogFooter>
						</AlertDialogContent>
					</AlertDialog>
				</div>
			</div>

			{isLoading && <p className="text-muted-foreground text-sm">Loading feature flags...</p>}
			{isError && (
				<div className="flex flex-col gap-2">
					<QueryErrorBanner testId="feature-flags-query-error" message={getErrorMessage(error) || "Failed to load feature flags."} />
					<Button type="button" size="sm" variant="outline" className="w-fit" onClick={() => void refetch()}>
						Retry
					</Button>
				</div>
			)}

			{!isLoading && !isError && (
				<div className="overflow-auto rounded-sm border">
					<Table data-testid="feature-flags-table">
						<TableHeader>
							<TableRow className="bg-muted/50">
								<TableHead className="font-semibold">Flag</TableHead>
								<TableHead className="w-40 text-right font-semibold">Enabled</TableHead>
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
										onEdit={openEdit}
										onDelete={onRequestDelete}
										isDeleting={isDeleting && flagToDelete?.id === flag.id}
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
	onEdit: (flag: FeatureFlagStatus) => void;
	onDelete: (flag: FeatureFlagStatus) => void;
	isDeleting: boolean;
}

function FeatureFlagRow({ flag, canUpdate, onToggle, onEdit, onDelete, isDeleting }: FeatureFlagRowProps) {
	const disabled = flag.locked || !flag.registered || !canUpdate;
	const isCustomOrUnregistered = !flag.locked && (!flag.registered || flag.source === "db");
	// Fall back to id when display_name is empty so unregistered orphans
	// still render something readable in the primary slot.
	const primaryLabel = flag.display_name || flag.id;

	return (
		<TableRow className="group hover:bg-muted/50 transition-colors">
			<TableCell className="align-middle">
				<div className="flex min-w-0 flex-col gap-1 py-1">
					<div className="flex flex-wrap items-center gap-2">
						<span className="text-sm font-medium">{primaryLabel}</span>
						{flag.display_name && <span className="text-muted-foreground font-mono text-xs">{flag.id}</span>}
						<SourceBadge source={flag.source} />
						{flag.enterprise_only && <EnterpriseBadge />}
						{flag.locked && !flag.enterprise_only && <LockedBadge />}
						{!flag.registered && <UnregisteredBadge />}
					</div>
					{flag.description && <p className="text-muted-foreground text-sm leading-snug">{flag.description}</p>}
					{!flag.registered && (
						<p className="text-muted-foreground text-xs">
							No code currently reads this flag. The override is stored but inert until a Register() call is added.
						</p>
					)}
				</div>
			</TableCell>
			<TableCell className="w-40 align-middle">
				<div className="flex items-center justify-end gap-1">
					<div className="flex size-8 shrink-0 items-center justify-center">
						{canUpdate && flag.registered && !flag.locked ? (
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										variant="ghost"
										size="icon"
										className="size-8 text-muted-foreground"
										onClick={() => onEdit(flag)}
										aria-label={`Edit ${primaryLabel}`}
									>
										<Pencil className="size-3.5" />
									</Button>
								</TooltipTrigger>
								<TooltipContent>Edit flag</TooltipContent>
							</Tooltip>
						) : null}
					</div>
					<Switch
						data-testid={`feature-flag-toggle-${flag.id}`}
						size="md"
						checked={flag.enabled}
						disabled={disabled}
						onAsyncCheckedChange={(checked) => onToggle(flag, checked)}
					/>
					<div className="flex size-8 shrink-0 items-center justify-center">
						{!flag.locked && canUpdate ? (
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										variant="ghost"
										size="icon"
										className="size-8 text-muted-foreground hover:text-destructive"
										disabled={isDeleting}
										onClick={() => onDelete(flag)}
										aria-label={`Delete ${primaryLabel}`}
									>
										<Trash2 className="size-3.5" />
									</Button>
								</TooltipTrigger>
								<TooltipContent>Delete feature flag</TooltipContent>
							</Tooltip>
						) : null}
					</div>
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