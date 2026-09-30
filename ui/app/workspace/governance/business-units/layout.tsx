import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernanceBusinessUnitsPage from "./page";

function RouteComponent() {
	// Must match API RBAC: /api/governance/business-units* → Governance.
	const hasAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="business units" />;
	}
	return <GovernanceBusinessUnitsPage />;
}

export const Route = createFileRoute("/workspace/governance/business-units")({
	component: RouteComponent,
});
