import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { ComboboxSelect } from "@/components/ui/combobox";
import ModelParameters from "@/components/ui/custom/modelParameters";
import { Label } from "@/components/ui/label";
import { useGetAllKeysQuery, useGetModelsQuery, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { getProviderLabel } from "@/lib/constants/logs";
import { Input } from "@/components/ui/input";
import { getErrorMessage, useGetVirtualKeysQuery, useGetProviderGovernanceQuery, useGetVirtualKeyBillingBlocksQuery, useGetTeamsQuery } from "@/lib/store";
import { toast } from "sonner";
import { useListSkillsQuery } from "@/lib/store/apis/skillsApi";
import { useGetMCPClientsQuery } from "@/lib/store/apis/mcpApi";
import { useGetUserVirtualKeysQuery } from "@enterprise/lib/store/apis/virtualKeyUsersApi";
import { useGetSessionUsersQuery } from "@/lib/store/apis/sessionUsersApi";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Eye, PanelRightClose, Wrench } from "lucide-react";
import { ModelProviderName } from "@/lib/types/config";
import { ModelParams } from "@/lib/types/prompts";
import { cn } from "@/lib/utils";
import { PromptDeploymentsAccordionItem } from "@enterprise/components/prompt-deployments/promptDeploymentsAccordionItem";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiKeySelectorView } from "../components/apiKeySelectorView";
import { VariablesTableView } from "../components/variablesTableView";
import { usePromptContext } from "../context";
import { useIsAuthEnabledQuery } from "@/lib/store";
import { isPromptMemberRole } from "../utils/memberRole";
import type { VirtualKey } from "@/lib/types/governance";

/** Providers and models listed on the given VKs (real provider_configs only — no invented catalogs). */
function providersFromVirtualKeys(keys: VirtualKey[]): { providers: string[]; wildcardProviders: boolean; concreteModels: string[]; modelScopes: string[] } {
	const providers = new Set<string>();
	const concreteModels: string[] = [];
	const modelScopes = new Set<string>();
	let wildcardProviders = false;
	for (const key of keys) {
		for (const pc of key.provider_configs ?? []) {
			const prov = (pc.provider || "").trim();
			if (!prov) continue;
			if (prov === "*") {
				wildcardProviders = true;
			} else {
				providers.add(prov);
			}
			const models = pc.allowed_models ?? [];
			const scopedProvider = prov.toLowerCase();
			if (models.includes("*")) {
				modelScopes.add(prov === "*" ? "*" : `${scopedProvider}/*`);
			}
			for (const m of models) {
				if (m && m !== "*") {
					concreteModels.push(m);
					modelScopes.add(`${scopedProvider}/${m}`);
				}
			}
		}
	}
	return { providers: [...providers], wildcardProviders, concreteModels, modelScopes: [...modelScopes] };
}

function modelScopeAllows(scopes: string[], providerName: string, modelName: string): boolean {
	const provider = providerName.trim().toLowerCase();
	const modelValue = modelName.trim().toLowerCase();
	return scopes.some((scope) => {
		const value = scope.trim().toLowerCase();
		if (value === "*" || value === modelValue) return true;
		const slash = value.indexOf("/");
		if (slash < 0) return false;
		const scopeProvider = value.slice(0, slash);
		const scopeModel = value.slice(slash + 1);
		return (scopeProvider === "*" || scopeProvider === provider) && (scopeModel === "*" || scopeModel === modelValue);
	});
}

function splitModelScope(value: string, fallbackProvider = ""): { provider: string; model: string } {
	const slash = value.indexOf("/");
	if (slash < 0) return { provider: fallbackProvider, model: value };
	return { provider: value.slice(0, slash), model: value.slice(slash + 1) };
}

function splitIds(value?: string): string[] {
	return (value ?? "").split(",").map((item) => item.trim()).filter(Boolean);
}

function mcpClientForTool(value: string, clientNames: string[]): string | undefined {
	return [...clientNames].sort((a, b) => b.length - a.length).find((name) => value === `${name}-*` || value.startsWith(`${name}-`));
}

export function SettingsPanel() {
	const {
		provider,
		setProvider,
		model,
		setModel: onModelChange,
		modelParams,
		setModelParams: onModelParamsChange,
		virtualKeyId,
		setVirtualKeyId,
		apiKeyId,
		setApiKeyId,
		skillId,
		setSkillId,
		mcpIncludeTools,
		setMcpIncludeTools,
		allowedProviders,
		setAllowedProviders,
		allowedModels,
		setAllowedModels,
		allowedVirtualKeys,
		setAllowedVirtualKeys,
		allowedApiKeys,
		setAllowedApiKeys,
		allowedSkills,
		setAllowedSkills,
		selectedPrompt,
		variables,
		setVariables,
		customHeaders,
		setCustomHeaders,
		requiredHeaders,
		selectedPromptId,
		toggleSettings,
		canUpdate,
	} = usePromptContext();

	const POLL_MS = 5000;
	const [authPollMs, setAuthPollMs] = useState(POLL_MS);
	const [vkPollMs, setVkPollMs] = useState(POLL_MS);
	const { data: authStatus, error: authPollError, refetch: refetchAuth } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });
	const isMemberOnly = isPromptMemberRole(authStatus?.role);

	const { data: virtualKeysData, isError: vkFailed, error: vkError, refetch: refetchVk } = useGetVirtualKeysQuery(undefined, { pollingInterval: vkPollMs });
	const focusUserId = isMemberOnly
		? (authStatus?.user_id || "")
		: selectedPrompt?.user_ids?.split(",").map((id) => id.trim()).find(Boolean) || selectedPrompt?.owner_user_id || "";
	const { data: focusUserKeys, isLoading: focusUserKeysLoading, refetch: refetchFocusUserKeys } = useGetUserVirtualKeysQuery(focusUserId, { skip: !focusUserId });
	const { data: teamsData, refetch: refetchTeams } = useGetTeamsQuery(undefined, { skip: isMemberOnly });
	const { data: sessionUsers, refetch: refetchSessionUsers } = useGetSessionUsersQuery(undefined, { skip: isMemberOnly });
	const { refetch: refetchProvGov } = useGetProviderGovernanceQuery({ fromMemory: true }, { skip: false });
	const { data: billingBlocks, refetch: refetchBilling } = useGetVirtualKeyBillingBlocksQuery(undefined, { skip: !isMemberOnly });

	const [isRefreshingBudgets, setIsRefreshingBudgets] = useState(false);
	const handleRefreshBudgets = useCallback(async () => {
		setIsRefreshingBudgets(true);
		try {
			await Promise.allSettled([
				refetchAuth?.(),
				refetchVk?.(),
				refetchFocusUserKeys?.(),
				refetchSessionUsers?.(),
				refetchTeams?.(),
				refetchProvGov?.(),
				refetchBilling?.(),
			]);
			toast.success("Budgets updated live");
		} catch (e) {
			// ignore
		} finally {
			setTimeout(() => setIsRefreshingBudgets(false), 500);
		}
	}, [refetchAuth, refetchVk, refetchFocusUserKeys, refetchSessionUsers, refetchTeams, refetchProvGov, refetchBilling]);
	const targetUser = useMemo(() => {
		if (isMemberOnly || !sessionUsers) return undefined;
		if (focusUserId) {
			const found = sessionUsers.find((u) => u.id === focusUserId || u.email === focusUserId || u.username === focusUserId);
			if (found) return found;
		}
		if (selectedPrompt?.name) {
			const foundByName = sessionUsers.find((u) => u.email === selectedPrompt.name || u.username === selectedPrompt.name);
			if (foundByName) return foundByName;
		}
		return undefined;
	}, [isMemberOnly, focusUserId, sessionUsers, selectedPrompt?.name]);

	const targetUserBudget = isMemberOnly ? undefined : (targetUser?.budget !== undefined ? Number(targetUser.budget) : undefined);
	const targetUserUsage = isMemberOnly ? undefined : (targetUser?.budget_current_usage !== undefined ? Number(targetUser.budget_current_usage) : undefined);
	const targetUserLabel = isMemberOnly ? undefined : (targetUser ? `1. User Budget (${targetUser.username || targetUser.email || "User"})` : undefined);
	const targetAssignedVirtualKeys = useMemo(() => {
		if (isMemberOnly || !selectedPrompt || !virtualKeysData?.virtual_keys) return [];
		const now = Date.now();
		const audienceUsers = new Set(splitIds(selectedPrompt.user_ids));
		const owner = selectedPrompt.owner_user_id ? sessionUsers?.find((user) => user.id === selectedPrompt.owner_user_id) : undefined;
		if (owner && !["admin", "super_admin"].includes(owner.role.trim().toLowerCase())) audienceUsers.add(owner.id);
		const audienceTeams = new Set(splitIds(selectedPrompt.team_ids));
		const audienceCustomers = new Set(splitIds(selectedPrompt.customer_ids));
		const audienceBusinessUnits = new Set<string>();
		for (const team of teamsData?.teams ?? []) {
			if (audienceTeams.has(team.id) && team.customer_id) audienceCustomers.add(team.customer_id);
		}
		for (const team of teamsData?.teams ?? []) {
			if (audienceTeams.has(team.id) || (team.customer_id && audienceCustomers.has(team.customer_id))) {
				if (team.business_unit_id) audienceBusinessUnits.add(team.business_unit_id);
			}
		}
		const matchingKeys = virtualKeysData.virtual_keys.filter((vk) => {
			const userIds = new Set([...(vk.user_ids ?? []), ...(vk.users ?? []).map((user) => user.user_id)]);
			const teamIds = new Set([vk.team_id, ...(vk.team_ids ?? []), ...(vk.teams ?? []).map((team) => team.id)].filter((id): id is string => Boolean(id)));
			const customerIds = new Set([vk.customer_id, ...(vk.customer_ids ?? []), ...(vk.customers ?? []).map((customer) => customer.id)].filter((id): id is string => Boolean(id)));
			const businessUnitIds = new Set([...(vk.business_unit_ids ?? []), ...(vk.business_units ?? []).map((unit) => unit.id)]);
			return [...audienceUsers].some((id) => userIds.has(id)) ||
				[...audienceTeams].some((id) => teamIds.has(id)) ||
				[...audienceCustomers].some((id) => customerIds.has(id)) ||
				[...audienceBusinessUnits].some((id) => businessUnitIds.has(id));
		});
		const inheritedUserKeys = audienceUsers.has(focusUserId) ? (focusUserKeys?.virtual_keys ?? []) : [];
		const uniqueKeys = new Map<string, VirtualKey>();
		const inheritedFullKeys = inheritedUserKeys
			.map((inherited) => virtualKeysData.virtual_keys.find((vk) => vk.id === inherited.id))
			.filter((vk): vk is VirtualKey => Boolean(vk));
		for (const vk of [...matchingKeys, ...inheritedFullKeys]) {
			if (vk.is_active === false || (vk.expires_at && new Date(vk.expires_at).getTime() <= now)) continue;
			uniqueKeys.set(vk.id, vk);
		}
		return [...uniqueKeys.values()].sort((a, b) => {
			const createdA = a.created_at ? new Date(a.created_at).getTime() : 0;
			const createdB = b.created_at ? new Date(b.created_at).getTime() : 0;
			return createdA - createdB || a.id.localeCompare(b.id);
		});
	}, [isMemberOnly, selectedPrompt, virtualKeysData, teamsData, focusUserKeys, focusUserId, sessionUsers]);

	useEffect(() => {
		setAuthPollMs(authPollError ? 0 : POLL_MS);
	}, [authPollError]);
	useEffect(() => {
		setVkPollMs(vkFailed ? 0 : POLL_MS);
	}, [vkFailed]);

	const { data: providers, isLoading: isLoadingProviders, isError: providersFailed, error: providersError } = useGetProvidersQuery();
	const { data: allKeys, isSuccess: hasLoadedAllKeys, isError: keysFailed, error: keysError } = useGetAllKeysQuery();
	const { data: skillsData, isError: skillsFailed, error: skillsError } = useListSkillsQuery({ limit: 100, offset: 0 });
	const { data: mcpClientsData, isError: mcpFailed, error: mcpError } = useGetMCPClientsQuery();
	const settingsQueryFailed = providersFailed || keysFailed || skillsFailed || mcpFailed || vkFailed;

	const onProviderChange = useCallback(
		(p: string) => {
			setProvider(p);
			if (!isMemberOnly) {
				setAllowedProviders((prev) => (prev?.includes("*") || (p && prev && prev.length > 1 && prev.some((item) => item.toLowerCase() === p.toLowerCase())) ? prev : p ? [p] : []));
				const hasPromptVk = Boolean((allowedVirtualKeys?.length ?? 0) > 0 || (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__"));
				if (!hasPromptVk) setAllowedModels(null);
			}
			setApiKeyId("__auto__");
			if (!isMemberOnly) {
				setAllowedApiKeys([]);
			}
			onModelChange("");
			onModelParamsChange((prev) => ({
				stream: true,
				temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
				max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
			}));
		},
		[setProvider, setApiKeyId, isMemberOnly, setAllowedApiKeys, setAllowedProviders, allowedVirtualKeys, virtualKeyId, onModelChange, setAllowedModels, onModelParamsChange],
	);

	const onVirtualKeyChange = useCallback(
		(id: string) => {
			setVirtualKeyId(id);
			if (!id || id === "__auto__" || id === "__none__") {
				setAllowedVirtualKeys([]);
			setMcpIncludeTools(null);
				return;
			}
			const targetVk = (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === id || vk.id === id);
			if (!targetVk) return;
			if (!isMemberOnly) setAllowedVirtualKeys([targetVk.value]);
		setMcpIncludeTools(null);
			setApiKeyId("__auto__");
			if (!isMemberOnly) {
				setAllowedApiKeys([]);
			}
			// Automatically select the connected provider and model for this VK
			const { providers: allowed, wildcardProviders, concreteModels, modelScopes } = providersFromVirtualKeys([targetVk]);
			const targetProviders = wildcardProviders ? ["*"] : allowed;
			const orderedProviders = [...new Set(targetProviders)].sort((a, b) => a.localeCompare(b));
			if (!isMemberOnly) setAllowedProviders(orderedProviders);
			if (orderedProviders.length > 0 && orderedProviders[0] !== "*") {
				onProviderChange(orderedProviders[0]);
			} else if (!wildcardProviders && provider) {
				onProviderChange("");
			}
			const selectedProvider = orderedProviders.includes(provider) ? provider : orderedProviders.find((item) => item !== "*") || provider;
			const matchingScope = modelScopes.find((scope) => {
				const pair = splitModelScope(scope, selectedProvider);
				return pair.model && pair.model !== "*" && (pair.provider === "*" || !selectedProvider || pair.provider.toLowerCase() === selectedProvider.toLowerCase());
			});
			if (matchingScope) onModelChange(splitModelScope(matchingScope, selectedProvider).model);
			else if (concreteModels.length > 0) onModelChange(concreteModels[0]);
			if (!isMemberOnly) setAllowedModels(modelScopes);
		},
		[setVirtualKeyId, virtualKeysData, setAllowedVirtualKeys, setApiKeyId, isMemberOnly, setAllowedApiKeys, onModelChange, provider, onProviderChange, setAllowedModels, setAllowedProviders],
	);

	const onApiKeyIdChange = useCallback(
		(id: string) => {
			setApiKeyId(id);
		},
		[setApiKeyId],
	);
	const skillOptions = useMemo(() => {
		const opts: { label: string; value: string }[] = (skillsData?.skills ?? []).map((s) => ({
			label: `skill --- ${s.name}`,
			value: s.id,
		}));
		if (skillId && !opts.some((o) => o.value === skillId)) {
			opts.unshift({ label: `skill --- ${skillId.slice(0, 8)}...`, value: skillId });
		}
		if (isMemberOnly) {
			const committedSkills = new Set<string>();
			if (allowedSkills !== null) {
				for (const s of allowedSkills) {
					if (s) committedSkills.add(s);
				}
			} else if (skillId) {
				committedSkills.add(skillId);
			}
			if (committedSkills.size === 0) {
				return [];
			}
			return opts.filter((o) => committedSkills.has(o.value));
		}
		return opts;
	}, [skillsData, skillId, isMemberOnly, allowedSkills]);

	const isInitialLoading = isLoadingProviders || !hasLoadedAllKeys;

	const configuredProviders = useMemo(() => {
		if (!hasLoadedAllKeys) {
			return [];
		}
		// A provider is only configured/connected if it has at least one valid API key in allKeys
		const keyedProviders = new Set(
			(allKeys ?? [])
				.map((k) => (k.provider || "").trim().toLowerCase())
				.filter(Boolean)
		);
		// Or if an active Virtual Key specifically configures this provider
		const activeVirtualKeys = virtualKeysData?.virtual_keys?.filter((vk) => vk.is_active) ?? [];
		const vkConfiguredProviders = new Set<string>();
		for (const vk of activeVirtualKeys) {
			for (const pc of vk.provider_configs ?? []) {
				const prov = (pc.provider || "").trim().toLowerCase();
				if (prov && prov !== "*") {
					vkConfiguredProviders.add(prov);
				}
			}
		}

		const isConnected = (name: string) => {
			const lower = (name || "").toLowerCase();
			return keyedProviders.has(lower) || vkConfiguredProviders.has(lower);
		};

		const list = (providers ?? []).filter((p) => {
			if (p.provider_status === "deleted") return false;
			return isConnected(p.name);
		});

		// Also ensure any provider present in allKeys is included even if omitted from providers array
		const existingNames = new Set(list.map((p) => (p.name || "").toLowerCase()));
		for (const pName of keyedProviders) {
			if (!existingNames.has(pName)) {
				list.push({
					name: pName as ModelProviderName,
					provider_status: "active" as any,
				});
				existingNames.add(pName);
			}
		}

		return list.sort((a, b) => a.name.localeCompare(b.name));
	}, [providers, virtualKeysData, allKeys, hasLoadedAllKeys]);

	// Clean up any stale or unconfigured providers from allowedProviders (e.g. from previously selecting all 93)
	useEffect(() => {
		if (isMemberOnly || !hasLoadedAllKeys || configuredProviders.length === 0) return;
		if (!allowedProviders || allowedProviders.length === 0) return;
		const validNames = new Set(configuredProviders.map((p) => p.name.toLowerCase()));
		const validList = allowedProviders.filter((p) => p !== "*" && validNames.has(p.toLowerCase()));
		if (validList.length !== allowedProviders.length) {
			const next = validList.length > 0 ? validList : [configuredProviders[0].name];
			setAllowedProviders(next);
			if (!next.includes(provider)) {
				onProviderChange(next[0]);
			}
		}
	}, [isMemberOnly, hasLoadedAllKeys, configuredProviders, allowedProviders, setAllowedProviders, provider, onProviderChange]);

	// Collect all VKs that are currently selected (from allowedVirtualKeys or virtualKeyId)
	const selectedVirtualKeys = useMemo(() => {
		const vks = virtualKeysData?.virtual_keys ?? [];
		if (isMemberOnly && virtualKeyId && virtualKeyId !== "__auto__") {
			return vks.filter((vk) => vk.value === virtualKeyId || vk.id === virtualKeyId);
		}
		const activeIds = new Set(
			allowedVirtualKeys && allowedVirtualKeys.length > 0
				? allowedVirtualKeys
				: (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__" ? [virtualKeyId] : [])
		);
		return vks.filter((vk) => activeIds.has(vk.value) || activeIds.has(vk.id));
	}, [virtualKeysData, allowedVirtualKeys, virtualKeyId, isMemberOnly]);

	// Only the user-selected VK — never fall back to the first catalog key.
	const selectedVirtualKey = useMemo(() => {
		if (selectedVirtualKeys.length === 1) return selectedVirtualKeys[0];
		if (!virtualKeyId || virtualKeyId === "__auto__" || virtualKeyId === "__none__") return undefined;
		const all = virtualKeysData?.virtual_keys ?? [];
		return all.find((vk) => vk.value === virtualKeyId || vk.id === virtualKeyId);
	}, [selectedVirtualKeys, virtualKeysData, virtualKeyId]);

	// Active VK list for cascading restrictions
	const activeVkList = useMemo(() => {
		if (selectedVirtualKeys.length > 0) return selectedVirtualKeys;
		if (selectedVirtualKey) return [selectedVirtualKey];
		return [];
	}, [selectedVirtualKeys, selectedVirtualKey]);

	const isVkSelected = isMemberOnly
		? Boolean(virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__")
		: Boolean(
				(allowedVirtualKeys && allowedVirtualKeys.length > 0) ||
				(virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__") ||
				activeVkList.length > 0
			);

	const activeMCPClients = useMemo(() => {
		const clients = (mcpClientsData?.clients ?? []).filter((client) => !client.config?.disabled);
		if (!isVkSelected || activeVkList.length === 0) return [];
		const vkIds = new Set(activeVkList.map((vk) => vk.id));

		const selectedVKMcpNames = new Set<string>();
		for (const vk of activeVkList) {
			for (const mc of vk.mcp_configs ?? []) {
				if (mc.mcp_client?.name) selectedVKMcpNames.add(mc.mcp_client.name);
			}
		}

		const vkEligibleClients = clients.filter((client) => {
			if (!client.config?.client_id) return false;
			if (client.config.allow_on_all_virtual_keys) return true;
			if (client.vk_configs?.some((vc) => vkIds.has(vc.virtual_key_id))) return true;
			if (client.config?.name && selectedVKMcpNames.has(client.config.name)) return true;
			return false;
		});
		if (!isMemberOnly || mcpIncludeTools === null) return vkEligibleClients;
		if (mcpIncludeTools.length === 0) return [];
		if (mcpIncludeTools.includes("*")) return vkEligibleClients;
		const names = vkEligibleClients.map((client) => client.config?.name || "").filter(Boolean);
		const selectedNames = new Set(mcpIncludeTools.map((tool) => mcpClientForTool(tool, names)).filter((name): name is string => Boolean(name)));
		return vkEligibleClients.filter((client) => client.config?.name && selectedNames.has(client.config.name));
	}, [mcpClientsData, activeVkList, isMemberOnly, mcpIncludeTools, isVkSelected]);

	const totalMCPTools = useMemo(
		() => activeMCPClients.reduce((acc, c) => acc + (c.tools?.length ?? 0), 0),
		[activeMCPClients],
	);

	// Virtual keys: Admin sees all active. Members ONLY see virtual keys committed by admin on this prompt!
	// If admin did NOT commit any virtual key, return [] (no VK auto-assigned).
	const providerVirtualKeys = useMemo(() => {
		const now = Date.now();
		const allActive = (virtualKeysData?.virtual_keys ?? []).filter(
			(vk) => vk.is_active !== false && (!vk.expires_at || new Date(vk.expires_at).getTime() > now),
		).sort((a, b) => {
			const createdA = a.created_at ? new Date(a.created_at).getTime() : 0;
			const createdB = b.created_at ? new Date(b.created_at).getTime() : 0;
			return createdA - createdB || a.id.localeCompare(b.id);
		});
		if (!isMemberOnly) {
			return allActive;
		}
		const committedVkIds = new Set<string>();
		if (Array.isArray(allowedVirtualKeys)) {
			for (const id of allowedVirtualKeys) {
				if (id) committedVkIds.add(id);
			}
		}
		const assignedIds = new Set((focusUserKeys?.virtual_keys ?? []).map((k) => k.id));
		if (assignedIds.size === 0) return [];
		if (allowedVirtualKeys === null) {
			const legacyVK = selectedPrompt?.latest_version?.model_params?.virtual_key_id;
			if (typeof legacyVK === "string" && legacyVK !== "__auto__" && legacyVK !== "__none__" && legacyVK.trim()) {
				committedVkIds.add(legacyVK.trim());
			}
			if (committedVkIds.size === 0) {
				// A legacy prompt without a saved VK scope inherits only the keys
				// actually assigned to this signed-in user.
				return allActive.filter((vk) => assignedIds.has(vk.id));
			}
		}
		const committedKeys = allActive.filter((vk) => (committedVkIds.has(vk.id) || committedVkIds.has(vk.value)) && assignedIds.has(vk.id));
		return committedKeys;
	}, [virtualKeysData, isMemberOnly, allowedVirtualKeys, selectedPrompt, focusUserKeys]);

	// Provider options:
	// If a Virtual Key is active, show ONLY providers configured in that VK.
	// Otherwise (Direct API key mode), show all providers that are enabled and have a connected API key.
	const providerOptions = useMemo(() => {
		if (isMemberOnly && allowedProviders !== null && allowedProviders.length === 0) return [];
		const list: { label: string; value: string }[] = configuredProviders.map((p) => ({
			label: getProviderLabel(p.name),
			value: p.name as ModelProviderName,
		}));

		let allowed: string[] = [];
		if (isMemberOnly) {
			const promptProviders = allowedProviders;
			if (activeVkList.length > 0) {
				const { providers: keyProviders, wildcardProviders } = providersFromVirtualKeys(activeVkList);
				const availableProviders = wildcardProviders ? list.map((item) => item.value) : keyProviders;
				allowed = promptProviders === null || promptProviders.includes("*")
					? availableProviders
					: availableProviders.filter((candidate) => promptProviders.some((value) => value.toLowerCase() === candidate.toLowerCase()));
			} else {
				const directKeyIds = allowedApiKeys !== null
					? allowedApiKeys
					: apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__" ? [apiKeyId] : [];
				const availableProviders = [...new Set((allKeys ?? [])
					.filter((key) => directKeyIds.includes(key.key_id))
					.map((key) => String(key.provider || "").trim())
					.filter(Boolean))];
				allowed = promptProviders === null || promptProviders.includes("*")
					? availableProviders
					: availableProviders.filter((candidate) => promptProviders.some((value) => value.toLowerCase() === candidate.toLowerCase()));
			}
			if (allowed.length === 0) return [];
		} else if (activeVkList.length > 0) {
			const { providers: vkProviders, wildcardProviders } = providersFromVirtualKeys(activeVkList);
			if (wildcardProviders) {
				return list;
			}
			if (vkProviders.length > 0) {
				allowed = vkProviders;
			}
		} else if (isMemberOnly && providerVirtualKeys.length > 0) {
			const { providers: vkProviders, wildcardProviders } = providersFromVirtualKeys(providerVirtualKeys);
			if (wildcardProviders) {
				return list;
			}
			if (vkProviders.length > 0) {
				allowed = vkProviders;
			}
		} else if (isMemberOnly && provider) {
			allowed = [provider];
		}

		if (allowed.length === 0 || allowed.includes("*")) {
			return list;
		}
		const allowedSet = new Set(allowed.map((p) => p.toLowerCase()));
		return list.filter((p) => allowedSet.has(p.value.toLowerCase()));
	}, [configuredProviders, isMemberOnly, allowedProviders, allowedApiKeys, apiKeyId, allKeys, activeVkList, providerVirtualKeys, provider]);

	// Auto-select single/first provider when options change or when current provider is invalid
	useEffect(() => {
		if (providerOptions.length > 0) {
			if (isMemberOnly || isVkSelected) {
				if (!provider || !providerOptions.some((p) => p.value.toLowerCase() === (provider || "").toLowerCase())) {
					onProviderChange(providerOptions[0].value);
				}
			}
		}
	}, [isMemberOnly, isVkSelected, providerOptions, provider, onProviderChange]);

	// Auto-select connected provider whenever a Virtual Key is active (User or Admin login)
	useEffect(() => {
		if (!isVkSelected || activeVkList.length === 0) {
			return;
		}
		const { providers: allowed, wildcardProviders } = providersFromVirtualKeys(activeVkList);
		if (!wildcardProviders && allowed.length > 0) {
			if (!provider || !allowed.some((p) => p.toLowerCase() === (provider || "").toLowerCase())) {
				onProviderChange(allowed[0]);
			}
		}
	}, [isVkSelected, activeVkList, provider, onProviderChange]);

	// When an admin opens an unconfigured prompt for a scoped audience, seed it
	// from every active VK inherited by that user/team/customer. Saved empty arrays
	// are intentional deselections and are therefore left empty.
	useEffect(() => {
		if (isMemberOnly || !selectedPrompt || !virtualKeysData || !hasLoadedAllKeys || allowedVirtualKeys !== null) return;
		if (focusUserId && focusUserKeysLoading) return;
		const savedParams = selectedPrompt.latest_version?.model_params;
		const hasSavedKeyScope = Array.isArray(savedParams?.prompt_allowed_virtual_keys) ||
			Array.isArray(savedParams?.prompt_allowed_api_keys) ||
			(Boolean(savedParams?.virtual_key_id) && savedParams?.virtual_key_id !== "__auto__" && savedParams?.virtual_key_id !== "__none__") ||
			(Boolean(savedParams?.api_key_id) && savedParams?.api_key_id !== "__auto__" && savedParams?.api_key_id !== "__none__");
		if (hasSavedKeyScope || targetAssignedVirtualKeys.length === 0) return;

		const keys = targetAssignedVirtualKeys;
		const keyValues = keys.map((vk) => vk.value).filter((value) => typeof value === "string" && value.startsWith("sk-uf-"));
		if (keyValues.length === 0) return;
		setAllowedVirtualKeys(keyValues);
		setVirtualKeyId(keyValues[0]);
		setApiKeyId("__auto__");
		setAllowedApiKeys([]);

		const { providers: keyProviders, wildcardProviders, concreteModels, modelScopes } = providersFromVirtualKeys(keys);
		const allowed = wildcardProviders
			? configuredProviders.map((item) => item.name)
			: keyProviders;
		const orderedProviders = [...new Set(allowed)].sort((a, b) => a.localeCompare(b));
		setAllowedProviders(orderedProviders);
		const selectedProvider = orderedProviders.includes(provider) ? provider : orderedProviders[0] || "";
		setProvider(selectedProvider);
		const selectedModels = modelScopes;
		setAllowedModels(selectedModels);
		const savedModel = selectedPrompt.latest_version?.model || "";
		const firstConcreteScope = selectedModels.find((scope) => {
			if (!scope || scope === "*" || scope.endsWith("/*")) return false;
			const slash = scope.indexOf("/");
			if (slash < 0) return true;
			const scopeProvider = scope.slice(0, slash);
			return scopeProvider === "*" || !selectedProvider || scopeProvider.toLowerCase() === selectedProvider.toLowerCase();
		});
		const fallbackModel = firstConcreteScope?.includes("/") ? firstConcreteScope.slice(firstConcreteScope.indexOf("/") + 1) : firstConcreteScope;
		onModelChange(savedModel && (selectedModels.includes("*") || modelScopeAllows(selectedModels, selectedProvider, savedModel))
			? savedModel
			: (fallbackModel || savedModel));
		setMcpIncludeTools(null);
	}, [
		isMemberOnly,
		selectedPrompt,
		virtualKeysData,
		hasLoadedAllKeys,
		allowedVirtualKeys,
		focusUserId,
		focusUserKeysLoading,
		targetAssignedVirtualKeys,
		configuredProviders,
		provider,
		setAllowedVirtualKeys,
		setApiKeyId,
		setAllowedApiKeys,
		setAllowedProviders,
		setProvider,
		setAllowedModels,
		onModelChange,
		setMcpIncludeTools,
	]);

	const providerKeys = useMemo(() => {
		if (isMemberOnly && allowedProviders !== null && allowedProviders.length === 0) return [];
		const activeProviders = (allowedProviders && allowedProviders.length > 0)
			? allowedProviders.map((p) => p.toLowerCase())
			: provider
				? [provider.toLowerCase()]
				: [];

		if (activeProviders.length === 0) {
			return [];
		}

		let keys = allKeys ?? [];
		if (!activeProviders.includes("*")) {
			const provSet = new Set(activeProviders);
			keys = keys.filter((k) => provSet.has(String(k.provider).toLowerCase()));
		}

		if (isMemberOnly) {
			const committedKeys = allowedApiKeys !== null
				? allowedApiKeys
				: (apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__")
					? [apiKeyId]
					: [];
			if (committedKeys.length === 0) {
				return [];
			}
			const allowedSet = new Set(committedKeys);
			return keys.filter((k) => allowedSet.has(k.key_id));
		}
		return keys;
	}, [allKeys, provider, allowedProviders, allowedApiKeys, apiKeyId, isMemberOnly]);

	// Auto-sync apiKeyId when Direct API Key mode is active (no VK selected)
	useEffect(() => {
		if (isVkSelected) {
			// When VK is active, keep direct API key OFF
			if (apiKeyId && apiKeyId !== "__auto__") {
				onApiKeyIdChange("__auto__");
			}
			return;
		}
		// Direct API Key mode: auto-select key for the chosen provider
		if (providerKeys.length > 0) {
			const isCurrentKeyValid = apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__" && providerKeys.some((k) => k.key_id === apiKeyId);
			if (!isCurrentKeyValid) {
				onApiKeyIdChange(providerKeys[0].key_id);
				if (!isMemberOnly) {
					setAllowedApiKeys([providerKeys[0].key_id]);
				}
			}
		} else if (apiKeyId && apiKeyId !== "__auto__") {
			onApiKeyIdChange("__auto__");
			if (!isMemberOnly) {
				setAllowedApiKeys([]);
			}
		}
	}, [isVkSelected, providerKeys, apiKeyId, onApiKeyIdChange, isMemberOnly, setAllowedApiKeys]);

	// Member: if no virtual key committed, ensure virtualKeyId stays __auto__ (not auto-assigned).
	// If 1 key committed, select it.
	useEffect(() => {
		if (isMemberOnly) {
			if (providerVirtualKeys.length === 0) {
				if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__") {
					onVirtualKeyChange("__auto__");
				}
			} else if (
				!(apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__") &&
				virtualKeyId !== "__none__" &&
				(!virtualKeyId || virtualKeyId === "__auto__" ||
					!providerVirtualKeys.some((vk) => (vk.value === virtualKeyId || vk.id === virtualKeyId) && !billingBlocks?.blocks?.[vk.id]))
			) {
				// Prefer an unblocked key; if every assigned key is at its limit, keep
				// the deterministic gateway default so the UI shows the exhausted meter.
				const defaultKey = providerVirtualKeys.find((vk) => !billingBlocks?.blocks?.[vk.id]) ?? providerVirtualKeys[0];
				onVirtualKeyChange(defaultKey.value);
			}
		}
	}, [isMemberOnly, providerVirtualKeys, virtualKeyId, apiKeyId, billingBlocks, onVirtualKeyChange]);

	// Separate keys/vks to pass to model fetch for filtering.
	const filterKeys = useMemo(() => {
		if (isMemberOnly) return undefined;
		const selected = (allowedApiKeys && allowedApiKeys.length > 0 ? allowedApiKeys : apiKeyId && apiKeyId !== "__auto__" ? [apiKeyId] : [])
			.filter((id) => providerKeys.some((k) => k.key_id === id));
		return selected.length > 0 ? selected : undefined;
	}, [apiKeyId, allowedApiKeys, providerKeys, isMemberOnly]);

	const filterVks = useMemo(() => {
		if (selectedVirtualKeys.length > 0) return selectedVirtualKeys.map((vk) => vk.id);
		if (selectedVirtualKey) return [selectedVirtualKey.id];
		return undefined;
	}, [selectedVirtualKeys, selectedVirtualKey]);

	const hasKeyFilter = Boolean((filterKeys && filterKeys.length > 0) || (filterVks && filterVks.length > 0));

	const effectiveProviderQuery = useMemo(() => {
		const activeProviders = (allowedProviders && allowedProviders.length > 0)
			? allowedProviders
			: (provider ? [provider] : []);
		if (activeProviders.length === 0) return undefined;
		if (activeProviders.includes("*")) {
			return configuredProviders.map((p) => p.name).join(",");
		}
		return activeProviders.join(",");
	}, [allowedProviders, provider, configuredProviders]);

	const shouldSkipModels = useMemo(() => {
		const hasProv = (allowedProviders && allowedProviders.length > 0) || Boolean(provider);
		return !hasProv;
	}, [allowedProviders, provider]);

	// Models come only from /models via provider + real API key and/or VK — never hardcoded defaults.
	const { data: modelsData } = useGetModelsQuery(
		{
			provider: effectiveProviderQuery,
			keys: filterKeys && filterKeys.length > 0 ? filterKeys : undefined,
			vks: filterVks && filterVks.length > 0 ? filterVks : undefined,
			limit: 5000,
			unfiltered: !hasKeyFilter,
		},
		{ skip: shouldSkipModels },
	);

	const hasProviderSelected = useMemo(() => {
		return (allowedProviders && allowedProviders.length > 0) || Boolean(provider);
	}, [allowedProviders, provider]);

	const modelOptions = useMemo(() => {
		if (isMemberOnly && allowedProviders !== null && allowedProviders.length === 0) return [];
		if (!hasProviderSelected) return [];
		const activeProviders = (allowedProviders && allowedProviders.length > 0 ? allowedProviders : provider ? [provider] : []);
		const selectedProviderSet = new Set(
			activeProviders.map((p) => p.toLowerCase())
		);
		const isWildcard = selectedProviderSet.has("*");

		let opts = (modelsData?.models ?? [])
			.filter((item) => {
				if (!isWildcard && selectedProviderSet.size > 0 && item.provider) {
					if (!selectedProviderSet.has(item.provider.toLowerCase())) {
						return false;
					}
				}
				return true;
			})
			.map((item) => {
				const prov = item.provider ? String(item.provider).toLowerCase() : provider.toLowerCase();
				const label = prov ? `${prov} --- ${item.name}` : item.name;
				return { label, value: prov ? `${prov}/${item.name}` : item.name };
			});

		// Provider-qualified option values keep same-named models from separate providers distinct.
		const seenLabels = new Set<string>();
		const uniqueOpts: { label: string; value: string }[] = [];
		for (const opt of opts) {
			if (!seenLabels.has(opt.label)) {
				seenLabels.add(opt.label);
				uniqueOpts.push(opt);
			}
		}
		opts = uniqueOpts;

		// If VK specifies model scopes, keep the provider/model pairs granted by that VK.
		if (activeVkList.length > 0) {
			const { modelScopes } = providersFromVirtualKeys(activeVkList);
			if (modelScopes.length === 0) return [];
			if (!modelScopes.includes("*")) {
				opts = opts.filter((option) => {
					const pair = splitModelScope(option.value, provider);
					return modelScopeAllows(modelScopes, pair.provider, pair.model);
				});
				for (const scope of modelScopes) {
					const pair = splitModelScope(scope, provider);
					if (!pair.model || pair.model === "*" || pair.provider === "*") continue;
					if (activeProviders.length > 0 && !isWildcard && !selectedProviderSet.has(pair.provider.toLowerCase())) continue;
					const value = pair.provider ? `${pair.provider.toLowerCase()}/${pair.model}` : pair.model;
					if (!opts.some((option) => option.value.toLowerCase() === value.toLowerCase())) {
						opts.push({ label: `${pair.provider || provider} --- ${pair.model}`, value });
					}
				}
			}
		}

		if (isMemberOnly || allowedModels !== null) {
			const committedModels = allowedModels !== null ? allowedModels : (model ? [model] : []);
			if (committedModels.length === 0) return [];
			if (!committedModels.includes("*")) {
				opts = opts.filter((option) => {
					const pair = splitModelScope(option.value, provider);
					return modelScopeAllows(committedModels, pair.provider, pair.model);
				});
				for (const scope of committedModels) {
					const pair = splitModelScope(scope, provider);
					if (!pair.model || pair.model === "*") continue;
					if (activeProviders.length > 0 && !isWildcard && !selectedProviderSet.has(pair.provider.toLowerCase())) continue;
					const value = pair.provider ? `${pair.provider.toLowerCase()}/${pair.model}` : pair.model;
					if (!opts.some((option) => option.value.toLowerCase() === value.toLowerCase())) {
						opts.push({ label: `${pair.provider || provider} --- ${pair.model}`, value });
					}
				}
			}
		}
		return opts;
	}, [hasProviderSelected, modelsData, model, provider, isMemberOnly, allowedModels, allowedProviders, activeVkList]);

	const selectedModelValues = useMemo(() => {
		if (allowedModels !== null) {
			if (allowedModels.length === 0) return [];
			if (allowedModels.includes("*")) return modelOptions.map((option) => option.value);
			return modelOptions.filter((option) => {
				const pair = splitModelScope(option.value, provider);
				return modelScopeAllows(allowedModels, pair.provider, pair.model);
			}).map((option) => option.value);
		}
		if (!model) return [];
		return modelOptions.filter((option) => {
			const pair = splitModelScope(option.value, provider);
			return pair.model.toLowerCase() === model.toLowerCase() && pair.provider.toLowerCase() === provider.toLowerCase();
		}).map((option) => option.value);
	}, [allowedModels, modelOptions, provider, model]);

	// Auto-select valid model whenever models change or when selected model is invalid for current provider/VK
	useEffect(() => {
		if (modelOptions.length > 0) {
			const currentScope = provider ? `${provider.toLowerCase()}/${model}` : model;
			const currentModelValid = model && model !== "*" && modelOptions.some((o) => o.value.toLowerCase() === currentScope.toLowerCase());
			if (!currentModelValid) {
				const currentProviderOption = modelOptions.find((option) => splitModelScope(option.value, provider).provider.toLowerCase() === provider.toLowerCase());
				const exactAllowedOption = modelOptions.find((option) => allowedModels?.includes(option.value));
				const targetScope = currentProviderOption?.value || exactAllowedOption?.value || modelOptions[0].value;
				const target = splitModelScope(targetScope, provider);
				if (target.provider && target.provider !== "*") setProvider(target.provider);
				onModelChange(target.model);
				if (!isMemberOnly && allowedModels === null) {
					setAllowedModels([targetScope]);
				}
			}
		}
	}, [modelOptions, model, provider, allowedModels, onModelChange, isMemberOnly, setAllowedModels, setProvider]);

	const mcpToolOptions = useMemo(() => {
		const clientsToUse = activeMCPClients;

		let opts: { label: string; value: string }[] = [];
		for (const client of clientsToUse) {
			const clientName = client.config?.name || "";
			if (!clientName) continue;
			const tools = client.tools ?? [];
			if (tools.length === 0) {
				opts.push({ label: `${clientName} --- all tools`, value: `${clientName}-*` });
				continue;
			}
			for (const tool of tools) {
				if (!tool.name) continue;
				opts.push({ label: `${clientName} --- ${tool.name}`, value: `${clientName}-${tool.name}` });
			}
		}
		if (isMemberOnly) {
			if (mcpIncludeTools === null) {
				return opts;
			}
			if (mcpIncludeTools.length === 0) {
				return [];
			}
			if (mcpIncludeTools.includes("*")) {
				return opts;
			}
			const allowedSet = new Set(mcpIncludeTools);
			opts = opts.filter((o) => allowedSet.has(o.value));
		}
		return opts;
	}, [activeMCPClients, isMemberOnly, mcpIncludeTools]);
	const mcpToolValues = useMemo(() => mcpToolOptions.map((option) => option.value), [mcpToolOptions]);
	const selectedMcpTools = (mcpIncludeTools ?? []).filter((value) => mcpToolValues.includes(value));

	// Default MCP selection to the union of tools reachable through selected VKs.
	useEffect(() => {
		if (!isVkSelected || activeVkList.length === 0 || mcpIncludeTools !== null) return;
		setMcpIncludeTools(mcpToolOptions.map((option) => option.value));
	}, [isVkSelected, activeVkList, mcpToolOptions, mcpIncludeTools, setMcpIncludeTools]);

	const handleModelParamsChange = useCallback(
		(params: Record<string, any>) => {
			onModelParamsChange(params as ModelParams);
		},
		[onModelParamsChange],
	);

	const hasModel = Boolean(model);

	// Key selector shows when either provider keys or virtual keys are available, or always for admins/members.
	const showKeySelector = true;
	// Members always get the budget strip (user budget always; VK meters only when a key is assigned).
	const showMemberBudgetStrip = isMemberOnly;
	const keySelector =
		showKeySelector || showMemberBudgetStrip ? (
			<ApiKeySelectorView
				providerKeys={providerKeys}
				virtualKeys={providerVirtualKeys}
				virtualKeyValue={virtualKeyId}
				onVirtualKeyChange={(v) => {
					if (isMemberOnly) setVirtualKeyId(v ?? "__auto__");
					else onVirtualKeyChange(v ?? "__auto__");
					if (v && v !== "__auto__" && v !== "__none__") {
						onApiKeyIdChange("__auto__");
						if (!isMemberOnly) setAllowedApiKeys([]);
					}
				}}
				virtualKeyValues={
					isMemberOnly
						? virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__"
							? [virtualKeyId]
							: []
						: allowedVirtualKeys && allowedVirtualKeys.length > 0
							? allowedVirtualKeys
							: virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__"
								? [virtualKeyId]
								: []
				}
				onVirtualKeyValuesChange={
					isMemberOnly
						? (next) => {
								if (next.length > 0) {
									setVirtualKeyId(next[next.length - 1]);
									setApiKeyId("__auto__");
								} else {
									setVirtualKeyId("__auto__");
								}
							}
						: (next) => {
								const selectedValues = next.includes("*") ? providerVirtualKeys.map((vk) => vk.value) : next;
								setAllowedVirtualKeys(selectedValues);
								setMcpIncludeTools(null);
								if (selectedValues.length === 0) {
									setVirtualKeyId("__none__");
									return;
								}
								// Auto-clear direct API key so user/admin isn't using both!
								setAllowedApiKeys([]);
								onApiKeyIdChange("__auto__");
								const selectedKeys = providerVirtualKeys.filter((vk) => selectedValues.includes(vk.value) || selectedValues.includes(vk.id));
								const activeKey = selectedKeys.find((vk) => vk.value === virtualKeyId || vk.id === virtualKeyId) || selectedKeys[0];
								if (!activeKey) return;
								setVirtualKeyId(activeKey.value);
								const { providers: keyProviders, wildcardProviders, concreteModels, modelScopes } = providersFromVirtualKeys(selectedKeys);
								const nextProviders = wildcardProviders ? configuredProviders.map((item) => item.name) : keyProviders;
								const uniqueProviders = [...new Set(nextProviders)].sort((a, b) => a.localeCompare(b));
								setAllowedProviders(uniqueProviders);
								const selectedProvider = uniqueProviders.includes(provider) ? provider : uniqueProviders[0] || "";
								setProvider(selectedProvider);
										const uniqueModels = modelScopes;
								setAllowedModels(uniqueModels);
										const nextModelScope = uniqueModels.find((item) => {
											const pair = splitModelScope(item, selectedProvider);
											return pair.model && pair.model !== "*" && (pair.provider === "*" || !selectedProvider || pair.provider.toLowerCase() === selectedProvider.toLowerCase());
										});
										onModelChange(nextModelScope ? splitModelScope(nextModelScope, selectedProvider).model : model || "");
						  }
				}
				apiKeyIdValue={apiKeyId}
				onApiKeyIdChange={(v) => {
					onApiKeyIdChange(v ?? "__auto__");
					if (v && v !== "__auto__" && v !== "__none__") {
						if (isMemberOnly) setVirtualKeyId("__none__");
						else {
							onVirtualKeyChange("__none__");
							setAllowedVirtualKeys([]);
						}
					}
				}}
				apiKeyValues={isMemberOnly ? (apiKeyId && apiKeyId !== "__auto__" && apiKeyId !== "__none__" ? [apiKeyId] : []) : (allowedApiKeys ?? [])}
				onApiKeyValuesChange={
					isMemberOnly
						? (next) => {
								setApiKeyId(next[next.length - 1] ?? "__auto__");
								if (next.length > 0) setVirtualKeyId("__none__");
							}
						: (next) => {
								setAllowedApiKeys(next);
								if (next.length === 0) {
									onApiKeyIdChange("__auto__");
									return;
								}
								// Auto-clear VK so admin isn't selecting both!
								setAllowedVirtualKeys([]);
								onVirtualKeyChange("__none__");
								const activeKeyId = next.includes(apiKeyId) ? apiKeyId : next[0];
								onApiKeyIdChange(activeKeyId);
									const selectedKeys = providerKeys.filter((key) => next.includes(key.key_id));
									const nextProviders = [...new Set(selectedKeys.map((key) => key.provider).filter(Boolean))].sort((a, b) => a.localeCompare(b)) as ModelProviderName[];
									setAllowedProviders(nextProviders);
									const selectedProvider = (nextProviders.includes(provider as ModelProviderName) ? provider : nextProviders[0] || "") as ModelProviderName;
									setProvider(selectedProvider);
									const apiModelScopes = new Set<string>();
									for (const key of selectedKeys) {
										const keyProvider = String(key.provider || "").toLowerCase();
										const models = key.models ?? [];
										if (models.includes("*")) apiModelScopes.add(`${keyProvider}/*`);
										for (const allowedModel of models) {
											if (allowedModel && allowedModel !== "*") apiModelScopes.add(`${keyProvider}/${allowedModel}`);
										}
									}
									const modelScopes = [...apiModelScopes];
									setAllowedModels(modelScopes);
									const nextModelScope = modelScopes.find((scope) => {
										const pair = splitModelScope(scope, selectedProvider);
										return pair.model && pair.model !== "*" && (pair.provider === "*" || !selectedProvider || pair.provider.toLowerCase() === selectedProvider.toLowerCase());
									});
									if (nextModelScope) onModelChange(splitModelScope(nextModelScope, selectedProvider).model);
						  }
				}
				provider={provider}
				model={model}
				providerLabel={getProviderLabel(provider)}
				targetUserBudget={targetUserBudget}
				targetUserUsage={targetUserUsage}
				targetUserLabel={targetUserLabel}
				onRefreshBudgets={handleRefreshBudgets}
				isRefreshingBudgets={isRefreshingBudgets}
				disabled={!canUpdate}
			/>
		) : null;

	type SettingsSection = "parameters" | "deployments";
	const [openSection, setOpenSection] = useState<SettingsSection | undefined>("parameters");

	if (isInitialLoading) {
		return (
			<div className="flex h-full flex-col">
				<div className="space-y-6 p-4">
					<div className="flex flex-col gap-2">
						<Skeleton className="h-4 w-16" />
						<Skeleton className="h-9 w-full rounded-sm" />
					</div>
					<div className="flex flex-col gap-2">
						<Skeleton className="h-4 w-12" />
						<Skeleton className="h-9 w-full rounded-sm" />
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex min-h-0 flex-1 flex-col px-4 pt-2 pb-4">
				{settingsQueryFailed ? (
					<div className="mb-3">
						<QueryErrorBanner
							testId="prompts-settings-query-error"
							message={
								getErrorMessage(providersError || keysError || skillsError || mcpError || vkError) ||
									"Failed to load prompt settings data."
							}
						/>
					</div>
				) : null}
				<div className="flex items-center justify-between pb-2 border-b mb-1">
					<span className="text-xs font-semibold uppercase text-muted-foreground tracking-wider">Settings & Keys</span>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="h-6 w-6 text-muted-foreground hover:text-foreground"
								onClick={toggleSettings}
								data-testid="settings-panel-close-btn"
								aria-label="Collapse settings"
							>
								<PanelRightClose className="h-3.5 w-3.5" />
							</Button>
						</TooltipTrigger>
						<TooltipContent side="left">Collapse settings</TooltipContent>
					</Tooltip>
				</div>
				{!canUpdate && (
					<div className="flex items-center gap-2 rounded-md bg-amber-500/10 border border-amber-500/30 px-3 py-2 text-xs text-amber-600 dark:text-amber-400 font-medium mb-2 mt-1" data-testid="settings-view-only-banner">
						<Eye className="h-4 w-4 shrink-0" />
						<span>View-only access: configuration cannot be modified.</span>
					</div>
				)}
				<Accordion
					type="single"
					collapsible
					value={openSection ?? ""}
					onValueChange={(v) => {
						if (v === "parameters" || v === "deployments") {
							setOpenSection(v);
						} else {
							setOpenSection(undefined);
						}
					}}
					className="flex min-h-0 flex-1 flex-col"
				>
					<AccordionItem
						value="parameters"
						className={cn("flex min-h-0 flex-col border-b-0", openSection === "parameters" ? "flex-1" : "shrink-0 overflow-hidden")}
					>
						<AccordionTrigger
							data-testid="prompts-configuration-trigger"
							className="text-muted-foreground shrink-0 py-3 pr-1 text-xs font-medium uppercase hover:no-underline"
						>
							<span className="min-w-0 flex-1 text-left font-semibold">Configuration</span>
						</AccordionTrigger>
						<AccordionContent
							containerClassName="data-[state=open]:flex data-[state=open]:min-h-0 data-[state=open]:flex-1 data-[state=open]:flex-col"
							className="min-h-0 flex-1 overflow-y-auto pt-0 pb-8 pr-1.5 custom-scrollbar"
						>
							<div className="space-y-3.5">
								<div className="flex flex-col gap-1.5" data-testid="settings-provider">
									<Label className="text-muted-foreground text-xs font-medium uppercase">Provider</Label>
									<ComboboxSelect
										multiple
										compactTrigger
										options={providerOptions}
										value={
											allowedProviders && allowedProviders.length > 0
												? allowedProviders
												: (provider ? [provider] : [])
										}
										onValueChange={(next) => {
											if (!isMemberOnly) {
												setAllowedProviders(next);
											}
											if (next.length === 0) {
												onProviderChange("");
												onModelChange("");
							if (!isMemberOnly) setAllowedModels([]);
											} else if (!next.includes(provider)) {
							if (!isMemberOnly && !isVkSelected) setAllowedModels([]);
												onProviderChange(next[0]);
											}
										}}
										placeholder="Select provider"
										searchPlaceholder="Search providers..."
										selectAllValue="*"
										selectAllLabel="Select all"
										disabled={!canUpdate || providerOptions.length === 0}
										data-testid="settings-provider-select"
									/>
								</div>

								<div className="flex flex-col gap-1.5" data-testid="settings-model">
									<Label className="text-muted-foreground text-xs font-medium uppercase">Model</Label>
									<ComboboxSelect
										multiple
										compactTrigger
										creatable={!isMemberOnly}
										createLabel={(q) => `Add "${q}"`}
										options={modelOptions}
						value={selectedModelValues}
										onValueChange={(next) => {
											if (next.length === 0) {
												onModelChange("");
											} else {
								const currentScope = provider && model ? `${provider.toLowerCase()}/${model}` : model;
								const activeScope = next.includes(currentScope) ? currentScope : next[next.length - 1] || next[0] || "";
								if (!isMemberOnly) setAllowedModels(next.includes("*") ? ["*"] : next);
								if (activeScope && activeScope !== "*") {
									const target = splitModelScope(activeScope, provider);
									if (target.provider && target.provider !== "*") setProvider(target.provider);
									onModelChange(target.model);
								}
											}
										}}
										placeholder={!hasProviderSelected ? "Select a provider first" : "Select model"}
										searchPlaceholder="Search models..."
										selectAllValue="*"
										selectAllLabel="Select all"
										disabled={!hasProviderSelected || !canUpdate || modelOptions.length === 0}
										data-testid="settings-model-select"
									/>
								</div>

								{keySelector}
								{isMemberOnly && providerVirtualKeys.length === 0 && providerKeys.length === 0 && (
									<p className="text-amber-600 text-xs" data-testid="settings-no-member-key">
										No virtual key or API key is available for this prompt.
									</p>
								)}
								{isVkSelected && (!isMemberOnly || (mcpIncludeTools && mcpIncludeTools.length > 0)) && (
									<div className="flex flex-col gap-1.5 rounded-md border border-border/60 bg-muted/20 p-2" data-testid="settings-mcp-status">
										<div className="flex items-center justify-between">
											<div className="flex items-center gap-1.5">
												<Wrench className="h-3.5 w-3.5 text-primary" />
												<Label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
													Connected MCP Tools
												</Label>
											</div>
											<Badge variant={activeMCPClients.length > 0 ? "outline" : "secondary"} className="text-[10px] font-mono">
												{totalMCPTools} {totalMCPTools === 1 ? "tool" : "tools"}
											</Badge>
										</div>
										{mcpToolOptions.length === 0 ? (
											<p className="text-muted-foreground text-[11px] leading-relaxed">
												No MCP servers attached to this key. Attach servers in MCP Catalog or enable &quot;Available to all virtual keys&quot;.
											</p>
										) : (
											<ComboboxSelect
												multiple
												compactTrigger
												options={mcpToolOptions}
												value={selectedMcpTools}
												onValueChange={(next) => setMcpIncludeTools(next)}
												placeholder="Select MCP tools"
												searchPlaceholder="Search MCP tools..."
												selectAllValue="*"
												selectAllLabel="Select all"
												disabled={!canUpdate}
												data-testid="settings-mcp-tools"
											/>
										)}
									</div>
								)}

								{(!isMemberOnly || (allowedSkills && allowedSkills.length > 0) || skillId) && (
									<div className="flex flex-col gap-1.5" data-testid="settings-skill">
										<Label className="text-muted-foreground text-xs font-medium uppercase">Skill (optional)</Label>
										<p className="text-muted-foreground text-xs">
											Injects the skill markdown as a system message for this playground run.
										</p>
										<div className="flex flex-col gap-1.5">
											<Label className="text-xs text-muted-foreground font-medium">Allowed skills</Label>
											<ComboboxSelect
												multiple
												compactTrigger
												options={skillOptions}
												value={allowedSkills ?? (skillId ? [skillId] : [])}
												onValueChange={(next) => {
													setAllowedSkills(next);
													setSkillId(next[0] || "");
												}}
												placeholder={skillOptions.length === 0 ? "No skills available" : "Select skills"}
												searchPlaceholder="Search skills..."
												selectAllValue="*"
												selectAllLabel="Select all"
												disabled={!canUpdate}
												data-testid="settings-allowed-skills-select"
											/>
										</div>
									</div>
								)}

								{Object.keys(variables).length > 0 && (
									<>
										<Separator />
										<VariablesTableView variables={variables} onChange={setVariables} disabled={!canUpdate} />
									</>
								)}

								{requiredHeaders.length > 0 && (
									<>
										<Separator />
										<div className="flex flex-col gap-2" data-testid="settings-required-headers">
											<Label className="text-muted-foreground text-xs font-medium uppercase">Required Headers</Label>
											<p className="text-muted-foreground text-xs">
												These headers are required by the server. Provide a value for each to send requests from the playground.
											</p>
											<div className="flex flex-col gap-2">
												{requiredHeaders.map((name) => (
													<div key={name} className="flex items-center gap-2">
														<Label htmlFor={`required-header-${name}`} className="w-40 shrink-0 truncate font-mono text-xs">
															{name}
														</Label>
														<Input
															id={`required-header-${name}`}
															value={customHeaders[name] ?? ""}
															onChange={(e) => setCustomHeaders((prev) => ({ ...prev, [name]: e.target.value }))}
															placeholder="value"
															disabled={!canUpdate}
															className="h-8 flex-1"
														/>
													</div>
												))}
											</div>
										</div>
									</>
								)}

								{hasModel && !isMemberOnly && (
									<>
										<Separator />
										<div className="flex flex-col gap-4">
											<ModelParameters model={model} config={modelParams} onChange={handleModelParamsChange} hideFields={["promptTools"]} disabled={!canUpdate} />
										</div>
									</>
								)}
							</div>
						</AccordionContent>
					</AccordionItem>
					{selectedPromptId && !isMemberOnly && <PromptDeploymentsAccordionItem activeSection={openSection} />}
				</Accordion>
			</div>
		</div>
	);
}
