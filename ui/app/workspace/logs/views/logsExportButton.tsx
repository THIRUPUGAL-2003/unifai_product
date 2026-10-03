import { ExportFormatsDropdown, type ExportFormatsPayload } from "@/components/exportFormatsDropdown";
import { getErrorMessage } from "@/lib/store";
import { useLazyGetLogsQuery } from "@/lib/store/apis/logsApi";
import { useLazyGetMCPLogsQuery } from "@/lib/store/apis/mcpLogsApi";
import type { LogEntry, LogFilters, MCPToolLogEntry, MCPToolLogFilters, Pagination } from "@/lib/types/logs";
import { getRangeForPeriod } from "@/lib/utils/timeRange";
import { useCallback } from "react";

const EXPORT_PAGE_SIZE = 1000;
const EXPORT_MAX_ROWS = 10000;

async function collectPages<T>(fetchPage: (pagination: Pagination) => Promise<T[]>): Promise<T[]> {
	const rows: T[] = [];
	while (rows.length < EXPORT_MAX_ROWS) {
		const limit = Math.min(EXPORT_PAGE_SIZE, EXPORT_MAX_ROWS - rows.length);
		const page = await fetchPage({ limit, offset: rows.length, sort_by: "timestamp", order: "desc" });
		rows.push(...page);
		if (page.length < limit) break;
	}
	return rows;
}

/** Fixed end time so new logs arriving mid-export don't shift page offsets. */
function pinEndTime<F extends { start_time?: string; end_time?: string; period?: string }>(filters: F): F {
	if (filters.period) {
		const { from, to } = getRangeForPeriod(filters.period);
		return { ...filters, period: undefined, start_time: from.toISOString(), end_time: to.toISOString() };
	}
	if (filters.end_time) return filters;
	return { ...filters, end_time: new Date().toISOString() };
}

function subtitleFor(count: number) {
	return count >= EXPORT_MAX_ROWS ? `First ${EXPORT_MAX_ROWS} entries (narrow the filters to export the rest)` : `${count} entries`;
}

const fmtCost = (cost?: number) => (cost == null ? "" : cost.toFixed(6));
const fmtLatency = (ms?: number) => (ms == null ? "" : `${Math.round(ms)}ms`);

export function LlmLogsExportButton({ filters, className }: { filters: LogFilters; className?: string }) {
	const [triggerGetLogs] = useLazyGetLogsQuery();

	const getPayload = useCallback(async (): Promise<ExportFormatsPayload> => {
		const pinned = pinEndTime(filters);
		let rows: LogEntry[];
		try {
			rows = await collectPages(async (pagination) => (await triggerGetLogs({ filters: pinned, pagination }).unwrap()).logs ?? []);
		} catch (err) {
			throw new Error(getErrorMessage(err));
		}
		return {
			filename: "llm-logs",
			title: "LLM Logs",
			subtitle: subtitleFor(rows.length),
			columns: [
				{ key: "time", header: "Time" },
				{ key: "status", header: "Status" },
				{ key: "provider", header: "Provider" },
				{ key: "model", header: "Model" },
				{ key: "type", header: "Type" },
				{ key: "user", header: "User" },
				{ key: "team", header: "Team" },
				{ key: "customer", header: "Customer" },
				{ key: "virtual_key", header: "Virtual Key" },
				{ key: "routing_rule", header: "Routing Rule" },
				{ key: "prompt_tokens", header: "Input Tokens" },
				{ key: "completion_tokens", header: "Output Tokens" },
				{ key: "total_tokens", header: "Total Tokens" },
				{ key: "cost", header: "Cost ($)" },
				{ key: "latency", header: "Latency" },
				{ key: "stop_reason", header: "Stop Reason" },
				{ key: "id", header: "Request ID" },
			],
			rows: rows.map((log) => ({
				time: log.timestamp ? new Date(log.timestamp).toLocaleString() : "",
				status: log.status || "",
				provider: log.provider || "",
				model: log.alias ? `${log.model} (alias ${log.alias})` : log.model || "",
				type: log.object || "",
				user: log.user_name || log.user_id || "",
				team: log.team_names?.join(", ") || log.team_name || "",
				customer: log.customer_names?.join(", ") || log.customer_name || "",
				virtual_key: log.virtual_key_name || log.virtual_key_id || "",
				routing_rule: log.routing_rule_name || "",
				prompt_tokens: String(log.token_usage?.prompt_tokens ?? ""),
				completion_tokens: String(log.token_usage?.completion_tokens ?? ""),
				total_tokens: String(log.token_usage?.total_tokens ?? ""),
				cost: fmtCost(log.cost),
				latency: fmtLatency(log.latency),
				stop_reason: log.stop_reason || "",
				id: log.id,
			})),
			json: rows,
		};
	}, [filters, triggerGetLogs]);

	return <ExportFormatsDropdown getPayload={getPayload} size="sm" className={className} testId="llm-logs-export-trigger" />;
}

export function McpLogsExportButton({ filters, className }: { filters: MCPToolLogFilters; className?: string }) {
	const [triggerGetLogs] = useLazyGetMCPLogsQuery();

	const getPayload = useCallback(async (): Promise<ExportFormatsPayload> => {
		const pinned = pinEndTime(filters);
		let rows: MCPToolLogEntry[];
		try {
			rows = await collectPages(async (pagination) => (await triggerGetLogs({ filters: pinned, pagination }).unwrap()).logs ?? []);
		} catch (err) {
			throw new Error(getErrorMessage(err));
		}
		return {
			filename: "mcp-logs",
			title: "MCP Tool Logs",
			subtitle: subtitleFor(rows.length),
			columns: [
				{ key: "time", header: "Time" },
				{ key: "status", header: "Status" },
				{ key: "server", header: "MCP Server" },
				{ key: "tool", header: "Tool" },
				{ key: "virtual_key", header: "Virtual Key" },
				{ key: "user", header: "User ID" },
				{ key: "team", header: "Team ID" },
				{ key: "customer", header: "Customer ID" },
				{ key: "cost", header: "Cost ($)" },
				{ key: "latency", header: "Latency" },
				{ key: "error", header: "Error" },
				{ key: "llm_request_id", header: "LLM Request ID" },
				{ key: "id", header: "ID" },
			],
			rows: rows.map((log) => ({
				time: log.timestamp ? new Date(log.timestamp).toLocaleString() : "",
				status: log.status || "",
				server: log.server_label || "",
				tool: log.tool_name || "",
				virtual_key: log.virtual_key_name || log.virtual_key_id || "",
				user: log.user_id || "",
				team: log.team_id || "",
				customer: log.customer_id || "",
				cost: fmtCost(log.cost),
				latency: fmtLatency(log.latency),
				error: log.error_details?.error?.message || "",
				llm_request_id: log.llm_request_id || "",
				id: log.id,
			})),
			json: rows,
		};
	}, [filters, triggerGetLogs]);

	return <ExportFormatsDropdown getPayload={getPayload} size="sm" className={className} testId="mcp-logs-export-trigger" />;
}
