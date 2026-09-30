import { MCP_ICON_FALLBACK, resolveMCPLibraryIconUrl } from "@/lib/utils/mcpLibraryIcon";
import type { MCPLibraryEntry } from "@/lib/types/mcp";
import { cn } from "@/lib/utils";

type Props = {
	server: Pick<MCPLibraryEntry, "icon_url" | "connection_url" | "name" | "slug" | "publisher">;
	className?: string;
	imgClassName?: string;
};

/** Shared MCP library icon with local-asset + favicon fallbacks. */
export function MCPLibraryIcon({ server, className, imgClassName }: Props) {
	const src = resolveMCPLibraryIconUrl(server);
	return (
		<div className={cn("bg-background flex shrink-0 items-center justify-center overflow-hidden rounded-md border shadow-xs", className)}>
			<img
				src={src}
				alt=""
				className={cn("h-full w-full object-contain p-1.5", imgClassName)}
				loading="lazy"
				decoding="async"
				referrerPolicy="no-referrer"
				onError={(event) => {
					const img = event.currentTarget;
					const fallbackHost = (() => {
						try {
							return server.connection_url ? new URL(server.connection_url).hostname : "";
						} catch {
							return "";
						}
					})();
					if (fallbackHost && !img.dataset.faviconTried) {
						img.dataset.faviconTried = "1";
						img.src = `https://www.google.com/s2/favicons?domain=${encodeURIComponent(fallbackHost)}&sz=128`;
						return;
					}
					img.onerror = null;
					img.src = MCP_ICON_FALLBACK;
				}}
			/>
		</div>
	);
}
