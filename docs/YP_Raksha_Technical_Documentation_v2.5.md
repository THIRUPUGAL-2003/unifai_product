# YES PANCHI — YP-RAKSHA
## Real-Time AI Knowledge Screening & Hazard Audit
### Technical Documentation — Version 2.5

---

### Document Information & Metadata

| Metadata Field | Specification Details |
| :--- | :--- |
| **Product Name** | YP-Raksha (YesPanchi Raksha AI Security Platform) |
| **Document Type** | Enterprise Architecture & Technical Specification Manual |
| **System Version** | Version 2.5 (Gateway Core Go 1.26.4 / Python 3.11+ mitmproxy Guard Agent 1.1.20) |
| **Security Classification** | Confidential — Enterprise Internal & Partner Technical Audit |
| **Target Audience** | Chief Information Security Officers (CISO), Enterprise Architects, SecOps, InfoSec Compliance Auditors |
| **Publishing Organization** | YesPanchi Group of Companies — Cyber Defense & Enterprise AI Division |
| **Release Date** | October 2026 |
| **Document Status** | Approved Enterprise Baseline Technical Specification |

---

## Table of Contents

| Section | Module & Topic Specification | Detailed Engineering Subsystems & Topics | Page |
| :---: | :--- | :--- | :---: |
| **1.0** | **Product Overview & Mission** | 1.1 Intro to Raksha \| 1.2 Purpose & Objectives \| 1.3 Key Features \| 1.4 Use Cases & Benefits \| 1.5 Scope & Limitations | **Page 3 - 4** |
| **2.0** | **System Architecture & High-Level Design** | 2.1 Dual Data Planes \| 2.2 Ingestion & Proxy Pipelines \| 2.3 Persistence & Multi-Storage Fabric \| 2.4 Cryptographic Boundaries | **Page 5** |
| **3.0** | **Browser Guard & Endpoint Fleet Mechanics** | 3.1 Python mitmproxy Agent \| 3.2 PAC Steering (18103/18195) \| 3.3 Dynamic Content Parsing \| 3.4 Triple Remediation Pipeline | **Page 6** |
| **4.0** | **Unified AI Gateway & Provider Fabric** | 4.1 OpenAI-Compatible `/v1/*` Modality Engine \| 4.2 28+ Provider Adapters \| 4.3 Failover & Load Balancing \| 4.4 Model Context Protocol | **Page 7** |
| **5.0** | **Governance, Budgets & Virtual Keys** | 5.1 Cryptographic Virtual Keys \| 5.2 Multi-Tier Budget Quotas \| 5.3 TPM/RPM Throttling \| 5.4 Chargebacks & Cost Attribution | **Page 8** |
| **6.0** | **Security Enforcement & Hazard Screening** | 6.1 Inline PII & Regulated Data Masking \| 6.2 CEL Policy Rules \| 6.3 Secondary AI Guard Bot \| 6.4 Jailbreak & Prompt Injection Defense | **Page 9** |
| **7.0** | **Identity Federation & Enterprise RBAC** | 7.1 Multi-Tenant RBAC Matrix \| 7.2 SCIM 2.0 Inbound Sync (Okta/Entra) \| 7.3 Active Directory LDAP/LDAPS \| 7.4 Session Security Policies | **Page 10** |
| **8.0** | **Observability, Telemetry & Audit Trails** | 8.1 Real-Time Prompt/Search Logs \| 8.2 OTel & Prometheus Exporters \| 8.3 Immutable Audit Trails \| 8.4 CISO Analytics Dashboard | **Page 11** |
| **9.0** | **Enterprise Deployment & Infrastructure** | 9.1 Containerized Stack (Docker & Helm) \| 9.2 Endpoint Packages (Inno Setup / Mac) \| 9.3 Secrets Hardening \| 9.4 High Availability | **Page 12** |
| **10.0** | **Technical Glossary & Reference Matrix** | 10.1 Key Terminology & Definitions \| 10.2 Network Port & Protocol Matrix \| 10.3 Failover Protocol & Diagnostic Runbook | **Page 13** |

> **Architectural Scope Note:** This technical specification is organized modularly to provide complete verification depth across all operational layers of YP-Raksha v2.5. Each section delivers verified implementation facts from source code, operational binaries, and configuration schemas, serving as the official technical baseline for deployment and enterprise infosec compliance.

---

# 1. Product Overview

## 1.1 Introduction to Raksha AI Security

Raksha AI Security is an enterprise-grade AI security and governance solution developed by YesPanchi to help organizations monitor, control, and secure the use of Artificial Intelligence (AI) applications and services.

As organizations increasingly adopt AI tools such as ChatGPT, Claude, Gemini, DeepSeek, and other AI-powered platforms, protecting sensitive business information becomes essential. Employees may unintentionally submit confidential data, customer information, internal documents, or other sensitive content to AI applications.

Raksha AI Security helps address these risks by providing centralized monitoring, security policy enforcement, data loss prevention (DLP), and AI usage governance. It enables organizations to establish security controls over AI interactions while maintaining visibility into how AI applications are used across the enterprise.

The solution is designed to support enterprise security requirements, including controlled AI access, sensitive-data protection, risk monitoring, and administrative oversight. It can be deployed according to an organization's infrastructure and security requirements, including on-premises deployment where supported and configured.

## 1.2 Product Purpose and Objectives

The primary purpose of Raksha AI Security is to enable organizations to use AI technologies securely while reducing the risk of confidential information exposure, unauthorized AI usage, and policy violations.

The key objectives are:

1. **AI Usage Monitoring:** Provide visibility into AI application usage and relevant interactions across monitored endpoints and supported platforms.

2. **Data Loss Prevention (DLP):** Identify and help prevent sensitive information, such as confidential business data and personally identifiable information, from being submitted to AI applications in violation of configured policies.

3. **Security Policy Enforcement:** Allow administrators to define and enforce organizational policies for AI application access, usage, and data protection.

4. **Risk Detection and Prevention:** Identify potentially risky AI interactions and apply configured security actions, such as warnings, blocking, or logging, where supported.

5. **Centralized Administration:** Provide a centralized interface for managing security configurations, monitoring activity, and reviewing relevant security events.

6. **AI Governance and Accountability:** Support organizations in establishing consistent controls, maintaining audit visibility, and improving oversight of enterprise AI usage.

7. **Enterprise Data Protection:** Support the implementation of organizational security and data-residency requirements through appropriate deployment architecture and configuration.

## 1.3 Key Features and Capabilities

Raksha AI Security provides a set of security and governance capabilities for enterprise AI environments.

### 1. AI Application Monitoring
Monitors supported AI applications and interactions through configured monitoring components, providing visibility into relevant AI usage and security events.

### 2. Real-Time Data Loss Prevention
Evaluates monitored content against configured security rules to identify sensitive information and apply appropriate protective actions.

### 3. Prompt Inspection and Risk Detection
Analyzes AI prompts and supported interaction data to identify potential security risks, sensitive information, and policy violations.

### 4. Policy and Rule Management
Enables administrators to configure security rules, monitoring policies, risk thresholds, and enforcement settings according to organizational requirements.

### 5. Website and Application Controls
Supports configured website and AI application monitoring and access-control policies for supported environments.

### 6. Security Logs and Audit Visibility
Provides relevant logs and event information to help administrators investigate policy violations, review AI usage, and understand detected risks.

### 7. Centralized Dashboard
Provides a consolidated view of available monitoring data, security events, risk indicators, and system activity to support administrative oversight.

### 8. Enterprise Access and Governance
Supports controlled administration of users, teams, customers, business units, and virtual keys, where applicable to the deployed Gateway configuration.

### 9. AI Gateway Governance
Supports centralized management of AI model providers, routing configurations, usage limits, and budget controls within the Gateway component.

### 10. Enterprise Deployment Support
Supports deployment and configuration according to organizational infrastructure requirements, including on-premises environments where supported.

*Note: Actual feature availability, enforcement behavior, supported applications, and deployment options depend on the installed product version, enabled modules, and validated configuration.*

## 1.4 Use Cases and Business Benefits

Raksha AI Security can support a range of enterprise security and governance scenarios.

### Use Case 1: Preventing Confidential Data Exposure
Employees may accidentally enter confidential company information into public AI tools. Raksha can inspect monitored interactions against configured policies and apply the appropriate security response.

**Business benefit:** Reduces the risk of unauthorized disclosure of sensitive business information.

### Use Case 2: Enterprise AI Usage Monitoring
Security and IT teams need visibility into how employees use supported AI applications.

**Business benefit:** Improves visibility, security oversight, and understanding of AI usage across the organization.

### Use Case 3: Enforcing Organizational AI Policies
Organizations may restrict the use of specific AI applications or prohibit certain categories of sensitive information from being submitted to AI services.

**Business benefit:** Helps enforce consistent AI usage policies and reduce policy violations.

### Use Case 4: Security Incident Investigation
When a potential AI-related security incident occurs, administrators need relevant event logs and monitoring information to investigate the incident.

**Business benefit:** Supports incident analysis, security reviews, and audit preparation.

### Use Case 5: Enterprise AI Gateway Governance
Organizations using multiple AI providers and models may need centralized routing, access management, and budget controls.

**Business benefit:** Helps improve governance, operational visibility, and control over configured AI usage and expenditure.

### Use Case 6: Regulated and Sensitive Environments
Organizations such as banks and financial institutions may require strict controls over sensitive information, infrastructure access, and data residency.

**Business benefit:** Supports the implementation of internal security and compliance controls when the solution is deployed and validated against the organization's requirements.

### Overall Business Benefits

- Improved visibility into supported AI usage.
- Reduced risk of sensitive-data exposure.
- More consistent enforcement of AI security policies.
- Centralized monitoring and administrative control.
- Improved support for security investigations and audits.
- Better governance of AI providers, models, and configured budgets.
- Support for enterprise security and data-residency requirements.

## 1.5 Product Scope and Limitations

### Product Scope

Raksha AI Security is intended to provide security monitoring, data protection, and governance capabilities for supported AI applications and configured enterprise AI environments.

The product scope includes:

- Monitoring supported AI applications and interactions.
- Inspecting monitored prompts and content against configured security rules.
- Detecting potential sensitive-data exposure and policy violations.
- Applying supported security actions according to configured policies.
- Maintaining relevant security logs and monitoring information.
- Providing administrative controls and dashboard visibility.
- Supporting configured AI Gateway governance, including provider, model, routing, and budget management.
- Supporting enterprise deployment architectures based on validated product capabilities and infrastructure requirements.

### Product Limitations

1. **Supported Applications:** Monitoring and enforcement depend on the AI applications, websites, browsers, operating systems, and integration methods supported by the deployed version.

2. **Monitoring Coverage:** Raksha can inspect only the interactions and data accessible to its configured monitoring components. It should not be assumed to monitor every application or every communication channel automatically.

3. **Detection Accuracy:** Detection performance depends on the configured rules, inspection methods, supported content types, and detection models. False positives and false negatives may occur.

4. **Endpoint Configuration:** Monitoring and enforcement may depend on successful installation, configuration, permissions, and operation of the relevant endpoint agent, extension, or other monitoring component.

5. **Network and Security Restrictions:** Firewalls, proxies, antivirus software, endpoint protection tools, and organizational security policies may affect installation, connectivity, or operation.

6. **Deployment Requirements:** On-premises deployment, high availability, disaster recovery, endpoint scalability, and offline operation must be validated against the actual product architecture and deployment configuration.

7. **Data Residency:** Keeping all data, logs, and telemetry within a bank's infrastructure or within India requires appropriate architecture, network restrictions, storage configuration, and verification. This must be confirmed through deployment testing and network-flow validation.

8. **Third-Party AI Services:** When users access external AI services, information submitted to those services may be processed according to the respective provider's policies and infrastructure. Raksha monitoring alone does not guarantee that external providers will not retain or process submitted information.

9. **Compliance Responsibility:** Raksha can support organizational security and governance processes but does not independently guarantee regulatory compliance or certification.

10. **Feature Availability:** Certain features may depend on the product edition, enabled modules, software version, licenses, or administrative configuration.

The deployment team should validate the supported environments, security controls, data flows, performance, and recovery procedures before production rollout.
