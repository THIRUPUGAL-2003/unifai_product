import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import { useGetClusterConfigQuery, useUpdateClusterConfigMutation } from "@enterprise/lib/store/apis/clusterApi";
import { ClusterConfig } from "@enterprise/lib/types/workspace";
import { Network, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

export default function ClusterPage() {
	const { data, isLoading: loading } = useGetClusterConfigQuery();
	const [updateCluster, { isLoading: saving }] = useUpdateClusterConfigMutation();
	const [config, setConfig] = useState<ClusterConfig>({
		enabled: false,
		type: "mesh",
		region: "unknown",
		peers: [],
	});
	const [peerText, setPeerText] = useState("");
	const [peerErrors, setPeerErrors] = useState<string[]>([]);

	const validatePeers = (peers: string[]) => {
		const errors: string[] = [];
		const hostPort = /^[a-zA-Z0-9._-]+:\d+$/;
		peers.forEach((peer) => {
			if (!hostPort.test(peer)) {
				errors.push(`Invalid peer "${peer}" — use host:port (e.g. 10.0.0.12:7946)`);
			}
		});
		if (config.enabled && peers.length === 0) {
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
		const peers = peerText
			.split(/\n|,/)
			.map((peer) => peer.trim())
			.filter(Boolean);
		if (!validatePeers(peers)) {
			toast.error("Fix peer validation errors before saving");
			return;
		}
		try {
			const next = await updateCluster({ ...config, peers }).unwrap();
			setConfig({ ...config, ...next, node: config.node });
			toast.success("Cluster configuration saved");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	if (loading) {
		return <div className="text-muted-foreground p-6 text-sm">Loading cluster config…</div>;
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

			<Card>
				<CardHeader>
					<CardTitle className="text-base">Cluster settings</CardTitle>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between rounded-lg border p-3">
						<div>
							<Label>Enable cluster mode</Label>
							<p className="text-muted-foreground text-xs">Persists cluster_config for this workspace.</p>
						</div>
						<Switch checked={config.enabled} onCheckedChange={(enabled) => setConfig({ ...config, enabled })} />
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="space-y-1">
							<Label>Type</Label>
							<select
								value={config.type || "mesh"}
								onChange={(e) => setConfig({ ...config, type: e.target.value })}
								className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
							>
								<option value="mesh">mesh</option>
								<option value="broker">broker</option>
							</select>
						</div>
						<div className="space-y-1">
							<Label>Region</Label>
							<Input value={config.region || ""} onChange={(e) => setConfig({ ...config, region: e.target.value })} />
						</div>
					</div>
					<div className="space-y-1">
						<Label>Peers (host:port, one per line)</Label>
						<textarea
							value={peerText}
							onChange={(e) => {
								setPeerText(e.target.value);
								const peers = e.target.value
									.split(/\n|,/)
									.map((peer) => peer.trim())
									.filter(Boolean);
								validatePeers(peers);
							}}
							className="border-input bg-background min-h-28 w-full rounded-md border p-3 font-mono text-sm"
							placeholder="10.0.0.12:7946"
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
							<Label>Gossip port</Label>
							<Input
								type="number"
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
							<Label>gRPC port</Label>
							<Input
								type="number"
								value={config.grpc?.port || 10102}
								onChange={(e) => setConfig({ ...config, grpc: { port: Number(e.target.value), dial_timeout_seconds: config.grpc?.dial_timeout_seconds || 5 } })}
							/>
						</div>
					</div>
					<div className="flex justify-end">
						<Button onClick={() => void save()} disabled={saving || peerErrors.length > 0}>
							<Save className="h-4 w-4" />
							{saving ? "Saving…" : "Save cluster config"}
						</Button>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
