import { useEffect, useState } from "react";
import { X } from "lucide-react";
import type { StudioEvent } from "../utils/fileStudio";

export function FileStudioPanel() {
	const [event, setEvent] = useState<StudioEvent | null>(null);

	useEffect(() => {
		const onStudio = (ev: Event) => {
			const detail = (ev as CustomEvent<StudioEvent>).detail;
			if (!detail) return;
			if (detail.phase === "close") {
				setEvent(null);
				return;
			}
			setEvent((prev) => ({ ...prev, ...detail }));
		};
		window.addEventListener("prompt-repo:studio", onStudio);
		return () => window.removeEventListener("prompt-repo:studio", onStudio);
	}, []);

	if (!event || event.phase === "close") return null;

	const running = event.phase === "open" || event.phase === "code" || event.phase === "run";

	return (
		<aside className="absolute top-0 right-0 bottom-0 z-40 flex w-[min(440px,48%)] flex-col border-l border-zinc-800 bg-zinc-950 text-zinc-100 shadow-2xl">
			<div className="flex items-center justify-between border-b border-zinc-800 px-3 py-2">
				<div className="min-w-0">
					<p className="text-[11px] uppercase tracking-wide text-zinc-400">Code</p>
					<p className="truncate text-sm font-medium">{event.title || "File studio"}</p>
				</div>
				<button
					type="button"
					className="rounded p-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
					onClick={() => setEvent(null)}
					aria-label="Close code panel"
				>
					<X className="size-4" />
				</button>
			</div>
			<pre className="custom-scrollbar flex-1 overflow-auto px-3 py-3 font-mono text-[11px] leading-5 whitespace-pre-wrap text-emerald-200">
				{event.code || ""}
			</pre>
			<div className="border-t border-zinc-800 px-3 py-2 text-[11px] text-zinc-300">
				{running ? "Running…" : event.phase === "error" ? "Stopped" : "Finished"}
				{event.log ? ` · ${event.log}` : ""}
			</div>
		</aside>
	);
}
