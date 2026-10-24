# YP-Raksha Gateway and Browser AI Guard Product Datasheet

## 1. Product overview and customer value

YP-Raksha is an enterprise product for governing application access to configured AI model services and managed-browser use of AI destinations. It brings these usage paths into one customer-operated product experience: applications use the Gateway; managed employee browsers use Browser AI Guard on qualified endpoints and browser surfaces.

The Gateway provides a supported API surface, configured provider/model choices, access and usage controls, routing options, and request records. Browser AI Guard supports target-site policy, prompt and upload inspection on covered request surfaces, employee-facing actions, agent fleet configuration, and policy event review. Administration, prompt/skills workspaces, optional MCP capabilities, and operational views support teams that configure and oversee these capabilities.

YP-Raksha does not provide model compute. A configured provider or private model service performs inference. The product complements, rather than replaces, enterprise identity, endpoint management, network enforcement, data classification, model operations and incident response controls.

| Product fact | Customer-facing description |
|---|---|
| Product | YP-Raksha Gateway and Browser AI Guard |
| Primary purpose | Govern supported application-to-model requests and managed-browser AI use |
| Audiences | Employees, application developers, platform administrators, security, governance and operations teams |
| Operating model | Customer-operated application services and approved dependencies |
| Model execution | Performed by a configured provider or private model endpoint |
| Browser enforcement | Depends on the selected Guard release, managed endpoint, extension, proxy path, target and policy coverage |
| Data boundary | Can be designed for a customer-controlled India boundary when dependencies are internal or disabled and network egress is enforced |

The product value is practical and control-oriented: provide a managed path to approved AI services, apply configured policies on supported use cases, allocate access and usage by approved scope, and give authorized administrators records to investigate product activity.

**Product portfolio at a glance.** The product combines application-facing Gateway services, provider and model administration, access governance, request guardrails, managed-browser AI Guard, shared prompt and Skills workspaces, optional MCP and plugin capabilities, and operational visibility. These are separately configurable product areas: enabling one does not automatically enable every other area or its data capture.

<!-- page-break -->

## 2. Problem statement, target users and use cases

**Problem statement.** Enterprise AI use can grow through separate application integrations, provider credentials, browser access paths and team-level experiments. Without a managed product path, it can be difficult to know which model routes are approved, control who can use them, apply the bank's prompt policy on managed browsers, limit usage and investigate operational or policy events.

**Product response.** YP-Raksha provides a configurable Gateway path for supported application requests and a managed-browser Guard path for supported web AI interactions. Organizations can assign scoped access, choose approved model destinations, configure usage controls and review authorized records. The bank still owns its identity, endpoint and network policies, data classification and approval process.

| Use case | Primary user | Product contribution | Customer must define |
|---|---|---|---|
| Route an internal application to an approved model | Application team | Gateway API, configured model route and application credential | Approved model, request shape, workload, owner and fallback policy |
| Govern employee browser AI use | Security and endpoint teams | Guard target/rule configuration and supported in-browser outcomes | Monitored destinations, covered surfaces, policy actions and endpoint rollout |
| Allocate AI access and usage | Platform or governance team | Virtual keys, scopes, budgets and rate controls where configured | Organization mapping, limits, exception approval and review cadence |
| Make reusable prompts available | Business and application teams | Prompt Repository and Skills workspaces where enabled | Content owner, sharing scope, review and version lifecycle |
| Review product activity | Operations, security and administrators | Configured logs, dashboards, policy events and health signals | Data minimization, authorized viewers, retention and escalation process |

Representative users include employees using a managed browser, developers integrating an application, administrators managing access and provider settings, security teams maintaining Guard policy, and operations teams reviewing service health. Name an owner for each route, model, rule set, integration and data-retention class.

Common adoption scenarios include an internal assistant routed to an approved private model, a developer service using an application-scoped key, a controlled browser pilot for selected AI sites, reuse of reviewed prompt templates by a business unit, and review of model usage or Guard policy events. Each scenario should identify its users, data class, model/tool destinations, owner and permitted outcome before configuration begins.

Use synthetic or approved masked material during evaluation. Customer data classifications, AI-use policy and regulated-workflow approvals remain customer decisions.

<!-- page-break -->

## 3. Capability portfolio and product boundaries

The product is organized around customer-facing capability areas. Feature availability can depend on release, configuration, permissions, enabled integrations and the customer deployment.

| Capability area | What users can do | Value to the organization |
|---|---|---|
| AI Gateway | Connect supported applications to configured model services through a controlled API surface | Consolidates approved application access and provider configuration |
| Model operations | Register providers/models, manage model settings and configure supported routes | Makes approved model choices visible to authorized teams |
| Governance | Assign scoped access and configure usage policies and limits | Supports accountability and controlled allocation |
| API guardrails | Configure supported request/response checks and provider settings where enabled | Adds policy controls to Gateway requests, separately from browser enforcement |
| Browser AI Guard | Apply configured rules to covered managed-browser AI interactions and review events | Extends governance to selected employee browser use |
| Prompt and Skills workspaces | Create, organize, version and share reusable content within configured scopes | Helps teams reuse reviewed prompts and skills |
| MCP capabilities | Configure selected servers, tool groups and access sessions where enabled | Provides a governed path to configured tool integrations |
| Administration | Manage users, permissions, providers, policies and product settings | Gives designated operators a common management experience |
| Observability | Review configured request, policy, health and operational signals | Helps teams investigate use and service behavior |
| Extensibility | Enable supported plugins or connectors for selected product functions | Allows approved extensions to participate in the deployment |

**Product boundary.** YP-Raksha is a governance and access layer. It is not an AI model, a general endpoint-management system, an enterprise identity provider, a universal web content filter, or a standalone certified DLP service. It does not make public model destinations private and does not independently guarantee data residency.

**System ownership.** The customer chooses the identity source, model endpoint, network rules, endpoint-management tool, database and storage services, retention policy and internal monitoring destination. These dependencies determine which capabilities can be enabled and how they are operated.

A feature in product code or a configurable connector is implementation evidence. Production support for an exact release, provider operation, browser build, endpoint version or integration must be confirmed for that release and customer environment.

**Optional data services.** The implementation has configurable response/semantic caching and vector-store paths. These are not mandatory for the basic Gateway or Guard flow. When enabled, cached prompts, responses, embeddings or indexes can become additional sensitive data stores; the customer must set locality, access, tenant separation, expiry and deletion behavior before use.

<!-- page-break -->

## 4. Gateway capabilities for application teams

Application teams can use the YP-Raksha Gateway as the configured entry point for supported model requests. The client uses the bank-provided internal Gateway address and an application-scoped credential; the Gateway applies configured authentication, model/provider selection, policy and request handling before returning or streaming the provider result.

| Application capability | Product behavior | Application-team consideration |
|---|---|---|
| Supported inference APIs | Receives supported request formats and routes them to the configured provider/model | Validate each required operation and payload shape in UAT |
| Application credentials | Supports virtual-key access for configured inference routes | Store keys in the application secret manager and scope to the intended application |
| Streaming requests | Supports configured streaming paths for compatible provider operations | Test timeouts, disconnects, partial output and retry behavior |
| Request controls | Applies enabled budgets, limits, routing rules and guardrail behavior | Coordinate request policy with the platform owner |
| Response handling | Returns the provider result through supported response behavior | Account for provider-specific response differences and error mapping |
| Request records | Can record configured request metadata and other approved fields | Agree payload capture, redaction, access and retention before production |

The Gateway is intended for applications that can reach an internal service and use a supported API/authentication shape. It is not a substitute for application-level authorization or safe output handling. Applications remain responsible for user context, business decisions, downstream effects, user-facing error behavior and protection of their own logs.

**Typical adoption pattern.** The application owner identifies required model operations and expected request profile, receives an approved route and credential, validates synthetic requests in UAT, then promotes the integration under bank release control. Exact endpoint paths, schemas, headers and error responses belong to the selected release's API reference.

Each production integration should have a named application owner, approved data class, permitted model/provider scope, usage limits, support contact and a process for rotating or revoking credentials. Do not embed credentials in a browser application or distributable client package.

**Operation families.** Depending on the provider adapter and release, the Gateway API surface includes text/chat and response-style inference, embeddings and reranking, OCR, audio, image, video, files and batch/async operations, plus selected real-time or WebSocket flows. The repository also contains compatibility/pass-through paths for common client and framework families. Treat these as operation families, not a promise that every provider implements every operation or parameter. Validate payload shape, streaming mode, limits, file lifecycle, response mapping and error behavior for each required integration.

Compatibility paths represented in the implementation include OpenAI, Anthropic, GenAI, Bedrock and Cohere API families, plus client/framework integrations such as Cursor, LiteLLM, LangChain and Pydantic AI. Confirm the required client version, authentication headers and operation subset for the selected release.

<!-- page-break -->
## 5. Provider, model and request compatibility

The product includes provider integration paths for hosted and self-managed model services. Available adapters and supported operations are determined by the selected release and provider SDK/API version used by that release.

| Provider or service family | Example integration path | Qualification needed |
|---|---|---|
| OpenAI-compatible services | Hosted API or compatible private endpoint | Confirm route, model identifier, request fields, response format and streaming |
| Anthropic-compatible services | Supported adapter or compatible endpoint | Confirm required message/tool operations and provider-specific options |
| Azure-hosted models | Configured Azure model/provider path | Confirm deployment identifier, API version, network and authentication |
| Amazon Bedrock | Provider path using configured regional service credentials | Confirm model access, region, IAM policy and operation coverage |
| Google Vertex/Gemini family | Configured Google provider endpoint | Confirm model availability, authentication, region and request/response behavior |
| Cohere, Mistral and other adapters | Adapter-dependent provider/model configuration | Verify exact operation and supported features for the release |
| Self-hosted inference | vLLM, Ollama or another compatible service where configured | Validate compatibility, concurrency, tokens, streaming and private network placement |
| Custom compatible endpoint | Configured OpenAI- or Anthropic-compatible service where supported | Validate protocol compatibility; do not assume complete feature parity |

The codebase contains adapters across a broader set of hosted and private providers, including OpenAI, Anthropic, Azure, Amazon Bedrock, Google Vertex/Gemini, Cohere, Mistral, OpenRouter, Groq, DeepSeek, Fireworks, Ollama and vLLM. This list is illustrative, not an entitlement or release support matrix. A named adapter does not prove availability in a specific bank region, a commercial license, a model's approval status or full endpoint compatibility.

The product can expose a configured model catalog and model-specific settings. Catalog records may be entered or synchronized depending on provider support and release behavior. Displaying a model does not mean it is licensed, reachable, available in the customer's region or approved for every data class.

**Operation-level fit matters.** A provider may support text generation but differ for embeddings, image inputs, audio/video, tool calls, structured output, streaming or other operations. Request parameters, token accounting, pricing and error semantics may also differ. Record the operations the application needs and validate each one against the exact provider/model/release combination.

A customer-operated private endpoint is appropriate for strict no-egress design. A public provider integration may send prompts and other request content outside the customer boundary and must be disabled or explicitly approved under the bank's policy.

<!-- page-break -->

## 6. Access governance and usage controls

YP-Raksha provides controls for assigning and limiting product access. The implementation includes named users, application roles and permissions, resource/operation grants, section visibility, organization-related scopes, virtual keys, budgets and rate controls where enabled. Exact permissions and assignment behavior should be checked against the selected release.

| Control | Product purpose | Customer governance decision |
|---|---|---|
| Named user and role | Separates employee/operator access from application credentials | Who may administer, review or use each product area |
| Resource and operation grants | Restricts management actions by resource and operation | Which teams may view, create, edit, delete or approve records |
| Section access | Limits which workspace areas an account can see | Which functions each role needs |
| Organization scope | Associates users and usage with configured customers, business units or teams | How the bank maps its own organization and accountability |
| Virtual key | Gives an application or approved user a revocable inference credential | Owner, model scope, lifecycle, rotation and revocation |
| Budget and rate limits | Constrains configured usage at supported scopes | Limit values, warning/denial behavior and exception handling |
| Audit path | Records supported management mutations and operational events | Who may inspect audit data, retention and escalation |

The implementation includes three seeded system role names (`admin`, `sub_admin` and `user`), with permissions that can be customized. The resource/operation model and workspace-section grants provide separate ways to constrain management actions and visible product areas. Access profiles can package reusable governance settings; customer/team/business-unit associations and virtual-key grants can scope approved usage. Optional OAuth2/SCIM provisioning paths are available in the implementation, subject to the selected release and bank identity design.

These controls support least-privilege administration, but do not replace the bank's identity provider, multifactor authentication, approval workflow, separation of duties, privileged-access management or user lifecycle policy. The product can use configured identity and provisioning integrations where supported; the bank remains responsible for selecting and validating them.

Give each integration a distinct credential and owner. Scope access to the smallest needed model, route, workspace or management action. Test representative role accounts, confirm denial for out-of-scope actions, and include emergency revocation in the customer's operating procedures.

Rate limits can exist at multiple layers: product governance, provider quotas, ingress/network policy and endpoint Guard ingestion. These are separate controls, not one shared capacity limit.

<!-- page-break -->

## 7. Model routing, cost controls and resilience

For supported requests, the product can select configured provider/model routes using customer-defined settings. The implementation includes routing rules, weighted or adaptive routing options, complexity-based selection paths, pricing settings and circuit-breaker behavior. These capabilities depend on configuration and must be qualified with required provider operations and workload.

| Routing or cost feature | Intended customer use | Qualification point |
|---|---|---|
| Routing rules | Direct eligible requests to configured model/provider targets | Test rule order, match conditions, default route and access scope |
| Weighted or adaptive distribution | Distribute eligible requests across configured targets | Confirm health input, weighting behavior and per-route concurrency |
| Complexity-based selection | Use configured request characteristics to choose a model tier | Confirm classification policy and validate representative synthetic requests |
| Circuit breaker | Avoid repeatedly sending requests to an unhealthy destination | Confirm open/close conditions, probe behavior and recovery timing |
| Eligible fallback | Use an approved alternate route when configured conditions apply | Ensure fallback does not broaden data access or route to an unapproved provider |
| Pricing and usage attribution | Apply configured model pricing values to supported usage records | Verify model names, token counts, overrides and reporting semantics |
| Request guardrails | Apply enabled rule-based input/output validation to Gateway calls | Review rule scope, provider behavior, false positives and fail behavior |
| Response/semantic cache | Reuse eligible prior results or semantic matches when the cache plugin is enabled | Define cache keys, tenant isolation, expiry, invalidation and permitted data classes |

Budgets and token/rate limits can help teams govern consumption at supported scopes. Actual billable cost is determined by provider contract and usage; product pricing metadata is an operational estimate unless reconciled with the provider statement.

Resilience is bounded by configured routes and dependencies. A circuit breaker or fallback cannot create availability when all approved models, network paths or Gateway dependencies are unavailable. A retry may repeat a request, and a timeout after submission can leave completion state unknown. Applications should use finite timeouts and safe, operation-aware retry behavior.

Validate routing with synthetic content and failure injection before relying on it for continuity. Keep provider credentials and route configuration under the customer's change and secret-management process.

<!-- page-break -->

## 8. Browser AI Guard capabilities

Browser AI Guard is intended to govern selected AI-related browser activity on managed endpoints. The product includes endpoint agent and browser extension packaging paths, target and rule configuration, fleet policy distribution, event ingestion, agent heartbeat and administrator review capabilities. Coverage depends on the exact release and endpoint/browser configuration.

| Guard capability | What the customer can configure or review |
|---|---|
| Target websites | Identify domains or supported target surfaces to monitor, exclude or block |
| Rule sets | Define enabled patterns, keywords, content checks, severity and applicable scope |
| Interaction controls | Configure supported prompt, search or upload behavior and policy outcomes |
| Endpoint fleet | Enroll managed agents, distribute settings and review version/heartbeat signals |
| Browser deployment | Distribute the extension and required managed proxy/certificate policy using enterprise tooling |
| Policy event review | Review permitted event details and matched policy information through authorized views |
| Operational coverage | Observe agent heartbeat, policy synchronization and event delivery where enabled |

A customer typically uses Guard to bring approved browser AI activity under a defined policy on bank-managed devices. Endpoint administrators own installation, browser policy, proxy/PAC configuration, certificate trust, update distribution and device identity. Security owners define monitored sites, rules and actions. Product administrators manage Guard settings and review records within assigned access.

Guard inspection is not universal browser visibility. A request must pass through the configured supported endpoint/proxy/extension path and use a supported target and content surface. A site redesign, browser update, alternate API endpoint, unmanaged browser or disabled agent may change coverage. Block mandatory public AI access at network or endpoint boundaries as well; the extension alone is not a complete egress control.

Windows and macOS packaging paths and candidate browsers do not establish a release-level certification matrix. See the platform scope page before counting endpoints as covered.

The fleet functions also include managed policy/configuration pulls, agent heartbeat and health/version visibility, package delivery for Windows/macOS, and a proxy-bundle update path where enabled. Setup and remote-removal controls are administrator-facing lifecycle features. They should be used through signed, bank-controlled endpoint distribution and a protected internal Gateway path; an installed extension alone is not the complete Guard deployment.

<!-- page-break -->
## 9. Guard policy outcomes and coverage lifecycle

Guard gives the bank configurable outcomes for supported captured interactions. The policy owner chooses the target, match logic, scope and response. Exact employee wording, action timing and bypass behavior must be validated on the release and browser surfaces planned for deployment.

| Outcome | Product intent | Policy-owner consideration |
|---|---|---|
| Allow | Continue a request when no blocking rule applies or policy permits it | Define whether and what event metadata is retained |
| Redact | Modify matched content through the configured supported redaction path | Validate resulting text and intended user behavior |
| Warn | Present a policy notice or confirmation behavior where supported | Decide whether proceeding is allowed and how exceptions are recorded |
| Block | Stop a supported submission that violates an active rule | Validate relevant entry points and user-facing message |
| Site-block | Prevent access to a configured target through the supported Guard path | Maintain network enforcement to prevent alternate access |
| AI Guard Bot response | Return a configured bot response for supported AI-assisted rules | Optional model-backed evaluation; approve the model, prompt, latency and data path |

Rules may use configured matching mechanisms such as patterns or keywords. A rule's result depends on target matching, rule order, content extraction, browser selectors, agent health and proxy coverage. Rule authors should use synthetic test prompts, benign examples and false-positive review before broad rollout.

**Customer policy lifecycle.** Define the risk and data class; select the target and intended user group; configure a narrow rule and action; test in isolated UAT; pilot on a representative managed-browser cohort; review false positives and event delivery; then promote through controlled endpoint management. Keep an owner and review date for each production rule.

The configured rule engine is not a claim of an independently certified DLP classifier. Do not assume every attachment type, browser input, clipboard action, image, voice interaction, dynamic page or third-party AI interface is inspected. Document exclusions and implement network and endpoint controls for mandatory restrictions.

Rules can be configured as pattern/regular-expression checks or AI-assisted checks using a configured model path. The Guard setup includes target/rule administration, rule import and test/generation helpers in the implementation. For supported captured upload flows, the proxy can extract text from selected Office, PDF and image content before evaluation; exact formats, size limits, extraction quality and behavior on encrypted, malformed or unsupported files must be release-qualified. Search activity and clicked-result metadata are represented separately from prompt/upload events where the selected target and policy capture them.

<!-- page-break -->

## 10. Prompt Repository and Skills workspaces

The Prompt Repository and Skills capabilities support teams that want to create and reuse approved content. They are product workspaces governed by configured user access and organizational scope; availability and exact lifecycle behavior depend on release and enabled features.

| Workspace capability | Customer use |
|---|---|
| Folder organization | Arrange shared and team-level prompts or skills by intended business use |
| Prompt authoring | Create and maintain reusable prompt content for authorized users |
| Version and history | Track approved revisions where the configured workflow provides version records |
| Template variables | Reuse prompts with defined input values in supported playground flows |
| Interactive playground | Try multi-turn prompt drafts against an approved model route using permitted content |
| Skills library | Store reusable task instructions or skill documents in configured folders |
| Deployment controls | Publish or expose content to approved scopes where the release supports it |
| Access scoping | Limit who can view, edit, share or use repository content through available permissions |

Prompt records can include organized content, saved versions/history and interactive sessions; deployment records can publish selected content to a configured scope. Skills are stored as reusable task-instruction documents and can be versioned and served through supported repository paths. These lifecycle features provide controlled reuse, but do not by themselves review factual quality, prevent prompt injection or guarantee that a model follows the content.

A prompt is content, not a substitute for a tested application workflow. Prompt owners should identify the intended task, input data class, output use, review owner and known limitations. Do not place secrets, customer data or regulated content into shared prompt templates unless that use is explicitly approved.

Use named owners, review dates, version notes, controlled sharing and an archival process. The organization should decide whether playground sessions and prompt history are retained, who can inspect them, how sensitive values are masked, and how deletion flows through backups. Permissions should align with the bank's business-unit and team model.

These workspaces complement, rather than replace, source control, software delivery, document-management policy or model evaluation. For production use, validate prompt behavior with approved datasets and keep changes reviewable.

<!-- page-break -->

## 11. MCP tools, plugins and extensibility

Where enabled, MCP capabilities help administrators configure selected Model Context Protocol servers, tools and access sessions. They can include server catalog management, connector library entries, tool groups, user/application authorization sessions, OAuth grants and tool activity records. Each server and tool can expose data or actions; treat it as a separately governed integration.

| Capability | Product role | Customer control |
|---|---|---|
| Server catalog | Record configured server details and transport settings | Approve destination, operator, authentication and data class |
| Connector library | Reuse selected connector definitions where available | Review connector version and destination before enabling |
| Tool groups | Associate a collection of tools with an intended scope | Grant the minimum tools and business scope required |
| Authorization sessions | Manage configured user/application access to MCP servers | Review identity mapping, token storage, lifetime and revocation |
| OAuth grants | Manage downstream authorization where supported | Approve scopes, callback behavior and credential lifecycle |
| Tool activity records | Review configured MCP execution history | Set content minimization, access and retention |
| Plugins and hooks | Extend selected behavior using enabled plugins | Approve code, lifecycle, privileges, egress and release compatibility |

MCP is optional. Its presence does not mean a server is safe, certified or appropriate for sensitive data. A configured tool can read or modify downstream information according to its own permissions. Evaluate each tool for destination trust, least privilege, secret handling, side effects, logging and recovery.

Plugins and connectors may be built-in or separately loaded depending on release. Availability, licensing, startup behavior and compatibility are release-specific. For no-egress operation, do not enable a plugin, collector, connector or MCP server that sends data to an unapproved external destination. Keep the approved inventory and owner records in the customer's integration catalogue.

Configured MCP connections can use supported remote or local transport modes (for example, SSE or stdio, where packaged and enabled). Local stdio execution may require the relevant runtime and command to exist in the deployment image. The Gateway can group tools and associate authorization sessions or OAuth grants, but tool-level authorization does not replace the downstream system's own permissions and audit controls.

<!-- page-break -->

## 12. Product experience and responsibility by role

YP-Raksha brings different product tasks into role-scoped workspaces. The application contains seeded role behavior and supports permissions or section grants; customers should test the exact role mappings used for the selected release.

| Persona | Typical product activities | Customer responsibility |
|---|---|---|
| Employee | Use approved model choices, managed browser and permitted prompt/skills content | Follow AI-use policy; avoid entering unnecessary sensitive information |
| Application developer | Integrate a service with an approved route, credential and model operation | Protect application secrets, handle errors and follow the integration owner process |
| Business content owner | Create or maintain shared prompts and skills where enabled | Own review, accuracy, data scope and content retirement |
| Platform administrator | Configure providers, models, access, routes, system settings and operational services | Change control, identity integration, secret protection, recovery and least privilege |
| Security administrator | Configure Guard targets/rules, review policy events and coordinate endpoint coverage | Policy ownership, false-positive handling, exception approval and incident process |
| Operations administrator | Review service health, request records, alerts and deployment signals | Internal monitoring, backup/restore, DR and escalation |
| Auditor or reviewer | Inspect records and sections granted to the account | Follow evidence-handling and retention policy |

The product role model does not by itself establish separation of duties. Assign named operator accounts, use the narrowest available grants, limit full administrators, review access periodically and retain evidence of privileged changes under bank policy.

Employees receive approved access routes and Guard feedback; developers receive a consistent service endpoint and scoped credentials; operators get configuration and records appropriate to their role. Exact page names and actions can change by version, and the selected release's interface and API references remain authoritative.

The embedded workspace groups product tasks into model/provider and routing views; governance and access-profile views; Browser AI overview, logs, search logs, rules, targets, agents and setup; Prompt/Skills repositories; MCP server, tool-group and session views; guardrail configuration; and logs, audit, metrics, connectors and alert channels. The Gateway serves the workspace with the application deployment, so a separate public-facing web tier is not required by the product packaging.

This datasheet describes product surfaces at a glance. The Technical Documentation contains detailed administrator and end-user procedures.

<!-- page-break -->

## 13. Observability and operational visibility

The product can expose operational and business-usage information through configured dashboards, logs, metrics, traces and event views. Enabled signals depend on release, settings, plugins and storage/collector configuration. Records can contain sensitive content or identifiers, so observability must be designed as a data-handling control.

| Signal family | Example product visibility | Customer use |
|---|---|---|
| Request activity | Request count, route/model, status and timing where captured | Identify usage changes and investigate application errors |
| Usage and cost metadata | Token counts, pricing metadata or budget attribution where available | Review configured consumption and reconcile with provider billing |
| Browser Guard events | Target, agent, policy action, rule match and status where recorded | Review policy operation, coverage and false positives |
| MCP activity | Tool use and configured execution outcomes | Investigate tool integrations and downstream access |
| Service health | Readiness, dependency status and runtime health signals | Operate service availability and investigate dependency problems |
| Metrics and traces | Internal metrics/traces or configured exporters | Diagnose performance and integration behavior |

The product workspace can also provide filtered log views, dashboard summaries, model/provider rankings and time histograms, Guard Insights, MCP activity, audit records and configured alert channels. Prometheus/OpenTelemetry-compatible paths are available in the implementation; actual signals, labels and export behavior depend on enabled plugins/settings. These are product operational views, not a guarantee that every event is retained or a replacement for the bank's central monitoring and audit systems.

Payload capture is configuration-dependent. A product log view should not be assumed to contain every request or response; it should also not be assumed to omit sensitive fields. Define whether prompts, outputs, attachments, headers, user identifiers or tool arguments may be recorded, then verify the selected release settings.

Point collectors and alert destinations to approved internal services, restrict diagnostic endpoints, minimize sensitive labels, separate operational diagnostics from business audit records, and set retention by record class. External telemetry integrations create external data paths and must be separately approved.

Observability helps teams investigate product operation; it is not a substitute for an enterprise SIEM, fraud monitoring, business audit or provider invoice. Validate which signals are available and what each one proves before designing compliance reporting around it.

<!-- page-break -->
## 14. Security and trust posture

YP-Raksha uses different access paths for application inference, administrator sessions and Guard fleet communication. Management operations use application authorization controls, while inference access can use virtual keys and configured governance. Guard communication uses its supported agent authentication path. This separation helps customers assign credentials to the relevant product function.

| Security area | Product contribution | Customer deployment responsibility |
|---|---|---|
| Authentication | Supports configured admin/session, inference-key and Guard-agent authentication paths | Connect approved identity, protect secrets and secure enrollment |
| Authorization | Applies role/resource and section permissions to supported management actions | Assign least privilege, test role boundaries and review exceptions |
| Secret protection | Supports configured encryption for selected stored secrets | Protect encryption key, database/provider secrets and recovery material |
| Transport protection | Operates behind configured internal TLS and private service paths | Enforce TLS, certificate validation and trusted proxy boundaries |
| Auditability | Provides audit paths for supported management changes and product events | Restrict record access, set retention and define review/escalation ownership |
| Network boundaries | Can use internal provider and operations destinations | Enforce default-deny egress and prevent direct endpoint bypass |

Application-level encryption of selected secrets is not the same as end-to-end encryption of every prompt, record, attachment, log or backup. Use bank-managed encryption at database, volume, backup and secret-store layers. Keep Guard fleet credentials out of extension code, browser-visible settings and general support bundles.

No external certification, independent security assessment, regulatory attestation or compliance status is asserted by this datasheet. Map the deployed product and customer controls to the bank's own standards using evidence from the exact release and environment. Security behavior can change between releases and should be requalified after material updates.

The implementation also includes session and inference authentication paths, login-abuse protections, security response headers, error sanitization, an audit path for management mutations and optional identity provisioning integrations. Their effective protection depends on production settings: disable bootstrap/open-auth modes, use TLS, secure cookies and trusted proxy boundaries, enforce strong secrets, and restrict administrative and diagnostic surfaces.

<!-- page-break -->

## 15. Data categories, privacy and residency

YP-Raksha may process operational configuration, AI request data and endpoint policy events. Actual fields stored or exported depend on product settings, enabled features and customer integrations.

| Data class | Examples | Product use | Customer decision |
|---|---|---|---|
| Identity and access | User, role, team, session and audit records | Authenticate users and apply scope | Identity source, retention, privileged review and deletion |
| Configuration | Provider/model settings, routes, virtual keys, budgets and Guard rules | Operate approved product behavior | Secret storage, change rights, export and recovery |
| Inference records | Request metadata, model, timing, status, token/cost fields and optional payload | Support usage visibility and troubleshooting | Payload minimization, access and retention |
| Browser Guard events | Target, agent, policy action, rule match and supported prompt/search data | Apply policy and authorized review | Employee notice, field capture, retention and permitted review |
| Attachments | File metadata and supported content bytes when captured | Inspect or retain supported upload evidence | File classes, size limits, access, retention and deletion |
| MCP records | Server/tool identity, execution metadata and configured arguments/results | Review configured tool activity | Data minimization and downstream destination approval |
| Operational telemetry | Logs, metrics, traces, alerts and health events | Support service operation | Internal destinations, sensitive-label filtering and retention |

**Residency.** Bank- and India-resident operation is a deployment property, not a product default. It requires every enabled model, database, attachment, backup, identity service, artifact source, telemetry, mail, connector, licensing/update path and support process to remain inside the approved boundary or be disabled. Network controls must enforce that decision.

**Retention and deletion.** Define retention separately for configuration/audit, inference events, Guard records, attachments, application logs, traces, backups and replicas. Verify deletion across metadata and file bytes, and understand backup expiration and disaster-recovery copies. Available cleanup controls vary by record class and release.

This datasheet does not claim that no data leaves the bank by default. The no-egress flow and network restrictions are described in the On-Premises Deployment Architecture and must be implemented in the bank's environment.

The product configuration includes a control to disable request-content logging for supported logging paths. Disabling payload capture can reduce stored content, while metadata, policy events, system logs or separately configured attachment/MCP records may still be produced. A per-request content-storage override can exist when explicitly enabled; review this setting and verify actual records with synthetic data before production. Do not interpret a “content logging disabled” setting as zero data retention.

<!-- page-break -->

## 16. Integration ecosystem and operating model

YP-Raksha can fit into a customer-operated service environment when required dependencies and destinations are available. Integration compatibility varies with release, configuration, customer network and upstream service version.

| Integration category | Product interaction | Customer-owned decision |
|---|---|---|
| Applications and SDKs | Call the Gateway's supported inference interface | API shape, credential scope, timeout, retry and application owner |
| Model/provider services | Execute configured inference and return results | Provider approval, private/public status, region, credentials and operation support |
| Enterprise identity | Authenticate users and optionally provision accounts | Identity protocol, claims/groups, MFA and lifecycle ownership |
| Database and durable storage | Persist product configuration and operational records | Engine/version, HA, backup, encryption, retention and recovery |
| Endpoint and browser management | Install Guard packages and managed extension configuration | Device cohorts, browser policy, certificate trust, proxy and update source |
| Monitoring and alerting | Receive configured internal signals and notifications | Approved collectors, destinations, field minimization and retention |
| Vector/cache services | Support selected cache or vector-store features where enabled | Data location, access control, indexing, retention and compatibility |
| MCP servers and plugins | Expose selected tools or extensibility behavior | Destination approval, scopes, side effects, secrets and compatibility |

The product can be deployed on customer-managed container or Kubernetes infrastructure, with a relational database and durable storage. The exact deployment package, image/chart mapping, dependency version and supported infrastructure version must match the selected release.

**Product onboarding.** Identify the business use case and data class; select approved model/provider routes; define access, Guard policy and retention; provision UAT with synthetic data; validate the chosen API, endpoint and integrations; pilot with a limited managed cohort; then promote the approved release through change control. This is a customer adoption model, not a substitute for detailed deployment instructions.

For bank-only operation, internal DNS, time, artifact mirrors, identity, secrets, model services, telemetry, backups, DR and support paths must be designed inside the approved service boundary.

Integration examples in the implementation include relational ConfigStore/LogsStore services, optional vector/cache backends, SMTP-based notification, Prometheus/OpenTelemetry telemetry, alert channels and log/analytics connectors. Some connectors are capable of forwarding records to external systems. Enable only the required destinations, configure bank-operated internal endpoints and check the actual network path rather than relying on the connector's label or UI state.

<!-- page-break -->
## 17. Supported platform scope and capacity reference

The table distinguishes build/package evidence from an unconditional support commitment. Exact supported versions, signatures and compatibility must be confirmed for the selected YP-Raksha release.

| Platform area | Implementation evidence | Planning position |
|---|---|---|
| Gateway hosting | Container and Helm deployment paths for Linux-based infrastructure | Use customer-supported container/Kubernetes platform matched to the release |
| Windows endpoint Guard | Windows executable and installer build paths | Confirm OS editions/builds, signing, privileges and endpoint deployment |
| macOS endpoint Guard | macOS application/package build paths | Confirm OS versions, signing/notarization, permissions and MDM policy |
| Linux endpoint Guard | No Linux endpoint installer documented in the reviewed implementation | Do not count Linux endpoints as Guard-covered without release evidence |
| Chrome and Edge | Chromium extension packaging and managed proxy/certificate mechanisms | Candidate browsers; qualify exact managed builds and policies |
| Firefox | Firefox extension packaging path exists | Qualify version, install policy and certificate trust |
| Safari | No qualified enterprise compatibility matrix established here | Unqualified until release packaging and validation evidence exist |
| VDI | No named vendor/version certification matrix established here | Qualify the exact Citrix, VMware Horizon, Azure Virtual Desktop or other session design |
| Database | PostgreSQL, MySQL and SQLite implementation paths; PostgreSQL is planning baseline | Confirm supported engine/version/topology; SQLite is not the HA choice at this scale |

**Technology profile at a glance.** The Gateway service is implemented in Go; the management workspace is a React-based UI embedded into the Gateway package. Browser Guard includes packaged Windows/macOS agent builds and browser extensions. The product repository contains Docker/Compose and Helm deployment paths and relational-store drivers. This describes implementation components, not a commitment to a specific language/runtime support lifetime or a production bill of materials; pin the release artifacts and dependencies used by the bank.

**120,000-endpoint planning reference.** The companion on-premises design uses 120,000 registered endpoints and a planning activity profile of 12,000 busy-period active users/endpoints. It models about 67 steady Guard events per second and 100 steady inference requests per second, with planning targets of 150 events and 200 inference requests per second. These are workload assumptions and design targets, not measured results, a certified maximum, SLA or guarantee.

Endpoint count does not equal concurrent activity. VDI session starts, login bursts, browser refresh, request size, attachments, model latency, database writes and retention can change demand. Private model compute is sized separately for the selected model and token/concurrency profile. Use the companion architecture for DC/DR/UAT resources, and validate the actual release and estate before procurement.

<!-- page-break -->

## 18. Product fit, limitations and terminology

The fit positions below summarize implementation evidence and customer qualification needs. They are not a score out of 10, performance benchmark, certification or service commitment.

| Customer requirement | Product fit position | Evidence and qualification |
|---|---|---|
| Central access path for supported application model requests | Strong functional fit | Gateway APIs, provider/model routes and governance controls exist; validate each required operation |
| Policy for selected managed-browser AI use | Strong functional fit on supported captured surfaces | Guard packages, target/rule configuration and event paths exist; qualify browser, target and enforcement behavior |
| 120,000 managed endpoint deployment | Conditional fit | Planning envelope exists; confirm activity, endpoint/VDI mix, private model capacity and measured load |
| Bank/India-resident operation | Conditional fit | Requires internal or disabled dependencies and enforced default-deny network egress |
| Named browser/VDI certification, fixed throughput or SLA | Not established by this datasheet | Obtain release-specific evidence and contractual terms before representing as supported or guaranteed |
| Regulatory certification or independent compliance status | Not asserted | Require formal evidence for exact product release and assessment scope |

**Important limitations.** Provider adapters do not guarantee feature parity. Guard only covers supported configured request surfaces. The rule engine is not an independently certified DLP service. Model execution, identity, network security, endpoint management, HA database, cross-site DR and backup are customer-operated dependencies. Application-level secret encryption does not mean every record or attachment is encrypted by the application. Product operation alone cannot guarantee residency when external routes remain open.

The product does not establish a fixed retention period, endpoint/browser/VDI certification, universal file-format coverage, guaranteed inference latency, fixed maximum concurrency, provider availability or contractual SLA in this datasheet. These depend on the selected release, workload, provider terms and customer deployment. The 120,000-endpoint scenario is a qualification design basis; publish production sizing only after the customer workload and measured results are reviewed.

**Product terms.** *Gateway* means the product interface used by supported applications to reach configured models. *Guard* means the managed endpoint agent/extension and associated policy/event functions. *Provider* is the configured upstream model service. *Virtual key* is an application-facing product credential. *MCP* is the optional Model Context Protocol integration area. *UAT* is the customer's isolated qualification environment.

Use this datasheet for product selection and scope discussion. Use the Technical Documentation for implementation and administration detail, and the On-Premises Deployment Architecture for DC/DR/UAT topology, data flows and the 120,000-endpoint planning design.
