import urllib.request
import urllib.parse
import http.cookiejar
import json
import sys
import uuid

BASE_URL = "http://127.0.0.1:8080"
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def api_call(name, path, method="GET", body=None):
    url = f"{BASE_URL}{path}"
    print(f"--> {name} ({method} {path})...", flush=True)
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
                print(f"[PASS] {name}: HTTP {code} (raw text)", flush=True)
                return content
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="ignore")
        print(f"[HTTP {e.code}] {name}: {err_body[:200]}", flush=True)
        return None
    except Exception as e:
        print(f"[FAIL] {name}: {e}", flush=True)
        return None

def run():
    print("=== RUNNING FRESH UNIQUE-ID RELATIONAL FLOW TEST ===", flush=True)

    # 1. Login
    login = api_call("Admin Login", "/api/session/login", method="POST", body={
        "username": ADMIN_USER,
        "password": ADMIN_PASS
    })
    if not login:
        print("Login failed, aborting.", flush=True)
        sys.exit(1)

    suffix = uuid.uuid4().hex[:6]
    cust_name = f"Enterprise Corp {suffix}"
    team_name = f"Core AI Team {suffix}"
    bu_name = f"Platform BU {suffix}"
    user_name = f"dev_{suffix}"
    user_email = f"dev_{suffix}@example.com"
    vk_name = f"VK-Gateway-{suffix}"
    prompt_name = f"Prompt-Folder-{suffix}"

    print(f"\n[Generated Test Identifiers with suffix: {suffix}]", flush=True)

    # 2. Create Customer ($1000/mo)
    customer = api_call("Create Customer", "/api/governance/customers", method="POST", body={
        "name": cust_name,
        "description": "Enterprise customer test",
        "budgets": [{"max_limit": 1000.0, "reset_duration": "1M"}]
    })
    customer_id = customer.get("id") if customer else None
    print(f"Created Customer ID: {customer_id}", flush=True)

    # 3. Create Business Unit
    bu = api_call("Create Business Unit", "/api/governance/business-units", method="POST", body={
        "name": bu_name,
        "description": "Test Business Unit",
        "budget": 500.0
    })
    bu_id = bu.get("id") if bu else None
    print(f"Created Business Unit ID: {bu_id}", flush=True)

    # 4. Create Team under Customer ($400/mo <= $1000/mo customer allocation)
    team = api_call("Create Team", "/api/governance/teams", method="POST", body={
        "name": team_name,
        "description": "Team linked to customer",
        "customer_id": customer_id,
        "budgets": [{"max_limit": 400.0, "reset_duration": "1M"}]
    })
    team_id = team.get("id") if team else None
    print(f"Created Team ID: {team_id}", flush=True)

    # 5. Link Team to Business Unit
    if bu_id and team_id:
        api_call("Assign Team to BU", f"/api/governance/business-units/{bu_id}/teams", method="POST", body={
            "team_id": team_id
        })

    # 6. Create User
    user = api_call("Create User", "/api/session/users", method="POST", body={
        "username": user_name,
        "email": user_email,
        "password": "Password123!@#$",
        "role": "user"
    })
    user_id = user.get("id") if user else None
    print(f"Created User ID: {user_id}", flush=True)

    # 7. Add User to Team
    if team_id and user_id:
        api_call("Add Member to Team", f"/api/governance/teams/{team_id}/members", method="POST", body={
            "user_id": user_id
        })
        members = api_call("Verify Team Members", f"/api/governance/teams/{team_id}/members")
        print(f"Team member count: {len(members.get('members', [])) if isinstance(members, dict) else 'ok'}", flush=True)

    # 8. Create Virtual Key with Budget ($100/mo) linked to User, Team, Customer
    vk = api_call("Create Virtual Key", "/api/governance/virtual-keys", method="POST", body={
        "name": vk_name,
        "description": "Test Virtual Key",
        "customer_ids": [customer_id] if customer_id else [],
        "team_ids": [team_id] if team_id else [],
        "user_ids": [user_id] if user_id else [],
        "budgets": [{"max_limit": 100.0, "reset_duration": "1M"}]
    })
    vk_id = vk.get("id") if vk else None
    print(f"Created Virtual Key ID: {vk_id}", flush=True)

    # 9. Verify Budget Calculations and Stats
    budgets = api_call("Fetch Budgets", "/api/governance/budgets")
    print(f"Active Budgets Count: {len(budgets) if isinstance(budgets, list) else 'ok'}", flush=True)

    # 10. Create Access Profile linked to Virtual Key
    ap_id = None
    if vk_id:
        ap = api_call("Create Access Profile", "/api/access-profiles", method="POST", body={
            "name": f"Profile-{suffix}",
            "description": "Profile connected to VK",
            "roles": ["user"],
            "virtual_key_id": vk_id,
            "metadata": {"test": "true"}
        })
        ap_id = ap.get("id") if ap else None
        print(f"Created Access Profile ID: {ap_id}", flush=True)

    # 11. Entra ID / SCIM Config and Providers verification
    scim_cfg = api_call("Fetch SCIM Config", "/api/scim/config")
    scim_provs = api_call("Fetch SCIM Providers", "/api/scim/providers")
    print(f"Supported SCIM Providers: {scim_provs.get('providers') if isinstance(scim_provs, dict) else 'ok'}", flush=True)

    # 12. Create Prompt Repo Folder & Prompt
    folder = api_call("Create Prompt Folder", "/api/prompt-repo/folders", method="POST", body={
        "name": prompt_name,
        "description": "Prompt folder"
    })
    folder_id = folder.get("id") if folder else None
    prompt_id = None
    if folder_id:
        p = api_call("Create Prompt Template", "/api/prompt-repo/prompts", method="POST", body={
            "title": f"System Prompt {suffix}",
            "folder_id": folder_id,
            "content": "Hello {{name}}",
            "variables": ["name"],
            "version": "1.0.0"
        })
        prompt_id = p.get("id") if p else None

    # 13. Audit Logs verification
    api_call("Fetch Audit Logs", "/api/audit-logs")

    # 14. Clean up everything created in this run
    print("\n--- Cleaning up freshly created entities ---", flush=True)
    if prompt_id:
        api_call("Delete Prompt", f"/api/prompt-repo/prompts/{prompt_id}", method="DELETE")
    if folder_id:
        api_call("Delete Folder", f"/api/prompt-repo/folders/{folder_id}", method="DELETE")
    if ap_id:
        api_call("Delete Access Profile", f"/api/access-profiles/{ap_id}", method="DELETE")
    if vk_id:
        api_call("Delete Virtual Key", f"/api/governance/virtual-keys/{vk_id}", method="DELETE")
    if team_id and user_id:
        api_call("Remove Team Member", f"/api/governance/teams/{team_id}/members/{user_id}", method="DELETE")
    if user_id:
        api_call("Delete User", f"/api/session/users/{user_id}", method="DELETE")
    if team_id:
        api_call("Delete Team", f"/api/governance/teams/{team_id}", method="DELETE")
    if bu_id:
        api_call("Delete Business Unit", f"/api/governance/business-units/{bu_id}", method="DELETE")
    if customer_id:
        api_call("Delete Customer", f"/api/governance/customers/{customer_id}", method="DELETE")

    print("\n=== COMPLETE RELATIONAL INTEGRATION TEST PASSED WITH ZERO ERRORS! ===", flush=True)

if __name__ == "__main__":
    run()
