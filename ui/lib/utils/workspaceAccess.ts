import {
	getDefaultPathForSections,
	isPathAllowedForUser,
	parseAdminAllowedSections,
} from "@/lib/constants/workspaceSections";
import { DEFAULT_POST_LOGIN_PATH, normalizeLoginGoto } from "@/lib/utils/loginGoto";
import { getApiBaseUrl } from "@/lib/utils/port";

export interface SessionAuth {
	is_auth_enabled: boolean;
	has_valid_token: boolean;
	role?: string;
	allowed_sections?: string;
}

let cachedAuth: { data: SessionAuth | null; timestamp: number } | null = null;
let pendingAuthPromise: Promise<SessionAuth | null> | null = null;
const AUTH_CACHE_TTL_MS = 30_000; // 30 seconds

export function invalidateSessionAuthCache() {
	cachedAuth = null;
	pendingAuthPromise = null;
}

export async function fetchSessionAuth(forceRefresh = false): Promise<SessionAuth | null> {
	const now = Date.now();
	if (!forceRefresh && cachedAuth && now - cachedAuth.timestamp < AUTH_CACHE_TTL_MS) {
		return cachedAuth.data;
	}
	if (!forceRefresh && pendingAuthPromise) {
		return pendingAuthPromise;
	}

	pendingAuthPromise = (async () => {
		try {
			const controller = new AbortController();
			const timeoutId = setTimeout(() => controller.abort(), 3000);
			const res = await fetch(`${getApiBaseUrl()}/session/is-auth-enabled`, {
				credentials: "include",
				signal: controller.signal,
			});
			clearTimeout(timeoutId);
			if (res.ok) {
				const data = await res.json();
				cachedAuth = { data, timestamp: Date.now() };
				return data;
			}
		} catch {
			// fall through
		} finally {
			pendingAuthPromise = null;
		}
		cachedAuth = { data: null, timestamp: Date.now() };
		return null;
	})();

	return pendingAuthPromise;
}

export function getDefaultWorkspacePath(auth: SessionAuth | null | undefined): string {
	if (auth?.role === "user") {
		return "/workspace/prompt-repo";
	}
	if (auth?.role === "admin" || auth?.role === "sub_admin") {
		const limited = parseAdminAllowedSections(auth.allowed_sections);
		if (limited) {
			return getDefaultPathForSections(limited);
		}
	}
	return "/workspace/dashboard";
}

export function resolvePostLoginPath(
	auth: Pick<SessionAuth, "role" | "allowed_sections"> | null | undefined,
	goto?: string | null,
): string {
	const safeGoto = normalizeLoginGoto(goto);
	const defaultPath = auth?.role ? getDefaultWorkspacePath(auth as SessionAuth) : DEFAULT_POST_LOGIN_PATH;

	if (!safeGoto || safeGoto === "/workspace" || safeGoto === "/workspace/") {
		return defaultPath;
	}

	if (auth?.role === "user") {
		if (safeGoto.startsWith("/workspace/prompt-repo")) {
			return safeGoto;
		}
		return defaultPath;
	}

	if (auth?.role === "admin" || auth?.role === "sub_admin") {
		const limited = parseAdminAllowedSections(auth.allowed_sections);
		if (limited) {
			if (isPathAllowedForUser(safeGoto, limited)) {
				return safeGoto;
			}
			return defaultPath;
		}
	}

	return safeGoto;
}

/** Non-null when the current workspace path must be replaced for this session. */
export function getWorkspaceAccessRedirect(
	auth: SessionAuth | null | undefined,
	pathname: string,
): string | null {
	if (auth?.role === "user") {
		if (!pathname.startsWith("/workspace/prompt-repo")) {
			return "/workspace/prompt-repo";
		}
		return null;
	}

	if (auth?.role === "admin" || auth?.role === "sub_admin") {
		const limited = parseAdminAllowedSections(auth.allowed_sections);
		if (limited && !isPathAllowedForUser(pathname, limited)) {
			return getDefaultPathForSections(limited);
		}
	}

	if (pathname === "/workspace" || pathname === "/workspace/") {
		return getDefaultWorkspacePath(auth);
	}

	return null;
}
