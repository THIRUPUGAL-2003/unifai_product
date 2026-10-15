#!/usr/bin/env python3
"""Client PDF: every website menu, with inputs, use, and connections."""

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
MUTED = colors.HexColor("#475569")


def styles():
    return {
        "kicker": ParagraphStyle("kicker", fontName="Times-Bold", fontSize=11, textColor=TEAL, spaceAfter=6),
        "title": ParagraphStyle("title", fontName="Times-Bold", fontSize=22, leading=26, textColor=NAVY, spaceAfter=6),
        "sub": ParagraphStyle("sub", fontName="Times-Roman", fontSize=11, leading=15, textColor=MUTED, spaceAfter=6),
        "h1": ParagraphStyle("h1", fontName="Times-Bold", fontSize=14, leading=18, textColor=NAVY, spaceBefore=10, spaceAfter=4),
        "h2": ParagraphStyle("h2", fontName="Times-Bold", fontSize=11, leading=14, textColor=TEAL, spaceBefore=6, spaceAfter=2),
        "body": ParagraphStyle("body", fontName="Times-Roman", fontSize=9.5, leading=12.5, textColor=colors.HexColor("#1e293b"), alignment=TA_JUSTIFY, spaceAfter=3),
        "cell": ParagraphStyle("cell", fontName="Times-Roman", fontSize=8, leading=10.5, textColor=colors.HexColor("#1e293b")),
        "cellb": ParagraphStyle("cellb", fontName="Times-Bold", fontSize=8, leading=10.5, textColor=colors.white),
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
        ("GRID", (0, 0), (-1, -1), 0.3, RULE),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 3),
        ("RIGHTPADDING", (0, 0), (-1, -1), 3),
        ("TOPPADDING", (0, 0), (-1, -1), 2),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 2),
    ]))
    return t


def screen(name, path, used_for, inputs, how, connects):
    return [
        P(name, "h1"),
        P("Open: " + path, "body"),
        P("What this is used for", "h2"),
        P(used_for, "body"),
        P("Inputs you fill", "h2"),
        table(["Input", "What you enter"], inputs, [46 * mm, 124 * mm]),
        Spacer(1, 1.5 * mm),
        P("How to use it", "h2"),
        P(how, "body"),
        P("How it connects", "h2"),
        P(connects, "body"),
    ]


def header_footer(canvas, doc):
    canvas.saveState()
    canvas.setFillColor(NAVY)
    canvas.rect(0, A4[1] - 12 * mm, A4[0], 12 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Times-Bold", 8)
    canvas.drawString(14 * mm, A4[1] - 7.5 * mm, "UnifAI  ·  every website section")
    canvas.setFont("Times-Roman", 8)
    canvas.drawRightString(A4[0] - 14 * mm, A4[1] - 7.5 * mm, "Inputs, use, and connection")
    canvas.setFillColor(TEAL)
    canvas.rect(0, 0, A4[0], 9 * mm, fill=1, stroke=0)
    canvas.setFillColor(colors.white)
    canvas.setFont("Times-Roman", 8)
    canvas.drawString(14 * mm, 3.5 * mm, "Client guide  ·  sample values only")
    canvas.drawRightString(A4[0] - 14 * mm, 3.5 * mm, f"Page {doc.page}")
    canvas.restoreState()


# name, path, used for, inputs, how to use, how it connects
GROUPS = [
    ("1. Observability", [
        ("Dashboard", "/workspace/dashboard",
         "Shows company totals for API traffic: requests, tokens, cost, and latency.",
         [("Date range", "Last 7 days"), ("No create fields", "This page is read-only")],
         "Open it after at least one virtual-key call has succeeded. Read the cards. A high cost sends you to LLM Logs for that model or key.",
         "Reads LLM Logs. Cost uses Pricing Overrides and Model Settings. Empty means Model Providers and Virtual Keys are not in use yet."),
        ("LLM Logs", "/workspace/logs",
         "One row for every API call made with a virtual key.",
         [("Date", "Today"), ("Virtual key", "Support bot — production"), ("Model", "gpt-4.1-mini"), ("Status", "success or error")],
         "Filter, open a row, and read the prompt, answer, tokens, cost, and latency. Export if you need a sheet. Clear logs or recalculate cost only when an admin means to.",
         "A row is written after Virtual Keys, Routing, Guardrails, and Model Providers finish a call. A 401 points at the provider key. A budget error points at Virtual Keys or Budgets & Limits."),
        ("MCP Logs", "/workspace/mcp-logs",
         "One row for every tool call made through MCP Gateway.",
         [("Client", "docs-server"), ("Tool", "search_docs"), ("User", "anitha.user"), ("Time", "Today")],
         "Filter and open the row to see the tool arguments and the result.",
         "Appears only after MCP Catalog is connected, MCP Auth Config is saved, a Tool Group allows the tool, and the feature flag mcp.tool_execution is on."),
        ("Connectors", "/workspace/observability",
         "Copies logs and metrics to a system the company already runs.",
         [("Connector", "Datadog, BigQuery, Kafka, Pub/Sub, New Relic, Prometheus, or OpenTelemetry"), ("URL or site", "The vendor endpoint"), ("API key or dataset", "The secret from that vendor account")],
         "Add connector, paste the vendor values, save, and send one test chat. Confirm the same request in LLM Logs and in the vendor.",
         "Source is LLM Logs and metrics. This does not send the short budget alerts. Those go to Alert Channels."),
        ("Logs Settings", "/workspace/config/logging",
         "Chooses what API log text is stored and for how long.",
         [("Content logging", "On for a pilot, off if prompts must not be stored"), ("Retention days", "30")],
         "Set the two values and save. New LLM Log rows follow this. Old rows stay until retention deletes them.",
         "Controls LLM Logs and the Observability Dashboard. It does not control Browser AI Prompt Logs."),
    ]),
    ("2. Browser AI", [
        ("Overview", "/workspace/browser-ai?tab=overview",
         "Counts monitored sites, active rules, recent agents, and recent decisions.",
         [("No create fields", "Read the counts after the other Browser AI tabs are filled")],
         "Use it as the check that the rollout is alive. Zero monitored sites means Target Websites is empty. Zero agents means Setup has not reported a laptop.",
         "Summarises Target Websites, Guard Rules, Prompt Logs, and Guard Agents."),
        ("Target Websites", "/workspace/browser-ai?tab=targets",
         "The public AI sites the laptop is allowed to watch.",
         [("Domain", "chatgpt.com"), ("Platform name", "ChatGPT"), ("Host role", "ui, chat, or file"), ("Monitored", "Yes"), ("Block entire website", "No"), ("Status", "MONITORED")],
         "Add one row or import a CSV with those columns. Add the chat and file hosts when the vendor uses more than one domain. Pause with status PAUSED.",
         "Gateway Guard downloads this list. A Guard Rule does nothing on a domain that is not here."),
        ("Guard Rules", "/workspace/browser-ai?tab=rules",
         "What the laptop does when a prompt or upload on a target site matches.",
         [("Rule name", "India PAN"), ("Rule type", "regex, or ai_bot typed in the screen"), ("Pattern", "[A-Z]{5}[0-9]{4}[A-Z]"), ("Severity", "CRITICAL, HIGH, or MEDIUM"), ("Action", "BLOCK, WARN, or REDACT"), ("Warning message", "Personal ID numbers cannot be sent to a public AI site."), ("Active", "TRUE")],
         "Create or import. BLOCK stops the send. WARN shows the message and sends the original. REDACT shows the message and sends the match as [REDACTED]. Test an ai_bot rule before Active is TRUE. AI bot rules cannot be imported.",
         "Runs on text sent to a Target Website. The result is a Prompt Log. This is not Guardrails. Guardrails check API calls."),
        ("Prompt Logs", "/workspace/browser-ai?tab=logs",
         "Every browser prompt the agent checked.",
         [("Date", "Today"), ("Action", "Blocked, Warned, Redacted, Allowed, SiteBlocked, or Bot Answered"), ("Search", "The laptop hostname")],
         "Filter and open the row. You see the prompt, the rule, the action, and the device.",
         "Written by Gateway Guard after a Guard Rule runs. Match the hostname on Guard Agents."),
        ("Search Logs", "/workspace/browser-ai?tab=search-logs",
         "Search queries the agent recorded, including private windows.",
         [("Engine", "Google"), ("Browser", "Chrome, Edge, or Brave"), ("Private window", "Yes or no"), ("Query", "The text the employee searched")],
         "Filter and open a row for the query and any result that was opened.",
         "Same laptop agent as Prompt Logs. It is not an LLM Log."),
        ("Setup", "/workspace/browser-ai?tab=setup",
         "Download the laptop package and publish Guard code updates.",
         [("Package", "Gateway_Guard_Setup.exe for Windows, or the Mac package"), ("Rebuild", "Use Rebuild and Publish only after Guard code on the server changed")],
         "Download the package and give it to IT. IT runs it once per laptop and leaves start at login checked.",
         "The package installs Gateway Guard, which force-installs YP-Raksha and then appears under Guard Agents."),
        ("Guard Agents", "/workspace/browser-ai?tab=agents",
         "Every laptop and every office proxy that has reported in.",
         [("Search", "DESKTOP-EXAMPLE"), ("Contact email", "it@example.com"), ("Uninstall key", "A secret kept in the IT vault, not in email")],
         "Confirm status active and a recent last-seen time. Remote uninstall moves the device to uninstall_pending, then uninstalled.",
         "The agent appears only after Setup has run and the laptop can reach this server. Prompt Logs use the same hostname."),
        ("Guard Insights", "/workspace/browser-ai?tab=telemetry",
         "Totals of allowed, blocked, warned, and redacted events per laptop.",
         [("No create fields", "Read the totals, then open Prompt Logs for the loudest hostname")],
         "Use a high Blocked count as a reason to open that laptop’s Prompt Logs before you edit the rule.",
         "Adds up Prompt Logs. It does not change Guard Rules."),
    ]),
    ("3. Models", [
        ("Model Providers", "/workspace/providers",
         "Attaches the company’s account at OpenAI, Anthropic, Azure, Bedrock, Gemini, or another vendor. Admin only for the secret.",
         [("Provider", "OpenAI"), ("Key name", "openai-prod-1"), ("API key", "The vendor key"), ("Azure or Bedrock extras", "Endpoint or cloud credentials, only if that form asks")],
         "Add provider, paste the key, save. You may store more than one key for the same vendor. A keyless provider needs no key.",
         "Model Catalog lists models from these keys. Routing sends calls here. The virtual key never contains this vendor key."),
        ("Model Catalog", "/workspace/model-catalog",
         "The models UnifAI is allowed to offer.",
         [("Model", "gpt-4.1-mini"), ("Enabled", "Yes for models under contract, no for the rest")],
         "Search, enable, and disable. Open a model to see its price.",
         "Fed by Model Providers. Virtual Keys and Routing Rules should name only enabled models."),
        ("Budgets & Limits", "/workspace/model-limits",
         "A spend cap and a request rate on one model, separate from a virtual-key budget.",
         [("Model", "gpt-4.1-mini"), ("Budget", "500"), ("Rate", "60 requests per minute")],
         "Open the model, set the ceiling, save. The first ceiling that is hit stops the call.",
         "Checked with the Virtual Key, the Team, and the Business Unit. The error is in LLM Logs."),
        ("Complexity Router", "/workspace/complexity-router",
         "Sends an easy prompt to a cheaper model and a hard prompt to a stronger model.",
         [("Enabled", "Yes"), ("Low complexity model", "gpt-4.1-mini"), ("High complexity model", "gpt-4.1"), ("Feature flag", "routing.complexity on")],
         "Save the map. Send a short hello and confirm LLM Logs shows the small model.",
         "Runs before the provider call. Routing Rules can still force one model. Both models must be enabled in the catalog."),
        ("Routing Rules", "/workspace/routing-rules",
         "Company policy: which provider and model for a team, user, key, or model name.",
         [("Condition", "Team is Support"), ("Target model", "gpt-4.1-mini"), ("Order", "Specific rule above the general rule")],
         "Add the rule and save. Send a Support-key call and confirm the model in LLM Logs.",
         "Reads the virtual key, user, and team. Sends the call to a Model Provider. Use Complexity Router for automatic difficulty. Use this screen for a fixed policy."),
        ("Circuit Breaker", "/workspace/circuit-breaker",
         "Stops sending traffic to a provider that is failing.",
         [("Failures before open", "5 in a minute"), ("Wait before retry", "30 seconds"), ("Feature flag", "routing.circuit_breaker on")],
         "Save. You need a second provider key if traffic must continue while one vendor is paused.",
         "Watches errors from Model Providers. Adaptive Routing and Routing Rules should use the healthy provider. LLM Logs show the errors that opened it."),
        ("Pricing Overrides", "/workspace/custom-pricing/overrides",
         "Your contract price, used instead of the default price.",
         [("Model", "gpt-4.1-mini"), ("Input price", "Your contract price per million tokens"), ("Output price", "Your contract price per million tokens")],
         "Save. After a change, an admin can recalculate cost from LLM Logs.",
         "Dashboard, LLM Logs, and budgets use this price when it is set. It wins over Model Settings when both exist."),
        ("Model Settings", "/workspace/custom-pricing",
         "Default parameters and the base price for a model.",
         [("Model", "gpt-4.1-mini"), ("Temperature", "0.2 for a stable support answer"), ("Base price", "Set here only if you are not using a Pricing Override")],
         "Open the model, set the defaults, save.",
         "Applied on the way to the provider unless the client sends its own value. Pricing Overrides win on price."),
    ]),
    ("4. MCP Gateway", [
        ("MCP Catalog", "/workspace/mcp-registry",
         "The MCP servers UnifAI will dial.",
         [("Name", "docs-server"), ("URL or command", "https://mcp.example.com/sse")],
         "Add the client, save, and reconnect if the first dial fails. Status should become connected.",
         "Tool Groups pick tools from here. MCP Auth Config stores the header. MCP Logs show the calls. Flag mcp.tool_execution must be on."),
        ("MCP Library", "/workspace/mcp-registry/library",
         "A ready list of known MCP servers. It is a template, not a live connection.",
         [("Server", "The documented server you need"), ("Sync", "Refresh the list if it looks stale")],
         "Add the chosen server into MCP Catalog, then fill the real URL and credentials there.",
         "Library does not dial anything. Catalog does."),
        ("Tool Groups", "/workspace/mcp-tool-groups",
         "A named set of tools one virtual key may call.",
         [("Group name", "Support tools"), ("Tools", "search_docs only"), ("Attached to", "The Support virtual key")],
         "Create the group, tick only those tools, and attach it to the key.",
         "A key without the group cannot call the tool. Calls appear in MCP Logs."),
        ("Auth Sessions", "/workspace/mcp-sessions",
         "Live sign-ins between a person and an MCP server.",
         [("User", "anitha.user"), ("Server", "docs-server"), ("State", "success or the failure reason")],
         "Open a session to see who connected. Disconnect a session that should stop.",
         "Created after MCP Auth Config and, when required, OAuth Grants. Feature flag mcp.oauth_grants turns on the consent screen."),
        ("OAuth Grants", "/workspace/oauth-grants",
         "The consent a person gave an MCP server to use an external account.",
         [("User", "anitha.user"), ("Server", "docs-server"), ("Action", "Revoke on the day the person leaves")],
         "Review the list. Revoke instead of leaving a grant in place.",
         "Sits between the user and the external service. Auth Sessions show whether the sign-in worked."),
        ("MCP Settings", "/workspace/mcp-settings",
         "Gateway-wide timeouts and switches for every MCP server.",
         [("Timeout", "Raise it only when a known server is slow")],
         "Change the value and save. Then reconnect the client from MCP Catalog.",
         "Applies to every Catalog server. The URL of each server stays on the catalog entry."),
        ("Plugins", "/workspace/plugins",
         "Turns the processing steps on the API path on or off, and sets their order.",
         [("Keep on", "governance and logging"), ("Leave off", "Any plugin you do not recognise")],
         "Enable, disable, and reorder only when you know which step must run first. Send one test chat and confirm LLM Logs still has a row.",
         "Governance off stops budget checks. Guardrails off stops API guardrail rules. This page is not the laptop Guard."),
    ]),
    ("5. Governance", [
        ("Virtual Keys", "/workspace/governance/virtual-keys",
         "The secret an application sends. It is not a website login and it is not the vendor key.",
         [("Name", "Support bot — production"), ("Team or user", "Support"), ("Budget", "200"), ("Rate", "120 per minute"), ("Models", "gpt-4.1-mini")],
         "Create, copy the key once into the application secret store, and rotate it when staff leave.",
         "The app calls /v1/chat/completions with Authorization: Bearer and this key. Budgets, Routing, Guardrails, and Model Providers run on that call. LLM Logs show it."),
        ("Users", "/workspace/governance/users",
         "Website accounts and who may open which menu.",
         [("Username", "anitha.user"), ("Email", "anitha.user@example.com"), ("Password", "A password with 8 characters, upper, lower, number, and a symbol"), ("Role", "user, sub_admin, admin, or a custom role"), ("Allowed sections", "prompt-repository, and browser-ai only for security staff"), ("Status", "Approve a pending sign-up, or disable a leaver")],
         "Create the user or approve the pending row. The person then signs in at /login.",
         "Allowed sections decide the menu. A virtual key can be assigned to this user. SMTP must already work if they signed up with an email code."),
        ("Teams", "/workspace/governance/teams",
         "A group of users with one shared budget.",
         [("Name", "Support"), ("Members", "anitha.user"), ("Budget", "200"), ("Rate", "120 per minute")],
         "Create the team, add the people, set the ceiling, save.",
         "Put the team on a Virtual Key and on a Routing Rule. A Business Unit above the team can stop spend even when the team budget remains."),
        ("Business Units", "/workspace/governance/business-units",
         "A department budget over several teams.",
         [("Name", "Customer Operations"), ("Teams", "Support"), ("Budget", "2000"), ("Rate", "The department ceiling")],
         "Create the unit, attach the teams, save.",
         "Sits above Teams. The unit ceiling stops every team under it."),
        ("Customers", "/workspace/governance/customers",
         "An external client you report and bill separately.",
         [("Name", "Northwind"), ("Teams", "Support")],
         "Create the customer and link the teams that do that client’s work.",
         "Reporting and budgets. The application still uses a Virtual Key."),
        ("User Provisioning", "/workspace/scim",
         "Connects Azure AD or Okta so joiners and leavers update Users automatically.",
         [("SCIM base URL", "https://unifai.example.com/api/scim/v2"), ("Token", "The token this page shows"), ("Map", "User name and email")],
         "Paste the URL and token into the identity provider. A person set inactive there becomes disabled here.",
         "Writes the same Users records. It does not install Gateway Guard and it does not replace SMTP."),
        ("Roles & Permissions", "/workspace/governance/rbac",
         "What a role may do on the API.",
         [("Role name", "Auditor"), ("Data scope", "all-data or own-data"), ("Allow", "Logs Read and View, Audit Logs Read and View, Dashboard Read and View"), ("Do not allow", "Delete, provider keys, Settings")],
         "Create the custom role and assign it on the user. Leave admin, sub_admin, and user in place unless you must narrow them.",
         "Every API call checks this, then the allowed sections on the user. Browser AI is under the Logs resource. The Users page itself stays with admin and sub_admin."),
        ("Access Profiles", "/workspace/governance/access-profiles",
         "A reusable bundle of budget, model, and key limits for one job.",
         [("Name", "Contractor"), ("Budget", "25"), ("Models", "gpt-4.1-mini only"), ("State", "Active")],
         "Create, activate, and apply it to those users. Clone it for a similar job.",
         "Applying it updates those users’ virtual-key limits."),
        ("Audit Logs", "/workspace/audit-logs",
         "Who changed configuration. You do not type these rows.",
         [("User", "priya.admin"), ("Date", "Today"), ("Action", "The create or update you just saved")],
         "Filter after an incident. Do not delete the rows as a habit.",
         "Records changes on Users, Virtual Keys, Guard Rules, Model Providers, and Settings."),
    ]),
    ("6. Guardrails", [
        ("Rules", "/workspace/guardrails/configuration",
         "Checks prompts and answers on API calls that use a virtual key.",
         [("Detection", "PII and secrets, prompt injection, toxicity, or grounding"), ("Action", "Block or redact the API call"), ("Feature flags", "guardrails.pii_masking and the other guardrails flags you rely on")],
         "Create the rule, point it at a Rule Provider if it needs one, and save. Send a test API call that contains a card number and read LLM Logs.",
         "Runs on the API path. It does not write a Browser AI Prompt Log. Browser checks stay in Guard Rules."),
        ("Rule Providers", "/workspace/guardrails/providers",
         "The safety engine a guardrail rule calls. This is not the chat vendor.",
         [("Provider", "The safety product you licensed"), ("Key or endpoint", "That product’s credential")],
         "Add it, save, then select it on the rule.",
         "Model Providers call the chat model. This page calls the checker."),
        ("Cluster Config", "/workspace/cluster",
         "More than one UnifAI server sharing configuration.",
         [("Peers", "Leave empty on a single server"), ("Cluster secret", "The same secret on every node, only when you have a pair")],
         "On one server, do not add peers. On a pair, each node lists the other.",
         "A virtual key created on one node must exist on the others. This does not replace the database."),
    ]),
    ("7. Adaptive Routing", [
        ("Dashboard", "/workspace/adaptive-routing",
         "How API traffic is split across provider keys right now. This is not the Observability Dashboard.",
         [("No create fields", "Read the weight and the health of each route")],
         "If one provider takes every call, open Adaptive Routing Settings or Routing Rules.",
         "Uses the Model Provider pool. Circuit Breaker removes a failing provider from this split."),
        ("Settings", "/workspace/adaptive-routing/settings",
         "The load-balancing policy when more than one key can serve the model.",
         [("Strategy", "The split you want"), ("Weight openai-prod-1", "70"), ("Weight openai-prod-2", "30"), ("Fallback flag", "routing.fallback_cascade on if a 429 or 500 must jump to the next provider")],
         "Set the weights and save.",
         "Routing Rules win when a rule names one provider. Circuit Breaker removes a sick key from the weights."),
    ]),
    ("8. Prompt Repository and Skills Repository", [
        ("Prompt Repository", "/workspace/prompt-repo",
         "Saved prompt text for people and for applications.",
         [("Folder", "Support"), ("Name", "Customer reply — refund tone"), ("Text", "Reply in a calm tone. Do not ask for card numbers or government IDs."), ("Tags", "support, refund")],
         "Create the folder, add the prompt, save. Limit folders on the user if another team must not see it.",
         "Does not choose a model. Prompt Deployments publishes a version. The call still needs a Virtual Key."),
        ("Skills Repository", "/workspace/skills-repo",
         "Shared skill instructions the company published.",
         [("Name", "Refund tone"), ("Body", "Three short sentences. No legal promises. Do not ask for a PAN or a card number.")],
         "Add the skill and save. People with this menu can browse it.",
         "Served to clients that call the skills API. It is not a Guard Rule and not a virtual key."),
    ]),
    ("9. Settings", [
        ("Client Settings", "/workspace/config/client-settings",
         "Limits for incoming API calls.",
         [("Allowed origins", "https://unifai.example.com"), ("Max body size", "100 MB unless you must raise it"), ("Require auth", "On")],
         "Save. Applications must call from an allowed origin and must still send a valid virtual key.",
         "This is not Target Websites. AI sites the laptop watches are under Browser AI."),
        ("Compatibility", "/workspace/config/compatibility",
         "Converts a client’s parameters into the shape a provider expects.",
         [("Conversion", "On when an OpenAI-style client calls a non-OpenAI model through the gateway")],
         "Turn on only the conversion you need and save.",
         "Sits between the virtual-key request and Model Providers. Routing still chooses the provider."),
        ("Caching", "/workspace/config/caching",
         "Reuses an earlier answer for a repeated request.",
         [("Direct cache", "On for a FAQ bot"), ("Semantic cache", "On only if similar questions should share an answer"), ("TTL", "10 minutes"), ("Feature flag", "cache.responses on, or every call still goes to the provider")],
         "Save. Turn the flag off while you are debugging a prompt.",
         "A cache hit does not call Model Providers. LLM Logs should show the hit."),
        ("Security", "/workspace/config/security",
         "Sign-in protection, and the SMTP mail server for codes and notices.",
         [("Lockout", "On"), ("SMTP enabled", "Yes"), ("Host", "smtp.example.com"), ("Port", "587"), ("Username", "mailer@example.com"), ("From email", "noreply@example.com"), ("From name", "UnifAI"), ("Use TLS", "Yes"), ("Notify on user create", "Yes")],
         "Save SMTP, then send a test to priya.admin@example.com. Sign-up codes and password reset start only after that test arrives.",
         "Used by Sign Up, Forgot password, and Forgot username. An email Alert Channel also needs this. Virtual keys do not use this login."),
        ("API Keys", "/workspace/config/api-keys",
         "The same vendor secrets as Model Providers, opened from Settings.",
         [("Key name", "openai-prod-1"), ("New secret", "The replacement vendor key")],
         "Rotate here when you are already in Settings. Prefer Model Providers for the full provider form. Send one test chat and read LLM Logs.",
         "The next routed call uses the new secret."),
        ("Performance Tuning", "/workspace/config/performance-tuning",
         "Server buffers and pool sizes.",
         [("Leave defaults", "On a new install"), ("Change", "Only after latency on the Observability Dashboard is high while the vendor itself is fast")],
         "Save and watch the Dashboard.",
         "Affects how fast calls are handled. It does not choose the model."),
        ("Feature Flags", "/workspace/config/feature-flags",
         "On and off switches for whole features.",
         [("browser.prompt_guard", "On, or laptop prompts are not stored"), ("browser.file_scan", "On if uploads must be checked"), ("cache.responses", "Off until answers are trusted"), ("routing.complexity", "On only if Complexity Router is in use"), ("routing.circuit_breaker", "On only if Circuit Breaker is in use"), ("routing.fallback_cascade", "On if a 429 or 500 must jump provider"), ("mcp.tool_execution", "On only if MCP tools must run"), ("mcp.oauth_grants", "On if MCP must show a consent screen")],
         "Add the preset and set it on or off. A flag gates the screen it names.",
         "browser.prompt_guard off means Prompt Logs stay empty. routing.circuit_breaker off means the Circuit Breaker page does not enforce."),
    ]),
    ("10. Beside the menu, in the same order", [
        ("Proxy", "/workspace/config/proxy",
         "The office HTTP proxy the UnifAI server uses for outbound calls. Not Gateway Guard and not the laptop PAC.",
         [("Enabled", "No, unless the server cannot reach vendors directly"), ("URL", "http://proxy.example.com:8080"), ("Enable for inference", "Yes, only when the proxy is on"), ("Enable for SCIM", "Yes only if the identity provider is reached through that proxy"), ("No proxy", "localhost,127.0.0.1")],
         "Leave it off on a normal pilot. If you turn it on, save and confirm one chat still succeeds in LLM Logs.",
         "Model Providers use it for vendor calls when inference is enabled. User Provisioning uses it when SCIM is enabled."),
        ("Alert Channels", "/workspace/alert-channels",
         "A short notice when a circuit breaker trips or a budget or rate limit is exhausted. The same incident is sent at most once every 10 minutes.",
         [("Name", "Ops Slack"), ("Type", "Webhook, Slack, Email, or PagerDuty"), ("URL, email, or routing key", "The Slack webhook URL, or the address, or the PagerDuty key"), ("Enabled", "Yes")],
         "Add the channel after a budget or a circuit breaker exists. Press test. Email type needs SMTP already tested.",
         "Connectors copy full logs. This page sends the short incident only."),
        ("MCP Auth Config", "/workspace/mcp-auth-config",
         "The header and per-user credential for an MCP server that is already in the catalog.",
         [("Client", "docs-server"), ("Header name", "Authorization"), ("Header value", "The server token"), ("Per-user header keys", "Authorization, when each person has their own credential")],
         "If the page says there are no clients, add the server in MCP Catalog first. Save headers, then save the per-user list. Reconnect the catalog client if the state stays failed.",
         "Auth Sessions and OAuth Grants show the result. MCP Logs show the tool call."),
        ("Prompt Deployments", "/workspace/prompt-repo",
         "Publishes one prompt version to production, staging, or preview.",
         [("Prompt", "Customer reply — refund tone"), ("Version", "1, created in Prompt Repository first"), ("Environment", "production, staging, or preview"), ("Enabled", "Yes")],
         "Select the prompt, deploy the version, and leave the switch on. Turn it off to stop that environment without deleting the prompt.",
         "The application sends header x-uf-prompt-environment: production, or x-uf-dim-environment, plus the virtual key. The model call still follows Routing and Model Providers."),
    ]),
    ("11. Items that are easy to miss", [
        ("Sign in, sign up, and sign out", "/login and /signup",
         "The only website login. Every role uses this page. The menu after login follows the role and the allowed sections.",
         [("Sign in username", "anitha.user"), ("Sign in password", "Anitha#User24"), ("Sign up email", "anitha.user@example.com"), ("Email code", "481926, only after SMTP test mail has arrived"), ("New password", "Anitha#Reset24"), ("Sign out", "Account menu. The address may include logged_out=1")],
         "Approve the user under Governance → Users before the first sign-in. If the screen says too many failed attempts, wait the minutes it shows. If it says verify your email, enter the code on Sign Up. If it says waiting for approval, an admin has not approved the row.",
         "SMTP on Settings → Security sends the code. Users stores the account. Allowed sections hide menus the person must not open. A disabled SCIM user cannot sign in."),
        ("File upload policy", "/workspace/browser-ai?tab=rules",
         "A switch on the Guard Rules page. It blocks every file or attachment sent to a monitored AI site, whatever is inside the file.",
         [("Block all uploads", "On when the company forbids attachments"), ("Upload warning text", "The sentence the employee sees when an upload is stopped"), ("Enabled", "The policy master switch on the same card")],
         "Turn the switch on, write the warning, and save. Then upload a small file on a monitored site and confirm Prompt Logs shows the block.",
         "Applies only to domains in Target Websites. It is separate from a regex Guard Rule. Feature flag browser.file_scan must stay on or uploads are not checked."),
        ("Warning email from a prompt log", "/workspace/browser-ai?tab=logs",
         "Sends the company warning to the contact email stored on that laptop’s Guard Agent.",
         [("Log row", "A Blocked or Warned row"), ("Contact email", "it@example.com on Guard Agents"), ("SMTP", "Already tested on Settings → Security")],
         "Open the row and send the warning. If the send fails, SMTP is missing or the agent has no contact email.",
         "Uses the same mail server as sign-up codes. The log row stays. Audit Logs can show the send."),
        ("Playground", "/workspace/prompt-repo",
         "A chat box inside Prompt Repository. It calls the gateway with the signed-in user’s access, so an admin can try a model without a separate application.",
         [("Model", "gpt-4.1-mini, already enabled in Model Catalog"), ("Message", "Summarise our refund policy in three bullets."), ("Prompt", "Optional. Select Customer reply — refund tone to send that saved text"), ("Skill", "Optional. A skill from Skills Repository can be injected as the system message")],
         "Select the model, type the message, and send. Then open LLM Logs and confirm the row.",
         "Uses Model Providers and Routing. It does not use the laptop Guard. A 401 means no provider key or the user’s access cannot call that model."),
        ("Correct YP-Raksha install", "On the laptop after Browser AI → Setup",
         "The browser extension Gateway Guard turns on. Employees do not load it from a store and do not get a Remove button on a correct install.",
         [("Program", "Gateway_Guard_Setup.exe"), ("Start at login", "Checked"), ("Extension id", "mipfhcmlcjfhmgldkokdckhndkebkllj"), ("Uninstall", "Company uninstall key, or remote uninstall from Guard Agents")],
         "IT runs the setup once. Approve the administrator prompt so the machine policy is written. Open Chrome or Edge only to confirm the extension is on.",
         "A correct install is managed by the organisation, on, including private windows, with no Remove button. Off, Remove, or the words not from any known source means the outside install is still present and IT must remove that registry path so the policy install can take over."),
        ("What an empty screen usually means", "Any menu",
         "Use this when a page has no rows. Fix the earlier step. Do not add a second copy of the same key or rule.",
         [("LLM Logs empty", "No virtual-key call yet, or Model Providers has no working key"), ("Prompt Logs empty", "No laptop has reported, the site is not a Target Website, or browser.prompt_guard is off"), ("MCP Logs empty", "Catalog is not connected, or mcp.tool_execution is off"), ("Sign-up code missing", "SMTP test on Settings → Security has not succeeded"), ("Model Catalog empty", "Model Providers has no key, or the vendor rejected it"), ("Guard Agents empty", "Setup has not been run on a laptop, or the laptop cannot reach the server")],
         "Walk the connection flow from step 1 again and stop at the first step that is not done.",
         "Each empty page points at one earlier page. The connection order at the start of this catalog is the checklist."),
    ]),
]


def full_catalog():
    """Connection flow plus every menu section. Included in every client PDF."""
    story = [
        P("Connection flow and every website section", "h1"),
        P("Do the steps in this order. Logs stay empty until a provider and a virtual key exist. Sample values are for training.", "body"),
        table(
            ["Order", "Do this first"],
            [
                ["1", "Settings → Security: save SMTP and confirm the test mail."],
                ["2", "Settings → Client Settings. Leave Proxy off unless the server needs an office proxy."],
                ["3", "Models → Model Providers, then Model Catalog."],
                ["4", "Governance → Teams, Users, then Virtual Keys."],
                ["5", "Routing Rules, Budgets & Limits, Pricing. Then Guardrails for API text."],
                ["6", "One API call, then Observability → LLM Logs."],
                ["7", "Browser AI: Target Websites, Guard Rules, Setup, then Guard Agents and Prompt Logs."],
                ["8", "MCP Catalog, MCP Auth Config, and Tool Groups only if a tool server is required."],
                ["9", "Prompt Repository, then Prompt Deployments. Skills Repository when you publish a skill."],
                ["10", "Alert Channels after a budget or circuit breaker exists. Connectors when logs must leave the server."],
            ],
            [18 * mm, 152 * mm],
        ),
        Spacer(1, 2 * mm),
        P("An API call connects like this: virtual key, then budget, then routing, then guardrails, then the model provider, then LLM Logs. A browser prompt connects like this: target website, then the laptop agent, then the guard rule, then Prompt Logs. That path does not use a virtual key.", "body"),
        P("Do not mix these up", "h2"),
        table(
            ["Two things with similar names", "Which one to use"],
            [
                ["Observability Dashboard and Adaptive Routing Dashboard", "Observability shows cost and tokens. Adaptive Routing shows how calls are split across provider keys."],
                ["Guardrails Rules and Browser AI Guard Rules", "Guardrails check API calls that use a virtual key. Guard Rules check prompts typed on public AI websites."],
                ["Settings → Proxy and Gateway Guard", "Settings → Proxy is the office proxy for the server. Gateway Guard is the program on the laptop."],
                ["Settings → API Keys and Governance → Virtual Keys", "API Keys are the vendor secrets. Virtual Keys are what applications send."],
                ["Connectors and Alert Channels", "Connectors copy full logs. Alert Channels send one short notice for a budget or a circuit breaker."],
                ["MCP Library and MCP Catalog", "The library is a template list. The catalog is the live server connection."],
                ["Users role and Allowed sections", "The role is what the API allows. Allowed sections are which menus appear after login."],
            ],
            [62 * mm, 108 * mm],
        ),
        Spacer(1, 2 * mm),
        P("Website logins. There is one Sign In page. The role decides the menu.", "h2"),
        table(
            ["Login", "Sample", "After sign-in"],
            [
                ["admin", "priya.admin / Priya#Admin24", "Every menu, including provider keys and Settings."],
                ["sub_admin", "ravi.security / Ravi#Secure24", "Users, keys, logs, Browser AI. Not master provider keys."],
                ["user", "anitha.user / Anitha#User24", "Only the allowed sections ticked on the user."],
                ["Custom role", "meena.audit / Meena#Audit24", "Only the resources ticked on that role."],
                ["SCIM user", "Created in Azure AD or Okta", "Same Sign In. Inactive in the identity provider means disabled here."],
                ["Virtual key", "Not a website login", "Sent by an application as Authorization: Bearer."],
            ],
            [32 * mm, 58 * mm, 80 * mm],
        ),
        Spacer(1, 2 * mm),
        P("Password rule for every website account: at least 8 characters, one uppercase letter, one lowercase letter, one number, and one special character. A session lasts about 24 hours. Sign out from the account menu. Sign out does not stop Gateway Guard or YP-Raksha.", "body"),
        P("Sign-up is always role user. Open /signup with username, email, password, and confirm password. If SMTP is already tested, enter the 6-digit code. Status stays pending until an admin approves the user. Forgot password and forgot username are on the Sign In page. The code lasts 10 minutes.", "body"),
    ]
    for title, screens in GROUPS:
        story.append(PageBreak())
        story.append(P(title, "h1"))
        for item in screens:
            story.extend(screen(*item))
    return story


def main():
    story = [
        Spacer(1, 18 * mm),
        P("CLIENT PACK  ·  DOCUMENT 06", "kicker"),
        P("Every website section", "title"),
        P("For each menu: what it is used for, the inputs you fill, how to use it, and how it connects.", "sub"),
        HRFlowable(width="100%", thickness=1, color=TEAL, spaceAfter=6),
    ]
    story.extend(full_catalog())

    path = OUT / "06_Every_Website_Screen.pdf"
    doc = SimpleDocTemplate(
        str(path), pagesize=A4,
        leftMargin=14 * mm, rightMargin=14 * mm,
        topMargin=16 * mm, bottomMargin=14 * mm,
        title="Every website section", author="UnifAI",
    )
    doc.build(story, onFirstPage=header_footer, onLaterPages=header_footer)
    print(path, path.stat().st_size)


if __name__ == "__main__":
    main()
