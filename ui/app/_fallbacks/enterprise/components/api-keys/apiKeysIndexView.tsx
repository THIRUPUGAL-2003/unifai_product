import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { useGetCoreConfigQuery, useGetGovernanceHealthQuery, useGetVirtualKeysQuery } from "@/lib/store";
import { getExampleBaseUrl } from "@/lib/utils/port";
import { Link } from "@tanstack/react-router";
import { CheckCircle2, Copy, ExternalLink, InfoIcon, Key, Shield } from "lucide-react";
import { useMemo } from "react";

export default function APIKeysView() {
	const { data: unifaiConfig, isLoading } = useGetCoreConfigQuery({ fromDB: true });
	const { data: virtualKeysData, isLoading: loadingKeys } = useGetVirtualKeysQuery({ limit: 20 });
	const { data: health, isFetching: testingHealth, refetch: testHealth } = useGetGovernanceHealthQuery();
	const { copy: copyToClipboard } = useCopyToClipboard();

	const isAuthConfigured = useMemo(() => unifaiConfig?.auth_config?.is_enabled, [unifaiConfig]);
	const isInferenceAuthDisabled = !(unifaiConfig?.client_config?.enforce_auth_on_inference ?? false);
	const baseUrl = getExampleBaseUrl() || (typeof window !== "undefined" ? window.location.origin : "");

	const adminCurlExample = `# 1. Sign in once; the session cookie is stored in cookies.txt
curl --location '${baseUrl}/api/session/login' \\
  --cookie-jar cookies.txt \\
  --header 'Content-Type: application/json' \\
  --data '{ "username": "<username>", "password": "<password>" }'

# 2. Call admin APIs with that session (sign out via /api/session/logout)
curl --location '${baseUrl}/api/governance/virtual-keys' \\
  --cookie cookies.txt`;

	const inferenceCurlExample = (key: string) => `curl --location '${baseUrl}/v1/chat/completions' \\
  --header 'Content-Type: application/json' \\
  --header 'Authorization: Bearer ${key}' \\
  --data '{
    "model": "openai/gpt-4o",
    "messages": [{ "role": "user", "content": "Hello" }]
  }'`;

	if (isLoading) {
		return <div className="text-muted-foreground p-6 text-sm">Loading API key settings…</div>;
	}

	if (!isAuthConfigured) {
		return (
			<Alert variant="default">
				<InfoIcon className="text-muted h-4 w-4" />
				<AlertDescription>
					<p className="text-md text-muted-foreground">
						To use admin APIs, set up admin username and password first.{" "}
						<Link to="/workspace/config/security" className="text-md text-primary underline">
							Configure Security Settings
						</Link>
						.
					</p>
				</AlertDescription>
			</Alert>
		);
	}

	const virtualKeys = virtualKeysData?.virtual_keys || [];

	return (
		<div className="mx-auto w-full max-w-5xl space-y-6">
			<div>
				<h1 className="flex items-center gap-2 text-2xl font-semibold">
					<Key className="h-6 w-6" />
					API Keys
				</h1>
				<p className="text-muted-foreground mt-1 text-sm">
					Session sign-in for dashboard / admin APIs, and Virtual Keys for inference / MCP access.
				</p>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2 text-base">
						<Shield className="h-4 w-4" />
						Admin authentication
					</CardTitle>
					<CardDescription>Used for dashboard and <code>/api/*</code> admin endpoints.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<Alert variant="default">
						<InfoIcon className="text-muted h-4 w-4" />
						<AlertDescription>
							{isInferenceAuthDisabled ? (
								<>
									Inference auth is <strong>disabled</strong> — <code>/v1/*</code> calls work without a key. Admin APIs still require a
									signed-in session (cookie or <code>Authorization: Bearer &lt;session token&gt;</code>); Basic auth is rejected.
								</>
							) : (
								<>
									Inference auth is <strong>enabled</strong>. Use a Virtual Key (Bearer) for <code>/v1/*</code> calls. Admin APIs require
									a signed-in session (cookie or <code>Authorization: Bearer &lt;session token&gt;</code>); Basic auth is rejected.
								</>
							)}
						</AlertDescription>
					</Alert>
					<div className="relative w-full min-w-0 overflow-x-auto">
						<Button variant="ghost" size="sm" onClick={() => copyToClipboard(adminCurlExample)} className="absolute top-2 right-2 z-10 h-8">
							<Copy className="h-4 w-4" />
						</Button>
						<pre className="bg-muted min-w-max rounded p-3 pr-12 font-mono text-sm whitespace-pre">{adminCurlExample}</pre>
					</div>
					<div className="flex items-center gap-3">
						<Button variant="outline" size="sm" onClick={() => void testHealth()} disabled={testingHealth}>
							{testingHealth ? "Testing…" : "Test admin API connection"}
						</Button>
						{health && (
							<span className="text-muted-foreground flex items-center gap-1 text-sm">
								<CheckCircle2 className="h-4 w-4 text-emerald-500" />
								Governance API reachable
							</span>
						)}
					</div>
				</CardContent>
			</Card>

			<Card>
				<CardHeader className="flex flex-row items-center justify-between">
					<div>
						<CardTitle className="text-base">Virtual Keys (inference)</CardTitle>
						<CardDescription>Issue and rotate keys under Governance → Virtual Keys.</CardDescription>
					</div>
					<Button asChild variant="outline" size="sm">
						<Link to="/workspace/governance/virtual-keys">
							Manage keys
							<ExternalLink className="ml-1 h-3.5 w-3.5" />
						</Link>
					</Button>
				</CardHeader>
				<CardContent>
					{loadingKeys ? (
						<p className="text-muted-foreground text-sm">Loading virtual keys…</p>
					) : virtualKeys.length === 0 ? (
						<div className="rounded-lg border border-dashed p-8 text-center">
							<p className="font-medium">No virtual keys yet</p>
							<p className="text-muted-foreground mt-1 text-sm">Create one to authenticate inference and MCP requests.</p>
							<Button asChild className="mt-4" size="sm">
								<Link to="/workspace/governance/virtual-keys">Create virtual key</Link>
							</Button>
						</div>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Name</TableHead>
									<TableHead>Key</TableHead>
									<TableHead>Status</TableHead>
									<TableHead className="text-right">Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{virtualKeys.map((vk) => (
									<TableRow key={vk.id}>
										<TableCell className="font-medium">{vk.name}</TableCell>
										<TableCell className="max-w-[220px] truncate font-mono text-xs" title={vk.value}>
											{vk.value ? `${vk.value.slice(0, 8)}…${vk.value.slice(-4)}` : "—"}
										</TableCell>
										<TableCell>
											<Badge variant={vk.is_active ? "default" : "secondary"}>{vk.is_active ? "Active" : "Inactive"}</Badge>
										</TableCell>
										<TableCell className="text-right">
											<Button
												size="sm"
												variant="ghost"
												onClick={() => copyToClipboard(vk.value)}
												disabled={!vk.value}
												title="Copy key"
											>
												<Copy className="h-4 w-4" />
											</Button>
											<Button
												size="sm"
												variant="ghost"
												onClick={() => copyToClipboard(inferenceCurlExample(vk.value))}
												disabled={!vk.value}
												title="Copy curl example"
											>
												curl
											</Button>
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
