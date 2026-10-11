#!/usr/bin/env python3
"""
Comprehensive Live Testing Suite for UnifAI User Provisioning
Verifies both systems under User Provisioning (/workspace/scim):
1. Cloud Identity Providers (SCIM 2.0: Okta, Entra ID, Keycloak)
2. Microsoft Active Directory (On-Premises LDAP / AD DS)
"""

import sys
import os
import json
import urllib.request
import urllib.error

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

sys.path.append(os.path.join(os.path.dirname(__file__)))
from db_config import get_base_url, _CONFIG

BASE_URL = get_base_url()
UI_PORT = _CONFIG.get("UI_PORT", "3100")
UI_URL = f"http://localhost:{UI_PORT}"
ADMIN_USER = _CONFIG.get("ADMIN_EMAIL", "")
ADMIN_PASSWORD = _CONFIG.get("ADMIN_PASSWORD", "")

passed = 0
failed = 0

def record(test_id: str, desc: str, ok: bool, detail: str = ""):
    global passed, failed
    if ok:
        passed += 1
        print(f"[PASS] ✅ {test_id:<14}: {desc}")
    else:
        failed += 1
        print(f"[FAIL] ❌ {test_id:<14}: {desc}")
    if detail:
        print(f"               Detail: {detail}")

print("=" * 80)
print(f"👥 UNIFAI USER PROVISIONING (SCIM & ACTIVE DIRECTORY) - LIVE TESTING")
print(f"   Backend Target: {BASE_URL}")
print(f"   UI Proxy:       {UI_URL}")
print("=" * 80)

# Step 1: Admin Login
login_payload = json.dumps({"username": ADMIN_USER, "password": ADMIN_PASSWORD}).encode()
login_req = urllib.request.Request(
    f"{BASE_URL}/api/session/login",
    data=login_payload,
    headers={"Content-Type": "application/json"}
)
try:
    with urllib.request.urlopen(login_req) as resp:
        cookie = resp.headers.get("Set-Cookie")
        record("AUTH-01", "Admin Session Login", resp.status == 200, f"Status {resp.status}")
except Exception as e:
    record("AUTH-01", "Admin Session Login", False, str(e))
    sys.exit(1)

def req_api(method, path, body=None, extra_headers=None, use_ui=False):
    target = UI_URL if use_ui else BASE_URL
    url = f"{target}{path}"
    headers = {"Content-Type": "application/json"}
    if cookie:
        headers["Cookie"] = cookie
    if extra_headers:
        headers.update(extra_headers)
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request) as resp:
            resp_data = resp.read().decode()
            try:
                parsed = json.loads(resp_data)
            except:
                parsed = resp_data
            return resp.status, parsed
    except urllib.error.HTTPError as e:
        err_body = e.read().decode()
        try:
            parsed = json.loads(err_body)
        except:
            parsed = err_body
        return e.code, parsed
    except Exception as e:
        return 0, str(e)

# ==============================================================================
# SECTION 1: CLOUD SCIM 2.0 PROVISIONING (TAB 1)
# ==============================================================================
print("\n" + "-" * 80)
print("📌 1. CLOUD IDENTITY PROVIDERS (SCIM 2.0)")
print("-" * 80)

# 1.1 Get SCIM Config
status, data = req_api("GET", "/api/scim/config")
record("SCIM-01", "GET /api/scim/config", status == 200, f"Enabled: {data.get('enabled')}")

# 1.2 Get SCIM Providers List
status, data = req_api("GET", "/api/scim/providers")
record("SCIM-02", "GET /api/scim/providers", status == 200, f"Providers: {data}")

# 1.3 Update SCIM Config: Enable with Okta provider
enable_payload = {
    "enabled": True,
    "provider": "okta",
    "config": {
        "domain": "company.okta.com"
    }
}
status, scim_cfg = req_api("PUT", "/api/scim/config", enable_payload)
bearer_token = scim_cfg.get("bearer_token") or scim_cfg.get("config", {}).get("bearer_token", "")
record("SCIM-03", "PUT /api/scim/config (Enable Okta)", status == 200 and scim_cfg.get("enabled") == True, f"Status {status}")
record("SCIM-04", "SCIM Auto-Generated Bearer Token", bool(bearer_token), f"Token: {bearer_token[:8]}..." if bearer_token else "No token")

# 1.4 Test SCIM 2.0 Protocol Endpoints using Bearer Token
scim_headers = {"Authorization": f"Bearer {bearer_token}"}

# ServiceProviderConfig
status, sp_cfg = req_api("GET", "/scim/v2/ServiceProviderConfig", extra_headers=scim_headers)
record("SCIM-05", "GET /scim/v2/ServiceProviderConfig (RFC 7644)", status == 200, f"Status {status}")

# Schemas
status, schemas_resp = req_api("GET", "/scim/v2/Schemas", extra_headers=scim_headers)
record("SCIM-06", "GET /scim/v2/Schemas (RFC 7644)", status == 200, f"Status {status}")

# ResourceTypes
status, rt_resp = req_api("GET", "/scim/v2/ResourceTypes", extra_headers=scim_headers)
record("SCIM-07", "GET /scim/v2/ResourceTypes (RFC 7644)", status == 200, f"Status {status}")

# 1.5 SCIM 2.0 User Provisioning Lifecycle
# Create user
test_scim_user = {
    "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
    "userName": "test.provisioned.user@example.com",
    "name": {
        "givenName": "Test",
        "familyName": "User"
    },
    "emails": [
        {"value": "test.provisioned.user@example.com", "primary": True}
    ],
    "active": True
}
status, create_user_resp = req_api("POST", "/scim/v2/Users", body=test_scim_user, extra_headers=scim_headers)
scim_user_id = create_user_resp.get("id") if isinstance(create_user_resp, dict) else None
record("SCIM-08", "POST /scim/v2/Users (Provision User)", status == 201 and bool(scim_user_id), f"Created User ID: {scim_user_id}")

# Query user by username filter
filter_path = "/scim/v2/Users?filter=userName%20eq%20%22test.provisioned.user@example.com%22"
status, find_user_resp = req_api("GET", filter_path, extra_headers=scim_headers)
total_results = find_user_resp.get("totalResults") if isinstance(find_user_resp, dict) else 0
record("SCIM-09", "GET /scim/v2/Users?filter (Query User)", status == 200 and total_results >= 1, f"Total: {total_results}")

# Deprovision / Delete user
if scim_user_id:
    status, del_resp = req_api("DELETE", f"/scim/v2/Users/{scim_user_id}", extra_headers=scim_headers)
    record("SCIM-10", "DELETE /scim/v2/Users/{id} (Deprovision User)", status == 204 or status == 200, f"Status {status}")
else:
    record("SCIM-10", "DELETE /scim/v2/Users/{id}", False, "User ID not available")

# Clean up SCIM: restore to clean state
status, _ = req_api("PUT", "/api/scim/config", {"enabled": False, "provider": "okta", "config": {}})
record("SCIM-11", "PUT /api/scim/config (Disable SCIM Cleanly)", status == 200)

# ==============================================================================
# SECTION 2: MICROSOFT ACTIVE DIRECTORY (LDAP) PROVISIONING (TAB 2)
# ==============================================================================
print("\n" + "-" * 80)
print("📌 2. MICROSOFT ACTIVE DIRECTORY (ON-PREMISES LDAP)")
print("-" * 80)

# 2.1 Get AD Config
status, ad_cfg = req_api("GET", "/api/scim/ad/config")
record("AD-01", "GET /api/scim/ad/config", status == 200, f"Server: {ad_cfg.get('server_url')}")

# 2.2 Update AD Config
ad_update_payload = {
    "enabled": False,
    "server_url": "ldap://127.0.0.1:389",
    "domain": "unifai.local",
    "base_dn": "DC=unifai,DC=local",
    "bind_dn": "CN=Administrator,CN=Users,DC=unifai,DC=local",
    "bind_password": "MySecretPassword123!",
    "use_tls": False,
    "insecure_skip_tls": True,
    "user_filter": "(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))",
    "username_attr": "sAMAccountName",
    "email_attr": "mail",
    "display_name_attr": "displayName",
    "default_role": "user"
}
status, ad_saved = req_api("PUT", "/api/scim/ad/config", ad_update_payload)
record("AD-02", "PUT /api/scim/ad/config (Save AD Settings)", status == 200 and ad_saved.get("domain") == "unifai.local", f"Status {status}")

# 2.3 Verify Password Redaction on read
status, ad_check = req_api("GET", "/api/scim/ad/config")
redacted = ad_check.get("bind_password") == "********" or "bind_password" not in ad_check or ad_check.get("bind_password") == ""
record("AD-03", "Password Redaction Protection", status == 200 and redacted, f"Password: {ad_check.get('bind_password')}")

# 2.4 Test Connection Endpoint
# Since 127.0.0.1:389 has no LDAP daemon running locally, test must return a structured JSON failure rather than crashing or HTTP 500
status, test_res = req_api("POST", "/api/scim/ad/test", ad_update_payload)
graceful_test = isinstance(test_res, dict) and "message" in test_res
record("AD-04", "POST /api/scim/ad/test (LDAP Probe)", graceful_test, f"Message: {test_res.get('message')}")

# 2.5 Test User Sync Endpoint (Disabled Protection)
status, sync_res = req_api("POST", "/api/scim/ad/sync")
disabled_msg = "disabled" in str(sync_res).lower()
record("AD-05A", "POST /api/scim/ad/sync (Disabled Guard)", status == 400 and disabled_msg, f"Status {status}: {sync_res.get('error', {}).get('message', sync_res)}")

# 2.6 Test User Sync Endpoint (Enabled Probe)
req_api("PUT", "/api/scim/ad/config", {**ad_update_payload, "enabled": True})
status, sync_res_enabled = req_api("POST", "/api/scim/ad/sync")
probe_msg = "failed to connect" in str(sync_res_enabled).lower() or "success" in str(sync_res_enabled).lower()
record("AD-05B", "POST /api/scim/ad/sync (Live Probe Attempt)", (status == 400 or status == 200) and probe_msg, f"Status {status}: {sync_res_enabled.get('error', {}).get('message', sync_res_enabled)}")

# Reset AD to disabled clean state
req_api("PUT", "/api/scim/ad/config", ad_update_payload)

# ==============================================================================
# SECTION 3: UI DEV SERVER PROXY & PAGE VERIFICATION
# ==============================================================================
print("\n" + "-" * 80)
print("📌 3. UI DEV SERVER PROXY & PAGE RENDERING")
print("-" * 80)

# 3.1 UI Proxy for SCIM Config
status, ui_scim = req_api("GET", "/api/scim/config", use_ui=True)
record("UI-01", f"UI Proxy: GET {UI_URL}/api/scim/config", status == 200, f"Status {status}")

# 3.2 UI Proxy for AD Config
status, ui_ad = req_api("GET", "/api/scim/ad/config", use_ui=True)
record("UI-02", f"UI Proxy: GET {UI_URL}/api/scim/ad/config", status == 200, f"Status {status}")

# 3.3 UI Provisioning Page
page_req = urllib.request.Request(f"{UI_URL}/workspace/scim")
try:
    with urllib.request.urlopen(page_req) as page_resp:
        record("UI-03", f"UI Page: GET {UI_URL}/workspace/scim", page_resp.status == 200, f"Status {page_resp.status}")
except Exception as e:
    record("UI-03", f"UI Page: GET {UI_URL}/workspace/scim", False, str(e))

print("\n" + "=" * 80)
total = passed + failed
print(f"📊 SUMMARY: {passed}/{total} PASSED ({(passed/total)*100:.1f}%)")
if failed == 0:
    print("🎉 ALL USER PROVISIONING FEATURES (SCIM + ACTIVE DIRECTORY) VERIFIED WORKING!")
else:
    print(f"⚠️ {failed} tests failed.")
print("=" * 80)
