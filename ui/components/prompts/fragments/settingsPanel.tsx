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
import { getErrorMessage, useGetVirtualKeysQuery } from "@/lib/store";
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

/** Providers listed on the given VKs (real provider_configs only — no invented catalogs). */
function providersFromVirtualKeys(keys: VirtualKey[]): { providers: string[]; wildcardProviders: boolean } {
	const providers = new Set<string>();
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
		}
	}
	return { providers: [...providers], wildcardProviders };
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
	const { data: authStatus, error: authPollError } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });
	const isMemberOnly = isPromptMemberRole(authStatus?.role);

	const { data: virtualKeysData, isError: vkFailed, error: vkError } = useGetVirtualKeysQuery(undefined, { pollingInterval: vkPollMs });
	const focusUserId =
		selectedPrompt?.owner_user_id ||
		selectedPrompt?.user_ids?.split(",").map((id) => id.trim()).find(Boolean) ||
		(isMemberOnly ? (authStatus?.user_id || "") : "") ||
		"";
	const { data: focusUserKeys } = useGetUserVirtualKeysQuery(focusUserId, { skip: !focusUserId });
	const { data: sessionUsers } = useGetSessionUsersQuery(undefined, { skip: isMemberOnly });
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
			onModelParamsChange((prev) => ({
				stream: true,
				temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
				max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
			}));
		},
		[setProvider, onModelParamsChange],
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
			// Drop stale provider/model that the new VK does not allow (deny-by-default).
			const { providers: allowed, wildcardProviders } = providersFromVirtualKeys([targetVk]);
			if (!wildcardProviders && (allowed.length === 0 || (provider && !allowed.includes(provider)))) {
				setProvider("");
			}
			onModelChange("");
			setMcpIncludeTools([]);
		},
		[setVirtualKeyId, virtualKeysData, setAllowedVirtualKeys, onModelChange, provider, setProvider, setMcpIncludeTools],
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

	const isInitialLoading = isLoadingProviders;

	const configuredProviders = useMemo(() => {
		const activeVirtualKeys = virtualKeysData?.virtual_keys?.filter((vk) => vk.is_active) ?? [];
		if (!hasLoadedAllKeys) {
			return providers ?? [];
		}
		const keyedProviders = new Set((allKeys ?? []).map((k) => k.provider));
		return (providers ?? []).filter((p) => {
			if (keyedProviders.has(p.name)) return true;
			// Include providers that have active virtual keys (wildcard or explicitly targeting this provider)
			return activeVirtualKeys.some(
				(vk) => !vk.provider_configs || vk.provider_configs.length === 0 || vk.provider_configs.some((pc) => pc.provider === p.name),
			);
		});
	}, [providers, virtualKeysData, allKeys, hasLoadedAllKeys]);

	// Collect all VKs that are currently selected (from allowedVirtualKeys or virtualKeyId)
	const selectedVirtualKeys = useMemo(() => {
		const vks = virtualKeysData?.virtual_keys ?? [];
		if (isMemberOnly && virtualKeyId && virtualKeyId !== "__auto__") {
			return vks.filter((vk) => vk.value === virtualKeyId || vk.id === virtualKeyId);
		}
		const activeIds = new Set(
			allowedVirtualKeys && allowedVirtualKeys.length > 0
				? allowedVirtualKeys
				: (virtualKeyId && virtualKeyId !== "__auto__" ? [virtualKeyId] : [])
		);
		return vks.filter((vk) => activeIds.has(vk.value) || activeIds.has(vk.id));
	}, [virtualKeysData, allowedVirtualKeys, virtualKeyId, isMemberOnly]);

	// Only the user-selected VK — never fall back to the first catalog key.
	const selectedVirtualKey = useMemo(() => {
		if (selectedVirtualKeys.length === 1) return selectedVirtualKeys[0];
		if (!virtualKeyId || virtualKeyId === "__auto__") return undefined;
		const all = virtualKeysData?.virtual_keys ?? [];
		return all.find((vk) => vk.value === virtualKeyId || vk.id === virtualKeyId);
	}, [selectedVirtualKeys, virtualKeysData, virtualKeyId]);

	const activeMCPClients = useMemo(() => {
		const clients = (mcpClientsData?.clients ?? []).filter((client) => !client.config?.disabled);
		if (isMemberOnly) {
			if (mcpIncludeTools && mcpIncludeTools.length > 0) {
				if (mcpIncludeTools.includes("*")) return clients;
				const prefixes = new Set(mcpIncludeTools.map((t) => t.split("-")[0]));
				return clients.filter((c) => c.config?.name && prefixes.has(c.config.name));
			}
		}
		if (selectedVirtualKeys.length === 0 && !selectedVirtualKey) return [];
		const vkIds = new Set(selectedVirtualKeys.map((vk) => vk.id));
		if (selectedVirtualKey) vkIds.add(selectedVirtualKey.id);

		const selectedVKMcpNames = new Set<string>();
		for (const vk of selectedVirtualKeys) {
			for (const mc of vk.mcp_configs ?? []) {
				if (mc.mcp_client?.name) selectedVKMcpNames.add(mc.mcp_client.name);
			}
		}
		if (selectedVirtualKey?.mcp_configs) {
			for (const mc of selectedVirtualKey.mcp_configs) {
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
	}, [mcpClientsData, selectedVirtualKeys, selectedVirtualKey, isMemberOnly, mcpIncludeTools]);

	const totalMCPTools = useMemo(
		() => activeMCPClients.reduce((acc, c) => acc + (c.tools?.length ?? 0), 0),
		[activeMCPClients],
	);

	// Provider options:
	// For member: strictly the provider(s) committed by admin on this prompt!
	// For admin: full gateway catalog.
	const providerOptions = useMemo(() => {
		if (isMemberOnly) {
			let allowed: string[] = [];
			if (allowedProviders && allowedProviders.length > 0) {
				allowed = allowedProviders;
			} else if (provider) {
				allowed = [provider];
			}
			if (allowed.length === 0) {
				return [];
			}
			if (allowed.includes("*")) {
				return configuredProviders.map((p) => ({ label: getProviderLabel(p.name), value: p.name as ModelProviderName }));
			}
			const allowedSet = new Set(allowed.map((p) => p.toLowerCase()));
			return configuredProviders
				.filter((p) => allowedSet.has(p.name.toLowerCase()))
				.map((p) => ({ label: getProviderLabel(p.name), value: p.name as ModelProviderName }));
		}
		return configuredProviders.map((p) => ({ label: getProviderLabel(p.name), value: p.name as ModelProviderName }));
	}, [configuredProviders, isMemberOnly, allowedProviders, provider]);

	// For member: auto-select single/first provider if none selected
	useEffect(() => {
		if (isMemberOnly && providerOptions.length > 0) {
			if (!provider || !providerOptions.some((p) => p.value === provider)) {
				onProviderChange(providerOptions[0].value);
			}
		}
	}, [isMemberOnly, providerOptions, provider, onProviderChange]);

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

		let keys = allKeys ?? [];
		if (activeProviders.length > 0 && !activeProviders.includes("*")) {
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

	// Member: auto-sync apiKeyId only if NO Virtual Key is assigned/committed
	useEffect(() => {
		if (isMemberOnly) {
			if (providerVirtualKeys.length > 0) {
				// If VK is committed, keep apiKeyId empty/auto so direct API key stays OFF
				if (apiKeyId && apiKeyId !== "__auto__") {
					onApiKeyIdChange("__auto__");
				}
			} else {
				if (providerKeys.length === 1 && apiKeyId !== providerKeys[0].key_id) {
					onApiKeyIdChange(providerKeys[0].key_id);
				} else if (providerKeys.length === 0 && apiKeyId && apiKeyId !== "__auto__") {
					onApiKeyIdChange("__auto__");
				}
			}
		}
	}, [isMemberOnly, providerKeys, providerVirtualKeys, apiKeyId, onApiKeyIdChange]);

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
		if (isMemberOnly) return provider || undefined;
		if (allowedProviders && allowedProviders.length > 0) {
			if (allowedProviders.includes("*")) return undefined;
			return allowedProviders.join(",");
		}
		return provider || undefined;
	}, [isMemberOnly, allowedProviders, provider]);

	const shouldSkipModels = useMemo(() => {
		if (isMemberOnly) return !provider;
		const hasProv = (allowedProviders && allowedProviders.length > 0) || Boolean(provider);
		return !hasProv;
	}, [isMemberOnly, allowedProviders, provider]);

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
		if (isMemberOnly) return Boolean(provider);
		return (allowedProviders && allowedProviders.length > 0) || Boolean(provider);
	}, [isMemberOnly, allowedProviders, provider]);

	const modelOptions = useMemo(() => {
		if (!hasProviderSelected) return [];
		let opts = (modelsData?.models ?? []).map((item) => {
			const prov = item.provider ? String(item.provider).toLowerCase() : "";
			const label = prov ? `${prov} --- ${item.name}` : item.name;
			return { label, value: item.name };
		});
		if (model && opts.length > 0 && !opts.some((option) => option.value === model)) {
			const prov = provider ? `${String(provider).toLowerCase()} --- ` : "";
			opts.unshift({ label: `${prov}${model}`, value: model });
		}
		if (isMemberOnly) {
			const committedModels = (allowedModels && allowedModels.length > 0)
				? allowedModels
				: model
					? [model]
					: [];
			if (committedModels.length > 0 && !committedModels.includes("*")) {
				const allowedSet = new Set(committedModels);
				opts = opts.filter((o) => allowedSet.has(o.value));
				for (const m of committedModels) {
					if (!opts.some((o) => o.value === m)) {
						const prov = provider ? `${String(provider).toLowerCase()} --- ` : "";
						opts.push({ label: `${prov}${m}`, value: m });
					}
				}
			}
		}
		return opts;
	}, [hasProviderSelected, modelsData, model, provider, isMemberOnly, allowedModels]);

	// Member: auto-select single committed model
	useEffect(() => {
		if (isMemberOnly && modelOptions.length > 0) {
			if (!model || !modelOptions.some((o) => o.value === model)) {
				onModelChange(modelOptions[0].value);
			}
		}
	}, [isMemberOnly, modelOptions, model, onModelChange]);

	// Never auto-pick a model for admin. If the saved/current model is no longer in the API list, clear it.
	useEffect(() => {
		if (isMemberOnly) return;
		const allModelNames = modelOptions.map((o) => o.value);
		if (!model) return;
		if (allModelNames.length > 0 && !allModelNames.includes(model)) {
			onModelChange("");
		}
		if (allowedModels && allowedModels.length > 0 && !allowedModels.includes(model) && !allModelNames.includes(model)) {
			onModelChange("");
		}
	}, [modelOptions, model, allowedModels, onModelChange, isMemberOnly]);

	const mcpToolOptions = useMemo(() => {
		const clientsToUse = isMemberOnly
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
	}, [activeMCPClients, isMemberOnly, mcpClientsData, mcpIncludeTools]);
	const mcpToolValues = useMemo(() => mcpToolOptions.map((option) => option.value), [mcpToolOptions]);
	const selectedMcpTools = (mcpIncludeTools ?? []).filter((value) => mcpToolValues.includes(value));

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
					if (isMemberOnly && v && v !== "__auto__" && v !== "__none__") {
						onApiKeyIdChange("__auto__");
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
									setVirtualKeyId("__auto__");
									return;
								}
								if (!next.includes(virtualKeyId)) {
									setVirtualKeyId(next.length === 1 ? next[0] : "__auto__");
								}
							}
				}
				apiKeyIdValue={apiKeyId}
				onApiKeyIdChange={(v) => {
					onApiKeyIdChange(v ?? "__auto__");
					if (isMemberOnly && v && v !== "__auto__" && v !== "__none__") {
						onVirtualKeyChange("__none__");
					}
				}}
				apiKeyValues={allowedApiKeys ?? []}
				onApiKeyValuesChange={
					isMemberOnly
						? undefined
						: (next) => {
								setAllowedApiKeys(next);
								if (next.length === 0 || !next.includes(apiKeyId)) {
									setApiKeyId(next.length === 1 ? next[0] : "__auto__");
								}
							}
				}
				provider={provider}
				model={model}
				providerLabel={getProviderLabel(provider)}
				targetUserBudget={targetUserBudget}
				targetUserUsage={targetUserUsage}
				targetUserLabel={targetUserLabel}
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
							className="min-h-0 flex-1 overflow-y-auto pt-0 pb-2"
						>
							<div className="space-y-6">
								<div className="flex flex-col gap-2" data-testid="settings-provider">
									<Label className="text-muted-foreground text-xs font-medium uppercase">Provider</Label>
									{isMemberOnly ? (
										<ComboboxSelect
											options={providerOptions}
											value={provider || null}
											onValueChange={(v) => onProviderChange(v ?? "")}
											placeholder="Select provider"
											searchPlaceholder="Search..."
											disabled={!canUpdate}
											data-testid="settings-provider-select"
										/>
									) : (
										<ComboboxSelect
											multiple
											compactTrigger
											options={providerOptions}
											value={allowedProviders ?? []}
											onValueChange={(next) => {
												setAllowedProviders(next);
												if (!next.includes(provider)) onProviderChange(next[0] || "");
												if (next.length === 0) {
													onProviderChange("");
													onModelChange("");
													setAllowedModels([]);
												}
											}}
											placeholder="Select providers"
											searchPlaceholder="Search..."
											selectAllValue="*"
											selectAllLabel="Select all"
											disabled={!canUpdate}
										/>
									)}
								</div>

								<div className="flex flex-col gap-2" data-testid="settings-model">
									<Label className="text-muted-foreground text-xs font-medium uppercase">Model</Label>
									{isMemberOnly ? (
										<ComboboxSelect
											options={modelOptions}
											value={model || null}
											onValueChange={(v) => onModelChange(v ?? "")}
											placeholder={!hasProviderSelected ? "Select a provider first" : "Select model"}
											searchPlaceholder="Search..."
											disabled={!hasProviderSelected || !canUpdate}
											data-testid="settings-model-select"
										/>
									) : (
										<ComboboxSelect
											multiple
											compactTrigger
											options={modelOptions}
											value={allowedModels ?? []}
											onValueChange={(next) => {
												setAllowedModels(next);
												if (!next.includes(model)) onModelChange(next[0] || "");
											}}
											placeholder={!hasProviderSelected ? "Select a provider first" : "Select models"}
											searchPlaceholder="Search..."
											disabled={!hasProviderSelected || !canUpdate}
											selectAllValue="*"
											selectAllLabel="Select all"
											selectAllTriggerLabel="All models"
											data-testid="settings-model-select"
										/>
									)}
								</div>

								{keySelector}
								{isMemberOnly && providerVirtualKeys.length === 0 && providerKeys.length === 0 && (
									<p className="text-amber-600 text-xs" data-testid="settings-no-member-key">
										No virtual key or API key is available for this prompt.
									</p>
								)}
								{(!isMemberOnly || (mcpIncludeTools && mcpIncludeTools.length > 0) || selectedVirtualKey) && (
									<div className="flex flex-col gap-2 rounded-md border border-border/60 bg-muted/20 p-2.5" data-testid="settings-mcp-status">
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
												searchPlaceholder="Search..."
												selectAllValue="*"
												selectAllLabel="Select all"
												disabled={!canUpdate}
												data-testid="settings-mcp-tools"
											/>
										)}
									</div>
								)}

								{(!isMemberOnly || (allowedSkills && allowedSkills.length > 0) || skillId) && (
									<div className="flex flex-col gap-2" data-testid="settings-skill">
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