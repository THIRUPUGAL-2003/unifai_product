import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { ComboboxSelect } from "@/components/ui/combobox";
import ModelParameters from "@/components/ui/custom/modelParameters";
import { Label } from "@/components/ui/label";
import { ModelMultiselect } from "@/components/ui/modelMultiselect";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { getProviderLabel } from "@/lib/constants/logs";
import { Input } from "@/components/ui/input";
import { useGetVirtualKeysQuery } from "@/lib/store";
import { useGetAllKeysQuery, useGetProvidersQuery } from "@/lib/store/apis/providersApi";
import { useListSkillsQuery } from "@/lib/store/apis/skillsApi";
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
		selectedPrompt,
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
	} = usePromptContext();

	const { data: authStatus } = useIsAuthEnabledQuery();
	const isMemberOnly = Boolean(authStatus?.role && authStatus.role !== "admin");

	const onProviderChange = useCallback(
		(p: string) => {
			setProvider(p);
			setApiKeyId("__auto__");
			onModelChange("");
			onModelParamsChange({} as ModelParams);
		},
		[setProvider, setApiKeyId, onModelChange, onModelParamsChange],
	);

	const onApiKeyIdChange = useCallback(
		(id: string) => {
			setApiKeyId(id);
		},
		[setApiKeyId],
	);
	// Dynamic providers
	const { data: providers, isLoading: isLoadingProviders } = useGetProvidersQuery();
	const { data: virtualKeysData } = useGetVirtualKeysQuery();
	// Keys for the API Key selector (from /api/keys endpoint, provider-filtered)
	const { data: allKeys, isSuccess: hasLoadedAllKeys } = useGetAllKeysQuery();
	const { data: skillsData } = useListSkillsQuery({ limit: 200, offset: 0 });
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

	// Ensure current provider always has a label-resolved option (even before providers query loads)
	const providerOptions = useMemo(() => {
		const opts = configuredProviders.map((p) => ({ label: getProviderLabel(p.name), value: p.name }));
		if (provider && !opts.find((o) => o.value === provider)) {
			opts.unshift({ label: getProviderLabel(provider), value: provider as ModelProviderName });
		}
		return opts;
	}, [configuredProviders, provider]);

	const providerKeys = useMemo(() => {
		// Members must use assigned Virtual Keys only — never raw provider keys.
		if (isMemberOnly) return [];
		return (allKeys ?? []).filter((k) => k.provider === provider);
	}, [allKeys, provider, isMemberOnly]);

	// Virtual keys filtered by selected provider (align with backend deny-by-default:
	// empty provider_configs means no providers, not all providers).
	const providerVirtualKeys = useMemo(() => {
		const vks = virtualKeysData?.virtual_keys ?? [];
		return vks.filter((vk) => {
			if (vk.is_active === false) return false;
			// Members already receive only assigned VKs from API — show all of them.
			if (isMemberOnly) return true;
			if (!vk.provider_configs || vk.provider_configs.length === 0) return false;
			return vk.provider_configs.some((pc) => pc.provider === provider);
		});
	}, [virtualKeysData, provider, isMemberOnly]);

	// Auto-bind first assigned VK for members so usage hits the correct budget meter.
	useEffect(() => {
		if (!isMemberOnly) return;
		const first = providerVirtualKeys.find((vk) => typeof vk.value === "string" && vk.value.startsWith("sk-uf-"));
		if (!first?.value) return;
		if (apiKeyId === "__auto__" || !providerVirtualKeys.some((vk) => vk.value === apiKeyId)) {
			setApiKeyId(first.value);
		}
	}, [isMemberOnly, providerVirtualKeys, apiKeyId, setApiKeyId]);

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
								{isMemberOnly && selectedPrompt?.latest_version ? (
									<div className="space-y-3 rounded-lg border bg-muted/40 p-3" data-testid="settings-locked-model">
										<div className="flex flex-col gap-1">
											<Label className="text-muted-foreground text-xs font-medium uppercase">Assigned Model</Label>
											<div className="font-semibold text-sm">
												{String(selectedPrompt.latest_version.provider || provider).toUpperCase()} — {selectedPrompt.latest_version.model || model}
											</div>
										</div>
										<p className="text-xs text-muted-foreground">
											This prompt is locked to its committed version model for members.
										</p>
									</div>
								) : (
									<>
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
												placeholder={!provider ? "Select a provider first" : "Select model"}
												disabled={!provider}
												unfiltered={true}
											/>
										</div>
									</>
								)}

								{(providerKeys.length > 0 || providerVirtualKeys.length > 0) && !!provider && (
									<ApiKeySelectorView
										providerKeys={providerKeys}
										virtualKeys={providerVirtualKeys}
										value={apiKeyId}
										onValueChange={(v) => onApiKeyIdChange(v ?? "__auto__")}
										disabled={!provider}
									/>
								)}
								{!!provider && (
									<p className="text-muted-foreground text-xs">
										MCP tools use the Virtual Key you pick here (Bearer <code className="text-[10px]">sk-uf-…</code>
										). Install servers with &quot;Available to all virtual keys&quot; on, or attach this key in MCP Catalog.
										Server must show <strong>connected</strong> with discovered tools.
									</p>
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

								{hasModel && (
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
					{selectedPromptId && <PromptDeploymentsAccordionItem activeSection={openSection} />}
				</Accordion>
			</div>
		</div>
	);
}