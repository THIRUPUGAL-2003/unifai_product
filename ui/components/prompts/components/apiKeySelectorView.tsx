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
	const { data: authStatus } = useIsAuthEnabledQuery();

	const allOptions = useMemo(() => {
		const apiKeyOpts = providerKeys.map((k) => ({ label: k.name, value: k.key_id, group: "api" as const }));
		const vkOpts = virtualKeys.map((vk) => ({ label: vk.name, value: vk.value, group: "virtual" as const }));
		return [{ label: "Auto (default)", value: "__auto__", group: "api" as const }, ...apiKeyOpts, ...vkOpts];
	}, [providerKeys, virtualKeys]);

	const filtered = useMemo(() => {
		if (!query) return allOptions;
		const q = query.toLowerCase();
		return allOptions.filter((o) => o.label.toLowerCase().includes(q));
	}, [allOptions, query]);

	const filteredApiKeys = useMemo(() => filtered.filter((o) => o.group === "api"), [filtered]);
	const filteredVirtualKeys = useMemo(() => filtered.filter((o) => o.group === "virtual"), [filtered]);

	const getLabel = useCallback((val: string | null) => allOptions.find((o) => o.value === val)?.label ?? val ?? "", [allOptions]);

	const selectedVK = useMemo(() => virtualKeys.find((vk) => vk.value === value), [virtualKeys, value]);
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
			{selectedVK && (
				<div className="bg-muted/20 border-border/50 rounded-md border p-2 text-xs space-y-1.5">
					<div className="flex items-center justify-between">
						<span className="text-muted-foreground font-medium flex items-center gap-1.5">
							<span className="h-1.5 w-1.5 rounded-full bg-teal-500 inline-block" />
							VK: {selectedVK.name}
						</span>
						{selectedVK.budgets && selectedVK.budgets.length > 0 ? (
							selectedVK.budgets.map((b) => (
								<span key={b.id || b.reset_duration} className="font-mono text-[11px]">
									{fmt(b.current_usage)} / {b.max_limit > 0 ? fmt(b.max_limit) : "Unlimited"}
								</span>
							))
						) : (
							<span className="text-muted-foreground text-[11px]">Budget: Unlimited</span>
						)}
					</div>
					{selectedVK.budgets?.map((b) =>
						b.max_limit > 0 ? (
							<div key={b.id || b.reset_duration} className="space-y-0.5">
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
						) : null,
					)}
					{selectedVK.budgets?.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) >= b.max_limit) && (
						<div className="flex items-center gap-1.5 text-rose-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Virtual Key budget limit reached — calls will be blocked</span>
						</div>
					)}
					{selectedVK.budgets?.some((b) => b.max_limit > 0 && (b.current_usage ?? 0) < b.max_limit && (b.current_usage ?? 0) / b.max_limit > 0.8) && (
						<div className="flex items-center gap-1.5 text-amber-500 font-medium text-[11px] pt-1">
							<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
							<span>Warning: Virtual Key budget exceeds 80%</span>
						</div>
					)}
				</div>
			)}

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