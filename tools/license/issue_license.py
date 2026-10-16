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
SCRIPT_DIR = Path(__file__).resolve().parent

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
    server_id: str = None,
    install_id: str = None,
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
    vendor_ip: str = None,
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

    clean_server_id = (server_id or "").strip()
    clean_install_id = (install_id or "").strip()
    if clean_server_id in ("*", "ANY"):
        clean_server_id = ""
    if clean_install_id in ("*", "ANY"):
        clean_install_id = ""

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
        matching_revs = [int(row.get("revision") or 0) for row in licenses.values() if clean_install_id and str(row.get("install_id") or "").strip().upper() == clean_install_id.upper()]
        revision = (max(matching_revs) + 1) if matching_revs else 1

    for other_id, row in list(licenses.items()):
        if other_id == license_id:
            continue
        if clean_install_id and str(row.get("install_id") or "").strip().upper() == clean_install_id.upper() and str(row.get("status") or "") == "active":
            row["status"] = "revoked"

    if not vendor_ip or not str(vendor_ip).strip():
        try:
            import socket
            vendor_ip = socket.gethostbyname(socket.gethostname())
        except Exception:
            vendor_ip = "127.0.0.1"
    else:
        vendor_ip = str(vendor_ip).strip()

    # Canonical Payload Dictionary (sorted keys for deterministic JSON serialization)
    payload_data = {
        "version": "1.0",
        "license_id": license_id,
        "issuer": issuer.strip(),
        "product": product.strip(),
        "client_name": client_name.strip(),
        "tier": tier.strip(),
        "max_seats": int(seats),
        "issued_from_ip": vendor_ip,
        "revision": int(revision),
        "features": sorted(features),
        "issued_at": now_iso,
        "expires_at": expiry_iso,
    }
    # Only include server_hardware_id / install_id if explicitly requested by vendor
    if clean_server_id:
        payload_data["server_hardware_id"] = clean_server_id.upper()
        payload_data["server_ip"] = clean_server_id
    if clean_install_id:
        payload_data["install_id"] = clean_install_id
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

DEFAULT_VENDOR_MASTER_SECRET = "YESPANCHI-MASTER-2026-GUARD-KEY"

def verify_vendor_authorization(args) -> bool:
    if getattr(args, "bypass_vendor_check", False):
        return True

    # 0. Possessing the Master Private Key file is direct cryptographic authorization
    key_file = Path(getattr(args, "key", None) or os.environ.get("MASTER_PRIVATE_KEY_PATH") or SCRIPT_DIR / "master_private.key")
    if key_file.is_file():
        return True

    # 1. Master Secret check (CLI argument or environment variable)
    secret = (getattr(args, "vendor_secret", "") or os.environ.get("YESPANCHI_MASTER_SECRET") or os.environ.get("GUARD_SECRET_KEY") or "").strip()
    if secret and secret == DEFAULT_VENDOR_MASTER_SECRET:
        return True

    # 2. Server IP / hostname check. Extra host comes from SERVER_DOMAIN, not a baked-in domain.
    import socket
    from urllib.parse import urlparse
    allowed_hosts = {"127.0.0.1", "::1", "localhost"}
    host_labels: list[str] = []
    server_domain = (os.environ.get("SERVER_DOMAIN") or "").strip()
    if server_domain:
        domain_host = (urlparse(server_domain).hostname or "").lower()
        if domain_host:
            allowed_hosts.add(domain_host)
            host_labels = [part for part in domain_host.split(".") if part and part not in {"com", "net", "org", "io"}]
    if getattr(args, "authorized_ip", None):
        allowed_hosts.add(args.authorized_ip.strip())

    try:
        hostname = socket.gethostname()
        local_ips = socket.gethostbyname_ex(hostname)[2]
        if any(ip in allowed_hosts for ip in local_ips) or any(label and label in hostname.lower() for label in host_labels):
            return True
    except Exception:
        pass

    return False


def main():
    parser = argparse.ArgumentParser(description="Issue an Ed25519 Signed Enterprise License (YesPanchi).")
    parser.add_argument("--client", required=True, help="Client organization name (e.g., 'ABC Corporation')")
    parser.add_argument("--seats", type=int, default=100, help="Maximum allowed laptop agent seats (default: 100)")
    parser.add_argument("--expiry", required=True, help="Expiry date in YYYY-MM-DD format (e.g., '2027-10-07')")
    parser.add_argument("--product-users", type=int, default=None, help="Dashboard accounts allowed in total (user + admin + sub-admin). Default: matches --seats. Set 0 for unlimited.")
    parser.add_argument("--issuer", default=DEFAULT_ISSUER, help=f"Vendor Issuer name (default: '{DEFAULT_ISSUER}')")
    parser.add_argument("--product", default=DEFAULT_PRODUCT, help=f"Product name (default: '{DEFAULT_PRODUCT}')")
    parser.add_argument("--tier", default="Enterprise On-Premise", help="License tier name")
    parser.add_argument("--server-ip", default=None, help="Bank client server IP address (e.g. '10.14.20.5' or '*' for any IP)")
    parser.add_argument("--server-id", default=None, help="Optional legacy Server Hardware ID (alias for --server-ip)")
    parser.add_argument("--install-id", default="*", help="Optional database install ID (default: '*' - not required)")
    parser.add_argument("--any-server", action="store_true", help="Allow running on any server host (floating)")
    parser.add_argument("--any-db", action="store_true", help="Allow running on any database instance (floating)")
    parser.add_argument("--license-id", default=None, help="Existing license ID when moving server or changing seats. Omit to issue a new license.")
    parser.add_argument("--revision", type=int, default=None, help="License revision number (must be greater than active DB revision)")
    parser.add_argument("--registry", default=None, help="Vendor registry JSON updated when a key is issued or replaced")
    parser.add_argument("--out", default=None, help="Output file path (default: license_<client>.lic)")
    parser.add_argument("--key", default=None, help="Custom path to master_private.key")
    # Vendor Authorization Protection
    parser.add_argument("--vendor-secret", default=None, help="Master Vendor authorization secret passphrase")
    parser.add_argument("--authorized-ip", default=None, help="Additional authorized vendor IP address")
    parser.add_argument("--vendor-ip", default=None, help="Explicit vendor issuing machine IP address (default: auto-detected local IP)")
    parser.add_argument("--bypass-vendor-check", action="store_true", help="Bypass vendor host / IP verification")

    args = parser.parse_args()

    # Vendor Host / Operator Verification
    if not verify_vendor_authorization(args):
        print("\n" + "=" * 68, file=sys.stderr)
        print(" [SECURITY REFUSAL] Unauthorized Vendor Environment!", file=sys.stderr)
        print("=" * 68, file=sys.stderr)
        print(" License issuance is restricted to authorized YesPanchi vendor servers.", file=sys.stderr)
        print(" To authorize, run with: --vendor-secret \"YESPANCHI-MASTER-2026-GUARD-KEY\"", file=sys.stderr)
        print(" Or pass --bypass-vendor-check to override.", file=sys.stderr)
        print("=" * 68 + "\n", file=sys.stderr)
        sys.exit(1)

    server_id = None
    if args.server_ip:
        server_id = args.server_ip.strip()
    elif args.server_id:
        server_id = args.server_id.strip()

    install_id = None
    if args.install_id and args.install_id not in ("*", "ANY") and not args.any_db:
        install_id = args.install_id.strip()

    effective_product_users = args.seats if args.product_users is None else args.product_users

    try:
        res = issue_license(
            client_name=args.client,
            seats=args.seats,
            expiry_date=args.expiry,
            server_id=server_id,
            install_id=install_id,
            license_id=args.license_id,
            product_users=effective_product_users,
            issuer=args.issuer,
            product=args.product,
            tier=args.tier,
            revision=args.revision,
            private_key_path=args.key,
            output_path=args.out,
            registry_path=args.registry,
            vendor_ip=args.vendor_ip,
        )
        p = res["envelope"]["payload"]
        print("\n" + "=" * 68)
        print(" [OK] Cryptographic Enterprise License Issued Successfully!")
        print("=" * 68)
        print(f" Issuer (Vendor):  {p['issuer']}")
        print(f" Product:          {p['product']}")
        print(f" License ID:       {p['license_id']}")
        print(f" Issued To Client: {p['client_name']}")
        print(f" Max Seats (Guard):{p['max_seats']} Employee Laptops")
        if p.get("max_product_users"):
            print(f" Dashboard Users:  {p['max_product_users']} Accounts (user + admin + sub-admin)")
        else:
            print(" Dashboard Users:  Unlimited / No Cap")
        print(f" Tier:             {p['tier']}")
        if p.get('server_ip') or p.get('server_hardware_id'):
            print(f" Server IP / Host: {p.get('server_ip') or p.get('server_hardware_id')}")
        else:
            print(" Server Binding:   Floating (Runs on Any Server)")
        if p.get('install_id'):
            print(f" Database Binding: Locked to {p.get('install_id')}")
        else:
            print(" Database Binding: Floating (Runs on Any Database)")
        print(f" Issued By Vendor: YesPanchi (Host: {p.get('issued_from_ip', 'Local')})")
        print(f" Revision:         {p['revision']}")
        print(f" Expires At:       {p['expires_at']}")
        print(f" File Path:        {res['output_file']}")
        print("=" * 68)
        print(" [AIR-GAPPED COMPLIANCE] ZERO phone-home signals, 100% offline verification.")
        print(" Email this .lic file to the client to upload into their Web Dashboard.")
        print("=" * 68 + "\n")
    except Exception as e:
        print(f"[ERROR] Failed to issue license: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
