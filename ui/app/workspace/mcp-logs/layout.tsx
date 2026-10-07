import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { createFileRoute } from "@tanstack/react-router";
import MCPLogsPage from "./page";

function RouteComponent() {
	// Align with API enforce.go: MCP logs readable with MCPLogs or Logs View.
	const hasViewMCPLogsAccess = useRbac(RbacResource.MCPLogs, RbacOperation.View);
	const hasViewLogsAccess = useRbac(RbacResource.Logs, RbacOperation.View);
	if (!hasViewMCPLogsAccess && !hasViewLogsAccess) {
		return <NoPermissionView entity="mcp logs" />;
	}
	return <MCPLogsPage />;
}

export const Route = createFileRoute("/workspace/mcp-logs")({
	component: RouteComponent,
});