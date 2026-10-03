import FullPageLoader from "@/components/fullPageLoader";
import { useIsAuthEnabledQuery } from "@/lib/store";
import {
	fetchSessionAuth,
	getWorkspaceAccessRedirect,
	hasNoWorkspaceSections,
	isPublicWorkspacePath,
} from "@/lib/utils/workspaceAccess";
import { createFileRoute, Outlet, redirect, useLocation } from "@tanstack/react-router";
import { ShieldOff } from "lucide-react";
import { ClientLayout } from "../clientLayout";

function WorkspaceLayout({ children }: { children: React.ReactNode }) {
	return <ClientLayout>{children}</ClientLayout>;
}

function NoSectionsAssigned() {
	return (
		<div className="flex h-full min-h-[60vh] items-center justify-center p-6" data-testid="workspace-no-sections">
			<div className="border-border bg-card max-w-md space-y-3 rounded-lg border p-6 text-center">
				<ShieldOff className="text-muted-foreground mx-auto h-8 w-8" />
				<h2 className="text-foreground text-lg font-semibold">No sections assigned</h2>
				<p className="text-muted-foreground text-sm">
					Your account does not have access to any workspace section yet. Ask an admin to grant you access under Users or
					Roles &amp; Permissions.
				</p>
			</div>
		</div>
	);
}

// Must render inside ClientLayout: useIsAuthEnabledQuery needs the ReduxProvider it mounts.
function WorkspaceOutlet() {
	const { data: authStatus } = useIsAuthEnabledQuery();
	const pathname = useLocation({ select: (l) => l.pathname });
	const blocked = hasNoWorkspaceSections(authStatus) && !isPublicWorkspacePath(pathname);
	return blocked ? <NoSectionsAssigned /> : <Outlet />;
}

function RouteComponent() {
	return (
		<WorkspaceLayout>
			<WorkspaceOutlet />
		</WorkspaceLayout>
	);
}

function PendingComponent() {
	return <FullPageLoader />;
}

export const Route = createFileRoute("/workspace")({
	beforeLoad: async ({ location }) => {
		const auth = await fetchSessionAuth(false);
		if (auth && auth.is_auth_enabled && !auth.has_valid_token) {
			if (!isPublicWorkspacePath(location.pathname)) {
				const goto = location.pathname + (location.searchStr ?? "");
				throw redirect({
					href: `/login?goto=${encodeURIComponent(goto)}`,
				});
			}
		}
		const redirectTo = getWorkspaceAccessRedirect(auth, location.pathname);
		if (redirectTo) {
			throw redirect({ to: redirectTo, replace: true });
		}
	},
	pendingComponent: PendingComponent,
	pendingMs: 800,
	component: RouteComponent,
});
