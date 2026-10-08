import React, { useEffect } from "react";
import { Bot, FileText, Loader2, Sparkles, X, Zap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { GUARD_BOT_REFERENCE_IMAGE_MAX_BYTES } from "./browserAiConstants";
import type { GuardRuleAIEvaluatorFieldsProps } from "./browserAiTypes";
import { isDownloadGuardSource, isMultimodalGuardModel, isVisionGuardModel } from "./browserAiGuardHelpers";
import { GuardBotOutsourceModelPicker } from "./guardBotModelPickers";

export function GuardRuleAIEvaluatorFields({
	botProvider,
	botModel,
	botPrompt,
	referenceImagePreview,
	evalMode,
	generatedPattern,
	generateError,
	generating,
	onProviderChange,
	onModelChange,
	onPromptChange,
	onEvalModeChange,
	onGeneratedPatternChange,
	onGenerateRegex,
	onReferenceImageChange,
	onReferenceImageClear,
	onTestEvaluate,
	testSample,
	onTestSampleChange,
	testResult,
	testing,
	outsourceProviderOptions,
}: GuardRuleAIEvaluatorFieldsProps) {
	const visionModel = isVisionGuardModel(botModel);
	const multimodalModel = isMultimodalGuardModel(botModel);
	const activeOutsourceProvider = (() => {
		const want = (botProvider || "").trim().toLowerCase();
		const match = outsourceProviderOptions.find((o) => o.value.toLowerCase() === want);
		if (match) return match.value;
		if (isDownloadGuardSource(botProvider)) return outsourceProviderOptions[0]?.value || "";
		return botProvider || "";
	})();

	useEffect(() => {
		if (!isDownloadGuardSource(botProvider)) return;
		const first = outsourceProviderOptions[0]?.value || "";
		if (botProvider !== first) onProviderChange(first);
		if (botModel) onModelChange("");
	}, [botProvider, botModel, outsourceProviderOptions, onProviderChange, onModelChange]);

	return (
		<div className="space-y-3 rounded-lg border border-purple-900/40 bg-purple-950/20 p-3">
			<div className="flex items-center gap-1.5 text-xs font-semibold text-purple-300">
				<Bot className="h-3.5 w-3.5" />
				<span>AI Evaluator</span>
			</div>

			<p className="text-[11px] text-muted-foreground">
				Uses the Model Provider and API key configured in this Gateway. Provider and model must match that key.
			</p>

			<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
					<div className="space-y-1.5">
						<Label className="text-xs">Provider</Label>
						<Select
							value={activeOutsourceProvider}
							onValueChange={(v) => {
								onProviderChange(v);
								onModelChange("");
							}}
						>
							<SelectTrigger>
								<SelectValue placeholder="Select provider" />
							</SelectTrigger>
							<SelectContent>
								{outsourceProviderOptions.length === 0 ? (
									<SelectItem value="__none" disabled>
										No providers configured
									</SelectItem>
								) : (
									outsourceProviderOptions.map((opt) => (
										<SelectItem key={opt.value} value={opt.value}>
											{opt.label}
										</SelectItem>
									))
								)}
							</SelectContent>
						</Select>
						{outsourceProviderOptions.length === 0 ? (
							<p className="text-[11px] text-amber-400">
								Add a provider and API key under{" "}
								<a href="/workspace/providers" className="underline underline-offset-2">
									Model Providers
								</a>
								.
							</p>
						) : null}
					</div>
					<div className="space-y-1.5">
						<Label className="text-xs">Model</Label>
						<GuardBotOutsourceModelPicker
							provider={activeOutsourceProvider}
							value={botModel}
							onChange={onModelChange}
						/>
					</div>
				</div>

			<p className="text-[11px] text-muted-foreground">
				{multimodalModel
					? "Gemma 4 multimodal — evaluates prompts, extracted file text, and image uploads (PDF pages, photos)."
					: visionModel
						? "LLaVA vision model — compares uploaded PDF/image content against your policy and reference template."
						: "Text model — evaluates prompts and extracted file text."}
			</p>

			<div className="space-y-1.5">
				<Label className="text-xs">Check mode</Label>
				<div className="grid grid-cols-2 gap-2 p-1 bg-background/50 rounded-lg border border-border">
					<button
						type="button"
						onClick={() => onEvalModeChange("ai")}
						className={`flex items-center justify-center gap-1.5 py-2 px-2 rounded-md text-[11px] font-semibold transition-all ${
							evalMode === "ai" ? "bg-purple-600 text-white shadow-sm" : "text-muted-foreground hover:text-foreground"
						}`}
					>
						<Bot className="h-3.5 w-3.5" />
						AI Prompt evaluate
					</button>
					<button
						type="button"
						onClick={() => onEvalModeChange("regex")}
						className={`flex items-center justify-center gap-1.5 py-2 px-2 rounded-md text-[11px] font-semibold transition-all ${
							evalMode === "regex" ? "bg-cyan-600 text-white shadow-sm" : "text-muted-foreground hover:text-foreground"
						}`}
					>
						<Zap className="h-3.5 w-3.5" />
						Generated Regex
					</button>
				</div>
				<p className="text-[11px] text-muted-foreground">
					{evalMode === "ai"
						? "Model evaluates Browser AI chat prompts + extracted file/audio text against only the policy you write below. No built-in rules. Generate Regex is optional."
						: "Model writes a regex from your policy for fast matching. For meaning-based policies prefer AI Prompt evaluate."}
				</p>
			</div>

			<div className="space-y-1.5">
				<Label className="text-xs">Security Policy / Evaluation Instruction (Prompt)</Label>
				<Textarea
					className="font-mono text-xs"
					placeholder="Write the policy you want enforced (your words only — nothing is pre-filled)."
					value={botPrompt}
					onChange={(e) => onPromptChange(e.target.value)}
					rows={4}
				/>
				<p className="text-[11px] text-muted-foreground">
					Write clear policy English. Example: &quot;Block human names, addresses, phone numbers, PIN codes, and ATM card numbers.&quot; Category intent is matched (e.g. pin code → 613002).
				</p>
			</div>

			{evalMode === "ai" && onTestEvaluate && onTestSampleChange ? (
				<div className="space-y-1.5 rounded-md border border-border/60 bg-background/40 p-2.5">
					<Label className="text-xs">Test model evaluate (sample Browser AI prompt)</Label>
					<Textarea
						className="font-mono text-xs"
						placeholder="Paste a sample employee prompt to test against your policy"
						value={testSample || ""}
						onChange={(e) => onTestSampleChange(e.target.value)}
						rows={2}
					/>
					<div className="flex flex-wrap items-center gap-2">
						<Button
							type="button"
							variant="secondary"
							size="sm"
							className="h-8 gap-1.5 text-xs"
							disabled={testing || !botPrompt.trim() || !(testSample || "").trim()}
							onClick={onTestEvaluate}
						>
							{testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Bot className="h-3.5 w-3.5" />}
							{testing ? "Evaluating…" : "Run model evaluate"}
						</Button>
					</div>
					{testResult ? (
						<p className={`text-[11px] whitespace-pre-wrap break-words ${testResult.startsWith("OK") || testResult.startsWith("BLOCK") || testResult.startsWith("REDACT") ? "text-foreground" : "text-red-400"}`}>
							{testResult}
						</p>
					) : null}
				</div>
			) : null}

			<div className="space-y-1.5">
				<div className="flex flex-wrap items-center justify-between gap-2">
					<Label className="text-xs">{evalMode === "ai" ? "Generated Regex (optional)" : "Generated Regex"}</Label>
					<Button
						type="button"
						variant="outline"
						size="sm"
						className="h-8 gap-1.5 text-xs"
						disabled={generating || !botPrompt.trim()}
						onClick={onGenerateRegex}
					>
						{generating ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
						{generating ? "Generating…" : "Generate Regex from policy"}
					</Button>
				</div>
				<Input
					className="font-mono text-xs"
					placeholder="Click Generate — or paste/edit a regex here"
					value={generatedPattern}
					onChange={(e) => onGeneratedPatternChange(e.target.value)}
				/>
				{generateError ? <p className="text-[11px] text-red-400">{generateError}</p> : null}
				<p className="text-[11px] text-muted-foreground">
					{evalMode === "ai"
						? "AI Prompt mode does not need a regex — Save works with your policy alone. Generate is only if you also want a fast pattern."
						: "Review/edit before save. Saving in Generated Regex mode creates a fast Regex rule."}
				</p>
			</div>

			{(visionModel || referenceImagePreview) && evalMode === "ai" && (
				<div className="space-y-1.5">
					<Label className="text-xs">Reference Template Image {visionModel ? "" : "(optional)"}</Label>
					<div className="flex flex-wrap items-start gap-3">
						<label className="border-input bg-background hover:bg-muted/40 inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-xs font-medium">
							<FileText className="h-3.5 w-3.5" />
							Upload template
							<input
								type="file"
								accept="image/png,image/jpeg,image/webp,image/gif"
								className="sr-only"
								onChange={(e) => {
									const file = e.target.files?.[0];
									if (file) onReferenceImageChange(file);
									e.target.value = "";
								}}
							/>
						</label>
						{referenceImagePreview ? (
							<div className="relative">
								<img
									src={referenceImagePreview}
									alt="Reference template preview"
									className="border-border h-20 w-auto max-w-[160px] rounded border object-contain"
								/>
								<Button
									type="button"
									variant="destructive"
									size="icon"
									className="absolute -top-2 -right-2 h-6 w-6"
									onClick={onReferenceImageClear}
								>
									<X className="h-3 w-3" />
								</Button>
							</div>
						) : null}
					</div>
					<p className="text-[11px] text-muted-foreground">
						Stored in the rule (DB) as a small reference only — not employee uploads. Max{" "}
						{Math.round(GUARD_BOT_REFERENCE_IMAGE_MAX_BYTES / 1024)} KB.
					</p>
				</div>
			)}
		</div>
	);
}
