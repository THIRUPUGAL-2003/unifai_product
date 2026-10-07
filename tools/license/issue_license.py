#!/usr/bin/env python3
"""
Enterprise On-Premise License Generator for Raksha / UnifAI.
Signs license parameters using the Vendor Master Private Key (Ed25519).

Default Issuer: YesPanchi Group of Companies
Default Product: Raksha - Real-time AI Knowledge Screening & Hazard Audit

Usage:
    python tools/license/issue_license.py --client "ABC Corporation" --seats 100 --expiry "2027-10-07"
"""

import os
import sys
import json
import uuid
import base64
import argparse
from datetime import datetime, timezone
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization

DEFAULT_ISSUER = "YesPanchi Group of Companies"
DEFAULT_PRODUCT = "Raksha - Real-time AI Knowledge Screening & Hazard Audit"

def issue_license(
    client_name: str,
    seats: int,
    expiry_date: str,
    issuer: str = DEFAULT_ISSUER,
    product: str = DEFAULT_PRODUCT,
    tier: str = "Enterprise On-Premise",
    features: list = None,
    private_key_path: str = None,
    output_path: str = None
) -> dict:
    if features is None:
        features = [
            "browser_ai_guard",
            "realtime_dlp",
            "file_redaction",
            "voice_inspection",
            "air_gapped_offline"
        ]

    script_dir = Path(__file__).resolve().parent
    priv_file = Path(private_key_path) if private_key_path else script_dir / "master_private.key"

    if not priv_file.exists():
        raise FileNotFoundError(
            f"Master private key not found at '{priv_file}'. "
            f"Run 'generate_keypair.py' first to generate your keys."
        )

    # Load Private Key
    with open(priv_file, "rb") as f:
        private_key = serialization.load_pem_private_key(f.read(), password=None)

    # Validate Expiry format
    try:
        if len(expiry_date) == 10:  # YYYY-MM-DD
            expiry_dt = datetime.strptime(expiry_date, "%Y-%m-%d").replace(tzinfo=timezone.utc)
            expiry_iso = expiry_dt.strftime("%Y-%m-%dT23:59:59Z")
        else:
            expiry_iso = expiry_date
    except Exception as e:
        raise ValueError(f"Invalid expiry date '{expiry_date}'. Use YYYY-MM-DD format: {e}")

    now_dt = datetime.now(timezone.utc)
    now_iso = now_dt.strftime("%Y-%m-%dT%H:%M:%SZ")
    license_id = f"YP-{now_dt.strftime('%Y%m')}-{uuid.uuid4().hex[:8].upper()}"

    # Canonical Payload Dictionary (sorted keys for deterministic JSON serialization)
    payload_data = {
        "version": "1.0",
        "license_id": license_id,
        "issuer": issuer.strip(),
        "product": product.strip(),
        "client_name": client_name.strip(),
        "tier": tier.strip(),
        "max_seats": int(seats),
        "features": sorted(features),
        "issued_at": now_iso,
        "expires_at": expiry_iso,
    }

    # Deterministic JSON bytes
    canonical_json = json.dumps(payload_data, sort_keys=True, separators=(",", ":")).encode("utf-8")

    # Ed25519 Digital Signature
    signature_bytes = private_key.sign(canonical_json)
    signature_b64 = base64.b64encode(signature_bytes).decode("utf-8")
    payload_b64 = base64.b64encode(canonical_json).decode("utf-8")

    license_envelope = {
        "format": "raksha_enterprise_license_v1",
        "payload": payload_data,
        "payload_b64": payload_b64,
        "signature": signature_b64
    }

    # Default output path
    safe_name = "".join(c if c.isalnum() else "_" for c in client_name.lower())
    out_file = Path(output_path) if output_path else script_dir / f"license_{safe_name}.lic"

    with open(out_file, "w", encoding="utf-8") as f:
        json.dump(license_envelope, f, indent=2)

    return {
        "envelope": license_envelope,
        "output_file": str(out_file),
        "canonical_json": canonical_json.decode("utf-8")
    }

def main():
    parser = argparse.ArgumentParser(description="Issue an Ed25519 Signed Enterprise License.")
    parser.add_argument("--client", required=True, help="Client organization name (e.g., 'ABC Corporation')")
    parser.add_argument("--seats", type=int, default=100, help="Maximum allowed laptop agent seats (default: 100)")
    parser.add_argument("--expiry", required=True, help="Expiry date in YYYY-MM-DD format (e.g., '2027-10-07')")
    parser.add_argument("--issuer", default=DEFAULT_ISSUER, help=f"Vendor Issuer name (default: '{DEFAULT_ISSUER}')")
    parser.add_argument("--product", default=DEFAULT_PRODUCT, help=f"Product name (default: '{DEFAULT_PRODUCT}')")
    parser.add_argument("--tier", default="Enterprise On-Premise", help="License tier name")
    parser.add_argument("--out", default=None, help="Output file path (default: license_<client>.lic)")
    parser.add_argument("--key", default=None, help="Custom path to master_private.key")

    args = parser.parse_args()

    try:
        res = issue_license(
            client_name=args.client,
            seats=args.seats,
            expiry_date=args.expiry,
            issuer=args.issuer,
            product=args.product,
            tier=args.tier,
            private_key_path=args.key,
            output_path=args.out
        )
        p = res["envelope"]["payload"]
        print("\n" + "=" * 68)
        print(" [OK] Cryptographic Enterprise License Issued Successfully!")
        print("=" * 68)
        print(f" Issuer (Vendor):  {p['issuer']}")
        print(f" Product:          {p['product']}")
        print(f" License ID:       {p['license_id']}")
        print(f" Issued To Client: {p['client_name']}")
        print(f" Max Seats:        {p['max_seats']} Laptops")
        print(f" Tier:             {p['tier']}")
        print(f" Expires At:       {p['expires_at']}")
        print(f" File Path:        {res['output_file']}")
        print("=" * 68)
        print(" Email this .lic file to the client. They can upload it via the")
        print(" Browser AI > Setup tab in their Web Dashboard.")
        print("=" * 68 + "\n")
    except Exception as e:
        print(f"[ERROR] Failed to issue license: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
