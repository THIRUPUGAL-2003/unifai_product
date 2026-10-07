"""Test suite for Raksha Guard Proxy Bundle AES-256-GCM encryption & in-memory decryption."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PARTS_DIR = ROOT / "proxy" / "raksha_proxy_parts"

import sys
sys.path.insert(0, str(PARTS_DIR))
import bundle_crypto


class ProxyEncryptionBundleTests(unittest.TestCase):
    def test_encrypt_and_decrypt_all_proxy_parts(self):
        """Verify that all 7 proxy parts can be compiled to bytecode, encrypted, and decrypted in RAM."""
        with tempfile.TemporaryDirectory() as tmpdir:
            enc_file = Path(tmpdir) / "test_bundle.enc"
            stats = bundle_crypto.encrypt_parts_bundle(PARTS_DIR, enc_file)
            self.assertGreaterEqual(len(stats), 7)
            self.assertTrue(enc_file.is_file())
            self.assertGreater(enc_file.stat().st_size, 50_000)

            # Decrypt in memory
            code_map = bundle_crypto.decrypt_parts_bundle(enc_file)
            self.assertEqual(len(code_map), len(stats))

            # Verify every expected part is present and is an executable code object
            for name in [
                "config_caches_rules.py",
                "helpers_prompts.py",
                "uploads_detect.py",
                "file_policy.py",
                "extract_office_backend.py",
                "responses_inject.py",
                "responses_addon.py",
            ]:
                self.assertIn(name, code_map)
                code_obj = code_map[name]
                self.assertEqual(type(code_obj).__name__, "code")

            # Execute sequentially in an isolated namespace to verify zero syntax/runtime errors
            ns: dict = {"__name__": "test_proxy_encrypted"}
            for name in [
                "config_caches_rules.py",
                "helpers_prompts.py",
                "uploads_detect.py",
                "file_policy.py",
                "extract_office_backend.py",
                "responses_inject.py",
                "responses_addon.py",
            ]:
                exec(code_map[name], ns)

            # Verify key classes and functions exist in namespace
            self.assertIn("BrowserAIInterceptor", ns)
            self.assertIn("extract_prompt_universal", ns)
            self.assertIn("detect_file_upload", ns)
            self.assertIn("enforce_file_send_policy", ns)

    def test_tamper_detection_rejects_corrupted_bundle(self):
        """Verify that modifying even a single byte of the encrypted container fails authentication."""
        with tempfile.TemporaryDirectory() as tmpdir:
            enc_file = Path(tmpdir) / "test_tamper.enc"
            bundle_crypto.encrypt_parts_bundle(PARTS_DIR, enc_file)

            raw = bytearray(enc_file.read_bytes())
            # Flip one bit in the middle of ciphertext
            raw[len(raw) // 2] ^= 0xFF
            enc_file.write_bytes(bytes(raw))

            with self.assertRaises(Exception):
                bundle_crypto.decrypt_parts_bundle(enc_file)


if __name__ == "__main__":
    unittest.main()
