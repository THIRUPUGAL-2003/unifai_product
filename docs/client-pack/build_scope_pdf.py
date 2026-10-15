#!/usr/bin/env python3
"""Product scope specification. Each requirement is written so a test case can be copied from it."""

from pathlib import Path

from reportlab.lib import colors
from reportlab.lib.enums import TA_JUSTIFY
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import mm
from reportlab.platypus import (
    HRFlowable,
    PageBreak,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)

OUT = Path(__file__).resolve().parent
NAVY = colors.HexColor("#0f2744")
TEAL = colors.HexColor("#0e7490")
RULE = colors.HexColor("#d6dde6")
ROW = colors.HexColor("#f4f7fb")


def styles():
    return {
        "kicker": ParagraphStyle("kicker", fontName="Times-Bold", fontSize=11, textColor=TEAL, spaceAfter=6),
        "title": ParagraphStyle("title", fontName="Times-Bold", fontSize=22, leading=26, textColor=NAVY, spaceAfter=6),
        "sub": ParagraphStyle("sub", fontName="Times-Roman", fontSize=11, leading=14, textColor=colors.HexColor("#475569"), spaceAfter=4),
        "h1": ParagraphStyle("h1", fontName="Times-Bold", fontSize=14, leading=18, textColor=NAVY, spaceBefore=10, spaceAfter=4),
        "h2": ParagraphStyle("h2", fontName="Times-Bold", fontSize=12, leading=15, textColor=TEAL, spaceBefore=8, spaceAfter=3),
        "body": ParagraphStyle("body", fontName="Times-Roman", fontSize=9.5, leading=12.5, alignment=TA_JUSTIFY, textColor=colors.HexColor("#1e293b"), spaceAfter=3),
        "cell": ParagraphStyle("cell", fontName="Times-Roman", fontSize=7.5, leading=9.5, textColor=colors.HexColor("#1e293b")),
        "cellb": ParagraphStyle("cellb", fontName="Times-Bold", fontSize=7.5, leading=9.5, textColor=colors.white),
    }


ST = styles()


def P(text, style="body"):
    return Paragraph(text, ST[style])


def table(headers, rows, widths):
    data = [[Paragraph(h, ST["cellb"]) for h in headers]]
    for row in rows:
        data.append([Paragraph(str(c), ST["cell"]) for c in row])
    t = Table(data, colWidths=widths, repeatRows=1)
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), NAVY),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, ROW]),
        ("GRID", (0, 0), (-1, -1), 0.25, RULE),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 2),
        ("RIGHTPADDING", (0, 0), (-1, -1), 2),
        ("TOPPADDING", (0, 0), (-1, -1), 2),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 2),
    ]))
    return t


def header_footer(canvas, doc):
    canvas.saveState()
    canvas.setFillColor(NAVY)
    canvas.rect(0, A4[1] - 11 * mm, A4[0], 11 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Times-Bold", 8)
    canvas.drawString(12 * mm, A4[1] - 7 * mm, "UnifAI  ·  Scope specification")
    canvas.setFont("Times-Roman", 8)
    canvas.drawRightString(A4[0] - 12 * mm, A4[1] - 7 * mm, "Source for test cases and the user manual")
    canvas.setFillColor(TEAL)
    canvas.rect(0, 0, A4[0], 8 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Times-Roman", 8)
    canvas.drawString(12 * mm, 3 * mm, "Requirement IDs are stable. Sample data is not a live secret.")
    canvas.drawRightString(A4[0] - 12 * mm, 3 * mm, f"Page {doc.page}")
    canvas.restoreState()


# ID, requirement, actor, preconditions, inputs, expected result
REQS = [
    ("1. Access and accounts", [
        ["AUTH-01", "A person signs in with username and password.", "Any role", "Account status is approved.", "Username anitha.user. Password meeting the password rule.", "Session cookie is set. The user lands on the first allowed page. Session lasts about 24 hours."],
        ["AUTH-02", "Sign-up always creates a user role and waits for approval.", "New employee", "SMTP test has succeeded if email codes are on.", "Username, email, password, confirm password.", "Status becomes pending, or email_unverified until the 6-digit code is entered. Sign-in is refused until an admin approves."],
        ["AUTH-03", "Password must meet the rule.", "Any person setting a password", "Sign-up, reset, or force-change screen.", "Shorter than 8, or missing upper, lower, number, or symbol.", "The form rejects the password and names the missing rule. No account change is saved."],
        ["AUTH-04", "Forgot password sends a code and then sets a new password.", "Approved user", "SMTP test succeeded.", "Email anitha.user@example.com. Code within 10 minutes. New password.", "Old password stops working. New password signs in."],
        ["AUTH-05", "Forgot username emails the username.", "Approved user", "SMTP test succeeded.", "Email on the account.", "The email contains the username. It does not contain the password."],
        ["AUTH-06", "Too many failed sign-ins lock the attempt.", "Any person", "Lockout is on in Security.", "Repeated wrong passwords.", "The screen states how many minutes remain. A correct password during the lock does not sign in."],
        ["AUTH-07", "Sign out ends the website session only.", "Signed-in user", "A session exists.", "Account menu, Sign out.", "The login page opens. Opening a workspace URL asks for sign-in. Gateway Guard and YP-Raksha stay on."],
        ["AUTH-08", "Pending, rejected, unverified, and disabled accounts cannot sign in.", "Those users", "User row is in that status.", "Correct password.", "Sign-in shows the matching reason. No workspace menu is shown."],
        ["AUTH-09", "A menu the account was not granted does not open.", "user or custom role", "Allowed sections omit that menu.", "Direct URL of a forbidden page.", "The site opens the first allowed page. It does not show the forbidden data."],
        ["AUTH-10", "Admin, sub_admin, user, custom role, and SCIM user share one Sign In page.", "Each actor", "Accounts exist.", "Each sample account.", "admin sees provider keys and Settings. sub_admin does not see master provider keys. user sees only granted sections. An inactive SCIM user is disabled."],
    ]),
    ("2. Governance", [
        ["GOV-01", "An admin approves or rejects a pending sign-up.", "admin or sub_admin", "User status is pending.", "Approve, or Reject.", "Approved can sign in. Rejected cannot. The email can be used to sign up again after reject."],
        ["GOV-02", "An admin creates a user directly.", "admin or sub_admin", "Signed in.", "Username, email, password, role, allowed sections, optional budget and rate.", "The user can sign in without the public queue. The menu matches allowed sections."],
        ["GOV-03", "A team holds members and one budget.", "admin or sub_admin", "Users exist.", "Name Support. Member anitha.user. Budget 200. Rate 120 per minute.", "The team can be selected on a virtual key and on a routing rule."],
        ["GOV-04", "A business unit budget stops every team under it.", "admin or sub_admin", "Team exists.", "Unit Customer Operations. Team Support. Budget 2000.", "A call stops when the unit ceiling is hit even if the team budget remains. LLM Logs shows the limit error."],
        ["GOV-05", "A customer groups teams for reporting.", "admin or sub_admin", "Team exists.", "Name Northwind. Team Support.", "The customer sheet shows the linked team. Calls still use a virtual key."],
        ["GOV-06", "A virtual key is the application secret.", "admin or sub_admin", "Team and an enabled model exist.", "Name, team, budget 200, rate 120, model gpt-4.1-mini.", "The key is shown once. Bearer calls to /v1/chat/completions succeed until budget, rate, or revoke."],
        ["GOV-07", "A virtual key over budget or rate is refused.", "Application", "Key exists with a low ceiling.", "Calls until the ceiling.", "The call returns an error. LLM Logs records it. Alert Channels can notify once per 10 minutes."],
        ["GOV-08", "Rotating a virtual key invalidates the old value.", "admin or sub_admin", "A key is in use.", "Rotate.", "Old Bearer value fails. New value succeeds."],
        ["GOV-09", "A custom role limits API operations.", "admin", "Signed in.", "Role Auditor. Logs and Audit Logs read and view only.", "That user can read logs and cannot delete users, change provider keys, or open Settings."],
        ["GOV-10", "An access profile applies one budget and model set to many users.", "admin or sub_admin", "Users exist.", "Profile Contractor. Budget 25. Model gpt-4.1-mini. Apply.", "Those users’ keys follow the profile."],
        ["GOV-11", "SCIM creates and disables users from the identity provider.", "Identity admin", "SCIM URL and token copied from User Provisioning.", "Provision a user. Then set the person inactive.", "A Users row appears. Inactive becomes status disabled and sign-in fails."],
        ["GOV-12", "Audit Logs record configuration changes.", "admin or sub_admin", "A change was just saved.", "Filter user priya.admin, today.", "The create or update appears. The row is not typed by hand."],
    ]),
    ("3. Models, routing, and guardrails", [
        ["MOD-01", "A model provider stores the vendor key.", "admin", "A vendor account exists.", "Provider OpenAI. Key name openai-prod-1. Vendor API key.", "Model Catalog lists models from that vendor. The virtual key never contains this secret."],
        ["MOD-02", "Only enabled catalog models can be chosen.", "admin", "Provider key works.", "Enable gpt-4.1-mini. Disable another model.", "Routing and virtual keys can use the enabled model. A call to a disabled model is refused."],
        ["MOD-03", "A model-level budget is separate from the virtual-key budget.", "admin", "Model is enabled.", "Budget 500. Rate 60 per minute on gpt-4.1-mini.", "The first ceiling reached stops the call."],
        ["MOD-04", "A pricing override replaces the default price in reports.", "admin", "Model exists.", "Input and output contract prices.", "Dashboard and LLM Logs use the override. Recalculate cost after a change if history must match."],
        ["MOD-05", "Model settings supply default parameters.", "admin", "Model exists.", "Temperature 0.2.", "A call that omits temperature uses 0.2. A client value still wins."],
        ["MOD-06", "A routing rule forces a model for a team.", "admin", "Team Support and the model exist.", "If team is Support, model is gpt-4.1-mini.", "A Support virtual-key call shows that model in LLM Logs."],
        ["MOD-07", "Complexity router sends easy prompts to a smaller model.", "admin", "Both models enabled. Flag routing.complexity on.", "Low model gpt-4.1-mini. High model gpt-4.1.", "A short hello is logged on the low model."],
        ["MOD-08", "Circuit breaker pauses a failing provider.", "admin", "Two provider keys. Flag routing.circuit_breaker on.", "Open after 5 failures. Retry after 30 seconds.", "Further calls skip the failing key while the breaker is open."],
        ["MOD-09", "Adaptive routing splits weight across healthy keys.", "admin", "Two keys for the same provider.", "Weights 70 and 30.", "The adaptive dashboard shows the split. A routing rule that names one provider still wins."],
        ["MOD-10", "Guardrail rules inspect API text, not browser text.", "admin", "Rule provider saved if the rule needs one.", "PII rule set to redact or block.", "A virtual-key call containing a card number is blocked or redacted. Browser AI Prompt Logs do not gain that row. LLM Logs do."],
        ["MOD-11", "Cluster config is unused on a single server.", "admin", "One UnifAI server.", "No peers.", "The server runs. A second node is added only with the same cluster secret."],
    ]),
    ("4. Observability and settings", [
        ["OBS-01", "The observability dashboard totals API usage.", "admin or granted user", "At least one successful virtual-key call.", "Date range last 7 days.", "Requests, tokens, cost, and latency are shown. Cost follows pricing."],
        ["OBS-02", "LLM Logs show one row per API call.", "admin or granted user", "A call was made.", "Filter by key Support bot — production.", "The row has model, tokens, cost, latency, and status. A provider 401 is visible as an error."],
        ["OBS-03", "Logs settings control stored prompt text and retention.", "admin", "Signed in to Settings.", "Content logging on or off. Retention 30 days.", "New LLM rows follow the choice. Browser Prompt Logs are not changed by this page."],
        ["OBS-04", "A connector copies logs to an external system.", "admin", "LLM Logs has rows.", "One of Datadog, BigQuery, Kafka, Pub/Sub, New Relic, Prometheus, OpenTelemetry, with that vendor’s URL and key.", "A new call appears in LLM Logs and in the external system."],
        ["OBS-05", "An alert channel sends a short incident.", "admin", "A budget or circuit breaker can fire. SMTP tested if type is email.", "Name Ops Slack. Type Slack. Webhook URL. Enabled.", "Test message arrives. A real incident is sent at most once every 10 minutes."],
        ["SET-01", "SMTP must accept a test before any product email is expected.", "admin", "Mail server credentials.", "Host smtp.example.com. Port 587. TLS on. From noreply@example.com.", "Test to priya.admin@example.com arrives. Sign-up and reset codes fail closed until this passes."],
        ["SET-02", "Client settings require auth and limit origins.", "admin", "Signed in.", "Origin https://unifai.example.com. Auth required on.", "A call without a virtual key is refused. A browser origin not on the list is refused."],
        ["SET-03", "The office proxy is optional and is not the laptop Guard.", "admin", "Server cannot reach the vendor directly, or it can.", "Proxy off, or URL http://proxy.example.com:8080 and inference on.", "With proxy off, vendor calls go direct. With proxy on, a test chat still succeeds and appears in LLM Logs."],
        ["SET-04", "Caching returns a stored answer only when the flag is on.", "admin", "cache.responses on. Direct cache on. TTL 10 minutes.", "The same request twice.", "The second call does not need the provider. LLM Logs shows the cache. Flag off sends every call to the provider."],
        ["SET-05", "Feature flags gate the feature they name.", "admin", "Flags saved.", "browser.prompt_guard off, then on.", "Off: laptop prompts are not stored in Prompt Logs. On: a matching prompt is stored."],
        ["SET-06", "Compatibility converts client parameters for a non-matching provider.", "admin", "An OpenAI-style client calls another vendor through the gateway.", "Conversion on.", "The call is accepted in the client’s shape and answered by the routed provider."],
    ]),
    ("5. Browser AI and the laptop", [
        ["BAI-01", "A target website is the only domain the laptop inspects.", "admin", "Signed in to Browser AI.", "Domain chatgpt.com. Platform ChatGPT. Host role ui. Monitored yes. Block entire website no.", "A prompt on that domain can hit a rule. A domain not on the list is not inspected by a rule."],
        ["BAI-02", "BLOCK stops the send and shows only the warning text.", "Employee on a monitored site", "Rule India PAN active. Pattern for a PAN. Laptop agent active.", "Prompt: My PAN is ABCDE1234F.", "The message is not sent. The warning text is shown. The rule name is not shown. Prompt Logs action is Blocked."],
        ["BAI-03", "WARN shows the warning and sends the original text.", "Employee", "WARN rule for the text live warn token.", "Prompt contains live warn token.", "Warning is shown. Original text is sent. Prompt Logs action is Warned."],
        ["BAI-04", "REDACT shows the warning and sends [REDACTED] for the match.", "Employee", "REDACT rule for live redact token.", "Prompt contains live redact token.", "Warning is shown. Sent text contains [REDACTED]. Prompt Logs action is Redacted."],
        ["BAI-05", "An allowed prompt is sent and logged.", "Employee", "No rule matches.", "Draft a polite refund reply.", "The site receives the text. Prompt Logs action is Allowed."],
        ["BAI-06", "Block entire website stops the site.", "admin then employee", "Target row with block entire website yes.", "Open that site.", "The site is blocked. Prompt Logs can show SiteBlocked."],
        ["BAI-07", "Block all uploads stops every attachment on monitored sites.", "admin then employee", "File upload policy on. browser.file_scan on.", "Upload any small file in a monitored chat.", "The upload stops. The upload warning is shown."],
        ["BAI-08", "Search logs record queries, including private windows.", "Employee", "Agent active.", "Search on a monitored engine, including a private window.", "Search Logs shows engine, browser, query, and the private-window flag."],
        ["BAI-09", "A laptop appears under Guard Agents after setup.", "IT", "Package from Browser AI → Setup.", "Run Gateway_Guard_Setup.exe. Start at login checked. Approve the admin prompt.", "The hostname appears as endpoint, status active, with a recent last-seen time."],
        ["BAI-10", "YP-Raksha is on, managed, and has no Remove button.", "Employee browser", "BAI-09 succeeded and the outside path install is absent.", "Open the browser extensions page.", "Extension id mipfhcmlcjfhmgldkokdckhndkebkllj is on, including private windows. There is no Remove control and no unknown-source warning."],
        ["BAI-11", "An outside path install is a failed install.", "Employee browser", "Registry path install exists.", "Open extensions.", "The extension can show Off, Remove, and not from a known source. This does not meet BAI-10."],
        ["BAI-12", "Uninstall requires the company key or a remote command.", "IT", "Agent active. Key stored by IT.", "Uninstall from Apps and enter the key, or remote uninstall.", "Status moves to uninstall_pending, then uninstalled. The extension policy is removed with the Guard."],
        ["BAI-13", "A warning email uses the agent contact and SMTP.", "admin", "Prompt log row. Contact email on the agent. SMTP tested.", "Send warning from the log.", "The contact receives the mail. The log row remains."],
        ["BAI-14", "Guard Insights totals actions per agent.", "admin", "Prompt logs exist.", "Open Guard Insights.", "Blocked, warned, redacted, and allowed totals match the logs for that hostname."],
        ["BAI-15", "AI bot rules are created on screen, not by spreadsheet.", "admin", "An LLM is available for the bot.", "Create an ai_bot rule and test it before Active.", "Import of an ai_bot row is rejected. A tested regex import still works."],
    ]),
    ("6. MCP, prompts, and skills", [
        ["MCP-01", "MCP Catalog connects a server.", "admin", "Server URL is reachable.", "Name docs-server. URL of the server.", "Status becomes connected. MCP Library alone does not connect."],
        ["MCP-02", "MCP Auth Config stores the header after the catalog client exists.", "admin", "Catalog client exists.", "Header Authorization and the token. Optional per-user header keys.", "A later tool call uses that header. Auth Sessions can show success."],
        ["MCP-03", "A tool group limits which tools a key may call.", "admin", "Catalog connected.", "Group Support tools. Tool search_docs only. Attach the Support key.", "That key can call search_docs. It cannot call a tool left unticked. The call appears in MCP Logs when mcp.tool_execution is on."],
        ["MCP-04", "An OAuth grant can be revoked.", "admin", "A grant exists.", "Revoke anitha.user on docs-server.", "That user no longer has consent. New tool calls that need the grant fail."],
        ["PR-01", "A prompt is saved in a folder.", "Granted user", "Prompt Repository is allowed.", "Folder Support. Name. Text. Tags.", "The prompt is listed for people who may see that folder."],
        ["PR-02", "A deployment publishes one version to an environment.", "admin", "A prompt version exists.", "Environment production. Version 1. Enabled.", "A call with header x-uf-prompt-environment: production and a virtual key uses that version."],
        ["PR-03", "The playground sends a trial message through the gateway.", "Signed-in user with model access", "A model is enabled.", "Model gpt-4.1-mini. Message Hello.", "An answer returns. LLM Logs has the row. The laptop Guard is not involved."],
        ["PR-04", "A skill is stored text for assistants.", "Granted user", "Skills Repository is allowed.", "Name and body.", "Clients that call the skills API can read it. It does not route a model and it is not a guard rule."],
    ]),
]


def main():
    story = [
        Spacer(1, 16 * mm),
        P("SCOPE SPECIFICATION", "kicker"),
        P("UnifAI, Gateway Guard, and YP-Raksha", "title"),
        P("This is the scope the team builds, tests, and writes manuals from. Each row is one requirement. A test case uses the same ID, the inputs, and the expected result.", "sub"),
        HRFlowable(width="100%", thickness=1, color=TEAL, spaceAfter=6),
        P("1. How to use this document", "h1"),
        P("Write one test case per requirement ID. Title the test with the ID. Steps are the inputs column, done in order. Pass means the expected result happened. Fail means it did not. Add a second test for each negative result already written in the expected column. The user manual uses the same ID as a heading, then the inputs as the fields the reader fills, then the expected result as what they should see.", "body"),
        P("Sample people used everywhere: admin priya.admin, sub_admin ravi.security, user anitha.user, auditor meena.audit. Website example https://unifai.example.com. Replace samples before a production test.", "body"),
        P("2. Product in one paragraph", "h1"),
        P("UnifAI is the website and the AI gateway. Applications call it with a virtual key. Employees sign in for prompts, logs, and administration. Gateway Guard is the program on each laptop. YP-Raksha is the browser extension that Guard turns on. Guard checks prompts on listed public AI sites and records the result. The gateway checks API calls with guardrails, budgets, and routing.", "body"),
        P("3. In scope", "h1"),
        table(
            ["Area", "In scope for this release"],
            [
                ["Website access", "Sign up, sign in, sign out, forgot password, forgot username, lockout, roles, allowed sections, SCIM disable."],
                ["Governance", "Users, teams, business units, customers, virtual keys, roles, access profiles, audit."],
                ["Models", "Providers, catalog, budgets, pricing, routing rules, complexity router, circuit breaker, adaptive routing."],
                ["API safety", "Guardrails and rule providers. Separate from browser rules."],
                ["Observability", "Dashboard, LLM logs, MCP logs, connectors, log settings, alert channels."],
                ["Browser AI", "Targets, rules, prompt logs, search logs, agents, insights, setup, upload policy, warning email."],
                ["Laptop", "Gateway Guard install, YP-Raksha force-install, uninstall only with the company key or remote uninstall."],
                ["MCP and content", "Catalog, library, auth, tool groups, sessions, grants, prompts, deployments, skills, playground."],
                ["Settings", "Client, compatibility, caching, security and SMTP, proxy, API keys, performance, feature flags."],
            ],
            [38 * mm, 146 * mm],
        ),
        P("4. Out of scope", "h1"),
        table(
            ["Item", "Why it is out"],
            [
                ["Safari extension on Windows", "This extension is for Chromium browsers. Safari is not a YP-Raksha target. The proxy can still cover a site that uses the PAC."],
                ["A public browser-store listing", "YP-Raksha is force-installed by company policy. It is not published as a store item in this scope."],
                ["Employee removal of YP-Raksha", "A correct install has no Remove button. Removal is IT only."],
                ["Replacing the vendor contract", "The client brings the OpenAI, Anthropic, Azure, or other key. This product stores and routes it."],
                ["Writing the identity provider itself", "SCIM connects to Azure AD or Okta. Those products are outside this system."],
            ],
            [50 * mm, 134 * mm],
        ),
        P("5. Actors", "h1"),
        table(
            ["Actor", "Can", "Cannot"],
            [
                ["admin", "All menus, vendor keys, settings, users.", "Nothing in this list is withheld."],
                ["sub_admin", "Users, keys, budgets, logs, Browser AI.", "Master provider keys and system settings."],
                ["user", "Menus granted in allowed sections.", "Any menu not granted, even by typing the URL."],
                ["Auditor custom role", "Read logs, audit, dashboard, as granted.", "Delete, provider keys, settings."],
                ["Application", "Call /v1 with a virtual key.", "Sign in to the website with that key."],
                ["Employee on a laptop", "Use monitored AI sites. Read a block, warn, or redact card.", "Turn off YP-Raksha or uninstall Guard without the company key."],
                ["IT operator", "Install Guard, hold the uninstall key, remote uninstall.", "Skip SMTP and still expect email codes."],
            ],
            [36 * mm, 74 * mm, 74 * mm],
        ),
        P("6. Order that must already be true", "h1"),
        P("Tests that need email start only after SET-01 passes. Tests that need a model start only after MOD-01 and MOD-02 pass. Browser tests start only after BAI-01 and BAI-09 pass. MCP tests start only after MCP-01. A failed earlier ID blocks the later ID. Do not file the later test as a product defect until the earlier ID passes.", "body"),
        table(
            ["Order", "IDs that must pass first", "What becomes possible"],
            [
                ["1", "SET-01", "AUTH-02, AUTH-04, AUTH-05, BAI-13, email alert channels"],
                ["2", "SET-02", "API calls must carry a virtual key"],
                ["3", "MOD-01, MOD-02", "GOV-06, MOD-06, PR-03, OBS-01, OBS-02"],
                ["4", "GOV-03, GOV-06", "Application chat and budget tests"],
                ["5", "MOD-10", "API PII block or redact"],
                ["6", "BAI-01, BAI-09, BAI-10", "BAI-02, BAI-03, BAI-04, BAI-05"],
                ["7", "MCP-01, MCP-02, MCP-03", "MCP Logs"],
                ["8", "PR-01, PR-02", "Environment header on a chat call"],
            ],
            [16 * mm, 52 * mm, 116 * mm],
        ),
    ]
    for title, rows in REQS:
        story.append(PageBreak())
        story.append(P(title, "h1"))
        story.append(P("Copy the ID into the test-case ID and into the user-manual section number.", "body"))
        story.append(table(
            ["ID", "Requirement", "Actor", "Before you start", "Inputs", "Expected result"],
            rows,
            [16 * mm, 34 * mm, 22 * mm, 32 * mm, 40 * mm, 40 * mm],
        ))
        story.append(Spacer(1, 2 * mm))

    story += [
        P("7. End-to-end flows the manual must tell in this order", "h1"),
        table(
            ["Flow", "Requirement IDs", "Manual chapter"],
            [
                ["Bring the platform up", "SET-01, SET-02, MOD-01, MOD-02, GOV-03, GOV-02, GOV-06", "Getting started"],
                ["Employee joins", "AUTH-02, GOV-01, AUTH-01, AUTH-07", "Your account"],
                ["Employee cannot sign in", "AUTH-08, AUTH-06, AUTH-04", "Account help"],
                ["Application calls a model", "GOV-06, MOD-06, OBS-02", "Using the API"],
                ["Spend or failures", "GOV-07, MOD-03, MOD-08, OBS-05", "Limits and alerts"],
                ["API text is unsafe", "MOD-10", "Guardrails"],
                ["Laptop is protected", "BAI-01, BAI-09, BAI-10", "Gateway Guard"],
                ["Browser prompt outcomes", "BAI-02, BAI-03, BAI-04, BAI-05", "What you see on an AI site"],
                ["Someone leaves", "GOV-11 or GOV-02 disable, GOV-08, BAI-12", "Leavers"],
                ["Tools", "MCP-01, MCP-02, MCP-03", "MCP"],
                ["Saved prompts", "PR-01, PR-02, PR-03, PR-04", "Prompts and skills"],
            ],
            [46 * mm, 69 * mm, 69 * mm],
        ),
        P("8. Non-functional requirements", "h1"),
        table(
            ["ID", "Requirement", "How to test"],
            [
                ["NFR-01", "Website session cookie is HTTP-only.", "After AUTH-01, confirm the token cookie is HTTP-only and, on HTTPS, Secure."],
                ["NFR-02", "Vendor keys and virtual keys are not written into the user manual or into logs meant for employees.", "Search the manual and a user-role screen for a vendor key. It must be absent."],
                ["NFR-03", "A user cannot read another team’s hidden prompt folder.", "Sign in as a user limited to one folder. The other folder is absent."],
                ["NFR-04", "Guard data stays on that laptop’s own Windows profile.", "Two Windows users on one PC have separate %LOCALAPPDATA%\\Gateway\\Guard folders."],
                ["NFR-05", "The same incident is not emailed or slacked more than once in 10 minutes.", "Trip a budget twice inside 10 minutes. OBS-05 sends one notice."],
                ["NFR-06", "Health of the server is visible.", "GET /health returns success when the service is up."],
            ],
            [20 * mm, 84 * mm, 80 * mm],
        ),
        P("9. Test-case writing rule", "h1"),
        P("For requirement AUTH-01 the test case ID is TC-AUTH-01. Preconditions are the Before you start column. Steps are numbered from the Inputs column. Expected result is copied and not rewritten. Add TC-AUTH-01-N for a negative only when this specification already states the failure. If a behaviour is not in this document, it is not in scope and it does not get a test case until the ID is added here.", "body"),
        P("10. User-manual writing rule", "h1"),
        P("One chapter per flow in section 7. Inside the chapter, one numbered section per requirement ID. Show the inputs as a form the reader fills. End the section with the expected result, written as what the reader sees. Do not document out-of-scope items in section 4 as features.", "body"),
    ]
    path = OUT / "07_Scope_Specification.pdf"
    doc = SimpleDocTemplate(
        str(path), pagesize=A4,
        leftMargin=12 * mm, rightMargin=12 * mm,
        topMargin=16 * mm, bottomMargin=14 * mm,
        title="UnifAI scope specification", author="UnifAI",
    )
    doc.build(story, onFirstPage=header_footer, onLaterPages=header_footer)
    print(path.name, path.stat().st_size)

    md = ["# UnifAI scope specification", "", "Source for test cases and the user manual. One requirement, one test case, one manual section.", "", "Test case ID = TC- plus the requirement ID. Manual section = the same ID.", ""]
    for title, rows in REQS:
        md.append(f"## {title}")
        md.append("")
        for row in rows:
            md.append(f"### {row[0]} — {row[1]}")
            md.append(f"- Actor: {row[2]}")
            md.append(f"- Before you start: {row[3]}")
            md.append(f"- Inputs: {row[4]}")
            md.append(f"- Expected result: {row[5]}")
            md.append(f"- Test case: TC-{row[0]}")
            md.append("")
    md_path = Path(__file__).resolve().parents[1] / "SCOPE_SPECIFICATION.md"
    md_path.write_text("\n".join(md), encoding="utf-8")
    print(md_path.name, md_path.stat().st_size)


if __name__ == "__main__":
    main()
