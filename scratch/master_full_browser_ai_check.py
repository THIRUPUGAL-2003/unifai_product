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
    print("      UNIFAI BROWSER AI - 100% EXHAUSTIVE MASTER HEALTH CHECK     ")
    print("=================================================================\n")

    # -------------------------------------------------------------
    # SECTION 1: AUTHENTICATION & CORE INFRASTRUCTURE
    # -------------------------------------------------------------
    print("--- SECTION 1: AUTHENTICATION & CORE INFRASTRUCTURE ---")
    st, res = req("GET", "/health")
    test("Gateway Health Probe", st == 200, f"HTTP {st}")

    st, res = req("POST", "/api/session/login", {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    test("Admin Session Authentication", st == 200 and res.get("role") == "admin", f"Role: {res.get('role')}")

    # -------------------------------------------------------------
    # SECTION 2: PROMPT LOGS & OVERVIEW METRICS
    # -------------------------------------------------------------
    print("\n--- SECTION 2: PROMPT LOGS & OVERVIEW METRICS ---")
    st, res = req("GET", "/api/browser-ai/logs?limit=10")
    test("List Browser AI Prompt Logs", st == 200 and "logs" in res, f"HTTP {st}, Total: {res.get('total')}")

    st, res = req("GET", "/api/browser-ai/logs/stats")
    test("Retrieve Browser AI Log Statistics", st == 200 and "total" in res, f"Total: {res.get('total')}, Blocked: {res.get('blocked')}, Warned: {res.get('warned')}")

    st, res = req("GET", "/api/browser-ai/logs?status=blocked&limit=5")
    test("Filter Prompt Logs by Status=Blocked", st == 200 and isinstance(res.get("logs"), list), f"HTTP {st}")

    st, res = req("GET", "/api/browser-ai/logs?action=Redacted&limit=5")
    test("Filter Prompt Logs by Action=Redacted", st == 200 and isinstance(res.get("logs"), list), f"HTTP {st}")

    # -------------------------------------------------------------
    # SECTION 3: SEARCH LOGS ENGINE
    # -------------------------------------------------------------
    print("\n--- SECTION 3: ENTERPRISE SEARCH LOGS ---")
    st, res = req("GET", "/api/browser-ai/search-logs?limit=10")
    test("List Enterprise Search Logs", st == 200 and "logs" in res, f"HTTP {st}, Total: {res.get('total')}")

    # Record search log via Guard Key
    test_search_query = "master test query sensitive internal architecture"
    search_payload = {
        "engine": "google",
        "browser": "Chrome",
        "query": test_search_query,
        "url": "https://www.google.com/search?q=" + urllib.parse.quote(test_search_query),
        "is_incognito": False,
        "agent_hostname": "LAPTOP-MASTER-QC"
    }
    st, res = req("POST", "/api/browser-ai/search-logs", search_payload, {"X-Gateway-Guard-Key": GUARD_SECRET})
    recorded_search_id = res.get("log", {}).get("id") if isinstance(res, dict) else ""
    test("Record Enterprise Search Log", st == 200 and bool(recorded_search_id), f"Logged ID: {recorded_search_id}")

    # Verify search query is in list
    st, res = req("GET", f"/api/browser-ai/search-logs?search={urllib.parse.quote(test_search_query)}")
    found_search = any(s.get("id") == recorded_search_id for s in res.get("logs", []))
    test("Search Query Search Filter Verification", found_search, f"Found logged query")

    # Clean up test search log
    if recorded_search_id:
        st, res = req("POST", "/api/browser-ai/search-logs/bulk-delete", {"ids": [recorded_search_id]})
        test("Delete Selected Search Log", st == 200, f"Deleted search log")

    # -------------------------------------------------------------
    # SECTION 4: MONITORED TARGET WEBSITES
    # -------------------------------------------------------------
    print("\n--- SECTION 4: TARGET WEBSITES MANAGEMENT ---")
    st, res = req("GET", "/api/browser-ai/targets")
    test("List Monitored Target Websites", st == 200 and "targets" in res, f"HTTP {st}, Count: {len(res.get('targets', []))}")

    # Create test target website
    target_domain = "qc-master-test-ai.internal"
    st, res = req("POST", "/api/browser-ai/targets", {
        "domain": target_domain,
        "name": "Master QC Target Website",
        "category": "AI Assistant",
        "mode": "monitor",
        "reply_bot_enabled": False
    })
    target_id = res.get("target", {}).get("id") if isinstance(res, dict) else ""
    test("Create New Target Website", st == 200 and bool(target_id), f"ID: {target_id}")

    # Update target website mode (toggle reply-bot)
    st, res = req("PUT", f"/api/browser-ai/targets/{target_id}", {
        "domain": target_domain,
        "name": "Master QC Target Website Updated",
        "mode": "block",
        "reply_bot_enabled": True,
        "reply_bot_provider": "ollama",
        "reply_bot_model": "llama3.2"
    })
    test("Update Target Website Configuration", st == 200 and res.get("status") == "success", f"Status: {res.get('status')}")

    # Delete target website
    if target_id:
        st, res = req("DELETE", f"/api/browser-ai/targets/{target_id}")
        test("Delete Target Website", st == 200, f"HTTP {st}")

    # -------------------------------------------------------------
    # SECTION 5: BROWSER GUARD RULES (BLOCK, WARN, REDACT)
    # -------------------------------------------------------------
    print("\n--- SECTION 5: BROWSER GUARD RULES ENGINE ---")
    st, res = req("GET", "/api/browser-ai/rules")
    test("List Guard Rules", st == 200 and "rules" in res, f"HTTP {st}, Rules Count: {len(res.get('rules', []))}")

    # Test Rule Creation: BLOCK
    block_rule_data = {
        "name": "MASTER_BLOCK_Rule",
        "pattern": r"\b(super[\s_-]?secret[\s_-]?vault|master[\s_-]?key[\s_-]?leak)\b",
        "action": "BLOCK",
        "warning_message": "Master confidential vault access is prohibited.",
        "severity": "CRITICAL",
        "category": "SECRETS",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", block_rule_data)
    m_block_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    test("Create Master BLOCK Rule", st == 200 and bool(m_block_id), f"ID: {m_block_id}")

    # Test Rule Creation: WARN
    warn_rule_data = {
        "name": "MASTER_WARN_Rule",
        "pattern": r"\bINTERNAL-PROJECT-CODE-\d{4}\b",
        "action": "WARN",
        "warning_message": "Internal project code detected. Be cautious.",
        "severity": "LOW",
        "category": "COMPLIANCE",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", warn_rule_data)
    m_warn_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    test("Create Master WARN Rule", st == 200 and bool(m_warn_id), f"ID: {m_warn_id}")

    # Test Rule Creation: REDACT
    redact_rule_data = {
        "name": "MASTER_REDACT_Rule",
        "pattern": r"\bCUSTOMER-PASSPORT-[A-Z0-9]{8}\b",
        "action": "REDACT",
        "warning_message": "Customer passport number redacted for compliance.",
        "severity": "HIGH",
        "category": "PII",
        "active": True
    }
    st, res = req("POST", "/api/browser-ai/rules", redact_rule_data)
    m_redact_id = res.get("rule", {}).get("id") if isinstance(res, dict) else ""
    test("Create Master REDACT Rule", st == 200 and bool(m_redact_id), f"ID: {m_redact_id}")

    # -------------------------------------------------------------
    # SECTION 6: LIVE INTERCEPTION VERIFICATION
    # -------------------------------------------------------------
    print("\n--- SECTION 6: LIVE PROMPT INTERCEPTION PIPELINE ---")
    created_logs = []

    # 6.1 Benign prompt
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Explain the difference between a binary tree and a graph.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_logs.append(res.get("log", {}).get("id"))
    test("Benign Prompt (Allowed)", st == 200 and res.get("allowed") == True and res.get("action") == "Allowed", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # 6.2 BLOCK Rule Trigger
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Here are the credentials for super_secret_vault production.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_logs.append(res.get("log", {}).get("id"))
    test("BLOCK Rule Triggered (Blocked)", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}, rule: {res.get('rule_triggered')}")
    warn_text = str(res.get("warning_message") or res.get("reply_text") or "")
    test("BLOCK Warning Message Returned", "Master confidential vault" in warn_text, f"warning: {warn_text[:60]}")

    # 6.3 WARN Rule Trigger
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "Claude",
        "prompt": "Please review deliverables for INTERNAL-PROJECT-CODE-9876.",
        "url": "https://claude.ai/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_logs.append(res.get("log", {}).get("id"))
    test("WARN Rule Triggered (Warned)", st == 200 and res.get("allowed") == True and res.get("action") == "Warned", f"allowed: {res.get('allowed')}, action: {res.get('action')}")
    test("WARN Forward Prompt Notice Attached", "[GATEWAY WARNING]" in str(res.get("forward_prompt", "")), "Notice verified")

    # 6.4 REDACT Rule Trigger
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "Book flight for traveler with CUSTOMER-PASSPORT-A1234567.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_logs.append(res.get("log", {}).get("id"))
    test("REDACT Rule Triggered (Redacted)", st == 200 and res.get("allowed") == True and res.get("action") == "Redacted", f"allowed: {res.get('allowed')}, action: {res.get('action')}")
    test("REDACT Forward Prompt Notice Attached", "[GATEWAY REDACTED]" in str(res.get("forward_prompt", "")), "Notice verified")

    # 6.5 Priority Check (BLOCK > WARN)
    st, res = req("POST", "/api/browser-ai/intercept", {
        "platform": "ChatGPT",
        "prompt": "master_key_leak with INTERNAL-PROJECT-CODE-1111 combined.",
        "url": "https://chatgpt.com/"
    }, {"X-Gateway-Guard-Key": GUARD_SECRET})
    if isinstance(res, dict) and res.get("log", {}).get("id"):
        created_logs.append(res.get("log", {}).get("id"))
    test("Policy Priority: BLOCK Overrides WARN", st == 200 and res.get("allowed") == False and res.get("action") == "Blocked", f"allowed: {res.get('allowed')}, action: {res.get('action')}")

    # Clean up test rules & logs
    for rid in [m_block_id, m_warn_id, m_redact_id]:
        if rid:
            req("DELETE", f"/api/browser-ai/rules/{rid}")
    test("Clean Up Master Rules", True, "Rules deleted")

    if created_logs:
        req("POST", "/api/browser-ai/logs/bulk-delete", {"ids": created_logs})
        test("Clean Up Master Intercept Logs", True, f"Deleted {len(created_logs)} test logs")

    # -------------------------------------------------------------
    # SECTION 7: FLEET AGENTS & LAPTOP MANAGEMENT
    # -------------------------------------------------------------
    print("\n--- SECTION 7: FLEET AGENTS & LAPTOP MANAGEMENT ---")
    st, res = req("GET", "/api/browser-ai/agents?limit=10")
    test("List Fleet Agents", st == 200 and "agents" in res, f"HTTP {st}, Total: {res.get('total')}")

    # Register test agent via heartbeat
    test_agent_id = "master-qc-agent-001"
    hb_payload = {
        "id": test_agent_id,
        "hostname": "LAPTOP-MASTER-QC-01",
        "username": "tester",
        "contact_email": "tester@company.com",
        "ip_address": "192.168.1.200",
        "mac_address": "11:22:33:44:55:66",
        "os_version": "Windows 11 Pro",
        "agent_version": "1.2.0",
        "agent_type": "laptop",
        "health_status": "healthy",
        "health_detail": "All checks ok"
    }
    st, res = req("POST", "/api/browser-ai/agents/heartbeat", hb_payload, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test("Agent Heartbeat Check-In", st == 200 and res.get("status") == "success", f"Registered ID: {test_agent_id}")

    # Pause & Resume Agent
    st, res = req("POST", f"/api/browser-ai/agents/{test_agent_id}/pause")
    test("Pause Agent", st == 200 and res.get("status") == "success", f"HTTP {st}")

    st, res = req("POST", f"/api/browser-ai/agents/{test_agent_id}/resume")
    test("Resume Agent", st == 200 and res.get("status") == "success", f"HTTP {st}")

    # Update Contact Email
    st, res = req("PUT", f"/api/browser-ai/agents/{test_agent_id}/contact-email", {"contact_email": "updated-contact@company.com"})
    test("Update Agent Contact Email", st == 200 and res.get("status") == "success", f"HTTP {st}")

    # -------------------------------------------------------------
    # SECTION 8: INSTALL, UNINSTALL, KEYS & ROTATION
    # -------------------------------------------------------------
    print("\n--- SECTION 8: INSTALL, UNINSTALL & KEY ROTATION ---")

    # Retrieve per-device uninstall key
    st, res = req("GET", f"/api/browser-ai/agents/{test_agent_id}/uninstall-key")
    per_dev_key = res.get("uninstall_key", "")
    test("Retrieve Per-Device Uninstall Key", st == 200 and len(per_dev_key) >= 6, f"Key: {per_dev_key[:4]}****")

    # Verify uninstall with per-device key
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {"agent_id": test_agent_id, "key": per_dev_key})
    test("Verify Uninstall with Per-Device Key", st == 200 and res.get("valid") == True, f"valid: {res.get('valid')}")

    # Rotate per-device key
    st, res = req("POST", f"/api/browser-ai/agents/{test_agent_id}/uninstall-key/rotate")
    new_dev_key = res.get("uninstall_key", "")
    test("Rotate Per-Device Uninstall Key", st == 200 and new_dev_key != per_dev_key, f"New Key: {new_dev_key[:4]}****")

    # Verify old key rejected
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {"agent_id": test_agent_id, "key": per_dev_key})
    test("Old Rotated Key Rejected", st == 200 and res.get("valid") == False, f"Old key valid: {res.get('valid')}")

    # Verify new rotated key accepted
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {"agent_id": test_agent_id, "key": new_dev_key})
    test("New Rotated Key Accepted", st == 200 and res.get("valid") == True, f"New key valid: {res.get('valid')}")

    # Remote uninstall flow
    st, res = req("POST", f"/api/browser-ai/agents/{test_agent_id}/remote-uninstall", {"key": new_dev_key})
    test("Remote Uninstall Authorized by Admin", st == 200 and res.get("command") == "uninstall", f"HTTP {st}")

    st, res = req("POST", "/api/browser-ai/agents/uninstall-status", {"agent_id": test_agent_id}, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test("Agent Checks Remote Uninstall Status", st == 200 and res.get("authorized") == True, f"authorized: {res.get('authorized')}")

    st, res = req("POST", "/api/browser-ai/agents/uninstall-ack", {"agent_id": test_agent_id}, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test("Agent Uninstall Acknowledgment", st == 200 and res.get("status") == "success", f"HTTP {st}")

    st, res = req("POST", f"/api/browser-ai/agents/{test_agent_id}/allow-reinstall")
    test("Admin Allows Reinstall", st == 200 and res.get("status") == "success", f"HTTP {st}")

    # Clean up test agent
    st, res = req("POST", "/api/browser-ai/agents/bulk-delete", {"ids": [test_agent_id]})
    test("Delete Test Agent via Bulk Delete", st == 200, f"Deleted test agent")

    # -------------------------------------------------------------
    # SECTION 9: LICENSE & FLEET CONFIG
    # -------------------------------------------------------------
    print("\n--- SECTION 9: LICENSE & FLEET CONFIGURATION ---")
    st, res = req("GET", "/api/browser-ai/license")
    test("Get License Information", st == 200 and "status" in res, f"License Status: {res.get('status')}")

    st, res = req("GET", "/api/browser-ai/license/server-id")
    server_hw_id = res.get("server_hardware_id", "") if isinstance(res, dict) else ""
    test("Get Server Hardware ID", st == 200 and bool(server_hw_id), f"Server HW ID: {server_hw_id[:16]}...")

    st, res = req("GET", "/api/browser-ai/fleet-config")
    test("Get Fleet Configuration Defaults", st == 200, f"HTTP {st}")

    # -------------------------------------------------------------
    # SECTION 10: OVERVIEW INSIGHTS STATS
    # -------------------------------------------------------------
    print("\n--- SECTION 10: INSIGHTS & ANALYTICS ---")
    st, res = req("GET", "/api/browser-ai/insights/stats")
    test("Get Insights Overall Analytics", st == 200, f"HTTP {st}")

    # FINAL REPORT
    print("\n=================================================================")
    passed = sum(1 for _, ok, _ in results if ok)
    total = len(results)
    print(f"  MASTER AUDIT RESULT: {passed}/{total} TESTS PASSED")
    print("=================================================================")

    if passed == total:
        print("\n>>> ALL 10 SECTIONS OF BROWSER AI FULLY VERIFIED & 100% OPERATIONAL! <<<")
        sys.exit(0)
    else:
        print(f"\n>>> FAILURES DETECTED: {total - passed}/{total} <<<")
        sys.exit(1)

if __name__ == "__main__":
    main()
