# UnifAI / Raksha Enterprise AI Platform
# Complete Documentation Suite & Master Directory Index

**Classification:** Enterprise System Documentation  
**Suite Release:** Version 2.4.0  
**Storage Directory:** `/pdf/` & `/docs/pdf/`  
**Generated Date:** October 2026  

---

## 1. Documentation Suite Overview

This documentation folder (`pdf/`) contains the complete technical, operational, architectural, and security manuals for the **UnifAI Enterprise AI Gateway** and **Raksha Browser Guard** endpoint protection suite.

All documentation is provided in both **Markdown (`.md`)** format for source tracking and **Adobe PDF (`.pdf`)** format for distribution, printing, and archiving.

---

## 2. Document Catalog & Purpose

| # | Document File | Formats Available | Primary Audience | Description & Coverage |
|---|---|---|---|---|
| **1** | **Technical Documentation** | `01_TECHNICAL_DOCUMENTATION.md`<br>`01_Technical_Documentation.pdf` | Architects, Lead Developers, Security Engineers | Complete system architecture, Fasthttp gateway engine, GORM data models, ~28 AI provider connectors, Browser Guard MitM proxy architecture, DLP engine, RBAC matrix, and plugin system. |
| **2** | **Server Implementation Guide** | `02_SERVER_IMPLEMENTATION_GUIDE.md`<br>`02_Server_Implementation_Guide.pdf` | DevOps, SRE, Systems Administrators | Hardware capacity planning, bare-metal Linux systemd deployment, complete `.env` configuration breakdown, Nginx & Caddy reverse proxy setup, Prometheus metrics, health checks, log management, and zero-downtime rolling upgrades. |
| **3** | **Admin Console Guide** | `03_ADMIN_CONSOLE_GUIDE.md`<br>`03_Admin_Console_Guide.pdf` | Platform Admins, InfoSec, Workspace Managers | First-time bootstrap setup, User lifecycle management, Custom RBAC role builder (33 resources × 6 operations), Access Profiles, Model Provider connections, Virtual Key spend limits, Browser AI Target Websites, DLP Rules, Fleet Agent monitoring, and Workspace Configuration views (Client Settings, Compatibility, Caching, Security, API Keys, Performance Tuning, Feature Flags). |
| **4** | **End-User & Developer Guide** | `04_USER_GUIDE.md`<br>`04_User_Guide.pdf` | Employees, Developers, Data Scientists | Account registration and approval, navigating the Prompt Repository, Skills Repository, Model Playground, Developer API integration (Python, Node.js, LangChain, Cursor, Claude Desktop), Desktop Browser Guard installation for Windows & macOS, in-browser DLP alerts, and privacy protections. |
| **5** | **Database Configuration Guide** | `05_DATABASE_CONFIGURATION_GUIDE.md`<br>`05_Database_Configuration_Guide.pdf` | DBAs, DevOps, Infrastructure Engineers | PostgreSQL 16 setup on Ubuntu/Debian/RHEL/Docker, UTF-8 schemas, `pg_hba.conf` network security, SSL/TLS database encryption, GORM table schemas, high-volume log partitioning, PgBouncer connection pooling, production performance tuning parameters, and automated daily backup scripts with disaster recovery restore procedures. |
| **6** | **Docker & SSL/TLS Configuration Guide** | `06_DOCKER_AND_SSL_CONFIGURATION_GUIDE.md`<br>`06_Docker_and_SSL_Configuration_Guide.pdf` | DevOps, Cloud Engineers, SREs, SecOps | Multi-container Docker Compose architecture, exhaustive Docker command reference (`up`, `down`, `logs`, `exec`, `build`, `prune`), Public HTTPS with Let's Encrypt / Certbot, OpenSSL self-signed certificate generation, Browser Guard Root CA generation (`raksha-ca.crt`), client trust store installation (Windows `certutil`, macOS `security`, Linux `update-ca-certificates`, Firefox NSS), Nginx reverse proxy configuration, and SSL handshake diagnostic commands. |

---

## 3. Quick Reference: Critical System Ports

- **Public Web / HTTPS:** `443` (Nginx / Caddy Reverse Proxy)
- **Plain HTTP Redirect:** `80` (Certbot ACME challenge & HTTPS redirect)
- **UnifAI Go Backend Core:** `6000` (Internal localhost bind: `127.0.0.1:6000`)
- **PostgreSQL Database:** `5432` (or `6432` with PgBouncer)
- **Local Ollama LLM Service:** `11434` (Internal service for AI Guard Bot)
- **Browser Guard Local MitM Proxy:** `18103` (Bound to client laptop loopback `127.0.0.1`)
- **Browser Guard Local PAC Server:** `18195` (Bound to client laptop loopback `127.0.0.1`)

---
*UnifAI Documentation Suite — Built for Enterprise AI Governance & Security.*
