/**
 * Builds a JavaScript RegExp that behaves like the gateway's RE2 guard matcher:
 * always case-insensitive, leading inline flag groups such as (?i), (?s), (?m), (?is)
 * hoisted into JS flags, and Python/RE2 named groups (?P<name>…) rewritten to (?<name>…).
 * Throws for constructs RE2 rejects (lookaround, backreferences) or invalid syntax.
 */
export function re2ToJsRegExp(pattern: string): RegExp {
	let body = pattern.trim();
	const flags = new Set<string>(["i"]);
	for (;;) {
		const m = /^\(\?([a-zA-Z-]+)\)/.exec(body);
		if (!m) break;
		const [enabled] = m[1].split("-");
		for (const f of enabled.toLowerCase()) {
			if (f === "s" || f === "m") flags.add(f);
		}
		body = body.slice(m[0].length);
	}
	if (/\(\?<?[=!]|\\[1-9]/.test(body)) {
		throw new Error("Lookahead/lookbehind/backreferences are not supported by the gateway regex engine (RE2)");
	}
	if (/\\[pP]\{|\[\[:/.test(body)) {
		throw new Error("\\p{…} and [[:class:]] are not supported by the browser proxy — use explicit classes like [A-Za-z] or [0-9]");
	}
	body = body.replace(/\(\?P</g, "(?<");
	return new RegExp(body, [...flags].join(""));
}
