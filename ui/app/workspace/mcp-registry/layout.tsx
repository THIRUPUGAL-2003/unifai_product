import { createFileRoute, Outlet, useChildMatches, useLocation } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbacAny } from "@enterprise/lib";
import MCPServersPage from "./page";

function RouteComponent() {
	const location = useLocation();
	const pageResource = location.pathname.startsWith("/workspace/mcp-registry/library") ? RbacResource.MCPLibrary : RbacResource.MCPCatalog;
	const hasMCPGatewayAccess = useRbacAny([pageResource, RbacResource.MCPGateway], RbacOperation.View);
	const childMatches = useChildMatches();
	if (!hasMCPGatewayAccess) {
		return <NoPermissionView entity="MCP gateway configuration" />;
	}
	return childMatches.length === 0 ? <MCPServersPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/mcp-registry")({
	component: RouteComponent,
});
