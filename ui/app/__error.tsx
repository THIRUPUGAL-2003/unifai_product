import { Button } from "@/components/ui/button";
import type { ErrorComponentProps } from "@tanstack/react-router";
import { useEffect } from "react";

const CHUNK_RELOAD_KEY = "raksha:chunk-reload-at";

// A redeploy replaces hashed chunks; tabs still running the old bundle fail to
// lazy-load routes until the page is reloaded.
function isChunkLoadError(error: unknown): boolean {
	const message = error instanceof Error ? error.message : String(error ?? "");
	return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|Loading chunk \S+ failed/i.test(
		message,
	);
}

export function ErrorComponent({ error }: Partial<ErrorComponentProps>) {
	const chunkError = isChunkLoadError(error);

	useEffect(() => {
		console.error("[Raksha] route error:", error);
		if (!chunkError) return;
		const last = Number(sessionStorage.getItem(CHUNK_RELOAD_KEY) || 0);
		if (Date.now() - last > 30_000) {
			sessionStorage.setItem(CHUNK_RELOAD_KEY, String(Date.now()));
			window.location.reload();
		}
	}, [error, chunkError]);

	const detail = error instanceof Error ? error.message : "";

	return (
		<main className="h-base flex items-center justify-center p-6">
			<div className="mx-auto w-full max-w-md text-center">
				<p className="text-foreground text-7xl font-bold tracking-tight">500</p>
				<h1 className="text-foreground mt-4 text-2xl font-semibold">Something went wrong</h1>
				<p className="text-muted-foreground mt-2 text-sm">
					{chunkError ? "A new version was deployed. Reload to continue." : "Something went wrong. Please refresh the page."}
				</p>
				{detail && !chunkError && (
					<p className="text-muted-foreground mt-3 break-words font-mono text-xs" data-testid="error-detail">
						{detail}
					</p>
				)}
				<div className="mt-6 flex items-center justify-center gap-3">
					<Button size={"sm"} data-testid="error-reload-btn" onClick={() => window.location.reload()}>
						Reload
					</Button>
				</div>
			</div>
		</main>
	);
}
