"""
Comprehensive Live Test Suite:
Target Domains, Prompt Interception, File Uploads, Voice Prompts, and Block Policies.
Tests against Gateway daemon on http://localhost:8080 and browser_ai_proxy engine.
"""

import urllib.request
import json
import http.cookiejar
import sys
import time
import os

BASE = "http://localhost:8080"
GUARD_SECRET = "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

# Add proxy directory to path for proxy engine testing
PROXY_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "apps", "browser-guard", "proxy"))
if PROXY_DIR not in sys.path:
    sys.path.insert(0, PROXY_DIR)

results = []
created_log_ids = []

def test(title, condition, detail=""):
    results.append((title, condition, detail))
    icon = "[PASS]" if condition else "[FAIL]"
    print(f"  {icon} {title}: {detail}")
    return condition

def main():
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

    def req(method, path, data=None, extra_headers=None):
        req_data = json.dumps(data).encode("utf-8") if data is not None else None
        headers = {"Content-Type": "application/json"} if req_data is not None else {}
        if extra_headers:
            headers.update(extra_headers)
        request = urllib.request.Request(f"{BASE}{path}", data=req_data, headers=headers, method=method)
        try:
            resp = opener.open(request)
            raw = resp.read().decode("utf-8")
            parsed = json.loads(raw) if raw.startswith("{") or raw.startswith("[") else raw
            return resp.status, parsed
        except urllib.error.HTTPError as e:
            err_body = e.read().decode("utf-8")
            try:
                parsed_err = json.loads(err_body)
            except Exception:
                parsed_err = err_body
            return e.code, parsed_err
        except Exception as e:
            return 0, str(e)

    def intercept(prompt, metadata, platform="Browser AI", upload_images=None):
        payload = {
            "platform": platform,
            "prompt": prompt,
            "client_ip": "192.168.1.150",
            "agent_id": "test-live-audit-agent",
            "agent_hostname": "AUDIT-LAPTOP-01",
            "metadata": metadata
        }
        if upload_images:
            payload["upload_images"] = upload_images
        st, res = req("POST", "/api/browser-ai/intercept", payload, {"X-Gateway-Guard-Key": GUARD_SECRET})
        if isinstance(res, dict) and "log" in res and isinstance(res["log"], dict) and "id" in res["log"]:
            created_log_ids.append(res["log"]["id"])
        return st, res

    print("==========================================================================")
    print("      UNIFAI BROWSER AI - TARGET DOMAINS, PROMPTS, FILES & VOICE AUDIT    ")
    print("==========================================================================\n")

    # --- SETUP & LOGIN ---
    print("--- PRE-CHECK: AUTHENTICATION & PROXY MODULE ---")
    st, res = req("POST", "/api/session/login", {"username": ADMIN_USER, "password": ADMIN_PASS})
    test("Admin Login", st == 200 and res.get("role") == "admin", f"HTTP {st}")

    # Load proxy module
    import browser_ai_proxy
    test("Proxy Engine Loaded", hasattr(browser_ai_proxy, "get_target_domains"), "browser_ai_proxy module initialized")

    # ----------------------------------------------------------------------
    # SECTION 1: TARGET DOMAINS DISCOVERY & CLIENT SYNC
    # ----------------------------------------------------------------------
    print("\n--- SECTION 1: TARGET DOMAINS DISCOVERY & CLIENT SYNC ---")
    st, res = req("GET", "/api/browser-ai/targets")
    targets_list = res.get("targets", []) if isinstance(res, dict) else []
    test("Targets Endpoint Status", st == 200, f"HTTP {st}")
    test("Configured Target Domains Count", len(targets_list) >= 190, f"Total targets: {len(targets_list)}")

    # Check representative domains
    domain_map = {t.get("domain"): t for t in targets_list if isinstance(t, dict)}
    test("ChatGPT Target Configured", "chatgpt.com" in domain_map or "ab.chatgpt.com" in domain_map, "chatgpt.com verified")
    test("Claude Target Configured", "claude.ai" in domain_map or "anthropic.com" in domain_map, "claude/anthropic verified")
    test("Gemini Target Configured", "gemini.google.com" in domain_map or "aistudio.google.com" in domain_map, "gemini verified")
    test("Perplexity Target Configured", "perplexity.ai" in domain_map, "perplexity.ai verified")
    test("DeepSeek Target Configured", "deepseek.com" in domain_map or "platform.deepseek.com" in domain_map, "deepseek verified")

    # Agent format endpoint
    st_agent, res_agent = req("GET", "/api/browser-ai/targets?for=agent")
    agent_targets = res_agent.get("targets", []) if isinstance(res_agent, dict) else []
    test("Targets for Agent Endpoint", st_agent == 200 and len(agent_targets) > 0, f"Count: {len(agent_targets)}")
    if agent_targets:
        sample_ag = agent_targets[0]
        test("Agent Target Schema", "domain" in sample_ag and "monitored" in sample_ag and "block_site" in sample_ag, f"Fields: {list(sample_ag.keys())}")

    # Proxy memory cache sync
    proxy_targets = browser_ai_proxy.get_target_domains()
    test("Proxy In-Memory Targets Synced", len(proxy_targets) >= 190, f"In-memory: {len(proxy_targets)} domains")

    # ----------------------------------------------------------------------
    # SECTION 2: TEXT PROMPT INTERCEPTION & RULE EVALUATION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 2: TEXT PROMPT INTERCEPTION ON TARGET DOMAINS ---")

    # 2.1 Safe prompt on chatgpt.com
    st, res = intercept("Explain quantum computing in simple words", {"domain": "chatgpt.com"}, platform="ChatGPT")
    test("Safe Prompt on chatgpt.com Allowed", st == 200 and res.get("allowed") is True and res.get("action") == "Allowed", f"Action: {res.get('action')}")

    # 2.2 Safe prompt on claude.ai
    st, res = intercept("Write a python function to sort an array", {"domain": "claude.ai"}, platform="Claude")
    test("Safe Prompt on claude.ai Allowed", st == 200 and res.get("allowed") is True and res.get("action") == "Allowed", f"Action: {res.get('action')}")

    # 2.3 BLOCK Rule (Credit Card) on gemini.google.com
    st, res = intercept("My credit card number is 4111111111111111 for billing", {"domain": "gemini.google.com"}, platform="Gemini")
    test("Credit Card on gemini.google.com Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Visa Card Number", f"Rule: {res.get('rule_triggered')}")

    # 2.4 BLOCK Rule (Database Connection String) on perplexity.ai
    st, res = intercept("Connect using postgres://dbadmin:Sup3rS3cr3t@db.internal:5432/production_db", {"domain": "perplexity.ai"}, platform="Perplexity")
    test("DB String on perplexity.ai Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Database Connection String", f"Rule: {res.get('rule_triggered')}")

    # 2.5 BLOCK Rule (AWS Secret Access Key) on deepseek.com
    st, res = intercept("My config has aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", {"domain": "deepseek.com"}, platform="DeepSeek")
    test("AWS Secret on deepseek.com Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "AWS Secret Access Key", f"Rule: {res.get('rule_triggered')}")

    # 2.6 REDACT Rule on chatgpt.com
    st, res = intercept("Hello GWREDACT9X please analyze this", {"domain": "chatgpt.com"}, platform="ChatGPT")
    fwd = res.get("forward_prompt", "")
    test("REDACT Rule on chatgpt.com Allowed with Masking", st == 200 and res.get("allowed") is True and res.get("action") == "Redacted" and "[GATEWAY REDACTED]" in fwd, f"Forward prompt: {fwd[:60]}...")

    # 2.7 Multi-rule priority (BLOCK overrides REDACT)
    st, res = intercept("Here is GWREDACT9X and card 4111111111111111", {"domain": "chatgpt.com"}, platform="ChatGPT")
    test("BLOCK Priority Override over REDACT", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Visa Card Number", f"Action: {res.get('action')}, Rule: {res.get('rule_triggered')}")

    # ----------------------------------------------------------------------
    # SECTION 3: FILE UPLOAD & FILE PROMPT INTERCEPTION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 3: FILE UPLOAD & FILE PROMPT INTERCEPTION ---")

    # 3.1 Clean File Upload (Upload block OFF)
    st, res = intercept(
        "[FILE UPLOAD] quarterly_financials_summary.pdf (128 KB)",
        {
            "domain": "chatgpt.com",
            "file_name": "quarterly_financials_summary.pdf",
            "upload_scan": True,
            "extracted_text": "Q3 Revenue reached 4.2M with a 15% growth rate."
        },
        platform="ChatGPT"
    )
    test("Safe File Upload on chatgpt.com Allowed", st == 200 and res.get("allowed") is True and res.get("action") == "Allowed", f"Action: {res.get('action')}")

    # 3.2 File Upload with Confidential DB Connection String in Extracted Text
    st, res = intercept(
        "[FILE UPLOAD] database_backup.sql (45 KB)",
        {
            "domain": "claude.ai",
            "file_name": "database_backup.sql",
            "upload_scan": True,
            "extracted_text": "Host: postgres://admin:secretPass123@prod-cluster.internal:5432/core_db"
        },
        platform="Claude"
    )
    test("Confidential DB File on claude.ai Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Database Connection String", f"Rule: {res.get('rule_triggered')}")

    # 3.3 File Upload with Visa Card Number in Extracted Text
    st, res = intercept(
        "[FILE UPLOAD] scanned_receipt.png (256 KB)",
        {
            "domain": "gemini.google.com",
            "file_name": "scanned_receipt.png",
            "upload_scan": True,
            "extracted_text": "Customer Payment Details: Card ending in 4111111111111111 verified."
        },
        platform="Gemini"
    )
    test("Card Number in File on gemini.google.com Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Visa Card Number", f"Rule: {res.get('rule_triggered')}")

    # 3.4 File Upload Evaluation-Only Mode (Proxy Pre-Scan)
    st, res = intercept(
        "Checking database credentials postgres://admin:sec@db.int/prod in pre-scan",
        {
            "domain": "chatgpt.com",
            "evaluation_only": True,
            "extracted_text": "postgres://admin:sec@db.int/prod"
        },
        platform="ChatGPT"
    )
    test("File Pre-Scan Evaluation Mode Works", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked", f"Pre-scan verdict: {res.get('security_verdict')}")

    # ----------------------------------------------------------------------
    # SECTION 4: VOICE PROMPT & AUDIO INTERCEPTION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 4: VOICE PROMPT & AUDIO INTERCEPTION ---")

    # 4.1 Safe Voice Note / Audio Prompt
    st, res = intercept(
        "voice -- summarize the morning standup notes",
        {
            "domain": "chatgpt.com",
            "is_voice": True,
            "file_name": "Voice Note"
        },
        platform="ChatGPT"
    )
    test("Safe Voice Note on chatgpt.com Allowed", st == 200 and res.get("allowed") is True and res.get("action") == "Allowed", f"Action: {res.get('action')}")

    # 4.2 Safe Voice Upload Marker
    st, res = intercept(
        "[VOICE UPLOAD] Voice Note (audio recording 22s)",
        {
            "domain": "perplexity.ai",
            "is_voice": True,
            "file_name": "Voice Note",
            "upload_scan": True,
            "extracted_text": "Can you give me a recipe for chicken soup?"
        },
        platform="Perplexity"
    )
    test("Safe Voice Upload Marker on perplexity.ai Allowed", st == 200 and res.get("allowed") is True and res.get("action") == "Allowed", f"Action: {res.get('action')}")

    # 4.3 Voice Note with Card Number in Spoken Transcript
    st, res = intercept(
        "voice -- my credit card number is 4111111111111111 for the subscription",
        {
            "domain": "deepseek.com",
            "is_voice": True,
            "file_name": "Voice Note"
        },
        platform="DeepSeek"
    )
    test("Voice Note Card Number on deepseek.com Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Visa Card Number", f"Rule: {res.get('rule_triggered')}")

    # 4.4 Voice Upload with Database Secret in Transcribed Speech
    st, res = intercept(
        "[VOICE UPLOAD] Voice Note (audio transcript)",
        {
            "domain": "chatgpt.com",
            "is_voice": True,
            "file_name": "Voice Note",
            "upload_scan": True,
            "extracted_text": "Please connect to postgres://admin:secretPass@prod.cluster:5432/main"
        },
        platform="ChatGPT"
    )
    test("Voice Upload DB Secret on chatgpt.com Blocked", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and res.get("rule_triggered") == "Database Connection String", f"Rule: {res.get('rule_triggered')}")

    # ----------------------------------------------------------------------
    # SECTION 5: UPLOAD BLOCK POLICY (GLOBAL & TARGET CONTROLS)
    # ----------------------------------------------------------------------
    print("\n--- SECTION 5: UPLOAD BLOCK POLICY ENFORCEMENT ---")

    # 5.1 Enable Block Upload in Controls
    st_get_ctrl, orig_ctrl = req("GET", "/api/browser-ai/controls")
    orig_enabled = orig_ctrl.get("controls", {}).get("enabled", False)
    test("Fetch Current Controls", st_get_ctrl == 200, f"Enabled: {orig_ctrl.get('controls', {}).get('block_upload')}")

    custom_warning = "STRICT SECURITY: All file attachments and voice uploads are forbidden by company policy."
    update_ctrl_payload = {
        "enabled": True,
        "block_upload": True,
        "upload_warning": custom_warning
    }
    st_put_ctrl, res_put_ctrl = req("PUT", "/api/browser-ai/controls", update_ctrl_payload)
    test("Enable Block Upload in Controls", st_put_ctrl == 200 and res_put_ctrl.get("controls", {}).get("block_upload") is True, "block_upload = True")

    # Let proxy sync controls immediately
    browser_ai_proxy.get_control_settings(force_network=True)
    test("Proxy Engine Synced block_upload=True", browser_ai_proxy.controls_active("block_upload") is True, f"controls_active('block_upload') = {browser_ai_proxy.controls_active('block_upload')}")

    # 5.2 Test File Upload Block Enforcement on target domain
    st, res = intercept(
        "[FILE UPLOAD] company_presentation.pptx (5.2 MB)",
        {
            "domain": "chatgpt.com",
            "file_name": "company_presentation.pptx",
            "is_blocked": True,
            "blocked_reason": "Block Upload"
        },
        platform="ChatGPT"
    )
    test("File Upload Blocked by Policy", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and "Block Upload" in res.get("log", {}).get("status", ""), f"Status: {res.get('log', {}).get('status')}")

    # 5.3 Test Voice Upload Block Enforcement on target domain
    st, res = intercept(
        "[VOICE UPLOAD] Voice Note (audio recording)",
        {
            "domain": "claude.ai",
            "file_name": "Voice Note",
            "is_voice": True,
            "is_blocked": True,
            "blocked_reason": "Block Upload"
        },
        platform="Claude"
    )
    test("Voice Upload Blocked by Policy", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked" and "Block Upload" in res.get("log", {}).get("status", ""), f"Status: {res.get('log', {}).get('status')}")

    # 5.4 Test Proxy file_policy handle_upload_send function directly
    block_msg = browser_ai_proxy.get_control_settings().get("upload_warning")
    test("Proxy returns configured Upload Warning", block_msg == custom_warning, f"Warning: {block_msg}")

    # 5.5 Revert Block Upload to original setting
    revert_payload = {
        "enabled": orig_enabled,
        "block_upload": False,
        "upload_warning": ""
    }
    st_rev, _ = req("PUT", "/api/browser-ai/controls", revert_payload)
    browser_ai_proxy.get_control_settings(force_network=True)
    test("Revert Block Upload Setting", st_rev == 200 and browser_ai_proxy.controls_active("block_upload") is False, "block_upload = False reverted")

    # ----------------------------------------------------------------------
    # SECTION 6: BLOCK ENTIRE WEBSITE (TARGET SITE BLOCK)
    # ----------------------------------------------------------------------
    print("\n--- SECTION 6: TARGET DOMAIN SITE BLOCK ---")

    # 6.1 Test Gateway Intercept on Block Entire Website
    st, res = intercept(
        "[SITE BLOCKED: Restricted Target Domain]",
        {
            "domain": "restricted-ai-site.com",
            "is_blocked": True,
            "blocked_reason": "Block Entire Website"
        },
        platform="Restricted AI"
    )
    test("Site Block Intercepted", st == 200 and res.get("allowed") is False and res.get("action") == "Blocked", f"Action: {res.get('action')}")
    test("Site Block Category and Risk Score", res.get("predicted_category") == "SITE_BLOCK" and res.get("risk_score") == 95, f"Category: {res.get('predicted_category')}, Risk: {res.get('risk_score')}")

    # 6.2 Test Proxy detect_site_block with temporary blocked target
    temp_target_domain = "temp-audit-blocked-site.com"
    st_tgt, _ = req("POST", "/api/browser-ai/targets", {
        "domain": temp_target_domain,
        "platform_name": "Temp Blocked AI",
        "monitored": True,
        "block_site": True
    })
    test("Create Target with Block Site Enabled", st_tgt == 200, f"Domain: {temp_target_domain}")
    browser_ai_proxy._refresh_targets_from_backend()

    is_blocked, blocked_dom, blocked_plat = browser_ai_proxy.detect_site_block(temp_target_domain)
    test("Proxy detect_site_block identifies Blocked Domain", is_blocked is True and blocked_dom == temp_target_domain, f"Blocked: {is_blocked}, Plat: {blocked_plat}")

    # Subdomain match test
    is_sub_blocked, _, _ = browser_ai_proxy.detect_site_block(f"chat.{temp_target_domain}")
    test("Proxy detect_site_block Subdomain Match", is_sub_blocked is True, f"Subdomain blocked: {is_sub_blocked}")

    # Cleanup temp target
    # Find ID of created target
    _, cur_targets = req("GET", "/api/browser-ai/targets")
    temp_id = None
    for t in cur_targets.get("targets", []):
        if t.get("domain") == temp_target_domain:
            temp_id = t.get("id")
            break
    if temp_id:
        st_del, _ = req("DELETE", f"/api/browser-ai/targets/{temp_id}")
        test("Cleaned up Temp Blocked Target", st_del == 200, f"ID: {temp_id}")

    # ----------------------------------------------------------------------
    # SECTION 7: CROSS-DOMAIN MATRIX AUDIT (10 REPRESENTATIVE TARGET PLATFORMS)
    # ----------------------------------------------------------------------
    print("\n--- SECTION 7: MULTI-TARGET DOMAIN MATRIX AUDIT ---")
    matrix_domains = [
        ("chatgpt.com", "ChatGPT"),
        ("claude.ai", "Claude"),
        ("gemini.google.com", "Gemini"),
        ("perplexity.ai", "Perplexity"),
        ("deepseek.com", "DeepSeek"),
        ("copilot.microsoft.com", "Copilot"),
        ("meta.ai", "Meta AI"),
        ("poe.com", "Poe"),
        ("mistral.ai", "Mistral AI"),
        ("huggingface.co", "Hugging Face")
    ]

    for dom, plat in matrix_domains:
        # Prompt clean check
        st, res = intercept(f"Hello from {plat} safety test", {"domain": dom}, platform=plat)
        pass_clean = (st == 200 and res.get("allowed") is True)

        # Prompt violation check
        st_v, res_v = intercept(f"Violating DB: postgres://u:p@{dom}:5432/db", {"domain": dom}, platform=plat)
        pass_viol = (st_v == 200 and res_v.get("allowed") is False and res_v.get("action") == "Blocked")

        test(f"Domain Matrix: {plat} ({dom})", pass_clean and pass_viol, f"Clean: {res.get('action')}, Violation: {res_v.get('action')}")

    # ----------------------------------------------------------------------
    # SECTION 8: AUDIT LOGS PURGE & ZERO DATABASE POLLUTION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 8: CLEANUP & ZERO LEAKS VERIFICATION ---")
    deleted_count = 0
    for lid in created_log_ids:
        st_del, _ = req("DELETE", f"/api/browser-ai/logs/{lid}")
        if st_del == 200:
            deleted_count += 1
    test("Purge All Created Test Logs", deleted_count == len(created_log_ids), f"Purged {deleted_count}/{len(created_log_ids)} logs")

    # Summary
    total_tests = len(results)
    passed_tests = sum(1 for _, ok, _ in results if ok)
    failed_tests = total_tests - passed_tests
    print("\n==========================================================================")
    print(f"  TARGET DOMAIN AUDIT COMPLETE: {passed_tests}/{total_tests} PASS (Failures: {failed_tests})")
    print("==========================================================================")

    if failed_tests > 0:
        sys.exit(1)

if __name__ == "__main__":
    main()
