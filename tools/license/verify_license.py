#!/usr/bin/env python3
"""
Offline License Verification Tool for Raksha / UnifAI.
Verifies digital signature against the Master Public Key.
"""

import sys
import json
import base64
import argparse
from datetime import datetime, timezone
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization

def verify_license_file(license_file_path: str, public_key_path: str = None) -> dict:
    script_dir = Path(__file__).resolve().parent
    lic_path = Path(license_file_path)
    pub_path = Path(public_key_path) if public_key_path else script_dir / "master_public.key"

    if not lic_path.exists():
        raise FileNotFoundError(f"License file not found: {lic_path}")
    if not pub_path.exists():
        raise FileNotFoundError(f"Master public key not found: {pub_path}")

    # Load Public Key
    with open(pub_path, "rb") as f:
        public_key = serialization.load_pem_public_key(f.read())

    # Load License Envelope
    with open(lic_path, "r", encoding="utf-8") as f:
        envelope = json.load(f)

    if not isinstance(envelope, dict):
        raise ValueError("Invalid license format: expected JSON object")

    payload_b64 = envelope.get("payload_b64")
    signature_b64 = envelope.get("signature")

    if not payload_b64 or not signature_b64:
        raise ValueError("Malformed license: missing 'payload_b64' or 'signature'")

    raw_payload = base64.b64decode(payload_b64)
    raw_signature = base64.b64decode(signature_b64)

    # Cryptographic verification
    try:
        public_key.verify(raw_signature, raw_payload)
        is_signature_valid = True
    except Exception:
        is_signature_valid = False

    payload = json.loads(raw_payload.decode("utf-8"))

    # Expiry Check
    expires_at_str = payload.get("expires_at", "")
    is_expired = False
    try:
        exp_dt = datetime.fromisoformat(expires_at_str.replace("Z", "+00:00"))
        is_expired = datetime.now(timezone.utc) > exp_dt
    except Exception:
        pass

    return {
        "is_valid": is_signature_valid and not is_expired,
        "signature_valid": is_signature_valid,
        "is_expired": is_expired,
        "payload": payload
    }

def main():
    parser = argparse.ArgumentParser(description="Verify a Raksha Enterprise License file.")
    parser.add_argument("file", help="Path to .lic file")
    parser.add_argument("--key", default=None, help="Path to master_public.key")
    args = parser.parse_args()

    try:
        res = verify_license_file(args.file, args.key)
        p = res["payload"]
        print("\n" + "=" * 60)
        print(" [VERIFICATION RESULT]")
        print("=" * 60)
        print(f" Signature Valid: {'[OK] YES' if res['signature_valid'] else '[FAIL] INVALID (TAMPERED)'}")
        print(f" Expired:         {'[FAIL] EXPIRED' if res['is_expired'] else '[OK] ACTIVE'}")
        print(f" Overall Status:  {'[OK] VALID & OPERATIONAL' if res['is_valid'] else '[FAIL] REJECTED'}")
        print("-" * 60)
        print(f" Issuer:          {p.get('issuer', 'YesPanchi Group of Companies')}")
        print(f" Product:         {p.get('product', 'Raksha Enterprise AI Governance')}")
        print(f" License ID:      {p.get('license_id', 'N/A')}")
        print(f" Issued To:       {p.get('client_name')}")
        print(f" Seats:           {p.get('max_seats')} Laptops")
        print(f" Tier:            {p.get('tier')}")
        print(f" Expires At:      {p.get('expires_at')}")
        print("=" * 60 + "\n")

        if not res["is_valid"]:
            sys.exit(1)
    except Exception as e:
        print(f"[ERROR] Verification failed: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
