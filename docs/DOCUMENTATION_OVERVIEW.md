# UnifAI / Gateway Enterprise AI Platform
# Complete Documentation Suite & Master Directory Index

**Classification:** Enterprise System Documentation  
**Suite Release:** Version 2.4.0 (Baseline commit `16d4c3e`)  
**Storage Directory:** `/pdf/` & `/docs/pdf/`  
**Generated Date:** October 2026  

---

## 1. Documentation Suite Overview

This documentation folder (`pdf/`) contains the complete technical, operational, architectural, and security manuals for the **UnifAI Enterprise AI Gateway** and **Gateway Guard** endpoint protection suite.

All documentation is provided in both **Markdown (`.md`)** format for source tracking and **Adobe PDF (`.pdf`)** format for distribution, printing, and archiving.

---

## 2. Document Catalog & Coverage

| # | Document File | Formats Available | Primary Audience | Description & Coverage |
|---|---|---|---|---|
| **1** | **Technical Documentation** | `01_TECHNICAL_DOCUMENTATION.md`<br>`01_Technical_Documentation.pdf` | Architects, Lead Developers, Security Engineers | Complete system overview, dual-plane architecture, Go 1.26+ `fasthttp` gateway engine, `core/providers` (~28 AI vendors), universal schemas, MCP client, key selectors, `framework/configstore`, `framework/logstore`, RBAC (33 resources × 6 operations, section grants), plugin pipeline, HTTP routes, ConfigStore & Browser AI tables, Browser Guard agent (`apps/browser-guard/agent`), mitmproxy addon parts, React 19 UI architecture, security model, and developer test workflows. |
| **2** | **Server Implementation Guide** | `02_SERVER_IMPLEMENTATION_GUIDE.md`<br>`02_Server_Implementation_Guide.pdf` | DevOps, SRE, Systems Administrators | Deployment specifications, system requirements (CPU, RAM, Disk, Kernel sysctl limits), complete `.env` configuration breakdown, `config.json` reference, multi-stage Docker build, volumes, network proxy profile, PostgreSQL database setup, Nginx reverse proxy configuration (SSL, HTTP/2, WebSocket upgrades, unbuffered SSE streaming), Kubernetes Helm chart (`deploy/helm/unifai`), operations (start/stop/restart, logs, health, backups, rolling upgrades, HA), and troubleshooting matrix. |
| **3** | **Admin Console Guide** | `03_ADMIN_CONSOLE_GUIDE.md`<br>`03_Admin_Console_Guide.pdf` | Platform Admins, InfoSec, Workspace Managers | Initial onboarding checklist, roles (`admin`, `sub_admin`, `user`, custom roles), User lifecycle management (approvals, rejections, SCIM), Access Profiles, Virtual Keys (budgets, RPM/TPM rate limits), Model Providers and catalog, Complexity Router, routing rules, circuit breakers, Guardrails vs Guard Rules, Browser AI control plane (Target Websites, Guard Rules, interaction controls, Fleet configuration, Guard Agents, Setup & packaging, company uninstall keys), and Workspace Config views. |
| **4** | **End-User & Developer Guide** | `04_USER_GUIDE.md`<br>`04_User_Guide.pdf` | Employees, Developers, Data Scientists | Platform introduction in plain language, account registration, email verification, login, password recovery, navigating the Workspace UI (Prompt Repository, Skills Repository, Model Playground, Personal Observability), Developer API integration (Virtual Keys, `curl`, Python OpenAI SDK, TypeScript, LangChain, Cursor IDE), Desktop Browser Guard employee guide (Windows & macOS installation, in-browser DLP alerts: Block, Redact, Warn, and privacy protections). |
| **5** | **Database Configuration Guide** | `05_DATABASE_CONFIGURATION_GUIDE.md`<br>`05_Database_Configuration_Guide.pdf` | DBAs, DevOps, Infrastructure Engineers | PostgreSQL 16 installation on Ubuntu/Debian/RHEL/Docker, UTF-8 schemas and user provisioning SQL (`agent_unify`, `gateway_new`), `pg_hba.conf` network security (SCRAM-SHA-256), SSL/TLS database encryption (generating `server.key` & `server.crt`, `DB_SSL_MODE=require`), GORM table reference, PgBouncer connection pooling setup (`pgbouncer.ini`), production performance tuning parameters for `postgresql.conf`, automated daily backup bash script with 30-day retention and crontab scheduling, and disaster recovery restore procedure. |
| **6** | **Docker & SSL/TLS Configuration Guide** | `06_DOCKER_AND_SSL_CONFIGURATION_GUIDE.md`<br>`06_Docker_and_SSL_Configuration_Guide.pdf` | DevOps, Cloud Engineers, SREs, SecOps | Multi-container Docker Compose architecture, exhaustive Docker command reference (`up`, `down`, `logs`, `exec`, `build`, `prune`), Public HTTPS with Let's Encrypt / Certbot (standalone, webroot, nginx plugin, auto-renew crontab), OpenSSL self-signed certificate generation (4096-bit RSA, SAN extensions), Browser Guard Root CA generation (`gateway-ca.crt`, `gateway-ca.key`, `mitmproxy-ca.pem`), client trust store installation (Windows `certutil`, macOS `security`, Linux `update-ca-certificates`, Firefox NSS), Nginx reverse proxy configuration, and SSL handshake diagnostic commands. |

---

## 3. Quick Reference: Critical System Ports

- **Public Web / HTTPS:** `443` (Nginx / Caddy Reverse Proxy)
- **Plain HTTP Redirect:** `80` (Certbot ACME challenge & HTTPS redirect)
- **UnifAI Go Backend Core:** `6000` (Internal localhost bind: `127.0.0.1:6000`)
- **Docker Network Proxy:** `18182` (Optional compose profile `network-proxy`)
- **PostgreSQL Database:** `5432` (or `6432` with PgBouncer)
- **Local Ollama LLM Service:** `11434` (Internal service for AI Guard Bot)
- **Browser Guard Local MitM Proxy:** `18103` (Bound to client laptop loopback `127.0.0.1`)
- **Browser Guard Local PAC Server:** `18195` (Bound to client laptop loopback `127.0.0.1`)

---
*UnifAI Documentation Suite — Built for Enterprise AI Governance & Security.*
