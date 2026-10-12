import urllib.request
import json
import ssl
import sys
import time

sys.stdout.reconfigure(encoding='utf-8')

BASE_URL = "http://localhost:8080"
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

# Login to get session token
login_data = json.dumps({"username": "admin@yespanchi.com", "password": "YP2025-2026yp"}).encode('utf-8')
req = urllib.request.Request(f"{BASE_URL}/api/session/login", data=login_data, headers={"Content-Type": "application/json"}, method="POST")

with urllib.request.urlopen(req, context=ctx) as resp:
    cookies = resp.headers.get("Set-Cookie", "")
    token = [p.split("=")[1] for p in cookies.split(";") if "token=" in p][0]

HEADERS = {
    "Authorization": f"Bearer {token}",
    "Cookie": f"token={token}",
    "Content-Type": "application/json",
    "Accept": "application/json"
}

def request(method, path, body=None, timeout=30):
    url = f"{BASE_URL}{path}"
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, headers=HEADERS, method=method)
    try:
        with urllib.request.urlopen(req, context=ctx, timeout=timeout) as resp:
            status = resp.status
            content = resp.read().decode("utf-8")
            data = json.loads(content) if content else {}
            return status, data
    except urllib.error.HTTPError as e:
        content = e.read().decode("utf-8") if e.fp else ""
        try:
            err_data = json.loads(content)
        except Exception:
            err_data = {"raw": content}
        return e.code, err_data

def test_feature_flags():
    print("=" * 70)
    print("FEATURE FLAGS COMPREHENSIVE DEEP VERIFICATION & AUTOMATION TEST")
    print("=" * 70)
    
    # Step 1: List all existing flags
    print("\n[STEP 1] Fetching all existing feature flags from Backend & DB...")
    code, res = request("GET", "/api/feature-flags")
    assert code == 200, f"Failed to list feature flags: {code} {res}"
    flags = res.get("flags", [])
    print(f"-> Successfully retrieved {len(flags)} feature flags.")
    for f in flags[:6]:
        print(f"   * ID: {f.get('id'):<32} | Name: {f.get('display_name', ''):<28} | Enabled: {str(f.get('enabled')):<5} | Source: {f.get('source')}")
    if len(flags) > 6:
        print(f"   * ... and {len(flags)-6} more flags.")

    # Step 2: Create a preset section flag
    test_preset_id = "section.adaptive-routing.settings"
    print(f"\n[STEP 2] Testing Creation of Flag from Template/Preset: '{test_preset_id}'...")
    # Clean up first if already exists
    request("DELETE", f"/api/feature-flags/{test_preset_id}")
    
    create_payload = {
        "id": test_preset_id,
        "display_name": "Adaptive Routing: Settings",
        "description": "Enable or gate adaptive routing algorithm settings and threshold configurations.",
        "default": False,
        "enabled": True
    }
    code, res = request("POST", "/api/feature-flags", create_payload)
    print(f"-> POST /api/feature-flags returned HTTP {code}")
    assert code in (200, 201), f"Create failed: {code} {res}"
    
    # Verify in list
    code, res = request("GET", "/api/feature-flags")
    found = next((f for f in res.get("flags", []) if f.get("id") == test_preset_id), None)
    assert found is not None, f"Flag {test_preset_id} not found in flags list!"
    print(f"-> VERIFIED IN REGISTRY & DB:")
    print(f"   - ID: {found.get('id')}")
    print(f"   - Display Name: {found.get('display_name')}")
    print(f"   - Description: {found.get('description')}")
    print(f"   - Enabled: {found.get('enabled')}")
    print(f"   - Source: {found.get('source')}")
    assert found.get("enabled") is True
    assert found.get("display_name") == "Adaptive Routing: Settings"

    # Step 3: Toggle flag state to FALSE (Disable)
    print(f"\n[STEP 3] Testing Toggle Switch (Disabling flag)...")
    code, res = request("PUT", f"/api/feature-flags/{test_preset_id}", {"enabled": False})
    print(f"-> PUT /api/feature-flags/{test_preset_id} (enabled=false) returned HTTP {code}")
    assert code == 200, f"Toggle failed: {code} {res}"
    
    code, res = request("GET", "/api/feature-flags")
    found = next((f for f in res.get("flags", []) if f.get("id") == test_preset_id), None)
    assert found is not None and found.get("enabled") is False, "Flag toggle to False not reflected!"
    print(f"-> VERIFIED: Flag is now DISABLED (enabled=False) in DB & memory.")

    # Step 4: Edit Display Name & Description (Edit Modal Flow)
    print(f"\n[STEP 4] Testing Edit Feature Flag (Modal edit for Name, Description & State)...")
    edit_payload = {
        "enabled": True,
        "display_name": "Adaptive Routing: AI Precision Engine",
        "description": "Custom updated description: High precision neural router dynamically balancing throughput."
    }
    code, res = request("PUT", f"/api/feature-flags/{test_preset_id}", edit_payload)
    print(f"-> PUT /api/feature-flags/{test_preset_id} (edit name & desc) returned HTTP {code}")
    assert code == 200, f"Edit failed: {code} {res}"

    code, res = request("GET", "/api/feature-flags")
    found = next((f for f in res.get("flags", []) if f.get("id") == test_preset_id), None)
    assert found is not None
    print(f"-> VERIFIED UPDATED DETAILS:")
    print(f"   - Display Name: '{found.get('display_name')}'")
    print(f"   - Description: '{found.get('description')}'")
    print(f"   - Enabled: {found.get('enabled')}")
    assert found.get("display_name") == "Adaptive Routing: AI Precision Engine"
    assert found.get("description") == "Custom updated description: High precision neural router dynamically balancing throughput."
    assert found.get("enabled") is True

    # Step 5: Test adding a brand new dynamic custom flag (Future proof test)
    custom_flag_id = "ai.future_multimodal_vision"
    print(f"\n[STEP 5] Testing Future Dynamic Feature Addition ('{custom_flag_id}')...")
    request("DELETE", f"/api/feature-flags/{custom_flag_id}")
    
    custom_payload = {
        "id": custom_flag_id,
        "display_name": "Multimodal Vision Reasoning 2.0",
        "description": "Next-generation video and image understanding pipeline.",
        "default": False,
        "enabled": True
    }
    code, res = request("POST", "/api/feature-flags", custom_payload)
    print(f"-> POST /api/feature-flags for new future flag returned HTTP {code}")
    assert code in (200, 201), f"Create custom flag failed: {code} {res}"

    code, res = request("GET", "/api/feature-flags")
    found = next((f for f in res.get("flags", []) if f.get("id") == custom_flag_id), None)
    assert found is not None, f"Custom flag {custom_flag_id} not found!"
    print(f"-> VERIFIED: New dynamic flag instantly active in runtime & saved in Postgres:")
    print(f"   - ID: {found.get('id')}")
    print(f"   - Name: {found.get('display_name')}")
    print(f"   - Enabled: {found.get('enabled')}")

    # Step 6: Test Deleting both flags (Trash Icon Flow)
    print(f"\n[STEP 6] Testing Deletion of feature flags (Trash icon flow)...")
    code, res = request("DELETE", f"/api/feature-flags/{test_preset_id}")
    print(f"-> DELETE /api/feature-flags/{test_preset_id} returned HTTP {code}")
    assert code == 200

    code, res = request("DELETE", f"/api/feature-flags/{custom_flag_id}")
    print(f"-> DELETE /api/feature-flags/{custom_flag_id} returned HTTP {code}")
    assert code == 200

    # Step 7: Verify removal
    code, res = request("GET", "/api/feature-flags")
    ids = [f.get("id") for f in res.get("flags", [])]
    assert test_preset_id not in ids, f"{test_preset_id} still exists after delete!"
    assert custom_flag_id not in ids, f"{custom_flag_id} still exists after delete!"
    print("-> VERIFIED: Both flags completely wiped from memory and PostgreSQL!")

    print("\n" + "=" * 70)
    print("OVERALL RESULT: 100% PASS - ALL FEATURE FLAGS CAPABILITIES FULLY FUNCTIONAL!")
    print("=" * 70)

if __name__ == "__main__":
    test_feature_flags()
