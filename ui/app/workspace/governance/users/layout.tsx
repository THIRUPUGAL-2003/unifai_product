import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernanceUsersPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.Users, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="users" />;
	}
	return <GovernanceUsersPage />;
}

export const Route = createFileRoute("/workspace/governance/users")({
	component: RouteComponent,
});
