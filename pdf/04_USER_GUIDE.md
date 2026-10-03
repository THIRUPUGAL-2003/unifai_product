# UnifAI — User Guide

**Audience:** employees and end users (including developers who were issued an API key)
**Baseline:** repo commit `16d4c3e`

---

## 1. What UnifAI is, in plain language

Your company runs two things on one platform:

1. **UnifAI Workspace (the dashboard)** — a web app where you sign in, keep prompts, and (if you have
   access) see logs, models, virtual keys and other company AI tooling.
2. **UnifAI Guard ("Browser AI")** — a small app installed on your work laptop that keeps company
   data safe while you use public AI websites (ChatGPT, Claude, Gemini, Copilot, DeepSeek, Grok,
   Perplexity, Poe, Mistral Le Chat, and others your IT team monitors).

In practice:
* You keep using AI websites normally in your browser.
* The Guard runs in the background on your laptop and checks prompts and file uploads against the
  company's rules before they leave your machine.
* Anything the rules flag is **blocked**, **redacted**, or **warned** — and the event is recorded for
  your company's security team.

The Guard is a **company security control**, not a personal app. Please do not try to disable or
bypass it — IT can see whether it is running.

---

## 2. Your account

### 2.1 Signing up
1. Open the UnifAI URL your IT team gave you (for example `https://unifai.yourcompany.com`).
2. Choose **Sign Up**.
3. Enter your details. Depending on company settings, you may receive a **code by e-mail** —
   enter it to verify your address (`email_unverified` status).
4. Your registration then goes to an administrator for approval (`pending`). You cannot sign in
   until it is approved.
5. Once approved you can log in. If it is rejected or your account is disabled, login shows the
   reason sent by the platform.

### 2.2 Signing in
* Go to the UnifAI URL and log in with your username and password.
* Your session lasts about **24 hours** by default and then you log in again.
* The session cookie is HTTP-only and `Secure` when your company serves the site over HTTPS.

### 2.3 Forgot password / username
1. On the login screen choose **Forgot password** (or **Forgot username**).
2. A one-time code is e-mailed to you (the code is valid for 10 minutes; there is a short cooldown
   between requests and a per-hour limit).
3. Enter the code, then set a new password.
4. Too many wrong codes invalidates the request — start again.

### 2.4 Safety limits you may hit
* Too many failed logins from your network or for your username triggers a temporary lockout with a
  message telling you how many minutes remain. Wait it out.
* If your account is `disabled` by IT (for example, it was deprovisioned), contact your IT helpdesk.

---

## 3. Using the workspace

After login you land in the workspace. Which menu items you see depends on the permissions your
administrator gave you. By default the `user` role has read-only access to the **Dashboard**,
**LLM/MCP Logs**, **Observability**, **Prompt Repository** and **MCP Gateway**, and your administrator
may narrow this down to specific sidebar sections (for example Prompt Repository only).

Common pages you may be granted:

| Page | What you can do |
|---|---|
| **Prompt Repository** | Save, organise and reuse prompts |
| **Skills Repository** | Browse and use shared skills |
| **Dashboard / LLM Logs / MCP Logs** | Read-only reporting about AI usage (if granted) |
| **Browser AI** | Team-wide view of Guard targets, rules, prompt logs, agents, insights and setup |
| **Virtual Keys / Governance** | Manage API credentials, budgets and limits (if granted) |
| **Model Catalog / Providers** | See which models are available (read-only) |
| **Playground** | Chat with a model through the company gateway |
| **Docs** | Links to the full product documentation |

Notes:
* If you open a page you were not granted, you are redirected to your first allowed page — you are
  not shown an error.
* Menu items disappear when you lack permission for that area (RBAC), so the menu is always a
  reflection of what you can actually do.

### 3.1 If you have a Virtual Key (API access)
Developers are often issued a **Virtual Key** instead of using the dashboard for AI calls.
* Point your AI client at the company gateway base URL and use the key as the bearer credential:
  ```bash
  curl https://unifai.yourcompany.com/v1/chat/completions \
    -H "Authorization: Bearer <YOUR_VIRTUAL_KEY>" \
    -H "Content-Type: application/json" \
    -d '{"model":"<model-id>","messages":[{"role":"user","content":"Hello"}]}'
  ```
* Existing SDKs can often keep their code and only change the base URL, because the gateway also
  exposes provider-shaped endpoints such as `/openai/v1/...` and `/anthropic/v1/...`.
* Your key may have a **budget** (cost limit) and a **rate limit** (requests per minute). If you hit
  them you get an error — ask your administrator to adjust the limits.
* Keep your key secret. Do not commit it to code repositories. Report a leaked key immediately so it
  can be rotated.

---

## 4. UnifAI Guard on your laptop

### 4.1 What it does
* Runs a **local security proxy** on your machine only (`127.0.0.1:18103`) and applies a **PAC** so
  your browser routes **only the monitored AI websites** through it.
* Checks the text you send and the files you upload against the company's **Guard Rules** before the
  request reaches the AI site.
* Applies the configured action: **allow**, **redact** (sensitive parts removed), **warn**
  (a message is shown) or **block** (request stopped).
* Reports the event to the company platform so IT can see what happened.
* Also records **search-engine queries** for monitored browsers (including **incognito/private**
  windows) and whether a search result was opened.
* Needs no access to the company database and talks only to your company's UnifAI server.

### 4.2 Installing (Windows)
1. Get `UnifAI_Guard_Windows.zip` from your IT team (or Browser AI → **Setup** if you have access).
2. **Extract** the ZIP completely (right-click → *Extract All*). Do not run Setup from inside the ZIP.
3. In the extracted folder run **`UnifAI_Guard_Setup.exe`**.
4. Keep **“Start automatically at Windows login”** checked.
5. Finish — the Guard starts in the background with no terminal window.
6. Open ChatGPT or another monitored AI site as usual.

### 4.3 Installing (macOS)
1. Get `UnifAI_Guard_macOS.zip` from IT (or Browser AI → Setup).
2. Extract it, then double-click **`Install_UnifAI_Guard.command`**.
3. Approve the macOS prompts (the Guard installs a trusted certificate so it can inspect HTTPS for
   the monitored sites only).
4. The Guard starts automatically and keeps running in the background.

### 4.4 Turning the Guard off / uninstalling
This is **gate-controlled** and needs the company **uninstall key** from IT.

* **Windows:** Settings → Apps → **UnifAI Guard** → Uninstall (or Start Menu → UnifAI Guard →
  Uninstall). Enter the key when prompted.
* **macOS:** double-click **`Uninstall_UnifAI_Guard.command`** and enter the key.

If your company disabled the key requirement, leaving it blank is allowed — IT will tell you.

IT can also remove the Guard **remotely**; on the server side this shows as
“uninstall pending / uninstalled”.

### 4.5 Where the Guard keeps its files (useful when IT asks)
| Platform | Location |
|---|---|
| Windows data / logs | `%LOCALAPPDATA%\UnifAI\Guard\` |
| Windows log file | `%LOCALAPPDATA%\UnifAI\Guard\unifai_guard.log` |
| Windows certificate status | `%LOCALAPPDATA%\UnifAI\Guard\ca_install_status.txt` |
| macOS data | `~/Library/Application Support/UnifAI/Guard/` |

Keep these files if IT asks for them — they show fastest whether the Guard is connected and whether
HTTPS inspection is trusted.

---

## 5. What you will actually see

| Situation | What happens |
|---|---|
| You send a normal prompt on a monitored site | Nothing — it is logged as **Allowed** |
| Your prompt contains something a rule flags with action **BLOCK** | The request stops and you see the rule's warning message |
| Action is **REDACT** | The sensitive part is removed and the request continues |
| Action is **WARN** | You see a warning; the request continues |
| You visit an AI site marked **Block Entire Website** | The whole site is blocked |
| You upload a file that a policy blocks | The upload stops with the configured message |
| An AI Guard Bot rule handles the prompt | You may see a bot answer instead of the AI site's reply |
| You search on Google / Bing / DuckDuckGo / etc. | The query and any opened result can be recorded, **including in incognito/private mode** |
| The company server is unreachable | Depending on configuration traffic may fail open (allowed) — IT controls this |

> Data to avoid putting into AI chats: personal IDs (Aadhaar, PAN), card numbers, salaries/HR
> information, API keys and secrets, private keys, credentials, and unreleased project names.
> These are typical rule examples and are usually **blocked or redacted**.

---

## 6. Troubleshooting (self-service)

| Problem | Try this |
|---|---|
| “Your connection is not private” / certificate warning on an AI site | Tell IT — the Guard's certificate is not trusted on your machine yet. Report `%LOCALAPPDATA%\UnifAI\Guard\ca_install_status.txt` (Windows) |
| The AI site will not load at all | Check whether the site is intentionally blocked by policy; otherwise contact IT |
| My prompt disappeared / was refused | A Guard rule blocked or redacted it; the warning message names the rule |
| The Guard seems not to be running | Restart the laptop, then check the log file; ask IT to confirm your device appears in Guard Agents |
| Login says “waiting for admin approval” | Your registration is pending — an administrator must approve it |
| Login says “verify your email first” | Enter the code from the Sign Up page |
| Login says “Too many failed attempts” | Wait the stated number of minutes before trying again |
| Password reset e-mail does not arrive | Check spam; the request may be rate-limited — wait a couple of minutes and retry. IT must have SMTP configured |
| I opened a page and got bounced elsewhere | That page is not in your permissions; you were redirected to your first allowed page |
| An AI client returns `401`/`403` with my Virtual Key | The key may be revoked/expired, or you exceeded budget/rate limits — ask your administrator |
| Streaming answers hiccup or arrive at once | Report it to IT (the reverse proxy must not buffer streaming) |

**Before contacting IT, note:** what you were doing, the site, the exact message, the time
(with timezone), and your device name. That lets them find the matching Prompt Log entry quickly.

---

## 7. Privacy and expectations (straight answers)

| Question | Answer |
|---|---|
| Can IT see the prompts I send to AI sites? | Yes, for **monitored** websites. Prompt text, site, domain, device and the verdict are stored so the security team can investigate incidents |
| Can IT see my personal browsing? | The Guard proxies only the AI websites configured as targets, so unrelated browsing is not inspected. Search-engine queries and clicked results are recorded for monitored browsers |
| Does incognito/private mode hide my activity? | No — for monitored browsers the Guard records the query and marks it as incognito/private |
| Are uploaded files inspected? | Yes. File type, name and size are recorded, and file bytes are stored on the server when policy requires review |
| Does the Guard send my data anywhere else? | No — only to your company's UnifAI server. Your requests still go to the AI website you are using |
| Can I turn the Guard off? | Only with the company uninstall key, or if IT removes it remotely. It is a company control on a company device |
| How long is data kept? | Company policy — up to the configured retention (default 365 days for logs) |
| Can I ask for my data to be deleted? | Contact your IT/security team; only administrators can delete logs |

---

## 8. Quick-reference card (share with colleagues)

```
Sign in            →  https://<unifai-url>   (ask IT for the key/URL)
Forgot password    →  Login page → Forgot password → e-mail code (valid 10 min)
Guard install (Win)→  Extract UnifAI_Guard_Windows.zip → UnifAI_Guard_Setup.exe
Guard install (Mac)→  Extract UnifAI_Guard_macOS.zip → Install_UnifAI_Guard.command
Guard off          →  Needs the company uninstall key from IT
Guard log (Win)    →  %LOCALAPPDATA%\UnifAI\Guard\unifai_guard.log
Cert status (Win)  →  %LOCALAPPDATA%\UnifAI\Guard\ca_install_status.txt
Report a problem   →  Site + message + time + device name
Never put in AI    →  IDs, card numbers, salary/HR data, API keys, credentials, secret projects
```