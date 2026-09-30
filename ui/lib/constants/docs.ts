const DOCS_BASE = "https://docs.unifai.ai";

/** Canonical documentation URLs used across the workspace UI. */
export const DOCS = {
	home: DOCS_BASE,
	quickStart: `${DOCS_BASE}/quickstart/gateway/setting-up`,
	architecture: `${DOCS_BASE}/architecture`,
	providerConfiguration: `${DOCS_BASE}/quickstart/gateway/provider-configuration`,
	contributing: `${DOCS_BASE}/contributing/setting-up-repo`,
	benchmarking: `${DOCS_BASE}/benchmarking/getting-started`,
	mcp: `${DOCS_BASE}/features/mcp`,
	governance: `${DOCS_BASE}/features/governance`,
	plugins: `${DOCS_BASE}/plugins/getting-started`,
	unifiedInterface: `${DOCS_BASE}/features/unified-interface`,
	dropInReplacement: `${DOCS_BASE}/features/drop-in-replacement`,
	complexityRouter: `${DOCS_BASE}/features/governance`,
	virtualKeys: `${DOCS_BASE}/features/governance`,
} as const;
