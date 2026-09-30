import { Badge } from "@/components/ui/badge";
import { ExportFormatsDropdown } from "@/components/exportFormatsDropdown";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage } from "@/lib/store";
import { useGetAuditLogsQuery, useLazyExportAuditLogsQuery } from "@enterprise/lib/store/apis/auditLogsApi";
import { ScrollText } from "lucide-react";
import { useCallback, useState } from "react";
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

export default function AuditLogsView() {
	const [search, setSearch] = useState("");
	const [action, setAction] = useState("");
	const [outcome, setOutcome] = useState("");
	const { data, isLoading: loading } = useGetAuditLogsQuery({
		search: search || undefined,
		action: action || undefined,
		outcome: outcome || undefined,
	});
	const [exportAuditLogs] = useLazyExportAuditLogsQuery();
	const logs = data?.logs || [];

	const getExportPayload = useCallback(async () => {
		try {
			const result = await exportAuditLogs().unwrap();
			const rows = result.logs || [];
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
					{ key: "path", header: "Path" },
					{ key: "ip", header: "IP" },
					{ key: "duration", header: "Duration" },
				],
				rows: rows.map((log) => ({
					date: formatAuditDate(log.created_at),
					time: formatAuditTime(log.created_at),
					action: log.action || "",
					outcome: log.outcome || "",
					initiator: log.initiator || "",
					path: `${log.method || ""} ${log.path || ""}`.trim(),
					ip: log.ip || "",
					duration: `${log.duration_ms ?? 0}ms`,
				})),
				json: rows,
			};
		} catch (err) {
			toast.error(getErrorMessage(err));
			return {
				filename: "audit-logs",
				title: "Audit Logs",
				columns: [],
				rows: [],
				json: [],
			};
		}
	}, [exportAuditLogs]);

	return (
		<div className="flex h-full w-full flex-col gap-4 p-4">
			<div className="flex flex-col justify-between gap-3 md:flex-row md:items-center">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-semibold">
						<ScrollText className="h-6 w-6" />
						Audit Logs
					</h1>
					<p className="text-muted-foreground text-sm">
						Every create / update / delete (and login / logout) across this workspace — Virtual Keys, Users,
						Browser AI, roles, and more.
					</p>
				</div>
				<ExportFormatsDropdown getPayload={getExportPayload} testId="audit-logs-export-trigger" />
			</div>
			<div className="flex flex-wrap gap-2">
				<Input className="max-w-xs" placeholder="Search initiator, path, IP…" value={search} onChange={(e) => setSearch(e.target.value)} />
				<select value={action} onChange={(e) => setAction(e.target.value)} className="border-input bg-background h-9 rounded-md border px-3 text-sm">
					<option value="">All actions</option>
					<option value="create">create</option>
					<option value="update">update</option>
					<option value="delete">delete</option>
					<option value="login">login</option>
					<option value="logout">logout</option>
				</select>
				<select value={outcome} onChange={(e) => setOutcome(e.target.value)} className="border-input bg-background h-9 rounded-md border px-3 text-sm">
					<option value="">All outcomes</option>
					<option value="success">success</option>
					<option value="failure">failure</option>
				</select>
			</div>
			<div className="min-h-0 flex-1 overflow-auto rounded-xl border">
				{loading ? (
					<p className="text-muted-foreground p-6 text-sm">Loading audit logs…</p>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead className="w-[100px]">Date</TableHead>
								<TableHead className="w-[100px]">Time</TableHead>
								<TableHead>Action</TableHead>
								<TableHead>Outcome</TableHead>
								<TableHead>Initiator</TableHead>
								<TableHead>Path</TableHead>
								<TableHead>IP</TableHead>
								<TableHead>Duration</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{logs.length === 0 ? (
								<TableRow>
									<TableCell colSpan={8} className="text-muted-foreground h-24 text-center text-sm">
										No audit entries yet. Create, update, or delete workspace resources to see them here.
									</TableCell>
								</TableRow>
							) : (
								logs.map((log) => (
									<TableRow key={log.id}>
										<TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
											{formatAuditDate(log.created_at)}
										</TableCell>
										<TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
											{formatAuditTime(log.created_at)}
										</TableCell>
										<TableCell>{log.action}</TableCell>
										<TableCell>
											<Badge variant={log.outcome === "success" ? "secondary" : "destructive"}>{log.outcome}</Badge>
										</TableCell>
										<TableCell>{log.initiator}</TableCell>
										<TableCell className="font-mono text-xs">
											{log.method} {log.path}
										</TableCell>
										<TableCell className="font-mono text-xs">{log.ip}</TableCell>
										<TableCell className="text-xs">{log.duration_ms}ms</TableCell>
									</TableRow>
								))
							)}
						</TableBody>
					</Table>
				)}
			</div>
		</div>
	);
}
