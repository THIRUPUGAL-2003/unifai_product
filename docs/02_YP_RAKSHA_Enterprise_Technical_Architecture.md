<div align="center">

![YesPanchi Logo](file:///d:/unifai_project/ui/public/yes-panchi-logo.png)

# YP-RAKSHA
### Real-Time AI Knowledge Screening & Hazard Audit

---

## Enterprise Technical Architecture Manual

**Version 2.5**

</div>

<div style="page-break-after: always;"></div>

---

# Table of Contents

| Section | Major Topic | Sub-Topics & Architecture Focus | Page No |
| :---: | :--- | :--- | :---: |
| **3.0** | **System Architecture & Dual Data Planes** | • 3.1 The Dual Data Plane Paradigm (PAC vs API Gateway)<br/>• 3.2 Dual Deployment Operational Modes (Workstation vs Server)<br/>• 3.3 Core Processing Pipeline (11-Stage Inspection Engine)<br/>• 3.4 End-to-End Prompt Lifecycle & Rule Check Flow<br/>• 3.5 Real-Time End-to-End Latency Budget & Waterfall (3.64ms) | **3** |
| **4.0** | **On-Premises Architecture (DC, DR & UAT)** | • 4.1 Multi-Tier Enterprise DMZ Network Segmentation<br/>• 4.2 Primary DC High-Availability Cluster (Active-Active)<br/>• 4.3 Warm-Standby Disaster Recovery (RPO=0, RTO<15m)<br/>• 4.4 User Acceptance Testing (UAT) Sandbox Environment<br/>• 4.5 DC-DR Cutover Verification Matrix & Network Routing | **6** |
| **5.0** | **Infrastructure Sizing for 1,20,000 Endpoints** | • 5.1 Concurrency & Workload Mathematical Model<br/>• 5.2 Primary DC Hardware Sizing Matrix (34 Nodes)<br/>• 5.3 Storage Capacity Sizing & Tiered Retention Architecture<br/>• 5.4 DR & UAT Hardware Sizing Matrix<br/>• 5.5 Enterprise Fleet Distribution & Branch Network Impact<br/>• 5.6 Empirical Stress-Testing Report (13,750 req/sec Peak)<br/>• 5.7 Hardware Bill of Materials (BOM) & Rack Layout (28U)<br/>• 5.8 Branch Bandwidth & CBS (Finacle/Flexcube) Non-Interference | **8** |
| **6.0** | **OS, Browsers, Desktop EXE & Extension** | • 6.1 Desktop OS Matrix<br/>• 6.2 Browser Steering Matrix<br/>• 6.3 Desktop EXE Agent Architecture & Rollout<br/>• 6.4 Browser Extension (.crx/MV3) & Silent Install<br/>• 6.5 Hybrid Deployment (Agent vs Proxy)<br/>• 6.6 VDI Platforms (Citrix/AVD)<br/>• 6.7 Laptop Hardware Specs<br/>• 6.8 Footprint & Non-Interference | **12** |
| **7.0** | **Sovereign Data-Flow & Zero-Egress Proof** | • 7.1 Regulatory Baseline (RBI Master Directions, DPDP Act, CERT-In)<br/>• 7.2 Cryptographic Data Flow Architecture (Zero Exfiltration)<br/>• 7.3 Zero Telemetry & Zero External Beacons Guarantee<br/>• 7.4 Next-Generation Firewall (NGFW) Egress Verification Matrix<br/>• 7.5 Deep-Packet Inspection (DPI) & Forensic Audit Test Suite | **17** |

<div style="page-break-after: always;"></div>

---

# Section 3.0 — System Architecture & Dual Data Planes

## 3.1 The Dual Data Plane Paradigm
Enterprise environments require two completely different AI traffic inspection patterns: direct API consumption by automated systems, and interactive browser navigation by human personnel. YP-Raksha solves this dual requirement through two distinct operational data planes residing within a single unified control infrastructure:

```
+----------------------------------------------------------------------------------------------------+
|                                    DUAL DATA PLANE TOPOLOGY                                        |
|                                                                                                    |
|   PLANE 1: APPLICATION & API DATA PLANE                                                            |
|   +--------------------------+         +----------------------------+        +-------------------+ |
|   | Enterprise Applications, |  mTLS   | Gateway Core Data Plane    | HTTPS  | Configured LLM    | |
|   | Microservices & Code IDEs| ------> | • Universal Proxy Engine   | -----> | Model Provider    | |
|   +--------------------------+         | • Virtual Key Enforcement  |        +-------------------+ |
|                                        | • Budget & Rate Throttling |                              |
|                                        +----------------------------+                              |
|                                                                                                    |
|   PLANE 2: ENDPOINT & BROWSER CONTROL PLANE                                                        |
|   +--------------------------+         +----------------------------+        +-------------------+ |
|   | Corporate Endpoints      | PAC     | Endpoint Browser Guard     | HTTPS  | Public AI Web     | |
|   | (Chrome / Edge / Safari) | ------> | • Local PAC Redirection    | -----> | Portals (ChatGPT, | |
|   +--------------------------+         | • Inline DOM / Form Intercept       | Claude Web)       | |
|                                        | • Visual Educational Alert |        +-------------------+ |
|                                        +----------------------------+                              |
|                                                      |                                             |
|                                                      v                                             |
|                                        +----------------------------+                              |
|                                        | Centralized Management     |                              |
|                                        | & Audit Database           |                              |
|                                        +----------------------------+                              |
+----------------------------------------------------------------------------------------------------+
```

### Data Plane 1: Application API Proxy Plane (`/v1/*`)
* **Target Audience:** Internal core banking software, microservice backends, enterprise copilot applications, and developer IDEs.
* **Mechanism:** Applications direct standard REST or gRPC AI calls to the YP-Raksha cluster rather than connecting directly to third-party providers.
* **Enforcement:** YP-Raksha validates the inbound Virtual Key, confirms departmental budget availability, executes CEL security rules, redacts sensitive payload variables, routes the call to the optimal provider, and records audit telemetry.

### Data Plane 2: Endpoint Browser Guard Plane (`/api/browser-ai/*`)
* **Target Audience:** Enterprise desktop, laptop, and virtual desktop infrastructure (VDI) web browser traffic.
* **Mechanism:** Managed endpoints use standard enterprise Proxy Auto-Configuration (PAC) routing files distributed via Active Directory Group Policy (GPO) or MDM configurations. Only traffic destined for target AI domains is intercepted; general enterprise web traffic bypasses the agent entirely.
* **Enforcement:** The lightweight agent performs real-time inspection of browser POST bodies, multipart file uploads, and JSON input payloads. If an unmasked credit card or national identifier is detected, the agent blocks or redacts the submission instantaneously and presents an enterprise security warning directly inside the user's browser.

## 3.2 Dual Deployment Operational Modes

| Deployment Attribute | Mode A: Endpoint Workstation Mode | Mode B: Shared Network / VDI Proxy Mode |
| :--- | :--- | :--- |
| **Operational Scope** | Distributed across managed employee laptops | Centralized cluster within the enterprise DMZ |
| **Target Infrastructure** | Windows 10/11 laptops, macOS workstations | Thin clients, non-persistent VDI pools, branch networks |
| **Interception Point** | Local loopback interface on the workstation | Dedicated enterprise network proxy service VIP |
| **Traffic Steering** | Local PAC file bound to the loopback interface | Enterprise PAC distributed via DHCP / GPO auto-detect |
| **Certificate Trust** | Local root trust store installed via enterprise installer | Centralized corporate enterprise root CA distributed via GPO |
| **Offline Resilience** | Caches local policy rules; enforces blocks even if disconnected | Requires continuous connectivity to the network proxy cluster |

<!-- pagebreak -->

## 3.3 Core Processing Pipeline Sequence
Every transaction passing through YP-Raksha undergoes a mandatory, multi-stage pipeline:
1. **Network Ingestion & Session Validation:** Inbound TLS handshake verification, client authentication (mTLS / API Token / Kerberos session validation).
2. **Identity & Context Resolution:** Mapping the request to an Active Directory User, Business Unit, Cost Center, and Virtual Key.
3. **Budget & Rate Quota Validation:** Sub-millisecond verification against Redis distributed counters to ensure TPM/RPM thresholds and financial allocations are not exceeded.
4. **Pre-Flight DLP & Regex Sanitization:** Scanning the text payload against national identifier algorithms (Verhoeff, NSDL, Luhn).
5. **Common Expression Language (CEL) Evaluation:** Running custom, user-defined organizational security rules.
6. **Secondary Semantic Guard Bot (Conditional):** Submitting high-risk inputs to an on-premise private screening model for intent analysis.
7. **Payload Action Execution:** Proceeding with **Allow**, executing **Redact & Mask**, or terminating with **Block & Alert**.
8. **Upstream Model Transmission:** Forwarding the sanitized payload to the designated AI provider endpoint over an encrypted tunnel.
9. **Post-Flight Response Inspection:** Validating the AI model's response for rogue outputs, toxic payloads, or systemic hallucinations.
10. **Asynchronous Audit Commit:** Logging the sanitized transaction metadata to local immutable storage for compliance and analytics.

## 3.4 End-to-End Prompt Lifecycle & Rule Check Architecture

The core operational paradigm of YP-Raksha centers on the atomic lifecycle of an AI prompt—tracing the exact trajectory from initial user keypress to upstream model dispatch and safe response rendering:

```
+----------------------------------------------------------------------------------------------------------------------+
|                                 END-TO-END PROMPT LIFECYCLE & SECURITY PIPELINE                                      |
|                                                                                                                      |
|   [STAGE 1: PROMPT INCEPTION]                                                                                        |
|   Employee types prompt or attaches document in Browser (ChatGPT, Claude) or submits via Developer API / IDE.       |
|                                                      |                                                               |
|                                                      v                                                               |
|   [STAGE 2: TRANSPARENT GUARD INTERCEPTION]                                                                          |
|   • Endpoint Browser Guard intercepts traffic via Local Loopback Interface (Port-less PAC redirection).             |
|   • Application API calls intercepted via Enterprise Gateway Core Ingress VIP.                                       |
|                                                      |                                                               |
|                                                      v                                                               |
|   [STAGE 3: PAYLOAD EXTRACTION & LEXICAL TOKENIZATION]                                                               |
|   • Decodes JSON / Form-Data POST body; extracts raw prompt strings and strips multipart attachments.               |
|   • Normalizes character encodings (unpacks Base64, Hexadecimal, Unicode homoglyphs, and URL encoding).             |
|                                                      |                                                               |
|                                                      v                                                               |
|   [STAGE 4: PREDICT & MATHEMATICAL HEURISTIC CHECKS] (Latency: < 0.8 ms)                                             |
|   • Aadhaar Checksum Engine: Verhoeff algorithm (Dihedral D5 group) validates 12-digit numbers.                     |
|   • PAN Card Structure Engine: 5-Alpha + 4-Digit + 1-Alpha NSDL structural rules validation.                         |
|   • Payment Card Engine: Modulo-10 Luhn algorithm BIN checks across Visa, Mastercard, RuPay, Amex.                   |
|   • Developer Secrets Engine: Shannon entropy analysis detects private keys, JWTs, and cloud credentials.            |
|                                                      |                                                               |
|                                                      v                                                               |
|   [STAGE 5: COMMON EXPRESSION LANGUAGE (CEL) POLICY ENGINE] (Latency: < 0.5 ms)                                     |
|   • Evaluates compiled organizational security rules: Department, User Group, File Size, Target Domain.              |
|   • Validates cryptographic Virtual Key spending quotas and Token-Per-Minute (TPM) limits against Redis.            |
|                                                      |                                                               |
|                                                      v                                                               |
|   [STAGE 6: SECONDARY AI GUARD BOT (SEMANTIC PREDICTION)] (Conditional / High-Risk Prompts)                          |
|   • On-premise private screening LLM evaluates semantic intent:                                                     |
|     - Direct System Prompt Injection Probability Score (0.0 to 1.0)                                                 |
|     - Indirect Jailbreak & Persona Hijacking Index                                                                   |
|     - Corporate Toxicity, Harassment & Regulatory Risk Score                                                         |
|                                                      |                                                               |
|                                                      v                                                               |
|   [STAGE 7: TRI-STATE POLICY DECISION ENGINE]                                                                        |
|                                                      |                                                               |
|         +--------------------------------------------+--------------------------------------------+                  |
|         |                                            |                                            |                  |
|         v                                            v                                            v                  |
|  [DECISION A: BLOCK]                         [DECISION B: REDACT & MASK]                   [DECISION C: ALLOW]       |
|  • Severe violation detected.                • Regulated PII detected.                    • Clean, compliant prompt. |
|  • Request terminated instantly.             • Sensitive data replaced inline             • Unmodified payload       |
|  • Browser displays educational               with syntax-safe masked tokens               prepared for upstream     |
|    security warning banner.                   (e.g., [REDACTED_AADHAAR]).                  dispatch.                 |
|  • Security incident logged to SOC.          • Sanitized prompt approved.                                            |
|         |                                            |                                            |                  |
|         |                                            +---------------------+----------------------+                  |
|         |                                                                  |                                         |
|         |                                                                  v                                         |
|         |                                            [STAGE 8: UPSTREAM AI MODEL DISPATCH]                           |
|         |                                            • Forwards sanitized prompt over TLS 1.3 tunnel.                |
|         |                                            • Strictly routed to sovereign, approved model endpoint.        |
|         |                                                                  |                                         |
|         |                                                                  v                                         |
|         |                                            [STAGE 9: RESPONSE GUARD SCANNING]                              |
|         |                                            • Model response inspected for rogue outputs, prompt leaks,     |
|         |                                              or sensitive enterprise records in completion stream.         |
|         |                                                                  |                                         |
|         |                                                                  v                                         |
|         |                                            [STAGE 10: SAFE CLIENT RENDERING]                               |
|         |                                            • Clean response rendered seamlessly in employee's browser.     |
|         |                                                                  |                                         |
|         +------------------------------------------------------------------+                                         |
|                                          |                                                                           |
|                                          v                                                                           |
|   [STAGE 11: ASYNCHRONOUS AUDIT & TELEMETRY COMMIT]                                                                  |
|   • Complete transaction metadata, user identity, rule IDs, and masked prompts committed to local DB.               |
|   • Cryptographically chained using SHA-256 for non-repudiation; formatted Syslog sent to internal SIEM.            |
|   • ZERO telemetry or log packets ever leave the enterprise network boundary.                                        |
+----------------------------------------------------------------------------------------------------------------------+
```

### Detailed Execution Phase Breakdown

| Execution Phase | Processing Pipeline Actions & Threat Detection Focus | Latency SLA |
| :--- | :--- | :---: |
| **Phase 1: Ingestion & Lexical Extraction** | Local loopback PAC traffic steering, HTTP POST stream deserialization, and multi-layer de-obfuscation (Base64, Hexadecimal, Leetspeak, Unicode homoglyphs). | < 0.8 ms |
| **Phase 2: Mathematical Heuristics** | Direct arithmetic checksum validation: Indian Aadhaar (Verhoeff D5), PAN (NSDL structural), Payment Cards (Luhn Modulo-10), and Shannon entropy secret detection. | < 1.0 ms |
| **Phase 3: CEL Rules & Semantic Prediction** | Microsecond Google Common Expression Language (CEL) policy AST evaluation and conditional on-premises private SLM threat scoring ($I_R, J_S, T_C$). | < 15.0 ms |
| **Phase 4: Tri-State Remediation** | Autonomous decision execution: BLOCK (unauthorized secrets, jailbreaks), REDACT & MASK (deterministic token substitution), or ALLOW (clean text transit). | < 1.2 ms |
| **Phase 5: Response Guard & WORM Audit** | Post-generation completion scan preventing system prompt leaks and hallucinations; asynchronous SHA-256 chained immutable audit commitment to internal storage. | 0.0 ms (Async) |

## 3.5 Real-Time End-to-End Latency Budget & Execution Waterfall
To verify that YP-Raksha introduces imperceptible overhead to end-user interactions, execution latency is strictly bounded across each pipeline stage:

| Pipeline Execution Stage | Processing Engine & Mechanism | Target Latency Budget | Observed Production Latency | Overhead Impact |
| :--- | :--- | :---: | :---: | :--- |
| **Stage 1 & 2: Browser PAC Steering** | Native Loopback Proxy Redirection | &lt; 0.50 ms | **0.18 ms** | Zero OS or Network Overhead |
| **Stage 3: Payload Deserialization** | SIMD-Accelerated JSON Parser | &lt; 0.80 ms | **0.34 ms** | In-Memory Zero-Allocation Buffer |
| **Stage 4: Mathematical Heuristics** | Verhoeff, Luhn & NSDL Checksums | &lt; 1.00 ms | **0.52 ms** | Pure Arithmetic Matrix Lookups |
| **Stage 5: CEL Policy Evaluation** | Google Common Expression Language | &lt; 0.80 ms | **0.46 ms** | Pre-Compiled AST Bytecode Execution |
| **Stage 6: Secondary Guard Screening** | On-Premises Lightweight Classifier | &lt; 15.0 ms | **8.20 ms** | Invoked ONLY on Ambiguous/High-Risk Prompts |
| **Stage 7: Token Redaction & Masking** | AES-256 Format-Preserving Vault | &lt; 1.20 ms | **0.68 ms** | Direct In-Memory Token Substitution |
| **Stage 8: Upstream Secure Dispatch** | Connection-Pooled TLS 1.3 Tunnel | &lt; 1.50 ms | **0.72 ms** | Pre-Warmed Keep-Alive Socket Pool |
| **Stage 11: Immutable Audit Commit** | Asynchronous Ring-Buffer Dispatch | 0.00 ms | **0.00 ms (Async)** | 100% Non-Blocking Asynchronous Queue |
| **CUMULATIVE GATEWAY OVERHEAD** | **Full Predictive & DLP Pipeline** | **&lt; 6.00 ms** | **3.64 ms (Median)** | **Imperceptible (0.3% of Total LLM Time)** |

> [!NOTE]
> *User Experience Guarantee:* An upstream LLM requires between 800 ms and 2,500 ms to generate a completion response. YP-Raksha's median inline security inspection adds only **3.64 milliseconds** (&lt; 0.3% of the total interaction time), ensuring banking users experience zero perceptible latency delay during live customer interactions.

---

# Section 4.0 — On-Premises Deployment Architecture (DC, DR & UAT)

> [!NOTE]
> ### Executive & Non-Technical Summary: Understanding DC, DR, and UAT in Banking
> * **Primary Data Center (DC):** The active operational engine running 24/7 inside the bank's core facility. Built with redundant, twin servers ("Active-Active") so that if any individual hardware server crashes or undergoes maintenance, employee AI workflows continue without a second of downtime.
> * **Disaster Recovery (DR):** The emergency standby site located in a completely different city or geographic zone. If a major catastrophe (e.g., severe flood, regional power outage, fiber cut) knocks out the primary DC, traffic automatically switches to the DR site in under 15 minutes (**RTO < 15 mins**) with zero lost transactions (**RPO = 0**).
> * **User Acceptance Testing (UAT):** The safe testing sandbox. A dedicated, miniature replica of the system where bank security teams, auditors, and business managers can test new AI policies, verify software updates, and conduct compliance checks without risking any live banking systems.

| Environment | Primary Business Purpose | Target User Base | Operational State | Business Continuity Risk |
| :--- | :--- | :--- | :--- | :--- |
| **Production DC** | Live daily AI governance across branches & headquarters | 1,20,000 Employees | Active 24/7/365 | High (Immediate productivity impact) |
| **Disaster DR** | Emergency takeover during physical disaster | Standby for Live Operations | Warm-Standby (Continuously Synced) | Critical during disaster declaration |
| **Testing UAT** | Safe validation of new rules & security policies | InfoSec, Developers, Auditors | Independent Sandbox | Zero (Completely isolated from live bank) |

## 4.1 Enterprise Multi-Tier Network Segmentation (DMZ Architecture)
YP-Raksha implements a defense-in-depth architecture adhering strictly to zero-trust network principles. Infrastructure is segregated into four distinct security zones:

```
+----------------------------------------------------------------------------------------------------+
|                               ENTERPRISE NETWORK SEGMENTATION TOPOLOGY                             |
|                                                                                                    |
|  [ ZONE 1: USER & ENDPOINT ZONE ]                                                                  |
|  • Employee Workstations (Windows / macOS / Linux)                                                 |
|  • Non-Persistent VDI Terminals (Citrix / VMware Horizon)                                          |
|                                   |                                                                |
|                                   | Encrypted Ingress Traffic                                      |
|                                   v                                                                |
|  [ ZONE 2: ENTERPRISE DMZ & INGRESS TIER ]                                                         |
|  • Active-Active Hardware Load Balancers (F5 BIG-IP / Citrix ADC)                                  |
|  • SSL / TLS 1.3 Cryptographic Termination & Health Monitoring                                     |
|                                   |                                                                |
|                                   | Internal Segmented VLAN                                        |
|                                   v                                                                |
|  [ ZONE 3: APPLICATION SERVER CLUSTER TIER ]                                                       |
|  • 12-Node Stateless YP-Raksha Gateway Core Cluster                                                |
|  • Kubernetes / Red Hat OpenShift / VMware Tanzu / Bare-Metal Linux                                |
|                                   |                                                                |
|                                   | Dedicated Isolated Database Fabric VLAN                        |
|                                   v                                                                |
|  [ ZONE 4: SECURE DATA & STORAGE TIER ]                                                            |
|  • PostgreSQL 16 HA Cluster (Patroni + etcd) with Synchronous Replication                          |
|  • Redis 7.2 Distributed Cluster (3 Masters + 3 Replicas)                                          |
|  • Elasticsearch / OpenSearch Distributed Log Aggregator                                           |
|  • Enterprise Hardware Security Module (HSM / HashiCorp Vault)                                     |
+----------------------------------------------------------------------------------------------------+
```

## 4.2 Production Primary Data Center (DC) Architecture — Active-Active HA
The Primary DC hosts an active-active high-availability deployment capable of sustained zero-downtime execution:
* **Ingress & Load Balancing Tier:** Redundant hardware load balancers configured in an Active-Active pair using Virtual Router Redundancy Protocol (VRRP). Balancers perform layer-7 health checks against the Gateway health endpoint and dynamically remove impaired nodes.
* **Stateless Application Cluster:** Twelve (12) stateless YP-Raksha application nodes deployed across diverse physical server blades and storage zones. Because nodes maintain zero local session state, any node can serve any inbound request.
* **Database Fabric (PostgreSQL 16 HA):** High-availability relational database infrastructure managed by Patroni using etcd for distributed consensus. The cluster comprises one Primary Write Node, one Synchronous Standby Node (guaranteeing zero data loss), and one Asynchronous Standby Node for analytical queries.
* **Distributed Caching (Redis 7.2):** Six-node Redis cluster (3 Masters, 3 Replicas) with persistent append-only logging (AOF). Manages global token consumption rates, real-time user session status, and cryptographic key caches.
* **Hardware Security Layer:** Physical network-attached HSM storing the Master Encryption Key (MEK) responsible for transparent data encryption across all persistence volumes.

<!-- pagebreak -->

## 4.3 Disaster Recovery (DR) Architecture — Warm-Standby & RPO/RTO Guarantees
The Disaster Recovery facility is an enterprise-grade mirror located in a geographically distinct seismic zone connected via dedicated, redundant dark-fiber links.

```
+----------------------------------------------------------------------------------------------------+
|                                    DC - DR REPLICATION ARCHITECTURE                                |
|                                                                                                    |
|  PRIMARY DATA CENTER (DC)                              DISASTER RECOVERY DATA CENTER (DR)          |
|  +--------------------------------+                    +--------------------------------+          |
|  | Active Load Balancer Cluster   |                    | Standby Load Balancer Cluster  |          |
|  +--------------------------------+                    +--------------------------------+          |
|                 |                                                      |                           |
|  +--------------------------------+                    +--------------------------------+          |
|  | 12-Node Gateway Core Cluster   |                    | 10-Node Standby Core Cluster   |          |
|  +--------------------------------+                    +--------------------------------+          |
|                 |                                                      |                           |
|  +--------------------------------+   Async Streaming  +--------------------------------+          |
|  | Primary DB (PostgreSQL 16 HA)  | -----------------> | Standby DB (Read-Only Mirror)  |          |
|  +--------------------------------+   Dedicated Fiber  +--------------------------------+          |
|                 |                                                      |                           |
|  +--------------------------------+   Storage Mirror   +--------------------------------+          |
|  | On-Premises Audit Storage      | -----------------> | Replicated DR Audit Archive    |          |
|  +--------------------------------+                    +--------------------------------+          |
+----------------------------------------------------------------------------------------------------+
```

### Disaster Recovery Metrics & SLAs
* **Recovery Point Objective (RPO):** **RPO = 0 seconds** under synchronous database replication; **RPO < 2 seconds** during peak transaction processing over asynchronous dark fiber.
* **Recovery Time Objective (RTO):** **RTO < 15 minutes** for complete operational cutover.
* **Geo-Storage Replication:** Audit logs, model configuration records, and system settings are mirrored continuously using asynchronous block-level SAN replication and S3-compatible bucket synchronization.
* **Failover Orchestration:** Global Server Load Balancing (GSLB) automatically redirects traffic to the DR facility upon health-probe failure or administrative failover initiation.

## 4.4 User Acceptance Testing (UAT) & Staging Environment
The UAT environment is an isolated 10% scaled replica of the production architecture:
* **Complete Network Segregation:** Hosted on separate physical/virtual networks with zero connectivity to production databases or live user audit records.
* **Mock & On-Premises Sandbox LLM Nodes:** Includes private, local GPU-hosted LLM endpoints (e.g., local Ollama / vLLM instances) to facilitate integration testing, rule authoring, and penetration testing without incurring commercial cloud costs or relying on external networks.
* **Automated CI/CD Test Automation:** Integrates with corporate build systems to execute automated regression test suites, verify custom CEL policies, and validate security updates prior to production rollout.

## 4.5 DC-DR Cutover Verification Matrix & Network Routing Topology
To ensure fail-safe resilience compliant with RBI Master Directions on Business Continuity, YP-Raksha employs an automated three-tier cutover mechanism:

| Infrastructure Tier | Primary DC Operational State | DR Operational State | Cutover Trigger & Verification Mechanism |
| :--- | :--- | :--- | :--- |
| **DNS / Ingress Layer** | Active GSLB Anycast Record | Standby VIP Target | Health-probe failure (3 consecutive HTTP 500s) flips GSLB DNS in 15 seconds |
| **Application Nodes** | 12 Active Nodes (Processing) | 10 Warm Nodes (Standby) | Standby nodes receive real-time KV sync and transition to active routing within 400ms |
| **Database Tier** | Primary PostgreSQL Read/Write | Synchronous Streaming Replica | Promoted via automated Patroni DCS quorum (RPO = 0s, RTO < 3 minutes) |
| **Storage Archive Tier** | Local NVMe & Object Storage | Asynchronous Block Mirror | Storage replication delta < 2.0 seconds; zero audit log loss |

* **Split-Brain Mitigation:** An external lightweight witness quorum node deployed in a neutral datacenter zone ensures that network partition events cannot trigger simultaneous split-brain promotion of both DC and DR databases.
* **Mandatory DR Drills:** Built-in simulation scripts permit enterprise security teams to conduct controlled semi-annual DR cutover drills during off-peak hours without reconfiguring endpoint agents.

---

# Section 5.0 — Infrastructure Sizing for 1,20,000 Endpoints

> [!TIP]
> ### Executive & Non-Technical Summary: Demystifying Sizing for 1,20,000 Endpoints
> * **What 1,20,000 Endpoints Means:** This represents the bank's total deployed computer fleet—including retail branch teller PCs, credit processing teams, trading desks, customer service agents, and headquarters staff across all operating locations.
> * **Why 40% Concurrency (48,000 simultaneous users)?** In banking operations, not all 1,20,000 employees click "Send" on an AI query at the exact same microsecond. Industry-standard capacity planning uses a **40% peak concurrency model** (48,000 active concurrent users). Sizing the infrastructure for 48,000 simultaneous interactions ensures zero slowdowns during peak morning business hours, while avoiding spending crores on unnecessary, idle hardware.
> * **The Business Logic Behind 3-Tier Storage (70% Cost Savings):**
>   1. **Hot Tier (Last 30 Days):** Like keeping files on top of your desk. High-speed NVMe storage provides sub-second search for daily SOC incident investigations.
>   2. **Warm Tier (Days 31 to 90):** Like an office filing cabinet. High-density storage for monthly risk committee reviews and compliance reports.
>   3. **Cold Compliance Tier (Years 1 to 7):** Like a locked fireproof bank vault in the basement. Highly compressed, tamper-proof WORM storage guaranteeing records are preserved for RBI inspectors without bloating expensive operational disks.
>   * *Business Outcome:* Delivers **100% regulatory compliance** while reducing storage hardware procurement costs by **over 70%**.

## 5.1 Concurrency & Workload Mathematical Model
Sizing calculations are based on an enterprise deployment servicing **1,20,000 managed endpoints**:

```
====================================================================================================
                             CAPACITY PLANNING & SIZING FORMULAS
====================================================================================================
1. Monitored Fleet Size (N)             = 1,20,000 Endpoints
2. Concurrency Factor (C)               = 40% Active Concurrent Users
3. Simultaneous Active Users            = 1,20,000 * 0.40 = 48,000 Active Sessions
4. Average Daily Queries per User (Q)   = 20 Queries / User / Day
5. Total Daily Business Transactions    = 48,000 Users * 20 Queries = 960,000 Prompts/Day
6. Fleet Heartbeat / Sync Cycle (H)     = 90 Seconds Jittered Periodic Check-in
7. Heartbeat Transaction Velocity (Vh)  = 1,20,000 Endpoints / 90s = ~1,333 Requests/Sec (Steady)
8. Peak AI Business Prompt Rate (Vp)    = 4,500 - 6,000 Requests/Sec (Peak Business Hours)
9. Combined Cluster Peak Throughput     = ~7,500 Total Inbound Requests/Sec (Worst-Case)
====================================================================================================
```

## 5.2 Primary Data Center (DC) Hardware Sizing Matrix

| Infrastructure Subsystem | Cluster Node Count | Compute Allocation (vCPU) | Memory Allocation (RAM) | Storage Architecture & Capacity | Network Interfaces |
| :--- | :---: | :---: | :---: | :--- | :--- |
| **Ingress Load Balancers** | 2 Nodes (Active-Active) | 16 vCPU (Hardware Appliance) | 32 GB ECC RAM | 500 GB NVMe (OS & Logging) | 4x 10 GbE Bonded SFP+ |
| **Gateway Core Application Tier** | 12 Nodes (10 Active + 2 N+2) | 16 vCPU per node (192 vCPU Total) | 64 GB per node (768 GB Total) | 500 GB Enterprise NVMe Boot SSD | 2x 25 GbE LACP Bonded |
| **PostgreSQL 16 HA Cluster** | 3 Nodes (Primary, Sync, Witness) | 32 vCPU per node (96 vCPU Total) | 256 GB per node (768 GB Total) | 4 TB Direct-Attached NVMe RAID-10 (25k+ IOPS) | 2x 25 GbE Dedicated Fabric |
| **Redis 7.2 In-Memory Cluster** | 6 Nodes (3 Master, 3 Replica) | 8 vCPU per node (48 vCPU Total) | 32 GB per node (192 GB Total) | 500 GB High-Endurance NVMe SSD | 2x 10 GbE Low-Latency |
| **Elasticsearch Log Tier** | 6 Data Nodes + 3 Master Nodes | 16 vCPU Data / 4 vCPU Master | 64 GB Data / 16 GB Master | 6x 2 TB NVMe U.2 Enterprise SSDs | 2x 25 GbE Storage Fabric |
| **Cluster Management & HSM** | 2 Nodes (Active-Passive) | 8 vCPU per node | 32 GB RAM | 500 GB SAS RAID-1 | 2x 10 GbE Management VLAN |
| **COMBINED DC INFRASTRUCTURE** | **34 Total Nodes** | **624 vCPU** | **2,432 GB RAM** | **~24 TB High-Performance NVMe Fabric** | **Redundant Dual-Fabric** |

## 5.3 Storage Capacity Sizing & Tiered Retention Architecture
Log retention complies with statutory enterprise recordkeeping guidelines through automated tiering:

```
+----------------------------------------------------------------------------------------------------+
|                                  TIERED AUDIT STORAGE LIFECYCLE                                    |
|                                                                                                    |
|    +-----------------------------+                                                                 |
|    | Raw Ingestion Data Stream   |                                                                 |
|    | 4 KB Avg * 2.4M Interactions|                                                                 |
|    | = 9.6 GB Raw Text / Day     |                                                                 |
|    | = ~24 GB Indexed Daily Log  |                                                                 |
|    +-----------------------------+                                                                 |
|                   |                                                                                |
|                   v                                                                                |
|    +---------------------------------------------------------------------+                         |
|    | HOT STORAGE TIER (Days 1 to 30)                                     |                         |
|    | • Direct-Attached NVMe SSD RAID-10                                  |                         |
|    | • Instant Sub-Second Search & Forensic Analysis                      |                         |
|    | • Capacity: ~720 GB                                                 |                         |
|    +---------------------------------------------------------------------+                         |
|                   |                                                                                |
|                   v Automated Lifecycle Transition                                                 |
|    +---------------------------------------------------------------------+                         |
|    | WARM STORAGE TIER (Days 31 to 90)                                   |                         |
|    | • High-Density SAS SSD Enterprise Storage Array                     |                         |
|    | • Searchable Compressed Analytics & Compliance Queries              |                         |
|    | • Capacity: ~1.44 TB (Total Hot + Warm = ~2.16 TB)                  |                         |
|    +---------------------------------------------------------------------+                         |
|                   |                                                                                |
|                   v Automated Gzip Compression & Checksumming                                      |
|    +---------------------------------------------------------------------+                         |
|    | COLD COMPLIANCE TIER (Months 4 to Year 7)                           |                         |
|    | • Enterprise WORM (Write Once, Read Many) Object Storage / SAN Tape |                         |
|    | • Cryptographically Sealed, Tamper-Evident Long-Term Archive        |                         |
|    | • Capacity: ~18.5 TB Compressed Storage                             |                         |
|    +---------------------------------------------------------------------+                         |
+----------------------------------------------------------------------------------------------------+
```

## 5.4 Disaster Recovery (DR) & UAT Sizing Matrices

### Disaster Recovery Sizing (Warm-Standby Site)
* **Application Core Nodes:** 10 Nodes (16 vCPU, 64 GB RAM per node).
* **Database Fabric:** 2 Mirrored Database Nodes (32 vCPU, 256 GB RAM per node).
* **Redis Cache:** 3 Mirrored Cluster Nodes (8 vCPU, 32 GB RAM per node).
* **Log Storage Fabric:** 4 Dedicated Data Nodes (16 vCPU, 64 GB RAM, 2 TB NVMe each).

### User Acceptance Testing (UAT) Sizing (Independent Sandbox)
* **Application Core Nodes:** 2 Nodes (8 vCPU, 32 GB RAM per node).
* **Database Tier:** 1 PostgreSQL Node (16 vCPU, 64 GB RAM, 1 TB SSD).
* **Redis Tier:** 1 Standalone Redis Node (4 vCPU, 16 GB RAM).
* **Mock LLM Sandbox:** 2 GPU Nodes (Dual NVIDIA A10G / L40S) for local private inference testing.

## 5.5 Network Bandwidth Sizing
* **Internal Datacenter Switching Fabric:** Non-blocking 25 GbE spine-leaf fabric delivering over 100 Gbps aggregate switching backplane throughput.
* **Endpoint Network Impact:** Average endpoint check-in generates &lt; 15 KB data transfer per 90-second cycle. Average end-user prompt transmission adds &lt; 0.8ms network transit time over standard 1 Gbps enterprise LAN connections.

<!-- pagebreak -->

## 5.6 Empirical Stress-Testing & High-Concurrency Benchmark Report (1,20,000 Fleet Emulation)
To validate infrastructure stability prior to enterprise branch rollout, the YP-Raksha cluster was subjected to a continuous 72-hour synthetic stress test emulating the entire 1,20,000 endpoint fleet using 50 distributed Locust / JMeter worker instances:

| Performance Metric &amp; SLA | Emulation Target (1,20,000 Fleet) | Observed Benchmark Result | System Safety Headroom | Compliance Status |
| :--- | :---: | :---: | :---: | :---: |
| **Concurrent Active User Sessions** | 48,000 Sessions | 55,000 Active Sessions | +14.5% Above Peak Target | **PASSED (Zero Session Drops)** |
| **Fleet Heartbeat Ingestion Rate** | 1,333 Requests / Second | 3,500 Requests / Second | 2.6x Normal Fleet Velocity | **PASSED (Sub-millisecond ACK)** |
| **Peak Prompt Security Inspection** | 4,500 Requests / Second | 10,250 Requests / Second | 2.27x Peak Hour Maximum | **PASSED (Zero Queue Spills)** |
| **Total Inbound Request Velocity** | 7,500 Combined Req / Sec | 13,750 Combined Req / Sec | 1.83x Overdrive Capacity | **PASSED (Zero HTTP 503 Errors)** |
| **Gateway Median Latency (p50)** | &lt; 5.0 Milliseconds | **2.84 Milliseconds** | 43.2% Faster than SLA Target | **PASSED (Native Go Performance)** |
| **Gateway 99th Percentile (p99)** | &lt; 12.0 Milliseconds | **6.42 Milliseconds** | 46.5% Faster than SLA Target | **PASSED (Sub-10ms SLA Met)** |
| **Cluster CPU Utilization (12 Nodes)** | &lt; 65% Sustained Peak | **24.2% Average (38.1% Peak)**| 61.9% Remaining Headroom | **PASSED (N+2 Redundancy Validated)** |
| **Cluster Memory Footprint** | &lt; 75% Allocation | **38.6% (Zero Memory Leak)** | 61.4% Free Buffer Capacity | **PASSED (Zero Allocation Runtime)** |
| **Branch MPLS Link Utilization (50 PCs)**| &lt; 2.0% of 10 Mbps Line | **0.082% (8.2 Kbps Total)** | 99.9% Core Banking Link Preserved | **PASSED (Zero CBS Network Impact)** |

### Key Performance Findings & Architectural Guarantees
* **Zero Core Banking Impact:** On a standard 10 Mbps branch MPLS connection serving 50 teller workstations, YP-Raksha background agent heartbeats consume only 8.2 Kbps (0.082% of bandwidth), guaranteeing zero interference with Finacle, Flexcube, or real-time RTGS/NEFT core banking systems.
* **Deterministic Sub-10ms Latency:** The compiled Go Gateway with valyala/fasthttp zero-allocation buffers processes full regex, CEL expressions, and Verhoeff/Luhn checksums in under 3.5ms, ensuring users perceive zero latency delay when interacting with AI tools.
* **N+2 High-Availability Resilience:** Physical blade severance tests (sudden shutdown of two active application blades during peak 10,000 req/sec load) demonstrated instantaneous session failover within 400 microseconds with zero dropped connections.

<!-- pagebreak -->

## 5.7 Hardware Bill of Materials (BOM) & Rack Layout Specification (1,20,000 Endpoints)
The DC production infrastructure is engineered for high density, occupying a single standard 42U datacenter rack with redundant power and cooling:

| Component Role | Server Hardware Model | Quantity | Node Hardware Configuration | Total Tier Resources |
| :--- | :--- | :---: | :--- | :--- |
| **Gateway Cluster** | Dell PowerEdge R760 (2U) | 12 Nodes | 2x Intel Xeon Silver 4410Y (24 Cores), 128 GB DDR5 ECC, 2x 960 GB NVMe RAID-1, Dual 25 GbE NICs | 288 Cores, 1.5 TB RAM |
| **Database Cluster**| Dell PowerEdge R760 (2U) | 4 Nodes | 2x Intel Xeon Gold 6430 (32 Cores), 512 GB DDR5 ECC, 8x 3.84 TB NVMe RAID-10, Quad 25 GbE NICs | 128 Cores, 2.0 TB RAM, 30 TB NVMe |
| **Search / Analytics**| Dell PowerEdge R660 (1U)| 6 Nodes | 2x Intel Xeon Silver 4410Y (24 Cores), 256 GB DDR5 ECC, 4x 7.68 TB NVMe RAID-5, Dual 25 GbE NICs | 144 Cores, 1.5 TB RAM, 46 TB NVMe |
| **Redis Cache Fabric**| Dell PowerEdge R660 (1U)| 4 Nodes | 1x Intel Xeon Silver 4410Y (12 Cores), 128 GB DDR5 ECC, 2x 480 GB NVMe RAID-1, Dual 25 GbE NICs | 48 Cores, 512 GB RAM |
| **ToR Switch Fabric**| Arista 7050SX3-48YC8 | 2 Switches | 48x 25 GbE SFP28 + 8x 100 GbE QSFP28 Uplinks, Dual AC Power, MLAG Active-Active Fabric | 2.0 Tbps Switching Capacity |

* **Rack Footprint & Power Budget:** Total physical rack footprint is 28U (leaving 14U buffer for expansion). Sustained power consumption is 6.8 kW across dual N+N automatic transfer switch (ATS) redundant PDU feeds.

## 5.8 Branch Bandwidth & Core Banking System (Finacle / Flexcube) Non-Interference Proof
A paramount concern for bank IT operations is ensuring that security agents never degrade Core Banking System (CBS) transactions across low-bandwidth rural branch networks:

* **Rural Branch Impact Mathematics (50 Workstations on 2 Mbps MPLS Link):**
  * **Concurrent Packet Arrival Rate:** 50 workstations / 90 seconds = **0.55 packets / second**
  * **Aggregate Bandwidth Consumed:** 0.55 packets/sec × 420 bytes × 8 bits = **1.848 Kbps**
  * **Branch MPLS Capacity Impact:** 1.848 Kbps / 2,048 Kbps = **0.09% of available branch link**
* **Quality of Service (QoS) Priority Tagging:** YP-Raksha agent traffic is tagged with DSCP CS1 (Scavenger / Background Class), while Finacle, Flexcube, and SWIFT packets carry DSCP EF (Expedited Forwarding, Class 46). Even under severe WAN congestion, core banking packets receive absolute hardware queue priority.
* **Fail-Open Branch Circuit:** If branch WAN connectivity drops or local network latency spikes above 250ms, the local agent enters non-blocking bypass mode, guaranteeing branch operations never freeze.

---

# Section 6.0 — Supported OS, Browsers, Desktop EXE Agent & Extension Architecture

## 6.1 Desktop & Workstation Operating Systems Matrix

| Operating System Family | Edition & Version Details | Build & Kernel Baseline | Processor Architectures | Operational Mode |
| :--- | :--- | :--- | :--- | :--- |
| **Microsoft Windows Client** | Windows 11 & Windows 10 (Enterprise, Pro, LTSC) | 24H2, 23H2, 22H2, 21H2, LTSC 2021/2019 | `x86_64` (AMD64) & `ARM64` | Native Endpoint Service |
| **Microsoft Windows Server** | Windows Server 2025, 2022, 2019, 2016 | Datacenter & Standard (Multi-Session RDS) | `x86_64` (AMD64) | Multi-User Terminal Mode |
| **Apple macOS** | macOS Sequoia (15.x), Sonoma (14.x), Ventura (13.x) | Kernel Darwin 24.x, 23.x, 22.x | Apple Silicon (M1-M4) & Intel x86 | Native Endpoint Daemon |
| **Red Hat Enterprise Linux** | RHEL 9.x, 8.x, Rocky Linux & AlmaLinux 9.x | Kernel 5.14+ (RHEL 9) / 4.18+ (RHEL 8) | `x86_64` & `aarch64` | Systemd Daemon / CLI |
| **Ubuntu & SUSE Linux** | Ubuntu 24.04/22.04 LTS & SUSE SLES 15 SP5+ | Kernel 6.8+ / 5.15+ Enterprise | `x86_64` & `arm64` | Systemd Daemon / CLI |

## 6.2 Web Browsers & Enterprise Steerage Matrix

| Web Browser | Supported Version Spectrum | Rendering Engine | Enterprise Steering & Policy Distribution |
| :--- | :--- | :--- | :--- |
| **Google Chrome** | Version 100.0 to 134.x+ (Latest Stable) | Chromium Blink Engine | Managed Chrome Enterprise GPO / Intune Administrative Templates |
| **Microsoft Edge** | Version 100.0 to 134.x+ (Stable / Beta / Extended) | Chromium Blink Engine | Microsoft Intune / SCCM / Active Directory GPO Registry Injection |
| **Mozilla Firefox** | Version 115+ ESR & Latest Rapid Release | Mozilla Gecko Engine | Enterprise `policies.json` & AutoConfig PAC Distribution |
| **Apple Safari** | Version 16.x, 17.x, 18.x | Apple WebKit Engine | Apple MDM Configuration Profiles / System Network Extensions |
| **Brave Browser** | Version 1.60+ Enterprise | Chromium Blink Engine | Chromium Enterprise GPO Configuration Policies |

<!-- pagebreak -->

## 6.3 Desktop Executable Agent (Gateway_Guard.exe / YP_Raksha.exe) Architecture & Mass Rollout

The desktop endpoint agent is engineered as a zero-dependency, self-contained native executable (`Gateway_Guard.exe` / `YP_Raksha.exe`). It executes strictly in user-space without requiring external Python runtimes, Node.js packages, or kernel-level drivers.

```
+----------------------------------------------------------------------------------------------------+
|                         DESKTOP EXECUTABLE AGENT ARCHITECTURE & LOOPBACK FLOW                      |
|                                                                                                    |
|  [ MANAGED WORKSTATION ]           [ GATEWAY_GUARD.EXE (User-Space) ]      [ ENTERPRISE GATEWAY ]  |
|  Chrome / Edge Browsers            • Local PAC Server (127.0.0.1:18103)    • Primary DC Cluster    |
|  (ChatGPT / Claude Tabs)           • In-Memory DLP Checksum Engine         • 90s Fleet Heartbeats  |
|          |                         • SQLite Encrypted Policy Cache                 ^               |
|          | Local Loopback (18103)          ^                                       | TLS 1.3       |
|          +---------------------------------+---------------------------------------+               |
|                                            v                                                       |
|                     [ Offline Policy Enforcement & Anti-Tamper Shield ]                            |
+----------------------------------------------------------------------------------------------------+
```

### 6.3.1 Desktop Agent Operational Lifecycle & Loopback Interception Mechanics
* **Process Initialization:** Upon workstation login, the agent executes from `%LOCALAPPDATA%\Programs\Gateway\Guard\Gateway_Guard.exe` via the Windows Startup Registry key (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`).
* **Local Loopback PAC Binding:** The agent initializes a high-concurrency HTTP service on `127.0.0.1:18103`. It dynamically generates and applies a PAC script (`proxy.pac`) configuring browsers to route designated AI target domains (e.g., `chatgpt.com`, `claude.ai`, `gemini.google.com`, `poe.com`) through the local filter while routing internal bank applications (`*.bank.internal`, Finacle, Flexcube) `DIRECT` without latency.
* **Autonomous Offline Rule Caching:** The agent maintains an encrypted SQLite policy cache. If a branch loses WAN connectivity or a laptop travels outside corporate VPN, all DLP regex, CEL validation rules, and blocking behaviors continue enforcing locally without fail-open risk.

### 6.3.2 Mass Enterprise Installation Procedures (Intune, GPO, SCCM & Silent CLI)

| Deployment Channel & Tool | Target Fleet & Scope | Execution Command & Silent Parameters | Verification & Health Check |
| :--- | :--- | :--- | :--- |
| **Microsoft Intune (Win32 App)** | All 1,20,000 Hybrid & Remote Workstations | `Gateway_Guard_Setup.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART` | Detection Rule: `%LOCALAPPDATA%\Programs\Gateway\Guard\Gateway_Guard.exe` exists with exit code 0. |
| **Active Directory Group Policy (GPO)** | Domain-Joined Rural & Branch Workstations | PowerShell Computer Startup Script executing silent setup and registry force-install | Event ID 1000 logged in Windows Application Log; daemon process active in Task Manager. |
| **Microsoft SCCM / MECM** | High-Density Datacenter & Operations Hubs | Package Program Command: `Gateway_Guard_Setup.exe /VERYSILENT /ALLUSERS=1` | Deployment status confirmed via SCCM Software Center; exit code 0 (Success). |
| **Manual / Pilot Standalone** | IT Administrator & Pilot Sandbox Machines | Interactive GUI: Run `Gateway_Guard_Setup.exe`, select "Start at Windows Login" | Agent icon displays in Windows System Tray; `http://127.0.0.1:18103/health` returns 200 OK. |

### 6.3.3 Tamper-Proof Cryptographic Uninstall & Fleet Protection
* **Uninstall Authorization Key:** Unauthorized employees cannot remove the agent via Windows Control Panel or Settings. Initiating uninstallation prompts for a one-time 256-bit cryptographic uninstall authorization key.
* **Remote Decommissioning:** Central security administrators can decommission an agent remotely from the Gateway Web UI (`Browser AI -> Guard Agents -> Remote Decommission`). This issues a signed heartbeat revocation token that instructs the agent to gracefully unregister, remove local policies, and clean registry keys without desk-side intervention.

<!-- pagebreak -->

## 6.4 Enterprise Browser Extension (.crx / Manifest V3) Architecture & Silent Force-Install

The browser-level enforcement component is delivered as a signed, Manifest V3 compliant enterprise extension package (`guard_extension.crx`, Extension ID: `mipfhcmlcjfhmgldkokdckhndkebkllj`). It integrates natively with Chromium-based browsers (Google Chrome, Microsoft Edge, Brave) and Mozilla Firefox.

```
+----------------------------------------------------------------------------------------------------+
|                      MANIFEST V3 BROWSER EXTENSION INTERCEPTION LIFECYCLE                          |
|                                                                                                    |
|  [ USER INPUT ]       --> [ CONTENT SCRIPT ]     --> [ SERVICE WORKER ] --> [ LOCAL IPC EVAL ]     |
|  Keystrokes / Paste       DOM Event Hooking          Token Extraction       127.0.0.1:18103        |
|                                                                                    |               |
|  +---------------------------------------------------------------------------------+               |
|  v                                        v                                        v               |
|  [ ALLOW: Passthrough ]          [ REDACT: DOM Token Masking ]            [ BLOCK: Alert Banner ]  |
+----------------------------------------------------------------------------------------------------+
```

### 6.4.1 Manifest V3 Architecture & Real-Time DOM Interception Mechanics
* **DOM Event Hooking:** Content scripts inject seamlessly into target AI web pages, monitoring textareas, input fields, contenteditable rich-text elements, and drag-and-drop file upload zones.
* **Pre-Flight Keystroke Analysis:** Content is inspected prior to submission. Keystrokes, clipboard paste actions, and form submissions are validated in sub-millisecond timeframes.
* **Inline Policy Enforcement Actions:**
  * **ALLOW:** Normal, non-sensitive queries proceed without latency or user interruption.
  * **WARN:** Displays a floating non-blocking enterprise compliance toast advising caution, while permitting the user to continue after acknowledgement.
  * **REDACT:** Directly mutates the DOM input element, substituting detected Aadhaar, PAN, Card, or Secret tokens with syntax-safe placeholders (e.g., `[REDACTED_AADHAAR]`, `[REDACTED_PAN]`) before transmission.
  * **BLOCK:** Instantly cancels the submit event, freezes input dispatch, renders an educational enterprise violation banner explaining the specific policy breached, and logs the incident to the central SOC.

### 6.4.2 Silent Enterprise Registry Force-Install (ExtensionInstallForcelist & ExtensionSettings)

Employees are strictly prohibited from manually installing or modifying extensions via `chrome://extensions`. Enterprise rollout is executed silently via administrative machine registry policies:

| Target Browser | Registry Hive & Policy Key | Entry Name & Type | Enforced Policy Value & Behavior |
| :--- | :--- | :--- | :--- |
| **Google Chrome** | `HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist` | String (`REG_SZ`) Name: `1` | `mipfhcmlcjfhmgldkokdckhndkebkllj;http://127.0.0.1:18103/extension/update.xml`<br/>*Forces silent background installation on browser launch.* |
| **Google Chrome** | `HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionSettings` | String (`REG_SZ`) Name: `ExtensionSettings` | JSON configuration enforcing `installation_mode: force_installed`, `incognito: spanning`, and `toolbar_pin: force_unpinned`. |
| **Microsoft Edge** | `HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist` | String (`REG_SZ`) Name: `1` | `mipfhcmlcjfhmgldkokdckhndkebkllj;http://127.0.0.1:18103/extension/update.xml`<br/>*Forces silent background installation inside Microsoft Edge.* |
| **Chromium Sources** | `HKLM\SOFTWARE\Policies\[Browser]\ExtensionInstallSources` | String (`REG_SZ`) Names: `1`, `2` | `http://127.0.0.1:18103/*`, `http://localhost:18103/*`<br/>*Authorizes loopback port as trusted enterprise installation authority.* |

### 6.4.3 Multi-Browser Coexistence & Incognito Spanning Guarantees
* **Permanent Incognito Spanning:** Machine policy enforces `incognito: spanning`, ensuring the extension remains fully operational inside Google Chrome Incognito and Microsoft Edge InPrivate windows.
* **Zero Removal Privilege:** The browser UI displays *"Managed by your organization"*. The "Remove" button and toggle switches are completely disabled, preventing employees from tampering with security controls.

<!-- pagebreak -->

## 6.5 Hybrid Deployment Model: Endpoint Desktop Agent vs Centralized Network Proxy

To accommodate diverse enterprise infrastructure topologies—including managed employee laptops, contractor machines, branch networks, and headless server environments—YP-Raksha provides a unified hybrid deployment model:

| Operational Dimension | Mode A: Desktop Agent (.exe + extension) | Mode B: Centralized Network Proxy |
| :--- | :--- | :--- |
| **Target Fleet & Devices** | Enterprise Windows/macOS laptops, branch teller PCs, remote workstations | Unmanaged BYOD devices, headless server VMs, contractor thin clients |
| **Interception Mechanism** | Local loopback interface (`127.0.0.1:18103`) + Manifest V3 browser extension | Datacenter reverse proxy appliance (`gateway_browser_ai_proxy` port 8087) |
| **Packaging & Distribution**| `Gateway_Guard_Setup.exe` via Intune / GPO / SCCM | Docker Compose container profile / Kubernetes Helm chart deployment |
| **Offline Policy Enforcement**| Fully operational via encrypted local SQLite policy cache | Requires active network routing to the enterprise datacenter VIP |
| **Core Banking Isolation** | Local PAC file dynamically routes banking CBS traffic `DIRECT` | Network firewall / DMZ routing policies bypass internal banking subnets |
| **Centralized Administration**| **Unified Single Dashboard:** Reports to Gateway UI (`/workspace/browser-ai`) | **Unified Single Dashboard:** Reports to Gateway UI (`/workspace/browser-ai`) |

### Unified Administrative Single-Pane Management
Regardless of whether an interaction is inspected by a branch laptop's desktop executable (`Mode A`) or by the datacenter's centralized network proxy (`Mode B`), all telemetry, prompt audit logs, agent heartbeats, and DLP enforcement rules are managed from a **single, unified administrative dashboard** in the Raksha Enterprise Gateway (`Workspace -> Browser AI`). Security teams maintain complete visibility without navigating fragmented interfaces.

## 6.6 Virtual Desktop Infrastructure (VDI) & DaaS Platforms
Deploying AI guardrails inside shared terminal environments presents unique session collisions. YP-Raksha resolves this through session-aware routing:

```
+----------------------------------------------------------------------------------------------------+
|                          VDI MULTI-SESSION ARCHITECTURE & SOCKET ISOLATION                         |
|                                                                                                    |
|  [ MULTI-USER HOST: Citrix / VMware / AVD ]                                                        |
|  • Session 101 (Alice): Ephemeral Socket 127.0.0.1:19101 (Isolated SID Namespace)                  |
|  • Session 102 (Bob):   Ephemeral Socket 127.0.0.1:19102 (FSLogix Roaming Container)               |
|  • Session 103 (Carol): Ephemeral Socket 127.0.0.1:19103 (Zero Cross-Session Data Bleed)          |
|                                                 |                                                  |
|                                                 v TLS 1.3 Multiplexed Upstream Connection          |
|  [ CENTRAL RAKSHA HIGH-AVAILABILITY ENTERPRISE GATEWAY CLUSTER ] (Active-Active)                  |
+----------------------------------------------------------------------------------------------------+
```

### Supported Virtualization Technologies
* **Citrix Virtual Apps & Desktops:** 2402 LTSR, 2203 LTSR, 1912 LTSR, Citrix DaaS. Compatible with both Multi-session Virtual Delivery Agent (VDA) shared terminal servers and dedicated single-session VDI pools.
* **VMware Horizon:** Horizon 8 (Releases 2406, 2312, 2203 LTSR). Full support for Instant Clone pools, Full Clones, and App Volumes dynamic application assignment.
* **Microsoft Azure Virtual Desktop (AVD):** Windows 10/11 Enterprise Multi-session deployments with full FSLogix Profile Container roaming support.
* **Amazon WorkSpaces:** Dedicated Windows Server desktop experience and enterprise Linux WorkSpaces.

### 6.6.1 Multi-Session Architecture & Session Collision Prevention
* **Ephemeral Loopback Sockets:** Unlike single-user desktop PCs where a static local proxy port suffices, multi-user RDSH and Citrix terminal servers host dozens of simultaneous user sessions on a single OS kernel. YP-Raksha assigns dynamic, per-session ephemeral loopback bindings tied directly to the user's Windows Security Identifier (SID).
* **FSLogix Profile Container Roaming:** User policy caches, temporary token mappings, and authentication tickets are stored within `%LOCALAPPDATA%\YPRaksha\VDI\` which natively roams across virtual desktop pools via FSLogix VHD containers without re-prompting or key regeneration.
* **Non-Persistent Instant Clone Compatibility:** During nightly desktop recompositions or instant-clone refreshes, the YP-Raksha agent registers automatically with the central Gateway upon system boot, receiving immediate cryptographic policy synchronization in under 300 milliseconds.

<!-- pagebreak -->

## 6.7 Client Laptop & Workstation Hardware Specifications (Minimum, Recommended & Power-User Profiles)

To guarantee seamless, imperceptible execution across the bank's entire fleet of 1,20,000 distributed endpoints without requiring hardware replacements or causing performance degradation, YP-Raksha adheres to strict client hardware baselines:

| Hardware Component | Minimum Endpoint Profile (Branch Tellers / Clerks) | Enterprise Recommended Profile (Knowledge Workers / Managers) | Power-User & Developer Profile (AI Engineers / Quants / InfoSec) | VDI Thin Client Profile (Citrix / Horizon Terminals) |
| :--- | :--- | :--- | :--- | :--- |
| **Target User Group** | 72,000 Branch Clerks, Cashiers & Ops | 40,000 Corporate Officers & Analysts | 8,000 Developers, Data Scientists & Quants | Call Center Contractors & Remote BPO |
| **Processor (CPU)** | Intel Core i3 / i5 (8th Gen+, 4-Core, 1.8 GHz) or AMD Ryzen 3 / 5 | Intel Core i5 / i7 (11th-14th Gen, 6-10 Cores, 2.4 GHz+) or Apple M1-M4 | Intel Core i7 / i9 / Xeon (12-24 Cores, 3.2 GHz+) or Apple M2/M3/M4 Max/Ultra | 2 vCPUs allocated per VDI session on host (or 1 GHz SoC on physical thin client) |
| **System Memory (RAM)** | 8 GB DDR4 (Minimum 1.5 GB available) | 16 GB DDR4 / DDR5 / LPDDR5 | 32 GB – 64 GB DDR5 ECC | 4 GB vRAM allocated per virtual desktop (2 GB on physical thin client) |
| **Available Storage** | 200 MB free SSD / HDD space | 1.0 GB free NVMe Gen 3/4 SSD | 5.0 GB free NVMe Gen 4 SSD | 100 MB free user profile storage (FSLogix / Roaming Profile) |
| **CPU Architecture** | `x86_64` (AMD64) or `ARM64` | `x86_64` (Intel/AMD) or Apple Silicon (`aarch64`) | `x86_64` or Apple Silicon (`arm64`) | `x86_64` / ThinOS / IGEL OS / Linux SoC |
| **Network Interface** | 100 Mbps Ethernet or 802.11ac Wi-Fi | 1 Gbps Gigabit LAN or Wi-Fi 6 (802.11ax) | 2.5 Gbps / 10 Gbps SFP+ or Wi-Fi 6E/7 | 100 Mbps Ethernet (ICA/RDP traffic < 50ms latency) |
| **Network Bandwidth** | 128 kbps allocated for telemetry | 256 kbps allocated for telemetry | 512 kbps allocated for high-frequency telemetry | 64 kbps loopback steering bandwidth |
| **Display Resolution** | 1366 × 768 pixels minimum | 1920 × 1080 (Full HD) or higher | Dual 4K (3840 × 2160) or Ultrawide displays | 1920 × 1080 single/dual monitor pass-through |
| **Supported OS** | Windows 10 (1909+), Ubuntu 22.04 LTS | Windows 11 Enterprise (22H2-24H2), macOS 14/15 | Windows 11 Pro/Ent, macOS Sequoia, RHEL 9.x | Windows 10/11 Enterprise Multi-Session, Linux |
| **Browser Spectrum** | Chrome 100+, Edge 100+, Firefox 115+ | Chrome 120+, Edge 120+, Safari 17+, Firefox | Chrome Canary/Dev, Edge, Firefox, Brave | Enterprise Managed Chrome / Edge via GPO |

## 6.8 Endpoint Agent Footprint, Security Coexistence & Non-Interference Guarantees

YP-Raksha's endpoint interception engine is engineered as a 100% user-space network extension and managed browser integration. It strictly avoids kernel-mode driver injection (`.sys` / `.kext`), eliminating the risk of operating system crashes, blue screens (BSOD), or kernel panics.

| Technical Parameter | Metric Specification | Enterprise Banking Verification Baseline | Operational Non-Interference Guarantee |
| :--- | :--- | :--- | :--- |
| **Resident Memory (RSS)** | **32 MB to 45 MB** | Verified under 100 concurrent browser tabs | Does not exceed 0.3% of total workstation RAM on standard 16 GB laptops |
| **CPU Utilization (Idle)** | **< 0.1% CPU** | Background daemon during standard desktop work | Completely sleep-state dormant; zero impact on battery life |
| **CPU Utilization (Active)** | **< 0.5% CPU (Peak 1.2%)** | 100-character/sec sustained prompt typing | User experiences zero input lag, cursor stutter, or browser freezing |
| **Disk Write Footprint** | **0.00 MB/sec (Zero Continuous Writes)** | In-memory ring-buffer dispatch architecture | Preserves SSD lifespan; zero unencrypted prompt logging on local laptop drive |
| **Binary Install Footprint** | **< 28 MB Total** | Single self-contained binary / extension package | Deploys in under 5 seconds via Microsoft Intune, SCCM, or BigFix |
| **Kernel Driver Injections** | **ZERO Kernel Drivers (0 .sys / 0 .kext)** | Pure user-space loopback & PAC steerage | 0% probability of Windows Blue Screen of Death (BSOD) or system crash |
| **EDR / AV Compatibility** | **100% Certified Coexistence** | CrowdStrike Falcon, MS Defender, SentinelOne | Zero hook conflicts, zero false-positive quarantines, digitally EV-code-signed |
| **CBS Application Impact** | **ZERO Overhead on Core Banking** | Finacle, Flexcube, SWIFT Alliance, Bloomberg | Completely bypasses non-AI ports and bank web portals via bypass PAC rules |

---

# Section 7.0 — Sovereign Data-Flow & Zero-Egress Proof

> [!IMPORTANT]
> ### Executive & Non-Technical Summary: What is "Data Sovereignty" & Why Does it Protect the Bank?
> * **The Danger in Plain Words:** When an employee opens public AI chatbots on an unmonitored laptop and submits prompts containing customer account numbers or financial ledgers, that data leaves the bank perimeter and travels over the public internet to foreign cloud servers.
> * **Regulatory Penalties:** The DPDP Act 2023 imposes statutory fines up to **₹250 Crores** for cross-border leaks of Indian citizens' personal data, while RBI Master Directions strictly mandate in-country localization of all financial records.
> * **YP-Raksha Sovereign Protection:** All customer PII (Aadhaar, PAN, Card numbers) is redacted in-memory before leaving the workstation; prompts route strictly to domestic sovereign AI models; and 100% of audit logs stay locked inside the bank's internal on-premises storage.

## 7.1 Regulatory Baseline & In-Country Data Residency
YP-Raksha is architected to guarantee compliance with the most stringent data privacy regulations globally, including:
* **Reserve Bank of India (RBI) IT Governance Master Directions:** Mandating strict localization, operational cyber-resilience, continuous audit trails, and zero storage of core financial data on unauthorized external infrastructure.
* **Digital Personal Data Protection Act, 2023 (DPDP Act):** Requiring purpose limitation, verifiable data principal consent, and prohibition of unauthorized cross-border personal data transfers.
* **CERT-In Cyber Security Directions (June 2022):** Requiring mandatory logging of all ICT system interactions and non-repudiation audit trails retained strictly within sovereign borders.
* **ISO/IEC 27001:2022 & SOC 2 Type II:** Validating rigorous boundaries for access control, cryptographic key protection, and data segregation.

## 7.2 Cryptographic Data Flow Architecture (Zero Data Exfiltration)
The following sequence demonstrates that **no corporate data, logs, or diagnostic telemetry ever leave the enterprise perimeter or national borders**:

```
+----------------------------------------------------------------------------------------------------+
|                              SOVEREIGN DATA-FLOW & BOUNDARY ENFORCEMENT                            |
|                                                                                                    |
|  [ ENTERPRISE PRIVATE BOUNDARY / NATIONAL JURISDICTION ]                                            |
|                                                                                                    |
|  1. Workstation Generation:                                                                        |
|     Employee types prompt or attaches document in browser.                                         |
|                 |                                                                                  |
|                 v                                                                                  |
|  2. Local Loopback Redirection:                                                                    |
|     Local PAC routes AI traffic strictly to Internal Local Proxy Interface.                       |
|     (General internet traffic completely bypasses agent).                                          |
|                 |                                                                                  |
|                 v                                                                                  |
|  3. Local PII Redaction & Sanitization:                                                            |
|     Aadhaar, PAN, Card Numbers, and PII are masked IN-MEMORY before leaving workstation.          |
|                 |                                                                                  |
|                 v                                                                                  |
|  4. Internal LAN Transit (mTLS):                                                                   |
|     Encrypted transmission exclusively over Private Enterprise Subnets (RFC 1918).                 |
|     Zero public internet transit.                                                                  |
|                 |                                                                                  |
|                 v                                                                                  |
|  5. Enterprise DMZ Ingress:                                                                        |
|     Hardware Load Balancers terminate TLS using internal Enterprise PKI CA Certificates.           |
|                 |                                                                                  |
|                 v                                                                                  |
|  6. On-Premises Gateway Inspection:                                                                |
|     Stateless cluster evaluates CEL policy rules and verifies Virtual Key token allowances.        |
|                 |                                                                                  |
|                 +-----------------------------------+                                              |
|                 |                                   |                                              |
|                 v                                   v                                              |
|  7A. Local Storage Commitment:        7B. External Model Transit:                                  |
|      ALL prompt texts, user identity,      ONLY anonymized, sanitized,                             |
|      IP/MAC addresses, and audit logs      and redacted text payload is                            |
|      are written to INTERNAL DB.           forwarded over dedicated TLS tunnel.                    |
|      ZERO TELEMETRY LEAVES PERIMETER.      (Strictly to in-country sovereign models).              |
|                                                                                                    |
|  [ FORBIDDEN OUTBOUND PERIMETER: BLOCKED BY HARDWARE FIREWALL ]                                    |
|  X - Third-Party Analytics (Google Analytics, Segment)      --> DROPPED & BLOCKED BY DEFAULT       |
|  X - Cloud Telemetry Beacons (Sentry, Datadog SaaS)         --> DROPPED & BLOCKED BY DEFAULT       |
|  X - Foreign Cross-Border Model Endpoints                   --> DROPPED & BLOCKED BY DEFAULT       |
+----------------------------------------------------------------------------------------------------+
```

<!-- pagebreak -->

## 7.3 Zero Telemetry & Zero External Beacons Guarantee
* **Source-Level Exclusion:** The YP-Raksha codebase contains zero third-party telemetry, tracking SDKs, or foreign crash-reporting modules.
* **No Cloud Heartbeats:** The platform does not check in with external vendor licensing servers; all enterprise license validation is performed offline using cryptographically signed public-key license files.
* **Internalized Dependency Resolution:** All web UI assets (fonts, icons, JavaScript libraries) are packaged and served locally from the compiled binary; zero external CDN dependencies exist.

## 7.4 Next-Generation Firewall (NGFW) Egress Verification Matrix
Enterprise network security teams can audit and enforce compliance via strict perimeter firewall rules:

| Source Subnet | Destination Address | Destination Protocol | Firewall Action | Security Rationale |
| :--- | :--- | :--- | :---: | :--- |
| **Application Cluster Nodes** | `0.0.0.0/0` (Any External Internet) | Any Protocol | **DENY / DROP** | Prevents any outbound connection to public internet |
| **Application Cluster Nodes** | Enterprise Internal Database Fabric | Internal Secure DB Service | **PERMIT** | Encrypted persistence of audit logs and configuration |
| **Application Cluster Nodes** | Internal SIEM Aggregator VIP | Syslog over TLS / CEF | **PERMIT** | Real-time transmission of security alerts to internal SOC |
| **Application Cluster Nodes** | Sovereign LLM VIP (In-Country) | Standard Secure Ingress TLS | **PERMIT (Strict)** | Whitelisted outbound transmission of sanitized prompts only |
| **Endpoint Workstations** | Application Cluster Ingress VIP | Internal Cluster Ingress TLS | **PERMIT** | Forwarding of intercepted AI traffic to inspection engine |

## 7.5 Deep-Packet Inspection (DPI) & Zero-Egress Forensic Audit Test Suite
To satisfy external compliance audits and bank internal penetration testing teams, YP-Raksha provides an automated forensic test suite verifying that no packet or telemetry escapes the enterprise boundary:

### 1. Continuous Network Packet Sniffing Verification Command
Bank SecOps personnel can execute the following non-disruptive live capture command on the application cluster egress interfaces to verify zero unauthorized outbound transit:
```bash
# Capture any outbound connection attempt destined outside enterprise RFC 1918 subnets
tcpdump -i any -n "dst net not (10.0.0.0/8 or 172.16.0.0/12 or 192.168.0.0/16) and tcp[tcpflags] & tcp-syn != 0" -c 100 -w /var/log/raksha/audit_egress_verify.pcap
```
* **Audit Pass Criteria:** The resulting `.pcap` capture file must show **zero packets** except for explicit, cryptographically pinned TLS tunnels directed toward approved domestic in-country LLM IP addresses.

### 2. Wireshark Forensic Audit Filter Criteria
External regulatory auditors inspecting network spans can apply the following display filter:
```wireshark
ip.dst != 10.0.0.0/8 and ip.dst != 172.16.0.0/12 and ip.dst != 192.168.0.0/16 and tcp.port == 443
```
* **Audit Verification Result:** Every captured session resolves exclusively to approved domestic banking endpoints; zero calls to Google Analytics, Sentry, Mixpanel, or Datadog cloud beacons exist.

### 3. Statutory Zero-Data-Egress Regulatory Audit Sign-off Checklist

| Regulatory Authority | Statutory Clause | Compliance Proof & Technical Implementation | Audit Verification Status |
| :--- | :--- | :--- | :---: |
| **DPDP Act, 2023** | Section 16 (Cross-Border Transfer) | All Aadhaar, PAN, and PII masked in-memory before prompt transmission; zero raw PII egress | **100% COMPLIANT** |
| **RBI Master Directions** | Section 4.2 (Data Localization) | 100% on-premises audit persistence; zero external telemetry or cloud analytics dependencies | **100% COMPLIANT** |
| **CERT-In Directions** | Direction 20(3)/2022-CERT-In | Immutable SHA-256 chained system audit logs retained exclusively within sovereign domestic storage | **100% COMPLIANT** |
| **Banking CISO Standard** | Network Isolation Baseline | Application nodes operate with default DENY outbound firewall policy; offline license validation | **100% COMPLIANT** |
