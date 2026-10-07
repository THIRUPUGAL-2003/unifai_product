import { createFileRoute, Outlet, useChildMatches, useLocation } from "@tanstack/react-router";
import FullPageLoader from "@/components/fullPageLoader";
import { NoPermissionView } from "@/components/noPermissionView";
import { Button } from "@/components/ui/button";
import { getErrorMessage, useGetCoreConfigQuery } from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import ConfigPage from "./page";

function RouteComponent() {
	const pathname = useLocation({ select: (l) => l.pathname });
	const hasSettingsAccess = useRbac(RbacResource.Settings, RbacOperation.View);
	const hasAPIKeysAccess = useRbac(RbacResource.APIKeys, RbacOperation.View);
	const childMatches = useChildMatches();

	const isAPIKeysRoute = pathname.startsWith("/workspace/config/api-keys");
	const requiredAccess = isAPIKeysRoute ? hasAPIKeysAccess : hasSettingsAccess;

	const { isLoading, isError, error, refetch } = useGetCoreConfigQuery({ fromDB: true }, { skip: !requiredAccess });

	if (!requiredAccess) {
		return <NoPermissionView entity="configuration" />;
	}

	if (isLoading) {
		return <FullPageLoader />;
	}

	if (isError) {
		return (
			<div className="flex h-full min-h-[40vh] flex-col items-center justify-center gap-3 p-6 text-center">
				<p className="text-destructive text-sm font-semibold">Failed to load configuration</p>
				<p className="text-muted-foreground max-w-md text-xs">{getErrorMessage(error)}</p>
				<Button type="button" variant="outline" size="sm" onClick={() => refetch()}>
					Retry
				</Button>
			</div>
		);
	}

	return childMatches.length === 0 ? <ConfigPage /> : <Outlet />;
}

export const Route = createFileRoute("/workspace/config")({
	component: RouteComponent,
});