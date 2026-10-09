import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import {
	useGetSCIMConfigQuery,
	useUpdateSCIMConfigMutation,
	useGetActiveDirectoryConfigQuery,
	useUpdateActiveDirectoryConfigMutation,
	useTestActiveDirectoryConnectionMutation,
	useSyncActiveDirectoryUsersMutation,
	ActiveDirectoryConfig,
	ADTestResponse,
} from "@enterprise/lib/store/apis/scimApi";
import { SCIMConfig } from "@enterprise/lib/types/workspace";
import {
	Save,
	UserRoundCog,
	Copy,
	Eye,
	EyeOff,
	Server,
	Cloud,
	RefreshCw,
	CheckCircle2,
	AlertCircle,
	Shield,
	PlugZap,
	Users,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { PRODUCT_NAME } from "@/lib/constants/config";
import { getExampleBaseUrl } from "@/lib/utils/port";

export default function SCIMView() {
	// Top Navigation Tabs
	const [activeTab, setActiveTab] = useState<"scim" | "active_directory">("scim");

	// ==========================================
	// 1. SCIM (Cloud Providers) State & Queries
	// ==========================================
	const { data: scimData, isLoading: scimLoading, isError: scimIsError, error: scimError, refetch: refetchScim } = useGetSCIMConfigQuery();
	const [updateSCIMConfig, { isLoading: scimSaving }] = useUpdateSCIMConfigMutation();
	const [config, setConfig] = useState<SCIMConfig>({ enabled: false, provider: "okta", config: {} });
	const { copy: copyToClipboard } = useCopyToClipboard();
	const [showToken, setShowToken] = useState(false);
	const bearerToken = config.bearer_token || String(config.config?.bearer_token || "");
	const scimBase = useMemo(() => {
		const origin = getExampleBaseUrl() || (typeof window !== "undefined" ? window.location.origin : "");
		return origin ? `${origin}/scim/v2` : "/scim/v2";
	}, []);

	useEffect(() => {
		if (scimData) setConfig({ ...scimData, config: scimData.config || {} });
	}, [scimData]);

	const setField = (key: string, value: string) => {
		setConfig((current) => ({ ...current, config: { ...current.config, [key]: value } }));
	};

	const saveScim = async () => {
		try {
			const next = await updateSCIMConfig(config).unwrap();
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
				const next = await updateSCIMConfig(nextConfig).unwrap();
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

	// ==========================================
	// 2. Active Directory (LDAP) State & Queries
	// ==========================================
	const {
		data: adData,
		isLoading: adLoading,
		isError: adIsError,
		error: adError,
		refetch: refetchAD,
	} = useGetActiveDirectoryConfigQuery();
	const [updateADConfig, { isLoading: adSaving }] = useUpdateActiveDirectoryConfigMutation();
	const [testADConnection, { isLoading: adTesting }] = useTestActiveDirectoryConnectionMutation();
	const [syncADUsers, { isLoading: adSyncing }] = useSyncActiveDirectoryUsersMutation();

	const [adConfig, setAdConfig] = useState<ActiveDirectoryConfig>({
		enabled: false,
		server_url: "ldap://127.0.0.1:389",
		domain: "",
		base_dn: "",
		bind_dn: "",
		bind_password: "",
		use_tls: false,
		insecure_skip_tls: false,
		user_filter: "(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))",
		username_attr: "sAMAccountName",
		email_attr: "mail",
		display_name_attr: "displayName",
		default_role: "user",
	});

	const [showADPassword, setShowADPassword] = useState(false);
	const [showAdvancedAD, setShowAdvancedAD] = useState(false);
	const [adTestResult, setAdTestResult] = useState<ADTestResponse | null>(null);

	useEffect(() => {
		if (adData) {
			setAdConfig({
				...adData,
				server_url: adData.server_url || "ldap://127.0.0.1:389",
				user_filter: adData.user_filter || "(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))",
				username_attr: adData.username_attr || "sAMAccountName",
				email_attr: adData.email_attr || "mail",
				display_name_attr: adData.display_name_attr || "displayName",
				default_role: adData.default_role || "user",
			});
		}
	}, [adData]);

	const handleSaveAD = async () => {
		try {
			const saved = await updateADConfig(adConfig).unwrap();
			setAdConfig((prev) => ({
				...prev,
				...saved,
				bind_password: saved.bind_password || prev.bind_password,
			}));
			toast.success("Microsoft Active Directory configuration saved");
		} catch (err) {
			toast.error(getErrorMessage(err) || "Failed to save Active Directory configuration");
		}
	};

	const handleTestAD = async () => {
		setAdTestResult(null);
		try {
			const res = await testADConnection(adConfig).unwrap();
			setAdTestResult(res);
			if (res.success) {
				toast.success(res.message);
			} else {
				toast.error(res.message || "Active Directory connection test failed");
			}
		} catch (err) {
			const msg = getErrorMessage(err) || "Active Directory connection test failed";
			setAdTestResult({ success: false, message: msg });
			toast.error(msg);
		}
	};

	const handleSyncAD = async () => {
		try {
			const res = await syncADUsers().unwrap();
			if (res.success) {
				toast.success(res.message);
				void refetchAD();
			} else {
				toast.error(res.message || "Active Directory sync failed");
			}
		} catch (err) {
			toast.error(getErrorMessage(err) || "Failed to trigger Active Directory sync");
		}
	};

	const handleToggleAD = async (enabled: boolean) => {
		const next = { ...adConfig, enabled };
		setAdConfig(next);
		try {
			const saved = await updateADConfig(next).unwrap();
			setAdConfig((prev) => ({ ...prev, ...saved }));
			toast.success(enabled ? "Active Directory sync enabled" : "Active Directory sync disabled");
		} catch (err) {
			setAdConfig(adConfig);
			toast.error(getErrorMessage(err));
		}
	};

	if (scimLoading && adLoading) {
		return <div className="text-muted-foreground p-6 text-sm">Loading user provisioning configuration…</div>;
	}

	if (scimIsError && activeTab === "scim") {
		return (
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-3 p-6">
				<QueryErrorBanner testId="scim-query-error" message={getErrorMessage(scimError) || "Failed to load SCIM configuration."} />
				<Button type="button" size="sm" variant="outline" className="w-fit" onClick={() => void refetchScim()}>
					Retry
				</Button>
			</div>
		);
	}

	if (adIsError && activeTab === "active_directory") {
		return (
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-3 p-6">
				<QueryErrorBanner testId="ad-query-error" message={getErrorMessage(adError) || "Failed to load Active Directory configuration."} />
				<Button type="button" size="sm" variant="outline" className="w-fit" onClick={() => void refetchAD()}>
					Retry
				</Button>
			</div>
		);
	}

	return (
		<div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-6">
			{/* Top Header */}
			<div>
				<h1 className="flex items-center gap-2 text-2xl font-semibold">
					<UserRoundCog className="h-6 w-6" />
					SCIM / User Provisioning
				</h1>
				<p className="text-muted-foreground mt-1 text-sm">
					Connect Okta, Entra, Keycloak, or on-premises Microsoft Active Directory so directory users appear in{" "}
					<a href="/workspace/governance/users" className="text-teal-400 underline-offset-2 hover:underline">
						Users
					</a>
					. Users are automatically provisioned with the user role; administrators can reassign roles and teams under Governance.
				</p>
			</div>

			{/* Directory Type Tabs */}
			<div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-1 rounded-xl bg-muted/40 border border-border">
				<button
					type="button"
					onClick={() => setActiveTab("scim")}
					className={`flex items-center justify-between gap-3 px-4 py-3 rounded-lg text-left transition-all ${
						activeTab === "scim"
							? "bg-teal-600 text-white shadow-md font-medium"
							: "hover:bg-muted/60 text-muted-foreground"
					}`}
				>
					<div className="flex items-center gap-2.5 min-w-0">
						<Cloud className="h-5 w-5 shrink-0" />
						<div className="truncate">
							<p className="text-xs font-semibold leading-tight">Cloud Identity Providers</p>
							<p className="text-[11px] opacity-80 leading-tight">SCIM 2.0 — Entra, Okta, Keycloak</p>
						</div>
					</div>
					{config.enabled && (
						<span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded font-medium bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">
							Active
						</span>
					)}
				</button>

				<button
					type="button"
					onClick={() => setActiveTab("active_directory")}
					className={`flex items-center justify-between gap-3 px-4 py-3 rounded-lg text-left transition-all ${
						activeTab === "active_directory"
							? "bg-teal-600 text-white shadow-md font-medium"
							: "hover:bg-muted/60 text-muted-foreground"
					}`}
				>
					<div className="flex items-center gap-2.5 min-w-0">
						<Server className="h-5 w-5 shrink-0" />
						<div className="truncate">
							<p className="text-xs font-semibold leading-tight">Microsoft Active Directory</p>
							<p className="text-[11px] opacity-80 leading-tight">On-Premises AD / LDAP Server</p>
						</div>
					</div>
					{adConfig.enabled && (
						<span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded font-medium bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">
							Active
						</span>
					)}
				</button>
			</div>

			{/* =======================================================
			    TAB 1: CLOUD IDP (SCIM 2.0 — ENTRA, OKTA, KEYCLOAK)
			    100% PRESERVED FROM ORIGINAL IMPLEMENTATION
			    ======================================================= */}
			{activeTab === "scim" && (
				<div className="space-y-6">
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
								<Switch checked={config.enabled} disabled={scimSaving} onCheckedChange={(enabled) => void handleToggleScim(enabled)} />
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
								<Button onClick={() => void saveScim()} disabled={scimSaving}>
									<Save className="h-4 w-4" />
									{scimSaving ? "Saving…" : "Save SCIM config"}
								</Button>
							</div>
						</CardContent>
					</Card>
				</div>
			)}

			{/* =======================================================
			    TAB 2: ON-PREMISES MICROSOFT ACTIVE DIRECTORY (LDAP)
			    ======================================================= */}
			{activeTab === "active_directory" && (
				<div className="space-y-6">
					{/* Overview Guide Card */}
					<Card>
						<CardHeader>
							<CardTitle className="text-base flex items-center gap-2">
								<Server className="h-5 w-5 text-teal-400" />
								On-Premises Microsoft Active Directory (LDAP)
							</CardTitle>
							<CardDescription>
								Directly query and sync users from your on-premises Windows Server Domain Controller (Active Directory Domain Services via LDAP/LDAPS).
							</CardDescription>
						</CardHeader>
						<CardContent className="text-muted-foreground space-y-2 text-sm">
							<ol className="list-decimal space-y-1 pl-5">
								<li>Enter the Domain Controller address (e.g. <code>ldap://192.168.1.50:389</code> or <code>ldaps://ad.company.local:636</code>).</li>
								<li>Specify your Domain (e.g. <code>company.local</code>) and Search Base DN (e.g. <code>DC=company,DC=local</code> or <code>OU=Employees,DC=company,DC=local</code>).</li>
								<li>Provide a read-only service account Bind DN and password to query directory users.</li>
								<li>Click <strong>Test Connection</strong> to verify AD bind and search results.</li>
								<li>Click <strong>Sync Users Now</strong> to import discovered Active Directory users directly into {PRODUCT_NAME}.</li>
							</ol>
						</CardContent>
					</Card>

					{/* Server Settings Card */}
					<Card>
						<CardHeader>
							<CardTitle className="text-base">Connection &amp; Credentials</CardTitle>
							<CardDescription>Active Directory Domain Controller connection parameters.</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							{/* Enable AD Sync Switch */}
							<div className="flex items-center justify-between rounded-lg border p-3">
								<div>
									<Label className="text-sm font-medium">Enable Active Directory Sync</Label>
									<p className="text-muted-foreground text-xs">Enables on-premises AD directory lookup and sync operations.</p>
								</div>
								<Switch
									checked={adConfig.enabled}
									disabled={adSaving}
									onCheckedChange={(enabled) => void handleToggleAD(enabled)}
								/>
							</div>

							<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
								<div className="space-y-1">
									<Label>Server URL (LDAP / LDAPS) *</Label>
									<Input
										placeholder="ldap://192.168.1.50:389 or ldaps://ad.company.local:636"
										value={adConfig.server_url}
										onChange={(e) => setAdConfig({ ...adConfig, server_url: e.target.value })}
									/>
									<p className="text-muted-foreground text-[11px]">Standard port: 389 (LDAP), 636 (LDAPS).</p>
								</div>

								<div className="space-y-1">
									<Label>AD Domain</Label>
									<Input
										placeholder="e.g. company.local or corp.example.com"
										value={adConfig.domain}
										onChange={(e) => setAdConfig({ ...adConfig, domain: e.target.value })}
									/>
									<p className="text-muted-foreground text-[11px]">Active Directory forest/domain name.</p>
								</div>
							</div>

							<div className="space-y-1">
								<Label>Base DN (Search Root) *</Label>
								<Input
									placeholder="DC=company,DC=local or OU=Staff,DC=company,DC=local"
									value={adConfig.base_dn}
									onChange={(e) => setAdConfig({ ...adConfig, base_dn: e.target.value })}
								/>
								<p className="text-muted-foreground text-[11px]">Top-level distinguished name under which users are searched.</p>
							</div>

							<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
								<div className="space-y-1">
									<Label>Bind DN (Username) *</Label>
									<Input
										placeholder="CN=Administrator,CN=Users,DC=company,DC=local or admin@company.local"
										value={adConfig.bind_dn}
										onChange={(e) => setAdConfig({ ...adConfig, bind_dn: e.target.value })}
									/>
									<p className="text-muted-foreground text-[11px]">User Principal Name (UPN) or Distinguished Name (DN).</p>
								</div>

								<div className="space-y-1">
									<Label>Bind Password *</Label>
									<div className="flex gap-2">
										<Input
											type={showADPassword ? "text" : "password"}
											autoComplete="new-password"
											placeholder={adConfig.bind_password ? "********" : "Enter AD service account password"}
											value={adConfig.bind_password || ""}
											onChange={(e) => setAdConfig({ ...adConfig, bind_password: e.target.value })}
										/>
										<Button
											type="button"
											variant="outline"
											size="icon"
											onClick={() => setShowADPassword(!showADPassword)}
										>
											{showADPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
										</Button>
									</div>
									<p className="text-muted-foreground text-[11px]">Stored encrypted in database.</p>
								</div>
							</div>

							{/* TLS Options */}
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
								<div className="flex items-center justify-between rounded-lg border p-3">
									<div>
										<Label className="text-xs">Use StartTLS</Label>
										<p className="text-muted-foreground text-[11px]">Upgrade plain LDAP port 389 connection to TLS</p>
									</div>
									<Switch
										checked={!!adConfig.use_tls}
										onCheckedChange={(checked) => setAdConfig({ ...adConfig, use_tls: checked })}
									/>
								</div>

								<div className="flex items-center justify-between rounded-lg border p-3">
									<div>
										<Label className="text-xs">Insecure Skip TLS Verify</Label>
										<p className="text-muted-foreground text-[11px]">Trust internal / self-signed AD certificates</p>
									</div>
									<Switch
										checked={!!adConfig.insecure_skip_tls}
										onCheckedChange={(checked) => setAdConfig({ ...adConfig, insecure_skip_tls: checked })}
									/>
								</div>
							</div>

							{/* Advanced LDAP Configuration */}
							<div className="pt-2">
								<Button
									type="button"
									variant="ghost"
									size="sm"
									onClick={() => setShowAdvancedAD(!showAdvancedAD)}
									className="text-xs text-muted-foreground hover:text-foreground"
								>
									{showAdvancedAD ? "− Hide Advanced LDAP Filter & Attributes" : "+ Show Advanced LDAP Filter & Attributes"}
								</Button>

								{showAdvancedAD && (
									<div className="mt-3 space-y-3 rounded-lg border p-4 bg-muted/20">
										<div className="space-y-1">
											<Label className="text-xs">LDAP User Search Filter</Label>
											<Input
												className="font-mono text-xs"
												placeholder="(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))"
												value={adConfig.user_filter || ""}
												onChange={(e) => setAdConfig({ ...adConfig, user_filter: e.target.value })}
											/>
											<p className="text-muted-foreground text-[11px]">
												Defaults to enabled users filter (filters out disabled user accounts with UAC flag 2).
											</p>
										</div>

										<div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
											<div className="space-y-1">
												<Label className="text-xs">Username Attribute</Label>
												<Input
													className="text-xs"
													placeholder="sAMAccountName"
													value={adConfig.username_attr || ""}
													onChange={(e) => setAdConfig({ ...adConfig, username_attr: e.target.value })}
												/>
											</div>
											<div className="space-y-1">
												<Label className="text-xs">Email Attribute</Label>
												<Input
													className="text-xs"
													placeholder="mail"
													value={adConfig.email_attr || ""}
													onChange={(e) => setAdConfig({ ...adConfig, email_attr: e.target.value })}
												/>
											</div>
											<div className="space-y-1">
												<Label className="text-xs">Display Name Attribute</Label>
												<Input
													className="text-xs"
													placeholder="displayName"
													value={adConfig.display_name_attr || ""}
													onChange={(e) => setAdConfig({ ...adConfig, display_name_attr: e.target.value })}
												/>
											</div>
										</div>
									</div>
								)}
							</div>
						</CardContent>
					</Card>

					{/* Connection Test Results */}
					{adTestResult && (
						<div
							className={`rounded-lg border p-4 ${
								adTestResult.success ? "border-emerald-500/30 bg-emerald-500/10" : "border-red-500/30 bg-red-500/10"
							}`}
						>
							<div className="flex items-start gap-3">
								{adTestResult.success ? (
									<CheckCircle2 className="h-5 w-5 text-emerald-400 mt-0.5 shrink-0" />
								) : (
									<AlertCircle className="h-5 w-5 text-red-400 mt-0.5 shrink-0" />
								)}
								<div className="space-y-1">
									<p className={`text-sm font-medium ${adTestResult.success ? "text-emerald-300" : "text-red-300"}`}>
										{adTestResult.message}
									</p>
									{adTestResult.sample_users && adTestResult.sample_users.length > 0 && (
										<p className="text-xs text-muted-foreground">
											Sample matching users: <code className="text-xs font-mono">{adTestResult.sample_users.join(", ")}</code>
										</p>
									)}
								</div>
							</div>
						</div>
					)}

					{/* Live Sync Status Summary */}
					{adConfig.last_sync_at && (
						<div className="rounded-lg border bg-muted/20 p-4 space-y-2">
							<div className="flex items-center justify-between">
								<div className="flex items-center gap-2">
									<Users className="h-4 w-4 text-teal-400" />
									<span className="text-xs font-medium">Last AD Synchronization</span>
								</div>
								<span className="text-xs text-muted-foreground">
									{new Date(adConfig.last_sync_at).toLocaleString()}
								</span>
							</div>
							<div className="flex items-center justify-between text-xs pt-1">
								<div className="flex items-center gap-2">
									<span>Status:</span>
									<span
										className={`px-2 py-0.5 rounded text-[10px] font-semibold ${
											adConfig.last_sync_status === "success"
												? "bg-emerald-500/20 text-emerald-300 border border-emerald-500/30"
												: "bg-red-500/20 text-red-300 border border-red-500/30"
										}`}
									>
										{adConfig.last_sync_status?.toUpperCase()}
									</span>
								</div>
								<p className="text-muted-foreground text-[11px]">{adConfig.last_sync_message}</p>
							</div>
						</div>
					)}

					{/* Action Buttons */}
					<div className="flex flex-wrap items-center justify-between gap-3 pt-2">
						<div className="flex flex-wrap items-center gap-2">
							<Button
								type="button"
								variant="outline"
								onClick={() => void handleTestAD()}
								disabled={adTesting || adSaving || adSyncing}
								className="gap-2 border-teal-500/30 hover:bg-teal-500/10 text-teal-300"
							>
								<PlugZap className={`h-4 w-4 ${adTesting ? "animate-spin" : ""}`} />
								{adTesting ? "Testing Connection…" : "Test Connection"}
							</Button>

							<Button
								type="button"
								onClick={() => void handleSyncAD()}
								disabled={adSyncing || adTesting || !adConfig.enabled}
								className="gap-2 bg-emerald-600 hover:bg-emerald-500 text-white"
							>
								<RefreshCw className={`h-4 w-4 ${adSyncing ? "animate-spin" : ""}`} />
								{adSyncing ? "Syncing Users…" : "Sync Users Now"}
							</Button>
						</div>

						<Button
							type="button"
							onClick={() => void handleSaveAD()}
							disabled={adSaving || adTesting || adSyncing}
							className="gap-2 bg-teal-500 hover:bg-teal-600 text-white"
						>
							<Save className="h-4 w-4" />
							{adSaving ? "Saving…" : "Save Configuration"}
						</Button>
					</div>
				</div>
			)}
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
