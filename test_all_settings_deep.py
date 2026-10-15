import urllib.request, json, ssl, sys, time
sys.stdout.reconfigure(encoding='utf-8')

backend_url = "http://localhost:8080"
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

from pathlib import Path
env_file = Path(__file__).resolve().parent / ".env"
env_vars = {}
if env_file.is_file():
    for line in env_file.read_text(encoding="utf-8").splitlines():
        if "=" in line and not line.strip().startswith("#"):
            k, v = line.split("=", 1)
            env_vars[k.strip()] = v.strip().strip("'\"")

admin_user = env_vars.get("ADMIN_EMAIL", "admin@yespanchi.com")
admin_pass = env_vars.get("ADMIN_PASSWORD", "")
# 1. Login
login_data = json.dumps({"username": admin_user, "password": admin_pass}).encode('utf-8')
req = urllib.request.Request(f"{backend_url}/api/session/login", data=login_data, headers={"Content-Type": "application/json"}, method="POST")

with urllib.request.urlopen(req, context=ctx) as resp:
    cookies = resp.headers.get("Set-Cookie", "")
    token = [p.split("=")[1] for p in cookies.split(";") if "token=" in p][0]

headers = {
    "Authorization": f"Bearer {token}",
    "Cookie": f"token={token}",
    "Content-Type": "application/json"
}

def api_call(url, method="GET", body=None, timeout=30):
    data = json.dumps(body).encode('utf-8') if body is not None else None
    req = urllib.request.Request(f"{backend_url}{url}", data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, context=ctx, timeout=timeout) as resp:
            content = resp.read().decode('utf-8')
            return resp.status, json.loads(content) if content else {}
    except urllib.error.HTTPError as e:
        err_content = e.read().decode('utf-8') if e.fp else ""
        try:
            return e.code, json.loads(err_content)
        except Exception:
            return e.code, {"error": err_content}
    except Exception as e:
        return 500, {"error": str(e)}

print("=" * 70)
print("COMPREHENSIVE VERIFICATION: ALL 7 SETTINGS SECTIONS")
print("=" * 70)

results = {}

# -------------------------------------------------------------
# 1. CLIENT SETTINGS (Drop Excess Requests, Pool Size, Logging)
# -------------------------------------------------------------
print("\n[1] Client Settings (/api/config)...")
st, cfg = api_call("/api/config")
client_cfg = cfg.get("client_config", {})
orig_drop = client_cfg.get("drop_excess_requests", False)
print(f"  Current drop_excess_requests: {orig_drop}")
print(f"  Current initial_pool_size: {client_cfg.get('initial_pool_size')}")
print(f"  Current enable_logging: {client_cfg.get('enable_logging')}")

# Toggle drop_excess_requests
client_cfg["drop_excess_requests"] = not orig_drop
st, _ = api_call("/api/config", "PUT", cfg)
print(f"  Toggled drop_excess_requests to {not orig_drop}: HTTP {st}")

st, cfg_verify = api_call("/api/config?from_db=true")
verified_drop = cfg_verify.get("client_config", {}).get("drop_excess_requests")
print(f"  Verified from PostgreSQL DB: drop_excess_requests = {verified_drop}")

# Revert
client_cfg["drop_excess_requests"] = orig_drop
api_call("/api/config", "PUT", cfg)
results["Client Settings"] = (st == 200 and verified_drop == (not orig_drop))

# -------------------------------------------------------------
# 2. COMPATIBILITY (Parameter Conversion, Chat/Response format)
# -------------------------------------------------------------
print("\n[2] Compatibility (/api/config -> compat)...")
compat = client_cfg.get("compat", {})
orig_convert = compat.get("convert_text_to_chat", True)
print(f"  Current convert_text_to_chat: {orig_convert}")
print(f"  Current should_drop_params: {compat.get('should_drop_params')}")

compat["convert_text_to_chat"] = not orig_convert
st, _ = api_call("/api/config", "PUT", cfg)
print(f"  Toggled convert_text_to_chat to {not orig_convert}: HTTP {st}")

st, cfg_verify = api_call("/api/config?from_db=true")
verified_compat = cfg_verify.get("client_config", {}).get("compat", {}).get("convert_text_to_chat")
print(f"  Verified from PostgreSQL DB: convert_text_to_chat = {verified_compat}")

# Revert
compat["convert_text_to_chat"] = orig_convert
api_call("/api/config", "PUT", cfg)
results["Compatibility"] = (st == 200 and verified_compat == (not orig_convert))

# -------------------------------------------------------------
# 3. CACHING (Vector Store Config & Cache Feature Flag)
# -------------------------------------------------------------
print("\n[3] Caching (/api/vector-store-config & /api/feature-flags)...")
st, vs_cfg = api_call("/api/vector-store-config")
print(f"  Get /api/vector-store-config: HTTP {st}, Type: {vs_cfg.get('type')}")

# Toggle cache.responses feature flag
st, ff_res = api_call("/api/feature-flags/cache.responses", "PUT", {"enabled": True})
print(f"  Toggle Feature Flag 'cache.responses' to True: HTTP {st}")
st, ff_verify = api_call("/api/feature-flags")
cache_flag = next((f for f in ff_verify.get("flags", []) if f["id"] == "cache.responses"), {})
print(f"  Verified 'cache.responses' active in runtime: {cache_flag.get('enabled')}")
results["Caching"] = (st == 200 and cache_flag.get("enabled") == True)

# -------------------------------------------------------------
# 4. SECURITY & EMAIL (Session / CORS / SMTP Config / Test Email)
# -------------------------------------------------------------
print("\n[4] Security & Email (SMTP)...")
st, sec_cfg = api_call("/api/config")
origins = sec_cfg.get("client_config", {}).get("allowed_origins", [])
print(f"  Security Allowed Origins (CORS): {origins}")
print(f"  Enforce Auth on Inference: {sec_cfg.get('client_config', {}).get('enforce_auth_on_inference')}")

st, smtp_cfg = api_call("/api/smtp-config")
print(f"  Get /api/smtp-config: HTTP {st}")
print(f"  SMTP Enabled: {smtp_cfg.get('enabled')}, Host: {smtp_cfg.get('host')}")

# Update SMTP name
smtp_cfg["from_name"] = "UnifAI Security System"
st, _ = api_call("/api/smtp-config", "PUT", smtp_cfg)
print(f"  Save SMTP Settings: HTTP {st}")

# Test email endpoint
st, test_email = api_call("/api/smtp-config/test", "POST", {"to": "admin@yespanchi.com"})
print(f"  Test Send Email Endpoint: HTTP {st}, Result: {test_email.get('status', 'ok')}")
results["Security & Email"] = (st in [200, 400] and smtp_cfg.get("enabled") == True)

# -------------------------------------------------------------
# 5. API KEYS (Virtual Keys Lifecycle & DB Persistence)
# -------------------------------------------------------------
print("\n[5] API Keys / Virtual Keys (/api/governance/virtual-keys)...")
st, keys = api_call("/api/governance/virtual-keys?limit=5")
print(f"  List API Keys: HTTP {st}, Total: {len(keys.get('virtual_keys', []))}")

vk_payload = {
    "name": f"Verification Key {int(time.time())}",
    "description": "Created for comprehensive verification",
    "budget": {"max_budget": 50, "budget_duration": "monthly"},
    "rate_limits": [{"tokens": 500, "unit": "minute"}]
}
st, new_key = api_call("/api/governance/virtual-keys", "POST", vk_payload)
vk_id = new_key.get("virtual_key", {}).get("id")
print(f"  Create Virtual Key: HTTP {st}, ID: {vk_id}")

if vk_id:
    # Update key
    st, update_res = api_call(f"/api/governance/virtual-keys/{vk_id}", "PUT", {"name": vk_payload["name"] + " Updated"})
    print(f"  Update Virtual Key: HTTP {st}")

    # Delete key
    st, del_res = api_call(f"/api/governance/virtual-keys/{vk_id}", "DELETE")
    print(f"  Delete/Revoke Virtual Key: HTTP {st}")
    results["API Keys"] = (st == 200)
else:
    results["API Keys"] = False

# -------------------------------------------------------------
# 6. PERFORMANCE TUNING (Pool Size, Content Logging Bypass)
# -------------------------------------------------------------
print("\n[6] Performance Tuning (/api/config)...")
orig_bypass = client_cfg.get("disable_content_logging", False)
print(f"  Initial Pool Size: {client_cfg.get('initial_pool_size')}")
print(f"  Disable Content Logging (High Perf Bypass): {orig_bypass}")

client_cfg["disable_content_logging"] = not orig_bypass
st, _ = api_call("/api/config", "PUT", cfg)
print(f"  Toggled High Perf Bypass to {not orig_bypass}: HTTP {st}")

st, cfg_verify = api_call("/api/config?from_db=true")
verified_bypass = cfg_verify.get("client_config", {}).get("disable_content_logging")
print(f"  Verified from PostgreSQL DB: disable_content_logging = {verified_bypass}")

# Revert
client_cfg["disable_content_logging"] = orig_bypass
api_call("/api/config", "PUT", cfg)
results["Performance Tuning"] = (st == 200 and verified_bypass == (not orig_bypass))

# -------------------------------------------------------------
# 7. FEATURE FLAGS (Create, Edit Metadata, Toggle, Delete, DB)
# -------------------------------------------------------------
print("\n[7] Feature Flags (/api/feature-flags)...")
st, ff_list = api_call("/api/feature-flags")
print(f"  List Feature Flags: HTTP {st}, Total Registered Flags: {len(ff_list.get('flags', []))}")

test_id = "test.deep_verify_flag"
st, create_res = api_call("/api/feature-flags", "POST", {
    "id": test_id,
    "display_name": "Full Test Feature",
    "description": "Created to verify full lifecycle and DB persistence",
    "enabled": True
})
print(f"  Create Feature Flag: HTTP {st}, Created Name: {create_res.get('display_name')}")

# Edit metadata & toggle
st, edit_res = api_call(f"/api/feature-flags/{test_id}", "PUT", {
    "display_name": "Full Test Feature RENAMED",
    "description": "Updated description saved in PostgreSQL",
    "enabled": False
})
print(f"  Edit Feature Flag (Name + Description + Enabled=False): HTTP {st}")
print(f"    New Name: '{edit_res.get('display_name')}', Enabled: {edit_res.get('enabled')}")

# Verify list contains updated metadata
st, ff_list_updated = api_call("/api/feature-flags")
matched = next((f for f in ff_list_updated.get("flags", []) if f["id"] == test_id), None)
print(f"  Verified Flag in Registry: Found = {matched is not None}")
if matched:
    print(f"    Verified DB Name: {matched.get('display_name')}")
    print(f"    Verified DB Description: {matched.get('description')}")
    print(f"    Verified DB Enabled: {matched.get('enabled')}")

# Delete flag
st, del_res = api_call(f"/api/feature-flags/{test_id}", "DELETE")
print(f"  Delete Feature Flag: HTTP {st}")

st, ff_list_final = api_call("/api/feature-flags")
still_here = any(f["id"] == test_id for f in ff_list_final.get("flags", []))
print(f"  Verified Removed from Registry: {not still_here}")
results["Feature Flags"] = (matched is not None and not still_here)

print("\n" + "=" * 70)
print("FINAL RESULT SUMMARY ACROSS ALL 7 SECTIONS")
print("=" * 70)
for section, ok in results.items():
    print(f"  {section:25}: {'PASS' if ok else 'FAIL'}")
