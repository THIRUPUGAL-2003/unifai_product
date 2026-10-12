import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select";
import { getErrorMessage } from "@/lib/store";
import { useGetVirtualKeyBillingBlocksQuery } from "@/lib/store/apis/governanceApi";
import { useIsAuthEnabledQuery } from "@/lib/store/apis/sessionApi";
import type { DBKey, VirtualKey } from "@/lib/types/governance";
import { AlertTriangle, KeyRound, Shield } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

export function ApiKeySelectorView({
	providerKeys,
	virtualKeys,
	virtualKeyValue,
	onVirtualKeyChange,
	apiKeyIdValue,
	onApiKeyIdChange,
	providerLabel,
	disabled,
	placeholder,
	// Backwards compatibility if legacy single value passed
	value,
	onValueChange,
}: {
	providerKeys: DBKey[];
	virtualKeys: VirtualKey[];
	virtualKeyValue?: string;
	onVirtualKeyChange?: (v: string | null) => void;
	apiKeyIdValue?: string;
	onApiKeyIdChange?: (v: string | null) => void;
	providerLabel?: string;
	disabled?: boolean;
	placeholder?: string;
	value?: string;
	onValueChange?: (v: string | null) => void;
}) {
	const POLL_MS = 5000;
	const [authPollMs, setAuthPollMs] = useState(POLL_MS);
	const [billingPollMs, setBillingPollMs] = useState(POLL_MS);
	const { data: authStatus, error: authError } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });

	const isMember = Boolean(authStatus?.role && authStatus.role !== "admin" && authStatus.role !== "super_admin");
	const isRegularUser = (authStatus?.role || "").trim().toLowerCase() === "user";

	const { data: billingBlocks, error: billingError, isError: billingFailed } = useGetVirtualKeyBillingBlocksQuery(undefined, {
		pollingInterval: isMember ? billingPollMs : 0,
		skip: !isMember,
	});

	useEffect(() => {
		setAuthPollMs(authError ? 0 : POLL_MS);
	}, [authError]);
	useEffect(() => {
		setBillingPollMs(billingError ? 0 : POLL_MS);
	}, [billingError]);
	const blocks = billingBlocks?.blocks;

	// Resolve effective Virtual Key selection
	const resolvedVkValue = useMemo(() => {
		if (virtualKeyValue !== undefined) {
			return virtualKeyValue;
		}
		if (value && value.startsWith("sk-uf-")) {
			return value;
		}
		return "__auto__";
	}, [virtualKeyValue, value]);

	// Resolve effective Provider API Key selection
	const resolvedApiKeyValue = useMemo(() => {
		if (apiKeyIdValue !== undefined) {
			return apiKeyIdValue;
		}
		if (value && !value.startsWith("sk-uf-") && value !== "__auto__") {
			return value;
		}
		return "__auto__";
	}, [apiKeyIdValue, value]);

	const handleVkChange = (v: string) => {
		if (onVirtualKeyChange) {
			onVirtualKeyChange(v);
		} else if (onValueChange) {
			onValueChange(v);
		}
	};

	const handleApiKeyChange = (v: string) => {
		if (onApiKeyIdChange) {
			onApiKeyIdChange(v);
		} else if (onValueChange) {
			onValueChange(v);
		}
	};

	const selectedVK = useMemo(() => virtualKeys.find((vk) => vk.value === resolvedVkValue), [virtualKeys, resolvedVkValue]);
	const selectedBlock = selectedVK ? blocks?.[selectedVK.id] : undefined;
	const fmt = (v?: number) => `$${(v ?? 0).toFixed(2)}`;

	const userBudget = authStatus?.budget;
	const userUsage = authStatus?.budget_current_usage ?? 0;
	// Members always see their personal budget. Users with a configured budget also see their meter.
	const showUserBudget = isMember || (userBudget !== undefined && userBudget > 0);
	const showVKBudget = Boolean(selectedVK);

	const selectVkValue = useMemo(() => {
		if (resolvedVkValue && resolvedVkValue !== "__auto__") {
			if (virtualKeys.some((vk) => vk.value === resolvedVkValue)) return resolvedVkValue;
		}
		if (resolvedVkValue === "__auto__") {
			return "__auto__";
		}
		if (isMember) {
			return virtualKeys[0]?.value ?? "__auto__";
		}
		return "__auto__";
	}, [resolvedVkValue, virtualKeys, isMember]);

	const selectApiKeyValue = useMemo(() => {
		if (resolvedApiKeyValue && resolvedApiKeyValue !== "__auto__") {
			if (providerKeys.some((k) => k.key_id === resolvedApiKeyValue)) return resolvedApiKeyValue;
		}
		return "__auto__";
	}, [resolvedApiKeyValue, providerKeys]);

	const hasVirtualKeysToShow = !isMember || virtualKeys.length > 0;
	const showProviderKeySection = !isMember || providerKeys.length > 0;

	return (
		<div className="flex flex-col gap-4">
			{isMember && billingFailed ? (
				<QueryErrorBanner
					testId="api-key-selector-billing-query-error"
					message={getErrorMessage(billingError) || "Failed to load virtual key billing blocks."}
				/>
			) : null}

			{/* 1. SEPARATE VIRTUAL KEY SECTION */}
			{hasVirtualKeysToShow && (
				<div className="flex flex-col gap-1.5">
					<div className="flex items-center gap-1.5">
						<Shield className="h-3.5 w-3.5 text-teal-500" />
						<Label className="text-muted-foreground text-xs font-semibold uppercase tracking-wider">Virtual Key</Label>
					</div>
					<p className="text-muted-foreground text-[11px] leading-snug">
						{isMember
							? isRegularUser
								? "Governance virtual key assigned to you (user or team). Routing, models, and budgets apply."
								: "Governance virtual key assigned to you (user, team, or customer). Routing, models, and budgets apply."
							: "Governance virtual keys. Routing, allowed models, budgets, and rate limits apply."}
					</p>
					<Select
						value={selectVkValue}
						onValueChange={(v) => {
							if (v && v !== "__none__") {
								handleVkChange(v);
							}
						}}
						disabled={disabled}
					>
						<SelectTrigger className="border-border bg-background h-9 w-full text-sm font-normal" data-testid={isMember ? "member-vk-select" : "admin-vk-select"}>
							<SelectValue placeholder={placeholder ?? "Select Virtual Key"} />
						</SelectTrigger>
						<SelectContent className="max-h-72">
							{!isMember && (
								<SelectItem value="__auto__">All (Auto - All Virtual Keys)</SelectItem>
							)}
							{isMember && virtualKeys.length > 1 && (
								<SelectItem value="__auto__">All (Auto - Assigned Keys)</SelectItem>
							)}
							<SelectGroup>
								<SelectLabel className="text-muted-foreground text-xs font-semibold">
									{isMember ? "Your Virtual Keys" : "Virtual Keys"}
								</SelectLabel>
								{virtualKeys.length > 0 ? (
									virtualKeys.map((vk) => {
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
										return (
											<SelectItem key={vk.value} value={vk.value}>
												{vk.name}{suffix}
											</SelectItem>
										);
									})
								) : (
									<SelectItem value="__none__" disabled>
										{isMember ? "No assigned keys" : "No active virtual keys"}
									</SelectItem>
								)}
							</SelectGroup>
						</SelectContent>
					</Select>

					{/* Every member always sees personal budget */}
					{showUserBudget && (
						<div className="bg-muted/10 border-border/40 mt-1 space-y-1 rounded-md border p-2 text-xs" data-testid="member-user-budget">
							<div className="flex items-center justify-between text-[11px]">
								<span className="text-muted-foreground font-medium">Your User Budget:</span>
								<span className="font-mono">
									{fmt(userUsage)} / {userBudget && userBudget > 0 ? fmt(userBudget) : "Unlimited"}
								</span>
							</div>
							{userBudget && userBudget > 0 ? (
								<div className="space-y-0.5">
									<div className="bg-muted/60 h-1.5 w-full overflow-hidden rounded-full">
										<div
											className={`h-full transition-all ${
												userUsage >= userBudget ? "bg-rose-500" : userUsage / userBudget > 0.8 ? "bg-amber-500" : "bg-teal-500"
											}`}
											style={{
												width: `${Math.min(100, Math.max(0, (userUsage / userBudget) * 100))}%`,
											}}
										/>
									</div>
									<div className="text-muted-foreground flex justify-between text-[10px]">
										<span>Monthly limit</span>
										<span>{Math.round((userUsage / userBudget) * 100)}% used</span>
									</div>
								</div>
							) : null}
							{userBudget && userBudget > 0 && userUsage >= userBudget && (
								<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-rose-500">
									<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
									<span>Personal budget limit reached</span>
								</div>
							)}
							{userBudget && userBudget > 0 && userUsage < userBudget && userUsage / userBudget > 0.8 && (
								<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-amber-500">
									<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
									<span>Warning: Personal budget exceeds 80%</span>
								</div>
							)}
						</div>
					)}

					{/* VK / team / customer budgets — only when a key is assigned to this user (direct / team / customer) */}
					{showVKBudget && selectedVK && (() => {
						const customerName = selectedVK.customer?.name || selectedVK.customers?.[0]?.name;
						const teamName = selectedVK.team?.name || selectedVK.teams?.[0]?.name;
						const hideOrgBudgets = isRegularUser || (authStatus?.role !== "admin" && authStatus?.role !== "super_admin");
						const scopeLabel = hideOrgBudgets
							? null
							: customerName
								? `Customer: ${customerName}`
								: teamName
									? `Team: ${teamName}`
									: null;
						// Never expose customer or team budgets to regular user accounts (only show VK budget and personal user budget)
						const customerBudgets = hideOrgBudgets ? [] : (selectedVK.customer?.budgets || selectedVK.customers?.[0]?.budgets || []);
						const teamBudgets = hideOrgBudgets ? [] : (selectedVK.team?.budgets || selectedVK.teams?.[0]?.budgets || []);
						const vkBudgets = selectedVK.budgets || [];

						return (
							<div className="bg-muted/20 border-border/50 mt-1 space-y-2 rounded-md border p-2 text-xs" data-testid="member-vk-budget">
								<div className="flex items-center justify-between">
									<span className="text-muted-foreground flex items-center gap-1.5 font-medium">
										<span className="inline-block h-1.5 w-1.5 rounded-full bg-teal-500" />
										VK: {selectedVK.name}
									</span>
									{scopeLabel && (
										<span className="rounded border border-teal-500/20 bg-teal-500/10 px-1.5 py-0.5 text-[10px] font-medium text-teal-600 dark:text-teal-400">
											{scopeLabel}
										</span>
									)}
								</div>

								{vkBudgets.length > 0 ? (
									vkBudgets.map((b) => (
										<div key={`vk-${b.id || b.reset_duration}`} className="space-y-0.5">
											<div className="flex items-center justify-between text-[11px]">
												<span className="text-muted-foreground font-medium">Key Budget:</span>
												<span className="font-mono">
													{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}
												</span>
											</div>
											{b.max_limit > 0 && (
												<div className="space-y-0.5">
													<div className="bg-muted/60 h-1.5 w-full overflow-hidden rounded-full">
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
														<span>Reset: {b.reset_duration || "monthly"}</span>
														<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
													</div>
												</div>
											)}
										</div>
									))
								) : customerBudgets.length === 0 && teamBudgets.length === 0 ? (
									<div className="flex items-center justify-between text-[11px]">
										<span className="text-muted-foreground font-medium">Key Budget:</span>
										<span className="text-muted-foreground">Unlimited</span>
									</div>
								) : null}

								{customerBudgets.map((b) => (
									<div key={`cust-${b.id || b.reset_duration}`} className="border-border/40 space-y-0.5 border-t pt-1">
										<div className="flex items-center justify-between text-[11px]">
											<span className="text-muted-foreground font-medium">Customer Budget ({customerName}):</span>
											<span className="font-mono">
												{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}
											</span>
										</div>
										{b.max_limit > 0 && (
											<div className="space-y-0.5">
												<div className="bg-muted/60 h-1.5 w-full overflow-hidden rounded-full">
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
													<span>Reset: {b.reset_duration || "monthly"}</span>
													<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
												</div>
											</div>
										)}
									</div>
								))}

								{teamBudgets.map((b) => (
									<div key={`team-${b.id || b.reset_duration}`} className="border-border/40 space-y-0.5 border-t pt-1">
										<div className="flex items-center justify-between text-[11px]">
											<span className="text-muted-foreground font-medium">Team Budget ({teamName}):</span>
											<span className="font-mono">
												{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}
											</span>
										</div>
										{b.max_limit > 0 && (
											<div className="space-y-0.5">
												<div className="bg-muted/60 h-1.5 w-full overflow-hidden rounded-full">
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
													<span>Reset: {b.reset_duration || "monthly"}</span>
													<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
												</div>
											</div>
										)}
									</div>
								))}
								{selectedVK.is_active === false && (
									<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-rose-500">
										<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
										<span>Virtual Key is inactive — prompt executions will be rejected</span>
									</div>
								)}
								{selectedVK.expires_at && new Date(selectedVK.expires_at).getTime() < Date.now() && (
									<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-rose-500">
										<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
										<span>Virtual Key has expired — prompt executions will be rejected</span>
									</div>
								)}
								{selectedVK.budgets?.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit) && (
									<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-rose-500">
										<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
										<span>Virtual Key budget limit reached — calls will be blocked</span>
									</div>
								)}
								{selectedBlock && (
									<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-rose-500" data-testid="vk-billing-block">
										<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
										<span>
											{selectedBlock.scope === "vk" ? "Virtual key" : selectedBlock.scope === "team" ? "Team" : "Customer"} budget for
											&quot;{selectedBlock.name}&quot; is used up — this key is blocked
											{selectedBlock.scope === "vk" ? " until an admin resets it" : ". Pick another key"}.
										</span>
									</div>
								)}
								{selectedVK.budgets?.some(
									(b) => b.max_limit > 0 && (b.current_usage ?? 0) < b.max_limit && (b.current_usage ?? 0) / b.max_limit > 0.8,
								) && (
									<div className="flex items-center gap-1.5 pt-1 text-[11px] font-medium text-amber-500">
										<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
										<span>Warning: Virtual Key budget exceeds 80%</span>
									</div>
								)}
							</div>
						);
					})()}
				</div>
			)}

			{/* 2. SEPARATE PROVIDER API KEY SECTION */}
			{showProviderKeySection && (
				<div className="flex flex-col gap-1.5 pt-1 border-t border-border/50">
					<div className="flex items-center gap-1.5">
						<KeyRound className="h-3.5 w-3.5 text-blue-500" />
						<Label className="text-muted-foreground text-xs font-semibold uppercase tracking-wider">API Key</Label>
					</div>
					<p className="text-muted-foreground text-[11px] leading-snug">
						Direct provider API key configured for {providerLabel || "the selected provider"}.
					</p>
					<Select
						value={selectApiKeyValue}
						onValueChange={(v) => {
							if (v && v !== "__none__") {
								handleApiKeyChange(v);
							}
						}}
						disabled={disabled}
					>
						<SelectTrigger className="border-border bg-background h-9 w-full text-sm font-normal" data-testid="admin-provider-key-select">
							<SelectValue placeholder="Select API Key" />
						</SelectTrigger>
						<SelectContent className="max-h-72">
							<SelectItem value="__auto__">Auto (Default Provider Key)</SelectItem>
							{providerKeys.length > 0 && (
								<SelectGroup>
									<SelectLabel className="text-muted-foreground text-xs font-semibold">Configured Provider Keys</SelectLabel>
									{providerKeys.map((k) => (
										<SelectItem key={k.key_id} value={k.key_id}>
											{k.name}
										</SelectItem>
									))}
								</SelectGroup>
							)}
						</SelectContent>
					</Select>
				</div>
			)}
		</div>
	);
}