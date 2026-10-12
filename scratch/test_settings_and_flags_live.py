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
    if status != 200:
        print("Failed to login as admin! Response:", res)
        sys.exit(1)
    print("Logged in successfully as admin!")

    print("\n=== 2. LIST FEATURE FLAGS ===")
    status, res = api_request("/api/feature-flags", "GET")
    print(f"List flags status: {status}")
    flags = res.get("flags", [])
    print(f"Found {len(flags)} registered flags.")
    assert status == 200, f"Expected 200, got {status}"
    assert len(flags) > 0, "Expected non-empty flags list"

    print("\n=== 3. CREATE NEW CUSTOM FEATURE FLAG (POST /api/feature-flags) ===")
    test_flag_id = "custom.test_analytics_v2"
    status, res = api_request("/api/feature-flags", "POST", {
        "id": test_flag_id,
        "display_name": "Test Analytics V2",
        "description": "Live E2E test flag for real-time analytics engine.",
        "default": False,
        "enabled": True
    })
    print(f"Create flag status: {status}, response: {res}")
    assert status == 201, f"Expected 201 Created, got {status}"
    assert res.get("id") == test_flag_id
    assert res.get("enabled") is True

    print("\n=== 4. VERIFY FLAG IN LIST ===")
    status, res = api_request("/api/feature-flags", "GET")
    flags = res.get("flags", [])
    matched = [f for f in flags if f.get("id") == test_flag_id]
    assert len(matched) == 1, f"Expected flag {test_flag_id} to be in flags list"
    print(f"Flag verified in list: {matched[0]['display_name']} (enabled: {matched[0]['enabled']})")

    print("\n=== 5. TOGGLE NEW FLAG (PUT /api/feature-flags/{id}) ===")
    status, res = api_request(f"/api/feature-flags/{test_flag_id}", "PUT", {
        "enabled": False
    })
    print(f"Toggle flag status: {status}, response: {res}")
    assert status == 200, f"Expected 200, got {status}"
    assert res.get("enabled") is False

    print("\n=== 6. DELETE CUSTOM FLAG (DELETE /api/feature-flags/{id}) ===")
    status, res = api_request(f"/api/feature-flags/{test_flag_id}", "DELETE")
    print(f"Delete flag status: {status}, response: {res}")
    assert status == 200, f"Expected 200, got {status}"
    assert res.get("status") == "deleted"

    print("\n=== 7. VERIFY FLAG DELETED ===")
    status, res = api_request("/api/feature-flags", "GET")
    flags = res.get("flags", [])
    matched = [f for f in flags if f.get("id") == test_flag_id]
    assert len(matched) == 0, f"Flag {test_flag_id} should be deleted"
    print("Flag deletion successfully verified!")

    print("\n=== 8. ERROR HANDLING TESTS ===")
    # 8.1 Invalid Flag ID format
    print("-> Test 8.1: Invalid flag ID format (special characters/uppercase)")
    status, res = api_request("/api/feature-flags", "POST", {
        "id": "INVALID_FLAG_ID!@#",
        "display_name": "Invalid Flag"
    })
    print(f"Status: {status}, response: {res}")
    assert status == 400, f"Expected 400, got {status}"

    # 8.2 Empty Flag ID
    print("-> Test 8.2: Empty flag ID")
    status, res = api_request("/api/feature-flags", "POST", {
        "id": "   ",
        "display_name": "Empty Flag"
    })
    print(f"Status: {status}, response: {res}")
    assert status == 400, f"Expected 400, got {status}"

    # 8.3 Duplicate Flag ID
    print("-> Test 8.3: Duplicate Flag ID registration")
    status, res = api_request("/api/feature-flags", "POST", {
        "id": "section.observability",
        "display_name": "Observability duplicate"
    })
    print(f"Status: {status}, response: {res}")
    assert status == 409, f"Expected 409 Conflict, got {status}"

    # 8.4 Toggle non-existent flag
    print("-> Test 8.4: Toggle non-existent flag")
    status, res = api_request("/api/feature-flags/non_existent_flag_abc_123", "PUT", {
        "enabled": True
    })
    print(f"Status: {status}, response: {res}")
    assert status == 404, f"Expected 404 Not Found, got {status}"

    # 8.5 Toggle missing enabled field
    print("-> Test 8.5: Missing enabled field")
    status, res = api_request("/api/feature-flags/browser.file_scan", "PUT", {})
    print(f"Status: {status}, response: {res}")
    assert status == 400, f"Expected 400 Bad Request, got {status}"

    # 8.6 Toggle with invalid payload
    print("-> Test 8.6: Malformed payload")
    url = f"{BASE_URL}/api/feature-flags/browser.file_scan"
    req = urllib.request.Request(url, method="PUT", headers={"Content-Type": "application/json"}, data=b"not a json")
    try:
        with opener.open(req) as resp:
            status = resp.status
    except urllib.error.HTTPError as e:
        status = e.code
    print(f"Status for malformed json: {status}")
    assert status == 400, f"Expected 400 Bad Request, got {status}"

    print("\n=== 9. SETTINGS ENDPOINTS AUDIT ===")
    endpoints = [
        ("/api/config", "Client & Core Config"),
        ("/api/caching-config", "Caching Config"),
        ("/api/proxy-config", "Proxy Config"),
        ("/api/smtp-config", "SMTP Config"),
        ("/api/auth-config", "Auth Config"),
        ("/api/vector-store-config", "Vector Store Config"),
        ("/api/compatibility-config", "Compatibility Config"),
        ("/api/system-info", "System Info"),
        ("/api/version", "Version Info")
    ]
    for path, name in endpoints:
        status, res = api_request(path, "GET")
        print(f"Checking [{name}] {path} -> HTTP {status}")
        assert status == 200, f"Endpoint {path} failed with {status}"

    print("\n ALL TESTS PASSED SUCCESSFULLY! EVERYTHING IS 100% OPERATIONAL!")

if __name__ == "__main__":
    run_tests()
