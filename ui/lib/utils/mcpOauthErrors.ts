/**
 * Human-readable MCP OAuth install / connect errors.
 * Vendors (Adobe IMS, AWS Sign-In, …) often reject UnifAI's DCR redirect —
 * surface next steps instead of raw Go / JSON toasts.
 */

export function mcpOAuthRedirectUri(baseUrl?: string): string {
	const base = (baseUrl || "").replace(/\/+$/, "");
	if (base && !base.startsWith("<")) {
		return `${base}/api/oauth/callback`;
	}
	if (typeof window !== "undefined" && window.location.origin) {
		return `${window.location.origin.replace(/\/+$/, "")}/api/oauth/callback`;
	}
	return "<YOUR_UNIFAI_URL>/api/oauth/callback";
}

/** Providers that commonly reject open Dynamic Client Registration for hosted UnifAI. */
export function oauthLikelyNeedsPreRegisteredClient(connectionUrl?: string, serverName?: string): boolean {
	const hay = `${connectionUrl || ""} ${serverName || ""}`.toLowerCase();
	return (
		hay.includes("adobe") ||
		hay.includes("ims") ||
		hay.includes("amazonaws") ||
		hay.includes("api.aws") ||
		hay.includes("signin.aws") ||
		hay.includes("marketplace-mcp") ||
		hay.includes("atlassian") ||
		hay.includes("microsoft") ||
		hay.includes("azure") ||
		hay.includes("google") ||
		hay.includes("github") ||
		hay.includes("slack") ||
		hay.includes("salesforce") ||
		hay.includes("notion") ||
		hay.includes("hubspot") ||
		hay.includes("canva") ||
		hay.includes("biorender") ||
		hay.includes("chronograph") ||
		hay.includes("localfalcon") ||
		hay.includes("local falcon") ||
		hay.includes("signnow") ||
		hay.includes("linear") ||
		hay.includes("neon") ||
		hay.includes("apollo") ||
		hay.includes("airops") ||
		hay.includes("miro") ||
		hay.includes("figma") ||
		hay.includes("gitlab") ||
		hay.includes("jira") ||
		hay.includes("confluence") ||
		hay.includes("zendesk") ||
		hay.includes("airtable") ||
		hay.includes("sentry") ||
		hay.includes("datadog")
	);
}

export function formatMcpOauthError(raw: string, redirectUri?: string): string {
	const message = (raw || "").trim();
	if (!message) {
		return "OAuth setup failed. Check Client ID and OAuth URLs, then try again.";
	}

	const redirectHint = redirectUri
		? ` Register this exact Redirect URI on the provider: ${redirectUri}`
		: " Register UnifAI's Redirect URI (Settings → MCP → External client URL + /api/oauth/callback) on the provider.";

	const lower = message.toLowerCase();

	if (
		lower.includes("oauth discovery failed") ||
		lower.includes("failed to fetch authorization server metadata") ||
		lower.includes("failed to fetch metadata from any authorization server") ||
		lower.includes("authorize_url could not be discovered") ||
		lower.includes("token_url could not be discovered")
	) {
		return (
			"OAuth endpoints could not be auto-discovered from this server. " +
			"Expand OAuth settings and enter Authorization URL + Token URL from the provider docs " +
			"(and Client ID from an OAuth app you create)." +
			redirectHint
		);
	}

	if (
		lower.includes("dynamic client registration failed") ||
		lower.includes("invalid_client_metadata") ||
		lower.includes("no ims client configuration") ||
		lower.includes("redirect uri")
	) {
		return (
			"This provider does not allow automatic client registration for UnifAI. " +
			"Create an OAuth app in the provider console, set the Redirect URI below, then paste Client ID (and Secret if required) here before Continue." +
			redirectHint
		);
	}

	if (lower.includes("client_id is required when the oauth provider does not support dynamic")) {
		return (
			"Client ID is required for this provider. Create an OAuth app, set the Redirect URI, then paste the Client ID." +
			redirectHint
		);
	}

	if (lower.includes("failed to initiate oauth flow:")) {
		const inner = message.replace(/^failed to initiate oauth flow:\s*/i, "");
		return formatMcpOauthError(inner, redirectUri);
	}

	// Keep short provider errors readable; append redirect when useful.
	if (message.length > 420) {
		return `${message.slice(0, 400)}…${redirectHint}`;
	}
	return message;
}
