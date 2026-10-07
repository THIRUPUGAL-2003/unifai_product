import { PRODUCT_NAME } from "@/lib/constants/config";
import {
	GUARD_BOT_OLLAMA_PROVIDER,
	GUARD_BOT_REFERENCE_IMAGE_MAX_BYTES,
} from "./browserAiConstants";

const PRODUCT_TAG = PRODUCT_NAME.toUpperCase();
import type { GuardRuleAction, GuardRuleNoticeCopy } from "./browserAiTypes";

export function isDownloadGuardSource(provider: string): boolean {
	const p = (provider || "").trim().toLowerCase();
	return !p || p === GUARD_BOT_OLLAMA_PROVIDER;
}

export function guardRuleNoticeCopy(action: GuardRuleAction): GuardRuleNoticeCopy {
	if (action === "BLOCK") {
		return {
			label: "Block message (shown when rejected)",
			placeholder: "Message employees see when this rule blocks their prompt...",
			hint: "Shown in chat when the rule blocks. Leave blank for the default block message.",
			listLabel: "Block message",
		};
	}
	if (action === "WARN") {
		return {
			label: "Warning message (shown in chat)",
			placeholder: "Warning employees see when this rule matches…",
			hint: `Prompt is still sent; this notice is appended as [${PRODUCT_TAG} WARNING]. Leave blank for the default warning.`,
			listLabel: "Warning message",
		};
	}
	return {
		label: "Redaction notice (appended in chat)",
		placeholder: "Notice appended when this rule redacts…",
		hint: `Prompt is still sent; this notice is appended as [${PRODUCT_TAG} REDACTED]. Leave blank for the default notice.`,
		listLabel: "Redaction notice",
	};
}

export function guardRuleActionHint(action: GuardRuleAction): string {
	if (action === "BLOCK") {
		return "Stops the prompt or file send and shows a block message.";
	}
	if (action === "WARN") {
		return "Allows send; shows a warning notice only (no block).";
	}
	return `Allows send; appends a [${PRODUCT_TAG} REDACTED] notice in chat.`;
}

export function isMultimodalGuardModel(model: string): boolean {
	const m = (model || "").toLowerCase();
	return m.includes("gemma4") || m.includes("gemma-4");
}

export function isVisionOnlyGuardModel(model: string): boolean {
	const m = (model || "").toLowerCase();
	if (isMultimodalGuardModel(m)) return false;
	return m.includes("llava") || m.includes("vision") || m.includes("bakllava");
}

export function isVisionGuardModel(model: string): boolean {
	return isVisionOnlyGuardModel(model) || isMultimodalGuardModel(model);
}

export function referenceImageDataUrl(base64: string, contentType = "image/png"): string {
	if (!base64) return "";
	if (base64.startsWith("data:")) return base64;
	return `data:${contentType || "image/png"};base64,${base64}`;
}

export async function readReferenceImageFile(file: File): Promise<{ data: string; type: string }> {
	if (!file.type.startsWith("image/")) {
		throw new Error("Reference template must be an image (PNG, JPG, WebP).");
	}
	if (file.size > GUARD_BOT_REFERENCE_IMAGE_MAX_BYTES) {
		throw new Error(`Reference image must be under ${Math.round(GUARD_BOT_REFERENCE_IMAGE_MAX_BYTES / 1024)} KB.`);
	}
	const dataUrl = await new Promise<string>((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(String(reader.result || ""));
		reader.onerror = () => reject(new Error("Failed to read image file."));
		reader.readAsDataURL(file);
	});
	const comma = dataUrl.indexOf("base64,");
	const data = comma >= 0 ? dataUrl.slice(comma + 7) : dataUrl;
	return { data, type: file.type || "image/png" };
}
