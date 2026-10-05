import {
	Combobox,
	ComboboxContent,
	ComboboxGroup,
	ComboboxInput,
	ComboboxItem,
	ComboboxLabel,
	ComboboxList,
	ComboboxSeparator,
} from "@/components/ui/combobox";
import { Label } from "@/components/ui/label";
import { useGetVirtualKeyBillingBlocksQuery } from "@/lib/store/apis/governanceApi";
import { useIsAuthEnabledQuery } from "@/lib/store/apis/sessionApi";
import type { DBKey, VirtualKey } from "@/lib/types/governance";
import { AlertTriangle } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

export function ApiKeySelectorView({
	providerKeys,
	virtualKeys,
	value,
	onValueChange,
	disabled,
	placeholder,
}: {
	providerKeys: DBKey[];
	virtualKeys: VirtualKey[];
	value: string;
	onValueChange: (v: string | null) => void;
	disabled?: boolean;
	placeholder?: string;
}) {
	const [query, setQuery] = useState("");
	const { data: authStatus } = useIsAuthEnabledQuery(undefined, { pollingInterval: 5000 });
	const { data: billingBlocks } = useGetVirtualKeyBillingBlocksQuery(undefined, { pollingInterval: 5000 });
	const blocks = billingBlocks?.blocks;

	const allOptions = useMemo(() => {
		const now = Date.now();
		const apiKeyOpts = providerKeys.map((k) => ({ label: k.name, value: k.key_id, group: "api" as const }));
		const vkOpts = virtualKeys.map((vk) => {
			const isInactive = vk.is_active === false;
			const isExpired = vk.expires_at ? new Date(vk.expires_at).getTime() < now : false;
			const block = blocks?.[vk.id];
			let suffix = "";
			if (isInactive) suffix = " (Inactive)";
			else if (isExpired) suffix = " (Expired)";
			else if (block) suffix = ` (${block.scope === "team" ? "Team" : "Customer"} budget used up)`;
			return { label: `${vk.name}${suffix}`, value: vk.value, group: "virtual" as const };
		});
		return [{ label: "Auto (default)", value: "__auto__", group: "api" as const }, ...apiKeyOpts, ...vkOpts];
	}, [providerKeys, virtualKeys, blocks]);

	const filtered = useMemo(() => {
		if (!query) return allOptions;
		const q = query.toLowerCase();
		return allOptions.filter((o) => o.label.toLowerCase().includes(q));
	}, [allOptions, query]);

	const filteredApiKeys = useMemo(() => filtered.filter((o) => o.group === "api"), [filtered]);
	const filteredVirtualKeys = useMemo(() => filtered.filter((o) => o.group === "virtual"), [filtered]);

	const getLabel = useCallback((val: string | null) => allOptions.find((o) => o.value === val)?.label ?? val ?? "", [allOptions]);

	const selectedVK = useMemo(() => virtualKeys.find((vk) => vk.value === value), [virtualKeys, value]);
	const selectedBlock = selectedVK ? blocks?.[selectedVK.id] : undefined;
	const fmt = (v?: number) => `$${(v ?? 0).toFixed(2)}`;

	const isMember = authStatus?.role !== "admin";
	const userBudget = authStatus?.budget;
	const userUsage = authStatus?.budget_current_usage ?? 0;
	const hasUserBudget = isMember && (userBudget !== undefined || userUsage > 0);

	return (
		<div className="flex flex-col gap-2">
			<Label className="text-muted-foreground text-xs font-medium uppercase">Virtual key / API Key</Label>
			<p className="text-muted-foreground text-[11px] leading-snug">
				Uses Models providers &amp; Governance virtual keys. Routing, budgets, and pricing overrides apply when you pick a VK or provider key.
			</p>
			<Combobox
				value={value}
				onValueChange={(v) => onValueChange(v)}
				onOpenChange={(open) => {
					if (open) setQuery("");
				}}
				onInputValueChange={(v) => setQuery(v)}
				filter={null}
				itemToStringLabel={getLabel}
			>
				<ComboboxInput placeholder={placeholder ?? "Select API key"} showClear={value !== "__auto__"} showTrigger disabled={disabled} />
				<ComboboxContent>
					<ComboboxList>
						{filteredApiKeys.length > 0 && (
							<ComboboxGroup>
								<ComboboxLabel>API Keys</ComboboxLabel>
								{filteredApiKeys.map((o) => (
									<ComboboxItem key={o.value} value={o.value}>
										{o.label}
									</ComboboxItem>
								))}
							</ComboboxGroup>
						)}
						{filteredApiKeys.length > 0 && filteredVirtualKeys.length > 0 && <ComboboxSeparator />}
						{filteredVirtualKeys.length > 0 && (
							<ComboboxGroup>
								<ComboboxLabel>Virtual Keys</ComboboxLabel>
								{filteredVirtualKeys.map((o) => (
									<ComboboxItem key={o.value} value={o.value}>
										{o.label}
									</ComboboxItem>
								))}
							</ComboboxGroup>
						)}
						{filtered.length === 0 && <div className="text-muted-foreground py-6 text-center text-sm">No results found.</div>}
					</ComboboxList>
				</ComboboxContent>
			</Combobox>

			{/* Real-time Virtual Key Budget Status */}
			{selectedVK && (() => {
				const customerName = selectedVK.customer?.name || selectedVK.customers?.[0]?.name;
				const teamName = selectedVK.team?.name || selectedVK.teams?.[0]?.name;
				const scopeLabel = customerName ? `Customer: ${customerName}` : teamName ? `Team: ${teamName}` : null;
				const customerBudgets = selectedVK.customer?.budgets || selectedVK.customers?.[0]?.budgets || [];
				const teamBudgets = selectedVK.team?.budgets || selectedVK.teams?.[0]?.budgets || [];
				const vkBudgets = selectedVK.budgets || [];

				return (
					<div className="bg-muted/20 border-border/50 rounded-md border p-2 text-xs space-y-2">
						<div className="flex items-center justify-between">
							<span className="text-muted-foreground font-medium flex items-center gap-1.5">
								<span className="h-1.5 w-1.5 rounded-full bg-teal-500 inline-block" />
								VK: {selectedVK.name}
							</span>
							{scopeLabel && (
								<span className="rounded bg-teal-500/10 text-teal-600 dark:text-teal-400 border border-teal-500/20 px-1.5 py-0.5 text-[10px] font-medium">
									{scopeLabel}
								</span>
							)}
						</div>

						{/* Key Budget */}
						{vkBudgets.length > 0 ? (
							vkBudgets.map((b) => (
								<div key={`vk-${b.id || b.reset_duration}`} className="space-y-0.5">
									<div className="flex items-center justify-between text-[11px]">
										<span className="text-muted-foreground font-medium">Key Budget:</span>
										<span className="font-mono">{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}</span>
									</div>
									{b.max_limit > 0 && (
										<div className="space-y-0.5">
											<div className="h-1.5 w-full overflow-hidden rounded-full bg-muted/60">
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
											<div className="flex justify-between text-[10px] text-muted-foreground">
												<span>Reset: {b.reset_duration || "monthly"}</span>
												<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
											</div>
										</div>
									)}
								</div>
							))
						) : customerBudgets.length === 0 && teamBudgets.length === 0 ? (
							<div className="flex items-center justify-between text-[11px]">
								<span className="text-muted-foreground font-medium">Budget:</span>
								<span className="text-muted-foreground">Unlimited</span>
							</div>
						) : null}

						{/* Customer Budget (only shown to admin) */}
						{!isMember && customerBudgets.map((b) => (
							<div key={`cust-${b.id || b.reset_duration}`} className="space-y-0.5 pt-1 border-t border-border/40">
								<div className="flex items-center justify-between text-[11px]">
									<span className="text-muted-foreground font-medium">Customer Budget ({customerName}):</span>
									<span className="font-mono">{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}</span>
								</div>
								{b.max_limit > 0 && (
									<div className="space-y-0.5">
										<div className="h-1.5 w-full overflow-hidden rounded-full bg-muted/60">
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
										<div className="flex justify-between text-[10px] text-muted-foreground">
											<span>Reset: {b.reset_duration || "monthly"}</span>
											<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
										</div>
									</div>
								)}
							</div>
						))}

						{/* Team Budget (only shown to admin) */}
						{!isMember && teamBudgets.map((b) => (
							<div key={`team-${b.id || b.reset_duration}`} className="space-y-0.5 pt-1 border-t border-border/40">
								<div className="flex items-center justify-between text-[11px]">
									<span className="text-muted-foreground font-medium">Team Budget ({teamName}):</span>
									<span className="font-mono">{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}</span>
								</div>
								{b.max_limit > 0 && (
									<div className="space-y-0.5">
										<div className="h-1.5 w-full overflow-hidden rounded-full bg-muted/60">
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
										<div className="flex justify-between text-[10px] text-muted-foreground">
											<span>Reset: {b.reset_duration || "monthly"}</span>
											<span>{Math.round(((b.current_usage ?? 0) / b.max_limit) * 100)}% used</span>
										</div>
									</div>
								)}
							</div>
						))}
					{selectedVK.is_active === false && (
						<div className="flex items-center gap-1.5 text-rose-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Virtual Key is inactive — prompt executions will be rejected</span>
						</div>
					)}
					{selectedVK.expires_at && new Date(selectedVK.expires_at).getTime() < Date.now() && (
						<div className="flex items-center gap-1.5 text-rose-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Virtual Key has expired — prompt executions will be rejected</span>
						</div>
					)}
					{selectedVK.budgets?.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit) && (
						<div className="flex items-center gap-1.5 text-rose-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Virtual Key budget limit reached — calls will be blocked</span>
						</div>
					)}
					{selectedBlock && (
						<div className="flex items-center gap-1.5 text-rose-500 font-medium text-[11px] pt-1" data-testid="vk-billing-block">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>
								{selectedBlock.scope === "team" ? "Team" : "Customer"} budget for &quot;{selectedBlock.name}&quot; is used up — this key is
								blocked. Pick another key.
							</span>
						</div>
					)}
					{selectedVK.budgets?.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) < b.max_limit && (b.current_usage ?? 0) / b.max_limit > 0.8) && (
						<div className="flex items-center gap-1.5 text-amber-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Warning: Virtual Key budget exceeds 80%</span>
						</div>
					)}
					</div>
				);
			})()}

			{/* Member Personal Budget Indicator */}
			{hasUserBudget && (
				<div className="bg-muted/10 border-border/40 rounded-md border p-2 text-xs space-y-1">
					<div className="flex items-center justify-between text-[11px]">
						<span className="text-muted-foreground font-medium">Your User Budget:</span>
						<span className="font-mono">
							{fmt(userUsage)} / {userBudget && userBudget > 0 ? fmt(userBudget) : "Unlimited"}
						</span>
					</div>
					{userBudget && userBudget > 0 ? (
						<div className="space-y-0.5">
							<div className="h-1.5 w-full overflow-hidden rounded-full bg-muted/60">
								<div
									className={`h-full transition-all ${
										userUsage >= userBudget
											? "bg-rose-500"
											: userUsage / userBudget > 0.8
												? "bg-amber-500"
												: "bg-teal-500"
									}`}
									style={{
										width: `${Math.min(100, Math.max(0, (userUsage / userBudget) * 100))}%`,
									}}
								/>
							</div>
							<div className="flex justify-between text-[10px] text-muted-foreground">
								<span>Monthly limit</span>
								<span>{Math.round((userUsage / userBudget) * 100)}% used</span>
							</div>
						</div>
					) : null}
					{userBudget && userBudget > 0 && userUsage >= userBudget && (
						<div className="flex items-center gap-1.5 text-rose-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Personal budget limit reached</span>
						</div>
					)}
					{userBudget && userBudget > 0 && userUsage < userBudget && userUsage / userBudget > 0.8 && (
						<div className="flex items-center gap-1.5 text-amber-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Warning: Personal budget exceeds 80%</span>
						</div>
					)}
				</div>
			)}
		</div>
	);
}