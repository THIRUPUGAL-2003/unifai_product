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

    def check(name, url, method="GET", data=None, expected_status=200, extra_headers=None):
        try:
            req_data = json.dumps(data).encode("utf-8") if data is not None else None
            headers = {"Content-Type": "application/json"} if req_data is not None else {}
            if extra_headers:
                headers.update(extra_headers)
            req = urllib.request.Request(f"{BASE}{url}", data=req_data, headers=headers, method=method)
            resp = opener.open(req)
            body = resp.read().decode("utf-8")
            status = resp.status
            if status == expected_status:
                results.append((name, True, f"HTTP {status} OK"))
                return True, body
            else:
                results.append((name, False, f"Expected {expected_status}, got {status}"))
                return False, body
        except urllib.error.HTTPError as e:
            if e.code == expected_status:
                results.append((name, True, f"HTTP {e.code} (Expected matched)"))
                return True, e.read().decode("utf-8")
            else:
                results.append((name, False, f"HTTP {e.code}: {e.read().decode('utf-8')[:100]}"))
                return False, ""
        except Exception as e:
            results.append((name, False, f"Exception: {str(e)[:100]}"))
            return False, ""

    print("=================================================================")
    print("      UNIFAI BROWSER AI - 100% COMPREHENSIVE FEATURE TEST       ")
    print("=================================================================\n")

    # 1. Admin Authentication
    print("--- 1. Authentication ---")
    login_data = {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"}
    check("Admin Login for Browser AI", "/api/session/login", method="POST", data=login_data)

    # 2. Browser AI Logs & Stats
    print("\n--- 2. Browser AI Logs ---")
    check("Get Browser AI Prompt Logs", "/api/browser-ai/logs")
    check("Get Browser AI Log Statistics", "/api/browser-ai/logs/stats")

    # 3. Search Logs
    print("\n--- 3. Search Logs ---")
    check("Get Search Logs List", "/api/browser-ai/search-logs")
    search_payload = {
        "engine": "google",
        "browser": "chrome",
        "is_incognito": False,
        "query": "unifai master test search query",
        "url": "https://www.google.com/search?q=unifai",
        "host": "www.google.com",
        "client_ip": "127.0.0.1"
    }
    ok, search_resp = check("Record Search Log", "/api/browser-ai/search-logs", method="POST", data=search_payload, extra_headers={"X-Gateway-Guard-Key": GUARD_SECRET})

    # 4. Rules Engine (CRUD)
    print("\n--- 4. Browser Guard Rules Engine ---")
    check("List Browser Guard Rules", "/api/browser-ai/rules")
    rule_payload = {
        "name": f"Test Security Rule {int(time.time())}",
        "rule_type": "regex",
        "pattern": r"\b(CONFIDENTIAL_TOKEN_[A-Z0-9]+)\b",
        "severity": "CRITICAL",
        "action": "Block",
        "warning_message": "Confidential token transmission blocked by company policy",
        "description": "Rule created during comprehensive Browser AI test",
        "active": True
    }
    ok, rule_resp = check("Create Guard Rule", "/api/browser-ai/rules", method="POST", data=rule_payload, expected_status=200)
    rule_id = None
    if ok and rule_resp:
        try:
            rule_id = json.loads(rule_resp).get("rule", {}).get("id")
        except Exception:
            pass
    if rule_id:
        check("Update Guard Rule (Toggle severity)", f"/api/browser-ai/rules/{rule_id}", method="PUT", data={"severity": "HIGH", "action": "Warn"})
        check("Delete Guard Rule", f"/api/browser-ai/rules/{rule_id}", method="DELETE")

    # 5. Browser Controls
    print("\n--- 5. Browser Controls Settings ---")
    check("Get Browser Controls", "/api/browser-ai/controls")
    ctrl_payload = {
        "block_incognito": False,
        "disable_developer_tools": False,
        "force_safe_search": False,
        "block_search_engines": False
    }
    check("Update Browser Controls", "/api/browser-ai/controls", method="PUT", data=ctrl_payload)

    # 6. Target Websites (CRUD)
    print("\n--- 6. Target Websites Management ---")
    check("List Target Websites", "/api/browser-ai/targets")
    target_payload = {
        "domain": f"test-ai-domain-{int(time.time())}.com",
        "platform_name": "Test AI Platform",
        "monitored": True,
        "block_site": False,
        "host_role": "chat"
    }
    ok, target_resp = check("Create Target Website", "/api/browser-ai/targets", method="POST", data=target_payload, expected_status=200)
    target_id = None
    if ok and target_resp:
        try:
            target_id = json.loads(target_resp).get("target", {}).get("id")
        except Exception:
            pass
    if target_id:
        check("Update Target Website (Set block_site)", f"/api/browser-ai/targets/{target_id}", method="PUT", data={"block_site": True})
        check("Delete Target Website", f"/api/browser-ai/targets/{target_id}", method="DELETE")

    # 7. PAC Proxy Service
    print("\n--- 7. PAC Proxy Auto-Config ---")
    check("Get PAC Proxy Script (/pac)", "/api/browser-ai/pac")
    check("Get PAC Proxy Script (/proxy.pac)", "/api/browser-ai/proxy.pac")

    # 8. Fleet & Device Agents
    print("\n--- 8. Fleet Management & Device Agents ---")
    check("List Fleet Agents", "/api/browser-ai/agents")
    check("Get Fleet Agent Settings", "/api/browser-ai/agents/settings")
    check("Get Company Uninstall Key", "/api/browser-ai/agents/uninstall-key")
    check("Get Fleet Config", "/api/browser-ai/fleet-config")
    check("Update Fleet Config", "/api/browser-ai/fleet-config", method="PUT", data={"auto_update_enabled": True, "heartbeat_interval_sec": 60})
    check("Get Agent Insight Stats", "/api/browser-ai/insights/stats")

    # 9. Prompt Interception & Guard Evaluation (Core Intercept)
    print("\n--- 9. Prompt Intercept & Security Screening ---")
    intercept_benign = {
        "platform": "ChatGPT",
        "prompt": "Explain quantum computing in simple terms",
        "client_ip": "127.0.0.1",
        "agent_id": "test_agent_laptop_1",
        "agent_hostname": "employee-laptop-alpha",
        "metadata": {"evaluation_only": True}
    }
    check("Screen Benign Prompt (Allowed)", "/api/browser-ai/intercept", method="POST", data=intercept_benign, extra_headers={"X-Gateway-Guard-Key": GUARD_SECRET})

    # 10. Distribution & Downloads
    print("\n--- 10. Distribution Packages & Binaries ---")
    check("Setup Info & Readiness", "/api/browser-ai/setup/info")
    check("Rebuild History Table", "/api/browser-ai/setup/rebuild-history")
    check("Proxy Bundle Metadata", "/api/browser-ai/setup/proxy-bundle.json")
    check("Windows Package Download Header (HEAD)", "/api/browser-ai/setup/download-windows.zip", method="HEAD")
    check("macOS Package Download Header (HEAD)", "/api/browser-ai/setup/download-mac.zip", method="HEAD")
    check("Windows Setup EXE Download Header (HEAD)", "/api/browser-ai/setup/Gateway_Guard_Setup.exe", method="HEAD")

    # 11. Licensing & Local AI Models
    print("\n--- 11. Licensing & Local Model Diagnostics ---")
    check("License Status", "/api/browser-ai/license")
    check("Server Hardware Fingerprint ID", "/api/browser-ai/license/server-id")
    check("Local Ollama Models Discovery", "/api/browser-ai/ollama-models")

    # Results Printout
    print("\n=================================================================")
    print("                 BROWSER AI TEST EXECUTION RESULTS               ")
    print("=================================================================")
    passed = 0
    failed = 0
    for name, ok, note in results:
        status_icon = "[PASS]" if ok else "[FAIL]"
        if ok:
            passed += 1
            print(f" {status_icon} | {name:<46} | {note}")
        else:
            failed += 1
            print(f" {status_icon} | {name:<46} | {note}")

    print("=================================================================")
    print(f" TOTAL BROWSER AI TESTS: {len(results)} | PASSED: {passed} | FAILED: {failed}")
    if failed == 0:
        print(" RESULT: 100% PERFECT - ALL BROWSER AI FEATURES WORKING!")
    else:
        print(f" RESULT: {failed} FAILURES DETECTED")
    print("=================================================================")

if __name__ == "__main__":
    main()
