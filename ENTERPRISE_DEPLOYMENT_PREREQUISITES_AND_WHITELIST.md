# YP-Raksha Enterprise Browser AI Governance & DLP Platform
## System Prerequisites, Network Whitelist, Docker & Enterprise Deployment Guide

> **Document Version:** 1.5.0  
> **Audience:** Enterprise IT Administrators, Information Security (InfoSec), DevOps Engineers, and Network Operations.  
> **Classification:** Enterprise Client Technical Documentation  

---

## 1. Executive Overview & Architecture Topology

**YP-Raksha** is an enterprise-grade AI Governance, Data Loss Prevention (DLP), and Threat Auditing platform designed to monitor, audit, redact, or block sensitive enterprise interactions across corporate AI platforms (e.g., ChatGPT, Claude, Microsoft Copilot, Google Gemini, DeepSeek, Perplexity, etc.).

### Architecture Topology

```
                  ┌────────────────────────────────────────────────────────┐
                  │                 USER WORKSTATION                       │
                  │                                                        │
                  │   ┌────────────────────────────────────────────────┐   │
                  │   │   Browser (Chrome / Edge / Brave / Firefox)    │   │
                  │   │   + YP-Raksha Guard Extension (Manifest V3)    │   │
                  │   └───────────────────────┬────────────────────────┘   │
                  │                           │ 127.0.0.1:18195 (PAC)      │
                  │                           ▼                            │
                  │   ┌────────────────────────────────────────────────┐   │
                  │   │   YP_Raksha.exe (Local Desktop Agent)          │   │
                  │   │   - Embedded PAC Server (Port 18195)           │   │
                  │   │   - Local MitM Interceptor (Port 18103)        │   │
                  │   │   - Local Rule Engine & Fail-Open Safety Net   │   │
                  │   └───────────────────────┬────────────────────────┘   │
                  └───────────────────────────┼────────────────────────────┘
                                              │ HTTPS Outbound (Port 443 / 6000)
                                              ▼
                  ┌────────────────────────────────────────────────────────┐
                  │              GATEWAY ENTERPRISE SERVER                 │
                  │                                                        │
                  │   ┌───────────────────────┐ ┌──────────────────────┐   │
                  │   │ Next.js Web Console   │ │ Go Gateway Backend   │   │
                  │   │ Port: 3100 (UI)       │ │ Port: 6000 (API/Core)│   │
                  │   └───────────┬───────────┘ └──────────┬───────────┘   │
                  │               │                        │               │
                  │               └───────────┬────────────┘               │
                  │                           ▼                            │
                  │            ┌─────────────────────────────┐             │
                  │            │ PostgreSQL Database (v14+)  │             │
                  │            │ Port: 5432 / Custom         │             │
                  │            └─────────────────────────────┘             │
                  └────────────────────────────────────────────────────────┘
```

---

## 2. Server Hardware & Operating System Requirements

### 2.1 Server Sizing Guidelines

| Deployment Tier | Endpoints Supported | Minimum CPU | Minimum RAM | Storage (NVMe/SSD) | Network Bandwidth |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Starter / PoC** | 1 – 100 Endpoints | 4 vCPU | 8 GB RAM | 60 GB SSD | 100 Mbps |
| **Enterprise Standard** | 100 – 1,000 Endpoints | 8 vCPU | 16 GB RAM | 250 GB NVMe | 1 Gbps |
| **Enterprise High-Scale** | 1,000 – 10,000+ Endpoints | 16+ vCPU | 32 GB RAM | 500 GB+ NVMe (RAID 10) | 10 Gbps |

### 2.2 Supported Server Operating Systems
* **Linux (Recommended for Production):**
  * Ubuntu 22.04 LTS / 24.04 LTS (64-bit)
  * Red Hat Enterprise Linux (RHEL) 8.x / 9.x
  * Rocky Linux 9.x / AlmaLinux 9.x
  * Debian 11 / 12
* **Windows Server:**
  * Windows Server 2019 / 2022 (x86_64)

---

## 3. Endpoint Workstation Requirements (Agent & Extension)

### 3.1 Supported Endpoint Operating Systems
* **Windows:**
  * Windows 10 (21H2 or later, 64-bit)
  * Windows 11 (22H2 or later, 64-bit)
  * Windows Server 2016, 2019, 2022 (Remote Desktop / VDI sessions)
* **macOS:**
  * macOS 12 (Monterey)
  * macOS 13 (Ventura)
  * macOS 14 (Sonoma)
  * macOS 15 (Sequoia)
  * *Architecture:* Apple Silicon (M1/M2/M3/M4) and Intel (x86_64)

### 3.2 Endpoint Resource Consumption
* **RAM Footprint:** 35 MB – 55 MB idle (under 95 MB during active 10MB multi-file attachment scans).
* **CPU Utilization:** < 0.2% average background consumption.
* **Disk Footprint:** ~85 MB installation footprint.

---

## 4. Software Runtimes & Programming Language Requirements

If hosting from source or compiling the binaries in-house, ensure the following compilers and runtimes are provisioned:

| Technology | Supported Version | Purpose |
| :--- | :--- | :--- |
| **Go (Golang)** | **1.22.x** or **1.23.x** | Core high-throughput backend server (`gateway-http`), rule matching engine, database broker. |
| **Node.js** | **v18.x LTS** or **v20.x LTS** | Frontend web dashboard (`Next.js 14`, Tailwind CSS, React). |
| **npm** | **v9.x** or **v10.x** | Node package dependency management. |
| **Python** | **3.10.x**, **3.11.x**, or **3.12.x** (64-bit) | Desktop Agent engine (`YP_Raksha.exe`), proxy crypto, PAC distribution service. |
| **PostgreSQL** | **14.x**, **15.x**, or **16.x** | Primary persistent relational datastore for logs, policies, fleet telemetry, and targets. |
| **Inno Setup** | **6.2+** *(Optional)* | Used only for compiling the Windows setup installer (`YP_Raksha_Setup.exe`). |

---

## 5. Docker & Containerized Production Deployment

### 5.1 Docker Engine Prerequisites
* **Docker Engine:** Version 24.0 or newer (API version 1.43+)
* **Docker Compose:** Version 2.20 or newer (v2 Compose plugin)

### 5.2 Production `docker-compose.yml`

Below is the verified production `docker-compose.yml` for enterprise deployment:

```yaml
version: "3.8"

services:
  gateway_tech:
    build:
      context: .
      dockerfile: deploy/docker/Dockerfile.local
    image: gateway-local:latest
    container_name: ${CONTAINER_NAME:-raksha_yespanchi}
    user: "0:0"
    restart: unless-stopped
    env_file:
      - .env
    environment:
      - APP_DIR=${APP_DIR:-/app/data}
      - APP_PORT=${APP_PORT:-6000}
      - APP_HOST=${APP_HOST:-0.0.0.0}
      - LOG_LEVEL=${LOG_LEVEL:-info}
      - SERVER_DOMAIN=${SERVER_DOMAIN:-https://raksha.yourcompany.com}
      - DB_TYPE=${DB_TYPE:-postgres}
      - DB_HOST=${DB_HOST}
      - DB_PORT=${DB_PORT:-5432}
      - DB_NAME=${DB_NAME:-raksha_db}
      - DB_USER=${DB_USER}
      - DB_PASSWORD=${DB_PASSWORD}
      - DB_SSL_MODE=${DB_SSL_MODE:-disable}
      - ADMIN_EMAIL=${ADMIN_EMAIL}
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}
      - GATEWAY_BRAND_NAME=${GATEWAY_BRAND_NAME:-Raksha}
      - GATEWAY_GUARD_SECRET=${GATEWAY_GUARD_SECRET}
      - GATEWAY_GUARD_REQUIRE_SECRET=${GATEWAY_GUARD_REQUIRE_SECRET:-1}
    extra_hosts:
      - "host.docker.internal:host-gateway"
    ports:
      - "${APP_PORT:-6000}:${APP_PORT:-6000}"
      - "${UI_PORT:-3100}:${UI_PORT:-3100}"
    dns:
      - 8.8.8.8
      - 1.1.1.1
    volumes:
      - ./data:/app/data
      - ./configs:/app/configs
      - ./apps/browser-guard/release:/app/release
      - ./apps/browser-guard/proxy:/app/guard-proxy:ro
      - ./apps/browser-guard/agent:/app/guard-agent:ro
    networks:
      - raksha-network

  # Optional Centralized Office/Lab Network Proxy (Disabled by default; Agent EXE is standard)
  gateway_browser_ai_proxy:
    profiles:
      - network-proxy
    image: mitmproxy/mitmproxy:latest
    container_name: ${NETWORK_PROXY_CONTAINER_NAME:-raksha_network_proxy}
    restart: unless-stopped
    command: >
      mitmdump -s /app/proxy/browser_ai_proxy.py -p ${PROXY_PORT:-18182}
      --set listen_host=0.0.0.0 --set block_global=false --set ssl_insecure=true
      --set ignore_hosts='^(localhost|127\.0\.0\.1|${CONTAINER_NAME})'
    environment:
      - GATEWAY_BACKEND_URL=http://${CONTAINER_NAME:-raksha_yespanchi}:${APP_PORT:-6000}
      - GATEWAY_GUARD_SECRET=${GATEWAY_GUARD_SECRET}
      - GATEWAY_FAIL_OPEN=0
      - GATEWAY_SERVER_MODE=1
      - GATEWAY_AGENT_TYPE=network
    ports:
      - "${PROXY_PORT:-18182}:${PROXY_PORT:-18182}"
    volumes:
      - ./apps/browser-guard/proxy:/app/proxy
    depends_on:
      - gateway_tech
    networks:
      - raksha-network

networks:
  raksha-network:
    name: ${DOCKER_NETWORK:-raksha-network}
    driver: bridge
```

### 5.3 Deploying with Docker

```bash
# 1. Clone repository & configure .env file
cp .env.example .env
nano .env

# 2. Build and launch services in background
docker compose up -d --build

# 3. Check container logs
docker compose logs -f gateway_tech
```

---

## 6. Network Ports & Firewall Whitelist Specification

The corporate firewall, web proxy, and endpoint host firewalls must allow traffic on the designated ports:

### 6.1 Server-Side Network Ports

| Port | Protocol | Direction | Scope / Interface | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| **6000** | TCP / HTTP(S) | **Inbound** | `0.0.0.0` (Public / Reverse Proxy) | Gateway API Backend (Prompt intercepts, fleet sync, telemetry, rule distribution). |
| **3100** | TCP / HTTP(S) | **Inbound** | Internal LAN / Admin Subnet | Web Admin Management Dashboard (Next.js UI). |
| **5432** | TCP | **Inbound** | Internal DB Host only | PostgreSQL Database connection (Never expose to public internet). |
| **18182** | TCP / HTTP(S) | **Inbound** | *(Optional)* Office LAN | Centralized Network Proxy Port (Used only if running shared network proxy container). |
| **18183** | TCP / HTTP | **Inbound** | Localhost / Admin Subnet | *(Optional)* MitM Proxy Web Inspection Diagnostic UI. |

### 6.2 Endpoint Workstation (Client-Side) Ports

> [!IMPORTANT]
> The ports below run strictly on the **loopback adapter (`127.0.0.1`)**. They do **not** accept external incoming network connections, preserving zero-trust endpoint boundaries.

| Port | Protocol | Interface | Access Scope | Function |
| :--- | :--- | :--- | :--- | :--- |
| **18195** | TCP / HTTP | `127.0.0.1` | Local Browser only | PAC (Proxy Auto-Configuration) script distribution and extension IPC heartbeat. |
| **18103** | TCP / HTTPS | `127.0.0.1` | Local Browser only | Local High-Speed MitM Loopback Interceptor (`GATEWAY_PROXY_ADDR`). |

---

## 7. Endpoint Antivirus, EDR & Security Whitelist Rules

To prevent false-positive quarantines by Endpoint Detection and Response (EDR) solutions (e.g., CrowdStrike Falcon, SentinelOne, Microsoft Defender for Endpoint, Carbon Black, Trellix), configure the following whitelists:

### 7.1 Binary & Process Exclusions

Whitelist the following executables by **Path** and **Process Name**:

* **Process Names:**
  * `YP_Raksha.exe`
  * `YP_Raksha_Setup.exe`
* **File System Directory Paths (Windows):**
  * `%LOCALAPPDATA%\Programs\YP\Raksha\*`
  * `%LOCALAPPDATA%\Gateway\Guard\*`
  * `C:\Program Files\YP\Raksha\*` *(if installed system-wide)*
* **File System Directory Paths (macOS):**
  * `/Applications/YP-Raksha.app/`
  * `~/Library/Application Support/YP-Raksha/`
  * `/Library/Application Support/YP-Raksha/`

### 7.2 Behavioral & Network Heuristic Clearances
* **Local Loopback Listening:** Allow `YP_Raksha.exe` to bind and listen on TCP loopback interfaces `127.0.0.1:18195` and `127.0.0.1:18103`.
* **Outbound HTTPS Telemetry:** Allow outbound HTTPS connections from `YP_Raksha.exe` to the corporate Gateway Server domain (e.g., `https://raksha.yourcompany.com`).
* **Root Certificate Trust (SSL Inspection):**
  * `YP_Raksha` generates/installs a local root certificate (`mitmproxy-ca-cert.cer`) into the OS **Trusted Root Certification Authorities** store.
  * *Purpose:* Required to perform local, in-memory SSL termination on monitored AI domains only (e.g., `chatgpt.com`), ensuring encrypted sensitive documents or prompts are audited prior to leaving the computer.
  * Non-AI websites (banking, intranet, ERP, personal browsing) are completely bypassed and never decrypted.

---

## 8. Browser Compatibility & Extension Whitelist (GPO / Registry)

### 8.1 Supported Browsers
| Browser | Minimum Version | Session Support |
| :--- | :--- | :--- |
| **Google Chrome** | v109+ (Manifest V3) | Standard Browsing + Incognito Mode |
| **Microsoft Edge** | v109+ (Chromium) | Standard Browsing + InPrivate Mode |
| **Brave Browser** | v1.50+ | Standard Browsing + Private Window |
| **Opera / Opera GX** | v95+ | Standard Browsing + Private Window |
| **Mozilla Firefox** | v115+ ESR | Standard Browsing + Private Browsing |

### 8.2 Enterprise Group Policy (GPO) Silent Deployment

Enterprise administrators can force-install the **Browser Guard Extension** across all corporate machines via Active Directory GPO or Intune.

#### Windows Registry Keys for Force-Installation:

**Google Chrome:**
* **Key:** `HKLM\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist`
* **Type:** `REG_SZ`
* **Value:** `<Extension_ID>;https://clients2.google.com/service/update2/crx`

**Microsoft Edge:**
* **Key:** `HKLM\SOFTWARE\Policies\Microsoft\Edge\ExtensionInstallForcelist`
* **Type:** `REG_SZ`
* **Value:** `<Extension_ID>;https://edge.microsoft.com/extensionwebstorebase/v1/crx`

#### Required Browser Extension Manifest Permissions:
* `declarativeNetRequest` (High-speed traffic policy routing)
* `webRequest` & `webRequestBlocking` (Pre-send request inspection)
* `storage` (Encrypted policy caching for offline DLP evaluation)
* `cookies` (Session authorization & enterprise domain binding)
* `tabs` & `scripting` (DOM-level copy/paste & clipboard DLP enforcement)

---

## 9. File DLP & Supported Attachment Inspection Types

YP-Raksha inspects file uploads to AI platforms in real time. The deep-content extraction engine parses and inspects the following MIME types:

| Category | File Extensions | Inspection Depth |
| :--- | :--- | :--- |
| **Documents** | `.pdf` | Multi-page text streams, form fields, and embedded text layers. |
| **Word Processing** | `.docx`, `.doc`, `.rtf`, `.odt` | Native XML parsing, headers, footers, tables, and paragraphs. |
| **Spreadsheets** | `.xlsx`, `.xls`, `.csv`, `.tsv` | Workbook sheets, cell text, formulas, and tabular customer records. |
| **Presentations** | `.pptx`, `.ppt` | Slide text bodies, speaker notes, and embedded shape text. |
| **Plain Text & Data** | `.txt`, `.json`, `.xml`, `.yaml`, `.yml`, `.log` | Full plain text scanning against regex & keyword dictionaries. |
| **Source Code & Keys** | `.py`, `.js`, `.ts`, `.go`, `.java`, `.cpp`, `.c`, `.sql`, `.sh`, `.env`, `.pem`, `.key` | High-risk credential and API key detection (AWS, Azure, OpenAI keys, private RSA certificates). |
| **Images & AI Vision** | `.png`, `.jpg`, `.jpeg`, `.webp` | High-fidelity OCR text extraction and multimodal AI vision guardrails. |

> [!NOTE]
> File extraction slices up to **5 MB – 10 MB** of raw textual content per document. Files exceeding standard size thresholds are either inspected via streaming chunk evaluation or blocked per configured enterprise file size policy.

---

## 10. Environment Variables (`.env`) Master Reference Manual

The table below details every configuration variable available in the master `.env` file:

```ini
# ==============================================================================
# 1. SERVER & PUBLIC DOMAIN CONFIGURATION
# ==============================================================================
SERVER_DOMAIN=https://raksha.yourcompany.com
# The public domain or IP where the Gateway Backend is reachable by agents.

GATEWAY_BACKEND_URL=https://raksha.yourcompany.com
# Internal URL used by proxy and agent to reach backend REST APIs.

APP_HOST=0.0.0.0
# Bind address for backend server (0.0.0.0 allows all incoming interfaces).

APP_PORT=6000
# TCP port for backend core HTTP service.

UI_PORT=3100
# TCP port for the Next.js web console dashboard.

CONTAINER_NAME=raksha_yespanchi
# Docker container name for the core backend.

DOCKER_NETWORK=raksha-network
# Docker bridge network name connecting services.

# ==============================================================================
# 2. LOCAL AGENT & PROXY PORT CONFIGURATION
# ==============================================================================
PAC_HTTP_PORT=18195
# Endpoint loopback port serving Proxy Auto-Configuration (PAC) script to browsers.

GATEWAY_PROXY_ADDR=127.0.0.1:18103
# Endpoint loopback port running the local MitM SSL decrypt/inspection engine.

PROXY_PORT=18182
# Port used only if running centralized network proxy container.

NETWORK_PROXY_CONTAINER_NAME=raksha_proxy
# Container name for optional centralized proxy.

# ==============================================================================
# 3. DATABASE CONFIGURATION (PostgreSQL)
# ==============================================================================
DB_TYPE=postgres
# Database dialect: postgres (recommended), mysql, mariadb, or sqlite.

DB_HOST=10.0.0.5
# IP or hostname of the PostgreSQL database instance.

DB_PORT=5432
# PostgreSQL connection port.

DB_NAME=gateway_production
# Target database name.

DB_USER=gateway_admin
# Database connection username.

DB_PASSWORD=YourSecureDatabasePassword123!
# Database user password.

DB_SSL_MODE=disable
# SSL connection mode: disable, require, verify-ca, verify-full.

# ==============================================================================
# 4. INITIAL ADMINISTRATOR ACCOUNT
# ==============================================================================
ADMIN_EMAIL=admin@yourcompany.com
# Default Super-Admin email provisioned on initial boot.

ADMIN_PASSWORD=InitialAdminPassword2026!
# Default Super-Admin password.

# ==============================================================================
# 5. ENTERPRISE BRANDING & CUSTOMIZATION
# ==============================================================================
GATEWAY_BRAND_NAME=Raksha
# Product brand label shown in navigation and headers.

GATEWAY_PRODUCT_NAME=Raksha
# Product title string.

GATEWAY_PRODUCT_SUBTITLE=Real-time AI Knowledge Screening & Hazard Audit
# Header subtitle in dashboard.

GATEWAY_COMPANY_NAME=Your Corporation Name
# Enterprise company title.

GATEWAY_COMPANY_SHORT_NAME=YourCorp
# Short corporate abbreviation.

GATEWAY_COMPANY_LOGO=/company-logo.png
# URL or asset path to corporate branding logo.

GATEWAY_FOOTER_COPYRIGHT=© 2026 Your Corporation. All rights reserved.
# Legal copyright displayed across login and audit reports.

# ==============================================================================
# 6. SECURITY & ENCRYPTION KEYS (CRITICAL)
# ==============================================================================
GATEWAY_ENCRYPTION_KEY=your-32-char-min-hex-encryption-key-here
# 256-bit symmetric encryption key used for database fields at rest.

UNIFAI_ENCRYPTION_KEY=your-32-char-min-hex-encryption-key-here
# Backward-compatibility key mirror.

GATEWAY_GUARD_SECRET=z_mwSwAoxfxsA6cec7tVZaJh2s-xnnROA58mVTAQTjZCWZQex6cNC44Q_W7Dzb3a
# Cryptographic shared secret ensuring only authorized YP_Raksha agents can post audit logs.
```

---

## 11. Health Checks, Smoke Testing & Verification

Once deployed, administrators can verify system health using the following tests:

### 11.1 Backend Health Check
```bash
curl -i http://localhost:6000/health
# Expected Output: HTTP 200 OK {"status":"healthy","database":"connected"}
```

### 11.2 Endpoint Agent Loopback Verification
From the employee workstation command prompt / PowerShell:
```powershell
curl -i http://127.0.0.1:18195/extension/ping
# Expected Output: HTTP 200 OK
# {"status":"ok","connected":true,"agent_version":"1.5.0"}
```

### 11.3 PAC Script Distribution Test
```powershell
curl -i http://127.0.0.1:18195/proxy.pac
# Expected Output: HTTP 200 OK
# Content-Type: application/x-ns-proxy-autoconfig
# Returns JavaScript FindProxyForURL(url, host) function targeting monitored AI hosts.
```

---

## 12. Support & Maintenance Contacts

* **Platform Documentation & Fleet Packages:** Accessible via `/workspace/browser-ai?tab=setup`
* **Technical Escalations:** `support@yespanchi.com` | `admin@yespanchi.com`
* **Release Artifacts:** Built standalone executables and macOS suites are located in `/apps/browser-guard/release/`.

