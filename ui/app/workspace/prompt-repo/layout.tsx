import { createFileRoute } from "@tanstack/react-router";
import { NoPermissionView } from "@/components/noPermissionView";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import PromptsPage from "./page";

function RouteComponent() {
	const hasPromptRepositoryAccess = useRbac(RbacResource.PromptRepository, RbacOperation.View);

	if (!hasPromptRepositoryAccess) {
		return <NoPermissionView entity="prompt repository" />;
	}

	return <PromptsPage />;
}

export const Route = createFileRoute("/workspace/prompt-repo")({
	component: RouteComponent,
});
