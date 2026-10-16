/**
 * Port and URL utility - single source of truth for gateway backend connectivity
 *
 * This utility handles:
 * - Development vs Production environment detection
 * - Dynamic port resolution
 * - URL generation for API calls and WebSocket connections
 * - Automatic protocol detection (http/https, ws/wss)
 */

interface PortConfig {
	port: string;
	isDevelopment: boolean;
	baseUrl: string;
	wsUrl: string;
	host: string;
}

/**
 * Gets the current port configuration based on environment
 */
function getPortConfig(): PortConfig {
	const serverDomain = (
		process.env.SERVER_DOMAIN ||
		process.env.GATEWAY_SERVER_DOMAIN ||
		""
	).trim().replace(/\/+$/, "");

	const configuredBackendUrl = (
		process.env.GATEWAY_BACKEND_URL ||
		serverDomain ||
		""
	).trim().replace(/\/+$/, "");

	// In browser, dynamically follow the actual window location
	if (typeof window !== "undefined" && window.location) {
		const isHttps = window.location.protocol === "https:" || serverDomain.startsWith("https:");
		const isRemoteHost = window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1";

		if (isRemoteHost || (serverDomain && !serverDomain.includes("localhost") && !serverDomain.includes("127.0.0.1"))) {
			const targetHost = isRemoteHost ? window.location.host : serverDomain.replace(/^https?:\/\//, "");
			const protocol = isHttps ? "https:" : "http:";
			const wsProtocol = isHttps ? "wss:" : "ws:";
			const port = window.location.port || (isHttps ? "443" : "80");

			return {
				port,
				isDevelopment: process.env.NODE_ENV === "development",
				baseUrl: `${protocol}//${targetHost}`,
				wsUrl: `${wsProtocol}//${targetHost}`,
				host: targetHost,
			};
		}
	}

	const isDevelopment = process.env.NODE_ENV === "development";
	const port = process.env.APP_PORT || process.env.GATEWAY_PORT || (typeof window !== "undefined" ? window.location.port : "") || "";
	const effectiveBackend = configuredBackendUrl || serverDomain || (typeof window !== "undefined" && window.location.origin ? window.location.origin : "");
	const isHttps = effectiveBackend.startsWith("https:");
	const wsProtocol = isHttps ? "wss:" : "ws:";
	const host = effectiveBackend.replace(/^https?:\/\//, "");
	const wsUrl = effectiveBackend ? `${wsProtocol}//${host}` : "";

	return {
		port,
		isDevelopment,
		baseUrl: effectiveBackend,
		wsUrl,
		host,
	};
}

/**
 * Gets the deployment origin for remote installers, agent scripts, and curl one-liners.
 * Precedence:
 * 1. SERVER_DOMAIN from .env (e.g. https://unifai.yespanchi.com)
 * 2. GATEWAY_BACKEND_URL from .env (if non-localhost)
 * 3. window.location.origin in browser
 * 4. getPortConfig().baseUrl
 */
export function getDeploymentOrigin(): string {
	const serverDomain = (
		process.env.SERVER_DOMAIN ||
		process.env.GATEWAY_SERVER_DOMAIN ||
		""
	).trim().replace(/\/+$/, "");
	if (serverDomain && !serverDomain.includes("localhost") && !serverDomain.includes("127.0.0.1")) {
		return serverDomain;
	}
	if (typeof window !== "undefined" && window.location?.origin && window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1") {
		return window.location.origin;
	}
	const backend = (process.env.GATEWAY_BACKEND_URL || "").trim().replace(/\/+$/, "");
	if (backend && !backend.includes("localhost") && !backend.includes("127.0.0.1")) {
		return backend;
	}
	if (serverDomain) return serverDomain;
	if (typeof window !== "undefined" && window.location?.origin) return window.location.origin;
	return getPortConfig().baseUrl || "";
}

/**
 * Get the current port number as a string
 */
export function getPort(): string {
	return getPortConfig().port;
}

/**
 * Get the base URL for API calls (includes protocol and host)
 */
export function getApiBaseUrl(): string {
	const config = getPortConfig();

	if (config.isDevelopment) {
		// Same origin as the Vite page so the session cookie is first-party.
		// Vite proxies /api to the gateway. A direct call to 127.0.0.1 from
		// localhost is cross-site, and the browser drops the login cookie.
		return "/api";
	} else {
		// Production mode: use relative URL for API calls
		return "/api";
	}
}

/**
 * Get the WebSocket URL for real-time connections
 */
export function getWebSocketUrl(path: string = ""): string {
	const config = getPortConfig();
	const cleanPath = path.startsWith("/") ? path : `/${path}`;

	return `${config.wsUrl}${cleanPath}`;
}

/**
 * Get the full base URL (for example code snippets)
 */
export function getExampleBaseUrl(): string {
	return getDeploymentOrigin() || getPortConfig().baseUrl;
}

/**
 * Get the host (hostname:port) for example code
 */
export function getExampleHost(): string {
	const origin = getDeploymentOrigin();
	if (origin) {
		return origin.replace(/^https?:\/\//, "");
	}
	return getPortConfig().host;
}

/**
 * Check if we're in development mode
 */
export function isDevelopmentMode(): boolean {
	return getPortConfig().isDevelopment;
}

/**
 * Generate a complete URL for a specific endpoint
 */
export function getEndpointUrl(endpoint: string): string {
	const config = getPortConfig();
	const cleanEndpoint = endpoint.startsWith("/") ? endpoint : `/${endpoint}`;

	if (config.isDevelopment) {
		return `${config.baseUrl}${cleanEndpoint}`;
	} else {
		// Production mode: use relative URLs
		return cleanEndpoint;
	}
}