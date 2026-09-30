import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernanceTeamsPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.Teams, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="teams" />;
	}
	return <GovernanceTeamsPage />;
}

export const Route = createFileRoute("/workspace/governance/teams")({
	component: RouteComponent,
});
