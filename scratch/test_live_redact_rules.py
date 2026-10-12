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

    def test_step(title, condition, detail=""):
        status_str = "PASS" if condition else "FAIL"
        results.append((title, condition, detail))
        icon = "[PASS]" if condition else "[FAIL]"
        print(f"  {icon} {title}: {detail}")
        return condition

    print("=================================================================")
    print("      UNIFAI BROWSER AI - 100% LIVE REDACT RULE VERIFICATION     ")
    print("=================================================================\n")

    # 1. Admin Authentication
    print("--- 1. ADMIN AUTHENTICATION ---")
    st, res = req("POST", "/api/session/login", {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    test_step("Admin Login", st == 200, f"HTTP {st}")

    created_rule_ids = []
    created_log_ids = []

    # 2. Create REDACT Rule
    print("\n--- 2. CREATE DEDICATED REDACT RULES ---")

    # Rule 1: REDACT Customer Social Security / Tax ID
    redact_ssn_rule = {
        "name": "TEST_Redact_Customer_SSN",
        "pattern": r"\b\d{3}-\d{2}-\d{4}\b",
        "action": "REDACT",
        "warning_message": "Customer SSN was redacted for privacy compliance.",
        "severity": "HIGH",
        "category": "PII",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", redact_ssn_rule)
    ssn_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if ssn_rule_id:
        created_rule_ids.append(ssn_rule_id)
    test_step("Create REDACT SSN Rule", st == 200 and bool(ssn_rule_id), f"ID: {ssn_rule_id}")

    # Rule 2: REDACT Internal Secret Key
    redact_key_rule = {
        "name": "TEST_Redact_Secret_Token",
        "pattern": r"\bTOK-SECRET-[A-Za-z0-9]{8,16}\b",
        "action": "REDACT",
        "warning_message": "Internal secret token was masked.",
        "severity": "CRITICAL",
        "category": "SECRETS",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", redact_key_rule)
    key_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if key_rule_id:
        created_rule_ids.append(key_rule_id)
    test_step("Create REDACT Token Rule", st == 200 and bool(key_rule_id), f"ID: {key_rule_id}")

    # 3. Live Interception with REDACT Rule
    print("\n--- 3. LIVE INTERCEPTION & REDACTION TESTING ---")

    test_prompt_1 = "Please check background report for customer with SSN 123-45-6789 immediately."
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": test_prompt_1,
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})

    log_entry = res.get("log", {}) if isinstance(res, dict) else {}
    if log_entry.get("id"):
        created_log_ids.append(log_entry.get("id"))

    test_step("REDACT Prompt Allowed (Not Blocked)", st == 200 and res.get("allowed") == True, f"allowed: {res.get('allowed')}")
    test_step("Action is 'Redacted'", res.get("action") == "Redacted", f"action: {res.get('action')}")
    test_step("Rule Triggered matches Rule Name", res.get("rule_triggered") == "TEST_Redact_Customer_SSN", f"rule_triggered: {res.get('rule_triggered')}")
    test_step("Warning Message matches Rule Config", "Customer SSN was redacted" in str(res.get("warning_message", "")), f"warning_message: {res.get('warning_message')}")

    # Check forward_prompt and redacted_prompt
    forward_prompt = res.get("forward_prompt", "")
    redacted_prompt = res.get("redacted_prompt", "")
    has_redact_tag = "[GATEWAY REDACTED]" in forward_prompt
    test_step("Forward Prompt Has [GATEWAY REDACTED] Notice", has_redact_tag, f"forward_prompt: {forward_prompt[:90]}...")
    test_step("Redacted Prompt Matches Forward Prompt", redacted_prompt == forward_prompt, f"redacted_prompt equal")

    # 4. Intercept Second REDACT Rule (Token)
    print("\n--- 4. SECOND REDACT RULE TESTING ---")
    test_prompt_2 = "Connecting to database with credentials TOK-SECRET-9876543210 ready."
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "Claude",
        "prompt": test_prompt_2,
        "url": "https://claude.ai/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})

    log_entry_2 = res.get("log", {}) if isinstance(res, dict) else {}
    if log_entry_2.get("id"):
        created_log_ids.append(log_entry_2.get("id"))

    test_step("Token REDACT Allowed", st == 200 and res.get("allowed") == True, f"allowed: {res.get('allowed')}")
    test_step("Token Action is 'Redacted'", res.get("action") == "Redacted", f"action: {res.get('action')}")
    test_step("Token Rule Triggered matches", res.get("rule_triggered") == "TEST_Redact_Secret_Token", f"rule: {res.get('rule_triggered')}")
    test_step("Token Forward Prompt Contains Tag", "[GATEWAY REDACTED]" in str(res.get("forward_prompt", "")), "Tag verified")

    # 5. Database Audit Log Verification
    print("\n--- 5. AUDIT LOG PERSISTENCE & VERIFICATION ---")
    st, res = req("GET", "/api/browser-ai/logs?action=Redacted&limit=10")
    logs = res.get("logs", []) if isinstance(res, dict) else []
    found_log = next((l for l in logs if l.get("id") == log_entry.get("id")), None)

    test_step("Redacted Log Saved in Database", found_log is not None, f"Found log ID: {log_entry.get('id')}")
    if found_log:
        test_step("Database Action is 'Redacted'", found_log.get("action") == "Redacted", f"action: {found_log.get('action')}")
        test_step("Database Status has Rule Name", "TEST_Redact_Customer_SSN" in str(found_log.get("status", "")), f"status: {found_log.get('status')}")
        test_step("Database Stores Original Prompt (Audit Trail)", found_log.get("user_prompt_full") == test_prompt_1, f"original prompt preserved for audit")
        test_step("Database Predicted Category is 'SUSPICIOUS_CONTENT'", found_log.get("predicted_category") == "SUSPICIOUS_CONTENT", f"category: {found_log.get('predicted_category')}")

    # 6. Overview Stats Aggregation Check
    print("\n--- 6. OVERVIEW STATS CHECK ---")
    st, res = req("GET", "/api/browser-ai/logs/stats")
    warned_stat = res.get("warned", 0) if isinstance(res, dict) else 0
    test_step("Stats Includes Redacted Count in 'warned'", warned_stat >= 2, f"warned count: {warned_stat}")

    # 7. Priority: BLOCK vs REDACT
    print("\n--- 7. RULE PRIORITY: BLOCK VS REDACT ---")
    block_rule = {
        "name": "TEST_Block_Priority_Check",
        "pattern": r"\bCONFIDENTIAL_OVERRIDE\b",
        "action": "BLOCK",
        "warning_message": "Blocked by priority rule.",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", block_rule)
    block_priority_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if block_priority_id:
        created_rule_ids.append(block_priority_id)

    # Prompt with BOTH BLOCK and REDACT triggers
    dual_prompt = "Customer SSN 999-88-7777 has CONFIDENTIAL_OVERRIDE flag."
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": dual_prompt,
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_log_ids.append(res.get("log", {}).get("id"))

    test_step("Priority: BLOCK Overrides REDACT", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}, rule: {res.get('rule_triggered')}")

    # 8. Clean up
    print("\n--- 8. CLEANUP TEST ARTIFACTS ---")
    del_rules = 0
    for rid in created_rule_ids:
        st, _ = req("DELETE", f"/api/browser-ai/rules/{rid}")
        if st == 200:
            del_rules += 1
    test_step(f"Delete All {len(created_rule_ids)} Test Rules", del_rules == len(created_rule_ids), f"Deleted: {del_rules}/{len(created_rule_ids)}")

    if created_log_ids:
        st, res = req("POST", "/api/browser-ai/logs/bulk-delete", {"ids": created_log_ids})
        test_step(f"Delete All {len(created_log_ids)} Test Logs", st == 200, f"HTTP {st}")

    # Summary
    print("\n=================================================================")
    passed = sum(1 for _, ok, _ in results if ok)
    total = len(results)
    print(f"  TOTAL RESULTS: {passed}/{total} TESTS PASSED")
    print("=================================================================")

    if passed == total:
        print("\n>>> ALL REDACT RULE WORKFLOWS PASSED 100%! <<<")
        sys.exit(0)
    else:
        print(f"\n>>> SOME TESTS FAILED ({total - passed} failures) <<<")
        sys.exit(1)

if __name__ == "__main__":
    main()
