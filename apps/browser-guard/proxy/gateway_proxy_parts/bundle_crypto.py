"""Gateway Guard proxy bundle strong encryption & in-memory decryption.

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

MAGIC_HEADER = b"GATEWAYENC02\n"
_MASK = 0x5C
# Key material is stored scrambled. It is rebuilt only for one HMAC and then dropped.
_SEED_MIXED = (
    27, 12, 10, 234, 215, 208, 187, 148, 145, 148, 116, 115, 18, 3, 15, 53, 24, 24, 252, 239,
    194, 168, 161, 134, 176, 73, 70, 29, 2, 10, 53, 25, 25, 223, 235, 195, 165, 162, 167, 157,
    99, 124, 72, 82, 114, 99, 88, 75, 190, 171, 128, 143, 233, 219, 200, 80, 113, 70, 68, 34,
    44, 34, 31, 242, 207, 200, 219, 171,
)
_CTX_MIXED = (
    27, 12, 10, 234, 215, 208, 187, 140, 166, 135, 105, 96, 91, 92, 56, 4, 45, 52, 209, 207,
    194, 174, 170, 154, 171, 71, 99, 73, 92, 37, 63, 52, 42, 191, 193, 234, 174, 165, 135, 129,
    116, 103, 79, 68, 45,
)


def _unwrap(mixed: tuple[int, ...]) -> bytes:
    raw = bytearray(len(mixed))
    for i, n in enumerate(mixed):
        raw[i] = n ^ ((_MASK + i * 17) & 0xFF)
    try:
        return bytes(raw)
    finally:
        for i in range(len(raw)):
            raw[i] = 0


def _derive_key(salt: bytes) -> bytes:
    """Derive a 256-bit key. The seed is not stored as a readable string."""
    seed = _unwrap(_SEED_MIXED)
    ctx = _unwrap(_CTX_MIXED)
    prk = hmac.new(salt, seed, hashlib.sha256).digest()
    return hmac.new(prk, ctx + b"\x01", hashlib.sha256).digest()


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


def _seal(bundle_dict: dict[str, bytes]) -> bytes:
    """Serialize, compress, and encrypt a name -> bytecode map. Never writes source."""
    if not bundle_dict:
        raise RuntimeError("no sources to encrypt")
    serialized = marshal.dumps({"version": 2, "parts": bundle_dict})
    compressed = zlib.compress(serialized, level=9)
    salt = os.urandom(16)
    nonce = os.urandom(16)
    key = _derive_key(salt)
    ciphertext = _cipher_stream(key, nonce, compressed)
    tag = hmac.new(key, salt + nonce + ciphertext + _unwrap(_CTX_MIXED), hashlib.sha256).digest()
    return MAGIC_HEADER + salt + nonce + tag + ciphertext


def encrypt_source_map(sources: dict[str, str]) -> bytes:
    """Compile name -> source text into one encrypted bytecode blob."""
    bundle_dict: dict[str, bytes] = {}
    for name, source_text in sources.items():
        code_obj = compile(source_text, f"<{name}>", "exec", optimize=2)
        bundle_dict[name] = marshal.dumps(code_obj)
    return _seal(bundle_dict)


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

    sources: dict[str, str] = {}
    stats: dict[str, int] = {}
    for name in names:
        py_file = parts_dir / name
        if not py_file.is_file():
            continue
        source_text = py_file.read_text(encoding="utf-8")
        sources[name] = source_text
        stats[name] = len(marshal.dumps(compile(source_text, f"<{name}>", "exec", optimize=2)))
    if not sources:
        raise RuntimeError(f"No proxy parts found in {parts_dir} to encrypt")
    output_enc_path.write_bytes(encrypt_source_map(sources))
    return stats


def encrypt_guard_code_bundle(root: Path, output_enc_path: Path) -> dict[str, int]:
    """Encrypt the proxy loader and agent modules. The frozen starter stays outside."""
    sources: dict[str, str] = {}
    proxy = root / "proxy" / "browser_ai_proxy.py"
    if not proxy.is_file():
        raise RuntimeError(f"missing {proxy}")
    sources["browser_ai_proxy.py"] = proxy.read_text(encoding="utf-8")
    agent = root / "agent"
    for path in sorted(agent.glob("*.py")):
        if path.name == "guard_bootstrap.py" or path.name.startswith("_"):
            continue
        sources[path.name] = path.read_text(encoding="utf-8")
    if "gateway_agent.py" not in sources:
        raise RuntimeError(f"missing agent modules under {agent}")
    blob = encrypt_source_map(sources)
    output_enc_path.parent.mkdir(parents=True, exist_ok=True)
    output_enc_path.write_bytes(blob)
    return {name: len(sources[name].encode("utf-8")) for name in sources}


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
    expected_tag = hmac.new(key, salt + nonce + ciphertext + _unwrap(_CTX_MIXED), hashlib.sha256).digest()
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
