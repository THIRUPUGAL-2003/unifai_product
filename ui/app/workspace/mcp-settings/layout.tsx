import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import MCPSettingsPage from "./page";

function RouteComponent() {
	// Allow View to open the page; Save remains gated by Update in the form.
	const hasMCPGatewayAccess = useRbac(RbacResource.MCPGateway, RbacOperation.View);
	if (!hasMCPGatewayAccess) {
		return <NoPermissionView entity="MCP gateway settings" />;
	}
	return <MCPSettingsPage />;
}

export const Route = createFileRoute("/workspace/mcp-settings")({
	component: RouteComponent,
});
