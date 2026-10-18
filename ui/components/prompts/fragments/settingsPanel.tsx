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
import { getErrorMessage, useGetVirtualKeyBillingBlocksQuery, useGetVirtualKeysQuery } from "@/lib/store";
import { useListSkillsQuery } from "@/lib/store/apis/skillsApi";
import { useGetMCPClientsQuery } from "@/lib/store/apis/mcpApi";
import { useGetBusinessUnitsQuery } from "@enterprise/lib/store/apis/businessUnitsApi";
import { useGetUserVirtualKeysQuery } from "@enterprise/lib/store/apis/virtualKeyUsersApi";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Clock, PanelRightClose, Trash2, Wrench } from "lucide-react";
import { toast } from "sonner";
import { ModelProviderName } from "@/lib/types/config";
import { ModelParams } from "@/lib/types/prompts";
import { cn } from "@/lib/utils";
import { PromptDeploymentsAccordionItem } from "@enterprise/components/prompt-deployments/promptDeploymentsAccordionItem";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiKeySelectorView } from "../components/apiKeySelectorView";
import { VariablesTableView } from "../components/variablesTableView";
import { usePromptContext } from "../context";
import { useIsAuthEnabledQuery } from "@/lib/store";

function explicitAssignments(keys: { provider_configs?: { provider?: string; allowed_models?: string[] }[] }[]) {
	const providers = new Set<string>();
	const models = new Set<string>();
	for (const key of keys) {
		for (const config of key.provider_configs ?? []) {
			if (config.provider && config.provider !== "*") providers.add(config.provider);
			for (const modelName of config.allowed_models ?? []) {
				if (modelName && modelName !== "*") models.add(modelName);
			}
		}
	}
	return { providers: [...providers], models: [...models] };
}

function isEntireCatalog(selected: string[] | null | undefined, catalog: string[]) {
	if (!selected || catalog.length <= 1) return false;
	return selected.length >= catalog.length && catalog.every((name) => selected.includes(name));
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
		folders,
		promptFilters,
		variables,
		setVariables,
		customHeaders,
		setCustomHeaders,
		requiredHeaders,
		selectedPromptId,
		toggleSettings,
	} = usePromptContext();

	const POLL_MS = 5000;
	const [authPollMs, setAuthPollMs] = useState(POLL_MS);
	const [vkPollMs, setVkPollMs] = useState(POLL_MS);
	const { data: authStatus, error: authPollError } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });
	const isMemberOnly = Boolean(authStatus?.role && authStatus.role !== "admin");

	const { data: virtualKeysData, isError: vkFailed, error: vkError } = useGetVirtualKeysQuery(undefined, { pollingInterval: vkPollMs });
	const { data: businessUnitsData } = useGetBusinessUnitsQuery();
	const focusUserId =
		selectedPrompt?.owner_user_id ||
		selectedPrompt?.user_ids?.split(",").map((id) => id.trim()).find(Boolean) ||
		"";
	const {
		data: focusUserKeys,
		isSuccess: focusUserKeysReady,
		isError: focusUserKeysFailed,
	} = useGetUserVirtualKeysQuery(focusUserId, { skip: !focusUserId });

	useEffect(() => {
		setAuthPollMs(authPollError ? 0 : POLL_MS);
	}, [authPollError]);
	useEffect(() => {
		setVkPollMs(vkFailed ? 0 : POLL_MS);
	}, [vkFailed]);

	const onProviderChange = useCallback(
		(p: string) => {
			setProvider(p);
			// Keep the picked virtual key when it also serves the new provider (or is a wildcard key).
			const current = (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === virtualKeyId);
			if (current?.provider_configs && current.provider_configs.length > 0 && !current.provider_configs.some((pc) => pc.provider === p || pc.provider === "*")) {
				setVirtualKeyId("__auto__");
			}
			onModelChange("");
			onModelParamsChange((prev) => ({
				stream: true,
				temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
				max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
			}));
		},
		[setProvider, setVirtualKeyId, onModelChange, onModelParamsChange, virtualKeysData, virtualKeyId],
	);

	const onVirtualKeyChange = useCallback(
		(id: string) => {
			setVirtualKeyId(id);
			onModelChange("");
			setMcpIncludeTools(null);
		},
		[setVirtualKeyId, onModelChange, setMcpIncludeTools],
	);

	const onApiKeyIdChange = useCallback(
		(id: string) => {
			setApiKeyId(id);
			onModelChange("");
		},
		[setApiKeyId, onModelChange],
	);
	// Dynamic providers
	const { data: providers, isLoading: isLoadingProviders, isError: providersFailed, error: providersError } = useGetProvidersQuery();
	// Keys for the API Key selector (from /api/keys endpoint, provider-filtered)
	const { data: allKeys, isSuccess: hasLoadedAllKeys, isError: keysFailed, error: keysError } = useGetAllKeysQuery(undefined, {
		skip: isMemberOnly,
	});
	const { data: skillsData, isError: skillsFailed, error: skillsError } = useListSkillsQuery({ limit: 100, offset: 0 });
	const { data: mcpClientsData, isError: mcpFailed, error: mcpError } = useGetMCPClientsQuery();
	const settingsQueryFailed = providersFailed || keysFailed || skillsFailed || mcpFailed || vkFailed;
	const skillOptions = useMemo(() => {
		const opts: { label: string; value: string }[] = (skillsData?.skills ?? []).map((s) => ({ label: s.name, value: s.id }));
		if (skillId && !opts.some((o) => o.value === skillId)) {
			opts.unshift({ label: `Skill (${skillId.slice(0, 8)}...)`, value: skillId });
		}
		return opts;
	}, [skillsData, skillId]);

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
		const activeIds = new Set(
			allowedVirtualKeys && allowedVirtualKeys.length > 0
				? allowedVirtualKeys
				: (virtualKeyId && virtualKeyId !== "__auto__" ? [virtualKeyId] : [])
		);
		return vks.filter((vk) => activeIds.has(vk.value) || activeIds.has(vk.id));
	}, [virtualKeysData, allowedVirtualKeys, virtualKeyId]);

	// Primary single virtual key (if only 1 key is selected, or matching virtualKeyId)
	const selectedVirtualKey = useMemo(() => {
		if (selectedVirtualKeys.length === 1) return selectedVirtualKeys[0];
		return (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === virtualKeyId);
	}, [selectedVirtualKeys, virtualKeysData, virtualKeyId]);

	const activeMCPClients = useMemo(() => {
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

		return (mcpClientsData?.clients ?? []).filter((client) => {
			if (!client.config?.client_id || client.config.disabled) return false;
			if (client.config.allow_on_all_virtual_keys) return true;
			if (client.vk_configs?.some((vc) => vkIds.has(vc.virtual_key_id))) return true;
			if (client.config?.name && selectedVKMcpNames.has(client.config.name)) return true;
			return false;
		});
	}, [mcpClientsData, selectedVirtualKeys, selectedVirtualKey]);

	const totalMCPTools = useMemo(
		() => activeMCPClients.reduce((acc, c) => acc + (c.tools?.length ?? 0), 0),
		[activeMCPClients],
	);

	// All providers across all selected VKs
	const selectedVKProviders = useMemo(() => {
		const provs = new Set<string>();
		for (const vk of selectedVirtualKeys) {
			for (const pc of vk.provider_configs ?? []) {
				if (pc.provider) provs.add(pc.provider);
			}
		}
		if (provs.size === 0 && selectedVirtualKey) {
			for (const pc of selectedVirtualKey.provider_configs ?? []) {
				if (pc.provider) provs.add(pc.provider);
			}
		}
		return [...provs];
	}, [selectedVirtualKeys, selectedVirtualKey]);

	useEffect(() => {
		if (selectedVirtualKeys.length === 0) return;
		const { providers: explicit } = explicitAssignments(selectedVirtualKeys);
		const catalog = configuredProviders.map((providerItem) => providerItem.name);
		const unset = !allowedProviders || allowedProviders.length === 0;
		const selectingEveryone = isEntireCatalog(allowedProviders, catalog);
		if (explicit.length > 0 && (unset || selectingEveryone)) {
			setAllowedProviders(explicit);
		} else if (explicit.length === 0 && (unset || selectingEveryone)) {
			const keep = provider && catalog.includes(provider as ModelProviderName) ? provider : catalog[0];
			if (keep) setAllowedProviders([keep as ModelProviderName]);
		}
		const allowed = explicit.length > 0 ? explicit : catalog;
		if (allowed.length > 0 && (!provider || (explicit.length > 0 && !explicit.some((name) => name.toLowerCase() === provider.toLowerCase())))) {
			const nextProv = explicit[0] || allowed[0] || "";
			if (nextProv && nextProv !== provider) {
				setProvider(nextProv);
				onModelChange("");
				onModelParamsChange((prev) => ({
					stream: true,
					temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
					max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
				}));
			}
		}
	}, [selectedVirtualKeys, provider, allowedProviders, configuredProviders, setAllowedProviders, setProvider, onModelChange, onModelParamsChange]);

	// Ensure current provider always has a label-resolved option (even before providers query loads)
	const providerOptions = useMemo(() => {
		if (isMemberOnly) {
			if (selectedVKProviders.length > 0 && !selectedVKProviders.includes("*")) {
				const opts = selectedVKProviders.map((p) => ({ label: getProviderLabel(p), value: p as ModelProviderName }));
				if (provider && !opts.find((o) => o.value === provider)) {
					opts.unshift({ label: getProviderLabel(provider), value: provider as ModelProviderName });
				}
				return opts;
			}
			const opts = configuredProviders.map((p) => ({ label: getProviderLabel(p.name), value: p.name }));
			if (provider && !opts.find((o) => o.value === provider)) {
				opts.unshift({ label: getProviderLabel(provider), value: provider as ModelProviderName });
			}
			return opts;
		}
		// Assigned-key providers stay selected, and every other configured provider stays pickable.
		const opts = configuredProviders.map((p) => ({ label: getProviderLabel(p.name), value: p.name }));
		if (provider && !opts.find((o) => o.value === provider)) {
			opts.unshift({ label: getProviderLabel(provider), value: provider as ModelProviderName });
		}
		return opts;
	}, [configuredProviders, provider, selectedVKProviders, isMemberOnly]);

	const providerKeys = useMemo(() => {
		// Members must use assigned Virtual Keys only — never raw provider keys.
		if (isMemberOnly) return [];
		return (allKeys ?? []).filter((k) => k.provider === provider);
	}, [allKeys, provider, isMemberOnly]);

	// Virtual keys: show active virtual keys that serve the selected provider (or are wildcard keys).
	// When no provider is selected, show all active keys.
	const providerVirtualKeys = useMemo(() => {
		let vks = (virtualKeysData?.virtual_keys ?? []).filter((vk) => vk.is_active !== false);
		if (isMemberOnly && allowedVirtualKeys) {
			vks = vks.filter((vk) => allowedVirtualKeys.includes(vk.value));
		}
		if (!provider) return vks;
		const targetProvider = provider.toLowerCase();
		return vks.filter((vk) => {
			// Wildcard virtual keys (empty or no provider_configs) serve all providers
			if (!vk.provider_configs || vk.provider_configs.length === 0) return true;
			// Explicit provider matching (case-insensitive or wildcard)
			return vk.provider_configs.some(
				(pc) => pc.provider === "*" || pc.provider.toLowerCase() === targetProvider,
			);
		});
	}, [virtualKeysData, provider, isMemberOnly, allowedVirtualKeys]);

	const virtualKeyScopeRef = useRef("");
	useEffect(() => {
		const scopeKey = `${selectedPrompt?.id || ""}|${focusUserId}|${folders?.length || 0}`;
		if (!virtualKeysData) return;
		if (focusUserId && !focusUserKeysReady && !focusUserKeysFailed) return;
		if (virtualKeyScopeRef.current === scopeKey && allowedVirtualKeys !== null) return;
		virtualKeyScopeRef.current = scopeKey;
		const inheritedKeyIds = new Set((focusUserKeys?.virtual_keys ?? []).map((key) => key.id));
		const vks = (virtualKeysData.virtual_keys ?? []).filter((vk) => vk.is_active !== false);

		// Collect target user, team, customer IDs from selectedPrompt, promptFilters, and folder tree
		const targetUserIds = new Set<string>();
		const targetTeamIds = new Set<string>();
		const targetCustomerIds = new Set<string>();

		if (selectedPrompt?.user_ids) {
			selectedPrompt.user_ids.split(",").map((s) => s.trim()).filter(Boolean).forEach((id) => targetUserIds.add(id));
		}
		if (selectedPrompt?.owner_user_id) {
			targetUserIds.add(selectedPrompt.owner_user_id);
		}
		if (selectedPrompt?.team_ids) {
			selectedPrompt.team_ids.split(",").map((s) => s.trim()).filter(Boolean).forEach((id) => targetTeamIds.add(id));
		}
		if (selectedPrompt?.customer_ids) {
			selectedPrompt.customer_ids.split(",").map((s) => s.trim()).filter(Boolean).forEach((id) => targetCustomerIds.add(id));
		}

		if (promptFilters?.user_ids) {
			promptFilters.user_ids.forEach((id) => targetUserIds.add(id));
		}
		if (promptFilters?.team_ids) {
			promptFilters.team_ids.forEach((id) => targetTeamIds.add(id));
		}
		if (promptFilters?.customer_ids) {
			promptFilters.customer_ids.forEach((id) => targetCustomerIds.add(id));
		}

		if (selectedPrompt?.folder_id && folders) {
			const fMap = new Map(folders.map((f) => [f.id, f]));
			let cur = fMap.get(selectedPrompt.folder_id);
			while (cur) {
				if (cur.entity_id) {
					const parent = cur.parent_id ? fMap.get(cur.parent_id) : undefined;
					if (cur.type === "system_users_root" || parent?.name === "Users" || parent?.type === "system_users_root") {
						targetUserIds.add(cur.entity_id);
					} else if (cur.type === "system_teams_root" || parent?.name === "Teams" || parent?.type === "system_teams_root") {
						targetTeamIds.add(cur.entity_id);
					} else if (cur.type === "system_business_units_root" || parent?.name === "Business Units" || parent?.type === "system_business_units_root" || cur.type === "business_unit") {
						// business unit folder
					} else if (cur.type === "system_customers_root" || parent?.name === "Customers" || parent?.type === "system_customers_root") {
						targetCustomerIds.add(cur.entity_id);
					}
				}
				cur = cur.parent_id ? fMap.get(cur.parent_id) : undefined;
			}
		}

		const hasSpecificTarget = targetUserIds.size > 0 || targetTeamIds.size > 0 || targetCustomerIds.size > 0;

		const matched = hasSpecificTarget
			? vks
					.filter((vk) => {
						const vkUsers = new Set([...(vk.user_ids || []), ...(vk.users || []).map((user) => user.user_id)]);
						const vkTeams = new Set([...(vk.team_ids || []), vk.team_id, ...(vk.teams || []).map((team) => team.id)].filter(Boolean));
						const vkCustomers = new Set(
							[...(vk.customer_ids || []), vk.customer_id, vk.customer?.id, ...(vk.customers || []).map((customer) => customer.id)].filter(Boolean),
						);
						const vkUnits = new Set(
							[...(vk.business_unit_ids || []), ...(vk.business_units || []).map((unit) => unit.id)].filter(Boolean),
						);
						const teamInAssignedUnit = [...targetTeamIds].some((teamId) =>
							(businessUnitsData?.business_units || []).some(
								(unit) => vkUnits.has(unit.id) && (unit.team_ids || []).includes(teamId),
							),
						);
						return (
							inheritedKeyIds.has(vk.id) ||
							[...targetUserIds].some((id) => vkUsers.has(id)) ||
							[...targetTeamIds].some((id) => vkTeams.has(id as string)) ||
							[...targetCustomerIds].some((id) => vkCustomers.has(id as string)) ||
							teamInAssignedUnit
						);
					})
					.map((vk) => vk.value)
			: [];

		if (matched.length > 0) {
			// Only select the keys assigned to that user / team / customer.
			setAllowedVirtualKeys(matched);
			if (!virtualKeyId || virtualKeyId === "__auto__" || !matched.includes(virtualKeyId)) {
				setVirtualKeyId(matched[0]);
			}
		} else if (hasSpecificTarget || focusUserId) {
			setAllowedVirtualKeys([]);
			setVirtualKeyId("__auto__");
			setAllowedProviders([]);
			setAllowedModels([]);
		} else {
			setAllowedVirtualKeys([]);
			setVirtualKeyId("__auto__");
		}
	}, [allowedVirtualKeys, virtualKeysData, businessUnitsData, focusUserId, focusUserKeys, focusUserKeysReady, focusUserKeysFailed, selectedPrompt, folders, promptFilters, virtualKeyId, setAllowedVirtualKeys, setVirtualKeyId, setAllowedProviders, setAllowedModels]);

	useEffect(() => {
		if (!allowedProviders?.length || allowedProviders.includes(provider)) return;
		onProviderChange(allowedProviders[0]);
	}, [allowedProviders, provider, onProviderChange]);

	useEffect(() => {
		if (!allowedModels?.length || allowedModels.includes(model)) return;
		onModelChange(allowedModels[0]);
	}, [allowedModels, model, onModelChange]);

	// Keep the active key inside the keys already assigned to this user.
	const { data: billingBlocks } = useGetVirtualKeyBillingBlocksQuery(undefined);
	useEffect(() => {
		if (!allowedVirtualKeys || allowedVirtualKeys.length === 0) return;
		const assignable = providerVirtualKeys.filter(
			(vk) => allowedVirtualKeys.includes(vk.value) && typeof vk.value === "string" && vk.value.startsWith("sk-uf-"),
		);
		const first = assignable.find((vk) => !billingBlocks?.blocks?.[vk.id]) ?? assignable[0];
		if (!first?.value) return;
		if (virtualKeyId === "__auto__" || !virtualKeyId || !assignable.some((vk) => vk.value === virtualKeyId)) {
			setVirtualKeyId(first.value);
		}
	}, [providerVirtualKeys, virtualKeyId, setVirtualKeyId, billingBlocks, allowedVirtualKeys]);

	// Separate keys/vks to pass to model fetch for filtering.
	const filterKeys = useMemo(() => {
		const isProviderKey = providerKeys.some((k) => k.key_id === apiKeyId);
		if (isProviderKey) return [apiKeyId];
		return undefined;
	}, [apiKeyId, providerKeys]);

	const filterVks = useMemo(() => {
		if (selectedVirtualKeys.length > 0) return selectedVirtualKeys.map((vk) => vk.id);
		if (selectedVirtualKey) return [selectedVirtualKey.id];
		return undefined;
	}, [selectedVirtualKeys, selectedVirtualKey]);

	const { data: modelsData } = useGetModelsQuery(
		{
			provider: provider || undefined,
			keys: filterKeys && filterKeys.length > 0 ? filterKeys : undefined,
			vks: filterVks,
			limit: 5000,
			unfiltered: !filterVks && (!filterKeys || filterKeys.length === 0),
		},
		{ skip: !provider },
	);
	const { data: assignedVkModelsData } = useGetModelsQuery(
		{
			vks: filterVks,
			limit: 5000,
		},
		{ skip: !filterVks || filterVks.length === 0 },
	);
	const modelOptions = useMemo(() => {
		const opts = (modelsData?.models ?? []).map((item) => ({ label: item.name, value: item.name }));
		if (model && !opts.some((option) => option.value === model)) {
			opts.unshift({ label: model, value: model });
		}
		return opts;
	}, [modelsData, model]);

	// Select only the models named on this user's keys. A wildcard must not check the whole catalog.
	useEffect(() => {
		if (selectedVirtualKeys.length === 0) return;
		const { models: explicit } = explicitAssignments(selectedVirtualKeys);
		const catalog = (assignedVkModelsData?.models ?? modelsData?.models ?? []).map((item) => item.name);
		const currentNames = (modelsData?.models ?? []).map((item) => item.name);
		const unset = !allowedModels || allowedModels.length === 0;
		const selectingEveryone = isEntireCatalog(allowedModels, catalog);
		if (explicit.length > 0 && (unset || selectingEveryone)) {
			setAllowedModels(explicit);
		} else if (explicit.length === 0 && (unset || selectingEveryone)) {
			const keep = model && currentNames.includes(model) ? model : currentNames[0];
			if (keep) setAllowedModels([keep]);
		}
		const preferred = explicit.filter((name) => currentNames.includes(name));
		const nextModel = preferred[0] || currentNames[0] || "";
		if (nextModel && (!model || !currentNames.includes(model))) {
			onModelChange(nextModel);
		}
	}, [assignedVkModelsData, modelsData, selectedVirtualKeys, allowedModels, model, setAllowedModels, onModelChange]);

	const mcpToolOptions = useMemo(() => {
		const opts: { label: string; value: string }[] = [];
		for (const client of activeMCPClients) {
			const clientName = client.config?.name || "";
			if (!clientName) continue;
			const tools = client.tools ?? [];
			if (tools.length === 0) {
				opts.push({ label: `${clientName} (all tools)`, value: `${clientName}-*` });
				continue;
			}
			for (const tool of tools) {
				if (!tool.name) continue;
				opts.push({ label: `${clientName} / ${tool.name}`, value: `${clientName}-${tool.name}` });
			}
		}
		return opts;
	}, [activeMCPClients]);
	const mcpToolValues = useMemo(() => mcpToolOptions.map((option) => option.value), [mcpToolOptions]);
	const selectedMcpTools = mcpIncludeTools === null ? mcpToolValues : mcpIncludeTools.filter((value) => mcpToolValues.includes(value));

	// Auto-select all MCP tools connected to active clients
	useEffect(() => {
		if (mcpToolValues.length > 0 && mcpIncludeTools === null) {
			setMcpIncludeTools(mcpToolValues);
		}
	}, [mcpToolValues, mcpIncludeTools, setMcpIncludeTools]);

	const handleModelParamsChange = useCallback(
		(params: Record<string, any>) => {
			onModelParamsChange(params as ModelParams);
		},
		[onModelParamsChange],
	);

	const hasModel = Boolean(model);

	// Key selector shows when either provider keys or virtual keys are available, or always for admins.
	const showKeySelector = !isMemberOnly || providerKeys.length > 0 || providerVirtualKeys.length > 0;
	// Members always get the budget strip (user budget always; VK meters only when a key is assigned).
	const showMemberBudgetStrip = isMemberOnly;
	const keySelector =
		showKeySelector || showMemberBudgetStrip ? (
			<ApiKeySelectorView
				providerKeys={
					isMemberOnly && allowedApiKeys
						? providerKeys.filter((key) => allowedApiKeys.includes(key.key_id))
						: providerKeys
				}
				virtualKeys={providerVirtualKeys}
				virtualKeyValue={virtualKeyId}
				onVirtualKeyChange={(v) => onVirtualKeyChange(v ?? "__auto__")}
				virtualKeyValues={
					allowedVirtualKeys && allowedVirtualKeys.length > 0
						? allowedVirtualKeys
						: (virtualKeyId && virtualKeyId !== "__auto__" ? [virtualKeyId] : (providerVirtualKeys.length > 0 ? [providerVirtualKeys[0].value] : []))
				}
				onVirtualKeyValuesChange={
					isMemberOnly
						? undefined
						: (next) => {
								setAllowedVirtualKeys(next);
								if (next.length > 0) setVirtualKeyId(next[0]);
								else setVirtualKeyId("__auto__");
							}
				}
				apiKeyIdValue={apiKeyId}
				onApiKeyIdChange={(v) => onApiKeyIdChange(v ?? "__auto__")}
				apiKeyValues={allowedApiKeys ?? []}
				onApiKeyValuesChange={
					isMemberOnly
						? undefined
						: (next) => {
								setAllowedApiKeys(next);
								if (next.length === 1) setApiKeyId(next[0]);
								else if (!next.includes(apiKeyId)) setApiKeyId("__auto__");
							}
				}
				providerLabel={getProviderLabel(provider)}
				disabled={false}
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
											options={
												allowedProviders
													? providerOptions.filter((option) => allowedProviders.includes(option.value))
													: providerOptions
											}
											value={provider}
											onValueChange={(v) => v && onProviderChange(v)}
											placeholder="Select provider"
											searchPlaceholder="Search..."
											hideClear
										/>
									) : (
										<ComboboxSelect
											multiple
											compactTrigger
											options={providerOptions}
											value={allowedProviders ?? (provider ? [provider] : [])}
											onValueChange={(next) => {
												setAllowedProviders(next);
												if (!next.includes(provider)) onProviderChange(next[0] || "");
											}}
											placeholder="Select providers"
											searchPlaceholder="Search..."
											selectAllValue="*"
											selectAllLabel="Select all"
										/>
									)}
								</div>

								<div className="flex flex-col gap-2" data-testid="settings-model">
									<Label className="text-muted-foreground text-xs font-medium uppercase">Model</Label>
									{isMemberOnly ? (
										<ComboboxSelect
											options={
												allowedModels ? modelOptions.filter((option) => allowedModels.includes(option.value)) : modelOptions
											}
											value={model || null}
											onValueChange={(v) => onModelChange(v ?? "")}
											placeholder={!provider ? "Select a provider first" : "Select model"}
											searchPlaceholder="Search..."
											disabled={!provider}
											hideClear
											data-testid="settings-model-select"
										/>
									) : (
										<ComboboxSelect
											multiple
											compactTrigger
											options={modelOptions}
											value={allowedModels ?? (model ? [model] : [])}
											onValueChange={(next) => {
												setAllowedModels(next);
												if (!next.includes(model)) onModelChange(next[0] || "");
											}}
											placeholder={!provider ? "Select a provider first" : "Select models"}
											searchPlaceholder="Search..."
											disabled={!provider}
											selectAllValue="*"
											selectAllLabel="Select all"
											data-testid="settings-model-select"
										/>
									)}
								</div>

								{keySelector}
								{isMemberOnly && providerVirtualKeys.length === 0 && (
									<p className="text-amber-600 text-xs" data-testid="settings-no-member-key">
										No virtual key is available to you yet. Ask your admin to assign one to you, your team or your customer.
									</p>
								)}
								{selectedVirtualKey && (
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
										{(isMemberOnly && mcpIncludeTools
											? mcpToolOptions.filter((option) => mcpIncludeTools.includes(option.value))
											: mcpToolOptions
										).length === 0 ? (
											<p className="text-muted-foreground text-[11px] leading-relaxed">
												No MCP servers attached to this key. Attach servers in MCP Catalog or enable &quot;Available to all virtual keys&quot;.
											</p>
										) : (
											<ComboboxSelect
												multiple
												compactTrigger
												options={
													isMemberOnly && mcpIncludeTools
														? mcpToolOptions.filter((option) => mcpIncludeTools.includes(option.value))
														: mcpToolOptions
												}
												value={selectedMcpTools}
												onValueChange={(next) => {
													if (isMemberOnly) {
														setMcpIncludeTools(next);
														return;
													}
													setMcpIncludeTools(next.length === mcpToolValues.length ? null : next);
												}}
												placeholder="Select MCP tools"
												searchPlaceholder="Search..."
												selectAllValue="*"
												selectAllLabel="Select all"
												data-testid="settings-mcp-tools"
											/>
										)}
									</div>
								)}

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
											data-testid="settings-allowed-skills-select"
										/>
									</div>
								</div>

								{Object.keys(variables).length > 0 && (
									<>
										<Separator />
										<VariablesTableView variables={variables} onChange={setVariables} />
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
											<ModelParameters model={model} config={modelParams} onChange={handleModelParamsChange} hideFields={["promptTools"]} />
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