#!/usr/bin/env python3
"""Build the UnifAI / Gateway Guard client documentation PDF pack."""

from pathlib import Path

from reportlab.lib import colors
from reportlab.lib.enums import TA_JUSTIFY, TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import mm
from reportlab.platypus import (
    CondPageBreak,
    HRFlowable,
    ListFlowable,
    ListItem,
    PageBreak,
    Paragraph,
    Preformatted,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)

OUT = Path(__file__).resolve().parent
import sys
sys.path.insert(0, str(OUT))
from build_screens_pdf import full_catalog  # noqa: E402
NAVY = colors.HexColor("#0f2744")
TEAL = colors.HexColor("#0e7490")
GOLD = colors.HexColor("#b45309")
RULE = colors.HexColor("#d6dde6")
ROW = colors.HexColor("#f4f7fb")
MUTED = colors.HexColor("#475569")


def styles():
    base = getSampleStyleSheet()
    s = {
        "cover_kicker": ParagraphStyle(
            "cover_kicker", fontName="Times-Bold", fontSize=11, textColor=TEAL,
            tracking=1.2, spaceAfter=8,
        ),
        "cover_title": ParagraphStyle(
            "cover_title", fontName="Times-Bold", fontSize=26, leading=30,
            textColor=NAVY, spaceAfter=8,
        ),
        "cover_sub": ParagraphStyle(
            "cover_sub", fontName="Times-Roman", fontSize=12, leading=16,
            textColor=MUTED, spaceAfter=6,
        ),
        "h1": ParagraphStyle(
            "h1", fontName="Times-Bold", fontSize=16, leading=20,
            textColor=NAVY, spaceBefore=14, spaceAfter=6,
        ),
        "h2": ParagraphStyle(
            "h2", fontName="Times-Bold", fontSize=13, leading=16,
            textColor=TEAL, spaceBefore=12, spaceAfter=4,
        ),
        "h3": ParagraphStyle(
            "h3", fontName="Times-Bold", fontSize=11.5, leading=14,
            textColor=NAVY, spaceBefore=8, spaceAfter=3,
        ),
        "body": ParagraphStyle(
            "body", fontName="Times-Roman", fontSize=10.5, leading=14.5,
            textColor=colors.HexColor("#1e293b"), alignment=TA_JUSTIFY, spaceAfter=6,
        ),
        "bullet": ParagraphStyle(
            "bullet", fontName="Times-Roman", fontSize=10.5, leading=14,
            textColor=colors.HexColor("#1e293b"), leftIndent=12, spaceAfter=2,
        ),
        "small": ParagraphStyle(
            "small", fontName="Times-Roman", fontSize=9, leading=12,
            textColor=MUTED, spaceAfter=4,
        ),
        "cell": ParagraphStyle(
            "cell", fontName="Times-Roman", fontSize=8.5, leading=11,
            textColor=colors.HexColor("#1e293b"),
        ),
        "cellb": ParagraphStyle(
            "cellb", fontName="Times-Bold", fontSize=8.5, leading=11, textColor=NAVY,
        ),
        "code": ParagraphStyle(
            "code", fontName="Courier", fontSize=8, leading=10.5,
            textColor=colors.HexColor("#0f172a"), backColor=colors.HexColor("#f8fafc"),
            leftIndent=4, rightIndent=4, spaceBefore=2, spaceAfter=8,
        ),
        "footer": ParagraphStyle(
            "footer", fontName="Times-Roman", fontSize=8, textColor=MUTED,
        ),
    }
    return s


S = styles()


def P(text, style="body"):
    return Paragraph(text, S[style])


def bullets(items):
    flow = []
    for item in items:
        flow.append(Paragraph("•  " + item, S["bullet"]))
    flow.append(Spacer(1, 4))
    return flow


def table(headers, rows, col_widths=None):
    head = [Paragraph(h, S["cellb"]) for h in headers]
    data = [head]
    for row in rows:
        data.append([Paragraph(str(c), S["cell"]) for c in row])
    width = 170 * mm
    if col_widths is None:
        col_widths = [width / len(headers)] * len(headers)
    t = Table(data, colWidths=col_widths, repeatRows=1)
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), NAVY),
        ("TEXTCOLOR", (0, 0), (-1, 0), colors.white),
        ("BACKGROUND", (0, 1), (-1, -1), colors.white),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, ROW]),
        ("GRID", (0, 0), (-1, -1), 0.3, RULE),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 4),
        ("RIGHTPADDING", (0, 0), (-1, -1), 4),
        ("TOPPADDING", (0, 0), (-1, -1), 3),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 3),
    ]))
    return t


def header_footer(canvas, doc, title):
    canvas.saveState()
    canvas.setFillColor(NAVY)
    canvas.rect(0, A4[1] - 12 * mm, A4[0], 12 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Times-Bold", 8)
    canvas.drawString(16 * mm, A4[1] - 7.5 * mm, "UnifAI  ·  Gateway Guard  ·  YP-Raksha")
    canvas.setFont("Times-Roman", 8)
    canvas.drawRightString(A4[0] - 16 * mm, A4[1] - 7.5 * mm, title)
    canvas.setFillColor(TEAL)
    canvas.rect(0, 0, A4[0], 10 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Times-Roman", 8)
    canvas.drawString(16 * mm, 4 * mm, "Client documentation  ·  Example data only  ·  Not a live password")
    canvas.drawRightString(A4[0] - 16 * mm, 4 * mm, f"Page {doc.page}")
    canvas.restoreState()


def attach_catalog(story):
    story.append(PageBreak())
    story.extend(full_catalog())
    return story


def build(filename, title, story):
    story = attach_catalog(story)
    path = OUT / filename
    doc = SimpleDocTemplate(
        str(path), pagesize=A4,
        leftMargin=16 * mm, rightMargin=16 * mm,
        topMargin=18 * mm, bottomMargin=16 * mm,
        title=title, author="UnifAI",
    )
    doc.build(story, onFirstPage=lambda c, d: header_footer(c, d, title),
              onLaterPages=lambda c, d: header_footer(c, d, title))
    return path


def cover(kicker, title, subtitle, audience):
    return [
        Spacer(1, 28 * mm),
        P(kicker, "cover_kicker"),
        P(title, "cover_title"),
        P(subtitle, "cover_sub"),
        Spacer(1, 4 * mm),
        HRFlowable(width="100%", thickness=1, color=TEAL, spaceAfter=8),
        P(audience, "body"),
        P("This pack describes the product as it is built: the UnifAI website, the Gateway Guard laptop agent, and the YP-Raksha browser extension. Sample names, emails, and passwords are examples for training. Replace them with your company values. Do not reuse the sample passwords.", "body"),
        PageBreak(),
    ]


W = [38 * mm, 132 * mm]


def website_manual():
    story = cover(
        "CLIENT PACK  ·  DOCUMENT 01",
        "Website user manual",
        "How to sign in, sign out, and use every main screen",
        "Audience: every person who opens the UnifAI website — employees, developers, security staff, and administrators.",
    )
    story += [
        P("1. What you are signing in to", "h1"),
        P("UnifAI is the company website for AI governance. One login page serves every role. After sign-in, the menu changes to match the permissions on that account. Gateway Guard is the program installed on the laptop. YP-Raksha is the browser extension that Guard turns on. You do not sign in to the extension. It follows the company policy.", "body"),
        table(
            ["Piece", "Where you use it"],
            [
                ["UnifAI website", "Browser. Sign in with username and password."],
                ["Gateway Guard", "Installed on the work laptop. Runs in the background. No daily login."],
                ["YP-Raksha", "Browser extension. Installed and kept on by company policy. No separate login."],
                ["Virtual Key", "Not a website login. A secret used by an application calling the AI API."],
            ],
            [40 * mm, 130 * mm],
        ),
        P("2. Website address", "h1"),
        P("Your IT team gives you the real address. In every example below the address is https://unifai.example.com. Use your company address instead.", "body"),
        P("3. All login types", "h1"),
        P("There is one Sign In page. The account role decides what you can open after login.", "body"),
        table(
            ["Login type", "Who", "What they can do after login"],
            [
                ["Bootstrap admin", "The first account created from the server settings (ADMIN_EMAIL / ADMIN_PASSWORD).", "Full control. Change this password on day one and create a named admin for daily work."],
                ["admin", "Platform owner.", "Users, roles, providers, master API keys, system settings, Browser AI, logs, everything."],
                ["sub_admin", "Security or workspace manager.", "Users, virtual keys, budgets, prompts, logs, Browser AI. Cannot change master provider keys or system settings."],
                ["user", "Employee.", "The pages an admin granted. Often Prompt Repository, and read-only dashboard or logs."],
                ["Custom role", "A role an admin created.", "Only the resources and sidebar sections on that role."],
                ["SCIM user", "Created by Azure AD or Okta.", "Same website login. If the identity provider sets the account inactive, status becomes disabled and sign-in stops."],
            ],
            [32 * mm, 55 * mm, 83 * mm],
        ),
        P("3.1 Sample accounts (training only)", "h2"),
        table(
            ["Role", "Username", "Email", "Sample password"],
            [
                ["admin", "priya.admin", "priya.admin@example.com", "Priya#Admin24"],
                ["sub_admin", "ravi.security", "ravi.security@example.com", "Ravi#Secure24"],
                ["user", "anitha.user", "anitha.user@example.com", "Anitha#User24"],
                ["custom (auditor)", "meena.audit", "meena.audit@example.com", "Meena#Audit24"],
            ],
            [32 * mm, 38 * mm, 55 * mm, 45 * mm],
        ),
        P("Password rule on the website: at least 8 characters, one uppercase letter, one lowercase letter, one number, and one special character.", "small"),
        P("3.2 How to sign in", "h2"),
        P("Open https://unifai.example.com/login.", "body"),
    ]
    story += bullets([
        "Username: anitha.user",
        "Password: Anitha#User24",
        "Click Sign In.",
        "A normal session lasts about 24 hours. After that, sign in again.",
    ])
    story += [
        P("Where you land depends on the role. An admin opens the full workspace. A user opens the first page they are allowed to see. If you open a page you were not granted, the site sends you to your first allowed page. It does not show an error.", "body"),
        P("3.3 Account states you may see", "h2"),
        table(
            ["Message / state", "Meaning", "What to do"],
            [
                ["email_unverified", "Sign-up started. The email code is not entered yet.", "Open Sign Up again with the same username and enter the 6-digit code."],
                ["pending", "Waiting for an administrator to approve the account.", "Wait. An admin approves it under Governance → Users."],
                ["rejected", "An administrator declined the request.", "Contact your administrator. A new sign-up can use the email again."],
                ["disabled", "The account was turned off, often by SCIM when the person left.", "Contact IT. Sign-in will not work."],
                ["approved", "The account can sign in.", "Use Sign In."],
                ["Too many failed attempts", "Temporary lock after wrong passwords.", "Wait the number of minutes shown on the screen."],
                ["Force change password", "A temporary password must be replaced.", "Enter the current password, then a new password that meets the rule above."],
            ],
            [38 * mm, 62 * mm, 70 * mm],
        ),
        P("3.4 Sign up (new employee)", "h2"),
        P("Open https://unifai.example.com/signup. The site always registers this form as role user. An administrator can later raise the role.", "body"),
        table(
            ["Field", "Example"],
            [
                ["Username", "anitha.user"],
                ["Email", "anitha.user@example.com"],
                ["Password", "Anitha#User24"],
                ["Confirm password", "Anitha#User24"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("The 6-digit code is sent only after an administrator has saved SMTP on Settings → Security and the test mail succeeded. Enter the code on the same page. Example code: 481926. The request then waits for approval. You cannot sign in while the state is pending.", "body"),
        P("3.5 Forgot password", "h2"),
    ]
    story += bullets([
        "On Sign In, choose Forgot password.",
        "Enter the username or the email on the account.",
        "Example: anitha.user@example.com",
        "Enter the one-time code from email. The code is valid for 10 minutes.",
        "Set a new password, for example Anitha#Reset24, and confirm it.",
        "Return to Sign In and use the new password.",
    ])
    story += [
        P("If the email does not arrive, check spam and wait a few minutes. The site limits how often a code can be sent. IT must have SMTP configured.", "body"),
        P("3.6 Forgot username", "h2"),
    ]
    story += bullets([
        "On Sign In, choose Forgot username.",
        "Enter the email on the account, for example anitha.user@example.com.",
        "The username is sent to that email.",
        "Go back to Sign In.",
    ])
    story += [
        P("3.7 Sign out", "h2"),
        P("Use Sign out in the account menu. The site clears the session cookie and returns you to the login page. The address may include logged_out=1 so the site does not send you straight back in. Closing the browser is not a substitute on a shared computer: use Sign out.", "body"),
        P("Signing out of the website does not stop Gateway Guard or YP-Raksha. Those stay on until IT uninstalls them with the company uninstall key.", "body"),
        P("4. Menu after login", "h1"),
        P("You only see sections your account is allowed to open. A typical employee with the user role sees a short menu. An admin sees the full menu below.", "body"),
        table(
            ["Menu", "What it is for"],
            [
                ["Dashboard", "Usage, cost, latency, and token totals."],
                ["LLM Logs", "Each AI API call: who, which model, tokens, cost, errors."],
                ["MCP Logs", "Tool calls made through the MCP gateway."],
                ["Observability", "Connectors, traces, and external monitoring."],
                ["Prompt Repository", "Save, organise, and reuse prompts."],
                ["Skills Repository", "Shared skills the company published."],
                ["Playground", "Try a model through the company gateway."],
                ["Browser AI", "Websites, guard rules, prompt logs, search logs, laptop agents, setup."],
                ["Virtual Keys", "API keys for applications, with budget and rate limit."],
                ["Governance", "Users, teams, customers, roles, access profiles, audit."],
                ["Model Providers / Catalog", "Which AI vendors and models the company uses."],
                ["Guardrails", "Rules on API traffic through the gateway. Separate from Browser AI rules."],
                ["Settings", "SMTP, security, cache, plugins, retention. Admin only."],
            ],
            [48 * mm, 122 * mm],
        ),
        P("5. How an employee uses the website day to day", "h1"),
        P("5.1 Prompt Repository — save a prompt", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Name", "Customer reply — refund tone"],
                ["Folder", "Support"],
                ["Prompt text", "You are a support agent for Example Company. Reply in a calm tone. Do not ask for card numbers or government IDs."],
                ["Tags", "support, refund"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("Save. The prompt stays in the company repository. If your account is limited to certain folders, other folders stay hidden.", "body"),
        P("5.2 Playground — send a test message", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Model", "The model your admin enabled, for example gpt-4.1-mini"],
                ["Message", "Summarise our refund policy in three bullets."],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("Send. The call goes through UnifAI, not directly to the public website. Cost and tokens appear in LLM Logs if you have access.", "body"),
        P("5.3 Developer — call the API with a Virtual Key", "h2"),
        P("This is not a website login. An admin creates the key and gives it to you once. Example key shape: sk-unifai-example-7f3a9c. Point the client at the company gateway.", "body"),
        Preformatted(
            'curl https://unifai.example.com/v1/chat/completions \\\n'
            '  -H "Authorization: Bearer sk-unifai-example-7f3a9c" \\\n'
            '  -H "Content-Type: application/json" \\\n'
            '  -d "{\\"model\\":\\"gpt-4.1-mini\\",\\"messages\\":[{\\"role\\":\\"user\\",\\"content\\":\\"Hello\\"}]}"',
            S["code"],
        ),
        P("If the call returns 401 or 403, the key may be revoked, expired, or over its budget or rate limit. Ask an administrator. Do not put the key in source control.", "body"),
        P("6. What you see from YP-Raksha while browsing", "h1"),
        P("You keep using ChatGPT, Claude, Gemini, and the other sites your company monitors. YP-Raksha checks the prompt before it is sent.", "body"),
        table(
            ["What you typed", "Company action", "What you see"],
            [
                ["Draft a polite reply to a customer.", "Allowed", "The message is sent. No card."],
                ["My PAN is ABCDE1234F", "Block", "The send stops. You see the warning text only."],
                ["The file contains live warn token", "Warn", "You see the warning. The original message is sent."],
                ["Reset token is live redact token", "Redact", "You see the warning. The matched text is sent as [REDACTED]."],
            ],
            [55 * mm, 32 * mm, 83 * mm],
        ),
        P("The warning card shows the warning text. It does not show the internal rule name.", "body"),
        P("7. If something fails", "h1"),
        table(
            ["What you see", "What to do"],
            [
                ["Waiting for admin approval", "Ask an admin to approve you under Governance → Users."],
                ["Verify your email first", "Enter the 6-digit code from Sign Up."],
                ["Too many failed attempts", "Wait the minutes shown, then try again."],
                ["Password reset email missing", "Check spam. Ask IT to confirm SMTP."],
                ["Page jumps somewhere else", "That page is not on your account. You were sent to an allowed page."],
                ["AI site shows a certificate warning", "Tell IT. Gateway Guard’s certificate is not trusted on that laptop yet."],
                ["YP-Raksha shows Remove or is Off", "That is not the managed install. Tell IT. Employees cannot turn the managed extension off."],
            ],
            [55 * mm, 115 * mm],
        ),
    ]
    return build("01_Website_User_Manual.pdf", "Website user manual", story)


def admin_manual():
    story = cover(
        "CLIENT PACK  ·  DOCUMENT 02",
        "Administrator operations manual",
        "Day-one setup, users, rules, and Browser AI",
        "Audience: platform administrators and security managers (admin and sub_admin).",
    )
    story += [
        P("1. First day", "h1"),
    ]
    story += bullets([
        "Sign in with the bootstrap admin (ADMIN_EMAIL and ADMIN_PASSWORD). Change that password. Create priya.admin for daily work.",
        "Settings → Security: save SMTP and send a test mail. Sign-up codes and password reset do not work before this.",
        "Settings → Client Settings: keep authentication required. Add the company website origin.",
        "Settings → Proxy: leave off unless the server must reach vendors through an office proxy. This is not the laptop Guard.",
        "Models → Model Providers: paste the vendor key. Then Model Catalog: enable the models.",
        "Governance: create the team, then the user, then the virtual key with a budget.",
        "Routing Rules, then Guardrails for API calls. Alert Channels only after a budget or circuit breaker exists to notify.",
        "Browser AI: Target Websites, then Guard Rules, then Setup. Confirm the laptop under Guard Agents before you judge Prompt Logs.",
        "Observability → LLM Logs after the first virtual-key call. MCP Catalog only if a tool server is required, then MCP Auth Config, then Tool Groups.",
        "Prompt Repository: save the prompt, then Prompt Deployments for production, staging, or preview.",
    ])
    story += [
        P("2. Create a user (filled example)", "h1"),
        P("Go to Governance → Users → Create user. Direct create skips the public sign-up queue.", "body"),
        table(
            ["Field", "Example", "Notes"],
            [
                ["Username", "anitha.user", "Used on the Sign In page."],
                ["Email", "anitha.user@example.com", "Used for reset codes and warning mail."],
                ["Password", "Anitha#User24", "Meet the password rule. Ask the person to change it."],
                ["Role", "user", "Use sub_admin or admin only when the job needs it."],
                ["Allowed sections", "prompt-repository", "Add browser-ai only for security staff."],
                ["Budget", "25", "Optional monthly cap in the company currency unit used by budgets."],
                ["Rate limit", "60", "Optional requests per minute."],
            ],
            [36 * mm, 55 * mm, 79 * mm],
        ),
        P("2.1 Approve a public sign-up", "h2"),
        P("Governance → Users. Filter for pending. Open anitha.user. Click Approve. Status becomes approved and the person can sign in. Reject sends the request back; the email can be used to sign up again.", "body"),
        P("2.2 Custom role example — Auditor", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Role name", "Auditor"],
                ["Data scope", "all-data for security review, or own-data for a personal view"],
                ["Permissions", "Logs: Read, View. AuditLogs: Read, View. Dashboard: Read, View."],
                ["Do not grant", "Users delete, provider keys, Settings, log delete."],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("Assign the role to meena.audit. The Users page itself stays limited to admin and sub_admin even if a custom role has the Users resource.", "body"),
        P("3. Virtual key (filled example)", "h1"),
        P("Governance → Virtual Keys → Create.", "body"),
        table(
            ["Field", "Example"],
            [
                ["Name", "Support bot — production"],
                ["Assigned user or team", "Support"],
                ["Budget", "200"],
                ["Rate limit", "120 requests per minute"],
                ["Models", "Only the models that team is allowed to call"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("Copy the key once and store it in the application secret store. Rotate it from the same page when someone leaves or the key leaks. A bulk rotate exists for a set of keys.", "body"),
        P("4. Model provider (filled example)", "h1"),
        P("Models → Model Providers. Only an admin can read or change master provider keys.", "body"),
        table(
            ["Field", "Example"],
            [
                ["Provider", "OpenAI"],
                ["Key name", "openai-prod-1"],
                ["API key", "Paste the vendor key. It is stored encrypted when the encryption key is set."],
                ["Timeout / retries", "Leave the company default unless the vendor needs a longer timeout."],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("After the key is saved, open Model Catalog and confirm the models you want are enabled. Set a pricing override if reports must match your contract price.", "body"),
        P("5. Browser AI — the control plane", "h1"),
        P("Workspace → Browser AI. These rules protect employees on public AI websites. They are separate from Guardrails, which protect API calls through the gateway.", "body"),
        P("5.1 Target website (filled example)", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Domain", "chatgpt.com"],
                ["Platform name", "ChatGPT"],
                ["Host role", "ui"],
                ["Monitored", "Yes"],
                ["Block entire website", "No"],
                ["Status", "MONITORED"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("Add the chat and file hosts for the same platform when the vendor uses more than one domain. Import columns: Domain, Platform Name, Host Role, Monitored, Block Entire Website.", "body"),
        P("5.2 Guard rule — Block (filled example)", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Rule name", "India PAN"],
                ["Rule type", "regex"],
                ["Pattern", "[A-Z]{5}[0-9]{4}[A-Z]"],
                ["Severity", "CRITICAL"],
                ["Action", "BLOCK"],
                ["Warning message", "Personal ID numbers cannot be sent to a public AI site."],
                ["Active", "TRUE"],
                ["Description", "Stops Indian PAN numbers."],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("5.3 Guard rule — Warn (filled example)", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Rule name", "live warn token"],
                ["Pattern", "live warn token"],
                ["Severity", "MEDIUM"],
                ["Action", "WARN"],
                ["Warning message", "Warning: confidential token detected"],
                ["Active", "TRUE"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("The employee sees the warning. The original message is still sent.", "body"),
        P("5.4 Guard rule — Redact (filled example)", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Rule name", "live redact token"],
                ["Pattern", "live redact token"],
                ["Severity", "HIGH"],
                ["Action", "REDACT"],
                ["Warning message", "REDACT live test"],
                ["Active", "TRUE"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("The employee sees the warning. The matched text is replaced with [REDACTED] and that text is what is sent.", "body"),
        P("Start a rollout with WARN and REDACT. Move a rule to BLOCK after you have checked Prompt Logs for false matches. AI bot rules are created in the screen, not by spreadsheet import. Test a bot rule before you set it active.", "body"),
        P("5.5 Company uninstall key", "h2"),
        P("Browser AI → Guard Agents → uninstall key. Example value to store in the IT vault: GK-EXAMPLE-2048-INDIA. Share it only with IT. An employee needs this key to remove Gateway Guard. Remote uninstall marks the laptop uninstall_pending, then uninstalled after the agent confirms.", "body"),
        P("5.6 Read a prompt log", "h2"),
        table(
            ["Filter", "Example"],
            [
                ["Date", "Today"],
                ["Action", "Blocked"],
                ["Search", "DESKTOP-EXAMPLE or the employee hostname"],
            ],
            W,
        ),
        Spacer(1, 3 * mm),
        P("Open the row. You see the prompt, the rule, the action, and the device. Status values: Allowed, Redacted, Warned, Blocked, SiteBlocked, Bot Answered.", "body"),
        P("6. Weekly admin routine", "h1"),
    ]
    story += bullets([
        "Approve or reject pending users.",
        "Browser AI → Prompt Logs: review Blocked and Redacted from the last seven days.",
        "Guard Agents: any laptop not seen for more than seven days is not protected.",
        "Virtual Keys: look for keys near their budget.",
        "Audit Logs: confirm administrator changes are recorded.",
        "Do not delete logs unless retention policy says so. Deletion is audited.",
        "Alert Channels: confirm the test message still arrives.",
    ])
    story += [
        P("7. Mail, office proxy, alerts, MCP auth, and prompt deploy", "h1"),
        P("Document 06 has every sidebar screen. These five sit beside that menu. Do them at the point in the order in section 1, not at the end of a project.", "body"),
        P("7.1 SMTP — Settings → Security", "h2"),
        table(
            ["Field", "Example"],
            [
                ["Enabled", "Yes"],
                ["Host", "smtp.example.com"],
                ["Port", "587"],
                ["Username", "mailer@example.com"],
                ["From email", "noreply@example.com"],
                ["From name", "UnifAI"],
                ["Use TLS", "Yes"],
                ["Notify on user create", "Yes"],
            ],
            W,
        ),
        Spacer(1, 2 * mm),
        P("Save, then send a test to priya.admin@example.com. Flow 1 and Flow 2 in document 04 need this test to succeed first.", "body"),
        P("7.2 Proxy — Settings → Proxy", "h2"),
        P("Turn the proxy on only when the UnifAI server cannot call the vendor directly. URL http://proxy.example.com:8080. Enable for inference yes. Enable for SCIM only if the identity provider must be reached through that proxy. Leave SOCKS5 and TCP unused. Gateway Guard on the laptop does not read this page.", "body"),
        P("7.3 Alert Channels — /workspace/alert-channels", "h2"),
        P("Add channel after Circuit Breaker or a virtual-key budget exists. Name Ops Slack. Type Slack. Paste the webhook URL. Enabled yes. Press test. Email type uses the SMTP server from section 7.1. The same incident is sent at most once every 10 minutes. Connectors still copy full logs. Alert Channels send the short incident only.", "body"),
        P("7.4 MCP Auth Config — /workspace/mcp-auth-config", "h2"),
        P("Add the server in MCP Catalog first. Then select docs-server. Header name Authorization. Header value the server token. Save headers. Per-user header keys Authorization when each person has their own credential. Auth Sessions shows the result.", "body"),
        P("7.5 Prompt Deployments — inside Prompt Repository", "h2"),
        P("Select Customer reply — refund tone. Save a version first. Environment production. Version 1. Deploy version. The application sends header x-uf-prompt-environment: production together with its virtual key.", "body"),
    ]
    return build("02_Administrator_Operations_Manual.pdf", "Administrator operations manual", story)


def technical():
    story = cover(
        "CLIENT PACK  ·  DOCUMENT 03",
        "Technical reference",
        "Architecture, ports, logins, and data paths",
        "Audience: architects, developers, and the team that runs the server.",
    )
    story += [
        P("1. Products in this repository", "h1"),
        P("One website and one database serve two jobs. UnifAI Gateway is the AI API and the admin website. Gateway Guard is the laptop agent. YP-Raksha is the extension the agent force-installs into Chromium browsers.", "body"),
        table(
            ["Path", "Role"],
            [
                ["transports/unifai-http", "Go HTTP server. Website, /api, and /v1 AI routes."],
                ["ui/", "React website embedded in the server."],
                ["core/ and framework/", "Providers, config store, logs, RBAC."],
                ["apps/browser-guard", "Gateway Guard agent, proxy, YP-Raksha extension, Windows setup."],
            ],
            [48 * mm, 122 * mm],
        ),
        P("2. Three runtime flows", "h1"),
        P("Flow A — employee browser. The browser loads a monitored AI site. The PAC file sends only those sites to the local proxy at 127.0.0.1:18103. The proxy evaluates guard rules. Block stops the send. Warn shows the message and sends the original text. Redact shows the message and sends the text with the match replaced by [REDACTED]. The agent posts the event to /api/browser-ai/intercept. The website shows it under Browser AI → Prompt Logs.", "body"),
        P("Flow B — application API. The application calls https://unifai.example.com/v1/chat/completions with Authorization: Bearer and a virtual key. The gateway authenticates the key, applies governance and guardrails, calls the configured provider, and writes an LLM log.", "body"),
        P("Flow C — website. The person signs in at /login. The server sets an HTTP-only session cookie named token. Later calls to /api use that cookie. Role and allowed sections decide which handler runs.", "body"),
        P("3. Ports", "h1"),
        table(
            ["Port", "Process", "Who binds it"],
            [
                ["443 / 80", "Public HTTPS and redirect", "Reverse proxy in front of UnifAI"],
                ["6000 or APP_PORT", "UnifAI server", "Server host"],
                ["5432", "PostgreSQL", "Database host"],
                ["127.0.0.1:18103", "Guard MITM proxy", "Each laptop"],
                ["127.0.0.1:18195", "Guard PAC and extension update", "Each laptop"],
            ],
            [38 * mm, 62 * mm, 70 * mm],
        ),
        P("4. Authentication", "h1"),
        table(
            ["Caller", "Credential", "Typical lifetime"],
            [
                ["Website user, any role", "Username + password, then cookie token", "About 24 hours"],
                ["AI application", "Virtual key, Bearer or x-api-key", "Until rotated or budget ends"],
                ["Gateway Guard agent", "Fleet secret on the intercept and heartbeat calls", "Installed with the agent"],
                ["SCIM", "Identity-provider token on /api/scim", "Set by the identity provider"],
            ],
            [42 * mm, 78 * mm, 50 * mm],
        ),
        P("Public registration always stores role user. Roles admin, sub_admin, user, and custom roles are assigned later by an administrator. System roles are seeded at startup. The admin role keeps the full permission catalog.", "body"),
        P("5. RBAC", "h1"),
        P("Every API check has two layers: a resource plus an operation, and the sidebar sections stored on the user. Operations are Read, View, Create, Update, Delete, and Download. Browser AI routes are gated by the Logs resource. A person who should manage Browser AI needs Logs access or the browser-ai section.", "body"),
        P("6. Laptop files (every employee, no personal name in the product)", "h1"),
        P("The product uses the Windows profile of whoever is logged in. It does not store a developer username.", "body"),
        table(
            ["Item", "Location"],
            [
                ["Guard program", "%LOCALAPPDATA%\\Programs\\Gateway\\Guard\\Gateway_Guard.exe"],
                ["Guard data and log", "%LOCALAPPDATA%\\Gateway\\Guard\\"],
                ["Extension package", "%LOCALAPPDATA%\\Gateway\\Guard\\guard_extension.crx"],
                ["YP-Raksha id", "mipfhcmlcjfhmgldkokdckhndkebkllj"],
            ],
            [48 * mm, 122 * mm],
        ),
        P("Managed install is a machine policy: ExtensionInstallForcelist plus ExtensionSettings with installation_mode force_installed and incognito spanning. The outside registry path under Software\\Google\\Chrome\\Extensions is not used. That path is what makes the browser show the extension as off, with Remove, and the text “not from any known source”.", "body"),
        P("7. Health checks an operator can run", "h1"),
    ]
    story += bullets([
        "GET /health on the UnifAI server returns 200.",
        "On a laptop, http://127.0.0.1:18195/extension/update.xml lists the current YP-Raksha version.",
        "Browser AI → Guard Agents shows the laptop hostname and a recent last-seen time.",
        "A test prompt that matches a BLOCK rule appears as Blocked in Prompt Logs.",
        "Settings → Security test mail returns success before any sign-up code is expected.",
    ])
    story += [
        P("8. Extra product paths used by the flows", "h1"),
        table(
            ["Path", "When the flow uses it"],
            [
                ["/workspace/config/security", "SMTP. Must succeed before sign-up and password reset."],
                ["/workspace/config/proxy", "Office proxy for vendor, API, or SCIM calls. Not the laptop Guard."],
                ["/workspace/alert-channels", "Webhook, Slack, email, or PagerDuty after a budget or circuit-breaker event."],
                ["/workspace/mcp-auth-config", "After MCP Catalog has a client. Before a tool call is trusted."],
                ["/workspace/prompt-repo", "Prompt Deployments. Client header x-uf-prompt-environment."],
            ],
            [55 * mm, 115 * mm],
        ),
    ]
    return build("03_Technical_Reference.pdf", "Technical reference", story)


def flows():
    story = cover(
        "CLIENT PACK  ·  DOCUMENT 04",
        "Process flows and sample data",
        "End-to-end steps with every field filled",
        "Audience: trainers, implementation leads, and the client project team.",
    )
    story += [
        P("Use these as scripts in a pilot. Replace example.com and the sample passwords before production. Do not start at Flow 1. SMTP and the provider must already be in place, or the later steps have nothing to send.", "body"),
        P("Flow 0 — Correct connection order", "h1"),
        table(
            ["Step", "Screen", "Sample result"],
            [
                ["1", "Settings → Security, SMTP", "Test mail arrives at priya.admin@example.com"],
                ["2", "Settings → Client Settings", "Auth required. Origin https://unifai.example.com"],
                ["3", "Settings → Proxy", "Off, unless the server needs http://proxy.example.com:8080 for vendor calls"],
                ["4", "Model Providers, then Model Catalog", "openai-prod-1 saved. gpt-4.1-mini enabled"],
                ["5", "Teams, Users, Virtual Keys", "Team Support. User anitha.user approved. Key Support bot — production"],
                ["6", "Routing Rules, then Guardrails", "Support uses gpt-4.1-mini. API PII rule on"],
                ["7", "One API call, then LLM Logs", "Hello returns. One success row for that key"],
                ["8", "Alert Channels", "Ops Slack test message arrives"],
                ["9", "Target Websites, Guard Rules, Setup, Guard Agents", "chatgpt.com monitored. PAN rule BLOCK. Laptop active"],
                ["10", "Prompt Logs", "PAN test Blocked. Normal sentence Allowed"],
                ["11", "MCP Catalog, then MCP Auth Config, then Tool Groups", "Only if a tool server is required"],
                ["12", "Prompt Repository, then Prompt Deployments", "production version 1. Client sends x-uf-prompt-environment: production"],
            ],
            [16 * mm, 62 * mm, 92 * mm],
        ),
        Spacer(1, 3 * mm),
        P("Flow 1 — New employee joins", "h1"),
        P("Start this only after Flow 0 step 1. If SMTP is not saved, the code in step 2 never arrives.", "body"),
        table(
            ["Step", "Who", "Action and sample data"],
            [
                ["1", "Employee", "Open /signup. Username anitha.user. Email anitha.user@example.com. Password Anitha#User24. Confirm the same password."],
                ["2", "System", "If SMTP is on, email a 6-digit code. Employee enters 481926. Status becomes pending."],
                ["3", "Admin", "Sign in as priya.admin. Governance → Users. Approve anitha.user."],
                ["4", "Employee", "Open /login. Username anitha.user. Password Anitha#User24. Land on the allowed page."],
                ["5", "Employee", "End of day: Sign out."],
            ],
            [16 * mm, 28 * mm, 126 * mm],
        ),
        P("Flow 2 — Employee forgot the password", "h1"),
        table(
            ["Step", "Sample data"],
            [
                ["1. Forgot password", "Email anitha.user@example.com"],
                ["2. Code from email", "390144 (valid 10 minutes)"],
                ["3. New password", "Anitha#Reset24 confirmed in both boxes"],
                ["4. Sign in", "Username anitha.user, password Anitha#Reset24"],
            ],
            [45 * mm, 125 * mm],
        ),
        P("Flow 3 — Administrator publishes a block rule and proves it", "h1"),
        table(
            ["Step", "Sample data"],
            [
                ["1. Sign in", "priya.admin / Priya#Admin24"],
                ["2. Target website", "Domain chatgpt.com, platform ChatGPT, role ui, monitored yes, block entire website no"],
                ["3. Rule", "Name India PAN. Type regex. Pattern [A-Z]{5}[0-9]{4}[A-Z]. Severity CRITICAL. Action BLOCK. Message: Personal ID numbers cannot be sent to a public AI site. Active TRUE."],
                ["4. Employee test", "On ChatGPT, type: My PAN is ABCDE1234F and try to send."],
                ["5. Expected result", "Send stops. Warning text appears. Prompt Logs shows Blocked, rule India PAN, user device hostname."],
                ["6. Allowed test", "Type: Draft a polite refund reply. Send succeeds. Prompt Logs shows Allowed."],
            ],
            [40 * mm, 130 * mm],
        ),
        P("Flow 4 — Warn and redact, same employee", "h1"),
        table(
            ["Prompt typed", "Action on the rule", "Result"],
            [
                ["Please check live warn token", "WARN, message Warning: confidential token detected", "Warning card. Original text is sent. Log action Warned."],
                ["Please check live redact token", "REDACT, message REDACT live test", "Warning card. Sent text contains [REDACTED]. Log action Redacted."],
            ],
            [52 * mm, 58 * mm, 60 * mm],
        ),
        P("Flow 5 — Give a developer API access", "h1"),
        P("The provider and catalog from Flow 0 step 4 already exist. This flow only issues the key and proves the log.", "body"),
        table(
            ["Step", "Sample data"],
            [
                ["1", "Team Support exists. Admin creates virtual key Support bot — production. Budget 200. Rate 120 per minute. Model gpt-4.1-mini."],
                ["2", "Admin copies the key once. Example: sk-unifai-example-7f3a9c"],
                ["3", "Developer calls POST /v1/chat/completions with Bearer that key, model gpt-4.1-mini, content Hello."],
                ["4", "Admin opens LLM Logs and sees the call, tokens, and cost under that key name."],
                ["5", "Optional. If the prompt is deployed, add header x-uf-prompt-environment: production on the same call."],
            ],
            [18 * mm, 152 * mm],
        ),
        P("Flow 6 — Install Gateway Guard on a laptop", "h1"),
        table(
            ["Step", "What happens"],
            [
                ["1", "Admin downloads the Windows package from Browser AI → Setup."],
                ["2", "IT runs Gateway_Guard_Setup.exe on the laptop. Leave start at Windows login checked."],
                ["3", "Guard starts. It writes machine policy so Chrome, Edge, and Brave force-install YP-Raksha and keep it on, including private windows."],
                ["4", "Admin opens Guard Agents and sees the laptop hostname with status active."],
                ["5", "Employee opens the browser. YP-Raksha is on. There is no Remove button on a correct policy install."],
            ],
            [18 * mm, 152 * mm],
        ),
        P("Flow 7 — Employee leaves the company", "h1"),
        table(
            ["Step", "Sample data"],
            [
                ["1", "Identity provider sets the person inactive, or admin sets the user disabled. Website login stops."],
                ["2", "Admin rotates or deletes virtual keys named for that person."],
                ["3", "Admin starts remote uninstall for that hostname, or IT enters the company uninstall key on the laptop."],
                ["4", "Agent status moves to uninstall_pending, then uninstalled. YP-Raksha policy is removed with the Guard."],
            ],
            [18 * mm, 152 * mm],
        ),
        P("Flow 8 — Investigate a suspected leak", "h1"),
    ]
    story += bullets([
        "Sign in as ravi.security (sub_admin).",
        "Browser AI → Prompt Logs. Date = the reported day. Search = the hostname or client IP.",
        "Open Blocked, Redacted, and Warned rows. Record the rule name, the prompt, and the time.",
        "If a warning email is required, send it from the log to the contact email stored on the agent.",
        "Do not delete the row. Audit logs should keep the investigation.",
    ])
    story += [
        P("Spreadsheet columns for a rule import", "h2"),
        P("Row 1 must be the header. AI bot rules cannot be imported.", "body"),
        Preformatted(
            "Rule Name,Rule Type,Pattern,Severity,Action,Warning Message,Active,Description\n"
            "India PAN,regex,[A-Z]{5}[0-9]{4}[A-Z],CRITICAL,BLOCK,Personal ID numbers cannot be sent to a public AI site.,TRUE,Stops PAN\n"
            "live warn token,regex,live warn token,MEDIUM,WARN,Warning: confidential token detected,TRUE,Pilot warn\n"
            "live redact token,regex,live redact token,HIGH,REDACT,REDACT live test,TRUE,Pilot redact",
            S["code"],
        ),
        P("Flow 9 — SMTP, then a password reset", "h2"),
        P("Settings → Security. Host smtp.example.com, port 587, TLS on, from noreply@example.com. Test to priya.admin@example.com. Only then run Flow 2. A reset code cannot be delivered before this test succeeds.", "body"),
        P("Flow 10 — Alert after a budget hit", "h2"),
        P("Virtual key budget is 200. Alert channel Ops Slack is enabled and the test succeeded. When spend reaches the budget, Slack receives one notice. A repeat of the same incident waits 10 minutes. The full request remains in LLM Logs.", "body"),
        P("Flow 11 — MCP tool call", "h2"),
        P("MCP Catalog: docs-server connected. MCP Auth Config: Authorization header saved. Tool group Support tools includes search_docs only and is attached to the Support virtual key. Feature flag mcp.tool_execution is on. The tool call appears in MCP Logs. Skip this flow when the pilot has no tool server.", "body"),
        P("Flow 12 — Deploy a prompt", "h2"),
        P("Prompt Repository: Customer reply — refund tone, version 1. Deployments: environment production, enabled. Application header x-uf-prompt-environment: production plus the virtual key. The model call still follows Flow 5.", "body"),
    ]
    return build("04_Process_Flows_and_Sample_Data.pdf", "Process flows and sample data", story)


def employee_guard():
    story = cover(
        "CLIENT PACK  ·  DOCUMENT 05",
        "Gateway Guard and YP-Raksha",
        "What is installed on each laptop and how to use it",
        "Audience: employees and the IT team that rolls the agent out.",
    )
    story += [
        P("1. What is on the laptop", "h1"),
        P("Gateway Guard is a company security program. It starts when Windows starts. YP-Raksha is the browser piece. On a correct install it is turned on in Chrome, Edge, and Brave, including private windows. The employee does not download it from a store and does not get a Remove button.", "body"),
        P("Safari on Windows is not a target for this extension. The Guard proxy still covers monitored sites when the browser uses the company PAC.", "body"),
        P("2. Install (IT)", "h1"),
    ]
    story += bullets([
        "Take Gateway_Guard_Setup.exe from Browser AI → Setup, or from the release package IT already built.",
        "Run the setup on the laptop. Keep start at Windows login checked.",
        "Approve the one-time administrator prompt if Windows asks. That prompt writes the machine policy.",
        "Do not ask each employee to open chrome://extensions and click Load unpacked.",
    ])
    story += [
        P("3. What a correct extension looks like", "h1"),
        table(
            ["Check", "Correct", "Not correct — call IT"],
            [
                ["State", "On, and the employee cannot switch it off", "Off, with a switch"],
                ["Remove", "No Remove button", "Remove button or a trash icon"],
                ["Source text", "Managed by the organisation", "Not from any known source / not listed in the Chrome Web Store"],
                ["Private windows", "Allowed, and locked on", "Allow in InPrivate switched off"],
            ],
            [32 * mm, 68 * mm, 70 * mm],
        ),
        P("4. Uninstall", "h1"),
        P("Only IT can remove it. Windows Settings → Apps → Gateway Guard → Uninstall, then enter the company uninstall key. A blank key works only if the company turned that requirement off. Remote uninstall from Browser AI → Guard Agents does the same job without a desk visit.", "body"),
        P("5. Files IT may ask for", "h1"),
        P("Log and data live under %LOCALAPPDATA%\\Gateway\\Guard\\. That folder is different on every laptop because Windows uses the signed-in profile. The product does not contain a person’s name.", "body"),
        P("6. Employee rules of use", "h1"),
    ]
    story += bullets([
        "Use AI sites as usual. Do not try to disable YP-Raksha or Gateway Guard.",
        "If a warning appears, read it. Block means the message was not sent. Warn means it was sent. Redact means the sensitive part was replaced with [REDACTED] and then sent.",
        "Do not type government IDs, card numbers, passwords, API keys, or private keys into a public AI site.",
        "Sign out of the UnifAI website on a shared PC. That sign-out does not remove the Guard.",
    ])
    return build("05_Gateway_Guard_and_YP-Raksha.pdf", "Gateway Guard and YP-Raksha", story)


def index_doc(files):
    story = cover(
        "CLIENT PACK  ·  DOCUMENT 00",
        "Documentation index",
        "Every document to hand to the client",
        "Audience: the client project owner. Hand over this folder as the full set.",
    )
    rows = []
    for name, who, what in files:
        rows.append([name, who, what])
    story += [
        P("Give the client this folder. Each PDF stands alone. Sample passwords inside are for training and must be replaced.", "body"),
        table(
            ["File", "Give it to", "What it answers"],
            rows,
            [62 * mm, 38 * mm, 70 * mm],
        ),
        Spacer(1, 6 * mm),
        P("How to read them", "h1"),
    ]
    story += bullets([
        "Every PDF in this folder contains the same connection flow and every website section, with the inputs, how to use it, and how it connects.",
        "Project sponsor: this index, then document 04. Flow 0 is the order. Do not start at Flow 1.",
        "Every employee: document 01 and document 05.",
        "Administrators: document 02 section 1 is the same order as document 04 Flow 0. Document 06 is the field detail for every screen, including SMTP, Proxy, Alert Channels, MCP Auth Config, and Prompt Deployments.",
        "Technical team: document 03, including the extra paths in section 8.",
    ])
    story += [
        P("Product names used in this pack", "h2"),
        table(
            ["Name", "Meaning"],
            [
                ["UnifAI", "The website and the AI gateway."],
                ["Gateway Guard", "The program installed on each laptop."],
                ["YP-Raksha", "The browser extension Gateway Guard turns on."],
            ],
            [40 * mm, 130 * mm],
        ),
    ]
    return build("00_Documentation_Index.pdf", "Documentation index", story)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    made = [
        website_manual(),
        admin_manual(),
        technical(),
        flows(),
        employee_guard(),
    ]
    catalog = [
        ("00_Documentation_Index.pdf", "Project owner", "List of the pack and who should read which file."),
        ("01_Website_User_Manual.pdf", "All website users", "Sign up, every login type, sign out, menus, and how to fill the main screens."),
        ("02_Administrator_Operations_Manual.pdf", "Admins", "Users, roles, virtual keys, providers, Browser AI rules, uninstall key."),
        ("03_Technical_Reference.pdf", "Technical team", "Architecture, ports, authentication, RBAC, laptop paths."),
        ("04_Process_Flows_and_Sample_Data.pdf", "Trainers", "Join, reset, block, warn, redact, API key, install, leaver, investigation."),
        ("05_Gateway_Guard_and_YP-Raksha.pdf", "Employees and IT", "What a correct install looks like, and how uninstall works."),
        ("06_Every_Website_Screen.pdf", "Admins and trainers", "Every menu: what to do, what it connects to, and a filled example."),
        ("07_Scope_Specification.pdf", "QA & Auditors", "Formal product scope, testable requirement statements, and verification criteria."),
        ("08_Enterprise_Connected_System_Flows.pdf", "Architects, Admins & Leads", "Master system flow, why each entity exists, how User-Team-Customer-Key-Providers connect, and full dashboard click-map."),
        ("09_Non_Technical_Business_Guide.pdf", "Executives & Business Owners", "Plain English & everyday analogies guide: What work each part does, zero technical jargon, and manager cheat sheet."),
    ]
    made.insert(0, index_doc(catalog))
    for path in made:
        print(path.name, path.stat().st_size)


if __name__ == "__main__":
    main()
