import urllib.request
import json
import http.cookiejar
import os
import sys

def main():
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    BASE = "http://localhost:8080"
    
    results = []

    def check(name, url, method="GET", data=None, expected_status=200):
        try:
            req_data = json.dumps(data).encode("utf-8") if data is not None else None
            headers = {"Content-Type": "application/json"} if req_data is not None else {}
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
                results.append((name, True, f"HTTP {e.code} (Expected error correctly returned)"))
                return True, e.read().decode("utf-8")
            else:
                results.append((name, False, f"HTTP {e.code}: {e.read().decode('utf-8')[:100]}"))
                return False, ""
        except Exception as e:
            results.append((name, False, f"Exception: {str(e)[:100]}"))
            return False, ""

    print("=================================================================")
    print("      UNIFAI GATEWAY - COMPLETE MASTER SYSTEM TEST SUITE         ")
    print("=================================================================\n")

    # SECTION 1: Public Endpoints & Auth Security Lifecycle
    print("--- 1. Auth & Security Lifecycle ---")
    check("System Version (Public)", "/api/version")
    check("Health Check (Public)", "/health")
    check("Password Policy (<8 chars rejection)", "/api/session/register", method="POST", data={"username": "short_pw", "email": "short@ex.com", "password": "123"}, expected_status=400)
    check("Admin Session Login", "/api/session/login", method="POST", data={"username": "admin@yespanchi.com", "password": "YP2025-2026yp"})
    check("Auth Status & Role Check", "/api/session/is-auth-enabled")
    check("Product User Quota", "/api/session/users/quota")
    check("List Users", "/api/session/users")

    # SECTION 2: Authenticated Health & System Configuration
    print("\n--- 2. Health & Config Endpoints ---")
    check("API Health Check (Admin)", "/api/health")
    check("System Info", "/api/system-info")
    check("Core Config", "/api/config")
    check("Caching Config", "/api/caching-config")
    check("Proxy Config", "/api/proxy-config")
    check("SMTP Config", "/api/smtp-config")
    check("Auth Config", "/api/auth-config")
    check("Vector Store Config", "/api/vector-store-config")
    check("Compatibility Config", "/api/compatibility-config")

    # SECTION 3: Feature Flags Engine
    print("\n--- 3. Feature Flags Engine ---")
    check("List Feature Flags", "/api/feature-flags")
    check("Create Feature Flag", "/api/feature-flags", method="POST", data={"id": "custom.master_audit_flag", "enabled": False, "description": "Master check flag"}, expected_status=201)
    check("Toggle Feature Flag", "/api/feature-flags/custom.master_audit_flag", method="PUT", data={"enabled": True})
    check("Delete Feature Flag", "/api/feature-flags/custom.master_audit_flag", method="DELETE")
    check("Feature Flag 404 on Missing", "/api/feature-flags/custom.master_audit_flag", method="DELETE", expected_status=404)

    # SECTION 4: Plugins Manager
    print("\n--- 4. Plugins Manager ---")
    check("List All Plugins", "/api/plugins")
    check("List Builtin Plugins", "/api/plugins/builtins")
    check("List Loaded Plugins", "/api/plugins/loaded")
    check("Create Custom Plugin", "/api/plugins", method="POST", data={"name": "test-master-audit-plugin", "path": "http://localhost:8080/mock", "priority": 10}, expected_status=201)
    check("Get Custom Plugin", "/api/plugins/test-master-audit-plugin")
    check("Delete Custom Plugin", "/api/plugins/test-master-audit-plugin", method="DELETE")

    # SECTION 5: Governance Hierarchy (Customers, Teams, VKs, Budgets)
    print("\n--- 5. Governance Hierarchy & Budgets ---")
    check("Governance Customers", "/api/governance/customers")
    check("Governance Teams", "/api/governance/teams")
    check("Governance Virtual Keys", "/api/governance/virtual-keys")
    check("Governance Billing Blocks", "/api/governance/virtual-keys/billing-blocks")
    check("Governance Budgets", "/api/governance/budgets")
    check("Governance Rate Limits", "/api/governance/rate-limits")

    # SECTION 6: Model Catalog, Providers & Routing Rules
    print("\n--- 6. Models, Providers & Routing Rules ---")
    check("OpenAI-Compatible Models List", "/v1/models")
    check("Governance Routing Rules", "/api/governance/routing-rules")
    check("Governance Model Configs", "/api/governance/model-configs")
    check("Governance Providers", "/api/governance/providers")

    # SECTION 7: MCP Catalog & Integration
    print("\n--- 7. MCP Catalog & Integration ---")
    check("MCP Clients List", "/api/mcp/clients")
    check("MCP Library Catalog", "/api/mcp/library")

    # SECTION 8: Prompt Repository & Chat History
    print("\n--- 8. Prompt Repository & Chat History ---")
    check("Prompt Folders", "/api/prompt-repo/folders")
    check("Prompt List", "/api/prompt-repo/prompts")
    check("Prompt History Settings", "/api/prompt-repo/history/settings")

    # SECTION 9: Audit Logs & Access Profiles
    print("\n--- 9. Audit Logs & Access Profiles ---")
    check("Audit Logs List", "/api/audit-logs")
    check("Audit Logs Settings", "/api/audit-logs/settings")
    check("Audit Logs Export", "/api/audit-logs/export")
    import time
    prof_name = f"Master Audit Profile {int(time.time())}"
    ok, prof_body = check("Create Access Profile", "/api/access-profiles", method="POST", data={"name": prof_name, "description": "Profile test", "rules": {}}, expected_status=201)
    if ok and prof_body:
        try:
            prof_id = json.loads(prof_body).get("id")
            if prof_id:
                check("Delete Access Profile", f"/api/access-profiles/{prof_id}", method="DELETE")
        except Exception:
            pass

    # SECTION 10: Browser Guard Fleet & Encrypted Assets
    print("\n--- 10. Browser Guard Fleet & Artifacts ---")
    check("Guard Setup Info", "/api/browser-ai/setup/info")
    check("Guard Rebuild History", "/api/browser-ai/setup/rebuild-history")
    check("Guard Proxy Bundle Info", "/api/browser-ai/proxy-bundle")

    # Check Physical Release Files on Disk
    bg_release = os.path.join("apps", "browser-guard", "release")
    bg_proxy = os.path.join("apps", "browser-guard", "proxy")
    files_to_check = [
        (os.path.join(bg_release, "Gateway_Guard_Setup.exe"), 10_000_000, "Windows Setup Installer (.exe)"),
        (os.path.join(bg_release, "Gateway_Guard.exe"), 10_000_000, "Windows Portable Agent (.exe)"),
        (os.path.join(bg_release, "Gateway_Guard_Windows.zip"), 10_000_000, "Windows Full Package (.zip)"),
        (os.path.join(bg_release, "Gateway_Guard_macOS.zip"), 10_000_000, "macOS Employee Package (.zip)"),
        (os.path.join(bg_release, "gateway_guard_config.json"), 100, "Server Guard Config JSON"),
        (os.path.join(bg_release, "VERSION.txt"), 1, "Guard Release Version File"),
        (os.path.join(bg_proxy, "gateway_guard_code.enc"), 50_000, "Encrypted Guard Code Bundle (AES-256-GCM)"),
        (os.path.join(bg_proxy, "gateway_proxy_parts", "gateway_proxy_parts.enc"), 100_000, "Encrypted Proxy Parts Bundle (AES-256-GCM)"),
    ]

    for path, min_size, label in files_to_check:
        if os.path.isfile(path) and os.path.getsize(path) >= min_size:
            size_mb = os.path.getsize(path) / (1024 * 1024)
            size_str = f"{size_mb:.2f} MB" if size_mb >= 1 else f"{os.path.getsize(path)} bytes"
            results.append((f"Artifact: {label}", True, f"Found ({size_str})"))
        else:
            results.append((f"Artifact: {label}", False, f"Missing or too small at {path}"))

    # Summary
    print("\n=================================================================")
    print("                     TEST EXECUTION RESULTS                      ")
    print("=================================================================")
    passed = 0
    failed = 0
    for name, ok, note in results:
        status_icon = "[PASS]" if ok else "[FAIL]"
        if ok:
            passed += 1
            print(f" {status_icon} | {name:<45} | {note}")
        else:
            failed += 1
            print(f" {status_icon} | {name:<45} | {note}")

    print("=================================================================")
    print(f" TOTAL TESTS: {len(results)} | PASSED: {passed} | FAILED: {failed}")
    if failed == 0:
        print(" RESULT: 100% PERFECT - ALL MODULES VERIFIED SUCCESSFULLY!")
    else:
        print(f" RESULT: {failed} FAILURES DETECTED")
    print("=================================================================")

if __name__ == "__main__":
    main()
