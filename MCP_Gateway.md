# UnifAI MCP Gateway Architecture & Operations — Complete Reference Guide

This document is the **definitive, production-grade guide** for all sub-modules under the **MCP Gateway** section in the UnifAI Gateway:
1. **MCP Catalog** (`/workspace/mcp-registry`)
2. **MCP Library** (`/workspace/mcp-registry/library`)
3. **Tool Groups** (`/workspace/mcp-tool-groups`)
4. **Auth Sessions** (`/workspace/mcp-sessions`)
5. **OAuth Grants** (`/workspace/oauth-grants`)
6. **MCP Settings** (`/workspace/mcp-settings`)

Every section covers:
- **System Flow & Transport Architecture**
- **Complete Parameters & Data Types**
- **Step-by-Step UI & API Operations**
- **Production cURL Commands & JSON Payloads**
- **Exhaustive Error Handling & Resolution Matrix**
- **Audit & Governance Compliance Tracking**

---

## Table of Contents

1. [MCP Gateway Architecture & Execution Flow](#1-mcp-gateway-architecture--execution-flow)
2. [Section 1: MCP Catalog (`/workspace/mcp-registry`)](#2-section-1-mcp-catalog)
3. [Section 2: MCP Library (`/workspace/mcp-registry/library`)](#3-section-2-mcp-library)
4. [Section 3: Tool Groups (`/workspace/mcp-tool-groups`)](#4-section-3-tool-groups)
5. [Section 4: Auth Sessions (`/workspace/mcp-sessions`)](#5-section-4-auth-sessions)
6. [Section 5: OAuth Grants (`/workspace/oauth-grants`)](#6-section-5-oauth-grants)
7. [Section 6: MCP Settings (`/workspace/mcp-settings`)](#7-section-6-mcp-settings)
8. [Database Schema: MCP Gateway Persistence](#8-database-schema-mcp-gateway-persistence)
9. [SOC-2 Compliance & Audit Logs Reference](#9-soc-2-compliance--audit-logs-reference)

---

## 1. MCP Gateway Architecture & Execution Flow

The **Model Context Protocol (MCP)** standardizes how Large Language Models and autonomous AI agents discover and execute external tools, query enterprise databases, interact with local filesystems, and communicate with SaaS APIs.

The UnifAI MCP Gateway functions as an **enterprise reverse proxy, policy firewall, and credential injector** between calling LLMs and external MCP tool servers.

```mermaid
flowchart TD
    LLM[LLM / Autonomous Agent] -->|Tool Invocation Request| GatewayProxy[UnifAI MCP Gateway Proxy]
    GatewayProxy --> VKAuth{Virtual Key Policy Check}
    VKAuth -->|Tool Not in Whitelist| Block403[403 Forbidden: Tool Execution Disallowed]
    VKAuth -->|Authorized| AuthInjector{Inject Auth Context}
    AuthInjector -->|Headers / API Key| InjectHeader[Inject Authorization: Bearer / ApiKey]
    AuthInjector -->|OAuth PKCE Session| InjectOAuth[Inject Active OAuth2 Access Token]
    AuthInjector -->|No Auth| DirectForward[Direct Forwarding]
    InjectHeader --> SelectTransport{Select Transport}
    InjectOAuth --> SelectTransport
    DirectForward --> SelectTransport
    SelectTransport -->|SSE Transport: http/https| RemoteServer[Remote SSE MCP Server Endpoint]
    SelectTransport -->|Stdio Transport: CLI Binary| LocalProcess[Spawn Local Child Process Pipe]
    RemoteServer --> ToolExecution[Execute Tool Logic]
    LocalProcess --> ToolExecution
    ToolExecution --> Sanitize[Sanitize Output & Check Truncation Buffer]
    Sanitize --> WriteAudit[Record Execution to 'mcp_logs']
    WriteAudit --> ReturnResponse[Return Tool Response Payload to Model]
```

### Key Security & Governance Guardrails:
* **Tool Whitelisting**: Restrict Virtual Keys to explicit tool lists (`["search_docs", "get_weather"]`) or wildcard access (`["*"]`).
* **Autonomous Execution Gate**: Differentiate between read-only tools approved for autonomous execution (`tools_to_auto_execute`) vs. mutating tools requiring interactive human confirmation.
* **Per-User Credential Isolation**: Support downstream OAuth consent flows where individual end-users authorize their personal GitHub/Google accounts rather than sharing a single global key.

---

## 2. Section 1: MCP Catalog

**Route:** `/workspace/mcp-registry`  
**API Endpoints:** `GET /api/mcp/clients`, `POST /api/mcp/client`, `GET /api/mcp/clients/{id}`, `PUT /api/mcp/clients/{id}`, `DELETE /api/mcp/clients/{id}`, `GET /api/mcp/clients/{id}/tools`

### 2.1 Purpose & Capabilities
The **MCP Catalog** displays all active MCP server connections registered in the gateway. It provides real-time health pings, protocol negotiation, and live introspection of exposed tool schemas (function names, parameter types, descriptions).

### 2.2 Connection Transports
UnifAI supports two fundamental MCP transport protocols:
1. **Remote SSE (Server-Sent Events)**:
   * Communicates over standard HTTP/HTTPS.
   * Best for cloud-hosted services, microservices, and enterprise APIs (e.g. `https://mcp.github.com/v1`).
2. **Local Stdio (Standard Input/Output)**:
   * The gateway spawns a localized child process on the host operating system.
   * Input is piped via `stdin`, output is streamed back via `stdout`.
   * Best for CLI utilities, local filesystem access, and air-gapped utilities (e.g. `npx -y @modelcontextprotocol/server-postgres`).

### 2.3 Configuration Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :---: | :--- |
| `name` | `string` | **Yes** | Unique identifier (e.g. `github-tools`, `prod-postgres-mcp`). |
| `description` | `string` | No | Description of the server's purpose and tool capabilities. |
| `transport` | `string` | **Yes** | Protocol type: `sse` or `stdio`. |
| `url` | `string` | Conditional | Full HTTP/HTTPS endpoint URL (mandatory if `transport: "sse"`). |
| `command` | `string` | Conditional | Executable binary or CLI command (e.g. `node`, `python`, `npx`) (mandatory if `transport: "stdio"`). |
| `args` | `string[]` | No | Command-line arguments passed to the stdio child process. |
| `env` | `object` | No | Environment variables injected into the process (e.g. `{"DATABASE_URL": "postgresql://..."}`). |
| `headers` | `object` | No | HTTP request headers forwarded upstream (e.g. `{"Authorization": "Bearer sk-..."}`). |
| `allow_on_all_virtual_keys` | `boolean` | No | When true, automatically grants tool access to all Virtual Keys across the workspace (default: `false`). |
| `is_active` | `boolean` | No | Administrative toggle to enable or disable the server connection. |

### 2.4 Step-by-Step Operations Guide

#### Registering a Remote SSE Server
1. Navigate to `/workspace/mcp-registry`.
2. Click **Register Server**.
3. Choose **SSE (Remote HTTP)** transport.
4. Enter **Server Name** (`slack-connector`).
5. Enter **Endpoint URL** (`https://slack-mcp.company.internal/sse`).
6. Under **Headers**, add `Authorization: Bearer xoxb-your-token`.
7. Click **Test & Register**.
8. UnifAI will immediately ping the endpoint, pull the tool manifest, and mark the server **Active**.

#### Registering a Local Stdio Server
1. Under **Register Server**, select **Stdio (Local Process)**.
2. Enter **Server Name** (`local-postgres`).
3. Set **Command** to `npx`.
4. Add **Arguments**: `-y`, `@modelcontextprotocol/server-postgres`, `postgresql://user:pass@localhost:5432/mydb`.
5. Click **Register Server**.

### 2.5 Practical Example: Registering via cURL

```bash
curl --location --request POST 'http://localhost:3100/api/mcp/client' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "name": "enterprise-filesystem",
    "description": "Secure file access for internal docs",
    "transport": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "D:/unifai_project/docs"],
    "allow_on_all_virtual_keys": false
  }'
```

### 2.6 Error Handling & Troubleshooting

| Error Message | Exact Cause | How to Fix |
| :--- | :--- | :--- |
| `MCP server name already exists` | A server with the same name is already registered. | Use a distinct server name or update the existing connection. |
| `SSE connection failed: dial tcp ...: connection refused` | The remote SSE server endpoint is offline or blocked by network firewalls. | Verify the host and port are listening and reachable from the gateway machine. |
| `Failed to spawn stdio process: executable file not found` | The specified command (`npx`, `python`, `node`) is not installed in the system `PATH`. | Install the runtime or specify the full path (e.g. `C:\Program Files\nodejs\npx.cmd`). |
| `Failed to parse tool schema: invalid JSON-RPC response` | The upstream server output invalid JSON-RPC during initialization handshake. | Inspect the MCP server logs to ensure standard protocol compatibility. |

---

## 3. Section 2: MCP Library

**Route:** `/workspace/mcp-registry/library`  
**API Endpoints:** `GET /api/mcp/library`, `GET /api/mcp/library/filterdata`, `POST /api/mcp/library/force-sync`, `POST /api/mcp/library`, `DELETE /api/mcp/library/{id}`

### 3.1 Certified Connector Catalog
The **MCP Library** is a curated repository of **290+ pre-certified connectors** covering enterprise databases, productivity apps, developer tools, and cloud platforms.

```mermaid
flowchart LR
    LibraryJSON[file:///app/configs/mcp-library.json] --> SyncWorker[Scheduled Library Sync Worker: 24h]
    SyncWorker --> MemoryCatalog[(In-Memory Certified Connectors: 290 Entries)]
    MemoryCatalog --> LibraryUI[MCP Library Directory View]
    LibraryUI -->|1-Click Install| RegisterModal[Auto-Populate Catalog Registration Modal]
    RegisterModal --> ActiveClient[Active Server in MCP Catalog]
```

### 3.2 Popular Certified Connectors

| Connector | Category | Transport | Auth Type | Description |
| :--- | :--- | :---: | :---: | :--- |
| **PostgreSQL** | Database | Stdio | Custom URI | Direct SQL querying, schema introspection, and read-only query planning. |
| **GitHub** | Developer | SSE / Stdio | Headers (PAT) | Search repos, inspect commits, read pull requests, manage issues. |
| **Slack** | Communication | SSE | OAuth | Read channels, summarize threads, post automated agent updates. |
| **AWS Marketplace** | Cloud | SSE | Headers | Query software products, pricing, and infrastructure solutions. |
| **Brave Search** | Web Search | SSE | Headers (API Key) | Real-time internet search and local news extraction. |
| **Puppeteer** | Web Scraping | Stdio | None | Headless browser execution, JavaScript rendering, screenshot capture. |
| **Memory** | Agent Memory | Stdio | None | Graph-based associative long-term memory persistence. |

### 3.3 Operations Guide
* **One-Click Install**: Browse or search for any connector (e.g. `Slack`), click **Install**, fill in your credentials, and activate immediately.
* **Force Library Sync**: Click **Sync Catalog** to pull the latest certified connector schemas from the upstream repository.
* **Publish Custom Connector**: Organizations can publish internal proprietary tools to the library using `POST /api/mcp/library`.

---

## 4. Section 3: Tool Groups

**Route:** `/workspace/mcp-tool-groups`  
**API Endpoints:** `GET /api/mcp/tool-groups`, `POST /api/mcp/tool-groups`, `PUT /api/mcp/tool-groups/{id}`, `DELETE /api/mcp/tool-groups/{id}`

### 4.1 Purpose & Team Delegation
When organizations deploy dozens of MCP servers hosting hundreds of tools, granting permissions tool-by-tool becomes unmanageable.

**Tool Groups** bundle tools from multiple distinct MCP servers into a single logical permission unit:
* **"DevOps Toolchain" Group**: Combines `github:read_pull_request`, `kubernetes:get_pods`, and `datadog:query_metrics`.
* **"Sales Intelligence" Group**: Combines `hubspot:get_contact`, `slack:post_message`, and `brave_search:web_search`.

```mermaid
flowchart TD
    subgraph MCPServers[Disparate MCP Servers]
        ServerA[GitHub MCP Server]
        ServerB[PostgreSQL MCP Server]
        ServerC[Slack MCP Server]
    end
    
    subgraph ToolGroupDef[Tool Group: 'Engineering Lead Suite']
        Tool1[github:list_pull_requests]
        Tool2[postgres:execute_readonly_query]
        Tool3[slack:post_incident_update]
    end
    
    ServerA --> Tool1
    ServerB --> Tool2
    ServerC --> Tool3
    
    ToolGroupDef --> VK1[Virtual Key: Backend Team]
    ToolGroupDef --> VK2[Virtual Key: Release Bot]
```

### 4.2 Configuration Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :---: | :--- |
| `name` | `string` | **Yes** | Unique name (e.g. `developer-essentials`). |
| `description` | `string` | No | Description of group scope and audience. |
| `tools` | `array` | **Yes** | List of selected tools with `client_id` and `tool_name`. |

### 4.3 Practical Example: Creating a Tool Group via cURL

```bash
curl --location --request POST 'http://localhost:3100/api/mcp/tool-groups' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "name": "Database Admin Tools",
    "description": "Read-only inspection tools for relational databases",
    "tools": [
      { "client_id": 1, "tool_name": "list_tables" },
      { "client_id": 1, "tool_name": "describe_schema" },
      { "client_id": 1, "tool_name": "execute_read_query" }
    ]
  }'
```

---

## 5. Section 4: Auth Sessions

**Route:** `/workspace/mcp-sessions`  
**API Endpoints:** `GET /api/mcp/sessions`, `DELETE /api/mcp/sessions/{id}`

### 5.1 Architecture & Session Lifecycle
Stateful MCP servers (such as interactive database transactions, SSH terminals, or authenticated user sessions) maintain dedicated **Auth Sessions** in the gateway.

### 5.2 Session Statuses
* **Active**: Session is live and accepting tool calls within the TTL window.
* **Expired**: TTL expired without renewal.
* **Revoked**: Administrator manually terminated the session.

### 5.3 Administrative Revocation
Administrators can inspect live sessions, filter by user identity (`identity`) or client (`mcp_client_id`), and revoke compromised sessions immediately.

---

## 6. Section 5: OAuth Grants

**Route:** `/workspace/oauth-grants`  
**API Endpoints:** `GET /api/oauth2/sessions`, `DELETE /api/oauth2/sessions/{id}`

### 6.1 Downstream OAuth 2.0 PKCE Architecture
For services like Google Drive, Microsoft 365, or Salesforce, AI models must act on behalf of specific human users rather than using a static service key.

The UnifAI Gateway implements standard **OAuth 2.0 Authorization Code Flow with PKCE**:
1. When a model attempts to invoke a protected tool, the gateway checks for an active OAuth token for that user.
2. If none exists, the user receives an authentication consent link.
3. Upon approval, the gateway exchanges the authorization code for an Access & Refresh Token.
4. Tokens are encrypted at rest with AES-256 (`GATEWAY_ENCRYPTION_KEY`).
5. A background worker automatically refreshes expired access tokens before invoking the tool.

### 6.2 Managing Grants
In `/workspace/oauth-grants`, administrators can view all active third-party grants, review requested scopes, and revoke grants on demand.

```bash
# Query active OAuth grants via cURL
curl --location 'http://localhost:3100/api/oauth2/sessions' \
  --cookie cookies.txt
```

---

## 7. Section 6: MCP Settings

**Route:** `/workspace/mcp-settings`  
**API Endpoints:** `GET /api/config?from_db=true`, `PUT /api/config`

### 7.1 Global Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `mcp_tool_execution_timeout` | `integer` | `30` | Maximum time in seconds permitted for an individual tool execution before timing out. |
| `mcp_agent_depth` | `integer` | `10` | Hard recursion ceiling preventing runaway infinite loops during multi-step tool reasoning chains. |
| `mcp_code_mode_binding_level` | `string` | `server` | Sandbox isolation mode for Starlark code execution. |
| `mcp_disable_auto_tool_inject` | `boolean` | `false` | When true, tools are not injected into model system prompts unless explicitly requested. |
| `mcp_enable_temp_token_auth` | `boolean` | `false` | Enables short-lived temporary tokens for browser-based tool execution. |
| `mcp_server_auth_mode` | `string` | `headers` | Global auth standard: `headers` or `bearer`. |

---

## 8. Database Schema: MCP Gateway Persistence

The MCP Gateway persists configuration and session states in the following relational tables (supported across SQLite and PostgreSQL):

```sql
-- 1. MCP Server Connection Registrations
CREATE TABLE IF NOT EXISTS mcp_clients (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    transport VARCHAR(50) NOT NULL, -- 'sse' or 'stdio'
    url TEXT,
    command TEXT,
    args TEXT,                     -- JSON Array of arguments
    env TEXT,                      -- JSON Object of environment variables
    headers TEXT,                  -- JSON Object of HTTP headers
    allow_on_all_virtual_keys BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 2. Certified Connector Library
CREATE TABLE IF NOT EXISTS mcp_library_entries (
    id SERIAL PRIMARY KEY,
    slug VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(100),
    connection_type VARCHAR(50),
    connection_url TEXT,
    auth_type VARCHAR(50),
    icon_url TEXT,
    docs_url TEXT,
    publisher VARCHAR(100),
    source VARCHAR(50) DEFAULT 'remote'
);

-- 3. Composite Tool Groups
CREATE TABLE IF NOT EXISTS mcp_tool_groups (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    tools TEXT,                    -- JSON Array of {client_id, tool_name}
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 4. Downstream OAuth2 User Refresh Tokens
CREATE TABLE IF NOT EXISTS oauth2_refresh_tokens (
    id VARCHAR(255) PRIMARY KEY,
    client_id VARCHAR(255) NOT NULL,
    uf_mode VARCHAR(50) NOT NULL,  -- 'user', 'vk', 'session'
    uf_sub VARCHAR(255) NOT NULL,   -- User ID or Virtual Key ID
    refresh_token TEXT NOT NULL,   -- AES-256 Encrypted
    scope TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used_at TIMESTAMP WITH TIME ZONE
);
```

---

## 9. SOC-2 Compliance & Audit Logs Reference

All actions in the MCP Gateway write tamper-evident records to `/workspace/audit-logs`:

| Action | Event Type | Target Entity | Audit Payload |
| :--- | :--- | :--- | :--- |
| Register MCP Client | `mcp_client.create` | Client Name | Transport type, URL/command, permissions. |
| Modify Client Credentials | `mcp_client.update` | Client Name | Encrypted header update, argument diff. |
| Delete MCP Client | `mcp_client.delete` | Client Name | Complete connection removal. |
| Execute Tool | `mcp_tool.execute` | Tool Name | Latency, input args, result status, caller identity. |
| Revoke OAuth Grant | `oauth_grant.revoke` | Grant ID | Client ID, user identity, revocation timestamp. |
| Create Tool Group | `tool_group.create` | Group Name | Tool count and member server IDs. |

*(End of MCP Gateway Reference Guide)*
