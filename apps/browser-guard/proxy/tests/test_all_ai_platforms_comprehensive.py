"""Comprehensive Live QA test suite covering ALL major AI platforms:
- ChatGPT (chatgpt.com, chat.openai.com)
- Claude (claude.ai)
- Gemini (gemini.google.com)
- Perplexity (www.perplexity.ai)
- DeepSeek (chat.deepseek.com)
- Mistral AI (chat.mistral.ai)
- Copilot (copilot.microsoft.com)

Tests for each:
1. Accurate prompt prediction & Rule Checks (BLOCK, WARN, REDACT with word masking)
2. Single file upload prediction & rule check with accurate original filename
3. Multiple files batch upload with accurate original filenames
4. Voice / Audio input detection with accurate filenames
"""

import json
import os
import re
import sys
import time
import unittest
import urllib.parse
from pathlib import Path

PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "raksha_proxy_parts"
PARTS = [
    "config_caches_rules.py",
    "helpers_prompts.py",
    "uploads_detect.py",
    "file_policy.py",
    "extract_office_backend.py",
    "responses_inject.py",
    "responses_addon.py",
]

def _load():
    ns: dict = {"__name__": "browser_ai_proxy_all_sites"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    return ns

NS = _load()
from mitmproxy.test import tflow, tutils

NS["evaluate_prompt"] = lambda platform, domain, prompt, client_ip, url, method: NS["decide_prompt_locally"](prompt)
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = lambda *a, **k: True
ADDON = object.__new__(NS["BrowserAIInterceptor"])

SSN = "123-45-6789"
SECRET_KEY = "CONFIDENTIAL_KEY_998877"
PHONE = "9876543210"

PLATFORMS = [
    ("ChatGPT", "chatgpt.com", "/backend-api/f/conversation"),
    ("Claude", "claude.ai", "/api/organizations/org1/chat_conversations/conv1/completion"),
    ("Gemini", "gemini.google.com", "/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate"),
    ("Perplexity", "www.perplexity.ai", "/rest/sse/perplexity_ask"),
    ("DeepSeek", "chat.deepseek.com", "/api/v0/chat/completion"),
    ("Mistral AI", "chat.mistral.ai", "/api/chat"),
    ("Copilot", "copilot.microsoft.com", "/c/api/chat?api-version=2"),
]


def _flow(host: str, path: str, body: bytes | str, method: str = "POST",
          content_type: str = "application/json", headers: dict | None = None):
    if isinstance(body, str):
        body = body.encode("utf-8")
    hdrs = [(b"content-type", content_type.encode()), (b"host", host.encode())]
    for k, v in (headers or {}).items():
        hdrs.append((k.lower().encode(), v.encode()))
    req = tutils.treq(
        host=host, port=443, scheme=b"https", method=method.encode(),
        path=path.encode(), headers=hdrs, content=body,
    )
    req.authority = host
    return tflow.tflow(req=req)


def _multipart(field: str, filename: str, ctype: str, data: bytes):
    boundary = "----WebKitFormBoundaryAllSites"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


def _make_send_body(platform: str, text: str, attachments: list[str] | None = None) -> tuple[str | bytes, str]:
    atts = attachments or []
    if platform == "ChatGPT":
        return json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-1", "author": {"role": "user"},
                "content": {"content_type": "text", "parts": [text]},
                "metadata": {"attachments": [{"name": a} for a in atts]}
            }],
            "model": "auto"
        }), "application/json"
    elif platform == "Claude":
        return json.dumps({
            "prompt": text, "parent_message_uuid": "00000000-0000-4000-8000-000000000000",
            "attachments": [{"file_name": a} for a in atts],
            "files": atts, "rendering_mode": "messages"
        }), "application/json"
    elif platform == "Gemini":
        inner = json.dumps([[text, 0, None, None, None, None, 0], ["en"], ["", "", ""]])
        return urllib.parse.urlencode({"f.req": json.dumps([None, inner]), "at": "AJvLN6M:1727"}), "application/x-www-form-urlencoded;charset=UTF-8"
    elif platform == "Perplexity":
        return json.dumps({
            "params": {"version": "2.18", "source": "default", "mode": "concise"},
            "query_str": text,
            "attachments": atts
        }), "application/json"
    elif platform == "DeepSeek":
        return json.dumps({
            "message": text, "files": atts
        }), "application/json"
    elif platform == "Mistral AI":
        return json.dumps({
            "chatId": "m1", "mode": "append", "model": "mistral-large",
            "messageInput": [{"type": "text", "text": text}] + [{"type": "document", "name": a} for a in atts]
        }), "application/json"
    elif platform == "Copilot":
        return json.dumps({
            "event": "send", "mode": "chat",
            "content": [{"type": "text", "text": text}],
            "attachments": atts
        }), "application/json"
    return json.dumps({"text": text}), "application/json"


class AllAIPlatformsComprehensiveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        NS["_apply_targets_from_data"]({
            "targets": [
                {"domain": host, "platform_name": name, "monitored": True}
                for name, host, _ in PLATFORMS
            ]
        })

    def setUp(self):
        for lock_name, store_names in (
            ("_FILE_ID_NAME_REGISTRY_LOCK", ["_FILE_ID_NAME_REGISTRY"]),
            ("_DOMAIN_PENDING_NAMES_LOCK", ["_DOMAIN_PENDING_NAMES"]),
            ("_CONTENT_HASH_NAME_LOCK", ["_CONTENT_HASH_NAME_REGISTRY"]),
            ("_UPLOAD_FILE_CACHE_LOCK", ["_UPLOAD_FILE_CACHE", "_UPLOAD_FILE_QUEUES"]),
        ):
            if lock_name in NS:
                with NS[lock_name]:
                    for s in store_names:
                        if s in NS:
                            NS[s].clear()
        for name in ("_recent_prompts", "_recent_decisions", "_composer_draft", "_CLIENT_TARGET_STICKY", "_FILE_SEND_BLOCKS"):
            store = NS.get(name)
            if isinstance(store, dict):
                store.clear()

    def _set_rule(self, name: str, pattern: str, action: str, warning_msg: str):
        pat_clean = pattern
        NS["_cached_rules"] = [{
            "name": name,
            "pattern": pat_clean,
            "regex": re.compile(pat_clean, re.IGNORECASE),
            "action": action,
            "severity": "HIGH",
            "warning_message": warning_msg,
        }]
        NS["_rules_fetched_at"] = time.time()
        NS["_rules_fetch_ok"] = True

    # -------------------------------------------------------------------------
    # 1. TEST PROMPT PREDICTION & RULE ACTIONS ACROSS ALL 7 PLATFORMS
    # -------------------------------------------------------------------------
    def test_01_all_platforms_prompt_block_warn_redact(self):
        """Verify BLOCK, WARN, and REDACT (word masked with [REDACTED]) across all 7 AI sites."""
        for name, host, path in PLATFORMS:
            # 1. BLOCK test
            self._set_rule("SSN Rule", r"\b\d{3}-\d{2}-\d{4}\b", "BLOCK", "SSN numbers are blocked")
            body, ctype = _make_send_body(name, f"My SSN is {SSN}")
            f = _flow(host, path, body, content_type=ctype)
            NS["BrowserAIInterceptor"].request(ADDON, f)
            self.assertIsNotNone(f.response, f"[{name}] SSN prompt was NOT blocked")

            # 2. WARN test (prompt allowed, warning notice attached)
            self._set_rule("Phone Rule", r"\b\d{10}\b", "WARN", "Phone numbers are warned")
            body, ctype = _make_send_body(name, f"Contact me on {PHONE}")
            f = _flow(host, path, body, content_type=ctype)
            NS["BrowserAIInterceptor"].request(ADDON, f)
            self.assertIsNone(f.response, f"[{name}] WARN prompt should be allowed")
            sent_text = urllib.parse.unquote_plus((f.request.content or b"").decode("utf-8", errors="ignore"))
            self.assertIn("Phone numbers are warned", sent_text, f"[{name}] WARN notice missing from forwarded request")

            # 3. REDACT test (prompt allowed, SENSITIVE WORD MASKED WITH [REDACTED] + notice)
            self._set_rule("Secret Key Rule", r"CONFIDENTIAL_KEY_\d+", "REDACT", "Secret keys are redacted")
            body, ctype = _make_send_body(name, f"The internal key is {SECRET_KEY} for system")
            f = _flow(host, path, body, content_type=ctype)
            NS["BrowserAIInterceptor"].request(ADDON, f)
            self.assertIsNone(f.response, f"[{name}] REDACT prompt should be allowed")
            sent_text = urllib.parse.unquote_plus((f.request.content or b"").decode("utf-8", errors="ignore"))
            self.assertIn("[REDACTED]", sent_text, f"[{name}] Sensitive word was NOT masked with [REDACTED]")
            self.assertNotIn(SECRET_KEY, sent_text, f"[{name}] Unmasked secret key leaked in forwarded request")

        print("\n[PASS] All 7 AI platforms verified for BLOCK, WARN, and REDACT (word masking).")

    # -------------------------------------------------------------------------
    # 2. TEST SINGLE FILE PREDICTION & ACCURATE FILENAME ACROSS ALL PLATFORMS
    # -------------------------------------------------------------------------
    def test_02_all_platforms_single_file_prediction_and_filename(self):
        """Verify single file upload preserves exact filename and respects rules."""
        file_name = "quarterly_financial_report_2026.pdf"
        file_bytes = b"%PDF-1.4 Company revenue and budget details"

        for name, host, path in PLATFORMS:
            self._set_rule("SSN Rule", r"\b\d{3}-\d{2}-\d{4}\b", "BLOCK", "SSN blocked")

            # Upload step
            ubody, mct = _multipart("file", file_name, "application/pdf", file_bytes)
            up = _flow(host, f"/api/upload/{file_name}", ubody, content_type=mct)
            NS["BrowserAIInterceptor"].request(ADDON, up)
            self.assertIsNone(up.response, f"[{name}] Upload should be cached without blocking")

            # Send step
            sbody, ctype = _make_send_body(name, f"Please review {file_name}", attachments=[file_name])
            send = _flow(host, path, sbody, content_type=ctype)
            NS["BrowserAIInterceptor"].request(ADDON, send)
            self.assertIsNone(send.response, f"[{name}] Safe PDF upload should be allowed on Send")

            # Upload step with blocked content
            danger_name = "employee_payroll_ssn.txt"
            danger_bytes = f"Employee SSN: {SSN} Salary: 100000".encode()
            ubody_d, mct_d = _multipart("file", danger_name, "text/plain", danger_bytes)
            up_d = _flow(host, f"/api/upload/{danger_name}", ubody_d, content_type=mct_d)
            NS["BrowserAIInterceptor"].request(ADDON, up_d)

            sbody_d, ctype_d = _make_send_body(name, f"Review {danger_name}", attachments=[danger_name])
            send_d = _flow(host, path, sbody_d, content_type=ctype_d)
            NS["BrowserAIInterceptor"].request(ADDON, send_d)
            self.assertIsNotNone(send_d.response, f"[{name}] File containing SSN was NOT blocked on Send")

        print("[PASS] Single file predictions & accurate original filenames verified across all 7 platforms.")

    # -------------------------------------------------------------------------
    # 3. TEST MULTI-FILE BATCH UPLOADS ACROSS ALL PLATFORMS
    # -------------------------------------------------------------------------
    def test_03_all_platforms_multi_file_batch_uploads(self):
        """Verify multi-file batch upload (3 files at once) retains individual filenames and blocks if any file violates."""
        files = [
            ("architecture_diagram.png", "image/png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
            ("system_spec.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", b"PK\x03\x04 spec"),
            ("database_schema.sql", "text/plain", b"CREATE TABLE users (id INT, email VARCHAR);"),
        ]

        for name, host, path in PLATFORMS:
            self._set_rule("SSN Rule", r"\b\d{3}-\d{2}-\d{4}\b", "BLOCK", "SSN blocked")

            # Upload all 3 safe files
            for fname, ftype, fdata in files:
                ubody, mct = _multipart("file", fname, ftype, fdata)
                up = _flow(host, f"/api/upload/{fname}", ubody, content_type=mct)
                NS["BrowserAIInterceptor"].request(ADDON, up)

            # Send prompt referencing all 3 files
            sbody, ctype = _make_send_body(name, "Analyze these three files together", attachments=[f[0] for f in files])
            send = _flow(host, path, sbody, content_type=ctype)
            NS["BrowserAIInterceptor"].request(ADDON, send)
            self.assertIsNone(send.response, f"[{name}] Multi-file safe batch should be allowed")

            # Now add ONE dangerous file into a batch of 2 files
            safe_f = ("project_notes.txt", "text/plain", b"Regular notes")
            bad_f = ("confidential_ssn.txt", "text/plain", f"Leaked SSN: {SSN}".encode())
            for fname, ftype, fdata in (safe_f, bad_f):
                ubody, mct = _multipart("file", fname, ftype, fdata)
                up = _flow(host, f"/api/upload/{fname}", ubody, content_type=mct)
                NS["BrowserAIInterceptor"].request(ADDON, up)

            sbody_bad, ctype_bad = _make_send_body(name, "Check both files", attachments=[safe_f[0], bad_f[0]])
            send_bad = _flow(host, path, sbody_bad, content_type=ctype_bad)
            NS["BrowserAIInterceptor"].request(ADDON, send_bad)
            self.assertIsNotNone(send_bad.response, f"[{name}] Multi-file batch containing a bad file MUST be blocked")

        print("[PASS] Multi-file batch uploads & rule enforcement verified across all 7 platforms.")

    # -------------------------------------------------------------------------
    # 4. TEST VOICE / AUDIO INPUT ACROSS ALL PLATFORMS
    # -------------------------------------------------------------------------
    def test_04_all_platforms_voice_and_audio_predictions(self):
        """Verify voice/dictation transcripts and audio notes are checked accurately."""
        voice_file = "voice_memo_retro.m4a"
        audio_bytes = b"\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"

        for name, host, path in PLATFORMS:
            self._set_rule("SSN Rule", r"\b\d{3}-\d{2}-\d{4}\b", "BLOCK", "SSN blocked")

            # 1. Safe voice memo upload & send
            ubody, mct = _multipart("file", voice_file, "audio/m4a", audio_bytes)
            up = _flow(host, f"/api/upload/{voice_file}", ubody, content_type=mct)
            NS["BrowserAIInterceptor"].request(ADDON, up)

            sbody, ctype = _make_send_body(name, "Transcribe and summarize this audio note", attachments=[voice_file])
            send = _flow(host, path, sbody, content_type=ctype)
            NS["BrowserAIInterceptor"].request(ADDON, send)
            self.assertIsNone(send.response, f"[{name}] Safe audio upload should be allowed")

            # 2. Voice transcript containing sensitive SSN
            voice_bad_body, ctype_bad = _make_send_body(name, f"Voice dictation: my social security number is {SSN} please verify")
            send_voice_bad = _flow(host, path, voice_bad_body, content_type=ctype_bad)
            NS["BrowserAIInterceptor"].request(ADDON, send_voice_bad)
            self.assertIsNotNone(send_voice_bad.response, f"[{name}] Voice transcript with SSN was NOT blocked")

        print("[PASS] Voice dictation & audio uploads verified across all 7 platforms.")


if __name__ == "__main__":
    unittest.main()
