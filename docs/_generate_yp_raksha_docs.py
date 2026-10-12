# Generates YP-RAKSHA Word manuals from verified repository facts.
# Does not modify application source.

from docx import Document
from docx.enum.text import WD_ALIGN_PARAGRAPH, WD_LINE_SPACING
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Inches, Pt, RGBColor, Cm, Twips
from datetime import date

from _yp_raksha_extra import add_system, add_tech, add_user

OUT = r"d:\unifai_project\docs"
DATE = "12 October 2026"
VERSION = "Repository baseline, Guard agent 1.1.20"
CLASSIFICATION = "Internal"


def set_run_font(run, name="Calibri", size=11, bold=False, color=None):
    run.font.name = name
    run._element.rPr.rFonts.set(qn("w:eastAsia"), name)
    run.font.size = Pt(size)
    run.bold = bold
    if color:
        run.font.color.rgb = RGBColor(*color)


def shade_cell(cell, hex_color):
    tc = cell._tePr if hasattr(cell, "_tePr") else cell._tc
    tcPr = tc.get_or_add_tcPr()
    shd = OxmlElement("w:shd")
    shd.set(qn("w:fill"), hex_color)
    shd.set(qn("w:val"), "clear")
    tcPr.append(shd)


def add_page_number(paragraph):
    run1 = paragraph.add_run()
    fld_begin = OxmlElement("w:fldChar")
    fld_begin.set(qn("w:fldCharType"), "begin")
    instr = OxmlElement("w:instrText")
    instr.set(qn("xml:space"), "preserve")
    instr.text = " PAGE "
    fld_end = OxmlElement("w:fldChar")
    fld_end.set(qn("w:fldCharType"), "end")
    run1._r.append(fld_begin)
    run1._r.append(instr)
    run1._r.append(fld_end)
    set_run_font(run1, size=9, color=(80, 80, 80))


def configure(doc, footer_title):
    section = doc.sections[0]
    section.page_width = Cm(21.0)
    section.page_height = Cm(29.7)
    section.left_margin = Inches(1)
    section.right_margin = Inches(1)
    section.top_margin = Inches(1)
    section.bottom_margin = Inches(1)
    section.header_distance = Cm(0.6)
    section.footer_distance = Cm(0.6)
    header = section.header
    header.is_linked_to_previous = False
    hp = header.paragraphs[0]
    hp.alignment = WD_ALIGN_PARAGRAPH.RIGHT
    r = hp.add_run("YP-RAKSHA  |  " + footer_title + "  |  " + CLASSIFICATION)
    set_run_font(r, size=9, color=(0, 90, 90))
    footer = section.footer
    footer.is_linked_to_previous = False
    fp = footer.paragraphs[0]
    fp.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r2 = fp.add_run("YesPanchi Group of Companies  ·  Page ")
    set_run_font(r2, size=9, color=(80, 80, 80))
    add_page_number(fp)
    styles = doc.styles
    for style_name, size, bold in (("Heading 1", 16, True), ("Heading 2", 13, True), ("Normal", 11, False)):
        st = styles[style_name]
        st.font.name = "Calibri"
        st.font.size = Pt(size)
        st.font.bold = bold
        st.font.color.rgb = RGBColor(0x0B, 0x3A, 0x3A) if style_name != "Normal" else RGBColor(0x1A, 0x1A, 0x1A)
        pf = st.paragraph_format
        pf.space_after = Pt(8)
        pf.space_before = Pt(12 if style_name != "Normal" else 0)
        pf.line_spacing = 1.15


def p(doc, text, size=11, bold=False, space_after=8):
    para = doc.add_paragraph()
    para.paragraph_format.space_after = Pt(space_after)
    para.paragraph_format.line_spacing = 1.15
    run = para.add_run(text)
    set_run_font(run, size=size, bold=bold)
    return para


def bullets(doc, items):
    for item in items:
        para = doc.add_paragraph(style="List Bullet")
        para.clear()
        run = para.add_run(item)
        set_run_font(run, size=11)
        para.paragraph_format.space_after = Pt(3)


def numbered(doc, items):
    for item in items:
        para = doc.add_paragraph(style="List Number")
        para.clear()
        run = para.add_run(item)
        set_run_font(run, size=11)
        para.paragraph_format.space_after = Pt(3)


def h1(doc, text):
    doc.add_heading(text, level=1)


def h2(doc, text):
    doc.add_heading(text, level=2)


def code(doc, text):
    para = doc.add_paragraph()
    para.paragraph_format.space_before = Pt(4)
    para.paragraph_format.space_after = Pt(8)
    para.paragraph_format.left_indent = Inches(0.2)
    run = para.add_run(text)
    set_run_font(run, name="Consolas", size=9, color=(20, 40, 40))


def table(doc, headers, rows):
    t = doc.add_table(rows=1 + len(rows), cols=len(headers))
    t.style = "Table Grid"
    for i, h in enumerate(headers):
        cell = t.rows[0].cells[i]
        cell.text = ""
        run = cell.paragraphs[0].add_run(h)
        set_run_font(run, size=10, bold=True, color=(255, 255, 255))
        shade_cell(cell, "0B5F5F")
    for r_i, row in enumerate(rows):
        for c_i, val in enumerate(row):
            cell = t.rows[r_i + 1].cells[c_i]
            cell.text = ""
            run = cell.paragraphs[0].add_run(val)
            set_run_font(run, size=10)
            if r_i % 2 == 1:
                shade_cell(cell, "F4F8F8")
    p(doc, "", size=6, space_after=4)


def cover(doc, title, audience):
    for _ in range(3):
        p(doc, "")
    r = doc.add_paragraph()
    r.alignment = WD_ALIGN_PARAGRAPH.LEFT
    run = r.add_run("YP-RAKSHA")
    set_run_font(run, size=28, bold=True, color=(11, 95, 95))
    p(doc, "Real-time AI Knowledge Screening & Hazard Audit", size=14, bold=True)
    p(doc, title, size=22, bold=True)
    p(doc, "")
    table(doc, ["Field", "Value"], [
        ["Product name used in this document", "YP-RAKSHA"],
        ["Name in the running software", "Gateway (default). Brand mark: Raksha."],
        ["Subtitle in the product", "Real-time AI Knowledge Screening & Hazard Audit"],
        ["Organisation default", "YesPanchi Group of Companies"],
        ["Document version", VERSION],
        ["Document date", DATE],
        ["Classification", CLASSIFICATION],
        ["Audience", audience],
        ["Source of truth", "This Git repository. Not a certification claim."],
    ])
    p(doc, "The repository default product string is Gateway, set by GATEWAY_PRODUCT_NAME. The brand string defaults to Raksha. This document set uses the requested product name YP-RAKSHA for the same implementation.")
    p(doc, "This document describes behaviour found in source code, configuration samples, and existing Markdown guides. Where a feature is partial or absent, the text says so. It does not claim ISO certification, complete hazard detection, or guaranteed AI safety.")
    doc.add_page_break()
    h1(doc, "Contents")
    para = doc.add_paragraph()
    run = para.add_run()
    fld_begin = OxmlElement("w:fldChar")
    fld_begin.set(qn("w:fldCharType"), "begin")
    instr = OxmlElement("w:instrText")
    instr.set(qn("xml:space"), "preserve")
    instr.text = ' TOC \\o "1-2" \\h \\z \\u '
    fld_sep = OxmlElement("w:fldChar")
    fld_sep.set(qn("w:fldCharType"), "separate")
    fld_end = OxmlElement("w:fldChar")
    fld_end.set(qn("w:fldCharType"), "end")
    run._r.append(fld_begin)
    run._r.append(instr)
    run._r.append(fld_sep)
    hint = OxmlElement("w:t")
    hint.text = "Open this document in Microsoft Word and update the table of contents if page numbers are not filled in."
    run2 = para.add_run()
    run2._r.append(fld_end)
    p(doc, "If the contents list is empty, press Ctrl+A then F9 in Word, or right-click the contents and choose Update Field.")
    doc.add_page_break()


def enable_update_fields(doc):
    settings = doc.settings.element
    el = OxmlElement("w:updateFields")
    el.set(qn("w:val"), "true")
    settings.append(el)


def build_technical():
    doc = Document()
    configure(doc, "Full Technical Documentation")
    cover(doc, "Full Technical Documentation", "Developers, architects, AI engineers, security specialists, integrators, and auditors")

    h1(doc, "1. Executive summary")
    p(doc, "YP-RAKSHA, as implemented in this repository, is an enterprise AI gateway with an endpoint browser-control agent. One Go service accepts OpenAI-compatible and provider-native API traffic, applies governance, and stores configuration and logs. A React workspace is the administration console. A Python desktop agent, Gateway Guard, inspects selected public AI websites in the employee browser and can block, redact, or warn.")
    p(doc, "The product does not ship a single trained hazard-classification model that scores every prompt. Hazard handling is policy driven. Gateway traffic can pass through optional guardrail plugins. Browser traffic is matched to administrator rules that are either regular expressions or an optional AI Guard Bot that calls a configured model. Requests that never enter the gateway or the Guard proxy are not screened.")
    p(doc, "Two planes share one database and one dashboard:")
    bullets(doc, [
        "Gateway plane: unified inference API, providers, virtual keys, budgets, routing, MCP, prompts, skills, plugins, observability, and RBAC.",
        "Browser AI plane: target websites, guard rules, local or network MITM proxy, prompt logs, search logs, and fleet agents.",
    ])

    h1(doc, "2. Scope, objectives, requirements, and assumptions")
    h2(doc, "2.1 Scope")
    bullets(doc, [
        "In scope: code under core, framework, transports, plugins, ui, apps/browser-guard, deploy, and docs.",
        "In scope: authentication, RBAC, SCIM, on-premises Active Directory LDAP sync, virtual keys, and audit logs.",
        "Out of scope for automatic screening: traffic that does not use the gateway API and is not steered through Guard PAC or the network proxy.",
        "Out of scope: a claim that every hazard category is detected. Detection is limited to configured rules and optional model checks.",
    ])
    h2(doc, "2.2 Objectives visible in the implementation")
    bullets(doc, [
        "Give applications one API surface while keys for many model vendors stay on the server.",
        "Limit spend and rate by virtual key, user, team, and customer budgets.",
        "Record LLM and MCP activity for operators.",
        "Let administrators decide which public AI sites employees may use and what prompt content is blocked, redacted, or warned.",
        "Provision people from cloud directories (SCIM) or on-premises Active Directory.",
    ])
    h2(doc, "2.3 Assumptions")
    bullets(doc, [
        "PostgreSQL is the intended production database. SQLite exists for local development paths.",
        "A reverse proxy terminates public TLS. The Go process default listen port used in local docs is 6000.",
        "Provider credentials are stored as configured keys, not hard-coded in source that this review treats as production secrets.",
        "Human administrators own rule quality. False positives and false negatives are expected for regex and model rules.",
    ])

    h1(doc, "3. Technology stack and repository structure")
    table(doc, ["Layer", "Implementation"], [
        ["Gateway language", "Go. HTTP engine: fasthttp. Router: fasthttp/router."],
        ["UI", "React 19, Vite, TypeScript. Workspace routes under ui/app/workspace."],
        ["Desktop Guard", "Python agent in apps/browser-guard/agent. Packaged with PyInstaller. Windows EXE and macOS .app."],
        ["Inspection proxy", "mitmproxy addon apps/browser-guard/proxy/browser_ai_proxy.py."],
        ["ORM / config", "GORM in framework/configstore."],
        ["Logs", "framework/logstore, including Browser AI tables."],
        ["Containers", "docker-compose.yml and deploy/docker."],
        ["Charts", "deploy/helm."],
    ])
    h2(doc, "3.1 Principal directories")
    table(doc, ["Path", "Role"], [
        ["core/", "Request schemas, provider adapters, routing helpers."],
        ["core/providers/", "Vendor adapters (OpenAI, Anthropic, Azure, Bedrock, Gemini, Vertex, Mistral, Cohere, Groq, Ollama, and others)."],
        ["framework/", "Config store, log store, encrypt, feature flags, RBAC helpers, object store."],
        ["transports/gateway-http/", "HTTP handlers, session, SCIM, Active Directory, Browser AI downloads, embedded UI."],
        ["plugins/", "Guardrails, governance, logging, telemetry, semantic cache, prompts, OTel, connectors, and others."],
        ["ui/", "Administration workspace."],
        ["apps/browser-guard/", "Agent, proxy, installer, and release packages."],
        ["docs/", "Existing Markdown guides and this generated set."],
    ])
    p(doc, "Provider packages present under core/providers include openai, azure, anthropic, bedrock, bedrockmantle, gemini, vertex, mistral, cohere, groq, cerebras, deepseek, fireworks, huggingface, nebius, ollama, openrouter, perplexity, replicate, runware, runway, sgl, vllm, xai, elevenlabs, parasail, and opencode. The exact call surface of each adapter is the Go files in that folder. Not every adapter implements every modality.")

    h1(doc, "4. System architecture and data flow")
    h2(doc, "4.1 Component diagram (logical)")
    code(doc, "Employee browser\n  -> PAC (local 127.0.0.1:18195 or server PAC)\n  -> Guard MITM 127.0.0.1:18103\n  -> rule engine (regex or AI Guard Bot)\n  -> POST /api/browser-ai/intercept\n  -> PostgreSQL logs + Workspace Browser AI\n\nApplication or IDE\n  -> Gateway /v1 or provider-native path\n  -> auth (virtual key or session)\n  -> optional plugins (guardrails, cache, governance)\n  -> provider adapter\n  -> upstream model API\n  -> logs, budgets, response")
    h2(doc, "4.2 Trust boundaries")
    bullets(doc, [
        "Boundary A: public internet and upstream model vendors. Provider API keys must not be given to end users.",
        "Boundary B: employee laptop. The Guard trusts its local proxy and a company CA installed for TLS inspection. Only domains listed as targets are intended to be intercepted.",
        "Boundary C: gateway process. Session cookies and virtual keys are the caller identity.",
        "Boundary D: database. Configuration, users, rules, and logs.",
        "Boundary E: directory. SCIM bearer token or LDAP bind account. Those credentials can create users.",
    ])
    p(doc, "Implemented control: gateway admin sessions bypass per-route RBAC in transports/gateway-http/handlers/rbac_enforce.go when session.Role is admin. The user role is resolved from the stored role permission catalogue. Sub-admin API rights come from the sub_admin role seed and later edits to that role, not from each person's checkbox matrix alone. Page visibility for user and sub_admin uses allowed_sections.")

    h1(doc, "5. Module analysis")
    h2(doc, "5.1 HTTP gateway")
    p(doc, "Handlers live in transports/gateway-http/handlers. Session login, user CRUD, and password policy are in session.go. RBAC enforcement is in rbac_enforce.go and framework/rbac/enforce.go. PathRequirementFor maps API prefixes to a resource and operation. Unknown routes are not all gated. A nil requirement means the route is not RBAC-gated.")
    h2(doc, "5.2 Configuration store")
    p(doc, "framework/configstore persists users, teams, customers, virtual keys, roles, prompts, skills, and workspace settings through GORM. Users are table governance_users (framework/configstore/tables/user.go). Username is unique. Email is indexed and is not a unique constraint. Role, status, budget, rate limit, allowed prompt repositories, and allowed_sections are columns on the user.")
    h2(doc, "5.3 Skills repository")
    p(doc, "Skills are stored and served by transports/gateway-http/handlers/skills.go and skills_serving.go. Public marketplace URLs do not use the dashboard session. Claude Code fetches marketplace JSON and clones plugin git URLs. Codex clones the marketplace git URL. Cursor and Antigravity harness names are included in framework/configstore/skills.go SkillHarnessNames so git routes are registered. Serving is an in-process git smart HTTP implementation. It does not require a system git binary.")
    h2(doc, "5.4 Browser Guard")
    p(doc, "apps/browser-guard/agent/agent_config.py sets AGENT_VERSION_BAKED. At the time of this document the baked value is 1.1.20. Runtime version prefers VERSION.txt beside the install. Local proxy port used in operations notes is 18103. Local PAC port is 18195. Release artefacts under apps/browser-guard/release include Gateway_Guard.exe, Gateway_Guard_Setup.exe, Gateway_Guard_Windows.zip, Gateway_Guard_macOS.zip, and Gateway_Guard_Setup.pkg. The pkg file on disk at review time was older than the 1.1.20 zip and is not what the macOS update command installs. The update command downloads the macOS zip.")
    h2(doc, "5.5 User interface")
    p(doc, "Workspace navigation is defined in ui/components/sidebar.tsx and mirrored by grant keys in ui/lib/constants/workspaceSections.ts. Sections include Observability, Browser AI, Models, MCP Gateway, Plugins, Governance, Guardrails, Adaptive Routing, Prompt Repository, Skills Repository, and Settings. Child grants use keys such as observability/llm-logs.")

    h1(doc, "6. AI and knowledge-screening behaviour")
    h2(doc, "6.1 What is implemented")
    bullets(doc, [
        "Gateway inference is pass-through to the selected provider after auth, budgets, and optional plugins. There is no universal classifier on every /v1 request unless a guardrail plugin is configured and ordered to run.",
        "Browser Guard extracts prompts, uploads, and search queries for hosts that match Target Websites, then evaluates Guard Rules.",
        "Rule actions implemented in the product language are BLOCK, REDACT, and WARN.",
        "A rule may be a pattern or an AI Guard Bot that calls a model the administrator selects. The bot is optional per rule. It is not a fixed embedded model.",
        "Prompt logs and search logs are stored and shown under Browser AI.",
    ])
    h2(doc, "6.2 What is not implemented")
    bullets(doc, [
        "No evidence of a single accuracy benchmark, published false-positive rate, or false-negative rate in this repository.",
        "No evidence that every hazard class (for example all regulated data types) is detected unless an administrator wrote a rule for it.",
        "AI Guard Bot quality depends on the chosen model, the rule prompt, and the text the proxy extracted. Extraction can miss content the site renders in a way the addon does not parse.",
    ])
    p(doc, "Human oversight is required. Administrators should review prompt logs, tune rules, and treat model judgements as advisory enforcement, not as a guarantee.")

    h1(doc, "7. Access control")
    h2(doc, "7.1 Roles")
    table(doc, ["Role", "Login behaviour found in code"], [
        ["admin", "API middleware allows the request without the resource catalogue. Workspace sections are not filtered."],
        ["user", "Pages come from allowed_sections. If none are stored, the UI and API fall back to Prompt Repository. Write APIs depend on the user role permission rows. The seed grants View and Read, not Create, Update, or Delete."],
        ["sub_admin", "Pages come from allowed_sections. API operations come from the sub_admin role catalogue. The seed is full control for some resources (Logs, Virtual Keys, Prompt Repository) and view or absent for others. Selecting a page on the user form does not by itself grant every write API for that page."],
    ])
    p(doc, "The Roles and Permissions screen can show a per-person matrix derived from allowed_sections: full ticks for admin, all operations on mapped resources for sub_admin, and View or Read only for user. Saving that person updates allowed_sections. The enforcement function ResolvePermissions loads the role row, not a separate per-user operation list. Treat the global All Users and All Sub Admins policies as the live API policy.")
    h2(doc, "7.2 Directory provisioning")
    p(doc, "Cloud SCIM is implemented at /scim/v2 for Users and Groups. Providers accepted in configuration are okta, entra, and keycloak. The bearer token is required. New SCIM users are created with role user. Matching uses external id, email, or user name, so the same email is not inserted twice. Groups become teams. A configured default team can be applied.")
    p(doc, "On-premises Microsoft Active Directory uses LDAP in transports/gateway-http/handlers/active_directory.go. Sync creates approved users with role from default_role, which defaults to user. It skips an existing username (case-insensitive). It does not look up email, so the same mailbox with a different username can create a second account. It assigns the SCIM default team if one is configured, sends a temporary password mail, and asks the prompt lifecycle to create a prompt named for the email. It does not set allowed_sections, a virtual key, or an admin role.")

    h1(doc, "8. APIs")
    p(doc, "Inference routes follow OpenAI-compatible /v1 paths and native integration prefixes. Exact route tables are registered in the transport server. Browser AI routes include /api/browser-ai/logs, targets, rules, agents, setup downloads, and intercept. Governance routes live under /api/governance and /api/session. SCIM routes are unauthenticated except for the bearer token middleware.")
    p(doc, "Errors use HTTP status codes. SCIM errors use the SCIM error schema with status, detail, and optional scimType. Dashboard RBAC failures return 403 with an insufficient-permissions or section message.")
    p(doc, "Do not publish real tokens. Examples in the system manual use placeholders such as <VIRTUAL_KEY> and <SCIM_BEARER_TOKEN>.")

    h1(doc, "9. Data")
    p(doc, "PostgreSQL is configured with DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD, and DB_SSL_MODE. Migrations run from the config store. Browser AI logs, agents, rules, and targets are part of the log store and related tables. Skill files may live in object storage or database blobs. Prompt bodies live in the prompt tables.")
    p(doc, "Personal data includes usernames, emails, prompt text stored in Browser AI logs, and virtual-key usage. Logs can contain confidential prompts. Retention and access should be treated as an operational control. This repository does not, by itself, prove a fixed retention period for every log type.")

    h1(doc, "10. Security, privacy, and standards mapping")
    p(doc, "The following mapping is an engineering aid. It is not a certificate and not evidence of a completed audit.")
    table(doc, ["Theme", "Where the code supports it", "Gap"], [
        ["Access control (ISO 27001 A.5 / 27002 access)", "Session login, RBAC resources, allowed_sections, virtual keys.", "Per-user operation matrix is not the runtime policy."],
        ["Cryptography", "Password hashes, TLS to AD when ldaps or StartTLS is used, Guard CA for inspection.", "AD can be configured with insecure TLS skip. That is an explicit risk."],
        ["Logging", "LLM logs, MCP logs, audit logs, Browser AI prompt and search logs.", "Log content may itself be sensitive."],
        ["Supplier AI services", "Provider adapters. Keys stay server-side.", "Upstream vendors process prompts. That is outside this codebase."],
        ["AI system lifecycle (ISO 42001 themes)", "Configurable policies, human-administered rules, logs for review.", "No formal model-risk file, no measured accuracy, no claim of continuous monitoring of model drift."],
    ])

    h1(doc, "11. Testing evidence")
    p(doc, "Go tests exist beside handlers, including browser AI, RBAC scope grants, and SCIM Entra cases (scim_entra_test.go). Python tests exist under apps/browser-guard/agent/tests. This document does not quote pass rates. A test file proves intent and some regressions. It does not prove production accuracy of hazard detection.")

    h1(doc, "12. Build and deployment (technical view)")
    bullets(doc, [
        "Gateway image: deploy/docker/Dockerfile.local referenced by docker-compose.yml service gateway_tech.",
        "Required environment includes APP_PORT, SERVER_DOMAIN, database settings, ADMIN_EMAIL, ADMIN_PASSWORD, and GATEWAY_GUARD_SECRET.",
        "Optional network proxy: docker compose profile network-proxy.",
        "UI development server port in Vite defaults from UI_PORT or 3100. It proxies to the gateway.",
        "Guard Windows build is a Windows plus Inno Setup pipeline. macOS .app and pkg require a Mac build for a new native binary. Windows can refresh VERSION.txt and encrypted code into an existing .app, which is what the 1.1.20 macOS zip reflects.",
    ])

    h1(doc, "13. Gaps, limitations, and recommendations")
    bullets(doc, [
        "Align sub-admin API rights with the features selected on the user form, or change the screen so it does not imply full access for every selected feature.",
        "Check email as well as username during Active Directory sync.",
        "Persist per-user operation overrides if the matrix is meant to change live access.",
        "Rebuild Gateway_Guard_Setup.pkg on a Mac when a signed package at 1.1.20 is required. The zip used by the macOS updater is the current channel.",
        "Publish an operator review cadence for Guard rules and prompt logs.",
        "Keep provider keys and the Guard shared secret out of tickets and documents.",
    ])

    h1(doc, "14. Handler and route map")
    p(doc, "Route registration is split by handler. The files below are the places to start a code review. A route existing in a RegisterRoutes function does not mean every deployment has enabled the feature behind it.")
    table(doc, ["Handler file", "Responsibility"], [
        ["handlers/inference.go", "Chat and text completions presented to callers."],
        ["integrations/router.go", "Provider-native integration routes."],
        ["handlers/providers.go", "Provider configuration API."],
        ["handlers/governance.go", "Virtual keys, users, teams, customers, budgets."],
        ["handlers/session.go", "Login, logout, password change, session cookie."],
        ["handlers/rbac_enforce.go", "Which API prefix requires which resource."],
        ["handlers/workspace.go", "Workspace settings, SCIM, Active Directory routes."],
        ["handlers/scim_handlers.go", "SCIM Users and Groups."],
        ["handlers/active_directory.go", "LDAP test and user sync."],
        ["handlers/skills.go and skills_serving.go", "Skill CRUD and public git marketplaces."],
        ["handlers/mcp.go and mcp*.go", "MCP catalog, sessions, OAuth, inference."],
        ["handlers/plugins.go", "Plugin install and ordering."],
        ["handlers/guardrails.go", "Guardrail rule configuration."],
        ["handlers/logging.go", "Log queries."],
        ["handlers/health.go", "Health endpoint."],
        ["handlers/ui.go", "Embedded React application."],
        ["handlers/browser AI download handlers", "Setup zip, version header, intercept ingest."],
    ])
    h2(doc, "14.1 Inference path")
    numbered(doc, [
        "The caller sends an OpenAI-style request with a virtual key or another accepted credential.",
        "The gateway resolves the virtual key, checks status, budgets, and rate limits.",
        "Routing rules, complexity routing, or an explicit model name select a provider.",
        "Plugins that are enabled and ordered for that request may modify or reject it. A guardrail plugin is not active unless an administrator configured it.",
        "The provider adapter in core/providers translates the request and calls the vendor.",
        "Usage is written to logs and budget counters. The response is returned to the caller.",
    ])
    p(doc, "Streaming responses use the same auth and budget checks before the stream starts. A mid-stream vendor failure is a provider error, not a hazard classification.")

    h1(doc, "15. Plugins")
    p(doc, "The plugins directory contains optional pipeline stages. Names present in the tree include governance, guardrails, logging, telemetry, semantic cache, prompts, otel, and data connectors. Each plugin runs only when it is enabled in configuration. Document a deployment by listing the enabled plugins from the Plugins screen, not by assuming the whole directory is active.")

    h1(doc, "16. MCP")
    p(doc, "MCP Gateway pages cover a catalog, a library, tool groups, auth sessions, OAuth grants, and settings. Virtual keys can carry MCP configuration. OAuth issuance and consent handlers exist so a client can be authorised without pasting a long-lived vendor token into the laptop. Tool calls that pass through the gateway can be logged as MCP logs. Tools that a user runs outside the gateway are not visible.")

    h1(doc, "17. Skills serving detail")
    table(doc, ["Harness", "How the client is expected to connect"], [
        ["Claude Code", "HTTP marketplace JSON, then clone of plugin URLs."],
        ["Codex", "Clone of the marketplace git URL itself."],
        ["Cursor", "Clone of /api/skills/serve/cursor. The marketplace file is .cursor-plugin/marketplace.json and plugin sources are paths inside that clone, not remote URLs."],
        ["Antigravity", "Plugin manifest at the repository root with name and description only. Install command uses the gateway-all-skills plugin URL."],
    ])
    p(doc, "Public skill git routes do not require a dashboard session. Treat published skill text as visible to anyone who can reach the gateway URL. Do not put secrets inside a skill file.")

    h1(doc, "18. Browser AI data flow")
    numbered(doc, [
        "An administrator lists target hosts and rules.",
        "The laptop agent sets a local PAC. Default local ports used by the product are 18195 for PAC and 18103 for the inspection proxy.",
        "Only matching hosts are intended to be decrypted and inspected.",
        "The proxy extracts prompt text, uploads, or search queries when its parsers recognise the site.",
        "Rules run. Actions are block, redact, or warn.",
        "The event is posted to the gateway intercept API and appears in Prompt Logs or Search Logs.",
        "An optional network proxy container exists behind the compose profile network-proxy. It is off by default. Employee production path documented in compose comments is the laptop program.",
    ])
    p(doc, "GATEWAY_FAIL_OPEN is set to 0 on the optional network proxy, meaning a failure to reach the gateway is not configured there as a silent allow. Confirm the laptop agent policy in the installed config before you state the same for desktops.")

    h1(doc, "19. Persistence tables")
    p(doc, "GORM models under framework/configstore/tables include, among others, users, teams, business units, customers, virtual keys and their provider and MCP bindings, providers, skills and skill file blobs, prompts and prompt sessions, RBAC roles, access profiles, audit logs, SMTP, login lockout, password reset, circuit breaker policies, feature flags, model pricing, and workspace settings. Browser AI log tables live with the log store. Exact column lists are the Go structs. Do not invent extra columns in an integration.")

    h1(doc, "20. Authentication mechanics")
    bullets(doc, [
        "Dashboard users authenticate with username or email and a password. Passwords are stored as hashes.",
        "A temporary password from user create, SCIM, or AD sync forces a password change. The policy requires length and character classes implemented in the session handler.",
        "Login lockout and device tables exist. Use them as evidence that lockout is a designed control. Confirm the live thresholds in configuration rather than assuming a number.",
        "API callers use virtual keys. A virtual key is not a user session and does not open the admin UI by itself.",
        "SCIM uses a bearer token. AD sync uses a bind account stored in the AD configuration.",
    ])

    h1(doc, "21. Threat model notes")
    table(doc, ["Threat", "Relevant control", "Residual risk"], [
        ["Stolen vendor API key", "Keys stored on the gateway. Users receive virtual keys.", "An admin who can open providers can read or replace keys. Protect that role."],
        ["Stolen virtual key", "Budgets, rate limits, status, and audit of usage.", "A key can still spend until it is revoked."],
        ["Employee pastes secrets into an unlisted AI site", "No control. The site is not a target.", "Policy and training. The product will not see the paste."],
        ["Regex misses a secret format", "Add or tune a rule. Optional model rule.", "False negatives remain."],
        ["Model rule blocks harmless text", "Warn action instead of block. Human review of logs.", "False positives remain."],
        ["AD insecure TLS", "Prefer ldaps and a trusted certificate.", "Skip-verify is available and is unsafe on a hostile network."],
        ["SCIM token leak", "Rotate the token. Restrict network path to the IdP.", "Anyone with the token can create users while SCIM is enabled."],
        ["Skill repository is public", "Do not publish secrets in skills.", "The marketplace URL is intentionally unauthenticated."],
    ])

    h1(doc, "22. Configuration identity")
    p(doc, "docker-compose.yml passes these identity variables into the container. Defaults are shown so operators can see what an empty override produces.")
    table(doc, ["Variable", "Compose default"], [
        ["GATEWAY_PRODUCT_NAME", "Gateway"],
        ["GATEWAY_BRAND_NAME", "Gateway"],
        ["GATEWAY_PRODUCT_SUBTITLE", "Real-time AI Knowledge Screening & Hazard Audit"],
        ["GATEWAY_COMPANY_NAME", "YesPanchi Group of Companies"],
        ["GATEWAY_COMPANY_SHORT_NAME", "YesPanchi"],
        ["GATEWAY_FOOTER_SUBTITLE", "Gateway - Real-time AI Knowledge Screening & Hazard Audit"],
        ["APP_HOST", "0.0.0.0"],
        ["LOG_LEVEL", "info"],
        ["LOG_STYLE", "json"],
        ["DB_TYPE", "postgres"],
        ["DB_SSL_MODE", "disable"],
        ["GATEWAY_GUARD_REQUIRE_SECRET", "1"],
        ["GATEWAY_PAC_ALLOW_QUERY_PROXY", "0"],
    ])
    p(doc, "APP_PORT, SERVER_DOMAIN, database credentials, ADMIN_EMAIL, ADMIN_PASSWORD, GATEWAY_GUARD_SECRET, and CONTAINER_NAME have no safe default. Compose fails closed if they are missing from .env.")

    h1(doc, "23. CI, quality, and dependency practice")
    p(doc, "Go tests and Python agent tests are in the tree. This review did not execute a full suite and does not report a pass percentage. Dependency updates should follow your normal process: review go.mod and ui/package.json, run tests, and avoid committing .env. There is no statement in code that a vulnerability scanner runs on every commit. If your pipeline has one, document that pipeline separately.")

    h1(doc, "24. Glossary")
    table(doc, ["Term", "Meaning in this product"], [
        ["Virtual key", "A gateway credential with budgets and provider scope. End users should use this, not a vendor master key."],
        ["Allowed sections", "Comma-separated workspace page grants stored on the user."],
        ["Guard rule", "An administrator policy applied to intercepted browser AI traffic."],
        ["Target website", "A host the Guard is instructed to inspect."],
        ["SCIM", "Directory provisioning API at /scim/v2."],
        ["AI Guard Bot", "Optional model call used by a guard rule. Not a built-in certified classifier."],
    ])
    add_tech(doc, h1, h2, p, bullets, numbered, table, code)
    enable_update_fields(doc)
    path = OUT + r"\YP_RAKSHA_Full_Technical_Documentation.docx"
    doc.save(path)
    return path


def build_system():
    doc = Document()
    configure(doc, "System Manual")
    cover(doc, "System Manual", "System administrators, DevOps, IT operations, and security teams")

    h1(doc, "1. System overview")
    p(doc, "Operators run one gateway service, one PostgreSQL database, and optionally a network proxy. Employees who need browser controls install Gateway Guard. Administrators use the workspace UI. This manual separates steps that match the repository from practices that are recommended but not encoded as a single script.")

    h1(doc, "2. Prerequisites")
    table(doc, ["Item", "Evidence in repository"], [
        ["Gateway", "Go service. Docker Compose is the packaged path."],
        ["Database", "DB_TYPE default postgres. Port comes from DB_PORT."],
        ["UI dev", "Node tooling in ui/. Production UI is embedded in the gateway binary."],
        ["Windows Guard build", "Windows host and Inno Setup for Setup.exe."],
        ["macOS Guard native rebuild", "Must be built on a Mac. A Windows host cannot produce a new signed Mach-O."],
        ["Directory", "Outbound LDAP or LDAPS to a domain controller, or inbound HTTPS from the IdP to /scim/v2."],
    ])
    p(doc, "Hardware sizing is not fixed in code. Existing docs discuss production sizing. Treat those as planning guidance and verify on your workload. This manual does not invent CPU or RAM minimums beyond what you measure.")

    h1(doc, "3. Installation with Docker Compose")
    p(doc, "Verified shape of deployment, from docker-compose.yml:")
    numbered(doc, [
        "Clone the repository onto the server.",
        "Create a .env file. Set CONTAINER_NAME, APP_PORT, SERVER_DOMAIN, DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD, and GATEWAY_GUARD_SECRET. Use your own values. Do not copy secrets into tickets.",
        "Optional brand variables: GATEWAY_PRODUCT_NAME, GATEWAY_PRODUCT_SUBTITLE, GATEWAY_COMPANY_NAME. Defaults are Gateway, the hazard-audit subtitle, and YesPanchi Group of Companies.",
        "From the repository root run: docker compose up -d --build",
        "Expected result: container gateway_tech (or your CONTAINER_NAME) is running and listens on APP_PORT.",
        "Open https://<SERVER_DOMAIN> through your reverse proxy, or http://<host>:<APP_PORT> on a trusted network.",
        "Sign in with ADMIN_EMAIL and the admin password. Change it after first login if policy requires.",
    ])
    p(doc, "Optional network proxy, only if you intend office PAC inspection on the server:")
    code(doc, "docker compose --profile network-proxy up -d")
    p(doc, "The compose comment states the production employee path is the Guard program on each laptop, not the shared proxy.")

    h1(doc, "4. Configuration reference")
    p(doc, "Only variables referenced by docker-compose.yml or the Guard and gateway code should be treated as real. Common ones:")
    table(doc, ["Variable", "Purpose"], [
        ["APP_PORT", "Gateway listen port."],
        ["APP_HOST", "Bind address. Compose default 0.0.0.0."],
        ["SERVER_DOMAIN", "Public origin used in links and Guard config."],
        ["DB_*", "PostgreSQL connection."],
        ["DB_SSL_MODE", "Default disable in compose. Set require when the database demands TLS."],
        ["ADMIN_EMAIL / ADMIN_PASSWORD", "Bootstrap administrator."],
        ["GATEWAY_GUARD_SECRET", "Shared secret for Guard downloads and agent calls. Required."],
        ["GATEWAY_GUARD_REQUIRE_SECRET", "Default 1. Keep it on."],
        ["LOG_LEVEL / LOG_STYLE", "Process logs. Style default json."],
    ])
    p(doc, "Local Guard ports, from operational notes in the repository, are 18103 for the MITM proxy and 18195 for the PAC server, both on 127.0.0.1. Do not publish those ports to the office network.")

    h1(doc, "5. Provider and model configuration")
    numbered(doc, [
        "Sign in as admin.",
        "Open Models, then Model Providers.",
        "Add a provider that exists in core/providers and paste the vendor key into the secret field. The key is stored by the gateway.",
        "Confirm the model appears in Model Catalog.",
        "Create a virtual key under Governance, Virtual Keys. Attach budgets if you need spend limits.",
        "Call the gateway with Authorization: Bearer <VIRTUAL_KEY>. Expected result: a chat completion returns when the provider accepts the model name.",
    ])
    p(doc, "If the provider returns an authentication error, the gateway is up and the vendor rejected the key. Rotate the key in the provider console and update the stored key. Do not send the master key to laptops.")

    h1(doc, "6. Database")
    numbered(doc, [
        "Create a PostgreSQL database and user matching DB_NAME and DB_USER.",
        "Grant that user rights on the database.",
        "Point DB_HOST and DB_PORT at the instance. Use 5432 unless you front the database with a pooler.",
        "Start the gateway. GORM migrations in framework/configstore run as part of startup.",
        "Verify by signing in. A successful admin login means the user tables were created and the bootstrap admin was accepted.",
    ])
    p(doc, "Backup and restore commands are operational. Take a logical dump with pg_dump using your own connection string. Restore with psql or pg_restore. The repository Markdown guides describe schedules. Confirm the schedule on your host. This manual does not claim an already installed cron job.")

    h1(doc, "7. Gateway Guard installation")
    h2(doc, "7.1 Windows")
    numbered(doc, [
        "In Browser AI, Setup, download Gateway_Guard_Windows.zip.",
        "Extract the zip fully. Do not run Setup from inside the zip.",
        "Run Gateway_Guard_Setup.exe. Leave start at login enabled if that is your standard.",
        "Expected result: Gateway Guard appears in Apps, and Browser AI, Guard Agents shows the laptop after it heartbeats.",
    ])
    p(doc, "Update: run Update_Gateway_Guard.ps1 beside a config file that contains backend_url and guard_secret. The script downloads /api/browser-ai/setup/download-windows.zip, extracts Gateway_Guard_Setup.exe, and installs it silently. Current release version advertised with the 1.1.20 packages is 1.1.20.")
    h2(doc, "7.2 macOS")
    numbered(doc, [
        "Download Gateway_Guard_macOS.zip from Browser AI, Setup.",
        "Or run Update_Gateway_Guard_macOS.command. It reads gateway_guard_config.json, downloads the macOS zip, and copies Gateway_Guard.app into the user or system Applications folder.",
        "Expected result: the app opens and the agent heartbeats. Version files inside the current zip are 1.1.20.",
    ])
    p(doc, "Unsupported as a current 1.1.20 installer: Gateway_Guard_Setup.pkg in the release folder was older at review time and is not inside the macOS zip. Do not distribute that pkg as the 1.1.20 update. Build a new pkg on a Mac if you need a package installer.")

    h1(doc, "8. Users, roles, and policies")
    numbered(doc, [
        "Open Governance, Users.",
        "Create a user with email. The server generates a temporary password and emails it. First login must set a new password.",
        "Role User: choose team, virtual key, workspace pages, and prompt repositories. Unchecked pages stay hidden, except Prompt Repository remains available for the user role.",
        "Role Sub-admin: choose workspace pages. Those pages show in the sidebar. Write access still follows the sub_admin role in Roles and Permissions, not every tick on the user form.",
        "Role Admin: full workspace and API access.",
        "Open Governance, Roles and Permissions to edit the All Users or All Sub Admins policy if the whole role must change.",
    ])
    h2(doc, "8.1 Browser screening rules")
    numbered(doc, [
        "Open Browser AI, Target Websites. Add hosts you intend to inspect.",
        "Open Guard Rules. Add a regex rule or an AI Guard Bot rule. Set the action to block, redact, or warn.",
        "Confirm a test prompt from a managed browser appears in Prompt Logs.",
        "If it does not, the site is not a target, the PAC is not applied, or the agent is offline.",
    ])
    p(doc, "There is no global switch in code that proves every AI site on the internet is screened. Only configured targets are in scope.")

    h1(doc, "9. Directory connection")
    h2(doc, "9.1 Cloud SCIM")
    numbered(doc, [
        "Open Governance, User Provisioning. Choose Cloud Identity Providers.",
        "Select Okta, Entra, or Keycloak. Enable SCIM. Save.",
        "Copy the base URL ending in /scim/v2 and the bearer token into the IdP. Store the token in a password manager.",
        "Assign a test user. Expected result: the person appears under Governance, Users with role User.",
        "Push a group if you use groups. Expected result: a team with that display name.",
    ])
    h2(doc, "9.2 On-premises Active Directory")
    numbered(doc, [
        "Open the Microsoft Active Directory tab.",
        "Set the server URL (ldap:// or ldaps://), domain, base DN, and bind account. Prefer LDAPS. Avoid insecure certificate skip except in a lab.",
        "Save, then Test connection. Expected result: a success message and a sample of users.",
        "Sync Users Now. New accounts are role User, approved, and emailed a temporary password.",
        "If a username already exists, that account is left unchanged.",
        "Set a default team in the SCIM configuration if new AD users must land in a team. Otherwise they have no team.",
        "After sync, edit each user who needs more than Prompt Repository: virtual key and workspace pages.",
    ])

    h1(doc, "10. Logging, health, backup, and upgrades")
    bullets(doc, [
        "Gateway logs follow LOG_LEVEL and LOG_STYLE.",
        "Product audit logs are the Audit Logs workspace page, when the caller is allowed to open it.",
        "LLM Logs and Browser AI Prompt Logs are the operational record of AI use.",
        "Health: process up, admin login works, a virtual-key chat returns, a Guard agent heartbeat appears.",
        "Upgrade the gateway by rebuilding the image and recreating the container. Take a database dump first.",
        "Upgrade Guard with the Windows or macOS update script, or by shipping the matching zip from Setup.",
    ])

    h1(doc, "11. Troubleshooting")
    table(doc, ["Symptom", "Check"], [
        ["Admin login fails", "ADMIN_EMAIL, password, and database connectivity. Read gateway logs."],
        ["Provider 401", "Stored vendor key and model name. The gateway itself is reachable."],
        ["User sees only Prompt Repository", "allowed_sections is empty. Edit the user and grant pages."],
        ["User cannot send a prompt", "No virtual key, or budget exhausted."],
        ["Sub-admin sees a page but save returns 403", "The sub_admin role catalogue does not include that write operation."],
        ["SCIM 403", "SCIM disabled or wrong bearer token."],
        ["AD test fails", "Firewall, bind DN, password, base DN, or TLS mode."],
        ["Duplicate person after AD sync", "Email matched an existing user with a different username. Merge manually. Sync will not do it."],
        ["Guard does not log prompts", "Agent offline, PAC not set, or host not in Target Websites."],
        ["macOS pkg looks old", "Use the zip. Do not ship the stale pkg."],
    ])

    h1(doc, "12. Startup, shutdown, and restart")
    numbered(doc, [
        "Start: docker compose up -d. Expected result: gateway container status is running.",
        "Stop: docker compose stop. Expected result: the port no longer accepts connections. The database is unchanged.",
        "Restart after a config edit in .env: docker compose up -d --force-recreate. Expected result: the new environment is visible in docker compose exec printenv for a non-secret key such as APP_PORT.",
        "Rebuild after code changes: docker compose up -d --build. Take a database backup first on a production host.",
    ])
    p(doc, "Do not delete the ./data volume as a restart step. That directory holds runtime data including config.json. The compose file warns not to bind-mount a single file onto /app/data/config.json because atomic saves then fail.")

    h1(doc, "13. Health checks you can run")
    numbered(doc, [
        "Container: docker compose ps. Expected result: gateway_tech is Up.",
        "Process health: GET http://<host>:<APP_PORT>/health or /api/health. Expected result: JSON with status ok when the process can answer. A database problem is reported inside that payload when DB pings are enabled.",
        "Application: sign in as admin in a browser.",
        "Model path: one chat call with a virtual key and a known model.",
        "Guard path: one online agent on Browser AI, Guard Agents, and one test prompt in Prompt Logs.",
    ])
    p(doc, "A green container is not proof that a provider key works or that browser rules match your sites.")

    h1(doc, "14. SMTP and mail")
    p(doc, "User creation, password reset, and AD sync send mail through the configured SMTP settings (handlers/smtp.go and tables/smtp.go). If mail is not configured, users are still created but they may not receive the temporary password. Check SMTP in settings before a bulk AD sync. Do not put the SMTP password in this document.")

    h1(doc, "15. Ports")
    table(doc, ["Port", "Where it is defined", "Exposure"], [
        ["APP_PORT", ".env, published by compose", "Reachable as configured. Put TLS in front for production."],
        ["DB_PORT", ".env", "Database network only."],
        ["PROXY_PORT", ".env, only with profile network-proxy", "Do not enable unless you intend a shared office proxy."],
        ["18103", "Laptop Guard MITM", "Loopback. Not a server firewall opening."],
        ["18195", "Laptop PAC", "Loopback."],
    ])
    p(doc, "Older Markdown in docs/ mentions additional host ports such as 6000. Those are examples from a specific deployment, not a value compose will choose for you. Use the APP_PORT in your .env.")

    h1(doc, "16. What this manual does not verify")
    bullets(doc, [
        "A live login on your server was not performed while writing this edition.",
        "A full Go test run was not used as a release gate for this document.",
        "Mac package signing and notarisation are not verified. The zip version label 1.1.20 is a file fact. A newly signed pkg is a separate Mac build.",
        "Backup cron, disk encryption, and SIEM forwarding are recommended operations. They are not installed by the repository automatically.",
    ])

    h1(doc, "17. Hardening checklist")
    numbered(doc, [
        "Place the gateway behind TLS. Do not expose database ports on the public internet.",
        "Set DB_SSL_MODE to the value your database requires.",
        "Keep GATEWAY_GUARD_REQUIRE_SECRET at 1.",
        "Use LDAPS for Active Directory.",
        "Restrict who can open Roles and Permissions and User Provisioning.",
        "Review prompt logs for sensitive data and limit who can export them.",
        "Rotate ADMIN_PASSWORD, provider keys, SCIM bearer token, and GATEWAY_GUARD_SECRET on staff changes.",
    ])
    add_system(doc, h1, h2, p, bullets, numbered, table, code)
    enable_update_fields(doc)
    path = OUT + r"\YP_RAKSHA_System_Manual.docx"
    doc.save(path)
    return path


def build_user():
    doc = Document()
    configure(doc, "User Manual")
    cover(doc, "User Manual", "Employees, business users, and administrators who use the website")

    h1(doc, "1. Introduction")
    p(doc, "YP-RAKSHA helps an organisation use AI services with control. People and applications send AI requests through the company gateway, or use approved websites that the company browser protection watches. The product records activity and can block, hide, or warn about content when an administrator has created a rule.")
    p(doc, "It does not read every AI conversation on the internet. It applies to the gateway and to the websites your administrator listed. A rule can miss something, or it can stop a harmless message. A person still has to decide what may be shared with an AI service.")

    h1(doc, "2. What you can do")
    bullets(doc, [
        "Sign in to the workspace your administrator gave you.",
        "Open the pages that appear in your sidebar. Hidden pages are not granted to you.",
        "Use Prompt Repository to work with prompts, when that page is available.",
        "If you are an administrator, manage users, keys, models, rules, and logs.",
        "If Gateway Guard is installed on your computer, browsing an approved AI site may be checked against company rules.",
    ])

    h1(doc, "3. Before you start")
    bullets(doc, [
        "You need the company web address, a user name, and a password.",
        "New accounts receive a temporary password by email. The first sign-in asks for a new password with at least 8 characters, including upper case, lower case, a number, and a symbol.",
        "Chat inside Prompt Repository needs a virtual key. Without one you can open prompts but you cannot send messages. Ask an administrator if the key is missing.",
        "Screenshot to add: the sign-in page at your company address.",
    ])

    h1(doc, "4. Sign in")
    numbered(doc, [
        "Open the company YP-RAKSHA address in a browser.",
        "Enter your email or user name and password.",
        "If this is the first time, set a new password when asked.",
        "Expected result: the workspace opens. You land on Prompt Repository if your role is User, or on a home page your grants allow.",
    ])
    p(doc, "If sign-in fails, check the password and caps lock. Use the password reset on the sign-in page if it is shown. If you still cannot enter, contact your administrator. Do not share the temporary password in chat.")

    h1(doc, "5. The sidebar")
    p(doc, "The left sidebar is the map of the product. You only see sections an administrator allowed. Names you may see, when allowed, are:")
    bullets(doc, [
        "Observability: Dashboard, LLM Logs, MCP Logs, Connectors, Logs Settings.",
        "Browser AI: Overview, Target Websites, Guard Rules, Prompt Logs, Search Logs, Setup, Guard Agents, Guard Insights.",
        "Models: providers, catalog, budgets, routing, circuit breaker, pricing.",
        "MCP Gateway: catalog, library, tool groups, sessions, OAuth grants, settings.",
        "Plugins.",
        "Governance: Virtual Keys, Users, Teams, Business Units, Customers, User Provisioning, Roles and Permissions, Access Profiles, Audit Logs.",
        "Guardrails: Rules, Rule Providers, Cluster Config.",
        "Adaptive Routing, Prompt Repository, Skills Repository, and Settings.",
    ])
    p(doc, "Screenshot to add: your sidebar after login, with private data covered.")

    h1(doc, "6. Prompt Repository")
    numbered(doc, [
        "Open Prompt Repository.",
        "Select a prompt you are allowed to use. If the list is empty, your administrator has not granted a repository.",
        "Choose a virtual key if the panel asks for one. The panel can show your personal budget and the key budget.",
        "Write the message and send it.",
        "Expected result: a reply appears, or a clear error if the key, budget, or model is blocked.",
    ])
    p(doc, "Customer budget figures are an organisation limit. Do not treat them as your personal allowance. If a message says a budget is used up, stop and ask an administrator. Do not try another key that was not issued to you.")

    h1(doc, "7. Browser protection, in plain language")
    p(doc, "Gateway Guard is a program on the computer. When you open a watched AI website, the program can see the text you are about to send and compare it with company rules.")
    bullets(doc, [
        "Block: the message is stopped.",
        "Redact: part of the message is hidden or replaced before it continues.",
        "Warn: you are told about a problem. Follow the instruction on the screen.",
    ])
    p(doc, "A rule may use a pattern, such as a format that looks like a secret, or it may ask an AI model to judge the text. Both can be wrong. If you believe a block is a mistake, do not try to sneak the text through another site. Contact the administrator and describe the task without pasting the sensitive text into email.")
    p(doc, "Screenshot to add: a block or warn banner from a test that contains no real customer data.")

    h1(doc, "8. If you are an administrator")
    h2(doc, "8.1 Add a person")
    numbered(doc, [
        "Open Governance, then Users.",
        "Choose Add. Enter user name and email.",
        "Choose the role. User is the normal employee role. Sub-admin manages selected areas. Admin can open everything.",
        "For a User, choose a team, a virtual key, the pages they may open, and the prompt repositories they may use.",
        "Save. Expected result: the person receives an email with a temporary password and appears in the user table.",
    ])
    p(doc, "Virtual keys on the user table stay short. One key is shown on the row. Several keys appear as a count. Select that count to open the list of key names and customer or team labels.")
    h2(doc, "8.2 Roles and Permissions")
    p(doc, "This page has a global policy for all admins, all sub-admins, and all users, and a row for each person. Live sign-in follows the role policy and the pages stored on the user. Changing ticks on one person updates which pages they have. It does not, by itself, give a sub-admin every edit right on a newly ticked page. Use All Sub Admins when the whole role must gain or lose an action.")
    h2(doc, "8.3 User Provisioning")
    p(doc, "Cloud directories (Microsoft Entra, Okta, Keycloak) can create users and teams. On-premises Active Directory can import users. Imported people arrive as role User. They do not receive a virtual key or extra pages automatically. After import, edit the user if they need more than Prompt Repository.")
    h2(doc, "8.4 Browser AI administration")
    numbered(doc, [
        "Add the AI websites you want to watch under Target Websites.",
        "Add Guard Rules and choose block, redact, or warn.",
        "Use Prompt Logs to see what was intercepted. Treat the text as confidential.",
        "Use Guard Agents to see which computers are reporting.",
        "Use Setup to download the Windows or Mac package. Current packages in this release line are version 1.1.20. Prefer the zip files. Do not hand out an old Mac package file.",
    ])

    h1(doc, "9. Your responsibilities")
    bullets(doc, [
        "Do not paste passwords, national identifiers, customer lists, or unpublished financials into an AI box unless a written company rule allows that exact use.",
        "Do not send your virtual key or the temporary password to another person.",
        "Do not turn off Gateway Guard or install a personal bypass proxy.",
        "If a warning appears, read it. Do not rephrase a secret until the warning disappears unless security staff told you that is acceptable.",
        "Report a suspected leak to security even if the tool did not block it.",
    ])

    h1(doc, "10. Common questions")
    table(doc, ["Question", "Answer"], [
        ["Why is my menu smaller than a colleague's?", "Your account has fewer pages. Ask an administrator. Do not share logins."],
        ["I can open a prompt but cannot send.", "A virtual key is missing, inactive, or out of budget."],
        ["I was imported from Active Directory. Why can I only see prompts?", "Import creates a basic user. Pages and keys are added afterwards."],
        ["Does the tool check every website?", "No. Only listed targets, and only when Guard is running."],
        ["Can the tool be wrong?", "Yes. It can block a safe sentence or miss a risky one. People remain responsible."],
        ["Who sees prompt logs?", "Administrators who were granted Browser AI logs. Assume the text is stored."],
    ])

    h1(doc, "11. Pages you may be asked to use")
    p(doc, "Each row is a real workspace path. You see it only when your account is allowed to open it.")
    table(doc, ["What you want", "Where to go"], [
        ["See AI usage", "Observability, LLM Logs"],
        ["See tool-call logs", "Observability, MCP Logs"],
        ["Watch a website", "Browser AI, Target Websites"],
        ["Stop or warn on text", "Browser AI, Guard Rules"],
        ["Read intercepted prompts", "Browser AI, Prompt Logs"],
        ["Install the computer program", "Browser AI, Setup"],
        ["See which computers report in", "Browser AI, Guard Agents"],
        ["Add a model vendor", "Models, Model Providers"],
        ["Create a key for an app", "Governance, Virtual Keys"],
        ["Add a colleague", "Governance, Users"],
        ["Connect a company directory", "Governance, User Provisioning"],
        ["Change what a role can do", "Governance, Roles and Permissions"],
        ["Read admin change history", "Governance, Audit Logs"],
        ["Work with prompts", "Prompt Repository"],
        ["Publish a skill", "Skills Repository"],
    ])

    h1(doc, "12. Reading a prompt log")
    numbered(doc, [
        "Open Browser AI, Prompt Logs.",
        "Find the time and the computer or user column.",
        "Read the action. Block means it was stopped. Redact means text was changed. Warn means the person was notified.",
        "Do not copy the prompt into chat, email, or a ticket if it contains personal or customer data. Describe the incident instead.",
        "Expected result: you can explain what the rule did. You cannot treat one log line as proof that no other copy of the text exists on the AI vendor side if the action was warn or if the request was not intercepted.",
    ])

    h1(doc, "13. Glossary")
    table(doc, ["Word", "Plain meaning"], [
        ["Gateway", "The company service that calls AI models for you so vendor passwords stay on the server."],
        ["Virtual key", "Your key for that gateway. It is not the same as a personal ChatGPT password."],
        ["Hazard", "In this product, content an administrator rule is trying to stop or mark. It is not a fixed legal list."],
        ["Audit log", "A record of administrative changes, separate from the text of AI prompts."],
        ["Prompt log", "A record of text the browser protection saw."],
    ])
    add_user(doc, h1, h2, p, bullets, numbered, table, code)
    enable_update_fields(doc)
    path = OUT + r"\YP_RAKSHA_User_Manual.docx"
    doc.save(path)
    return path


if __name__ == "__main__":
    paths = [build_technical(), build_system(), build_user()]
    for path in paths:
        print(path)
