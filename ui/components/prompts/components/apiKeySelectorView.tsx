import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { ComboboxSelect } from "@/components/ui/combobox";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getErrorMessage } from "@/lib/store";
import { useGetProviderGovernanceQuery, useGetVirtualKeyBillingBlocksQuery } from "@/lib/store/apis/governanceApi";
import { useIsAuthEnabledQuery } from "@/lib/store/apis/sessionApi";
import type { Budget, DBKey, VirtualKey } from "@/lib/types/governance";
import { AlertTriangle, KeyRound, RotateCw, Shield } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

export function ApiKeySelectorView({
	providerKeys,
	virtualKeys,
	virtualKeyValue,
	onVirtualKeyChange,
	virtualKeyValues,
	onVirtualKeyValuesChange,
	apiKeyIdValue,
	onApiKeyIdChange,
	apiKeyValues,
	onApiKeyValuesChange,
	provider,
	model,
	providerLabel,
	disabled,
	targetUserBudget,
	targetUserUsage,
	targetUserLabel,
	placeholder,
	onRefreshBudgets,
	isRefreshingBudgets,
	// Backwards compatibility if legacy single value passed
	value,
	onValueChange,
}: {
	providerKeys: DBKey[];
	virtualKeys: VirtualKey[];
	virtualKeyValue?: string;
	onVirtualKeyChange?: (v: string | null) => void;
	virtualKeyValues?: string[];
	onVirtualKeyValuesChange?: (values: string[]) => void;
	apiKeyIdValue?: string;
	onApiKeyIdChange?: (v: string | null) => void;
	apiKeyValues?: string[];
	onApiKeyValuesChange?: (values: string[]) => void;
	provider?: string;
	model?: string;
	providerLabel?: string;
	disabled?: boolean;
	targetUserBudget?: number;
	targetUserUsage?: number;
	targetUserLabel?: string;
	placeholder?: string;
	onRefreshBudgets?: () => void;
	isRefreshingBudgets?: boolean;
	value?: string;
	onValueChange?: (v: string | null) => void;
}) {
	const POLL_MS = 5000;
	const [authPollMs, setAuthPollMs] = useState(POLL_MS);
	const [billingPollMs, setBillingPollMs] = useState(POLL_MS);
	const [provGovPollMs, setProvGovPollMs] = useState(POLL_MS);
	const { data: authStatus, error: authError } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });

	const isMember = Boolean(authStatus?.role && authStatus.role !== "admin" && authStatus.role !== "super_admin");
	const isRegularUser = (authStatus?.role || "").trim().toLowerCase() === "user";

	const { data: billingBlocks, error: billingError, isError: billingFailed } = useGetVirtualKeyBillingBlocksQuery(undefined, {
		pollingInterval: isMember ? billingPollMs : 0,
		skip: !isMember,
	});

	const { data: providerGovData, error: providerGovError } = useGetProviderGovernanceQuery({ fromMemory: true }, {
		pollingInterval: provGovPollMs,
	});

	useEffect(() => {
		setAuthPollMs(authError ? 0 : POLL_MS);
	}, [authError]);
	useEffect(() => {
		setBillingPollMs(billingError ? 0 : POLL_MS);
	}, [billingError]);
	useEffect(() => {
		setProvGovPollMs(providerGovError ? 0 : POLL_MS);
	}, [providerGovError]);
	const blocks = billingBlocks?.blocks;

	// Resolve effective Virtual Key selection
	const resolvedVkValue = useMemo(() => {
		if (virtualKeyValue !== undefined) {
			return virtualKeyValue;
		}
		if (value && value.startsWith("sk-uf-")) {
			return value;
		}
		return "";
	}, [virtualKeyValue, value]);

	// Resolve effective Provider API Key selection
	const resolvedApiKeyValue = useMemo(() => {
		if (apiKeyIdValue !== undefined) {
			return apiKeyIdValue;
		}
		if (value && !value.startsWith("sk-uf-") && value !== "__auto__") {
			return value;
		}
		return "";
	}, [apiKeyIdValue, value]);

	const [userPreferredMode, setUserPreferredMode] = useState<"vk" | "api" | null>(null);

	const handleVkChange = (v: string) => {
		if (isMember && v && v !== "__auto__" && v !== "__none__") {
			setUserPreferredMode("vk");
			if (onApiKeyIdChange) onApiKeyIdChange("");
			if (onApiKeyValuesChange) onApiKeyValuesChange([]);
		}
		if (onVirtualKeyChange) {
			onVirtualKeyChange(v);
		} else if (onValueChange) {
			onValueChange(v);
		}
	};

	const handleApiKeyChange = (v: string) => {
		if (isMember && v && v !== "__auto__" && v !== "__none__") {
			setUserPreferredMode("api");
			if (onVirtualKeyChange) onVirtualKeyChange("__none__");
			if (onVirtualKeyValuesChange) onVirtualKeyValuesChange([]);
		}
		if (onApiKeyIdChange) {
			onApiKeyIdChange(v);
		} else if (onValueChange) {
			onValueChange(v);
		}
	};

	const vkOptions = useMemo(() => {
		const opts: { label: string; value: string }[] = [];
		for (const vk of virtualKeys) {
			const isInactive = vk.is_active === false;
			const isExpired = vk.expires_at ? new Date(vk.expires_at).getTime() < Date.now() : false;
			const block = blocks?.[vk.id];
			let suffix = "";
			if (isInactive) suffix = " (Inactive)";
			else if (isExpired) suffix = " (Expired)";
			else if (block) {
				const scopeLabel = block.scope === "vk" ? "Key" : block.scope === "team" ? "Team" : isRegularUser ? "Key" : "Customer";
				suffix = ` (${scopeLabel} budget used up)`;
			} else if (vk.budgets?.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit)) {
				suffix = " (Key budget used up)";
			}
			const custName = vk.customer?.name || (vk.customers && vk.customers.length > 0 ? vk.customers[0].name : "");
			const prefix = custName ? `[${custName}] ` : "";
			opts.push({ label: `${prefix}${vk.name}${suffix}`, value: vk.value });
		}
		return opts;
	}, [virtualKeys, blocks, isRegularUser]);

	const effectiveVkValues = useMemo(() => {
		const resolveToOptionValue = (raw: string) => {
			if (vkOptions.some((option) => option.value === raw)) return raw;
			const byId = virtualKeys.find((vk) => vk.id === raw);
			if (byId && vkOptions.some((option) => option.value === byId.value)) return byId.value;
			return "";
		};
		if (virtualKeyValues !== undefined) {
			const matched = virtualKeyValues.map(resolveToOptionValue).filter(Boolean);
			if (matched.length > 0) return matched;
		}
		if (resolvedVkValue && resolvedVkValue !== "__auto__") {
			const resolved = resolveToOptionValue(resolvedVkValue);
			if (resolved) return [resolved];
		}
		// Never pretend the first key is selected — that caused "1 selected" / budget UI
		// while execution still sent __auto__ and got 401.
		return [];
	}, [virtualKeyValues, vkOptions, resolvedVkValue, virtualKeys]);

	const selectVkValue = useMemo(() => {
		if (resolvedVkValue && resolvedVkValue !== "__auto__" && resolvedVkValue !== "__none__") {
			const found = virtualKeys.find((vk) => vk.value === resolvedVkValue || vk.id === resolvedVkValue);
			if (found) return found.value;
		}
		return "";
	}, [resolvedVkValue, virtualKeys]);

	const selectedVK = useMemo(() => {
		if (resolvedVkValue && resolvedVkValue !== "__auto__" && resolvedVkValue !== "__none__") {
			const found = virtualKeys.find((vk) => vk.value === resolvedVkValue || vk.id === resolvedVkValue);
			if (found) return found;
		}
		if (effectiveVkValues.length > 0) {
			const targetVal = effectiveVkValues[0];
			const found = virtualKeys.find((vk) => vk.value === targetVal || vk.id === targetVal);
			if (found) return found;
		}
		if (virtualKeys.length === 1 && (selectVkValue || effectiveVkValues.length > 0)) {
			return virtualKeys[0];
		}
		return undefined;
	}, [virtualKeys, resolvedVkValue, effectiveVkValues, selectVkValue]);

	const selectedBlock = selectedVK ? blocks?.[selectedVK.id] : undefined;
	const fmt = (v?: number) => {
		const n = v ?? 0;
		if (n === 0) return "$0.00";
		if (n < 0.001) return `$${n.toFixed(6)}`;
		if (n < 1000) return `$${n.toFixed(4)}`;
		if (n >= 1_000_000) return `$${n.toLocaleString(undefined, { maximumFractionDigits: 2 })}`;
		return `$${n.toFixed(2)}`;
	};
	const fmtLimit = (v?: number) => {
		const n = v ?? 0;
		if (n <= 0) return "Unlimited";
		if (n >= 1_000_000_000) return `$${(n / 1_000_000_000).toFixed(0)}B`;
		if (n >= 1_000_000) return `$${(n / 1_000_000).toFixed(0)}M`;
		if (n >= 1_000) return `$${n.toLocaleString(undefined, { maximumFractionDigits: 2 })}`;
		return `$${n.toFixed(2)}`;
	};
	const fmtReset = (d?: string) => {
		if (!d) return "monthly";
		if (d === "1m") return "1 min (resets every min)";
		if (d === "1M") return "monthly";
		if (d === "1d") return "daily";
		if (d === "1h") return "hourly";
		if (d === "1w") return "weekly";
		return d;
	};

	const effectiveUserBudget = targetUserBudget !== undefined ? targetUserBudget : authStatus?.budget;
	const effectiveUserUsage = targetUserUsage !== undefined ? targetUserUsage : (authStatus?.budget_current_usage ?? 0);
	const cleanUserDisplayName = useMemo(() => {
		if (targetUserLabel) {
			const stripped = targetUserLabel
				.replace(/^\d+\.\s*/, "")
				.replace(/^User Budget\s*/i, "")
				.replace(/^\(/, "")
				.replace(/\)$/, "")
				.trim();
			if (stripped) return stripped;
		}
		if (authStatus?.username) return authStatus.username;
		if (authStatus?.email) return authStatus.email;
		return "";
	}, [targetUserLabel, authStatus]);

	const showUserBudget = isMember || (effectiveUserBudget !== undefined && effectiveUserBudget > 0) || Boolean(targetUserLabel);

	const selectApiKeyValue = useMemo(() => {
		if (resolvedApiKeyValue && resolvedApiKeyValue !== "__auto__" && resolvedApiKeyValue !== "__none__") {
			if (providerKeys.some((k) => k.key_id === resolvedApiKeyValue)) return resolvedApiKeyValue;
		}
		return "";
	}, [resolvedApiKeyValue, providerKeys]);

	const hasSelectedVk = Boolean(selectVkValue || (effectiveVkValues && effectiveVkValues.length > 0));
	const showVKBudget = Boolean(selectedVK) && hasSelectedVk;

	// Target provider resolution
	const targetProvider = useMemo(() => {
		const raw = (provider || providerLabel || "").trim().toLowerCase();
		if (raw) return raw;
		const selectedKey = providerKeys.find((k) => k.key_id === (selectApiKeyValue || resolvedApiKeyValue));
		return (selectedKey?.provider || "").trim().toLowerCase();
	}, [provider, providerLabel, providerKeys, selectApiKeyValue, resolvedApiKeyValue]);

	const activeProviderGov = useMemo(() => {
		if (!targetProvider) return undefined;
		return (providerGovData?.providers ?? []).find(
			(p) => p.provider?.trim().toLowerCase() === targetProvider,
		);
	}, [providerGovData, targetProvider]);

	const entityBudgetGroups = useMemo(() => {
		if (!selectedVK) return [] as { id: string; label: string; budgets: Budget[] }[];
		const groups = new Map<string, { id: string; label: string; budgets: Budget[] }>();
		const teams = new Map<string, NonNullable<VirtualKey["team"]>>();
		for (const team of [...(selectedVK.teams ?? []), ...(selectedVK.team ? [selectedVK.team] : [])]) teams.set(team.id, team);
		const customers = new Map<string, NonNullable<VirtualKey["customer"]>>();
		for (const customer of [...(selectedVK.customers ?? []), ...(selectedVK.customer ? [selectedVK.customer] : [])]) customers.set(customer.id, customer);
		for (const team of teams.values()) {
			if (team.budgets?.length) groups.set(`team:${team.id}`, { id: `team:${team.id}`, label: `Team Budget (${team.name})`, budgets: team.budgets });
			if (team.customer) customers.set(team.customer.id, team.customer);
		}
		for (const customer of customers.values()) {
			if (customer.budgets?.length) groups.set(`customer:${customer.id}`, { id: `customer:${customer.id}`, label: `Customer Budget (${customer.name})`, budgets: customer.budgets });
		}
		for (const unit of selectedVK.business_units ?? []) {
			const budget = unit.budget;
			const maxLimit = Number(budget?.max_limit ?? 0);
			if (maxLimit > 0) {
				groups.set(`business-unit:${unit.id}`, {
					id: `business-unit:${unit.id}`,
					label: `Business Unit Budget (${unit.name})`,
					budgets: [{
						id: `business-unit:${unit.id}`,
						max_limit: maxLimit,
						current_usage: Number(budget?.current_usage ?? 0),
						reset_duration: budget?.reset_duration || "1M",
						last_reset: budget?.last_reset || "",
					}],
				});
			}
		}
		const matchingConfigs = (selectedVK.provider_configs ?? []).filter((config) => {
			const configProvider = config.provider.trim().toLowerCase();
			return config.budgets?.length && (configProvider === targetProvider || configProvider === "*");
		});
		for (const config of matchingConfigs) {
			const configProvider = config.provider === "*" ? (providerLabel || "all providers") : (providerLabel || config.provider);
			groups.set(`vk-provider:${config.provider}`, {
				id: `vk-provider:${config.provider}`,
				label: `Key Provider Budget (${selectedVK.name} · ${configProvider})`,
				budgets: config.budgets ?? [],
			});
		}
		return [...groups.values()];
	}, [selectedVK, targetProvider, providerLabel]);

	const providerBudgets = useMemo(() => {
		if (!activeProviderGov) return [];
		if (activeProviderGov.budgets && activeProviderGov.budgets.length > 0) {
			return activeProviderGov.budgets;
		}
		if (activeProviderGov.budget) {
			return [activeProviderGov.budget];
		}
		return [];
	}, [activeProviderGov]);

	const hasSelectedApiKey = Boolean(selectApiKeyValue || (apiKeyValues && apiKeyValues.length > 0));
	const showProviderBudget = Boolean(targetProvider && (hasSelectedApiKey || hasSelectedVk));

	const hasVirtualKeysToShow = !isMember || virtualKeys.length > 0;
	const showProviderKeySection = !isMember || providerKeys.length > 0;

	const apiKeyOptions = useMemo(
		() =>
			providerKeys.map((key) => {
				const prov = key.provider ? `${key.provider} --- ` : "";
				return { label: `${prov}${key.name}`, value: key.key_id };
			}),
		[providerKeys],
	);

	const activeMode = useMemo<"vk" | "api">(() => {
		if (userPreferredMode) return userPreferredMode;
		if (resolvedApiKeyValue && resolvedApiKeyValue !== "__auto__" && resolvedApiKeyValue !== "__none__") {
			return "api";
		}
		if (resolvedVkValue && resolvedVkValue !== "__auto__" && resolvedVkValue !== "__none__") {
			return "vk";
		}
		if (virtualKeys.length > 0) return "vk";
		if (providerKeys.length > 0) return "api";
		return "vk";
	}, [userPreferredMode, resolvedApiKeyValue, resolvedVkValue, virtualKeys, providerKeys]);

	const switchToVk = () => {
		setUserPreferredMode("vk");
		handleApiKeyChange("");
		if (onApiKeyValuesChange) onApiKeyValuesChange([]);
		if (!selectVkValue && virtualKeys.length > 0) {
			handleVkChange(virtualKeys[0].value);
			if (onVirtualKeyValuesChange) onVirtualKeyValuesChange([virtualKeys[0].value]);
		}
	};

	const switchToApi = () => {
		setUserPreferredMode("api");
		handleVkChange("__none__");
		if (onVirtualKeyValuesChange) onVirtualKeyValuesChange([]);
		if (!selectApiKeyValue && providerKeys.length > 0) {
			handleApiKeyChange(providerKeys[0].key_id);
			if (onApiKeyValuesChange) onApiKeyValuesChange([providerKeys[0].key_id]);
		}
	};

	return (
		<div className="flex flex-col gap-4">
			{isMember && billingFailed ? (
				<QueryErrorBanner
					testId="api-key-selector-billing-query-error"
					message={getErrorMessage(billingError) || "Failed to load virtual key billing blocks."}
				/>
			) : null}

			{/* Mode Switcher: Shown strictly on User login side (isMember) when both keys are available */}
			{isMember && hasVirtualKeysToShow && showProviderKeySection && (
				<div className="flex items-center gap-1 p-1 bg-muted/60 border border-border/50 rounded-lg text-xs" data-testid="key-mode-switcher">
					<button
						type="button"
						onClick={switchToApi}
						disabled={disabled || providerKeys.length === 0}
						className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 px-3 rounded-md font-medium transition-all ${
							activeMode === "api"
								? "bg-background text-foreground shadow-sm border border-border/40 font-semibold"
								: "text-muted-foreground hover:text-foreground opacity-75"
						}`}
					>
						<KeyRound className={`h-3.5 w-3.5 ${activeMode === "api" ? "text-blue-500" : "text-muted-foreground"}`} />
						<span>Direct API Key</span>
						<span className={`text-[10px] px-1.5 py-0.2 rounded font-bold ${activeMode === "api" ? "bg-blue-500/15 text-blue-600 dark:text-blue-400" : "bg-muted text-muted-foreground"}`}>
							{activeMode === "api" ? "ON" : "OFF"}
						</span>
					</button>
					<button
						type="button"
						onClick={switchToVk}
						disabled={disabled || virtualKeys.length === 0}
						className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 px-3 rounded-md font-medium transition-all ${
							activeMode === "vk"
								? "bg-background text-foreground shadow-sm border border-border/40 font-semibold"
								: "text-muted-foreground hover:text-foreground opacity-75"
						}`}
					>
						<Shield className={`h-3.5 w-3.5 ${activeMode === "vk" ? "text-teal-500" : "text-muted-foreground"}`} />
						<span>Virtual Key</span>
						<span className={`text-[10px] px-1.5 py-0.2 rounded font-bold ${activeMode === "vk" ? "bg-teal-500/15 text-teal-600 dark:text-teal-400" : "bg-muted text-muted-foreground"}`}>
							{activeMode === "vk" ? "ON" : "OFF"}
						</span>
					</button>
				</div>
			)}

			{/* 1. DIRECT API KEY SECTION (Item 3 in Sidebar Order: Provider -> Model -> API Key) */}
			{showProviderKeySection && (
				<div className={`flex flex-col gap-1.5 transition-opacity ${isMember && activeMode !== "api" ? "opacity-60" : ""}`}>
					<div className="flex items-center justify-between gap-1.5">
						<div className="flex items-center gap-1.5">
							<KeyRound className={`h-3.5 w-3.5 ${(!isMember || activeMode === "api") ? "text-blue-500" : "text-muted-foreground"}`} />
							<Label className="text-muted-foreground text-xs font-semibold uppercase tracking-wider">API Key</Label>
						</div>
						{isMember && hasVirtualKeysToShow && showProviderKeySection && (
							<span className={`text-[10px] px-1.5 py-0.5 rounded font-bold ${activeMode === "api" ? "bg-blue-500/15 text-blue-600 dark:text-blue-400" : "bg-muted text-muted-foreground border border-border/40"}`}>
								{activeMode === "api" ? "ON" : "OFF"}
							</span>
						)}
					</div>
					<p className="text-muted-foreground text-[11px] leading-snug">
						{isMember && activeMode !== "api"
							? "Direct API key is turned OFF because Virtual Key is selected."
							: `Direct provider API key configured for ${providerLabel || "the selected provider"}.`}
					</p>
					<ComboboxSelect
						multiple
						compactTrigger
						options={apiKeyOptions}
						value={
							apiKeyValues && apiKeyValues.length > 0
								? apiKeyValues.filter((value) => apiKeyOptions.some((option) => option.value === value))
								: (!isMember || activeMode === "api") && selectApiKeyValue !== "__auto__" && selectApiKeyValue !== "__none__" && selectApiKeyValue
									? [selectApiKeyValue]
									: []
						}
						onValueChange={(next) => {
							if (next.length > 0) setUserPreferredMode("api");
							if (onApiKeyValuesChange) {
								onApiKeyValuesChange(next);
							}
							if (next.length === 0) {
								handleApiKeyChange("");
							} else if (!next.includes(selectApiKeyValue)) {
								handleApiKeyChange(isMember ? next[next.length - 1] : next[0]);
							}
						}}
						disabled={disabled || (isMember && activeMode !== "api") || apiKeyOptions.length === 0}
						placeholder={isMember && activeMode !== "api" ? "OFF (Virtual Key is active)" : apiKeyOptions.length === 0 ? "No API keys" : "Select API keys"}
						searchPlaceholder="Search API keys..."
						selectAllValue="*"
						selectAllLabel="Select all"
						data-testid={isMember ? "member-provider-key-select" : "admin-provider-key-select"}
					/>

					{isMember && activeMode !== "api" && hasVirtualKeysToShow && (
						<button
							type="button"
							onClick={switchToApi}
							className="text-[11px] text-blue-600 dark:text-blue-400 hover:underline text-left self-start mt-0.5"
						>
							↳ Click here to turn ON Direct API Key
						</button>
					)}

					{/* PROVIDER BUDGET CARD — Directly below API Key */}
					{showProviderBudget && (
						<div className="bg-muted/20 border-border/50 mt-1 space-y-1.5 rounded-md border p-2 text-xs" data-testid="provider-budget-card">
							{providerBudgets.length > 0 ? (
								providerBudgets.map((b) => (
									<div key={`prov-${b.id || b.reset_duration}`} className="space-y-0.5">
										<div className="flex items-center justify-between text-[11px]">
											<span className="text-muted-foreground font-medium flex items-center gap-1.5">
												<span className="inline-block h-1.5 w-1.5 rounded-full bg-blue-500" />
												Provider Budget ({providerLabel || (targetProvider.charAt(0).toUpperCase() + targetProvider.slice(1))}):
											</span>
											<div className="flex items-center gap-1.5">
												<span className="font-mono">
													{fmt(b.current_usage)} / {fmtLimit(b.max_limit)}
												</span>
												{onRefreshBudgets && (
													<button
														type="button"
														onClick={onRefreshBudgets}
														disabled={isRefreshingBudgets}
														className="text-muted-foreground hover:text-foreground transition-colors p-0.5 rounded"
														title="Refresh live budget"
													>
														<RotateCw className={`h-3 w-3 ${isRefreshingBudgets ? "animate-spin text-primary" : ""}`} />
													</button>
												)}
											</div>
										</div>
										{b.max_limit > 0 && (
											<div className="space-y-0.5">
												<div className="bg-muted/60 h-1 w-full overflow-hidden rounded-full">
													<div
														className={`h-full transition-all ${
															(b.current_usage ?? 0) >= b.max_limit
																? "bg-rose-500"
																: (b.current_usage ?? 0) / b.max_limit > 0.8
																	? "bg-amber-500"
																	: "bg-blue-500"
														}`}
														style={{
															width: `${Math.min(100, Math.max(0, ((b.current_usage ?? 0) / b.max_limit) * 100))}%`,
														}}
													/>
												</div>
												<div className="text-muted-foreground flex justify-between text-[10px]">
													<span>Reset: {fmtReset(b.reset_duration)}</span>
													<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
												</div>
											</div>
										)}
									</div>
								))
							) : (
								<div className="flex items-center justify-between text-[11px]">
									<span className="text-muted-foreground font-medium flex items-center gap-1.5">
										<span className="inline-block h-1.5 w-1.5 rounded-full bg-blue-500" />
										Provider Budget ({providerLabel || (targetProvider.charAt(0).toUpperCase() + targetProvider.slice(1))}):
									</span>
									<div className="flex items-center gap-1.5">
										<span className="text-muted-foreground">Unlimited</span>
										{onRefreshBudgets && (
											<button
												type="button"
												onClick={onRefreshBudgets}
												disabled={isRefreshingBudgets}
												className="text-muted-foreground hover:text-foreground transition-colors p-0.5 rounded"
												title="Refresh live budget"
											>
												<RotateCw className={`h-3 w-3 ${isRefreshingBudgets ? "animate-spin text-primary" : ""}`} />
											</button>
										)}
									</div>
								</div>
							)}
						</div>
					)}

					{/* If VK section is not shown (e.g. member with API key only), show User Budget here */}
					{!hasVirtualKeysToShow && showUserBudget && (
						<div className="bg-muted/10 border-border/40 mt-1 space-y-1 rounded-md border p-2 text-xs" data-testid="member-user-budget">
							<div className="flex items-center justify-between text-[11px]">
								<span className="text-muted-foreground font-medium flex items-center gap-1.5">
									<span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
									User Budget{cleanUserDisplayName ? ` (${cleanUserDisplayName})` : ""}:
								</span>
								<div className="flex items-center gap-1.5">
									<span className="font-mono">
										{fmt(effectiveUserUsage)} / {effectiveUserBudget && effectiveUserBudget > 0 ? fmt(effectiveUserBudget) : "Unlimited"}
									</span>
									{onRefreshBudgets && (
										<button
											type="button"
											onClick={onRefreshBudgets}
											disabled={isRefreshingBudgets}
											className="text-muted-foreground hover:text-foreground transition-colors p-0.5 rounded"
											title="Refresh live budget"
										>
											<RotateCw className={`h-3 w-3 ${isRefreshingBudgets ? "animate-spin text-primary" : ""}`} />
										</button>
									)}
								</div>
							</div>
							{effectiveUserBudget && effectiveUserBudget > 0 ? (
								<div className="space-y-0.5">
									<div className="bg-muted/60 h-1 w-full overflow-hidden rounded-full">
										<div
											className={`h-full transition-all ${
												effectiveUserUsage >= effectiveUserBudget ? "bg-rose-500" : effectiveUserUsage / effectiveUserBudget > 0.8 ? "bg-amber-500" : "bg-emerald-500"
											}`}
											style={{
												width: `${Math.min(100, Math.max(0, (effectiveUserUsage / effectiveUserBudget) * 100))}%`,
											}}
										/>
									</div>
									<div className="text-muted-foreground flex justify-between text-[10px]">
										<span>Reset: monthly</span>
										<span>{Math.round((effectiveUserUsage / effectiveUserBudget) * 100)}% used</span>
									</div>
								</div>
							) : null}
						</div>
					)}
				</div>
			)}

			{/* 2. VIRTUAL KEY SECTION (Item 4 in Sidebar Order: Provider -> Model -> API Key -> VK) */}
			{hasVirtualKeysToShow && (
				<div className={`flex flex-col gap-1.5 ${showProviderKeySection ? "pt-2 border-t border-border/50" : ""} transition-opacity ${isMember && activeMode !== "vk" ? "opacity-60" : ""}`}>
					<div className="flex items-center justify-between gap-1.5">
						<div className="flex items-center gap-1.5">
							<Shield className={`h-3.5 w-3.5 ${(!isMember || activeMode === "vk") ? "text-teal-500" : "text-muted-foreground"}`} />
							<Label className="text-muted-foreground text-xs font-semibold uppercase tracking-wider">Virtual Key</Label>
						</div>
						{isMember && hasVirtualKeysToShow && showProviderKeySection && (
							<span className={`text-[10px] px-1.5 py-0.5 rounded font-bold ${activeMode === "vk" ? "bg-teal-500/15 text-teal-600 dark:text-teal-400" : "bg-muted text-muted-foreground border border-border/40"}`}>
								{activeMode === "vk" ? "ON" : "OFF"}
							</span>
						)}
					</div>
					<p className="text-muted-foreground text-[11px] leading-snug">
						{isMember && activeMode !== "vk"
							? "Virtual Key is turned OFF because Direct API Key is selected."
							: isMember
								? isRegularUser
									? "Governance virtual key assigned to you (user or team). Routing, models, and budgets apply."
									: "Governance virtual key assigned to you (user, team, or customer). Routing, models, and budgets apply."
								: "Governance virtual keys. Routing, allowed models, budgets, and rate limits apply."}
					</p>
					<ComboboxSelect
						multiple
						compactTrigger
						options={vkOptions}
						value={effectiveVkValues}
						onValueChange={(next) => {
							if (next.length > 0) setUserPreferredMode("vk");
							if (next.length === 0) {
								handleVkChange("__none__");
							} else if (!next.includes(selectVkValue)) {
								handleVkChange(isMember ? next[next.length - 1] : next[0]);
							}
							if (onVirtualKeyValuesChange) {
								onVirtualKeyValuesChange(next);
							}
						}}
						disabled={disabled || (isMember && activeMode !== "vk") || vkOptions.length === 0}
						placeholder={isMember && activeMode !== "vk" ? "OFF (Direct API Key is active)" : (placeholder ?? (vkOptions.length === 0 ? (isMember ? "No assigned keys" : "No active virtual keys") : "Select Virtual Key"))}
						searchPlaceholder="Search virtual keys..."
						selectAllValue="*"
						selectAllLabel="Select all"
						data-testid={isMember ? "member-vk-select" : "admin-vk-select"}
					/>

					{isMember && activeMode !== "vk" && showProviderKeySection && (
						<button
							type="button"
							onClick={switchToVk}
							className="text-[11px] text-teal-600 dark:text-teal-400 hover:underline text-left self-start mt-0.5"
						>
							↳ Click here to turn ON Virtual Key
						</button>
					)}

					{/* VK BUDGET CARD — Directly below Virtual Key selector */}
					{showVKBudget && selectedVK && (() => {
						const vkBudgets = selectedVK.budgets || [];
						const isKeyInactive = isMember && activeMode !== "vk";

						return (
							<div className="bg-muted/20 border-border/50 mt-1 space-y-1.5 rounded-md border p-2 text-xs" data-testid="member-vk-budget">
								<div className="flex items-center justify-between">
									<span className="text-muted-foreground flex items-center gap-1.5 font-medium">
										<span className={`inline-block h-1.5 w-1.5 rounded-full ${isKeyInactive ? "bg-muted-foreground" : "bg-teal-500"}`} />
										Key Budget ({selectedVK.name}){isKeyInactive ? " (OFF)" : ""}:
									</span>
									{onRefreshBudgets && (
										<button
											type="button"
											onClick={onRefreshBudgets}
											disabled={isRefreshingBudgets}
											className="text-muted-foreground hover:text-foreground transition-colors p-0.5 rounded"
											title="Refresh live budget"
										>
											<RotateCw className={`h-3 w-3 ${isRefreshingBudgets ? "animate-spin text-primary" : ""}`} />
										</button>
									)}
								</div>

								{vkBudgets.length > 0 ? (
									vkBudgets.map((b) => (
										<div key={`vk-${b.id || b.reset_duration}`} className="space-y-0.5">
											<div className="flex items-center justify-between text-[11px]">
												<span className="text-muted-foreground font-medium">Limit:</span>
												<span className="font-mono">
													{fmt(b.current_usage)} / {fmtLimit(b.max_limit)}
												</span>
											</div>
											{b.max_limit > 0 && (
												<div className="space-y-0.5">
													<div className="bg-muted/60 h-1 w-full overflow-hidden rounded-full">
														<div
															className={`h-full transition-all ${
																(b.current_usage ?? 0) >= b.max_limit
																	? "bg-rose-500"
																	: (b.current_usage ?? 0) / b.max_limit > 0.8
																		? "bg-amber-500"
																		: "bg-teal-500"
															}`}
															style={{
																width: `${Math.min(100, Math.max(0, ((b.current_usage ?? 0) / b.max_limit) * 100))}%`,
															}}
														/>
													</div>
													<div className="text-muted-foreground flex justify-between text-[10px]">
														<span>Reset: {fmtReset(b.reset_duration)}</span>
														<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
													</div>
												</div>
											)}
										</div>
									))
								) : (
									<div className="flex items-center justify-between text-[11px]">
										<span className="text-muted-foreground font-medium">Limit:</span>
										<span className="text-muted-foreground">Unlimited</span>
									</div>
								)}
							</div>
						);
					})()}

					{/* USER BUDGET CARD — Directly below VK Budget */}
					{showUserBudget && (
						<div className="bg-muted/10 border-border/40 mt-1 space-y-1 rounded-md border p-2 text-xs" data-testid="member-user-budget">
							<div className="flex items-center justify-between text-[11px]">
								<span className="text-muted-foreground font-medium flex items-center gap-1.5">
									<span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
									User Budget{cleanUserDisplayName ? ` (${cleanUserDisplayName})` : ""}:
								</span>
								<div className="flex items-center gap-1.5">
									<span className="font-mono">
										{fmt(effectiveUserUsage)} / {effectiveUserBudget && effectiveUserBudget > 0 ? fmt(effectiveUserBudget) : "Unlimited"}
									</span>
									{onRefreshBudgets && (
										<button
											type="button"
											onClick={onRefreshBudgets}
											disabled={isRefreshingBudgets}
											className="text-muted-foreground hover:text-foreground transition-colors p-0.5 rounded"
											title="Refresh live budget"
										>
											<RotateCw className={`h-3 w-3 ${isRefreshingBudgets ? "animate-spin text-primary" : ""}`} />
										</button>
									)}
								</div>
							</div>
							{effectiveUserBudget && effectiveUserBudget > 0 ? (
								<div className="space-y-0.5">
									<div className="bg-muted/60 h-1 w-full overflow-hidden rounded-full">
										<div
											className={`h-full transition-all ${
												effectiveUserUsage >= effectiveUserBudget ? "bg-rose-500" : effectiveUserUsage / effectiveUserBudget > 0.8 ? "bg-amber-500" : "bg-emerald-500"
											}`}
											style={{
												width: `${Math.min(100, Math.max(0, (effectiveUserUsage / effectiveUserBudget) * 100))}%`,
											}}
										/>
									</div>
									<div className="text-muted-foreground flex justify-between text-[10px]">
										<span>Reset: monthly</span>
										<span>{Math.round((effectiveUserUsage / effectiveUserBudget) * 100)}% used</span>
									</div>
								</div>
							) : null}
						</div>
					)}
				</div>
			)}

			{entityBudgetGroups.length > 0 && (
				<div className="bg-muted/10 border-border/40 space-y-2 rounded-md border p-2 text-xs" data-testid="inherited-scope-budgets">
					{entityBudgetGroups.map((group) => (
						<div key={group.id} className="space-y-1">
							<div className="flex items-center justify-between text-[11px] font-medium text-foreground">
								<span>{group.label}</span>
								{onRefreshBudgets && (
									<button type="button" onClick={onRefreshBudgets} disabled={isRefreshingBudgets} className="text-muted-foreground hover:text-foreground rounded p-0.5" title="Refresh live budget">
										<RotateCw className={`h-3 w-3 ${isRefreshingBudgets ? "animate-spin text-primary" : ""}`} />
									</button>
								)}
							</div>
							{group.budgets.map((budget) => (
								<div key={`${group.id}:${budget.id || budget.reset_duration}`} className="flex items-center justify-between text-[11px] text-muted-foreground">
									<span>Reset: {fmtReset(budget.reset_duration)}</span>
									<span className="font-mono text-foreground">{fmt(budget.current_usage)} / {fmtLimit(budget.max_limit)}</span>
								</div>
							))}
						</div>
					))}
				</div>
			)}
		</div>
	);
}
