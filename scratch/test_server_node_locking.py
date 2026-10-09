#!/usr/bin/env python3
"""
Test Enterprise Node-Locking Enforcement:
1. Query Server Hardware ID via API (/api/browser-ai/license/server-id)
2. Issue a License locked to THIS server ID -> Activation must SUCCEED (200)
3. Issue a License locked to a MISMATCHED server ID (SRV-ROGUE-9999) -> Activation must FAIL with LICENSE_SERVER_MISMATCH (400)
4. Issue a Floating License (no server ID) -> Activation must SUCCEED (200)
"""

import sys
import json
import urllib.request
import urllib.error
import http.cookiejar
from pathlib import Path

# Add tools/license to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "tools" / "license"))
from issue_license import issue_license

BASE_URL = "http://127.0.0.1:8080"
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def login():
    url = f"{BASE_URL}/api/session/login"
    data = json.dumps({"username": ADMIN_USER, "password": ADMIN_PASS}).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")
    with opener.open(req) as resp:
        return resp.getcode() == 200

def get_ids():
    url = f"{BASE_URL}/api/browser-ai/license/server-id"
    req = urllib.request.Request(url)
    with opener.open(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        print(f"[1] Server and database IDs: {data}")
        return data.get("server_hardware_id"), data.get("install_id")

def activate_license(lic_path):
    url = f"{BASE_URL}/api/browser-ai/license/activate"
    with open(lic_path, "r", encoding="utf-8") as f:
        content = f.read()
    
    req_body = json.dumps({"license_key": content}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=req_body,
        headers={"Content-Type": "application/json", "X-Role": "admin", "X-User-Email": "admin@yespanchi.com"}
    )
    try:
        with opener.open(req) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return resp.status, data
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            data = json.loads(err_body)
        except Exception:
            data = {"raw": err_body}
        return e.code, data

def main():
    print("=== Testing Enterprise Server Hardware Node-Locking ===")
    assert login(), "Admin login failed"
    print("[*] Logged in as Admin successfully.")

    server_id, install_id = get_ids()
    assert server_id and server_id.startswith("SRV-"), f"Invalid server ID format: {server_id}"
    assert install_id and install_id.startswith("DB-"), f"Invalid install ID format: {install_id}"
    print(f"[*] Verified host {server_id} database {install_id}")

    # TEST A: Valid Node-Locked License for this Host
    print("\n--- Test A: Activate License Locked to Current Host ID ---")
    res_a = issue_license(
        client_name="Titan Secure Bank",
        seats=500,
        expiry_date="2028-12-31",
        server_id=server_id,
        install_id=install_id,
        output_path="scratch/license_titan_locked.lic"
    )
    status_a, data_a = activate_license("scratch/license_titan_locked.lic")
    print(f"Status: {status_a}")
    print(f"Response: {data_a}")
    assert status_a == 200, f"Expected 200, got {status_a}"
    assert data_a["license"]["is_hardware_bound"] is True, "Expected is_hardware_bound == True"
    assert data_a["license"]["server_hardware_id"] == server_id, "Server Hardware ID mismatch"
    print("[PASS] Test A Passed: Node-locked license activated successfully!")

    # TEST B: Fraudulent Server ID (e.g. Copied to another server)
    print("\n--- Test B: Activate License Locked to WRONG Server ID ---")
    res_b = issue_license(
        client_name="Titan Rogue Deployment",
        seats=500,
        expiry_date="2028-12-31",
        server_id="SRV-BAD00000-00000000",
        install_id=install_id,
        output_path="scratch/license_titan_rogue.lic"
    )
    status_b, data_b = activate_license("scratch/license_titan_rogue.lic")
    print(f"Status: {status_b}")
    print(f"Response: {data_b}")
    assert status_b == 400, f"Expected 400 for server mismatch, got {status_b}"
    error_msg = str(data_b.get("error", ""))
    assert "LICENSE_SERVER_MISMATCH" in error_msg, f"Expected LICENSE_SERVER_MISMATCH error, got: {error_msg}"
    print("[PASS] Test B Passed: Rogue server reuse rejected with LICENSE_SERVER_MISMATCH!")

    # TEST C: A key without database id cannot be issued
    print("\n--- Test C: Reject license that is not locked to a database ---")
    try:
        issue_license(
            client_name="Floating Standard Corp",
            seats=100,
            expiry_date="2028-12-31",
            server_id=server_id,
            install_id="",
            output_path="scratch/license_floating.lic"
        )
        raise AssertionError("expected install_id to be required")
    except ValueError as exc:
        assert "install_id" in str(exc), exc
    print("[PASS] Test C Passed: unbound license is rejected.")

    print("\n" + "="*60)
    print(" ALL NODE-LOCKING TESTS PASSED PERFECTLY! [100% SUCCESS]")
    print("="*60)

if __name__ == "__main__":
    main()
