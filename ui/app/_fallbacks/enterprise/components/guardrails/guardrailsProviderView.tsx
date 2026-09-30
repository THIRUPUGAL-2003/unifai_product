import React, { useMemo, useState } from "react";
import { Plus, Trash, Edit, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import { getErrorMessage } from "@/lib/store";
import {
	useGetGuardrailsConfigQuery,
	useUpdateGuardrailsConfigMutation,
	GuardrailProvider,
} from "@/lib/store/apis/guardrailsApi";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { collectGuardrailIds, nextGuardrailId } from "./utils";

function validateProviderForm(provider: Partial<GuardrailProvider> | null, patternText: string): string | null {
	if (!provider?.id) return "Provider ID is missing";
	if (!provider.policy_name?.trim()) return "Policy name is required";
	if (!patternText.trim()) return "Add at least one regex pattern";
	return null;
}

export default function GuardrailsProviderView() {
	const { data: config, isLoading, isError, error, refetch } = useGetGuardrailsConfigQuery();
	const [updateConfig] = useUpdateGuardrailsConfigMutation();

	const [isModalOpen, setIsModalOpen] = useState(false);
	const [editingProvider, setEditingProvider] = useState<Partial<GuardrailProvider> | null>(null);

	const providers = config?.guardrail_providers || [];
	const rules = config?.guardrail_rules || [];

	const getRegexPatterns = (provider: Partial<GuardrailProvider>) => {
		const raw = provider.config?.patterns;
		if (!Array.isArray(raw)) return "";
		return raw
			.map((item) => {
				if (typeof item === "string") return item;
				if (item && typeof item === "object" && "pattern" in item) {
					return String((item as { pattern?: string }).pattern || "");
				}
				return "";
			})
			.filter((p) => p.trim() !== "")
			.join("\n");
	};

	const patternText = getRegexPatterns(editingProvider || {});
	const formError = useMemo(() => validateProviderForm(editingProvider, patternText), [editingProvider, patternText]);

	if (isLoading) return <div className="p-4">Loading providers...</div>;

	if (isError) {
		return (
			<div className="flex flex-col items-center justify-center gap-3 p-10 text-center">
				<p className="text-destructive text-sm font-medium">Failed to load guardrails providers</p>
				{error ? <p className="text-muted-foreground max-w-md text-xs">{getErrorMessage(error)}</p> : null}
				<Button type="button" variant="outline" size="sm" onClick={() => refetch()} data-testid="guardrails-providers-retry-btn">
					Retry
				</Button>
			</div>
		);
	}

	const handleDeleteProvider = async (providerId: number) => {
		if (!config) return;

		const linkedRule = rules.find((rule) => (rule.provider_config_ids || []).includes(providerId));
		if (linkedRule) {
			toast.error(`Provider is linked to rule "${linkedRule.name}". Remove it from the rule first.`);
			return;
		}

		const updatedProviders = providers.filter((p) => p.id !== providerId);
		try {
			await updateConfig({ ...config, guardrail_providers: updatedProviders }).unwrap();
			toast.success("Provider deleted");
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const handleSaveProvider = async () => {
		if (!config || !editingProvider) return;

		const validationError = validateProviderForm(editingProvider, patternText);
		if (validationError) {
			toast.error(validationError);
			return;
		}

		const providerToSave: GuardrailProvider = {
			id: editingProvider.id!,
			provider_name: "regex",
			policy_name: editingProvider.policy_name!.trim(),
			enabled: editingProvider.enabled ?? true,
			config: editingProvider.config || { patterns: [] },
		};

		let updatedProviders = [...providers];
		const existingIndex = updatedProviders.findIndex((p) => p.id === providerToSave.id);

		if (existingIndex >= 0) {
			updatedProviders[existingIndex] = providerToSave;
		} else {
			updatedProviders.push(providerToSave);
		}

		try {
			await updateConfig({ ...config, guardrail_providers: updatedProviders }).unwrap();
			toast.success(existingIndex >= 0 ? "Provider updated" : "Provider created");
			setIsModalOpen(false);
			setEditingProvider(null);
		} catch (error) {
			toast.error(getErrorMessage(error));
		}
	};

	const setRegexPatterns = (val: string) => {
		const patterns = val
			.split("\n")
			.map((p) => p.trim())
			.filter((p) => p !== "")
			.map((pattern) => ({ pattern, description: pattern, flags: "i" }));
		setEditingProvider((prev) => ({
			...prev,
			config: { ...(prev?.config || {}), patterns },
		}));
	};

	const openCreateProvider = () => {
		const nextId = nextGuardrailId(collectGuardrailIds(rules, providers));
		setEditingProvider({
			id: nextId,
			provider_name: "regex",
			policy_name: "",
			enabled: true,
			config: { patterns: [] },
		});
		setIsModalOpen(true);
	};

	const isEditingExisting = editingProvider?.id != null && providers.some((provider) => provider.id === editingProvider.id);

	return (
		<div className="flex h-full flex-col gap-6 p-6">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-bold tracking-tight">Rule Providers</h1>
					<p className="text-muted-foreground mt-1 text-sm">
						Create regex scanner providers and attach them under{" "}
						<a href="/workspace/guardrails/configuration" className="text-primary underline underline-offset-2">
							Rules
						</a>
						.
					</p>
				</div>
				<Button onClick={openCreateProvider} data-testid="guardrails-create-provider-button">
					<Plus className="mr-2 h-4 w-4" />
					Create Provider
				</Button>
			</div>

			<div className="bg-card rounded-md border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Provider ID</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Policy Name</TableHead>
							<TableHead className="text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{providers.length === 0 ? (
							<TableRow>
								<TableCell colSpan={4} className="text-muted-foreground h-32 text-center">
									<div className="flex flex-col items-center justify-center">
										<ShieldAlert className="text-muted-foreground/50 mb-2 h-8 w-8" />
										<p>No rule providers configured yet.</p>
									</div>
								</TableCell>
							</TableRow>
						) : (
							providers.map((provider) => (
								<TableRow key={provider.id}>
									<TableCell className="font-mono text-sm font-medium">{provider.id}</TableCell>
									<TableCell className="capitalize">{provider.provider_name}</TableCell>
									<TableCell>{provider.policy_name}</TableCell>
									<TableCell className="text-right">
										<Button
											variant="ghost"
											size="icon"
											onClick={() => {
												setEditingProvider(provider);
												setIsModalOpen(true);
											}}
										>
											<Edit className="h-4 w-4" />
										</Button>
										<Button variant="ghost" size="icon" onClick={() => handleDeleteProvider(provider.id)}>
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
				<DialogContent className="sm:max-w-[500px]">
					<DialogHeader>
						<DialogTitle>{isEditingExisting ? "Edit Provider" : "Create Provider"}</DialogTitle>
					</DialogHeader>
					<div className="grid gap-4 py-4">
						<div className="grid gap-2">
							<Label htmlFor="id">Provider ID</Label>
							<Input id="id" value={editingProvider?.id ?? ""} disabled className="font-mono" />
							<p className="text-muted-foreground text-xs">Auto-generated ID used when linking providers to rules.</p>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="policy_name">Policy Name</Label>
							<Input
								id="policy_name"
								value={editingProvider?.policy_name || ""}
								onChange={(e) => setEditingProvider({ ...editingProvider, policy_name: e.target.value })}
								placeholder="e.g. PAN Regex Matcher"
								data-testid="guardrails-provider-policy-name-input"
							/>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="provider_name">Type</Label>
							<Select value="regex" disabled>
								<SelectTrigger id="provider_name">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="regex">Regex matcher</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="patterns">Regex Patterns (One per line)</Label>
							<Textarea
								id="patterns"
								value={patternText}
								onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setRegexPatterns(e.target.value)}
								placeholder="[A-Z]{5}[0-9]{4}[A-Z]"
								className="font-mono text-sm"
								rows={4}
								data-testid="guardrails-provider-patterns-input"
							/>
						</div>
						{formError ? <p className="text-destructive text-xs">{formError}</p> : null}
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setIsModalOpen(false)}>
							Cancel
						</Button>
						<Button
							onClick={() => void handleSaveProvider()}
							disabled={!!formError}
							data-testid="guardrails-provider-save-button"
						>
							Save Provider
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
