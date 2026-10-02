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
					<div className="min-h-screen bg-[#090b12]">
						<LoginPage />
					</div>
				</NuqsAdapter>
			</ReduxProvider>
		</ThemeProvider>
	);
}

function PendingComponent() {
	return (
		<ThemeProvider attribute="class" defaultTheme="dark" enableSystem={false}>
			<div className="flex min-h-screen items-center justify-center p-4">
				<div className="w-full max-w-md">
					<div className="border-border bg-card w-full space-y-6 rounded-sm border p-8">
						<div className="flex items-center justify-center">
							<img src={COMPANY_LOGO} alt={COMPANY_NAME} width={160} height={26} />
						</div>
						<div className="flex items-center justify-center py-6">
							<div className="text-muted-foreground text-sm">Checking authentication...</div>
						</div>
					</div>
				</div>
			</div>
		</ThemeProvider>
	);
}

export const Route = createFileRoute("/login")({
	loader: async ({ location }) => {
		invalidateSessionAuthCache();
		const params = new URLSearchParams(location.searchStr);
		const isExplicitLogout = params.get("logged_out") === "1" || params.get("logout") === "true";
		const goto = getLoginGotoFromSearch(location.searchStr);
		const data = await fetchSessionAuth(true);
		// Never auto-bounce back to workspace if user arrived via explicit logout
		if (!isExplicitLogout && data && (!data.is_auth_enabled || data.has_valid_token)) {
			throw redirect({ href: resolvePostLoginPath(data, goto) });
		}
	},
	pendingComponent: PendingComponent,
	pendingMs: 0,
	component: RouteComponent,
});