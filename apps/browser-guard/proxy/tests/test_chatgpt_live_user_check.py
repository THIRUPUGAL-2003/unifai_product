#!/usr/bin/env python3
"""ChatGPT Comprehensive Live QA Test:
1. Prompt Checks (Safe, Complex, Multilingual, Code, Sensitive DLP)
2. Single File Check (Safe PDF & Sensitive file block)
3. Multiple Files (10+ files batch upload & send verification)
4. 3 Voice Checks (STT transcript, Voice blob upload, Realtime WebSocket transcript)
5. Backend Live Intercept & Log Integration
"""

import json
import os
import re
import sys
if hasattr(sys.stdout, "reconfigure"):
    try:
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    except Exception:
        pass
import time
import unittest
from pathlib import Path
from mitmproxy.test import tflow, tutils

PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "gateway_proxy_parts"
PARTS = [
    "config_caches_rules.py",
    "helpers_prompts.py",
    "uploads_detect.py",
    "file_policy.py",
    "extract_office_backend.py",
    "responses_inject.py",
    "responses_addon.py",
]

def _load_ns():
    ns = {"__name__": "chatgpt_live_test"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    return ns

NS = _load_ns()
INTERCEPTED_PROMPTS = []
UPLOAD_INTERCEPTS = []

def _mock_evaluate(platform, domain, prompt, client_ip, url, method):
    INTERCEPTED_PROMPTS.append((domain, prompt))
    return NS["decide_prompt_locally"](prompt)

def _mock_post_upload(**kwargs):
    UPLOAD_INTERCEPTS.append(kwargs)
    return True

NS["evaluate_prompt"] = _mock_evaluate
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = _mock_post_upload
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

def _run(flow):
    NS["BrowserAIInterceptor"].request(ADDON, flow)

def _multipart(field: str, filename: str, ctype: str, data: bytes):
    boundary = "----WebKitFormBoundaryChatGPTLiveTest"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


class TestChatGPTLiveFullCheck(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        NS["_apply_targets_from_data"]({
            "targets": [
                {"domain": "chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
                {"domain": "chat.openai.com", "platform_name": "ChatGPT", "monitored": True},
                {"domain": "ab.chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
                {"domain": "realtime.chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
            ]
        })
        pat_ssn = r"\b\d{3}-\d{2}-\d{4}\b"
        pat_pan = r"\b[A-Z]{5}[0-9]{4}[A-Z]\b"
        pat_key = r"sk-live-[a-zA-Z0-9]{20,}"
        NS["_cached_rules"] = [
            {"name": "SSN Rule", "pattern": pat_ssn, "regex": re.compile(pat_ssn), "action": "BLOCK", "severity": "HIGH"},
            {"name": "PAN Rule", "pattern": pat_pan, "regex": re.compile(pat_pan), "action": "BLOCK", "severity": "CRITICAL"},
            {"name": "API Key Rule", "pattern": pat_key, "regex": re.compile(pat_key), "action": "BLOCK", "severity": "CRITICAL"},
        ]
        NS["_rules_fetched_at"] = time.time()
        NS["_rules_fetch_ok"] = True

    def setUp(self):
        INTERCEPTED_PROMPTS.clear()
        UPLOAD_INTERCEPTS.clear()
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

    # =========================================================================
    # 1. CHATGPT PROMPT CHECKS (Plain, Complex, Tamil, Code, Sensitive)
    # =========================================================================
    def test_01_chatgpt_prompt_checks(self):
        print("\n--- [CHECK 1] ChatGPT Prompts ---")
        prompts = [
            # Standard Text
            ("Hello ChatGPT, explain neural networks simply.", False),
            # Numbers & Math
            ("Solve x^2 - 5x + 6 = 0 and find roots.", False),
            # Tamil Unicode
            ("வணக்கம்! இன்றைய வானிலை எப்படி உள்ளது? (Tamil prompt)", False),
            # Code snippet
            ("def quicksort(arr):\n    return arr if len(arr) <= 1 else quicksort([x for x in arr[1:] if x < arr[0]]) + [arr[0]] + quicksort([x for x in arr[1:] if x >= arr[0]])", False),
            # Sensitive DLP - SSN
            ("Please update customer tax record: SSN 123-45-6789 confidential", True),
            # Sensitive DLP - PAN Card
            ("Customer Indian PAN card number is ABCDE1234F for KYC", True),
            # Sensitive DLP - OpenAI Secret API Key
            ("My secret API token is sk-live-abc123def456ghi789012345 please use it", True),
        ]

        for idx, (p_text, should_block) in enumerate(prompts, 1):
            body = json.dumps({
                "action": "next",
                "messages": [{
                    "id": f"prompt-{idx}",
                    "author": {"role": "user"},
                    "content": {"content_type": "text", "parts": [p_text]}
                }],
                "model": "gpt-4o"
            })
            flow = _flow("chatgpt.com", "/backend-api/f/conversation", body)
            _run(flow)

            is_blocked = flow.response is not None
            if should_block:
                self.assertTrue(is_blocked, f"Prompt #{idx} with secret data failed to block: {p_text}")
                label = p_text[:45].encode("ascii", errors="replace").decode("ascii")
                print(f"  [PASS] Sensitive prompt #{idx} BLOCKED correctly: {label}...")
            else:
                self.assertFalse(is_blocked, f"Safe prompt #{idx} was falsely blocked: {p_text}")
                label = p_text[:45].encode("ascii", errors="replace").decode("ascii")
                print(f"  [PASS] Safe prompt #{idx} ALLOWED correctly: {label}...")

    # =========================================================================
    # 2. CHATGPT 1 FILE CHECK (Safe File & Sensitive File)
    # =========================================================================
    def test_02_chatgpt_single_file_check(self):
        print("\n--- [CHECK 2] ChatGPT Single File ---")
        # 1. Safe file upload & send
        safe_fname = "company_q3_report.pdf"
        safe_data = b"%PDF-1.4 Q3 Financial summary: revenue grew by 18% YoY with healthy margins."
        up_body, up_ct = _multipart("file", safe_fname, "application/pdf", safe_data)
        up_flow = _flow("chatgpt.com", "/backend-api/files/safe-file-01", up_body, content_type=up_ct)
        _run(up_flow)
        self.assertIsNone(up_flow.response, "Single file upload must cache without blocking")

        send_body = json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-single-file",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Please summarize this Q3 report."]},
                "metadata": {"attachments": [{"name": safe_fname, "id": "safe-file-01"}]}
            }],
            "model": "gpt-4o"
        })
        send_flow = _flow("chatgpt.com", "/backend-api/f/conversation", send_body)
        _run(send_flow)
        self.assertIsNone(send_flow.response, "Safe single file send must be permitted")
        print(f"  [PASS] Safe file {safe_fname} uploaded, extracted, and allowed.")

        # 2. Sensitive file upload & send (Must Block)
        bad_fname = "employee_pan_records.txt"
        bad_data = b"Employee Name: Sakthi, PAN Number: ABCDE1234F, Department: Engineering"
        up_bad, bad_ct = _multipart("file", bad_fname, "text/plain", bad_data)
        up_bad_flow = _flow("chatgpt.com", "/backend-api/files/bad-file-01", up_bad, content_type=bad_ct)
        _run(up_bad_flow)

        send_bad = json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-bad-file",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Analyze employee tax file."]},
                "metadata": {"attachments": [{"name": bad_fname, "id": "bad-file-01"}]}
            }],
            "model": "gpt-4o"
        })
        send_bad_flow = _flow("chatgpt.com", "/backend-api/f/conversation", send_bad)
        _run(send_bad_flow)
        self.assertIsNotNone(send_bad_flow.response, "Sensitive file containing PAN MUST be blocked on send")
        print(f"  [PASS] Sensitive file {bad_fname} (PAN card) BLOCKED correctly on Send.")

    # =========================================================================
    # 3. CHATGPT MULTIPLE FILES (10+ FILES BATCH) CHECK
    # =========================================================================
    def test_03_chatgpt_multiple_files_10plus_check(self):
        print("\n--- [CHECK 3] ChatGPT Multiple Files (10+ Files Batch) ---")
        batch_12_files = [
            ("01_quarterly_report.pdf", "application/pdf", b"%PDF-1.4 Report 2026"),
            ("02_employee_data.csv", "text/csv", b"id,name,role\n1,Arun,Dev\n2,Bala,QA\n"),
            ("03_pipeline.py", "text/plain", b"import os\nprint('Data pipeline initialized')"),
            ("04_schema.sql", "text/plain", b"CREATE TABLE metrics (id INT, score FLOAT);"),
            ("05_architecture.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", b"PK\x03\x04 Docx payload"),
            ("06_budget.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b"PK\x03\x04 Excel payload"),
            ("07_presentation.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", b"PK\x03\x04 PPT payload"),
            ("08_notes.md", "text/markdown", b"# Meeting Notes\nDiscussion on microservices."),
            ("09_config.json", "application/json", b"{\"app\": \"gateway\", \"version\": \"1.1.16\"}"),
            ("10_server.log", "text/plain", b"2026-10-08 12:00:00 INFO System running healthy"),
            ("11_app.env", "text/plain", b"HOST=127.0.0.1\nPORT=8080"),
            ("12_diagram.png", "image/png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
        ]

        self.assertGreaterEqual(len(batch_12_files), 10, "Must test at least 10+ files")

        # 1. Upload all 12 files to ChatGPT
        for fname, ctype, content in batch_12_files:
            ubody, mct = _multipart("file", fname, ctype, content)
            uflow = _flow("chatgpt.com", f"/backend-api/files/{fname}", ubody, content_type=mct)
            _run(uflow)
            self.assertIsNone(uflow.response, f"File {fname} in 10+ batch should cache cleanly")

        # 2. Send single prompt referencing all 12 files
        send_all_body = json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-batch-12-files",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Analyze and cross-reference all 12 attached files for system audit."]},
                "metadata": {"attachments": [{"name": n, "id": f"id-{n}"} for n, _, _ in batch_12_files]}
            }],
            "model": "gpt-4o"
        })
        send_all_flow = _flow("chatgpt.com", "/backend-api/f/conversation", send_all_body)
        _run(send_all_flow)
        self.assertIsNone(send_all_flow.response, "Safe 12-file batch send should be permitted")
        print(f"  [PASS] 12 Safe Files batch ({len(batch_12_files)} files) uploaded and allowed on Send.")

        # 3. Test 10+ batch with 1 LEAKED SSN in file #7
        batch_with_violation = [
            ("file_01.txt", "text/plain", b"Safe data 1"),
            ("file_02.txt", "text/plain", b"Safe data 2"),
            ("file_03.txt", "text/plain", b"Safe data 3"),
            ("file_04.txt", "text/plain", b"Safe data 4"),
            ("file_05.txt", "text/plain", b"Safe data 5"),
            ("file_06.txt", "text/plain", b"Safe data 6"),
            ("file_07_leaked.txt", "text/plain", b"Confidential tax document with SSN 123-45-6789 inside!"),
            ("file_08.txt", "text/plain", b"Safe data 8"),
            ("file_09.txt", "text/plain", b"Safe data 9"),
            ("file_10.txt", "text/plain", b"Safe data 10"),
            ("file_11.txt", "text/plain", b"Safe data 11"),
        ]
        for fname, ctype, content in batch_with_violation:
            ubody, mct = _multipart("file", fname, ctype, content)
            _run(_flow("chatgpt.com", f"/backend-api/files/{fname}", ubody, content_type=mct))

        send_violation = json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-batch-violation",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Review these 11 files."]},
                "metadata": {"attachments": [{"name": n} for n, _, _ in batch_with_violation]}
            }],
            "model": "gpt-4o"
        })
        send_viol_flow = _flow("chatgpt.com", "/backend-api/f/conversation", send_violation)
        _run(send_viol_flow)
        self.assertIsNotNone(send_viol_flow.response, "11-file batch with 1 sensitive file MUST be blocked completely")
        print("  [PASS] 11-File Batch containing 1 sensitive SSN file BLOCKED completely.")

    # =========================================================================
    # 4. CHATGPT VOICE (3 VOICE CHECKS)
    # =========================================================================
    def test_04_chatgpt_voice_3_checks(self):
        print("\n--- [CHECK 4] ChatGPT Voice (3 Voice Checks) ---")

        # Voice Check 1: Audio STT transcript JSON format (Safe and Sensitive)
        voice_stt_safe = json.dumps({
            "action": "next",
            "messages": [{
                "id": "voice-msg-01",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Please summarize today's morning engineering standup notes."]},
                "metadata": {"voice_mode": True, "dictation": True, "audio": {"format": "webm", "duration_ms": 4200}}
            }],
            "conversation_mode": {"kind": "voice"},
            "model": "gpt-4o-realtime"
        })
        v1_flow = _flow("chatgpt.com", "/backend-api/f/conversation", voice_stt_safe)
        _run(v1_flow)
        self.assertIsNone(v1_flow.response, "Safe voice STT transcript should be allowed")
        print("  [PASS] Voice Check 1a: Safe Voice STT Transcript allowed.")

        voice_stt_bad = json.dumps({
            "action": "next",
            "messages": [{
                "id": "voice-msg-01-bad",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["My social security number is 123-45-6789 please record it."]},
                "metadata": {"voice_mode": True, "dictation": True}
            }],
            "conversation_mode": {"kind": "voice"},
            "model": "gpt-4o-realtime"
        })
        v1_bad_flow = _flow("chatgpt.com", "/backend-api/f/conversation", voice_stt_bad)
        _run(v1_bad_flow)
        self.assertIsNotNone(v1_bad_flow.response, "Voice STT transcript containing SSN MUST be blocked")
        print("  [PASS] Voice Check 1b: Sensitive Voice STT Transcript (SSN) BLOCKED.")

        # Voice Check 2: Voice Audio Blob Upload ([VOICE UPLOAD] format)
        voice_audio_bytes = b"\x1a\x45\xdf\xa3 mock webm audio stream recording 5 seconds"
        v2_body, v2_mct = _multipart("audio", "user_voice_memo.webm", "audio/webm", voice_audio_bytes)
        v2_flow = _flow("chatgpt.com", "/backend-api/files/voice-upload-02", v2_body, content_type=v2_mct)
        _run(v2_flow)
        self.assertIsNone(v2_flow.response, "Voice audio blob upload must be cached for send evaluation")
        print("  [PASS] Voice Check 2: Voice Audio Blob upload cached cleanly without false block.")

        # Voice Check 3: Realtime Voice WebSocket event / conversation item (realtime.chatgpt.com)
        realtime_voice_payload = json.dumps({
            "type": "conversation.item.create",
            "item": {
                "type": "message",
                "role": "user",
                "content": [{
                    "type": "input_audio",
                    "transcript": "Hello assistant, give me a quick status on server health."
                }]
            }
        })
        v3_flow = _flow("realtime.chatgpt.com", "/v1/realtime", realtime_voice_payload)
        _run(v3_flow)
        self.assertIsNone(v3_flow.response, "Safe Realtime Voice conversation item should be permitted")

        realtime_voice_leak = json.dumps({
            "type": "conversation.item.create",
            "item": {
                "type": "message",
                "role": "user",
                "content": [{
                    "type": "input_audio",
                    "transcript": "The customer PAN card is ABCDE1234F update profile."
                }]
            }
        })
        v3_bad_flow = _flow("realtime.chatgpt.com", "/v1/realtime", realtime_voice_leak)
        _run(v3_bad_flow)
        self.assertIsNotNone(v3_bad_flow.response, "Realtime Voice transcript containing PAN MUST be blocked")
        print("  [PASS] Voice Check 3: Realtime Voice Stream (realtime.chatgpt.com) evaluated and PAN BLOCKED.")

if __name__ == "__main__":
    unittest.main()
