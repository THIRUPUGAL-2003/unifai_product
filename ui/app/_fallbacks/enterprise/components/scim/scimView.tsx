import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import { useGetSCIMConfigQuery, useUpdateSCIMConfigMutation } from "@enterprise/lib/store/apis/scimApi";
import { SCIMConfig } from "@enterprise/lib/types/workspace";
import { Save, UserRoundCog, Copy, Eye, EyeOff } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { PRODUCT_NAME } from "@/lib/constants/config";
import { getExampleBaseUrl } from "@/lib/utils/port";

export default function SCIMView() {
	const { data, isLoading: loading, isError, error, refetch } = useGetSCIMConfigQuery();
	const [updateConfig, { isLoading: saving }] = useUpdateSCIMConfigMutation();
	const [config, setConfig] = useState<SCIMConfig>({ enabled: false, provider: "okta", config: {} });
	const { copy: copyToClipboard } = useCopyToClipboard();
	const [showToken, setShowToken] = useState(false);
	const bearerToken = config.bearer_token || String(config.config?.bearer_token || "");
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

	if (isError) {
		return (
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-3 p-6">
				<QueryErrorBanner testId="scim-query-error" message={getErrorMessage(error) || "Failed to load SCIM configuration."} />
				<Button type="button" size="sm" variant="outline" className="w-fit" onClick={() => void refetch()}>
					Retry
				</Button>
			</div>
		);
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
					. Users are automatically provisioned with the user role; administrators can reassign roles and teams under Governance.
				</p>
			</div>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">Connect chain (IdP → {PRODUCT_NAME})</CardTitle>
					<CardDescription>
						Provisioning is inbound SCIM: the identity provider pushes create / update / deactivate / delete to {PRODUCT_NAME}.
						Optional provider fields below are metadata only — connection requires Enable + bearer token + Tenant URL in the IdP.
					</CardDescription>
				</CardHeader>
				<CardContent className="text-muted-foreground space-y-2 text-sm">
					<ol className="list-decimal space-y-1 pl-5">
						<li>Enable SCIM, pick your IdP, click Save (generates bearer token if empty).</li>
						<li>Copy Base URL + bearer token into the IdP enterprise app / SCIM client.</li>
						<li>
							{config.provider === "entra" && "Entra: Enterprise app → Provisioning → Automatic → Tenant URL = Base URL, Secret Token = bearer."}
							{config.provider === "okta" && "Okta: Applications → Provisioning → Integration → SCIM connector base URL + HTTP Header Authorization."}
							{config.provider === "keycloak" && "Keycloak: Realm → Clients / SCIM or Identity Brokering push → SCIM endpoint + bearer."}
							{!config.provider && "Configure the IdP SCIM client with Base URL + bearer token."}
						</li>
						<li>Assign users/groups in the IdP and start provisioning (or Test Connection).</li>
						<li>
							Create/update syncs into Users; soft-delete sets <code className="text-xs">active=false</code>; hard DELETE removes the local user.
						</li>
					</ol>
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle className="text-base">SCIM endpoints (for your IdP)</CardTitle>
					<CardDescription>Configure these in Okta, Entra, or Keycloak when provisioning users into {PRODUCT_NAME}.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-3 text-sm">
					<EndpointRow label="Base URL (Tenant URL)" value={scimBase} onCopy={copyToClipboard} />
					<EndpointRow label="Users endpoint" value={`${scimBase}/Users`} onCopy={copyToClipboard} />
					<EndpointRow label="Groups endpoint" value={`${scimBase}/Groups`} onCopy={copyToClipboard} />
					<EndpointRow label="ServiceProviderConfig" value={`${scimBase}/ServiceProviderConfig`} onCopy={copyToClipboard} />
					<EndpointRow label="ResourceTypes" value={`${scimBase}/ResourceTypes`} onCopy={copyToClipboard} />
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
					<div className="space-y-1">
						<Label>Default role for provisioned users</Label>
						<select
							value={String(config.config?.defaultRole || config.config?.default_role || "user")}
							onChange={(e) => setField("defaultRole", e.target.value)}
							className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
						>
							<option value="user">user</option>
							<option value="sub_admin">sub_admin</option>
							<option value="admin">admin</option>
						</select>
						<p className="text-muted-foreground text-xs">Used when the IdP does not send a recognised role mapping.</p>
					</div>
					{config.provider === "okta" && (
						<>
							<p className="text-muted-foreground text-xs">Optional notes for your Okta app (not used for inbound SCIM auth).</p>
							<Field label="Issuer URL" value={config.config.issuerUrl || ""} onChange={(value) => setField("issuerUrl", value)} />
							<Field label="Client ID" value={config.config.clientId || ""} onChange={(value) => setField("clientId", value)} />
							<Field label="Client secret" type="password" value={config.config.clientSecret || ""} onChange={(value) => setField("clientSecret", value)} />
							<Field label="API token" type="password" value={config.config.apiToken || ""} onChange={(value) => setField("apiToken", value)} />
						</>
					)}
					{config.provider === "entra" && (
						<>
							<p className="text-muted-foreground text-xs">Optional notes for your Entra enterprise app (not used for inbound SCIM auth).</p>
							<Field label="Tenant ID" value={config.config.tenantId || ""} onChange={(value) => setField("tenantId", value)} />
							<Field label="Client ID" value={config.config.clientId || ""} onChange={(value) => setField("clientId", value)} />
							<Field label="Client secret" type="password" value={config.config.clientSecret || ""} onChange={(value) => setField("clientSecret", value)} />
						</>
					)}
					{config.provider === "keycloak" && (
						<>
							<p className="text-muted-foreground text-xs">Optional notes for your Keycloak realm (not used for inbound SCIM auth).</p>
							<Field label="Issuer URL" value={config.config.issuerUrl || ""} onChange={(value) => setField("issuerUrl", value)} />
							<Field label="Client ID" value={config.config.clientId || ""} onChange={(value) => setField("clientId", value)} />
							<Field label="Client secret" type="password" value={config.config.clientSecret || ""} onChange={(value) => setField("clientSecret", value)} />
							<Field label="Realm" value={config.config.realm || ""} onChange={(value) => setField("realm", value)} />
						</>
					)}

					<div className="space-y-1">
						<Label>SCIM bearer token (for IdP → {PRODUCT_NAME})</Label>
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
