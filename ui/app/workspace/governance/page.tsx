import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export default function GovernancePage() {
	const navigate = useNavigate();
	const hasVirtualKeysAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.View);
	const hasUsersAccess = useRbac(RbacResource.Users, RbacOperation.View);
	const hasTeamsAccess = useRbac(RbacResource.Teams, RbacOperation.View);
	const hasCustomersAccess = useRbac(RbacResource.Customers, RbacOperation.View);
	const hasBusinessUnitsAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	const hasRbacAccess = useRbac(RbacResource.RBAC, RbacOperation.View);
	const hasAccessProfilesAccess = useRbac(RbacResource.AccessProfiles, RbacOperation.View);
	const hasAuditLogsAccess = useRbac(RbacResource.AuditLogs, RbacOperation.View);

	const firstAllowed =
		(hasVirtualKeysAccess && "/workspace/governance/virtual-keys") ||
		(hasUsersAccess && "/workspace/governance/users") ||
		(hasTeamsAccess && "/workspace/governance/teams") ||
		(hasCustomersAccess && "/workspace/governance/customers") ||
		(hasBusinessUnitsAccess && "/workspace/governance/business-units") ||
		(hasRbacAccess && "/workspace/governance/rbac") ||
		(hasAccessProfilesAccess && "/workspace/governance/access-profiles") ||
		(hasAuditLogsAccess && "/workspace/audit-logs") ||
		null;

	useEffect(() => {
		if (firstAllowed) {
			navigate({ to: firstAllowed, replace: true });
		}
	}, [navigate, firstAllowed]);

	if (!firstAllowed) {
		return <NoPermissionView entity="governance" />;
	}
	return null;
}
