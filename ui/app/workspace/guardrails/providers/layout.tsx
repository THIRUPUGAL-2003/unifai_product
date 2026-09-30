import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GuardrailsProvidersPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.GuardrailsProviders, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="guardrails providers" />;
	}
	return <GuardrailsProvidersPage />;
}

export const Route = createFileRoute("/workspace/guardrails/providers")({
	component: RouteComponent,
});
