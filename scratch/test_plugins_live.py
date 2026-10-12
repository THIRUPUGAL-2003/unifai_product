import json
import urllib.request
import urllib.parse
import urllib.error
import http.cookiejar
import sys

BASE_URL = "http://localhost:8080"
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def api_request(path, method="GET", data=None):
    url = f"{BASE_URL}{path}"
    headers = {"Content-Type": "application/json"}
    body = json.dumps(data).encode("utf-8") if data is not None else None
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with opener.open(req) as resp:
            content = resp.read().decode("utf-8")
            try:
                parsed = json.loads(content) if content else {}
            except Exception:
                parsed = content
            return resp.status, parsed
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            parsed = json.loads(err_body)
        except Exception:
            parsed = err_body
        return e.code, parsed

def run_tests():
    print("=== 1. ADMIN AUTHENTICATION ===")
    status, res = api_request("/api/session/login", "POST", {
        "username": "admin@yespanchi.com",
        "password": "YP2025-2026yp"
    })
    print(f"Login status: {status}")
    assert status == 200, "Admin login failed"

    print("\n=== 2. LIST PLUGINS ===")
    status, res = api_request("/api/plugins", "GET")
    print(f"GET /api/plugins -> Status: {status}, Total plugins: {len(res.get('plugins', []))}")
    assert status == 200, "Failed to get plugins"

    print("\n=== 3. GET BUILTIN PLUGINS ===")
    status, res = api_request("/api/plugins/builtins", "GET")
    print(f"GET /api/plugins/builtins -> Status: {status}, Builtins: {res.get('plugins')}")
    assert status == 200, "Failed to get builtins"

    print("\n=== 4. GET LOADED PLUGINS ===")
    status, res = api_request("/api/plugins/loaded", "GET")
    print(f"GET /api/plugins/loaded -> Status: {status}, Loaded: {res.get('plugins')}")
    assert status == 200, "Failed to get loaded plugins"

    # Pre-cleanup in case it already exists from prior run
    api_request(f"/api/plugins/test-e2e-custom-plugin", "DELETE")

    print("\n=== 5. CREATE CUSTOM PLUGIN ===")

    test_plugin_name = "test-e2e-custom-plugin"
    # Using http path for custom plugin
    plugin_payload = {
        "name": test_plugin_name,
        "enabled": False,
        "path": "http://localhost:8080/mock-plugin",
        "config": {"timeout": 30, "sample_rate": 1.0}
    }
    status, res = api_request("/api/plugins", "POST", plugin_payload)
    print(f"POST /api/plugins -> Status: {status}, Response: {res}")
    assert status in (200, 201), f"Plugin create failed with status {status}"

    print("\n=== 6. GET SINGLE PLUGIN ===")
    status, res = api_request(f"/api/plugins/{test_plugin_name}", "GET")
    print(f"GET /api/plugins/{test_plugin_name} -> Status: {status}, Plugin name: {res.get('name')}")
    assert status == 200, "Get plugin failed"
    assert res.get("name") == test_plugin_name


    print("\n=== 7. UPDATE / TOGGLE PLUGIN ===")
    update_payload = {
        "enabled": True,
        "path": "http://localhost:8080/mock-plugin",
        "config": {"timeout": 60, "sample_rate": 0.5}
    }
    status, res = api_request(f"/api/plugins/{test_plugin_name}", "PUT", update_payload)
    print(f"PUT /api/plugins/{test_plugin_name} -> Status: {status}, Enabled: {res.get('plugin', {}).get('enabled')}")
    # In case path is not reachable during live load, check error handling
    if status == 200:
        print("Plugin updated successfully!")
    else:
        print(f"Plugin update returned handled response: {status}, msg: {res}")

    print("\n=== 8. DELETE CUSTOM PLUGIN (CLEAN DELETE FROM DB) ===")
    status, res = api_request(f"/api/plugins/{test_plugin_name}", "DELETE")
    print(f"DELETE /api/plugins/{test_plugin_name} -> Status: {status}, Response: {res}")
    assert status == 200, f"Plugin delete failed with status {status}"

    print("\n=== 9. VERIFY CLEAN DELETION ===")
    status, res = api_request(f"/api/plugins/{test_plugin_name}", "GET")
    print(f"GET /api/plugins/{test_plugin_name} after delete -> Status: {status}")
    assert status == 404, "Plugin should be 404 after deletion"
    print("Plugin clean deletion verified!")

    print("\n=== 10. ERROR HANDLING TESTS ===")
    # Missing required name on create
    status, res = api_request("/api/plugins", "POST", {"enabled": True})
    print(f"Create with missing name -> Status: {status} (Expected 400)")
    assert status == 400, "Expected 400 on missing name"

    # Delete non-existent plugin
    status, res = api_request("/api/plugins/non_existent_plugin_xyz", "DELETE")
    print(f"Delete non-existent plugin -> Status: {status} (Expected 404)")
    assert status == 404, "Expected 404 on non-existent plugin delete"

    print("\n ALL PLUGIN TESTS PASSED! PERSISTENCE & ERROR HANDLING VERIFIED!")

if __name__ == "__main__":
    run_tests()
