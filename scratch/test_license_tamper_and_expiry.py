import json
import base64
import uuid
import datetime
from datetime import timezone
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization
import urllib.request
import urllib.error
import http.cookiejar

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

def activate_license(lic_envelope):
    url = f"{BASE_URL}/api/browser-ai/license/activate"
    data = json.dumps({"license_key": json.dumps(lic_envelope)}).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")
    try:
        with opener.open(req) as resp:
            return resp.getcode(), json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read().decode("utf-8"))

def create_license(client, seats, expiry_iso, priv_key):
    now_iso = datetime.datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    payload = {
        "version": "1.0",
        "license_id": f"TEST-{uuid.uuid4().hex[:8].upper()}",
        "issuer": "YesPanchi Group of Companies",
        "product": "Gateway - Real-time AI Knowledge Screening & Hazard Audit",
        "client_name": client,
        "tier": "Enterprise On-Premise",
        "max_seats": seats,
        "features": ["air_gapped_offline", "browser_ai_guard", "file_redaction", "realtime_dlp"],
        "issued_at": now_iso,
        "expires_at": expiry_iso,
    }
    canonical_json = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8")
    sig = priv_key.sign(canonical_json)
    return {
        "format": "gateway_enterprise_license_v1",
        "payload": payload,
        "payload_b64": base64.b64encode(canonical_json).decode("utf-8"),
        "signature": base64.b64encode(sig).decode("utf-8")
    }

def run_tests():
    print("=== STARTING RIGOROUS LICENSE TAMPER, EXPIRY & SECURITY VERIFICATION ===")
    
    # 1. Login
    if not login():
        print("[-] Failed to login as admin")
        return
    print("[+] 1. Admin authenticated")

    # Load master private key
    with open("tools/license/master_private.key", "rb") as f:
        priv_key = serialization.load_pem_private_key(f.read(), password=None)

    # TEST A: Valid Future License (Valid until 2027)
    future_expiry = "2027-12-31T23:59:59Z"
    valid_lic = create_license("Acme Corp Security Test", 500, future_expiry, priv_key)
    code, res = activate_license(valid_lic)
    print(f"\n[TEST A] Valid License Activation (Expires 2027): HTTP {code}")
    print(f"         Result Message: {res.get('message')}")
    print(f"         Seats: {res.get('license', {}).get('max_seats')}, Active: {res.get('license', {}).get('is_active')}, Expired: {res.get('license', {}).get('is_expired')}")
    assert code == 200, "Valid license failed"
    assert res.get('license', {}).get('is_expired') == False

    # TEST B: Expired License (Expired Yesterday)
    past_expiry = "2024-01-01T00:00:00Z"
    expired_lic = create_license("Acme Corp Security Test", 500, past_expiry, priv_key)
    code, res = activate_license(expired_lic)
    print(f"\n[TEST B] Expired License Activation (Expired in Past): HTTP {code}")
    print(f"         Error Message: {res.get('error', {}).get('message') or res.get('message')}")
    assert code == 400 or "LICENSE_EXPIRED" in str(res), "Expired license was not rejected!"
    print("         --> STRICT EXPIRY ENFORCEMENT PASSED! Gateway rejected expired license!")

    # TEST C: Tampered Payload (Customer tries to change 500 seats -> 99999 seats without private key)
    tampered_lic = json.loads(json.dumps(valid_lic))
    # Modifying the payload in payload_b64
    tampered_payload = tampered_lic["payload"].copy()
    tampered_payload["max_seats"] = 99999
    tampered_canonical = json.dumps(tampered_payload, sort_keys=True, separators=(",", ":")).encode("utf-8")
    tampered_lic["payload_b64"] = base64.b64encode(tampered_canonical).decode("utf-8")
    # Note: signature is NOT updated because customer doesn't have private key
    code, res = activate_license(tampered_lic)
    print(f"\n[TEST C] Tampered License (Attempt to inflate seats without key): HTTP {code}")
    print(f"         Error Message: {res.get('error', {}).get('message') or res.get('message')}")
    assert "CRYPTOGRAPHIC_SIGNATURE_INVALID" in str(res), "Tampered license was not blocked!"
    print("         --> TAMPER-PROOF SECURITY PASSED! Gateway rejected modified payload!")

    # TEST D: Forged License with a Fake Private Key (Rogue Hacker generated their own key)
    fake_priv = ed25519.Ed25519PrivateKey.generate()
    forged_lic = create_license("Acme Corp Rogue", 10000, future_expiry, fake_priv)
    code, res = activate_license(forged_lic)
    print(f"\n[TEST D] Forged License with Fake Private Key: HTTP {code}")
    print(f"         Error Message: {res.get('error', {}).get('message') or res.get('message')}")
    assert "CRYPTOGRAPHIC_SIGNATURE_INVALID" in str(res), "Forged license was not blocked!"
    print("         --> CRYPTOGRAPHIC FORGERY DEFENSE PASSED! Only vendor master key is accepted!")

    print("\n====================================================================")
    print(" [ALL 4 TESTS PASSED] LICENSE SECURITY & EXPIRY IS 100% BULLETPROOF!")
    print("====================================================================")

if __name__ == "__main__":
    run_tests()
