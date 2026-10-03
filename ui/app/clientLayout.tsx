import FullPageLoader from "@/components/fullPageLoader";
import NotAvailableBanner from "@/components/notAvailableBanner";
import ProgressProvider from "@/components/progressBar";
import Sidebar from "@/components/sidebar";
import { LanguageSelector } from "@/components/languageSelector";
import { ThemeProvider } from "@/components/themeProvider";
import TrialExpiryBanner from "@/components/trialExpiryBanner";
import { SidebarProvider } from "@/components/ui/sidebar";
import { useStoreSync } from "@/hooks/useStoreSync";
import { WebSocketProvider } from "@/hooks/useWebSocket";
import { getErrorMessage, ReduxProvider, useGetCoreConfigQuery, useIsAuthEnabledQuery } from "@/lib/store";
import { RakshaConfig } from "@/lib/types/config";
import { RbacProvider, useRbacContext } from "@enterprise/lib/contexts/rbacContext";
import { useLocation, useMatches } from "@tanstack/react-router";
import { NuqsAdapter } from "nuqs/adapters/tanstack-router";
import { lazy, Suspense, useEffect, useState } from "react";
import { CookiesProvider } from "react-cookie";
import { toast, Toaster } from "sonner";

// Lazy import — only loaded in development, completely excluded from prod bundle
const DevProfilerLazy = lazy(() =>
	import("@/components/devProfiler").then((mod) => ({
		default: mod.DevProfiler,
	})),
);
const DevProfiler = () => (
	<Suspense fallback={null}>
		<DevProfilerLazy />
	</Suspense>
);

function StoreSyncInitializer() {
	useStoreSync();
	return null;
}

function AppContent({ children }: { children: React.ReactNode }) {
	const pathname = useLocation({ select: (l) => l.pathname });
	const onWorkspace = pathname.startsWith("/workspace");
	// Routes can declare `staticData: { tempTokenScoped: true }` to advertise that
	// they're reachable via a server-emitted, temp-token-bearing URL by visitors
	// without a dashboard session. The actual layout choice is made per-visitor:
	// an authenticated admin still sees the full dashboard chrome, while an
	// anonymous visitor arriving with `#t=<token>` gets a stripped MinimalShell.
	// The auth-via-temp-token half lives in <TempTokenScope>.
	const matches = useMatches();
	const tempTokenScoped = matches.some((m) => (m.staticData as { tempTokenScoped?: boolean } | undefined)?.tempTokenScoped === true);
	// publicShell: route declares it's a static, auth-free page that should
	// always render MinimalShell — no chrome, no auth probe, no API calls.
	// Used by the post-OAuth "authentication successful" landing, which has
	// neither a fragment nor a cookie to drive the tempTokenScoped per-visitor
	// logic.
	const publicShell = matches.some((m) => (m.staticData as { publicShell?: boolean } | undefined)?.publicShell === true);

	// Probe dashboard auth state on opted-in routes. is-auth-enabled is whitelisted
	// (no 401 risk) and returns whether the current cookie is a valid session.
	const { data: authState, isLoading: authLoading } = useIsAuthEnabledQuery(undefined, {
		skip: !tempTokenScoped && !onWorkspace,
	});

	// Snapshot fragment presence at mount: TempTokenScope strips the fragment
	// shortly after, so re-reading window.location.hash would flip false on
	// re-render. Only fragment-bearing arrivals are MinimalShell candidates.
	const [hadFragmentTempToken] = useState(() => {
		if (typeof window === "undefined") return false;
		const fragment = window.location.hash;
		if (!fragment || fragment.length < 2) return false;
		return !!new URLSearchParams(fragment.slice(1)).get("t");
	});

	const useMinimalShell = tempTokenScoped && !!authState?.is_auth_enabled && !authState?.has_valid_token && hadFragmentTempToken;

	const {
		data: rakshaConfig,
		error,
		isLoading,
		refetch: refetchConfig,
	} = useGetCoreConfigQuery(
		{},
		{
			skip: publicShell || useMinimalShell || (tempTokenScoped && authLoading),
		},
	);

	// Permissions are restored from sessionStorage (async) and refreshed from the
	// API. Until that first resolve, useRbac() returns false for everything, which
	// would briefly collapse the sidebar to a single tab and flash NoPermissionView
	// on the active route. Gate the full dashboard chrome on it; the cached read is
	// a single frame so this is imperceptible. Minimal/public shells don't use RBAC
	// and are handled by the early returns below.
	const { isLoading: rbacLoading } = useRbacContext();

	useEffect(() => {
		if (error) {
			toast.error(getErrorMessage(error));
		}
	}, [error]);

	if (publicShell) {
		return <MinimalShell>{children}</MinimalShell>;
	}
	if (tempTokenScoped && authLoading) {
		return <FullPageLoader />;
	}
	if (useMinimalShell) {
		return <MinimalShell>{children}</MinimalShell>;
	}

	if (rbacLoading) {
		return <FullPageLoader />;
	}

	if (onWorkspace && authLoading) {
		return <FullPageLoader />;
	}

	return (
		<WebSocketProvider>
			<CookiesProvider>
				<StoreSyncInitializer />
				<SidebarProvider>
					<Sidebar />
					<div className="dark:bg-card content-container my-[0.5rem] mr-[0.5rem] flex h-[calc(100dvh-1rem)] min-w-0 flex-1 flex-col overflow-hidden rounded-md border border-gray-200 bg-white px-10 dark:border-zinc-800">
						<div className="z-30 -mx-10 flex shrink-0 items-center justify-end px-6 py-2 border-b border-border/40 bg-background/85 backdrop-blur-md">
							<LanguageSelector />
						</div>
						<TrialExpiryBanner />
						<main className="no-scrollbar content-container-inner relative mx-auto flex min-h-0 w-full flex-1 flex-col overflow-x-hidden overflow-y-auto p-4">
							{isLoading ? (
								<FullPageLoader />
							) : (
								<FullPage config={rakshaConfig} error={error} onRetry={refetchConfig}>
									{children}
								</FullPage>
							)}
						</main>
					</div>
				</SidebarProvider>
			</CookiesProvider>
		</WebSocketProvider>
	);
}

// MinimalShell renders a centered container without sidebar, websocket,
// store-sync, or any dashboard-config fetches. Used for routes that opt
// in via `staticData.tempTokenScoped` — typically public, scoped pages
// like the MCP per-user OAuth auth page.
function MinimalShell({ children }: { children: React.ReactNode }) {
	return (
		<div className="dark:bg-card content-container my-[0.5rem] flex h-[calc(100dvh-1rem)] w-full flex-col overflow-hidden rounded-md border border-gray-200 bg-white px-10 dark:border-zinc-800">
			<div className="z-30 -mx-10 flex shrink-0 items-center justify-end px-6 py-2 border-b border-border/40 bg-background/85 backdrop-blur-md">
				<LanguageSelector />
			</div>
			<main className="no-scrollbar content-container-inner relative mx-auto flex min-h-0 w-full flex-1 flex-col overflow-x-hidden overflow-y-auto p-4">
				{children}
			</main>
		</div>
	);
}

function FullPage({
	config,
	error,
	onRetry,
	children,
}: {
	config: RakshaConfig | undefined;
	error?: unknown;
	onRetry?: () => void;
	children: React.ReactNode;
}) {
	const pathname = useLocation({ select: (l) => l.pathname });
	if (config && config.is_db_connected) {
		return children;
	}
	// Logs-only deployments: allow all Observability log surfaces that hit the logs store.
	if (
		config &&
		config.is_logs_connected &&
		(pathname.startsWith("/workspace/logs") ||
			pathname.startsWith("/workspace/mcp-logs") ||
			pathname.startsWith("/workspace/dashboard"))
	) {
		return children;
	}
	// Surface server or auth connection errors with a retry action instead of infinite spinner
	if (error) {
		return (
			<div className="flex h-full min-h-[50vh] flex-col items-center justify-center gap-4 text-center">
				<p className="text-sm font-medium text-destructive">Failed to load system configuration</p>
				<p className="max-w-md text-xs text-muted-foreground">{getErrorMessage(error)}</p>
				<div className="flex items-center gap-2">
					{onRetry && (
						<button
							type="button"
							onClick={onRetry}
							className="rounded-md border border-border bg-card px-3 py-1.5 text-xs font-medium hover:bg-accent cursor-pointer"
						>
							Retry Connection
						</button>
					)}
					<button
						type="button"
						onClick={() => window.location.assign("/login")}
						className="rounded-md bg-teal-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-teal-500 cursor-pointer"
					>
						Go to Sign In
					</button>
				</div>
			</div>
		);
	}
	// Avoid flashing the "config store missing" banner while config is still
	// loading or after a transient auth race right after login.
	if (!config) {
		return <FullPageLoader />;
	}
	return <NotAvailableBanner />;
}

export function ClientLayout({ children }: { children: React.ReactNode }) {
	return (
		<ProgressProvider>
			<ThemeProvider attribute="class" defaultTheme="light" enableSystem={false}>
				<Toaster closeButton />
				<ReduxProvider>
					<NuqsAdapter>
						<RbacProvider>
							<AppContent>{children}</AppContent>
							{process.env.NODE_ENV === "development" && !process.env.RAKSHA_DISABLE_PROFILER && <DevProfiler />}
						</RbacProvider>
					</NuqsAdapter>
				</ReduxProvider>
			</ThemeProvider>
		</ProgressProvider>
	);
}