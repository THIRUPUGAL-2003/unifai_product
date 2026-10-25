# UnifAI Settings & System Configuration Architecture — Complete Reference Guide

This document is the **definitive, production-grade guide** for all configuration subsystems under the **Settings** module (`/workspace/config`) in the UnifAI Gateway.

Every section provides:
- **Architecture & Usage**: What the subsystem does, why it exists, and how it impacts gateway behavior.
- **Detailed Parameters**: Every setting, data type, default value, and constraints.
- **Input Examples & Payloads**: Practical cURL, JSON, and UI form values.
- **Step-by-Step Operations**: How to Add, Edit, Toggle, and Delete settings.
- **Exhaustive Error Handling**: Exact error messages, root causes, and step-by-step resolution steps.
- **Audit & Compliance Impact**: How changes are logged to the Audit Logs for SOC-2 compliance.

---

## Table of Contents

1. [Global Storage & Hot-Reload Architecture](#1-global-storage--hot-reload-architecture)
2. [Section 1: Client Settings (`/workspace/config/client-settings`)](#2-section-1-client-settings)
3. [Section 2: Compatibility (`/workspace/config/compatibility`)](#3-section-2-compatibility)
4. [Section 3: Caching & Local Cache Hit Rate (`/workspace/config/caching`)](#4-section-3-caching--local-cache-hit-rate)
5. [Section 4: Security, Authentication & SMTP (`/workspace/config/security`)](#5-section-4-security-authentication--smtp)
6. [Section 5: API Keys & Authentication Models (`/workspace/config/api-keys`)](#6-section-5-api-keys--authentication-models)
7. [Section 6: Performance Tuning (`/workspace/config/performance-tuning`)](#7-section-6-performance-tuning)
8. [Section 7: Feature Flags & Section Gating (`/workspace/config/feature-flags`)](#8-section-7-feature-flags--section-gating)
9. [Section 8: Global Outbound Proxy (`/workspace/config/proxy`)](#9-section-8-global-outbound-proxy)
10. [Section 9: Logs & Data Retention Settings (`/workspace/config/logging`)](#10-section-9-logs--data-retention-settings)
11. [Section 10: Troubleshooting & SOC-2 Audit Trail Matrix](#11-section-10-troubleshooting--soc-2-audit-trail-matrix)

---

## 1. Global Storage & Hot-Reload Architecture

Settings in UnifAI are persisted in the configured database (SQLite `configs/config.db` for zero-setup local execution or PostgreSQL for production/multi-node clusters) across `workspace_settings`, `client_config`, `feature_flags`, and `smtp_config` tables, and mirrored in a process-wide in-memory configuration cache.

```mermaid
flowchart TD
    AdminUI[Admin Dashboard UI /workspace/config/*] -->|PUT /api/config/*| GatewayHTTP[Gateway HTTP Transport Handler]
    GatewayHTTP --> Validator[Schema Validation & Security Guard]
    Validator -->|Success| DB[(Database: SQLite / PostgreSQL)]
    DB --> InMemStore[In-Memory Atomic Configuration Store]
    InMemStore --> HotReload[Runtime Hot-Reload Manager]
    HotReload --> EngineWorkers[Auth / Proxy / Cache / Routing Workers]
    Validator -->|Violation 400/403/409| ErrorResponse[Detailed JSON Error + Toast Notification]
```

### Hot-Reload vs. Restart Matrix
* **Instant Hot-Reload (Zero Downtime, immediate next request)**:
  * Client Settings (`drop_excess_requests`, `dump_errors_in_console_logs`, `header_filter_config`, `async_job_result_ttl`)
  * Compatibility plugin reload/unload (`convert_text_to_chat`, `should_drop_params`, etc.)
  * Caching & Vector Store connection (`semantic_cache`, Qdrant/Weaviate/Pinecone/Redis reloads)
  * Security (`allowed_origins`, `allowed_headers`, `whitelisted_routes`, `enforce_auth_on_inference`)
  * Performance Tuning (`max_request_body_size_mb`)
  * Feature Flags (Live toggling of `section.*` and product behavior flags)
  * SMTP Configuration & Test Email dispatch
* **Requires Restart / Process Recycle**:
  * Outbound HTTP Proxy (`/api/proxy-config`) — Sets `RestartRequiredConfig = true` because persistent outbound client pools need re-binding.
  * Logging Engine instantiation (`enable_logging`) if the log store was completely uninitialized at startup.

---

## 2. Section 1: Client Settings

**Route:** `/workspace/config/client-settings`  
**API Endpoints:** `GET /api/config?from_db=true`, `PUT /api/config`, `GET/PUT /api/large-payload-config`

### 2.1 Purpose & Usage
Client Settings govern inbound HTTP connection dynamics, health check behavior, custom request header forwarding, and external storage offloading for massive payloads (e.g. multi-modal images, audio, long prompt completions).

### 2.2 Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `drop_excess_requests` | `boolean` | `false` | When enabled, drops excess incoming requests with HTTP 429 rather than letting them queue indefinitely when workers are saturated. |
| `disable_db_pings_in_health` | `boolean` | `false` | When enabled, `/health` and `/healthz` endpoints return `200 OK` without pinging PostgreSQL. Eliminates database connection exhaustion under rapid Kubernetes liveness probes. |
| `dump_errors_in_console_logs` | `boolean` | `false` | Dumps raw error stack traces to console stdout for deep debugging. |
| `async_job_result_ttl` | `integer` | `0` | TTL in seconds for asynchronous inference job results stored in logstore. `0` resets to the system default (86,400s / 24h). |
| `header_filter_config.allowlist` | `string[]` | `[]` | List of custom request headers forwarded upstream to providers. Wildcard `*` only allowed at the end (e.g. `x-custom-*`). |
| `header_filter_config.denylist` | `string[]` | `[]` | List of headers explicitly stripped before reaching upstream providers. |
| `large_payload.enabled` | `boolean` | `false` | Enables S3/GCS/MinIO blob offloading for payloads exceeding threshold sizes. |
| `large_payload.request_threshold_bytes` | `integer` | `1048576` (1MB) | Min 1024 bytes. Inbound request bodies larger than this are offloaded to object storage. |
| `large_payload.response_threshold_bytes` | `integer` | `1048576` (1MB) | Min 1024 bytes. Provider response bodies larger than this are offloaded to object storage. |
| `large_payload.prefetch_size_bytes` | `integer` | `65536` (64KB) | Min 1024 bytes. Initial byte chunk prefetched for streaming inspection. |
| `large_payload.max_payload_bytes` | `integer` | `104857600` (100MB) | Min 1024 bytes. Hard ceiling on payload size. Must be $\ge$ request and response thresholds. |
| `large_payload.truncated_log_bytes` | `integer` | `4096` (4KB) | Min 1024 bytes. Payload chunk recorded in database logs before truncating. |

### 2.3 Forbidden Security Headers
The following headers are **strictly forbidden** from being placed in `allowlist` or `denylist`:
`authorization`, `proxy-authorization`, `cookie`, `host`, `content-length`, `connection`, `transfer-encoding`, `x-api-key`, `x-goog-api-key`, `x-uf-api-key`, `x-uf-vk`.

### 2.4 Practical Examples & Payloads

#### Updating Client Settings via cURL
```bash
curl --location --request PUT 'http://localhost:3100/api/config' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "client_config": {
      "drop_excess_requests": true,
      "disable_db_pings_in_health": true,
      "dump_errors_in_console_logs": false,
      "async_job_result_ttl": 3600,
      "header_filter_config": {
        "allowlist": ["x-tenant-id", "x-correlation-*"],
        "denylist": ["x-internal-debug"]
      }
    }
  }'
```

### 2.5 Operations Guide
1. **Toggle Switches**: Click the toggle next to *Drop Excess Requests*, *Disable DB Pings in Health*, or *Dump Errors in Console*.
2. **Add Header to Allowlist/Denylist**: Click **+ Add Header**, type the header name or trailing pattern (e.g. `x-custom-*`), and press Enter.
3. **Delete Header**: Click the **X** icon next to the header badge.
4. **Save**: Click **Save Changes**. Toast notification confirms: `"Client settings updated. Changes apply on the next request."`

### 2.6 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `the following headers are not allowed to be configured: [header]. These headers are security headers and are always blocked` | User added `authorization`, `x-api-key`, `cookie`, or `x-uf-vk` into allowlist/denylist. | Remove the reserved security header from the list. These headers are managed automatically by the gateway's security core. |
| `invalid pattern "...": wildcard (*) is only supported at the end of a pattern` | Pattern formatted like `x-*-id` or `*custom`. | Move the wildcard to the end: `x-custom-*`. Leading or middle wildcards are rejected. |
| `async_job_result_ttl must be 0 (default) or a positive number of seconds` | Negative number entered into TTL field. | Enter `0` for default (24h) or a positive integer (e.g., `3600` for 1 hour). |
| `All byte values must be at least 1024 (1 KB).` | A large payload threshold field is set below 1024 bytes. | Increase the value to at least 1024 (e.g., `1048576` for 1 MB). |
| `Max payload size must be greater than or equal to the request threshold.` | `max_payload_bytes` is smaller than `request_threshold_bytes`. | Ensure `max_payload_bytes` is larger than or equal to both request and response thresholds. |

---

## 3. Section 2: Compatibility

**Route:** `/workspace/config/compatibility`  
**API Endpoints:** `GET /api/config?from_db=true`, `PUT /api/config`

### 3.1 Purpose & Usage
The Compatibility module acts as an intelligent shim between diverse LLM provider formats. It allows applications built for the OpenAI Chat Completions API (`/v1/chat/completions`) to seamlessly communicate with completion-only models, Anthropic Claude, Bedrock, or Cohere without rewriting client code.

```mermaid
flowchart LR
    App[Client Application] -->|OpenAI Chat Request| Gateway[UnifAI Gateway]
    Gateway --> CompatPlugin{Compat Plugin Enabled?}
    CompatPlugin -->|Convert Text to Chat| Shims1[Transforms Prompt to messages[]]
    CompatPlugin -->|Drop Unsupported Params| Shims2[Strips vendor-incompatible fields]
    CompatPlugin -->|Convert Param Values| Shims3[Maps max_tokens <-> max_completion_tokens]
    CompatPlugin -->|Disabled| Direct[Direct Forwarding]
    Shims1 & Shims2 & Shims3 --> Upstream[Target LLM Provider: Anthropic / Bedrock / OpenAI]
```

### 3.2 Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `compat.convert_text_to_chat` | `boolean` | `true` | Automatically wraps legacy `/v1/completions` prompt requests into `messages: [{"role": "user", "content": prompt}]` for chat-only models. |
| `compat.convert_chat_to_responses` | `boolean` | `false` | Converts chat completion requests into text responses for legacy completion models. |
| `compat.should_drop_params` | `boolean` | `true` | Drops unsupported parameters based on the model catalog parameter allowlist (e.g. removes `frequency_penalty` when forwarding to Claude 3.5 Sonnet to prevent upstream 400 Bad Request errors). |
| `compat.should_convert_params` | `boolean` | `true` | Normalizes vendor-specific parameter variations (e.g. converting `max_tokens` to `max_completion_tokens` on OpenAI `o1` reasoning models). |

### 3.3 Dynamic Hot-Reload Behavior
When any of the 4 flags is enabled, the Gateway HTTP runtime calls:
```go
h.configManager.ReloadPlugin(ctx, compat.PluginName, nil, compatCfg, nil, nil)
```
If all 4 flags are toggled off, the Gateway removes the plugin from memory entirely (`RemovePlugin`), eliminating translation overhead.

### 3.4 Practical Example & Payload
```bash
curl --location --request PUT 'http://localhost:3100/api/config' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "client_config": {
      "compat": {
        "convert_text_to_chat": true,
        "convert_chat_to_responses": false,
        "should_drop_params": true,
        "should_convert_params": true
      }
    }
  }'
```

### 3.5 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `Failed to load compat plugin` | Gateway plugin pipeline failed to initialize the in-memory compatibility plugin hook. | Check server logs for memory or plugin registration conflicts. Toggle settings off and re-save. |
| `Upstream 400 Bad Request: unknown field 'top_k'` | `should_drop_params` is disabled, and an OpenAI request with `top_k` was sent to a provider that rejects it. | Enable **Drop Unsupported Params** (`should_drop_params: true`) and save. |

---

## 4. Section 3: Caching & Local Cache Hit Rate

**Route:** `/workspace/config/caching`  
**API Endpoints:** `GET /api/vector-store-config`, `PUT /api/vector-store-config`, `GET/POST/PUT /api/plugins` (`semantic_cache`)

### 4.1 Purpose & Usage
UnifAI provides a dual-mode caching engine to dramatically reduce LLM inference costs and response latency:
1. **Direct Cache (Exact Hash Matching)**:
   * Hashes the exact request parameters (model, prompt/messages, temperature).
   * Operates completely in-memory (or via Redis key lookup) without embedding models.
   * Delivers sub-millisecond cache hits for identical user prompts.
2. **Semantic Cache (Vector Embedding Similarity)**:
   * Converts the user's prompt into a vector embedding using an embedding model.
   * Queries an external Vector Database (Qdrant, Weaviate, Pinecone, or Redis) using cosine similarity.
   * If similarity score $\ge$ configured threshold (e.g., 0.85), returns the cached response, saving up to 80% on LLM API costs.

### 4.2 Dashboard Integration: Local Cache Hit Rate
On the Observability Dashboard (`/workspace/observability`), the **"Local Cache Hit Rate"** gauge directly correlates with this subsystem:
* **Metric Calculation**:
  $$\text{Local Cache Hit Rate} = \frac{\text{DirectCacheHits} + \text{SemanticCacheHits}}{\text{Total Inference Requests}} \times 100\%$$
* Recorded into the `cache_debug` column in database request logs.
* **CRITICAL GATE**: Caching requires the **`cache.responses`** Feature Flag to be enabled (`/workspace/config/feature-flags`). If this flag is off, all cache checks are bypassed!

### 4.3 Vector Store Connection Parameters

#### 1. Qdrant (Recommended)
* **Host**: IP or hostname (e.g. `localhost` or `qdrant.internal`).
* **Port**: Default `6334` (gRPC port).
* **API Key**: Optional token for Qdrant Cloud.

#### 2. Weaviate
* **Scheme**: `http` or `https`.
* **Host**: e.g. `localhost:8080` or `cluster.weaviate.network`.
* **API Key**: Optional API key for Weaviate Cloud.

#### 3. Pinecone
* **API Key**: Pinecone API Secret Key.
* **Index Host**: Full index URL (e.g. `https://unifai-cache-xyz.svc.pinecone.io`).

#### 4. Redis
* **Addr**: Host and port (e.g. `redis.internal:6379`).
* **Username / Password**: Optional Redis ACL credentials.

### 4.4 Cache Tuning Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `ttl` | `integer` | `300` | Expiration time in seconds for cached responses. `0` = indefinite retention. |
| `threshold` | `float` | `0.80` | Cosine similarity threshold (0.0 to 1.0). Higher = stricter semantic match; lower = looser match. |
| `dimension` | `integer` | `1` (Direct) / `> 1` (Semantic) | Embedding vector dimension. Must match the embedding model (e.g. `1536` for `text-embedding-3-small`). |
| `conversation_history_threshold`| `integer` | `3` | Number of previous conversation turns included in the prompt cache hash. |
| `exclude_system_prompt` | `boolean` | `false` | When true, system prompts are excluded from semantic distance calculation. |
| `cache_by_model` | `boolean` | `true` | When true, cache hits are isolated per model. |
| `cache_by_provider` | `boolean` | `true` | When true, cache hits are isolated per provider. |
| `vector_store_namespace` | `string` | `null` | Multi-tenant partition namespace in the vector database. |

### 4.5 Step-by-Step Configuration Guides

#### Setting up Direct Caching
1. Navigate to `/workspace/config/caching`.
2. Under **Cache Mode**, select **Direct**.
3. Set **TTL (Seconds)** to desired value (e.g., `3600` for 1 hour).
4. Check **Cache by Model** and **Cache by Provider**.
5. Click **Save Cache Settings**.

#### Setting up Semantic Caching with Qdrant
1. Under **Vector Store Connection**, select **Qdrant**.
2. Enter **Host** (`localhost`), **Port** (`6334`), and optional API Key.
3. Click **Connect Vector Store**. The status badge changes to **Connected**.
4. Under **Cache Mode**, switch to **Semantic**.
5. Choose an **Embedding Provider** (e.g., `openai`).
6. Select an **Embedding Model** (e.g., `text-embedding-3-small`).
7. Enter **Dimension** (`1536`).
8. Set **Similarity Threshold** (e.g., `0.85`).
9. Click **Save Cache Settings**.

### 4.6 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `Pick an embedding provider for semantic mode, or switch to Direct only.` | Mode is set to Semantic, but no embedding provider was selected. | Select an embedding provider (OpenAI, Azure, Bedrock, Cohere) or switch back to Direct mode. |
| `Semantic mode requires the embedding model's real dimension (must be > 1).` | Dimension is set to `1` or `0` in Semantic mode. | Enter the exact dimension of your embedding model (e.g., `1536` for OpenAI `text-embedding-3-small`, `768` for Titan). |
| `Similarity threshold must be between 0 and 1.` | Threshold entered is $< 0$ or $> 1$. | Enter a float between `0.0` and `1.0` (typically `0.80` to `0.90`). |
| `Qdrant host is required.` | Vector store is enabled with Qdrant type, but host field is empty. | Provide the hostname or IP address of your Qdrant instance. |
| `Pinecone index host is required.` | Pinecone index host was omitted. | Provide the full Pinecone host URL (e.g. `https://my-index.svc.pinecone.io`). |
| `External vector store disconnected. Cache is using process memory until you reconnect.` | External vector database became unreachable. | The gateway automatically falls back to in-memory caching to prevent inference downtime. Check vector DB network connectivity and re-save. |
| `Local Cache Hit Rate on Dashboard is always 0%` | Either no cache hits occurred yet, or the `cache.responses` feature flag is disabled. | 1. Go to `/workspace/config/feature-flags` and ensure `cache.responses` is **ON**.<br>2. Send identical requests to verify Direct Cache hits. |

---

## 5. Section 4: Security, Authentication & SMTP

**Route:** `/workspace/config/security`  
**API Endpoints:** `GET/PUT /api/config`, `GET/PUT /api/smtp-config`, `POST /api/smtp-config/test`

### 5.1 Architecture & Capabilities
The Security module enforces network origin protection, administrative authentication, password complexity policies, Super Admin lockout immunity, and transactional SMTP email delivery.

```mermaid
flowchart TD
    Inbound[Inbound HTTP Request] --> OriginCheck{Origin in allowed_origins?}
    OriginCheck -->|No| RejectCORS[403 CORS Forbidden]
    OriginCheck -->|Yes| ReqHeaderCheck{Required Headers Present?}
    ReqHeaderCheck -->|Missing| RejectHeader[400 Bad Request: Missing Required Header]
    ReqHeaderCheck -->|Pass| AuthCheck{Is Admin /api/* Route?}
    AuthCheck -->|Yes| SessionValid{Valid Session Cookie or Token?}
    SessionValid -->|Valid| ExecuteAdmin[Allow Admin Request]
    SessionValid -->|Invalid| CheckLockout{Failed Attempts >= 3?}
    CheckLockout -->|Super Admin| AllowRetry[Exempt from Lockout: Prompt Password Again]
    CheckLockout -->|Regular Admin| Lockout20m[423 Account Locked for 20 Minutes]
```

### 5.2 Network Security Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `allowed_origins` | `string[]` | `[]` | Allowed CORS origins. Supports exact URLs (`https://app.example.com`), wildcards (`https://*.example.com`), or `*` for all. |
| `allowed_headers` | `string[]` | `[]` | HTTP headers permitted during CORS preflight `OPTIONS` requests. |
| `required_headers` | `string[]` | `[]` | Mandatory headers for all incoming requests. Missing headers return 400 Bad Request. |
| `whitelisted_routes` | `string[]` | `[]` | Gateway routes exempt from authentication (e.g. `/health`, `/api/version`). |
| `enforce_auth_on_inference` | `boolean` | `false` | When true, all `/v1/*` inference endpoints require a valid Virtual Key (`Authorization: Bearer <key>` or `x-uf-vk`). |
| `allow_direct_keys` | `boolean` | `false` | When true, permits clients to bypass configured provider keys using `x-uf-direct-key`. |

### 5.3 Administrative Password Complexity Policy
All admin passwords (Super Admin and standard admins) must satisfy **all 5 requirements**:
1. At least **8 characters** in length.
2. At least **one uppercase letter** (`[A-Z]`).
3. At least **one lowercase letter** (`[a-z]`).
4. At least **one digit** (`[0-9]`).
5. At least **one special character** (`[^A-Za-z0-9]`, e.g. `!@#$%^&*`).

> [!NOTE]
> When retrieving configuration via `GET /api/config`, existing passwords return as `<redacted>`. Submitting `<redacted>` during a settings save preserves the existing hashed password without re-hashing or invalidating active sessions.

### 5.4 Super Admin Account Lockout Exemption Rule
* **Standard Admins**: Incur a **20-minute account lockout** (`loginLockoutDuration`) after **3 consecutive failed login attempts** (`loginMaxFailedAttempts`).
* **Super Admin (Exempt)**: The primary administrative account defined in environment variables (`ADMIN_USERNAME` / `ADMIN_PASSWORD`) or configured in Security settings is **strictly EXEMPT from account lockout**. Even after 10 failed attempts, the Super Admin can still log in once the correct password is provided. This prevents administrative denial-of-service.
* **3 Ways to Update Super Admin Password**:
  1. **Environment Variables**: Update `ADMIN_PASSWORD` in `.env` and restart the gateway container.
  2. **Security Settings UI**: Navigate to `/workspace/config/security`, enter the new password, and click **Save Changes**.
  3. **Forgot Password Flow**: Click "Forgot Password" on the login screen; an OTP is sent via SMTP to verify and reset the password.

> [!IMPORTANT]
> Any change to admin credentials immediately invokes `store.ConfigStore.FlushSessions(ctx)`, terminating all active sessions and requiring re-login.

### 5.5 SMTP Configuration & Email Alerts
Transactional email service is used for password reset OTPs, welcome credentials for new users, and login alert notifications.

SMTP connection details are saved and managed in **Settings → Security**. Outgoing mail uses that saved configuration; there is no `SMTP_*` environment-variable fallback.

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | `boolean` | `false` | Enables transactional SMTP email delivery. |
| `host` | `string` | `""` | SMTP server hostname (e.g. `smtp.gmail.com`, `smtp.office365.com`, `email-smtp.us-east-1.amazonaws.com`). |
| `port` | `integer` | `587` | Port number. `587` for STARTTLS, `465` for direct SSL/TLS. |
| `username` | `string` | `""` | SMTP authentication username / email. |
| `password` | `string` | `""` | SMTP authentication password (for Gmail, use an **App Password**). |
| `from_email` | `string` | `""` | Sender email address (e.g. `security@unifai.company.com`). |
| `from_name` | `string` | `Raksha Security System` | Sender display name. |
| `use_tls` | `boolean` | `true` | Enforces TLS encryption for the SMTP handshake. |
| `notify_on_login` | `boolean` | `false` | Sends an email alert on every successful admin login. |
| `notify_on_user_create` | `boolean` | `true` | Sends a welcome email with temporary credentials when new users are created. |

> [!WARNING]
> Before saving SMTP passwords, the environment variable `GATEWAY_ENCRYPTION_KEY` must be configured with a 32-byte AES key. The gateway encrypts SMTP passwords at rest in PostgreSQL (`tables.EncryptionStatusEncrypted`).

### 5.6 Practical Examples & Payloads

#### Updating Security Configuration
```bash
curl --location --request PUT 'http://localhost:3100/api/config' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "client_config": {
      "allowed_origins": ["http://localhost:3000", "https://*.corp.internal"],
      "allowed_headers": ["Content-Type", "Authorization", "x-uf-vk"],
      "enforce_auth_on_inference": true,
      "allow_direct_keys": false
    },
    "auth_config": {
      "is_enabled": true,
      "admin_username": { "value": "superadmin" },
      "admin_password": { "value": "P@ssw0rd2026!" }
    }
  }'
```

#### Testing SMTP Connection
```bash
curl --location --request POST 'http://localhost:3100/api/smtp-config/test' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{ "to": "admin@company.com" }'
```

### 5.7 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `Password must include at least 8 characters, one uppercase letter, one lowercase letter, one number, one special character.` | Password violates one or more of the 5 complexity rules. | Choose a password that meets all 5 criteria (e.g. `UnifAI#Secure2026`). |
| `Invalid origins: [origin]. Origins must be valid URLs like https://example.com, wildcard patterns like https://*.example.com, or "*"` | Origin lacks scheme (`example.com` instead of `https://example.com`) or has invalid syntax. | Include the scheme: `https://example.com` or `https://*.example.com`. |
| `Set GATEWAY_ENCRYPTION_KEY before saving SMTP passwords so credentials are encrypted at rest` | `GATEWAY_ENCRYPTION_KEY` is missing from the environment. | Set `GATEWAY_ENCRYPTION_KEY="your-32-byte-hex-or-base64-secret"` in `.env` and restart the gateway. |
| `SMTP test failed: 535 5.7.8 Username and Password not accepted` | Incorrect SMTP username or password; for Gmail, two-factor auth requires an App Password. | Generate an **App Password** in your Google Account Security settings and paste it into the SMTP Password field. |
| `enable and save SMTP settings first` | User clicked **Test SMTP** without saving the configuration or before toggling SMTP **Enabled**. | Toggle **Enable SMTP**, fill in Host/Username/Password, click **Save SMTP Settings**, then click **Send Test Email**. |

---

## 6. Section 5: API Keys & Authentication Models

**Route:** `/workspace/config/api-keys`  
**API Endpoints:** `POST /api/session/login`, `GET /api/session/user`, `GET/POST /api/governance/virtual-keys`

### 6.1 Two Distinct Authentication Paradigms
UnifAI maintains a strict separation between **Platform Management** and **Model Inference**:

```mermaid
flowchart TD
    Client[Client Request] --> AuthRouter{Target Endpoint?}
    AuthRouter -->|/api/* Management Endpoints| AdminAuth[Admin Session Token / Cookie]
    AuthRouter -->|/v1/* Inference Endpoints| VKAuth[Virtual Key: Authorization Bearer or x-uf-vk]
    AdminAuth --> SessionStore[In-Memory Session Store / Redis]
    SessionStore --> RBAC[Role-Based Access Control]
    VKAuth --> VKStore[Virtual Key Store & Policy Engine]
    VKStore --> Budgets[Budget & Rate Limit Check]
```

#### 1. Admin Session Authentication (`/api/*`)
* **Scope**: All management endpoints (`/api/config`, `/api/feature-flags`, `/api/governance/*`, `/api/users`, `/api/roles`).
* **Authentication Method**:
  * Web Browser: Secure HTTP-Only session cookie (`unifai_session`).
  * Automation / CLI: `Authorization: Bearer <session-token>` header obtained from `POST /api/session/login`.
* **Important**: Basic Auth (`Authorization: Basic ...`) is **strictly rejected**.

#### 2. Virtual Keys for Inference (`/v1/*` & `/mcp/*`)
* **Scope**: LLM inference (`/v1/chat/completions`, `/v1/embeddings`) and Model Context Protocol tool execution.
* **Authentication Method**:
  * Header: `Authorization: Bearer unifai_vk_...`
  * Header: `x-uf-vk: unifai_vk_...`
* **Features**: Scoped by model allowlists, monthly spending budgets, rate limits, expiration dates, and team access profiles.

### 6.2 Practical Examples & cURL Commands

#### Admin Login & API Call
```bash
# 1. Sign in to obtain a session cookie
curl --location 'http://localhost:3100/api/session/login' \
  --cookie-jar cookies.txt \
  --header 'Content-Type: application/json' \
  --data '{
    "username": "superadmin",
    "password": "P@ssw0rd2026!"
  }'

# 2. Call admin endpoints using the stored cookie
curl --location 'http://localhost:3100/api/governance/virtual-keys' \
  --cookie cookies.txt
```

#### LLM Inference with a Virtual Key
```bash
curl --location 'http://localhost:3100/v1/chat/completions' \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer unifai_vk_9a8b7c6d5e4f3a2b1c' \
  --data '{
    "model": "openai/gpt-4o",
    "messages": [
      { "role": "user", "content": "Explain quantum computing in one sentence." }
    ]
  }'
```

### 6.3 Quick Virtual Key Creation
In `/workspace/config/api-keys`:
1. Enter a name in the **Key name** input (e.g. `production-backend-key`).
2. Click **Create Key**.
3. A toast displays the full secret key (`unifai_vk_...`). Copy it immediately—the full secret is never displayed again.

### 6.4 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `401 Unauthorized: Invalid session or session expired` | Session cookie or token is missing, expired, or invalidated by an admin password change. | Re-authenticate via `POST /api/session/login` to obtain a fresh session token. |
| `401 Unauthorized: Authentication required for inference requests` | `enforce_auth_on_inference` is enabled in Security settings, but no Virtual Key was sent. | Add `Authorization: Bearer <your_virtual_key>` or `x-uf-vk: <your_virtual_key>` header to the inference request. |
| `To use admin APIs, set up admin username and password first` | Security settings have not been initialized. | Go to `/workspace/config/security`, enable Dashboard Authentication, set credentials, and save. |

---

## 7. Section 6: Performance Tuning

**Route:** `/workspace/config/performance-tuning`  
**API Endpoints:** `GET /api/config?from_db=true`, `PUT /api/config`

### 7.1 Architecture & Worker Pools
UnifAI Gateway is powered by **Fasthttp**, an ultra-high-performance Go HTTP engine. The Performance Tuning section optimizes memory buffer allocations and payload acceptance limits.

```mermaid
flowchart LR
    ClientTraffic[Concurrent Client Traffic] --> WorkerPool[Fasthttp Object Worker Pool (initial_pool_size)]
    WorkerPool --> MemoryBuf[Pre-allocated Byte Slices]
    MemoryBuf --> BodyLimiter{Request Size <= max_request_body_size_mb?}
    BodyLimiter -->|Yes| FastRouting[Fast Pipeline Execution]
    BodyLimiter -->|No| Reject413[413 Payload Too Large]
```

### 7.2 Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `initial_pool_size` | `integer` | `1000` | Pre-allocated worker pool capacity for fasthttp request/response objects. Raising or lowering replaces the worker pool on save. |
| `max_request_body_size_mb` | `integer` | `100` | Largest HTTP request body (in MB) accepted by the gateway. Applies immediately to the next request without server restart. |

### 7.3 Sizing Guide & Recommendations
* **Low / Dev Environments (< 100 RPS)**:
  * `initial_pool_size`: `1000`
  * `max_request_body_size_mb`: `50`
* **Production High-Throughput (> 500 RPS)**:
  * `initial_pool_size`: `5000` to `10000` (reduces memory re-allocation and Go garbage collection pauses).
  * `max_request_body_size_mb`: `100` to `200` (allows large document/image embedding payloads).

### 7.4 Practical Example & Payload
```bash
curl --location --request PUT 'http://localhost:3100/api/config' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "client_config": {
      "initial_pool_size": 5000,
      "max_request_body_size_mb": 150
    }
  }'
```

### 7.5 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `Initial pool size must be a positive number.` | Value entered is $\le 0$, non-numeric, or blank. | Enter a positive integer (e.g. `1000`). |
| `Max request body size must be a positive number.` | Value entered is $\le 0$, non-numeric, or blank. | Enter a positive integer (e.g. `100`). |
| `413 Request Entity Too Large` | Client uploaded a file or prompt larger than `max_request_body_size_mb`. | Increase `max_request_body_size_mb` in Performance Tuning or enable Large Payload object storage offloading. |

---

## 8. Section 7: Feature Flags & Section Gating

**Route:** `/workspace/config/feature-flags`  
**API Endpoints:** `GET /api/feature-flags`, `POST /api/feature-flags`, `PUT /api/feature-flags/{id}`, `DELETE /api/feature-flags/{id}`

### 8.1 Dual Architecture
UnifAI features a two-tiered feature flag engine:

```mermaid
flowchart TD
    Request[HTTP Request] --> GateMiddleware{Path matches /api/section-name/* ?}
    GateMiddleware -->|Yes| CheckFlag{Is section.name enabled?}
    CheckFlag -->|Disabled| Forbidden403[403 Forbidden: Section is disabled]
    CheckFlag -->|Enabled| AllowEndpoint[Execute Section Handler]
    GateMiddleware -->|No: Inference Path| ProductFlag{Check Platform Flags}
    ProductFlag -->|cache.responses| RunCache[Execute Caching Engine]
    ProductFlag -->|browser.prompt_guard| RunGuard[Scan Prompts with Guardrails]
```

#### 1. Section Flags (`section.<id>`)
Section flags govern workspace menus and their backing `/api/*` endpoints via `SectionFlagMiddleware`.
* **When ON**: Menu appears in the sidebar, and `/api/*` requests succeed.
* **When OFF**: Menu is hidden from the sidebar, and any direct HTTP request to that module's API returns:
  ```json
  { "error": "This section is currently disabled in Feature Flags" }
  ```
  *(HTTP 403 Forbidden)*

##### The 15 Standard Section Flags:
1. `section.observability`: Dashboard, LLM logs, MCP logs, connectors.
2. `section.browser-ai`: Browser AI Guard overview, rules, prompt logs.
3. `section.models`: Providers, catalog, budgets, routing rules, circuit breakers.
4. `section.mcp-gateway`: MCP catalog, library, tool groups, OAuth grants.
5. `section.plugins`: Plugin list and plugin execution order.
6. `section.governance`: Virtual keys, users, teams, customers, roles.
7. `section.guardrails`: Guardrail rules, rule providers, and cluster configs.
8. `section.adaptive-routing`: Adaptive routing dashboard and settings.
9. `section.prompt-repository`: Saved prompts and templates.
10. `section.skills-repository`: AI skill files stored for the workspace.
11. `section.cluster`: Cluster nodes, load balancer routing, high availability.
12. `section.scim`: SCIM directory integrations and Active Directory sync.
13. `section.audit-logs`: Governance audit trail for administrative actions.
14. `section.access-profiles`: Multi-entity permission policies.
15. `section.business-units`: Organizational business unit and team hierarchies.

#### 2. Platform Behavior Flags (`<domain>.<feature>`)
These flags modify runtime gateway processing on the next inference request:
* `cache.responses`: Enables/disables response caching (direct and semantic) across the entire gateway.
* `browser.prompt_guard`: Evaluates prompts from Browser Guard agents.
* `browser.file_scan`: Scans file and voice uploads from Guard agents for malware/sensitive data.

### 8.2 Adding a Custom Feature Flag
* **ID Naming Rule**: Must match `FLAG_ID_REGEX`: lowercase letters, numbers, and `._-` separators (e.g. `routing.circuit_breaker`, `custom.experimental_stream`).
* **API Payload**:
  ```bash
  curl --location 'http://localhost:3100/api/feature-flags' \
    --header 'Content-Type: application/json' \
    --cookie cookies.txt \
    --data '{
      "id": "custom.streaming_parser_v2",
      "display_name": "Streaming Parser V2",
      "description": "High-throughput token streaming parser.",
      "default": false,
      "enabled": true
    }'
  ```

### 8.3 Toggling / Editing a Flag
```bash
curl --location --request PUT 'http://localhost:3100/api/feature-flags/section.observability' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "enabled": false
  }'
```

### 8.4 Deleting a Flag
```bash
curl --location --request DELETE 'http://localhost:3100/api/feature-flags/custom.streaming_parser_v2' \
  --cookie cookies.txt
```

### 8.5 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `Invalid flag ID: must contain lowercase letters, numbers, and .-_ separators (e.g. custom.my_feature)` | Flag ID contains uppercase letters, spaces, or illegal special characters (`@`, `!`, `#`). | Use only lowercase alphanumeric characters, dots, underscores, and hyphens (e.g., `my_feature.v1`). |
| `Feature flag is already registered` | Attempted to create a flag with an ID that already exists. | Use a unique flag ID or update the existing flag via `PUT /api/feature-flags/{id}`. |
| `Cannot delete feature flag locked by config.json / Helm` | Attempted to delete a flag locked at boot via environment or Helm chart values. | Remove the flag lock from `config.json` or Helm configuration first. |
| `403 Forbidden: This section is currently disabled in Feature Flags` | Client attempted to access an API whose section flag is toggled off. | Go to `/workspace/config/feature-flags` and toggle the corresponding section flag back **ON**. |

---

## 9. Section 8: Global Outbound Proxy

**Route:** `/workspace/config/proxy`  
**API Endpoints:** `GET /api/proxy-config`, `PUT /api/proxy-config`

### 9.1 Purpose & Usage
In enterprise environments, all outbound traffic to external LLM providers (OpenAI, Anthropic, AWS Bedrock, Google Vertex AI) must route through corporate forward proxies for inspection and compliance.

### 9.2 Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | `boolean` | `false` | Enables global forward proxying for outbound HTTP requests. |
| `type` | `string` | `"http"` | Proxy protocol. Currently, **`http`** is supported. `socks5` and `tcp` are reserved for future releases. |
| `url` | `string` | `""` | Corporate proxy URL (e.g. `http://proxy.corp.internal:8080` or `http://squid.company.lan:3128`). |
| `username` | `string` | `""` | Proxy authentication username. |
| `password` | `string` | `""` | Proxy authentication password. Redacted as `<redacted>` on read. |
| `no_proxy` | `string` | `""` | Comma-separated bypass list of domains, IPs, or CIDR blocks (e.g. `localhost, 127.0.0.1, .internal, 10.0.0.0/8`). |
| `timeout` | `integer` | `30` | Egress connection timeout in seconds. |
| `skip_tls_verify` | `boolean` | `false` | When true, skips TLS certificate validation on the proxy handshake (useful for corporate TLS inspection certificates). |

> [!NOTE]
> Modifying proxy configuration sets `RestartRequiredConfig = true` in PostgreSQL. An alert banner will notify administrators: *"Proxy configuration has been updated. A restart is required for all changes to take full effect."*

### 9.3 Practical Example & Payload
```bash
curl --location --request PUT 'http://localhost:3100/api/proxy-config' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "enabled": true,
    "type": "http",
    "url": "http://proxy.corp.internal:8080",
    "username": "unifai_egress",
    "password": "CorporateProxySecret#2026",
    "no_proxy": "localhost, 127.0.0.1, .corp.local",
    "timeout": 60,
    "skip_tls_verify": false
  }'
```

### 9.4 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `proxy URL is required when proxy is enabled` | Enabled toggle was checked, but the `url` field was left empty. | Enter the complete proxy URL (e.g. `http://proxy.corp.internal:8080`). |
| `proxy type socks5 is not yet supported` | Selected `socks5` or `tcp`. | Select `http` as the proxy type. |
| `proxy timeout must be non-negative` | Negative number entered into timeout field. | Enter `0` (system default) or a positive integer in seconds. |

---

## 10. Section 9: Logs & Data Retention Settings

**Route:** `/workspace/config/logging`  
**API Endpoints:** `GET/PUT /api/config`, `POST /api/logs/cleanup`

### 10.1 Purpose & Usage
Logs settings control request payload recording, privacy redaction, and automatic database pruning for SOC-2 and HIPAA compliance.

### 10.2 Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enable_logging` | `boolean` | `true` | Master switch for recording inference requests and MCP execution logs. |
| `disable_content_logging` | `boolean` | `false` | When enabled, prompts and completion text are never written to disk/DB (zero-data-retention mode for strict privacy). Metadata and token metrics are still logged. |
| `allow_per_request_content_storage_override` | `boolean` | `false` | Allows client requests to override content logging on a per-request basis using the `x-uf-log-content: true/false` header. |
| `allow_per_request_raw_override` | `boolean` | `false` | Allows client requests to store raw HTTP request/response payloads using `x-uf-log-raw: true`. |
| `log_retention_days` | `integer` | `0` | Number of days to retain logs in PostgreSQL. `0` = indefinite retention. Any value $> 0$ automatically schedules periodic log pruning. |
| `hide_deleted_virtual_keys_in_filters` | `boolean` | `true` | Excludes soft-deleted Virtual Keys from dashboard log search dropdowns. |
| `logging_headers` | `string[]` | `[]` | Extra request/response HTTP headers captured and indexed into log records (e.g. `x-user-id`, `x-request-id`). |

### 10.3 Manual Log Cleanup Execution
When saving a `log_retention_days` value $> 0$, the gateway automatically invokes `triggerLogCleanup` to prune older records immediately:
```bash
curl --location --request POST 'http://localhost:3100/api/logs/cleanup' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{ "retention_days": 30 }'
```
Response:
```json
{
  "status": "success",
  "deleted_logs": 1420,
  "deleted_mcp_logs": 85
}
```

### 10.4 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `log_retention_days cannot be negative` | Negative number entered into retention days. | Enter `0` for indefinite retention or a positive integer (e.g. `30` or `90`). |
| `Content logs still appear even though disable_content_logging is true` | A client sent `x-uf-log-content: true` and `allow_per_request_content_storage_override` was enabled. | Disable `allow_per_request_content_storage_override` to enforce strict zero-content retention globally. |

---

## 11. Section 10: Troubleshooting & SOC-2 Audit Trail Matrix

### 11.1 Audit Trail Event Mapping
Every change made across any Settings module produces an immutable audit event in the `audit_logs` table (viewable at `/workspace/audit-logs`):

| Settings Subsystem | Target Endpoint | Audit Action | Audit Target Name Recorded |
| :--- | :--- | :--- | :--- |
| **Client Settings** | `PUT /api/config` | `UPDATE` | `Configuration` |
| **Compatibility** | `PUT /api/config` | `UPDATE` | `Configuration` |
| **Vector Store** | `PUT /api/vector-store-config` | `UPDATE` | `Vector Store Configuration` |
| **Semantic Cache Plugin** | `POST/PUT /api/plugins` | `UPDATE` | `Plugin: semantic_cache` |
| **Security Settings** | `PUT /api/config` | `UPDATE` | `Configuration` |
| **SMTP Settings** | `PUT /api/smtp-config` | `UPDATE` | `SMTP Configuration` |
| **SMTP Test Email** | `POST /api/smtp-config/test`| `EXECUTE` | `SMTP Test Email` |
| **Performance Tuning** | `PUT /api/config` | `UPDATE` | `Configuration` |
| **Feature Flag (Create)**| `POST /api/feature-flags` | `CREATE` | `Feature Flag: <flag_id>` |
| **Feature Flag (Update)**| `PUT /api/feature-flags/{id}`| `UPDATE` | `Feature Flag: <flag_id>` |
| **Feature Flag (Delete)**| `DELETE /api/feature-flags/{id}`| `DELETE` | `Feature Flag: <flag_id>` |
| **Proxy Settings** | `PUT /api/proxy-config` | `UPDATE` | `Proxy Configuration` |
| **Log Pruning** | `POST /api/logs/cleanup` | `DELETE` | `Log Store Prune` |

### 11.2 Master Diagnostic & Triage Workflow
If any setting fails to apply or returns an unexpected error:
1. **Check Audit Logs**: Open `/workspace/audit-logs` to confirm whether the mutation reached the database, who initiated it, and the response code.
2. **Inspect Browser Console & Network**: Inspect the `PUT /api/config` or `PUT /api/vector-store-config` response payload for specific field validation errors.
3. **Verify Feature Flags**: Ensure dependent feature flags (e.g. `cache.responses` for caching or `section.<name>` for menus) are enabled.
4. **Inspect Container Logs**:
   ```bash
   docker logs unifai-gateway --tail 100 -f | grep -E "ERROR|WARN"
   ```
5. **Verify Database Connectivity**: If changes revert after gateway restart, verify that PostgreSQL write permissions and migrations are up to date.

---

## 12. Section 11: Environment Variables (`.env`) & Inspection Commands

The Gateway reads initial boot credentials and hardware binds from `.env` at startup.

### 12.1 Master Environment Variables Reference

| Category | Variable Name | Purpose & Default |
| :--- | :--- | :--- |
| **Server** | `SERVER_DOMAIN` | Public URL (e.g. `https://unifai.yespanchi.com`). |
| **Server** | `APP_PORT` | HTTP Gateway listening port (default: `6000`). |
| **Server** | `UI_PORT` | Dashboard frontend port (default: `3100`). |
| **Database** | `DB_TYPE` | Engine: `postgres`, `sqlite`, or `mysql`. |
| **Database** | `DB_HOST` | Database hostname or IP (e.g. `76.13.243.253` or `127.0.0.1`). |
| **Database** | `DB_PORT` | Database port (`5432` or `32768`). |
| **Database** | `DB_NAME` | Database schema name (`Unifai_test`). |
| **Database** | `DB_USER` | Database username (`Unifai_test`). |
| **Database** | `DB_PASSWORD` | Database password. |
| **Database** | `DB_SSL_MODE` | `disable`, `require`, or `verify-full`. |
| **Admin** | `ADMIN_EMAIL` | Super Admin initial username / email. |
| **Admin** | `ADMIN_PASSWORD` | Super Admin initial password. |
| **Security** | `GATEWAY_ENCRYPTION_KEY` | AES-256 GCM key for credentials at rest. |
| **Security** | `PASSWORD_RESET_SECRET` | HMAC secret for password-reset tokens. |
| **Cluster** | `CLUSTER_REPLICATE_SECRET` | Cryptographic secret for multi-node KV state replication. |

---

### 12.2 How to Get / Read `.env` Values from CLI

#### Option A: Using the Built-in PowerShell Helper Script (Recommended)
We provide a dedicated PowerShell script [scripts/get_env.ps1](file:///d:/unifai_project/scripts/get_env.ps1) that safely displays all configuration while automatically masking sensitive secrets:

```powershell
# 1. View all active .env variables (passwords automatically masked):
powershell -ExecutionPolicy Bypass -File .\scripts\get_env.ps1

# 2. Query a specific variable value:
powershell -ExecutionPolicy Bypass -File .\scripts\get_env.ps1 DB_HOST
powershell -ExecutionPolicy Bypass -File .\scripts\get_env.ps1 CLUSTER_REPLICATE_SECRET
```

#### Option B: Direct PowerShell One-Liners

```powershell
# View all non-secret environment variables:
Get-Content .env | Where-Object { $_ -match "^[A-Z]" -and $_ -notmatch "PASSWORD|SECRET|KEY" }

# Extract a specific value (e.g. DB_HOST):
(Get-Content .env | Select-String "^DB_HOST=").Line.Split("=")[1].Trim()

# Test network connectivity to the DB_HOST & DB_PORT defined in .env:
$host = (Get-Content .env | Select-String "^DB_HOST=").Line.Split("=")[1].Trim()
$port = (Get-Content .env | Select-String "^DB_PORT=").Line.Split("=")[1].Trim()
Test-NetConnection -ComputerName $host -Port $port
```

#### Option C: Query Active Runtime Config via Gateway API

```bash
# 1. Inspect live core config:
curl --location 'http://localhost:6000/api/config' --cookie cookies.txt

# 2. Inspect live cluster config:
curl --location 'http://localhost:6000/api/cluster' --cookie cookies.txt

# 3. Inspect live SMTP email config:
curl --location 'http://localhost:6000/api/smtp-config' --cookie cookies.txt
```

