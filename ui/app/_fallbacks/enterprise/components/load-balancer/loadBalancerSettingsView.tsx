import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Link } from "@tanstack/react-router";
import { getErrorMessage } from "@/lib/store";
import { useGetLoadBalancerConfigQuery, useUpdateLoadBalancerConfigMutation } from "@enterprise/lib/store/apis/loadBalancerApi";
import { LoadBalancerConfig } from "@enterprise/lib/types/workspace";
import { ArrowLeft, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

const defaults: LoadBalancerConfig = {
	enabled: false,
	direction_selection_enabled: true,
	route_selection_enabled: true,
	reroute_failed_directions: false,
	prune_failed_fallbacks: false,
};

export default function LoadBalancerSettingsView() {
	const { data, isLoading: loading } = useGetLoadBalancerConfigQuery();
	const [updateConfig, { isLoading: saving }] = useUpdateLoadBalancerConfigMutation();
	const [config, setConfig] = useState<LoadBalancerConfig>(defaults);

	useEffect(() => {
		if (data) setConfig({ ...defaults, ...data });
	}, [data]);

	const save = async () => {
		try {
			const next = await updateConfig(config).unwrap();
			setConfig({ ...defaults, ...next });
			toast.success("Load balancer settings saved");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const toggle = (key: keyof LoadBalancerConfig) => {
		setConfig((current) => ({ ...current, [key]: !current[key] }));
	};

	if (loading) {
		return <div className="text-muted-foreground p-6 text-sm">Loading settings…</div>;
	}

	const rows: { key: keyof LoadBalancerConfig; title: string; description: string }[] = [
		{ key: "enabled", title: "Enable adaptive load balancing", description: "Use live route scores when choosing a provider and key." },
		{ key: "direction_selection_enabled", title: "Direction selection", description: "Pick the healthiest provider for a requested model." },
		{ key: "route_selection_enabled", title: "Route selection", description: "Weight individual API keys inside the chosen provider." },
		{ key: "reroute_failed_directions", title: "Reroute failed directions", description: "If a pinned provider is unhealthy, send the request to a healthy one." },
		{ key: "prune_failed_fallbacks", title: "Prune failed fallbacks", description: "Drop unhealthy directions from a request's configured fallbacks." },
	];

	return (
		<div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-1">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-semibold">Adaptive Routing Settings</h1>
					<p className="text-muted-foreground mt-1 text-sm">Persisted selection policy for provider and key routing.</p>
				</div>
				<Button asChild variant="ghost">
					<Link to="/workspace/adaptive-routing">
						<ArrowLeft className="h-4 w-4" />
						Back
					</Link>
				</Button>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">Selection policy</CardTitle>
					<CardDescription>These flags are stored in the config database and applied on the next request cycle.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{rows.map((row) => (
						<div key={row.key} className="flex items-start justify-between gap-4 rounded-lg border p-4">
							<div>
								<p className="font-medium">{row.title}</p>
								<p className="text-muted-foreground text-sm">{row.description}</p>
							</div>
							<Switch checked={config[row.key]} onCheckedChange={() => toggle(row.key)} />
						</div>
					))}
					<div className="flex justify-end">
						<Button onClick={() => void save()} disabled={saving}>
							<Save className="h-4 w-4" />
							{saving ? "Saving…" : "Save settings"}
						</Button>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
