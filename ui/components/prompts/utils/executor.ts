import { Message, type CompletionUsage, type MessageContent, type ToolCall, type VariableMap, replaceVariablesInMessages } from "@/lib/message";
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

		// If retrying without images due to upstream non-vision error (3051),
		// force vision capability to false so toAPIMessages sends pure text
		const effectiveModel = retryWithoutImages ? "text-fallback" : config.model;

		const payload: Record<string, any> = {
			model: `${config.provider}/${config.model}`,
			messages: Message.toAPIMessages(resolvedMessages, effectiveModel, config.provider),
			...rawParams,
			stream: rawParams.stream !== false,
		};

		if (rawParams.temperature !== undefined && rawParams.temperature !== null && !isNaN(Number(rawParams.temperature))) {
			payload.temperature = Number(rawParams.temperature);
		}
		if (rawParams.max_tokens !== undefined && rawParams.max_tokens !== null && !isNaN(Number(rawParams.max_tokens))) {
			const tokens = Math.max(1, Math.round(Number(rawParams.max_tokens)));
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
				response.status === 400 &&
				(lowerErr.includes("3051") ||
					lowerErr.includes("image input is not enabled") ||
					lowerErr.includes("not support image") ||
					lowerErr.includes("does not support image"));

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
