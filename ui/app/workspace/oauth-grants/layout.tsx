import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import OAuthGrantsPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.MCPGateway, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="OAuth grants" />;
	}
	return <OAuthGrantsPage />;
}

export const Route = createFileRoute("/workspace/oauth-grants")({
	component: RouteComponent,
});
