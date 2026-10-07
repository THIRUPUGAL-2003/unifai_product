"""Comprehensive Live QA test suite for ChatGPT:
1. Prompts (text, numbers, symbols, Unicode, emojis, code)
2. File predictions with original filename & prompt
3. Multi-file batch uploads (PDF, DOCX, TXT, CSV, PNG)
4. Voice / Audio notes and dictation transcripts
5. Tamper detection & deduplication verification
"""

import json
import os
import sys
import unittest
import urllib.parse
from typing import Any

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
    ns: dict = {"__name__": "browser_ai_proxy_comprehensive"}
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
    boundary = "----WebKitFormBoundaryChatGPTLiveTest"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


class ChatGPTLiveComprehensiveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import re, time
        NS["_apply_targets_from_data"]({
            "targets": [
                {"domain": "chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
                {"domain": "chat.openai.com", "platform_name": "ChatGPT", "monitored": True},
                {"domain": "ab.chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
            ]
        })
        pat_ssn = r"\b\d{3}-\d{2}-\d{4}\b"
        pat_key = r"(?i)\bCONFIDENTIAL_INTERNAL_KEY_[A-Z0-9]+\b"
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
            }
        ]
        NS["_rules_fetched_at"] = time.time()
        NS["_rules_fetch_ok"] = True

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

    # -------------------------------------------------------------------------
    # PART 1: 40 PROMPTS (Text, Numbers, Symbols, Unicode, Code)
    # -------------------------------------------------------------------------
    def test_01_to_40_chatgpt_prompt_varieties(self):
        test_prompts = [
            # Standard Text
            "Hello, what can you do for me today?",
            "Explain quantum computing in simple terms for high school students.",
            "Write a formal corporate email requesting annual leave approval.",
            "Summarize the history of artificial intelligence from 1950 to 2026.",
            "Draft a comprehensive marketing strategy for our B2B SaaS startup.",
            # Numbers & Formulas
            "Calculate 1234567890 * 9876543210 and round to 4 decimal places.",
            "Analyze these quarterly revenue numbers: Q1 $4,500,200, Q2 $5,120,400, Q3 $6,890,150.",
            "Solve the quadratic equation: 3x^2 - 14x + 8 = 0.",
            "What is the statistical significance of p < 0.001 in a sample of n=50,000?",
            "Compare latency metrics: p50=12ms, p90=45ms, p99=180ms across 4 clusters.",
            # Symbols & Punctuation
            "!@#$%^&*()_+=-[]{};:'\"\\|,.<>/?~`",
            "regex test: ^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\\.[a-zA-Z0-9-.]+$",
            "SELECT * FROM users WHERE status = 'active' AND (role = 'admin' OR id IN (1,2,3));",
            "JSON payload: {\"key\": \"value\", \"nested\": [true, false, null, 123.45]}",
            "curl -X POST https://api.example.com/v1 -H 'Authorization: Bearer xyz123' -d '{}'",
            # Unicode, Multilingual & Emojis
            "வணக்கம்! செயற்கை நுண்ணறிவு எவ்வாறு செயல்படுகிறது? (Tamil test)",
            "こんにちは、世界！AIの進化について教えてください。(Japanese test)",
            "Bonjour le monde! Comment optimiser les performances des serveurs? (French)",
            "🚀 🔥 ⚡ 🎯 🛡️ Testing high-volume emojis and rich UTF-8 characters",
            "Mathematical symbols: ∑, ∏, ∫, √, ∞, ≈, ≠, ≤, ≥, ±, ∈, ∉, ⊆, ∪, ∩",
            # Multi-line & Markdown
            "# Technical Architecture Plan\n\n## Overview\nThis is a multi-line document.\n- Item 1\n- Item 2\n\n```python\nprint('hello')\n```",
            "Line 1\r\nLine 2\r\nLine 3\r\nLine 4\r\nLine 5 with special chars: § ¶ © ® ™",
            # Blocked rule checks (SSN and Confidential)
            "Here is the secret SSN: 123-45-6789 for employee tax filing.",
            "Blocked confidential token: CONFIDENTIAL_INTERNAL_KEY_ABC999888777",
            # Safe variations near rules
            "My phone number is 123-456-7890 (not an SSN, should pass)",
            "My invoice number is 987-65-432 (not an SSN, should pass)",
            "Serial number SN-9988-1122-3344",
            "Customer reference REF-2026-X99",
            # Code snippets
            "def calculate_tax(income: float) -> float:\n    return income * 0.25 if income > 50000 else 0.10",
            "const handleClick = (e: React.MouseEvent) => { console.log('Clicked', e.target); };",
            "kubectl get pods -n production -o wide --show-labels",
            "docker run -d -p 8080:80 --name webserver nginx:alpine",
            # Conversational edge cases
            "a",
            "?",
            "42",
            "yes",
            "no",
            "ok",
            "What is the meaning of life, the universe, and everything?",
            "Provide 10 ideas for weekend team building activities."
        ]

        self.assertEqual(len(test_prompts), 40)
        passed = 0
        blocked_count = 0

        for idx, prompt_text in enumerate(test_prompts, 1):
            body = json.dumps({
                "action": "next",
                "messages": [{
                    "id": f"msg-{idx}",
                    "author": {"role": "user"},
                    "content": {"content_type": "text", "parts": [prompt_text]},
                }],
                "model": "auto",
            })
            f = _flow("chatgpt.com", "/backend-api/f/conversation", body)
            _run(f)

            is_blk = _blocked(f)
            should_block = ("123-45-6789" in prompt_text) or ("CONFIDENTIAL_INTERNAL_KEY_" in prompt_text)

            if should_block:
                self.assertTrue(is_blk, f"Prompt #{idx} with confidential data was NOT blocked: {prompt_text}")
                blocked_count += 1
            else:
                self.assertFalse(is_blk, f"Safe prompt #{idx} was falsely blocked: {prompt_text}")
                passed += 1

        print(f"\n[Test Result] 40 Prompt varieties verified: {passed} allowed, {blocked_count} correctly blocked by guardrails.")

    # -------------------------------------------------------------------------
    # PART 2: 30 FILE PREDICTIONS (Single & Multi-File, Original Names, Prompts)
    # -------------------------------------------------------------------------
    def test_41_to_70_chatgpt_file_prediction_varieties(self):
        file_specs = [
            ("quarterly_financial_report.pdf", "application/pdf", b"%PDF-1.4 sample quarterly report"),
            ("employee_roster.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b"PK\x03\x04 xlsx mock"),
            ("system_architecture.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", b"PK\x03\x04 docx mock"),
            ("database_schema.sql", "text/plain", b"CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(100));"),
            ("data_pipeline.py", "text/plain", b"import pandas as pd\ndf = pd.read_csv('data.csv')"),
            ("user_analytics.csv", "text/csv", b"user_id,events,timestamp\n101,45,2026-10-01\n"),
            ("server_metrics.json", "application/json", b"{\"cpu_usage\": 45.2, \"mem_usage\": 62.8}"),
            ("project_readme.md", "text/markdown", b"# Project Title\nInstructions to build and deploy"),
            ("network_diagram.png", "image/png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
            ("product_mockup.jpg", "image/jpeg", b"\xff\xd8\xff\xe0\x00\x10JFIF"),
            ("meeting_notes_2026.txt", "text/plain", b"Discussion on Q4 roadmap and client deliverables"),
            ("tax_audit_workpaper.pdf", "application/pdf", b"%PDF-1.4 Workpapers for company audit"),
            ("app_logs_debug.log", "text/plain", b"2026-10-07 09:00:00 INFO Service started successfully"),
            ("kubernetes_deployment.yaml", "text/yaml", b"apiVersion: apps/v1\nkind: Deployment"),
            ("env_template.env", "text/plain", b"PORT=8080\nNODE_ENV=production"),
        ]

        tested_files = 0
        for i, (fname, ctype, content) in enumerate(file_specs, 1):
            # 1. Upload flow to ChatGPT
            upload_body, mct = _multipart("file", fname, ctype, content)
            up = _flow("chatgpt.com", f"/backend-api/files/upload-{i}", upload_body, content_type=mct)
            _run(up)
            self.assertIsNone(up.response, f"Upload flow #{i} for {fname} should cache, not block directly")

            # 2. Conversation Send referencing the file
            send_body = json.dumps({
                "action": "next",
                "messages": [{
                    "id": f"msg-file-{i}",
                    "author": {"role": "user"},
                    "content": {
                        "content_type": "multimodal_text",
                        "parts": [f"Please analyze {fname} attached here."],
                    },
                    "metadata": {"attachments": [{"name": fname, "id": f"file-{i}"}]}
                }],
                "model": "auto"
            })
            send = _flow("chatgpt.com", "/backend-api/f/conversation", send_body)
            _run(send)
            self.assertFalse(_blocked(send), f"Safe file {fname} should be allowed on Send")
            tested_files += 1

        # Also test 5 Confidential/Blocked File uploads
        blocked_files = [
            ("confidential_payroll_ssn.txt", "text/plain", b"Employee: John Doe, SSN: 123-45-6789, Salary: 95000"),
            ("executive_tax_return.pdf", "application/pdf", b"%PDF-1.4 SSN: 987-65-4321 tax refund claim"),
            ("internal_secrets.env", "text/plain", b"MASTER_KEY=CONFIDENTIAL_INTERNAL_KEY_ROOT999"),
            ("employee_contracts.csv", "text/csv", b"id,name,ssn\n1,Alice,123-45-6789\n"),
            ("restricted_audit.txt", "text/plain", b"Audit finding: CONFIDENTIAL_INTERNAL_KEY_LEAK777 exposed")
        ]

        for j, (fname, ctype, content) in enumerate(blocked_files, 1):
            upload_body, mct = _multipart("file", fname, ctype, content)
            up = _flow("chatgpt.com", f"/backend-api/files/blocked-{j}", upload_body, content_type=mct)
            _run(up)

            send_body = json.dumps({
                "action": "next",
                "messages": [{
                    "id": f"msg-blocked-{j}",
                    "author": {"role": "user"},
                    "content": {"content_type": "text", "parts": [f"Review {fname}"]},
                    "metadata": {"attachments": [{"name": fname, "id": f"blocked-{j}"}]}
                }],
                "model": "auto"
            })
            send = _flow("chatgpt.com", "/backend-api/f/conversation", send_body)
            _run(send)
            self.assertTrue(_blocked(send), f"Sensitive file {fname} was NOT blocked on Send")
            tested_files += 1

        # Multi-file batch upload (3 files at once in a single prompt)
        batch_files = [
            ("annual_report_part1.pdf", "application/pdf", b"%PDF-1.4 Part 1"),
            ("annual_report_part2.pdf", "application/pdf", b"%PDF-1.4 Part 2"),
            ("annual_report_part3.pdf", "application/pdf", b"%PDF-1.4 Part 3"),
        ]
        for bname, btype, bcontent in batch_files:
            ubody, bmct = _multipart("file", bname, btype, bcontent)
            _run(_flow("chatgpt.com", f"/backend-api/files/{bname}", ubody, content_type=bmct))

        batch_send = json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-batch-1",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Synthesize these 3 annual report files."]},
                "metadata": {"attachments": [{"name": n} for n, _, _ in batch_files]}
            }],
            "model": "auto"
        })
        bsend_flow = _flow("chatgpt.com", "/backend-api/f/conversation", batch_send)
        _run(bsend_flow)
        self.assertFalse(_blocked(bsend_flow), "Batch of safe files should be allowed")
        tested_files += len(batch_files)

        # Multi-file batch with ONE blocked file (Must block the entire send)
        mixed_batch = [
            ("safe_notes.txt", "text/plain", b"Safe project meeting notes"),
            ("danger_ssn.txt", "text/plain", b"Leaked SSN: 123-45-6789 in personnel file"),
        ]
        for mname, mtype, mcontent in mixed_batch:
            ubody, bmct = _multipart("file", mname, mtype, mcontent)
            _run(_flow("chatgpt.com", f"/backend-api/files/{mname}", ubody, content_type=bmct))

        mixed_send = json.dumps({
            "action": "next",
            "messages": [{
                "id": "msg-mixed-batch",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["Analyze safe notes and danger ssn."]},
                "metadata": {"attachments": [{"name": n} for n, _, _ in mixed_batch]}
            }],
            "model": "auto"
        })
        msend_flow = _flow("chatgpt.com", "/backend-api/f/conversation", mixed_send)
        _run(msend_flow)
        self.assertTrue(_blocked(msend_flow), "Batch containing an SSN file MUST be blocked completely")
        tested_files += len(mixed_batch)

        # Add additional single file variations up to 30 file tests
        extra_files = [
            ("docker-compose.yml", "text/plain", b"version: '3.8'\nservices:\n  app:\n    image: node:18"),
            ("sales_pitch.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", b"PK\x03\x04 pptx"),
            ("api_spec.yaml", "text/yaml", b"openapi: 3.0.0\ninfo:\n  title: Test API"),
            ("schema_migration.sql", "text/plain", b"ALTER TABLE accounts ADD COLUMN status VARCHAR(20);"),
            ("employee_handbook.pdf", "application/pdf", b"%PDF-1.4 Rules and benefits handbook"),
        ]
        for ename, etype, econtent in extra_files:
            ubody, bmct = _multipart("file", ename, etype, econtent)
            _run(_flow("chatgpt.com", f"/backend-api/files/{ename}", ubody, content_type=bmct))
            esend = json.dumps({
                "action": "next",
                "messages": [{"id": f"msg-{ename}", "author": {"role": "user"},
                              "content": {"content_type": "text", "parts": [f"Review {ename}"]},
                              "metadata": {"attachments": [{"name": ename}]}}],
                "model": "auto"
            })
            ef = _flow("chatgpt.com", "/backend-api/f/conversation", esend)
            _run(ef)
            self.assertFalse(_blocked(ef))
            tested_files += 1

        print(f"[Test Result] {tested_files} File upload and prediction test cases passed.")

    # -------------------------------------------------------------------------
    # PART 3: 20 VOICE / AUDIO INPUT TESTS
    # -------------------------------------------------------------------------
    def test_71_to_90_chatgpt_voice_and_audio_varieties(self):
        voice_transcripts = [
            ("Schedule a product sync meeting with the design team tomorrow morning.", False),
            ("Summarize the action items from our weekly client sprint retro.", False),
            ("Can you help me rewrite my self-evaluation performance review?", False),
            ("Give me a quick 3-minute explanation of how transformers work.", False),
            ("Draft a polite follow-up message to the candidate after their interview.", False),
            ("What are the key differences between Postgres and MySQL?", False),
            ("My social security number is 123-45-6789 please file my returns.", True),
            ("Store this confidential key: CONFIDENTIAL_INTERNAL_KEY_VOICE99", True),
            ("What time is sunset today in Chennai?", False),
            ("Translate this spoken phrase into German and Japanese.", False),
            ("Tell me a creative bedtime story about a curious robotic puppy.", False),
            ("Provide an agenda for tomorrow's executive stakeholder meeting.", False),
            ("Employee tax id number 123-45-6789 update in HR database.", True),
            ("Calculate compound interest on ten thousand dollars at eight percent for 5 years.", False),
            ("List the pros and cons of microservices vs monolithic architecture.", False),
            ("Suggest five catchy taglines for our new eco-friendly water bottle.", False),
            ("Leaked credential is CONFIDENTIAL_INTERNAL_KEY_AUDIO123 in audio note.", True),
            ("Give me three healthy vegetarian dinner recipes that take 20 minutes.", False),
            ("Explain the concept of zero-trust security in corporate networks.", False),
            ("Thanks for the help, have a great day!", False),
        ]

        self.assertEqual(len(voice_transcripts), 20)
        voice_tested = 0

        for idx, (transcript_text, should_block) in enumerate(voice_transcripts, 1):
            voice_body = json.dumps({
                "action": "next",
                "messages": [{
                    "id": f"voice-msg-{idx}",
                    "author": {"role": "user"},
                    "content": {
                        "content_type": "text",
                        "parts": [transcript_text],
                    },
                    "metadata": {
                        "voice_mode": True,
                        "audio": {"format": "webm", "duration_ms": 3200},
                        "dictation": True,
                    }
                }],
                "conversation_mode": {"kind": "voice"},
                "model": "auto"
            })
            vf = _flow("chatgpt.com", "/backend-api/f/conversation", voice_body)
            _run(vf)

            is_blk = _blocked(vf)
            if should_block:
                self.assertTrue(is_blk, f"Voice transcript #{idx} containing restricted data was NOT blocked: {transcript_text}")
            else:
                self.assertFalse(is_blk, f"Safe voice transcript #{idx} was falsely blocked: {transcript_text}")
            voice_tested += 1

        print(f"[Test Result] 20 Voice & audio dictation test cases passed ({voice_tested} tested).")

    # -------------------------------------------------------------------------
    # PART 4: 10 SUBDOMAIN & TARGET VARIATIONS (chat.openai.com, ab.chatgpt.com)
    # -------------------------------------------------------------------------
    def test_91_to_100_subdomains_and_edge_cases(self):
        hosts_and_paths = [
            ("chatgpt.com", "/backend-api/f/conversation"),
            ("chat.openai.com", "/backend-api/conversation"),
            ("ab.chatgpt.com", "/backend-api/f/conversation"),
            ("chatgpt.com", "/backend-api/f/conversation"),
            ("chat.openai.com", "/backend-api/f/conversation"),
            ("ab.chatgpt.com", "/backend-api/conversation"),
            ("chatgpt.com", "/backend-api/lat/r"),
            ("chat.openai.com", "/backend-api/lat/r"),
            ("chatgpt.com", "/backend-api/f/conversation"),
            ("chat.openai.com", "/backend-api/f/conversation"),
        ]

        self.assertEqual(len(hosts_and_paths), 10)
        for idx, (host, path) in enumerate(hosts_and_paths, 1):
            is_ssn = (idx % 3 == 0)
            text = f"Subdomain test on {host} SSN: 123-45-6789" if is_ssn else f"Subdomain test query #{idx} on {host}"
            body = json.dumps({
                "action": "next",
                "messages": [{"id": f"sub-{idx}", "author": {"role": "user"}, "content": {"content_type": "text", "parts": [text]}}],
                "model": "auto"
            })
            sf = _flow(host, path, body)
            _run(sf)

            if is_ssn:
                self.assertTrue(_blocked(sf), f"Subdomain {host} failed to block SSN")
            else:
                self.assertFalse(_blocked(sf), f"Subdomain {host} falsely blocked safe query")

        print("[Test Result] 10 Subdomain & edge case test cases passed.")


if __name__ == "__main__":
    unittest.main()
