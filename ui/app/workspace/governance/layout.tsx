import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GovernancePage from "./page";

function RouteComponent() {
	const hasVirtualKeysAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.View);
	const hasTeamsAccess = useRbac(RbacResource.Teams, RbacOperation.View);
	const hasUsersAccess = useRbac(RbacResource.Users, RbacOperation.View);
	const hasCustomersAccess = useRbac(RbacResource.Customers, RbacOperation.View);
	// Must match API RBAC: /api/governance/business-units* → Governance (not UserProvisioning).
	const hasBusinessUnitsAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	const hasRbacAccess = useRbac(RbacResource.RBAC, RbacOperation.View);
	const hasAccessProfilesAccess = useRbac(RbacResource.AccessProfiles, RbacOperation.View);
	const hasAuditLogsAccess = useRbac(RbacResource.AuditLogs, RbacOperation.View);

	const hasAnyGovernanceAccess =
		hasVirtualKeysAccess ||
		hasTeamsAccess ||
		hasUsersAccess ||
		hasCustomersAccess ||
		hasBusinessUnitsAccess ||
		hasRbacAccess ||
		hasAccessProfilesAccess ||
		hasAuditLogsAccess;

	const childMatches = useChildMatches();
	if (!hasAnyGovernanceAccess) {
		return <NoPermissionView entity="governance" />;
	}
	return childMatches.length === 0 ? <GovernancePage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/governance")({
	component: RouteComponent,
});