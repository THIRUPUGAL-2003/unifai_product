import { isRateLimitMessage, shortenRateLimitMessage } from "@/lib/constants/logs";

const PROVIDER_WARNING_PATTERNS = [
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
