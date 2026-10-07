import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { createFileRoute } from "@tanstack/react-router";
import LoggingPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.Settings, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="logs settings" />;
	}
	return <LoggingPage />;
}

export const Route = createFileRoute("/workspace/config/logging")({
	component: RouteComponent,
});
