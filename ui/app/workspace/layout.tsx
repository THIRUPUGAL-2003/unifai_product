import FullPageLoader from "@/components/fullPageLoader";
import { fetchSessionAuth, getWorkspaceAccessRedirect } from "@/lib/utils/workspaceAccess";
import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { ClientLayout } from "../clientLayout";

function WorkspaceLayout({ children }: { children: React.ReactNode }) {
	return <ClientLayout>{children}</ClientLayout>;
}

function RouteComponent() {
	return (
		<WorkspaceLayout>
			<Outlet />
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
			const isPublicScoped =
				location.pathname.startsWith("/workspace/mcp-sessions/auth") ||
				location.pathname.startsWith("/workspace/oauth");
			if (!isPublicScoped) {
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
