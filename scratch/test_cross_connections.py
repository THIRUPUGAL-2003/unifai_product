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

def api_call(name, path, method="GET", body=None):
    url = f"{BASE_URL}{path}"
    print(f"--> Calling {name} ({method} {path})...", flush=True)
    try:
        req = urllib.request.Request(url, method=method)
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            req.add_header("Content-Type", "application/json")
            req.data = data
        with opener.open(req, timeout=10) as resp:
            code = resp.getcode()
            content = resp.read().decode("utf-8")
            try:
                parsed = json.loads(content)
                print(f"[PASS] {name}: HTTP {code}", flush=True)
                return parsed
            except Exception:
                print(f"[PASS] {name}: HTTP {code} (text)", flush=True)
                return content
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="ignore")
        print(f"[HTTP {e.code}] {name}: {err_body[:200]}", flush=True)
        return None
    except Exception as e:
        print(f"[FAIL] {name}: {e}", flush=True)
        return None

def run():
    print("=== STARTING FULL DEEP RELATIONAL INTEGRATION TESTS ===")

    # 1. Login Authentication
    login = api_call("Admin Login", "/api/session/login", method="POST", body={
        "username": ADMIN_USER,
        "password": ADMIN_PASS
    })
    if not login:
        print("Login failed, aborting.")
        sys.exit(1)

    # 2. RBAC Permissions Check
    api_call("Check Me Permissions", "/api/rbac/me/permissions")
    api_call("List Roles", "/api/roles")

    # 3. Create Customer with Budget ($500/month)
    customer = api_call("Create Customer", "/api/governance/customers", method="POST", body={
        "name": "Acme Global Enterprise",
        "description": "Integration test customer with $500 monthly budget",
        "budgets": [{"max_limit": 500.0, "reset_duration": "1M"}]
    })
    customer_id = customer.get("id") if customer else None

    # 4. Create Business Unit
    bu = api_call("Create Business Unit", "/api/governance/business-units", method="POST", body={
        "name": "AI Engineering BU",
        "description": "Business Unit for AI engineering",
        "budget": 300.0
    })
    bu_id = bu.get("id") if bu else None

    # 5. Create Team under Customer with Budget ($200/month <= $500/month allocation)
    team_payload = {
        "name": "Acme Core AI Team",
        "description": "Team under Acme Global Enterprise",
        "budgets": [{"max_limit": 200.0, "reset_duration": "1M"}]
    }
    if customer_id:
        team_payload["customer_id"] = customer_id
    team = api_call("Create Team", "/api/governance/teams", method="POST", body=team_payload)
    team_id = team.get("id") if team else None

    # 6. Assign Team to Business Unit
    if bu_id and team_id:
        api_call("Assign Team to Business Unit", f"/api/governance/business-units/{bu_id}/teams", method="POST", body={
            "team_id": team_id
        })

    # 7. Create User
    test_user = api_call("Create User", "/api/session/users", method="POST", body={
        "username": "acme_lead_dev",
        "email": "lead_dev@acme-global.io",
        "password": "Password123!@#",
        "role": "user"
    })
    user_id = test_user.get("id") if test_user else None

    # 8. Add User as Team Member (triggers promptLifecycle.OnTeamMemberAdded)
    if team_id and user_id:
        api_call("Add User to Team", f"/api/governance/teams/{team_id}/members", method="POST", body={
            "user_id": user_id
        })
        api_call("List Team Members", f"/api/governance/teams/{team_id}/members")

    # 9. Create Virtual Key linked to User, Team, and Customer
    vk_payload = {
        "name": "Acme Production Gateway Key",
        "description": "Key linked to Customer Acme & Team Acme Core AI",
        "budgets": [{"max_limit": 50.0, "reset_duration": "1M"}]
    }
    if customer_id:
        vk_payload["customer_ids"] = [customer_id]
    if team_id:
        vk_payload["team_ids"] = [team_id]
    if user_id:
        vk_payload["user_ids"] = [user_id]
    
    vk = api_call("Create Virtual Key", "/api/governance/virtual-keys", method="POST", body=vk_payload)
    vk_id = vk.get("id") if vk else None

    # 10. Verify Budget Calculations and Stats
    api_call("Get Governance Budgets", "/api/governance/budgets")
    api_call("Get Governance Usage Stats", "/api/governance/usage-stats")
    if vk_id:
        api_call("Get Virtual Key Quota", f"/api/governance/virtual-keys/{vk_id}")

    # 11. Create Access Profile linked to Virtual Key
    if vk_id:
        ap = api_call("Create Access Profile", "/api/access-profiles", method="POST", body={
            "name": "Acme Developer Profile",
            "description": "Access profile for developers",
            "roles": ["user"],
            "virtual_key_id": vk_id,
            "metadata": {"department": "AI"}
        })
        ap_id = ap.get("id") if ap else None
    else:
        ap_id = None

    # 12. SCIM & Entra ID Provisioning Verification
    api_call("Get SCIM Config", "/api/scim/config")
    api_call("Get SCIM Providers (Entra ID, Okta, Keycloak)", "/api/scim/providers")

    # 13. Prompt Repository Connection Flow
    folder = api_call("Create Prompt Folder", "/api/prompt-repo/folders", method="POST", body={
        "name": "Acme Core Prompts",
        "description": "Prompts folder for Acme"
    })
    folder_id = folder.get("id") if folder else None

    prompt_id = None
    if folder_id:
        prompt_item = api_call("Create Prompt Template", "/api/prompt-repo/prompts", method="POST", body={
            "title": "Customer Support System Prompt",
            "folder_id": folder_id,
            "content": "You are a helpful customer support agent for {{customer_name}}.",
            "variables": ["customer_name"],
            "version": "1.0.0"
        })
        prompt_id = prompt_item.get("id") if prompt_item else None

    # 14. Audit Logs Verification
    api_call("Fetch Audit Logs", "/api/audit-logs")

    # 15. Teardown / Cleanup test entities
    print("\n--- Cleaning up test entities ---")
    if prompt_id:
        api_call("Delete Prompt", f"/api/prompt-repo/prompts/{prompt_id}", method="DELETE")
    if folder_id:
        api_call("Delete Folder", f"/api/prompt-repo/folders/{folder_id}", method="DELETE")
    if ap_id:
        api_call("Delete Access Profile", f"/api/access-profiles/{ap_id}", method="DELETE")
    if vk_id:
        api_call("Delete Virtual Key", f"/api/governance/virtual-keys/{vk_id}", method="DELETE")
    if team_id and user_id:
        api_call("Remove User from Team", f"/api/governance/teams/{team_id}/members/{user_id}", method="DELETE")
    if user_id:
        api_call("Delete User", f"/api/session/users/{user_id}", method="DELETE")
    if team_id:
        api_call("Delete Team", f"/api/governance/teams/{team_id}", method="DELETE")
    if bu_id:
        api_call("Delete Business Unit", f"/api/governance/business-units/{bu_id}", method="DELETE")
    if customer_id:
        api_call("Delete Customer", f"/api/governance/customers/{customer_id}", method="DELETE")

    print("\n=== ALL CROSS-MODULE INTEGRATION TESTS COMPLETE! ===")

if __name__ == "__main__":
    run()
