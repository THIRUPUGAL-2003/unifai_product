import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import { useGetTeamsQuery } from "@/lib/store/apis/governanceApi";
import { useGetRolesQuery } from "@enterprise/lib/store/apis/rbacApi";
import { useGetSCIMConfigQuery, useUpdateSCIMConfigMutation } from "@enterprise/lib/store/apis/scimApi";
import { SCIMConfig } from "@enterprise/lib/types/workspace";
import { Save, UserRoundCog, Copy, Eye, EyeOff } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { getExampleBaseUrl } from "@/lib/utils/port";

export default function SCIMView() {
	const { data, isLoading: loading } = useGetSCIMConfigQuery();
	const { data: rolesData } = useGetRolesQuery();
	const { data: teamsData } = useGetTeamsQuery();
	const [updateConfig, { isLoading: saving }] = useUpdateSCIMConfigMutation();
	const [config, setConfig] = useState<SCIMConfig>({ enabled: false, provider: "okta", config: {} });
	const { copy: copyToClipboard } = useCopyToClipboard();
	const [showToken, setShowToken] = useState(false);
	const bearerToken = config.bearer_token || String(config.config?.bearer_token || "");
	const roleNames = useMemo(() => (rolesData?.roles || []).map((r) => r.name), [rolesData]);
	const teams = useMemo(() => teamsData?.teams || [], [teamsData]);
	const scimBase = useMemo(() => {
		const origin = getExampleBaseUrl() || (typeof window !== "undefined" ? window.location.origin : "");
		return origin ? `${origin}/scim/v2` : "/scim/v2";
	}, []);

	useEffect(() => {
		if (data) setConfig({ ...data, config: data.config || {} });
	}, [data]);

	const setField = (key: string, value: string) => {
		setConfig((current) => ({ ...current, config: { ...current.config, [key]: value } }));
	};

	const save = async () => {
		try {
			const next = await updateConfig(config).unwrap();
			setConfig({ ...next, config: next.config || {} });
			toast.success("SCIM configuration saved");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const handleToggleScim = async (enabled: boolean) => {
		if (!enabled) {
			const nextConfig = { ...config, enabled: false };
			setConfig(nextConfig);
			try {
				const next = await updateConfig(nextConfig).unwrap();
				setConfig({ ...next, config: next.config || {} });
				toast.success("SCIM provisioning disabled and saved");
			} catch (err) {
				setConfig(config);
				toast.error(getErrorMessage(err));
			}
			return;
		}
		setConfig({ ...config, enabled: true });
		toast.info("SCIM enabled. Configure credentials and click Save to persist.");
	};

	if (loading) {
		return <div className="text-muted-foreground p-6 text-sm">Loading SCIM config…</div>;
	}

	return (
		<div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-6">
			<div>
				<h1 className="flex items-center gap-2 text-2xl font-semibold">
					<UserRoundCog className="h-6 w-6" />
					SCIM / User Provisioning
				</h1>
				<p className="text-muted-foreground mt-1 text-sm">
					Connect Okta, Entra, or Keycloak so IdP users appear in{" "}
					<a href="/workspace/governance/users" className="text-teal-400 underline-offset-2 hover:underline">
						Users
					</a>
					. Set a default role below (or send SCIM <code className="text-xs">roles[0].value</code>).
				</p>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">SCIM endpoints (for your IdP)</CardTitle>
					<CardDescription>Configure these in Okta, Entra, or Keycloak when provisioning users into UnifAI.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-3 text-sm">
					<EndpointRow label="Base URL" value={scimBase} onCopy={copyToClipboard} />
					<EndpointRow label="Users endpoint" value={`${scimBase}/Users`} onCopy={copyToClipboard} />
					<EndpointRow label="Groups endpoint" value={`${scimBase}/Groups`} onCopy={copyToClipboard} />
					<EndpointRow label="ServiceProviderConfig" value={`${scimBase}/ServiceProviderConfig`} onCopy={copyToClipboard} />
					<p className="text-muted-foreground text-xs">
						Authentication: Bearer token (set below). Token is required on every SCIM request from your identity provider.
					</p>
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">Provider</CardTitle>
					<CardDescription>Settings persist in the config database as scim_config.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between rounded-lg border p-3">
						<div>
							<Label>Enable SCIM</Label>
							<p className="text-muted-foreground text-xs">Turns on the provisioning configuration for this workspace.</p>
						</div>
						<Switch checked={config.enabled} disabled={saving} onCheckedChange={(enabled) => void handleToggleScim(enabled)} />
					</div>
					<div className="space-y-1">
						<Label>Identity provider</Label>
						<select
							value={config.provider || "okta"}
							onChange={(e) => setConfig({ ...config, provider: e.target.value })}
							className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
						>
							<option value="okta">Okta</option>
							<option value="entra">Microsoft Entra</option>
							<option value="keycloak">Keycloak</option>
						</select>
					</div>
					{config.provider === "okta" && (
						<>
							<Field label="Issuer URL" value={config.config.issuerUrl || ""} onChange={(value) => setField("issuerUrl", value)} />
							<Field label="Client ID" value={config.config.clientId || ""} onChange={(value) => setField("clientId", value)} />
							<Field label="Client secret" type="password" value={config.config.clientSecret || ""} onChange={(value) => setField("clientSecret", value)} />
							<Field label="API token" type="password" value={config.config.apiToken || ""} onChange={(value) => setField("apiToken", value)} />
						</>
					)}
					{config.provider === "entra" && (
						<>
							<Field label="Tenant ID" value={config.config.tenantId || ""} onChange={(value) => setField("tenantId", value)} />
							<Field label="Client ID" value={config.config.clientId || ""} onChange={(value) => setField("clientId", value)} />
							<Field label="Client secret" type="password" value={config.config.clientSecret || ""} onChange={(value) => setField("clientSecret", value)} />
						</>
					)}
					{config.provider === "keycloak" && (
						<>
							<Field label="Issuer URL" value={config.config.issuerUrl || ""} onChange={(value) => setField("issuerUrl", value)} />
							<Field label="Client ID" value={config.config.clientId || ""} onChange={(value) => setField("clientId", value)} />
							<Field label="Client secret" type="password" value={config.config.clientSecret || ""} onChange={(value) => setField("clientSecret", value)} />
							<Field label="Realm" value={config.config.realm || ""} onChange={(value) => setField("realm", value)} />
						</>
					)}
					<div className="space-y-1">
						<Label>Default role for provisioned users</Label>
						<p className="text-muted-foreground text-xs">
							Used when the IdP does not send a SCIM role. Create custom roles under Roles &amp; Permissions first.
						</p>
						<select
							value={config.config.defaultRole || "user"}
							onChange={(e) => setField("defaultRole", e.target.value)}
							className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
							data-testid="scim-default-role"
						>
							{(roleNames.includes(config.config.defaultRole || "user")
								? roleNames
								: [...roleNames, config.config.defaultRole || "user"].filter(Boolean)
							).map((name) => (
								<option key={name} value={name}>
									{name}
								</option>
							))}
							{roleNames.length === 0 ? (
								<>
									<option value="user">user</option>
									<option value="admin">admin</option>
								</>
							) : null}
						</select>
					</div>
					<div className="space-y-1">
						<Label>Default team for provisioned users</Label>
						<p className="text-muted-foreground text-xs">
							New SCIM users will be automatically added to this team. Leave blank to skip auto-assignment.
						</p>
						<select
							value={config.config.defaultTeam || ""}
							onChange={(e) => setField("defaultTeam", e.target.value)}
							className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
							data-testid="scim-default-team"
						>
							<option value="">— None —</option>
							{teams.map((t) => (
								<option key={t.id} value={t.id}>
									{t.name}
								</option>
							))}
						</select>
					</div>
					<div className="space-y-1">
						<Label>SCIM bearer token (for IdP → UnifAI)</Label>
						<div className="flex gap-2">
							<Input
								type={showToken ? "text" : "password"}
								autoComplete="new-password"
								data-1p-ignore="true"
								data-lpignore="true"
								value={bearerToken}
								onChange={(e) => {
									setConfig((current) => ({
										...current,
										bearer_token: e.target.value,
										config: { ...current.config, bearer_token: e.target.value },
									}));
								}}
								placeholder="Auto-generated on save when empty"
							/>
							{bearerToken && bearerToken !== "********" && (
								<>
									<Button
										type="button"
										variant="outline"
										size="icon"
										onClick={() => setShowToken((v) => !v)}
										aria-label={showToken ? "Hide token" : "Show token"}
										data-testid="scim-token-toggle"
									>
										{showToken ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
									</Button>
									<Button
										type="button"
										variant="outline"
										size="icon"
										onClick={() => copyToClipboard(bearerToken)}
										aria-label="Copy token"
										data-testid="scim-token-copy"
									>
										<Copy className="h-4 w-4" />
									</Button>
								</>
							)}
						</div>
					</div>
					<div className="flex justify-end">
						<Button onClick={() => void save()} disabled={saving}>
							<Save className="h-4 w-4" />
							{saving ? "Saving…" : "Save SCIM config"}
						</Button>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}

function EndpointRow({ label, value, onCopy }: { label: string; value: string; onCopy: (text: string) => void }) {
	return (
		<div className="flex items-center justify-between gap-3 rounded-lg border p-3">
			<div className="min-w-0">
				<p className="text-muted-foreground text-xs">{label}</p>
				<p className="truncate font-mono text-xs">{value}</p>
			</div>
			<Button size="icon" variant="ghost" onClick={() => onCopy(value)} title="Copy">
				<Copy className="h-4 w-4" />
			</Button>
		</div>
	);
}

function Field({
	label,
	value,
	onChange,
	type = "text",
}: {
	label: string;
	value: string;
	onChange: (value: string) => void;
	type?: string;
}) {
	return (
		<div className="space-y-1">
			<Label>{label}</Label>
			<Input
				type={type}
				autoComplete={type === "password" ? "new-password" : "off"}
				value={value}
				onChange={(e) => onChange(e.target.value)}
			/>
		</div>
	);
}
