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

    print("\n=== 2. AUDIT LOGS ENDPOINTS ===")
    status, res = api_request("/api/audit-logs", "GET")
    print(f"GET /api/audit-logs -> Status: {status}")
    assert status == 200, "Failed to get audit logs"

    status, res = api_request("/api/audit-logs/settings", "GET")
    print(f"GET /api/audit-logs/settings -> Status: {status}")
    assert status == 200, "Failed to get audit settings"

    status, res = api_request("/api/audit-logs/export", "GET")
    print(f"GET /api/audit-logs/export -> Status: {status}")
    assert status == 200, "Failed to export audit logs"

    print("\n=== 3. ACCESS PROFILES ENDPOINTS ===")
    status, res = api_request("/api/access-profiles", "GET")
    print(f"GET /api/access-profiles -> Status: {status}")
    assert status == 200, "Failed to get access profiles"

    import time
    unique_suffix = int(time.time())
    profile_payload = {
        "name": f"E2E Test Security Profile {unique_suffix}",
        "description": "Profile created by automated live test",
        "allowed_networks": ["0.0.0.0/0"],
        "max_session_duration": 3600
    }

    status, res = api_request("/api/access-profiles", "POST", profile_payload)
    print(f"POST /api/access-profiles -> Status: {status}, Response: {res}")
    assert status in (200, 201), "Failed to create access profile"
    profile_id = res.get("access_profile", {}).get("id") or res.get("id")
    print(f"Created profile ID: {profile_id}")

    if profile_id:
        status, res = api_request(f"/api/access-profiles/{profile_id}/activate", "POST")
        print(f"Activate profile -> Status: {status}")

        status, res = api_request(f"/api/access-profiles/{profile_id}/deactivate", "POST")
        print(f"Deactivate profile -> Status: {status}")

        status, res = api_request(f"/api/access-profiles/{profile_id}/clone", "POST", {"name": "Cloned Test Profile"})
        print(f"Clone profile -> Status: {status}")
        cloned_id = res.get("access_profile", {}).get("id") or res.get("id")

        # Clean delete cloned profile
        if cloned_id:
            status, res = api_request(f"/api/access-profiles/{cloned_id}", "DELETE")
            print(f"DELETE cloned profile -> Status: {status}")
            assert status == 200, "Failed to delete cloned profile"

        # Clean delete created profile
        status, res = api_request(f"/api/access-profiles/{profile_id}", "DELETE")
        print(f"DELETE profile -> Status: {status}")
        assert status == 200, "Failed to delete access profile"

    print("\n=== 4. ACCESS PROFILE ERROR HANDLING ===")
    # Invalid ID format (string instead of int)
    status, res = api_request("/api/access-profiles/invalid_str_id", "DELETE")
    print(f"Delete with invalid format ID -> Status: {status} (Expected 400)")
    assert status == 400, "Expected 400 on invalid format ID"

    # Non-existent integer ID
    status, res = api_request("/api/access-profiles/999999", "DELETE")
    print(f"Delete non-existent integer ID -> Status: {status} (Expected 404)")
    assert status == 404, "Expected 404 on non-existent profile delete"


    print("\n ALL AUDIT LOGS & ACCESS PROFILE TESTS PASSED!")

if __name__ == "__main__":
    run_tests()
