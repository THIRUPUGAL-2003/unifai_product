#!/usr/bin/env python3
"""
Enriches Volume I: Product Datasheet & Technical Specifications
Integrates the complete engineering architecture from TECHNICAL.md:
  - Multi-language runtime stack (Go 1.26.4, fasthttp, Python 3.11+, React 19, TypeScript 5.9, Vite 8)
  - Full repository layout & module architecture (core, framework, plugins, transports, ui, apps/browser-guard)
  - Universal provider fabric (28+ enterprise models)
  - Plugin execution chain & dynamic .so plugin loading
  - Guard Desktop Agent internals (unifai_agent.py, agent_pac_*.py, etc.)
  - mitmproxy multipart pipeline in MANIFEST.txt order
  - Guard UI architecture & component framework (React 19 SPA, Monaco, Recharts, Browser AI tabs)
  - Complete HTTP API surface & route catalog (/v1/*, /api/browser-ai/*, /api/governance/*, etc.)
  - Technical extension points matrix (10 core extension vectors)
  - Complete GORM relational database models (ConfigStore, LogsStore, Browser AI tables)
  - Enterprise cryptography, FIPS-140 & complete security model
  - Production environment variables & deployment parameters
  - Developer, build & automated test workflow (Make targets, Inno Setup, Docker, Helm)
  - Enterprise SBOM & SLSA Level 3 vulnerability management
"""

from pathlib import Path
import re

BASE_DIR = Path(r"d:\unifai_project")
DOCS_DIR = BASE_DIR / "docs"
MD1_PATH = DOCS_DIR / "01_YP_RAKSHA_Product_Datasheet.md"

# Read existing Section 1.0 from MD1_PATH (from line 1 down to right before # Section 2.0)
content = MD1_PATH.read_text(encoding="utf-8")
sec2_split = content.split("# Section 2.0")
header_and_sec1 = sec2_split[0].strip()

# Prepare Enriched Section 2.0 Content
sec2_enriched = """# Section 2.0 — Product Datasheet & Technical Specifications

## 2.1 Core Engineering Stack & Multi-Language Runtime Matrix

YP-Raksha is engineered as a zero-compromise, ultra-low-latency dual data plane platform. The architecture separates high-throughput gateway proxying from reactive workstation endpoint enforcement through specialized native runtimes:

| Architectural Layer | Core Technology & Runtime | Implementation Details & Source of Truth |
| :--- | :--- | :--- |
| **High-Throughput Gateway** | Compiled Go (Version 1.26.4) | `valyala/fasthttp` zero-allocation HTTP engine, `fasthttp/router`, `automaxprocs` (`transports/unifai-http/main.go`, `go.work`) |
| **Relational Persistence** | GORM Object-Relational Mapping | PostgreSQL 16.4+, Enterprise EDB Postgres, MySQL 8+, SQLite with CGO (`framework/configstore`, `framework/logstore`) |
| **Model Provider Adapters** | Native Go Provider Implementations | Vendor HTTP streaming + official SDKs normalizing 28+ LLM formats (`core/providers/*`) |
| **Extensibility Plugins** | Go Interfaces & Dynamic `.so` | In-process execution chain + dynamic shared object loading via `SharedObjectPluginLoader` (`plugins/`, `server/plugins.go`) |
| **MCP Integration Engine** | Model Context Protocol Specification | Full MCP client & server, code execution mode, OAuth2 token sessions (`core/mcp`, `handlers/mcp*.go`) |
| **Administrative UI Engine** | React 19, TypeScript 5.9, Vite 8 | Embedded Single Page Application (SPA), TanStack Router/Table, Redux Toolkit, Radix UI, Tailwind CSS v4, Monaco Editor, Recharts (`ui/`) |
| **Workstation Guard Agent** | Compiled Python 3.11+ / Embedded C++ | Native desktop daemon, mitmproxy inspection engine, PyInstaller hermetic packaging, Inno Setup 6 (Windows), PKG/DMG (macOS) (`apps/browser-guard/`) |
| **Container & Orchestration** | Docker (Multi-Stage) & Helm 3 | Kubernetes Helm chart with Horizontal Pod Autoscaler (HPA), Ingress TLS, Patroni PostgreSQL, Redis Sentinel (`deploy/`, `docker-compose.yml`) |
| **Build & Quality Tooling** | GNU Make, gotestsum, Playwright | End-to-end automated regression suite, oxlint/oxfmt, Newman API collection runners (`Makefile`) |

```
+----------------------------------------------------------------------------------------------------+
|                               UNIF-AI / YP-RAKSHA SYSTEM TOPOLOGY                                   |
|                                                                                                    |
|                     +--------------------------- UnifAI Go Binary (fasthttp) -------------------+ |
|                     | /v1/*         Inference Gateway (OpenAI-compatible)                       | |
|  AI Applications -> | /openai|anthropic|genai|bedrock|cohere|cursor|litellm|langchain  Pass-thru| |
|                     | /api/*        Dashboard, Governance, Observability, Browser AI Control   | |
|  Admin UI (Embed) ->| /ws, /v1/realtime  WebSocket + Realtime Bi-directional Streams            | |
|                     | /health, /metrics, /robots.txt, /pprof (Dev profiling)                   | |
|                     +-----------------------------+-------------------------------+-------------+ |
|                                                   |                               |               |
|                                         ConfigStore (Postgres)          LogsStore (Postgres)      |
|                                         Users, Roles, Virtual Keys,     LLM Logs, MCP Logs,       |
|                                         Providers, Governance Policies  Browser AI Logs, Traces   |
|                                                   |                               |               |
|                                     +-------------+-------------+                 |               |
|                                     |    UnifAI Core Engine     | ----------------+               |
|                                     |  Plugins & MCP Client     | -> Upstream LLM Providers       |
|                                     +---------------------------+    (~28 Enterprise Vendors)     |
|                                                                                                    |
|  Guard Fleet (Workstations & VDI) --- HTTPS ---> /api/browser-ai/* ----> Postgres browser_ai_*    |
+----------------------------------------------------------------------------------------------------+
```

---

## 2.2 Architectural Repository Layout & Core Engine Modules

The codebase is organized into a modular Go workspace (`go.work`) enabling strict package isolation, hermetic testing, and high developer velocity:

| Directory Path | Architecture Module | Functional Responsibilities & Core Subsystems |
| :--- | :--- | :--- |
| **`core/`** | Core AI Inference Engine | Vendor provider implementations (`core/providers/<vendor>`), shared schema contracts (`core/schemas`), Model Context Protocol (`core/mcp`), weighted key selection (`core/keyselectors`), and resilient network transport (`core/network`). |
| **`framework/`** | Enterprise Subsystems | ConfigStore relational metadata (`framework/configstore`), LogsStore audit persistence (`framework/logstore`), fine-grained RBAC (`framework/rbac`), hardware-accelerated encryption (`framework/encrypt`), clustering (`cluster`), routing rules (`routing`), circuit breakers (`circuitbreaker`), and connectors (`connectors`). |
| **`plugins/`** | Interception & Governance | Telemetry metrics, prompt repository management, audit logging, FinOps governance, OpenTelemetry tracing, vector semantic caching, client parameter compatibility, and inline guardrails. |
| **`transports/`** | Native Server Binary | Main HTTP entry point (`unifai-http/main.go`), server lifecycle & middleware assembly (`server/`), route handlers (`handlers/*.go`), provider pass-through bridges (`integrations/`), and embedded UI assets (`ui/`). |
| **`ui/`** | Management Console | React 19 single-page administrative dashboard (`ui/app/workspace/**`), RTK Query API client store, and interactive Browser AI governance controls. |
| **`apps/browser-guard/`** | Workstation Guard Fleet | Endpoint desktop agent daemon (`agent/`), mitmproxy inspection pipeline (`proxy/`), native OS installers (`installer/`), and release artifacts (`release/`). |
| **`configs/` & `deploy/`** | Infrastructure Deployment | Default runtime configurations (`config.json`, `pricing.json`), Docker container definitions (`deploy/docker/`), and enterprise Kubernetes Helm charts (`deploy/helm/`). |

### Core Provider Capability Matrix (`core/providers/`)
Each provider under `core/providers/<vendor>` implements the shared streaming and batch contract from `core/schemas`:
* **OpenAI (`core/providers/openai`):** Chat completions, responses lifecycle, text embeddings, image generation, speech audio, transcription, video processing, batch API, realtime WebSockets, and pass-through usage metering.
* **Anthropic (`core/providers/anthropic`):** Chat completions, token counting, Claude 3.5 Sonnet streaming, and thinking token capture.
* **Google Gemini & Vertex AI (`core/providers/gemini`, `core/providers/vertex`):** Multimodal prompt inspection, cached contents, embeddings, semantic reranking, and enterprise Vertex endpoints.
* **AWS Bedrock (`core/providers/bedrock`):** Multi-region AWS SDK v2 client, SigV4 request body signing (`bodysigner.go`), Converse API, embeddings, and Titan batch processing.
* **Mistral AI (`core/providers/mistral`):** Multimodal document parsing, native OCR engine (`ocr.go`), Codestral completions, and semantic embeddings.
* **Private On-Premises GPU Engines:** vLLM, Ollama, Hugging Face TGI, SGLang, and generic OpenAI-compatible local engines with dynamic endpoint discovery (`core/providers/openaicompat/registry.go`).

---

## 2.3 Universal AI Model Provider Fabric (28+ Model Adapters)

YP-Raksha acts as an enterprise translation gateway normalizing proprietary API shapes into a standard interface:

```
+----------------------------------------------------------------------------------------------------+
|                             UNIVERSAL AI GATEWAY ADAPTER FABRIC                                    |
|                                                                                                    |
|                                  +-----------------------+                                         |
|                                  |   Client Application  |                                         |
|                                  |   (OpenAI / Custom)   |                                         |
|                                  +-----------------------+                                         |
|                                              |                                                     |
|                                              v                                                     |
|                        +-------------------------------------------+                               |
|                        |       Universal AI Gateway Engine         |                               |
|                        |        (Normalization & Security)         |                               |
|                        +-------------------------------------------+                               |
|                                              |                                                     |
|         +-------------------+----------------+-------------------+-------------------+             |
|         |                   |                                    |                   |             |
|         v                   v                                    v                   v             |
|  +--------------+    +--------------+                     +--------------+    +--------------+     |
|  | Commercial   |    | Sovereign    |                     | Cloud Native |    | On-Premises  |     |
|  | Hyperscalers |    | Instances    |                     | Frameworks   |    | Private GPUs |     |
|  | • OpenAI     |    | • Azure (IN) |                     | • Bedrock    |    | • Ollama     |     |
|  | • Anthropic  |    | • AWS (IN)   |                     | • Vertex AI  |    | • vLLM       |     |
|  | • Mistral    |    | • GCP (IN)   |                     | • Cohere     |    | • TGI Server |     |
|  +--------------+    +--------------+                     +--------------+    +--------------+     |
+----------------------------------------------------------------------------------------------------+
```

### Supported Provider Integration Matrix
* **Tier-1 Commercial Hyperscalers:** OpenAI (GPT-4o, GPT-4 Turbo, o1, o3-mini), Anthropic (Claude 3.5 Sonnet, Claude 3 Opus, Claude 3.5 Haiku), Mistral AI (Mistral Large, Pixtral, Codestral), Cohere (Command R+), DeepSeek (DeepSeek-V3, DeepSeek-R1).
* **Enterprise Sovereign Cloud Endpoints:** Microsoft Azure OpenAI Service (Restricted exclusively to sovereign domestic data centers), Amazon Web Services Bedrock (Mumbai Sovereign Region), Google Cloud Platform Vertex AI (Mumbai / Delhi Sovereign Zones).
* **Private On-Premises Model Engines (Air-Gapped):** Local vLLM Inference Server, Ollama Enterprise Daemon, Hugging Face Text Generation Inference (TGI), NVIDIA Triton Inference Server, TensorRT-LLM on on-premise GPU clusters.
* **Developer Framework Compatibility:** LangChain, LlamaIndex, LiteLLM, AutoGen, CrewAI, PydanticAI, Cursor IDE, GitHub Copilot (via enterprise endpoint override).

<!-- pagebreak -->

## 2.4 Built-In Plugin Execution Chain & Dynamic `.so` Architecture

Every inference request traversing the YP-Raksha gateway passes through a strictly ordered, high-performance plugin pipeline. Plugins execute sequentially at fixed placement indexes:

| Order | Plugin Name | System Responsibility | Operational Interception Point |
| :---: | :--- | :--- | :--- |
| **1** | **`telemetry`** | Latency histograms, HTTP status counters, active connection tracking | Pre-Hook (Request arrival) & Post-Hook (Response completion) |
| **2** | **`prompts`** | Prompt repository resolution, template variable hydration, version tracking | Pre-Hook (Inspects incoming system & user prompts) |
| **3** | **`logging`** | Asynchronous audit log persistence, request/response payload hashing | Post-Hook (Writes audit records to LogsStore without blocking) |
| **4** | **`governance`** | Virtual Key validation, budget allocation, leaky-bucket rate limiting | Pre-Hook (Verifies departmental token quotas before provider transit) |
| **5** | **`otel`** | OpenTelemetry W3C distributed trace context injection and span export | Full Request Lifecycle (Propagates trace spans to SOC collectors) |
| **6** | **`semanticcache`**| Vector embedding response caching via Redis / Qdrant to eliminate cost | Pre-Hook (Bypasses upstream LLM on cosine similarity hit >= 0.95) |
| **7** | **`compat`** | Parameter translation between client shapes (e.g. converting Anthropic parameters to OpenAI) | Pre-Hook (Normalizes parameters prior to provider dispatch) |
| **8** | **`maxim`** | Maxim enterprise AI observability and evaluation platform connector | Post-Hook (Streams evaluation payloads to external observability sinks) |
| **—** | **`guardrails`** | Rule-based regex screening, arithmetic checksums, and hazard blocking | Pre-Hook & Streaming Post-Hook (Inline redaction / blocking) |
| **—** | **`connectors`** | Enterprise streaming sinks: Apache Kafka, Google Cloud PubSub, Datadog | Post-Hook (Asynchronous delivery to enterprise event brokers) |
| **—** | **`modelcatalogresolver`**| Dynamic model pricing lookup, token multiplier calculation | Pre-Hook (Enriches request context with exact cost parameters) |

### Dynamic Shared Object (`.so`) Plugin Loader
In addition to compiled built-in plugins, YP-Raksha incorporates a dynamic loader (`SharedObjectPluginLoader` in `server/plugins.go`). Enterprise security teams can compile proprietary Go plugins into shared objects (`.so`) using `go build -buildmode=plugin` and load them at runtime without recompiling the core gateway binary. Plugins can be dynamically enabled, disabled, and reordered via `/api/plugins`.

---

## 2.5 Guard Desktop Agent & mitmproxy Pipeline Architecture

The Workstation Guard fleet (`apps/browser-guard/`) provides client-side protection across enterprise laptops, branch workstations, and VDI sessions:

### Desktop Agent Internal Modules (`apps/browser-guard/agent`)
* **`unifai_agent.py`:** Core desktop service entry point and process lifecycle coordinator.
* **`guard_bootstrap.py`:** Single-instance mutex lock, logging bootstrap, and version verification.
* **`agent_config.py`:** Hierarchical configuration parser (CLI flags -> Environment variables -> JSON config).
* **`agent_identity.py`:** Hardware-bound persistent agent UUID generator and host metadata inventory.
* **`agent_heartbeat.py`:** Periodic 90-second heartbeat with randomized jitter, remote-uninstall poller, and health reporting.
* **`agent_health.py`:** Continuous localhost port monitor (`port_open`, `free_proxy_port`) ensuring proxy availability.
* **`agent_pac_content.py`:** Gateway connectivity verification, PAC script fetch, and atomic local file writing.
* **`agent_pac_server.py`:** Local loopback PAC HTTP server running on port 18195 (with automatic fallback to 18196/18197).
* **`agent_pac_orchestration.py`:** Windows Internet Settings (WinINet) registry orchestration with fail-open fallback.
* **`agent_proxy_engine.py`:** Process supervisor spawning and monitoring the background mitmproxy worker process.
* **`agent_proxy_bundle.py`:** SHA-256 verified hot-update bundle manager supporting dynamic zero-reboot rule updates.
* **`agent_certs.py`:** Installs the local mitmproxy root certificate into the Windows Root Certificate Store (`ca_install_status.txt`).
* **`agent_browser_policy.py`:** GPO and registry manager disabling browser QUIC/HTTP3 and DNS-over-HTTPS (DoH).
* **`agent_autoupdate.py`:** Compares local runtime version with server release manifest and executes silent in-place upgrades.
* **`agent_lifecycle.py`:** First-run onboarding wizard, employee notification dialogs, and key-gated uninstallation.

### mitmproxy Inspection Pipeline (`unifai_proxy_parts/MANIFEST.txt`)
The inspection engine executes as an ordered sequence of modules sharing a unified global namespace:
1. **`config_caches_rules.py`:** In-memory rule caching, target website catalog lookup, and regex pre-compilation.
2. **`helpers_prompts.py`:** Multipart form-data parser, JSON prompt extraction, and URL decoding.
3. **`uploads_detect.py`:** Deep inspection of file attachments uploaded to web chatbots.
4. **`file_policy.py`:** File-type policy enforcement (permitting or blocking PDF, DOCX, XLSX, CSV, and code files).
5. **`extract_office_backend.py`:** Text extraction from Office documents, PDF rasterization, and local OCR.
6. **`responses_inject.py`:** Browser-visible response rewriting and Server-Sent Events (SSE) block injection (`unifai_guard_blocked`).
7. **`responses_addon.py`:** Dynamic chatbot response analysis and conversation context learning.

<!-- pagebreak -->

## 2.6 Guard UI Architecture & Component Framework

The administrative dashboard is a modern React 19 Single Page Application (SPA) designed for security analysts and compliance officers:

| UI Subsystem | Technology Stack | Architecture & Operational Capabilities |
| :--- | :--- | :--- |
| **Core Framework** | React 19 & TypeScript 5.9 | Concurrent rendering, strict type contracts, and zero runtime layout shifts. |
| **Client-Side Routing** | TanStack Router | Type-safe, file-based routing (`ui/app/routeTree.gen.ts`) with deep URL state synchronization. |
| **Global State Management**| Redux Toolkit & RTK Query | Automated cache invalidation, optimistic UI mutations, and real-time polling. |
| **Component Primitives** | Radix UI & Tailwind CSS v4 | Accessible unstyled primitives, high-contrast dark mode, and bespoke slate/emerald styling. |
| **Code & Policy Editor** | Monaco Editor (VS Code Engine) | Live syntax highlighting, auto-completion, and inline validation for CEL and JSON policies. |
| **Data Visualization** | Recharts & Lucide Icons | Real-time threat distribution charts, token burn gauges, and interactive latency histograms. |
| **Binary Embedding** | Go `//go:embed` Virtual FS | The compiled production UI is embedded directly into the Go binary; zero separate web server required. |

### Browser AI Dedicated Management Suite (`ui/app/workspace/browser-ai/`)
* **Overview Tab (`overview`):** Fleet-wide metrics, active workstation counts, 24-hour threat heatmaps, and top blocked domains.
* **Logs Tab (`logs`):** Full-text searchable audit log of every intercepted prompt, file attachment, risk score, and policy verdict.
* **Search Logs Tab (`search-logs`):** Search-engine queries and outbound link clicks captured across monitored browsers.
* **Rules Tab (`rules`):** Policy builder supporting regular expression patterns, arithmetic checksums, and AI Guard Bot evaluators.
* **Targets Tab (`targets`):** Domain governance catalog defining which AI sites are monitored, intercepted, or completely blocked.
* **Agents Tab (`agents`):** Live fleet inventory displaying hostname, OS, IP address, heartbeat status, and proxy bundle version.
* **Telemetry Tab (`telemetry`):** Real-time interception latency waterfall, CPU/memory usage, and branch network metrics.
* **Setup Tab (`setup`):** Download links for Windows Setup.exe and macOS PKG installers, and one-click proxy bundle re-publishing.

---

## 2.7 Complete HTTP API Surface & Route Catalog

The gateway binary exposes two distinct operational planes: the `/v1/*` data plane and the `/api/*` control plane:

| Route Namespace | Key HTTP Endpoints | Authentication & RBAC Enforcement |
| :--- | :--- | :--- |
| **Inference Data Plane** | `POST /v1/chat/completions`<br/>`POST /v1/completions`<br/>`POST /v1/embeddings`<br/>`POST /v1/rerank`<br/>`POST /v1/ocr`<br/>`POST /v1/audio/transcriptions`<br/>`POST /v1/realtime` | Authenticated via Virtual Key or Master Provider Key (`Authorization: Bearer <key>` or `x-api-key: <key>`). Enforces FinOps budget quotas and rate limits. |
| **Provider Pass-Through** | `POST /openai/*`<br/>`POST /anthropic/*`<br/>`POST /genai/*`<br/>`POST /bedrock/*`<br/>`POST /cohere/*`<br/>`POST /cursor/*` | Direct native provider pass-through preserving vendor-specific headers and payload extensions while enforcing inline DLP. |
| **Browser AI Control** | `GET /api/browser-ai/logs`<br/>`GET /api/browser-ai/search-logs`<br/>`GET\|POST /api/browser-ai/rules`<br/>`GET\|PUT /api/browser-ai/controls`<br/>`GET /api/browser-ai/targets`<br/>`GET /api/browser-ai/agents` | Gated by RBAC resource `Logs: View/Update` or section grant `browser-ai`. Intercept events authenticated via `UNIFAI_GUARD_SECRET`. |
| **Guard Fleet Ingest** | `POST /api/browser-ai/intercept`<br/>`POST /api/browser-ai/intercept-file`<br/>`POST /api/browser-ai/agents/heartbeat`<br/>`GET /api/browser-ai/proxy.pac` | Secret-validated agent endpoints (`UNIFAI_GUARD_SECRET`). Fails closed if secret is missing or invalid (`UNIFAI_GUARD_REQUIRE_SECRET=1`). |
| **Governance & FinOps** | `GET\|POST /api/governance/virtual-keys`<br/>`GET\|POST /api/governance/teams`<br/>`GET\|POST /api/governance/business-units`<br/>`GET\|POST /api/governance/routing-rules` | Administrative session cookie or API key. Enforces fine-grained Resource × Operation RBAC. |
| **Workspace & Users** | `GET\|POST /api/users`<br/>`GET\|PUT /api/roles`<br/>`GET /api/permissions`<br/>`GET\|POST /api/access-profiles` | Admin-only privileged routes. Audited automatically by `WorkspaceAuditMiddleware`. |
| **Model Context Protocol**| `GET /api/mcp/clients`<br/>`GET /api/mcp/library`<br/>`GET\|POST /api/mcp/tool-groups`<br/>`POST /api/mcp/inference` | Gated by `MCPGateway: View/Create/Update` permissions and per-user credential vault. |
| **Health & Metrics** | `GET /health`<br/>`GET /metrics`<br/>`GET /robots.txt`<br/>`GET /api/version` | Unauthenticated public endpoints. `/health` performs concurrent pings to ConfigStore, LogsStore, and VectorStore. |

<!-- pagebreak -->

## 2.8 Technical Extension Points Matrix

YP-Raksha is architected for frictionless enterprise customization. The following matrix guides engineering teams extending platform functionality:

| Architectural Extension Goal | Primary Implementation Location | Integration Contract & Design Rules |
| :--- | :--- | :--- |
| **Add a New LLM Provider** | `core/providers/<vendor>/` | Create a new provider package implementing the `Provider` interface in `core/schemas`, then register in `core/providers/registry.go`. |
| **Add a Pass-Through API** | `transports/unifai-http/integrations/` | Implement the vendor HTTP bridge and register route prefix in `integrations/router.go`. |
| **Add an Interception Plugin** | `plugins/<plugin_name>/` | Implement `BasePlugin`, `LLMPlugin`, or `HTTPTransportPlugin`. Register in `server/plugins.go` (built-in) or compile as dynamic `.so`. |
| **Add a Dashboard API Route** | `transports/unifai-http/handlers/` | Create a new handler file with `RegisterRoutes(router, middlewares...)` and wire into `server.RegisterAPIRoutes`. |
| **Gate a Route with RBAC** | `framework/rbac/enforce.go` | Add the route mapping in `PathRequirementFor(method, path)` and configure section grants in `framework/rbac/sections.go`. |
| **Add an RBAC Resource** | `framework/configstore/workspace.go` | Add resource identifier to `RBACResourceNames`. All 6 operations (`Read`, `View`, `Create`, `Update`, `Delete`, `Download`) are generated automatically. |
| **Add a UI Dashboard Page** | `ui/app/workspace/<route>/` | Create `page.tsx` and `layout.tsx`, register the route in `ui/lib/constants/workspaceSections.ts`, and add navigation entry in `sidebar.tsx`. |
| **Add a Guard Detection Rule** | `apps/browser-guard/proxy/unifai_proxy_parts/` | Implement detection logic in the relevant proxy part, respecting the strict load order in `MANIFEST.txt`, and rebuild installer. |
| **Add a New Guard Rule Type** | `framework/logstore/browser_ai.go` | Update `BrowserGuardRule` GORM model, add frontend fields in `guardRuleAIEvaluatorFields.tsx`, and update proxy evaluator. |
| **Customize Inline Guardrails**| `plugins/guardrails/` | Extend inline inspection algorithms and configure validation policies via `/api/guardrails/*`. |

---

## 2.9 Complete GORM Relational Database Schemas

The database schema is strictly normalized across PostgreSQL relational tables managed via automated GORM migrations:

### Core Governance & ConfigStore Tables
* **`governance_users`:** Central user identity table (`id`, `username`, `email`, `password_hash`, `role`, `status`, `budget_id`, `rate_limit_id`, `allowed_sections`, `reviewed_at`).
* **`rbac_roles`:** Fine-grained role definitions (`id`, `name`, `description`, `is_system_role`, `dac` [`all-data` | `own-data`], `permission_ids` [JSON array]).
* **`access_profiles`:** Reusable enterprise governance policy templates (`id`, `name`, `version`, `is_active`, `calendar_aligned`, `spec` [JSON]).
* **`governance_virtual_keys`:** Cryptographic virtual credentials issued to applications (`id`, `key_hash`, `name`, `user_id`, `team_id`, `customer_id`, `budget`, `rate_limit`).
* **`governance_business_units`:** Departmental cost centers (`id`, `name`, `team_ids` [JSON], `budget` [JSON], `rate_limit` [JSON]).
* **`governance_routing_rules`:** Dynamic LLM model routing policies (`id`, `name`, `priority`, `condition_expression`, `target_provider_id`, `target_model_id`).

### Browser AI & Workstation Fleet Tables (`framework/logstore/browser_ai.go`)
* **`browser_ai_logs`:** Intercepted prompt and file upload records (`id`, `timestamp`, `platform`, `domain`, `user_prompt_full`, `action`, `status`, `rule_triggered`, `risk_score`, `predictive_risk`, `agent_id`, `agent_hostname`, `client_ip`, `attachment_name`, `attachment_size`, `attachment_path`).
* **`browser_ai_search_logs`:** Search engine activity (`id`, `timestamp`, `engine`, `browser`, `is_incognito`, `query`, `clicked_url`, `clicked_title`, `agent_id`).
* **`browser_guard_rules`:** Screening rules (`id`, `name`, `pattern`, `rule_type` [`regex` | `ai_bot`], `severity`, `action` [`BLOCK` | `REDACT` | `WARN`], `warning_message`, `active`, `bot_provider`, `bot_model`, `bot_prompt`).
* **`browser_ai_target_websites`:** Monitored domain catalog (`id`, `domain`, `platform_name`, `monitored`, `block_entire_website`, `host_role` [`ui` | `chat` | `file`], `status` [`MONITORED` | `PAUSED` | `BLOCKED`]).
* **`browser_ai_control_settings`:** Interaction policies (`id`, `allow_file_uploads`, `allow_file_downloads`, `warning_banner_text`, `enforce_strict_ssl`).
* **`browser_ai_agents`:** Workstation fleet inventory (`id`, `hostname`, `agent_type` [`endpoint` | `network`], `status` [`active` | `uninstall_pending` | `uninstalled`], `last_seen_at`, `health_detail`, `contact_email`, `uninstall_key_hash`, `uninstall_key_enc`, `proxy_bundle_sha`).
* **`browser_ai_agent_settings`:** Master enterprise uninstallation secret (`id`, `uninstall_key_enc`).
* **`browser_guard_fleet_config`:** Company-wide default configurations pulled by agents on heartbeat (`id`, `heartbeat_interval`, `pac_sync_interval`).
* **`browser_guard_rebuild_logs`:** Audit trail of proxy package builds (`id`, `status`, `bundle_sha`, `message`, `created_at`).
* **`browser_ai_warning_emails`:** Outbound security notification log (`id`, `sent_at`, `to_email`, `message`, `agent_id`, `error_detail`).

<!-- pagebreak -->

## 2.10 Enterprise Cryptography, FIPS-140 Baseline & Security Model

YP-Raksha enforces a multi-tier defense-in-depth security model designed to satisfy stringent banking and defense baselines:

| Security Dimension | Engineering Mechanism & Technical Implementation |
| :--- | :--- |
| **FIPS-140 Baseline** | All cryptographic operations execute via NIST-approved primitives (`crypto/tls`, `crypto/aes`, `crypto/sha256`). Master secrets derived via PKCS#11 HSM or HashiCorp Vault. |
| **Secrets at Rest** | Secrets stored in the database (provider master keys, webhook secrets, uninstall keys) are encrypted using AES-256-GCM via `framework/encrypt` when `UNIFAI_ENCRYPTION_KEY` is configured. |
| **Session Security** | Dashboard sessions use cryptographic UUIDs stored in the database, delivered via `HttpOnly`, `SameSite=Lax`, and `Secure` cookies with 24-hour expiration TTL. |
| **WebSocket Security** | WebSockets reject URL query token authentication to eliminate token leaks in logs; short-lived tickets are issued via authenticated `POST /api/session/ws-ticket`. |
| **Brute-Force Lockout** | Per-IP rate limiting (maximum 30 failed login attempts per 15-minute sliding window) combined with per-username database lockouts. |
| **Password Reset Security** | HMAC-SHA256 signed tokens using `PASSWORD_RESET_SECRET` (minimum 32 characters) with 10-minute validity, strict 5 requests/hour limits, and 2-minute cooldowns. |
| **Fleet Mutual Trust** | Workstation agents authenticate to backend APIs using `UNIFAI_GUARD_SECRET`. When `UNIFAI_GUARD_REQUIRE_SECRET=1`, all unauthenticated agent requests fail closed. |
| **PAC Anti-Hijack** | By default, `UNIFAI_PAC_ALLOW_QUERY_PROXY=0` forces the PAC script to serve only administrator-approved proxy IP addresses, completely preventing PAC-redirection attacks. |
| **Anti-Tamper Uninstallation**| Workstation agent uninstallation requires a 32-character enterprise uninstallation key verified against a salt-hashed SHA-256 digest in the central database. |

---

## 2.11 Production Environment Variables & Deployment Parameters

| Environment Variable | Type / Default | Operational Purpose & Architectural Impact |
| :--- | :--- | :--- |
| `APP_PORT` / `-port` | Integer (`8001`) | Primary TCP listening port for the unified Go HTTP server. |
| `APP_HOST` / `-host` | String (`localhost`) | Network interface binding (`0.0.0.0` binds to all network interfaces). |
| `APP_DIR` / `-app-dir` | Path (`./data`) | Local directory path for file attachments, offline buffers, and SQLite databases. |
| `UNIFAI_DB_TYPE` | String (`postgres`) | Persistence engine selection: `postgres`, `mysql`, or `sqlite`. |
| `UNIFAI_DB_DSN` | Connection String | Database Data Source Name (e.g. `postgresql://user:pass@host:5432/raksha_db?sslmode=verify-full`). |
| `UNIFAI_ENCRYPTION_KEY` | Hex String (64 chars) | 256-bit AES cryptographic master key used to encrypt database secrets at rest. |
| `UNIFAI_GUARD_SECRET` | String (Alphanumeric) | Shared secret used to authenticate workstation Guard agents to the `/api/browser-ai/*` backend. |
| `UNIFAI_GUARD_REQUIRE_SECRET`| Boolean (`1`) | When set to `1`, forces all agent ingest endpoints to fail closed if the secret is missing or invalid. |
| `UNIFAI_PAC_ALLOW_QUERY_PROXY`| Boolean (`0`) | When set to `0`, prevents clients from overriding the PAC proxy IP via URL query parameters. |
| `SERVER_DOMAIN` | String (FQDN) | Fully qualified domain name used to configure CORS allowed origins and generate PAC URLs. |
| `PASSWORD_RESET_SECRET` | String (Min 32 chars)| Secret key used to sign HMAC tokens for self-service password reset workflows. |
| `LOG_LEVEL` | String (`info`) | Logging verbosity: `debug`, `info`, `warn`, `error`. Production default is `info`. |

---

## 2.12 Developer, Build & Automated Test Workflow

The repository includes a comprehensive `Makefile` automating the entire continuous integration, containerization, and distribution pipeline:

```bash
# Gateway & Core Development Workflow
make setup-workspace     # Initializes the multi-module Go workspace (go work use ./core ./framework ...)
make dev                 # Concurrently runs Vite UI and Air live-reloading Go backend
make build               # Builds the production React UI and compiles the hermetic Go binary
make fmt ; make lint     # Enforces gofmt code styling and oxlint/golangci-lint quality gates

# Automated Verification & Test Suite
make test                # Executes unit tests for unifai-http transport via gotestsum
make test-core           # Tests core provider adapters (PROVIDER=openai TESTCASE=...)
make test-framework      # Runs relational persistence and RBAC verification tests
make test-plugins        # Validates plugin pipeline ordering and latency budgets
make test-all            # Executes the complete unit, integration, and performance test suite
make run-e2e             # Runs Playwright browser end-to-end automation test suite

# Workstation Guard Packaging Workflow
make sync-guard-config   # Synchronizes agent JSON configurations and documentation from .env
make build-guard-windows # Compiles Windows executable and packages Inno Setup 6 Setup.exe installer
make build-guard-mac     # Packages macOS Application Bundle (.app) and signed PKG installer
```

---

## 2.13 Enterprise SBOM, Third-Party Library Governance & SLSA Level 3

YP-Raksha adheres to Supply-chain Levels for Software Artifacts (SLSA) Level 3 security standards:
* **Hermetic & Reproducible Builds:** Gateways and host agents are compiled in fully isolated environments with pinned toolchains (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`). The resulting binary is bit-for-bit reproducible.
* **Cryptographic Provenance Attestation:** Every build generates an in-toto attestation signed with the bank's internal PKI or Cosign cryptographic keys, capturing the exact commit SHA, build environment, and SHA-256 hashes of all input sources.
* **Automated Daily SCA & CVE Auditing:** Automated pipelines scan the codebase daily using Syft, Grype, and Trivy against the NIST National Vulnerability Database (NVD). Releases with unresolved CVSS scores >= 7.0 are automatically rejected.
* **Tamper-Evident Delivery:** Artifacts are delivered alongside GPG signatures and SHA-256 checksums, verified by the bank's automated deployment pipelines prior to staging.
"""

# Reassemble Document 1
enriched_content = header_and_sec1 + "\n\n" + sec2_enriched.strip() + "\n"
MD1_PATH.write_text(enriched_content, encoding="utf-8")
print(f"Updated {MD1_PATH.name} ({len(enriched_content)} chars)")
