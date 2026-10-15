#!/usr/bin/env python3
"""Build Non-Technical Business & Operations Guide PDF (09_Non_Technical_Business_Guide.pdf)"""

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
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)

OUT = Path(__file__).resolve().parent
PDF_PATH = OUT / "09_Non_Technical_Business_Guide.pdf"

NAVY = colors.HexColor("#0f2744")
TEAL = colors.HexColor("#0e7490")
AMBER = colors.HexColor("#d97706")
RULE = colors.HexColor("#d6dde6")
ROW = colors.HexColor("#f8fafc")
CARD_BG = colors.HexColor("#f1f5f9")
DARK = colors.HexColor("#0f172a")
MUTED = colors.HexColor("#475569")


def styles():
    return {
        "kicker": ParagraphStyle("kicker", fontName="Helvetica-Bold", fontSize=10, textColor=TEAL, spaceAfter=4),
        "title": ParagraphStyle("title", fontName="Helvetica-Bold", fontSize=20, leading=24, textColor=NAVY, spaceAfter=6),
        "sub": ParagraphStyle("sub", fontName="Helvetica", fontSize=10, leading=14, textColor=MUTED, spaceAfter=8),
        "h1": ParagraphStyle("h1", fontName="Helvetica-Bold", fontSize=12.5, leading=15.5, textColor=NAVY, spaceBefore=10, spaceAfter=4),
        "h2": ParagraphStyle("h2", fontName="Helvetica-Bold", fontSize=10, leading=13, textColor=TEAL, spaceBefore=6, spaceAfter=3),
        "body": ParagraphStyle("body", fontName="Helvetica", fontSize=8.0, leading=11.0, alignment=TA_JUSTIFY, textColor=DARK, spaceAfter=4),
        "bullet": ParagraphStyle("bullet", fontName="Helvetica", fontSize=8.0, leading=11.0, textColor=DARK, leftIndent=10, spaceAfter=2),
        "callout": ParagraphStyle("callout", fontName="Helvetica-Bold", fontSize=9.0, leading=12.5, textColor=NAVY),
        "cell": ParagraphStyle("cell", fontName="Helvetica", fontSize=7.0, leading=9.0, textColor=DARK),
        "cell_bold": ParagraphStyle("cell_bold", fontName="Helvetica-Bold", fontSize=7.0, leading=9.0, textColor=DARK),
        "cell_analogy": ParagraphStyle("cell_analogy", fontName="Helvetica-Bold", fontSize=7.0, leading=9.0, textColor=TEAL),
        "cell_header": ParagraphStyle("cell_header", fontName="Helvetica-Bold", fontSize=7.5, leading=9.5, textColor=colors.white),
    }


ST = styles()


def P(text, style="body"):
    return Paragraph(text, ST[style])


def table(headers, rows, widths, is_analogy=False):
    data = [[Paragraph(h, ST["cell_header"]) for h in headers]]
    for row in rows:
        formatted_row = []
        for idx, c in enumerate(row):
            if isinstance(c, str):
                if is_analogy and idx == 0:
                    formatted_row.append(Paragraph(c, ST["cell_bold"]))
                elif is_analogy and idx == 1:
                    formatted_row.append(Paragraph(c, ST["cell_analogy"]))
                elif idx == 0:
                    formatted_row.append(Paragraph(c, ST["cell_bold"]))
                else:
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
    canvas.drawString(12 * mm, A4[1] - 7 * mm, "UnifAI Enterprise  ·  Non-Technical Business & Operations Guide")
    canvas.setFont("Helvetica", 8)
    canvas.drawRightString(A4[0] - 12 * mm, A4[1] - 7 * mm, "Executive & Operations Edition")
    canvas.setFillColor(TEAL)
    canvas.rect(0, 0, A4[0], 8 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Helvetica", 8)
    canvas.drawString(12 * mm, 3 * mm, "Plain English & Everyday Analogies  ·  Zero Technical Jargon")
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

    # Title Banner
    story.append(P("BUSINESS & EXECUTIVE OVERVIEW", "kicker"))
    story.append(P("UnifAI Non-Technical Business & Operations Guide", "title"))
    story.append(
        P(
            "<b>Audience:</b> Business Owners, HR Heads, Operations Managers, Client Executives, and Non-Technical Stakeholders &nbsp;|&nbsp; <b>Style:</b> Everyday Analogies & Plain Language",
            "sub",
        )
    )
    story.append(HRFlowable(width="100%", thickness=1.5, color=TEAL, spaceBefore=2, spaceAfter=6))

    # Section 1: What is UnifAI in Simple Words?
    story.append(P("1. What is UnifAI in Simple Words?", "h1"))
    story.append(
        P(
            "Imagine running an office with 500 employees. Every day, your staff use modern AI (like ChatGPT or Google Gemini) to draft emails, summarize documents, and code faster. But without governance, three dangerous problems occur:"
        )
    )
    story.append(P("<b>1. Data Leak Risk:</b> Employees paste customer bank records, patient PII, or company passwords into public AI tools.", "bullet"))
    story.append(P("<b>2. Runaway Bills:</b> Employees ask silly or basic questions to ultra-expensive AI models, draining corporate cards.", "bullet"))
    story.append(P("<b>3. Sudden Downtime:</b> If OpenAI server has an outage in the US, company chatbots and workflows completely freeze.", "bullet"))
    story.append(Spacer(1, 3))

    # Highlight Box
    box_data = [[
        Paragraph(
            "<b>The Core Solution:</b> UnifAI acts as your company's digital <b>Smart Manager + Gate Security Guard + Accountant</b> that sits between your staff and AI vendors. It ensures 100% protection against data leaks, cuts monthly AI bills by 70% to 80%, and guarantees zero downtime.",
            ST["callout"],
        )
    ]]
    t_box = Table(box_data, colWidths=[527])
    t_box.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), CARD_BG),
        ("LINEBEFORE", (0, 0), (0, -1), 3, TEAL),
        ("BOX", (0, 0), (-1, -1), 0.5, RULE),
        ("TOPPADDING", (0, 0), (-1, -1), 5),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
        ("LEFTPADDING", (0, 0), (-1, -1), 8),
        ("RIGHTPADDING", (0, 0), (-1, -1), 8),
    ]))
    story.append(t_box)
    story.append(Spacer(1, 6))

    # Section 2: Real-World Analogies Master Table
    story.append(P("2. What Each Part Actually Does (Everyday Real-World Analogies)", "h1"))
    story.append(
        P(
            "The table below explains what each platform feature does using simple, relatable office and daily life situations:"
        )
    )

    analogies_headers = ["Platform Feature", "Everyday Analogy", "What Work It Does in the Office", "Business Value"]
    analogies_rows = [
        [
            "Customer / Tenant",
            "The Entire Company Building",
            "The parent boundary of the company. All departments, laptops, and bills belong here. A competitor company cannot see or touch your data.",
            "Strict corporate data privacy and legal isolation.",
        ],
        [
            "Teams",
            "Floors in the Office (Departments)",
            "Groups staff into departments (Engineering, Finance, HR). Allows assigning a shared departmental monthly budget (e.g. $500/mo).",
            "No need to configure 500 people one-by-one.",
        ],
        [
            "Users",
            "Employees with ID Badges",
            "Individual staff accounts with role permissions (Admin, Manager, Member). If an employee leaves the company, disable access in 1 click.",
            "Accountability: traces who used AI and when.",
        ],
        [
            "Virtual Keys",
            "Prepaid Metro Card instead of Master Card",
            "Instead of handing developers your company master credit card, you give them a prepaid virtual token (vk-live-...). If lost, cancel in 1 second.",
            "Real AI account passwords are never exposed or stolen.",
        ],
        [
            "Model Providers",
            "Wholesale Suppliers / Supermarkets",
            "The wholesale vendors your company buys AI from (OpenAI, Anthropic Claude, Google Gemini). Safely stores real supplier credentials in a vault.",
            "Centralizes all AI vendor contracts in one place.",
        ],
        [
            "Model Catalog",
            "The Cafeteria Menu Card",
            "Inventory of all available AI models. Managers can turn OFF ultra-expensive models with 1 switch so staff don't order a $50 meal when a $1 meal works.",
            "Prevents staff from calling overpriced models.",
        ],
        [
            "Complexity Router",
            "Smart Hospital Receptionist",
            "Mild cold? Sent to junior clinic doctor ($10 fee). Major heart surgery? Sent to super-specialist ($1,000 fee). Simple greeting prompts route to cheap AI.",
            "Cuts enterprise monthly AI expenses by 70% to 80%.",
        ],
        [
            "Routing Rules",
            "Car Stepney Tyre / Backup Road",
            "If the main US server (OpenAI) crashes or has an outage, traffic instantly diverts to Google Gemini or Claude without any error to employees.",
            "99.99% application uptime with zero freeze.",
        ],
        [
            "Circuit Breaker",
            "The Electrical Fuse / MCB Switch",
            "If an AI vendor fails 3 times in a row, it trips the fuse to OPEN and stops knocking on that broken door. Instantly sends users to healthy backup AI.",
            "Users never stare at frozen 60-second loading spinners.",
        ],
        [
            "Budgets & Limits",
            "Mobile Prepaid Recharge Cap",
            "Just like 1.5GB daily data pack cuts off when full, sets a monthly hard cap (e.g. $300). Stops infinite loops or accidental runaway scripts.",
            "Guarantees no surprise $10,000 credit card bills.",
        ],
        [
            "Pricing Overrides",
            "Negotiated Corporate Wholesale Rate",
            "Retail price in shop is $100, but company has negotiated $60 wholesale contract. Ledgers calculate internal costs using wholesale rate.",
            "Accurate internal department cost attribution.",
        ],
        [
            "Model Settings",
            "Locked Office AC Thermostat",
            "Locking the AC at 24°C so staff don't tamper. Sets global standards for AI response length, accuracy, and connection timeout.",
            "Standardized, reliable AI outputs across the company.",
        ],
        [
            "Browser Guard (YP-Raksha)",
            "Gate Security Guard & Scanner",
            "Runs on employee laptops. If staff paste credit cards, customer phone numbers, or company secrets into public ChatGPT, it halts and blocks it!",
            "100% protection against employee leaks and data theft.",
        ],
        [
            "Audit Logs & Dashboard",
            "CCTV Camera + Bank Passbook",
            "Live manager screen showing: Who asked what, how much money was spent, which model was used, and if anyone attempted to leak confidential data.",
            "Complete executive visibility and compliance audit readiness.",
        ],
    ]
    # Total width = 527pt
    story.append(table(analogies_headers, analogies_rows, [80, 115, 197, 135], is_analogy=True))
    story.append(PageBreak())

    # Section 3: Supported AI Websites & Platforms
    story.append(P("3. Supported AI Websites & Platforms (Monitored by YP-Raksha)", "h1"))
    story.append(
        P(
            "UnifAI automatically inspects and enforces DLP rules across all modern AI services used by employees:"
        )
    )

    site_headers = ["AI Platform / Service", "Monitored Domains", "What Gets Inspected & Protected?"]
    site_rows = [
        ["OpenAI ChatGPT", "chatgpt.com, openai.com", "Prompts, code pastes, document & PDF attachments, image uploads."],
        ["Anthropic Claude", "claude.ai", "Long prompt texts, source code, uploaded technical documentation."],
        ["Google Gemini", "gemini.google.com", "Chat prompts, Google Workspace content, attached image analysis."],
        ["Microsoft Copilot", "copilot.microsoft.com", "Web chat queries, Edge sidebar Copilot interactions."],
        ["DeepSeek", "deepseek.com, chat.deepseek.com", "DeepSeek V3/R1 reasoning prompts, uploaded files and code."],
        ["Perplexity AI", "perplexity.ai", "AI search queries, research follow-up prompts and uploaded files."],
        ["Mistral AI (Le Chat)", "chat.mistral.ai, mistral.ai", "Developer queries, internal prompt evaluations."],
        ["xAI Grok", "grok.com, x.com/i/grok", "Direct Grok chatbot prompt conversations."],
        ["Poe (Quora)", "poe.com", "Multi-bot chat interactions and external bot integrations."],
        ["Qwen AI (Alibaba)", "chat.qwen.ai, qwen.ai", "Developer prompts, technical code analysis chats."],
        ["Custom AI Domains", "Any custom corporate domain", "Add any internal or custom AI website in 1 click via Dashboard."],
        ["Search Engines", "google.com, bing.com, duckduckgo.com, yahoo.com", "Predictive threat scoring to detect intent (e.g. searching how to dump database passwords)."],
    ]
    story.append(table(site_headers, site_rows, [110, 180, 237]))
    story.append(Spacer(1, 8))

    # Section 4: Supported Browsers & Dual-Layer Coverage
    story.append(P("4. Master Browser Coverage & Dual-Layer Protection Matrix", "h1"))
    story.append(
        P(
            "UnifAI provides <b>Dual-Layer Security</b>: (1) In-Browser Extension hook directly on the webpage, and (2) Operating System PAC Proxy fallback, ensuring zero unmonitored loopholes:"
        )
    )

    browser_headers = ["Browser Name", "Supported Editions", "Extension Hook", "System PAC Proxy", "Deployment & Anti-Tamper Method"]
    browser_rows = [
        ["Google Chrome", "Stable, Beta, Dev, Canary", "Yes (.crx)", "Yes", "Windows Registry ExtensionInstallForcelist + Desktop .lnk hooks"],
        ["Microsoft Edge", "Stable, Beta, Dev, SxS", "Yes (.crx)", "Yes", "Native Windows Enterprise Group Policy (Cannot be removed by staff)"],
        ["Brave Browser", "Standard, Nightly", "Yes (.crx)", "Yes", "Chromium Enterprise Policy & Desktop Shortcut Injection"],
        ["Mozilla Firefox", "Standard, Developer, ESR", "Yes (.xpi)", "Yes", "Mozilla Distribution policies.json & Windows Registry policy"],
        ["Opera & Opera GX", "Desktop Stable, Opera GX", "Yes", "Yes", "Chromium Policy & Shortcut Injection (Note: Opera Mini is mobile-only)"],
        ["Vivaldi Browser", "All Desktop versions", "Yes", "Yes", "Chromium Registry Policies & Shortcut Patching"],
        ["Chromium", "Pure Open-Source Builds", "Yes", "Yes", "Native Chromium Group Policy Root (HKLM)"],
        ["Apple Safari", "macOS Safari 16, 17, 18+", "Proxy Mode", "Yes", "macOS System Network Proxy (networksetup) + Managed Config Profile"],
        ["Arc & Other Browsers", "Windows & macOS", "Yes", "Yes", "System PAC Proxy & Shared Unpacked Extension directory"],
    ]
    story.append(table(browser_headers, browser_rows, [95, 105, 75, 75, 177]))
    story.append(PageBreak())

    # Section 5: Extension Formats & Anti-Tamper
    story.append(P("5. Extension Packaging Formats & Enterprise Security Features", "h1"))
    story.append(
        P(
            "The YP-Raksha desktop extension is compiled and distributed in multiple enterprise-grade formats:"
        )
    )

    fmt_headers = ["Packaging Format", "File / Deployment Path", "Target Platform", "Enterprise Security Function"]
    fmt_rows = [
        [
            ".crx Package",
            "guard_extension.crx",
            "Chrome, Edge, Brave, Opera, Vivaldi",
            "Cryptographically signed with 2048-bit RSA key; locks extension ID to mipfhcmlcjfhmgldkokdckhndkebkllj.",
        ],
        [
            ".xpi Package",
            "yp-raksha.xpi",
            "Mozilla Firefox",
            "Standard Firefox Add-on bundle distributed via Firefox policies.json in Program Files.",
        ],
        [
            "Unpacked Folder",
            "C:\\ProgramData\\Gateway\\Guard\\guard_extension",
            "System-wide Common Directory",
            "Zero user-specific profile paths; enables multi-user terminal servers and shortcut --load-extension.",
        ],
        [
            "update.xml Protocol",
            "http://127.0.0.1:18195/extension/update.xml",
            "Enterprise Auto-Update Engine",
            "Monitors code edits and automatically triggers silent browser background updates without user prompts.",
        ],
        [
            "Incognito Spanning",
            "Built-in Manifest V3 Capability",
            "All Browsers (Private Windows)",
            "Configured with incognito: spanning mode; DLP protection remains 100% active inside private browsing.",
        ],
        [
            "Machine Policy Lock",
            "HKLM\\Software\\Policies\\...",
            "Anti-Tamper Lockout",
            "Force-installed via Administrator policies; employees cannot toggle off or uninstall the extension.",
        ],
    ]
    story.append(table(fmt_headers, fmt_rows, [85, 140, 115, 187]))
    story.append(Spacer(1, 8))

    # Section 6: Manager's Quick Problem-Solver Matrix
    story.append(P("6. Manager's Quick Action Guide: What is Your Problem? Where to Click?", "h1"))
    story.append(
        P(
            "As an administrator, manager, or business executive, here is your quick cheat-sheet for common operational needs:"
        )
    )

    action_headers = ["Your Business Need", "What Action to Take", "Where to Go on Dashboard", "Expected Result"]
    action_rows = [
        [
            "A new employee joined the company",
            "Click 'Add User' button top-right",
            "Identity & Access > Users (/workspace/users)",
            "Employee receives welcome email and gets departmental AI access.",
        ],
        [
            "Want to set a $300 limit for Marketing",
            "Click 'Teams' tab > Set Monthly Quota",
            "Identity & Access > Users > Teams",
            "Marketing department is automatically capped at $300 for the month.",
        ],
        [
            "A developer needs an API key for a project",
            "Click 'Create Virtual Key'",
            "Model Hub > Virtual Keys (/workspace/virtual-keys)",
            "Issues safe proxy key (vk-live-...). Real OpenAI master password is never shared.",
        ],
        [
            "Monthly AI bills are too high",
            "Turn on 'Complexity Router' & disable costly models",
            "Routing > Complexity Router & Model Catalog",
            "Simple queries go to cheap models; saves 70% to 80% on bills.",
        ],
        [
            "OpenAI server is down in the US",
            "UnifAI handles this automatically via Fallback Chain",
            "Routing > Routing Rules (/workspace/routing-rules)",
            "Workflows switch to Google Gemini or Claude with zero manual intervention.",
        ],
        [
            "Check if any employee pasted company secrets",
            "Inspect 'Activity Logs' table for Red Blocks",
            "Browser AI > Activity Logs (/workspace/browser-ai)",
            "Shows laptop name, employee ID, exact prompt text, and blocked reason.",
        ],
        [
            "Check total AI spend and token usage",
            "Inspect live request ledger table",
            "Observability > Logs (/workspace/logs)",
            "Displays real-time breakdown of cost, model used, and response latency.",
        ],
    ]
    story.append(table(action_headers, action_rows, [115, 125, 145, 142]))
    story.append(Spacer(1, 8))

    # Section 7: Executive Takeaway
    story.append(P("7. Executive Takeaway in One Sentence", "h1"))
    summary_box = [[
        Paragraph(
            "<b>Final Summary:</b> UnifAI empowers your organization to adopt modern Artificial Intelligence at <b>80% lower cost</b>, with <b>zero fear of vendor outages</b>, and <b>100% protection against employee confidential data leaks</b> across all major browsers and AI platforms.",
            ST["callout"],
        )
    ]]
    t_sum = Table(summary_box, colWidths=[527])
    t_sum.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), CARD_BG),
        ("LINEBEFORE", (0, 0), (0, -1), 3, AMBER),
        ("BOX", (0, 0), (-1, -1), 0.5, RULE),
        ("TOPPADDING", (0, 0), (-1, -1), 5),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
        ("LEFTPADDING", (0, 0), (-1, -1), 8),
        ("RIGHTPADDING", (0, 0), (-1, -1), 8),
    ]))
    story.append(t_sum)
    story.append(Spacer(1, 6))
    story.append(
        P(
            "<b>Need Assistance?</b> Contact YesPanchi Support: <b>support@yespanchi.com</b> &nbsp;|&nbsp; Portal: <b>https://unifai.yespanchi.com</b>",
            "sub",
        )
    )

    doc.build(story, onFirstPage=header_footer, onLaterPages=header_footer)
    print(f"Successfully generated {PDF_PATH} ({PDF_PATH.stat().st_size} bytes)")


if __name__ == "__main__":
    build_pdf()
