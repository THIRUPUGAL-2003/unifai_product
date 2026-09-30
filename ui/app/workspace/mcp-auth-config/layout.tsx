import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import MCPAuthConfigPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.MCPGateway, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="MCP auth configuration" />;
	}
	return <MCPAuthConfigPage />;
}

export const Route = createFileRoute("/workspace/mcp-auth-config")({
	component: RouteComponent,
});
