import type { GuardrailProvider, GuardrailRule } from "@/lib/store/apis/guardrailsApi";

export type GuardrailPromptScope = "all" | "prompts" | "custom";

export function nextGuardrailId(existingIds: number[]): number {
	if (existingIds.length === 0) {
		return 1;
	}
	return Math.max(...existingIds) + 1;
}

export function collectGuardrailIds(rules: GuardrailRule[], providers: GuardrailProvider[]): number[] {
	const ids = new Set<number>();
	for (const rule of rules) {
		ids.add(rule.id);
	}
	for (const provider of providers) {
		ids.add(provider.id);
	}
	return Array.from(ids);
}

export function providerLabel(provider: GuardrailProvider): string {
	return `${provider.policy_name} (#${provider.id})`;
}

export function formatLinkedProviders(rule: GuardrailRule, providers: GuardrailProvider[]): string {
	const linked = (rule.provider_config_ids || [])
		.map((id) => providers.find((provider) => provider.id === id))
		.filter((provider): provider is GuardrailProvider => provider != null)
		.map((provider) => provider.policy_name);

	if (linked.length === 0) {
		return "None";
	}
	return linked.join(", ");
}

export function buildCelFromPromptSelection(scope: "all" | "prompts", promptIds: string[]): string {
	if (scope === "all") {
		return "true";
	}
	const ids = promptIds.map((id) => id.trim()).filter(Boolean);
	if (ids.length === 0) {
		return "";
	}
	if (ids.length === 1) {
		return `request.prompt_id == '${ids[0]}'`;
	}
	const list = ids.map((id) => `'${id}'`).join(", ");
	return `[${list}].exists(p, p == request.prompt_id)`;
}

export function parseCelPromptSelection(celExpression: string): { scope: GuardrailPromptScope; promptIds: string[] } {
	const trimmed = (celExpression || "").trim();
	if (trimmed === "true") {
		return { scope: "all", promptIds: [] };
	}

	const singleMatch = trimmed.match(/^request\.prompt_id\s*==\s*'([^']+)'$/);
	if (singleMatch?.[1]) {
		return { scope: "prompts", promptIds: [singleMatch[1]] };
	}

	const multiMatch = trimmed.match(/^\[(.*)\]\.exists\(p,\s*p\s*==\s*request\.prompt_id\)$/);
	if (multiMatch?.[1]) {
		const promptIds = [...multiMatch[1].matchAll(/'([^']+)'/g)].map((match) => match[1]).filter(Boolean);
		if (promptIds.length > 0) {
			return { scope: "prompts", promptIds };
		}
	}

	return { scope: "custom", promptIds: [] };
}

export function formatRuleTriggerSummary(celExpression: string, promptNameById: Map<string, string>): string {
	const { scope, promptIds } = parseCelPromptSelection(celExpression);
	if (scope === "all") {
		return "All requests";
	}
	if (scope === "prompts") {
		const names = promptIds.map((id) => promptNameById.get(id) || id).filter(Boolean);
		return names.length > 0 ? `Prompts: ${names.join(", ")}` : "Selected prompts";
	}
	return celExpression || "Custom expression";
}

export function formatRuleConnectionPreview(opts: {
	applyTo: string;
	promptScope: GuardrailPromptScope;
	selectedPromptIds: string[];
	promptNameById: Map<string, string>;
	providerLabels: string[];
}): string {
	const phase =
		opts.applyTo === "both" ? "user input and model output" : opts.applyTo === "output" ? "model output" : "user input";
	const providers = opts.providerLabels.length > 0 ? opts.providerLabels.join(", ") : "no providers selected";
	let when = "every chat/completions request through UnifAI";
	if (opts.promptScope === "prompts") {
		const names = opts.selectedPromptIds.map((id) => opts.promptNameById.get(id) || id).filter(Boolean);
		when = names.length
			? `only when Prompt Repo runs: ${names.join(", ")} (requires x-uf-prompt-id)`
			: "selected Prompt Repo prompts (pick at least one)";
	} else if (opts.promptScope === "custom") {
		when = "when your custom CEL expression matches (request.model / request.prompt_id)";
	}
	return `Will scan ${phase} with [${providers}] on ${when}. Does not apply to Browser AI Guard or MCP-only traffic.`;
}
