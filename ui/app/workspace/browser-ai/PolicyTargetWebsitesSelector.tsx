import React, { useMemo } from "react";
import { Label } from "@/components/ui/label";
import { ComboboxSelect, type ComboboxSelectOption } from "@/components/ui/combobox";
import type { BrowserTargetWebsite } from "@/lib/store/apis/browserAiApi";
import { cn } from "@/lib/utils";

import { groupTargetsByParent } from "./relatedHosts";

export interface PolicyTargetWebsitesSelectorProps {
	policyName?: string;
	value?: string;
	targets: BrowserTargetWebsite[];
	onChange: (nextValue: string) => void;
	disabled?: boolean;
	className?: string;
}

export const PolicyTargetWebsitesSelector: React.FC<PolicyTargetWebsitesSelectorProps> = ({
	value,
	targets,
	onChange,
	disabled = false,
	className,
}) => {
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
		if (!raw || raw === "all" || raw === "*") {
			return options.map((o) => o.value);
		}
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

	const handleValueChange = (next: string[]) => {
		if (!next || next.length === 0) {
			onChange("");
			return;
		}
		if (next.length === options.length) {
			onChange("all");
			return;
		}
		const allDomains = next.flatMap((val) => optionValueToDomains.get(val) || [val]);
		onChange(Array.from(new Set(allDomains)).join(", "));
	};

	return (
		<div className={cn("flex flex-col gap-1.5 pt-3 border-t border-border/40", className)}>
			<Label className="text-xs font-semibold text-foreground">Target Websites</Label>
			<div className="w-full sm:max-w-md">
				<ComboboxSelect
					multiple
					compactTrigger
					options={options}
					value={selectedValues}
					onValueChange={handleValueChange}
					disabled={disabled || options.length === 0}
					placeholder="Select target websites..."
					searchPlaceholder="Search target websites..."
					emptyMessage="No matching target websites found."
					selectAllValue="*"
					selectAllLabel="Select all"
					selectAllTriggerLabel={options.length > 0 ? `All Target Websites (${options.length})` : "All Target Websites"}
					className="h-8 w-full bg-background border-border text-foreground text-xs hover:bg-muted/40 font-normal"
				/>
			</div>
		</div>
	);
};

