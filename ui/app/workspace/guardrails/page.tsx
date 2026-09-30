import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export default function GuardrailsPage() {
	const navigate = useNavigate();
	const hasConfig = useRbac(RbacResource.GuardrailsConfig, RbacOperation.View);
	const hasProviders = useRbac(RbacResource.GuardrailsProviders, RbacOperation.View);

	const firstAllowed =
		(hasConfig && "/workspace/guardrails/configuration") || (hasProviders && "/workspace/guardrails/providers") || null;

	useEffect(() => {
		if (firstAllowed) {
			navigate({ to: firstAllowed, replace: true });
		}
	}, [navigate, firstAllowed]);

	if (!firstAllowed) {
		return <NoPermissionView entity="guardrails" />;
	}
	return null;
}
