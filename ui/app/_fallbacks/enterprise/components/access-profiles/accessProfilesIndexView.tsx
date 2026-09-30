import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ComboboxSelect } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ModelMultiselect } from "@/components/ui/modelMultiselect";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getProviderLabel } from "@/lib/constants/logs";
import { getErrorMessage, useGetMCPClientsQuery, useGetProvidersQuery, useGetVirtualKeysQuery } from "@/lib/store";
import {
	useActivateAccessProfileMutation,
	useCloneAccessProfileMutation,
	useCreateAccessProfileMutation,
	useDeleteAccessProfileMutation,
	useGetAccessProfilesQuery,
	useUpdateAccessProfileMutation,
} from "@enterprise/lib/store/apis/accessProfileApi";
import { AccessProfile } from "@enterprise/lib/types/workspace";
import { Copy, IdCard, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

type ProfileFormState = {
	name: string;
	description: string;
	tags: string;
	providerName: string;
	allowedModels: string[];
	selectedVkIds: string[];
	selectedMcpNames: string[];
	budgetMax: string;
	budgetReset: string;
	requestMax: string;
	requestReset: string;
	tokenMax: string;
	tokenReset: string;
};

const emptyForm = (): ProfileFormState => ({
	name: "",
	description: "",
	tags: "",
	providerName: "",
	allowedModels: [],
	selectedVkIds: [],
	selectedMcpNames: [],
	budgetMax: "",
	budgetReset: "1M",
	requestMax: "",
	requestReset: "1m",
	tokenMax: "",
	tokenReset: "1m",
});

function buildPayload(form: ProfileFormState) {
	const budgets =
		form.budgetMax.trim() !== ""
			? [
					{
						max_limit: Number(form.budgetMax),
						reset_duration: form.budgetReset.trim() || "1M",
					},
				]
			: [];
	const rate_limit =
		form.requestMax.trim() || form.tokenMax.trim()
			? {
					...(form.requestMax.trim()
						? {
								request_max_limit: Number(form.requestMax),
								request_reset_duration: form.requestReset.trim() || "1m",
							}
						: {}),
					...(form.tokenMax.trim()
						? {
								token_max_limit: Number(form.tokenMax),
								token_reset_duration: form.tokenReset.trim() || "1m",
							}
						: {}),
				}
			: undefined;

	const allowAll =
		form.allowedModels.length === 0 || form.allowedModels.includes("*");
	const allowedModels = allowAll ? [] : form.allowedModels.filter((m) => m && m !== "*");

	return {
		name: form.name,
		description: form.description,
		tags: form.tags
			.split(",")
			.map((tag) => tag.trim())
			.filter(Boolean),
		provider_configs: form.providerName
			? [
					{
						provider_name: form.providerName.trim(),
						all_models_allowed: allowAll,
						allowed_models: allowedModels,
					},
				]
			: [],
		virtual_key_ids: form.selectedVkIds,
		mcp_servers: form.selectedMcpNames.map((name) => ({
			mcp_client_name: name,
			tools_to_execute: ["*"],
		})),
		budgets,
		rate_limit,
	};
}

export default function AccessProfilesIndexView() {
	const [search, setSearch] = useState("");
	const [open, setOpen] = useState(false);
	const [editOpen, setEditOpen] = useState(false);
	const [editing, setEditing] = useState<AccessProfile | null>(null);
	const [form, setForm] = useState<ProfileFormState>(emptyForm);

	const { data, isLoading } = useGetAccessProfilesQuery({ search: search || undefined });
	const { data: providersData = [] } = useGetProvidersQuery();
	const { data: vkData } = useGetVirtualKeysQuery({ limit: 200, offset: 0 });
	const { data: mcpClientsData } = useGetMCPClientsQuery({ limit: 200, offset: 0 });
	const virtualKeys = vkData?.virtual_keys || [];
	const mcpClients = mcpClientsData?.clients || [];
	const [createProfile] = useCreateAccessProfileMutation();
	const [activateProfile] = useActivateAccessProfileMutation();
	const [cloneProfile] = useCloneAccessProfileMutation();
	const [deleteProfile] = useDeleteAccessProfileMutation();
	const [updateProfile] = useUpdateAccessProfileMutation();

	const profiles = data?.access_profiles || [];

	const providerOptions = useMemo(() => {
		const names = new Set<string>([
			...providersData.map((p) => p.name),
			...(form.providerName ? [form.providerName] : []),
		]);
		return Array.from(names)
			.filter(Boolean)
			.sort((a, b) => a.localeCompare(b))
			.map((name) => ({
				value: name,
				label: getProviderLabel(name) || name,
			}));
	}, [providersData, form.providerName]);

	const resetForm = () => {
		setForm(emptyForm());
		setEditing(null);
	};

	const openEdit = (profile: AccessProfile) => {
		setEditing(profile);
		const cfg = profile.provider_configs?.[0];
		const budget = profile.budgets?.[0];
		const rl = profile.rate_limit;
		const mcpNames = (profile.mcp_servers || [])
			.map((server) => String(server.mcp_client_name || server.name || ""))
			.filter(Boolean);
		const allAllowed = cfg?.all_models_allowed !== false && !(cfg?.allowed_models?.length);
		setForm({
			name: profile.name,
			description: profile.description || "",
			tags: (profile.tags || []).join(", "),
			providerName: cfg?.provider_name || "",
			allowedModels: allAllowed ? [] : cfg?.allowed_models || [],
			selectedVkIds: profile.virtual_key_ids || [],
			selectedMcpNames: mcpNames,
			budgetMax: budget?.max_limit != null ? String(budget.max_limit) : "",
			budgetReset: budget?.reset_duration || "1M",
			requestMax: rl?.request_max_limit != null ? String(rl.request_max_limit) : "",
			requestReset: rl?.request_reset_duration || "1m",
			tokenMax: rl?.token_max_limit != null ? String(rl.token_max_limit) : "",
			tokenReset: rl?.token_reset_duration || "1m",
		});
		setEditOpen(true);
	};

	const toggleVk = (id: string) => {
		setForm((prev) => ({
			...prev,
			selectedVkIds: prev.selectedVkIds.includes(id) ? prev.selectedVkIds.filter((x) => x !== id) : [...prev.selectedVkIds, id],
		}));
	};

	const toggleMcp = (name: string) => {
		setForm((prev) => ({
			...prev,
			selectedMcpNames: prev.selectedMcpNames.includes(name)
				? prev.selectedMcpNames.filter((x) => x !== name)
				: [...prev.selectedMcpNames, name],
		}));
	};

	const create = async () => {
		if (!form.name.trim()) {
			toast.error("Name is required");
			return;
		}
		try {
			await createProfile({ ...buildPayload(form), is_active: true }).unwrap();
			toast.success("Access profile created and applied to selected virtual keys");
			setOpen(false);
			resetForm();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const saveEdit = async () => {
		if (!editing) return;
		if (!form.name.trim()) {
			toast.error("Name is required");
			return;
		}
		try {
			await updateProfile({ id: editing.id, updates: buildPayload(form) }).unwrap();
			toast.success("Access profile updated and applied to selected virtual keys");
			setEditOpen(false);
			resetForm();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const formFields = (
		<div className="max-h-[70vh] space-y-3 overflow-y-auto py-2">
			<div className="space-y-1">
				<Label>Name</Label>
				<Input value={form.name} onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))} />
			</div>
			<div className="space-y-1">
				<Label>Description</Label>
				<Input value={form.description} onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))} />
			</div>
			<div className="space-y-1">
				<Label>Tags (comma separated)</Label>
				<Input value={form.tags} onChange={(e) => setForm((p) => ({ ...p, tags: e.target.value }))} />
			</div>
			<div className="space-y-1">
				<Label>Provider</Label>
				<ComboboxSelect
					options={providerOptions}
					value={form.providerName || null}
					onValueChange={(value) =>
						setForm((prev) => ({
							...prev,
							providerName: value ?? "",
							allowedModels: prev.providerName !== (value ?? "") ? [] : prev.allowedModels,
						}))
					}
					placeholder="Select provider..."
					noPortal
				/>
			</div>
			<div className="space-y-1">
				<Label>Allowed models (blank / All = all)</Label>
				<ModelMultiselect
					provider={form.providerName || undefined}
					value={form.allowedModels}
					onChange={(models) => setForm((prev) => ({ ...prev, allowedModels: models }))}
					unfiltered
					allowAllOption
					placeholder={!form.providerName ? "Select a provider first" : "Search models..."}
					disabled={!form.providerName}
					menuPosition="absolute"
					clearable
				/>
			</div>
			<div className="grid grid-cols-2 gap-2">
				<div className="space-y-1">
					<Label>Budget max (optional)</Label>
					<Input
						type="number"
						value={form.budgetMax}
						onChange={(e) => setForm((p) => ({ ...p, budgetMax: e.target.value }))}
						placeholder="100"
					/>
				</div>
				<div className="space-y-1">
					<Label>Budget reset</Label>
					<Input value={form.budgetReset} onChange={(e) => setForm((p) => ({ ...p, budgetReset: e.target.value }))} placeholder="1M" />
				</div>
			</div>
			<div className="grid grid-cols-2 gap-2">
				<div className="space-y-1">
					<Label>Request max / min (optional)</Label>
					<Input
						type="number"
						value={form.requestMax}
						onChange={(e) => setForm((p) => ({ ...p, requestMax: e.target.value }))}
						placeholder="60"
					/>
				</div>
				<div className="space-y-1">
					<Label>Request reset</Label>
					<Input
						value={form.requestReset}
						onChange={(e) => setForm((p) => ({ ...p, requestReset: e.target.value }))}
						placeholder="1m"
					/>
				</div>
			</div>
			<div className="grid grid-cols-2 gap-2">
				<div className="space-y-1">
					<Label>Token max (optional)</Label>
					<Input
						type="number"
						value={form.tokenMax}
						onChange={(e) => setForm((p) => ({ ...p, tokenMax: e.target.value }))}
						placeholder="100000"
					/>
				</div>
				<div className="space-y-1">
					<Label>Token reset</Label>
					<Input value={form.tokenReset} onChange={(e) => setForm((p) => ({ ...p, tokenReset: e.target.value }))} placeholder="1m" />
				</div>
			</div>
			<div className="space-y-1">
				<Label>MCP servers to grant</Label>
				<p className="text-muted-foreground text-xs">Selected servers are added to the chosen virtual keys on save.</p>
				<div className="border-input max-h-36 space-y-1 overflow-y-auto rounded-md border p-2">
					{mcpClients.length === 0 ? (
						<p className="text-muted-foreground text-xs">No MCP servers found.</p>
					) : (
						mcpClients.map((client) => {
							const name = client.config?.name || "";
							if (!name) return null;
							return (
								<label key={name} className="flex cursor-pointer items-center gap-2 text-sm">
									<input type="checkbox" checked={form.selectedMcpNames.includes(name)} onChange={() => toggleMcp(name)} />
									<span className="truncate">{name}</span>
								</label>
							);
						})
					)}
				</div>
			</div>
			<div className="space-y-1">
				<Label>Virtual keys to apply template</Label>
				<p className="text-muted-foreground text-xs">
					Selected keys receive provider, MCP, budget, and rate-limit settings on save.
				</p>
				<div className="border-input max-h-36 space-y-1 overflow-y-auto rounded-md border p-2">
					{virtualKeys.length === 0 ? (
						<p className="text-muted-foreground text-xs">No virtual keys found.</p>
					) : (
						virtualKeys.map((vk) => (
							<label key={vk.id} className="flex cursor-pointer items-center gap-2 text-sm">
								<input type="checkbox" checked={form.selectedVkIds.includes(vk.id)} onChange={() => toggleVk(vk.id)} />
								<span className="truncate">{vk.name}</span>
							</label>
						))
					)}
				</div>
			</div>
		</div>
	);

	return (
		<div className="flex h-full w-full flex-col gap-4">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-semibold">
						<IdCard className="h-6 w-6" />
						Access Profiles
					</h1>
					<p className="text-muted-foreground text-sm">
						Reusable provider, MCP, model, budget, and rate-limit templates applied to selected virtual keys.
					</p>
				</div>
				<Button data-testid="access-profiles-create" onClick={() => setOpen(true)}>
					<Plus className="h-4 w-4" />
					New profile
				</Button>
			</div>
			<Input className="max-w-sm" placeholder="Search profiles…" value={search} onChange={(e) => setSearch(e.target.value)} />
			{isLoading ? (
				<p className="text-muted-foreground text-sm">Loading profiles…</p>
			) : profiles.length === 0 ? (
				<div className="rounded-xl border border-dashed p-10 text-center">
					<p className="font-medium">No access profiles</p>
					<p className="text-muted-foreground mt-1 text-sm">
						Create a template, pick virtual keys, then activate — those keys inherit providers, budgets, and MCP from the profile.
						Assign a key to a user under Users so the profile takes effect for that member.
					</p>
				</div>
			) : (
				<TableView
					profiles={profiles}
					onEdit={openEdit}
					onToggle={async (profile) => {
						try {
							await activateProfile({ id: profile.id, activate: !profile.is_active }).unwrap();
						} catch (err) {
							toast.error(getErrorMessage(err));
						}
					}}
					onClone={async (id) => {
						try {
							await cloneProfile(id).unwrap();
							toast.success("Profile cloned");
						} catch (err) {
							toast.error(getErrorMessage(err));
						}
					}}
					onDelete={async (id) => {
						try {
							await deleteProfile(id).unwrap();
							toast.success("Profile deleted");
						} catch (err) {
							toast.error(getErrorMessage(err));
						}
					}}
				/>
			)}

			<Dialog open={open} onOpenChange={setOpen}>
				<DialogContent className="sm:max-w-lg">
					<DialogHeader>
						<DialogTitle>Create access profile</DialogTitle>
					</DialogHeader>
					{formFields}
					<DialogFooter>
						<Button variant="outline" onClick={() => setOpen(false)}>
							Cancel
						</Button>
						<Button onClick={() => void create()}>Create</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<Dialog
				open={editOpen}
				onOpenChange={(v) => {
					setEditOpen(v);
					if (!v) resetForm();
				}}
			>
				<DialogContent className="sm:max-w-lg">
					<DialogHeader>
						<DialogTitle>Edit access profile</DialogTitle>
					</DialogHeader>
					{formFields}
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => {
								setEditOpen(false);
								resetForm();
							}}
						>
							Cancel
						</Button>
						<Button onClick={() => void saveEdit()}>Save changes</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}

function TableView({
	profiles,
	onEdit,
	onToggle,
	onClone,
	onDelete,
}: {
	profiles: AccessProfile[];
	onEdit: (profile: AccessProfile) => void;
	onToggle: (profile: AccessProfile) => void;
	onClone: (id: number) => void;
	onDelete: (id: number) => void;
}) {
	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead>Name</TableHead>
					<TableHead>Providers</TableHead>
					<TableHead>MCP</TableHead>
					<TableHead>Virtual keys</TableHead>
					<TableHead>Active</TableHead>
					<TableHead>Version</TableHead>
					<TableHead className="text-right">Actions</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{profiles.map((profile) => (
					<TableRow key={profile.id}>
						<TableCell>
							<div className="font-medium">{profile.name}</div>
							<div className="text-muted-foreground text-xs">{profile.description}</div>
						</TableCell>
						<TableCell className="text-xs">{profile.provider_configs?.map((cfg) => cfg.provider_name).join(", ") || "—"}</TableCell>
						<TableCell className="text-xs">
							{profile.mcp_servers?.length
								? profile.mcp_servers
										.map((s) => String(s.mcp_client_name || s.name || ""))
										.filter(Boolean)
										.join(", ") || `${profile.mcp_servers.length} server(s)`
								: "—"}
						</TableCell>
						<TableCell className="text-xs">
							{profile.virtual_key_ids?.length ? `${profile.virtual_key_ids.length} key(s)` : "—"}
						</TableCell>
						<TableCell>
							<Switch checked={profile.is_active} onCheckedChange={() => onToggle(profile)} />
						</TableCell>
						<TableCell>
							<Badge variant="secondary">v{profile.version}</Badge>
						</TableCell>
						<TableCell className="text-right">
							<Button size="icon" variant="ghost" onClick={() => onEdit(profile)} title="Edit">
								<Pencil className="h-4 w-4" />
							</Button>
							<Button size="icon" variant="ghost" onClick={() => onClone(profile.id)} title="Clone">
								<Copy className="h-4 w-4" />
							</Button>
							<Button size="icon" variant="ghost" onClick={() => onDelete(profile.id)}>
								<Trash2 className="h-4 w-4" />
							</Button>
						</TableCell>
					</TableRow>
				))}
			</TableBody>
		</Table>
	);
}
