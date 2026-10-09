#!/usr/bin/env python3
"""
Enterprise On-Premise License Generator for Gateway / UnifAI.
Signs license parameters using the Vendor Master Private Key (Ed25519).

Default Issuer: YesPanchi Group of Companies
Default Product: Gateway - Real-time AI Knowledge Screening & Hazard Audit

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
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives import serialization

DEFAULT_ISSUER = "YesPanchi Group of Companies"
DEFAULT_PRODUCT = "Gateway - Real-time AI Knowledge Screening & Hazard Audit"

# Must match framework/logstore/license_wrap.go licenseFileKey.
_LICENSE_FILE_KEY = bytes([
    0x91, 0x3C, 0xE7, 0x4A, 0x18, 0xB2, 0x5D, 0x06,
    0xCF, 0x77, 0x21, 0x9E, 0x44, 0xD8, 0x0B, 0x63,
    0xAA, 0x15, 0x6F, 0x82, 0x39, 0xC4, 0x5E, 0x10,
    0x7B, 0xE1, 0x48, 0x9A, 0x2D, 0xF6, 0x53, 0x0C,
])


def seal_license_file(inner: bytes) -> str:
    """Return a single GWLIC1 key. The client file does not contain readable seat or client fields."""
    nonce = os.urandom(12)
    sealed = AESGCM(_LICENSE_FILE_KEY).encrypt(nonce, inner, b"GWLIC1.")
    return "GWLIC1." + base64.b64encode(nonce + sealed).decode("ascii")

def _registry_path(script_dir: Path, registry_path: str = None) -> Path:
    return Path(registry_path) if registry_path else script_dir / "license_registry.json"


def _load_registry(path: Path) -> dict:
    if not path.exists():
        return {"licenses": {}}
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, dict):
        return {"licenses": {}}
    data.setdefault("licenses", {})
    return data


def _save_registry(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)


def issue_license(
    client_name: str,
    seats: int,
    expiry_date: str,
    server_id: str,
    install_id: str,
    license_id: str = None,
    product_users: int = 0,
    issuer: str = DEFAULT_ISSUER,
    product: str = DEFAULT_PRODUCT,
    tier: str = "Enterprise On-Premise",
    revision: int = None,
    features: list = None,
    private_key_path: str = None,
    output_path: str = None,
    registry_path: str = None,
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

    install_id = (install_id or "").strip()
    server_id = (server_id or "").strip().upper()
    if not install_id:
        raise ValueError("install_id is required. Copy it from the client dashboard.")
    if not server_id:
        raise ValueError("server_id is required. Copy the server hardware ID from the client dashboard.")

    now_dt = datetime.now(timezone.utc)
    now_iso = now_dt.strftime("%Y-%m-%dT%H:%M:%SZ")
    reg_file = _registry_path(script_dir, registry_path)
    registry = _load_registry(reg_file)
    licenses = registry.setdefault("licenses", {})

    if revision is not None and revision > 0:
        revision = int(revision)
        if not license_id:
            license_id = f"YP-{now_dt.strftime('%Y%m')}-{uuid.uuid4().hex[:8].upper()}"
    elif license_id and str(license_id).strip():
        license_id = str(license_id).strip()
        previous = licenses.get(license_id) or {}
        revision = int(previous.get("revision") or 0) + 1
    else:
        license_id = f"YP-{now_dt.strftime('%Y%m')}-{uuid.uuid4().hex[:8].upper()}"
        matching_revs = [int(row.get("revision") or 0) for row in licenses.values() if str(row.get("install_id") or "").strip().upper() == install_id.upper()]
        revision = (max(matching_revs) + 1) if matching_revs else 1

    for other_id, row in list(licenses.items()):
        if other_id == license_id:
            continue
        if str(row.get("install_id") or "").strip().upper() == install_id.upper() and str(row.get("status") or "") == "active":
            row["status"] = "revoked"

    # Canonical Payload Dictionary (sorted keys for deterministic JSON serialization)
    payload_data = {
        "version": "1.0",
        "license_id": license_id,
        "issuer": issuer.strip(),
        "product": product.strip(),
        "client_name": client_name.strip(),
        "tier": tier.strip(),
        "max_seats": int(seats),
        "install_id": install_id,
        "server_hardware_id": server_id,
        "revision": int(revision),
        "features": sorted(features),
        "issued_at": now_iso,
        "expires_at": expiry_iso,
    }
    if int(product_users) > 0:
        payload_data["max_product_users"] = int(product_users)

    # Deterministic JSON bytes
    canonical_json = json.dumps(payload_data, sort_keys=True, separators=(",", ":")).encode("utf-8")

    # Ed25519 Digital Signature
    signature_bytes = private_key.sign(canonical_json)
    signature_b64 = base64.b64encode(signature_bytes).decode("utf-8")
    payload_b64 = base64.b64encode(canonical_json).decode("utf-8")

    inner = json.dumps({
        "format": "gateway_enterprise_license_v1",
        "payload_b64": payload_b64,
        "signature": signature_b64,
    }, separators=(",", ":")).encode("utf-8")
    license_token = seal_license_file(inner)
    license_envelope = {
        "format": "gateway_enterprise_license_v1",
        "payload": payload_data,
        "payload_b64": payload_b64,
        "signature": signature_b64,
        "license_key": license_token,
    }

    # Default output path. The file is only the opaque key.
    safe_name = "".join(c if c.isalnum() else "_" for c in client_name.lower())
    out_file = Path(output_path) if output_path else script_dir / f"license_{safe_name}.lic"

    with open(out_file, "w", encoding="utf-8") as f:
        f.write(license_token + "\n")

    licenses[license_id] = {
        "license_id": license_id,
        "install_id": install_id,
        "server_hardware_id": server_id,
        "max_seats": int(seats),
        "max_product_users": int(product_users),
        "revision": int(revision),
        "expires_at": expiry_iso,
        "status": "active",
        "client_name": client_name.strip(),
    }
    _save_registry(reg_file, registry)

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
    parser.add_argument("--server-id", required=True, help="Server Hardware ID from the client dashboard")
    parser.add_argument("--install-id", required=True, help="Database install ID from the client dashboard")
    parser.add_argument("--license-id", default=None, help="Existing license ID when moving server or changing seats. Omit to issue a new license.")
    parser.add_argument("--product-users", type=int, default=0, help="Dashboard accounts allowed in total (user + admin + sub-admin). 0 means no cap.")
    parser.add_argument("--registry", default=None, help="Vendor registry JSON updated when a key is issued or replaced")
    parser.add_argument("--out", default=None, help="Output file path (default: license_<client>.lic)")
    parser.add_argument("--key", default=None, help="Custom path to master_private.key")

    args = parser.parse_args()

    try:
        res = issue_license(
            client_name=args.client,
            seats=args.seats,
            expiry_date=args.expiry,
            server_id=args.server_id,
            install_id=args.install_id,
            license_id=args.license_id,
            product_users=args.product_users,
            issuer=args.issuer,
            product=args.product,
            tier=args.tier,
            private_key_path=args.key,
            output_path=args.out,
            registry_path=args.registry,
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
        if p.get("max_product_users"):
            print(f" Dashboard Users:  {p['max_product_users']} (user + admin + sub-admin)")
        print(f" Tier:             {p['tier']}")
        print(f" Database Install: {p['install_id']}")
        print(f" Server Hardware:  {p['server_hardware_id']}")
        print(f" Revision:         {p['revision']}")
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
