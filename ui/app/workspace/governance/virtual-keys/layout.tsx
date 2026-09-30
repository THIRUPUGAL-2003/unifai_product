import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernanceVirtualKeysPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="virtual keys" />;
	}
	return <GovernanceVirtualKeysPage />;
}

export const Route = createFileRoute("/workspace/governance/virtual-keys")({
	component: RouteComponent,
});
