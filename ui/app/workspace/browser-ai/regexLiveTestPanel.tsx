import React, { useMemo, useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** Live-test admin regex against a sample prompt before save (JS RegExp ≈ RE2 for common DLP). */
export function RegexLiveTestPanel({ pattern }: { pattern: string }) {
	const [sample, setSample] = useState("613002");
	const result = useMemo(() => {
		const p = (pattern || "").trim();
		const s = sample;
		if (!p) return { ok: false as const, msg: "Enter a pattern to test." };
		try {
			let body = p;
			if (body.toLowerCase().startsWith("(?i)")) body = body.slice(4);
			const re = new RegExp(body, "i");
			const m = re.exec(s);
			if (m) {
				return { ok: true as const, msg: `MATCH — would trigger on: “${m[0]}”` };
			}
			return {
				ok: false as const,
				msg: "NO MATCH — this prompt would be Allowed (pattern does not cover this text).",
			};
		} catch (e) {
			return { ok: false as const, msg: `Invalid regex: ${e instanceof Error ? e.message : "error"}` };
		}
	}, [pattern, sample]);

	return (
		<div className="rounded-lg border border-border bg-muted/20 p-3 space-y-2">
			<Label className="text-xs">Test pattern before save</Label>
			<Input
				placeholder="Sample employee prompt (e.g. 613002 or pin 613002)"
				value={sample}
				onChange={(e) => setSample(e.target.value)}
				className="h-8 text-xs"
			/>
			<p className={`text-[11px] font-medium ${result.ok ? "text-emerald-700 dark:text-emerald-400" : "text-amber-800 dark:text-amber-300"}`}>{result.msg}</p>
			<p className="text-[10px] text-muted-foreground">
				Bare digits like <code className="text-[10px]">613002</code> need a pattern that matches digits alone (e.g.{" "}
				<code className="text-[10px]">{"\\b[1-9][0-9]{5}\\b"}</code>), not only “pin/otp …”.
			</p>
		</div>
	);
}
