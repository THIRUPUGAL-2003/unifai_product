import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ExportFormatsDropdown } from "@/components/exportFormatsDropdown";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage, useIsAuthEnabledQuery } from "@/lib/store";
import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { useGetAuditLogsQuery, useLazyExportAuditLogsQuery } from "@enterprise/lib/store/apis/auditLogsApi";
import type { AuditLog } from "@enterprise/lib/types/workspace";
import { Check, Copy, Eye, RefreshCw, ScrollText, Search, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { DataTablePagination } from "@/components/table/dataTablePagination";
import { toast } from "sonner";

function formatAuditDate(ts: string) {
	const d = new Date(ts);
	if (Number.isNaN(d.getTime())) return "—";
	return d.toLocaleDateString(undefined, { year: "numeric", month: "numeric", day: "numeric" });
}

function formatAuditTime(ts: string) {
	const d = new Date(ts);
	if (Number.isNaN(d.getTime())) return "—";
	return d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit", second: "2-digit" });
}

function formatPrettyJSON(raw?: string): string {
	if (!raw || !raw.trim()) return "";
	try {
		return JSON.stringify(JSON.parse(raw), null, 2);
	} catch {
		return raw;
	}
}

function getActionBadge(action: string) {
	switch (action.toLowerCase()) {
		case "create":
			return (
				<Badge className="border-emerald-500/30 bg-emerald-500/15 text-emerald-700 dark:text-emerald-400">
					create
				</Badge>
			);
		case "update":
			return (
				<Badge className="border-blue-500/30 bg-blue-500/15 text-blue-700 dark:text-blue-400">
					update
				</Badge>
			);
		case "delete":
			return <Badge variant="destructive">delete</Badge>;
		case "login":
			return (
				<Badge className="border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-400">
					login
				</Badge>
			);
		case "logout":
			return (
				<Badge variant="outline" className="text-muted-foreground">
					logout
				</Badge>
			);
		case "email_sent":
			return (
				<Badge className="border-purple-500/30 bg-purple-500/15 text-purple-700 dark:text-purple-400">
					email_sent
				</Badge>
			);
		default:
			return <Badge variant="secondary">{action}</Badge>;
	}
}

export default function AuditLogsView() {
	const [search, setSearch] = useState("");
	const [action, setAction] = useState("");
	const [outcome, setOutcome] = useState("");
	const [page, setPage] = useState(0);
	const [pageSize, setPageSize] = useState(50);
	const [isLive, setIsLive] = useState(true);
	const [selectedLog, setSelectedLog] = useState<AuditLog | null>(null);
	const [copied, setCopied] = useState(false);

	const filters = useMemo(
		() => ({ search: search.trim() || undefined, action: action || undefined, outcome: outcome || undefined }),
		[search, action, outcome],
	);

	useEffect(() => setPage(0), [filters]);

	const {
		data,
		isLoading: loading,
		isFetching,
		isError,
		error,
		refetch,
	} = useGetAuditLogsQuery(
		{ ...filters, limit: pageSize, offset: page * pageSize },
		{ pollingInterval: isLive ? 3000 : 0 },
	);

	const [exportAuditLogs] = useLazyExportAuditLogsQuery();
	const { data: authStatus } = useIsAuthEnabledQuery();
	const isSuperAdmin = authStatus?.role === "admin";
	const hasDownloadAccess = useRbac(RbacResource.AuditLogs, RbacOperation.Download) || isSuperAdmin;

	const logs = data?.logs || [];
	const total = data?.total_count ?? 0;

	const handleCopyDetail = () => {
		if (!selectedLog?.detail) return;
		navigator.clipboard.writeText(formatPrettyJSON(selectedLog.detail));
		setCopied(true);
		toast.success("Audit detail payload copied to clipboard");
		setTimeout(() => setCopied(false), 2000);
	};

	const getExportPayload = useCallback(async () => {
		let rows;
		try {
			rows = (await exportAuditLogs(filters).unwrap()).logs || [];
		} catch (err) {
			throw new Error(getErrorMessage(err));
		}
		return {
			filename: "audit-logs",
			title: "Audit Logs",
			subtitle: `${rows.length} entries`,
			columns: [
				{ key: "date", header: "Date" },
				{ key: "time", header: "Time" },
				{ key: "action", header: "Action" },
				{ key: "outcome", header: "Outcome" },
				{ key: "initiator", header: "Initiator" },
				{ key: "target", header: "Target" },
				{ key: "path", header: "Path" },
				{ key: "ip", header: "IP" },
				{ key: "duration", header: "Duration" },
				{ key: "detail", header: "Detail" },
			],
			rows: rows.map((log) => ({
				date: formatAuditDate(log.created_at),
				time: formatAuditTime(log.created_at),
				action: log.action || "",
				outcome: log.outcome || "",
				initiator: log.initiator || "",
				target: log.target || "",
				path: `${log.method || ""} ${log.path || ""}`.trim(),
				ip: log.ip || "",
				duration: `${log.duration_ms ?? 0}ms`,
				detail: log.detail || "",
			})),
			json: rows,
		};
	}, [exportAuditLogs, filters]);

	return (
		<div className="flex h-full w-full flex-col gap-4 p-4">
			{/* Header */}
			<div className="flex flex-col justify-between gap-3 md:flex-row md:items-center">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-semibold">
						<ScrollText className="h-6 w-6 text-primary" />
						Audit Logs
					</h1>
					<p className="text-muted-foreground text-sm">
						Live recording of every administrative mutation across the workspace — roles, permissions, access profiles, virtual keys, users, and logins.
					</p>
				</div>
				<div className="flex items-center gap-2">
					<button
						type="button"
						onClick={() => setIsLive(!isLive)}
						className={`flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs font-medium transition-all ${
							isLive
								? "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
								: "border-border bg-muted/40 text-muted-foreground hover:bg-muted/70"
						}`}
						title={isLive ? "Click to pause live polling" : "Click to resume live polling"}
					>
						<span className="relative flex h-2 w-2">
							{isLive && (
								<span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
							)}
							<span
								className={`relative inline-flex h-2 w-2 rounded-full ${isLive ? "bg-emerald-500" : "bg-muted-foreground/60"}`}
							/>
						</span>
						{isLive ? "Live (3s)" : "Paused"}
					</button>

					<Button
						variant="outline"
						size="sm"
						onClick={() => refetch()}
						disabled={isFetching}
						className="h-8 gap-1.5 text-xs"
						title="Refresh logs immediately"
					>
						<RefreshCw className={`h-3.5 w-3.5 ${isFetching ? "animate-spin" : ""}`} />
						Refresh
					</Button>

					<ExportFormatsDropdown
						getPayload={getExportPayload}
						disabled={!hasDownloadAccess || logs.length === 0}
						testId="audit-logs-export-trigger"
					/>
				</div>
			</div>

			{/* Filters */}
			<div className="flex flex-wrap items-center gap-2">
				<div className="relative max-w-xs flex-1 min-w-[200px]">
					<Search className="text-muted-foreground pointer-events-none absolute left-2.5 top-2.5 h-4 w-4" />
					<Input
						className="pl-8 text-sm"
						placeholder="Search initiator, target, path, IP…"
						value={search}
						onChange={(e) => setSearch(e.target.value)}
					/>
					{search && (
						<button
							type="button"
							onClick={() => setSearch("")}
							className="text-muted-foreground hover:text-foreground absolute right-2.5 top-2.5"
						>
							<X className="h-4 w-4" />
						</button>
					)}
				</div>
				<select
					value={action}
					onChange={(e) => setAction(e.target.value)}
					className="border-input bg-background h-9 rounded-md border px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
				>
					<option value="">All actions</option>
					<option value="create">create</option>
					<option value="update">update</option>
					<option value="delete">delete</option>
					<option value="login">login</option>
					<option value="logout">logout</option>
					<option value="email_sent">email_sent</option>
				</select>
				<select
					value={outcome}
					onChange={(e) => setOutcome(e.target.value)}
					className="border-input bg-background h-9 rounded-md border px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
				>
					<option value="">All outcomes</option>
					<option value="success">success</option>
					<option value="failure">failure</option>
				</select>
			</div>

			{isError ? (
				<QueryErrorBanner
					message={getErrorMessage(error) || "Failed to load audit logs"}
					testId="audit-logs-query-error"
				/>
			) : null}

			{/* Table */}
			<div className="min-h-0 flex-1 overflow-auto rounded-xl border bg-card">
				{loading ? (
					<p className="text-muted-foreground p-6 text-sm">Loading audit logs…</p>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead className="w-[100px]">Date</TableHead>
								<TableHead className="w-[100px]">Time</TableHead>
								<TableHead className="w-[100px]">Action</TableHead>
								<TableHead className="w-[90px]">Outcome</TableHead>
								<TableHead className="w-[130px]">Initiator</TableHead>
								<TableHead className="min-w-[150px]">Target</TableHead>
								<TableHead className="min-w-[180px]">Path</TableHead>
								<TableHead className="w-[110px]">IP</TableHead>
								<TableHead className="w-[85px]">Duration</TableHead>
								<TableHead className="w-[50px] text-center">Inspect</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{logs.length === 0 ? (
								<TableRow>
									<TableCell colSpan={10} className="text-muted-foreground h-28 text-center text-sm">
										No audit entries match the current filter. Perform actions in Roles & Permissions or Access Profiles to see live events.
									</TableCell>
								</TableRow>
							) : (
								logs.map((log) => (
									<TableRow
										key={log.id}
										onClick={() => setSelectedLog(log)}
										className="cursor-pointer transition-colors hover:bg-muted/50"
										title="Click to inspect audit payload & details"
									>
										<TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
											{formatAuditDate(log.created_at)}
										</TableCell>
										<TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
											{formatAuditTime(log.created_at)}
										</TableCell>
										<TableCell>{getActionBadge(log.action)}</TableCell>
										<TableCell>
											<Badge variant={log.outcome === "success" ? "secondary" : "destructive"}>
												{log.outcome}
											</Badge>
										</TableCell>
										<TableCell className="font-medium text-xs truncate max-w-[130px]" title={log.initiator}>
											{log.initiator || "system"}
										</TableCell>
										<TableCell className="font-medium text-xs max-w-[180px] truncate" title={log.target}>
											{log.target || "—"}
										</TableCell>
										<TableCell className="font-mono text-xs max-w-[220px] truncate text-muted-foreground" title={`${log.method} ${log.path}`}>
											<span className="font-semibold text-foreground/80">{log.method}</span> {log.path}
										</TableCell>
										<TableCell className="font-mono text-xs text-muted-foreground">{log.ip || "—"}</TableCell>
										<TableCell className="text-xs text-muted-foreground whitespace-nowrap">{log.duration_ms ?? 0}ms</TableCell>
										<TableCell className="text-center">
											<Button
												variant="ghost"
												size="icon"
												className="h-7 w-7 text-muted-foreground hover:text-foreground"
												onClick={(e) => {
													e.stopPropagation();
													setSelectedLog(log);
												}}
											>
												<Eye className="h-3.5 w-3.5" />
											</Button>
										</TableCell>
									</TableRow>
								))
							)}
						</TableBody>
					</Table>
				)}
			</div>

			{/* Pagination */}
			<DataTablePagination
				offset={page * pageSize}
				limit={pageSize}
				totalCount={total}
				onOffsetChange={(newOffset) => setPage(Math.floor(newOffset / pageSize))}
				onLimitChange={(newLimit) => {
					setPageSize(newLimit);
					setPage(0);
				}}
				itemLabel="audit logs"
				perPageLabel="Logs per page"
				dataTestId="audit-logs-pagination"
			/>

			{/* Detail Dialog */}
			<Dialog open={!!selectedLog} onOpenChange={(open) => !open && setSelectedLog(null)}>
				<DialogContent className="max-w-2xl max-h-[85vh] flex flex-col">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							<ScrollText className="h-5 w-5 text-primary" />
							Audit Event Details
						</DialogTitle>
						<DialogDescription>
							Event recorded at {selectedLog ? `${formatAuditDate(selectedLog.created_at)} ${formatAuditTime(selectedLog.created_at)}` : ""}
						</DialogDescription>
					</DialogHeader>

					{selectedLog && (
						<div className="flex-1 overflow-auto space-y-4 py-2">
							{/* Metadata summary cards */}
							<div className="grid grid-cols-2 sm:grid-cols-3 gap-3 text-xs">
								<div className="rounded-lg border p-2.5 bg-muted/20">
									<p className="text-muted-foreground font-medium mb-1">Action</p>
									{getActionBadge(selectedLog.action)}
								</div>
								<div className="rounded-lg border p-2.5 bg-muted/20">
									<p className="text-muted-foreground font-medium mb-1">Outcome</p>
									<Badge variant={selectedLog.outcome === "success" ? "secondary" : "destructive"}>
										{selectedLog.outcome}
									</Badge>
								</div>
								<div className="rounded-lg border p-2.5 bg-muted/20">
									<p className="text-muted-foreground font-medium mb-1">Initiator</p>
									<p className="font-semibold text-foreground truncate" title={selectedLog.initiator}>
										{selectedLog.initiator || "system"}
									</p>
								</div>
								<div className="rounded-lg border p-2.5 bg-muted/20">
									<p className="text-muted-foreground font-medium mb-1">Target Entity</p>
									<p className="font-semibold text-foreground truncate" title={selectedLog.target}>
										{selectedLog.target || "—"}
									</p>
								</div>
								<div className="rounded-lg border p-2.5 bg-muted/20">
									<p className="text-muted-foreground font-medium mb-1">Client IP</p>
									<p className="font-mono text-foreground truncate">{selectedLog.ip || "—"}</p>
								</div>
								<div className="rounded-lg border p-2.5 bg-muted/20">
									<p className="text-muted-foreground font-medium mb-1">Duration</p>
									<p className="font-mono text-foreground">{selectedLog.duration_ms ?? 0} ms</p>
								</div>
							</div>

							{/* HTTP Route */}
							<div className="rounded-lg border p-3 bg-muted/30">
								<p className="text-muted-foreground text-xs font-medium mb-1">HTTP Request</p>
								<p className="font-mono text-xs font-semibold break-all">
									<span className="text-primary">{selectedLog.method}</span> {selectedLog.path}
								</p>
							</div>

							{/* Detail Payload */}
							<div className="rounded-lg border p-3">
								<div className="flex items-center justify-between mb-2">
									<p className="text-xs font-medium text-foreground">Payload Details (Sanitized)</p>
									{selectedLog.detail && (
										<Button
											variant="ghost"
											size="sm"
											onClick={handleCopyDetail}
											className="h-7 text-xs gap-1 text-muted-foreground hover:text-foreground"
										>
											{copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
											{copied ? "Copied" : "Copy JSON"}
										</Button>
									)}
								</div>
								{selectedLog.detail ? (
									<pre className="p-3 rounded-md bg-muted/60 font-mono text-xs overflow-auto max-h-[220px] text-foreground leading-relaxed whitespace-pre-wrap break-all">
										{formatPrettyJSON(selectedLog.detail)}
									</pre>
								) : (
									<p className="text-muted-foreground text-xs italic p-2">
										No request body was recorded for this event.
									</p>
								)}
							</div>
						</div>
					)}

					<DialogFooter>
						<Button variant="outline" size="sm" onClick={() => setSelectedLog(null)}>
							Close
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
