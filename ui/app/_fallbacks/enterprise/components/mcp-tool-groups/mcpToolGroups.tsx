import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage, useGetMCPClientsQuery, useGetVirtualKeysQuery } from "@/lib/store";
import {
	useCreateMCPToolGroupMutation,
	useDeleteMCPToolGroupMutation,
	useGetMCPToolGroupsQuery,
	useUpdateMCPToolGroupMutation,
} from "@enterprise/lib/store/apis/mcpToolGroupsApi";
import { MCPToolGroup } from "@enterprise/lib/types/workspace";
import { Boxes, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

/** Build runtime tool patterns: clientName-toolName (governance format). */
function buildToolEntries(clientId: string, clientName: string, toolNamesRaw: string): { mcp_client_id: string; mcp_client_name: string; tool_names?: string[]; name: string }[] {
	const names = toolNamesRaw
		.split(",")
		.map((tool) => tool.trim())
		.filter(Boolean);
	if (!clientId) return [];
	const prefix = (clientName || clientId).trim();
	if (names.length === 0) {
		return [
			{
				mcp_client_id: clientId,
				mcp_client_name: prefix,
				name: `${prefix}-*`,
			},
		];
	}
	return names.map((tool) => ({
		mcp_client_id: clientId,
		mcp_client_name: prefix,
		tool_names: [tool],
		name: `${prefix}-${tool}`,
	}));
}

function parseToolsFromGroup(group: MCPToolGroup): { clientId: string; toolNames: string } {
	const tools = group.tools || [];
	if (tools.length === 0) return { clientId: "", toolNames: "" };
	const first = tools[0] as Record<string, unknown>;
	const clientId = String(first.mcp_client_id || "");
	const names: string[] = [];
	for (const raw of tools) {
		const t = raw as Record<string, unknown>;
		const toolNames = t.tool_names;
		if (Array.isArray(toolNames) && toolNames.length > 0) {
			names.push(...toolNames.map(String));
			continue;
		}
		const name = String(t.name || "");
		const prefix = String(t.mcp_client_name || clientId);
		if (name.endsWith("-*") || name === `${prefix}-*`) {
			continue;
		}
		if (prefix && name.startsWith(`${prefix}-`)) {
			names.push(name.slice(prefix.length + 1));
		}
	}
	return { clientId, toolNames: names.join(", ") };
}

export default function MCPToolGroups() {
	const { data } = useGetMCPClientsQuery({ limit: 200, offset: 0 });
	const clients = data?.clients || [];
	const { data: vkData } = useGetVirtualKeysQuery({ limit: 200, offset: 0 });
	const virtualKeys = vkData?.virtual_keys || [];
	const { data: groupData } = useGetMCPToolGroupsQuery();
	const [createGroup] = useCreateMCPToolGroupMutation();
	const [updateGroup] = useUpdateMCPToolGroupMutation();
	const [deleteGroup] = useDeleteMCPToolGroupMutation();
	const groups = groupData?.tool_groups || [];
	const [open, setOpen] = useState(false);
	const [editing, setEditing] = useState<MCPToolGroup | null>(null);
	const [name, setName] = useState("");
	const [description, setDescription] = useState("");
	const [clientId, setClientId] = useState("");
	const [toolNames, setToolNames] = useState("");
	const [selectedVkIds, setSelectedVkIds] = useState<string[]>([]);
	const enabled = true;

	const selectedClientName = useMemo(() => {
		const c = clients.find((client) => client.config.client_id === clientId);
		return c?.config.name || "";
	}, [clients, clientId]);

	const resetForm = () => {
		setEditing(null);
		setName("");
		setDescription("");
		setToolNames("");
		setClientId("");
		setSelectedVkIds([]);
	};

	const openCreate = () => {
		resetForm();
		setOpen(true);
	};

	const openEdit = (group: MCPToolGroup) => {
		const parsed = parseToolsFromGroup(group);
		setEditing(group);
		setName(group.name);
		setDescription(group.description || "");
		setClientId(parsed.clientId);
		setToolNames(parsed.toolNames);
		setSelectedVkIds(group.virtual_key_ids || []);
		setOpen(true);
	};

	const save = async () => {
		if (!name.trim()) {
			toast.error("Name is required");
			return;
		}
		if (!clientId) {
			toast.error("Select an MCP client");
			return;
		}
		if (selectedVkIds.length === 0) {
			toast.error("Select at least one virtual key — empty key list restricts ALL keys to only these tools");
			return;
		}
		const payload = {
			name: name.trim(),
			description,
			enabled: editing ? editing.enabled : enabled,
			tools: buildToolEntries(clientId, selectedClientName, toolNames),
			virtual_key_ids: selectedVkIds,
		};
		try {
			if (editing) {
				await updateGroup({ ...editing, ...payload }).unwrap();
				toast.success("Tool group updated");
			} else {
				await createGroup(payload).unwrap();
				toast.success("Tool group created");
			}
			setOpen(false);
			resetForm();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const toggle = async (group: MCPToolGroup) => {
		try {
			await updateGroup({ ...group, enabled: !group.enabled }).unwrap();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const remove = async (id: number) => {
		try {
			await deleteGroup(id).unwrap();
			toast.success("Tool group deleted");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const toggleVk = (id: string) => {
		setSelectedVkIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
	};

	return (
		<div className="flex w-full flex-col gap-6 p-1">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-semibold">
						<Boxes className="h-6 w-6" />
						MCP Tool Groups
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">
						Bundle tools from an MCP client and scope them to specific virtual keys. Matching keys may only execute tools in the
						group — pick keys carefully so other MCP tools are not blocked.
					</p>
				</div>
				<Button onClick={openCreate}>
					<Plus className="h-4 w-4" />
					New group
				</Button>
			</div>

			{groups.length === 0 ? (
				<div className="rounded-xl border border-dashed p-10 text-center">
					<p className="font-medium">No tool groups</p>
					<p className="text-muted-foreground mt-1 text-sm">Create a group to share a curated tool set.</p>
				</div>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Tools</TableHead>
							<TableHead>Virtual keys</TableHead>
							<TableHead>Enabled</TableHead>
							<TableHead />
						</TableRow>
					</TableHeader>
					<TableBody>
						{groups.map((group) => (
							<TableRow key={group.id}>
								<TableCell>
									<div className="font-medium">{group.name}</div>
									<div className="text-muted-foreground text-xs">{group.description}</div>
								</TableCell>
								<TableCell>
									<Badge variant="secondary">{group.tools?.length || 0} tool pattern(s)</Badge>
								</TableCell>
								<TableCell>
									<Badge variant="outline">
										{group.virtual_key_ids?.length ? `${group.virtual_key_ids.length} key(s)` : "No keys (restricted)"}
									</Badge>
								</TableCell>
								<TableCell>
									<Switch checked={group.enabled} onCheckedChange={() => void toggle(group)} />
								</TableCell>
								<TableCell className="text-right">
									<Button size="icon" variant="ghost" onClick={() => openEdit(group)} title="Edit">
										<Pencil className="h-4 w-4" />
									</Button>
									<Button size="icon" variant="ghost" onClick={() => void remove(group.id)}>
										<Trash2 className="h-4 w-4" />
									</Button>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}

			<Dialog
				open={open}
				onOpenChange={(v) => {
					setOpen(v);
					if (!v) resetForm();
				}}
			>
				<DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
					<DialogHeader>
						<DialogTitle>{editing ? "Edit tool group" : "Create tool group"}</DialogTitle>
					</DialogHeader>
					<div className="space-y-3 py-2">
						<div className="space-y-1">
							<Label>Name</Label>
							<Input value={name} onChange={(e) => setName(e.target.value)} />
						</div>
						<div className="space-y-1">
							<Label>Description</Label>
							<Input value={description} onChange={(e) => setDescription(e.target.value)} />
						</div>
						<div className="space-y-1">
							<Label>MCP client</Label>
							<select value={clientId} onChange={(e) => setClientId(e.target.value)} className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm">
								<option value="">Select client</option>
								{clients.map((client) => (
									<option key={client.config.client_id} value={client.config.client_id}>
										{client.config.name}
									</option>
								))}
							</select>
						</div>
						<div className="space-y-1">
							<Label>Tool names (blank = all tools on this client)</Label>
							<Input value={toolNames} onChange={(e) => setToolNames(e.target.value)} placeholder="search, fetch" />
						</div>
						<div className="space-y-1">
							<Label>Virtual keys (required)</Label>
							<p className="text-muted-foreground text-xs">Only selected keys are restricted to this tool set.</p>
							<div className="border-input max-h-40 space-y-1 overflow-y-auto rounded-md border p-2">
								{virtualKeys.length === 0 ? (
									<p className="text-muted-foreground text-xs">No virtual keys found.</p>
								) : (
									virtualKeys.map((vk) => (
										<label key={vk.id} className="flex cursor-pointer items-center gap-2 text-sm">
											<input type="checkbox" checked={selectedVkIds.includes(vk.id)} onChange={() => toggleVk(vk.id)} />
											<span className="truncate">{vk.name}</span>
										</label>
									))
								)}
							</div>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setOpen(false)}>
							Cancel
						</Button>
						<Button onClick={() => void save()}>{editing ? "Save changes" : "Create"}</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
