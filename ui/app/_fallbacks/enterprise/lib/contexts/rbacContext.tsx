"use client";

import { IS_ENTERPRISE } from "@/lib/constants/config";
import { useGetMyRBACPermissionsQuery } from "@enterprise/lib/store/apis/rbacApi";
import { createContext, useCallback, useContext, useMemo } from "react";

// RBAC Resource Names (must match backend definitions)
export enum RbacResource {
	// Core & legacy resources
	GuardrailsConfig = "GuardrailsConfig",
	GuardrailsProviders = "GuardrailsProviders",
	GuardrailRules = "GuardrailRules",
	UserProvisioning = "UserProvisioning",
	Cluster = "Cluster",
	Settings = "Settings",
	Users = "Users",
	Logs = "Logs",
	Observability = "Observability",
	Dashboard = "Dashboard",
	VirtualKeys = "VirtualKeys",
	ModelProvider = "ModelProvider",
	Plugins = "Plugins",
	MCPGateway = "MCPGateway",
	MCPToolGroups = "MCPToolGroups",
	MCPLogs = "MCPLogs",
	AdaptiveRouter = "AdaptiveRouter",
	AuditLogs = "AuditLogs",
	Customers = "Customers",
	Teams = "Teams",
	RBAC = "RBAC",
	Governance = "Governance",
	RoutingRules = "RoutingRules",
	PromptRepository = "PromptRepository",
	PromptDeploymentStrategy = "PromptDeploymentStrategy",
	SkillsRepository = "SkillsRepository",
	AccessProfiles = "AccessProfiles",
	APIKeys = "APIKeys",
	Inference = "Inference",
	Metrics = "Metrics",
	FeatureFlags = "FeatureFlags",
	CircuitBreaker = "CircuitBreaker",

	// Observability sub-features
	LogsSettings = "LogsSettings",

	// Browser AI sub-features
	BrowserAIOverview = "BrowserAIOverview",
	BrowserAITargets = "BrowserAITargets",
	BrowserAIRules = "BrowserAIRules",
	BrowserAILogs = "BrowserAILogs",
	BrowserAISearchLogs = "BrowserAISearchLogs",
	BrowserAISetup = "BrowserAISetup",
	BrowserAIAgents = "BrowserAIAgents",
	BrowserAIInsights = "BrowserAIInsights",

	// Models sub-features
	ModelCatalog = "ModelCatalog",
	BudgetsLimits = "BudgetsLimits",
	ComplexityRouter = "ComplexityRouter",
	PricingOverrides = "PricingOverrides",
	ModelSettings = "ModelSettings",

	// MCP Gateway sub-features
	MCPCatalog = "MCPCatalog",
	MCPLibrary = "MCPLibrary",
	MCPAuthSessions = "MCPAuthSessions",
	MCPOAuthGrants = "MCPOAuthGrants",
	MCPSettings = "MCPSettings",

	// Governance sub-features
	BusinessUnits = "BusinessUnits",

	// Adaptive Routing sub-features
	AdaptiveRoutingDashboard = "AdaptiveRoutingDashboard",
	AdaptiveRoutingSettings = "AdaptiveRoutingSettings",

	// Settings sub-features
	ClientSettings = "ClientSettings",
	Compatibility = "Compatibility",
	Caching = "Caching",
	Security = "Security",
	PerformanceTuning = "PerformanceTuning",
	Proxy = "Proxy",
}

// RBAC Operation Names (must match backend definitions)
export enum RbacOperation {
	Read = "Read",
	View = "View",
	Create = "Create",
	Update = "Update",
	Delete = "Delete",
	Download = "Download",
}

interface RbacContextType {
	isAllowed: (resource: RbacResource, operation: RbacOperation) => boolean;
	permissions: Record<string, Record<string, boolean>>;
	isLoading: boolean;
	isError: boolean;
	error: unknown;
	refetch: () => void;
}

const RbacContext = createContext<RbacContextType | null>(null);

function hasPermission(
	permissions: Record<string, Record<string, boolean>>,
	resource: RbacResource,
	operation: RbacOperation,
): boolean {
	const ops = permissions[resource];
	if (!ops) {
		return false;
	}
	if (ops[operation]) {
		return true;
	}
	if (operation === RbacOperation.View && ops[RbacOperation.Read]) {
		return true;
	}
	if (operation === RbacOperation.Read && ops[RbacOperation.View]) {
		return true;
	}
	return false;
}

export function RbacProvider({ children }: { children: React.ReactNode }) {
	const { data, isLoading, isError, error, refetch } = useGetMyRBACPermissionsQuery(undefined);
	const permissions = data?.permissions ?? {};
	// Never default to "admin" while loading — that fail-opens every gated control.
	const role = data?.role ?? "";

		const isAllowed = useCallback(
			(resource: RbacResource, operation: RbacOperation) => {
				if (isLoading || isError || !data) {
					return false;
				}
				// Built-in user: Prompt Repository is always full access (chat / edit / deploy own prompts).
				// Other workspace sections stay View-only unless raised in Roles & Permissions.
				if (
					role === "user" &&
					(resource === RbacResource.PromptRepository || resource === RbacResource.PromptDeploymentStrategy)
				) {
					return true;
				}
				// If permissions matrix is available from the backend, enforce it
				if (permissions && Object.keys(permissions).length > 0) {
					return hasPermission(permissions, resource, operation);
				}
				if (role === "admin" || role === "sub_admin") {
					return true;
				}
				return false;
			},
			[permissions, role, isLoading, isError, data],
		);

	const value = useMemo(
		() => ({
			isAllowed,
			permissions,
			isLoading,
			isError,
			error,
			refetch,
		}),
		[isAllowed, permissions, isLoading, isError, error, refetch],
	);

	return <RbacContext.Provider value={value}>{children}</RbacContext.Provider>;
}

export function useRbac(resource: RbacResource, operation: RbacOperation): boolean {
	const context = useContext(RbacContext);
	if (!context) {
		// Outside provider: OSS builds stay open; enterprise stays closed.
		return !IS_ENTERPRISE;
	}
	if (context.isLoading) {
		return false;
	}
	return context.isAllowed(resource, operation);
}

export function useRbacContext() {
	const context = useContext(RbacContext);
	if (!context) {
		return {
			isAllowed: () => !IS_ENTERPRISE,
			permissions: {},
			isLoading: false,
			isError: false,
			error: undefined,
			refetch: () => {},
		};
	}
	return context;
}
