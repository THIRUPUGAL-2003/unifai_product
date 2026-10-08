import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { ComboboxSelect } from "@/components/ui/combobox";
import ModelParameters from "@/components/ui/custom/modelParameters";
import { Label } from "@/components/ui/label";
import { ModelMultiselect } from "@/components/ui/modelMultiselect";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { getProviderLabel } from "@/lib/constants/logs";
import { Input } from "@/components/ui/input";
import { getErrorMessage, useGetVirtualKeyBillingBlocksQuery, useGetVirtualKeysQuery } from "@/lib/store";
import { useGetAllKeysQuery, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { useListSkillsQuery } from "@/lib/store/apis/skillsApi";
import { useGetMCPClientsQuery } from "@/lib/store/apis/mcpApi";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { PanelRightClose, Wrench } from "lucide-react";
import { ModelProviderName } from "@/lib/types/config";
import { ModelParams } from "@/lib/types/prompts";
import { cn } from "@/lib/utils";
import { PromptDeploymentsAccordionItem } from "@enterprise/components/prompt-deployments/promptDeploymentsAccordionItem";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiKeySelectorView } from "../components/apiKeySelectorView";
import { VariablesTableView } from "../components/variablesTableView";
import { usePromptContext } from "../context";
import { useIsAuthEnabledQuery } from "@/lib/store";

export function SettingsPanel() {
	const {
		provider,
		setProvider,
		model,
		setModel: onModelChange,
		modelParams,
		setModelParams: onModelParamsChange,
		apiKeyId,
		setApiKeyId,
		skillId,
		setSkillId,
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

	useEffect(() => {
		setAuthPollMs(authPollError ? 0 : POLL_MS);
	}, [authPollError]);
	useEffect(() => {
		setVkPollMs(vkFailed ? 0 : POLL_MS);
	}, [vkFailed]);

	const onProviderChange = useCallback(
		(p: string) => {
			setProvider(p);
			// Keep the picked virtual key when it also serves the new provider.
			const current = (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === apiKeyId);
			if (!current?.provider_configs?.some((pc) => pc.provider === p)) {
				setApiKeyId("__auto__");
			}
			onModelChange("");
			onModelParamsChange((prev) => ({
				stream: true,
				temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
				max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
			}));
		},
		[setProvider, setApiKeyId, onModelChange, onModelParamsChange, virtualKeysData, apiKeyId],
	);

	const onApiKeyIdChange = useCallback(
		(id: string) => {
			setApiKeyId(id);
		},
		[setApiKeyId],
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
	const skillOptions = useMemo(
		() => [
			{ label: "None", value: "" },
			...(skillsData?.skills ?? []).map((s) => ({ label: s.name, value: s.id })),
		],
		[skillsData],
	);

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

	// Picking a virtual key narrows Provider to the providers configured on that key.
	const selectedVirtualKey = useMemo(
		() => (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === apiKeyId),
		[virtualKeysData, apiKeyId],
	);
	const activeMCPClients = useMemo(() => {
		if (!selectedVirtualKey) return [];
		const vkId = selectedVirtualKey.id;
		return (mcpClientsData?.clients ?? []).filter((client) => {
			if (!client.config?.client_id || client.config.disabled) return false;
			if (client.config.allow_on_all_virtual_keys) return true;
			if (client.vk_configs?.some((vc) => vc.virtual_key_id === vkId)) return true;
			if (selectedVirtualKey.mcp_configs?.some((mc) => mc.mcp_client?.name === client.config?.name)) return true;
			return false;
		});
	}, [mcpClientsData, selectedVirtualKey]);

	const totalMCPTools = useMemo(
		() => activeMCPClients.reduce((acc, c) => acc + (c.tools?.length ?? 0), 0),
		[activeMCPClients],
	);

	const selectedVKProviders = useMemo(
		() => [...new Set((selectedVirtualKey?.provider_configs ?? []).map((pc) => pc.provider))],
		[selectedVirtualKey],
	);
	useEffect(() => {
		if (selectedVKProviders.length === 0) return;
		if (provider && selectedVKProviders.includes(provider)) return;
		setProvider(selectedVKProviders[0]);
		onModelChange("");
		onModelParamsChange((prev) => ({
			stream: true,
			temperature: typeof prev?.temperature === "number" ? prev.temperature : 0.7,
			max_tokens: typeof prev?.max_tokens === "number" ? prev.max_tokens : 4096,
		}));
	}, [selectedVKProviders, provider, setProvider, onModelChange, onModelParamsChange]);

	// Ensure current provider always has a label-resolved option (even before providers query loads)
	const providerOptions = useMemo(() => {
		if (isMemberOnly) {
			return selectedVKProviders.map((p) => ({ label: getProviderLabel(p), value: p as ModelProviderName }));
		}
		const scoped =
			selectedVKProviders.length > 0 ? configuredProviders.filter((p) => selectedVKProviders.includes(p.name)) : configuredProviders;
		const opts = scoped.map((p) => ({ label: getProviderLabel(p.name), value: p.name }));
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

	// Virtual keys: show all active virtual keys so any assigned or available key can be selected,
	// driving provider and model filtering.
	const providerVirtualKeys = useMemo(() => {
		const vks = virtualKeysData?.virtual_keys ?? [];
		return vks.filter((vk) => vk.is_active !== false);
	}, [virtualKeysData]);

	// Auto-bind first assigned VK for members so usage hits the correct budget meter,
	// skipping keys whose team or customer budget is used up.
	const { data: billingBlocks, isError: billingFailed, error: billingError } = useGetVirtualKeyBillingBlocksQuery(undefined, {
		skip: !isMemberOnly,
	});
	useEffect(() => {
		if (!isMemberOnly) return;
		const assignable = providerVirtualKeys.filter((vk) => typeof vk.value === "string" && vk.value.startsWith("sk-uf-"));
		const first = assignable.find((vk) => !billingBlocks?.blocks?.[vk.id]) ?? assignable[0];
		if (!first?.value) return;
		if (apiKeyId === "__auto__" || !providerVirtualKeys.some((vk) => vk.value === apiKeyId)) {
			setApiKeyId(first.value);
		}
	}, [isMemberOnly, providerVirtualKeys, apiKeyId, setApiKeyId, billingBlocks]);

	// Separate keys/vks to pass to model fetch for filtering.
	const filterKeys = useMemo(() => {
		const isProviderKey = providerKeys.some((k) => k.key_id === apiKeyId);
		if (isProviderKey) return [apiKeyId];
		const isVirtualKey = providerVirtualKeys.some((vk) => vk.value === apiKeyId);
		if (isVirtualKey) return undefined;
		// Auto: pass all provider key IDs
		return providerKeys.map((k) => k.key_id);
	}, [apiKeyId, providerKeys, providerVirtualKeys]);

	const filterVks = useMemo(() => {
		const virtualKey = providerVirtualKeys.find((vk) => vk.value === apiKeyId);
		if (virtualKey) return [virtualKey.id];
		return undefined;
	}, [apiKeyId, providerVirtualKeys]);

	const handleModelParamsChange = useCallback(
		(params: Record<string, any>) => {
			onModelParamsChange(params as ModelParams);
		},
		[onModelParamsChange],
	);

	const hasModel = Boolean(model);

	// Key selector shows when either provider keys or virtual keys are available.
	const showKeySelector = providerKeys.length > 0 || providerVirtualKeys.length > 0;
	// Members always get the budget strip (user budget always; VK meters only when a key is assigned).
	const showMemberBudgetStrip = isMemberOnly;
	const keySelector =
		showKeySelector || showMemberBudgetStrip ? (
			<ApiKeySelectorView
				providerKeys={providerKeys}
				virtualKeys={providerVirtualKeys}
				value={apiKeyId}
				onValueChange={(v) => onApiKeyIdChange(v ?? "__auto__")}
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
									<ComboboxSelect
										options={providerOptions}
										value={provider}
										onValueChange={(v) => v && onProviderChange(v)}
										placeholder="Select provider"
										hideClear
									/>
								</div>

								<div className="flex flex-col gap-2" data-testid="settings-model">
									<Label className="text-muted-foreground text-xs font-medium uppercase">Model</Label>
									<ModelMultiselect
										provider={provider}
										keys={filterKeys && filterKeys.length > 0 ? filterKeys : undefined}
										vks={filterVks}
										value={model}
										onChange={(v) => onModelChange(v)}
										isSingleSelect
										placeholder={
											!provider ? "Select a provider first" : filterVks ? "Select a model from this key" : "Select model"
										}
										disabled={!provider}
										unfiltered={true}
									/>
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
										{activeMCPClients.length === 0 ? (
											<p className="text-muted-foreground text-[11px] leading-relaxed">
												No MCP servers attached to this key. Attach servers in MCP Catalog or enable &quot;Available to all virtual keys&quot;.
											</p>
										) : (
											<div className="flex flex-col gap-1.5 mt-1">
												{activeMCPClients.map((client) => {
													const toolCount = client.tools?.length ?? 0;
													const isConnected = client.state === "connected";
													return (
														<div
															key={client.config.client_id}
															className="flex items-center justify-between rounded bg-background/80 px-2 py-1 text-xs border border-border/40"
														>
															<div className="flex items-center gap-1.5 min-w-0">
																<span
																	className={cn(
																		"h-2 w-2 rounded-full shrink-0",
																		isConnected ? "bg-emerald-500 animate-pulse" : "bg-amber-500",
																	)}
																/>
																<span className="font-medium truncate">{client.config.name}</span>
																{client.config.allow_on_all_virtual_keys && (
																	<span className="text-[10px] text-muted-foreground font-mono">(Global)</span>
																)}
															</div>
															<span className="text-[11px] text-muted-foreground font-mono shrink-0">
																{toolCount} {toolCount === 1 ? "tool" : "tools"}
															</span>
														</div>
													);
												})}
											</div>
										)}
									</div>
								)}

								{skillOptions.length > 1 && (
									<div className="flex flex-col gap-2" data-testid="settings-skill">
										<Label className="text-muted-foreground text-xs font-medium uppercase">Skill (optional)</Label>
										<p className="text-muted-foreground text-xs">
											Injects the skill markdown as a system message for this playground run.
										</p>
										<ComboboxSelect
											options={skillOptions}
											value={skillId}
											onValueChange={(v) => setSkillId(v ?? "")}
											placeholder="Select skill"
										/>
									</div>
								)}

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