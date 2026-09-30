import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernanceRbacPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.RBAC, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="roles and permissions" />;
	}
	return <GovernanceRbacPage />;
}

export const Route = createFileRoute("/workspace/governance/rbac")({
	component: RouteComponent,
});
