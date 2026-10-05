import {
	getDefaultPathForSections,
	hasAnyWorkspaceSection,
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
		if (cachedAuth?.data) {
			return cachedAuth.data;
		}
		cachedAuth = { data: null, timestamp: Date.now() };
		return null;
	})();

	return pendingAuthPromise;
}

export const USER_ROLE_HOME_PATH = "/workspace/prompt-repo";

const PUBLIC_WORKSPACE_PATHS = [
	"/workspace/mcp-sessions/auth",
	"/workspace/mcp-sessions/auth-success",
	"/workspace/mcp-sessions/auth-failed",
	"/workspace/oauth",
];

/** Workspace pages reachable without a session or section grant (OAuth / MCP auth callbacks). */
export function isPublicWorkspacePath(pathname: string): boolean {
	return PUBLIC_WORKSPACE_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

/**
 * Section grants that apply to this session, or null for "no section filter".
 * Admins (and sessions without a role, e.g. auth disabled) are unrestricted.
 * The built-in "user" role is server-locked to Prompt Repository (any other /api
 * path is rejected), so its stored allowed_sections are ignored here.
 * Every other role sees only the sections it was granted — none by default.
 */
export function getScopedWorkspaceSections(
	auth: Pick<SessionAuth, "role" | "allowed_sections"> | null | undefined,
): Set<string> | null {
	if (!auth || !auth.role || auth.role === "admin") {
		return null;
	}
	if (auth.role === "user") {
		const parsed = parseAdminAllowedSections(auth.allowed_sections);
		if (parsed.size > 0) {
			parsed.add("prompt-repository");
			return parsed;
		}
		return new Set(["prompt-repository"]);
	}
	return parseAdminAllowedSections(auth.allowed_sections);
}

/** True for a scoped session whose grants unlock no workspace page at all. */
export function hasNoWorkspaceSections(auth: Pick<SessionAuth, "role" | "allowed_sections"> | null | undefined): boolean {
	const limited = getScopedWorkspaceSections(auth);
	return !!limited && !hasAnyWorkspaceSection(limited);
}

export function getDefaultWorkspacePath(auth: SessionAuth | null | undefined): string {
	const limited = getScopedWorkspaceSections(auth);
	if (auth?.role === "user") {
		if (!limited || isPathAllowedForUser(USER_ROLE_HOME_PATH, limited)) {
			return USER_ROLE_HOME_PATH;
		}
		if (hasAnyWorkspaceSection(limited)) {
			return getDefaultPathForSections(limited);
		}
		return USER_ROLE_HOME_PATH;
	}
	if (limited && hasAnyWorkspaceSection(limited)) {
		return getDefaultPathForSections(limited);
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

	const limited = getScopedWorkspaceSections(auth);
	if (limited) {
		return hasAnyWorkspaceSection(limited) && isPathAllowedForUser(safeGoto, limited) ? safeGoto : defaultPath;
	}

	return safeGoto;
}

/** Non-null when the current workspace path must be replaced for this session. */
export function getWorkspaceAccessRedirect(
	auth: SessionAuth | null | undefined,
	pathname: string,
): string | null {
	if (isPublicWorkspacePath(pathname)) {
		return null;
	}
	let target: string | null = null;

	const limited = getScopedWorkspaceSections(auth);
	if (limited) {
		if (!isPathAllowedForUser(pathname, limited)) {
			target = getDefaultWorkspacePath(auth);
		} else if (pathname === "/workspace" || pathname === "/workspace/") {
			target = getDefaultWorkspacePath(auth);
		}
	} else if (hasNoWorkspaceSections(auth)) {
		// The workspace layout renders a "No sections assigned" panel instead.
		return null;
	} else if (pathname === "/workspace" || pathname === "/workspace/") {
		target = getDefaultWorkspacePath(auth);
	}

	// Grants with no recognised keys resolve to a fallback path that may itself be
	// disallowed; redirecting to the current path would loop forever.
	if (target && target.split("?")[0] === pathname) {
		return null;
	}
	return target;
}
