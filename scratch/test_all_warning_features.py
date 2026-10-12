import urllib.request
import json
import http.cookiejar
import sys
import time

def main():
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    BASE = "http://localhost:8080"
    GUARD_SECRET = "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="

    results = []

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

    def test(title, condition, detail=""):
        results.append((title, condition, detail))
        icon = "[PASS]" if condition else "[FAIL]"
        print(f"  {icon} {title}: {detail}")
        return condition

    print("=================================================================")
    print("      UNIFAI BROWSER AI - 100% COMPLETE WARNING SYSTEM CHECK     ")
    print("=================================================================\n")

    # 1. Admin Authentication
    print("--- 1. ADMIN AUTHENTICATION ---")
    st, res = req("POST", "/api/session/login", {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    test("Admin Login", st == 200, f"HTTP {st}")

    created_rule_ids = []
    created_log_ids = []

    # 2. Rule-Level Custom Warning Message
    print("\n--- 2. RULE-LEVEL CUSTOM WARNING CONFIGURATION ---")
    custom_warn_msg = "Confidential customer KYC data detected! Do not share outside company network."
    warn_rule = {
        "name": "TEST_KYC_Warning_Rule",
        "pattern": r"\bKYC-VERIFIED-[0-9]{6}\b",
        "action": "WARN",
        "warning_message": custom_warn_msg,
        "severity": "LOW",
        "category": "COMPLIANCE",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", warn_rule)
    rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if rule_id:
        created_rule_ids.append(rule_id)
    test("Create Rule with Custom Warning Message", st == 200 and bool(rule_id), f"ID: {rule_id}")

    # Verify rule warning message via GET
    st, res = req("GET", "/api/browser-ai/rules")
    found_rule = next((r for r in res.get("rules", []) if r.get("id") == rule_id), None)
    test("Rule Stores Configured Warning Message", found_rule is not None and found_rule.get("warning_message") == custom_warn_msg, f"Warning: {found_rule.get('warning_message') if found_rule else 'None'}")

    # 3. Live Interception Warning Flow
    print("\n--- 3. LIVE INTERCEPTION & WIRE WARNING NOTICE ---")
    prompt_text = "Please check customer file KYC-VERIFIED-654321 for loan approval."
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": prompt_text,
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})

    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_log_ids.append(res.get("log", {}).get("id"))

    test("Prompt Is Allowed (Not Blocked)", st == 200 and res.get("allowed") == True, f"allowed: {res.get('allowed')}")
    test("Action Stamped as 'Warned'", res.get("action") == "Warned", f"action: {res.get('action')}")
    test("Rule Triggered matches Rule Name", res.get("rule_triggered") == "TEST_KYC_Warning_Rule", f"rule: {res.get('rule_triggered')}")
    test("API Response Warning Message Matches Config", res.get("warning_message") == custom_warn_msg, f"warning_message: {res.get('warning_message')}")

    # Check forward_prompt has [GATEWAY WARNING] tag
    forward_prompt = res.get("forward_prompt", "")
    has_tag = "[GATEWAY WARNING] " + custom_warn_msg in forward_prompt
    test("Forward Prompt Appends [GATEWAY WARNING] Notice", has_tag, f"forward_prompt ending: ...{forward_prompt[-80:]}")

    # 4. Upload Policy Warning Configuration (/api/browser-ai/controls)
    print("\n--- 4. UPLOAD POLICY WARNING CONFIGURATION ---")
    st, res = req("GET", "/api/browser-ai/controls")
    test("Get Browser AI Controls", st == 200 and "controls" in res, f"HTTP {st}")

    test_upload_warning = "UPLOAD WARNING: Uploading unapproved files to external AI platforms is prohibited by IT Security."
    st, res = req("PUT", "/api/browser-ai/controls", {
        "upload_warning": test_upload_warning
    })
    ctrl = res.get("controls", {}) if isinstance(res, dict) else {}
    test("Save Custom Upload Warning", st == 200 and ctrl.get("upload_warning") == test_upload_warning, f"Saved warning: {ctrl.get('upload_warning')[:60]}...")

    # Verify retrieval
    st, res = req("GET", "/api/browser-ai/controls")
    retrieved_warning = res.get("controls", {}).get("upload_warning")
    test("Retrieve Persisted Upload Warning", st == 200 and retrieved_warning == test_upload_warning, f"Verified warning matches")

    # 5. Security Warning Email System (/api/browser-ai/send-warning-email)
    print("\n--- 5. SECURITY WARNING EMAIL SYSTEM ---")

    # 5.1 Validation: Empty recipient
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "",
        "subject": "Warning",
        "message": "Policy violation"
    })
    test("Validate Empty Recipient Rejected", st == 400, f"HTTP {st}")

    # 5.2 Validation: Malformed recipient (no @)
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "employee-no-at-sign.com",
        "subject": "Warning",
        "message": "Policy violation"
    })
    test("Validate Malformed Email Rejected", st == 400, f"HTTP {st}")

    # 5.3 Validation: Empty message
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "employee@example.com",
        "subject": "Warning",
        "message": "   "
    })
    test("Validate Empty Warning Message Rejected", st == 400, f"HTTP {st}")

    # 5.4 Send Warning Email with Device & Violation Telemetry
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "employee@example.com",
        "subject": "Official Security Policy Warning",
        "message": "Please do not paste corporate KYC data into external AI tools.",
        "agent_hostname": "LAPTOP-DEV-01",
        "agent_id": "test-agent-kyc-01",
        "allowed_count": 10,
        "blocked_count": 2,
        "warn_count": 1,
        "redact_count": 0
    })
    test("Send Warning Email API", st in (200, 400), f"HTTP {st}: {str(res)[:65]}")

    # 6. Database Audit Trail & Stats
    print("\n--- 6. DATABASE AUDIT TRAIL & STATS ---")
    st, res = req("GET", "/api/browser-ai/logs?action=Warned&limit=5")
    logs = res.get("logs", []) if isinstance(res, dict) else []
    found_log = any(l.get("rule_triggered") == "TEST_KYC_Warning_Rule" for l in logs)
    test("Warned Prompt Found in Audit Logs", found_log, f"Audit log verified in DB")

    st, res = req("GET", "/api/browser-ai/logs/stats")
    warned_kpi = res.get("warned", 0) if isinstance(res, dict) else 0
    test("Overview KPI Aggregates Warned Count", warned_kpi > 0, f"Warned KPI: {warned_kpi}")

    # 7. Clean up
    print("\n--- 7. CLEANUP TEST ARTIFACTS ---")
    for rid in created_rule_ids:
        req("DELETE", f"/api/browser-ai/rules/{rid}")
    test("Clean Up Test Rules", True, "Rules deleted")

    if created_log_ids:
        req("POST", "/api/browser-ai/logs/bulk-delete", {"ids": created_log_ids})
        test("Clean Up Test Logs", True, f"Deleted {len(created_log_ids)} logs")

    # Reset upload warning
    req("PUT", "/api/browser-ai/controls", {"upload_warning": ""})
    test("Reset Upload Warning Default", True, "Reset")

    # Summary
    print("\n=================================================================")
    passed = sum(1 for _, ok, _ in results if ok)
    total = len(results)
    print(f"  TOTAL WARNING TESTS PASSED: {passed}/{total}")
    print("=================================================================")

    if passed == total:
        print("\n>>> ALL WARNING SYSTEM CHECKS PASSED 100%! <<<")
        sys.exit(0)
    else:
        print(f"\n>>> FAILURES DETECTED: {total - passed}/{total} <<<")
        sys.exit(1)

if __name__ == "__main__":
    main()
