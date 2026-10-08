import urllib.request
import urllib.parse
import http.cookiejar
import json
import sys

BASE_URL = "http://127.0.0.1:8080"
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def log(msg):
    print(msg, flush=True)

def test_endpoint(name, path, method="GET", body=None, timeout=10):
    url = f"{BASE_URL}{path}"
    try:
        req = urllib.request.Request(url, method=method)
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            req.add_header("Content-Type", "application/json")
            req.data = data
        with opener.open(req, timeout=timeout) as resp:
            code = resp.getcode()
            content = resp.read().decode("utf-8")
            try:
                parsed = json.loads(content)
                log(f"[PASS] {name}: HTTP {code}")
                return parsed
            except:
                log(f"[PASS] {name}: HTTP {code} (text)")
                return content
    except urllib.error.HTTPError as e:
        err_body = e.read().decode('utf-8', errors='ignore')
        log(f"[HTTP {e.code}] {name}: {err_body[:200]}")
        return None
    except Exception as e:
        log(f"[FAIL] {name}: {e}")
        return None

def main():
    log("=== Master Diagnostic Test for All Remaining Workspace Modules ===")

    # Login
    login = test_endpoint("Admin Login", "/api/session/login", method="POST", body={
        "username": ADMIN_USER,
        "password": ADMIN_PASS
    })
    if not login:
        log("Login failed, aborting.")
        sys.exit(1)

    # 1. Skills Repository (/workspace/skills-repo)
    log("\n--- 1. Skills Repository ---")
    test_endpoint("List Skills", "/api/skills")
    test_endpoint("Get All Skills Version", "/api/skills/all/version")

    # 2. Settings (/workspace/config/*)
    log("\n--- 2. Settings (Client, Compatibility, Caching, Security, Performance, Flags) ---")
    test_endpoint("Get Core Config (DB)", "/api/config?from_db=true")
    test_endpoint("List Feature Flags", "/api/feature-flags")
    test_endpoint("Get Vector Store Config", "/api/vector-store-config")
    test_endpoint("Get SMTP Config", "/api/smtp-config")

    # 3. Guardrails (/workspace/guardrails/*)
    log("\n--- 3. Guardrails (Rules, Providers, Cluster) ---")
    test_endpoint("Get Guardrails Config", "/api/guardrails/config")
    test_endpoint("Get Cluster Config", "/api/cluster")

    # 4. Adaptive Routing (/workspace/adaptive-routing/*)
    log("\n--- 4. Adaptive Routing (Dashboard, Settings) ---")
    test_endpoint("Get Load Balancer Config", "/api/load-balancer")
    test_endpoint("Get Load Balancer Routes", "/api/load-balancer/routes")

    # 5. Prompt Repository (/workspace/prompt-repo)
    log("\n--- 5. Prompt Repository ---")
    test_endpoint("List Prompt Folders", "/api/prompt-repo/folders")
    test_endpoint("List Prompts", "/api/prompt-repo/prompts")
    test_endpoint("List Prompt Deployments", "/api/prompt-deployments")

    # 6. Plugins (/workspace/plugins)
    log("\n--- 6. Plugins ---")
    test_endpoint("List Plugins", "/api/plugins")
    test_endpoint("List Builtin Plugins", "/api/plugins/builtins")
    test_endpoint("List Loaded Plugins", "/api/plugins/loaded")

    # 7. Governance (/workspace/governance/*)
    log("\n--- 7. Governance (Virtual Keys, Users, Teams, Business Units, Customers, SCIM, RBAC, Access Profiles, Audit Logs) ---")
    test_endpoint("List Virtual Keys", "/api/governance/virtual-keys")
    test_endpoint("List Users", "/api/governance/users")
    test_endpoint("List Teams", "/api/governance/teams")
    test_endpoint("List Business Units", "/api/governance/business-units")
    test_endpoint("List Customers", "/api/governance/customers")
    test_endpoint("Get SCIM Config", "/api/scim/config")
    test_endpoint("List SCIM Providers", "/api/scim/providers")
    test_endpoint("List RBAC Roles", "/api/roles")
    test_endpoint("Get My RBAC Permissions", "/api/rbac/me/permissions")
    test_endpoint("Get RBAC Scope Grants", "/api/rbac/scope-grants")
    test_endpoint("List Access Profiles", "/api/access-profiles")
    test_endpoint("List Audit Logs", "/api/audit-logs")
    test_endpoint("Get Audit Settings", "/api/audit-logs/settings")

    log("\n=== Master Verification Completed ===")

if __name__ == "__main__":
    main()
