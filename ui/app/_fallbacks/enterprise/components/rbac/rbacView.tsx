import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage } from "@/lib/store";
import {
	useCreateRoleMutation,
	useDeleteRoleMutation,
	useGetPermissionsQuery,
	useGetRolePermissionsQuery,
	useGetRolesQuery,
	useUpdateRolePermissionsMutation,
} from "@enterprise/lib/store/apis/rbacApi";
import { RBACPermission, RBACRole } from "@enterprise/lib/types/workspace";
import { Plus, Shield, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

export default function RBACView() {
	const [selected, setSelected] = useState<RBACRole | null>(null);
	const [selectedPerms, setSelectedPerms] = useState<number[]>([]);
	const [open, setOpen] = useState(false);
	const [name, setName] = useState("");
	const [description, setDescription] = useState("");
	const [dac, setDac] = useState("all-data");
	const { data: roleData } = useGetRolesQuery();
	const { data: permData } = useGetPermissionsQuery();
	const { data: rolePermData } = useGetRolePermissionsQuery(selected?.id ?? 0, { skip: !selected });
	const [createRole] = useCreateRoleMutation();
	const [updatePerms] = useUpdateRolePermissionsMutation();
	const [deleteRole] = useDeleteRoleMutation();
	const roles = roleData?.roles || [];
	const permissions = permData?.permissions || [];

	useEffect(() => {
		if (!selected) return;
		setSelectedPerms((rolePermData?.permissions || []).map((perm) => perm.id));
	}, [selected, rolePermData]);

	const create = async () => {
		try {
			await createRole({ name, description, dac }).unwrap();
			toast.success("Role created — assign it on Users → Add/Edit User");
			setOpen(false);
			setName("");
			setDescription("");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const savePerms = async () => {
		if (!selected) return;
		if (selectedPerms.length === 0 && !Boolean(selected.is_system_role || selected.name === "admin")) {
			toast.warning("No permissions selected — this role will deny all non-admin actions until you grant some.");
		}
		try {
			await updatePerms({ id: selected.id, permission_ids: selectedPerms }).unwrap();
			toast.success("Permissions updated");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const remove = async (role: RBACRole) => {
		try {
			await deleteRole(role.id).unwrap();
			if (selected?.id === role.id) setSelected(null);
			toast.success("Role deleted");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const grouped = useMemo(() => {
		const map = new Map<string, RBACPermission[]>();
		for (const perm of permissions) {
			const list = map.get(perm.resource) || [];
			list.push(perm);
			map.set(perm.resource, list);
		}
		return Array.from(map.entries());
	}, [permissions]);

	return (
		<div className="flex h-full flex-col gap-4">
			<div className="grid h-full grid-cols-1 gap-4 lg:grid-cols-[320px_1fr]">
			<div className="flex flex-col gap-3">
				<div className="flex items-center justify-between">
					<h1 className="flex items-center gap-2 text-xl font-semibold">
						<Shield className="h-5 w-5" />
						RBAC
					</h1>
					<div className="flex items-center gap-2">
						<a
							href="/workspace/governance/users"
							className="text-muted-foreground hover:text-foreground text-xs underline-offset-2 hover:underline"
						>
							Assign on Users
						</a>
						<Button size="sm" onClick={() => setOpen(true)}>
							<Plus className="h-4 w-4" />
							Role
						</Button>
					</div>
				</div>
				<p className="text-muted-foreground text-xs">
					Create roles here, set permissions, then assign the role when creating or editing a user.
				</p>
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Role</TableHead>
							<TableHead />
						</TableRow>
					</TableHeader>
					<TableBody>
						{roles.map((role) => {
							const isSysRole = Boolean(role.is_system_role || role.name?.toLowerCase() === "admin" || role.name?.toLowerCase() === "user");
							return (
								<TableRow key={role.id} className={selected?.id === role.id ? "bg-muted/40" : ""} onClick={() => setSelected(role)}>
									<TableCell>
										<div className="font-medium">{role.name}</div>
										<div className="text-muted-foreground text-xs">{role.dac}</div>
										{isSysRole && <Badge variant="secondary">system</Badge>}
									</TableCell>
									<TableCell className="text-right">
										{!isSysRole && (
											<Button
												size="icon"
												variant="ghost"
												onClick={(e) => {
													e.stopPropagation();
													void remove(role);
												}}
											>
												<Trash2 className="h-4 w-4" />
											</Button>
										)}
									</TableCell>
								</TableRow>
							);
						})}
					</TableBody>
				</Table>
			</div>

			<div className="overflow-auto rounded-xl border p-4">
				{!selected ? (
					<p className="text-muted-foreground text-sm">Select a role to edit its permission matrix.</p>
				) : (
					<div className="space-y-4">
						<div>
							<h2 className="text-lg font-semibold">{selected.name}</h2>
							<p className="text-muted-foreground text-sm">{selected.description || "No description"}</p>
						</div>
						{grouped.map(([resource, perms]) => (
							<div key={resource} className="rounded-lg border p-3">
								<p className="mb-2 text-sm font-medium">{resource}</p>
								<div className="flex flex-wrap gap-3">
									{perms.map((perm) => (
										<label key={perm.id} className="flex items-center gap-2 text-sm">
											<input
												type="checkbox"
												checked={selectedPerms.includes(perm.id)}
												onChange={(e) => {
													setSelectedPerms((current) =>
														e.target.checked ? [...current, perm.id] : current.filter((id) => id !== perm.id),
													);
												}}
											/>
											{perm.operation}
										</label>
									))}
								</div>
							</div>
						))}
						<Button onClick={() => void savePerms()}>Save permissions</Button>
					</div>
				)}
			</div>

			<Dialog open={open} onOpenChange={setOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Create role</DialogTitle>
					</DialogHeader>
					<div className="space-y-3 py-2">
						<div className="space-y-1">
							<Label>Name</Label>
							<Input value={name} onChange={(e) => setName(e.target.value)} placeholder="analyst" />
						</div>
						<div className="space-y-1">
							<Label>Description</Label>
							<Input value={description} onChange={(e) => setDescription(e.target.value)} />
						</div>
						<div className="space-y-1">
							<Label>Data access</Label>
							<select value={dac} onChange={(e) => setDac(e.target.value)} className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm">
								<option value="all-data">all-data</option>
								<option value="team-data">team-data</option>
								<option value="own-data">own-data</option>
							</select>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setOpen(false)}>
							Cancel
						</Button>
						<Button onClick={() => void create()}>Create</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
		</div>
	);
}
