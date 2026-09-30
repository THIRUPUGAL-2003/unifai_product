import React from "react";
import { formatLogDate, formatLogDateTimeTitle, formatLogTime } from "./browserAiFormat";

/** Date on first line, time on second — Prompt Logs / Overview / Search Logs. */
export function LogTimestampCell({ timestamp }: { timestamp?: string | number | Date | null }) {
	const date = formatLogDate(timestamp);
	const time = formatLogTime(timestamp);
	const title = formatLogDateTimeTitle(timestamp);
	return (
		<div className="flex flex-col gap-0.5 py-1 font-mono text-xs leading-tight text-muted-foreground" title={title}>
			<span className="whitespace-nowrap text-foreground/80">{date}</span>
			<span className="whitespace-nowrap">{time}</span>
		</div>
	);
}
