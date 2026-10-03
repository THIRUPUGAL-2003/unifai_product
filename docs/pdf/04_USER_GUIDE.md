# UnifAI / Raksha Enterprise End-User & Developer Guide
# Comprehensive User Handbook: Portal, API Integration & Browser Guard

**Document Version:** 2.4.0  
**Classification:** Enterprise End-User Documentation  
**Target Audience:** Employees, Software Developers, AI Engineers, Data Scientists, Knowledge Workers  

---

## 1. Welcome to UnifAI & Raksha Enterprise

Your organization provides UnifAI to enable secure, productive, and compliant use of modern Generative Artificial Intelligence. The platform consists of two primary touchpoints:

1. **UnifAI Workspace (Web Portal):** An intuitive web application where you can explore approved AI models, test prompts in the playground, maintain your personal and team prompt repositories, discover specialized AI skills, and view your usage metrics.
2. **Raksha Browser Guard (Desktop Agent):** A secure background utility installed on your work laptop that safeguards company confidential data, source code, and customer privacy while you interact with web-based AI tools (such as ChatGPT, Claude, Gemini, Perplexity, and DeepSeek).

---

## 2. Getting Started: Account Lifecycle

### 2.1 Registration & Sign-Up
1. Open your corporate portal URL in your web browser (e.g., `https://unifai.yourcompany.com`).
2. Click **Create Account** or **Sign Up**.
3. Enter your corporate email address (e.g., `jane.doe@yourcompany.com`), full name, and select a secure password.
4. **Email Verification:** A 6-digit confirmation code will be sent to your inbox. Enter the code on screen to verify your email address.
5. **Admin Approval:** Once verified, your account enters the `Pending Approval` queue. Your IT or security team will approve your account and assign your initial role and permissions. You will receive an email confirmation once approved.

### 2.2 Signing In & Session Security
- Log in with your email and password.
- Enterprise sessions remain active for **24 hours** by default.
- If Multi-Factor Authentication (MFA) is enabled by your administrator, enter your one-time authenticator passcode when prompted.

### 2.3 Password Recovery
If you forget your password:
1. Click **Forgot Password** on the login screen.
2. Enter your registered email address.
3. Check your email for a time-sensitive verification code (valid for 10 minutes).
4. Enter the code and set your new password.

---

## 3. Navigating the Workspace Portal

Once authenticated, your sidebar displays the features assigned to your user profile:

```
    Workspace Navigation
    ├── Playground         → Chat directly with authorized enterprise AI models
    ├── Prompt Repository  → Store, version, test, and share high-performing prompts
    ├── Skills Repository  → Pre-built agent skills and workflow automations
    ├── Virtual Keys       → View and manage developer API keys (if authorized)
    ├── Observability      → Track your personal token usage, latency, and costs
    └── Settings           → Manage profile details, notifications, and theme
```

### 3.1 Model Playground
The Playground provides a clean, interactive chat interface to interact with company-approved LLMs without requiring personal accounts or credit cards:
- Select an active model from the top dropdown (e.g., `gpt-4o`, `claude-3-5-sonnet`, `gemini-1.5-pro`, `llama-3.3-70b`).
- Adjust parameters: Temperature (creativity), Max Tokens (response length), and System Instructions.
- Supports multi-turn conversations, code syntax highlighting, and instant copying of outputs.

### 3.2 Prompt Repository
Save time by cataloging reusable prompt templates for yourself and your team:
- Click **+ New Prompt**.
- Define your prompt template using variables (e.g., `Summarize the following customer feedback into 3 bullet points: {{customer_text}}`).
- Tag with categories: `Engineering`, `Marketing`, `Legal`, `Support`.
- Share with your department or keep private in your personal library.

### 3.3 Observability & Usage Analytics
Review your personal AI consumption:
- Total requests sent over the past 24 hours, 7 days, or 30 days.
- Total tokens consumed (input vs. output tokens).
- Estimated cost and latency metrics.

---

## 4. Developer Integration Guide (Virtual Keys & APIs)

Software engineers can integrate corporate AI capabilities directly into scripts, backend services, CI/CD pipelines, and internal tools using **Virtual Keys**.

### 4.1 Authentication & Endpoint Discovery
- All API requests are authenticated via standard Bearer tokens in the HTTP Authorization header:
  `Authorization: Bearer <YOUR_VIRTUAL_KEY>`
- The gateway is 100% wire-compatible with the OpenAI REST specification.

### 4.2 Standard `curl` Request Example
```bash
curl https://unifai.yourcompany.com/v1/chat/completions \
  -H "Authorization: Bearer vk-live-abcdef1234567890abcdef1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "system", "content": "You are a helpful software engineering assistant."},
      {"role": "user", "content": "Write a Python function to parse JSON lines."}
    ],
    "temperature": 0.2
  }'
```

### 4.3 Python Integration (Official OpenAI SDK)
Simply point the `base_url` to your company's UnifAI gateway:

```python
import os
from openai import OpenAI

# Initialize client pointing to enterprise gateway
client = OpenAI(
    api_key=os.environ.get("UNIFAI_VIRTUAL_KEY", "vk-live-your-key-here"),
    base_url="https://unifai.yourcompany.com/v1"
)

# Standard chat completion call
response = client.chat.completions.create(
    model="claude-3-5-sonnet", # Use Anthropic, OpenAI, or Gemini interchangeably!
    messages=[
        {"role": "system", "content": "You are an enterprise AI assistant."},
        {"role": "user", "content": "Explain microservices architecture in 3 bullet points."}
    ],
    temperature=0.3,
    max_tokens=300
)

print(response.choices[0].message.content)
```

### 4.4 Node.js / TypeScript Integration
```typescript
import OpenAI from "openai";

const openai = new OpenAI({
  apiKey: process.env.UNIFAI_VIRTUAL_KEY || "vk-live-your-key-here",
  baseURL: "https://unifai.yourcompany.com/v1",
});

async function main() {
  const completion = await openai.chat.completions.create({
    model: "gpt-4o-mini",
    messages: [{ role: "user", content: "Generate a TypeScript interface for a User profile." }],
  });

  console.log(completion.choices[0].message.content);
}

main();
```

### 4.5 LangChain & LiteLLM Integration
```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="gpt-4o",
    api_key="vk-live-your-key-here",
    base_url="https://unifai.yourcompany.com/v1"
)

response = llm.invoke("What are the key benefits of vector embeddings?")
print(response.content)
```

### 4.6 IDE Integration (Cursor, VS Code Cline / Continue)
To route your AI coding tools through the secure corporate gateway:
- **Cursor Settings $\rightarrow$ Models:**
  - Override OpenAI Base URL: `https://unifai.yourcompany.com/v1`
  - Enter your Virtual Key in the OpenAI API Key field.
- **VS Code Continue / Cline Plugin:**
  - Set provider to `openai`.
  - Set `apiBase`: `https://unifai.yourcompany.com/v1`
  - Set `apiKey`: `vk-live-your-key-here`

---

## 5. Raksha Browser Guard: Employee Desktop Guide

### 5.1 What is Raksha Browser Guard?
Raksha Browser Guard is an endpoint security utility installed on your company computer. It runs silently in the background and acts as an intelligent safety shield when you visit public generative AI websites (ChatGPT, Claude.ai, Gemini, etc.).

### 5.2 Privacy Notice: What is Monitored vs. Ignored
- **Strictly Scoped:** The guard **ONLY** inspects traffic destined for configured AI websites (e.g. `chatgpt.com`, `claude.ai`).
- **Complete Privacy for All Other Traffic:** Your general web browsing, personal banking, social media, corporate email, and internal intranet sites **completely bypass the guard** and are never intercepted or viewed.

```
       Employee Laptop Web Traffic
                    │
                    ├── General Browsing (Google, News, Banks)  ──► DIRECT TO INTERNET (Zero Interception)
                    ├── Corporate Intranet / Slack / Email     ──► DIRECT TO INTERNET (Zero Interception)
                    │
                    └── Generative AI Websites (ChatGPT, etc.) ──► RAKSHA GUARD (Local Safety Inspection)
                                                                            │
                                                       ┌────────────────────┴────────────────────┐
                                                       ▼                                         ▼
                                                Safe Prompt Allowed                     Confidential Data Redacted / Blocked
```

### 5.3 Windows Installation Guide
1. Obtain the installer (`Raksha_Guard_Setup.exe`) from your IT portal or downloads section.
2. Double-click the installer and follow the on-screen wizard:
   - Accept the license agreement.
   - Click **Install** (Administrative elevation prompt may appear).
3. Once installation completes, the guard launches automatically.
4. **Verification:** Look for the **Raksha shield icon** in your Windows System Tray (near the clock). A green indicator indicates the guard is active and protecting your sessions.

### 5.4 macOS Installation Guide
1. Download the macOS installer package (`Raksha_Guard_macOS.pkg` or ZIP).
2. Open the package and complete the guided installation.
3. When prompted, allow the installer to trust the local security certificate in your macOS System Keychain.
4. **Verification:** The Raksha shield icon will appear in the macOS top menu bar.

### 5.5 In-Browser Experience & What You Will See

| Event | What Happens | What You See in Browser |
|---|---|---|
| **Normal Prompt** | Prompt contains no confidential or restricted data. | Prompt sends immediately to ChatGPT/Claude as normal. |
| **Redacted Prompt** | Prompt contains sensitive data like a credit card number or internal IP address. | The sensitive text is automatically masked with `[REDACTED_PCI_DATA]` before reaching the AI. |
| **Blocked Prompt** | Prompt violates enterprise policy (e.g., contains proprietary source code or confidential merger documents). | The request is stopped. An in-browser modal explains: *"Prompt blocked by corporate security policy (Rule: Proprietary Code Protection)"*. |

### 5.6 Troubleshooting & Helpdesk
- **Status is Offline:** Right-click the system tray icon and select **Restart Guard**.
- **Certificate Warning in Browser:** If Chrome or Edge shows an SSL warning on ChatGPT, open the tray icon and select **Reinstall Certificates**, then restart your browser.
- **Support Contact:** Reach out to your internal IT Helpdesk or email your security administrators at the contact address listed in your portal.

---
*End of End-User & Developer Guide.*
