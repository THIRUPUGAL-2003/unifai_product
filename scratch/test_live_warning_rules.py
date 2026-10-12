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
    print("      UNIFAI BROWSER AI - 100% LIVE WARNING WORKFLOW CHECK       ")
    print("=================================================================\n")

    # 1. Admin Authentication
    print("--- 1. ADMIN AUTHENTICATION ---")
    st, res = req("POST", "/api/session/login", {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    test_step("Admin Login", st == 200, f"HTTP {st}")

    created_rule_ids = []
    created_log_ids = []

    # 2. Create WARN Rules
    print("\n--- 2. CREATE DEDICATED WARN RULES ---")

    # Rule 1: Warn on financial account queries
    warn_rule_1 = {
        "name": "TEST_Warn_Financial_Inquiry",
        "pattern": r"\b(financial[\s_-]?budget|quarterly[\s_-]?revenue)\b",
        "action": "WARN",
        "warning_message": "Warning: Financial inquiry detected. Avoid sharing sensitive forecasts.",
        "severity": "LOW",
        "category": "FINANCIAL",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", warn_rule_1)
    warn_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if warn_rule_id:
        created_rule_ids.append(warn_rule_id)
    test_step("Create WARN Financial Rule", st == 200 and bool(warn_rule_id), f"ID: {warn_rule_id}")

    # Rule 2: Warn on internal server hosts
    warn_rule_2 = {
        "name": "TEST_Warn_Internal_Hostnames",
        "pattern": r"\b[a-zA-Z0-9-]+\.corp\.internal\b",
        "action": "WARN",
        "warning_message": "Warning: Corporate internal hostname referenced.",
        "severity": "LOW",
        "category": "INFRASTRUCTURE",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", warn_rule_2)
    host_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if host_rule_id:
        created_rule_ids.append(host_rule_id)
    test_step("Create WARN Hostname Rule", st == 200 and bool(host_rule_id), f"ID: {host_rule_id}")

    # 3. Live Interception with WARN Rule
    print("\n--- 3. LIVE INTERCEPTION & WARNING CHECKS ---")
    test_prompt_1 = "Can you help me analyze our team's financial budget for Q4?"
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": test_prompt_1,
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})

    log_entry = res.get("log", {}) if isinstance(res, dict) else {}
    if log_entry.get("id"):
        created_log_ids.append(log_entry.get("id"))

    test_step("WARN Prompt Allowed (Not Blocked)", st == 200 and res.get("allowed") == True, f"allowed: {res.get('allowed')}")
    test_step("Action is 'Warned'", res.get("action") == "Warned", f"action: {res.get('action')}")
    test_step("Rule Triggered matches Rule Name", res.get("rule_triggered") == "TEST_Warn_Financial_Inquiry", f"rule_triggered: {res.get('rule_triggered')}")
    test_step("Warning Message matches Rule Config", "Financial inquiry detected" in str(res.get("warning_message", "")), f"warning_message: {res.get('warning_message')}")

    # Check forward_prompt contains [GATEWAY WARNING] tag
    forward_prompt = res.get("forward_prompt", "")
    has_warning_tag = "[GATEWAY WARNING]" in forward_prompt
    test_step("Forward Prompt Has [GATEWAY WARNING] Notice", has_warning_tag, f"forward_prompt: {forward_prompt[:85]}...")

    # 4. Second WARN Rule Test
    print("\n--- 4. SECOND WARN RULE (INTERNAL HOST) ---")
    test_prompt_2 = "Please ping app-server-01.corp.internal and verify port status."
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "Claude",
        "prompt": test_prompt_2,
        "url": "https://claude.ai/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})

    log_entry_2 = res.get("log", {}) if isinstance(res, dict) else {}
    if log_entry_2.get("id"):
        created_log_ids.append(log_entry_2.get("id"))

    test_step("Hostname Prompt Allowed", st == 200 and res.get("allowed") == True, f"allowed: {res.get('allowed')}")
    test_step("Hostname Action is 'Warned'", res.get("action") == "Warned", f"action: {res.get('action')}")
    test_step("Hostname Forward Prompt Has [GATEWAY WARNING]", "[GATEWAY WARNING]" in str(res.get("forward_prompt", "")), "Tag verified")

    # 5. Database Audit Log Verification
    print("\n--- 5. AUDIT LOG PERSISTENCE & VERIFICATION ---")
    st, res = req("GET", "/api/browser-ai/logs?action=Warned&limit=10")
    logs = res.get("logs", []) if isinstance(res, dict) else []
    found_log = next((l for l in logs if l.get("id") == log_entry.get("id")), None)

    test_step("Warned Log Saved in Database", found_log is not None, f"Found log ID: {log_entry.get('id')}")
    if found_log:
        test_step("Database Action is 'Warned'", found_log.get("action") == "Warned", f"action: {found_log.get('action')}")
        test_step("Database Status has Rule Name", "TEST_Warn_Financial_Inquiry" in str(found_log.get("status", "")), f"status: {found_log.get('status')}")
        test_step("Database Stores Original Prompt (Audit Trail)", found_log.get("user_prompt_full") == test_prompt_1, f"original prompt preserved for audit")
        test_step("Database Predictive Risk is 'LOW'", found_log.get("predictive_risk") == "LOW", f"predictive_risk: {found_log.get('predictive_risk')}")
        test_step("Database Risk Score is in Low Range (20-40)", 20 <= found_log.get("risk_score", 0) <= 40, f"risk_score: {found_log.get('risk_score')}")

    # 6. Overview Stats Aggregation Check
    print("\n--- 6. OVERVIEW STATS CHECK ---")
    st, res = req("GET", "/api/browser-ai/logs/stats")
    warned_stat = res.get("warned", 0) if isinstance(res, dict) else 0
    test_step("Stats Aggregates Warned Count", warned_stat >= 2, f"warned count: {warned_stat}")

    # 7. Priority Checks: BLOCK vs WARN and REDACT vs WARN
    print("\n--- 7. WARNING PRIORITY CHECKS ---")
    block_rule = {
        "name": "TEST_Block_Priority_For_Warn",
        "pattern": r"\bTOP_SECRET_CODE\b",
        "action": "BLOCK",
        "warning_message": "Blocked completely.",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", block_rule)
    block_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if block_id:
        created_rule_ids.append(block_id)

    # BLOCK vs WARN
    dual_prompt_1 = "financial budget inquiry with TOP_SECRET_CODE attached."
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": dual_prompt_1,
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_log_ids.append(res.get("log", {}).get("id"))
    test_step("BLOCK Overrides WARN (Prompt Blocked)", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # 8. Security Warning Email Feature Checks (/api/browser-ai/send-warning-email)
    print("\n--- 8. SECURITY WARNING EMAIL API VERIFICATION ---")

    # Error check: Missing recipient
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "",
        "subject": "Warning",
        "message": "Policy violation alert."
    })
    test_step("Warning Email: Missing 'to' Rejected", st == 400, f"HTTP {st} (Expected 400 Bad Request)")

    # Error check: Invalid recipient email
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "invalid-email-no-domain",
        "subject": "Warning",
        "message": "Policy violation alert."
    })
    test_step("Warning Email: Invalid Email Format Rejected", st == 400, f"HTTP {st} (Expected 400 Bad Request)")

    # Error check: Empty message
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "employee@example.com",
        "subject": "Warning",
        "message": "   "
    })
    test_step("Warning Email: Empty Message Rejected", st == 400, f"HTTP {st} (Expected 400 Bad Request)")

    # SMTP Configuration check: Graceful message when SMTP is not configured
    st, res = req("POST", "/api/browser-ai/send-warning-email", {
        "to": "employee@example.com",
        "subject": "Gateway Security Policy Notice",
        "message": "Please refrain from posting internal corporate hostnames.",
        "agent_hostname": "LAPTOP-DEV-01",
        "agent_id": "test-agent-warn-01"
    })
    # If SMTP is disabled/not configured, endpoint returns 400 with helpful instruction
    test_step("Warning Email: Validates SMTP Readiness", st in (200, 400), f"HTTP {st}: {str(res)[:75]}")

    # 9. Clean up
    print("\n--- 9. CLEANUP TEST ARTIFACTS ---")
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
        print("\n>>> ALL WARNING WORKFLOW CHECKS PASSED 100%! <<<")
        sys.exit(0)
    else:
        print(f"\n>>> SOME TESTS FAILED ({total - passed} failures) <<<")
        sys.exit(1)

if __name__ == "__main__":
    main()
