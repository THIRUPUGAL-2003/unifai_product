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
	const isDevelopment = process.env.NODE_ENV === "development";

	if (isDevelopment) {
		// Development mode: Vite dev server connects to configured backend
		const configuredBackendUrl = (
			process.env.GATEWAY_BACKEND_URL ||
			""
		).trim();
		const port = process.env.GATEWAY_PORT || process.env.APP_PORT || "6000";
		const baseUrl = configuredBackendUrl || `http://localhost:${port}`;
		const isHttps = baseUrl.startsWith("https:");
		const wsProtocol = isHttps ? "wss:" : "ws:";
		const host = baseUrl.replace(/^https?:\/\//, "");
		const wsUrl = `${wsProtocol}//${host}`;

		return {
			port,
			isDevelopment: true,
			baseUrl,
			wsUrl,
			host,
		};
	} else {
		// Production mode: UI is served by the same Go server or under SERVER_DOMAIN
		// Use current window location for automatic port detection
		if (typeof window !== "undefined") {
			const protocol = window.location.protocol === "https:" ? "https:" : "http:";
			const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";

			return {
				port: window.location.port || (window.location.protocol === "https:" ? "443" : "80"),
				isDevelopment: false,
				baseUrl: `${protocol}//${window.location.host}`,
				wsUrl: `${wsProtocol}//${window.location.host}`,
				host: window.location.host,
			};
		} else {
			// Server-side rendering fallback
			const serverDomain = (process.env.SERVER_DOMAIN || "").trim().replace(/\/+$/, "");
			return {
				port: process.env.APP_PORT || "6000",
				isDevelopment: false,
				baseUrl: serverDomain || "",
				wsUrl: "",
				host: "",
			};
		}
	}
}

/**
 * Gets the deployment origin for remote installers, agent scripts, and curl one-liners.
 * Precedence:
 * 1. SERVER_DOMAIN from .env (e.g. https://unifai.yespanchi.com)
 * 2. GATEWAY_BACKEND_URL from .env
 * 3. window.location.origin in browser
 * 4. getPortConfig().baseUrl
 */
export function getDeploymentOrigin(): string {
	if (typeof process !== "undefined" && process.env) {
		const domain = (process.env.SERVER_DOMAIN || "").trim().replace(/\/+$/, "");
		if (domain) return domain;
		const backend = (process.env.GATEWAY_BACKEND_URL || "").trim().replace(/\/+$/, "");
		if (backend) return backend;
	}
	if (typeof window !== "undefined" && window.location?.origin) {
		return window.location.origin;
	}
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
		return `${config.baseUrl}/api`;
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