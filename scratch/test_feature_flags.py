import urllib.request
import urllib.parse
import http.cookiejar
import json

BASE_URL = "http://localhost:8080"
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

# 1. Login as Admin
login_data = json.dumps({
    "username": "admin@yespanchi.com",
    "password": "YP2025-2026yp"
}).encode('utf-8')

req = urllib.request.Request(f"{BASE_URL}/api/session/login", data=login_data, headers={"Content-Type": "application/json"})
with opener.open(req) as resp:
    print(f"Admin Login: {resp.status}")

# 2. Get list of flags
req = urllib.request.Request(f"{BASE_URL}/api/feature-flags")
with opener.open(req) as resp:
    flags_data = json.loads(resp.read().decode('utf-8'))
    flags = flags_data.get("flags", [])
    print(f"Total feature flags found: {len(flags)}")
    for f in flags[:10]:
        print(f" - [{f.get('id')}] enabled={f.get('enabled')}, locked={f.get('locked')}, source={f.get('source')}, registered={f.get('registered')}")

# 3. Test toggling an unlockable flag
# Pick a flag that is not locked
toggle_flag = next((f for f in flags if not f.get("locked") and f.get("registered")), None)
if toggle_flag:
    flag_id = toggle_flag["id"]
    original_val = toggle_flag["enabled"]
    new_val = not original_val
    print(f"\nTesting toggle on flag: {flag_id} from {original_val} to {new_val}")
    
    put_data = json.dumps({"enabled": new_val}).encode('utf-8')
    put_req = urllib.request.Request(f"{BASE_URL}/api/feature-flags/{urllib.parse.quote(flag_id)}", data=put_data, headers={"Content-Type": "application/json"}, method="PUT")
    with opener.open(put_req) as put_resp:
        updated = json.loads(put_resp.read().decode('utf-8'))
        print(f"Toggle response status: {put_resp.status}, New state: {updated.get('enabled')}")
        assert updated.get("enabled") == new_val, "Flag state did not match expected!"

    # Revert back to original
    revert_data = json.dumps({"enabled": original_val}).encode('utf-8')
    rev_req = urllib.request.Request(f"{BASE_URL}/api/feature-flags/{urllib.parse.quote(flag_id)}", data=revert_data, headers={"Content-Type": "application/json"}, method="PUT")
    with opener.open(rev_req) as rev_resp:
        reverted = json.loads(rev_resp.read().decode('utf-8'))
        print(f"Revert response status: {rev_resp.status}, Reverted state: {reverted.get('enabled')}")
        assert reverted.get("enabled") == original_val, "Flag state did not revert!"
    print("Toggle and Revert SUCCESS!")
else:
    print("No unlocked registered flag found to test toggle")

# 4. Error handling test on feature flags:
# Test 4a: Non-existent flag
print("\nTesting Error Handling:")
try:
    bad_req = urllib.request.Request(f"{BASE_URL}/api/feature-flags/non_existent_flag_xyz", data=json.dumps({"enabled": True}).encode('utf-8'), headers={"Content-Type": "application/json"}, method="PUT")
    with opener.open(bad_req) as resp:
        print("Expected error, but got status:", resp.status)
except urllib.error.HTTPError as e:
    print(f"4a. Non-existent flag properly rejected: {e.code} - {e.read().decode('utf-8')}")

# Test 4b: Invalid payload (missing 'enabled')
try:
    bad_payload_req = urllib.request.Request(f"{BASE_URL}/api/feature-flags/{urllib.parse.quote(flags[0]['id'])}", data=json.dumps({"invalid_field": 123}).encode('utf-8'), headers={"Content-Type": "application/json"}, method="PUT")
    with opener.open(bad_payload_req) as resp:
        print("Expected error, but got status:", resp.status)
except urllib.error.HTTPError as e:
    print(f"4b. Missing 'enabled' properly rejected: {e.code} - {e.read().decode('utf-8')}")

# Test 4c: Malformed JSON
try:
    bad_json_req = urllib.request.Request(f"{BASE_URL}/api/feature-flags/{urllib.parse.quote(flags[0]['id'])}", data=b"{bad_json", headers={"Content-Type": "application/json"}, method="PUT")
    with opener.open(bad_json_req) as resp:
        print("Expected error, but got status:", resp.status)
except urllib.error.HTTPError as e:
    print(f"4c. Malformed JSON properly rejected: {e.code} - {e.read().decode('utf-8')}")
