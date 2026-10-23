import React, { useState, useMemo } from "react";
import { Globe, Check, X, Search, ChevronDown, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { BrowserTargetWebsite } from "@/lib/store/apis/browserAiApi";

export interface PolicyTargetWebsitesSelectorProps {
	policyName: string;
	value?: string;
	targets: BrowserTargetWebsite[];
	onChange: (nextValue: string) => void;
	disabled?: boolean;
}

export const PolicyTargetWebsitesSelector: React.FC<PolicyTargetWebsitesSelectorProps> = ({
	policyName,
	value,
	targets,
	onChange,
	disabled = false,
}) => {
	const [searchQuery, setSearchQuery] = useState("");
	const [isOpen, setIsOpen] = useState(false);

	const selectedDomains = useMemo(() => {
		const raw = (value || "").trim().toLowerCase();
		if (!raw || raw === "all" || raw === "*") {
			return ["all"];
		}
		return raw
			.split(/[,\s;]+/)
			.map((s) => s.trim().toLowerCase())
			.filter(Boolean);
	}, [value]);

	const isAll = selectedDomains.includes("all");

	const filteredTargets = useMemo(() => {
		const q = searchQuery.trim().toLowerCase();
		if (!q) return targets;
		return targets.filter(
			(t) =>
				t.domain.toLowerCase().includes(q) ||
				(t.platform_name && t.platform_name.toLowerCase().includes(q))
		);
	}, [targets, searchQuery]);

	const handleSelectAll = () => {
		onChange("all");
		setIsOpen(false);
	};

	const handleToggleDomain = (domain: string) => {
		const dom = domain.trim().toLowerCase();
		let next: string[];
		if (isAll) {
			// If currently all, toggling an individual domain scopes down to just that domain
			next = [dom];
		} else if (selectedDomains.includes(dom)) {
			// Remove it
			next = selectedDomains.filter((d) => d !== dom);
			if (next.length === 0) {
				next = ["all"];
			}
		} else {
			next = [...selectedDomains, dom];
		}
		onChange(next.includes("all") ? "all" : next.join(", "));
	};

	const handleRemoveDomain = (dom: string) => {
		const next = selectedDomains.filter((d) => d !== dom);
		onChange(next.length === 0 ? "all" : next.join(", "));
	};

	return (
		<div className="space-y-2.5 pt-3 border-t border-border/50">
			<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1">
				<div>
					<p className="text-xs font-semibold text-foreground flex items-center gap-1.5">
						<Globe className="h-3.5 w-3.5 text-muted-foreground" />
						Target AI Platforms Scoping
					</p>
					<p className="text-[11px] text-muted-foreground">
						Select which AI websites {policyName.toLowerCase()} applies to.
					</p>
				</div>
				{!isAll && (
					<Button
						type="button"
						variant="ghost"
						size="sm"
						disabled={disabled}
						onClick={handleSelectAll}
						className="h-7 text-xs text-muted-foreground hover:text-foreground gap-1 px-2 self-start sm:self-center"
					>
						<RotateCcw className="h-3 w-3" />
						Enforce on All Websites
					</Button>
				)}
			</div>

			{/* Dropdown Selector */}
			<div className="flex items-center gap-2">
				<Popover open={isOpen} onOpenChange={setIsOpen}>
					<PopoverTrigger asChild>
						<Button
							type="button"
							variant="outline"
							disabled={disabled}
							className="h-8 justify-between text-xs font-normal border-border bg-background hover:bg-muted/40 w-full sm:max-w-md"
						>
							<div className="flex items-center gap-2 truncate">
								<Globe className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
								{isAll ? (
									<span className="font-medium text-foreground truncate">
										🌐 All Monitored Target Websites ({targets.length})
									</span>
								) : (
									<span className="font-medium text-foreground truncate">
										Scoped to {selectedDomains.length} of {targets.length} Target Websites
									</span>
								)}
							</div>
							<ChevronDown className={`h-3.5 w-3.5 text-muted-foreground shrink-0 ml-2 transition-transform duration-200 ${isOpen ? "rotate-180" : ""}`} />
						</Button>
					</PopoverTrigger>
					<PopoverContent className="w-[340px] p-2 space-y-2" align="start">
						<div className="relative">
							<Search className="h-3.5 w-3.5 absolute left-2.5 top-2.5 text-muted-foreground" />
							<Input
								placeholder="Search target AI platform or domain..."
								value={searchQuery}
								onChange={(e) => setSearchQuery(e.target.value)}
								className="h-8 text-xs pl-8 font-sans"
							/>
						</div>

						<div className="max-h-[220px] overflow-y-auto space-y-0.5 pr-1">
							{/* Option: Enforce on All Target Websites */}
							<button
								type="button"
								onClick={handleSelectAll}
								className={`w-full flex items-center justify-between px-2.5 py-1.5 rounded-md text-xs transition-colors ${
									isAll
										? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 font-medium"
										: "text-foreground hover:bg-muted/60"
								}`}
							>
								<div className="flex items-center gap-2 truncate">
									<Globe className="h-3.5 w-3.5 shrink-0" />
									<span>All Monitored Target Websites ({targets.length})</span>
								</div>
								{isAll && <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />}
							</button>

							<div className="h-px bg-border my-1" />

							{/* Individual target websites */}
							{filteredTargets.length === 0 ? (
								<p className="text-xs text-muted-foreground text-center py-3 italic">
									No matching AI platforms found.
								</p>
							) : (
								filteredTargets.map((tgt) => {
									const dom = tgt.domain.toLowerCase();
									const isSelected = !isAll && selectedDomains.includes(dom);
									return (
										<button
											key={tgt.id || tgt.domain}
											type="button"
											onClick={() => handleToggleDomain(tgt.domain)}
											className={`w-full flex items-center justify-between px-2.5 py-1.5 rounded-md text-xs transition-colors ${
												isSelected
													? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 font-medium"
													: "text-foreground hover:bg-muted/60"
											}`}
										>
											<div className="flex items-center gap-2 truncate">
												<span className={`h-1.5 w-1.5 rounded-full shrink-0 ${isSelected ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
												<span className="truncate">{tgt.platform_name || tgt.domain}</span>
												<span className="text-[10px] text-muted-foreground font-mono truncate">({tgt.domain})</span>
											</div>
											{isSelected && <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />}
										</button>
									);
								})
							)}
						</div>
					</PopoverContent>
				</Popover>
			</div>

			{/* SHOW SELECTED: Shows only the selected websites */}
			{isAll ? (
				<div className="flex items-center gap-2 px-3 py-2 rounded-lg border border-border bg-muted/20 text-xs">
					<Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />
					<span className="text-muted-foreground">
						Currently enforced on <strong className="text-foreground font-medium">all {targets.length} monitored target websites</strong>.
					</span>
				</div>
			) : (
				<div className="space-y-1.5">
					<div className="flex items-center justify-between">
						<span className="text-[11px] font-medium text-muted-foreground">
							Selected Target AI Platforms ({selectedDomains.length}):
						</span>
						<span className="text-[10px] text-muted-foreground">
							Click ✕ to remove platform from scope
						</span>
					</div>
					<div className="flex flex-wrap items-center gap-1.5 p-2 rounded-lg border border-border bg-muted/15 min-h-[40px]">
						{selectedDomains.map((dom) => {
							const tgt = targets.find((t) => t.domain.toLowerCase() === dom);
							const name = tgt?.platform_name || dom;
							return (
								<span
									key={dom}
									className="inline-flex items-center gap-1.5 pl-2.5 pr-1.5 py-1 rounded-md text-xs border border-emerald-500/30 bg-emerald-500/10 text-emerald-800 dark:text-emerald-200"
								>
									<span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
									<span className="font-medium">{name}</span>
									<span className="text-[10px] font-mono text-emerald-700/70 dark:text-emerald-300/70">({dom})</span>
									<button
										type="button"
										onClick={() => handleRemoveDomain(dom)}
										disabled={disabled}
										className="p-0.5 rounded text-emerald-700/80 hover:text-emerald-900 hover:bg-emerald-500/20 dark:text-emerald-300/80 dark:hover:text-white"
										title={`Remove ${name}`}
									>
										<X className="h-3 w-3" />
									</button>
								</span>
							);
						})}
					</div>
				</div>
			)}
		</div>
	);
};
