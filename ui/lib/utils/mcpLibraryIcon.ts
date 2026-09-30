import { MCP_LIBRARY_LOCAL_ICONS } from "@/lib/utils/mcpLibraryLocalIcons";

export const MCP_ICON_FALLBACK = "/images/mcp.svg";

type MCPIconSource = {
	icon_url?: string | null;
	connection_url?: string | null;
	name?: string | null;
	slug?: string | null;
	publisher?: string | null;
};

function normalizeKey(value: string): string {
	return value
		.toLowerCase()
		.replace(/\.mcp$/i, "")
		.replace(/[^a-z0-9]+/g, "")
		.trim();
}

function hostFromURL(url?: string | null): string | null {
	if (!url?.trim()) return null;
	try {
		return new URL(url.trim()).hostname || null;
	} catch {
		return null;
	}
}

function localIconFor(server: MCPIconSource): string | null {
	const candidates = [server.slug, server.name, server.publisher]
		.filter(Boolean)
		.flatMap((value) => {
			const raw = String(value);
			const parts = raw.split(/[@/.\s_-]+/).filter(Boolean);
			return [raw, ...parts];
		});

	for (const candidate of candidates) {
		const key = normalizeKey(candidate);
		if (key && MCP_LIBRARY_LOCAL_ICONS[key]) {
			return MCP_LIBRARY_LOCAL_ICONS[key];
		}
	}
	return null;
}

/** Resolve the best available icon for an MCP library entry. */
export function resolveMCPLibraryIconUrl(server: MCPIconSource): string {
	const explicit = server.icon_url?.trim();
	if (explicit) return explicit;

	const local = localIconFor(server);
	if (local) return local;

	const host = hostFromURL(server.connection_url);
	if (host) {
		return `https://www.google.com/s2/favicons?domain=${encodeURIComponent(host)}&sz=128`;
	}

	return MCP_ICON_FALLBACK;
}
