import {
	Activity,
	ArrowUpRight,
	BookOpenText,
	BookUser,
	Boxes,
	BoxIcon,
	BugIcon,
	Building,
	Building2,
	ChartColumnBig,
	ChevronsLeftRightEllipsis,
	Construction,
	DatabaseZap,
	FileText,
	Flag,
	ShieldHalf,
	FlaskConical,
	FolderGit,
	Globe,
	KeyRound,
	Landmark,
	LayoutGrid,
	LogOut,
	Logs,
	Network,
	PanelLeftClose,
	PanelLeftOpen,
	Plug,
	Puzzle,
	Radio,
	ScrollText,
	Search,
	SearchCheck,
	Settings,
	Settings2Icon,
	Shield,
	ShieldCheck,
	Shuffle,
	SlidersHorizontal,
	Telescope,
	Terminal,
	ToolCase,
	TrendingUp,
	User,
	UserRoundCheck,
	Users,
	Wallet,
	WalletCards,
	CircuitBoard,
	GitCompareArrows,
} from "lucide-react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
	Sidebar,
	SidebarContent,
	SidebarGroup,
	SidebarGroupContent,
	SidebarHeader,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarMenuSub,
	SidebarMenuSubButton,
	SidebarMenuSubItem,
	SidebarRail,
	useSidebar,
} from "@/components/ui/sidebar";
import { useWebSocket } from "@/hooks/useWebSocket";
import { IS_ENTERPRISE, COMPANY_NAME, COMPANY_LOGO, COMPANY_SHORT_NAME } from "@/lib/constants/config";
import { useGetCoreConfigQuery, useGetLatestReleaseQuery, useGetVersionQuery, useLogoutMutation, useIsAuthEnabledQuery } from "@/lib/store";
import { clearAuthStorage } from "@/lib/store/apis";
import { invalidateSessionAuthCache } from "@/lib/utils/workspaceAccess";
import {
	parseAdminAllowedSections,
	SECTION_KEY_BY_TITLE,
	isPathAllowedForUser,
	getDefaultPathForSections,
	isSectionGranted,
	isSidebarItemGranted,
	type WorkspaceSectionKey,
} from "@/lib/constants/workspaceSections";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import type { UserInfo } from "@enterprise/lib/store/utils/tokenManager";
import { getUserInfo } from "@enterprise/lib/store/utils/tokenManager";
import { BooksIcon, DiscordLogoIcon, GithubLogoIcon } from "@phosphor-icons/react";
import { Link, useLocation, useNavigate } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import { useTheme } from "next-themes";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useCookies } from "react-cookie";
import { cn } from "@/lib/utils";
import { ThemeToggle } from "./themeToggle";
import { Badge } from "./ui/badge";
import { PromoCardStack } from "./ui/promoCardStack";

// Cookie name for dismissing production setup card
const PRODUCTION_SETUP_DISMISSED_COOKIE = "unifai_production_setup_dismissed";

const newBadgeClassName =
	"relative overflow-hidden after:pointer-events-none after:absolute after:inset-y-0 after:-left-full after:w-full after:skew-x-[-18deg] after:bg-gradient-to-r after:from-transparent after:via-primary/25 after:to-transparent after:opacity-0 after:content-[''] after:animate-[sidebar-new-badge-shine_1200ms_cubic-bezier(0.22,1,0.36,1)_260ms_both]";

// Custom MCP Icon Component
const MCPIcon = ({ className }: { className?: string }) => (
	<svg
		className={className}
		fill="currentColor"
		fillRule="evenodd"
		height="1em"
		style={{ flex: "none", lineHeight: 1 }}
		viewBox="0 0 24 24"
		width="1em"
		xmlns="http://www.w3.org/2000/svg"
		aria-label="MCP clients icon"
	>
		<title>MCP clients icon</title>
		<path d="M15.688 2.343a2.588 2.588 0 00-3.61 0l-9.626 9.44a.863.863 0 01-1.203 0 .823.823 0 010-1.18l9.626-9.44a4.313 4.313 0 016.016 0 4.116 4.116 0 011.204 3.54 4.3 4.3 0 013.609 1.18l.05.05a4.115 4.115 0 010 5.9l-8.706 8.537a.274.274 0 000 .393l1.788 1.754a.823.823 0 010 1.18.863.863 0 01-1.203 0l-1.788-1.753a1.92 1.92 0 010-2.754l8.706-8.538a2.47 2.47 0 000-3.54l-.05-.049a2.588 2.588 0 00-3.607-.003l-7.172 7.034-.002.002-.098.097a.863.863 0 01-1.204 0 .823.823 0 010-1.18l7.273-7.133a2.47 2.47 0 00-.003-3.537z" />
		<path d="M14.485 4.703a.823.823 0 000-1.18.863.863 0 00-1.204 0l-7.119 6.982a4.115 4.115 0 000 5.9 4.314 4.314 0 006.016 0l7.12-6.982a.823.823 0 000-1.18.863.863 0 00-1.204 0l-7.119 6.982a2.588 2.588 0 01-3.61 0 2.47 2.47 0 010-3.54l7.12-6.982z" />
	</svg>
);

// Main navigation items

// External links
const externalLinks: any[] = [];

// Base promotional card (memoized outside component to prevent recreation)
const productionSetupHelpCard = {
	id: "production-setup",
	title: "Need help with production setup?",
	description: (
		<>
			We offer help with production setup including custom integrations and dedicated enterprise support.
		</>
	),
	dismissible: true,
};

// Sidebar item interface
interface SidebarItem {
	title: string;
	url: string;
	icon: React.ComponentType<{ className?: string }>;
	description: string;
	isAllowed?: boolean;
	hasAccess: boolean;
	subItems?: SidebarItem[];
	tag?: string;
	new?: boolean;
	isExternal?: boolean;
	queryParam?: string; // Optional: for tab-based subitems (e.g., "client-settings")
}

const getSidebarItemHref = (item: Pick<SidebarItem, "url" | "queryParam">) => {
	return item.queryParam ? `${item.url}?tab=${item.queryParam}` : item.url;
};

const splitUrlAndQuery = (url: string): { to: string; search?: any } => {
	const [to, query] = url.split("?");
	if (!query) return { to };
	const search = Object.fromEntries(new URLSearchParams(query).entries());
	return { to, search };
};

/** Active state for tab= queryParam sub-items (e.g. Browser AI). Default tab = overview. */
const isQueryParamTabActive = (pathname: string, search: string, subItem: Pick<SidebarItem, "url" | "queryParam">) => {
	if (!subItem.queryParam || pathname !== subItem.url) return false;
	const params = new URLSearchParams(search.startsWith("?") ? search.slice(1) : search);
	const tab = params.get("tab");
	if (!tab) return subItem.queryParam === "overview";
	return tab === subItem.queryParam;
};

const slug = (s: string) => s.toLowerCase().replace(/\s+/g, "-");

const TimeFilterPages = new Set(["/workspace/dashboard", "/workspace/logs", "/workspace/mcp-logs"]);

const preserveTimeFilters = (baseHref: string, subItemUrl: string, pathname: string, search: string): string => {
	if (TimeFilterPages.has(subItemUrl) && TimeFilterPages.has(pathname)) {
		const currentParams = new URLSearchParams(search);
		const startTime = currentParams.get("start_time");
		const endTime = currentParams.get("end_time");
		const period = currentParams.get("period");
		if ((startTime && endTime) || period) {
			const params = new URLSearchParams();
			if (startTime) params.set("start_time", startTime);
			if (endTime) params.set("end_time", endTime);
			if (period) params.set("period", period);
			const sep = baseHref.includes("?") ? "&" : "?";
			return `${baseHref}${sep}${params.toString()}`;
		}
	}
	return baseHref;
};

const SidebarItemView = ({
	item,
	isActive,
	isExternal,
	isWebSocketConnected,
	isExpanded,
	onToggle,
	pathname,
	search,
	isSidebarCollapsed,
	expandSidebar,
	highlightedUrl,
}: {
	item: SidebarItem;
	isActive: boolean;
	isExternal?: boolean;
	isWebSocketConnected: boolean;
	isExpanded?: boolean;
	onToggle?: () => void;
	pathname: string;
	search: string;
	isSidebarCollapsed: boolean;
	expandSidebar: () => void;
	highlightedUrl?: string;
}) => {
	const [flyoutOpen, setFlyoutOpen] = useState(false);
	const flyoutCloseTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
	const openFlyout = () => {
		if (flyoutCloseTimer.current) clearTimeout(flyoutCloseTimer.current);
		setFlyoutOpen(true);
	};
	const closeFlyout = () => {
		if (flyoutCloseTimer.current) clearTimeout(flyoutCloseTimer.current);
		flyoutCloseTimer.current = setTimeout(() => {
			setFlyoutOpen(false);
			flyoutCloseTimer.current = null;
		}, 80);
	};
	useEffect(() => {
		return () => {
			if (flyoutCloseTimer.current) clearTimeout(flyoutCloseTimer.current);
		};
	}, []);
	const hasSubItems = "subItems" in item && item.subItems && item.subItems.length > 0;
	const isRouteMatch = (url: string) => {
		// Exact-match base paths that have sibling tab routes nested under them, so the base
		// tab isn't also highlighted when a child tab (e.g. /settings) is active.
		if (url === "/workspace/custom-pricing" || url === "/workspace/adaptive-routing") return pathname === url;
		// Avoid double-highlighting with "/workspace/mcp-registry/library"
		if (url === "/workspace/mcp-registry") return !pathname.startsWith("/workspace/mcp-registry/library") && pathname.startsWith(url);
		return pathname.startsWith(url);
	};
	const isAnySubItemActive =
		hasSubItems &&
		item.subItems?.some((subItem) => {
			return isRouteMatch(subItem.url);
		});

	const handleClick = (e: React.MouseEvent) => {
		if (hasSubItems && item.hasAccess) {
			e.preventDefault();
			// If sidebar is collapsed, expand it first then toggle the submenu
			if (isSidebarCollapsed) {
				expandSidebar();
				// Small delay to allow sidebar to expand before toggling submenu
				setTimeout(() => {
					if (onToggle) onToggle();
				}, 100);
			} else if (onToggle) {
				onToggle();
			}
		}
	};

	const isHighlighted = !hasSubItems && highlightedUrl === item.url;

	const buttonClassName = `group/nav-item relative h-7.5 cursor-pointer rounded-sm border px-3 transition-all duration-200 ${isHighlighted
			? "bg-sidebar-accent text-accent-foreground border-primary/20"
			: isActive || isAnySubItemActive
				? "bg-sidebar-accent text-primary border-primary/20"
				: item.hasAccess
					? "hover:bg-sidebar-accent hover:text-accent-foreground border-transparent text-slate-500 dark:text-zinc-400"
					: "hover:bg-destructive/5 hover:text-muted-foreground text-muted-foreground cursor-not-allowed border-transparent"
		} `;

	const innerContent = (
		<div className="flex w-full items-center justify-between">
			<div className="flex w-full items-center gap-2">
				<item.icon className={`h-4 w-4 shrink-0 ${isActive || isAnySubItemActive ? "text-primary" : "text-muted-foreground"}`} />
				<span className={`text-sm group-data-[collapsible=icon]:hidden ${isActive || isAnySubItemActive ? "font-medium" : "font-normal"}`}>
					{item.title}
				</span>
				{item.new && (
					<Badge data-new-badge="true" className={cn("ml-auto group-data-[collapsible=icon]:hidden", newBadgeClassName)}>
						New
					</Badge>
				)}
				{item.tag && (
					<Badge variant="secondary" className="text-muted-foreground ml-auto text-xs group-data-[collapsible=icon]:hidden">
						{item.tag}
					</Badge>
				)}
			</div>
			{hasSubItems && (
				<ChevronRight
					className={`h-4 w-4 transition-transform duration-200 group-data-[collapsible=icon]:hidden ${isExpanded ? "rotate-90" : ""}`}
				/>
			)}
			{!hasSubItems && item.url === "/logs" && isWebSocketConnected && (
				<div className="h-2 w-2 animate-pulse rounded-full bg-green-800 dark:bg-green-200" />
			)}
			{isExternal && <ArrowUpRight className="text-muted-foreground h-4 w-4 group-data-[collapsible=icon]:hidden" size={16} />}
		</div>
	);

	// Render strategy:
	//   - Items with sub-items: <button> (toggle, not navigation)
	//   - Leaf items, no access: <button> (disabled-style, non-clickable)
	//   - Leaf items, external:  <a target="_blank">
	//   - Leaf items, internal:  TanStack <Link> with preload-on-hover
	let menuButton: React.ReactNode;
	if (hasSubItems) {
		menuButton = (
			<SidebarMenuButton tooltip={isSidebarCollapsed ? undefined : item.title} className={buttonClassName} onClick={handleClick}>
				{innerContent}
			</SidebarMenuButton>
		);
	} else if (!item.hasAccess) {
		menuButton = (
			<SidebarMenuButton tooltip={item.title} data-nav-url={item.url} className={buttonClassName}>
				{innerContent}
			</SidebarMenuButton>
		);
	} else if (isExternal) {
		menuButton = (
			<SidebarMenuButton asChild tooltip={item.title} className={buttonClassName}>
				<a
					href={item.url}
					target="_blank"
					rel="noopener noreferrer"
					data-nav-url={item.url}
					onClick={isSidebarCollapsed ? (e: React.MouseEvent) => e.stopPropagation() : undefined}
				>
					{innerContent}
				</a>
			</SidebarMenuButton>
		);
	} else {
		const { to: itemTo, search: itemSearch } = splitUrlAndQuery(item.url);
		menuButton = (
			<SidebarMenuButton asChild tooltip={item.title} className={buttonClassName}>
				<Link
					to={itemTo as any}
					search={itemSearch as any}
					preload="intent"
					data-nav-url={item.url}
					onClick={isSidebarCollapsed ? (e: React.MouseEvent) => e.stopPropagation() : undefined}
				>
					{innerContent}
				</Link>
			</SidebarMenuButton>
		);
	}

	return (
		<SidebarMenuItem key={item.title}>
			{isSidebarCollapsed && hasSubItems ? (
				<Popover open={flyoutOpen} onOpenChange={setFlyoutOpen}>
					<PopoverTrigger asChild onMouseEnter={openFlyout} onMouseLeave={closeFlyout}>
						<div data-testid={`sidebar-flyout-trigger-${slug(item.title)}`}>{menuButton}</div>
					</PopoverTrigger>
					<PopoverContent
						side="right"
						align="start"
						sideOffset={8}
						className="w-48 p-1"
						onMouseEnter={openFlyout}
						onMouseLeave={closeFlyout}
						data-testid={`sidebar-flyout-content-${slug(item.title)}`}
					>
						<div className="text-muted-foreground px-2 py-1.5 text-xs font-medium">{item.title}</div>
						{item.subItems?.map((subItem) => {
							const baseHref = getSidebarItemHref(subItem);
							const href = preserveTimeFilters(baseHref, subItem.url, pathname, search);
							const isSubItemActive = subItem.queryParam
								? isQueryParamTabActive(pathname, search, subItem)
								: isRouteMatch(subItem.url);
							const SubItemIcon = subItem.icon;
							const subSlug = slug(subItem.title);
							const inner = (
								<div className="flex items-center gap-2">
									{SubItemIcon && <SubItemIcon className={`h-3.5 w-3.5 ${isSubItemActive ? "text-primary" : "text-muted-foreground"}`} />}
									<span className={`text-sm ${isSubItemActive ? "text-primary font-medium" : "text-slate-500 dark:text-zinc-400"}`}>
										{subItem.title}
									</span>
									{subItem.new && (
										<Badge data-new-badge="true" className={cn("ml-auto", newBadgeClassName)}>
											New
										</Badge>
									)}
									{subItem.tag && (
										<Badge variant="secondary" className="text-muted-foreground ml-auto text-xs">
											{subItem.tag}
										</Badge>
									)}
								</div>
							);
							const { to: flyoutTo, search: flyoutSearch } = splitUrlAndQuery(href);
							return (
								<div key={subItem.title} data-testid={`sidebar-flyout-subitem-${subSlug}`} onClick={() => setFlyoutOpen(false)}>
									{subItem.hasAccess === false ? (
										<div
											data-testid={`sidebar-subitem-disabled-${subSlug}`}
											className="text-muted-foreground hover:bg-destructive/5 flex h-7 cursor-not-allowed items-center rounded-sm px-2"
										>
											{inner}
										</div>
									) : (
										<Link
											to={flyoutTo as any}
											search={flyoutSearch as any}
											preload="intent"
											data-testid={`sidebar-subitem-link-${subSlug}`}
											className={`flex h-7 items-center rounded-sm px-2 ${isSubItemActive ? "bg-sidebar-accent" : "hover:bg-sidebar-accent"}`}
										>
											{inner}
										</Link>
									)}
								</div>
							);
						})}
					</PopoverContent>
				</Popover>
			) : (
				menuButton
			)}
			{hasSubItems && isExpanded && (
				<SidebarMenuSub className="border-sidebar-border mt-1 ml-4 space-y-0.5 border-l pl-2">
					{item.subItems?.map((subItem: SidebarItem) => {
						const baseHref = getSidebarItemHref(subItem);
						const subItemHref = preserveTimeFilters(baseHref, subItem.url, pathname, search);
						const isSubItemActive = subItem.queryParam
							? isQueryParamTabActive(pathname, search, subItem)
							: isRouteMatch(subItem.url);
						const isSubItemHighlighted = highlightedUrl ? subItemHref.startsWith(highlightedUrl) : false;
						const SubItemIcon = subItem.icon;
						const subItemClassName = `group/nav-item h-7 cursor-pointer rounded-sm px-2 transition-all duration-200 ${isSubItemHighlighted
								? "bg-sidebar-accent text-accent-foreground"
								: isSubItemActive
									? "bg-sidebar-accent text-primary font-medium"
									: subItem.hasAccess === false
										? "hover:bg-destructive/5 hover:text-muted-foreground text-muted-foreground cursor-not-allowed border-transparent"
										: "hover:bg-sidebar-accent hover:text-accent-foreground text-slate-500 dark:text-zinc-400"
							}`;
						const subInner = (
							<div className="flex w-full items-center gap-2">
								{SubItemIcon && <SubItemIcon className={`h-3.5 w-3.5 ${isSubItemActive ? "text-primary" : "text-muted-foreground"}`} />}
								<span className={`text-sm ${isSubItemActive ? "font-medium" : "font-normal"}`}>{subItem.title}</span>
								{subItem.new && (
									<Badge data-new-badge="true" className={cn("ml-auto", newBadgeClassName)}>
										New
									</Badge>
								)}
								{subItem.tag && (
									<Badge variant="secondary" className="text-muted-foreground ml-auto text-xs">
										{subItem.tag}
									</Badge>
								)}
							</div>
						);
						const { to: subItemTo, search: subItemSearch } = splitUrlAndQuery(subItemHref);
						return (
							<SidebarMenuSubItem key={subItem.title}>
								{subItem.hasAccess === false ? (
									<SidebarMenuSubButton data-nav-url={subItemHref} className={subItemClassName}>
										{subInner}
									</SidebarMenuSubButton>
								) : (
									<SidebarMenuSubButton asChild className={subItemClassName}>
										<Link to={subItemTo as any} search={subItemSearch as any} preload="intent" data-nav-url={subItemHref}>
											{subInner}
										</Link>
									</SidebarMenuSubButton>
								)}
							</SidebarMenuSubItem>
						);
					})}
				</SidebarMenuSub>
			)}
		</SidebarMenuItem>
	);
};

function displayAppVersion(version?: string): string {
	const value = version?.trim() ?? "";
	if (!value || value === "v" || /^v?unknown$/i.test(value)) {
		return "";
	}
	return value;
}

// Helper function to compare semantic versions
const compareVersions = (v1: string, v2: string): number => {
	// Remove 'v' prefix if present
	const cleanV1 = v1.startsWith("v") ? v1.slice(1) : v1;
	const cleanV2 = v2.startsWith("v") ? v2.slice(1) : v2;

	// Split into main version and prerelease
	const [mainV1, prereleaseV1] = cleanV1.split("-");
	const [mainV2, prereleaseV2] = cleanV2.split("-");

	// Compare main version numbers (major.minor.patch)
	const partsV1 = mainV1.split(".").map(Number);
	const partsV2 = mainV2.split(".").map(Number);

	for (let i = 0; i < Math.max(partsV1.length, partsV2.length); i++) {
		const num1 = partsV1[i] || 0;
		const num2 = partsV2[i] || 0;

		if (num1 > num2) return 1;
		if (num1 < num2) return -1;
	}

	// If main versions are equal, check prerelease
	// Version without prerelease is higher than version with prerelease
	if (!prereleaseV1 && prereleaseV2) return 1;
	if (prereleaseV1 && !prereleaseV2) return -1;

	// Both have prereleases, compare them
	if (prereleaseV1 && prereleaseV2) {
		// Extract prerelease number (e.g., "prerelease1" -> 1)
		const prereleaseNum1 = parseInt(prereleaseV1.replace(/\D/g, "")) || 0;
		const prereleaseNum2 = parseInt(prereleaseV2.replace(/\D/g, "")) || 0;

		if (prereleaseNum1 > prereleaseNum2) return 1;
		if (prereleaseNum1 < prereleaseNum2) return -1;
	}
	return 0;
};

export default function AppSidebar() {
	const { data: authStatus } = useIsAuthEnabledQuery();
	const isAuthEnabled = authStatus?.is_auth_enabled || authStatus?.has_valid_token || false;
	const scopedSidebarSections = useMemo(() => {
		// Only admins and sub-admins get workspace section scopes.
		if (authStatus?.role === "admin" || authStatus?.role === "sub_admin") {
			return parseAdminAllowedSections(authStatus.allowed_sections);
		}
		return null;
	}, [authStatus?.role, authStatus?.allowed_sections]);
	const pathname = useLocation({ select: (l) => l.pathname });
	const search = useLocation({ select: (l) => l.searchStr ?? "" });
	const tsNavigate = useNavigate();
	// Wrapper that accepts arbitrary string URLs (TanStack Router's `to` is
	// strictly typed, but our sidebar items come from a runtime config).
	const navigate = useCallback((url: string) => {
		const { to, search } = splitUrlAndQuery(url);
		tsNavigate({ to: to as any, search: search as any });
	}, [tsNavigate]);
	const [mounted, setMounted] = useState(false);
	const [expandedItems, setExpandedItems] = useState<Set<string>>(new Set());
	const [areCardsEmpty, setAreCardsEmpty] = useState(false);
	const [userPopoverOpen, setUserPopoverOpen] = useState(false);
	const [searchQuery, setSearchQuery] = useState("");
	const [focusedIndex, setFocusedIndex] = useState(-1);
	const searchInputRef = useRef<HTMLInputElement>(null);
	const [cookies, setCookie] = useCookies([PRODUCTION_SETUP_DISMISSED_COOKIE]);
	const isProductionSetupDismissed = !!cookies[PRODUCTION_SETUP_DISMISSED_COOKIE];
	const { data: latestRelease } = useGetLatestReleaseQuery(undefined, {
		skip: !mounted, // Only fetch after component is mounted
	});
	const hasLogsAccess = useRbac(RbacResource.Logs, RbacOperation.View);
	const hasObservabilityAccess = useRbac(RbacResource.Observability, RbacOperation.View);
	const hasDashboardAccess = useRbac(RbacResource.Dashboard, RbacOperation.View);
	const hasModelProvidersAccess = useRbac(RbacResource.ModelProvider, RbacOperation.View);
	const hasMCPGatewayAccess = useRbac(RbacResource.MCPGateway, RbacOperation.View);
	const hasMCPToolGroupsAccess = useRbac(RbacResource.MCPToolGroups, RbacOperation.View);
	const hasMCPLogsAccess = useRbac(RbacResource.MCPLogs, RbacOperation.View);
	const hasPluginsAccess = useRbac(RbacResource.Plugins, RbacOperation.View);
	// /api/session/users is admin/sub_admin only on the backend, so custom roles cannot load this page.
	const hasUsersRbac = useRbac(RbacResource.Users, RbacOperation.View);
	const hasUsersAccess = hasUsersRbac && (authStatus?.role === "admin" || authStatus?.role === "sub_admin");
	const hasUserProvisioningAccess = useRbac(RbacResource.UserProvisioning, RbacOperation.View);
	const hasAuditLogsAccess = useRbac(RbacResource.AuditLogs, RbacOperation.View);
	const hasCustomersAccess = useRbac(RbacResource.Customers, RbacOperation.View);
	const hasTeamsAccess = useRbac(RbacResource.Teams, RbacOperation.View);
	// Align with backend PathRequirement: business-units APIs use Governance resource.
	const hasBusinessUnitsAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	const hasRbacAccess = useRbac(RbacResource.RBAC, RbacOperation.View);
	const hasVirtualKeysAccess = useRbac(RbacResource.VirtualKeys, RbacOperation.View);
	const hasGovernanceLegacyAccess = useRbac(RbacResource.Governance, RbacOperation.View);
	const hasRoutingRulesAccess = useRbac(RbacResource.RoutingRules, RbacOperation.View);
	const hasGuardrailsProvidersAccess = useRbac(RbacResource.GuardrailsProviders, RbacOperation.View);
	const hasGuardrailsConfigAccess = useRbac(RbacResource.GuardrailsConfig, RbacOperation.View);
	const hasCircuitBreakerAccess = useRbac(RbacResource.CircuitBreaker, RbacOperation.View);
	const hasClusterConfigAccess = useRbac(RbacResource.Cluster, RbacOperation.View);
	const isAdaptiveRoutingAllowed = useRbac(RbacResource.AdaptiveRouter, RbacOperation.View);
	const hasSettingsAccess = useRbac(RbacResource.Settings, RbacOperation.View);
	const hasFeatureFlagsAccess = useRbac(RbacResource.FeatureFlags, RbacOperation.View);
	const hasAPIKeyAccess = useRbac(RbacResource.APIKeys, RbacOperation.View);
	const hasPromptRepositoryAccess = useRbac(RbacResource.PromptRepository, RbacOperation.View);
	const hasSkillsRepositoryAccess = useRbac(RbacResource.SkillsRepository, RbacOperation.View);
	const hasAccessProfilesAccess = useRbac(RbacResource.AccessProfiles, RbacOperation.View);
	// Browser AI APIs map to Logs RBAC; admins/sub_admins also reach it via workspace section grants.
	const hasBrowserAiAccess =
		hasLogsAccess || authStatus?.role === "admin" || authStatus?.role === "sub_admin";
	const hasAnyGovernanceAccess =
		hasVirtualKeysAccess ||
		hasTeamsAccess ||
		hasUsersAccess ||
		hasCustomersAccess ||
		hasBusinessUnitsAccess ||
		hasRbacAccess ||
		hasAccessProfilesAccess ||
		hasGovernanceLegacyAccess ||
		hasAuditLogsAccess;
	const { data: coreConfig } = useGetCoreConfigQuery({});
	const isDbConnected = coreConfig?.is_db_connected ?? false;
	const envLabel = coreConfig?.env_label ?? null;

	const items = useMemo(() => {
		const allItems = [
			{
				title: "Observability",
				url: "/workspace/logs",
				icon: Telescope,
				description: "Request logs & monitoring",
				hasAccess: hasLogsAccess,
				subItems: [
					{
						title: "Dashboard",
						url: "/workspace/dashboard",
						icon: ChartColumnBig,
						description: "Dashboard",
						hasAccess: hasDashboardAccess,
					},
					{
						title: "LLM Logs",
						url: "/workspace/logs",
						icon: Logs,
						description: "LLM request logs & monitoring",
						hasAccess: hasLogsAccess,
					},
					{
						title: "MCP Logs",
						url: "/workspace/mcp-logs",
						icon: MCPIcon,
						description: "MCP tool execution logs",
						hasAccess: hasMCPLogsAccess,
					},
					{
						title: "Connectors",
						url: "/workspace/observability",
						icon: ChevronsLeftRightEllipsis,
						description: "Log connectors",
						hasAccess: hasObservabilityAccess,
					},
					{
						title: "Logs Settings",
						url: "/workspace/config/logging",
						icon: Settings,
						description: "Logs configuration",
						hasAccess: hasSettingsAccess,
					},
				],
			},
			{
				title: "Browser AI",
				url: "/workspace/browser-ai",
				icon: Globe,
				description: "Browser AI observability & DLP proxy",
				hasAccess: hasBrowserAiAccess,
				subItems: [
					{
						title: "Overview",
						url: "/workspace/browser-ai",
						icon: Activity,
						description: "Browser AI overview",
						hasAccess: hasBrowserAiAccess,
						queryParam: "overview",
					},
					{
						title: "Target Websites",
						url: "/workspace/browser-ai",
						icon: Globe,
						description: "Monitored / blocked websites",
						hasAccess: hasBrowserAiAccess,
						queryParam: "targets",
					},
					{
						title: "Guard Rules",
						url: "/workspace/browser-ai",
						icon: Shield,
						description: "DLP guard rules",
						hasAccess: hasBrowserAiAccess,
						queryParam: "rules",
					},
					{
						title: "Prompt Logs",
						url: "/workspace/browser-ai",
						icon: FileText,
						description: "Intercepted prompt logs",
						hasAccess: hasBrowserAiAccess,
						queryParam: "logs",
					},
					{
						title: "Search Logs",
						url: "/workspace/browser-ai",
						icon: Search,
						description: "Browser search query logs",
						hasAccess: hasBrowserAiAccess,
						queryParam: "search-logs",
					},
					{
						title: "Setup",
						url: "/workspace/browser-ai",
						icon: Terminal,
						description: "Guard install & PAC setup",
						hasAccess: hasBrowserAiAccess,
						queryParam: "setup",
					},
					{
						title: "Guard Agents",
						url: "/workspace/browser-ai",
						icon: Radio,
						description: "Installed Guard agents",
						hasAccess: hasBrowserAiAccess,
						queryParam: "agents",
					},
					{
						title: "Guard Insights",
						url: "/workspace/browser-ai",
						icon: ChartColumnBig,
						description: "Per-host allow / block / warn / redact activity",
						hasAccess: hasBrowserAiAccess,
						queryParam: "telemetry",
					},
				],
			},
			{
				title: "Models",
				url: "/workspace/providers",
				icon: BoxIcon,
				description: "Configure models",
				hasAccess: true,
				subItems: [
					{
						title: "Model Providers",
						url: "/workspace/providers",
						icon: Boxes,
						description: "Configure models",
						hasAccess: hasModelProvidersAccess,
					},
					{
						title: "Model Catalog",
						url: "/workspace/model-catalog",
						icon: LayoutGrid,
						description: "Providers, usage overview, and model attributes",
						hasAccess: hasModelProvidersAccess,
					},
					{
						title: "Budgets & Limits",
						url: "/workspace/model-limits",
						icon: Wallet,
						description: "Per-model budgets and rate limits",
						hasAccess: hasGovernanceLegacyAccess,
					},
					{
						title: "Complexity Router",
						url: "/workspace/complexity-router",
						icon: GitCompareArrows,
						description: "Sets complexity_tier for Routing Rules",
						hasAccess: hasRoutingRulesAccess,
					},
					{
						title: "Routing Rules",
						url: "/workspace/routing-rules",
						icon: Network,
						description: "CEL rules for provider/model selection",
						hasAccess: hasRoutingRulesAccess,
					},
					{
						title: "Circuit Breaker",
						url: "/workspace/circuit-breaker",
						icon: CircuitBoard,
						description: "Header-signal failover (not CEL routing)",
						hasAccess: hasCircuitBreakerAccess,
					},
					{
						title: "Pricing Overrides",
						url: "/workspace/custom-pricing/overrides",
						icon: SlidersHorizontal,
						description: "Scoped pricing overrides",
						hasAccess: hasSettingsAccess,
					},
					{
						title: "Model Settings",
						url: "/workspace/custom-pricing",
						icon: Settings,
						description: "Model and routing configuration",
						hasAccess: hasSettingsAccess,
					},
				],
			},
			{
				title: "MCP Gateway",
				icon: MCPIcon,
				description: "MCP configuration",
				url: "/workspace/mcp-registry",
				hasAccess: hasMCPGatewayAccess || hasMCPToolGroupsAccess,
				subItems: [
					{
						title: "MCP Catalog",
						url: "/workspace/mcp-registry",
						icon: LayoutGrid,
						description: "MCP tool catalog",
						hasAccess: hasMCPGatewayAccess,
					},
					{
						title: "MCP Library",
						url: "/workspace/mcp-registry/library",
						icon: Boxes,
						description: "Install curated MCP servers",
						hasAccess: hasMCPGatewayAccess,
					},
					{
						title: "Tool Groups",
						url: "/workspace/mcp-tool-groups",
						icon: ToolCase,
						description: "Tool Groups",
						hasAccess: hasMCPToolGroupsAccess,
					},
					{
						title: "Auth Sessions",
						url: "/workspace/mcp-sessions",
						icon: KeyRound,
						description: "Per-user OAuth sessions",
						hasAccess: hasMCPGatewayAccess,
					},
					{
						title: "OAuth Grants",
						url: "/workspace/oauth-grants",
						icon: ShieldCheck,
						description: "Downstream OAuth grants",
						hasAccess: hasMCPGatewayAccess,
					},
					{
						title: "MCP Settings",
						url: "/workspace/mcp-settings",
						icon: Settings,
						description: "MCP configuration",
						hasAccess: hasMCPGatewayAccess,
					},
				],
			},
			{
				title: "Plugins",
				url: "/workspace/plugins",
				icon: Puzzle,
				description: "Manage custom plugins",
				hasAccess: hasPluginsAccess,
			},
			{
				title: "Governance",
				url: "/workspace/governance",
				icon: Landmark,
				description: "Virtual keys, users, teams, customers & roles",
				hasAccess: hasAnyGovernanceAccess,
				subItems: [
					{
						title: "Virtual Keys",
						url: "/workspace/governance/virtual-keys",
						icon: KeyRound,
						description: "Manage virtual keys & access",
						hasAccess: hasVirtualKeysAccess,
					},
					{
						title: "Users",
						url: "/workspace/governance/users",
						icon: Users,
						description: "Manage users",
						hasAccess: hasUsersAccess,
					},
					{
						title: "Teams",
						url: "/workspace/governance/teams",
						icon: Building,
						description: "Manage teams",
						hasAccess: hasTeamsAccess,
					},
					{
						title: "Business Units",
						url: "/workspace/governance/business-units",
						icon: Building2,
						description: "Manage business units",
						hasAccess: hasBusinessUnitsAccess,
					},
					{
						title: "Customers",
						url: "/workspace/governance/customers",
						icon: WalletCards,
						description: "Manage customers",
						hasAccess: hasCustomersAccess,
					},
					{
						title: "User Provisioning",
						url: "/workspace/scim",
						icon: BookUser,
						description: "User management and provisioning",
						hasAccess: hasUserProvisioningAccess,
					},
					{
						title: "Roles & Permissions",
						url: "/workspace/governance/rbac",
						icon: UserRoundCheck,
						description: "User roles and permissions",
						hasAccess: hasRbacAccess,
					},
					{
						title: "Access Profiles",
						url: "/workspace/governance/access-profiles",
						icon: ShieldCheck,
						description: "Manage access profiles for roles",
						hasAccess: hasAccessProfilesAccess,
					},
					{
						title: "Audit Logs",
						url: "/workspace/audit-logs",
						icon: ScrollText,
						description: "Audit logs and compliance",
						hasAccess: hasAuditLogsAccess,
					},
				],
			},
			{
				title: "Guardrails",
				url: "/workspace/guardrails",
				icon: Construction,
				description: "Guardrails configuration",
				hasAccess: hasGuardrailsConfigAccess || hasGuardrailsProvidersAccess || hasClusterConfigAccess,
				subItems: [
					{
						title: "Rules",
						url: "/workspace/guardrails/configuration",
						icon: SearchCheck,
						description: "Guardrail rules",
						hasAccess: hasGuardrailsConfigAccess,
					},
					{
						title: "Rule Providers",
						url: "/workspace/guardrails/providers",
						icon: Boxes,
						description: "Rule providers configuration",
						hasAccess: hasGuardrailsProvidersAccess,
					},
					{
						title: "Cluster Config",
						url: "/workspace/cluster",
						icon: Network,
						description: "Manage cluster",
						hasAccess: hasClusterConfigAccess,
					},
				],
			},
			{
				title: "Adaptive Routing",
				url: "/workspace/adaptive-routing",
				icon: Shuffle,
				description: "Manage adaptive routing",
				hasAccess: isAdaptiveRoutingAllowed,
				subItems: [
					{
						title: "Dashboard",
						url: "/workspace/adaptive-routing",
						icon: ChartColumnBig,
						description: "Adaptive routing metrics",
						hasAccess: isAdaptiveRoutingAllowed,
					},
					{
						title: "Settings",
						url: "/workspace/adaptive-routing/settings",
						icon: Settings,
						description: "Adaptive routing settings",
						hasAccess: isAdaptiveRoutingAllowed,
					},
				],
			},
			...(isDbConnected
				? [
					{
						title: "Prompt Repository",
						url: "/workspace/prompt-repo",
						icon: FolderGit,
						description: "Prompt repository",
						hasAccess: hasPromptRepositoryAccess,
					},
					{
						title: "Skills Repository",
						url: "/workspace/skills-repo",
						icon: BookOpenText,
						description: "Skills repository",
						hasAccess: hasSkillsRepositoryAccess,
					},
				]
				: []),

			{
				title: "Settings",
				url: "/workspace/config",
				icon: Settings2Icon,
				description: "Settings",
				hasAccess: hasSettingsAccess || hasAuditLogsAccess || hasUserProvisioningAccess,
				subItems: [
					{
						title: "Client Settings",
						url: "/workspace/config/client-settings",
						icon: Settings,
						description: "Client configuration settings",
						hasAccess: hasSettingsAccess,
					},
					{
						title: "Compatibility",
						url: "/workspace/config/compatibility",
						icon: Plug,
						description: "Compatibility conversion settings",
						hasAccess: hasSettingsAccess,
					},
					{
						title: "Caching",
						url: "/workspace/config/caching",
						icon: DatabaseZap,
						description: "Caching configuration",
						hasAccess: hasSettingsAccess,
					},
					{
						title: "Security",
						url: "/workspace/config/security",
						icon: ShieldCheck,
						description: "Security settings",
						hasAccess: hasSettingsAccess,
					},
					...(IS_ENTERPRISE
						? [
							{
								title: "Proxy",
								url: "/workspace/config/proxy",
								icon: Globe,
								description: "Proxy configuration",
								hasAccess: hasSettingsAccess,
							},
						]
						: []),
					{
						title: "API Keys",
						url: "/workspace/config/api-keys",
						icon: KeyRound,
						description: "API keys management",
						hasAccess: hasAPIKeyAccess,
					},
					{
						title: "Performance Tuning",
						url: "/workspace/config/performance-tuning",
						icon: TrendingUp,
						description: "Performance tuning settings",
						hasAccess: hasSettingsAccess,
					},
					{
						title: "Feature Flags",
						url: "/workspace/config/feature-flags",
						icon: Flag,
						description: "Toggle feature flags",
						hasAccess: hasFeatureFlagsAccess,
					},
				],
			},
		];
		// User role is locked to Prompt Repository. Other roles (admin, sub_admin) are filtered by RBAC / scopes.
		if (authStatus?.role && authStatus.role === "user") {
			return allItems.filter((item) => item.title === "Prompt Repository");
		}
		if (scopedSidebarSections) {
			return allItems
				.map((item) => {
					const sectionKey = SECTION_KEY_BY_TITLE[item.title] as WorkspaceSectionKey | undefined;
					if (!sectionKey || !isSectionGranted(sectionKey, scopedSidebarSections)) {
						return null;
					}
					if (item.subItems?.length) {
						const visibleSubItems = item.subItems.filter((sub) =>
							isSidebarItemGranted(sectionKey, getSidebarItemHref(sub), scopedSidebarSections),
						);
						if (visibleSubItems.length === 0) return null;
						return { ...item, subItems: visibleSubItems };
					}
					return item;
				})
				.filter(Boolean) as typeof allItems;
		}
		return allItems;
	}, [
		hasLogsAccess,
		hasBrowserAiAccess,
		hasAPIKeyAccess,
		hasObservabilityAccess,
		hasDashboardAccess,
		hasModelProvidersAccess,
		hasMCPGatewayAccess,
		hasMCPToolGroupsAccess,
		hasMCPLogsAccess,
		hasPluginsAccess,
		hasUsersAccess,
		hasUserProvisioningAccess,
		hasAuditLogsAccess,
		hasCustomersAccess,
		hasTeamsAccess,
		hasBusinessUnitsAccess,
		hasRbacAccess,
		hasVirtualKeysAccess,
		hasGovernanceLegacyAccess,
		hasAnyGovernanceAccess,
		hasRoutingRulesAccess,
		hasGuardrailsProvidersAccess,
		hasGuardrailsConfigAccess,
		hasCircuitBreakerAccess,
		hasClusterConfigAccess,
		isAdaptiveRoutingAllowed,
		hasSettingsAccess,
		hasFeatureFlagsAccess,
		hasPromptRepositoryAccess,
		hasSkillsRepositoryAccess,
		hasAccessProfilesAccess,
		isDbConnected,
		authStatus?.role,
		scopedSidebarSections,
	]);

	const accessibleItems: SidebarItem[] = useMemo(() => {
		return items
			.map((item) => {
				const hadSubItems = !!item.subItems?.length;
				if (hadSubItems) {
					const visibleSubItems = item.subItems!.filter((sub) => sub.hasAccess !== false);
					if (visibleSubItems.length === 0) return null;
					return { ...item, subItems: visibleSubItems, hasAccess: true };
				}
				if (item.hasAccess === false) return null;
				return item;
			})
			.filter(Boolean) as SidebarItem[];
	}, [items]);

	const filteredItems: SidebarItem[] = useMemo(() => {
		const query = searchQuery.trim().toLowerCase();
		if (!query) return accessibleItems;

		return accessibleItems
			.map((item) => {
				const parentMatches = item.title.toLowerCase().includes(query);
				if (parentMatches) return item;

				if (item.subItems) {
					const matchingSubItems = item.subItems.filter((sub) => sub.title.toLowerCase().includes(query));
					if (matchingSubItems.length > 0) {
						return { ...item, subItems: matchingSubItems };
					}
				}
				return null;
			})
			.filter(Boolean) as SidebarItem[];
	}, [accessibleItems, searchQuery]);

	const { data: version } = useGetVersionQuery();
	const visibleVersion = displayAppVersion(version);
	const { resolvedTheme } = useTheme();
	const [logout] = useLogoutMutation();

	// Get user info from localStorage (for enterprise SCIM OAuth)
	const [userInfo, setUserInfo] = useState<UserInfo | null>(null);

	useEffect(() => {
		if (IS_ENTERPRISE) {
			const info = getUserInfo();
			setUserInfo(info);
		}
	}, []);

	const showNewReleaseBanner = useMemo(() => {
		if (IS_ENTERPRISE) return false;
		if (latestRelease && visibleVersion) {
			return compareVersions(latestRelease.name, visibleVersion) > 0;
		}
		return false;
	}, [latestRelease, visibleVersion]);

	useEffect(() => {
		setMounted(true);
	}, []);

	useEffect(() => {
		// Plain "user" role is confined to Prompt Repository (URL hard-lock).
		// admin / sub_admin use workspace section grants (allowed_sections).
		if (authStatus?.role === "user") {
			if (!pathname.startsWith("/workspace/prompt-repo")) {
				navigate("/workspace/prompt-repo");
			}
			return;
		}
		if (scopedSidebarSections) {
			const pathWithTab =
				pathname === "/workspace/browser-ai" && search
					? `${pathname}${search.startsWith("?") ? search : `?${search}`}`
					: pathname;
			if (!isPathAllowedForUser(pathWithTab, scopedSidebarSections)) {
				navigate(getDefaultPathForSections(scopedSidebarSections));
			}
		}
	}, [authStatus?.role, scopedSidebarSections, pathname, search, navigate]);

	// Auto-expand items when their subitems are active
	useEffect(() => {
		const newExpandedItems = new Set<string>();
		const isRouteMatch = (url: string) => {
			if (url === "/workspace/custom-pricing" || url === "/workspace/adaptive-routing") return pathname === url;
			return pathname.startsWith(url);
		};
		items.forEach((item) => {
			if (item.subItems?.some((subItem) => isRouteMatch(subItem.url))) {
				newExpandedItems.add(item.title);
			}
		});
		if (newExpandedItems.size > 0) {
			setExpandedItems((prev) => new Set([...prev, ...newExpandedItems]));
		}
	}, [pathname, items]);

	// Auto-expand parents when search matches their subItems
	useEffect(() => {
		const query = searchQuery.trim().toLowerCase();
		if (!query) return;
		const toExpand = new Set<string>();
		items.forEach((item) => {
			if (!item.subItems?.length) return;
			const parentMatches = item.title.toLowerCase().includes(query);
			if (parentMatches) return;
			const hasMatchingChild = item.subItems.some((sub) => sub.title.toLowerCase().includes(query));
			if (hasMatchingChild) {
				toExpand.add(item.title);
			}
		});
		if (toExpand.size > 0) {
			setExpandedItems((prev) => {
				const hasAll = [...toExpand].every((t) => prev.has(t));
				if (hasAll) return prev;
				return new Set([...prev, ...toExpand]);
			});
		}
	}, [searchQuery, items]);

	// Cmd+K to focus search input
	useEffect(() => {
		const handleKeyDown = (event: KeyboardEvent) => {
			if (event.key === "k" && (event.metaKey || event.ctrlKey)) {
				event.preventDefault();
				searchInputRef.current?.focus();
			}
		};
		window.addEventListener("keydown", handleKeyDown);
		return () => window.removeEventListener("keydown", handleKeyDown);
	}, []);

	// Flat list of navigable items for keyboard navigation
	const navigableItems = useMemo(() => {
		const result: {
			title: string;
			url: string;
			queryParam?: string;
			isExternal?: boolean;
		}[] = [];
		for (const item of filteredItems) {
			if (item.isExternal) {
				if (item.hasAccess) result.push({ title: item.title, url: item.url, isExternal: true });
				continue;
			}
			const hasSubItems = item.subItems && item.subItems.length > 0;
			if (hasSubItems) {
				// When search is active or parent is expanded, include visible subItems
				if (searchQuery.trim() || expandedItems.has(item.title)) {
					for (const sub of item.subItems!) {
						if (sub.hasAccess === false) continue;
						result.push({
							title: sub.title,
							url: getSidebarItemHref(sub),
							queryParam: sub.queryParam,
						});
					}
				} else {
					// Parent is collapsed - include parent as a toggle target
					if (item.hasAccess) result.push({ title: item.title, url: item.url });
				}
			} else {
				if (item.hasAccess) result.push({ title: item.title, url: item.url });
			}
		}
		return result;
	}, [filteredItems, expandedItems, searchQuery]);

	const handleSearchKeyDown = useCallback(
		(e: React.KeyboardEvent<HTMLInputElement>) => {
			if (e.key === "ArrowDown") {
				e.preventDefault();
				setFocusedIndex((prev) => Math.min(prev + 1, navigableItems.length - 1));
			} else if (e.key === "ArrowUp") {
				e.preventDefault();
				setFocusedIndex((prev) => Math.max(prev - 1, 0));
			} else if (e.key === "Enter") {
				e.preventDefault();
				const target = navigableItems[focusedIndex];
				if (target) {
					const url = target.url;
					if (target.isExternal || e.metaKey || e.ctrlKey) {
						window.open(url, "_blank", "noopener,noreferrer");
					} else {
						navigate(url);
					}
					setSearchQuery("");
					setFocusedIndex(-1);
					searchInputRef.current?.blur();
				}
			} else if (e.key === "Escape") {
				setSearchQuery("");
				setFocusedIndex(-1);
				searchInputRef.current?.blur();
			}
		},
		[navigableItems, focusedIndex, navigate],
	);

	// Auto-scroll focused item into view
	useEffect(() => {
		if (focusedIndex < 0) return;
		const url = navigableItems[focusedIndex]?.url;
		if (!url) return;
		const el = document.querySelector(`[data-nav-url="${url}"]`);
		el?.scrollIntoView({ block: "nearest" });
	}, [focusedIndex, navigableItems]);

	const toggleItem = (title: string) => {
		setExpandedItems((prev) => {
			const next = new Set(prev);
			if (next.has(title)) {
				next.delete(title);
			} else {
				next.add(title);
			}
			return next;
		});
	};

	const configExceptions = ["/workspace/config/logging"];

	const isActiveRoute = (url: string) => {
		if (url === "/" && pathname === "/") return true;
		// Avoid double-highlighting with "/workspace/custom-pricing/overrides"
		if (url === "/workspace/custom-pricing") return pathname === url;
		if (url !== "/" && pathname.startsWith(url)) {
			if (url === "/workspace/config" && configExceptions.some((e) => pathname.startsWith(e))) {
				return false;
			}
			return true;
		}
		return false;
	};

	// Always render the light theme version for SSR to avoid hydration mismatch
	const logoSrc = COMPANY_LOGO;
	const iconSrc = COMPANY_LOGO;

	const { isConnected: isWebSocketConnected } = useWebSocket();

	// New release image - based on theme
	const newReleaseImage = mounted && resolvedTheme === "dark" ? "/images/new-release-image-dark.webp" : "/images/new-release-image.webp";

	// Memoize promo cards array to prevent duplicates and unnecessary re-renders
	const promoCards = useMemo(() => {
		const cards = [];
		// Restart required card - non-dismissible, shown first
		if (coreConfig?.restart_required?.required) {
			cards.push({
				id: "restart-required",
				title: "Restart Required",
				description: (
					<div className="text-xs text-amber-700 dark:text-amber-300/80">
						{coreConfig.restart_required.reason || "Configuration changes require a server restart to take effect."}
					</div>
				),
				dismissible: false,
				variant: "warning" as const,
			});
		}
		if (showNewReleaseBanner && latestRelease) {
			cards.push({
				id: "new-release",
				title: `${latestRelease.name} is now available.`,
				description: (
					<div className="flex h-full flex-col gap-2">
						<img src={newReleaseImage} alt="UnifAI" className="h-[95px] rounded-md object-cover" />
						<a
							href={`https://docs.unifai.ai/changelogs/${latestRelease.name}`}
							target="_blank"
							rel="noopener noreferrer"
							className="text-primary mt-auto pb-1 font-medium underline"
						>
							View release notes
						</a>
					</div>
				),
				dismissible: true,
			});
		}
		return cards;
	}, [coreConfig?.restart_required, showNewReleaseBanner, latestRelease, newReleaseImage, isProductionSetupDismissed, mounted]);

	// Reset areCardsEmpty when promoCards changes
	useEffect(() => {
		if (promoCards.length > 0) {
			setAreCardsEmpty(false);
		}
	}, [promoCards]);

	const hasPromoCards = promoCards.length > 0 && !areCardsEmpty;
	// When cards are present: 13rem (header 3rem + bottom section ~10rem)
	// When no cards: 8rem (header 3rem + bottom section without cards ~5rem)
	const sidebarGroupHeight = hasPromoCards ? "h-[calc(100vh-13rem)]" : "h-[calc(100vh-8rem)]";

	const handleCardsEmpty = () => {
		setAreCardsEmpty(true);
	};

	const handlePromoDismiss = useCallback(
		(cardId: string) => {
			if (cardId === "production-setup") {
				const expiryDate = new Date();
				expiryDate.setDate(expiryDate.getDate() + 7);
				setCookie(PRODUCTION_SETUP_DISMISSED_COOKIE, "true", {
					path: "/",
					expires: expiryDate,
				});
			}
		},
		[setCookie],
	);

	const handleLogout = async () => {
		setUserPopoverOpen(false);
		invalidateSessionAuthCache();
		clearAuthStorage();
		const currentTarget = pathname + (search ? (search.startsWith("?") ? search : `?${search}`) : "");
		const gotoParam = currentTarget && currentTarget !== "/" && !currentTarget.startsWith("/login")
			? `&goto=${encodeURIComponent(currentTarget)}`
			: "";
		const loginUrl = `/login?logged_out=1${gotoParam}`;
		try {
			await logout().unwrap();
			window.location.assign(loginUrl);
		} catch {
			// Even if logout fails on server, redirect to login with logged_out flag
			window.location.assign(loginUrl);
		}
	};

	const { state: sidebarState, toggleSidebar } = useSidebar();

	return (
		<Sidebar collapsible="icon" className="overflow-y-clip border-none bg-transparent">
			<SidebarHeader className="mt-1 ml-2 flex justify-between px-0 group-data-[collapsible=icon]:ml-0 group-data-[collapsible=icon]:h-auto">
				{/* Expanded state: horizontal layout */}
				<div className="flex h-10 w-full items-center justify-between px-1.5 group-data-[collapsible=icon]:hidden">
					<Link to="/workspace/logs" className="group flex items-center gap-2 pl-2">
						<img className="h-8 w-auto max-w-[180px] object-contain" src={logoSrc} alt={COMPANY_NAME} width={180} height={32} />
					</Link>
					<button
						onClick={toggleSidebar}
						type="button"
						data-testid="sidebar-collapse-btn"
						className="text-muted-foreground hover:text-foreground hover:bg-sidebar-accent flex h-7 w-7 items-center justify-center rounded-md transition-colors"
						aria-label="Collapse sidebar"
					>
						<PanelLeftClose className="h-4 w-4" />
					</button>
				</div>
				{/* Collapsed state: vertical layout */}
				<div
					className="hidden w-full cursor-pointer flex-col items-center gap-2 py-2 group-data-[collapsible=icon]:flex"
					onClick={toggleSidebar}
				>
					<img className="h-7 w-auto max-w-[28px] object-contain" src={iconSrc} alt={COMPANY_SHORT_NAME} width={28} height={28} />
				</div>
			</SidebarHeader>
			{envLabel && (
				<div className="mx-2 -mt-1 mb-2">
					{/* Expanded: full label text */}
					<div className="flex items-center justify-center rounded-sm bg-amber-400/20 px-2 py-1 group-data-[collapsible=icon]:hidden">
						<span className="font-mono text-[10px] font-semibold tracking-widest text-amber-700 dark:text-amber-400">{envLabel}</span>
					</div>
					{/* Collapsed: dot indicator */}
					<div className="hidden justify-center group-data-[collapsible=icon]:flex">
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="h-1.5 w-1.5 rounded-full bg-amber-500 dark:bg-amber-400" />
							</TooltipTrigger>
							<TooltipContent side="right">{envLabel}</TooltipContent>
						</Tooltip>
					</div>
				</div>
			)}
			<div className="mx-2 pb-1 group-data-[collapsible=icon]:hidden">
				<div className="relative">
					<Search className="text-muted-foreground absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2" />
					<input
						ref={searchInputRef}
						type="search"
						name="unifai-sidebar-nav-search"
						autoComplete="off"
						autoCorrect="off"
						autoCapitalize="off"
						spellCheck={false}
						data-1p-ignore="true"
						data-lpignore="true"
						data-form-type="other"
						aria-label="Search sidebar navigation"
						placeholder="Search..."
						value={searchQuery}
						onChange={(e) => {
							// Browser password managers ignore autoComplete="off" and may inject a saved
							// username here when a page has a password field; real typing requires focus.
							if (typeof document !== "undefined" && document.activeElement !== e.currentTarget) return;
							setSearchQuery(e.target.value);
							setFocusedIndex(-1);
						}}
						onKeyDown={handleSearchKeyDown}
						className="border-input text-foreground placeholder:text-shadow-muted-foreground focus:ring-ring h-8 w-full rounded-sm border bg-transparent pr-14 pl-8 text-sm outline-none focus:bg-transparent"
					/>
					<kbd className="text-muted-foreground pointer-events-none absolute top-1/2 right-2 flex -translate-y-1/2 gap-0.5 text-[10px]">
						<span className="border-border bg-muted rounded-sm px-1 font-mono shadow-sm">⌘</span>
						<span className="border-border bg-muted rounded-sm px-1 font-mono shadow-sm">K</span>
					</kbd>
				</div>
			</div>
			<SidebarContent className="overflow-hidden pb-4">
				<SidebarGroup className={`custom-scrollbar ${sidebarGroupHeight} overflow-scroll`}>
					<SidebarGroupContent>
						<SidebarMenu className="space-y-0.5">
							{filteredItems.map((item) => {
								const isActive = isActiveRoute(item.url);

								const highlightedUrl = focusedIndex >= 0 ? navigableItems[focusedIndex]?.url : undefined;
								return (
									<SidebarItemView
										key={item.title}
										item={item}
										isActive={isActive}
										isExternal={item.isExternal ?? false}
										isWebSocketConnected={isWebSocketConnected}
										isExpanded={expandedItems.has(item.title)}
										onToggle={() => toggleItem(item.title)}
										pathname={pathname}
										search={search}
										isSidebarCollapsed={sidebarState === "collapsed"}
										expandSidebar={() => toggleSidebar()}
										highlightedUrl={highlightedUrl}
									/>
								);
							})}
						</SidebarMenu>
					</SidebarGroupContent>
				</SidebarGroup>
				<div className="flex flex-col gap-4 px-3 group-data-[collapsible=icon]:px-1">
					<div className="mx-1 group-data-[collapsible=icon]:hidden">
						<PromoCardStack cards={promoCards} onCardsEmpty={handleCardsEmpty} onDismiss={handlePromoDismiss} />
					</div>
					<div className="flex flex-row">
						<div className="mx-auto flex flex-row gap-4 group-data-[collapsible=icon]:flex-col group-data-[collapsible=icon]:gap-2">
							{sidebarState !== "collapsed" &&
								externalLinks.map((item, index) => (
									<a
										key={index}
										href={item.url}
										target="_blank"
										rel="noopener noreferrer"
										className="group flex w-full items-center justify-between"
										title={item.title}
									>
										<div className="flex items-center space-x-3">
											<item.icon
												className="hover:text-primary text-muted-foreground h-5 w-5"
												size={22}
												weight="regular"
												strokeWidth={item.strokeWidth}
											/>
										</div>
									</a>
								))}
							<ThemeToggle />
							{IS_ENTERPRISE && userInfo && (userInfo.name || userInfo.email) ? (
								<Popover open={userPopoverOpen} onOpenChange={setUserPopoverOpen}>
									<PopoverTrigger asChild>
										<button
											className="hover:text-primary text-muted-foreground flex cursor-pointer items-center space-x-3 p-0.5"
											type="button"
											aria-label="User menu"
										>
											<User className="hover:text-primary text-muted-foreground h-4 w-4" size={20} strokeWidth={2} />
										</button>
									</PopoverTrigger>
									<PopoverContent side="top" align="start" className="w-56 p-0">
										<div className="flex flex-col">
											<div className="px-4 py-3">
												<p className="text-sm font-medium">{userInfo.name || userInfo.email || "User"}</p>
											</div>
											<Separator />
											<button
												onClick={handleLogout}
												className="hover:bg-accent hover:text-accent-foreground flex w-full items-center gap-2 px-4 py-2.5 text-left text-sm transition-colors"
												type="button"
											>
												<LogOut className="h-4 w-4" strokeWidth={2} />
												<span>Logout</span>
											</button>
										</div>
									</PopoverContent>
								</Popover>
							) : isAuthEnabled ? (
								<div className="flex items-center gap-2">
									{authStatus?.username && (
										<span className="text-muted-foreground max-w-[80px] truncate text-xs font-medium select-none group-data-[collapsible=icon]:hidden">
											{authStatus.username}
										</span>
									)}
									<button
										className="hover:text-primary text-muted-foreground flex cursor-pointer items-center space-x-3 p-0.5"
										onClick={handleLogout}
										type="button"
										aria-label="Logout"
									>
										<LogOut className="hover:text-primary text-muted-foreground h-4 w-4" size={20} strokeWidth={2} />
									</button>
								</div>
							) : null}
							<div className="hidden w-full cursor-pointer flex-col items-center group-data-[collapsible=icon]:flex">
								<button
									onClick={toggleSidebar}
									type="button"
									data-testid="sidebar-expand-btn"
									className="text-muted-foreground hover:text-foreground hover:bg-sidebar-accent flex cursor-pointer items-center justify-center rounded-md transition-colors"
									aria-label="Expand sidebar"
								>
									<PanelLeftOpen className="h-4 w-4" />
								</button>
							</div>
						</div>
					</div>
					{visibleVersion ? (
						<div className="mx-auto flex flex-col items-center gap-1 group-data-[collapsible=icon]:hidden">
							<div className="font-mono text-xs">{visibleVersion}</div>
						</div>
					) : null}
				</div>
			</SidebarContent>
			<SidebarRail />
		</Sidebar>
	);
}