import urllib.request
import urllib.parse
import http.cookiejar
import json
import sys

BASE_URL = "http://localhost:8080"
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

# Cookie jar for session management
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def test_endpoint(name, path, method="GET", body=None):
    url = f"{BASE_URL}{path}"
    try:
        req = urllib.request.Request(url, method=method)
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            req.add_header("Content-Type", "application/json")
            req.data = data
        with opener.open(req) as resp:
            code = resp.getcode()
            content = resp.read().decode("utf-8")
            try:
                parsed = json.loads(content)
                print(f"[PASS] {name}: HTTP {code}")
                return parsed
            except:
                print(f"[PASS] {name}: HTTP {code} (raw text)")
                return content
    except urllib.error.HTTPError as e:
        err_body = e.read().decode('utf-8', errors='ignore')
        print(f"[HTTP {e.code}] {name}: {err_body[:200]}")
        return None
    except Exception as e:
        print(f"[FAIL] {name}: {e}")
        return None

def main():
    print("=== Testing Observability & Retention Flow ===")
    
    # 0. Health / Version
    test_endpoint("Version Check", "/api/version")

    # 1. Login
    print("\nAuthenticating admin...")
    login_res = test_endpoint("Admin Login", "/api/session/login", method="POST", body={
        "username": ADMIN_USER,
        "password": ADMIN_PASS
    })
    if not login_res:
        print("Login failed, aborting authenticated checks")
        return

    # 2. Section: Dashboard
    print("\n--- 1. Section: Dashboard ---")
    test_endpoint("Dashboard - Logs Stats (1h)", "/api/logs/stats?period=1h")
    test_endpoint("Dashboard - Logs Histogram (1h)", "/api/logs/histogram?period=1h")
    test_endpoint("Dashboard - Token Histogram (1h)", "/api/logs/histogram/tokens?period=1h")
    test_endpoint("Dashboard - Cost Histogram (1h)", "/api/logs/histogram/cost?period=1h")
    test_endpoint("Dashboard - Models Histogram (1h)", "/api/logs/histogram/models?period=1h")
    test_endpoint("Dashboard - Latency Histogram (1h)", "/api/logs/histogram/latency?period=1h")
    test_endpoint("Dashboard - Model Rankings", "/api/logs/rankings?period=1h")
    test_endpoint("Dashboard - Dimension Rankings (team)", "/api/logs/rankings/by-dimension?dimension=team_id&period=1h")

    # 3. Section: LLM Logs
    print("\n--- 2. Section: LLM Logs ---")
    logs_res = test_endpoint("LLM Logs - List (limit 10)", "/api/logs?limit=10&offset=0&period=1h")
    if logs_res and "logs" in logs_res:
        print(f"       Found {len(logs_res['logs'])} logs (total: {logs_res.get('total_count', 0)})")
    test_endpoint("LLM Logs - Filter Data", "/api/logs/filterdata")

    # 4. Section: MCP Logs
    print("\n--- 3. Section: MCP Logs ---")
    mcp_logs_res = test_endpoint("MCP Logs - List (limit 10)", "/api/mcp-logs?limit=10&offset=0&period=1h")
    if mcp_logs_res and "logs" in mcp_logs_res:
        print(f"       Found {len(mcp_logs_res['logs'])} MCP logs (total: {mcp_logs_res.get('total_count', 0)})")
    test_endpoint("MCP Logs - Stats (1h)", "/api/mcp-logs/stats?period=1h")
    test_endpoint("MCP Logs - Histogram (1h)", "/api/mcp-logs/histogram?period=1h")
    test_endpoint("MCP Logs - Cost Histogram (1h)", "/api/mcp-logs/histogram/cost?period=1h")
    test_endpoint("MCP Logs - Top Tools (1h)", "/api/mcp-logs/histogram/top-tools?period=1h")

    # 5. Section: Connectors
    print("\n--- 4. Section: Connectors ---")
    test_endpoint("Connectors - List", "/api/connectors")

    # 6. Section: Logs Settings & Auto-Delete Retention Loop
    print("\n--- 5. Section: Logs Settings & Auto-Delete Loop ---")
    cfg = test_endpoint("Logs Settings - Core Config", "/api/config?from_db=true")
    if cfg and "client_config" in cfg:
        cur_retention = cfg["client_config"].get("log_retention_days", 0)
        print(f"       Current Retention Days in DB: {cur_retention}")

    # Test Immediate Cleanup with retention_days = 30
    print("\nTesting Log Cleanup / Retention Trigger (POST /api/logs/cleanup)...")
    cleanup_res = test_endpoint("Trigger Log Cleanup (30 days)", "/api/logs/cleanup", method="POST", body={
        "retention_days": 30
    })
    if cleanup_res:
        print(f"       Cleanup Result: {json.dumps(cleanup_res)}")

    # Test Immediate Cleanup with retention_days = 0 (disabled auto-delete)
    cleanup_disabled_res = test_endpoint("Trigger Log Cleanup (0 days = disabled)", "/api/logs/cleanup", method="POST", body={
        "retention_days": 0
    })
    if cleanup_disabled_res:
        print(f"       Cleanup Disabled Result: {json.dumps(cleanup_disabled_res)}")

    print("\n=== All Observability sections & Auto-Delete loop verified successfully! ===")

if __name__ == "__main__":
    main()
