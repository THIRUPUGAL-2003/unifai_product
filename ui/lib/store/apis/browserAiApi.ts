import { baseApi } from "./baseApi";

export interface BrowserAILogEntry {
	id: string;
	timestamp: string;
	platform: string;
	domain?: string;
	user_prompt_preview: string;
	user_prompt_full: string;
	est_tokens: number;
	client_ip: string;
	agent_id?: string;
	agent_hostname?: string;
	status: string;
	action: string;
	rule_triggered?: string;
	risk_score?: number;
	predictive_risk?: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL";
	predicted_category?: string;
	reply_bot_provider?: string;
	reply_bot_model?: string;
	reply_bot_text?: string;
	attachment_name?: string;
	attachment_stored_name?: string;
	attachment_content_type?: string;
	metadata?: string;
	created_at: string;
}

export interface BrowserAISearchLogEntry {
	id: string;
	timestamp: string;
	engine: string;
	browser: string;
	is_incognito: boolean;
	query: string;
	clicked_url?: string;
	clicked_title?: string;
	url: string;
	host: string;
	client_ip: string;
	agent_hostname?: string;
	agent_id?: string;
	risk_score?: number;
	predictive_risk?: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL";
	risk_category?: string;
	created_at: string;
}

export interface BrowserAIAgent {
	id: string;
	hostname: string;
	username: string;
	contact_email?: string;
	contact_email_pinned?: boolean;
	ip_address: string;
	mac_address?: string;
	transport_name?: string;
	os_version: string;
	agent_version: string;
	/** endpoint = laptop Guard; network = shared/server proxy — same dashboard */
	agent_type?: "endpoint" | "network" | string;
	health_status?: string;
	health_detail?: string;
	status: "active" | "uninstalled" | "uninstall_pending" | string;
	uninstall_requested?: boolean;
	/** True when this Guard has an auto-generated per-device uninstall key. */
	has_uninstall_key?: boolean;
	uninstall_key_rotated_at?: string;
	last_seen_at: string;
	installed_at: string;
	/** First heartbeat on the current agent_version (fresh install or auto-update). */
	version_updated_at?: string;
	/** SHA-256 of the server-published proxy code this Guard runs ("" = code built into its installer). */
	proxy_bundle_sha?: string;
	proxy_bundle_updated_at?: string;
	uninstalled_at?: string;
	created_at: string;
	updated_at: string;
}

/** Proxy code published by Rebuild & Publish; installed Guards hot-swap to it. */
export interface BrowserAIProxyBundle {
	sha256: string;
	published_at: string;
	files: number;
	size: number;
	guard_versions: string[];
}

/** Guard packages the server currently serves to laptops (auto-update source). */
export interface BrowserAISetupInfo {
	version: string;
	mac_version: string;
	windows_ready: boolean;
	macos_ready: boolean;
	windows_built_at?: string;
	macos_built_at?: string;
	can_rebuild: boolean;
	proxy_source_available?: boolean;
	proxy_bundle?: BrowserAIProxyBundle;
}

export interface BrowserAIAgentInsightStats {
	agent_id: string;
	allowed_count: number;
	blocked_count: number;
	warn_count: number;
	redact_count: number;
	total_count: number;
}

export interface BrowserAIAgentSettings {
	id: string;
	require_uninstall_key: boolean;
	key_configured: boolean;
	updated_at: string;
	updated_by: string;
}

/** Company-wide Guard defaults stored in Postgres (same DB as Browser AI). */
export interface BrowserGuardFleetConfig {
	id?: string;
	default_proxy_addr?: string;
	pac_advertise_addr?: string;
	server_mode_policy?: string;
	listen_host_policy?: string;
	pac_sync_seconds?: number;
	agent_type_default?: string;
	backend_url_hint?: string;
	notes?: string;
	updated_at?: string;
	updated_by?: string;
}

export interface BrowserGuardRule {
	id: string;
	name: string;
	rule_type?: "regex" | "ai_bot";
	bot_provider?: string;
	bot_model?: string;
	bot_prompt?: string;
	bot_reference_image?: string;
	bot_reference_image_type?: string;
	severity: "CRITICAL" | "HIGH" | "MEDIUM";
	action: "BLOCK" | "WARN" | "REDACT";
	pattern?: string;
	active: boolean;
	description: string;
	warning_message?: string;
	created_at?: string;
}

export interface BrowserControlSettings {
	id: string;
	enabled: boolean;
	block_upload: boolean;
	upload_warning?: string;
	/** When true, search logs older than retention are purged automatically. */
	search_log_auto_delete?: boolean;
	/** Retention window: 1d | 7d | 30d | 90d | 180d | 365d */
	search_log_retention?: "1d" | "7d" | "30d" | "90d" | "180d" | "365d" | string;
	/** When true, prompt logs older than retention are purged automatically. */
	prompt_log_auto_delete?: boolean;
	/** Retention window: 1d | 7d | 30d | 90d | 180d | 365d */
	prompt_log_retention?: "1d" | "7d" | "30d" | "90d" | "180d" | "365d" | string;
	updated_at?: string;
}

export interface BrowserGuardRebuildLog {
	id: string;
	status: "success" | "failed" | string;
	mode: string;
	version: string;
	mac_version: string;
	windows_ready: boolean;
	macos_ready: boolean;
	bundle_sha: string;
	bundle_files: number;
	guard_versions: string;
	message: string;
	log: string;
	requested_by: string;
	duration_ms: number;
	created_at: string;
}

export interface BrowserAITargetImportResult {
	status: string;
	imported: number;
	updated: number;
	skipped: number;
	total: number;
	already_exists?: number;
	duplicates_in_file?: number;
	nested?: number;
	failed?: number;
	errors?: string[];
}

export interface BrowserTargetWebsite {
	id: string;
	domain: string;
	platform_name: string;
	monitored: boolean;
	/** When true, Guard blocks opening the whole website (not only prompts). */
	block_site?: boolean;
	intercepted_count: number;
	status: string;
	reply_bot_enabled?: boolean;
	reply_bot_provider?: string;
	reply_bot_model?: string;
	/** "violations" (default) | "all" */
	reply_bot_mode?: string;
	/** Related host nested under this parent Target Website. Empty = top-level domain. */
	parent_id?: string;
	/** Admin label: ui | chat | file | empty (auto). Tells Guard how to intercept this host. */
	host_role?: "ui" | "chat" | "file" | "" | string;
	created_at?: string;
}

export const browserAiApi = baseApi.injectEndpoints({
	endpoints: (builder) => ({
		getBrowserAiLogs: builder.query<
			{ logs: BrowserAILogEntry[]; total: number; limit: number; offset: number },
			{ platform?: string; status?: string; action?: string; search?: string; limit?: number; offset?: number } | void
		>({
			query: (params) => ({
				url: "/browser-ai/logs",
				params: params || {},
			}),
			providesTags: ["BrowserAiLogs" as any],
		}),

		getBrowserAiLogStats: builder.query<
			{ total: number; blocked: number; warned: number; high_risk: number; avg_risk: number },
			void
		>({
			query: () => ({ url: "/browser-ai/logs/stats" }),
			providesTags: ["BrowserAiLogs" as any],
		}),

		clearBrowserAiLogs: builder.mutation<
			{ status?: string; message?: string } | void,
			{ period?: "1d" | "7d" | "30d" | "all"; date?: string } | void
		>({
			query: (params) => ({
				url: "/browser-ai/logs",
				method: "DELETE",
				params: params || {},
			}),
			invalidatesTags: ["BrowserAiLogs" as any, "BrowserAiInsightStats" as any],
		}),

		deleteBrowserAiLogs: builder.mutation<{ status: string; deleted: number }, { ids: string[] }>({
			query: (body) => ({
				url: "/browser-ai/logs/bulk-delete",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiLogs" as any, "BrowserAiInsightStats" as any],
		}),

		getBrowserAiSearchLogs: builder.query<
			{
				logs: BrowserAISearchLogEntry[];
				total: number;
				incognito_count: number;
				queries_count: number;
				clicks_count: number;
				limit: number;
				offset: number;
			},
			{ engine?: string; browser?: string; is_incognito?: string; search?: string; limit?: number; offset?: number } | void
		>({
			query: (params) => ({
				url: "/browser-ai/search-logs",
				params: params || {},
			}),
			providesTags: ["BrowserAiSearchLogs" as any],
		}),

		clearBrowserAiSearchLogs: builder.mutation<
			{ status?: string; message?: string } | void,
			{ period?: "1d" | "7d" | "30d" | "all"; date?: string } | void
		>({
			query: (params) => ({
				url: "/browser-ai/search-logs",
				method: "DELETE",
				params: params || {},
			}),
			invalidatesTags: ["BrowserAiSearchLogs" as any, "BrowserAiInsightStats" as any],
		}),

		deleteBrowserAiSearchLogs: builder.mutation<{ status: string; deleted: number }, { ids: string[] }>({
			query: (body) => ({
				url: "/browser-ai/search-logs/bulk-delete",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiSearchLogs" as any, "BrowserAiInsightStats" as any],
		}),

		recordBrowserAiSearchLog: builder.mutation<{ status: string; log: BrowserAISearchLogEntry }, Partial<BrowserAISearchLogEntry>>({
			query: (body) => ({
				url: "/browser-ai/search-logs",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiSearchLogs" as any],
		}),

		getBrowserAiRules: builder.query<{ rules: BrowserGuardRule[] }, void>({
			query: () => "/browser-ai/rules",
			providesTags: ["BrowserAiRules" as any],
		}),

		getBrowserAiOllamaModels: builder.query<{ models: string[]; base_url?: string }, void>({
			query: () => "/browser-ai/ollama-models",
		}),

		createBrowserAiRule: builder.mutation<{ status: string; rule: BrowserGuardRule }, Partial<BrowserGuardRule>>({
			query: (body) => ({
				url: "/browser-ai/rules",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiRules" as any],
		}),

		importBrowserAiRules: builder.mutation<
			{
				status: string;
				imported: number;
				updated: number;
				skipped: number;
				total: number;
				already_exists?: number;
				duplicates_in_file?: number;
				failed?: number;
				errors?: string[];
			},
			{ rules: Partial<BrowserGuardRule>[]; overwrite?: boolean }
		>({
			// Prefer bulk POST /rules/import. If the gateway build is older (405/404 from
			// PUT /rules/{id} path collision), fall back to per-rule create/update so Import
			// still works as a product feature without waiting on redeploy.
			async queryFn(arg, _api, _extraOptions, baseQuery) {
				const bulk = await baseQuery({
					url: "/browser-ai/rules/import",
					method: "POST",
					body: arg,
				});
				if (!bulk.error) {
					return { data: bulk.data as { status: string; imported: number; updated: number; skipped: number; total: number; errors?: string[] } };
				}
				const status = (bulk.error as { status?: number })?.status;
				if (status !== 405 && status !== 404) {
					return { error: bulk.error };
				}

				const listRes = await baseQuery({ url: "/browser-ai/rules" });
				if (listRes.error) {
					return { error: listRes.error };
				}
				const listBody = listRes.data as { rules?: BrowserGuardRule[] } | BrowserGuardRule[];
				const existing = Array.isArray(listBody) ? listBody : listBody?.rules || [];
				const byName = new Map(
					existing.map((r) => [String(r.name || "").trim().toLowerCase(), r] as const),
				);

				let imported = 0;
				let updated = 0;
				let skipped = 0;
				const errors: string[] = [];
				const overwrite = Boolean(arg.overwrite);
				const reason = (err: unknown) => {
					const d = (err as { data?: { error?: { message?: string } | string; message?: string } })?.data;
					const e = d?.error;
					return (typeof e === "object" && e?.message) || (typeof e === "string" && e) || d?.message || "";
				};

				for (const rule of arg.rules || []) {
					const name = String(rule.name || "").trim();
					if (!name) {
						skipped += 1;
						errors.push("Missing rule name");
						continue;
					}
					const rt = String(rule.rule_type || "regex").toLowerCase();
					if (rt === "ai_bot") {
						skipped += 1;
						errors.push(`${name}: AI Guard Bot rules cannot be imported from Excel`);
						continue;
					}
					const key = name.toLowerCase();
					const found = byName.get(key);
					if (found?.id) {
						if (!overwrite) {
							skipped += 1;
							continue;
						}
						const upd = await baseQuery({
							url: `/browser-ai/rules/${encodeURIComponent(found.id)}`,
							method: "PUT",
							body: {
								name,
								rule_type: "regex",
								pattern: rule.pattern,
								severity: rule.severity,
								action: rule.action,
								warning_message: rule.warning_message,
								description: rule.description,
								active: rule.active ?? true,
							},
						});
						if (upd.error) {
							errors.push(`${name}: update failed${reason(upd.error) ? ` (${reason(upd.error)})` : ""}`);
							continue;
						}
						updated += 1;
					} else {
						const cre = await baseQuery({
							url: "/browser-ai/rules",
							method: "POST",
							body: {
								name,
								rule_type: "regex",
								pattern: rule.pattern,
								severity: rule.severity,
								action: rule.action,
								warning_message: rule.warning_message,
								description: rule.description,
								active: rule.active ?? true,
							},
						});
						if (cre.error) {
							errors.push(`${name}: create failed${reason(cre.error) ? ` (${reason(cre.error)})` : ""}`);
							continue;
						}
						const created = (cre.data as { rule?: BrowserGuardRule })?.rule;
						if (created?.id) {
							byName.set(key, created);
						}
						imported += 1;
					}
				}

				return {
					data: {
						status: "success",
						imported,
						updated,
						skipped,
						total: (arg.rules || []).length,
						errors: errors.length ? errors : undefined,
					},
				};
			},
			invalidatesTags: ["BrowserAiRules" as any],
		}),

		updateBrowserAiRule: builder.mutation<{ status: string }, { id: string; updates: Partial<BrowserGuardRule> }>({
			query: ({ id, updates }) => ({
				url: `/browser-ai/rules/${id}`,
				method: "PUT",
				body: updates,
			}),
			invalidatesTags: ["BrowserAiRules" as any],
		}),

		deleteBrowserAiRule: builder.mutation<{ status: string }, string>({
			query: (id) => ({
				url: `/browser-ai/rules/${id}`,
				method: "DELETE",
			}),
			invalidatesTags: ["BrowserAiRules" as any],
		}),

		testBrowserAiGuardBot: builder.mutation<
			{
				status: string;
				violation: boolean;
				security_verdict: string;
				security_message: string;
				security_met: boolean;
				would_block: boolean;
				would_warn: boolean;
				eval_error?: string;
				model_raw?: string;
				rule_name?: string;
				bot_provider?: string;
				bot_model?: string;
			},
			{
				bot_provider: string;
				bot_model: string;
				bot_prompt: string;
				sample_prompt: string;
				action?: string;
				name?: string;
			}
		>({
			query: (body) => ({
				url: "/browser-ai/rules/test-bot",
				method: "POST",
				body,
			}),
		}),

		generateBrowserAiRegexFromPolicy: builder.mutation<
			{
				status: string;
				pattern: string;
				focus?: string;
				notes?: string;
				model?: string;
				provider?: string;
			},
			{
				bot_provider: string;
				bot_model: string;
				bot_prompt: string;
			}
		>({
			query: (body) => ({
				url: "/browser-ai/rules/generate-regex",
				method: "POST",
				body,
			}),
		}),

		getBrowserAiControls: builder.query<{ controls: BrowserControlSettings }, void>({
			query: () => "/browser-ai/controls",
			providesTags: ["BrowserAiControls" as any],
		}),

		updateBrowserAiControls: builder.mutation<{ status: string; controls: BrowserControlSettings }, Partial<BrowserControlSettings>>({
			query: (body) => ({
				url: "/browser-ai/controls",
				method: "PUT",
				body,
			}),
			invalidatesTags: ["BrowserAiControls" as any],
		}),

		getBrowserAiTargets: builder.query<{ targets: BrowserTargetWebsite[] }, void>({
			query: () => "/browser-ai/targets",
			providesTags: ["BrowserAiTargets" as any],
		}),

		createBrowserAiTarget: builder.mutation<{ status: string; target: BrowserTargetWebsite }, Partial<BrowserTargetWebsite>>({
			query: (body) => ({
				url: "/browser-ai/targets",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiTargets" as any],
		}),

		importBrowserAiTargets: builder.mutation<
			BrowserAITargetImportResult,
			{ targets: Partial<BrowserTargetWebsite>[]; overwrite?: boolean }
		>({
			async queryFn(arg, _api, _extraOptions, baseQuery) {
				const bulk = await baseQuery({
					url: "/browser-ai/targets/import",
					method: "POST",
					body: arg,
				});
				if (!bulk.error) {
					return { data: bulk.data as BrowserAITargetImportResult };
				}
				const status = (bulk.error as { status?: number })?.status;
				if (status !== 405 && status !== 404) {
					return { error: bulk.error };
				}

				const listRes = await baseQuery({ url: "/browser-ai/targets" });
				if (listRes.error) {
					return { error: listRes.error };
				}
				const listBody = listRes.data as { targets?: BrowserTargetWebsite[] } | BrowserTargetWebsite[];
				const existing = Array.isArray(listBody) ? listBody : listBody?.targets || [];
				const byDomain = new Map(
					existing.map((t) => [String(t.domain || "").trim().toLowerCase(), t] as const),
				);

				let imported = 0;
				let updated = 0;
				let skipped = 0;
				const errors: string[] = [];
				const overwrite = Boolean(arg.overwrite);
				const reason = (err: unknown) => {
					const d = (err as { data?: { error?: { message?: string } | string; message?: string } })?.data;
					const e = d?.error;
					return (typeof e === "object" && e?.message) || (typeof e === "string" && e) || d?.message || "";
				};

				for (const target of arg.targets || []) {
					const rawDomain = String(target.domain || "").trim();
					if (!rawDomain) {
						skipped += 1;
						errors.push("Missing target domain");
						continue;
					}
					const domainKey = rawDomain.toLowerCase();
					const found = byDomain.get(domainKey);
					if (found?.id) {
						if (!overwrite) {
							skipped += 1;
							continue;
						}
						const upd = await baseQuery({
							url: `/browser-ai/targets/${encodeURIComponent(found.id)}`,
							method: "PUT",
							body: {
								domain: target.domain,
								platform_name: target.platform_name || target.domain,
								host_role: target.host_role || "",
								monitored: target.monitored ?? true,
								block_site: Boolean(target.block_site),
								status: target.block_site ? "BLOCKED" : target.monitored === false ? "PAUSED" : "MONITORED",
							},
						});
						if (upd.error) {
							errors.push(`${rawDomain}: update failed${reason(upd.error) ? ` (${reason(upd.error)})` : ""}`);
							continue;
						}
						updated += 1;
					} else {
						const cre = await baseQuery({
							url: "/browser-ai/targets",
							method: "POST",
							body: {
								domain: target.domain,
								platform_name: target.platform_name || target.domain,
								host_role: target.host_role || "",
								monitored: target.monitored ?? true,
								block_site: Boolean(target.block_site),
								status: target.block_site ? "BLOCKED" : target.monitored === false ? "PAUSED" : "MONITORED",
							},
						});
						if (cre.error) {
							errors.push(`${rawDomain}: create failed${reason(cre.error) ? ` (${reason(cre.error)})` : ""}`);
							continue;
						}
						const created = (cre.data as { target?: BrowserTargetWebsite })?.target;
						if (created?.domain) {
							byDomain.set(created.domain.toLowerCase(), created);
						}
						imported += 1;
					}
				}

				return {
					data: {
						status: "success",
						imported,
						updated,
						skipped,
						total: (arg.targets || []).length,
						errors: errors.length ? errors : undefined,
					},
				};
			},
			invalidatesTags: ["BrowserAiTargets" as any],
		}),

		updateBrowserAiTarget: builder.mutation<{ status: string }, { id: string; updates: Partial<BrowserTargetWebsite> }>({
			query: ({ id, updates }) => ({
				url: `/browser-ai/targets/${id}`,
				method: "PUT",
				body: updates,
			}),
			invalidatesTags: ["BrowserAiTargets" as any],
		}),

		deleteBrowserAiTarget: builder.mutation<{ status: string }, string>({
			query: (id) => ({
				url: `/browser-ai/targets/${id}`,
				method: "DELETE",
			}),
			invalidatesTags: ["BrowserAiTargets" as any],
		}),

		getBrowserAiAgents: builder.query<
			{
				agents: BrowserAIAgent[];
				total: number;
				limit: number;
				offset: number;
				latest_version?: string;
				latest_mac_version?: string;
				active_count?: number;
				uninstalled_count?: number;
			},
			{ status?: string; search?: string; agent_type?: string; limit?: number; offset?: number } | void
		>({
			query: (params) => ({
				url: "/browser-ai/agents",
				params: params || {},
			}),
			providesTags: ["BrowserAiAgents" as any],
		}),

		getBrowserAiSetupInfo: builder.query<BrowserAISetupInfo, void>({
			query: () => "/browser-ai/setup/info",
			providesTags: ["BrowserAiAgentSettings" as any],
		}),

		getBrowserAiRebuildHistory: builder.query<{ history: BrowserGuardRebuildLog[] }, number | void>({
			query: (limit) => `/browser-ai/setup/rebuild-history?limit=${limit || 20}`,
			providesTags: ["BrowserAiAgentSettings" as any],
		}),

		getBrowserAiAgentSettings: builder.query<{ settings: BrowserAIAgentSettings; uninstall_key?: string }, void>({
			query: () => "/browser-ai/agents/settings",
			providesTags: ["BrowserAiAgentSettings" as any],
		}),

		getBrowserAiCompanyUninstallKey: builder.query<{ uninstall_key: string; key_configured: boolean }, void>({
			query: () => "/browser-ai/agents/uninstall-key",
			providesTags: ["BrowserAiAgentSettings" as any],
		}),

		getBrowserAiFleetConfig: builder.query<{ fleet_config: BrowserGuardFleetConfig }, void>({
			query: () => "/browser-ai/fleet-config",
			providesTags: ["BrowserAiFleetConfig" as any],
		}),

		saveBrowserAiFleetConfig: builder.mutation<
			{ status: string; fleet_config: BrowserGuardFleetConfig },
			Partial<BrowserGuardFleetConfig>
		>({
			query: (body) => ({
				url: "/browser-ai/fleet-config",
				method: "PUT",
				body,
			}),
			invalidatesTags: ["BrowserAiFleetConfig" as any],
		}),

		saveBrowserAiUninstallKey: builder.mutation<
			{ status: string; settings: BrowserAIAgentSettings },
			{ key?: string; require_uninstall_key?: boolean; updated_by?: string }
		>({
			query: (body) => ({
				url: "/browser-ai/agents/uninstall-key",
				method: "PUT",
				body,
			}),
			invalidatesTags: ["BrowserAiAgentSettings" as any],
		}),

		remoteUninstallBrowserAiAgent: builder.mutation<
			{ status: string; agent: BrowserAIAgent },
			{ id: string; key: string }
		>({
			query: ({ id, key }) => ({
				url: `/browser-ai/agents/${encodeURIComponent(id)}/remote-uninstall`,
				method: "POST",
				body: { key },
			}),
			invalidatesTags: ["BrowserAiAgents" as any],
		}),

		getBrowserAiAgentUninstallKey: builder.query<
			{ agent_id: string; hostname: string; uninstall_key: string; has_key: boolean; uninstall_key_rotated_at?: string },
			string
		>({
			query: (id) => `/browser-ai/agents/${encodeURIComponent(id)}/uninstall-key`,
		}),

		rotateBrowserAiAgentUninstallKey: builder.mutation<
			{ status: string; agent_id: string; hostname: string; uninstall_key: string; uninstall_key_rotated_at?: string },
			string
		>({
			query: (id) => ({
				url: `/browser-ai/agents/${encodeURIComponent(id)}/uninstall-key/rotate`,
				method: "POST",
			}),
			invalidatesTags: ["BrowserAiAgents" as any],
		}),

		deleteBrowserAiAgent: builder.mutation<{ status: string; deleted: number }, string>({
			query: (id) => ({
				url: `/browser-ai/agents/${encodeURIComponent(id)}`,
				method: "DELETE",
			}),
			invalidatesTags: ["BrowserAiAgents" as any],
		}),

		bulkDeleteBrowserAiAgents: builder.mutation<{ status: string; deleted: number }, { ids: string[] }>({
			query: (body) => ({
				url: "/browser-ai/agents/bulk-delete",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiAgents" as any],
		}),

		sendBrowserAiWarningEmail: builder.mutation<
			{ status: string; message: string; to: string },
			{
				to: string;
				subject?: string;
				message: string;
				agent_id?: string;
				agent_hostname?: string;
				allowed_count?: number;
				blocked_count?: number;
				warn_count?: number;
				redact_count?: number;
			}
		>({
			query: (body) => ({
				url: "/browser-ai/send-warning-email",
				method: "POST",
				body,
			}),
			invalidatesTags: ["BrowserAiAgents" as any, "BrowserAiInsightStats" as any],
		}),

		getBrowserAiInsightStats: builder.query<
			{
				stats: BrowserAIAgentInsightStats[];
				totals: { allowed?: number; blocked?: number; warn?: number; redact?: number; total?: number };
			},
			void
		>({
			// Soft-fail: older gateways SPA-fallback HTML or 404 — Overview still uses agents+logs.
			async queryFn(_arg, _api, _extra, baseQuery) {
				const res = await baseQuery({ url: "/browser-ai/insights/stats" });
				if (res.error) {
					return { data: { stats: [], totals: {} } };
				}
				const raw = res.data as
					| {
							stats?: BrowserAIAgentInsightStats[];
							totals?: { allowed?: number; blocked?: number; warn?: number; redact?: number; total?: number };
					  }
					| string
					| null;
				if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
					return { data: { stats: [], totals: {} } };
				}
				return {
					data: {
						stats: Array.isArray(raw.stats) ? raw.stats : [],
						totals: raw.totals && typeof raw.totals === "object" ? raw.totals : {},
					},
				};
			},
			providesTags: ["BrowserAiInsightStats" as any],
		}),

		updateBrowserAiAgentContactEmail: builder.mutation<
			{ status: string; agent: BrowserAIAgent },
			{ id: string; contact_email: string }
		>({
			query: ({ id, contact_email }) => ({
				url: `/browser-ai/agents/${encodeURIComponent(id)}/contact-email`,
				method: "PUT",
				body: { contact_email },
			}),
			invalidatesTags: ["BrowserAiAgents" as any],
		}),
	}),
});

export const {
	useGetBrowserAiLogsQuery,
	useGetBrowserAiLogStatsQuery,
	useClearBrowserAiLogsMutation,
	useDeleteBrowserAiLogsMutation,
	useGetBrowserAiSearchLogsQuery,
	useClearBrowserAiSearchLogsMutation,
	useDeleteBrowserAiSearchLogsMutation,
	useRecordBrowserAiSearchLogMutation,
	useGetBrowserAiRulesQuery,
	useGetBrowserAiOllamaModelsQuery,
	useCreateBrowserAiRuleMutation,
	useImportBrowserAiRulesMutation,
	useUpdateBrowserAiRuleMutation,
	useDeleteBrowserAiRuleMutation,
	useTestBrowserAiGuardBotMutation,
	useGenerateBrowserAiRegexFromPolicyMutation,
	useGetBrowserAiControlsQuery,
	useUpdateBrowserAiControlsMutation,
	useGetBrowserAiTargetsQuery,
	useCreateBrowserAiTargetMutation,
	useImportBrowserAiTargetsMutation,
	useUpdateBrowserAiTargetMutation,
	useDeleteBrowserAiTargetMutation,
	useGetBrowserAiAgentsQuery,
	useGetBrowserAiSetupInfoQuery,
	useGetBrowserAiRebuildHistoryQuery,
	useGetBrowserAiAgentSettingsQuery,
	useGetBrowserAiCompanyUninstallKeyQuery,
	useLazyGetBrowserAiCompanyUninstallKeyQuery,
	useGetBrowserAiFleetConfigQuery,
	useSaveBrowserAiFleetConfigMutation,
	useSaveBrowserAiUninstallKeyMutation,
	useRemoteUninstallBrowserAiAgentMutation,
	useLazyGetBrowserAiAgentUninstallKeyQuery,
	useRotateBrowserAiAgentUninstallKeyMutation,
	useDeleteBrowserAiAgentMutation,
	useBulkDeleteBrowserAiAgentsMutation,
	useSendBrowserAiWarningEmailMutation,
	useGetBrowserAiInsightStatsQuery,
	useUpdateBrowserAiAgentContactEmailMutation,
} = browserAiApi;
