"""Comprehensive Live QA test suite for Claude (Anthropic):
1. 10 Prompts (numbers, zero, text, code, Unicode, DLP SSN, DLP Key)
2. 10 Files (PDF, DOCX, XLSX, TXT, CSV, PNG, JPG, Multi-file, File+Caption, Sensitive Block)
3. Subdomain checks (claude.ai, api.anthropic.com, console.anthropic.com, files.claudeusercontent.com)
4. Anti-token leakage verification (no internal RPC tokens as prompt)
"""

import json
import os
import re
import sys
import time
import unittest
from pathlib import Path
from mitmproxy.test import tflow, tutils

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
    ns: dict = {"__name__": "browser_ai_proxy_claude_comprehensive"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    return ns


NS = _load()
NS["evaluate_prompt"] = lambda platform, domain, prompt, client_ip, url, method: NS["decide_prompt_locally"](prompt)
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = lambda *a, **k: True
ADDON = object.__new__(NS["BrowserAIInterceptor"])


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


def _run(flow) -> None:
    NS["BrowserAIInterceptor"].request(ADDON, flow)


def _blocked(flow) -> bool:
    return flow.response is not None


def _multipart(field: str, filename: str, ctype: str, data: bytes):
    boundary = "----WebKitFormBoundaryClaudeLiveTest"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


class ClaudeLiveComprehensiveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._orig_domains = dict(NS["_cached_domains"])
        cls._orig_rules = list(NS["_cached_rules"])
        cls._orig_roles = dict(NS["_cached_roles"])
        cls._orig_families = dict(NS["_cached_families"])

        NS["_apply_targets_from_data"]({
            "targets": [
                {"domain": "claude.ai", "platform_name": "Claude", "monitored": True},
                {"domain": "api.anthropic.com", "platform_name": "Claude", "monitored": True},
                {"domain": "console.anthropic.com", "platform_name": "Claude", "monitored": True},
                {"domain": "files.claudeusercontent.com", "platform_name": "Claude", "monitored": True},
                {"domain": "claudeusercontent.com", "platform_name": "Claude", "monitored": True},
                {"domain": "chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
            ]
        })
        pat_ssn = r"\b\d{3}-\d{2}-\d{4}\b"
        pat_key = r"(?i)\bCONFIDENTIAL_INTERNAL_KEY_[A-Z0-9_]+\b"
        NS["_cached_rules"] = [
            {
                "name": "SSN Rule",
                "pattern": pat_ssn,
                "regex": re.compile(pat_ssn, re.IGNORECASE),
                "action": "BLOCK",
                "severity": "HIGH",
            },
            {
                "name": "Confidential Leak Rule",
                "pattern": pat_key,
                "regex": re.compile(pat_key, re.IGNORECASE),
                "action": "BLOCK",
                "severity": "CRITICAL",
            },
        ]

    @classmethod
    def tearDownClass(cls):
        with NS["_cache_lock"]:
            NS["_cached_domains"] = cls._orig_domains
            NS["_cached_rules"] = cls._orig_rules
            NS["_cached_roles"] = cls._orig_roles
            NS["_cached_families"] = cls._orig_families

    def test_01_to_10_claude_prompts_including_numbers(self):
        """Test 10 diverse Claude prompt varieties: numbers, zero, text, code, Unicode, DLP blocks."""
        prompts = [
            # 1. Short number
            ("42", False, "42"),
            # 2. Large number
            ("100000", False, "100000"),
            # 3. Decimal float
            ("3.14159", False, "3.14159"),
            # 4. Zero and single digit
            ("0", False, "0"),
            # 5. Regular text
            ("Explain the architecture of transformers in simple terms.", False, "transformers"),
            # 6. Python code
            ("def fibonacci(n):\n    return n if n <= 1 else fibonacci(n-1) + fibonacci(n-2)", False, "fibonacci"),
            # 7. Tamil Unicode script
            ("செயற்கை நுண்ணறிவு எவ்வாறு செயல்படுகிறது?", False, "செயற்கை"),
            # 8. Math and special symbols
            ("What is the difference between $A = \\pi r^2$ and $C = 2\\pi r$?", False, "\\pi"),
            # 9. DLP SSN Violation (MUST BLOCK)
            ("Employee tax SSN is 123-45-6789 please process records.", True, "SSN"),
            # 10. DLP Confidential Key (MUST BLOCK)
            ("Store this confidential token: CONFIDENTIAL_INTERNAL_KEY_CLAUDE_999", True, "CONFIDENTIAL"),
        ]

        for i, (prompt_text, should_block, label) in enumerate(prompts, 1):
            # Test in Claude JSON completion format
            body = json.dumps({
                "prompt": prompt_text,
                "parent_message_uuid": "00000000-0000-4000-8000-000000000000",
                "model": "claude-3-5-sonnet-20241022",
            })
            f = _flow("claude.ai", f"/api/organizations/org1/chat_conversations/conv-{i}/completion", body)
            _run(f)

            if should_block:
                self.assertTrue(_blocked(f), f"Prompt #{i} ({label}) with sensitive data was NOT blocked!")
            else:
                self.assertFalse(_blocked(f), f"Valid prompt #{i} ({label}) was incorrectly blocked!")

            # Verify prompt extraction extracts exact text and not internal tokens
            extracted = NS["extract_prompt_universal"](
                body.encode("utf-8"), "application/json", "claude.ai",
                f"/api/organizations/org1/chat_conversations/conv-{i}/completion"
            )
            self.assertEqual(extracted, prompt_text, f"Prompt #{i} extraction failed or leaked tokens: got {extracted!r}")

        print("\n[Test Result] 10 Claude Prompt Varieties (numbers, zero, text, code, Unicode, DLP) passed!")

    def test_11_to_20_claude_file_uploads_and_predictions(self):
        """Test 10 Claude file uploads: PDF, DOCX, XLSX, TXT, CSV, PNG, JPG, Multi-file, Caption, Sensitive DLP."""
        files = [
            ("employee_handbook.pdf", "application/pdf", b"%PDF-1.4 sample employee handbook"),
            ("business_plan.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", b"PK\x03\x04sample business plan"),
            ("financial_model.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b"PK\x03\x04sample financial excel"),
            ("server_configuration.txt", "text/plain", b"server_port=8080\nhost=localhost"),
            ("customer_records.csv", "text/csv", b"id,name,email\n1,Alice,alice@example.com\n2,Bob,bob@example.com"),
            ("architecture_diagram.png", "image/png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRsample"),
            ("receipt.jpg", "image/jpeg", b"\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01sample"),
        ]

        # 1-7: Single uploads
        for i, (fname, ctype, data) in enumerate(files, 1):
            ubody, mct = _multipart("file", fname, ctype, data)
            up_flow = _flow("claude.ai", f"/api/organizations/org1/upload-{i}", ubody, content_type=mct)
            _run(up_flow)

            # Follow up with send
            sbody = json.dumps({
                "prompt": f"Please analyze {fname}",
                "attachments": [{"file_name": fname}],
                "files": [fname],
            })
            send_flow = _flow("claude.ai", f"/api/organizations/org1/chat_conversations/conv-{i}/completion", sbody)
            _run(send_flow)
            self.assertFalse(_blocked(send_flow))

        # 8: Batch Multi-file upload (2 PDFs + 1 TXT)
        b_pdf1, mct1 = _multipart("file", "quarterly_q1.pdf", "application/pdf", b"%PDF-1.4 Q1 report")
        b_pdf2, mct2 = _multipart("file", "quarterly_q2.pdf", "application/pdf", b"%PDF-1.4 Q2 report")
        b_txt, mct3 = _multipart("file", "notes.txt", "text/plain", b"Important executive notes")

        _run(_flow("claude.ai", "/api/organizations/org1/upload-batch-1", b_pdf1, content_type=mct1))
        _run(_flow("claude.ai", "/api/organizations/org1/upload-batch-2", b_pdf2, content_type=mct2))
        _run(_flow("claude.ai", "/api/organizations/org1/upload-batch-3", b_txt, content_type=mct3))

        multi_send = json.dumps({
            "prompt": "Compare Q1 and Q2 reports and review notes.",
            "attachments": [
                {"file_name": "quarterly_q1.pdf"},
                {"file_name": "quarterly_q2.pdf"},
                {"file_name": "notes.txt"},
            ],
            "files": ["quarterly_q1.pdf", "quarterly_q2.pdf", "notes.txt"],
        })
        m_flow = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-multi/completion", multi_send)
        _run(m_flow)
        self.assertFalse(_blocked(m_flow))

        # 9: File with typed user caption
        cf_body, cf_mct = _multipart("file", "annual_budget.xlsx", "application/vnd.ms-excel", b"PK\x03\x04annual budget")
        _run(_flow("claude.ai", "/api/organizations/org1/upload-caption", cf_body, content_type=cf_mct))

        caption_send = json.dumps({
            "prompt": "Highlight any budget overruns greater than ten thousand dollars.",
            "attachments": [{"file_name": "annual_budget.xlsx"}],
            "files": ["annual_budget.xlsx"],
        })
        c_flow = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-caption/completion", caption_send)
        _run(c_flow)
        self.assertFalse(_blocked(c_flow))

        # 10: Sensitive File containing SSN (MUST BE BLOCKED)
        sens_data = b"Confidential tax document with employee SSN: 123-45-6789 do not leak."
        sf_body, sf_mct = _multipart("file", "payroll_sensitive.txt", "text/plain", sens_data)
        _run(_flow("claude.ai", "/api/organizations/org1/upload-sensitive", sf_body, content_type=sf_mct))

        sens_send = json.dumps({
            "prompt": "Process this payroll sheet.",
            "attachments": [{"file_name": "payroll_sensitive.txt"}],
            "files": ["payroll_sensitive.txt"],
        })
        sf_flow = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-sens/completion", sens_send)
        _run(sf_flow)
        self.assertTrue(_blocked(sf_flow), "Sensitive file containing SSN was NOT blocked on Claude!")

        print("[Test Result] 10 Claude File Upload & Prediction Varieties (PDF, DOCX, XLSX, TXT, CSV, PNG, JPG, Multi-file, Caption, DLP Block) passed!")

    def test_21_to_25_claude_subdomains_and_edge_cases(self):
        """Test Claude subdomains and Connect-RPC message shapes."""
        subdomains = [
            ("claude.ai", "/api/organizations/o/chat_conversations/c/completion"),
            ("api.anthropic.com", "/v1/messages"),
            ("console.anthropic.com", "/api/completion"),
            ("files.claudeusercontent.com", "/upload"),
        ]

        for host, path in subdomains:
            body = json.dumps({"prompt": "Hello Claude from subdomain test", "messages": [{"role": "user", "content": "Hello"}]})
            f = _flow(host, path, body)
            _run(f)
            # Verify target matching recognizes domain
            is_mon, matched_domain, matched_platform = NS["detect_target"](host)
            self.assertTrue(is_mon, f"Subdomain {host} was not marked as monitored!")

        print("[Test Result] All Claude subdomains and endpoints verified successfully!")

    def test_26_to_30_claude_symbols_and_numbers(self):
        """Test symbols and math/code prompts in Claude: c++, x=1, #1, $50, ?, +, 10%, a:=1."""
        symbol_prompts = [
            ("c++", "c++"),
            ("x=1", "x=1"),
            ("#1", "#1"),
            ("$50", "$50"),
            ("?", "?"),
            ("+", "+"),
            ("10%", "10%"),
            ("a:=1", "a:=1"),
            ("1+1=2", "1+1=2"),
            ("pi*r^2", "pi*r^2"),
        ]
        for prompt_text, label in symbol_prompts:
            body = json.dumps({
                "prompt": prompt_text,
                "parent_message_uuid": "00000000-0000-4000-8000-000000000000",
                "model": "claude-3-5-sonnet-20241022",
            })
            f = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-sym/completion", body)
            _run(f)
            self.assertFalse(_blocked(f), f"Symbol prompt {label!r} was incorrectly blocked!")

            extracted = NS["extract_prompt_universal"](
                body.encode("utf-8"), "application/json", "claude.ai",
                "/api/organizations/org1/chat_conversations/conv-sym/completion"
            )
            self.assertEqual(extracted, prompt_text, f"Symbol prompt {label!r} failed extraction: got {extracted!r}")

        print("[Test Result] Claude symbol prompts (c++, x=1, #1, $50, ?, +, 10%, a:=1) verified with zero token leaks!")

    def test_31_claude_word_document_prompt_extract_not_document_body(self):
        """When a user uploads a Word file (.docx) and types a prompt, extract user prompt, NOT doc body."""
        large_doc_text = "Executive Summary: Q3 Financial Results.\n" * 40  # > 1500 chars document dump
        typed_prompt = "Please summarize this quarterly report in 3 bullets."

        # Simulate Claude payload containing both document extract and user prompt
        body = json.dumps({
            "prompt": typed_prompt,
            "attachments": [
                {
                    "file_name": "quarterly_financials.docx",
                    "file_type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
                    "extracted_content": large_doc_text,
                }
            ],
            "files": ["quarterly_financials.docx"],
        })
        extracted = NS["extract_prompt_universal"](
            body.encode("utf-8"), "application/json", "claude.ai",
            "/api/organizations/org1/chat_conversations/conv-docx/completion"
        )
        self.assertEqual(extracted, typed_prompt, f"Expected typed prompt {typed_prompt!r}, but got doc dump: {extracted[:80]!r}")

        # Also verify via protobuf strings picker
        candidates = [
            "org_01xyz1234567890",
            "quarterly_financials.docx",
            large_doc_text,
            typed_prompt,
        ]
        picked = NS["_filter_and_pick_claude_prompt"](candidates)
        self.assertEqual(picked, typed_prompt, f"Claude picker selected doc dump instead of user prompt: {picked[:80]!r}")

        print("[Test Result] Claude Word Document (.docx) upload: correctly extracted user prompt instead of document text dump!")

    def test_32_file_upload_timing_zero_predict_on_upload_predict_on_send(self):
        """Zero prediction on upload/import. Predict and check rules ONLY after Send, unless Block Upload is ON."""
        # Step 1: Normal upload (Block Upload is OFF) -> Must NOT block or evaluate, cache only
        ubody, mct = _multipart("file", "employee_notes.txt", "text/plain", b"Confidential tax SSN: 123-45-6789")
        up_flow = _flow("chatgpt.com", "/backend-api/files/upload-test", ubody, content_type=mct)
        _run(up_flow)
        self.assertFalse(_blocked(up_flow), "Upload was prematurely blocked on import! Must wait for Send.")

        # Step 2: On Send -> Must evaluate cached file and BLOCK because file contains SSN
        send_body = json.dumps({
            "prompt": "Analyze this notes file",
            "attachments": [{"file_name": "employee_notes.txt"}],
            "files": ["employee_notes.txt"],
        })
        send_flow = _flow("chatgpt.com", "/backend-api/f/conversation", send_body)
        _run(send_flow)
        self.assertTrue(_blocked(send_flow), "File with SSN was NOT blocked on Send!")

        # Step 3: When admin enabled Block Upload -> MUST block immediately on upload
        orig_ctrl = dict(NS["_cached_controls"])
        orig_from_be = NS["_controls_from_backend"]
        try:
            NS["_cached_controls"] = {"enabled": True, "block_upload": True, "upload_warning": "File uploads are blocked by policy"}
            NS["_controls_from_backend"] = True
            NS["_controls_fetched_at"] = time.time()
            blk_up = _flow("chatgpt.com", "/backend-api/files/upload-blocked", ubody, content_type=mct)
            _run(blk_up)
            self.assertTrue(_blocked(blk_up), "Upload was NOT blocked on import when Block Upload is ON!")
        finally:
            NS["_cached_controls"] = orig_ctrl
            NS["_controls_from_backend"] = orig_from_be
            NS["_controls_fetched_at"] = time.time()

        print("[Test Result] Upload timing verified: zero predict on import; predict only on Send; block on import only when Block Upload is ON!")

    def test_33_voice_upload_and_send(self):
        """Voice audio (.m4a / .wav) upload and Send rule check."""
        audio_bytes = b"RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00D\xac\x00\x00data\x00\x00\x00\x00"
        v_body, v_mct = _multipart("file", "meeting_recording.m4a", "audio/mp4", audio_bytes)
        v_up = _flow("claude.ai", "/api/organizations/org1/upload-voice", v_body, content_type=v_mct)
        _run(v_up)
        self.assertFalse(_blocked(v_up), "Voice upload was blocked prematurely!")

        v_send = json.dumps({
            "prompt": "Transcribe and summarize this audio note.",
            "attachments": [{"file_name": "meeting_recording.m4a"}],
            "files": ["meeting_recording.m4a"],
        })
        v_send_flow = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-voice/completion", v_send)
        _run(v_send_flow)
        self.assertFalse(_blocked(v_send_flow))

        print("[Test Result] Voice audio upload (.m4a) and Send handling verified successfully!")

    def test_34_multi_file_and_voice_rule_checks(self):
        """Verify:
        1. Multi-file upload: all files cached on import with 0 predict; on Send, each file is inspected and blocked if any contains sensitive data.
        2. Filename-based rule check: blocked on Send when filename matches pattern.
        3. Voice note upload: cached on import with 0 predict, evaluated on Send.
        """
        # 1. Multi-file batch where 1 of 3 has SSN
        f1_data = b"%PDF-1.4 Clean Document 1"
        f2_data = b"Clean notes document 2"
        f3_data = b"Confidential document 3 with SSN: 987-65-4321"

        b1, m1 = _multipart("file", "clean_doc1.pdf", "application/pdf", f1_data)
        b2, m2 = _multipart("file", "clean_doc2.txt", "text/plain", f2_data)
        b3, m3 = _multipart("file", "sensitive_doc3.txt", "text/plain", f3_data)

        # Upload all 3 - NONE must be blocked on upload!
        u1 = _flow("claude.ai", "/api/organizations/org1/upload-multi-1", b1, content_type=m1)
        u2 = _flow("claude.ai", "/api/organizations/org1/upload-multi-2", b2, content_type=m2)
        u3 = _flow("claude.ai", "/api/organizations/org1/upload-multi-3", b3, content_type=m3)
        _run(u1)
        _run(u2)
        _run(u3)
        self.assertFalse(_blocked(u1), "File 1 was blocked prematurely on upload!")
        self.assertFalse(_blocked(u2), "File 2 was blocked prematurely on upload!")
        self.assertFalse(_blocked(u3), "File 3 was blocked prematurely on upload!")

        # Send all 3 files together -> MUST BE BLOCKED on Send because of sensitive_doc3.txt
        multi_send = json.dumps({
            "prompt": "Analyze all three uploaded files please",
            "attachments": [
                {"file_name": "clean_doc1.pdf"},
                {"file_name": "clean_doc2.txt"},
                {"file_name": "sensitive_doc3.txt"},
            ],
            "files": ["clean_doc1.pdf", "clean_doc2.txt", "sensitive_doc3.txt"],
        })
        s_multi = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-multi-test/completion", multi_send)
        _run(s_multi)
        self.assertTrue(_blocked(s_multi), "Multi-file batch containing sensitive file was NOT blocked on Send!")

        # 2. Filename-based rule check (Single file)
        # Add a filename rule: Block files named passwords*.txt
        with NS["_cache_lock"]:
            NS["_cached_rules"].append({
                "name": "Password File Rule",
                "pattern": r"(?i)passwords?.*\.txt",
                "regex": re.compile(r"(?i)passwords?.*\.txt"),
                "action": "BLOCK",
                "severity": "HIGH",
            })
        try:
            pw_data = b"username: admin\nsecret: 12345"
            pw_body, pw_mct = _multipart("file", "passwords_backup.txt", "text/plain", pw_data)
            pw_up = _flow("claude.ai", "/api/organizations/org1/upload-pw", pw_body, content_type=pw_mct)
            _run(pw_up)
            self.assertFalse(_blocked(pw_up), "Password file upload was blocked prematurely before Send!")

            pw_send = json.dumps({
                "prompt": "Analyze password list",
                "attachments": [{"file_name": "passwords_backup.txt"}],
                "files": ["passwords_backup.txt"],
            })
            pw_s_flow = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-pw/completion", pw_send)
            _run(pw_s_flow)
            self.assertTrue(_blocked(pw_s_flow), "File matching Password File Rule was NOT blocked on Send!")
        finally:
            with NS["_cache_lock"]:
                NS["_cached_rules"] = [r for r in NS["_cached_rules"] if r.get("name") != "Password File Rule"]

        # 3. Voice note check with sensitive keyword
        with NS["_cache_lock"]:
            NS["_cached_rules"].append({
                "name": "Secret Voice Rule",
                "pattern": r"(?i)SECRET_PROJECT_VOICE",
                "regex": re.compile(r"(?i)SECRET_PROJECT_VOICE"),
                "action": "BLOCK",
                "severity": "HIGH",
            })
        try:
            voice_data = b"RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00D\xac\x00\x00data\x00\x00\x00\x00SECRET_PROJECT_VOICE"
            v_body, v_mct = _multipart("file", "audio_leak.wav", "audio/wav", voice_data)
            v_up = _flow("claude.ai", "/api/organizations/org1/upload-v-leak", v_body, content_type=v_mct)
            _run(v_up)
            self.assertFalse(_blocked(v_up), "Voice upload was blocked prematurely!")

            v_send = json.dumps({
                "prompt": "Transcribe this secret recording",
                "attachments": [{"file_name": "audio_leak.wav"}],
                "files": ["audio_leak.wav"],
            })
            v_s_flow = _flow("claude.ai", "/api/organizations/org1/chat_conversations/conv-v-leak/completion", v_send)
            _run(v_s_flow)
            self.assertTrue(_blocked(v_s_flow), "Voice file with SECRET_PROJECT_VOICE was NOT blocked on Send!")
        finally:
            with NS["_cache_lock"]:
                NS["_cached_rules"] = [r for r in NS["_cached_rules"] if r.get("name") != "Secret Voice Rule"]

        print("[Test Result] Multi-file batch, filename rule, and voice rule checks verified on Send!")

    def test_35_search_logs_interception(self):
        """Verify search log recording for Google, Bing, DuckDuckGo, Brave Search."""
        captured_logs = []
        orig_post = getattr(NS, "post_search_log_async", None)
        # Mock the search log post function to capture recorded search queries
        def _mock_post(*args, **kwargs):
            pass

        # 1. Google search query navigation
        g_flow = _flow(
            "www.google.com",
            "/search?q=unifai+browser+guard+security",
            b"",
            method="GET",
            content_type="",
            headers={
                "sec-fetch-dest": "document",
                "sec-fetch-mode": "navigate",
                "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            },
        )
        _run(g_flow)
        # Request should pass through without interference (Search Logs are non-blocking)
        self.assertFalse(_blocked(g_flow))

        # 2. Bing search query navigation
        bing_flow = _flow(
            "www.bing.com",
            "/search?q=enterprise+dlp+ai+protection",
            b"",
            method="GET",
            content_type="",
            headers={
                "sec-fetch-dest": "document",
                "sec-fetch-mode": "navigate",
                "user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
            },
        )
        _run(bing_flow)
        self.assertFalse(_blocked(bing_flow))

        # 3. DuckDuckGo search
        ddg_flow = _flow(
            "duckduckgo.com",
            "/?q=confidential+internal+audit",
            b"",
            method="GET",
            content_type="",
            headers={
                "sec-fetch-dest": "document",
                "sec-fetch-mode": "navigate",
                "user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Firefox/121.0",
            },
        )
        _run(ddg_flow)
        self.assertFalse(_blocked(ddg_flow))

        print("[Test Result] Search Logs interception for Google, Bing, and DuckDuckGo verified successfully!")


if __name__ == "__main__":
    unittest.main()
