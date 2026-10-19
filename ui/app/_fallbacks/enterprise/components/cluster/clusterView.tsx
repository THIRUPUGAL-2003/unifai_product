import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { useGetClusterConfigQuery, useUpdateClusterConfigMutation } from "@enterprise/lib/store/apis/clusterApi";
import { ClusterConfig } from "@enterprise/lib/types/workspace";
import { AlertTriangle, Network, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

function splitPeers(text: string) {
	return text
		.split(/\n|,/)
		.map((peer) => peer.trim())
		.filter(Boolean);
}

/** Same rules as the gateway: host:port, or http(s)://host[:port] with no path. */
function invalidClusterPeer(peer: string): string | null {
	const example = "10.0.0.12:6000";
	const raw = peer.trim();
	if (!raw) return null;
	const hasHttp = /^https?:\/\//i.test(raw);
	if (!hasHttp && raw.includes("://")) {
		return `Invalid peer "${raw}" — only http:// or https:// URLs are supported (e.g. ${example})`;
	}
	let parsed: URL;
	try {
		parsed = new URL(hasHttp ? raw : `http://${raw}`);
	} catch {
		return `Invalid peer "${raw}" — use host:port or http(s)://host:port (e.g. ${example})`;
	}
	if (!parsed.hostname) {
		return `Invalid peer "${raw}" — use host:port or http(s)://host:port (e.g. ${example})`;
	}
	if (parsed.pathname && parsed.pathname !== "/") {
		return `Invalid peer "${raw}" — give only the gateway address, without a path`;
	}
	if (parsed.username || parsed.password) {
		return `Invalid peer "${raw}" — do not include a username or password`;
	}
	if (!parsed.port && !hasHttp) {
		return `Invalid peer "${raw}" — port is required (e.g. ${example})`;
	}
	return null;
}

export default function ClusterPage() {
	const { data, isLoading: loading, isError, error, refetch } = useGetClusterConfigQuery();
	const [updateCluster, { isLoading: saving }] = useUpdateClusterConfigMutation();
	const [config, setConfig] = useState<ClusterConfig>({
		enabled: false,
		type: "mesh",
		region: "unknown",
		peers: [],
	});
	const [peerText, setPeerText] = useState("");
	const [peerErrors, setPeerErrors] = useState<string[]>([]);

	const canEdit = useRbac(RbacResource.Cluster, RbacOperation.Update);

	const validatePeers = (peers: string[], enabled = config.enabled) => {
		const errors: string[] = [];
		peers.forEach((peer) => {
			const message = invalidClusterPeer(peer);
			if (message) errors.push(message);
		});
		if (enabled && peers.length === 0) {
			errors.push("Add at least one peer when cluster mode is enabled.");
		}
		setPeerErrors(errors);
		return errors.length === 0;
	};

	useEffect(() => {
		if (!data) return;
		setConfig(data);
		setPeerText((data.peers || []).join("\n"));
	}, [data]);

	const save = async () => {
		const peers = splitPeers(peerText);
		if (!validatePeers(peers)) {
			toast.error("Fix peer validation errors before saving");
			return;
		}
		try {
			const next = await updateCluster({ ...config, peers }).unwrap();
			setConfig({ ...config, ...next });
			setPeerText((next.peers || peers).join("\n"));
			toast.success("Cluster configuration saved");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const handleToggleCluster = async (enabled: boolean) => {
		const peers = splitPeers(peerText);
		if (!enabled) {
			const peersToSave = peers.filter((peer) => !invalidClusterPeer(peer));
			const nextConfig = { ...config, enabled: false, peers: peersToSave };
			setConfig(nextConfig);
			setPeerErrors([]);
			try {
				const next = await updateCluster(nextConfig).unwrap();
				setConfig({ ...config, ...next });
				setPeerText((next.peers || peersToSave).join("\n"));
				toast.success("Cluster mode disabled and saved");
			} catch (err) {
				setConfig(config);
				toast.error(getErrorMessage(err));
			}
			return;
		}
		if (peers.length === 0) {
			setConfig({ ...config, enabled: true });
			setPeerErrors(["Add at least one peer when cluster mode is enabled."]);
			toast.info("Cluster mode turned on. Add at least one peer gateway address and click Save.");
			return;
		}
		if (!validatePeers(peers, true)) {
			setConfig({ ...config, enabled: true });
			toast.error("Please fix peer addresses before saving.");
			return;
		}
		const nextConfig = { ...config, enabled: true, peers };
		setConfig(nextConfig);
		try {
			const next = await updateCluster(nextConfig).unwrap();
			setConfig({ ...config, ...next });
			toast.success("Cluster mode enabled and saved");
		} catch (err) {
			setConfig(config);
			toast.error(getErrorMessage(err));
		}
	};

	if (loading) {
		return <div className="text-muted-foreground p-6 text-sm">Loading cluster config…</div>;
	}

	if (isError) {
		return (
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-3 p-6">
				<QueryErrorBanner testId="cluster-query-error" message={getErrorMessage(error) || "Failed to load cluster configuration."} />
				<Button type="button" size="sm" variant="outline" className="w-fit" onClick={() => void refetch()}>
					Retry
				</Button>
			</div>
		);
	}

	return (
		<div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-6">
			<div className="flex items-center gap-3">
				<Network className="text-muted-foreground h-8 w-8" strokeWidth={1.5} />
				<div>
					<h1 className="text-xl font-semibold">Cluster Config</h1>
					<p className="text-muted-foreground text-sm">Enable mesh or broker clustering and register peer addresses.</p>
				</div>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">This node</CardTitle>
					<CardDescription>Runtime view of the current process.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-3 text-sm">
					<div className="flex items-center justify-between">
						<span className="text-muted-foreground">Mode</span>
						<Badge variant="secondary">{config.enabled ? "Cluster" : config.node?.mode || "Standalone"}</Badge>
					</div>
					<div className="flex items-center justify-between">
						<span className="text-muted-foreground">Address</span>
						<span className="font-mono">{config.node?.address || window.location.host}</span>
					</div>
				</CardContent>
			</Card>

			{(config.warnings?.length ?? 0) > 0 && (
				<div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-4 text-sm" data-testid="cluster-warnings">
					<div className="mb-2 flex items-center gap-2 font-medium text-amber-700 dark:text-amber-400">
						<AlertTriangle className="h-4 w-4" />
						Replication is not fully working
					</div>
					<ul className="list-disc space-y-1 pl-5">
						{config.warnings?.map((warning) => (
							<li key={warning}>{warning}</li>
						))}
					</ul>
				</div>
			)}

			<Card>
				<CardHeader>
					<CardTitle className="text-base">Cluster settings</CardTitle>
					<CardDescription>
						Nodes replicate shared state over each peer&apos;s gateway HTTP address (<code>/internal/cluster/kv</code>). Every node needs the
						same <code>CLUSTER_REPLICATE_SECRET</code> environment variable
						{config.replicate_secret_configured === false ? " — it is not set on this node." : "."}
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between rounded-lg border p-3">
						<div>
							<Label>Enable cluster mode</Label>
							<p className="text-muted-foreground text-xs">Persists cluster_config for this workspace.</p>
						</div>
						<Switch
							checked={config.enabled}
							disabled={saving || !canEdit}
							onCheckedChange={(enabled) => void handleToggleCluster(enabled)}
						/>
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-1">
							<Label>Type</Label>
							<select
								value={config.type || "mesh"}
								disabled={!canEdit}
								onChange={(e) => setConfig({ ...config, type: e.target.value })}
								className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
							>
								<option value="mesh">mesh</option>
								<option value="broker">broker</option>
							</select>
						</div>
						<div className="space-y-1">
							<Label>Region</Label>
							<Input value={config.region || ""} disabled={!canEdit} onChange={(e) => setConfig({ ...config, region: e.target.value })} />
						</div>
					</div>
					<div className="space-y-1">
						<Label>Peer gateway addresses (one per line)</Label>
						<p className="text-muted-foreground text-xs">
							The other nodes&apos; HTTP address — the same host:port the dashboard/API uses — e.g. <code>https://node-2.internal</code> or <code>10.0.0.12:6000</code>. Not the gossip port.
						</p>
						<textarea
							value={peerText}
							disabled={!canEdit}
							onChange={(e) => {
								setPeerText(e.target.value);
								validatePeers(splitPeers(e.target.value));
							}}
							className="border-input bg-background min-h-28 w-full rounded-md border p-3 font-mono text-sm"
							placeholder={"https://node-2.internal\nhttps://node-3.internal"}
						/>
						{peerErrors.length > 0 && (
							<ul className="text-destructive space-y-1 text-xs">
								{peerErrors.map((err) => (
									<li key={err}>{err}</li>
								))}
							</ul>
						)}
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-1">
							<Label>Gossip port (reserved)</Label>
							<Input
								type="number"
								disabled={!canEdit}
								value={config.gossip?.port || 7946}
								onChange={(e) =>
									setConfig({
										...config,
										gossip: {
											port: Number(e.target.value),
											config: config.gossip?.config || { timeout_seconds: 5, success_threshold: 1, failure_threshold: 3 },
										},
									})
								}
							/>
						</div>
						<div className="space-y-1">
							<Label>gRPC port (reserved)</Label>
							<Input
								type="number"
								disabled={!canEdit}
								value={config.grpc?.port || 10102}
								onChange={(e) => setConfig({ ...config, grpc: { port: Number(e.target.value), dial_timeout_seconds: config.grpc?.dial_timeout_seconds || 5 } })}
							/>
						</div>
					</div>
					<p className="text-muted-foreground text-xs">
						Gossip and gRPC ports are stored for future transports; replication today uses the peer gateway addresses above.
					</p>
					<div className="flex justify-end">
						<Button onClick={() => void save()} disabled={saving || peerErrors.length > 0 || !canEdit}>
							<Save className="h-4 w-4" />
							{saving ? "Saving…" : "Save cluster config"}
						</Button>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
