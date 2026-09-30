import { IS_ENTERPRISE } from "@/lib/constants/config";
import { UnifAIErrorResponse } from "@/lib/types/config";
import { getApiBaseUrl } from "@/lib/utils/port";
import { createBaseQueryWithRefresh } from "@enterprise/lib/store/utils/baseQueryWithRefresh";
import { clearOAuthStorage } from "@enterprise/lib/store/utils/tokenManager";
import { createApi, fetchBaseQuery } from "@reduxjs/toolkit/query/react";
import { getActiveTempToken, getSuppressGlobal401 } from "./tempToken";

// Auth tokens are now stored in HTTP-only cookies (set by server)
// No client-side token needed — handled by credentials: "include"
export const getTokenFromStorage = (): Promise<string | null> => {
	return Promise.resolve(null);
};

// Helper function to set auth token
// Non-enterprise: no-op — auth relies on HTTPOnly cookies set by the server
// Enterprise: handled separately via tokenManager
export const setAuthToken = (_token: string | null) => {
	// Non-enterprise auth is cookie-based; no client-side token storage needed.
	// Enterprise token management is handled by the tokenManager module.
};

// Helper function to clear all auth-related storage
export const clearAuthStorage = () => {
	if (typeof window === "undefined") {
		return;
	}
	try {
		// Clear traditional auth token
		localStorage.removeItem("unifai-auth-token");

		// Clear enterprise OAuth tokens using tokenManager
		if (IS_ENTERPRISE) {
			clearOAuthStorage();
		}
	} catch (error) {
		console.error("Error clearing auth storage:", error);
	}
};

// Define the base query with authentication headers
const baseQuery = fetchBaseQuery({
	baseUrl: getApiBaseUrl(),
	credentials: "include",
	prepareHeaders: async (headers) => {
		// Do not force a default Content-Type here. JSON bodies are handled by
		// fetchBaseQuery, while FormData uploads need the browser-generated
		// multipart boundary.
		// Automatically include token from localStorage in Authorization header
		const token = await getTokenFromStorage();
		if (token) {
			headers.set("Authorization", `Bearer ${token}`);
		}
		// Attach a temp token when a TempTokenScope wrapper is mounted. The
		// dashboard cookie (if present) still takes precedence on the server
		// side; the temp token is the fallback that rescues unauthenticated
		// browsers visiting a scoped page.
		const tempToken = getActiveTempToken();
		if (tempToken) {
			headers.set("X-UnifAI-Temp-Token", tempToken);
		}
		return headers;
	},
});

// Wrap base query with enterprise refresh logic (or passthrough for non-enterprise)
const baseQueryWithRefresh = createBaseQueryWithRefresh(baseQuery);

// Enhanced base query with error handling
const baseQueryWithErrorHandling: typeof baseQueryWithRefresh = async (args: any, api: any, extraOptions: any) => {
	// First apply refresh logic (enterprise-specific, handles 401)
	const result = await baseQueryWithRefresh(args, api, extraOptions);

	// Then handle other error types
	if (result.error) {
		const error = result.error as any;

		// Handle 401 for non-enterprise (no refresh available)
		if (error?.status === 401 && !IS_ENTERPRISE) {
			// When a TempTokenScope wrapper is active, the wrapped page handles
			// its own 401 display (an "invalid/expired link" view). Skip the
			// global redirect so the user stays on the page they opened.
			if (getSuppressGlobal401()) {
				return result;
			}
			clearAuthStorage();
			if (typeof window !== "undefined" && !window.location.pathname.includes("/login")) {
				const goto = window.location.pathname + window.location.search;
				window.location.href = `/login?goto=${encodeURIComponent(goto)}`;
			}
			return result;
		}

		// Handle specific error types
		if (error?.status === "FETCH_ERROR") {
			// Network error
			return {
				...result,
				error: {
					...error,
					data: {
						error: {
							message: "Network error: Unable to connect to the server",
						},
					},
				},
			};
		}

		// Handle other errors with proper UnifAIErrorResponse format
		if (error?.data) {
			const errorData = error.data as UnifAIErrorResponse;
			if (errorData.error?.message) {
				return result;
			}
		}

		// Fallback error message
		return {
			...result,
			error: {
				...error,
				data: {
					error: {
						message: "An unexpected error occurred",
					},
				},
			},
		};
	}

	return result;
};

// Create the base API
export const baseApi = createApi({
	reducerPath: "api",
	baseQuery: baseQueryWithErrorHandling,
	tagTypes: [
		"Logs",
		"MCPLogs",
		"Providers",
		"MCPClients",
		"Config",
		"CacheConfig",
		"VirtualKeys",
		"Teams",
		"Customers",
		"Budgets",
		"RateLimits",
		"UsageStats",
		"DebugStats",
		"HealthCheck",
		"DBKeys",
		"ProviderKeys",
		"Models",
		"BaseModels",
		"ModelConfigs",
		"ProviderGovernance",
		"Plugins",
		"SCIMProviders",
		"User",
		"Guardrails",
		"ClusterNodes",
		"Users",
		"GuardrailRules",
		"Roles",
		"Resources",
		"Operations",
		"Permissions",
		"APIKeys",
		"OAuth2Config",
		"RoutingRules",
		"PricingOverrides",
		"MCPToolGroups",
		"AuditLogs",
		"UserGovernance",
		"LargePayloadConfig",
		"LoadBalancerConfig",
		"Folders",
		"Prompts",
		"Versions",
		"Sessions",
		"AccessProfiles",
		"BusinessUnits",
		"PromptDeployments",
		"AuthType",
		"MCPSessions",
		"MCPPerUserHeaderCredentials",
		"MCPLibrary",
		"FeatureFlags",
		"ComplexityAnalyzerConfig",
		"Skills",
		"OAuth2Grants",
		"CircuitBreakerPolicies",
		"CircuitBreakerState",
		"AlertChannels",
		"Connectors",
		"BrowserAiLogs",
		"BrowserAiRules",
		"BrowserAiTargets",
		"BrowserAiControls",
		"BrowserAiAgents",
		"BrowserAiAgentSettings",
	],
	endpoints: () => ({}),
});

// Helper function to extract error message from RTK Query error
export const getErrorMessage = (error: unknown): string => {
	if (error === undefined || error === null) {
		return "An unexpected error occurred";
	}
	if (error instanceof Error) {
		return sanitizeErrorText(error.message);
	}
	if (
		typeof error === "object" &&
		error &&
		"data" in error &&
		error.data &&
		typeof error.data === "object" &&
		"error" in error.data &&
		error.data.error &&
		typeof error.data.error === "object" &&
		"message" in error.data.error &&
		typeof error.data.error.message === "string"
	) {
		const message = error.data.error.message;
		const capitalized = message.charAt(0).toUpperCase() + message.slice(1);
		return sanitizeErrorText(capitalized);
	}
	if (typeof error === "object" && error && "message" in error && typeof error.message === "string") {
		return sanitizeErrorText(error.message);
	}
	return "An unexpected error occurred";
};

/** Strip HTML / Cloudflare dumps and cap length so toasts stay readable. */
function sanitizeErrorText(raw: string): string {
	const text = (raw || "").trim();
	if (!text) {
		return "An unexpected error occurred";
	}
	const lower = text.toLowerCase();
	const looksLikeHtml =
		lower.includes("<!doctype") ||
		lower.includes("<html") ||
		lower.includes("<head") ||
		lower.includes("<body") ||
		lower.includes("<script") ||
		lower.includes("cloudflare") ||
		lower.includes("_status_page_config_") ||
		lower.includes("429_title") ||
		lower.includes("\\u003c") ||
		lower.includes("\u003c");

	const statusMatch = text.match(/(?:status|http)[^\d]{0,12}(\d{3})/i);
	const status = statusMatch ? Number(statusMatch[1]) : NaN;

	if (looksLikeHtml || text.length > 500) {
		if (status === 429) {
			return "Upstream returned HTTP 429 (rate limited or blocked) — this URL is not a usable MCP endpoint, or authentication is required";
		}
		if (status === 401 || status === 403) {
			return `Upstream returned HTTP ${status} (unauthorized) — switch Authentication to Headers or OAuth and add a valid API key / token`;
		}
		if (Number.isFinite(status) && status >= 400) {
			return `Upstream returned HTTP ${status} — check the MCP server URL and authentication`;
		}
		return "Upstream returned a non-MCP response (HTML or oversized body) — use the real MCP endpoint URL from the provider docs, not a website homepage";
	}

	const mcpClean = sanitizeMCPConnectErrorText(text);
	if (mcpClean) {
		return mcpClean;
	}

	if (text.length > 280) {
		return `${text.slice(0, 280).trim()}…`;
	}
	return text;
}

/** Collapse nested "failed to connect MCP client …" wrappers into one clear toast. */
function sanitizeMCPConnectErrorText(text: string): string | null {
	const lower = text.toLowerCase();
	if (!lower.includes("mcp client") && !lower.includes("waiting for endpoint") && !lower.includes("mcp")) {
		return null;
	}
	if (
		!lower.includes("failed to connect") &&
		!lower.includes("waiting for endpoint") &&
		!lower.includes("timeout") &&
		!lower.includes("connection refused")
	) {
		return null;
	}

	const nameMatch =
		text.match(/failed to connect MCP client\s+'([^']+)'/i) || text.match(/failed to connect MCP client\s+([A-Za-z0-9_.-]+)/i);
	const name = nameMatch?.[1]?.trim() || "";

	let root = text;
	for (let i = 0; i < 8; i++) {
		const next = root
			.replace(/(?:failed to connect MCP client(?:\s+'[^']+'|\s+[A-Za-z0-9_.-]+)?\s*:\s*)+/gi, "")
			.replace(/failed to start MCP client transport(?: after \d+ retries)?\s*:\s*/gi, "")
			.trim();
		if (next === root) break;
		root = next;
	}

	const rootLower = root.toLowerCase();
	let detail = root;
	if (rootLower.includes("waiting for endpoint") || (rootLower.includes("timeout") && rootLower.includes("endpoint"))) {
		detail =
			"endpoint timed out — check the MCP URL is reachable, the server is running, and auth (if required) is configured";
	} else if (rootLower.includes("connection refused")) {
		detail = "connection refused — nothing is listening at that host/port";
	} else if (rootLower.includes("no such host") || rootLower.includes("name resolution")) {
		detail = "DNS lookup failed — check the MCP hostname";
	} else if (rootLower.includes("deadline exceeded") || rootLower.includes("context deadline")) {
		detail = "connection timed out — check the MCP URL, network, and authentication";
	} else if (root.length > 220) {
		detail = `${root.slice(0, 220).trim()}…`;
	}

	if (name) {
		return `Could not connect to MCP client "${name}": ${detail}`;
	}
	if (detail !== text) {
		return detail.charAt(0).toUpperCase() + detail.slice(1);
	}
	return null;
}