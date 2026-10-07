#!/usr/bin/env python3
"""
Master Keypair Generator for Gateway Enterprise Licensing.
Generates an Ed25519 asymmetric cryptographic keypair.

- The PRIVATE KEY stays strictly with the Gateway/UnifAI vendor (YOU).
  Keep it safe. Never commit it to git or share it with clients.
- The PUBLIC KEY is embedded into the Go backend binary.
  It is safe to distribute; it can only verify signatures, never create them.
"""

import os
import sys
import base64
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization

def main():
    script_dir = Path(__file__).resolve().parent
    priv_file = script_dir / "master_private.key"
    pub_file = script_dir / "master_public.key"

    if priv_file.exists():
        print(f"[!] Existing private key found at: {priv_file}")
        resp = input("Overwrite and generate a NEW master keypair? (y/N): ").strip().lower()
        if resp != "y":
            print("Aborted. Kept existing keypair.")
            return

    # Generate Ed25519 Private Key
    private_key = ed25519.Ed25519PrivateKey.generate()
    public_key = private_key.public_key()

    # Save Private Key (PEM format)
    priv_bytes = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption()
    )
    with open(priv_file, "wb") as f:
        f.write(priv_bytes)

    # Save Public Key (PEM format)
    pub_bytes = public_key.public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo
    )
    with open(pub_file, "wb") as f:
        f.write(pub_bytes)

    # Raw 32-byte public key for Go backend embedding
    raw_pub = public_key.public_bytes(
        encoding=serialization.Encoding.Raw,
        format=serialization.PublicFormat.Raw
    )
    pub_b64 = base64.b64encode(raw_pub).decode("utf-8")
    go_byte_array = ", ".join(f"0x{b:02x}" for b in raw_pub)

    print("\n" + "=" * 60)
    print(" [OK] Master Ed25519 Keypair Generated Successfully!")
    print("=" * 60)
    print(f" Private Key (KEEP SECRET): {priv_file}")
    print(f" Public Key (DISTRIBUTABLE): {pub_file}")
    print("\n--- Go Backend Public Key Embedding ---")
    print(f'Base64: "{pub_b64}"')
    print(f"Go Bytes: []byte{{{go_byte_array}}}")
    print("=" * 60 + "\n")

if __name__ == "__main__":
    main()
