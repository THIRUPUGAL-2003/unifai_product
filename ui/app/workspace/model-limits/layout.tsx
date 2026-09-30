import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import ModelLimitsPage from "./page";

function RouteComponent() {
	const hasGovernanceAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	if (!hasGovernanceAccess) {
		return <NoPermissionView entity="budgets and model limits" />;
	}
	return <ModelLimitsPage />;
}

export const Route = createFileRoute("/workspace/model-limits")({
	component: RouteComponent,
});
