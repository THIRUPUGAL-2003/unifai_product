/** Guard Bot / Ollama defaults for Browser AI rules. Endpoint comes from env only. */

export const GUARD_BOT_OLLAMA_PROVIDER = "ollama";
export const GUARD_BOT_OLLAMA_MODEL = "llama3.2";

/** Prefer build/runtime env; never ship a customer host in source. */
export const GUARD_BOT_OLLAMA_ENDPOINT =
	(typeof process !== "undefined" &&
		(process.env.UNIFAI_OLLAMA_URL || process.env.OLLAMA_URL || process.env.BROWSER_AI_OLLAMA_URL)) ||
	"";

export const GUARD_BOT_REFERENCE_IMAGE_MAX_BYTES = 512 * 1024;
