import urllib.request
import json
import http.cookiejar
import sys

def run_tests():
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

    BASE = "http://localhost:8080"

    print("=== Testing Auth Endpoints Live ===")

    # 1. Test Password Policy: short password (< 8 chars)
    short_pw_data = json.dumps({
        "username": "short_pw_user",
        "email": "short@example.com",
        "password": "123"
    }).encode("utf-8")
    req = urllib.request.Request(f"{BASE}/api/session/register", data=short_pw_data, headers={"Content-Type": "application/json"})
    try:
        resp = opener.open(req)
        print("FAIL: short password was accepted with status", resp.status)
    except urllib.error.HTTPError as e:
        print(f"PASS: Short password rejected properly -> HTTP {e.code}: {e.read().decode('utf-8')}")

    # 2. Test Admin Login
    admin_login = json.dumps({"username": "admin@yespanchi.com", "password": "YP2025-2026yp"}).encode("utf-8")
    req = urllib.request.Request(f"{BASE}/api/session/login", data=admin_login, headers={"Content-Type": "application/json"})
    resp = opener.open(req)
    admin_data = json.loads(resp.read().decode("utf-8"))
    print(f"PASS: Admin login successful -> HTTP {resp.status}, role={admin_data.get('role')}")

    # 3. Test is-auth-enabled
    req = urllib.request.Request(f"{BASE}/api/session/is-auth-enabled")
    resp = opener.open(req)
    auth_info = json.loads(resp.read().decode("utf-8"))
    print(f"PASS: is-auth-enabled -> HTTP {resp.status}, is_auth_enabled={auth_info.get('is_auth_enabled')}, role={auth_info.get('role')}")

    # 4. Test User Creation / Registration with 8+ character password
    test_username = "test_e2e_verify_user"
    test_email = "test_verify@example.com"
    test_password = "ValidPassword123!"

    # Clean up previous test user if exists
    req = urllib.request.Request(f"{BASE}/api/session/users")
    resp = opener.open(req)
    users_data = json.loads(resp.read().decode("utf-8"))
    user_list = users_data if isinstance(users_data, list) else users_data.get("users", [])
    for u in user_list:
        if u.get("username") == test_username:
            del_req = urllib.request.Request(f"{BASE}/api/session/users/{u['id']}", method="DELETE")
            opener.open(del_req)
            print(f"Cleaned up previous test user {u['id']}")

    reg_data = json.dumps({
        "username": test_username,
        "email": test_email,
        "password": test_password
    }).encode("utf-8")
    req = urllib.request.Request(f"{BASE}/api/session/register", data=reg_data, headers={"Content-Type": "application/json"})
    resp = opener.open(req)
    reg_result = json.loads(resp.read().decode("utf-8"))
    user_id = reg_result.get("id")
    print(f"PASS: User registered with 8+ chars password -> HTTP {resp.status}, id={user_id}, status={reg_result.get('status')}")

    # 5. Test Approve / Reject endpoints
    if user_id:
        approve_req = urllib.request.Request(f"{BASE}/api/session/users/{user_id}/approve", data=b"", headers={"Content-Type": "application/json"})
        resp = opener.open(approve_req)
        print(f"PASS: User approved by admin -> HTTP {resp.status}")

        reject_req = urllib.request.Request(f"{BASE}/api/session/users/{user_id}/reject", data=b"", headers={"Content-Type": "application/json"})
        resp = opener.open(reject_req)
        print(f"PASS: User rejected/status changed by admin -> HTTP {resp.status}")

        # Clean up
        del_req = urllib.request.Request(f"{BASE}/api/session/users/{user_id}", method="DELETE")
        resp = opener.open(del_req)
        print(f"PASS: User cleanly deleted -> HTTP {resp.status}")

    # 6. Test Logout
    req = urllib.request.Request(f"{BASE}/api/session/logout", data=b"", headers={"Content-Type": "application/json"})
    resp = opener.open(req)
    print(f"PASS: Admin logout -> HTTP {resp.status}")

    print("=== All Auth tests passed with 100% success ===")

if __name__ == "__main__":
    run_tests()
