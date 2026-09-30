import { NoPermissionView } from "@/components/noPermissionView";
import { createFileRoute } from "@tanstack/react-router";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import GuardrailsConfigurationPage from "./page";

function RouteComponent() {
	const hasAccess = useRbac(RbacResource.GuardrailsConfig, RbacOperation.View);
	if (!hasAccess) {
		return <NoPermissionView entity="guardrails rules" />;
	}
	return <GuardrailsConfigurationPage />;
}

export const Route = createFileRoute("/workspace/guardrails/configuration")({
	component: RouteComponent,
});
