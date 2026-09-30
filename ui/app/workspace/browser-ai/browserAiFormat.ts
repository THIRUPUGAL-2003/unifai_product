import type { PlatformBadgeInfo } from "./browserAiTypes";

export function oneLinePreview(text?: string): string {
	return (text || "").replace(/\s+/g, " ").trim();
}

/** Split timestamp for tables: date on one line, time on the next (no truncate mash). */
export function formatLogDate(ts?: string | number | Date | null): string {
	if (ts == null || ts === "") return "—";
	const d = ts instanceof Date ? ts : new Date(ts);
	if (Number.isNaN(d.getTime())) return "—";
	return d.toLocaleDateString(undefined, { year: "numeric", month: "numeric", day: "numeric" });
}

export function formatLogTime(ts?: string | number | Date | null): string {
	if (ts == null || ts === "") return "—";
	const d = ts instanceof Date ? ts : new Date(ts);
	if (Number.isNaN(d.getTime())) return "—";
	return d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit", second: "2-digit" });
}

export function formatLogDateTimeTitle(ts?: string | number | Date | null): string {
	if (ts == null || ts === "") return "";
	const d = ts instanceof Date ? ts : new Date(ts);
	if (Number.isNaN(d.getTime())) return "";
	return d.toLocaleString();
}

export function platformBadgeLabel(platform: string): PlatformBadgeInfo {
	const p = (platform || "").toLowerCase();
	if (p.includes("claude")) return { label: "Claude", className: "bg-purple-950/60 text-purple-300 border-purple-700/60" };
	if (p.includes("chatgpt") || p.includes("openai")) return { label: "ChatGPT", className: "bg-emerald-950/60 text-emerald-300 border-emerald-700/60" };
	// Only real Gemini/Bard — do NOT map every "google" host (Drive/Docs) to Gemini
	if (p.includes("gemini") || p.includes("bard")) return { label: "Gemini", className: "bg-blue-950/60 text-blue-300 border-blue-700/60" };
	if (p.includes("copilot") || (p.includes("microsoft") && !p.includes("google"))) return { label: "Copilot", className: "bg-cyan-950/60 text-cyan-300 border-cyan-700/60" };
	if (p.includes("perplexity")) return { label: "Perplexity", className: "bg-amber-950/60 text-amber-300 border-amber-700/60" };
	if (p.includes("deepseek")) return { label: "DeepSeek", className: "bg-indigo-950/60 text-indigo-300 border-indigo-700/60" };
	const raw = (platform || "AI").trim();
	const label = raw.length > 14 ? `${raw.slice(0, 12)}…` : raw || "AI";
	return { label, className: "bg-slate-800 text-slate-300 border-slate-700" };
}
