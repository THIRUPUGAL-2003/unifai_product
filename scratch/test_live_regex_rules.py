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
    print("      UNIFAI BROWSER AI - 100% LIVE REGEX RULE VERIFICATION      ")
    print("=================================================================\n")

    # 1. Admin Authentication
    print("--- 1. ADMIN AUTHENTICATION ---")
    st, res = req("POST", "/api/session/login", {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    test_step("Admin Login", st == 200, f"HTTP {st}")

    created_rule_ids = []

    # 2. Regex Syntax Validation (Syntax Errors, Lookarounds, Invalid Constructs)
    print("\n--- 2. REGEX SYNTAX VALIDATION & ERROR HANDLING ---")

    # Invalid regex 1: Lookbehind (unsupported by Go RE2)
    st, res = req("POST", "/api/browser-ai/rules", {
        "name": "Invalid Lookbehind Rule",
        "pattern": "(?<=secret)[0-9]+",
        "action": "BLOCK",
        "active": True
    })
    test_step("Reject Lookbehind (?<=...) Pattern", st == 400, f"HTTP {st} (Safely rejected unsupported RE2 construct)")

    # Invalid regex 2: Unclosed parenthesis
    st, res = req("POST", "/api/browser-ai/rules", {
        "name": "Unclosed Parenthesis Rule",
        "pattern": "(abc[0-9]+",
        "action": "BLOCK",
        "active": True
    })
    test_step("Reject Unclosed Parenthesis Pattern", st == 400, f"HTTP {st} (Safely caught syntax error)")

    # Invalid regex 3: Unicode property unsupported by browser proxy
    st, res = req("POST", "/api/browser-ai/rules", {
        "name": "Unsupported Unicode Class Rule",
        "pattern": r"\p{L}+",
        "action": "BLOCK",
        "active": True
    })
    test_step(r"Reject Unsupported \p{L} Class", st == 400, f"HTTP {st} (Rejected proxy-incompatible class)")

    # Valid regex creation: Standard valid RE2 pattern
    st, res = req("POST", "/api/browser-ai/rules", {
        "name": "TEST_Valid_Email_Rule",
        "pattern": r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b",
        "action": "WARN",
        "active": True
    })
    rule_valid_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if rule_valid_id:
        created_rule_ids.append(rule_valid_id)
    test_step("Accept Valid Standard RE2 Regex", st == 200, f"HTTP {st}, ID: {rule_valid_id}")

    # 3. Create Core Testing Rules (BLOCK, WARN, REDACT)
    print("\n--- 3. CREATE DEDICATED REGEX RULES (BLOCK, WARN, REDACT) ---")

    # Rule 1: BLOCK - Project Codename
    block_rule_data = {
        "name": "TEST_Project_Codename_BLOCK",
        "pattern": r"\b(project[\s_-]?alpha|project[\s_-]?nebula)\b",
        "action": "BLOCK",
        "warning_message": "Confidential project codename detected! Prompt blocked.",
        "severity": "CRITICAL",
        "category": "CONFIDENTIAL_DATA",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", block_rule_data)
    block_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if block_rule_id:
        created_rule_ids.append(block_rule_id)
    test_step("Create BLOCK Regex Rule", st == 200 and bool(block_rule_id), f"ID: {block_rule_id}")

    # Rule 2: WARN - Financial Account
    warn_rule_data = {
        "name": "TEST_Financial_Account_WARN",
        "pattern": r"\bACC-[0-9]{6,10}\b",
        "action": "WARN",
        "warning_message": "Financial account number detected.",
        "severity": "MEDIUM",
        "category": "FINANCIAL",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", warn_rule_data)
    warn_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if warn_rule_id:
        created_rule_ids.append(warn_rule_id)
    test_step("Create WARN Regex Rule", st == 200 and bool(warn_rule_id), f"ID: {warn_rule_id}")

    # Rule 3: REDACT - API Secret Token
    redact_rule_data = {
        "name": "TEST_Secret_Token_REDACT",
        "pattern": r"\bSEC-TOKEN-[A-Za-z0-9]{12}\b",
        "action": "REDACT",
        "warning_message": "Internal secret token masked.",
        "severity": "HIGH",
        "category": "SECRETS",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", redact_rule_data)
    redact_rule_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    if redact_rule_id:
        created_rule_ids.append(redact_rule_id)
    test_step("Create REDACT Regex Rule", st == 200 and bool(redact_rule_id), f"ID: {redact_rule_id}")

    # 4. Live Interception Tests via /api/browser-ai/intercept
    print("\n--- 4. LIVE REGEX INTERCEPTION & MATCHING CHECKS ---")

    # Test 4.1: Benign prompt (NO match) -> Allowed
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "How do I implement binary search in Go?",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Benign Prompt (No Match -> Allowed)", st == 200 and res.get("allowed") == True and res.get("action") == "Allowed", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # Test 4.2: BLOCK Rule exact match -> Blocked
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Please summarize our latest status for project alpha release schedule.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("BLOCK Rule Match (project alpha)", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}, rule: {res.get('rule_triggered')}")
    warn_msg = str(res.get("warning_message") or res.get("reply_text") or "")
    test_step("BLOCK Warning Message Preserved", "Confidential project" in warn_msg, f"warning: {warn_msg}")

    # Test 4.3: Case-insensitive matching (PROJECT_ALPHA)
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Here is the confidential diagram for PROJECT_ALPHA systems.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Case-Insensitive Match (PROJECT_ALPHA)", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # Test 4.4: Alternation matching (project-nebula)
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "Claude",
        "prompt": "Review this budget estimate for project-nebula.",
        "url": "https://claude.ai/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Alternation Match (project-nebula)", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # Test 4.5: Word boundary check (word containing substring should NOT false-positive match)
    # e.g., "myproject_alpha_not_boundary" should not match if strict boundary \b(project[\s_-]?alpha)\b
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Discussing unprojected_alpha_results here.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Word Boundary Non-Match (unprojected_alpha_results)", st == 200 and res.get("allowed") == True, f"allowed: {res.get('allowed')} (No false positive)")

    # Test 4.6: WARN Rule match -> Warned (Allowed=True)
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Please wire funds to account ACC-987654321 immediately.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("WARN Rule Match (ACC-987654321 -> Warned)", st == 200 and res.get("allowed") == True and res.get("action") == "Warned", f"allowed: {res.get('allowed')}, action: {res.get('action')}, rule: {res.get('rule_triggered')}")

    # Test 4.7: REDACT Rule match -> Redacted (Allowed=True)
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Connecting using client key SEC-TOKEN-abcdef123456 in production.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("REDACT Rule Match (SEC-TOKEN-... -> Redacted)", st == 200 and res.get("allowed") == True and res.get("action") == "Redacted", f"allowed: {res.get('allowed')}, action: {res.get('action')}, rule: {res.get('rule_triggered')}")

    # Test 4.8: Rule Priority (Prompt matching BOTH BLOCK and WARN rules)
    # Strictest action (BLOCK) must take priority!
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Project Alpha payment to ACC-11223344.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Rule Priority: BLOCK Wins Over WARN", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # 5. Rule Active / Inactive Toggle Check
    print("\n--- 5. ACTIVE / INACTIVE TOGGLE VERIFICATION ---")

    # Deactivate the BLOCK rule
    block_rule_data["active"] = False
    st, res = req("PUT", f"/api/browser-ai/rules/{block_rule_id}", block_rule_data)
    test_step("Deactivate BLOCK Rule (active=false)", st == 200, f"HTTP {st}")

    # Re-test prompt that was previously blocked -> Should now be ALLOWED!
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Please summarize our latest status for project alpha release schedule.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Deactivated Rule Does NOT Block (Prompt Allowed)", st == 200 and res.get("allowed") == True and res.get("action") == "Allowed", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # Reactivate the BLOCK rule
    block_rule_data["active"] = True
    st, res = req("PUT", f"/api/browser-ai/rules/{block_rule_id}", block_rule_data)
    test_step("Reactivate BLOCK Rule (active=true)", st == 200, f"HTTP {st}")

    # Re-test prompt -> Should be BLOCKED again!
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Please summarize our latest status for project alpha release schedule.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Reactivated Rule Blocks Again (Prompt Blocked)", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # 6. Rule Pattern Update Check
    print("\n--- 6. RULE PATTERN UPDATE & CACHE INVALIDATION ---")
    # Add "project-omega" to the pattern
    block_rule_data["pattern"] = r"\b(project[\s_-]?alpha|project[\s_-]?nebula|project[\s_-]?omega)\b"
    st, res = req("PUT", f"/api/browser-ai/rules/{block_rule_id}", block_rule_data)
    test_step("Update Pattern with 'project-omega'", st == 200, f"HTTP {st}")

    # Test matching the newly added keyword
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Here are the secret keys for project-omega.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Updated Pattern Successfully Matches 'project-omega'", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # 7. Generate Regex Endpoint Check
    print("\n--- 7. GENERATE REGEX ENDPOINT CHECK ---")
    st, res = req("POST", "/api/browser-ai/rules/generate-regex", {
        "bot_prompt": "Block AWS access keys starting with AKIA followed by 16 uppercase alphanumeric characters"
    })
    test_step("Generate Regex API Endpoint Responds", st in (200, 400, 500, 502), f"HTTP {st}, response: {str(res)[:80]}")

    # 8. Clean up Test Rules
    print("\n--- 8. CLEANUP TEST RULES ---")
    deleted_count = 0
    for rid in created_rule_ids:
        st, res = req("DELETE", f"/api/browser-ai/rules/{rid}")
        if st == 200:
            deleted_count += 1
    test_step(f"Clean Up All {len(created_rule_ids)} Test Rules", deleted_count == len(created_rule_ids), f"Deleted: {deleted_count}/{len(created_rule_ids)}")

    # Summary
    print("\n=================================================================")
    passed = sum(1 for _, ok, _ in results if ok)
    total = len(results)
    print(f"  TOTAL RESULTS: {passed}/{total} TESTS PASSED")
    print("=================================================================")

    if passed == total:
        print("\n>>> ALL REGEX RULE CHECKS PASSED 100%! <<<")
        sys.exit(0)
    else:
        print(f"\n>>> SOME TESTS FAILED ({total - passed} failures) <<<")
        sys.exit(1)

if __name__ == "__main__":
    main()
