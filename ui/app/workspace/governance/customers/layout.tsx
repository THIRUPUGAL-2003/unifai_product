import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernanceCustomersPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.Customers, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="customers" />;
	}
	return <GovernanceCustomersPage />;
}

export const Route = createFileRoute("/workspace/governance/customers")({
	component: RouteComponent,
});
