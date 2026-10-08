#!/usr/bin/env python3
"""Comprehensive E2E Test Suite for Claude (claude.ai) Multi-File Upload & Send.

Tests:
1. 5 Mixed File Upload & Send (PDF, CSV, DOCX, PNG, TXT) - All clean -> Allowed (1/5) ... (5/5).
2. 5 Mixed File Upload & Send where 1 file contains sensitive data (SSN) -> BLOCKED on Send.
3. In-browser client extracted_content multi-file attachments (Claude Web worker).
4. Pure files UUIDs array on Send without attachments metadata -> All files bound from cache.
5. Claude Connect-RPC / Protobuf multi-file stream with 5-byte framing.
6. Filename-based DLP rule in multi-file batch (e.g. passwords*.txt blocked).
7. Fallback naming: unknown documents fall back to "Document", unknown images to "Image", never "attachment".
"""

from __future__ import annotations

import base64
import json
import re
import sys
import time
import unittest
from pathlib import Path

PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "gateway_proxy_parts"


def _load_proxy_parts():
    names = [
        "config_caches_rules.py",
        "helpers_prompts.py",
        "uploads_detect.py",
        "file_policy.py",
        "extract_office_backend.py",
        "responses_inject.py",
    ]
    ns: dict = {"__name__": "browser_ai_proxy_test"}
    for name in names:
        path = PARTS_DIR / name
        with open(path, "r", encoding="utf-8") as f:
            code = compile(f.read(), str(path), "exec")
            exec(code, ns)
    return ns


class TestClaudeMultiFileE2E(unittest.TestCase):
    def setUp(self):
        self.ns = _load_proxy_parts()
        with self.ns["_UPLOAD_FILE_CACHE_LOCK"]:
            self.ns["_UPLOAD_FILE_CACHE"].clear()
            self.ns["_UPLOAD_FILE_QUEUES"].clear()
        with self.ns["_cache_lock"]:
            self.ns["_cached_rules"] = [
                {
                    "name": "SSN Rule",
                    "pattern": r"\b\d{3}-\d{2}-\d{4}\b",
                    "regex": re.compile(r"\b\d{3}-\d{2}-\d{4}\b"),
                    "action": "BLOCK",
                    "severity": "HIGH",
                },
                {
                    "name": "Secret Key Rule",
                    "pattern": r"(?i)api[_-]?key[_-]?secret[0-9a-f]{8,}",
                    "regex": re.compile(r"(?i)api[_-]?key[_-]?secret[0-9a-f]{8,}"),
                    "action": "BLOCK",
                    "severity": "CRITICAL",
                },
                {
                    "name": "Password Backup Rule",
                    "pattern": r"(?i)passwords?.*\.txt",
                    "regex": re.compile(r"(?i)passwords?.*\.txt"),
                    "action": "BLOCK",
                    "severity": "HIGH",
                },
            ]

    def test_claude_5_mixed_files_all_allowed(self):
        """User uploads 5 clean files to Claude -> All 5 logged with (1/5) ... (5/5)."""
        domain = "claude.ai"
        logged_events = []
        self.ns["post_upload_intercept"] = lambda **kwargs: logged_events.append(kwargs) or True

        # 5 distinct files: PDF, CSV, DOCX, PNG, TXT
        files = [
            ("quarterly_report.pdf", "application/pdf", b"%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF", "uuid-pdf-1"),
            ("sales_pipeline.csv", "text/csv", b"Stage,Lead,Value\nProspect,Acme Corp,50000\nClosing,Globex,120000", "uuid-csv-2"),
            ("project_brief.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", b"PK\x03\x04\x14\x00\x00\x00\x08\x00" + b"\x00" * 300, "uuid-docx-3"),
            ("architecture.png", "image/png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82", "uuid-png-4"),
            ("meeting_notes.txt", "text/plain", b"Discussion on Q3 deliverables and hiring plan", "uuid-txt-5"),
        ]

        # Simulate upload requests for all 5 files
        for fname, ctype, fbytes, fid in files:
            self.ns["cache_upload_file"](
                domain,
                file_name=fname,
                raw_bytes=fbytes,
                content_type=ctype,
                upload_reason="upload endpoint",
                file_id=fid,
            )

        # Send request from Claude Web
        send_payload = {
            "prompt": "Please review all 5 attached files and summarize findings",
            "attachments": [{"file_name": f[0], "file_size": len(f[2])} for f in files],
            "files": [f[3] for f in files],
        }
        send_text = json.dumps(send_payload)

        should_block, block_msg, redact, n_proc, cap_done = self.ns["enforce_file_send_policy"](
            platform="Claude",
            domain="claude.ai",
            host="claude.ai",
            client_ip="127.0.0.1",
            url="https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion",
            method="POST",
            raw_text=send_text,
            content_type="application/json",
            file_name_hint="",
            path="/api/organizations/org-1/chat_conversations/conv-1/completion",
        )

        self.assertFalse(should_block, "Clean 5-file batch was blocked unexpectedly!")
        self.assertEqual(n_proc, 5, f"Expected 5 files processed, got {n_proc}")
        self.assertEqual(len(logged_events), 5, f"Expected 5 log rows, got {len(logged_events)}")

        logged_names = [ev.get("file_name") for ev in logged_events]
        expected_names = [f[0] for f in files]
        self.assertEqual(logged_names, expected_names, "Logged file names did not match uploaded names!")

        # Check multi-file numbering in prompts
        for i, ev in enumerate(logged_events):
            prompt_str = ev.get("prompt", "")
            self.assertIn(f"({i+1}/5)", prompt_str)
            self.assertIn("Allowed", prompt_str)
            self.assertNotIn("attachment", prompt_str.lower().split(".")[0])

    def test_claude_5_mixed_files_with_sensitive_blocked(self):
        """User uploads 5 files where 1 file has SSN -> Blocked on Send."""
        domain = "claude.ai"
        logged_events = []
        self.ns["post_upload_intercept"] = lambda **kwargs: logged_events.append(kwargs) or True

        files = [
            ("quarterly_report.pdf", "application/pdf", b"%PDF-1.4\nClean report data", "uuid-1"),
            ("clean_notes.txt", "text/plain", b"General project notes", "uuid-2"),
            ("sensitive_payroll.csv", "text/csv", b"Employee,Role,SSN,Salary\nAlice,VP,123-45-6789,150000", "uuid-3"),
            ("clean_diagram.png", "image/png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR" + b"\x00" * 30, "uuid-4"),
            ("project_summary.txt", "text/plain", b"Project wrap up", "uuid-5"),
        ]

        for fname, ctype, fbytes, fid in files:
            self.ns["cache_upload_file"](
                domain,
                file_name=fname,
                raw_bytes=fbytes,
                content_type=ctype,
                upload_reason="upload endpoint",
                file_id=fid,
            )

        send_payload = {
            "prompt": "Analyze all 5 uploaded documents",
            "attachments": [{"file_name": f[0]} for f in files],
            "files": [f[3] for f in files],
        }
        send_text = json.dumps(send_payload)

        should_block, block_msg, redact, n_proc, cap_done = self.ns["enforce_file_send_policy"](
            platform="Claude",
            domain="claude.ai",
            host="claude.ai",
            client_ip="127.0.0.1",
            url="https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion",
            method="POST",
            raw_text=send_text,
            content_type="application/json",
            file_name_hint="",
            path="/api/organizations/org-1/chat_conversations/conv-1/completion",
        )

        self.assertTrue(should_block, "Multi-file batch containing sensitive SSN was NOT blocked!")
        self.assertIn("SSN Rule", block_msg)

    def test_claude_extracted_content_inlines(self):
        """Claude in-browser extracted_content attachments -> Scanned & logged properly."""
        logged_events = []
        self.ns["post_upload_intercept"] = lambda **kwargs: logged_events.append(kwargs) or True

        payload = {
            "prompt": "Compare both files",
            "attachments": [
                {
                    "file_name": "q1_revenue.csv",
                    "file_type": "text/csv",
                    "extracted_content": "Month,Revenue\nJan,100k\nFeb,120k",
                },
                {
                    "file_name": "q2_revenue.csv",
                    "file_type": "text/csv",
                    "extracted_content": "Month,Revenue\nApr,150k\nMay,170k",
                },
            ],
        }
        send_text = json.dumps(payload)

        should_block, block_msg, redact, n_proc, cap_done = self.ns["enforce_file_send_policy"](
            platform="Claude",
            domain="claude.ai",
            host="claude.ai",
            client_ip="127.0.0.1",
            url="https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion",
            method="POST",
            raw_text=send_text,
            content_type="application/json",
            file_name_hint="",
            path="/api/organizations/org-1/chat_conversations/conv-1/completion",
        )

        self.assertFalse(should_block)
        self.assertEqual(n_proc, 2)
        self.assertEqual(len(logged_events), 2)
        self.assertEqual(logged_events[0].get("file_name"), "q1_revenue.csv")
        self.assertEqual(logged_events[1].get("file_name"), "q2_revenue.csv")

    def test_claude_files_uuid_only_send(self):
        """Send payload contains ONLY files: [uuid1, uuid2, uuid3] -> All files resolved from cache."""
        domain = "claude.ai"
        logged_events = []
        self.ns["post_upload_intercept"] = lambda **kwargs: logged_events.append(kwargs) or True

        files = [
            ("specs.pdf", "application/pdf", b"%PDF-1.4 Specs", "uuid-a"),
            ("budget.csv", "text/csv", b"Cost,Item\n100,Servers", "uuid-b"),
            ("notes.txt", "text/plain", b"Meeting action items", "uuid-c"),
        ]

        for fname, ctype, fbytes, fid in files:
            self.ns["cache_upload_file"](
                domain,
                file_name=fname,
                raw_bytes=fbytes,
                content_type=ctype,
                upload_reason="upload endpoint",
                file_id=fid,
            )

        # Send with NO attachments array, ONLY files UUIDs
        send_payload = {
            "prompt": "Review my uploaded files",
            "files": ["uuid-a", "uuid-b", "uuid-c"],
        }
        send_text = json.dumps(send_payload)

        should_block, block_msg, redact, n_proc, cap_done = self.ns["enforce_file_send_policy"](
            platform="Claude",
            domain="claude.ai",
            host="claude.ai",
            client_ip="127.0.0.1",
            url="https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion",
            method="POST",
            raw_text=send_text,
            content_type="application/json",
            file_name_hint="",
            path="/api/organizations/org-1/chat_conversations/conv-1/completion",
        )

        self.assertFalse(should_block)
        self.assertEqual(n_proc, 3, f"Expected 3 files processed from files UUIDs, got {n_proc}")
        logged_names = [ev.get("file_name") for ev in logged_events]
        self.assertEqual(logged_names, ["specs.pdf", "budget.csv", "notes.txt"])

    def test_claude_connect_rpc_protobuf_multi_file(self):
        """Claude Connect-RPC / protobuf StreamMessage with multiple filenames."""
        def encode_varint(n):
            res = bytearray()
            while True:
                towrite = n & 0x7f
                n >>= 7
                if n: res.append(towrite | 0x80)
                else: res.append(towrite); break
            return bytes(res)

        def make_pb_string_field(field_num, s):
            data = s.encode("utf-8")
            tag = (field_num << 3) | 2
            return encode_varint(tag) + encode_varint(len(data)) + data

        pb_data = (
            make_pb_string_field(1, "Please compare both documents") +
            make_pb_string_field(2, "confidential_q3.pdf") +
            make_pb_string_field(3, "vendor_contracts.xlsx")
        )
        rpc_frame = b"\x00" + len(pb_data).to_bytes(4, "big") + pb_data

        names = self.ns["extract_all_attachment_filenames_from_send"](rpc_frame.decode("latin-1"))
        self.assertIn("confidential_q3.pdf", names)
        self.assertIn("vendor_contracts.xlsx", names)

    def test_claude_filename_rule_blocked_in_batch(self):
        """Batch containing passwords_backup.txt is blocked by filename rule."""
        domain = "claude.ai"
        self.ns["cache_upload_file"](domain, file_name="readme.txt", raw_bytes=b"Instructions", content_type="text/plain", upload_reason="up", file_id="f1")
        self.ns["cache_upload_file"](domain, file_name="passwords_backup.txt", raw_bytes=b"admin:secret", content_type="text/plain", upload_reason="up", file_id="f2")

        send_payload = {
            "prompt": "Check these two files",
            "attachments": [{"file_name": "readme.txt"}, {"file_name": "passwords_backup.txt"}],
            "files": ["f1", "f2"],
        }
        send_text = json.dumps(send_payload)

        should_block, block_msg, redact, n_proc, cap_done = self.ns["enforce_file_send_policy"](
            platform="Claude",
            domain="claude.ai",
            host="claude.ai",
            client_ip="127.0.0.1",
            url="https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion",
            method="POST",
            raw_text=send_text,
            content_type="application/json",
            file_name_hint="",
            path="/api/organizations/org-1/chat_conversations/conv-1/completion",
        )

        self.assertTrue(should_block)
        self.assertIn("Password Backup Rule", block_msg)

    def test_claude_unknown_name_fallbacks(self):
        """Unknown document falls back to Document, unknown image to Image, never attachment."""
        domain = "claude.ai"
        logged_events = []
        self.ns["post_upload_intercept"] = lambda **kwargs: logged_events.append(kwargs) or True

        # Nameless document and nameless image
        self.ns["cache_upload_file"](domain, file_name="attachment", raw_bytes=b"%PDF-1.4 unnamed pdf", content_type="application/pdf", upload_reason="up", file_id="f1")
        self.ns["cache_upload_file"](domain, file_name="attachment", raw_bytes=b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR" + b"\x00" * 30, content_type="image/png", upload_reason="up", file_id="f2")

        send_payload = {
            "prompt": "Analyze",
            "files": ["f1", "f2"],
        }
        send_text = json.dumps(send_payload)

        should_block, block_msg, redact, n_proc, cap_done = self.ns["enforce_file_send_policy"](
            platform="Claude",
            domain="claude.ai",
            host="claude.ai",
            client_ip="127.0.0.1",
            url="https://claude.ai/api/organizations/org-1/chat_conversations/conv-1/completion",
            method="POST",
            raw_text=send_text,
            content_type="application/json",
            file_name_hint="",
            path="/api/organizations/org-1/chat_conversations/conv-1/completion",
        )

        self.assertFalse(should_block)
        self.assertEqual(len(logged_events), 2)
        labels = [ev.get("file_name") for ev in logged_events]
        for lbl in labels:
            self.assertNotIn("attachment", lbl.lower())
        self.assertTrue(any("Document" in lbl for lbl in labels))
        self.assertTrue(any("Image" in lbl for lbl in labels))


if __name__ == "__main__":
    unittest.main()
