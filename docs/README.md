# UnifAI — Product Analysis & Documentation Set

> Source of truth: this repository at commit `16d4c3e` (`cline/708c4`).
> Everything below is derived from the code that is actually in this repo — no invented features.

---

## 1. What this product is

This monorepo ships **two products that share one backend, one database and one dashboard**:

### A. UnifAI Gateway — "The Fastest LLM Gateway" (Go)
A single Go binary (`transports/unifai-http`) built on `fasthttp`, with the built React
admin UI embedded into the binary via `go:embed`. It provides:

* **Unified AI API** — OpenAI-compatible `/v1/*` surface: chat completions, text completions,
  responses, embeddings, rerank, OCR, audio (speech/transcription), images (generate/edit/variation),
  videos, batches, files, containers, realtime (`/v1/realtime`) and WebSocket variants.
* **~28 provider integrations** (`core/providers/*`): OpenAI, Azure, Anthropic, Bedrock, Vertex/Gemini,
  Mistral, Cohere, Groq, Cerebras, DeepSeek, Fireworks, HuggingFace, Nebius, Ollama, OpenRouter,
  Perplexity, Replicate, Runware, Runway, SGL, vLLM, xAI, ElevenLabs, Parasail, OpenCode, Bedrock Mantle…
* **Native pass-through integrations** (`transports/unifai-http/integrations/*`) — clients keep their
  existing SDK and only change the base URL: `/openai/*`, `/anthropic/*`, `/genai/*`, `/bedrock/*`,
  `/cohere/*`, `/cursor/*`, `/litellm/*`, `/langchain/*`, `/pydanticai/*`.
* **Governance** — Virtual Keys, users/teams/customers/business units, budgets & rate limits,
  access profiles, RBAC roles/permissions, SCIM provisioning, audit logs.
* **Observability** — LLM logs, MCP logs, dashboard, connectors (BigQuery/Kafka/Datadog/New Relic/PubSub),
  alert channels (webhooks), tracing, Prometheus/OTel.
* **Platform features** — Guardrails, semantic cache, Prompt Repository, Skills Repository,
  MCP Gateway (catalog/library/clients/tool groups/OAuth sessions), Complexity Router,
  Routing Rules, Circuit Breaker, Adaptive/Load-Balancer routing, cluster/HA, feature flags, plugins.

### B. UnifAI Guard — "Browser AI" (Python desktop agent + optional network proxy)
An endpoint DLP control plane for **employee use of public AI websites in browsers**:

* A Python desktop agent (`apps/browser-guard/agent`, PyInstaller → Windows EXE / macOS `.app` / `.pkg`)
  runs a **local MITM proxy** (mitmproxy addon `apps/browser-guard/proxy/browser_ai_proxy.py`) and
  applies a **PAC** so only monitored AI domains are inspected.
* It detects **prompts, uploads, files and search queries**, applies admin **Guard Rules**
  (regex rules *or* LLM "AI Guard Bot" rules) with actions **BLOCK / REDACT / WARN**, and reports
  every event to the backend (`POST /api/browser-ai/intercept`).
* A **network/server proxy** mode (docker compose profile `network-proxy`, or bare metal with
  `UNIFAI_SERVER_MODE=1`) covers office PAC/GPO deployments — same rules, same dashboard.
* Admin control plane in the UI: Target Websites, Guard Rules (+ Excel/CSV import), Controls,
  Prompt Logs, Search Logs, Guard Agents (fleet), Guard Insights, Setup (packages, rebuild,
  uninstall key), fleet config, remote uninstall, warning e-mails.

**One dashboard:** Workspace → Browser AI shows laptop (`endpoint`) and network (`network`) agents,
rules, targets and logs together.

---

## 2. Analysis — how the pieces fit (request flows)

**Flow 1 — Employee browser → Guard → backend → dashboard**
```
Browser (ChatGPT / Claude / Gemini / …)
  → PAC  (from /api/browser-ai/pac, or the local PAC server on 127.0.0.1:18195)
  → local Guard MITM proxy (127.0.0.1:18103)
  → browser_ai_proxy.py: target match → prompt/upload extraction → rule evaluation
        (regex rules locally; AI Guard Bot rules → Ollama/LLM)
  → ALLOW / REDACT / WARN / BLOCK decision + injected browser response
  → POST /api/browser-ai/intercept   (Guard fleet secret; no session cookie)
  → Postgres table browser_ai_logs
  → UI: Workspace → Browser AI → Prompt Logs
```

**Flow 2 — AI app / developer → Gateway → provider**
```
Client (/v1/chat/completions, /openai/…, /anthropic/…)
  → fasthttp Server
    → SecurityHeaders → CORS → RequestDecompression
    → Auth.InferenceMiddleware (Virtual Key / provider API key)
    → Tracing → TransportInterceptor
    → handler → plugin chain (telemetry → prompts → logging → governance → otel →
        semantic cache → compat → maxim → guardrails → …)
    → provider (core/providers/*) → upstream LLM
  → LogsStore (Postgres) + Logging/Connectors plugins + WebSocket events
```

**Flow 3 — Admin UI → backend**
```
React UI (embedded, served by the Go binary)
  → session cookie `token` (HttpOnly, SameSite=Lax)
  → Auth.APIMiddleware → WorkspaceAuditMiddleware → RBACMiddleware
  → /api/* handler → ConfigStore / WorkspaceStore / LogsStore (Postgres)
```

**Bootstrap order (`server.Bootstrap`, `transports/unifai-http/server/server.go`)**
1. Load `config.json` from the app dir (`APP_DIR`, default `/app/data`).
2. Build ConfigStore + LogsStore (+ VectorStore) from config (Postgres via `env.DB_*`).
3. Start the log-retention cleaner (if `log_retention_days > 0`) and async-job cleaner.
4. Load plugins (built-ins + `.so` dynamic plugins), then the UnifAI core client.
5. Seed the model catalog from every configured provider and fetch live models.
6. Build middlewares: auth, workspace audit, RBAC, CORS, decompression, tracing.
7. Register API routes → inference routes → MCP clients → `/robots.txt` → UI routes.
8. `Start()` binds the listener, then handles SIGINT/SIGTERM with a 30 s graceful shutdown.

---

## 3. Component map

| Path | Language | Role |
|---|---|---|
| `core/` | Go (`github.com/unifai/unifai/core`) | LLM engine: providers, schemas, MCP client, key selectors, networking |
| `framework/` | Go | ConfigStore (postgres/mysql/sqlite), LogsStore, RBAC, encryption at rest, cluster/HA, routing, connectors, OAuth2, mailer, vector store |
| `plugins/` | Go | `telemetry, prompts, logging, governance, otel, semanticcache, compat, maxim, guardrails, connectors, modelcatalogresolver, mocker, jsonparser` |
| `transports/unifai-http` | Go | HTTP server (fasthttp), all `/api/*` + `/v1/*` handlers, integrations, WebSocket/realtime, embedded UI |
| `ui/` | React 19 + TS + Vite + TanStack Router + Redux Toolkit + Radix + Tailwind | Admin/user dashboard |
| `apps/browser-guard` | Python 3.11+ | Desktop Guard agent, mitmproxy addon, PAC, installers (Inno Setup / macOS pkg) |
| `configs/` | JSON | `config.json` (gateway config), `mcp-library.json`, `model-parameters.json`, `pricing.json` |
| `deploy/` | Docker + Helm | `Dockerfile(.local)`, compose entrypoint, Helm chart (Postgres/Qdrant/Redis/Weaviate, HPA, ingress) |
| `Makefile`, `go.work` | Make / Go | Build, dev, test, Guard packaging, Helm index, Docker image |

---

## 4. Key facts (as implemented)

| Item | Value |
|---|---|
| Brand | UnifAI; sample branding `UNIFAI_COMPANY_NAME=YesPanchi Group of Companies` |
| Gateway runtime | Go 1.26.4 workspace (`go.work`, 15 modules) |
| HTTP framework | `valyala/fasthttp` + `fasthttp/router` |
| Server default host/port | `localhost` / `8001`; env `APP_HOST`, `APP_PORT` (`.env.example` uses `6000`) |
| UI stack | React 19.2.3, Vite 8.0.16, TanStack Router 1.168, Redux Toolkit 2.8, Radix UI, Tailwind 4, Monaco, Recharts |
| Databases | Postgres (prod default), MySQL, SQLite (dev) — for both ConfigStore and LogsStore |
| Dashboard auth | Cookie `token` (HttpOnly, SameSite=Lax), default session TTL **24 h**; Bearer session token also accepted |
| Inference auth | Virtual Key / provider key via `Authorization: Bearer …` or `x-api-key` |
| Roles | `admin`, `sub_admin`, `user`, plus custom RBAC roles (system roles auto-seeded) |
| RBAC catalog | 33 resources × 6 operations (`Read, View, Create, Update, Delete, Download`) |
| Guard agent version | `1.1.15` (`apps/browser-guard/release/VERSION.txt`) |
| Guard local proxy | `127.0.0.1:18103` (`UNIFAI_PROXY_ADDR`) |
| Guard PAC server | `18195` (`PAC_HTTP_PORT`) |
| Docker network proxy | `18182` (`PROXY_PORT`, compose profile `network-proxy`) |
| Guard rule actions | `BLOCK`, `REDACT`, `WARN` |
| Guard rule severities | `CRITICAL`, `HIGH`, `MEDIUM` |
| Guard log actions | `Allowed`, `Redacted`, `Warned`, `Blocked`, `SiteBlocked`, `Bot Answered` |
| Agent types / statuses | `endpoint`, `network` / `active`, `uninstall_pending`, `uninstalled` |

---

## 5. Documentation set

| Document | Audience | Contents |
|---|---|---|
| **[TECHNICAL.md](./TECHNICAL.md)** | Developers, architects | Architecture, modules, data model, routes, plugins, Guard internals, UI structure, security model, extension points |
| **[SERVER.md](./SERVER.md)** | DevOps / IT | Requirements, env var reference, Docker/Helm deployment, DB & migrations, TLS/reverse proxy, backups, monitoring, upgrades, troubleshooting, hardening |
| **[USER_GUIDE.md](./USER_GUIDE.md)** | Employees / end users | Account lifecycle, login, workspace usage, Guard install/turn-off, what is monitored, troubleshooting, FAQ |
| **[ADMIN_GUIDE.md](./ADMIN_GUIDE.md)** | Admins / IT security | First-run admin, users & approvals, RBAC, access profiles, budgets, providers, Browser AI administration, fleet & uninstall keys, audit, backups, incident response |

---

## 6. Known limitations observed in the code

* `apps/browser-guard/config/unifai_guard_config.json` currently contains a **live-looking `guard_secret`**.
  The file itself carries a `_secret_note` stating it must never be committed. Rotate it and keep
  real secrets only in `.env` (git-ignored).
* Secrets (`UNIFAI_ENCRYPTION_KEY`, `UNIFAI_GUARD_SECRET`, `PASSWORD_RESET_SECRET`,
  `CLUSTER_REPLICATE_SECRET`) are **required** in production; the code fails closed — for example
  password reset returns HTTP 500 when `PASSWORD_RESET_SECRET` is missing or shorter than 32 chars.
* Guard EXE/.app and the Docker **network proxy** must not both MITM the same browser session
  (double-MITM). Typical split: office network → corporate PAC, remote/home → laptop Guard.
* Excel/CSV Guard Rule import supports **regex rules only**; `ai_bot` rules must be created in the UI.
* The in-app **Docs** page (`ui/lib/constants/docs.ts`) links to external UnifAI documentation;
  it is not generated from this `docs/` folder.
* The repo contains large sample/build artifacts (`Makefile` ~115 KB, `scratch_targets_backup.json`
  ~127 KB, `release/*.exe|.pkg|.zip`) — treat `apps/browser-guard/release/` as a build output directory.