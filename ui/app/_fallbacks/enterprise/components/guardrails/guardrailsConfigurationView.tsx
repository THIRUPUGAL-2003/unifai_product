import React, { useEffect, useMemo, useState } from "react";
import { Plus, Trash, Edit, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import {
	useGetGuardrailsConfigQuery,
	useUpdateGuardrailsConfigMutation,
	GuardrailRule,
} from "@/lib/store/apis/guardrailsApi";
import { getErrorMessage } from "@/lib/store";
import { useGetPromptsQuery } from "@/lib/store/apis/promptsApi";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { MultiSelect } from "@/components/ui/multiSelect";
import {
	buildCelFromPromptSelection,
	collectGuardrailIds,
	formatLinkedProviders,
	formatRuleConnectionPreview,
	formatRuleTriggerSummary,
	GuardrailPromptScope,
	nextGuardrailId,
	parseCelPromptSelection,
	providerLabel,
} from "./utils";

function validateRuleForm(
	rule: Partial<GuardrailRule> | null,
	promptScope: GuardrailPromptScope,
	selectedPromptIds: string[],
): string | null {
	if (!rule?.name?.trim()) {
		return "Rule name is required";
	}
	if ((rule.provider_config_ids || []).length === 0) {
		return "Select at least one guardrail provider";
	}
	if (promptScope === "prompts" && selectedPromptIds.length === 0) {
		return "Select at least one prompt from the repository";
	}
	if (promptScope === "custom" && !rule.cel_expression?.trim()) {
		return "CEL expression is required for custom rules";
	}
	return null;
}

export default function GuardrailsConfigurationView() {
	const { data: config, isLoading, isError, error, refetch } = useGetGuardrailsConfigQuery();
	const { data: promptsData } = useGetPromptsQuery();
	const [updateConfig] = useUpdateGuardrailsConfigMutation();

	const [isModalOpen, setIsModalOpen] = useState(false);
	const [editingRule, setEditingRule] = useState<Partial<GuardrailRule> | null>(null);
	const [promptScope, setPromptScope] = useState<GuardrailPromptScope>("all");
	const [selectedPromptIds, setSelectedPromptIds] = useState<string[]>([]);

	const rules = config?.guardrail_rules || [];
	const providers = config?.guardrail_providers || [];
	const isEnabled = rules.length > 0 && rules.some((r) => r.enabled);

	const promptNameById = useMemo(() => {
		const map = new Map<string, string>();
		for (const prompt of promptsData?.prompts || []) {
			map.set(prompt.id, prompt.name);
		}
		return map;
	}, [promptsData?.prompts]);

	const promptOptions = useMemo(
		() =>
			(promptsData?.prompts || []).map((prompt) => ({
				label: prompt.name,
				value: prompt.id,
			})),
		[promptsData?.prompts],
	);

	const providerOptions = useMemo(
		() =>
			providers.map((provider) => ({
				label: providerLabel(provider),
				value: String(provider.id),
			})),
		[providers],
	);

	const selectedProviderIds = (editingRule?.provider_config_ids || []).map(String);
	const formError = useMemo(
		() => validateRuleForm(editingRule, promptScope, selectedPromptIds),
		[editingRule, promptScope, selectedPromptIds],
	);

	useEffect(() => {
		if (!isModalOpen || !editingRule) {
			return;
		}
		const parsed = parseCelPromptSelection(editingRule.cel_expression || "");
		setPromptScope(parsed.scope === "custom" ? "custom" : parsed.scope);
		setSelectedPromptIds(parsed.promptIds);
	}, [isModalOpen, editingRule?.id, editingRule?.cel_expression]);

	if (isLoading) return <div className="p-4">Loading guardrails configuration...</div>;

	if (isError) {
		return (
			<div className="flex flex-col items-center justify-center gap-3 p-10 text-center">
				<p className="text-destructive text-sm font-medium">Failed to load guardrails rules</p>
				{error ? <p className="text-muted-foreground max-w-md text-xs">{getErrorMessage(error)}</p> : null}
				<Button type="button" variant="outline" size="sm" onClick={() => refetch()} data-testid="guardrails-rules-retry-btn">
					Retry
				</Button>
			</div>
		);
	}

	const handleToggleGuardrails = async (checked: boolean) => {
		if (!config) return;
		const updatedRules = rules.map((r) => ({ ...r, enabled: checked }));
		try {
			await updateConfig({ ...config, guardrail_rules: updatedRules }).unwrap();
			toast.success(checked ? "All rules enabled" : "All rules disabled");
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const handleDeleteRule = async (ruleId: number) => {
		if (!config) return;
		const updatedRules = rules.filter((r) => r.id !== ruleId);
		try {
			await updateConfig({ ...config, guardrail_rules: updatedRules }).unwrap();
			toast.success("Rule deleted");
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const handleSaveRule = async () => {
		if (!config || !editingRule) return;

		const validationError = validateRuleForm(editingRule, promptScope, selectedPromptIds);
		if (validationError) {
			toast.error(validationError);
			return;
		}

		const celExpression =
			promptScope === "custom"
				? (editingRule.cel_expression || "").trim()
				: buildCelFromPromptSelection(promptScope, selectedPromptIds);

		const ruleToSave: GuardrailRule = {
			...(editingRule as GuardrailRule),
			name: editingRule.name!.trim(),
			description: editingRule.description?.trim() || "",
			cel_expression: celExpression,
			provider_config_ids: editingRule.provider_config_ids || [],
			apply_to: editingRule.apply_to || "input",
			enabled: editingRule.enabled ?? true,
		};

		let updatedRules = [...rules];
		if (editingRule.id) {
			updatedRules = updatedRules.map((r) => (r.id === editingRule.id ? ruleToSave : r));
		} else {
			updatedRules.push({
				...ruleToSave,
				id: nextGuardrailId(collectGuardrailIds(rules, providers)),
			});
		}

		try {
			await updateConfig({ ...config, guardrail_rules: updatedRules }).unwrap();
			toast.success(editingRule.id ? "Rule updated" : "Rule created");
			setIsModalOpen(false);
			setEditingRule(null);
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const openCreateRule = () => {
		setEditingRule({ apply_to: "input", provider_config_ids: [], enabled: true, cel_expression: "true" });
		setPromptScope("all");
		setSelectedPromptIds([]);
		setIsModalOpen(true);
	};

	return (
		<div className="flex h-full flex-col gap-6 p-6">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-bold tracking-tight">Guardrails Rules</h1>
					<p className="text-muted-foreground mt-1 text-sm">
						Configure CEL-based rules to scan and enforce input and output guardrails on LLM traffic.
					</p>
				</div>
				<div className="flex items-center gap-4">
					<div className="flex items-center gap-2">
						<Switch id="guardrails-enabled" checked={isEnabled} onCheckedChange={handleToggleGuardrails} />
						<Label htmlFor="guardrails-enabled" className="font-medium">
							Enable all rules
						</Label>
					</div>
					<Button onClick={openCreateRule} data-testid="guardrails-create-rule-button">
						<Plus className="mr-2 h-4 w-4" />
						Create Rule
					</Button>
				</div>
			</div>

			<div className="bg-card rounded-md border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="w-[70px]">On</TableHead>
							<TableHead>Rule Name</TableHead>
							<TableHead>Description</TableHead>
							<TableHead>Apply To</TableHead>
							<TableHead>Providers</TableHead>
							<TableHead>Applies When</TableHead>
							<TableHead className="text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{rules.length === 0 ? (
							<TableRow>
								<TableCell colSpan={7} className="text-muted-foreground h-32 text-center">
									<div className="flex flex-col items-center justify-center">
										<ShieldAlert className="text-muted-foreground/50 mb-2 h-8 w-8" />
										<p>No guardrail rules configured yet.</p>
									</div>
								</TableCell>
							</TableRow>
						) : (
							rules.map((rule) => (
								<TableRow key={rule.id}>
									<TableCell>
										<Switch
											checked={rule.enabled}
											aria-label={`Toggle rule ${rule.name}`}
											data-testid={`guardrails-rule-enabled-${rule.id}`}
											onCheckedChange={(checked) => {
												void (async () => {
													if (!config) return;
													const updatedRules = rules.map((r) => (r.id === rule.id ? { ...r, enabled: checked } : r));
													try {
														await updateConfig({ ...config, guardrail_rules: updatedRules }).unwrap();
														toast.success(checked ? "Rule enabled" : "Rule disabled");
													} catch (err) {
														toast.error(getErrorMessage(err));
													}
												})();
											}}
										/>
									</TableCell>
									<TableCell className="font-medium">{rule.name}</TableCell>
									<TableCell>{rule.description}</TableCell>
									<TableCell className="capitalize">{rule.apply_to}</TableCell>
									<TableCell>{formatLinkedProviders(rule, providers)}</TableCell>
									<TableCell className="max-w-[280px] truncate text-xs">
										{formatRuleTriggerSummary(rule.cel_expression, promptNameById)}
									</TableCell>
									<TableCell className="text-right">
										<Button
											variant="ghost"
											size="icon"
											onClick={() => {
												setEditingRule(rule);
												setIsModalOpen(true);
											}}
										>
											<Edit className="h-4 w-4" />
										</Button>
										<Button variant="ghost" size="icon" onClick={() => void handleDeleteRule(rule.id)}>
											<Trash className="text-destructive h-4 w-4" />
										</Button>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</div>

			<Dialog open={isModalOpen} onOpenChange={setIsModalOpen}>
				<DialogContent className="sm:max-w-[560px]">
					<DialogHeader>
						<DialogTitle>{editingRule?.id ? "Edit Rule" : "Create Rule"}</DialogTitle>
					</DialogHeader>
					<div className="grid gap-4 py-4">
						<div className="grid gap-2">
							<Label htmlFor="name">Rule Name</Label>
							<Input
								id="name"
								value={editingRule?.name || ""}
								onChange={(e) => setEditingRule({ ...editingRule, name: e.target.value })}
								placeholder="e.g. Block PII"
								data-testid="guardrails-rule-name-input"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="description">Description</Label>
							<Input
								id="description"
								value={editingRule?.description || ""}
								onChange={(e) => setEditingRule({ ...editingRule, description: e.target.value })}
								placeholder="e.g. Blocks sensitive information"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="apply_to">Apply To</Label>
							<Select
								value={editingRule?.apply_to || "input"}
								onValueChange={(val: "input" | "output" | "both") => setEditingRule({ ...editingRule, apply_to: val })}
							>
								<SelectTrigger data-testid="guardrails-rule-apply-to-select">
									<SelectValue placeholder="Select target phase" />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="input">Input (Prompt)</SelectItem>
									<SelectItem value="output">Output (Response)</SelectItem>
									<SelectItem value="both">Both</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-2">
							<Label>Providers</Label>
							{providerOptions.length === 0 ? (
								<p className="text-muted-foreground text-sm">Create a guardrail provider before adding rules.</p>
							) : (
								<MultiSelect
									options={providerOptions}
									defaultValue={selectedProviderIds}
									resetOnDefaultValueChange
									onValueChange={(values) =>
										setEditingRule({
											...editingRule,
											provider_config_ids: values.map((value) => Number.parseInt(value, 10)).filter(Number.isFinite),
										})
									}
									placeholder="Select providers to run when this rule matches"
									emptyIndicator="No guardrail providers configured."
									maxCount={2}
									className="border-input text-foreground hover:bg-accent hover:text-accent-foreground h-9 rounded-sm bg-transparent font-normal"
									popoverClassName="w-[var(--radix-popover-trigger-width)]"
									data-testid="guardrails-rule-providers-select"
								/>
							)}
							<p className="text-muted-foreground text-xs">
								Selected providers scan the prompt or response when the rule condition matches.
							</p>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="prompt_scope">Apply When</Label>
							<Select
								value={promptScope}
								onValueChange={(value: GuardrailPromptScope) => {
									setPromptScope(value);
									if (value === "all") {
										setSelectedPromptIds([]);
									}
								}}
							>
								<SelectTrigger data-testid="guardrails-rule-prompt-scope-select">
									<SelectValue placeholder="Choose when this rule runs" />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="all">All requests</SelectItem>
									<SelectItem value="prompts">Selected prompts from repository</SelectItem>
									<SelectItem value="custom">Custom CEL expression (advanced)</SelectItem>
								</SelectContent>
							</Select>
						</div>
						{promptScope === "prompts" && (
							<div className="grid gap-2">
								<Label>Prompts</Label>
								{promptOptions.length === 0 ? (
									<p className="text-muted-foreground text-sm">No prompts found in the repository. Create prompts first.</p>
								) : (
									<MultiSelect
										options={promptOptions}
										defaultValue={selectedPromptIds}
										resetOnDefaultValueChange
										onValueChange={setSelectedPromptIds}
										placeholder="Select prompts from repository"
										emptyIndicator="No prompts found."
										maxCount={2}
										className="border-input text-foreground hover:bg-accent hover:text-accent-foreground h-9 rounded-sm bg-transparent font-normal"
										popoverClassName="w-[var(--radix-popover-trigger-width)]"
										data-testid="guardrails-rule-prompts-select"
									/>
								)}
								<p className="text-muted-foreground text-xs">
									Rule runs only when the request uses one of these prompts via <code>x-uf-prompt-id</code> header.
								</p>
							</div>
						)}
						{promptScope === "custom" && (
							<div className="grid gap-2">
								<Label htmlFor="expression">CEL Expression</Label>
								<Textarea
									id="expression"
									value={editingRule?.cel_expression || ""}
									onChange={(e) => setEditingRule({ ...editingRule, cel_expression: e.target.value })}
									placeholder="e.g. request.model.contains('gpt-4') || request.prompt_id == 'your-prompt-id'"
									className="font-mono text-sm"
									rows={4}
									data-testid="guardrails-rule-expression-input"
								/>
								<p className="text-muted-foreground text-xs">
									Advanced mode. Variables: <code>request.model</code>, <code>request.prompt_id</code>. Use this to
									limit rules to specific models — not every model blindly.
								</p>
							</div>
						)}
						<div
							className="bg-muted/40 rounded-md border p-3 text-xs leading-relaxed"
							data-testid="guardrails-rule-connection-preview"
						>
							<p className="text-foreground font-medium">Before you save — where this connects</p>
							<p className="text-muted-foreground mt-1">
								{formatRuleConnectionPreview({
									applyTo: editingRule?.apply_to || "input",
									promptScope,
									selectedPromptIds,
									promptNameById,
									providerLabels: selectedProviderIds
										.map((id) => providers.find((p) => String(p.id) === id))
										.filter((p): p is NonNullable<typeof p> => p != null)
										.map((p) => providerLabel(p)),
								})}
							</p>
						</div>
						{formError ? <p className="text-destructive text-xs">{formError}</p> : null}
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setIsModalOpen(false)}>
							Cancel
						</Button>
						<Button
							onClick={() => void handleSaveRule()}
							disabled={!!formError || providerOptions.length === 0}
							data-testid="guardrails-rule-save-button"
						>
							Save Rule
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
