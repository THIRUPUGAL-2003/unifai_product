import React from "react";
import { Globe, AlertTriangle, Bot, AlertCircle, CheckCircle2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { isSiteBlockLog } from "./browserAiLogHelpers";
import { platformBadgeLabel } from "./browserAiFormat";
import type { LogActionBadgeFields } from "./browserAiTypes";

export function logActionBadge(log: LogActionBadgeFields) {
	// Fixed-width chip so Action column stays the same size every row.
	const chip = (className: string, icon: React.ReactNode, label: string) => (
		<Badge className={`inline-flex h-6 w-[108px] shrink-0 items-center justify-center gap-1 px-1.5 text-[10px] font-medium ${className}`}>
			{icon}
			<span className="truncate">{label}</span>
		</Badge>
	);
	if (isSiteBlockLog(log)) {
		return chip("bg-rose-950/80 text-rose-300 border-rose-700/60", <Globe className="h-3 w-3 shrink-0" />, "Site Block");
	}
	if (log.action === "Blocked") {
		return chip("bg-red-950/80 text-red-400 border-red-700/60", <AlertTriangle className="h-3 w-3 shrink-0" />, "Blocked");
	}
	if (log.action === "Bot Answered") {
		return chip("bg-sky-950/80 text-sky-300 border-sky-700/60", <Bot className="h-3 w-3 shrink-0" />, "Bot");
	}
	if (log.action === "Redacted" || log.action === "Warned") {
		return chip("bg-amber-950/80 text-amber-300 border-amber-700/60", <AlertCircle className="h-3 w-3 shrink-0" />, "Redacted");
	}
	return chip("bg-emerald-950/80 text-emerald-400 border-emerald-700/60", <CheckCircle2 className="h-3 w-3 shrink-0" />, "Allowed");
}

export function getPlatformBadge(platform: string) {
	const { label, className } = platformBadgeLabel(platform);
	return (
		<Badge className={`inline-flex h-6 max-w-full items-center border px-2 text-[10px] font-medium ${className}`} title={platform || ""}>
			<span className="truncate">{label}</span>
		</Badge>
	);
}
