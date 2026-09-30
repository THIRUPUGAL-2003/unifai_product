# UnifAI Helm Charts

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/unifai)](https://artifacthub.io/packages/helm/unifai/unifai)

Official Helm charts for deploying [UnifAI](https://github.com/unifai/unifai) - a high-performance AI gateway with unified interface for multiple providers.

**Latest Version:** 2.1.25

## Changelog

### 2.1.25

- Added `unifai.circuitBreakerConfig`. Renders into `circuit_breaker_config` in the generated config JSON.
- Extended `unifai.loadBalancer` with four new behavioural flags: `directionSelectionEnabled`, `routeSelectionEnabled`, `rerouteFailedDirections`, and `pruneFailedFallbacks`. All are optional booleans; omitting a flag preserves the server default rather than forcing a value. Renders into the corresponding config.json fields inside `load_balancer_config`.
- Added `evaluation_mode` to guardrail rules. Accepts `bundled` (default, evaluate all turns together) or `per_turn` (evaluate each turn independently). Set under `unifai.guardrails.rules[].evaluation_mode`.
- Added `group_traces_by_session` to the OTEL and Datadog plugin configs. When `true`, requests sharing the same `x-uf-session-id` header are grouped into a single trace. An inbound W3C `traceparent` always takes priority. Defaults to `false`.
- Added `storage.configStore.vaultStore` to `values.yaml` with full commented-out examples for all three backends: `aws-secrets-manager`, `gcp-secret-manager`, and `hashicorp-vault`. Set `accessMode: read_and_write` to automatically store plaintext config fields as vault secrets; `read_only` (default) only resolves existing `vault.<path>` references.
- `dns_names` in cluster discovery config now accepts `env.VAR_NAME` references in addition to literal hostnames, consistent with how other secret-bearing fields work across the chart.

### 2.1.24

- `allow_private_network` (provider `networkConfig`) is now rendered by `_helpers.tpl`; it was in the schema but never wired in, so it had no effect.
- `unifai.envLabel` (max 10 chars) → `env_label`; shows an environment label in the management UI sidebar.
- Datadog plugin: separate `agent_host`/`agent_port` and `dogstatsd_host`/`dogstatsd_port` as an alternative to the combined `*_addr` (defaults `8126`/`8125`).
- `passwordCommand` for the PostgreSQL store (config + logs): runs a command that prints the password on stdout.
- Async log `writer` tuning block (`maxBatchSize`, `batchInterval`, `maxBatchBytes`, `writeQueueCapacity`, `deferredUsageConcurrency`) for SQLite and PostgreSQL.
- Deployment update strategy via top-level `strategy` (Deployment only); empty `{}` keeps the Kubernetes default.
- `unifai.client.allowDirectKeys` and `unifai.client.mcpExternalClientUrl`; previously unmapped in `_helpers.tpl` and silently dropped, now render.
- `blacklisted_models` alongside `allowed_models` in provider config.
- `unifai.skillsRegistry` (`enabled` + `skills[]`) rendered verbatim into `skills_registry`.

### 2.1.23

- Introduced `unifai.governance.complexityAnalyzerConfig` for complexity router boundaries/keywords; renders into `governance.complexity_analyzer_config`.
- `pluginSpanFilter` (`mode`/`plugins`) is now supported in OTEL config (single- and multi-profile), with a shared `$defs` definition reused across OTEL, Datadog, and BigQuery connectors.
- Brought `plugin_span_filter` support to the Datadog plugin config.
- New `bigquery` plugin defintion: `project_id`, `dataset_id`, `table_id`, `location`, `service_account_key`, `create_table_if_not_exists`, `flush_interval_seconds`, `buffer_size`, `custom_labels`, `disable_content_logging`, `request_headers`, `plugin_span_filter`.
- Extended Datadog plugin with `ml_app`, `dogstatsd_addr`, `enable_metrics`, `enable_llm_obs`, `agentless`, `api_key` (required when agentless), and `site`. Credentials support `env.VAR_NAME`.
- `key_ids` is now accepted in nested provider config inside virtual providers. Use `["*"]` for all keys; empty/omitted denies all (v2 default).
- New `kafka` plugin definition: requires `brokers` + `topic`; optional SASL, TLS, `compression`, `batch_size`, `flush_interval_ms`, `auto_create_topic`, `disable_content_logging`, `plugin_span_filter`.
- New `pubsub` plugin definition: requires `project_id` + `topic_id`; optional `service_account_key` (or ADC), `auto_create_topic`, `disable_content_logging`, `plugin_span_filter`.
- Introduced `unifai.framework.pricing.mcpLibraryUrl` and `mcpLibrarySyncInterval` for configuring a custom MCP server catalog.
- `ingress` now accepts a named map where each key produces a separate `Ingress` named `<release>-<key>`. Legacy `ingress.enabled` shape is unchanged.
- Configurable HTTP server read buffer size via `unifai.server.readBufferSize` (controls header-reading buffer; default 65536 bytes). Maps to `server.read_buffer_size` in config.json.

### 2.1.22

- Added `unifai.governance.roles` array to `values.yaml`, `values.schema.json`, and `_helpers.tpl`. Each role requires a `name` and accepts optional `description`, `dac` (`own-data` | `team-data` | `all-data`, default `all-data`), `access_profile`, and `permissions[]` (`resource` + `operation`).
- `unifai.plugins.otel.config` now accepts either the existing single-profile shape or a new `profiles` wrapper (`otelProfilesConfig`) with an array of profiles. Each profile is independently enabled/disabled. A shared `plugin_span_filter` can be set at the top level in either shape.
- Added `disable_content_logging` to OTEL config (both single-profile and per-profile). When `true`, message content (input/output messages, embeddings, tool definitions, tool call arguments/results) is dropped from exported spans — only metadata (model, tokens, latency) is sent to the collector.
- Added `otelPluginSpanFilter` (`mode`: `include`/`exclude`, `plugins` array) to the OTEL config schema, available in both single-profile and multi-profile shapes.
- Added `calendar_aligned` to `unifai.governance.modelConfigs[]`. 
- Added `model_config_id` and `customer_id` as budget owner fields in `governance.budgets[]`, alongside the existing `virtual_key_id`, `provider_config_id`, and `team_id`.
- Extended `attributeTeamMappings` and `attributeBusinessUnitMappings` in SCIM auth config with optional `attributeType` (`user` | `group`) and `attributeValue` fields to enable SCIM-driven team/business-unit provisioning.
- Added OAuth MCP client config example to `values.yaml` showing `authType: oauth` with `oauthConfigId`.
- Added `unifai.sourceOfTruth` (`split` | `config.json`, optional). When set to `"config.json"`, sections explicitly present in the file become authoritative on startup — database-only rows for those sections are pruned. Omitting the field preserves the default `"split"` merge behavior.
- Added `allow_private_network` to `networkConfig` in `values.schema.json`. When `true`, allows connections to RFC 1918 private IPs (10.x, 172.16.x, 192.168.x) — useful for providers on a k8s pod network, LAN, or private VPC.

### 2.1.21

- Add `per_user_oauth`/`per_user_headers` to `authType` enum in mcpClientConfig
- Added `scope` and `scope_id` fields to `unifai.governance.modelConfigs[]` items in `values.yaml` and `values.schema.json`. `scope` accepts `"global"` (default, applies to all traffic) or `"virtual_key"` (applies to a specific virtual key); `scope_id` is required when `scope` is `"virtual_key"` and must reference a virtual key `id`. The `_helpers.tpl` already passes `modelConfigs` through as-is so no template change was needed.

### 2.1.20

- Added `tlsConfig` to `unifai.mcp.clientConfigs[]` for HTTP and SSE MCP connection types:
  - `insecureSkipVerify` — disable TLS certificate verification (development/testing only; takes priority over `caCertPem`).
  - `caCertPem` — PEM-encoded CA certificate for MCP servers that use a self-signed or private CA. Accepts a literal PEM string or an `env.VAR_NAME` reference (e.g. `"env.MY_MCP_CA_CERT"`).
  - Chart maps `tlsConfig.insecureSkipVerify` → `tls_config.insecure_skip_verify` and `tlsConfig.caCertPem` → `tls_config.ca_cert_pem` in the generated config JSON.
- Added `authServerType` to the Okta SCIM config in `values.schema.json` and `config.schema.json`. Accepts `"org"` (Org Authorization Server) or `"custom"` (Custom Authorization Server); auto-detected from the issuer URL when omitted. Previously the field was documented but rejected by `additionalProperties: false` in both schemas.
- Added `attributeRoleMappings`, `attributeTeamMappings`, and `attributeBusinessUnitMappings` to the Okta provider branch in `config.schema.json`, aligning the transport runtime schema with the Helm chart schema which already included them.

### 2.1.19

- Added `unifai.modelCatalog.modelParametersUrl` to `values.yaml`, `values.schema.json`, and `_helpers.tpl`, allowing operators to override the URL UnifAI uses to fetch model parameter definitions.
- Added `existingSecret` support for hosted PostgreSQL (`postgresql.enabled: true`). Set `postgresql.auth.existingSecret` and `postgresql.auth.passwordKey` to reference a Kubernetes secret (e.g. from Vault Secrets Operator) instead of a plaintext password in values. Both the postgres pod and the unifai pod will read the password from the secret; the chart-managed secret is not created when `existingSecret` is set.
- Added `postgresql.primary.podSecurityContext` and `postgresql.primary.containerSecurityContext` to allow configuring pod- and container-level security contexts on the hosted PostgreSQL deployment. Defaults to `podSecurityContext: { fsGroup: 999 }` (preserving prior behaviour) and `containerSecurityContext: {}` (no container security context). Required for clusters enforcing strict Kyverno/OPA policies (e.g. `runAsNonRoot`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile`).
- Added `unifai.featureFlags` map to `values.yaml` and `_helpers.tpl`. Renders into `feature_flags.flags` in the generated config JSON. Each entry accepts a literal boolean or `"env.NAME"` string.
- Fixed Deployment not exposing the cluster gRPC container port; fixed `service.yaml` missing the gRPC service port. Both now match StatefulSet/headless service behaviour.
- Fixed Weaviate PVC rendering when `vectorStore.weaviate.persistence.enabled=false`; PVC is now gated on persistence being enabled.
- Fixed Redis probes passing password via `-a` flag in process args; switched to `REDISCLI_AUTH` env var.
- Fixed nondeterministic env var order for `providerSecrets` and `weaviate.env` map iterations; keys are now sorted with `sortAlpha`.
- Corrected guardrail `timeout` examples in `values.yaml`: provider default is `30s`, rule default is `60s`.

### 2.1.18

- Added `unifai.framework.pricing.modelParametersUrl` to `values.yaml`, `values.schema.json`, and `_helpers.tpl`, allowing operators to override the URL UnifAI uses to fetch model parameter definitions.

### 2.1.17

- Added `max_turns_to_send` to guardrail rules. The integer caps how many historical conversation turns are sent to the guardrail provider on apply; the latest message is always included on top, and `0` (default) sends all turns. Wired into `values.schema.json`, `config.schema.json`, and `templates/_helpers.tpl` so it renders into `guardrails_config.guardrail_rules[].max_turns_to_send`.
- Extended SCIM/SSO support so attribute mappings work for every supported provider, not just Keycloak:
  - Added `attributeRoleMappings`, `attributeTeamMappings`, and `attributeBusinessUnitMappings` to `unifai.scim.config` for the Okta and Entra (Azure AD) provider branches. Previously these fields were rejected by `additionalProperties: false` even though the enterprise runtime renders them into `config.json`.
  - Tightened the existing Keycloak mapping items from the placeholder `{type: object}` to a strict shape (`attribute`, `value`, plus `role`/`team`/`business_unit`, `additionalProperties: false`) so typos surface at `helm template` time. The same strict item shape is applied to Okta, Entra, Zitadel, and Google.
  - Added two more SCIM providers to the schema enum and provided full config blocks for them: `zitadel` (`domain`, `clientId`, optional `clientSecret`/`projectId`/`audience`, plus service-account fields for Management API access) and `google` (Google Workspace OIDC with `domain`, `clientId`, `credentialMode`, service-account sources, and `adminEmail` for domain-wide delegation).
  - Added matching `helm template`-time validation in `_helpers.tpl` for Zitadel (requires `domain`, `clientId`) and Google Workspace (requires `domain`, `clientId`).
  - Documented every new field as commented examples under `unifai.scim.config` in `values.yaml`.

### 2.1.16

- Widened `unifai.mcp.toolManagerConfig.toolExecutionTimeout` in `values.schema.json` from `integer` to `["integer", "string"]` so a Go duration string like `"30s"` or `"2m"` is accepted alongside the legacy bare integer. Updated the description to clarify "integer = seconds, string = Go duration" and recommend the string form, and changed the default from `30` to `"30s"`.
- Updated the `values.yaml` example to use `toolExecutionTimeout: "30s"` instead of `toolExecutionTimeout: 30`, matching the new recommended form.
- Paired with the upstream runtime fix (PR #3432) that reinterprets bare integers on this field as seconds rather than nanoseconds, and includes `mcp.tool_manager_config` in the client config hash so file-level changes survive the hash-based reconciliation pipeline on restart.

### 2.1.15

- Added `storage.logsStore.matviewRefreshInterval` to `values.yaml` and `values.schema.json`, letting operators control how often PostgreSQL materialized views are refreshed in the logs store (e.g. `"30s"`, `"5m"`, `"1h"`; minimum `5s`).
- Wired `matviewRefreshInterval` through `_helpers.tpl` so it renders into the generated PostgreSQL `logs_store.matview_refresh_interval` field when set, and is omitted when not.
- Bumped `appVersion` from `1.5.0-prerelease7` to `1.5.0` (first chart release pinned to the stable `1.5.0` app image).

### 2.1.14

- Removed the obsolete `unifai.client.allowDirectKeys` assertion from `validate-helm-config-fields.sh`. The field was deleted from the chart schema and codebase in a prior release, so the test was rendering an invalid values file and helm was rejecting it via `additionalProperties: false`.
- Hardened `render_config()` in `validate-helm-config-fields.sh` so a failing `helm template` actually surfaces its stderr instead of being swallowed by the script's `set -e` (the previous post-hoc `$?` check was unreachable).

### 2.1.13

- Surfaced `unifai.client.enforceAuthOnInference` in `values.yaml` as a commented default with usage notes. The field was already wired in `_helpers.tpl` to render to `client.enforce_auth_on_inference` and declared in `values.schema.json`; this change makes the knob discoverable without altering default rendered config.
- Marked `unifai.client.enforceGovernanceHeader` as deprecated in `values.yaml` (use `enforceAuthOnInference` instead). Schema description was already deprecated in 2.1.11.

### 2.1.12

- Added Helm support for `storage.logsStore.objectStorageExcludeFields` and render path to `logs_store.object_storage_exclude_fields` in generated config.

### 2.1.11

- Added `description` and `default` fields to numerous properties that previously had neither, including `initialPoolSize`, `disableDbPingsInHealth`, `logRetentionDays`, `asyncJobResultTTL`, `mcpAgentDepth`, `mcpToolExecutionTimeout`, `hideDeletedVirtualKeysInFilters`, `mcpDisableAutoToolInject`, and MCP `toolManagerConfig` fields
- Added `additionalProperties: false` to multiple objects (`unifai.config`, `unifai.pricing`, `proxyConfig`, `concurrencyConfig`, `providerConfig`, `credentialsSecret`, and auth provider configs) to reject unknown keys at validation time
- Added three new `unifai.client` fields:
  - `allowPerRequestContentStorageOverride` — controls whether per-request headers can override content logging behavior
  - `allowPerRequestRawOverride` — controls whether per-request headers can override raw provider request/response passthrough
  - `mcpExternalBaseUrl` — public base URL for OAuth callbacks and discovery metadata behind a reverse proxy, supporting both string and env-var object forms
- Added two new `unifai.cluster.discovery` fields:
  - `bindPort` — port to bind for cluster communication
  - `dialTimeout` — timeout for discovery dial operations as a Go duration string
- Changed `allowedOrigins` items from `oneOf` to `anyOf` and removed the redundant `not: { const: "*" }` constraint on the URI branch
- Tightened the env-var pattern to require a valid identifier start character (`[A-Za-z_]`) for proxyConfig.url
- Expanded `toolSyncInterval` to accept either a Go duration string (with a stricter regex) or a legacy integer (nanoseconds) for backward compatibility.
- Marked `enforceGovernanceHeader` as deprecated in its description
- Added `mdnsService` description for local network discovery

### 2.1.10

- Added `unifai.cluster.grpc` block for the cluster gRPC counter-sync transport (enterprise):
  - New values: `unifai.cluster.grpc.port` (default `10102`) and `unifai.cluster.grpc.dialTimeoutSeconds` (default `5`).
  - Rendered into `cluster_config.grpc` (`port`, `dial_timeout_seconds`) by `templates/_helpers.tpl`.
  - StatefulSet exposes the port as a named `grpc` container port; `service-headless` exposes it as a named service port so peers can dial each other.
  - Both port additions are guarded by `if .Values.unifai.cluster.grpc` so values overrides that omit the block render cleanly.

### 2.1.9

- Added Kubernetes pod-discovery RBAC templates for cluster discovery:
  - Added `templates/rbac.yaml` to render a namespaced `Role`/`RoleBinding` for pod `get/list/watch`.
  - Added `rbac.podDiscovery.enabled` to `values.yaml` and `values.schema.json` for controlled enablement (defaults to `true`).
  - RBAC resources render only when `rbac.podDiscovery.enabled`, `unifai.cluster.enabled`, and `unifai.cluster.discovery.enabled` are true, with discovery `type: kubernetes`.

### 2.1.8

- Added provider key backward compatibility in Helm rendering:
  - If `unifai.providers.<provider>.keys[].id` is omitted and `name` is present, Helm now auto-populates `id = name`.
  - This preserves legacy values files that only defined key names while still supporting `governance.virtualKeys[].provider_configs[].key_ids`.

### 2.1.7

- Added semantic cache Helm layers and examples:
  - Added Redis deployment template for semantic cache.
  - Extended Helm values/schema coverage for semantic cache and client-config examples.
- Added enterprise/governance Helm support:
  - Added governance `business_units` support in Helm schema/template rendering.
  - Added deferred virtual-key/provider-config budget ordering handling in Helm rendering.
- Added MCP tool-groups support in Helm:
  - Added `mcp.tool_groups` config support with governance bindings.
  - Added camelCase alias compatibility for related Helm config fields.

### 2.1.6

- Includes unreleased `2.1.5` changes
- Built-in plugin versioning for DB-backed deployments:
  - Added `version` field support for built-in plugins.
  - Added default `version: 1` for built-in plugins in `values.yaml` (`telemetry`, `logging`, `governance`, `maxim`, `semanticCache`, `otel`, `datadog`).
  - Updated `_helpers.tpl` to include plugin `version` in rendered config when set (cast as integer).
- Updated StatefulSet PVC template labels to be immutable-safe:
  - `spec.volumeClaimTemplates.metadata.labels` now uses stable selector labels (without chart/app version labels).
- Governance schema and validation updates:
  - Added `governance.budgets[].virtual_key_id` support.
  - Removed stale `budget_id` references from virtual keys and provider configs in templates/tests.
  - `validate-helm-config-fields.sh` assertions were updated accordingly.
- Query/schema compatibility updates:
  - Tightened `query` validation in `values.schema.json` and `config.schema.json` to valid RuleGroupType shape (`null` or `{ combinator, rules }`).
- Config/input alias support updates:
  - Added support for `env.*` references in proxy/TLS fields (`ca_cert_pem`, `url`, `username`, `password`).
  - Added `provider_key_name` alias for routing targets and pricing overrides (resolved to `key_id` at config load time).
- MCP config improvements:
  - Added Go duration string support for `mcp.toolSyncInterval` (legacy numeric nanoseconds still supported).
  - Added hash-based MCP client config reconciliation for DB-backed config store updates.
- Upgrade impact:
  - Existing SQLite StatefulSets created from older chart templates may require a one-time StatefulSet recreation during upgrade because `spec.volumeClaimTemplates` is immutable in Kubernetes.
- Migration notes (only if upgrade fails with StatefulSet immutable-field error):
  1. Identify StatefulSet name and namespace for your Helm release.
  2. Delete only the StatefulSet while preserving dependents:
     - `kubectl delete statefulset <statefulset-name> -n <namespace> --cascade=orphan`
  3. Run Helm upgrade:
     - `helm upgrade <release-name> unifai/unifai -n <namespace> -f <values-file> --set image.tag=<tag>`
  4. If needed, re-apply/recreate the StatefulSet from the upgraded chart manifests.
  5. Verify PVCs are preserved and pods become healthy:
     - `kubectl get pvc -n <namespace>`
     - `kubectl get pods -n <namespace>`

### 2.1.5 (not released separately)

- Merged into `2.1.6` release notes above.

### 2.1.4

- Added stricter cluster discovery validation in Helm templates:
  - Require `unifai.cluster.discovery.serviceName` when `unifai.cluster.discovery.type` is `consul`, `etcd`, or `udp`.
  - For `udp` discovery, require both:
    - `unifai.cluster.discovery.udpBroadcastPort`
    - `unifai.cluster.discovery.allowedAddressSpace`
- Added/updated template fail-fast errors so invalid discovery config is rejected at render time instead of failing later at runtime.

### 2.1.3

- For `unifai.cluster.discovery.type` set to `consul`, `etcd`, or `udp`, set `unifai.cluster.discovery.serviceName` explicitly during upgrade.

### v2.1.2

- Removed `encryption_key` requirement — field is now optional; UnifAI will operate without encryption when omitted

### v2.1.1

- Made `unifai.governance.virtualKeys[].value` optional — template no longer fails when the field is omitted, allowing the backend to auto-generate the virtual key value
- When `value` is absent, the rendered `config.json` omits the field entirely (consistent with other optional VK fields)

### v2.1.0-prerelease2 (prerelease)

- Synced helm `values.schema.json` with transport `config.schema.json` — fixed virtual key and budget drift:
  - Removed `required: [mcp_client_id]` constraint on `virtualKeys[].mcp_configs[]` items — canonical schema accepts either `mcp_client_id` (DB form) or `mcp_client_name` (config-file form, resolved to ID at startup)
  - Added `mcp_client_name` as an allowed property on `virtualKeys[].mcp_configs[]` items
  - Added `calendar_aligned` (boolean) on `virtualKeys[]` — field now lives on the virtual key, applies uniformly to all budgets under it
  - Removed stale `budget_id` from `virtualKeys[]` — `TableVirtualKey` has no `BudgetID`; budgets link via foreign key from the budget table
  - Removed stale `calendar_aligned` from `budgets[]` — moved to virtual key level

### v2.0.17

- Added object storage support (S3/GCS) for offloading log payloads from the database
- Added `storage.logsStore.objectStorage` configuration with S3 and GCS backend support
- Added object storage credential injection from Kubernetes secrets (`existingSecret`)
- Added `object_storage` schema to `config.schema.json` under `logs_store`
- Updated deployment and stateful templates with object storage secret env vars

### v2.0.16

- Fixed disabled custom plugins being completely removed from rendered config.json instead of being kept with `enabled: false`

### v2.0.15

- Synced helm schema with transport `config.schema.json` — added missing properties:
  - `client.mcpDisableAutoToolInject` — disable automatic MCP tool injection
  - `governance.budgets[].calendar_aligned` — snap budget resets to calendar boundaries
  - `governance.pricingOverrides` — scoped pricing overrides for the model catalog
  - `mcp.clientConfigs[].allowedExtraHeaders` — header allowlist per MCP client
  - `mcp.clientConfigs[].allowOnAllVirtualKeys` — make MCP server accessible to all virtual keys
  - `mcp.toolManagerConfig.disableAutoToolInject` — disable auto tool injection at manager level
  - `networkConfig.beta_header_overrides` — override Anthropic beta header support per provider
  - `websocket` — full WebSocket gateway tuning (connections, pool, transcript buffer)
- Fixed SSE `connectionString` not being rendered in `_helpers.tpl` for MCP clients
- Added template rendering for all new properties in `_helpers.tpl`

### v2.0.14

- Added `placement` and `order` fields to custom plugin schema and template rendering
- Added plugin property completeness check to `validate-helm-schema.sh`
- Added custom plugin placement/order rendering tests to `validate-helm-templates.sh`
- Added `PluginConfig` struct validation to `validate-go-config-fields.sh`

### v2.0.13

- Added missing client config properties: `asyncJobResultTTL`, `requiredHeaders`, `loggingHeaders`, `allowedHeaders`, `mcpAgentDepth`, `mcpToolExecutionTimeout`, `mcpCodeModeBindingLevel`, `mcpToolSyncInterval`, `hideDeletedVirtualKeysInFilters`
- Added MCP new fields: top-level `toolSyncInterval`, per-client `clientId`, `isCodeModeClient`, `toolSyncInterval`, `isPingAvailable`, `toolPricing`, and `codeModeBindingLevel` in tool manager config
- Added governance `modelConfigs` and `providers` top-level properties
- Added cluster `region` property
- Added guardrail provider `timeout` field (was missing from schema and template rendering)
- Fixed `isPingAvailable` rendering bug in `_helpers.tpl` (was using wrong key name)
- Added `is_ping_available` and `tool_pricing` to `config.schema.json` MCP client config
- Added new CI script `validate-go-config-fields.sh` for Go struct-to-schema drift detection
- Expanded all 3 existing CI validation scripts with Gap 1-8 property coverage

### v2.0.12

- Fixed health probe paths to use `/health` instead of `/metrics`

### v2.0.11

- Bumped appVersion to 1.4.11

### v2.0.10

- Added missing plugin config properties from Go implementations:
  - governance: `required_headers`, `is_enterprise`
  - logging: `disable_content_logging`, `logging_headers`
  - otel: `headers`, `tls_ca_cert`, `insecure`
  - telemetry: `custom_labels`

### v2.0.9

- Bumped appVersion to 1.4.8

### v2.0.8

- Added comprehensive config field coverage for all `config.schema.json` fields
- Added Pinecone vector store support (external only) with secret injection
- Added governance routing rules template support
- Added OTEL metrics fields (metrics_enabled, metrics_endpoint, metrics_push_interval)
- Added advanced Redis connection pool fields (pool_size, timeouts, idle conns, etc.)
- Added Weaviate timeout and className fields
- Expanded values.yaml with commented examples for all provider types (Azure, Vertex, Bedrock), network config, concurrency, proxy config, and governance entities
- Added helm config field validation CI test (246 assertions covering all config.schema.json fields)

### v2.0.7

- Previous release

### v2.0.6

- Fixes MCP client config template to convert camelCase Helm values to snake_case config format

### v2.0.5

- Fixes config field validation parity

### v2.0.2

- Added Qdrant vector store support with deployment, service, and PVC templates
- Added headless service template for StatefulSet DNS resolution
- Fixed gitignore pattern that was excluding template files from version control

### v2.0.1

- Added missing StatefulSet template for SQLite with persistence mode
- Added headless service for StatefulSet DNS resolution
- v2.0.0 documented StatefulSet support but the template was not included - this release fixes that

### v2.0.0 (Breaking Change)

#### StatefulSet for SQLite with Persistence

This release fixes the multi-attach volume error when running multiple replicas with SQLite storage mode.

#### What Changed

- When using `storage.mode: sqlite` with `storage.persistence.enabled: true`, UnifAI now deploys as a **StatefulSet** instead of a Deployment
- Each pod gets its own dedicated PersistentVolumeClaim (e.g., `data-unifai-0`, `data-unifai-1`, `data-unifai-2`)
- A headless service is created for StatefulSet DNS resolution
- HorizontalPodAutoscaler now correctly references StatefulSet or Deployment based on storage configuration

#### Who Is Affected

- Users running SQLite mode with persistence enabled and multiple replicas
- Users upgrading existing SQLite deployments need to migrate (see below)

#### Who Is NOT Affected

- Users running PostgreSQL mode (`storage.mode: postgres`) - no changes, still uses Deployment
- Users running SQLite without persistence (`storage.persistence.enabled: false`)
- Users running SQLite with an existing PVC claim (`storage.persistence.existingClaim`)

#### Migration Guide for Existing SQLite Deployments

Since Kubernetes doesn't allow in-place conversion from Deployment to StatefulSet, you need to:

1. Back up your data (if needed)
2. Uninstall the existing release: `helm uninstall unifai`
3. Delete the old PVC: `kubectl delete pvc unifai-data`
4. Install with the new chart version: `helm install unifai unifai/unifai --set image.tag=<latest-image>`

**Note:** For production high-availability setups, we recommend using PostgreSQL mode which scales horizontally without these concerns.

### v1.7.0

- Previous stable release with Deployment-based architecture for all storage modes

## Quick Start

```bash
# Add the UnifAI Helm repository
helm repo add unifai https://maximhq.github.io/unifai/helm-charts

# Update your local Helm chart repository cache
helm repo update

# Install UnifAI with default configuration (SQLite storage)
helm install unifai unifai/unifai --set image.tag=v1.4.3
```

## Prerequisites

- Kubernetes 1.23+
- Helm 3.2.0+
- PV provisioner support in the underlying infrastructure (for persistent storage)

## Installation

### From Helm Repository (Recommended)

```bash
# Add repository
helm repo add unifai https://maximhq.github.io/unifai/helm-charts
helm repo update

# Install with default values
helm install unifai unifai/unifai --set image.tag=v1.4.3

# Or install with custom values
helm install unifai unifai/unifai -f my-values.yaml
```

### From Source

```bash
# Clone the repository
git clone https://github.com/unifai/unifai.git
cd unifai/helm-charts

# Install from local chart
helm install unifai ./unifai --set image.tag=v1.5.2
```

### Interactive Installation

Use the included installation script for guided setup:

```bash
cd unifai/helm-charts/unifai
./scripts/install.sh
```

## Configuration

### Image Configuration

| Parameter          | Description                    | Default                     |
| ------------------ | ------------------------------ | --------------------------- |
| `image.repository` | Container image repository     | `docker.io/unifai/unifai` |
| `image.tag`        | Container image tag (required) | `""`                        |
| `image.pullPolicy` | Image pull policy              | `IfNotPresent`              |

> **Important:** You must specify the `image.tag`. See available tags at [Docker Hub](https://hub.docker.com/r/unifai/unifai/tags).

### Enterprise Private Registry

For enterprise customers with private container registries, simply override the `image.repository` with your full registry URL:

```yaml
# Google Artifact Registry
image:
  repository: us-west1-docker.pkg.dev/unifai-enterprise/your-org/unifai
  tag: v1.5.0

# AWS ECR
image:
  repository: 123456789.dkr.ecr.us-east-1.amazonaws.com/unifai
  tag: v1.5.0

# Azure Container Registry
image:
  repository: yourregistry.azurecr.io/unifai
  tag: v1.5.0

# Self-hosted registry
image:
  repository: registry.yourcompany.com/ai/unifai
  tag: v1.5.0
```

If your private registry requires authentication, configure `imagePullSecrets`:

```yaml
image:
  repository: us-west1-docker.pkg.dev/unifai-enterprise/your-org/unifai
  tag: v1.5.0

imagePullSecrets:
  - name: my-registry-secret
```

Create the secret beforehand:

```bash
kubectl create secret docker-registry my-registry-secret \
  --docker-server=us-west1-docker.pkg.dev \
  --docker-username=_json_key \
  --docker-password="$(cat key.json)" \
  --docker-email=your-email@example.com
```

### Storage Configuration

UnifAI supports two storage backends (SQLite and PostgreSQL) that can be configured independently for config and logs stores.

| Parameter                                      | Description                                                             | Default                    |
| ---------------------------------------------- | ----------------------------------------------------------------------- | -------------------------- |
| `storage.mode`                                 | Default storage backend (fallback when per-store type not set)          | `sqlite`                   |
| `storage.persistence.enabled`                  | Enable persistent storage for SQLite                                    | `true`                     |
| `storage.persistence.size`                     | Storage size                                                            | `10Gi`                     |
| `storage.configStore.enabled`                  | Enable configuration store                                              | `true`                     |
| `storage.configStore.type`                     | Config store backend: `sqlite`, `postgres`, or `""`                     | `""` (uses `storage.mode`) |
| `storage.logsStore.enabled`                    | Enable logs store                                                       | `true`                     |
| `storage.logsStore.type`                       | Logs store backend: `sqlite`, `postgres`, or `""`                       | `""` (uses `storage.mode`) |
| `storage.logsStore.objectStorageExcludeFields` | Payload DB fields to keep in DB instead of offloading to object storage | `[]`                       |

#### Mixed Backend Example

You can use different backends for config and logs stores:

```yaml
storage:
  mode: sqlite # Default fallback
  configStore:
    enabled: true
    type: sqlite # Config in SQLite (fast, local)
  logsStore:
    enabled: true
    type: postgres # Logs in PostgreSQL (scalable, queryable)

postgresql:
  enabled: true
  # ... PostgreSQL configuration for logs store
```

### PostgreSQL Configuration

| Parameter                            | Description                                                                                                                                                                                                                                      | Default            |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------ |
| `postgresql.enabled`                 | Deploy PostgreSQL as part of this chart                                                                                                                                                                                                          | `false`            |
| `postgresql.auth.username`           | Database username                                                                                                                                                                                                                                | `unifai`          |
| `postgresql.auth.password`           | Database password (ignored when `existingSecret` is set)                                                                                                                                                                                         | `unifai_password` |
| `postgresql.auth.database`           | Database name                                                                                                                                                                                                                                    | `unifai`          |
| `postgresql.auth.existingSecret`     | Name of an existing Kubernetes secret containing the password. When set, the chart does not create its own secret — both the postgres pod and the unifai pod read from this secret. Useful with secret managers such as Vault Secrets Operator. | `""`               |
| `postgresql.auth.passwordKey`        | Key inside `existingSecret` that holds the password                                                                                                                                                                                              | `password`         |
| `postgresql.external.enabled`        | Use external PostgreSQL (e.g. RDS) instead of deploying a pod                                                                                                                                                                                    | `false`            |
| `postgresql.external.host`           | External PostgreSQL host                                                                                                                                                                                                                         | `""`               |
| `postgresql.external.existingSecret` | Name of an existing Kubernetes secret containing the password for the external instance                                                                                                                                                          | `""`               |
| `postgresql.external.passwordKey`    | Key inside the external `existingSecret` that holds the password                                                                                                                                                                                 | `password`         |

#### Using an Existing Secret for Hosted PostgreSQL

If you manage secrets externally (e.g. with Vault Secrets Operator, External Secrets Operator, or Sealed Secrets), point `existingSecret` at the synced Kubernetes secret instead of providing a plaintext password:

```yaml
storage:
  mode: postgres

postgresql:
  enabled: true
  auth:
    username: unifai
    database: unifai
    existingSecret: vault-postgres-secret # VSO-synced secret name
    passwordKey: password # key inside the secret
```

The chart will skip creating its own secret. Both the postgres pod (`POSTGRES_PASSWORD`) and the unifai pod (`UNIFAI_POSTGRES_PASSWORD`) will mount the password directly from your secret.

For external PostgreSQL (e.g. RDS), use `postgresql.external.existingSecret` instead:

```yaml
postgresql:
  external:
    enabled: true
    host: my-db.us-east-1.rds.amazonaws.com
    user: unifai
    database: unifai
    existingSecret: vault-rds-secret
    passwordKey: password
    sslMode: require
```

### Vector Store Configuration (Semantic Caching)

UnifAI supports multiple vector stores for semantic caching:

| Parameter             | Description                                              | Default |
| --------------------- | -------------------------------------------------------- | ------- |
| `vectorStore.enabled` | Enable vector store                                      | `false` |
| `vectorStore.type`    | Vector store type: `none`, `weaviate`, `redis`, `qdrant` | `none`  |

#### Weaviate

```yaml
vectorStore:
  enabled: true
  type: weaviate
  weaviate:
    enabled: true # Deploy Weaviate
    # Or use external:
    # external:
    #   enabled: true
    #   host: "weaviate.example.com"
```

#### Redis

```yaml
vectorStore:
  enabled: true
  type: redis
  redis:
    enabled: true # Deploy Redis
    # Or use external:
    # external:
    #   enabled: true
    #   host: "redis.example.com"
```

#### Qdrant

```yaml
vectorStore:
  enabled: true
  type: qdrant
  qdrant:
    enabled: true # Deploy Qdrant
    # Or use external:
    # external:
    #   enabled: true
    #   host: "qdrant.example.com"
```

### UnifAI Application Configuration

| Parameter               | Description                       | Default   |
| ----------------------- | --------------------------------- | --------- |
| `unifai.port`          | Application port                  | `8080`    |
| `unifai.host`          | Bind address                      | `0.0.0.0` |
| `unifai.logLevel`      | Log level                         | `info`    |
| `unifai.logStyle`      | Log format: `json` or `text`      | `json`    |
| `unifai.encryptionKey` | Encryption key for sensitive data | `""`      |

### Provider Configuration

Configure AI provider API keys:

> **Note:** `keys[].weight` is optional in Helm values. If omitted, the chart renders it as `1`.

```yaml
unifai:
  providers:
    openai:
      keys:
        - value: "sk-..."
          weight: 1
    anthropic:
      keys:
        - value: "sk-ant-..."
          weight: 1
```

### Plugins Configuration

| Plugin         | Parameter                               | Description                      |
| -------------- | --------------------------------------- | -------------------------------- |
| Telemetry      | `unifai.plugins.telemetry.enabled`     | Enable metrics collection        |
| Logging        | `unifai.plugins.logging.enabled`       | Enable request logging           |
| Governance     | `unifai.plugins.governance.enabled`    | Enable budget management         |
| Semantic Cache | `unifai.plugins.semanticCache.enabled` | Enable semantic caching          |
| OTEL           | `unifai.plugins.otel.enabled`          | Enable OpenTelemetry integration |
| Maxim          | `unifai.plugins.maxim.enabled`         | Enable Maxim observability       |
| Datadog        | `unifai.plugins.datadog.enabled`       | Enable Datadog APM integration   |
| Custom         | `unifai.plugins.custom`                | Array of custom/dynamic plugins  |

#### Custom Plugins

You can add custom/dynamic plugins using the `unifai.plugins.custom` array:

```yaml
unifai:
  plugins:
    custom:
      - name: "my-custom-plugin"
        enabled: true
        path: "/plugins/my-plugin.so"
        version: 1
        placement: "pre_builtin" # or "post_builtin" (default)
        order: 0 # execution order within placement group
        config:
          key: value
```

### Client Configuration

| Parameter                                     | Description                                 | Default |
| --------------------------------------------- | ------------------------------------------- | ------- |
| `unifai.client.disableDbPingsInHealth`       | Disable DB pings in health check            | `false` |
| `unifai.client.dumpErrorsInConsoleLogs`      | Dump full error details to server console   | `false` |
| `unifai.client.headerFilterConfig.allowlist` | Headers allowed to forward to LLM providers | `[]`    |
| `unifai.client.headerFilterConfig.denylist`  | Headers blocked from forwarding             | `[]`    |

### MCP Configuration

| Parameter                                                  | Description                                                                                                                                                                                                    | Default  |
| ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `unifai.mcp.enabled`                                      | Enable MCP (Model Context Protocol)                                                                                                                                                                            | `false`  |
| `unifai.mcp.clientConfigs`                                | Array of MCP client configurations                                                                                                                                                                             | `[]`     |
| `unifai.mcp.toolManagerConfig.toolExecutionTimeout`       | Tool execution timeout. Integer = seconds, string = Go duration (e.g. `"30s"`, `"2m"`). Prefer the string form.                                                                                                | `"30s"`  |
| `unifai.mcp.toolManagerConfig.maxAgentDepth`              | Maximum agent depth                                                                                                                                                                                            | `10`     |
| `unifai.mcp.toolManagerConfig.codeModeBindingLevel`       | Code mode binding level (`server` or `tool`)                                                                                                                                                                   | `server` |
| `unifai.mcp.toolManagerConfig.disableAutoToolInject`      | Disable automatic MCP tool injection                                                                                                                                                                           | `false`  |
| `unifai.mcp.toolSyncInterval`                             | Global MCP tool sync interval. Prefer a Go duration string (for example, `10m`); legacy numeric nanoseconds are still supported for backward compatibility, but string format is recommended.                  | `10m`    |
| `unifai.mcp.clientConfigs[].tlsConfig.insecureSkipVerify` | **[Upcoming]** Disable TLS certificate verification for HTTP/SSE MCP connections. Takes priority over `caCertPem`. For development/testing only — not recommended for production.                              | `false`  |
| `unifai.mcp.clientConfigs[].tlsConfig.caCertPem`          | **[Upcoming]** PEM-encoded CA certificate to trust for HTTP/SSE MCP server connections. Accepts a literal PEM string or an `env.VAR_NAME` reference. Use when the MCP server uses a self-signed or private CA. | `""`     |

#### MCP Migration Guide (`client.mcp*` -> `mcp.*`)

Prefer MCP settings under `unifai.mcp` going forward. Older `unifai.client.mcp*`
keys are retained for backward compatibility, but new configs should migrate to the
`mcp.toolManagerConfig` and `mcp.toolSyncInterval` fields.

| Old key                                   | New key                                               |
| ----------------------------------------- | ----------------------------------------------------- |
| `unifai.client.mcpAgentDepth`            | `unifai.mcp.toolManagerConfig.maxAgentDepth`         |
| `unifai.client.mcpToolExecutionTimeout`  | `unifai.mcp.toolManagerConfig.toolExecutionTimeout`  |
| `unifai.client.mcpCodeModeBindingLevel`  | `unifai.mcp.toolManagerConfig.codeModeBindingLevel`  |
| `unifai.client.mcpDisableAutoToolInject` | `unifai.mcp.toolManagerConfig.disableAutoToolInject` |
| `unifai.client.mcpToolSyncInterval`      | `unifai.mcp.toolSyncInterval`                        |

### Ingress Configuration

```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: unifai.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: unifai-tls
      hosts:
        - unifai.example.com
```

### Auto-scaling Configuration

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80
```

### Referencing Secrets in MCP Headers

`unifai.mcp.clientConfigs[].headers` is a free-form `map<string, string>`
whose values can contain auth tokens. The chart does not wrap this map with
a bespoke `secretRef` — a per-header dict would explode the values surface.
Instead, use the standard pattern:

1. Write `env.MY_HEADER_VAR` as the header value in `values.yaml`:
   ```yaml
   unifai:
     mcp:
       clientConfigs:
         - name: "my-mcp"
           connectionType: "http"
           headers:
             Authorization: "env.MY_MCP_AUTH"
   ```
2. Inject the env var into the pod via the chart's top-level `envFrom:` or
   `env:` pass-through — e.g., in `values.yaml`:
   ```yaml
   envFrom:
     - secretRef:
         name: my-mcp-auth-secret
   # OR:
   env:
     - name: MY_MCP_AUTH
       valueFrom:
         secretKeyRef:
           name: my-mcp-auth-secret
           key: authorization
   ```

For `unifai.mcp.clientConfigs[].connectionString` itself, prefer the
chart-native `secretRef` (`name` + `connectionStringKey`) instead — the
chart will inject `UNIFAI_MCP_<NAME>_CONNECTION_STRING` and rewrite the
config automatically.

## Example Configurations

The chart includes pre-configured examples in `values-examples/`:

| Configuration            | Description                                             |
| ------------------------ | ------------------------------------------------------- |
| `sqlite-only.yaml`       | Simple setup with SQLite (local development)            |
| `postgres-only.yaml`     | PostgreSQL for config and logs                          |
| `mixed-backend.yaml`     | SQLite for config + PostgreSQL for logs (mixed backend) |
| `postgres-weaviate.yaml` | PostgreSQL + Weaviate for semantic caching              |
| `postgres-redis.yaml`    | PostgreSQL + Redis for semantic caching                 |
| `postgres-qdrant.yaml`   | PostgreSQL + Qdrant for semantic caching                |
| `sqlite-weaviate.yaml`   | SQLite + Weaviate                                       |
| `sqlite-redis.yaml`      | SQLite + Redis                                          |
| `sqlite-qdrant.yaml`     | SQLite + Qdrant                                         |
| `external-postgres.yaml` | Using external PostgreSQL                               |
| `production-ha.yaml`     | Production high-availability setup                      |

### Using Example Configurations

```bash
# From Helm repository
helm install unifai unifai/unifai \
  -f https://raw.githubusercontent.com/unifai/unifai/main/helm-charts/unifai/values-examples/postgres-only.yaml \
  --set image.tag=v1.5.2

# From local source
helm install unifai ./unifai -f ./unifai/values-examples/postgres-only.yaml
```

## Production Deployment

For production deployments, we recommend:

1. **Use PostgreSQL** for reliable data persistence
2. **Enable semantic caching** with Weaviate, Redis, or Qdrant
3. **Configure auto-scaling** for handling variable load
4. **Set up Ingress** with TLS termination
5. **Use external secrets** for sensitive data

### Example Production Setup

```yaml
# production-values.yaml
replicaCount: 3

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10

storage:
  mode: postgres

postgresql:
  enabled: true
  auth:
    password: "SECURE_PASSWORD_HERE"
  primary:
    persistence:
      size: 50Gi

vectorStore:
  enabled: true
  type: weaviate
  weaviate:
    enabled: true
    persistence:
      size: 50Gi

ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: unifai.yourdomain.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: unifai-tls
      hosts:
        - unifai.yourdomain.com

unifai:
  client:
    initialPoolSize: 1000
    allowedOrigins:
      - "https://yourdomain.com"
  plugins:
    semanticCache:
      enabled: true
    telemetry:
      enabled: true
    logging:
      enabled: true
```

## Upgrading

```bash
# Update repository
helm repo update

# Upgrade release
helm upgrade unifai unifai/unifai --set image.tag=v1.5.2

# Or with custom values
helm upgrade unifai unifai/unifai -f my-values.yaml
```

## Uninstalling

```bash
# Uninstall release
helm uninstall unifai

# If you want to delete persistent volumes
kubectl delete pvc -l app.kubernetes.io/name=unifai
```

## Accessing UnifAI

After installation, access UnifAI using one of these methods:

### Port Forwarding (Development)

```bash
kubectl port-forward svc/unifai 8080:8080
# Then visit http://localhost:8080
```

### LoadBalancer

```yaml
service:
  type: LoadBalancer
```

### Ingress

Configure the `ingress` section as shown above.

## Monitoring

UnifAI exposes Prometheus metrics at `/metrics`:

```bash
# Get metrics
curl http://localhost:8080/metrics
```

For OpenTelemetry integration:

```yaml
unifai:
  plugins:
    otel:
      enabled: true
      config:
        service_name: "unifai"
        collector_url: "http://otel-collector:4317"
        trace_type: "genai_extension"
        protocol: "grpc"
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -l app.kubernetes.io/name=unifai
kubectl describe pod <pod-name>
```

### View Logs

```bash
kubectl logs -l app.kubernetes.io/name=unifai -f
```

### Check Configuration

```bash
# View generated configmap
kubectl get configmap unifai -o yaml

# View generated secrets
kubectl get secret unifai -o yaml
```

### Common Issues

**Pod stuck in Pending state:**

- Check if PersistentVolume is available: `kubectl get pv`
- Check storage class: `kubectl get storageclass`

**Pod CrashLoopBackOff:**

- Check logs: `kubectl logs <pod-name>`
- Verify environment variables and secrets

**Cannot connect to PostgreSQL:**

- Ensure PostgreSQL pod is running
- Check connection string in configmap/secrets
- Verify network policies allow connectivity

## Resources

- [UnifAI Documentation](https://docs.unifai.ai)
- [GitHub Repository](https://github.com/unifai/unifai)
- [Docker Hub](https://hub.docker.com/r/unifai/unifai)
- [Discord Community](https://discord.gg/exN5KAydbU)

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](../LICENSE) file for details.

Built with ❤️ by [Maxim](https://github.com/maximhq)
