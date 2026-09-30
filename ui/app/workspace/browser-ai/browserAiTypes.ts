import type { HostRole } from "./relatedHosts";

export type RelatedHostEntry = { host: string; role: HostRole };

export type GuardRuleAction = "BLOCK" | "REDACT" | "WARN";

export type GuardRuleSeverity = "CRITICAL" | "HIGH" | "MEDIUM";

export type GuardRuleNoticeCopy = {
	label: string;
	placeholder: string;
	hint: string;
	listLabel: string;
};

export type SecurityVerdictTone = "ok" | "bad" | "warn" | "neutral";

export type SecurityVerdict = {
	title: string;
	detail: string;
	tone: SecurityVerdictTone;
};

export type PlatformBadgeInfo = {
	label: string;
	className: string;
};

/** Minimal log fields for site-block detection and action badges. */
export type SiteBlockLogFields = {
	user_prompt_preview?: string;
	user_prompt_full?: string;
	rule_triggered?: string;
	predicted_category?: string;
};

export type LogActionBadgeFields = SiteBlockLogFields & {
	action?: string;
};

export type GuardBotModelPickerProps = {
	provider: string;
	value: string;
	onChange: (model: string) => void;
	disabled?: boolean;
};

export type GuardRuleEvalMode = "ai" | "regex";

export type GuardRuleAIEvaluatorFieldsProps = {
	botProvider: string;
	botModel: string;
	botPrompt: string;
	referenceImagePreview: string;
	evalMode: GuardRuleEvalMode;
	generatedPattern: string;
	generateError?: string;
	generating?: boolean;
	onProviderChange: (value: string) => void;
	onModelChange: (value: string) => void;
	onPromptChange: (value: string) => void;
	onEvalModeChange: (mode: GuardRuleEvalMode) => void;
	onGeneratedPatternChange: (value: string) => void;
	onGenerateRegex: () => void;
	onReferenceImageChange: (file: File) => void;
	onReferenceImageClear: () => void;
	onTestEvaluate?: () => void;
	testSample?: string;
	onTestSampleChange?: (value: string) => void;
	testResult?: string;
	testing?: boolean;
	outsourceProviderOptions: { label: string; value: string }[];
};
