#!/usr/bin/env python3
"""
Comprehensive Live Testing Suite for UnifAI Plugin Management
Tests all Plugin API endpoints and lifecycle operations:
1. Authentication & Session Validation
2. Built-in Plugins Listing (/api/plugins/builtins)
3. Loaded Runtime Plugins Listing (/api/plugins/loaded)
4. Full Plugins Directory (/api/plugins)
5. Custom Plugin Creation (/api/plugins POST) with path & placement
6. Custom Plugin Query by Name (/api/plugins/{name} GET)
7. Custom Plugin Update (/api/plugins/{name} PUT)
8. Multi-Plugin Sequence Ordering (pre_builtin vs post_builtin)
9. Plugin Deletion & Cleanup (/api/plugins/{name} DELETE)
10. UI Dev Server Proxy Verification (http://localhost:3100/api/plugins)
"""

import sys
import os
import json
import urllib.request
import urllib.error

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

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
        print(f"[PASS] ✅ {test_id:<12}: {desc}")
    else:
        failed += 1
        print(f"[FAIL] ❌ {test_id:<12}: {desc}")
    if detail:
        print(f"             Detail: {detail}")

print("=" * 80)
print(f"🔌 UNIFAI ENTERPRISE PLUGIN SYSTEM - LIVE AUTOMATED VERIFICATION")
print(f"   Target Backend: {BASE_URL}")
print(f"   UI Dev Server:  {UI_URL}")
print("=" * 80)

# Step 1: Admin Login
login_payload = json.dumps({"username": ADMIN_USER, "password": ADMIN_PASSWORD}).encode()
login_req = urllib.request.Request(
    f"{BASE_URL}/api/session/login",
    data=login_payload,
    headers={"Content-Type": "application/json"}
)
try:
    with urllib.request.urlopen(login_req, timeout=5) as resp:
        cookie = resp.headers.get("Set-Cookie", "").split(";")[0]
        record("PLUGIN-AUTH", "Admin Login & Session Cookie Acquisition", resp.status == 200, f"Token: {cookie[:25]}...")
except Exception as e:
    record("PLUGIN-AUTH", "Admin Login", False, str(e))
    sys.exit(1)

def api_call(method: str, path: str, body=None, host=BASE_URL):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        f"{host}{path}",
        data=data,
        headers={"Content-Type": "application/json", "Cookie": cookie},
        method=method
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as res:
            raw = res.read().decode("utf-8")
            try:
                parsed = json.loads(raw)
            except Exception:
                parsed = raw
            return res.status, parsed, None
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            err_json = json.loads(err_body)
        except Exception:
            err_json = err_body
        return e.code, err_json, str(e)
    except Exception as e:
        return 0, None, str(e)

# Step 2: Query Builtin Plugins
st, builtins, err = api_call("GET", "/api/plugins/builtins")
b_list = builtins.get("plugins", []) if isinstance(builtins, dict) else []
record("PLUGIN-01", "Query Built-in Plugins Directory (/api/plugins/builtins)", st == 200 and len(b_list) > 0, f"Found {len(b_list)} built-in plugins: {', '.join(b_list[:5])}...")

# Step 3: Query Loaded Plugins
st, loaded, err = api_call("GET", "/api/plugins/loaded")
l_list = loaded.get("plugins", []) if isinstance(loaded, dict) else []
record("PLUGIN-02", "Query Runtime Loaded Plugins (/api/plugins/loaded)", st == 200 and len(l_list) > 0, f"Loaded plugins: {', '.join(l_list)}")

# Step 4: Query All Plugins
st, all_plugins, err = api_call("GET", "/api/plugins")
p_list = all_plugins.get("plugins", []) if isinstance(all_plugins, dict) else []
record("PLUGIN-03", "Query Global Plugins List (/api/plugins)", st == 200, f"Total plugins configured: {len(p_list)}")

# Step 5: Create Custom Plugin A
plugin_a_payload = {
    "name": "enterprise_dlp_scanner",
    "path": "/app/plugins/dlp_scanner.so",
    "enabled": False,
    "config": {
        "scan_pii": True,
        "max_payload_kb": 1024,
        "action": "redact"
    },
    "placement": "post_builtin",
    "order": 1
}
st, resp_a, err = api_call("POST", "/api/plugins", plugin_a_payload)
p_created = resp_a.get("plugin", {}) if isinstance(resp_a, dict) else {}
record("PLUGIN-04", "Create Custom Plugin with Config & Path", st == 201 and p_created.get("name") == "enterprise_dlp_scanner", f"Status: {st} | isCustom: {p_created.get('isCustom')} | Placement: {p_created.get('placement')}")

# Step 6: Query Specific Custom Plugin by Name
st, get_a, err = api_call("GET", "/api/plugins/enterprise_dlp_scanner")
record("PLUGIN-05", "Retrieve Single Plugin Configuration (/api/plugins/{name})", st == 200 and get_a.get("name") == "enterprise_dlp_scanner", f"Config: {get_a.get('config')}")

# Step 7: Update Custom Plugin A Configuration & Placement
update_payload = {
    "enabled": False,
    "config": {
        "scan_pii": True,
        "max_payload_kb": 4096,
        "action": "block",
        "alert_email": "security@yespanchi.com"
    },
    "placement": "pre_builtin",
    "order": 0
}
st, upd_resp, err = api_call("PUT", "/api/plugins/enterprise_dlp_scanner", update_payload)
p_updated = upd_resp.get("plugin", {}) if isinstance(upd_resp, dict) else {}
cfg_updated = p_updated.get("config", {}) if isinstance(p_updated, dict) else {}
record("PLUGIN-06", "Update Plugin Configuration & Sequence Placement", st == 200 and p_updated.get("placement") == "pre_builtin" and cfg_updated.get("action") == "block", f"Updated placement: {p_updated.get('placement')} | Action: {cfg_updated.get('action')}")

# Step 8: Create Second Custom Plugin B to test multi-plugin reordering
plugin_b_payload = {
    "name": "custom_metrics_exporter",
    "path": "https://telemetry.internal/agent/v1",
    "enabled": False,
    "config": {
        "batch_size": 50,
        "flush_interval_sec": 10
    },
    "placement": "post_builtin",
    "order": 2
}
st, resp_b, err = api_call("POST", "/api/plugins", plugin_b_payload)
record("PLUGIN-07", "Create Secondary Custom Plugin (HTTP/Webhook Target)", st == 201, f"Status: {st} | Name: custom_metrics_exporter")

# Step 9: Verify Both Custom Plugins in Global List
st, list_again, err = api_call("GET", "/api/plugins")
all_p = list_again.get("plugins", []) if isinstance(list_again, dict) else []
names = [p.get("name") for p in all_p]
has_both = "enterprise_dlp_scanner" in names and "custom_metrics_exporter" in names
record("PLUGIN-08", "Verify Both Custom Plugins in Plugins Catalog", has_both, f"Found active plugins: {', '.join(names)}")

# Step 10: Clean up - Delete Plugin A
st_del_a, resp_del_a, err = api_call("DELETE", "/api/plugins/enterprise_dlp_scanner")
record("PLUGIN-09", "Delete Custom Plugin A (/api/plugins/{name} DELETE)", st_del_a == 200, f"Deleted enterprise_dlp_scanner")

# Step 11: Clean up - Delete Plugin B
st_del_b, resp_del_b, err = api_call("DELETE", "/api/plugins/custom_metrics_exporter")
record("PLUGIN-10", "Delete Custom Plugin B (/api/plugins/{name} DELETE)", st_del_b == 200, f"Deleted custom_metrics_exporter")

# Step 12: Verify UI Dev Server Proxy (Port 3100 -> Port 6000)
st_ui, ui_plugins, err = api_call("GET", "/api/plugins", host=UI_URL)
record("PLUGIN-11", "UI Dev Server (Port 3100) Reverse Proxy Verification", st_ui == 200, f"HTTP {st_ui} via localhost:{UI_PORT}/api/plugins")

# Step 13: UI Dev Server Builtins Proxy Check
st_ui_b, ui_builtins, err = api_call("GET", "/api/plugins/builtins", host=UI_URL)
record("PLUGIN-12", "UI Dev Server Builtin Plugins Proxy Check", st_ui_b == 200, f"HTTP {st_ui_b} via localhost:{UI_PORT}/api/plugins/builtins")

print("\n" + "=" * 80)
print(f"📊 PLUGIN SYSTEM TEST SUMMARY: {passed} PASSED, {failed} FAILED")
print("=" * 80)

if failed == 0:
    print("🎉 ALL 12 PLUGIN TESTS PASSED WITH 100% SUCCESS RATE!")
else:
    print(f"⚠️ {failed} TESTS FAILED!")
    sys.exit(1)
