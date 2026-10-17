import { ThemeProvider } from "@/components/themeProvider";
import { COMPANY_LOGO, COMPANY_NAME } from "@/lib/constants/config";
import { ReduxProvider } from "@/lib/store/provider";
import { getLoginGotoFromSearch } from "@/lib/utils/loginGoto";
import { fetchSessionAuth, invalidateSessionAuthCache, resolvePostLoginPath } from "@/lib/utils/workspaceAccess";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { NuqsAdapter } from "nuqs/adapters/tanstack-router";
import LoginPage from "./page";

function RouteComponent() {
	return (
		<ThemeProvider attribute="class" defaultTheme="dark" enableSystem={false}>
			<ReduxProvider>
				<NuqsAdapter>
					<div className="min-h-screen bg-[#f6f8fd]">
						<LoginPage />
					</div>
				</NuqsAdapter>
			</ReduxProvider>
		</ThemeProvider>
	);
}

function PendingComponent() {
	return (
		<div className="flex min-h-screen items-center justify-center bg-[#f6f8fd] px-6">
			<p className="text-sm font-medium text-slate-500">Checking authentication...</p>
		</div>
	);
}

export const Route = createFileRoute("/login")({
	loader: async ({ location }) => {
		invalidateSessionAuthCache();
		const params = new URLSearchParams(location.searchStr);
		const isExplicitLogout = params.get("logged_out") === "1" || params.get("logout") === "true";
		const goto = getLoginGotoFromSearch(location.searchStr);
		const data = await fetchSessionAuth(true);
		// Never auto-bounce back to workspace if user arrived via explicit logout or must change temporary password
		if (!isExplicitLogout && data && (!data.is_auth_enabled || data.has_valid_token)) {
			if (data.must_change_password) {
				return;
			}
			throw redirect({ href: resolvePostLoginPath(data, goto) });
		}
	},
	pendingComponent: PendingComponent,
	pendingMs: 0,
	component: RouteComponent,
});