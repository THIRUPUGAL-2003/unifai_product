import React, { useMemo } from "react";
import { ComboboxSelect } from "@/components/ui/combobox";
import { useGetModelsQuery } from "@/lib/store/apis/providersApi";
import type { GuardBotModelPickerProps } from "./browserAiTypes";

export function GuardBotOutsourceModelPicker({ provider, value, onChange, disabled }: GuardBotModelPickerProps) {
	const { data, isFetching, isError } = useGetModelsQuery(
		{ provider: provider || undefined, limit: 1000, unfiltered: true },
		{ skip: !provider },
	);
	const options = useMemo(() => {
		const seen = new Set<string>();
		const preferred: { label: string; value: string }[] = [];
		const rest: { label: string; value: string }[] = [];
		for (const m of data?.models || []) {
			const name = String(m?.name || "").trim();
			if (!name || seen.has(name)) continue;
			seen.add(name);
			const low = name.toLowerCase();
			const weak = low.includes(":free") || low.includes("code") || low.includes("embed") || low.includes("whisper");
			const opt = { label: weak ? `${name} (not recommended for Guard)` : name, value: name };
			if (weak) rest.push(opt);
			else preferred.push(opt);
		}
		const opts = [...preferred, ...rest];
		const current = String(value || "").trim();
		if (current && !seen.has(current)) {
			opts.unshift({ label: current, value: current });
		}
		return opts;
	}, [data, value]);

	React.useEffect(() => {
		if (provider && !value && options.length > 0) {
			const firstPreferred = options[0]?.value;
			if (firstPreferred) {
				onChange(firstPreferred);
			}
		}
	}, [provider, value, options, onChange]);

	return (
		<>
			<ComboboxSelect
				options={options}
				value={value || null}
				onValueChange={(v) => onChange(String(v || ""))}
				placeholder={!provider ? "Select provider first" : isFetching ? "Loading catalog models..." : isError ? "Could not load models" : "Select model"}
				hideClear
				disabled={disabled || !provider}
				emptyMessage={!provider ? "Pick a provider" : "No models — add API keys under Model Providers"}
				searchPlaceholder="Search catalog models..."
				data-testid="browser-ai-guard-bot-outsource-model"
			/>
		</>
	);
}
