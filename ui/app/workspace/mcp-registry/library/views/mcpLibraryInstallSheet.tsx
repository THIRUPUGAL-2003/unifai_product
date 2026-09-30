import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SecretVarInput } from "@/components/ui/secretVarInput";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { HeadersTable } from "@/components/ui/headersTable";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { useToast } from "@/hooks/use-toast";
import { getErrorMessage, useCreateMCPClientMutation, useGetCoreConfigQuery } from "@/lib/store";
import { CreateMCPClientRequest, SecretVar, MCPAuthType, MCPLibraryEntry, MCPTLSConfig } from "@/lib/types/mcp";
import { parseArrayFromText } from "@/lib/utils/array";
import {
	formatMcpOauthError,
	mcpOAuthRedirectUri,
	oauthLikelyNeedsPreRegisteredClient,
} from "@/lib/utils/mcpOauthErrors";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { Check, Copy, Globe, Info, KeyRound, Radio, ShieldCheck, Terminal } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { MCPHeadersAuthorizer } from "../../views/mcpHeadersAuthorizer";
import { OAuth2Authorizer } from "../../views/oauth2Authorizer";
import { getExternalBaseUrl } from "../../views/mcpUsageGuide/utils";
import { MCPLibraryIcon } from "./mcpLibraryIcon";

interface MCPLibraryInstallSheetProps {
	server: MCPLibraryEntry;
	open: boolean;
	onClose: () => void;
	onInstalled: () => void;
}

const emptySecretVar: SecretVar = { value: "", ref: "" };

/** Strips empty TLS config so we don't send `{}` to the server. */
function buildTLSConfigPayload(tls: MCPTLSConfig | undefined): MCPTLSConfig | undefined {
	if (!tls) return undefined;
	const hasSkipVerify = tls.insecure_skip_verify === true;
	const hasCACert = tls.ca_cert_pem?.value?.trim() || tls.ca_cert_pem?.ref?.trim();
	if (!hasSkipVerify && !hasCACert) return undefined;
	return { insecure_skip_verify: tls.insecure_skip_verify, ca_cert_pem: hasCACert ? tls.ca_cert_pem : undefined };
}

/**
 * Sanitize a catalog server name into a valid MCP client name. The backend
 * only allows [a-zA-Z0-9_] and disallows a leading digit, so we slugify by
 * replacing any run of invalid characters with a single underscore. Case is
 * preserved (e.g. "Maxim AI" -> "Maxim_AI").
 */
export function sanitizeServerName(name: string): string {
	const cleaned = name
		.trim()
		.replace(/[^a-zA-Z0-9_]+/g, "_")
		.replace(/^_+|_+$/g, "");
	// Prefix if it ends up empty or starts with a digit (leading-digit is rejected).
	return /^[0-9]/.test(cleaned) || cleaned === "" ? `mcp_${cleaned}` : cleaned;
}

/** True when an installed client name is the catalog name or a numbered duplicate (canva, canva2, canva_2). */
export function mcpClientNameMatchesCatalog(clientName: string, catalogName: string): boolean {
	const client = (clientName || "").trim().toLowerCase();
	const base = sanitizeServerName(catalogName).toLowerCase();
	if (!client || !base) return false;
	if (client === base) return true;
	// Migration / reinstall suffixes: canva2, canva3, canva_2, canva_3
	return new RegExp(`^${base.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}[_]?\\d+$`).test(client);
}

function inferLibraryAuthType(server: MCPLibraryEntry): MCPAuthType {
	const catalog = (server.auth_type || "none") as MCPAuthType;
	if (catalog !== "none") return catalog;
	const url = (server.connection_url || "").toLowerCase();
	// Supabase edge functions and similar gateways reject anonymous MCP init with 401.
	if (url.includes("supabase.co") || url.includes("/functions/v1/")) {
		return "headers";
	}
	return "none";
}

function hasFilledHeaders(headers?: Record<string, SecretVar> | null): boolean {
	if (!headers) return false;
	return Object.values(headers).some((h) => Boolean(h?.value?.trim() || h?.ref?.trim()));
}

function buildInitialValues(server: MCPLibraryEntry): CreateMCPClientRequest {
	const authType = inferLibraryAuthType(server);
	const isStdio = server.connection_type === "stdio";
	const requiredKeys = server.required_header_keys?.length
		? server.required_header_keys
		: authType === "headers"
			? ["Authorization"]
			: [];
	const headers =
		authType === "headers"
			? Object.fromEntries(requiredKeys.map((key) => [key, { value: "", ref: "" }]))
			: undefined;
	return {
		name: sanitizeServerName(server.name),
		is_code_mode_client: false,
		is_ping_available: true,
		connection_type: server.connection_type || "http",
		connection_string: isStdio ? undefined : server.connection_url ? { value: server.connection_url, ref: "" } : emptySecretVar,
		stdio_config: isStdio && server.stdio_config ? server.stdio_config : undefined,
		auth_type: authType,
		headers,
	};
}

function transportLabel(connectionType?: string): string {
	switch (connectionType) {
		case "stdio":
			return "stdio";
		case "sse":
			return "SSE";
		default:
			return "HTTP";
	}
}

function TransportIcon({ connectionType }: { connectionType?: string }) {
	switch (connectionType) {
		case "stdio":
			return <Terminal className="size-3.5" />;
		case "sse":
			return <Radio className="size-3.5" />;
		default:
			return <Globe className="size-3.5" />;
	}
}

function authLabel(authType?: MCPAuthType | string): string {
	switch (authType) {
		case "headers":
			return "Headers";
		case "oauth":
			return "OAuth 2.0";
		case "per_user_oauth":
			return "Per-user OAuth";
		case "per_user_headers":
			return "User headers";
		default:
			return "No auth";
	}
}

function authHelpText(authType?: MCPAuthType | string): string {
	switch (authType) {
		case "headers":
			return "Add the request headers UnifAI should send with each tool call.";
		case "oauth":
			return "Most enterprise MCP servers need an OAuth app you create at the provider. Paste Client ID below, then Continue — empty Client ID only works when the provider allows open Dynamic Client Registration.";
		case "per_user_oauth":
			return "Configure the shared OAuth app (Client ID + Redirect URI), then each user completes their own login.";
		case "per_user_headers":
			return "Declare the header names each caller must supply, then verify a sample set on install.";
		default:
			return "No credentials are required for this catalog entry.";
	}
}

export function MCPLibraryInstallSheet({ server, open, onClose, onInstalled }: MCPLibraryInstallSheetProps) {
	const hasCreateMCPClientAccess = useRbac(RbacResource.MCPGateway, RbacOperation.Create);
	const { toast } = useToast();
	const [createMCPClient] = useCreateMCPClientMutation();
	const { data: unifaiConfig } = useGetCoreConfigQuery({ fromDB: true }, { skip: !open });
	const [isLoading, setIsLoading] = useState(false);
	const [scopesText, setScopesText] = useState("");
	const [envVars, setEnvVars] = useState<Record<string, string>>({});
	const [copiedRedirect, setCopiedRedirect] = useState(false);
	const [oauthFlow, setOauthFlow] = useState<{
		authorizeUrl: string;
		oauthConfigId: string;
		mcpClientId: string;
		isPerUserOauth?: boolean;
	} | null>(null);

	// Per-user-headers admin flow: admin declares the required key names,
	// then on install the MCPHeadersAuthorizer dialog runs a sample-values
	// verify and returns discovered tools.
	const [perUserHeaderKeys, setPerUserHeaderKeys] = useState<string[]>([]);
	const [newHeaderKeyInput, setNewHeaderKeyInput] = useState("");
	const [headersFlow, setHeadersFlow] = useState<{ payload: CreateMCPClientRequest } | null>(null);

	// UI splits the canonical `auth_type` into two dropdowns:
	//   - authKind: none | headers | oauth
	//   - authScope: shared | per_user (hidden when authKind = none)
	// They recombine into the wire `auth_type` so the backend contract is
	// unchanged.
	const [authScope, setAuthScope] = useState<"shared" | "per_user">("shared");

	const defaultValues = useMemo(() => buildInitialValues(server), [server]);
	const form = useForm<CreateMCPClientRequest>({ defaultValues });
	const { control, handleSubmit, reset, setValue, watch, setError, clearErrors } = form;
	const authType = watch("auth_type") || "none";
	const headers = watch("headers");

	const authKind: "none" | "headers" | "oauth" =
		authType === "oauth" || authType === "per_user_oauth"
			? "oauth"
			: authType === "headers" || authType === "per_user_headers"
				? "headers"
				: "none";

	const applyAuthKind = (kind: "none" | "headers" | "oauth") => {
		clearErrors();
		if (kind === "none") {
			setValue("auth_type", "none");
			setValue("headers", undefined);
			setValue("oauth_config", undefined);
			return;
		}
		if (kind === "oauth") {
			setValue("auth_type", authScope === "per_user" ? "per_user_oauth" : "oauth");
			setValue("headers", undefined);
			setValue("oauth_config", { client_id: emptySecretVar });
			return;
		}
		setValue("auth_type", authScope === "per_user" ? "per_user_headers" : "headers");
		setValue("oauth_config", undefined);
		setValue("headers", { Authorization: { value: "", ref: "" } });
	};

	const applyAuthScope = (scope: "shared" | "per_user") => {
		setAuthScope(scope);
		if (authKind === "oauth") {
			setValue("auth_type", scope === "per_user" ? "per_user_oauth" : "oauth");
		} else if (authKind === "headers") {
			setValue("auth_type", scope === "per_user" ? "per_user_headers" : "headers");
		}
	};

	// Build initial env vars map from stdio_config.envs
	const initialEnvVars = useMemo(() => {
		if (server.connection_type !== "stdio" || !server.stdio_config?.envs) return {};
		const map: Record<string, string> = {};
		for (const env of server.stdio_config.envs) {
			map[env] = "";
		}
		return map;
	}, [server]);

	useEffect(() => {
		if (!open) return;
		reset(defaultValues);
		setScopesText("");
		setEnvVars(initialEnvVars);
		setOauthFlow(null);
		setHeadersFlow(null);
		setPerUserHeaderKeys([]);
		setNewHeaderKeyInput("");
		setAuthScope("shared");
		setIsLoading(false);
		setCopiedRedirect(false);
	}, [defaultValues, initialEnvVars, open, reset]);

	const oauthRedirectUri = useMemo(
		() => mcpOAuthRedirectUri(getExternalBaseUrl(unifaiConfig?.client_config)),
		[unifaiConfig?.client_config],
	);
	const needsPreRegisteredOauth = useMemo(
		() => oauthLikelyNeedsPreRegisteredClient(server.connection_url, server.name),
		[server.connection_url, server.name],
	);

	const headersValidationError = useMemo(() => {
		if ((authType !== "headers" && authType !== "per_user_headers") || !headers) return null;
		for (const [key, secretVar] of Object.entries(headers)) {
			if (!secretVar.value && !secretVar.ref) {
				return `Header "${key}" must have a value`;
			}
		}
		return null;
	}, [authType, headers]);

	const onSubmit = async (data: CreateMCPClientRequest) => {
		let hasErrors = false;

		if (!data.name.trim()) {
			setError("name", { message: "Server name is required" });
			hasErrors = true;
		} else if (!/^[a-zA-Z0-9_]+$/.test(data.name)) {
			setError("name", {
				message: "Server name can only contain letters, numbers, and underscores",
			});
			hasErrors = true;
		}

		if (authType === "oauth" || authType === "per_user_oauth") {
			if (data.oauth_config?.authorize_url && !/^https?:\/\/.+$/.test(data.oauth_config.authorize_url)) {
				setError("oauth_config.authorize_url", {
					message: "Authorize URL must start with http:// or https://",
				});
				hasErrors = true;
			}
			if (data.oauth_config?.token_url && !/^https?:\/\/.+$/.test(data.oauth_config.token_url)) {
				setError("oauth_config.token_url", {
					message: "Token URL must start with http:// or https://",
				});
				hasErrors = true;
			}
			if (data.oauth_config?.registration_url && !/^https?:\/\/.+$/.test(data.oauth_config.registration_url)) {
				setError("oauth_config.registration_url", {
					message: "Registration URL must start with http:// or https://",
				});
				hasErrors = true;
			}
		}

		if (authType === "per_user_headers") {
			if (perUserHeaderKeys.length === 0) {
				toast({
					title: "Header keys required",
					description: "Declare at least one header name users must supply.",
					variant: "destructive",
				});
				hasErrors = true;
			}
		}

		if (authType === "headers" && !hasFilledHeaders(data.headers)) {
			toast({
				title: "API credentials required",
				description: "This server needs Headers auth. Add a token (e.g. Authorization), then install again.",
				variant: "destructive",
			});
			hasErrors = true;
		}

		if (server.connection_type === "stdio" && server.stdio_config?.envs?.length) {
			const missing = server.stdio_config.envs.filter((name) => !envVars[name]?.trim());
			if (missing.length > 0) {
				toast({
					title: "Environment variables required",
					description: `Fill in: ${missing.join(", ")}`,
					variant: "destructive",
				});
				hasErrors = true;
			}
		}

		if (headersValidationError || hasErrors) return;

		const isStdio = server.connection_type === "stdio";
		const connectionUrl = server.connection_url || "";
		const stdioConfig =
			isStdio && server.stdio_config
				? {
						command: server.stdio_config.command,
						args: server.stdio_config.args || [],
						envs: (server.stdio_config.envs || []).map((name) => {
							const val = envVars[name]?.trim();
							return val ? `${name}=${val}` : name;
						}),
					}
				: undefined;
		const payload: CreateMCPClientRequest = {
			...data,
			connection_type: server.connection_type || "http",
			connection_string: isStdio ? undefined : { value: connectionUrl, ref: "" },
			stdio_config: stdioConfig,
			is_code_mode_client: false,
			is_ping_available: true,
			tls_config: !isStdio ? buildTLSConfigPayload(data.tls_config) : undefined,
			oauth_config:
				authType === "oauth" || authType === "per_user_oauth"
					? {
							client_id: data.oauth_config?.client_id ?? emptySecretVar,
							client_secret:
								data.oauth_config?.client_secret?.value?.trim() || data.oauth_config?.client_secret?.ref?.trim()
									? data.oauth_config.client_secret
									: undefined,
							authorize_url: data.oauth_config?.authorize_url || undefined,
							token_url: data.oauth_config?.token_url || undefined,
							registration_url: data.oauth_config?.registration_url || undefined,
							scopes: scopesText.trim() ? parseArrayFromText(scopesText) : undefined,
							server_url: connectionUrl || undefined,
						}
					: undefined,
			headers:
				(authType === "headers" || authType === "per_user_headers") && data.headers && Object.keys(data.headers).length > 0
					? data.headers
					: undefined,
			per_user_header_keys: authType === "per_user_headers" ? perUserHeaderKeys : undefined,
			tools_to_execute: ["*"],
			// Library install should be usable immediately; Catalog can tighten VK access later.
			allow_on_all_virtual_keys: true,
		};

		// Per-user-headers: stash the payload and open the headers test dialog.
		if (authType === "per_user_headers") {
			setHeadersFlow({ payload });
			return;
		}

		try {
			setIsLoading(true);
			const response = await createMCPClient(payload).unwrap();
			setIsLoading(false);

			if (response.status === "pending_oauth" && response.authorize_url) {
				setOauthFlow({
					authorizeUrl: response.authorize_url,
					oauthConfigId: response.oauth_config_id,
					mcpClientId: response.mcp_client_id,
					isPerUserOauth: authType === "per_user_oauth",
				});
				return;
			}

			toast({
				title: "Installed",
				description: `${server.name} MCP server installed.`,
			});
			onInstalled();
			onClose();
		} catch (error) {
			setIsLoading(false);
			if ((error as any)?.status === 409) {
				const message = getErrorMessage(error);
				setError("name", { message });
				toast({
					title: "Already installed",
					description:
						message ||
						"This MCP server (same name or connection URL) is already installed. Open MCP Catalog and reuse the existing entry — do not create canva2 / canva3 duplicates.",
					variant: "destructive",
				});
				return;
			}
			const message = getErrorMessage(error);
			const needsAuthHint = /\b401\b|\b403\b|unauthorized|authentication required|www-authenticate/i.test(message);
			const isOauthError = /oauth|authorize_url|token_url|client_id|redirect uri|ims|discovery|registration/i.test(message);
			toast({
				title: isOauthError ? "OAuth setup needed" : "Error",
				description: isOauthError
					? formatMcpOauthError(message, oauthRedirectUri)
					: needsAuthHint
						? `${message} — switch Authentication to Headers (or OAuth) and add a valid API key / token, then try again.`
						: message,
				variant: "destructive",
			});
		}
	};

	const isStdio = server.connection_type === "stdio";
	const isOauth = authType === "oauth" || authType === "per_user_oauth";
	const isPerUserHeaders = authType === "per_user_headers";
	const displayUrl =
		server.connection_url || (server.stdio_config ? `${server.stdio_config.command} ${(server.stdio_config.args || []).join(" ")}` : "—");
	const installButtonLabel = isOauth || isPerUserHeaders ? "Continue" : "Install";

	return (
		<Sheet open={open} onOpenChange={(sheetOpen) => !sheetOpen && !oauthFlow && !headersFlow && onClose()}>
			<SheetContent className="flex w-full flex-col overflow-x-hidden p-0 pt-4 sm:max-w-2xl">
				<SheetHeader className="flex flex-col items-start px-0 py-4" headerClassName="mb-0 sticky px-8 -top-4 bg-card z-10">
					<SheetTitle>Install MCP server</SheetTitle>
					<SheetDescription>Confirm the catalog configuration before adding this server to UnifAI.</SheetDescription>
				</SheetHeader>

				<Form {...form}>
					<form onSubmit={handleSubmit(onSubmit)} className="flex min-h-0 flex-1 flex-col">
						<div className="flex-1 space-y-6 px-8 pt-5 pb-6">
							<section className="border-b pb-5">
								<div className="bg-muted/10 flex items-start gap-3 rounded-sm border p-3">
									<MCPLibraryIcon server={server} className="h-10 w-10 rounded-sm" imgClassName="p-1" />
									<div className="min-w-0 flex-1 space-y-2">
										<div className="min-w-0">
											<p className="truncate text-sm font-medium">{server.name}</p>
											<p className="text-muted-foreground truncate font-mono text-xs">{displayUrl}</p>
										</div>
										<div className="flex min-w-0 flex-wrap items-center gap-1.5">
											<Badge variant="outline" className="bg-background">
												<TransportIcon connectionType={server.connection_type} />
												{transportLabel(server.connection_type)}
											</Badge>
											<Badge variant="outline" className="bg-background">
												<ShieldCheck className="size-3.5" />
												{authLabel(authType)}
											</Badge>
											{server.category && (
												<Badge variant="secondary" className="max-w-full truncate">
													{server.category}
												</Badge>
											)}
										</div>
									</div>
								</div>
							</section>

							<section className="space-y-4">
								<div className="space-y-1">
									<h3 className="text-sm font-medium">Client details</h3>
									<p className="text-muted-foreground text-sm">UnifAI uses this name internally when routing MCP tool calls.</p>
								</div>

								<FormField
									control={control}
									name="name"
									rules={{
										required: "Server name is required",
										minLength: {
											value: 3,
											message: "Server name must be at least 3 characters",
										},
										maxLength: {
											value: 50,
											message: "Server name cannot exceed 50 characters",
										},
										validate: {
											format: (value) => /^[a-zA-Z0-9_]+$/.test(value) || "Server name can only contain letters, numbers, and underscores",
											noLeadingDigit: (value) => !/^[0-9]/.test(value) || "Server name cannot start with a number",
										},
									}}
									render={({ field }) => (
										<FormItem>
											<FormLabel>Server name</FormLabel>
											<FormControl>
												<Input {...field} data-testid="library-mcp-name-input" maxLength={50} />
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>
							</section>

							{isStdio && server.stdio_config?.envs && server.stdio_config.envs.length > 0 && (
								<section className="space-y-4 border-t pt-5">
									<div className="space-y-1">
										<div className="flex items-center gap-2">
											<h3 className="text-sm font-medium">Launch environment</h3>
											<TooltipProvider>
												<Tooltip>
													<TooltipTrigger asChild>
														<Info className="text-muted-foreground h-4 w-4 cursor-help" />
													</TooltipTrigger>
													<TooltipContent className="max-w-xs">
														<p>Leave a value blank to read it from the environment where UnifAI runs.</p>
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										</div>
										<p className="text-muted-foreground text-sm">Values used when UnifAI starts this stdio MCP server.</p>
									</div>
									<HeadersTable
										value={envVars}
										onChange={setEnvVars}
										fixedKeys={server.stdio_config.envs}
										keyPlaceholder="Variable name"
										valuePlaceholder="Value (or host env)"
										label=""
									/>
								</section>
							)}

							<section className="space-y-4 border-t pt-5">
								<div className="space-y-1">
									<div className="flex items-center gap-2">
										<KeyRound className="text-muted-foreground size-4" />
										<h3 className="text-sm font-medium">Authentication</h3>
									</div>
									<p className="text-muted-foreground text-sm">{authHelpText(authType)}</p>
								</div>

								{/* Authentication Type */}
								<FormItem className="w-full">
									<FormLabel>Authentication type</FormLabel>
									<Select value={authKind} onValueChange={(value: "none" | "headers" | "oauth") => applyAuthKind(value)}>
										<FormControl>
											<SelectTrigger className="w-full" data-testid="library-auth-type-select">
												<SelectValue placeholder="Select authentication type" />
											</SelectTrigger>
										</FormControl>
										<SelectContent>
											<SelectItem value="none" data-testid="library-auth-type-none">
												None
											</SelectItem>
											<SelectItem value="headers" data-testid="library-auth-type-headers">
												Headers
											</SelectItem>
											<SelectItem value="oauth" data-testid="library-auth-type-oauth">
												OAuth 2.0
											</SelectItem>
										</SelectContent>
									</Select>
								</FormItem>

								{/* Auth Scope — only meaningful when there's an auth flow */}
								{authKind !== "none" && (
									<FormItem className="w-full">
										<FormLabel>Auth Scope</FormLabel>
										<Select value={authScope} onValueChange={(value: "shared" | "per_user") => applyAuthScope(value)}>
											<FormControl>
												<SelectTrigger className="w-full" data-testid="library-auth-scope-select">
													<SelectValue placeholder="Select auth scope" />
												</SelectTrigger>
											</FormControl>
											<SelectContent>
												<SelectItem value="shared" data-testid="library-auth-scope-shared">
													Shared
												</SelectItem>
												<SelectItem value="per_user" data-testid="library-auth-scope-per-user">
													Per-User
												</SelectItem>
											</SelectContent>
										</Select>
									</FormItem>
								)}

								{authType === "headers" && (
									<FormField
										control={control}
										name="headers"
										render={({ field }) => (
											<FormItem data-testid="library-mcp-headers-table">
												<HeadersTable
													value={field.value || {}}
													onChange={field.onChange}
													keyPlaceholder="Header name"
													valuePlaceholder="Header value"
													label="Headers"
													useSecretVarInput
												/>
												{headersValidationError && <p className="text-destructive text-xs">{headersValidationError}</p>}
												<FormMessage />
											</FormItem>
										)}
									/>
								)}

								{authType === "per_user_headers" && (
									<div className="space-y-4">
										{/* Required header keys (admin schema). End users supply values
										    per-user on install via the MCPHeadersAuthorizer dialog. */}
										<div className="space-y-1">
											<div className="space-y-0.5">
												<div className="text-sm font-medium">Required Headers</div>
												<p className="text-muted-foreground text-sm">
													Comma-separated list of header names each caller must supply when they first use this server (e.g.{" "}
													<code>X-API-Key, X-Tenant-ID</code>). Values are submitted per user - never stored on this server config.
												</p>
											</div>
											<Textarea
												id="library-per-user-header-keys"
												data-testid="library-per-user-header-keys-textarea"
												className="h-24"
												placeholder="X-API-Key, X-Tenant-ID"
												value={newHeaderKeyInput}
												onChange={(e) => {
													setNewHeaderKeyInput(e.target.value);
													setPerUserHeaderKeys(parseArrayFromText(e.target.value));
												}}
											/>
										</div>

										{/* Optional static admin headers (e.g. a fixed tenant header) */}
										<FormField
											control={control}
											name="headers"
											render={({ field }) => (
												<FormItem>
													<HeadersTable
														value={field.value || {}}
														onChange={field.onChange}
														keyPlaceholder="Header name"
														valuePlaceholder="Header value"
														label="Static Headers (optional, applied alongside user values)"
														useSecretVarInput
													/>
													{headersValidationError && <p className="text-destructive text-xs">{headersValidationError}</p>}
													<FormMessage />
												</FormItem>
											)}
										/>
									</div>
								)}

				{isOauth && (
									<div className="space-y-4 rounded-sm border p-4" data-testid="library-oauth-setup">
										<div className="space-y-1">
											<h4 className="text-sm font-medium">OAuth setup</h4>
											<p className="text-muted-foreground text-sm">
												{needsPreRegisteredOauth
													? "This provider usually blocks automatic client registration. Create an OAuth app in the provider console first, then paste Client ID below."
													: "If Continue fails with discovery or registration errors, create an OAuth app at the provider and fill Client ID + URLs here."}
											</p>
										</div>

										<div className="space-y-2">
											<Label htmlFor="library-oauth-redirect-uri">Redirect URI (register on provider)</Label>
											<div className="flex gap-2">
												<Input
													id="library-oauth-redirect-uri"
													readOnly
													value={oauthRedirectUri}
													className="font-mono text-xs"
													data-testid="library-oauth-redirect-uri"
												/>
												<Button
													type="button"
													variant="outline"
													size="icon"
													aria-label="Copy redirect URI"
													data-testid="library-oauth-redirect-copy"
													onClick={async () => {
														try {
															await navigator.clipboard.writeText(oauthRedirectUri);
															setCopiedRedirect(true);
															window.setTimeout(() => setCopiedRedirect(false), 2000);
															toast({ title: "Copied", description: "Redirect URI copied to clipboard." });
														} catch {
															toast({
																title: "Copy failed",
																description: "Select and copy the Redirect URI manually.",
																variant: "destructive",
															});
														}
													}}
												>
													{copiedRedirect ? <Check className="size-4" /> : <Copy className="size-4" />}
												</Button>
											</div>
										</div>

										<FormField
											control={control}
											name="oauth_config.client_id"
											render={({ field }) => (
												<FormItem>
													<div className="flex items-center gap-2">
														<FormLabel>OAuth client ID</FormLabel>
														<TooltipProvider>
															<Tooltip>
																<TooltipTrigger asChild>
																	<Info className="text-muted-foreground h-4 w-4 cursor-help" />
																</TooltipTrigger>
																<TooltipContent className="max-w-xs">
																	<p>
																		{needsPreRegisteredOauth
																			? "Required for most Adobe / AWS / Google-style MCP servers. Empty Client ID only works when the provider allows open DCR for UnifAI."
																			: "Leave empty only if the provider supports Dynamic Client Registration for UnifAI's redirect URI."}
																	</p>
																</TooltipContent>
															</Tooltip>
														</TooltipProvider>
													</div>
													<FormControl>
														<SecretVarInput
															value={field.value}
															onChange={field.onChange}
															placeholder={needsPreRegisteredOauth ? "Required — from provider OAuth app" : "your-client-id (or leave empty for DCR)"}
															data-testid="library-oauth-client-id"
														/>
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>

										<FormField
											control={control}
											name="oauth_config.client_secret"
											render={({ field }) => (
												<FormItem>
													<FormLabel>OAuth client secret</FormLabel>
													<FormControl>
														<SecretVarInput
															value={field.value}
															onChange={field.onChange}
															placeholder="optional for PKCE"
															hideValueWhenEnv
															maskNonEnvValue
															data-testid="library-oauth-client-secret"
														/>
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>

										<Accordion
											type="single"
											collapsible
											className="w-full"
											defaultValue={needsPreRegisteredOauth ? "oauth-endpoints" : undefined}
										>
											<AccordionItem value="oauth-endpoints" className="border-b-0">
												<AccordionTrigger className="py-0" data-testid="library-oauth-advanced-trigger">
													<span className="text-sm font-medium">Authorization / Token URLs (if auto-discovery fails)</span>
												</AccordionTrigger>
												<AccordionContent className="space-y-4 pt-4 pb-0">
													<div className="grid gap-4 sm:grid-cols-2">
														<FormField
															control={control}
															name="oauth_config.authorize_url"
															render={({ field }) => (
																<FormItem>
																	<FormLabel>Authorization URL</FormLabel>
																	<FormControl>
																		<Input
																			{...field}
																			value={field.value ?? ""}
																			onChange={(event) => {
																				field.onChange(event);
																				clearErrors("oauth_config.authorize_url");
																			}}
																			placeholder="https://…/authorize"
																			data-testid="library-oauth-authorize-url"
																		/>
																	</FormControl>
																	<FormMessage />
																</FormItem>
															)}
														/>

														<FormField
															control={control}
															name="oauth_config.token_url"
															render={({ field }) => (
																<FormItem>
																	<FormLabel>Token URL</FormLabel>
																	<FormControl>
																		<Input
																			{...field}
																			value={field.value ?? ""}
																			onChange={(event) => {
																				field.onChange(event);
																				clearErrors("oauth_config.token_url");
																			}}
																			placeholder="https://…/token"
																			data-testid="library-oauth-token-url"
																		/>
																	</FormControl>
																	<FormMessage />
																</FormItem>
															)}
														/>
													</div>

													<FormField
														control={control}
														name="oauth_config.registration_url"
														render={({ field }) => (
															<FormItem>
																<FormLabel>Registration URL (optional)</FormLabel>
																<FormControl>
																	<Input
																		{...field}
																		value={field.value ?? ""}
																		onChange={(event) => {
																			field.onChange(event);
																			clearErrors("oauth_config.registration_url");
																		}}
																		placeholder="Only if provider documents DCR"
																		data-testid="library-oauth-registration-url"
																	/>
																</FormControl>
																<FormMessage />
															</FormItem>
														)}
													/>

													<div className="space-y-2">
														<Label>Scopes</Label>
														<Input
															value={scopesText}
															onChange={(event) => setScopesText(event.target.value)}
															placeholder="read, write, admin"
															data-testid="library-oauth-scopes-input"
														/>
													</div>
												</AccordionContent>
											</AccordionItem>
										</Accordion>
									</div>
								)}

								{/* TLS / Certificate — only for remote (HTTP/SSE) connections */}
								{!isStdio && (
									<Accordion type="single" collapsible className="w-full">
										<AccordionItem value="tls-config" className="border-b-0">
											<AccordionTrigger className="py-0" data-testid="library-tls-config-trigger">
												<span className="text-sm font-medium">TLS / Certificate</span>
											</AccordionTrigger>
											<AccordionContent className="space-y-4 pt-4 pb-0">
												<FormField
													control={control}
													name="tls_config.insecure_skip_verify"
													render={({ field }) => (
														<FormItem className="flex flex-row items-center justify-between rounded-lg border p-4">
															<div className="space-y-0.5">
																<FormLabel>Skip TLS verification</FormLabel>
																<p className="text-muted-foreground text-sm">
																	Disable TLS certificate verification. Use only in trusted isolated environments. Takes priority over CA
																	certificate.
																</p>
															</div>
															<FormControl>
																<Switch
																	checked={field.value ?? false}
																	onCheckedChange={field.onChange}
																	data-testid="library-mcp-tls-insecure-skip-verify"
																/>
															</FormControl>
														</FormItem>
													)}
												/>
												<FormField
													control={control}
													name="tls_config.ca_cert_pem"
													render={({ field }) => (
														<FormItem>
															<FormLabel>CA Certificate (PEM) (Optional)</FormLabel>
															<FormControl>
																<SecretVarInput
																	variant="textarea"
																	placeholder={`-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE----- or env.MCP_CA_CERT_PEM`}
																	className="font-mono text-xs"
																	rows={6}
																	hideValueWhenEnv
																	redactNonEnvValue
																	{...field}
																	value={field.value}
																	data-testid="library-mcp-tls-ca-cert-pem"
																/>
															</FormControl>
															<p className="text-muted-foreground text-sm">
																PEM-encoded CA certificate to trust for MCP server connections (e.g. self-signed or private CA).
															</p>
															<FormMessage />
														</FormItem>
													)}
												/>
											</AccordionContent>
										</AccordionItem>
									</Accordion>
								)}
							</section>
						</div>

						<div className="border-border bg-card sticky bottom-0 z-10 border-t px-8 py-4">
							<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
								<p className="text-muted-foreground text-sm">
									{isOauth
										? "OAuth authorization starts after this step."
										: isPerUserHeaders
											? "Header verification starts after this step."
											: "All discovered tools will be enabled after install."}
								</p>
								<div className="flex justify-end gap-2">
									<Button type="button" variant="outline" onClick={onClose} disabled={isLoading} data-testid="library-install-cancel-btn">
										Cancel
									</Button>
									<TooltipProvider>
										<Tooltip>
											<TooltipTrigger asChild>
												<span className="inline-block">
													<Button
														type="submit"
														disabled={isLoading || !hasCreateMCPClientAccess}
														isLoading={isLoading}
														data-testid="library-install-submit-btn"
													>
														{installButtonLabel}
													</Button>
												</span>
											</TooltipTrigger>
											{!hasCreateMCPClientAccess && (
												<TooltipContent>
													<p>You don't have permission to perform this action</p>
												</TooltipContent>
											)}
										</Tooltip>
									</TooltipProvider>
								</div>
							</div>
						</div>
					</form>
				</Form>
			</SheetContent>

			{oauthFlow && (
				<OAuth2Authorizer
					open={!!oauthFlow}
					onClose={() => setOauthFlow(null)}
					onSuccess={() => {
						toast({
							title: "Installed",
							description: `${server.name} MCP server connected with OAuth.`,
						});
						setOauthFlow(null);
						onInstalled();
						onClose();
					}}
					onError={(error) => {
						toast({
							title: "OAuth Error",
							description: error,
							variant: "destructive",
						});
					}}
					onConflict={(error) => {
						setOauthFlow(null);
						setError("name", { message: error });
					}}
					authorizeUrl={oauthFlow.authorizeUrl}
					oauthConfigId={oauthFlow.oauthConfigId}
					mcpClientId={oauthFlow.mcpClientId}
					isPerUserOauth={oauthFlow.isPerUserOauth}
				/>
			)}

			{/* Per-user-headers create dialog. Collects sample values inline,
			    then calls POST /api/mcp/client once — the server verifies
			    upstream + discovers tools + persists atomically. */}
			{headersFlow && (
				<MCPHeadersAuthorizer
					open={!!headersFlow}
					onClose={() => setHeadersFlow(null)}
					onSuccess={() => {
						setHeadersFlow(null);
						toast({
							title: "Installed",
							description: `${server.name} MCP server connected with per-user headers.`,
						});
						onInstalled();
						onClose();
					}}
					onError={() => {
						/* error toast handled by the dialog itself */
					}}
					onConflict={(error) => {
						setHeadersFlow(null);
						setError("name", { message: error });
					}}
					payload={headersFlow.payload}
					perUserHeaderKeys={perUserHeaderKeys}
				/>
			)}
		</Sheet>
	);
}