import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useGetMCPClientsQuery, useUpdateMCPClientMutation } from "@/lib/store";
import type { MCPAuthType, MCPClient } from "@/lib/types/mcp";
import { KeyRound, Save } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

export default function MCPAuthConfigView() {
	const { data, isLoading } = useGetMCPClientsQuery({ limit: 200, offset: 0 });
	const [updateClient] = useUpdateMCPClientMutation();
	const clients = data?.clients || [];
	const [selectedId, setSelectedId] = useState<string>("");
	const [headerName, setHeaderName] = useState("Authorization");
	const [headerValue, setHeaderValue] = useState("");
	const [perUserKeys, setPerUserKeys] = useState("Authorization");

	const selected = useMemo(() => clients.find((client) => client.config.client_id === selectedId) || null, [clients, selectedId]);

	const saveHeaders = async (client: MCPClient) => {
		try {
			await updateClient({
				id: client.config.client_id,
				data: {
					headers: {
						[headerName]: { value: headerValue, ref: "" },
					},
				},
			}).unwrap();
			toast.success(`Updated headers for ${client.config.name}`);
			setHeaderValue("");
		} catch (err) {
			toast.error(err instanceof Error ? err.message : "Failed to update MCP auth");
		}
	};

	const savePerUser = async (client: MCPClient) => {
		try {
			await updateClient({
				id: client.config.client_id,
				data: {
					per_user_header_keys: perUserKeys
						.split(",")
						.map((key) => key.trim())
						.filter(Boolean),
				},
			}).unwrap();
			toast.success(`Updated per-user header schema for ${client.config.name}`);
		} catch (err) {
			toast.error(err instanceof Error ? err.message : "Failed to update per-user headers");
		}
	};

	return (
		<div className="flex w-full flex-col gap-6 p-1">
			<div>
				<h1 className="flex items-center gap-2 text-2xl font-semibold">
					<KeyRound className="h-6 w-6" />
					MCP Auth Config
				</h1>
				<p className="text-muted-foreground mt-1 text-sm">
					Configure headers, OAuth, and per-user credentials for each deployed MCP client.
				</p>
			</div>

			{isLoading ? (
				<p className="text-muted-foreground text-sm">Loading MCP clients…</p>
			) : clients.length === 0 ? (
				<div className="rounded-xl border border-dashed p-10 text-center">
					<p className="font-medium">No MCP clients</p>
					<p className="text-muted-foreground mt-1 text-sm">Add a client in MCP Clients first, then configure auth here.</p>
				</div>
			) : (
				<div className="grid gap-4 lg:grid-cols-[1fr_360px]">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Client</TableHead>
								<TableHead>Auth</TableHead>
								<TableHead>State</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{clients.map((client) => (
								<TableRow
									key={client.config.client_id}
									className={selectedId === client.config.client_id ? "bg-muted/40" : ""}
									onClick={() => {
										setSelectedId(client.config.client_id);
										setPerUserKeys((client.config.per_user_header_keys || []).join(", "));
									}}
								>
									<TableCell className="font-medium">{client.config.name}</TableCell>
									<TableCell>
										<Badge variant="secondary">{(client.config.auth_type || "none") as MCPAuthType}</Badge>
									</TableCell>
									<TableCell>{client.state}</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>

					<div className="space-y-4 rounded-xl border p-4">
						{!selected ? (
							<p className="text-muted-foreground text-sm">Select a client to edit auth.</p>
						) : (
							<>
								<h2 className="font-semibold">{selected.config.name}</h2>
								<p className="text-muted-foreground text-xs">Auth type is set when the client is created. Rotate credentials below.</p>
								{(selected.config.auth_type === "headers" || !selected.config.auth_type || selected.config.auth_type === "none") && (
									<div className="space-y-2">
										<Label>Header name</Label>
										<Input value={headerName} onChange={(e) => setHeaderName(e.target.value)} />
										<Label>Header value</Label>
										<Input type="password" value={headerValue} onChange={(e) => setHeaderValue(e.target.value)} />
										<Button onClick={() => void saveHeaders(selected)}>
											<Save className="h-4 w-4" />
											Save header
										</Button>
									</div>
								)}
								{selected.config.auth_type === "per_user_headers" && (
									<div className="space-y-2">
										<Label>Per-user header keys</Label>
										<Input value={perUserKeys} onChange={(e) => setPerUserKeys(e.target.value)} placeholder="Authorization, X-Api-Key" />
										<Button onClick={() => void savePerUser(selected)}>
											<Save className="h-4 w-4" />
											Save schema
										</Button>
									</div>
								)}
								{(selected.config.auth_type === "oauth" || selected.config.auth_type === "per_user_oauth") && (
									<p className="text-sm">OAuth credentials are rotated from the MCP client editor. This client is using {selected.config.auth_type}.</p>
								)}
							</>
						)}
					</div>
				</div>
			)}
		</div>
	);
}
