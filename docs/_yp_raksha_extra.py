# Extra chapters for the YP-RAKSHA manuals. Facts taken from this repository.


def add_tech(doc, h1, h2, p, bullets, numbered, table, code):
    h1(doc, "25. Inference API catalogue")
    p(doc, "These routes are registered in transports/gateway-http/handlers/inference.go. A route being registered means the gateway accepts the call. The selected provider adapter must implement that modality or the call fails at the vendor.")
    table(doc, ["Method and path", "Purpose"], [
        ["GET /v1/models", "List models visible to the caller."],
        ["POST /v1/chat/completions", "Chat completion."],
        ["POST /v1/completions", "Text completion."],
        ["POST /v1/responses", "Responses API. Also retrieve, delete, cancel, and input items."],
        ["POST /v1/embeddings", "Embeddings."],
        ["POST /v1/rerank", "Rerank."],
        ["POST /v1/ocr", "OCR."],
        ["POST /v1/audio/speech", "Speech synthesis."],
        ["POST /v1/audio/transcriptions", "Transcription."],
        ["POST /v1/images/generations", "Image generation. Edits and variations are separate routes."],
        ["POST /v1/videos", "Video generation, plus list, retrieve, content, delete, and remix."],
        ["POST /v1/batches", "Batch create, list, retrieve, cancel, and results."],
        ["POST /v1/files", "File upload, list, retrieve, delete, and content."],
        ["POST /v1/containers", "Container and container-file routes."],
        ["POST /v1/responses/input_tokens", "Token count."],
        ["POST /v1/responses/compact", "Compaction."],
    ])
    p(doc, "Callers send Authorization: Bearer <VIRTUAL_KEY>. Request bodies follow the OpenAI-compatible shapes those handlers decode. This document does not paste a full JSON schema for every modality. Read the handler when you integrate a less common route such as video or containers.")

    h1(doc, "26. Two different screening mechanisms")
    p(doc, "The repository contains two controls that both sound like guardrails. They do not run on the same traffic.")
    h2(doc, "26.1 Gateway plugin named guardrails")
    bullets(doc, [
        "Code: plugins/guardrails. Plugin name constant is guardrails.",
        "It runs inside the gateway request pipeline only when the plugin is enabled and ordered.",
        "Rules can be CEL expressions. The CEL environment exposes request.model, request.prompt_id, request.virtual_key_id, and request.virtual_key_name.",
        "The provider switch in initializeProviders enables a regex provider. Any other provider name is skipped.",
        "Input and output validation methods exist on the provider interface. They apply to gateway requests, not to random websites.",
    ])
    h2(doc, "26.2 Browser Guard rules")
    bullets(doc, [
        "Code: apps/browser-guard and /api/browser-ai.",
        "Actions in the proxy are BLOCK, REDACT, and WARN. BLOCK is preferred when several rules match.",
        "A rule may be a pattern or an AI Guard Bot. The bot is tested through POST /api/browser-ai/rules/test-bot. Regex can be drafted through POST /api/browser-ai/rules/generate-regex. Both are administrator tools, not a guarantee of detection quality.",
        "Ollama model names can be listed with GET /api/browser-ai/ollama-models when that local runtime is reachable. That does not mean Ollama is required.",
    ])
    p(doc, "A chat call to POST /v1/chat/completions is not inspected by Browser Guard. A ChatGPT page in the browser is not inspected by the gateway guardrails plugin unless that page's traffic is also sent through the gateway, which the normal website does not do.")

    h1(doc, "27. Browser AI API catalogue")
    p(doc, "Registered in transports/gateway-http/handlers/browser_ai.go. Agent calls such as heartbeat and intercept are authenticated with the guard secret, not with a user session. Admin calls use the dashboard session and RBAC.")
    table(doc, ["Area", "Routes"], [
        ["Prompt logs", "GET, DELETE /api/browser-ai/logs. Stats, bulk-delete, and delete by id."],
        ["Search logs", "GET, POST, DELETE /api/browser-ai/search-logs, plus bulk-delete and delete by id."],
        ["Rules", "GET, POST /api/browser-ai/rules. Import, update, delete, test-bot, generate-regex."],
        ["Targets", "GET, POST /api/browser-ai/targets. Import, update, delete."],
        ["Controls", "GET and PUT /api/browser-ai/controls."],
        ["PAC", "GET /api/browser-ai/pac and /api/browser-ai/proxy.pac."],
        ["Setup", "download.zip, download-windows.zip, download-mac.zip, Windows exe names Gateway_Guard_Setup.exe and Raksha_Guard_Setup.exe, rebuild, info, rebuild-history, proxy-bundle json and zip."],
        ["Agents", "List, heartbeat, wait-command, pause, resume, delete, bulk-delete, remote uninstall, uninstall key, contact email."],
        ["Fleet", "GET and PUT /api/browser-ai/fleet-config. GET /api/browser-ai/agents/settings."],
        ["Intercept", "POST /api/browser-ai/intercept and /api/browser-ai/intercept-file. GET attachment by id."],
        ["Insights and mail", "GET /api/browser-ai/insights/stats. POST /api/browser-ai/send-warning-email."],
        ["License", "GET /api/browser-ai/license, GET license/server-id, POST license/activate. Licence terms are not defined in this document."],
    ])
    p(doc, "Log delete query periods used by tests are 1d, 7d, 30d, and all. An invalid period is rejected. A bad date format is rejected.")

    h1(doc, "28. RBAC resource map")
    p(doc, "framework/rbac/enforce.go PathRequirementFor decides the resource. Admin sessions skip this map. If no requirement is returned, the route is not RBAC-gated. GET /api/config and GET /api/rbac/me/permissions are explicitly ungated for an authenticated session.")
    table(doc, ["Path prefix", "Read resource", "Write note"], [
        ["/api/logs", "Logs Read", "Deletes require Logs Delete."],
        ["/api/logs/stats and histograms", "Logs or Dashboard", "Dashboard charts."],
        ["/api/mcp-logs", "MCPLogs or Logs", "Deletes allow MCPLogs or Logs."],
        ["/api/browser-ai/logs", "Logs Read", "Deletes require Logs Delete."],
        ["/api/browser-ai other", "Logs View", "Writes use Logs Update, including rules and targets."],
        ["/api/plugins", "Plugins or Observability", "Update on write."],
        ["/api/providers, /api/keys, /api/models", "ModelProvider", "Follows the same resource."],
        ["/api/governance/virtual-keys", "VirtualKeys or Governance", ""],
        ["/api/governance/teams", "Teams or Governance", ""],
        ["/api/governance/customers", "Customers or Governance", ""],
        ["/api/governance/routing-rules", "RoutingRules or Governance", ""],
        ["/api/mcp", "MCPGateway", "Tool groups use MCPToolGroups."],
        ["/api/guardrails", "GuardrailsConfig or GuardrailsProviders", ""],
        ["/api/prompt-repo", "PromptRepository", ""],
        ["/api/skills", "SkillsRepository", "Public git serve paths are a separate unauthenticated surface."],
        ["/api/scim configuration UI", "UserProvisioning", "The IdP protocol itself is /scim/v2 with a bearer token."],
        ["/api/roles and /api/rbac", "RBAC", ""],
        ["/api/audit-logs", "AuditLogs", ""],
        ["/api/smtp-config and proxy config", "Settings", ""],
    ])
    p(doc, "Browser AI is enforced as the Logs resource. The workspace section key is browser-ai. A role can therefore see a mismatch if Logs is granted in the API catalogue but the sidebar section was not stored on the user, or the reverse.")

    h1(doc, "29. Plugin catalogue")
    p(doc, "Each directory under plugins/ is an optional pipeline component. Presence in the tree is not the same as enabled in a deployment.")
    table(doc, ["Directory", "What the code is for"], [
        ["governance", "Budgets, routing, complexity analysis, circuit breaker hooks."],
        ["guardrails", "CEL rules and a regex provider on gateway traffic."],
        ["logging", "Request and response logging."],
        ["telemetry", "Metrics export. A Prometheus compose file is included."],
        ["otel", "OpenTelemetry HTTP and gRPC export."],
        ["semanticcache", "Semantic cache lookup."],
        ["prompts", "Prompt resolution used by other plugins."],
        ["maxim", "Maxim integration plugin."],
        ["connectors", "Observability connectors such as the views for Kafka, Pub/Sub, BigQuery, Datadog, New Relic."],
        ["modelcatalogresolver", "Resolves model catalog entries."],
        ["jsonparser", "JSON parsing helper plugin."],
        ["mocker", "Mock responses for tests and demos."],
        ["compat", "Request compatibility and parameter dropping."],
    ])

    h1(doc, "30. Session and password behaviour")
    bullets(doc, [
        "Login is POST /api/session/login. Missing or wrong credentials return 401 with the text Invalid username or password.",
        "Temporary password change is POST /api/session/change-temporary-password.",
        "Forgot password is POST /api/session/forgot-password. Reset is POST /api/session/reset-password.",
        "If PASSWORD_RESET_SECRET is missing or shorter than 32 characters, the session handler logs that OTP reset will return HTTP 500. Set a long random secret before you rely on email reset.",
        "Generated temporary passwords use upper case, lower case, digits, and a symbol from a limited set, and are at least 8 characters. The login policy matches that shape.",
        "Login lockout storage exists so one account cannot be sprayed without limit. Exact thresholds belong in the live security settings, not in a guessed number.",
    ])

    h1(doc, "31. Sequence: browser prompt")
    code(doc, "Employee types in a watched site\n  -> local PAC 127.0.0.1:18195\n  -> local MITM 127.0.0.1:18103\n  -> proxy extracts text\n  -> rules: BLOCK stops, REDACT changes text, WARN notifies\n  -> POST /api/browser-ai/intercept with guard secret\n  -> prompt log row\n  -> administrator opens Browser AI, Prompt Logs")

    h1(doc, "32. Sequence: gateway chat")
    code(doc, "App sends POST /v1/chat/completions\n  Authorization: Bearer <VIRTUAL_KEY>\n  -> key status, budget, rate limit\n  -> routing or explicit model\n  -> enabled plugins, including guardrails only if configured\n  -> core/providers/<vendor>\n  -> vendor HTTPS API\n  -> usage log and budget update\n  -> response to the app")

    h1(doc, "33. Data lifecycle")
    bullets(doc, [
        "Create: administrators, SCIM, or AD sync insert governance_users. Virtual keys, rules, and targets are separate records.",
        "Use: inference writes log rows and budget counters. Browser intercept writes prompt or search logs and may store an attachment.",
        "Delete: log APIs support single delete, bulk delete, and period delete (1 day, 7 days, 30 days, or all). Deleting a log is an administrator action and is itself subject to Logs Delete.",
        "Retain: no single retention number is hard-coded for every table. If you need a legal retention period, enforce it with an operational job and the delete APIs. Do not assume the product already does this.",
        "Export: treat prompt text as confidential. Do not attach raw logs to tickets.",
    ])

    h1(doc, "34. Settings pages")
    p(doc, "The Settings section in ui/lib/constants/workspaceSections.ts contains Client Settings, Compatibility, Caching, Security, Proxy, API Keys, Performance Tuning, and Feature Flags. Paths sit under /workspace/config. API writes for SMTP, proxy, and vector store configuration require the Settings resource.")

    h1(doc, "35. Standards alignment appendix")
    p(doc, "This appendix lists themes an auditor might ask about. It is not a statement of conformity and not a certificate.")
    table(doc, ["Theme", "Evidence you can show", "What you cannot claim from this repo alone"], [
        ["Access control", "Roles, allowed_sections, virtual keys, SCIM bearer, AD bind.", "That every screen tick is enforced as an API permission."],
        ["Cryptography", "Password hashes, optional LDAPS, Guard CA for inspection.", "That TLS is enabled on the database. Compose defaults DB_SSL_MODE to disable."],
        ["Operations security", "Docker restart policy, health routes, JSON logs.", "That backups and a SIEM are already running."],
        ["Supplier relationships", "Provider adapters. Keys stay on the server.", "The vendor's own compliance."],
        ["AI policy and oversight", "Human-authored rules, logs, block redact warn, optional model judge.", "Measured accuracy, bias tests, or continuous model monitoring."],
        ["Privacy", "Prompt logs exist and can be deleted by period.", "A completed privacy impact assessment or a fixed retention schedule."],
    ])

    h1(doc, "36. Known implementation gaps")
    numbered(doc, [
        "The per-user Roles and Permissions matrix updates allowed_sections. API enforcement uses the role catalogue. Those two can disagree.",
        "Sub-admin seed rights are not full control of every selected sidebar feature.",
        "Browser AI API checks use the Logs resource. Sidebar grants use the browser-ai section. They are not the same key.",
        "The user role still receives Prompt Repository when allowed_sections is empty.",
        "Active Directory sync matches username only. The same email with a different username creates a second user. Cloud SCIM matches email and external id.",
        "AD sync does not assign allowed_sections, a virtual key, or a non-user role.",
        "Gateway guardrails ignore provider types other than regex.",
        "CEL guardrail variables do not include the raw prompt text. A CEL rule in this plugin cannot, by itself, scan prompt content. Content scan on the gateway path depends on the regex provider. Content scan on the browser path depends on Guard rules.",
        "Mac Gateway_Guard_Setup.pkg in the release folder was older than the 1.1.20 zip at review time.",
        "Customer budget can appear in the prompt key panel. Hiding it from the user role was requested and is not described here as already done.",
        "Public skill git endpoints do not require a dashboard login.",
    ])

    h1(doc, "37. Appendix: repository map for reviewers")
    table(doc, ["Question", "Start here"], [
        ["Who may call an API?", "framework/rbac/enforce.go and handlers/rbac_enforce.go"],
        ["How is a user stored?", "framework/configstore/tables/user.go"],
        ["How does login work?", "transports/gateway-http/handlers/session.go"],
        ["How does SCIM work?", "handlers/scim_handlers.go"],
        ["How does AD sync work?", "handlers/active_directory.go"],
        ["How are skills served?", "handlers/skills_serving.go"],
        ["How does the browser proxy decide?", "apps/browser-guard/proxy/gateway_proxy_parts/extract_office_backend.py"],
        ["What version is the agent?", "apps/browser-guard/agent/agent_config.py AGENT_VERSION_BAKED"],
        ["What does compose require?", "docker-compose.yml"],
        ["What pages exist?", "ui/lib/constants/workspaceSections.ts"],
    ])
    p(doc, "End of full technical documentation.")


def add_system(doc, h1, h2, p, bullets, numbered, table, code):
    h1(doc, "18. Password reset operations")
    numbered(doc, [
        "Generate a random PASSWORD_RESET_SECRET of at least 32 characters. Store it only in .env or the secret store.",
        "Restart the gateway so the process reads it.",
        "Use Forgot password on the sign-in page with a mailbox that SMTP can reach.",
        "Expected result: the user receives a code or link and can set a new password.",
        "If the secret is missing, the server log says OTP reset will return HTTP 500. Fix the secret. Do not debug this by weakening password rules.",
    ])

    h1(doc, "19. Browser AI day-to-day operations")
    h2(doc, "19.1 Targets and rules")
    numbered(doc, [
        "Add a host under Target Websites. Save.",
        "Add a Guard Rule. Choose pattern or AI Guard Bot, and an action of block, redact, or warn.",
        "Use the product test action for the bot or the regex helper if the screen offers it. Those calls are POST /api/browser-ai/rules/test-bot and generate-regex.",
        "From a managed laptop, send a harmless test string that matches the rule.",
        "Expected result: Prompt Logs shows the event and the action.",
    ])
    h2(doc, "19.2 Agents")
    numbered(doc, [
        "Open Guard Agents. A healthy laptop heartbeats.",
        "Pause stops inspection for that agent. Resume turns it back on. Use pause during a fault, not as a permanent exception.",
        "Remote uninstall asks the agent to remove itself. Keep the uninstall key in a password manager. The API can rotate it.",
        "Deleting an agent row removes the dashboard record. It is not a substitute for uninstalling the program if the laptop is still running it.",
    ])
    h2(doc, "19.3 Log cleanup")
    numbered(doc, [
        "Prompt Logs and Search Logs support clearing by period: 1 day, 7 days, 30 days, or all.",
        "The caller needs Logs Delete. A user with only View will receive 403.",
        "Expected result: older rows disappear. Take an export first if audit policy requires a copy.",
    ])
    h2(doc, "19.4 Rebuild packages")
    p(doc, "POST /api/browser-ai/setup/rebuild refreshes setup packages from the server. GET /api/browser-ai/setup/info and rebuild-history show what was produced. Employees should download from Setup, not from a copied USB of an old pkg. Windows zip and macOS zip are the update channels used by the update scripts.")

    h1(doc, "20. Verification commands")
    code(doc, "docker compose ps\ncurl -sS http://127.0.0.1:<APP_PORT>/health\ncurl -sS http://127.0.0.1:<APP_PORT>/api/health")
    p(doc, "Expected health body includes status ok when the process is healthy. If database pings are enabled, a database failure is reported in that JSON instead of a silent ok.")
    code(doc, "curl -sS http://127.0.0.1:<APP_PORT>/v1/models -H \"Authorization: Bearer <VIRTUAL_KEY>\"")
    p(doc, "Expected result: a model list, or a clear provider error. 401 means the virtual key was rejected.")

    h1(doc, "21. Feature flags and section gates")
    p(doc, "handlers/section_flag_gate.go can gate /api/browser-ai with the flag section.browser-ai. If Browser AI disappears for everyone, including an admin who should see it, check feature flags before you debug the agent.")

    h1(doc, "22. Network proxy profile")
    numbered(doc, [
        "Set PROXY_PORT and NETWORK_PROXY_CONTAINER_NAME in .env if you use this profile.",
        "Start with: docker compose --profile network-proxy up -d",
        "The container runs mitmdump with GATEWAY_FAIL_OPEN=0, GATEWAY_SERVER_MODE=1, and agent type network.",
        "Point a lab PAC at https://<backend>/api/browser-ai/pac only if you intentionally want a shared proxy.",
        "Do not enable this profile on a production laptop fleet. The compose comment says the employee path is the Guard program on each laptop.",
    ])

    h1(doc, "23. Directory operations checklist")
    table(doc, ["Step", "Cloud SCIM", "On-premises AD"], [
        ["Identity match", "external id, email, or user name", "username only"],
        ["Default role", "user", "user, unless default_role is changed in stored config"],
        ["Teams", "IdP groups become teams", "SCIM default team only, if set"],
        ["Virtual key", "not assigned", "not assigned"],
        ["Pages", "not assigned", "not assigned"],
        ["Duplicate email", "existing user is reused", "a second user can be created"],
    ])

    h1(doc, "24. Operational risks")
    bullets(doc, [
        "DB_SSL_MODE default disable sends database traffic without TLS until you change it.",
        "The gateway container is started as user 0:0 in compose. Treat the host Docker socket and the .env file as high privilege.",
        "Skill marketplace URLs are public. Review skill text before you publish.",
        "Prompt logs store employee text. Limit who has Logs Read and Browser AI pages.",
        "An AI Guard Bot sends extracted text to the configured model. That is another disclosure. Prefer regex for known secret formats when a model call is not acceptable.",
        "False negatives: a site that is not a target, a parser that misses the text box, or a rule that does not match.",
        "False positives: a regex that is too wide, or a model that flags ordinary language.",
    ])
    p(doc, "End of system manual.")


def add_user(doc, h1, h2, p, bullets, numbered, table, code):
    h1(doc, "14. Password rules")
    bullets(doc, [
        "Use at least 8 characters.",
        "Include an upper-case letter, a lower-case letter, a number, and a symbol.",
        "The first password you receive by email is temporary. The site will ask you to replace it.",
        "If Forgot password does nothing or shows a server error, ask an administrator. Email reset must be switched on. Do not keep guessing.",
    ])

    h1(doc, "15. What each main area is for")
    table(doc, ["Area", "Use it when"], [
        ["Dashboard and LLM Logs", "You need to see gateway AI usage, and you were given Observability."],
        ["Browser AI", "You manage watched websites, rules, logs, or the computer program."],
        ["Models", "You connect a model vendor, prices, budgets, or routing."],
        ["MCP Gateway", "You connect tools that an AI assistant may call."],
        ["Plugins", "You turn optional gateway behaviours on or off."],
        ["Governance", "You manage people, teams, keys, customers, and permissions."],
        ["Guardrails", "You set rules on traffic that applications send through the gateway. This is separate from Browser AI rules."],
        ["Adaptive Routing", "You let the gateway choose among models."],
        ["Prompt Repository", "You write and send prompts with a virtual key."],
        ["Skills Repository", "You publish instruction packs for coding assistants."],
        ["Settings", "You change client, cache, security, proxy, and feature flags."],
    ])
    p(doc, "Screenshot to add: one image per area you actually use, with names and prompt text covered.")

    h1(doc, "16. Skills Repository, for a person installing a coding assistant")
    p(doc, "An administrator copies a command from the Skills page. The exact address is your company gateway. These are the shapes of the commands, not a promise that your assistant is already connected.")
    numbered(doc, [
        "Claude Code uses a marketplace web address under /api/skills/serve/claude-code.",
        "Codex uses a marketplace add command whose address ends in /api/skills/serve/codex.",
        "Cursor uses git clone of the address ending in /api/skills/serve/cursor, then the skill folders are copied into the Cursor skills directory.",
        "Antigravity uses a plugin install address ending in /api/skills/serve/antigravity/plugins/gateway-all-skills.",
    ])
    p(doc, "If the command fails, you may be off the company network, or the skill was not published. Ask the administrator. Do not paste a personal access token into a skill file.")

    h1(doc, "17. If the computer program warns you")
    numbered(doc, [
        "Read the message. Note whether it says the text was blocked, changed, or only warned.",
        "Do not retype the same sensitive text on a different website to get around it.",
        "If you believe the warning is wrong, contact the administrator and describe the task. Do not forward the original text if it contains secrets or customer data.",
        "Expected result: either you change the message so it is allowed, or the administrator adjusts the rule after review.",
    ])
    p(doc, "Screenshot to add: a warning from a test message such as a fake sample, never a real customer record.")

    h1(doc, "18. Errors you may see")
    table(doc, ["What you see", "What to do"], [
        ["Invalid username or password", "Check the email and password. Use reset if it is offered. After several failures the account may pause login attempts."],
        ["You must change your password", "Set a new password that meets the rules in section 14."],
        ["Insufficient permissions or 403", "That page or button is not allowed for your role. Ask an administrator. Do not borrow another login."],
        ["Cannot send in Prompt Repository", "Ask for a virtual key, or ask whether the budget is used up."],
        ["No reply from a model", "The vendor or the model name may be unavailable. Try later or ask the person who manages Models."],
        ["Guard does not seem to run", "Confirm the program is open and you are on a website your company listed. Other sites are not watched."],
        ["A page is missing from the sidebar", "It was not granted. Prompt Repository is the page a basic user still has."],
    ])

    h1(doc, "19. More questions")
    table(doc, ["Question", "Answer"], [
        ["Are gateway rules and browser rules the same?", "No. Gateway rules apply to applications that call the company API. Browser rules apply to watched websites on a computer running Gateway Guard."],
        ["Does a warning mean the company model read my text?", "A pattern rule does not need a model. An AI Guard Bot rule does send the extracted text to the model your administrator chose."],
        ["Can I turn Guard off?", "No. Pausing an agent is an administrator action for a fault, not a personal setting."],
        ["Who can delete logs?", "Only people whose role includes delete on logs. Deleting logs does not delete the copy an outside AI vendor may already have stored."],
        ["I was added from Microsoft AD. What do I have?", "A user login and Prompt Repository. Extra pages and a virtual key are added by an administrator afterwards."],
        ["Is every hazard caught?", "No. The tool catches what the rules match. People remain responsible for what they type."],
    ])
    p(doc, "End of user manual. Add the screenshots marked above before you send this document to employees.")
