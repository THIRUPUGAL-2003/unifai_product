import { Message, type CompletionUsage, type MessageContent, type ToolCall, type VariableMap, replaceVariablesInMessages } from "@/lib/message";
import { emitStudio, extractFileRequest, lastFile, runDocumentStudio } from "./fileStudio";
import { getErrorMessage } from "@/lib/store";
import type { ModelParams } from "@/lib/types/prompts";
import { transcribeAudioFile, voiceTranscriptAttachment } from "./transcribeAudio";
import { ocrImageFile, ocrTextAttachment } from "./ocrImage";

export interface ExecutionConfig {
	provider: string;
	model: string;
	modelParams: ModelParams;
	apiKeyId: string;
	virtualKeyId?: string;
	variables?: VariableMap;
	customHeaders?: Record<string, string>;
	/** Optional Skills Repository body injected as a system message for this run. */
	skillSystemPrompt?: string;
	/** Prefer server-side inject via x-uf-skill-id when set. */
	skillId?: string;
	/** Prompt Repository id — stamps x-uf-prompt-id so Guardrails prompt-scoped rules match. */
	promptId?: string;
	/** Ticked MCP tools (`client-tool`). Omit to allow every tool on the virtual key. */
	mcpIncludeTools?: string[];
	/** Internal retry flag when upstream model does not support image input (error 3051) */
	_retryWithoutImages?: boolean;
}

function getBaseUrl() {
	// Same-origin (vite proxies /v1 + /api in dev) so session cookies and
	// x-uf-mcp-* headers work without CORS breakage.
	return "";
}

function buildHeaders(config: Pick<ExecutionConfig, "apiKeyId" | "virtualKeyId" | "customHeaders" | "skillId" | "promptId" | "mcpIncludeTools">): Record<string, string> {
	const headers: Record<string, string> = { "Content-Type": "application/json" };
	if (config.virtualKeyId && config.virtualKeyId !== "__auto__" && config.virtualKeyId.startsWith("sk-uf-")) {
		headers["Authorization"] = `Bearer ${config.virtualKeyId}`;
	}
	if (config.apiKeyId && config.apiKeyId !== "__auto__" && !config.apiKeyId.startsWith("sk-uf-")) {
		headers["x-uf-api-key-id"] = config.apiKeyId;
	}
	// Playground always opts into MCP tool injection so it still works when
	// MCP Settings has "disable auto tool inject" enabled. Virtual-key grants
	// (and allow_on_all clients) still gate which tools are actually usable.
	headers["x-uf-mcp-include-clients"] = "*";
	if (config.mcpIncludeTools) {
		headers["x-uf-mcp-include-tools"] = config.mcpIncludeTools.join(",");
	}
	if (config.skillId?.trim()) {
		headers["x-uf-skill-id"] = config.skillId.trim();
	}
	if (config.promptId?.trim()) {
		// The playground already sends the whole conversation; only tag the request with the prompt.
		headers["x-uf-prompt-id"] = config.promptId.trim();
		headers["x-uf-prompt-inject"] = "false";
	}
	if (config.customHeaders) {
		const reserved = new Set([
			"content-type",
			"authorization",
			"x-uf-api-key-id",
			"x-uf-mcp-include-clients",
			"x-uf-mcp-include-tools",
			"x-uf-skill-id",
			"x-uf-prompt-id",
			"x-uf-prompt-version",
			"x-uf-prompt-inject",
		]);
		for (const [name, value] of Object.entries(config.customHeaders)) {
			const trimmedName = name.trim();
			const trimmedValue = value.trim();
			if (!trimmedName || !trimmedValue) continue;
			if (reserved.has(trimmedName.toLowerCase())) {
				console.warn(`Ignoring custom header "${trimmedName}" — reserved by the playground.`);
				continue;
			}
			headers[trimmedName] = trimmedValue;
		}
	}
	return headers;
}

function formatPlaygroundError(raw: string, status?: number): string {
	const text = (raw || "").trim();
	const lower = text.toLowerCase();

	if (!text || lower.includes("failed to fetch") || lower === "networkerror when attempting to fetch resource." || lower.includes("networkerror")) {
		return "Network error talking to the gateway (not your laptop offline). Check VPN/proxy, gateway URL, and that the provider accepts this audio/file format.";
	}
	if (lower.includes("audio") && (lower.includes("format") || lower.includes("unsupported") || lower.includes("invalid"))) {
		return `${text} — Voice was converted to WAV when possible. Prefer an audio-capable model, or attach a wav/mp3 file.`;
	}
	if (lower.includes("vision") || (lower.includes("image") && lower.includes("not support")) || lower.includes("3051") || lower.includes("image input is not enabled")) {
		return `${text} — Select a vision-capable model (e.g. Pixtral, GPT-4o, Claude 3) for direct image inputs, or OCR extracted text will be used automatically.`;
	}
	if (lower.includes("guardrail")) {
		let cleaned = text.replace(/\s*\(guardrail_violation\)\s*$/i, "").trim();
		cleaned = cleaned.replace(/:\s*(?:input|output)?\s*matches blocked pattern:.*$/i, ": Restricted content detected.");
		cleaned = cleaned.replace(/matches blocked pattern:\s*\S+/i, "Restricted content detected.");
		cleaned = cleaned.replace(/:\s*[\^\$\[\\].*$/i, ": Restricted content detected.");
		return cleaned.startsWith("Guardrail") ? cleaned : `Guardrail blocked this request: ${cleaned}`;
	}
	if (lower.includes("budget is used up")) {
		return text;
	}
	if (status === 402 || lower.includes("budget exceeded") || lower.includes("budget_exceeded") || lower.includes("insufficient balance") || lower.includes("payment required")) {
		if (lower.includes("team budget exceeded")) {
			return `Team Budget Exhausted: ${text}. Please contact your Team Administrator or Workspace Admin to increase the Team budget.`;
		}
		if (lower.includes("customer budget exceeded")) {
			return `Customer Budget Exhausted: ${text}. Please contact your Customer Administrator to replenish the customer budget.`;
		}
		if (lower.includes("user budget exceeded")) {
			return `User Budget Exhausted: ${text}. Your personal user budget allocation has been reached.`;
		}
		return `Budget is used up: ${text}. Please select another virtual key or contact your administrator.`;
	}
	if (status === 401 || status === 403 || lower.includes("unauthorized") || lower.includes("authentication fails") || lower.includes("permission denied")) {
		return `Authentication failed (${status || 401}): Virtual Key or provider credentials are invalid, expired, or lack permission. Check your assigned key in Settings.`;
	}
	if (status === 404 || lower.includes("no such model") || lower.includes("unknown model") || lower.includes("model not found")) {
		return `Model or provider endpoint not found (${status || 404}): ${text}. Check the provider and model configured in Settings.`;
	}
	if (status === 429 || lower.includes("rate limit") || lower.includes("too many requests")) {
		return `Rate limit reached (${status || 429}): ${text}. Please wait a moment before sending another request.`;
	}
	if (status === 413 || lower.includes("too large") || lower.includes("payload")) {
		return "Attachment or request is too large for this gateway/provider. Use a smaller file (max ~20 MB) or extract text and paste it.";
	}
	if (status && status >= 500) {
		return `Upstream provider error (${status}): The AI provider service is temporarily unavailable (${text}). Please retry shortly.`;
	}
	return text;
}

function parseErrorPayload(data: unknown, fallback: string): string {
	if (!data || typeof data !== "object") return fallback;
	const root = data as Record<string, unknown>;
	const err = root.error;
	if (typeof err === "string" && err.trim()) return err.trim();
	if (err && typeof err === "object") {
		const e = err as Record<string, unknown>;
		const message = typeof e.message === "string" ? e.message : "";
		const nested = typeof e.error === "string" ? e.error : "";
		const code = typeof e.code === "string" ? e.code : typeof e.type === "string" ? e.type : "";
		const base = (message || nested || "").trim() || fallback;
		if (code && code !== "guardrail_violation" && !base.toLowerCase().includes(code.toLowerCase())) {
			return `${base} (${code})`;
		}
		return base;
	}
	if (typeof root.message === "string" && root.message.trim()) return root.message.trim();
	return fallback;
}

export interface ExecutionCallbacks {
	onStreamingStart: (allMessages: Message[], placeholder: Message) => void;
	onStreamChunk: (content: string) => void;
	onComplete: (content: string, usage?: CompletionUsage) => void;
	onToolCallComplete: (content: string, toolCalls: ToolCall[], usage?: CompletionUsage) => void;
	onEmptyResponse: () => void;
	onError: (error: string) => void;
	onFinally: () => void;
}

async function enrichVoiceWithWhisper(messages: Message[], apiKeyId: string, signal?: AbortSignal): Promise<Message[]> {
	const out: Message[] = [];
	for (const msg of messages) {
		const attachments = msg.attachments;
		if (!attachments.some((a) => a.type === "input_audio" && a.input_audio?.data)) {
			out.push(msg);
			continue;
		}
		const nextAttachments: MessageContent[] = [];
		let changed = false;
		for (const part of attachments) {
			if (part.type !== "input_audio" || !part.input_audio?.data) {
				nextAttachments.push(part);
				continue;
			}
			const format = part.input_audio.format || "wav";
			const mime = format === "mp3" ? "audio/mpeg" : `audio/${format}`;
			try {
				const binary = Uint8Array.from(atob(part.input_audio.data), (c) => c.charCodeAt(0));
				const file = new File([binary], `voice.${format}`, { type: mime });
				const transcript = await transcribeAudioFile(file, { apiKeyId, signal });
				if (transcript?.text) {
					nextAttachments.push(voiceTranscriptAttachment(`voice.${format}`, transcript.text));
					changed = true;
					continue;
				}
			} catch {
				/* keep raw audio */
			}
			nextAttachments.push(part);
		}
		if (!changed) {
			out.push(msg);
			continue;
		}
		const clone = msg.clone();
		clone.attachments = nextAttachments;
		out.push(clone);
	}
	return out;
}

/**
 * Background OCR extraction safeguard before execution:
 * Ensures any image attachment has its text extracted so text-only and non-vision
 * models can accurately respond to image receipts/documents without error 3051.
 */
async function enrichImagesWithOcr(
	messages: Message[],
	opts?: { apiKeyId?: string; virtualKeyId?: string; signal?: AbortSignal },
): Promise<Message[]> {
	const out: Message[] = [];
	for (const msg of messages) {
		const attachments = msg.attachments;
		const imageAttachments = attachments.filter((a) => a.type === "image_url" && a.image_url?.url);
		if (imageAttachments.length === 0) {
			out.push(msg);
			continue;
		}

		// Detect if an OCR text part already exists for attached images
		const existingFilenames = new Set(
			attachments
				.filter((a) => a.type === "text" && a.text && a.text.includes("--- OCR extracted content ---"))
				.map((a) => {
					const m = a.text?.match(/^Attached file:\s*([^\n]+)/i);
					return m?.[1]?.trim().toLowerCase() || "";
				})
				.filter(Boolean),
		);

		let changed = false;
		const nextAttachments = [...attachments];

		for (const img of imageAttachments) {
			const fn = (img.image_url?.filename || "image").toLowerCase();
			if (existingFilenames.has(fn)) continue;

			const url = img.image_url?.url || "";
			if (!url || !url.startsWith("data:")) continue;

			try {
				const ocr = await ocrImageFile(url, { apiKeyId: opts?.apiKeyId, virtualKeyId: opts?.virtualKeyId, signal: opts?.signal });
				if (ocr?.text?.trim()) {
					nextAttachments.push(ocrTextAttachment(img.image_url?.filename || "image", ocr.text.trim()));
					existingFilenames.add(fn);
					changed = true;
				}
			} catch (err) {
				console.warn("Background OCR fallback failed during prompt execution:", err);
			}
		}

		if (!changed) {
			out.push(msg);
			continue;
		}

		const clone = msg.clone();
		clone.attachments = nextAttachments;
		out.push(clone);
	}
	return out;
}

interface ResolvedTarget {
	model: string;
	provider: string;
	cleanPrompt: string;
	candidates: string[];
}

/** Resolves the target image model across all supported providers (OpenAI, Gemini, Replicate, Bedrock, xAI, Runway, etc.) */
function resolveImageModel(prompt: string, currentProvider: string, currentModel: string): ResolvedTarget {
	let cleanPrompt = prompt;
	let preferredModel = "";
	let preferredProvider = currentProvider;

	// Check if user explicitly mentioned a model or provider in the prompt
	const explicitMatch = cleanPrompt.match(/(?:\s+(?:using|with|via|in|on)\s+(dall-?e(?:-3|-2)?|imagen(?:-3)?|flux(?:-schnell|-dev|-pro)?|stable-?diffusion|sdxl|titan|grok|runway|runware|fireworks|together))/i);
	if (explicitMatch) {
		const keyword = explicitMatch[1].toLowerCase().replace(/\s+/g, "");
		cleanPrompt = cleanPrompt.replace(explicitMatch[0], "").trim();
		if (keyword.includes("dall")) {
			preferredModel = keyword.includes("2") ? "openai/dall-e-2" : "openai/dall-e-3";
			preferredProvider = "openai";
		} else if (keyword.includes("imagen")) {
			preferredModel = "gemini/imagen-3.0-generate-001";
			preferredProvider = "gemini";
		} else if (keyword.includes("flux")) {
			preferredModel = "replicate/black-forest-labs/flux-schnell";
			preferredProvider = "replicate";
		} else if (keyword.includes("titan")) {
			preferredModel = "bedrock/amazon.titan-image-generator-v1";
			preferredProvider = "bedrock";
		} else if (keyword.includes("stable") || keyword.includes("sdxl")) {
			preferredModel = "bedrock/stability.stable-diffusion-xl-v1";
			preferredProvider = "bedrock";
		} else if (keyword.includes("grok")) {
			preferredModel = "xai/grok-2-image";
			preferredProvider = "xai";
		} else if (keyword.includes("runway")) {
			preferredModel = "runway/gen-3-alpha";
			preferredProvider = "runway";
		} else if (keyword.includes("runware")) {
			preferredModel = "runware/runware:100@1";
			preferredProvider = "runware";
		}
	}

	const prov = (currentProvider || "").toLowerCase();
	const mod = (currentModel || "").toLowerCase();

	if (!preferredModel) {
		if (prov === "openai" || mod.includes("gpt") || mod.includes("dall")) {
			preferredModel = mod.includes("dall") ? `${prov}/${currentModel}` : "openai/dall-e-3";
			preferredProvider = "openai";
		} else if (prov === "gemini" || prov === "vertex" || mod.includes("gemini") || mod.includes("imagen")) {
			preferredModel = mod.includes("imagen") ? `${prov}/${currentModel}` : "gemini/imagen-3.0-generate-001";
			preferredProvider = "gemini";
		} else if (prov === "replicate" || mod.includes("flux")) {
			preferredModel = mod.includes("flux") ? `${prov}/${currentModel}` : "replicate/black-forest-labs/flux-schnell";
			preferredProvider = "replicate";
		} else if (prov === "bedrock" || mod.includes("titan")) {
			preferredModel = "bedrock/amazon.titan-image-generator-v1";
			preferredProvider = "bedrock";
		} else if (prov === "xai" || mod.includes("grok")) {
			preferredModel = "xai/grok-2-image";
			preferredProvider = "xai";
		} else if (prov === "runway") {
			preferredModel = "runway/gen-3-alpha";
			preferredProvider = "runway";
		} else if (prov === "runware") {
			preferredModel = "runware/runware:100@1";
			preferredProvider = "runware";
		} else if (prov === "fireworks") {
			preferredModel = "fireworks/accounts/fireworks/models/flux-1-schnell";
			preferredProvider = "fireworks";
		} else if (prov === "together") {
			preferredModel = "together/black-forest-labs/FLUX.1-schnell";
			preferredProvider = "together";
		} else {
			// Current provider is text-only (e.g. mistral, anthropic, deepseek).
			// Use gateway cross-provider routing with standard defaults
			preferredModel = "gemini/imagen-3.0-generate-001";
			preferredProvider = "gemini";
		}
	}

	const allImageModels = [
		preferredModel,
		"gemini/imagen-3.0-generate-001",
		"openai/dall-e-3",
		"replicate/black-forest-labs/flux-schnell",
		"bedrock/amazon.titan-image-generator-v1",
		"xai/grok-2-image",
	];
	const candidates = Array.from(new Set(allImageModels.filter(Boolean)));

	return { model: preferredModel, provider: preferredProvider, cleanPrompt, candidates };
}

/** Resolves the target video model across all supported providers (Gemini/Veo, Runway, Luma, Kling, Sora, Replicate) */
function resolveVideoModel(prompt: string, currentProvider: string, currentModel: string): ResolvedTarget {
	let cleanPrompt = prompt;
	let preferredModel = "";
	let preferredProvider = currentProvider;

	const explicitMatch = cleanPrompt.match(/(?:\s+(?:using|with|via|in|on)\s+(veo(?:-2|-3)?|runway|gen-3|luma|dream-machine|kling|sora|replicate|cogvideo))/i);
	if (explicitMatch) {
		const keyword = explicitMatch[1].toLowerCase().replace(/\s+/g, "");
		cleanPrompt = cleanPrompt.replace(explicitMatch[0], "").trim();
		if (keyword.includes("veo")) {
			preferredModel = "gemini/veo-2.0-generate-001";
			preferredProvider = "gemini";
		} else if (keyword.includes("runway") || keyword.includes("gen-3")) {
			preferredModel = "runway/gen-3-alpha";
			preferredProvider = "runway";
		} else if (keyword.includes("luma") || keyword.includes("dream")) {
			preferredModel = "luma/dream-machine";
			preferredProvider = "luma";
		} else if (keyword.includes("kling")) {
			preferredModel = "kling/kling-v1";
			preferredProvider = "kling";
		} else if (keyword.includes("sora")) {
			preferredModel = "openai/sora";
			preferredProvider = "openai";
		} else if (keyword.includes("replicate") || keyword.includes("cogvideo")) {
			preferredModel = "replicate/lucataco/animate-diff";
			preferredProvider = "replicate";
		}
	}

	const prov = (currentProvider || "").toLowerCase();
	const mod = (currentModel || "").toLowerCase();

	if (!preferredModel) {
		if (prov === "runway" || mod.includes("runway") || mod.includes("gen-3")) {
			preferredModel = "runway/gen-3-alpha";
			preferredProvider = "runway";
		} else if (prov === "gemini" || prov === "vertex" || mod.includes("gemini") || mod.includes("veo")) {
			preferredModel = "gemini/veo-2.0-generate-001";
			preferredProvider = "gemini";
		} else if (prov === "luma" || mod.includes("luma")) {
			preferredModel = "luma/dream-machine";
			preferredProvider = "luma";
		} else if (prov === "kling" || mod.includes("kling")) {
			preferredModel = "kling/kling-v1";
			preferredProvider = "kling";
		} else if (prov === "openai" || mod.includes("sora")) {
			preferredModel = "openai/sora";
			preferredProvider = "openai";
		} else if (prov === "replicate") {
			preferredModel = "replicate/lucataco/animate-diff";
			preferredProvider = "replicate";
		} else {
			preferredModel = "gemini/veo-2.0-generate-001";
			preferredProvider = "gemini";
		}
	}

	const allVideoModels = [
		preferredModel,
		"gemini/veo-2.0-generate-001",
		"runway/gen-3-alpha",
		"luma/dream-machine",
		"kling/kling-v1",
		"replicate/lucataco/animate-diff",
	];
	const candidates = Array.from(new Set(allVideoModels.filter(Boolean)));

	return { model: preferredModel, provider: preferredProvider, cleanPrompt, candidates };
}

/** Detects if a user message is explicitly asking to generate an image */
function extractImageGenerationPrompt(text: string): string | null {
	const t = (text || "").trim();
	if (!t) return null;
	const patterns = [
		/^(?:please\s+)?(?:generate|create|draw|make|paint)\s+(?:an?\s+)?image\s+(?:of|for|showing|depicting)?\s*:?\s*(.+)$/i,
		/^(?:please\s+)?(?:generate|create|draw|make|paint)\s+(?:a\s+)?picture\s+(?:of|for|showing|depicting)?\s*:?\s*(.+)$/i,
		/^(?:image|picture)\s+(?:generation|generate)\s*:?\s*(.+)$/i,
		/^(?:image|picture|படம்)\s*(?:generate|create|வரைஞ்சு|பண்ணு)\s*:?\s*(.+)$/i,
		/^draw\s+(?:me\s+)?(?:an?\s+)?(.+)$/i,
	];
	for (const p of patterns) {
		const m = t.match(p);
		if (m && m[1]?.trim()) {
			const candidate = m[1].trim();
			if (/(?:in\s+(?:react|python|css|html|javascript|typescript|code)|component|function|code)/i.test(candidate)) {
				continue;
			}
			return candidate;
		}
	}
	return null;
}

/** Detects if a user message is explicitly asking to generate a video */
function extractVideoGenerationPrompt(text: string): string | null {
	const t = (text || "").trim();
	if (!t) return null;
	const patterns = [
		/^(?:please\s+)?(?:generate|create|make)\s+(?:a\s+)?video\s+(?:of|for|showing|depicting)?\s*:?\s*(.+)$/i,
		/^video\s+(?:generation|generate)\s*:?\s*(.+)$/i,
		/^(?:video|வீடியோ)\s*(?:generate|create|பண்ணு)\s*:?\s*(.+)$/i,
	];
	for (const p of patterns) {
		const m = t.match(p);
		if (m && m[1]?.trim()) {
			const candidate = m[1].trim();
			if (/(?:in\s+(?:react|python|css|html|javascript|typescript|code)|component|function|code|player)/i.test(candidate)) {
				continue;
			}
			return candidate;
		}
	}
	return null;
}

export async function executePrompt(
	currentMessages: Message[],
	pendingMessage: Message | undefined,
	config: ExecutionConfig,
	callbacks: ExecutionCallbacks,
	signal?: AbortSignal,
) {
	let allMessages: Message[];
	if (pendingMessage) {
		allMessages = [...currentMessages, pendingMessage];
	} else {
		allMessages = [...currentMessages];
	}

	const placeholder = Message.response("");
	callbacks.onStreamingStart(allMessages, placeholder);

	let resolvedMessages = config.variables ? replaceVariablesInMessages(allMessages, config.variables) : allMessages;
	const skillPrompt = !config.skillId?.trim() ? config.skillSystemPrompt?.trim() : "";
	if (skillPrompt) {
		resolvedMessages = [Message.system(skillPrompt), ...resolvedMessages];
	}

	try {
		resolvedMessages = await enrichVoiceWithWhisper(resolvedMessages, config.apiKeyId, signal);
		resolvedMessages = await enrichImagesWithOcr(resolvedMessages, {
			apiKeyId: config.apiKeyId,
			virtualKeyId: config.virtualKeyId,
			signal,
		});
		const headers = buildHeaders(config);

		// Check for Image / Video generation intent from the latest user message
		const lastUserMsg = [...resolvedMessages].reverse().find((m) => m.role === "user");
		const userText = lastUserMsg?.content || "";

		const imagePrompt = extractImageGenerationPrompt(userText);
		if (imagePrompt && !config._retryWithoutImages) {
			const imageTarget = resolveImageModel(imagePrompt, config.provider, config.model);
			callbacks.onStreamChunk(`🎨 Generating image for: **"${imageTarget.cleanPrompt}"**...\n\nTrying model: \`${imageTarget.model}\`...`);

			let lastError = "";
			let generatedImageUrl = "";
			let usedModel = "";

			for (const candidateModel of imageTarget.candidates) {
				try {
					callbacks.onStreamChunk(`🎨 Requesting image generation with **${candidateModel}** for: *"${imageTarget.cleanPrompt}"*...`);
					const imgRes = await fetch(`${getBaseUrl()}/v1/images/generations`, {
						method: "POST",
						headers,
						credentials: "include",
						signal,
						body: JSON.stringify({
							prompt: imageTarget.cleanPrompt,
							model: candidateModel,
							n: 1,
							size: "1024x1024",
						}),
					});

					if (imgRes.ok) {
						const imgData = await imgRes.json();
						const item = imgData.data?.[0];
						const url = item?.url || (item?.b64_json ? `data:image/png;base64,${item.b64_json}` : "");
						if (url) {
							generatedImageUrl = url;
							usedModel = candidateModel;
							break;
						}
					} else {
						let errText = `HTTP ${imgRes.status}`;
						try {
							const errData = await imgRes.json();
							errText = parseErrorPayload(errData, errText);
						} catch {}
						lastError = `${candidateModel}: ${errText}`;
						console.warn(`Image generation with ${candidateModel} failed:`, errText);
						if (imageTarget.candidates.length === 1) break;
					}
				} catch (err: any) {
					lastError = `${candidateModel}: ${err?.message || "Network error"}`;
					console.warn(`Image generation attempt error:`, err);
					if (imageTarget.candidates.length === 1) break;
				}
			}

			if (generatedImageUrl) {
				callbacks.onComplete(
					`Here is your generated image for **"${imageTarget.cleanPrompt}"**:\n\n` +
					`![${imageTarget.cleanPrompt}](${generatedImageUrl})\n\n` +
					`*(Generated via **${usedModel}**)*`
				);
				callbacks.onFinally();
				return;
			}

			callbacks.onComplete(
				`### 🎨 Image Generation Status\n\n` +
				`**Prompt:** *"${imageTarget.cleanPrompt}"*\n\n` +
				`⚠️ **Image generation could not be completed:**\n${lastError || "No active image generation provider credentials found"}\n\n` +
				`💡 **Supported Image Models across Providers:**\n` +
				`• **Google Gemini:** \`gemini/imagen-3.0-generate-001\`\n` +
				`• **OpenAI:** \`openai/dall-e-3\`, \`openai/dall-e-2\`\n` +
				`• **Replicate (Flux / SDXL):** \`replicate/black-forest-labs/flux-schnell\`\n` +
				`• **AWS Bedrock:** \`bedrock/amazon.titan-image-generator-v1\`, \`bedrock/stability.stable-diffusion-xl-v1\`\n` +
				`• **xAI:** \`xai/grok-2-image\`\n` +
				`• **Runway / Runware / Fireworks / Together**\n\n` +
				`👉 **Fix:** Add an API Key for any of the above image providers under **Workspace > Providers**!`
			);
			callbacks.onFinally();
			return;
		}

		const videoPrompt = extractVideoGenerationPrompt(userText);
		if (videoPrompt && !config._retryWithoutImages) {
			const videoTarget = resolveVideoModel(videoPrompt, config.provider, config.model);
			callbacks.onStreamChunk(`🎥 Submitting video generation request for: **"${videoTarget.cleanPrompt}"**...\n\nTrying model: \`${videoTarget.model}\`...`);

			let lastVidError = "";
			let generatedVideoUrl = "";
			let generatedVideoId = "";
			let usedVidModel = "";

			for (const candidateModel of videoTarget.candidates) {
				try {
					callbacks.onStreamChunk(`🎥 Requesting video generation with **${candidateModel}** for: *"${videoTarget.cleanPrompt}"*...`);
					const vidRes = await fetch(`${getBaseUrl()}/v1/videos/generations`, {
						method: "POST",
						headers,
						credentials: "include",
						signal,
						body: JSON.stringify({
							prompt: videoTarget.cleanPrompt,
							model: candidateModel,
						}),
					});

					if (vidRes.ok) {
						const vidData = await vidRes.json();
						const url = vidData.videos?.[0]?.url || vidData.video_url || "";
						const id = vidData.id || "";
						if (url || id) {
							generatedVideoUrl = url;
							generatedVideoId = id;
							usedVidModel = candidateModel;
							break;
						}
					} else {
						let errText = `HTTP ${vidRes.status}`;
						try {
							const errData = await vidRes.json();
							errText = parseErrorPayload(errData, errText);
						} catch {}
						lastVidError = `${candidateModel}: ${errText}`;
						console.warn(`Video generation with ${candidateModel} failed:`, errText);
						if (videoTarget.candidates.length === 1) break;
					}
				} catch (err: any) {
					lastVidError = `${candidateModel}: ${err?.message || "Network error"}`;
					console.warn(`Video generation attempt error:`, err);
					if (videoTarget.candidates.length === 1) break;
				}
			}

			if (generatedVideoUrl) {
				callbacks.onComplete(
					`Here is your generated video for **"${videoTarget.cleanPrompt}"**:\n\n` +
					`<video controls playsinline src="${generatedVideoUrl}" style="max-width: 100%; border-radius: 8px; margin-top: 8px;"></video>\n\n` +
					`*(Generated via **${usedVidModel}**)*`
				);
				callbacks.onFinally();
				return;
			} else if (generatedVideoId) {
				callbacks.onComplete(
					`### 🎥 Video Generation Started!\n\n` +
					`**Model:** \`${usedVidModel}\`\n` +
					`**Job ID:** \`${generatedVideoId}\`\n` +
					`**Prompt:** *"${videoTarget.cleanPrompt}"*\n\n` +
					`The video is being rendered. You can view the live progress and watch the generated video in **Workspace > Logs**!`
				);
				callbacks.onFinally();
				return;
			}

			callbacks.onComplete(
				`### 🎥 Video Generation Status\n\n` +
				`**Prompt:** *"${videoTarget.cleanPrompt}"*\n\n` +
				`⚠️ **Video generation could not be completed:**\n${lastVidError || "No active video generation provider credentials found"}\n\n` +
				`💡 **Supported Video Models across Providers:**\n` +
				`• **Google Gemini / Veo:** \`gemini/veo-2.0-generate-001\`\n` +
				`• **Runway:** \`runway/gen-3-alpha\`\n` +
				`• **Luma AI:** \`luma/dream-machine\`\n` +
				`• **Kling AI:** \`kling/kling-v1\`\n` +
				`• **OpenAI Sora:** \`openai/sora\`\n` +
				`• **Replicate:** \`replicate/lucataco/animate-diff\`\n\n` +
				`👉 **Fix:** Add an API Key for any of the above video providers under **Workspace > Providers**!`
			);
			callbacks.onFinally();
			return;
		}

		const fileRequest = extractFileRequest(userText);
		if (fileRequest && !config._retryWithoutImages) {
			const context = resolvedMessages
				.slice(-6)
				.map((m) => `${m.role}: ${(m.content || "").slice(0, 4000)}`)
				.join("\n\n")
				.slice(-12000);
			try {
				callbacks.onStreamChunk("Writing the file in the code panel…");
				const filename = await runDocumentStudio({
					kind: fileRequest.kind,
					instruction: fileRequest.instruction,
					context,
					revise: fileRequest.revise,
					previous: fileRequest.revise ? lastFile() : null,
					askModel: async (system, user) => {
						const res = await fetch(`${getBaseUrl()}/v1/chat/completions`, {
							method: "POST",
							headers,
							credentials: "include",
							signal,
							body: JSON.stringify({
								model: `${config.provider}/${config.model}`,
								messages: [
									{ role: "system", content: system },
									{ role: "user", content: user },
								],
								stream: false,
							}),
						});
						if (!res.ok) {
							let errText = `HTTP ${res.status}`;
							try {
								errText = parseErrorPayload(await res.json(), errText);
							} catch {}
							throw new Error(errText);
						}
						const data = await res.json();
						return String(data.choices?.[0]?.message?.content || "");
					},
				});
				callbacks.onComplete(`The file is ready and the download has started.\n\n**${filename}**\n\nThe model wrote the code in the side panel.`);
			} catch (err) {
				const message = err instanceof Error ? err.message : String(err);
				emitStudio({ phase: "error", log: message });
				callbacks.onComplete(`The file could not be built.\n\n${message}`);
			}
			callbacks.onFinally();
			return;
		}

		const {
			api_key_id: _,
			skill_id: _skill,
			prompt_allowed_providers: _providers,
			prompt_allowed_models: _models,
			prompt_allowed_virtual_keys: _vks,
			prompt_allowed_api_keys: _apiKeys,
			prompt_allowed_mcp_tools: _mcpTools,
			_retryWithoutImages: retryWithoutImages,
			...rawParams
		} = config.modelParams || {};

		let concreteModel = config.model;
		if (!concreteModel || concreteModel === "*") {
			if (config.provider === "openrouter") concreteModel = "aion-labs/aion-2.0";
			else if (config.provider === "openai") concreteModel = "gpt-4o-mini";
			else if (config.provider === "anthropic") concreteModel = "claude-3-5-haiku-20241022";
		}

		// If retrying without images due to upstream non-vision error (3051),
		// force vision capability to false so toAPIMessages sends pure text
		const effectiveModel = retryWithoutImages ? "text-fallback" : concreteModel;

		const payload: Record<string, any> = {
			model: `${config.provider}/${concreteModel}`,
			messages: Message.toAPIMessages(resolvedMessages, effectiveModel, config.provider),
			...rawParams,
			stream: rawParams.stream !== false,
		};

		if (rawParams.temperature !== undefined && rawParams.temperature !== null && !isNaN(Number(rawParams.temperature))) {
			payload.temperature = Number(rawParams.temperature);
		}
		const rawMaxTokens = rawParams.max_tokens ?? 4096;
		if (rawMaxTokens !== undefined && rawMaxTokens !== null && !isNaN(Number(rawMaxTokens))) {
			const tokens = Math.max(1, Math.round(Number(rawMaxTokens)));
			payload.max_tokens = tokens;
			payload.max_completion_tokens = tokens;
		}

		// Reasoning models (e.g. OpenAI o1, o3, o4) reject the 'temperature' parameter in chat completions API
		const isReasoningModel = /(^|\/)(o1|o3|o4)(-|$)/i.test(config.model);
		if (isReasoningModel) {
			delete payload.temperature;
		}

		const response = await fetch(`${getBaseUrl()}/v1/chat/completions`, {
			method: "POST",
			headers,
			credentials: "include",
			signal,
			body: JSON.stringify(payload),
		});

		if (!response.ok) {
			let errorMessage = `HTTP error! status: ${response.status}`;
			try {
				const data = await response.json();
				errorMessage = parseErrorPayload(data, errorMessage);
			} catch (error) {
				console.error("Failed to parse error response:", error);
			}

			// If upstream rejected image input (code 3051 or similar non-vision error),
			// transparently retry without raw image_url blocks using the background-extracted OCR text!
			const lowerErr = errorMessage.toLowerCase();
			const isImageInputError =
				(response.status === 400 || response.status === 422) &&
				(lowerErr.includes("3051") ||
					lowerErr.includes("image input is not enabled") ||
					lowerErr.includes("not support image") ||
					lowerErr.includes("does not support image") ||
					lowerErr.includes("image_url") ||
					lowerErr.includes("multimodal") ||
					(lowerErr.includes("vision") && lowerErr.includes("not")) ||
					(lowerErr.includes("image") && lowerErr.includes("not supported")));

			if (isImageInputError && !config._retryWithoutImages) {
				console.warn("Provider rejected image input (3051). Transparently retrying with OCR extracted text...");
				return executePrompt(
					currentMessages,
					pendingMessage,
					{ ...config, _retryWithoutImages: true },
					callbacks,
					signal,
				);
			}

			throw new Error(formatPlaygroundError(errorMessage, response.status));
		}

		const contentType = response.headers.get("content-type") || "";
		const isStreamResponse = contentType.includes("text/event-stream");

		if (!isStreamResponse) {
			const data = await response.json();
			const content = data.choices?.[0]?.message?.content ?? "";
			const toolCalls = data.choices?.[0]?.message?.tool_calls as ToolCall[] | undefined;
			const usage = data.usage as CompletionUsage | undefined;
			if (toolCalls && toolCalls.length > 0) {
				callbacks.onToolCallComplete(content, toolCalls, usage);
			} else if (content) {
				callbacks.onComplete(content, usage);
			} else {
				callbacks.onEmptyResponse();
			}
		} else {
			const reader = response.body?.getReader();
			if (!reader) throw new Error("No response body");

			const decoder = new TextDecoder();
			let assistantContent = "";
			let streamUsage: CompletionUsage | undefined;
			const toolCallsMap = new Map<number, ToolCall>();
			let buffer = "";

			while (true) {
				const { done, value } = await reader.read();
				if (done) break;

				buffer += decoder.decode(value, { stream: true });
				const lines = buffer.split("\n");
				buffer = lines.pop() ?? "";

				for (const line of lines) {
					const trimmed = line.trim();
					if (!trimmed.startsWith("data: ")) continue;
					const data = trimmed.slice(6);
					if (data === "[DONE]") continue;

					let parsed: Record<string, unknown>;
					try {
						parsed = JSON.parse(data) as Record<string, unknown>;
					} catch {
						continue;
					}

					if (parsed.error) {
						throw new Error(formatPlaygroundError(parseErrorPayload(parsed, "Stream error"), response.status));
					}

					const choices = parsed.choices as Array<{ delta?: { content?: string; tool_calls?: unknown } }> | undefined;
					const delta = choices?.[0]?.delta;

					if (parsed.usage) {
						streamUsage = parsed.usage as CompletionUsage;
					}

					const content = delta?.content;
					if (content) {
						assistantContent += content;
						callbacks.onStreamChunk(assistantContent);
					}

					const deltaToolCalls = delta?.tool_calls as Array<{
						index: number;
						id?: string;
						type?: string;
						function?: { name?: string; arguments?: string };
					}>;
					if (deltaToolCalls) {
						for (const dtc of deltaToolCalls) {
							const idx = dtc.index;
							const existing = toolCallsMap.get(idx);
							if (existing) {
								if (dtc.function?.arguments) {
									existing.function.arguments += dtc.function.arguments;
								}
							} else {
								toolCallsMap.set(idx, {
									type: "function",
									id: dtc.id ?? "",
									function: {
										name: dtc.function?.name ?? "",
										arguments: dtc.function?.arguments ?? "",
									},
								});
							}
						}
					}
				}
			}

			const toolCalls = Array.from(toolCallsMap.values());
			if (toolCalls.length > 0) {
				callbacks.onToolCallComplete(assistantContent, toolCalls, streamUsage);
			} else if (assistantContent) {
				callbacks.onComplete(assistantContent, streamUsage);
			} else {
				callbacks.onEmptyResponse();
			}
		}
	} catch (err) {
		if (err instanceof DOMException && err.name === "AbortError") {
			// User cancelled — no error to display
		} else {
			callbacks.onError(formatPlaygroundError(getErrorMessage(err)));
		}
	} finally {
		callbacks.onFinally();
	}
}

export class MCPAuthRequiredError extends Error {
	kind: "oauth" | "headers";
	mcpClientName: string;
	authorizeUrl: string;

	constructor(opts: { kind: "oauth" | "headers"; mcpClientName: string; authorizeUrl: string; message: string }) {
		super(opts.message);
		this.name = "MCPAuthRequiredError";
		this.kind = opts.kind;
		this.mcpClientName = opts.mcpClientName;
		this.authorizeUrl = opts.authorizeUrl;
	}
}

export async function executeToolCall(toolCall: ToolCall, config: Pick<ExecutionConfig, "apiKeyId" | "virtualKeyId" | "customHeaders">): Promise<string> {
	const headers = buildHeaders(config);

	const response = await fetch(`${getBaseUrl()}/v1/mcp/tool/execute`, {
		method: "POST",
		headers,
		credentials: "include",
		body: JSON.stringify({
			id: toolCall.id,
			type: toolCall.type,
			index: 0,
			function: {
				name: toolCall.function.name,
				arguments: toolCall.function.arguments,
			},
		}),
	});

	if (!response.ok) {
		let errorMessage = `HTTP error! status: ${response.status}`;
		try {
			const data = await response.json();
			errorMessage = parseErrorPayload(data, errorMessage);

			const authRequired = (data as { extra_fields?: { mcp_auth_required?: Record<string, string> } }).extra_fields?.mcp_auth_required;
			if (authRequired) {
				throw new MCPAuthRequiredError({
					kind: (authRequired.kind as "oauth" | "headers") || "oauth",
					mcpClientName: authRequired.mcp_client_name || "MCP server",
					authorizeUrl: authRequired.authorize_url || authRequired.submit_url || "",
					message: authRequired.message || errorMessage,
				});
			}
		} catch (e) {
			if (e instanceof MCPAuthRequiredError) throw e;
		}
		throw new Error(formatPlaygroundError(errorMessage, response.status));
	}

	const data = await response.json();
	return typeof data.content === "string" ? data.content : JSON.stringify(data.content);
}
