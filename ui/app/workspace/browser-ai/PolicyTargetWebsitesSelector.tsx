import React, { useMemo, useState, useEffect } from "react";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ComboboxSelect, type ComboboxSelectOption } from "@/components/ui/combobox";
import type { BrowserTargetWebsite } from "@/lib/store/apis/browserAiApi";
import { cn } from "@/lib/utils";
import { Save, Check } from "lucide-react";
import { useToast } from "@/hooks/use-toast";

import { groupTargetsByParent } from "./relatedHosts";

export interface PolicyTargetWebsitesSelectorProps {
	policyName?: string;
	value?: string;
	targets: BrowserTargetWebsite[];
	onChange: (nextValue: string) => void | Promise<any>;
	disabled?: boolean;
	className?: string;
}

export const PolicyTargetWebsitesSelector: React.FC<PolicyTargetWebsitesSelectorProps> = ({
	policyName,
	value,
	targets,
	onChange,
	disabled = false,
	className,
}) => {
	const { toast } = useToast();
	const [isSaving, setIsSaving] = useState(false);
	const [savedRecently, setSavedRecently] = useState(false);

	const { options, domainToOptionValue, optionValueToDomains } = useMemo(() => {
		// Use the Target Websites hierarchy: only main parent domains are options, subdomains are nested under them
		const groups = groupTargetsByParent(targets ?? []);

		const domToVal = new Map<string, string>();
		const valToDoms = new Map<string, string[]>();

		const opts: ComboboxSelectOption[] = groups.map((g) => {
			const parentDom = (g.parent.domain || "").trim().toLowerCase();
			const pName = (g.parent.platform_name || "").trim();
			const label = pName || parentDom;

			const allDoms = [
				parentDom,
				...g.children.map((c) => (c.domain || "").trim().toLowerCase()).filter(Boolean),
			];

			valToDoms.set(parentDom, allDoms);
			for (const d of allDoms) {
				domToVal.set(d, parentDom);
			}
			domToVal.set(label.toLowerCase(), parentDom);

			return {
				value: parentDom,
				label,
			};
		});

		opts.sort((a, b) => a.label.localeCompare(b.label));

		return {
			options: opts,
			domainToOptionValue: domToVal,
			optionValueToDomains: valToDoms,
		};
	}, [targets]);

	const selectedValues = useMemo<string[]>(() => {
		const raw = (value || "").trim().toLowerCase();
		if (!raw || raw === "none" || raw === "all" || raw === "*") return [];
		const tokens = raw
			.split(/[,\s;]+/)
			.map((s) => s.trim().toLowerCase())
			.filter(Boolean);

		const selected = new Set<string>();
		for (const tok of tokens) {
			const optVal = domainToOptionValue.get(tok);
			if (optVal) {
				selected.add(optVal);
			} else if (options.some((o) => o.value === tok)) {
				selected.add(tok);
			}
		}

		return Array.from(selected);
	}, [value, options, domainToOptionValue]);

	const [draft, setDraft] = useState<string[]>(selectedValues);

	useEffect(() => {
		setDraft(selectedValues);
	}, [selectedValues]);

	const isDirty = useMemo(() => {
		if (draft.length !== selectedValues.length) return true;
		const sSet = new Set(selectedValues);
		return draft.some((d) => !sSet.has(d));
	}, [draft, selectedValues]);

	const computeValue = (selected: string[]): string => {
		if (!selected || selected.length === 0) {
			return "none";
		}
		const allDomains = selected.flatMap((val) => optionValueToDomains.get(val) || [val]);
		return Array.from(new Set(allDomains)).join(", ");
	};

	const handleValueChange = (next: string[]) => {
		setDraft(next);
	};

	const handleSave = async () => {
		setIsSaving(true);
		try {
			const computed = computeValue(draft);
			await onChange(computed);
			setSavedRecently(true);
			setTimeout(() => setSavedRecently(false), 2500);
			const displayName = policyName || "Policy";
			toast({
				title: `${displayName} Target Websites Saved`,
				description:
					draft.length === 0
						? "No target websites selected. Policy is disabled for all sites."
						: `Saved ${draft.length} target website(s) in database.`,
			});
		} catch {
			toast({
				title: "Could not save target websites",
				variant: "destructive",
			});
		} finally {
			setIsSaving(false);
		}
	};

	const isAllSelected = options.length > 0 && draft.length === options.length;

	return (
		<div className={cn("flex flex-col gap-2 pt-3 border-t border-border/40", className)}>
			<div className="flex items-center justify-between flex-wrap gap-2">
				<div className="flex items-center gap-2">
					<Label className="text-xs font-semibold text-foreground">
						{policyName ? `${policyName} Target Websites` : "Target Websites"}
					</Label>
					{draft.length > 0 ? (
						<Badge
							variant="outline"
							className="text-[10px] px-2 py-0.5 border-blue-500/50 text-blue-600 dark:text-blue-400 bg-blue-500/10 font-normal"
						>
							{draft.length} of {options.length} Selected
						</Badge>
					) : (
						<Badge variant="outline" className="text-[10px] text-muted-foreground font-normal">
							None Selected
						</Badge>
					)}
				</div>
				<div className="flex items-center gap-2.5">
					{!isAllSelected && options.length > 0 ? (
						<button
							type="button"
							onClick={() => handleValueChange(options.map((o) => o.value))}
							className="text-[11px] text-primary hover:underline cursor-pointer"
						>
							Select all ({options.length})
						</button>
					) : null}
					{draft.length > 0 ? (
						<button
							type="button"
							onClick={() => handleValueChange([])}
							className="text-[11px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
						>
							Deselect all
						</button>
					) : null}
				</div>
			</div>
			<div className="flex items-center gap-2 w-full">
				<div className="flex-1 min-w-0 sm:max-w-md">
					<ComboboxSelect
						multiple
						compactTrigger
						options={options}
						value={draft}
						onValueChange={handleValueChange}
						disabled={disabled || options.length === 0}
						placeholder="Select target websites (None selected)..."
						searchPlaceholder="Search target websites..."
						emptyMessage="No matching target websites found."
						selectAllValue="*"
						selectAllLabel="Select all"
						selectAllTriggerLabel={options.length > 0 ? `All Selected (${options.length})` : "All Selected"}
						className="h-8 w-full bg-background border-border text-foreground text-xs hover:bg-muted/40 font-normal"
					/>
				</div>
				<Button
					type="button"
					size="sm"
					variant={isDirty ? "default" : "outline"}
					disabled={disabled || isSaving}
					onClick={handleSave}
					className="h-8 text-xs gap-1.5 shrink-0"
					title="Save target websites to database"
				>
					{savedRecently ? (
						<>
							<Check className="h-3.5 w-3.5 text-emerald-400" />
							<span className="text-emerald-400 font-medium">Saved</span>
						</>
					) : (
						<>
							<Save className="h-3.5 w-3.5" />
							<span>{isSaving ? "Saving..." : isDirty ? "Save Changes" : "Save"}</span>
						</>
					)}
				</Button>
			</div>
		</div>
	);
};
