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
    print("  LIVE LAPTOP INSTALL, UNINSTALL, KEY ROTATION & ERROR HANDLING  ")
    print("=================================================================\n")

    # 1. Admin Login
    print("--- 1. ADMIN AUTHENTICATION ---")
    st, res = req("POST", "/api/session/login", {"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    test_step("Admin Login", st == 200, f"HTTP {st}")

    # 2. Company Common Key Configuration
    print("\n--- 2. COMPANY COMMON UNINSTALL KEY CONFIGURATION ---")
    company_key = "CORP-COMMON-UNINSTALL-KEY-TEST-2026!"
    st, res = req("PUT", "/api/browser-ai/agents/uninstall-key", {
        "key": company_key,
        "updated_by": "admin@yespanchi.com"
    })
    test_step("Set Company Common Uninstall Key", st == 200 and res.get("status") == "success", f"HTTP {st}, key set")

    st, res = req("GET", "/api/browser-ai/agents/uninstall-key")
    test_step("Retrieve Company Common Key (Admin)", st == 200 and res.get("uninstall_key") == company_key, f"Verified key: {res.get('uninstall_key')}")

    # 3. Live Laptop Agent Registration / Install (Heartbeat)
    print("\n--- 3. LIVE LAPTOP AGENT INSTALLATION / REGISTRATION ---")
    agent_id = "test-live-laptop-agent-999"
    heartbeat_payload = {
        "id": agent_id,
        "hostname": "LAPTOP-QC-TEST-01",
        "username": "tester",
        "contact_email": "tester@testcorp.com",
        "ip_address": "192.168.1.155",
        "mac_address": "AA:BB:CC:DD:EE:01",
        "os_version": "Windows 11 Pro 23H2",
        "agent_version": "1.2.0",
        "agent_type": "laptop",
        "health_status": "healthy",
        "health_detail": "All services active"
    }
    st, res = req("POST", "/api/browser-ai/agents/heartbeat", heartbeat_payload, {"X-Gateway-Guard-Key": GUARD_SECRET})
    registered_agent = res.get("agent", {}) if isinstance(res, dict) else {}
    test_step("Register Laptop Agent via Heartbeat", st == 200 and registered_agent.get("id") == agent_id, f"Agent ID: {registered_agent.get('id')}, Status: {registered_agent.get('status')}")
    test_step("Auto-generated Uninstall Key Flag", registered_agent.get("has_uninstall_key") == True, "has_uninstall_key: true")

    # 4. Per-Device Uninstall Key Retrieval
    print("\n--- 4. PER-DEVICE UNINSTALL KEY RETRIEVAL ---")
    st, res = req("GET", f"/api/browser-ai/agents/{agent_id}/uninstall-key")
    per_device_key = res.get("uninstall_key", "") if isinstance(res, dict) else ""
    rotated_at_orig = res.get("uninstall_key_rotated_at", "") if isinstance(res, dict) else ""
    test_step("Retrieve Per-Device Key", st == 200 and len(per_device_key) >= 8, f"Key: {per_device_key[:4]}*****, RotatedAt: {rotated_at_orig}")

    # 5. Verify Uninstall Key using Per-Device Key
    print("\n--- 5. UNINSTALL KEY VERIFICATION (PER-DEVICE KEY) ---")
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {
        "agent_id": agent_id,
        "key": per_device_key
    })
    test_step("Verify with Per-Device Key", st == 200 and res.get("valid") == True, f"valid: {res.get('valid')}")

    # 6. Verify Uninstall Key using Company Common Key
    print("\n--- 6. UNINSTALL KEY VERIFICATION (COMPANY COMMON KEY FALLBACK) ---")
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {
        "agent_id": agent_id,
        "key": company_key
    })
    test_step("Verify with Company Common Key", st == 200 and res.get("valid") == True, f"valid: {res.get('valid')} (Common key fallback works)")

    # 7. Key Rotation (eotytre / rotate)
    print("\n--- 7. PER-DEVICE KEY ROTATION (ROTATE / EOTYTRE) ---")
    st, res = req("POST", f"/api/browser-ai/agents/{agent_id}/uninstall-key/rotate")
    new_per_device_key = res.get("uninstall_key", "") if isinstance(res, dict) else ""
    rotated_at_new = res.get("uninstall_key_rotated_at", "") if isinstance(res, dict) else ""
    key_changed = (new_per_device_key != per_device_key) and (len(new_per_device_key) >= 8)
    test_step("Rotate Per-Device Uninstall Key", st == 200 and key_changed, f"New Key: {new_per_device_key[:4]}*****, RotatedAt: {rotated_at_new}")

    # 8. Verify Old Key Invalidation & New Key Acceptance
    print("\n--- 8. ROTATED KEY VALIDATION ---")
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {
        "agent_id": agent_id,
        "key": per_device_key # OLD key
    })
    test_step("Old Key Invalidation Check", st == 200 and res.get("valid") == False, f"Old key valid: {res.get('valid')} (Properly rejected!)")

    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {
        "agent_id": agent_id,
        "key": new_per_device_key # NEW rotated key
    })
    test_step("New Rotated Key Acceptance", st == 200 and res.get("valid") == True, f"New key valid: {res.get('valid')} (Successfully accepted!)")

    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {
        "agent_id": agent_id,
        "key": company_key # Company key should STILL work
    })
    test_step("Company Common Key Still Valid after Rotation", st == 200 and res.get("valid") == True, f"Company key valid: {res.get('valid')}")

    # 9. Remote Uninstall Workflow
    print("\n--- 9. REMOTE UNINSTALL WORKFLOW ---")
    # Admin triggers remote uninstall
    st, res = req("POST", f"/api/browser-ai/agents/{agent_id}/remote-uninstall", {
        "key": new_per_device_key
    })
    test_step("Admin Trigger Remote Uninstall", st == 200 and res.get("command") == "uninstall", f"HTTP {st}, command: uninstall")

    # Agent checks uninstall status
    st, res = req("POST", "/api/browser-ai/agents/uninstall-status", {"agent_id": agent_id}, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Agent Checks Remote Uninstall Status", st == 200 and res.get("authorized") == True, f"authorized: {res.get('authorized')}")

    # Agent acknowledges uninstall
    st, res = req("POST", "/api/browser-ai/agents/uninstall-ack", {"agent_id": agent_id}, {"X-Gateway-Guard-Key": GUARD_SECRET})
    test_step("Agent Acknowledges Remote Uninstall (uninstall-ack)", st == 200 and res.get("status") == "success", f"HTTP {st}")

    # Check agent status is uninstalled
    st, res = req("GET", f"/api/browser-ai/agents?search={agent_id}")
    agents = res.get("agents", []) if isinstance(res, dict) else []
    found_agent = next((a for a in agents if a.get("id") == agent_id), {})
    test_step("Agent Status is 'uninstalled'", found_agent.get("status") == "uninstalled", f"Status: {found_agent.get('status')}")

    # 10. Allow Reinstall Workflow
    print("\n--- 10. ALLOW REINSTALL WORKFLOW ---")
    st, res = req("POST", f"/api/browser-ai/agents/{agent_id}/allow-reinstall")
    test_step("Admin Allows Reinstall", st == 200 and res.get("status") == "success", f"HTTP {st}")

    # Reinstall / Heartbeat check-in
    st, res = req("POST", "/api/browser-ai/agents/heartbeat", heartbeat_payload, {"X-Gateway-Guard-Key": GUARD_SECRET})
    reinstalled_agent = res.get("agent", {}) if isinstance(res, dict) else {}
    test_step("Reinstall Agent via Heartbeat", st == 200 and reinstalled_agent.get("status") == "active", f"Status restored to: {reinstalled_agent.get('status')}")

    # 11. Error Handling & Security Checks
    print("\n--- 11. ERROR HANDLING & SECURITY CHECKS ---")
    # Empty key error
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {"agent_id": agent_id, "key": ""})
    test_step("Empty Key Handling", st == 200 and res.get("valid") == False, f"valid: {res.get('valid')}")

    # Wrong / Invalid key error
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", {"agent_id": agent_id, "key": "TOTALLY-WRONG-KEY-XYZ"})
    test_step("Invalid Key Handling", st == 200 and res.get("valid") == False, f"valid: {res.get('valid')}")

    # Missing payload
    st, res = req("POST", "/api/browser-ai/agents/uninstall-verify", "not-a-json")
    test_step("Invalid JSON Payload Error Handling", st == 400, f"HTTP {st} (Expected 400 Bad Request)")

    # Direct unauthorized uninstall attempt
    st, res = req("POST", "/api/browser-ai/agents/uninstall", {"agent_id": agent_id, "key": "BOGUS-ATTEMPT-KEY"})
    test_step("Unauthorized Direct Uninstall Rejection", st == 403, f"HTTP {st} (Expected 403 Forbidden)")

    # Tamper detection verification: check agent health status
    st, res = req("GET", f"/api/browser-ai/agents?search={agent_id}")
    agents = res.get("agents", []) if isinstance(res, dict) else []
    found_agent = next((a for a in agents if a.get("id") == agent_id), {})
    test_step("Tamper Event Recorded on Agent", found_agent.get("health_status") == "tampered" or "tamper" in str(found_agent.get("health_detail", "")).lower(), f"Health: {found_agent.get('health_status')}, Detail: {found_agent.get('health_detail')}")

    # Rate limiting / Lockout protection (5 consecutive failed attempts)
    print("\n--- 12. RATE LIMITING / BRUTE-FORCE LOCKOUT PROTECTION ---")
    lockout_agent_id = "test-lockout-probe"
    # Send remaining failed attempts to reach maxUninstallAttempts (5)
    last_status = 0
    last_body = {}
    for i in range(5):
        last_status, last_body = req("POST", "/api/browser-ai/agents/uninstall", {"agent_id": lockout_agent_id, "key": f"GUESS_FAIL_{i}"})
        print(f"    Attempt {i+1} -> HTTP {last_status}")

    test_step("Brute-Force Defense Lockout (HTTP 429)", last_status == 429, f"HTTP {last_status}: {last_body}")

    # 13. Clean up Test Agents
    print("\n--- 13. CLEANUP TEST AGENTS ---")
    st, res = req("POST", "/api/browser-ai/agents/bulk-delete", {"ids": [agent_id, lockout_agent_id]})
    test_step("Clean Up Test Agents via Bulk Delete", st == 200, f"HTTP {st}, deleted: {res.get('deleted')}")

    # Summary
    print("\n=================================================================")
    passed = sum(1 for _, ok, _ in results if ok)
    total = len(results)
    print(f"  TOTAL RESULTS: {passed}/{total} TESTS PASSED")
    print("=================================================================")

    if passed == total:
        print("\n>>> ALL LIVE INSTALL, UNINSTALL, KEY ROTATION & ERROR HANDLING TESTS PASSED! <<<")
        sys.exit(0)
    else:
        print(f"\n>>> SOME TESTS FAILED ({total - passed} failures) <<<")
        sys.exit(1)

if __name__ == "__main__":
    main()
