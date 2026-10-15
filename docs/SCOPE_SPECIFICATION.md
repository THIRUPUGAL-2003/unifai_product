# UnifAI scope specification

Source for test cases and the user manual. One requirement, one test case, one manual section.

Test case ID = TC- plus the requirement ID. Manual section = the same ID.

## 1. Access and accounts

### AUTH-01 — A person signs in with username and password.
- Actor: Any role
- Before you start: Account status is approved.
- Inputs: Username anitha.user. Password meeting the password rule.
- Expected result: Session cookie is set. The user lands on the first allowed page. Session lasts about 24 hours.
- Test case: TC-AUTH-01

### AUTH-02 — Sign-up always creates a user role and waits for approval.
- Actor: New employee
- Before you start: SMTP test has succeeded if email codes are on.
- Inputs: Username, email, password, confirm password.
- Expected result: Status becomes pending, or email_unverified until the 6-digit code is entered. Sign-in is refused until an admin approves.
- Test case: TC-AUTH-02

### AUTH-03 — Password must meet the rule.
- Actor: Any person setting a password
- Before you start: Sign-up, reset, or force-change screen.
- Inputs: Shorter than 8, or missing upper, lower, number, or symbol.
- Expected result: The form rejects the password and names the missing rule. No account change is saved.
- Test case: TC-AUTH-03

### AUTH-04 — Forgot password sends a code and then sets a new password.
- Actor: Approved user
- Before you start: SMTP test succeeded.
- Inputs: Email anitha.user@example.com. Code within 10 minutes. New password.
- Expected result: Old password stops working. New password signs in.
- Test case: TC-AUTH-04

### AUTH-05 — Forgot username emails the username.
- Actor: Approved user
- Before you start: SMTP test succeeded.
- Inputs: Email on the account.
- Expected result: The email contains the username. It does not contain the password.
- Test case: TC-AUTH-05

### AUTH-06 — Too many failed sign-ins lock the attempt.
- Actor: Any person
- Before you start: Lockout is on in Security.
- Inputs: Repeated wrong passwords.
- Expected result: The screen states how many minutes remain. A correct password during the lock does not sign in.
- Test case: TC-AUTH-06

### AUTH-07 — Sign out ends the website session only.
- Actor: Signed-in user
- Before you start: A session exists.
- Inputs: Account menu, Sign out.
- Expected result: The login page opens. Opening a workspace URL asks for sign-in. Gateway Guard and YP-Raksha stay on.
- Test case: TC-AUTH-07

### AUTH-08 — Pending, rejected, unverified, and disabled accounts cannot sign in.
- Actor: Those users
- Before you start: User row is in that status.
- Inputs: Correct password.
- Expected result: Sign-in shows the matching reason. No workspace menu is shown.
- Test case: TC-AUTH-08

### AUTH-09 — A menu the account was not granted does not open.
- Actor: user or custom role
- Before you start: Allowed sections omit that menu.
- Inputs: Direct URL of a forbidden page.
- Expected result: The site opens the first allowed page. It does not show the forbidden data.
- Test case: TC-AUTH-09

### AUTH-10 — Admin, sub_admin, user, custom role, and SCIM user share one Sign In page.
- Actor: Each actor
- Before you start: Accounts exist.
- Inputs: Each sample account.
- Expected result: admin sees provider keys and Settings. sub_admin does not see master provider keys. user sees only granted sections. An inactive SCIM user is disabled.
- Test case: TC-AUTH-10

## 2. Governance

### GOV-01 — An admin approves or rejects a pending sign-up.
- Actor: admin or sub_admin
- Before you start: User status is pending.
- Inputs: Approve, or Reject.
- Expected result: Approved can sign in. Rejected cannot. The email can be used to sign up again after reject.
- Test case: TC-GOV-01

### GOV-02 — An admin creates a user directly.
- Actor: admin or sub_admin
- Before you start: Signed in.
- Inputs: Username, email, password, role, allowed sections, optional budget and rate.
- Expected result: The user can sign in without the public queue. The menu matches allowed sections.
- Test case: TC-GOV-02

### GOV-03 — A team holds members and one budget.
- Actor: admin or sub_admin
- Before you start: Users exist.
- Inputs: Name Support. Member anitha.user. Budget 200. Rate 120 per minute.
- Expected result: The team can be selected on a virtual key and on a routing rule.
- Test case: TC-GOV-03

### GOV-04 — A business unit budget stops every team under it.
- Actor: admin or sub_admin
- Before you start: Team exists.
- Inputs: Unit Customer Operations. Team Support. Budget 2000.
- Expected result: A call stops when the unit ceiling is hit even if the team budget remains. LLM Logs shows the limit error.
- Test case: TC-GOV-04

### GOV-05 — A customer groups teams for reporting.
- Actor: admin or sub_admin
- Before you start: Team exists.
- Inputs: Name Northwind. Team Support.
- Expected result: The customer sheet shows the linked team. Calls still use a virtual key.
- Test case: TC-GOV-05

### GOV-06 — A virtual key is the application secret.
- Actor: admin or sub_admin
- Before you start: Team and an enabled model exist.
- Inputs: Name, team, budget 200, rate 120, model gpt-4.1-mini.
- Expected result: The key is shown once. Bearer calls to /v1/chat/completions succeed until budget, rate, or revoke.
- Test case: TC-GOV-06

### GOV-07 — A virtual key over budget or rate is refused.
- Actor: Application
- Before you start: Key exists with a low ceiling.
- Inputs: Calls until the ceiling.
- Expected result: The call returns an error. LLM Logs records it. Alert Channels can notify once per 10 minutes.
- Test case: TC-GOV-07

### GOV-08 — Rotating a virtual key invalidates the old value.
- Actor: admin or sub_admin
- Before you start: A key is in use.
- Inputs: Rotate.
- Expected result: Old Bearer value fails. New value succeeds.
- Test case: TC-GOV-08

### GOV-09 — A custom role limits API operations.
- Actor: admin
- Before you start: Signed in.
- Inputs: Role Auditor. Logs and Audit Logs read and view only.
- Expected result: That user can read logs and cannot delete users, change provider keys, or open Settings.
- Test case: TC-GOV-09

### GOV-10 — An access profile applies one budget and model set to many users.
- Actor: admin or sub_admin
- Before you start: Users exist.
- Inputs: Profile Contractor. Budget 25. Model gpt-4.1-mini. Apply.
- Expected result: Those users’ keys follow the profile.
- Test case: TC-GOV-10

### GOV-11 — SCIM creates and disables users from the identity provider.
- Actor: Identity admin
- Before you start: SCIM URL and token copied from User Provisioning.
- Inputs: Provision a user. Then set the person inactive.
- Expected result: A Users row appears. Inactive becomes status disabled and sign-in fails.
- Test case: TC-GOV-11

### GOV-12 — Audit Logs record configuration changes.
- Actor: admin or sub_admin
- Before you start: A change was just saved.
- Inputs: Filter user priya.admin, today.
- Expected result: The create or update appears. The row is not typed by hand.
- Test case: TC-GOV-12

## 3. Models, routing, and guardrails

### MOD-01 — A model provider stores the vendor key.
- Actor: admin
- Before you start: A vendor account exists.
- Inputs: Provider OpenAI. Key name openai-prod-1. Vendor API key.
- Expected result: Model Catalog lists models from that vendor. The virtual key never contains this secret.
- Test case: TC-MOD-01

### MOD-02 — Only enabled catalog models can be chosen.
- Actor: admin
- Before you start: Provider key works.
- Inputs: Enable gpt-4.1-mini. Disable another model.
- Expected result: Routing and virtual keys can use the enabled model. A call to a disabled model is refused.
- Test case: TC-MOD-02

### MOD-03 — A model-level budget is separate from the virtual-key budget.
- Actor: admin
- Before you start: Model is enabled.
- Inputs: Budget 500. Rate 60 per minute on gpt-4.1-mini.
- Expected result: The first ceiling reached stops the call.
- Test case: TC-MOD-03

### MOD-04 — A pricing override replaces the default price in reports.
- Actor: admin
- Before you start: Model exists.
- Inputs: Input and output contract prices.
- Expected result: Dashboard and LLM Logs use the override. Recalculate cost after a change if history must match.
- Test case: TC-MOD-04

### MOD-05 — Model settings supply default parameters.
- Actor: admin
- Before you start: Model exists.
- Inputs: Temperature 0.2.
- Expected result: A call that omits temperature uses 0.2. A client value still wins.
- Test case: TC-MOD-05

### MOD-06 — A routing rule forces a model for a team.
- Actor: admin
- Before you start: Team Support and the model exist.
- Inputs: If team is Support, model is gpt-4.1-mini.
- Expected result: A Support virtual-key call shows that model in LLM Logs.
- Test case: TC-MOD-06

### MOD-07 — Complexity router sends easy prompts to a smaller model.
- Actor: admin
- Before you start: Both models enabled. Flag routing.complexity on.
- Inputs: Low model gpt-4.1-mini. High model gpt-4.1.
- Expected result: A short hello is logged on the low model.
- Test case: TC-MOD-07

### MOD-08 — Circuit breaker pauses a failing provider.
- Actor: admin
- Before you start: Two provider keys. Flag routing.circuit_breaker on.
- Inputs: Open after 5 failures. Retry after 30 seconds.
- Expected result: Further calls skip the failing key while the breaker is open.
- Test case: TC-MOD-08

### MOD-09 — Adaptive routing splits weight across healthy keys.
- Actor: admin
- Before you start: Two keys for the same provider.
- Inputs: Weights 70 and 30.
- Expected result: The adaptive dashboard shows the split. A routing rule that names one provider still wins.
- Test case: TC-MOD-09

### MOD-10 — Guardrail rules inspect API text, not browser text.
- Actor: admin
- Before you start: Rule provider saved if the rule needs one.
- Inputs: PII rule set to redact or block.
- Expected result: A virtual-key call containing a card number is blocked or redacted. Browser AI Prompt Logs do not gain that row. LLM Logs do.
- Test case: TC-MOD-10

### MOD-11 — Cluster config is unused on a single server.
- Actor: admin
- Before you start: One UnifAI server.
- Inputs: No peers.
- Expected result: The server runs. A second node is added only with the same cluster secret.
- Test case: TC-MOD-11

## 4. Observability and settings

### OBS-01 — The observability dashboard totals API usage.
- Actor: admin or granted user
- Before you start: At least one successful virtual-key call.
- Inputs: Date range last 7 days.
- Expected result: Requests, tokens, cost, and latency are shown. Cost follows pricing.
- Test case: TC-OBS-01

### OBS-02 — LLM Logs show one row per API call.
- Actor: admin or granted user
- Before you start: A call was made.
- Inputs: Filter by key Support bot — production.
- Expected result: The row has model, tokens, cost, latency, and status. A provider 401 is visible as an error.
- Test case: TC-OBS-02

### OBS-03 — Logs settings control stored prompt text and retention.
- Actor: admin
- Before you start: Signed in to Settings.
- Inputs: Content logging on or off. Retention 30 days.
- Expected result: New LLM rows follow the choice. Browser Prompt Logs are not changed by this page.
- Test case: TC-OBS-03

### OBS-04 — A connector copies logs to an external system.
- Actor: admin
- Before you start: LLM Logs has rows.
- Inputs: One of Datadog, BigQuery, Kafka, Pub/Sub, New Relic, Prometheus, OpenTelemetry, with that vendor’s URL and key.
- Expected result: A new call appears in LLM Logs and in the external system.
- Test case: TC-OBS-04

### OBS-05 — An alert channel sends a short incident.
- Actor: admin
- Before you start: A budget or circuit breaker can fire. SMTP tested if type is email.
- Inputs: Name Ops Slack. Type Slack. Webhook URL. Enabled.
- Expected result: Test message arrives. A real incident is sent at most once every 10 minutes.
- Test case: TC-OBS-05

### SET-01 — SMTP must accept a test before any product email is expected.
- Actor: admin
- Before you start: Mail server credentials.
- Inputs: Host smtp.example.com. Port 587. TLS on. From noreply@example.com.
- Expected result: Test to priya.admin@example.com arrives. Sign-up and reset codes fail closed until this passes.
- Test case: TC-SET-01

### SET-02 — Client settings require auth and limit origins.
- Actor: admin
- Before you start: Signed in.
- Inputs: Origin https://unifai.example.com. Auth required on.
- Expected result: A call without a virtual key is refused. A browser origin not on the list is refused.
- Test case: TC-SET-02

### SET-03 — The office proxy is optional and is not the laptop Guard.
- Actor: admin
- Before you start: Server cannot reach the vendor directly, or it can.
- Inputs: Proxy off, or URL http://proxy.example.com:8080 and inference on.
- Expected result: With proxy off, vendor calls go direct. With proxy on, a test chat still succeeds and appears in LLM Logs.
- Test case: TC-SET-03

### SET-04 — Caching returns a stored answer only when the flag is on.
- Actor: admin
- Before you start: cache.responses on. Direct cache on. TTL 10 minutes.
- Inputs: The same request twice.
- Expected result: The second call does not need the provider. LLM Logs shows the cache. Flag off sends every call to the provider.
- Test case: TC-SET-04

### SET-05 — Feature flags gate the feature they name.
- Actor: admin
- Before you start: Flags saved.
- Inputs: browser.prompt_guard off, then on.
- Expected result: Off: laptop prompts are not stored in Prompt Logs. On: a matching prompt is stored.
- Test case: TC-SET-05

### SET-06 — Compatibility converts client parameters for a non-matching provider.
- Actor: admin
- Before you start: An OpenAI-style client calls another vendor through the gateway.
- Inputs: Conversion on.
- Expected result: The call is accepted in the client’s shape and answered by the routed provider.
- Test case: TC-SET-06

## 5. Browser AI and the laptop

### BAI-01 — A target website is the only domain the laptop inspects.
- Actor: admin
- Before you start: Signed in to Browser AI.
- Inputs: Domain chatgpt.com. Platform ChatGPT. Host role ui. Monitored yes. Block entire website no.
- Expected result: A prompt on that domain can hit a rule. A domain not on the list is not inspected by a rule.
- Test case: TC-BAI-01

### BAI-02 — BLOCK stops the send and shows only the warning text.
- Actor: Employee on a monitored site
- Before you start: Rule India PAN active. Pattern for a PAN. Laptop agent active.
- Inputs: Prompt: My PAN is ABCDE1234F.
- Expected result: The message is not sent. The warning text is shown. The rule name is not shown. Prompt Logs action is Blocked.
- Test case: TC-BAI-02

### BAI-03 — WARN shows the warning and sends the original text.
- Actor: Employee
- Before you start: WARN rule for the text live warn token.
- Inputs: Prompt contains live warn token.
- Expected result: Warning is shown. Original text is sent. Prompt Logs action is Warned.
- Test case: TC-BAI-03

### BAI-04 — REDACT shows the warning and sends [REDACTED] for the match.
- Actor: Employee
- Before you start: REDACT rule for live redact token.
- Inputs: Prompt contains live redact token.
- Expected result: Warning is shown. Sent text contains [REDACTED]. Prompt Logs action is Redacted.
- Test case: TC-BAI-04

### BAI-05 — An allowed prompt is sent and logged.
- Actor: Employee
- Before you start: No rule matches.
- Inputs: Draft a polite refund reply.
- Expected result: The site receives the text. Prompt Logs action is Allowed.
- Test case: TC-BAI-05

### BAI-06 — Block entire website stops the site.
- Actor: admin then employee
- Before you start: Target row with block entire website yes.
- Inputs: Open that site.
- Expected result: The site is blocked. Prompt Logs can show SiteBlocked.
- Test case: TC-BAI-06

### BAI-07 — Block all uploads stops every attachment on monitored sites.
- Actor: admin then employee
- Before you start: File upload policy on. browser.file_scan on.
- Inputs: Upload any small file in a monitored chat.
- Expected result: The upload stops. The upload warning is shown.
- Test case: TC-BAI-07

### BAI-08 — Search logs record queries, including private windows.
- Actor: Employee
- Before you start: Agent active.
- Inputs: Search on a monitored engine, including a private window.
- Expected result: Search Logs shows engine, browser, query, and the private-window flag.
- Test case: TC-BAI-08

### BAI-09 — A laptop appears under Guard Agents after setup.
- Actor: IT
- Before you start: Package from Browser AI → Setup.
- Inputs: Run Gateway_Guard_Setup.exe. Start at login checked. Approve the admin prompt.
- Expected result: The hostname appears as endpoint, status active, with a recent last-seen time.
- Test case: TC-BAI-09

### BAI-10 — YP-Raksha is on, managed, and has no Remove button.
- Actor: Employee browser
- Before you start: BAI-09 succeeded and the outside path install is absent.
- Inputs: Open the browser extensions page.
- Expected result: Extension id mipfhcmlcjfhmgldkokdckhndkebkllj is on, including private windows. There is no Remove control and no unknown-source warning.
- Test case: TC-BAI-10

### BAI-11 — An outside path install is a failed install.
- Actor: Employee browser
- Before you start: Registry path install exists.
- Inputs: Open extensions.
- Expected result: The extension can show Off, Remove, and not from a known source. This does not meet BAI-10.
- Test case: TC-BAI-11

### BAI-12 — Uninstall requires the company key or a remote command.
- Actor: IT
- Before you start: Agent active. Key stored by IT.
- Inputs: Uninstall from Apps and enter the key, or remote uninstall.
- Expected result: Status moves to uninstall_pending, then uninstalled. The extension policy is removed with the Guard.
- Test case: TC-BAI-12

### BAI-13 — A warning email uses the agent contact and SMTP.
- Actor: admin
- Before you start: Prompt log row. Contact email on the agent. SMTP tested.
- Inputs: Send warning from the log.
- Expected result: The contact receives the mail. The log row remains.
- Test case: TC-BAI-13

### BAI-14 — Guard Insights totals actions per agent.
- Actor: admin
- Before you start: Prompt logs exist.
- Inputs: Open Guard Insights.
- Expected result: Blocked, warned, redacted, and allowed totals match the logs for that hostname.
- Test case: TC-BAI-14

### BAI-15 — AI bot rules are created on screen, not by spreadsheet.
- Actor: admin
- Before you start: An LLM is available for the bot.
- Inputs: Create an ai_bot rule and test it before Active.
- Expected result: Import of an ai_bot row is rejected. A tested regex import still works.
- Test case: TC-BAI-15

## 6. MCP, prompts, and skills

### MCP-01 — MCP Catalog connects a server.
- Actor: admin
- Before you start: Server URL is reachable.
- Inputs: Name docs-server. URL of the server.
- Expected result: Status becomes connected. MCP Library alone does not connect.
- Test case: TC-MCP-01

### MCP-02 — MCP Auth Config stores the header after the catalog client exists.
- Actor: admin
- Before you start: Catalog client exists.
- Inputs: Header Authorization and the token. Optional per-user header keys.
- Expected result: A later tool call uses that header. Auth Sessions can show success.
- Test case: TC-MCP-02

### MCP-03 — A tool group limits which tools a key may call.
- Actor: admin
- Before you start: Catalog connected.
- Inputs: Group Support tools. Tool search_docs only. Attach the Support key.
- Expected result: That key can call search_docs. It cannot call a tool left unticked. The call appears in MCP Logs when mcp.tool_execution is on.
- Test case: TC-MCP-03

### MCP-04 — An OAuth grant can be revoked.
- Actor: admin
- Before you start: A grant exists.
- Inputs: Revoke anitha.user on docs-server.
- Expected result: That user no longer has consent. New tool calls that need the grant fail.
- Test case: TC-MCP-04

### PR-01 — A prompt is saved in a folder.
- Actor: Granted user
- Before you start: Prompt Repository is allowed.
- Inputs: Folder Support. Name. Text. Tags.
- Expected result: The prompt is listed for people who may see that folder.
- Test case: TC-PR-01

### PR-02 — A deployment publishes one version to an environment.
- Actor: admin
- Before you start: A prompt version exists.
- Inputs: Environment production. Version 1. Enabled.
- Expected result: A call with header x-uf-prompt-environment: production and a virtual key uses that version.
- Test case: TC-PR-02

### PR-03 — The playground sends a trial message through the gateway.
- Actor: Signed-in user with model access
- Before you start: A model is enabled.
- Inputs: Model gpt-4.1-mini. Message Hello.
- Expected result: An answer returns. LLM Logs has the row. The laptop Guard is not involved.
- Test case: TC-PR-03

### PR-04 — A skill is stored text for assistants.
- Actor: Granted user
- Before you start: Skills Repository is allowed.
- Inputs: Name and body.
- Expected result: Clients that call the skills API can read it. It does not route a model and it is not a guard rule.
- Test case: TC-PR-04
