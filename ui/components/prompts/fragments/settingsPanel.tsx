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
import { getErrorMessage, useGetVirtualKeysQuery, useGetProviderGovernanceQuery, useGetVirtualKeyBillingBlocksQuery } from "@/lib/store";
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
function providersFromVirtualKeys(keys: VirtualKey[]): { providers: string[]; wildcardProviders: boolean; concreteModels: string[] } {
	const providers = new Set<string>();
	const concreteModels: string[] = [];
	let wildcardProviders = false;
	for (const key of keys) {
		for (const pc of key.provider_configs ?? []) {
			const prov = (pc.provider || "").trim();
			if (!prov) continue;
			if (prov === "*") {
				wildcardProviders = true;
				continue;
			}
			providers.add(prov);
			for (const m of pc.allowed_models ?? []) {
				if (m && m !== "*") {
					concreteModels.push(m);
				}
			}
		}
	}
	return { providers: [...providers], wildcardProviders, concreteModels };
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
	const focusUserId =
		selectedPrompt?.owner_user_id ||
		selectedPrompt?.user_ids?.split(",").map((id) => id.trim()).find(Boolean) ||
		(isMemberOnly ? (authStatus?.user_id || "") : "") ||
		"";
	const { data: focusUserKeys, refetch: refetchFocusUserKeys } = useGetUserVirtualKeysQuery(focusUserId, { skip: !focusUserId });
	const { data: sessionUsers, refetch: refetchSessionUsers } = useGetSessionUsersQuery(undefined, { skip: isMemberOnly });
	const { refetch: refetchProvGov } = useGetProviderGovernanceQuery({ fromMemory: true }, { skip: false });
	const { refetch: refetchBilling } = useGetVirtualKeyBillingBlocksQuery(undefined, { skip: !isMemberOnly });

	const [isRefreshingBudgets, setIsRefreshingBudgets] = useState(false);
	const handleRefreshBudgets = useCallback(async () => {
		setIsRefreshingBudgets(true);
		try {
			await Promise.allSettled([
				refetchAuth?.(),
				refetchVk?.(),
				refetchFocusUserKeys?.(),
				refetchSessionUsers?.(),
				refetchProvGov?.(),
				refetchBilling?.(),
			]);
			toast.success("Budgets updated live");
		} catch (e) {
			// ignore
		} finally {
			setTimeout(() => setIsRefreshingBudgets(false), 500);
		}
	}, [refetchAuth, refetchVk, refetchFocusUserKeys, refetchSessionUsers, refetchProvGov, refetchBilling]);
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
				setAllowedProviders(p ? [p] : []);
			}
			setApiKeyId("__auto__");
			if (!isMemberOnly) {
				setAllowedApiKeys([]);
			}
			onModelChange("");
			if (!isMemberOnly) {
				setAllowedModels([]);
			}
			onModelParamsChange((prev) => ({
				stream: true,
				temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
				max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
			}));
		},
		[setProvider, setApiKeyId, isMemberOnly, setAllowedApiKeys, setAllowedProviders, onModelChange, setAllowedModels, onModelParamsChange],
	);

	const onVirtualKeyChange = useCallback(
		(id: string) => {
			setVirtualKeyId(id);
			if (!id || id === "__auto__" || id === "__none__") {
				setAllowedVirtualKeys([]);
				return;
			}
			const targetVk = (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === id || vk.id === id);
			if (!targetVk) return;
			setAllowedVirtualKeys([targetVk.value]);
			setApiKeyId("__auto__");
			if (!isMemberOnly) {
				setAllowedApiKeys([]);
			}
			// Automatically select the connected provider and model for this VK
			const { providers: allowed, wildcardProviders, concreteModels } = providersFromVirtualKeys([targetVk]);
			if (allowed.length > 0) {
				onProviderChange(allowed[0]);
			} else if (!wildcardProviders && provider) {
				onProviderChange("");
			}
			if (concreteModels.length > 0) {
				onModelChange(concreteModels[0]);
				if (!isMemberOnly) {
					setAllowedModels([concreteModels[0]]);
				}
			}
		},
		[setVirtualKeyId, virtualKeysData, setAllowedVirtualKeys, setApiKeyId, isMemberOnly, setAllowedApiKeys, onModelChange, provider, onProviderChange, setAllowedModels],
	);

	// Admin inspecting a user's prompt: auto-select user's assigned VK if none selected yet on this prompt
	useEffect(() => {
		if (isMemberOnly) return;
		if (focusUserKeys?.virtual_keys && focusUserKeys.virtual_keys.length > 0) {
			const assignedVk = focusUserKeys.virtual_keys[0];
			if ((!virtualKeyId || virtualKeyId === "__auto__") && (!allowedVirtualKeys || allowedVirtualKeys.length === 0)) {
				onVirtualKeyChange(assignedVk.id);
			}
		}
	}, [isMemberOnly, focusUserKeys, virtualKeyId, allowedVirtualKeys, onVirtualKeyChange]);

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
			if (Array.isArray(allowedSkills) && allowedSkills.length > 0) {
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

	const isVkSelected = Boolean(
		(allowedVirtualKeys && allowedVirtualKeys.length > 0) ||
		(virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__") ||
		activeVkList.length > 0
	);

	const activeMCPClients = useMemo(() => {
		const clients = (mcpClientsData?.clients ?? []).filter((client) => !client.config?.disabled);
		if (isMemberOnly) {
			if (mcpIncludeTools && mcpIncludeTools.length > 0) {
				if (mcpIncludeTools.includes("*")) return clients;
				const prefixes = new Set(mcpIncludeTools.map((t) => t.split("-")[0]));
				return clients.filter((c) => c.config?.name && prefixes.has(c.config.name));
			}
		}
		if (activeVkList.length === 0) return [];
		const vkIds = new Set(activeVkList.map((vk) => vk.id));

		const selectedVKMcpNames = new Set<string>();
		for (const vk of activeVkList) {
			for (const mc of vk.mcp_configs ?? []) {
				if (mc.mcp_client?.name) selectedVKMcpNames.add(mc.mcp_client.name);
			}
		}

		return clients.filter((client) => {
			if (!client.config?.client_id) return false;
			if (client.config.allow_on_all_virtual_keys) return true;
			if (client.vk_configs?.some((vc) => vkIds.has(vc.virtual_key_id))) return true;
			if (client.config?.name && selectedVKMcpNames.has(client.config.name)) return true;
			return false;
		});
	}, [mcpClientsData, activeVkList, isMemberOnly, mcpIncludeTools]);

	const totalMCPTools = useMemo(
		() => activeMCPClients.reduce((acc, c) => acc + (c.tools?.length ?? 0), 0),
		[activeMCPClients],
	);

	// Virtual keys: Admin sees all active. Members ONLY see virtual keys committed by admin on this prompt!
	// If admin did NOT commit any virtual key, return [] (no VK auto-assigned).
	const providerVirtualKeys = useMemo(() => {
		const allActive = (virtualKeysData?.virtual_keys ?? []).filter((vk) => vk.is_active !== false);
		if (!isMemberOnly) {
			return allActive;
		}
		const committedVkIds = new Set<string>();
		if (allowedVirtualKeys && allowedVirtualKeys.length > 0) {
			for (const id of allowedVirtualKeys) {
				if (id) committedVkIds.add(id);
			}
		}
		if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__") {
			committedVkIds.add(virtualKeyId);
		}
		if (committedVkIds.size === 0) {
			return [];
		}

		const committedKeys = allActive.filter((vk) => committedVkIds.has(vk.id) || committedVkIds.has(vk.value));
		if (committedKeys.length === 0) return [];

		const assignedIds = new Set((focusUserKeys?.virtual_keys ?? []).map((k) => k.id));
		if (assignedIds.size > 0) {
			const intersected = committedKeys.filter((vk) => assignedIds.has(vk.id));
			if (intersected.length > 0) return intersected;
		}
		return committedKeys;
	}, [virtualKeysData, isMemberOnly, allowedVirtualKeys, virtualKeyId, focusUserKeys]);

	// Provider options:
	// If a Virtual Key is active, show ONLY providers configured in that VK.
	// Otherwise (Direct API key mode), show all providers that are enabled and have a connected API key.
	const providerOptions = useMemo(() => {
		const list: { label: string; value: string }[] = configuredProviders.map((p) => ({
			label: getProviderLabel(p.name),
			value: p.name as ModelProviderName,
		}));

		let allowed: string[] = [];
		if (isMemberOnly && allowedProviders && allowedProviders.length > 0) {
			allowed = allowedProviders;
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
	}, [configuredProviders, isMemberOnly, allowedProviders, activeVkList, providerVirtualKeys, provider]);

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

	// Admin: Start empty until the user selects. Members: never wipe committed settings!
	useEffect(() => {
		if (isMemberOnly) return;
		if (allowedProviders === null) setAllowedProviders([]);
		if (allowedModels === null) setAllowedModels([]);
		if (allowedVirtualKeys === null) setAllowedVirtualKeys([]);
		if (allowedApiKeys === null) setAllowedApiKeys([]);
		if (mcpIncludeTools === null) setMcpIncludeTools([]);
	}, [
		isMemberOnly,
		allowedProviders,
		allowedModels,
		allowedVirtualKeys,
		allowedApiKeys,
		mcpIncludeTools,
		setAllowedProviders,
		setAllowedModels,
		setAllowedVirtualKeys,
		setAllowedApiKeys,
		setMcpIncludeTools,
	]);

	const providerKeys = useMemo(() => {
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
			const committedKeys = (allowedApiKeys && allowedApiKeys.length > 0)
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
				if (virtualKeyId && virtualKeyId !== "__auto__") {
					onVirtualKeyChange("__auto__");
				}
			} else if (providerVirtualKeys.length === 1 && virtualKeyId !== providerVirtualKeys[0].value && virtualKeyId !== providerVirtualKeys[0].id) {
				onVirtualKeyChange(providerVirtualKeys[0].value);
			}
		}
	}, [isMemberOnly, providerVirtualKeys, virtualKeyId, onVirtualKeyChange]);

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
				const prov = item.provider ? String(item.provider).toLowerCase() : "";
				const label = prov ? `${prov} --- ${item.name}` : item.name;
				return { label, value: item.name };
			});

		// Deduplicate models by label (provider --- model name) so models from different providers are preserved
		const seenLabels = new Set<string>();
		const uniqueOpts: { label: string; value: string }[] = [];
		for (const opt of opts) {
			if (!seenLabels.has(opt.label)) {
				seenLabels.add(opt.label);
				uniqueOpts.push(opt);
			}
		}
		opts = uniqueOpts;

		// If VK specifies concrete allowed models, filter strictly to them
		if (activeVkList.length > 0) {
			const { concreteModels } = providersFromVirtualKeys(activeVkList);
			if (concreteModels.length > 0 && !concreteModels.includes("*")) {
				const allowedSet = new Set(concreteModels.map((m) => m.toLowerCase()));
				opts = opts.filter((o) => allowedSet.has(o.value.toLowerCase()));
				for (const m of concreteModels) {
					if (!opts.some((o) => o.value.toLowerCase() === m.toLowerCase())) {
						const prov = provider ? `${String(provider).toLowerCase()} --- ` : "";
						opts.push({ label: `${prov}${m}`, value: m });
					}
				}
			}
		}

		if (isMemberOnly) {
			const committedModels = (allowedModels && allowedModels.length > 0)
				? allowedModels
				: model
					? [model]
					: [];
			if (committedModels.length > 0 && !committedModels.includes("*")) {
				const allowedSet = new Set(committedModels.map((m) => m.toLowerCase()));
				opts = opts.filter((o) => allowedSet.has(o.value.toLowerCase()));
				for (const m of committedModels) {
					if (!opts.some((o) => o.value.toLowerCase() === m.toLowerCase())) {
						const prov = provider ? `${String(provider).toLowerCase()} --- ` : "";
						opts.push({ label: `${prov}${m}`, value: m });
					}
				}
			}
		}
		return opts;
	}, [hasProviderSelected, modelsData, model, provider, isMemberOnly, allowedModels, allowedProviders, activeVkList]);

	// Auto-select valid model whenever models change or when selected model is invalid for current provider/VK
	useEffect(() => {
		if (modelOptions.length > 0) {
			const currentModelValid = model && model !== "*" && modelOptions.some((o) => o.value === model);
			if (!currentModelValid) {
				const firstAllowed = (allowedModels ?? []).find((m) => m && m !== "*" && modelOptions.some((o) => o.value === m));
				const targetModel = firstAllowed || modelOptions[0].value;
				onModelChange(targetModel);
				if (!isMemberOnly && (!allowedModels || allowedModels.length === 0)) {
					setAllowedModels([targetModel]);
				}
			}
		}
	}, [modelOptions, model, allowedModels, onModelChange, isMemberOnly, setAllowedModels]);

	const mcpToolOptions = useMemo(() => {
		const clientsToUse = (isMemberOnly || isVkSelected)
			? activeMCPClients
			: (mcpClientsData?.clients ?? []).filter((c) => !c.config?.disabled);

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
			if (!mcpIncludeTools || mcpIncludeTools.length === 0) {
				return [];
			}
			if (mcpIncludeTools.includes("*")) {
				return opts;
			}
			const allowedSet = new Set(mcpIncludeTools);
			opts = opts.filter((o) => allowedSet.has(o.value));
		}
		return opts;
	}, [activeMCPClients, isMemberOnly, isVkSelected, mcpClientsData, mcpIncludeTools]);
	const mcpToolValues = useMemo(() => mcpToolOptions.map((option) => option.value), [mcpToolOptions]);
	const selectedMcpTools = (mcpIncludeTools ?? []).filter((value) => mcpToolValues.includes(value));

	// Auto-select MCP tools assigned with the active VK
	useEffect(() => {
		if (!isVkSelected || activeVkList.length === 0) return;
		const activeVk = activeVkList[0];
		const vkMcpNames = new Set((activeVk.mcp_configs ?? []).map((mc) => mc.mcp_client?.name).filter(Boolean));
		if (vkMcpNames.size > 0 && mcpToolOptions.length > 0) {
			if (!mcpIncludeTools || mcpIncludeTools.length === 0) {
				const matchingTools = mcpToolOptions
					.filter((opt) => {
						const clientName = opt.value.split("-")[0];
						return vkMcpNames.has(clientName);
					})
					.map((opt) => opt.value);
				if (matchingTools.length > 0) {
					setMcpIncludeTools(matchingTools);
				}
			}
		} else if (vkMcpNames.size === 0) {
			if (mcpIncludeTools && mcpIncludeTools.length > 0) {
				setMcpIncludeTools([]);
			}
		}
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
					onVirtualKeyChange(v ?? "__auto__");
					if (v && v !== "__auto__" && v !== "__none__") {
						onApiKeyIdChange("__auto__");
						setAllowedApiKeys([]);
					}
				}}
				virtualKeyValues={
					allowedVirtualKeys && allowedVirtualKeys.length > 0
						? allowedVirtualKeys
						: virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId !== "__none__"
							? [virtualKeyId]
							: []
				}
				onVirtualKeyValuesChange={
					isMemberOnly
						? undefined
						: (next) => {
								setAllowedVirtualKeys(next);
								if (next.length === 0) {
									setVirtualKeyId("__none__");
									return;
								}
								// Auto-clear direct API key so user/admin isn't using both!
								setAllowedApiKeys([]);
								onApiKeyIdChange("__auto__");
								const activeKeyId = next.includes(virtualKeyId) ? virtualKeyId : next[0];
								onVirtualKeyChange(activeKeyId);
						  }
				}
				apiKeyIdValue={apiKeyId}
				onApiKeyIdChange={(v) => {
					onApiKeyIdChange(v ?? "__auto__");
					if (v && v !== "__auto__" && v !== "__none__") {
						onVirtualKeyChange("__none__");
						setAllowedVirtualKeys([]);
					}
				}}
				apiKeyValues={allowedApiKeys ?? []}
				onApiKeyValuesChange={
					isMemberOnly
						? undefined
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
										value={
											allowedModels && allowedModels.length > 0
												? allowedModels
												: (model ? [model] : [])
										}
										onValueChange={(next) => {
											if (!isMemberOnly) {
												setAllowedModels(next);
											}
											if (next.length === 0) {
												onModelChange("");
											} else {
												const activeM = next.includes(model) ? model : (next[next.length - 1] || next[0] || "");
												onModelChange(activeM);
												// Auto-sync provider to this model's provider if belonging to another selected provider
												if (activeM) {
													const found = (modelsData?.models ?? []).find((m) => m.name === activeM);
													if (found?.provider && found.provider.toLowerCase() !== (provider || "").toLowerCase()) {
														onProviderChange(found.provider.toLowerCase());
													}
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
								{(!isMemberOnly || (mcpIncludeTools && mcpIncludeTools.length > 0)) && (
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