import { Message, type CompletionUsage, type MessageContent, type ToolCall, type VariableMap, replaceVariablesInMessages } from "@/lib/message";
import { getErrorMessage } from "@/lib/store";
import type { ModelParams } from "@/lib/types/prompts";
import { transcribeAudioFile, voiceTranscriptAttachment } from "./transcribeAudio";

export interface ExecutionConfig {
	provider: string;
	model: string;
	modelParams: ModelParams;
	apiKeyId: string;
	variables?: VariableMap;
	customHeaders?: Record<string, string>;
	/** Optional Skills Repository body injected as a system message for this run. */
	skillSystemPrompt?: string;
	/** Prefer server-side inject via x-uf-skill-id when set. */
	skillId?: string;
	/** Prompt Repository id — stamps x-uf-prompt-id so Guardrails prompt-scoped rules match. */
	promptId?: string;
}

function getBaseUrl() {
	// Same-origin (vite proxies /v1 + /api in dev) so session cookies and
	// x-uf-mcp-* headers work without CORS breakage.
	return "";
}

function buildHeaders(config: Pick<ExecutionConfig, "apiKeyId" | "customHeaders" | "skillId" | "promptId">): Record<string, string> {
	const headers: Record<string, string> = { "Content-Type": "application/json" };
	if (config.apiKeyId && config.apiKeyId !== "__auto__") {
		if (config.apiKeyId.startsWith("sk-uf-")) {
			headers["Authorization"] = `Bearer ${config.apiKeyId}`;
		} else {
			headers["x-uf-api-key-id"] = config.apiKeyId;
		}
	}
	// Playground always opts into MCP tool injection so it still works when
	// MCP Settings has "disable auto tool inject" enabled. Virtual-key grants
	// (and allow_on_all clients) still gate which tools are actually usable.
	headers["x-uf-mcp-include-clients"] = "*";
	if (config.skillId?.trim()) {
		headers["x-uf-skill-id"] = config.skillId.trim();
	}
	if (config.promptId?.trim()) {
		headers["x-uf-prompt-id"] = config.promptId.trim();
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
	if (lower.includes("vision") || (lower.includes("image") && lower.includes("not support"))) {
		return `${text} — Select a vision-capable model for images.`;
	}
	if (lower.includes("guardrail")) {
		return text.startsWith("Guardrail") ? text : `Guardrail blocked this request: ${text}`;
	}
	if (status === 413 || lower.includes("too large") || lower.includes("payload")) {
		return "Attachment or request is too large for this gateway/provider. Use a smaller file (max ~20 MB) or extract text and paste it.";
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
		if (code && !base.toLowerCase().includes(code.toLowerCase())) {
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
		const headers = buildHeaders(config);

		const { api_key_id: _, ...requestParams } = config.modelParams;
		const response = await fetch(`${getBaseUrl()}/v1/chat/completions`, {
			method: "POST",
			headers,
			credentials: "include",
			signal,
			body: JSON.stringify({
				model: `${config.provider}/${config.model}`,
				messages: Message.toAPIMessages(resolvedMessages),
				...requestParams,
				stream: requestParams.stream,
			}),
		});

		if (!response.ok) {
			let errorMessage = `HTTP error! status: ${response.status}`;
			try {
				const data = await response.json();
				errorMessage = parseErrorPayload(data, errorMessage);
			} catch (error) {
				console.error("Failed to parse error response:", error);
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

export async function executeToolCall(toolCall: ToolCall, config: Pick<ExecutionConfig, "apiKeyId" | "customHeaders">): Promise<string> {
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
