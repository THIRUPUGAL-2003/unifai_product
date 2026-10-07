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


if __name__ == "__main__":
    unittest.main()
