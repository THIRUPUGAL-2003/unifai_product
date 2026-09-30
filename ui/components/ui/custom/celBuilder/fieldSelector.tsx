/**
 * Field Selector Component for CEL Rule Builder
 * Allows selection of fields for building CEL expressions
 * For keyValue fields (headers/params), renders a smart searchable Combobox
 * with pre-filled suggestions and creatable custom inputs, plus a manual input toggle.
 */

import { Button } from "@/components/ui/button";
import { ComboboxSelect, ComboboxSelectOption } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useGetCoreConfigQuery } from "@/lib/store/apis/configApi";
import { ListFilter, PenLine } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { FieldSelectorProps, RuleGroupType, RuleType } from "react-querybuilder";

// Popular pre-configured Header suggestions
const COMMON_HEADER_SUGGESTIONS: ComboboxSelectOption[] = [
	// Rate Limit & Quota Headers
	{ label: "x-ratelimit-remaining-requests (Remaining Calls)", value: "x-ratelimit-remaining-requests" },
	{ label: "x-ratelimit-remaining-tokens (Remaining Tokens)", value: "x-ratelimit-remaining-tokens" },
	{ label: "x-ratelimit-limit-requests (RPM Limit)", value: "x-ratelimit-limit-requests" },
	{ label: "x-ratelimit-limit-tokens (TPM Limit)", value: "x-ratelimit-limit-tokens" },
	{ label: "x-ratelimit-reset-requests (Reset Duration)", value: "x-ratelimit-reset-requests" },
	{ label: "x-ratelimit-reset-tokens (Token Reset Duration)", value: "x-ratelimit-reset-tokens" },
	{ label: "retry-after (Rate Limit Retry Seconds)", value: "retry-after" },

	// Gateway & Auth Headers
	{ label: "authorization (Bearer Token / Virtual Key)", value: "authorization" },
	{ label: "x-uf-vk (Virtual Key Header)", value: "x-uf-vk" },
	{ label: "x-uf-api-key-id (Specific Key Pinning)", value: "x-uf-api-key-id" },
	{ label: "x-api-key (API Key)", value: "x-api-key" },
	{ label: "x-environment (dev / staging / prod)", value: "x-environment" },
	{ label: "x-team-id (Team ID)", value: "x-team-id" },
	{ label: "x-customer-id (Customer ID)", value: "x-customer-id" },
	{ label: "x-user-id (User ID)", value: "x-user-id" },
	{ label: "x-purpose (Purpose / Use-case Tag)", value: "x-purpose" },
];

// Popular pre-configured Query Parameter suggestions
const COMMON_PARAM_SUGGESTIONS: ComboboxSelectOption[] = [
	{ label: "env (Environment - staging / prod)", value: "env" },
	{ label: "environment (Environment)", value: "environment" },
	{ label: "tier (User Tier - free / pro / vip)", value: "tier" },
	{ label: "user_tier (User Tier)", value: "user_tier" },
	{ label: "region (Region - us / eu / apac)", value: "region" },
	{ label: "customer_id (Customer ID)", value: "customer_id" },
	{ label: "user_id (User ID)", value: "user_id" },
	{ label: "team_id (Team ID)", value: "team_id" },
	{ label: "version (API Version)", value: "version" },
	{ label: "model (Requested Model)", value: "model" },
	{ label: "stream (Streaming Flag)", value: "stream" },
	{ label: "debug (Debug Mode)", value: "debug" },
];

function normalizeHeaderKey(raw: string): string {
	return String(raw || "")
		.trim()
		.toLowerCase()
		.replace(/^x-uf-eh-/i, "");
}

/**
 * Recursively find and update a rule's value by path in the query tree.
 */
function updateRuleValueAtPath(query: RuleGroupType, targetPath: number[], newValue: string): RuleGroupType {
	if (targetPath.length === 0) return query;

	const [currentIndex, ...restPath] = targetPath;
	const newRules = [...query.rules];

	if (restPath.length === 0) {
		const rule = newRules[currentIndex] as RuleType;
		newRules[currentIndex] = { ...rule, value: newValue };
	} else {
		newRules[currentIndex] = updateRuleValueAtPath(newRules[currentIndex] as RuleGroupType, restPath, newValue);
	}

	return { ...query, rules: newRules };
}

export function FieldSelector({ value, handleOnChange, options, rule, path, schema }: FieldSelectorProps) {
	const [isManualInput, setIsManualInput] = useState(false);

	// Resolve field by name or value — RQB Field objects keep custom props (inputType) from our mapping.
	const fieldData = useMemo(() => {
		const fields = schema?.fields ?? [];
		for (const field of fields) {
			if (!field || typeof field !== "object" || "options" in field) continue;
			const candidate = field as { name?: string; value?: string; inputType?: string; label?: string };
			if (candidate.name === value || candidate.value === value) {
				return candidate;
			}
		}
		return null;
	}, [schema?.fields, value]);

	const isHeaderField = fieldData?.name === "headers" || value === "headers";
	const isParamField = fieldData?.name === "params" || value === "params";
	// Fallback: headers/params are always keyValue even if schema lookup misses custom props.
	const isKeyValueField = Boolean(fieldData?.inputType === "keyValue" || isHeaderField || isParamField);

	useEffect(() => {
		setIsManualInput(false);
	}, [value]);

	const { data: coreConfig } = useGetCoreConfigQuery({});

	const headerOptions = useMemo(() => {
		const rawReq = coreConfig?.client_config?.required_headers || [];
		const rawLog = coreConfig?.client_config?.logging_headers || [];
		const rawAllow = coreConfig?.client_config?.header_filter_config?.allowlist || [];
		const configured = Array.from(
			new Set([...rawReq, ...rawLog, ...rawAllow].map((h) => normalizeHeaderKey(String(h))).filter(Boolean)),
		);

		const existingKeys = new Set(COMMON_HEADER_SUGGESTIONS.map((s) => s.value.toLowerCase()));
		const customConfiguredOptions: ComboboxSelectOption[] = configured
			.filter((key) => !existingKeys.has(key))
			.map((key) => ({
				label: `${key} (Configured Header)`,
				value: key,
			}));

		return [...COMMON_HEADER_SUGGESTIONS, ...customConfiguredOptions];
	}, [coreConfig]);

	const headerKey = useMemo(() => {
		if (!isKeyValueField || !rule?.value || typeof rule.value !== "string") return "";
		const colonIndex = rule.value.indexOf(":");
		if (colonIndex > 0) return rule.value.substring(0, colonIndex).trim();
		return rule.value.trim();
	}, [isKeyValueField, rule?.value]);

	const keyOptions = useMemo(() => {
		const base = isHeaderField ? headerOptions : isParamField ? COMMON_PARAM_SUGGESTIONS : [];
		const current = headerKey.trim();
		if (current && !base.some((o) => o.value.toLowerCase() === current.toLowerCase())) {
			return [{ label: current, value: current }, ...base];
		}
		return base;
	}, [isHeaderField, isParamField, headerOptions, headerKey]);

	const handleKeyChange = useCallback(
		(newKey: string) => {
			if (!schema || !path) return;
			const currentValue = typeof rule?.value === "string" ? rule.value : "";
			const colonIndex = currentValue.indexOf(":");
			const valuePart = colonIndex > 0 ? currentValue.substring(colonIndex + 1).trim() : "";

			let updatedValue: string;
			if (newKey && valuePart) {
				updatedValue = `${newKey}:${valuePart}`;
			} else if (newKey) {
				updatedValue = newKey;
			} else {
				updatedValue = "";
			}

			const currentQuery = schema.getQuery() as RuleGroupType;
			const updatedQuery = updateRuleValueAtPath(currentQuery, path, updatedValue);
			schema.dispatchQuery(updatedQuery);
		},
		[schema, path, rule?.value],
	);

	const handleFieldChange = useCallback(
		(nextField: string) => {
			handleOnChange(nextField);
		},
		[handleOnChange],
	);

	return (
		<div className="flex items-center gap-2">
			<Select value={value || ""} onValueChange={handleFieldChange}>
				<SelectTrigger className="w-[180px]" data-testid="cel-builder-field-selector-select">
					<SelectValue placeholder="Select field..." />
				</SelectTrigger>
				<SelectContent>
					{options.map((opt) => {
						const option = opt as { name?: string; label?: string; disabled?: boolean; options?: unknown };
						if ("options" in option && option.options) {
							return null;
						}
						if (!option.name) {
							return null;
						}
						return (
							<SelectItem key={option.name} value={option.name} disabled={option.disabled}>
								{option.label}
							</SelectItem>
						);
					})}
				</SelectContent>
			</Select>

			{isKeyValueField && (
				<div className="flex items-center gap-1.5">
					<span className="text-muted-foreground text-sm whitespace-nowrap">has key</span>

					{!isManualInput ? (
						<div className="flex items-center gap-1">
							<ComboboxSelect
								options={keyOptions}
								value={headerKey || null}
								onValueChange={(val) => handleKeyChange(val || "")}
								placeholder={isHeaderField ? "Select header key..." : isParamField ? "Select param key..." : "Select key..."}
								searchPlaceholder={isHeaderField ? "Search or type header..." : "Search or type param..."}
								creatable
								createLabel={(typed) => `Use "${typed}"`}
								className="w-[230px]"
								data-testid="cel-builder-field-selector-combobox"
							/>
							<Button
								type="button"
								variant="ghost"
								size="icon"
								className="text-muted-foreground hover:text-foreground h-8 w-8"
								onClick={() => setIsManualInput(true)}
								title="Switch to manual text typing"
								data-testid="cel-builder-field-selector-manual-toggle"
							>
								<PenLine className="h-3.5 w-3.5" />
							</Button>
						</div>
					) : (
						<div className="flex items-center gap-1">
							<Input
								type="text"
								value={headerKey}
								onChange={(e) => handleKeyChange(e.target.value)}
								placeholder={
									isHeaderField
										? "Header name (e.g., x-api-key)"
										: isParamField
											? "Param name (e.g., user_id)"
											: `${fieldData?.label || "Key"} name`
								}
								className="w-[230px]"
								data-testid="cel-builder-field-selector-key-input"
								autoFocus
							/>
							<Button
								type="button"
								variant="ghost"
								size="icon"
								className="text-muted-foreground hover:text-foreground h-8 w-8"
								onClick={() => setIsManualInput(false)}
								title="Switch to dropdown list"
								data-testid="cel-builder-field-selector-dropdown-toggle"
							>
								<ListFilter className="h-3.5 w-3.5" />
							</Button>
						</div>
					)}
				</div>
			)}
		</div>
	);
}
