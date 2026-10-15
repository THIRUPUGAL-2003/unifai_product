#!/usr/bin/env python3
"""Build Enterprise Connected System Flow Documentation PDF (08_Enterprise_Connected_System_Flows.pdf)"""

from pathlib import Path
from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER, TA_JUSTIFY, TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import mm
from reportlab.platypus import (
    HRFlowable,
    KeepTogether,
    PageBreak,
    Paragraph,
    Preformatted,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)

OUT = Path(__file__).resolve().parent
PDF_PATH = OUT / "08_Enterprise_Connected_System_Flows.pdf"

NAVY = colors.HexColor("#0f2744")
TEAL = colors.HexColor("#0e7490")
RULE = colors.HexColor("#d6dde6")
ROW = colors.HexColor("#f8fafc")
ACCENT = colors.HexColor("#1e40af")
MUTED = colors.HexColor("#475569")
DARK = colors.HexColor("#0f172a")
CARD_BG = colors.HexColor("#f1f5f9")


def styles():
    return {
        "kicker": ParagraphStyle("kicker", fontName="Helvetica-Bold", fontSize=10, textColor=TEAL, spaceAfter=4),
        "title": ParagraphStyle("title", fontName="Helvetica-Bold", fontSize=20, leading=24, textColor=NAVY, spaceAfter=6),
        "sub": ParagraphStyle("sub", fontName="Helvetica", fontSize=10, leading=14, textColor=MUTED, spaceAfter=8),
        "h1": ParagraphStyle("h1", fontName="Helvetica-Bold", fontSize=13, leading=16, textColor=NAVY, spaceBefore=12, spaceAfter=6),
        "h2": ParagraphStyle("h2", fontName="Helvetica-Bold", fontSize=11, leading=14, textColor=TEAL, spaceBefore=8, spaceAfter=4),
        "body": ParagraphStyle("body", fontName="Helvetica", fontSize=8.5, leading=11.5, alignment=TA_JUSTIFY, textColor=DARK, spaceAfter=4),
        "bullet": ParagraphStyle("bullet", fontName="Helvetica", fontSize=8.5, leading=11.5, textColor=DARK, leftIndent=12, spaceAfter=2),
        "code": ParagraphStyle("code", fontName="Courier", fontSize=7.0, leading=8.5, textColor=colors.HexColor("#0284c7")),
        "cell": ParagraphStyle("cell", fontName="Helvetica", fontSize=7.2, leading=9.2, textColor=DARK),
        "cell_bold": ParagraphStyle("cell_bold", fontName="Helvetica-Bold", fontSize=7.2, leading=9.2, textColor=DARK),
        "cell_header": ParagraphStyle("cell_header", fontName="Helvetica-Bold", fontSize=7.5, leading=9.5, textColor=colors.white),
        "callout": ParagraphStyle("callout", fontName="Helvetica-Oblique", fontSize=8.0, leading=11.0, textColor=NAVY),
    }


ST = styles()


def P(text, style="body"):
    return Paragraph(text, ST[style])


def table(headers, rows, widths):
    data = [[Paragraph(h, ST["cell_header"]) for h in headers]]
    for row in rows:
        formatted_row = []
        for c in row:
            if isinstance(c, str):
                formatted_row.append(Paragraph(c, ST["cell"]))
            else:
                formatted_row.append(c)
        data.append(formatted_row)
    t = Table(data, colWidths=widths, repeatRows=1)
    t.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, 0), NAVY),
                ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, ROW]),
                ("GRID", (0, 0), (-1, -1), 0.3, RULE),
                ("VALIGN", (0, 0), (-1, -1), "TOP"),
                ("LEFTPADDING", (0, 0), (-1, -1), 4),
                ("RIGHTPADDING", (0, 0), (-1, -1), 4),
                ("TOPPADDING", (0, 0), (-1, -1), 3),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 3),
            ]
        )
    )
    return t


def header_footer(canvas, doc):
    canvas.saveState()
    canvas.setFillColor(NAVY)
    canvas.rect(0, A4[1] - 11 * mm, A4[0], 11 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Helvetica-Bold", 8)
    canvas.drawString(12 * mm, A4[1] - 7 * mm, "UnifAI Enterprise  ·  Connected System Architecture & Flow Guide")
    canvas.setFont("Helvetica", 8)
    canvas.drawRightString(A4[0] - 12 * mm, A4[1] - 7 * mm, "Technical & Operations Manual v3.0")
    canvas.setFillColor(TEAL)
    canvas.rect(0, 0, A4[0], 8 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Helvetica", 8)
    canvas.drawString(12 * mm, 3 * mm, "Confidential  ·  YesPanchi Group of Companies / UnifAI Engineering")
    canvas.drawRightString(A4[0] - 12 * mm, 3 * mm, f"Page {doc.page}")
    canvas.restoreState()


def build_pdf():
    doc = SimpleDocTemplate(
        str(PDF_PATH),
        pagesize=A4,
        leftMargin=12 * mm,
        rightMargin=12 * mm,
        topMargin=15 * mm,
        bottomMargin=12 * mm,
    )
    story = []

    # Title & Metadata
    story.append(P("ENTERPRISE PLATFORM ARCHITECTURE & OPERATIONS GUIDE", "kicker"))
    story.append(P("UnifAI Connected System Flow Documentation", "title"))
    story.append(
        P(
            "<b>Edition:</b> v3.0 Enterprise &nbsp;|&nbsp; <b>Scope:</b> Tenant Hierarchy, Entity Lifecycles, End-to-End Request Pipelines & Dashboard Navigation &nbsp;|&nbsp; <b>Author:</b> UnifAI Core Engineering",
            "sub",
        )
    )
    story.append(HRFlowable(width="100%", thickness=1.5, color=TEAL, spaceBefore=2, spaceAfter=8))

    # Section 1: Executive Overview
    story.append(P("1. Executive Summary & Architectural Overview", "h1"))
    story.append(
        P(
            "The <b>UnifAI Enterprise Platform</b> unifies generative AI governance, multi-provider model routing, cost intelligence, and employee endpoint Data Loss Prevention (DLP) into a single coherent system. The platform bridges two operational tiers: <b>(1) The AI Gateway Control Plane</b> running on the server/cloud to manage API traffic, spend caps, failover, and token pricing, and <b>(2) Browser Guard & YP-Raksha</b> running on employee endpoints to prevent confidential data leakage across public AI services (ChatGPT, Claude, Gemini)."
        )
    )
    story.append(Spacer(1, 4))

    # Topology Preformatted Block
    topo_text = (
        "+---------------------------------------------------------------------------------------------------------+\n"
        "|                                    CUSTOMER / TENANT ORGANIZATION                                      |\n"
        "|   +------------------------------------+             +----------------------------------------------+   |\n"
        "|   |         TEAMS / DEPARTMENTS        |             |               USERS & IDENTITIES             |   |\n"
        "|   |  - Engineering   - Finance         | <---------> |  - Admins      - Developers                  |   |\n"
        "|   |  - Customer Care - Legal & HR      |  (Belongs)  |  - Managers    - Support Staff               |   |\n"
        "|   +-----------------+------------------+             +----------------------+-----------------------+   |\n"
        "|                     |                                                       |                           |\n"
        "|                     | (Inherits or Issues)                                  | (Directly Issued)         |\n"
        "|                     v                                                       v                           |\n"
        "|   +-------------------------------------------------------------------------------------------------+   |\n"
        "|   |                                     VIRTUAL KEYS (GATEWAY API)                                  |   |\n"
        "|   |  - Proxy Key (vk-live-...) - Spend Cap ($) - Rate Limits (RPM/TPM) - Allowed Models Restriction |   |\n"
        "|   +-------------------------------------------------+-----------------------------------------------+   |\n"
        "|                                                     | Sends AI Prompts                                  |\n"
        "|                                                     v                                                   |\n"
        "|   +-------------------------------------------------------------------------------------------------+   |\n"
        "|   |                                  UNIFAI AI GATEWAY CORE ENGINE                                  |   |\n"
        "|   |   +-----------------------+     +-----------------------+     +-----------------------------+   |   |\n"
        "|   |   |  1. BUDGETS & LIMITS  | --> | 2. COMPLEXITY ROUTER  | --> |    3. ROUTING & FAILOVER    |   |   |\n"
        "|   |   |  Check Daily/Monthly  |     | Classify Prompt:      |     | Priority Fallback Chains    |   |   |\n"
        "|   |   |  Caps & Token Buckets |     | Simple vs Complex     |     | Weighted Load Balancing     |   |   |\n"
        "|   |   +-----------------------+     +-----------------------+     +--------------+--------------+   |   |\n"
        "|   |                                                                              |                  |   |\n"
        "|   |   +-----------------------+     +-----------------------+     +--------------v--------------+   |   |\n"
        "|   |   |  6. AUDIT & LOGSTORE  | <-- | 5. PRICING OVERRIDES  | <-- |     4. CIRCUIT BREAKER      |   |   |\n"
        "|   |   | Real-time prompt logs |     | Calculate exact token |     | Monitor upstream outages    |   |   |\n"
        "|   |   | Cost & latency record |     | cost (custom rates)   |     | Auto-trip to OPEN if 5xx    |   |   |\n"
        "|   |   +-----------------------+     +-----------------------+     +--------------+--------------+   |   |\n"
        "|   +------------------------------------------------------------------------------+------------------+   |\n"
        "|                                                                                  | Outbound LLM Call|\n"
        "|                                                                                  v                  |\n"
        "|   +-------------------------------------------------------------------------------------------------+   |\n"
        "|   |       UPSTREAM MODEL PROVIDERS: [ OpenAI ]   [ Anthropic ]   [ Gemini ]   [ Groq ]   [ Ollama ] |   |\n"
        "|   +-------------------------------------------------------------------------------------------------+   |\n"
        "|   +-------------------------------------------------------------------------------------------------+   |\n"
        "|   |      CLIENT ENDPOINTS (LAPTOPS): Gateway_Guard.exe <--> YP-Raksha Extension <--> Browser AI     |   |\n"
        "|   +-------------------------------------------------------------------------------------------------+   |\n"
        "+---------------------------------------------------------------------------------------------------------+"
    )
    t_style = ParagraphStyle("topo", fontName="Courier", fontSize=5.3, leading=6.6, textColor=NAVY)
    story.append(Preformatted(topo_text, t_style))
    story.append(Spacer(1, 8))

    # Section 2: Why Each Component Exists
    story.append(P("2. Why Each Component Exists & What Problem It Solves", "h1"))
    story.append(
        P(
            "Every module in UnifAI is engineered to resolve a critical operational, financial, or governance failure that occurs when companies attempt to deploy raw AI APIs directly."
        )
    )

    comp_headers = ["Component", "Why It Exists (Problem Solved)", "What Happens If Missing?", "Dashboard Location"]
    comp_rows = [
        [
            "Customer / Tenant",
            "Represents the enterprise parent company. Enforces strict multi-tenant isolation, data boundaries, and root license enforcement.",
            "Data between corporations would mingle; no centralized root governance or compliance boundary.",
            "Platform Root / Settings",
        ],
        [
            "Teams",
            "Groups users into logical departments (Engineering, Finance, HR). Enables department-level spend caps and shared virtual keys.",
            "Admins must configure thousands of employees individually one-by-one; no departmental budgeting.",
            "/workspace/users > Teams",
        ],
        [
            "Users",
            "Individual employee identities with Role-Based Access Control (Admin, Sub-Admin, Member, Viewer) and authentication audit trail.",
            "Zero individual accountability; cannot trace who sent prompts or who modified security policies.",
            "/workspace/users > Users",
        ],
        [
            "Virtual Keys",
            "Secure proxy API keys (vk-live-...) masking raw upstream vendor credentials. Allows individual revocation, spend caps, and model ACLs.",
            "Developers store master provider keys on laptops, leading to stolen credentials, leakages, and runaway bills.",
            "/workspace/virtual-keys",
        ],
        [
            "Model Providers",
            "Secure vault storing real upstream API keys, custom base URLs (e.g. Ollama localhost:11434), and connection testing.",
            "The gateway has no secure store for AI vendor credentials and cannot route requests upstream.",
            "/workspace/providers",
        ],
        [
            "Model Catalog",
            "Master inventory of all available AI models across all providers. Allows instant company-wide model enabling/disabling.",
            "Employees call deprecated, ultra-expensive, or non-compliant models without administrator consent.",
            "/workspace/model-catalog",
        ],
        [
            "Complexity Router",
            "Evaluates prompt semantic complexity in real time. Routes simple questions to cheap models and complex reasoning to flagship models.",
            "Massive cost inflation: simple greeting or summary prompts burn expensive 10x flagship token budgets.",
            "/workspace/complexity-router",
        ],
        [
            "Routing Rules",
            "Defines fallback chains (Primary -> Fallback 1 -> Fallback 2) and weighted multi-provider load balancing (e.g. 70/30).",
            "A single vendor outage (e.g. OpenAI 500 error) brings down all enterprise applications and chatbots.",
            "/workspace/routing-rules",
        ],
        [
            "Circuit Breaker",
            "Monitors error rates. Trips OPEN after 3 consecutive failures to bypass dead APIs instantly without 60s timeout hangs.",
            "End users suffer 60-second timeouts and spinning wheels while the app fruitlessly waits on dead providers.",
            "/workspace/circuit-breaker",
        ],
        [
            "Budgets & Limits",
            "Sets hard spending limits ($ / INR) and rate limits (RPM / TPM) per team, user, or virtual key with auto-cutoff.",
            "Runaway scripts or infinite loops drain the company's entire credit card balance overnight.",
            "/workspace/model-limits",
        ],
        [
            "Pricing Overrides",
            "Enables entering negotiated corporate token prices ($/1M input & output tokens) instead of vendor retail prices.",
            "Internal billing, department chargebacks, and analytics reflect inflated retail prices instead of actual cost.",
            "/workspace/custom-pricing/overrides",
        ],
        [
            "Model Settings",
            "Sets global enterprise runtime parameters (temperature, max output tokens, top_p, gateway timeout).",
            "Inconsistent behavior; developers must redundantly hardcode runtime settings in every individual application.",
            "/workspace/custom-pricing",
        ],
        [
            "Browser Guard (YP-Raksha)",
            "Local agent and browser extension scanning ChatGPT, Claude, and Gemini prompt inputs for PII, API keys, and corporate IP.",
            "Staff paste proprietary source code, patient records, customer credit cards, and legal secrets into public AI web tools.",
            "/workspace/browser-ai",
        ],
        [
            "Audit Logs & Observability",
            "Central audit repository recording prompt previews, token usage, cost, latency, risk score, and DLP blocks.",
            "Complete blindness into AI adoption, departmental spend distribution, and security incident investigations.",
            "/workspace/logs & /browser-ai",
        ],
    ]
    # Page width = 210mm - 24mm = 186mm = ~527pt
    story.append(table(comp_headers, comp_rows, [85, 175, 145, 122]))
    story.append(PageBreak())

    # Section 3: The 4 Core End-to-End System Flows
    story.append(P("3. The 4 Core End-to-End Functional Flows", "h1"))

    # Flow 1
    story.append(P("Flow 1: Identity, Teams & Access Provisioning Pipeline", "h2"))
    story.append(
        P(
            "<b>Objective:</b> Seamless onboarding of a department and employee with automated budget allocation, credential issuance, and license compliance verification."
        )
    )

    f1_headers = ["Step #", "Actor / Component", "Action Executed", "Backend Event & State Change", "Dashboard Impact"]
    f1_rows = [
        [
            "1.1",
            "Enterprise Admin",
            "Navigates to /workspace/users > Teams tab. Clicks 'Create Team'.",
            "Inserts record into 'teams' table with default budget allocation (e.g. $200/mo).",
            "New Team appears immediately in Teams directory with 0 members and active status.",
        ],
        [
            "1.2",
            "Enterprise Admin",
            "Clicks 'Add User' on Users tab. Enters Name, Email, Role=Member, selects Team.",
            "Checks 'browser_ai_license' MaxSeats quota. Hashes temporary password.",
            "If seats full, shows 403 Forbidden. If seats available, user row created with status=Active.",
        ],
        [
            "1.3",
            "System SMTP",
            "Dispatches automated welcome email with secure one-time login link.",
            "Sets 'must_change_password' = true on the user database record.",
            "User receives email invitation with corporate portal URL and temporary credentials.",
        ],
        [
            "1.4",
            "Employee",
            "Clicks link and signs in to UnifAI portal for the first time.",
            "Session initiated; UI detects 'must_change_password' flag and locks navigation.",
            "User is presented with compulsory 'Update Your Password' security modal.",
        ],
        [
            "1.5",
            "Employee & Gateway",
            "Submits strong new password. Portal loads personalized workspace.",
            "Updates user record 'must_change_password' = false; binds user to Team Virtual Key.",
            "User dashboard renders allowed models, team budget balance, and personal activity log.",
        ],
    ]
    story.append(table(f1_headers, f1_rows, [30, 80, 140, 150, 127]))
    story.append(Spacer(1, 8))

    # Flow 2
    story.append(P("Flow 2: Live AI Request, Intelligent Routing & Cost Deduct Pipeline", "h2"))
    story.append(
        P(
            "<b>Objective:</b> High-throughput prompt processing with sub-millisecond policy evaluation, complexity classification, failover protection, and real-time ledger accounting."
        )
    )

    f2_headers = ["Step #", "Engine Stage", "Internal Operation", "Decision / Policy Matrix", "Failure / Edge Case"]
    f2_rows = [
        [
            "2.1",
            "API Authentication",
            "Receives POST /v1/chat/completions with Bearer vk-live-xxx.",
            "Hashes incoming key and queries virtual_keys table for status and validity.",
            "HTTP 401 Unauthorized if key is revoked, expired, or non-existent.",
        ],
        [
            "2.2",
            "Budget & Rate Limiting",
            "Checks virtual key, user, and team spend balances in governance store.",
            "Compares against monthly spend cap ($200) and RPM/TPM token bucket counters.",
            "HTTP 429 Too Many Requests if budget cap reached or rate limit exceeded.",
        ],
        [
            "2.3",
            "Complexity Classifier",
            "Scans prompt length, vocabulary density, and reasoning depth indicators.",
            "Calculates score: Score < 0.35 -> gpt-4o-mini; Score > 0.70 -> gpt-4o / claude-3-5-sonnet.",
            "Falls back to model explicitly specified in payload if complexity routing disabled.",
        ],
        [
            "2.4",
            "Circuit Breaker Check",
            "Inspects memory state for target provider (CLOSED vs OPEN).",
            "If CLOSED, proceeds normally. If OPEN, diverts traffic to pre-configured fallback model.",
            "If all providers in chain are OPEN, returns HTTP 503 Upstream Unavailable.",
        ],
        [
            "2.5",
            "Upstream LLM Dispatch",
            "Extracts real provider key from vault; dispatches payload to vendor API.",
            "Tracks round-trip latency, streaming chunks, and completion token count.",
            "If upstream returns 5xx error or times out, increments Circuit Breaker failure counter.",
        ],
        [
            "2.6",
            "Pricing & Audit Commit",
            "Applies Pricing Overrides (e.g. $0.15/1M input tokens) rather than vendor retail list.",
            "Deducts exact token cost from Team balance and inserts audit record into Logstore.",
            "Logs immediately stream to /workspace/logs dashboard with full latency breakdown.",
        ],
    ]
    story.append(table(f2_headers, f2_rows, [30, 85, 140, 150, 122]))
    story.append(PageBreak())

    # Flow 3
    story.append(P("Flow 3: Circuit Breaker Resilience & Self-Healing Failover State Machine", "h1"))
    story.append(
        P(
            "<b>Objective:</b> Guarantee 99.99% uptime for AI applications by automatically isolating failing providers and self-healing when upstream health recovers."
        )
    )

    cb_state_text = (
        "  +---------------------------------------------------------------------------------------------------+\n"
        "  |                                   CIRCUIT BREAKER STATE MACHINE                                   |\n"
        "  |                                                                                                   |\n"
        "  |   [ STATE 1: CLOSED (Healthy) ]                                                                   |\n"
        "  |   - All traffic sent to Primary Provider (e.g. OpenAI gpt-4o).                                    |\n"
        "  |   - 200 OK responses reset failure counter to 0/3.                                                |\n"
        "  |   - If 3 consecutive 5xx/timeouts occur within 60s ----> TRIPS TO OPEN!                          |\n"
        "  |                                                                                                   |\n"
        "  |                                        | (3 consecutive errors)                                   |\n"
        "  |                                        v                                                          |\n"
        "  |                                                                                                   |\n"
        "  |   [ STATE 2: OPEN (Tripped / Bypassing Primary) ]                                                 |\n"
        "  |   - Primary provider immediately BYPASSED without waiting for timeouts.                          |\n"
        "  |   - All traffic seamlessly diverted to Fallback Provider (e.g. Anthropic claude-3-5-haiku).      |\n"
        "  |   - Internal Cooldown Timer starts (Default: 30 seconds).                                         |\n"
        "  |                                                                                                   |\n"
        "  |                                        | (Cooldown expires)                                       |\n"
        "  |                                        v                                                          |\n"
        "  |                                                                                                   |\n"
        "  |   [ STATE 3: HALF-OPEN (Testing Recovery Probe) ]                                                 |\n"
        "  |   - Permits exactly 1 real probe request through to Primary Provider.                             |\n"
        "  |   - IF PROBE SUCCEEDS (200 OK)  ----> RECOVERS TO CLOSED (Normal traffic restored).               |\n"
        "  |   - IF PROBE FAILS (5xx/Timeout) ----> TRIPS BACK TO OPEN (Cooldown restarted for 30s).          |\n"
        "  +---------------------------------------------------------------------------------------------------+"
    )
    story.append(Preformatted(cb_state_text, t_style))
    story.append(Spacer(1, 6))

    # Flow 4
    story.append(P("Flow 4: Employee Laptop DLP & Browser AI Guard Lifecycle", "h2"))
    story.append(
        P(
            "<b>Objective:</b> Zero-leakage enterprise DLP protecting confidential source code, customer PII, and financial records when staff use web-based AI tools."
        )
    )

    f4_headers = ["Phase", "Component", "Runtime Event", "DLP Evaluation & Enforcement", "Security Audit Outcome"]
    f4_rows = [
        [
            "1. Intercept",
            "YP-Raksha Extension",
            "Employee types prompt in ChatGPT/Claude and hits Enter or Send.",
            "DOM event hook halts web transmission; extracts raw prompt string and target URL.",
            "Browser request is paused in-flight; spinner displayed to user.",
        ],
        [
            "2. Local Proxy",
            "Gateway_Guard.exe (Agent)",
            "Extension forwards prompt to local background service on 127.0.0.1:18195.",
            "Validates agent registration against local license seat counter (MaxSeats check).",
            "If agent offline, policy dictates Fail-Open or Fail-Closed mode.",
        ],
        [
            "3. DLP Analysis",
            "DLP Rule Engine",
            "Scans prompt against regex patterns (Credit Cards, SSNs, AWS Secrets, Private Keys).",
            "Matches against custom corporate keyword lists ('Project Orion', 'Acquisition M&A').",
            "Generates Risk Score (0-100) and tags detected entity types.",
        ],
        [
            "4. Enforcement",
            "Enforcement Layer",
            "Evaluates action: BLOCK (Risk > 80), WARN (Risk 40-79), or ALLOW (Risk < 40).",
            "If BLOCK: Cancels network call and renders red alert bubble on web page.",
            "If WARN: Shows yellow caution toast but allows transmission to AI vendor.",
        ],
        [
            "5. Central Log",
            "Logstore Server",
            "Agent asynchronously uploads encrypted incident packet to Central Gateway.",
            "Records timestamp, user ID, laptop MAC, prompt preview, and rule matched.",
            "Incident immediately visible on /workspace/browser-ai Activity Logs table.",
        ],
    ]
    story.append(table(f4_headers, f4_rows, [45, 75, 135, 145, 127]))
    story.append(PageBreak())

    # Section 4: Complete Dashboard Click-Map
    story.append(P("4. Master Dashboard Click-Map: Where to Click, What It Opens & What It Shows", "h1"))
    story.append(
        P(
            "This section acts as the primary operational navigation guide for administrators, operators, and compliance managers."
        )
    )

    click_headers = ["Goal / Task", "Sidebar Click", "Page Action", "What It Opens / What It Shows"]
    click_rows = [
        [
            "Add a new employee user",
            "Identity & Access > Users (/workspace/users)",
            "Click 'Add User' button top-right.",
            "Opens User Creation modal: Name, Email, Role (Admin, Member, Viewer), Team assignment, and key inheritance.",
        ],
        [
            "Group users by department",
            "Identity & Access > Users (/workspace/users)",
            "Click 'Teams' tab at top of table.",
            "Displays team directory (Engineering, Finance, HR); allows setting departmental monthly spend caps.",
        ],
        [
            "Create developer proxy key",
            "Model Hub > Virtual Keys (/workspace/virtual-keys)",
            "Click 'Create Virtual Key' button.",
            "Generates secure proxy key (vk-live-...); allows setting RPM/TPM rate limits, budgets, and allowed models.",
        ],
        [
            "Connect AI provider credentials",
            "Model Hub > Model Providers (/workspace/providers)",
            "Click 'Add Provider' dropdown.",
            "Opens Provider modal: Enter API Key, set Custom Base URL (Ollama localhost:11434), test live connection.",
        ],
        [
            "Enable or disable AI models",
            "Model Hub > Model Catalog (/workspace/model-catalog)",
            "Toggle 'Status' switch on any model row.",
            "Enables or disables model company-wide. Disabled models are blocked from all incoming virtual key calls.",
        ],
        [
            "Set spend limits & rate limits",
            "Governance > Budgets & Limits (/workspace/model-limits)",
            "Click 'Create Budget Policy'.",
            "Sets spend cap ($/INR), time window (Daily/Monthly), and breach action (Soft Warning vs Hard Block).",
        ],
        [
            "Auto-route simple prompts",
            "Routing > Complexity Router (/workspace/complexity-router)",
            "Click 'Configure Tiers' button.",
            "Configures score thresholds: <0.35 -> gpt-4o-mini; >0.70 -> gpt-4o. Shows live classifier distribution.",
        ],
        [
            "Configure automatic failover",
            "Routing > Routing Rules (/workspace/routing-rules)",
            "Click 'New Routing Rule' > Fallback Chain.",
            "Visual rule builder: sets Primary Provider (OpenAI) -> Fallback 1 (Anthropic) -> Fallback 2 (Gemini).",
        ],
        [
            "Manage downtime circuit breakers",
            "Routing > Circuit Breaker (/workspace/circuit-breaker)",
            "Inspect table or click 'Create Policy'.",
            "Live status monitor: CLOSED (Healthy), OPEN (Tripped), HALF-OPEN (Testing). Includes Manual Reset button.",
        ],
        [
            "Set custom billing token rates",
            "Cost & Pricing > Custom Pricing (/custom-pricing/overrides)",
            "Click 'Add Pricing Override'.",
            "Sets custom Input & Output Token Price per 1M tokens. Overrides vendor retail rates in ledger logs.",
        ],
        [
            "Configure global model defaults",
            "Cost & Pricing > Model Settings (/custom-pricing)",
            "Edit inputs and click 'Save Settings'.",
            "Sets global Temperature (0.7), Max Output Tokens (4096), Top-P, and Gateway request timeout.",
        ],
        [
            "Check laptop seat quota & license",
            "Browser AI > License & Fleet (/workspace/browser-ai)",
            "Inspect top 'Seat Capacity' card.",
            "Displays active seat count (e.g. 14 / 1000 in Use), expiry date, server IP match, and license upload.",
        ],
        [
            "View employee prompt DLP logs",
            "Browser AI > Activity Logs (/workspace/browser-ai)",
            "Click on any incident log row.",
            "Opens prompt drawer: full text, AI tool used, DLP violation tag (Blocked/Warned), and laptop hostname.",
        ],
        [
            "Inspect real-time API logs & cost",
            "Observability > Logs (/workspace/logs)",
            "Click on request row in live table.",
            "Opens request inspector: virtual key used, prompt tokens, completion tokens, latency (ms), and exact cost.",
        ],
    ]
    story.append(table(click_headers, click_rows, [95, 105, 105, 222]))
    story.append(PageBreak())

    # Section 5: Database Cardinality & Cross-Reference
    story.append(P("5. Entity Cardinality & Relational Database Cross-Reference", "h1"))
    story.append(
        P(
            "The platform enforces strict relational integrity between organization tenants, departments, individual identities, and security policies. The matrix below defines how database tables correspond to platform entities."
        )
    )

    db_headers = ["Platform Entity", "Database Table", "Primary Key", "Foreign Keys & Relationships", "Storage Engine"]
    db_rows = [
        ["User", "users", "id (UUID)", "team_id -> teams.id, customer_id -> customers.id", "PostgreSQL / SQLite"],
        ["Team", "teams", "id (UUID)", "customer_id -> customers.id, budget_id -> budgets.id", "PostgreSQL / SQLite"],
        ["Team Member", "team_members", "id (UUID)", "team_id -> teams.id, user_id -> users.id", "PostgreSQL / SQLite"],
        ["Virtual Key", "virtual_keys", "id (UUID)", "team_id -> teams.id, budget_id -> budgets.id", "PostgreSQL / SQLite"],
        ["Model Provider", "model_providers", "id (UUID)", "vault_ref_id -> key_vault.id", "PostgreSQL / SQLite"],
        ["Model Pricing", "model_pricings", "id (UUID)", "model_name -> model_catalog.model_id", "PostgreSQL / SQLite"],
        ["Routing Policy", "routing_policies", "id (UUID)", "customer_id -> customers.id", "PostgreSQL / SQLite"],
        ["Circuit Breaker", "circuit_breaker_policies", "id (UUID)", "provider_id -> model_providers.id", "PostgreSQL / SQLite"],
        ["Browser Guard Agent", "browser_ai_agents", "id (UUID)", "customer_id -> customers.id", "PostgreSQL / SQLite"],
        ["Enterprise License", "browser_ai_license", "id (UUID)", "None (Root cryptographically signed envelope)", "PostgreSQL / SQLite"],
        ["Prompt Audit Log", "prompt_audit_logs", "id (UUID)", "virtual_key_id -> virtual_keys.id", "Logstore / ClickHouse"],
    ]
    story.append(table(db_headers, db_rows, [90, 95, 60, 202, 80]))
    story.append(Spacer(1, 10))

    # Section 6: Operational Verification & Contact
    story.append(P("6. Operational Verification & Enterprise SLA", "h1"))
    story.append(
        P(
            "All functional pipelines, circuit breaker failovers, token override ledgers, and endpoint DLP controls described in this document are active and verifiable in <b>UnifAI Enterprise v3.0</b>. All endpoints comply with enterprise-grade data protection regulations including SOC2 Type II, GDPR, and ISO 27001."
        )
    )
    story.append(Spacer(1, 4))
    story.append(
        P(
            "<b>Enterprise Support & Assistance:</b><br/>"
            "Email: <b>support@yespanchi.com</b> &nbsp;|&nbsp; Web Portal: <b>https://unifai.yespanchi.com</b> &nbsp;|&nbsp; Emergency Operations Desk: 24/7 Priority Channel"
        )
    )

    doc.build(story, onFirstPage=header_footer, onLaterPages=header_footer)
    print(f"Successfully generated {PDF_PATH} ({PDF_PATH.stat().st_size} bytes)")


if __name__ == "__main__":
    build_pdf()
