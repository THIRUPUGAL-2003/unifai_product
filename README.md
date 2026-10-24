# YP-Raksha Enterprise AI Gateway & Governance Platform

Welcome to the **YP-Raksha Platform Documentation Suite**. This repository contains the source code, backend services, and enterprise documentation for the YP-Raksha AI Gateway, Observability, and Governance infrastructure.

---

## Complete Section Documentation Sitemap

Every module in the YP-Raksha platform has a dedicated, production-grade architectural and operational guide:

| Section | Documentation File | Description & Capabilities |
| :--- | :--- | :--- |
| **Authentication & Access** | [Login.md](file:///d:/unifai_project/md/Login.md) | Authentication flows, session cookies, MFA, lockout thresholds, temporary passwords, and login security. |
| **Governance & Organizations** | [Governance.md](file:///d:/unifai_project/md/Governance.md) | Virtual Keys, Users, Teams, Business Units, Customers, SCIM provisioning, RBAC roles, and Audit Logs. |
| **Prompt Repository** | [Prompt_Repository.md](file:///d:/unifai_project/md/Prompt_Repository.md) | Prompt templates, folder trees, versioning, playground sessions, model params, and bidirectional Governance sync. |
| **Skills Repository** | [Skills_Repository.md](file:///d:/unifai_project/md/Skills_Repository.md) | Agent skills catalog, `SKILL.md` bundles, versioning, and top-priority LLM system message injection. |
| **Observability & Analytics** | [Observability.md](file:///d:/unifai_project/md/Observability.md) | Real-time analytics dashboard, LLM request/response tracing, MCP logs, APM connectors, and retention settings. |
| **Models & Routing** | [Models.md](file:///d:/unifai_project/md/Models.md) | Multi-provider catalog, rate limits, CEL routing rules, Circuit Breakers, complexity routing, and 6-tier pricing overrides. |
| **MCP Gateway** | [MCP_Gateway.md](file:///d:/unifai_project/md/MCP_Gateway.md) | Model Context Protocol servers (SSE & Stdio), tool whitelists, wildcard execution (`*`), OAuth grants, and tool groups. |
| **Guardrails & Clustering** | [Guardrails.md](file:///d:/unifai_project/md/Guardrails.md) | Real-time input/output DLP guardrails, PII masking, moderation providers, and cross-node cluster state replication. |
| **Adaptive Routing** | [Adaptive_Routing.md](file:///d:/unifai_project/md/Adaptive_Routing.md) | Dynamic multi-provider load balancing, weighted key rotation, fallback pruning, and automatic failure rerouting. |
| **Browser AI** | [Browser_AI.md](file:///d:/unifai_project/md/Browser_AI.md) | Client-side extension DLP for external web AI (ChatGPT, Claude, Gemini), target site rules, and prompt audit logs. |
| **Cluster & High Availability** | [Cluster_Config.md](file:///d:/unifai_project/md/Cluster_Config.md) | Multi-node HA cluster setup, peer mesh/broker topologies, heartbeat health, and distributed sync. |
| **Governance Architecture & Keys** | [Governance_Architecture.md](file:///d:/unifai_project/md/Governance_Architecture.md) | Virtual Key universal assignment, multi-tier budget hierarchy, cross-customer isolation, and DB sync. |
| **Plugin Subsystem** | [Plugins.md](file:///d:/unifai_project/md/Plugins.md) | Extensible hook lifecycle (`PreRequestHook`, `PreLLMHook`, `PostLLMHook`), native built-ins, and custom `.so` plugins. |
| **Platform Settings** | [Settings.md](file:///d:/unifai_project/md/Settings.md) | Client CORS, upstream API compatibility shims, multi-tier caching (Redis/Semantic), forward proxy, and performance tuning. |

---

## High-Level Platform Architecture

```mermaid
flowchart TD
    subgraph Ingestion["1. Ingestion & Identity"]
        Client[Client SDK / REST / Browser Extension] --> Auth[Login & Session Layer (Login.md)]
        Auth --> Gov[Governance & Virtual Keys (Governance.md)]
    end

    subgraph Inspection["2. Inspection & Guardrails"]
        Gov --> Guard[Input Guardrails & DLP (Guardrails.md)]
        Guard --> Prompts[Prompt & Skill Injection (Prompt_Repository.md, Skills_Repository.md)]
    end

    subgraph Routing["3. Intelligent Routing"]
        Prompts --> RouteEngine[CEL Rules & Adaptive Load Balancer (Models.md, Adaptive_Routing.md)]
        RouteEngine --> Circuit[Circuit Breaker Health Check (Models.md)]
    end

    subgraph Execution["4. Upstream Execution & Tools"]
        Circuit --> Providers[Upstream LLMs: OpenAI / Anthropic / Bedrock / Vertex]
        Circuit --> MCP[MCP Tool Gateway (MCP_Gateway.md)]
    end

    subgraph Observability["5. Observability & Auditing"]
        Providers --> OutGuard[Output Guardrails & Redaction (Guardrails.md)]
        MCP --> OutGuard
        OutGuard --> Tracing[LogStore & Metrics Exporters (Observability.md)]
        Tracing --> Response[Unified Client Response]
    end
```

---

## Banking deployment pack

- [YP-Raksha Technical Documentation — Word](docs/bank-deployment-pack/01-yp-raksha-technical-documentation.docx) · [PDF](docs/bank-deployment-pack/01-yp-raksha-technical-documentation.pdf) · [Markdown](docs/bank-deployment-pack/01-technical-documentation.md)
- [YP-Raksha Product Datasheet — Word](docs/bank-deployment-pack/02-yp-raksha-product-datasheet.docx) · [PDF](docs/bank-deployment-pack/02-yp-raksha-product-datasheet.pdf) · [Markdown](docs/bank-deployment-pack/02-product-datasheet.md)
- [On-Premises Deployment Architecture — Word](docs/bank-deployment-pack/03-on-premises-deployment-architecture.docx) · [PDF](docs/bank-deployment-pack/03-on-premises-deployment-architecture.pdf) · [Markdown](docs/bank-deployment-pack/03-on-premises-deployment-architecture.md) — DC, DR, UAT, 120,000-endpoint sizing, platform support, and India-only data flow.

---

## Quick Reference Links

* **Live Frontend:** `http://localhost:3100`
* **Gateway API Base:** `http://localhost:6000`
* **Swagger / OpenAPI Schema:** `d:\unifai_project\transports\config.schema.json`
* **Configuration Sync:** Run `sync_all_from_env.py` to synchronize environment settings.
