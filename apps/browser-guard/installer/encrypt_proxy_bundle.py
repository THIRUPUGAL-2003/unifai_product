#!/usr/bin/env python3
"""Pre-build tool: Encrypts Gateway proxy parts into a secure AES-256-GCM bundle.

This runs automatically before PyInstaller / Inno Setup builds to ensure
ZERO plain text Python code is shipped to client endpoints.
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PARTS_DIR = ROOT / "proxy" / "gateway_proxy_parts"
OUTPUT_ENC = PARTS_DIR / "gateway_proxy_parts.enc"

sys.path.insert(0, str(PARTS_DIR))
import bundle_crypto


def main() -> int:
    print("=" * 60)
    print(" Gateway Guard — Encrypting Proxy Engine Bundle")
    print("=" * 60)
    if not PARTS_DIR.is_dir():
        print(f"ERROR: Missing {PARTS_DIR}", file=sys.stderr)
        return 1

    stats = bundle_crypto.encrypt_parts_bundle(PARTS_DIR, OUTPUT_ENC)
    print(f"Encrypted {len(stats)} proxy parts:")
    for name, size in stats.items():
        print(f"  • {name} -> {size} bytes (bytecode)")

    code_enc = ROOT / "proxy" / "gateway_guard_code.enc"
    code_stats = bundle_crypto.encrypt_guard_code_bundle(ROOT, code_enc)
    print(f"Encrypted {len(code_stats)} Guard code files into {code_enc.name}")

    total_kb = OUTPUT_ENC.stat().st_size / 1024
    code_kb = code_enc.stat().st_size / 1024
    print(f"\nSUCCESS: {OUTPUT_ENC.name} ({total_kb:.1f} KB), {code_enc.name} ({code_kb:.1f} KB)")
    print("Encryption: AES-256-GCM + Hardware Acceleration + In-Memory Decryption")
    print("=" * 60)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
