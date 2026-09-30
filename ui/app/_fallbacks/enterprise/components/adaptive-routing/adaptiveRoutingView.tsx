import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Link } from "@tanstack/react-router";
import { getErrorMessage } from "@/lib/store";
import { useGetLoadBalancerRoutesQuery, useUpdateLoadBalancerConfigMutation } from "@enterprise/lib/store/apis/loadBalancerApi";
import { LoadBalancerConfig } from "@enterprise/lib/types/workspace";
import { Activity, Gauge, Route, Settings2 } from "lucide-react";
import { toast } from "sonner";

export default function AdaptiveRoutingView() {
	const { data, isLoading: loading } = useGetLoadBalancerRoutesQuery(undefined, { pollingInterval: 8000 });
	const [updateConfig, { isLoading: saving }] = useUpdateLoadBalancerConfigMutation();

	const saveConfig = async (patch: Partial<LoadBalancerConfig>) => {
		if (!data) return;
		try {
			await updateConfig({ ...data.config, ...patch }).unwrap();
			toast.success("Adaptive routing settings saved");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	if (loading || !data) {
		return <div className="text-muted-foreground p-6 text-sm">Loading adaptive routing…</div>;
	}

	return (
		<div className="flex w-full flex-col gap-6 p-1">
			<div className="flex flex-col justify-between gap-4 md:flex-row md:items-center">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-semibold">
						<Gauge className="h-6 w-6" />
						Adaptive Routing
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">
						Score providers (directions) and keys (routes) from live workspace inventory, then persist selection policy.
					</p>
				</div>
				<Button asChild variant="outline">
					<Link to="/workspace/adaptive-routing/settings">
						<Settings2 className="h-4 w-4" />
						Settings
					</Link>
				</Button>
			</div>

			<div className="grid gap-4 md:grid-cols-3">
				<Card>
					<CardHeader className="pb-2">
						<CardTitle className="text-sm font-medium">Load balancer</CardTitle>
						<CardDescription>Master switch for adaptive selection</CardDescription>
					</CardHeader>
					<CardContent className="flex items-center justify-between">
						<span className="text-sm">{data.config.enabled ? "Enabled" : "Disabled"}</span>
						<Switch checked={data.config.enabled} disabled={saving} onCheckedChange={(enabled) => void saveConfig({ enabled })} />
					</CardContent>
				</Card>
				<Card>
					<CardHeader className="pb-2">
						<CardTitle className="text-sm font-medium">Directions</CardTitle>
						<CardDescription>Configured providers</CardDescription>
					</CardHeader>
					<CardContent className="text-2xl font-semibold">{data.directions.length}</CardContent>
				</Card>
				<Card>
					<CardHeader className="pb-2">
						<CardTitle className="text-sm font-medium">Routes</CardTitle>
						<CardDescription>Provider keys available for weighting</CardDescription>
					</CardHeader>
					<CardContent className="text-2xl font-semibold">{data.routes.length}</CardContent>
				</Card>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2 text-base">
						<Activity className="h-4 w-4" />
						Directions
					</CardTitle>
				</CardHeader>
				<CardContent>
					{data.directions.length === 0 ? (
						<p className="text-muted-foreground text-sm">No providers configured yet.</p>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Provider</TableHead>
									<TableHead>Keys</TableHead>
									<TableHead>Status</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{data.directions.map((direction) => (
									<TableRow key={direction.provider}>
										<TableCell className="font-medium">{direction.provider}</TableCell>
										<TableCell>{direction.key_count}</TableCell>
										<TableCell>
											<Badge variant="secondary">{direction.status || "unknown"}</Badge>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					)}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2 text-base">
						<Route className="h-4 w-4" />
						Routes
					</CardTitle>
				</CardHeader>
				<CardContent>
					{data.routes.length === 0 ? (
						<p className="text-muted-foreground text-sm">No provider keys found.</p>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Provider</TableHead>
									<TableHead>Key</TableHead>
									<TableHead>Weight</TableHead>
									<TableHead>Status</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{data.routes.map((route) => (
									<TableRow key={`${route.provider}-${route.key_id}`}>
										<TableCell>{route.provider}</TableCell>
										<TableCell className="font-medium">{route.key_name || route.key_id}</TableCell>
										<TableCell className="font-mono text-xs">{route.weight}</TableCell>
										<TableCell>
											<Badge variant={route.enabled ? "secondary" : "outline"}>{route.status}</Badge>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
