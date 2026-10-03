# UnifAI / Raksha Enterprise Administrator Guide
# Complete Admin Console Manual: Configuration, Governance & Fleet Management

**Document Version:** 2.4.0  
**Classification:** Enterprise Administration & Security Governance  
**Target Audience:** Platform Administrators, InfoSec Teams, Compliance Officers, Workspace Managers  

---

## 1. Initial Access & First-Run Security Checklist

Upon launching the UnifAI platform, follow this mandatory security onboarding sequence before inviting users or onboarding endpoints:

1. **Initial Root Sign-In:**
   - Navigate to your instance: `https://unifai.yourcompany.com/auth/login`.
   - Authenticate using the bootstrap credentials defined in your server environment (`ADMIN_EMAIL` and `ADMIN_PASSWORD`).
2. **Immediate Password Rotation:**
   - Click your profile avatar in the top-right corner $\rightarrow$ **Account Settings** $\rightarrow$ **Security**.
   - Change the password immediately to a strong, high-entropy passphrase (minimum 16 characters).
3. **Dedicated Admin Account Provisioning:**
   - Navigate to **Governance $\rightarrow$ Users**.
   - Create a named personal administrator account with the `admin` role.
   - Demote or securely store the bootstrap account for break-glass emergency use only.
4. **SMTP / Email Notification Setup:**
   - Navigate to **Settings $\rightarrow$ SMTP Configuration**.
   - Provide your corporate SMTP gateway credentials (host, port 587/465, TLS, user, password, from-address).
   - Send a test email to verify that registration verification codes, password resets, and high-severity compliance alerts function correctly.
5. **Branding & Legal Notice Customization:**
   - Under **Settings $\rightarrow$ General Settings**, update the Company Name, Portal Logo, and End-User Terms of Use notice shown upon login.

---

## 2. Governance, User Lifecycle & RBAC Administration

### 2.1 User Lifecycle Management
Located at **Governance $\rightarrow$ Users** (`/workspace/governance/users`):

| Operation | Steps in Admin UI | Underlying API Route |
|---|---|---|
| **Approve Pending User** | Select user row in `Pending` tab $\rightarrow$ Click **Approve**. Assign Role and initial Allowed Sections. | `POST /api/session/users/{id}/approve` |
| **Reject Sign-Up** | Select user row $\rightarrow$ Click **Reject**. Enter optional explanation reason. | `POST /api/session/users/{id}/reject` |
| **Direct User Creation** | Click **+ Create User**. Enter Email, Full Name, Initial Password, Role, Budget, and Rate Limit. | `POST /api/session/users` |
| **Role Assignment** | Open user profile $\rightarrow$ Select Role dropdown (`admin`, `sub_admin`, `user`, or Custom Role). | `PUT /api/users/{id}/role` |
| **Section-Scoped Access** | Under **Allowed Sections**, toggle specific sidebar items (e.g. Prompt Repository, Logs, Browser AI). | `PUT /api/session/users/{id}` |
| **Account Deactivation** | Click user actions menu $\rightarrow$ **Disable Account**. Immediately revokes all active sessions. | `DELETE /api/session/users/{id}` |

### 2.2 Roles & Permissions Engine (RBAC)
Located at **Governance $\rightarrow$ Roles & Permissions** (`/workspace/governance/roles`):

UnifAI features a granular enterprise authorization matrix based on:
- **33 System Resources:** `GuardrailsConfig, GuardrailsProviders, GuardrailRules, UserProvisioning, Cluster, Settings, Users, Logs, Observability, Dashboard, VirtualKeys, ModelProvider, Plugins, MCPGateway, MCPToolGroups, MCPLogs, AdaptiveRouter, AuditLogs, Customers, Teams, RBAC, Governance, RoutingRules, PromptRepository, PromptDeploymentStrategy, SkillsRepository, AccessProfiles, APIKeys, Inference, Metrics, FeatureFlags, CircuitBreaker`.
- **6 Discrete Operations:** `Create`, `Read`, `Update`, `Delete`, `View`, `Download`.
- **Data Access Control (DAC):**
  - `all-data`: Role grants visibility across the entire enterprise tenant.
  - `own-data`: Restricts user visibility strictly to records, keys, or logs authored by the authenticated user.

#### Creating a Custom Role:
1. Click **+ Create Custom Role**.
2. Name the role (e.g., `ComplianceAuditor`, `PromptEngineerLead`).
3. Set the Data Access Control scope (`all-data` or `own-data`).
4. Select the specific resource checkboxes and assign allowable operations.
5. Click **Save Role**. Custom roles immediately appear in user assignment menus.

### 2.3 Access Profiles (Policy Templates)
Located at **Governance $\rightarrow$ Access Profiles** (`/workspace/governance/access-profiles`):
Access Profiles allow you to bundle permissions, model allowances, rate limits, and allowed sections into reusable, versioned policy packages.
- **Creating a Profile:** Click **+ New Access Profile**, enter Name (e.g., `Contractor-Limited-Profile`), Description, and assign specific AI models allowed for inference.
- **Binding to Users:** When onboarding dozens of employees, simply select the Access Profile to automatically synchronize limits and permissions.

---

## 3. Model Providers & Model Catalog

Located at **Models $\rightarrow$ Model Providers** (`/workspace/models/providers`) and **Model Catalog**:

### 3.1 Connecting Commercial & Private AI Providers
UnifAI supports over 28 providers out of the box:

```
                               ┌────────────────────────────────────────┐
                               │           UnifAI Model Catalog         │
                               └───────────────────┬────────────────────┘
                                                   │
         ┌───────────────────┬─────────────────────┼─────────────────────┬───────────────────┐
         ▼                   ▼                     ▼                     ▼                   ▼
    ┌─────────┐        ┌───────────┐         ┌───────────┐         ┌───────────┐       ┌───────────┐
    │ OpenAI  │        │ Anthropic │         │  Gemini   │         │  Bedrock  │       │  Ollama   │
    │ gpt-4o  │        │ claude-3.5│         │ 1.5-pro   │         │  claude-3 │       │ llama3.3  │
    │ o1, o3  │        │ sonnet    │         │ 2.0-flash │         │  titan    │       │ deepseek  │
    └─────────┘        └───────────┘         └───────────┘         └───────────┘       └───────────┘
```

#### Step-by-Step Provider Connection:
1. Navigate to **Models $\rightarrow$ Model Providers**.
2. Click **+ Add Provider** and choose the vendor (e.g., OpenAI, Anthropic, Google Gemini, Azure OpenAI, Ollama).
3. Fill in the connection parameters:
   - **Provider Name:** Unique identifier (e.g., `openai-production-tier1`).
   - **API Key:** Third-party vendor API secret (stored with AES-256-GCM encryption).
   - **Base URL (Optional):** Custom proxy URL or private Azure/Ollama endpoint (e.g. `http://ollama-server:11434`).
   - **Weight / Priority:** Integer weight used for automatic weighted load balancing.
4. Click **Test Connection**. The gateway will execute a lightweight health probe to verify credentials.
5. Click **Save Provider**. Available models will automatically synchronize into the **Model Catalog**.

### 3.2 Fallback Cascades & Adaptive Routing
Configure fallback chains to guarantee 99.999% availability:
- If `gpt-4o` returns an upstream HTTP 500/503 or hits a rate limit, the gateway automatically cascades the request to `claude-3-5-sonnet` or an internal Ollama model with zero client code changes.

---

## 4. Virtual Keys & Spend Governance

Located at **Virtual Keys** (`/workspace/virtual-keys`):

Virtual Keys are the gateway's primary credential mechanism for client applications, developers, and internal systems. They completely shield upstream provider API keys from exposure.

### 4.1 Creating a Virtual Key
1. Click **+ Create Virtual Key**.
2. Provide key parameters:
   - **Key Name:** Identifier (e.g., `customer-support-agent-prod`).
   - **Workspace / Team:** Assign to a department or project.
   - **Allowed Models:** Restrict the key to specific models (e.g., only `gpt-4o-mini` and `claude-3-haiku`).
   - **Budget Limit ($):** Monthly or total hard dollar spend cap (e.g., `$500.00`).
   - **Rate Limits:**
     - Requests Per Minute (RPM) cap (e.g., `120 RPM`).
     - Tokens Per Minute (TPM) cap (e.g., `100,000 TPM`).
3. Click **Generate Key**.
4. **Copy the generated key immediately.** The secret (`vk-live-...`) is displayed only once.

---

## 5. Browser AI / Guard Fleet Management

Located at **Browser AI** (`/workspace/browser-ai/*`):

The Browser AI suite allows administrators to manage, monitor, and enforce corporate security policies across all employee web interactions with public AI platforms.

```
       Browser AI Admin Menu
       ├── Target Websites      → Configure which AI sites are inspected
       ├── Guard Rules          → Define DLP, regex, keywords, and AI Guard Bot rules
       ├── Prompt Logs          → Full audit trail of prompts, blocks, and redactions
       ├── Guard Agents         → Fleet health, OS versions, and agent status
       ├── Security Insights    → Visual analytics on top violations and risky prompts
       └── Setup & Download     → Build, package, and distribute Windows/macOS installers
```

### 5.1 Configuring Target Websites
Navigate to **Browser AI $\rightarrow$ Target Websites**:
1. Click **+ Add Target Website**.
2. Enter the domain or regex pattern:
   - Example 1: `chatgpt.com` (Protocol: HTTPS, Target Name: "OpenAI ChatGPT")
   - Example 2: `claude.ai` (Target Name: "Anthropic Claude")
   - Example 3: `perplexity.ai` (Target Name: "Perplexity AI")
3. Toggle **Enabled** to `Active`.
4. When saved, all connected endpoint guards immediately receive the updated PAC and proxy routing list on their next heartbeat.

### 5.2 Creating Guard Rules (DLP & Guard Bot)
Navigate to **Browser AI $\rightarrow$ Guard Rules**:
1. Click **+ Add Rule**.
2. Define Rule Criteria:
   - **Rule Name:** e.g., `Block Credit Card Numbers (PCI-DSS)`.
   - **Target Scope:** Select All Websites or specific target platforms.
   - **Matching Engine:**
     - `Regex`: e.g. `\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13})\b`
     - `Keywords`: Comma-separated list (e.g. `CONFIDENTIAL_PROJECT_APOLLO, merger_terms`).
     - `AI Guard Bot`: Semantic assessment prompt evaluated by local Ollama LLM.
   - **Action:**
     - `BLOCK`: Drop the prompt and display an in-browser compliance violation notification.
     - `REDACT`: Mask the sensitive substring with `[REDACTED_PCI_DATA]` and let the prompt proceed.
     - `WARN`: Allow the prompt but flag an alert in security logs and trigger an admin webhook.
3. Set **Priority** (1 – 100, where 1 is highest priority).
4. Click **Save Rule**.

### 5.3 Guard Agent Fleet Monitoring
Navigate to **Browser AI $\rightarrow$ Guard Agents**:
- View the active fleet of connected laptops:
  - Agent ID, Employee Username, Hostname, Operating System (Windows 11, macOS Sequoia).
  - Guard Version (e.g. `v1.1.15`).
  - Heartbeat status: `Online` (green indicator, active within 60s) or `Offline`.
  - Last synced policy revision.

### 5.4 Rebuilding & Packaging Guard Installers
Navigate to **Browser AI $\rightarrow$ Setup**:
1. View the current distribution packages for **Windows (EXE)** and **macOS (PKG/ZIP)**.
2. If you updated the server domain or fleet secret, click **Rebuild & Publish Packages**.
3. Download the standalone installers directly or copy the one-line deployment script to distribute via MDM (Microsoft Intune, Jamf Pro, Kandji).
4. **Company Uninstall Key:** Set the mandatory master secret required if an employee or technician attempts to uninstall the guard client.

---

## 6. Workspace Configuration Views

UnifAI provides seven dedicated configuration views located under **Workspace $\rightarrow$ Config** (`/workspace/config/*`):

### 6.1 Client Settings (`/workspace/config/client-settings`)
- Configures default timeouts, client retry counts, backoff multipliers, and streaming chunk buffer sizes for all outbound gateway connections.

### 6.2 Compatibility (`/workspace/config/compatibility`)
- Manages legacy OpenAI API version compatibility layers, Anthropic message formatting adapters, and custom vendor header rewriting rules.

### 6.3 Caching (`/workspace/config/caching`)
- Configures the In-Memory, Redis, or Vector-based Semantic Cache.
- Set cache TTL (Time to Live in seconds), maximum memory allocation, and similarity thresholds (e.g. `0.92`) for semantic hit matching.

### 6.4 Security (`/workspace/config/security`)
- Enforces session expiration timers (default: 24 hours), MFA policies, brute-force lockout thresholds (maximum failed attempts and lockout window in minutes), and trusted proxy CIDRs.

### 6.5 API Keys (`/workspace/config/api-keys`)
- Enterprise gate for managing root master API access, key rotation schedules, and third-party credential vaults.

### 6.6 Performance Tuning (`/workspace/config/performance-tuning`)
- Fine-tunes fasthttp worker pool size, maximum concurrent connections, TCP keep-alive intervals, and read/write buffer allocations.

### 6.7 Feature Flags (`/workspace/config/feature-flags`)
- Toggles runtime system capabilities dynamically without restarting containers:
  - `enable_mcp_gateway`: Activates Model Context Protocol proxying.
  - `enable_semantic_cache`: Activates vector-based caching engine.
  - `enable_ai_guard_bot`: Enables Ollama-powered semantic guardrails.
  - `enforce_strict_dlp`: Switches guard fleet to hard fail-closed enforcement.

---

## 7. Audit Logging & Security Forensics

Located at **Observability $\rightarrow$ Audit Logs** (`/workspace/observability/audit-logs`) and **Browser AI $\rightarrow$ Prompt Logs**:
- Every mutating administrative action (key creation, user approval, rule change, provider update) is permanently recorded with timestamp, actor email, source IP address, and JSON diff of changes.
- Prompt logs record every intercepted employee prompt, match scores, triggered DLP rules, and redaction diffs for compliance auditing (HIPAA, SOC 2, ISO 27001, GDPR).

---
*End of Administrator Guide.*
