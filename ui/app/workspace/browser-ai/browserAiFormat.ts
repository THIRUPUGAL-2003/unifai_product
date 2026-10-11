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
	const raw = (platform || "AI").trim();
	const label = raw.length > 14 ? `${raw.slice(0, 12)}…` : raw || "AI";
	return { label, className: "bg-slate-800 text-slate-300 border-slate-700" };
}
