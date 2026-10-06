import { isRateLimitMessage, shortenRateLimitMessage } from "@/lib/constants/logs";

const BUDGET_WARNING_PATTERN = /budget is used up|budget exceeded|budget_exceeded/i;

const PROVIDER_WARNING_PATTERNS = [
	BUDGET_WARNING_PATTERN,
	/provider api error/i,
	/http error! status:/i,
	/\bstatus\s*(?:4\d{2}|5\d{2})\b/i,
	/insufficient balance/i,
	/payment required/i,
	/terms acceptance/i,
	/model.*not found/i,
	/not found/i,
	/invalid model/i,
	/does not exist/i,
	/no such model/i,
	/unknown model/i,
	/unauthorized/i,
	/authentication fails/i,
	/permission denied/i,
];

/**
 * Playground/provider failures that should render as a soft warning (not a hard error block).
 */
export function isPromptWarningMessage(message?: string | null): boolean {
	if (!message) {
		return false;
	}
	if (isRateLimitMessage(message)) {
		return true;
	}
	return PROVIDER_WARNING_PATTERNS.some((pattern) => pattern.test(message));
}

/** User-facing copy for prompt playground warnings (rate limits + provider failures). */
export function formatPromptWarningMessage(message?: string | null): string {
	if (!message) {
		return "";
	}
	if (isRateLimitMessage(message)) {
		return shortenRateLimitMessage(message);
	}

	if (BUDGET_WARNING_PATTERN.test(message)) {
		return message;
	}

	const statusMatch = message.match(/status\s*(\d{3})/i);
	const status = statusMatch?.[1];

	if (status === "404" || /not found|no such model|unknown model|does not exist/i.test(message)) {
		return "Model or endpoint not found. Check the provider, model name, and base URL (no /v1 suffix for custom providers).";
	}
	if (status === "402" || /insufficient balance|payment required/i.test(message)) {
		return "Provider account needs credits or billing setup. Add balance on the provider dashboard or pick a free model.";
	}
	if (status === "401" || /unauthorized|authentication fails/i.test(message)) {
		return "Provider API key is missing or invalid. Add or update the key under Model Providers.";
	}
	if (status === "400" || /invalid model/i.test(message)) {
		return "Invalid request for this model. Verify the exact model ID from the provider catalog.";
	}
	if (/terms acceptance/i.test(message)) {
		return message;
	}
	if (/provider api error/i.test(message)) {
		return message.replace(/^provider api error\s*/i, "Provider request failed: ");
	}

	return message;
}

/** Formats guardrail errors into short, clean, human-friendly text without regex or technical codes. */
export function formatCleanGuardrailMessage(message?: string | null): string {
	if (!message) return "";
	let text = message.replace(/\s*\(guardrail_violation\)\s*$/i, "").trim();
	text = text.replace(/:\s*(?:input|output)?\s*matches blocked pattern:.*$/i, ": Restricted content detected.");
	text = text.replace(/matches blocked pattern:\s*\S+/i, "Restricted content detected.");
	text = text.replace(/:\s*[\^\$\[\\].*$/i, ": Restricted content detected.");
	return text;
}

