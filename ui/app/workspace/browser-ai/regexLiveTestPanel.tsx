import React, { useMemo, useState } from "react";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { re2ToJsRegExp } from "@/lib/utils/re2";

/** Try a prompt against the regex that will be saved. Shows the rule action, not only match/no match. */
export function RegexLiveTestPanel({ pattern, action }: { pattern: string; action?: string }) {
	const [sample, setSample] = useState("");
	const result = useMemo(() => {
		const p = (pattern || "").trim();
		const s = sample;
		if (!p) return { hit: false as const, msg: "Enter a regex first, then paste a prompt here." };
		if (!s.trim()) return { hit: false as const, msg: "Paste a prompt to see if this regex would catch it." };
		try {
			const re = re2ToJsRegExp(p);
			const m = re.exec(s);
			if (!m) {
				return { hit: false as const, msg: "ALLOWED — this prompt does not match the regex." };
			}
			const act = (action || "BLOCK").toUpperCase();
			const matched = m[0];
			if (act === "REDACT") {
				const globalRe = new RegExp(re.source, re.flags.includes("g") ? re.flags : `${re.flags}g`);
				const redacted = s.replace(globalRe, "[REDACTED]");
				return { hit: true as const, msg: `REDACT — matched “${matched}”.\nPrompt sent on:\n${redacted}` };
			}
			if (act === "WARN") {
				return { hit: true as const, msg: `WARN — matched “${matched}”. The prompt is allowed, and a warning is shown.` };
			}
			return { hit: true as const, msg: `BLOCK — matched “${matched}”. This prompt would be blocked.` };
		} catch (e) {
			return { hit: false as const, msg: `Invalid regex: ${e instanceof Error ? e.message : "error"}` };
		}
	}, [pattern, sample, action]);

	return (
		<div className="space-y-2 rounded-lg border border-border bg-muted/20 p-3">
			<Label className="text-xs">Test this regex with a prompt</Label>
			<Textarea
				placeholder="Paste a prompt, for example: my post code is 613002"
				value={sample}
				onChange={(e) => setSample(e.target.value)}
				rows={3}
				className="text-xs"
			/>
			<p className={`whitespace-pre-wrap text-[11px] font-medium ${result.hit ? "text-emerald-700 dark:text-emerald-400" : "text-amber-800 dark:text-amber-300"}`}>
				{result.msg}
			</p>
		</div>
	);
}
