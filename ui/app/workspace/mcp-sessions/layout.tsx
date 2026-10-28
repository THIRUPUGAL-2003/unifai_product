import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac, useRbacAny } from "@enterprise/lib";
import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import MCPSessionsPage from "./page";

function RouteComponent() {
	// Yield to child routes (/auth, /oauth-callback) when one matches;
	// otherwise render the list view as the index of this section.
	const childMatches = useChildMatches();
	const hasAccess = useRbacAny([RbacResource.MCPAuthSessions, RbacResource.MCPGateway], RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="MCP auth sessions" />;
	}
	return childMatches.length === 0 ? <MCPSessionsPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/mcp-sessions")({
	component: RouteComponent,
});
