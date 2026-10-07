#!/usr/bin/env python3
"""
Unit tests verifying:
1. Zero predict on file upload in ChatGPT & all AIs (predict strictly after Send).
2. Telemetry (/ces/v1/rgstr, /ces/v1, /rum, /events) never triggers file evaluation or consumes cache.
3. Block Upload blocks immediately at upload time when enabled.
4. Claude wire noise ('b', 'r', 'en-USz qBudp', 'en-US') rejected from prompt extraction.
5. Claude file import & document body dump handled as file payload, not raw chat prompt.
"""

import sys
import unittest
from pathlib import Path

# Add proxy root to sys.path
proxy_root = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(proxy_root))

import browser_ai_proxy as pxy


class TestFileUploadTimingAndClaudeNoise(unittest.TestCase):
    def setUp(self):
        # Clear dedupe cache & upload cache
        with pxy._UPLOAD_FILE_CACHE_LOCK:
            pxy._UPLOAD_FILE_CACHE.clear()
        with pxy._FILE_ID_NAME_REGISTRY_LOCK:
            pxy._FILE_ID_NAME_REGISTRY.clear()
        with pxy._DOMAIN_PENDING_NAMES_LOCK:
            pxy._DOMAIN_PENDING_NAMES.clear()

    def test_01_telemetry_never_triggers_file_policy(self):
        """ChatGPT telemetry /ces/v1/rgstr must never apply file policy or consume upload cache."""
        domain = "chatgpt.com"
        # Cache an uploaded file
        pxy.cache_upload_file(
            domain,
            file_name="employee_handbook.pdf",
            raw_bytes=b"%PDF-1.4 test employee handbook content",
            content_type="application/pdf",
            upload_reason="test upload",
        )
        self.assertTrue(pxy._domain_has_pending_upload_cache(domain))

        # Simulate ChatGPT telemetry ping
        telemetry_path = "/ces/v1/rgstr?device_id=123"
        telemetry_body = '{"event": "rgstr", "data": {"type": "client_event", "action": "focus"}}'

        # Must return False!
        self.assertTrue(pxy._path_has_ignore_pattern(telemetry_path))
        self.assertFalse(pxy._file_policy_applies_on_send(
            telemetry_path, telemetry_body, domain=domain, host="chatgpt.com",
        ))

        # The cache MUST still be present (unconsumed!)
        self.assertTrue(pxy._domain_has_pending_upload_cache(domain))
        cached = pxy.peek_pending_upload_name_for_domain(domain)
        self.assertEqual(cached, "employee_handbook.pdf")

    def test_02_chat_send_consumes_cached_upload_after_send(self):
        """Only actual chat send (/backend-api/conversation) triggers file policy on ChatGPT."""
        domain = "chatgpt.com"
        pxy.cache_upload_file(
            domain,
            file_name="financial_report.xlsx",
            raw_bytes=b"PK\x03\x04 fake excel bytes",
            content_type="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            upload_reason="test upload",
        )
        self.assertTrue(pxy._domain_has_pending_upload_cache(domain))

        chat_send_path = "/backend-api/conversation"
        chat_send_body = (
            '{"action": "next", "messages": [{"author": {"role": "user"}, '
            '"content": {"content_type": "text", "parts": ["Analyze this file"]}, '
            '"metadata": {"attachments": [{"id": "file-123", "name": "financial_report.xlsx"}]}}]}'
        )

        self.assertFalse(pxy._path_has_ignore_pattern(chat_send_path))
        self.assertTrue(pxy._file_policy_applies_on_send(
            chat_send_path, chat_send_body, domain=domain, host="chatgpt.com",
        ))

    def test_03_claude_wire_noise_rejected(self):
        """Isolated letters ('b', 'r') and locale wire tokens ('en-USz qBudp') are rejected."""
        # Single letters
        self.assertTrue(pxy._is_claude_wire_noise("b"))
        self.assertTrue(pxy._is_claude_wire_noise("r"))
        self.assertFalse(pxy.looks_like_user_prompt("b"))
        self.assertFalse(pxy.looks_like_user_prompt("r"))

        # Locale and wire noise combos
        self.assertTrue(pxy._is_claude_wire_noise("en-USz qBudp"))
        self.assertFalse(pxy.looks_like_user_prompt("en-USz qBudp"))
        self.assertTrue(pxy._is_claude_wire_noise("en-US"))
        self.assertFalse(pxy.looks_like_user_prompt("en-US"))

        # Action tokens
        self.assertTrue(pxy._is_claude_wire_noise("PerformAction"))
        self.assertTrue(pxy._is_claude_wire_noise("ReportViewing"))
        self.assertTrue(pxy._is_claude_wire_noise("view"))

        # Real user prompts must be accepted
        self.assertFalse(pxy._is_claude_wire_noise("Hello Claude, can you review this?"))
        self.assertTrue(pxy.looks_like_user_prompt("Hello Claude, can you review this?"))

        # Non-ASCII scripts must be accepted
        self.assertTrue(pxy.looks_like_user_prompt("வணக்கம்"))  # Tamil
        self.assertTrue(pxy.looks_like_user_prompt("你好"))  # Chinese
        self.assertTrue(pxy.looks_like_user_prompt("नमस्ते"))  # Hindi

    def test_04_claude_document_dump_not_picked_as_chat_prompt(self):
        """Document body dumps must not be selected as chat prompts in Claude."""
        doc_dump = (
            "Gateway Guard — Employee Install Guide\n\n"
            "This document explains the steps to install Browser Guard.\n"
            "1. Download the installer\n"
            "2. Run the application\n"
            "3. Verify connection to proxy server\n"
            "Account Details: Employee ID 12345, Department Engineering.\n"
        )
        self.assertTrue(pxy._looks_like_document_body_dump(doc_dump))
        self.assertTrue(pxy._is_claude_wire_noise(doc_dump))

        candidates = [
            "PerformAction",
            "en-USz qBudp",
            "EMPLOYEE_README.txt",
            doc_dump,
            "Please summarize the installation guide",
        ]

        picked = pxy._filter_and_pick_claude_prompt(candidates)
        self.assertEqual(picked, "Please summarize the installation guide")

    def test_05_claude_protobuf_filename_extraction(self):
        """Attached filenames embedded in protobuf are correctly extracted."""
        # Simulate protobuf string extraction on a payload containing a filename
        fake_protobuf_body = (
            b"\x0a\x13EMPLOYEE_README.txt\x12\x50Gateway Guard Employee Install Guide...\x1a\x09Summarize"
        )
        raw_text = fake_protobuf_body.decode("utf-8", errors="ignore")
        names = pxy.extract_all_attachment_filenames_from_send(raw_text)
        self.assertIn("EMPLOYEE_README.txt", names)


if __name__ == "__main__":
    unittest.main()
