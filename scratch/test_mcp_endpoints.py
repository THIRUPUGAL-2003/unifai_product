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
        log(f"[HTTP {e.code}] {name}: {err_body[:250]}")
        return None
    except Exception as e:
        log(f"[FAIL] {name}: {e}")
        return None

def main():
    log("=== Testing MCP Gateway Module & Sub-Sections ===")

    # 0. Login
    login = test_endpoint("Admin Login", "/api/session/login", method="POST", body={
        "username": ADMIN_USER,
        "password": ADMIN_PASS
    })
    if not login:
        log("Login failed, aborting.")
        sys.exit(1)

    # 1. MCP Catalog (/workspace/mcp-registry)
    log("\n--- 1. MCP Catalog (/workspace/mcp-registry) ---")
    clients = test_endpoint("List MCP Clients", "/api/mcp/clients")

    # 2. MCP Library (/workspace/mcp-registry/library)
    log("\n--- 2. MCP Library (/workspace/mcp-registry/library) ---")
    lib = test_endpoint("List MCP Library", "/api/mcp/library?limit=10")
    filter_data = test_endpoint("MCP Library Filter Data", "/api/mcp/library/filterdata")
    
    # Test Create & Delete custom MCP Library entry
    sample_entry = {
        "name": "Diagnostic MCP Test Server",
        "description": "Test entry for automated verification",
        "category": "Development",
        "connection_type": "http",
        "connection_url": "https://mcp.test.internal/v1",
        "tags": ["test", "diagnostic"]
    }
    created_entry = test_endpoint("Create MCP Library Entry", "/api/mcp/library", method="POST", body=sample_entry)
    if created_entry and "server" in created_entry:
        entry_id = created_entry["server"]["id"]
        test_endpoint(f"Delete MCP Library Entry ({entry_id})", f"/api/mcp/library/{entry_id}", method="DELETE")
    elif created_entry and "id" in created_entry:
        entry_id = created_entry["id"]
        test_endpoint(f"Delete MCP Library Entry ({entry_id})", f"/api/mcp/library/{entry_id}", method="DELETE")

    # 3. Tool Groups (/workspace/mcp-tool-groups)
    log("\n--- 3. Tool Groups (/workspace/mcp-tool-groups) ---")
    tool_groups = test_endpoint("List MCP Tool Groups", "/api/mcp/tool-groups")
    # Need a virtual key ID to create a tool group
    vks = test_endpoint("Get Virtual Keys for Tool Group Scope", "/api/governance/virtual-keys?limit=1")
    vk_id = None
    if vks and "virtual_keys" in vks and len(vks["virtual_keys"]) > 0:
        vk_id = vks["virtual_keys"][0]["id"]
    if vk_id:
        sample_tg = {
            "name": "Diagnostic Tool Group",
            "description": "Test tool group",
            "enabled": True,
            "virtual_key_ids": [vk_id],
            "tools": [{"name": "test-tool"}]
        }
        created_tg = test_endpoint("Create MCP Tool Group", "/api/mcp/tool-groups", method="POST", body=sample_tg)
        if created_tg and "id" in created_tg:
            tg_id = created_tg["id"]
            test_endpoint(f"Get MCP Tool Group ({tg_id})", f"/api/mcp/tool-groups/{tg_id}")
            test_endpoint(f"Delete MCP Tool Group ({tg_id})", f"/api/mcp/tool-groups/{tg_id}", method="DELETE")

    # 4. Auth Sessions (/workspace/mcp-sessions)
    log("\n--- 4. Auth Sessions (/workspace/mcp-sessions) ---")
    sessions = test_endpoint("List MCP Auth Sessions", "/api/mcp/sessions")

    # 5. OAuth Grants (/workspace/oauth-grants)
    log("\n--- 5. OAuth Grants (/workspace/oauth-grants) ---")
    grants = test_endpoint("List OAuth2 Grants", "/api/oauth2/sessions")

    # 6. MCP Settings (/workspace/mcp-settings)
    log("\n--- 6. MCP Settings (/workspace/mcp-settings) ---")
    cfg = test_endpoint("Get MCP Settings (DB Config)", "/api/config?from_db=true")

    log("\n=== MCP Gateway All Tests Completed ===")

if __name__ == "__main__":
    main()
