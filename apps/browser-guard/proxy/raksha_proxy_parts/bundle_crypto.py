"""Raksha Guard Proxy Bundle Strong Encryption & In-Memory Decryption.

Zero external dependencies (uses standard library hashlib, hmac, marshal, zlib).
Guarantees 100% consistent execution across macOS and Windows regardless of C-extension availability.
- Compiles Python sources to optimized bytecode.
- Encrypts using 256-bit authenticated CTR keystream + HMAC-SHA256 integrity tag.
- Decrypts directly into RAM via marshal — NEVER writes decrypted source or bytecode to disk.
"""

from __future__ import annotations

import hashlib
import hmac
import marshal
import os
import sys
import zlib
from pathlib import Path
from typing import Any

MAGIC_HEADER = b"RAKSHAENC02\n"
AUTH_CONTEXT = b"Raksha_Browser_AI_Proxy_Bundle_V2_Enterprise"
# Proprietary master secret seed
_MASTER_SEED = b"RakshaGuard::EnterpriseDLP::CoreRulesEngine::2026.09::SecretKeySeed"


def _derive_key(salt: bytes) -> bytes:
    """Derive 256-bit key from master seed + salt using standard HKDF-SHA256."""
    prk = hmac.new(salt, _MASTER_SEED, hashlib.sha256).digest()
    return hmac.new(prk, AUTH_CONTEXT + b"\x01", hashlib.sha256).digest()


def _cipher_stream(key: bytes, nonce: bytes, data: bytes) -> bytes:
    """Fast authenticated keystream cipher (CTR mode via HMAC-SHA256)."""
    blocks = []
    needed = len(data)
    produced = 0
    counter = 0
    while produced < needed:
        counter += 1
        blk = hmac.new(key, nonce + counter.to_bytes(4, "big"), hashlib.sha256).digest()
        blocks.append(blk)
        produced += len(blk)
    keystream = b"".join(blocks)[:needed]
    return bytes(d ^ k for d, k in zip(data, keystream))


def encrypt_parts_bundle(parts_dir: Path, output_enc_path: Path) -> dict[str, int]:
    """Compile all proxy part scripts into encrypted bytecode bundle.

    Returns dict of part_name -> size.
    """
    manifest_path = parts_dir / "MANIFEST.txt"
    if manifest_path.is_file():
        names = [
            ln.strip().lstrip("\ufeff")
            for ln in manifest_path.read_text(encoding="utf-8-sig").splitlines()
            if ln.strip().lstrip("\ufeff") and not ln.strip().startswith("#")
        ]
    else:
        names = sorted(
            p.name for p in parts_dir.glob("*.py")
            if not p.name.startswith("_") and p.name != "bundle_crypto.py"
        )

    bundle_dict: dict[str, bytes] = {}
    stats: dict[str, int] = {}
    for name in names:
        py_file = parts_dir / name
        if not py_file.is_file():
            continue
        source_text = py_file.read_text(encoding="utf-8")
        # Compile directly to optimized Python bytecode (strip docstrings, assert)
        code_obj = compile(source_text, f"<raksha_proxy_parts/{name}>", "exec", optimize=2)
        code_bytes = marshal.dumps(code_obj)
        bundle_dict[name] = code_bytes
        stats[name] = len(code_bytes)

    if not bundle_dict:
        raise RuntimeError(f"No proxy parts found in {parts_dir} to encrypt")

    # Serialize bundle dict -> compress with zlib max level
    serialized = marshal.dumps({"version": 2, "parts": bundle_dict})
    compressed = zlib.compress(serialized, level=9)

    salt = os.urandom(16)
    nonce = os.urandom(16)
    key = _derive_key(salt)

    ciphertext = _cipher_stream(key, nonce, compressed)
    # 256-bit HMAC tag covering salt + nonce + ciphertext
    tag = hmac.new(key, salt + nonce + ciphertext + AUTH_CONTEXT, hashlib.sha256).digest()

    # Format: MAGIC(12) + SALT(16) + NONCE(16) + TAG(32) + CIPHERTEXT
    output_enc_path.write_bytes(MAGIC_HEADER + salt + nonce + tag + ciphertext)
    return stats


def decrypt_parts_bundle(enc_path: Path) -> dict[str, Any]:
    """Decrypt bundle strictly in RAM and return dict of part_name -> code_object.

    NEVER touches disk.
    """
    raw_data = enc_path.read_bytes()
    if not raw_data.startswith(MAGIC_HEADER):
        raise ValueError(f"Invalid bundle magic header in {enc_path}")

    offset = len(MAGIC_HEADER)
    salt = raw_data[offset:offset + 16]
    offset += 16
    nonce = raw_data[offset:offset + 16]
    offset += 16
    tag = raw_data[offset:offset + 32]
    offset += 32
    ciphertext = raw_data[offset:]

    key = _derive_key(salt)
    expected_tag = hmac.new(key, salt + nonce + ciphertext + AUTH_CONTEXT, hashlib.sha256).digest()
    if not hmac.compare_digest(tag, expected_tag):
        raise PermissionError("Bundle integrity check failed (tampered ciphertext)")

    compressed = _cipher_stream(key, nonce, ciphertext)
    serialized = zlib.decompress(compressed)
    payload = marshal.loads(serialized)

    parts_raw = payload.get("parts") or {}
    code_map: dict[str, Any] = {}
    for name, code_bytes in parts_raw.items():
        code_map[name] = marshal.loads(code_bytes)

    # Clean memory buffers
    del compressed
    del serialized
    del payload
    return code_map
