import { createFileRoute, Outlet, useChildMatches } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GuardrailsPage from "./page";

function RouteComponent() {
	// Sidebar allows Config OR Providers; parent layout must not block Providers-only users.
	const hasGuardrailsConfigAccess = useRbac(RbacResource.GuardrailsConfig, RbacOperation.View);
	const hasGuardrailsProvidersAccess = useRbac(RbacResource.GuardrailsProviders, RbacOperation.View);
	const hasGuardrailsAccess = hasGuardrailsConfigAccess || hasGuardrailsProvidersAccess;
	const childMatches = useChildMatches();
	if (!hasGuardrailsAccess) {
		return <NoPermissionView entity="guardrails" />;
	}
	return childMatches.length === 0 ? <GuardrailsPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/guardrails")({
	component: RouteComponent,
});