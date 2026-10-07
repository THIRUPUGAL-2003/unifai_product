#!/usr/bin/env python3
"""Claude: prompt-only exact predict, no mid-word noise, CDN file → filename -- prompt + rules."""

from __future__ import annotations

import json
import re
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


def _load():
    ns: dict = {"__name__": "browser_ai_proxy_claude_predict"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    ns["_refresh_targets_from_backend"] = lambda: None
    return ns


NS = _load()
EVALS: list[tuple[str, str]] = []
UPLOAD_LOGS: list[dict] = []


def _fake_evaluate(platform, domain, prompt, client_ip, url, method):
    EVALS.append((domain, prompt))
    return NS["decide_prompt_locally"](prompt)


def _fake_upload_log(**kwargs):
    UPLOAD_LOGS.append(kwargs)
    return True


NS["evaluate_prompt"] = _fake_evaluate
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = _fake_upload_log
ADDON = object.__new__(NS["BrowserAIInterceptor"])


def _arm() -> None:
    NS["_apply_targets_from_data"]({
        "targets": [
            {"domain": "claude.ai", "platform_name": "Claude", "monitored": True},
            {
                "domain": "files.claudeusercontent.com",
                "platform_name": "Claude",
                "monitored": True,
                "host_role": "file",
            },
        ]
    })
    pat = r"\b\d{3}-\d{2}-\d{4}\b"
    NS["_cached_rules"] = [{
        "name": "SSN Rule",
        "pattern": pat,
        "regex": re.compile(pat, re.IGNORECASE),
        "action": "BLOCK",
        "severity": "HIGH",
        "warning_message": "SSN blocked",
    }]
    NS["_rules_fetched_at"] = time.time()
    NS["_rules_fetch_ok"] = True
    NS["_controls_fetched_at"] = time.time()


def _reset() -> None:
    EVALS.clear()
    UPLOAD_LOGS.clear()
    for lock_name, store_names in (
        ("_FILE_ID_NAME_REGISTRY_LOCK", ["_FILE_ID_NAME_REGISTRY"]),
        ("_DOMAIN_PENDING_NAMES_LOCK", ["_DOMAIN_PENDING_NAMES"]),
        ("_CONTENT_HASH_NAME_LOCK", ["_CONTENT_HASH_NAME_REGISTRY"]),
        ("_UPLOAD_FILE_CACHE_LOCK", ["_UPLOAD_FILE_CACHE", "_UPLOAD_FILE_QUEUES"]),
        ("_CLIENT_TARGET_STICKY_LOCK", ["_CLIENT_TARGET_STICKY"]),
    ):
        with NS[lock_name]:
            for s in store_names:
                NS[s].clear()
    for name in ("_recent_prompts", "_recent_decisions", "_composer_draft", "_FILE_SEND_BLOCKS"):
        store = NS.get(name)
        if isinstance(store, dict):
            store.clear()


def _flow(host: str, path: str, body: bytes | str, *, method: str = "POST",
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


def _multipart(filename: str, ctype: str, data: bytes):
    boundary = "----WebKitFormBoundaryClaudePredict"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\n"
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


class ClaudePromptFilePredictTests(unittest.TestCase):
    def setUp(self) -> None:
        _reset()
        _arm()

    def test_claude_domain_is_monitored(self) -> None:
        ok, domain, plat = NS["detect_target"]("claude.ai")
        self.assertTrue(ok)
        self.assertEqual(domain, "claude.ai")
        self.assertEqual(plat, "Claude")
        self.assertTrue(NS["detect_target"]("files.claudeusercontent.com")[0])

    def test_prompt_only_predicts_exact_text_once(self) -> None:
        text = "Summarize quarterly risk for my team"
        body = json.dumps({
            "prompt": text,
            "parent_message_uuid": "00000000-0000-4000-8000-000000000000",
        })
        f = _flow("claude.ai", "/api/organizations/o1/chat_conversations/c1/completion", body)
        _run(f)
        prompts = [p for _, p in EVALS]
        self.assertEqual(prompts, [text], EVALS)
        self.assertFalse(UPLOAD_LOGS)

    def test_mid_typing_and_wire_tokens_are_not_predicted(self) -> None:
        # Partial typing / prepare must not land in Prompt Logs as mid-words.
        draft = json.dumps({"prompt": "Sum", "status": "in_progress"})
        f = _flow("claude.ai", "/api/organizations/o1/chat_conversations/c1/prepare", draft)
        _run(f)
        self.assertEqual(EVALS, [])

        for noise in ("b", "r", "en-US", "PerformAction", "org_01abcdefghijkl"):
            self.assertTrue(NS["_is_claude_wire_noise"](noise), noise)
            self.assertFalse(NS["looks_like_user_prompt"](noise), noise)

    def test_cdn_file_then_send_logs_filename_and_caption_and_blocks_ssn(self) -> None:
        # Visit chat first so sticky bind works even for 127.0.0.1 laptop Guard.
        warm = _flow(
            "claude.ai",
            "/api/organizations/o1/chat_conversations/c1/completion",
            json.dumps({"prompt": "warmup sticky bind"}),
        )
        _run(warm)
        EVALS.clear()
        UPLOAD_LOGS.clear()

        fname = "payroll_notes.txt"
        data = b"employee tax SSN 123-45-6789 keep private\n" * 3
        ubody, mct = _multipart(fname, "text/plain", data)
        up = _flow(
            "files.claudeusercontent.com",
            "/api/organizations/o1/files",
            ubody,
            content_type=mct,
            headers={"Referer": "https://claude.ai/chat/abc"},
        )
        _run(up)
        self.assertIsNone(up.response, "upload must cache only")
        self.assertEqual(EVALS, [], "zero predict on Claude CDN upload")
        self.assertEqual(UPLOAD_LOGS, [], "zero Prompt Log on Claude CDN upload")

        caption = "review this payroll sheet"
        send = _flow(
            "claude.ai",
            "/api/organizations/o1/chat_conversations/c1/completion",
            json.dumps({
                "prompt": caption,
                "attachments": [{"file_name": fname}],
                "files": [fname],
            }),
        )
        t0 = time.time()
        _run(send)
        elapsed = time.time() - t0
        self.assertIsNotNone(send.response, "SSN file must block Send")
        # Wait briefly for fire-and-forget file log thread.
        for _ in range(40):
            if UPLOAD_LOGS:
                break
            time.sleep(0.05)
        self.assertTrue(UPLOAD_LOGS, "file Prompt Log missing after Send")
        self.assertLess(elapsed, 1.2, f"Claude file Send too slow: {elapsed:.2f}s")

        joined = " | ".join(str(u.get("prompt") or "") for u in UPLOAD_LOGS)
        self.assertIn(fname, joined, UPLOAD_LOGS)
        self.assertIn(f" -- {caption}", joined, UPLOAD_LOGS)
        self.assertIn("Blocked", joined, UPLOAD_LOGS)
        # Prompt-only row must not steal mid-words; file path owns the Send.
        mid = [p for _, p in EVALS if p != caption and caption not in p and fname not in p]
        self.assertEqual(mid, [], f"unexpected mid-word predicts: {EVALS}")

    def test_prompt_only_clean_file_format_when_allowed(self) -> None:
        warm = _flow(
            "claude.ai",
            "/api/organizations/o1/chat_conversations/c1/completion",
            json.dumps({"prompt": "hi"}),
        )
        _run(warm)
        EVALS.clear()
        UPLOAD_LOGS.clear()

        fname = "agenda.txt"
        ubody, mct = _multipart(fname, "text/plain", b"team lunch Friday at noon\n" * 4)
        up = _flow(
            "files.claudeusercontent.com",
            "/upload",
            ubody,
            content_type=mct,
            headers={"Origin": "https://claude.ai"},
        )
        _run(up)
        caption = "summarize the agenda"
        send = _flow(
            "claude.ai",
            "/api/organizations/o1/chat_conversations/c1/completion",
            json.dumps({
                "prompt": caption,
                "attachments": [{"file_name": fname}],
                "files": [fname],
            }),
        )
        _run(send)
        for _ in range(40):
            if UPLOAD_LOGS:
                break
            time.sleep(0.05)
        self.assertTrue(UPLOAD_LOGS)
        prompt = UPLOAD_LOGS[0].get("prompt") or ""
        self.assertIn(fname, prompt)
        self.assertIn(f" -- {caption}", prompt)
        self.assertIn("Allowed", prompt)
        self.assertTrue(str(prompt).startswith("[FILE UPLOAD]") or "[FILE UPLOAD]" in prompt)


if __name__ == "__main__":
    unittest.main()
