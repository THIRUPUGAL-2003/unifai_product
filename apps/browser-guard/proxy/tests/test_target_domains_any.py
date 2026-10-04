#!/usr/bin/env python3
"""Whatever domain / subdomain the admin adds in Target Websites is monitored the same way.

Targets arrive in the raw form the server may store (wildcards, trailing dots, URLs,
upper case), exactly as /api/browser-ai/targets?for=agent returns them.
"""

from __future__ import annotations

import json
import re
import time
import unittest

try:
    from .test_target_predict_e2e import EVALS, NS, _flow, _reset, _run
except ImportError:
    try:
        from test_target_predict_e2e import EVALS, NS, _flow, _reset, _run
    except ImportError:
        from tests.test_target_predict_e2e import EVALS, NS, _flow, _reset, _run

NUMBER = "7468486848"


def _chat_body(text: str) -> str:
    return json.dumps({"messages": [{"role": "user", "content": text}], "stream": True})


def _set_targets(rows: list[dict]) -> None:
    NS["_apply_targets_from_data"]({"targets": rows})


def _set_block_rule() -> None:
    pat = r"\b\d{10}\b"
    NS["_cached_rules"] = [{
        "name": "Ten digit number", "pattern": pat, "regex": re.compile(pat, re.IGNORECASE),
        "action": "BLOCK", "severity": "HIGH", "warning_message": "Numbers not allowed",
    }]
    NS["_rules_fetched_at"] = time.time()
    NS["_rules_fetch_ok"] = True


ADMIN_TARGETS = [
    {"id": "t1", "domain": "https://www.ChatGPT.com/", "platform_name": "ChatGPT", "monitored": True},
    {"id": "t2", "domain": "*.deepseek.com", "platform_name": "DeepSeek", "monitored": True},
    {"id": "t3", "domain": "copilot.microsoft.com.", "platform_name": "Copilot", "monitored": True},
    {"id": "t4", "domain": "google.com", "platform_name": "Google", "monitored": True},
    {"id": "t5", "domain": "gemini.google.com", "platform_name": "Gemini", "monitored": True},
    {"id": "t6", "domain": "claude.ai", "platform_name": "Claude", "monitored": True},
    {"id": "t7", "domain": "files.claudeusercontent.com", "platform_name": "Claude", "monitored": True,
     "parent_id": "t6", "host_role": "file"},
    {"id": "t8", "domain": "assets.acme-ai.io", "platform_name": "Acme AI", "monitored": True},
    {"id": "t9", "domain": "xn--bcher-kva.example", "platform_name": "Bucher AI", "monitored": True},
    {"id": "t10", "domain": "grok.com", "platform_name": "Grok", "monitored": False},
    {"id": "t11", "domain": "blocked-ai.example", "platform_name": "Locked AI", "monitored": True,
     "block_site": True},
]


class AnyTargetDomainTests(unittest.TestCase):
    def setUp(self) -> None:
        _reset()
        _set_targets(ADMIN_TARGETS)
        _set_block_rule()

    def test_parent_subdomain_and_raw_admin_forms_match(self) -> None:
        dt = NS["detect_target"]
        for host, platform in [
            ("chatgpt.com", "ChatGPT"), ("ab.chatgpt.com", "ChatGPT"), ("WWW.CHATGPT.COM", "ChatGPT"),
            ("chat.deepseek.com", "DeepSeek"), ("api.chat.deepseek.com", "DeepSeek"),
            ("copilot.microsoft.com", "Copilot"), ("gemini.google.com", "Gemini"),
            ("xn--bcher-kva.example", "Bucher AI"), ("chatgpt.com:443", "ChatGPT"),
        ]:
            ok, _domain, plat = dt(host)
            self.assertTrue(ok, f"{host} not monitored")
            self.assertEqual(plat, platform, f"{host} matched the wrong platform")

    def test_lookalike_hosts_are_not_monitored(self) -> None:
        dt = NS["detect_target"]
        for host in ("evilchatgpt.com", "chatgpt.com.evil.io", "deepseek.co", "microsoft.com", "example.com"):
            self.assertFalse(dt(host)[0], f"{host} must not match any target")

    def test_send_on_any_subdomain_is_blocked(self) -> None:
        misses = []
        for host, path in [
            ("ab.chatgpt.com", "/backend-api/f/conversation"),
            ("chat.deepseek.com", "/api/v0/chat/completion"),
            ("api.chat.deepseek.com", "/v1/chat/completions"),
            ("assets.acme-ai.io", "/api/chat"),
            ("xn--bcher-kva.example", "/v1/chat/completions"),
        ]:
            _reset()
            _set_targets(ADMIN_TARGETS)
            _set_block_rule()
            f = _flow(host, path, _chat_body(f"call {NUMBER}"))
            _run(f)
            if f.response is None:
                misses.append((host, [p for _, p in EVALS]))
        self.assertEqual(misses, [], f"not blocked: {misses}")

    def test_paused_target_is_left_alone(self) -> None:
        f = _flow("grok.com", "/rest/app-chat/conversations/new", json.dumps({"message": f"call {NUMBER}"}))
        _run(f)
        self.assertIsNone(f.response)
        self.assertEqual(EVALS, [])

    def test_removed_target_stops_being_monitored(self) -> None:
        _set_targets([t for t in ADMIN_TARGETS if t["platform_name"] != "DeepSeek"])
        f = _flow("chat.deepseek.com", "/api/v0/chat/completion", json.dumps({"prompt": f"call {NUMBER}"}))
        _run(f)
        self.assertIsNone(f.response)
        self.assertEqual(EVALS, [])

    def test_block_entire_website_locks_every_subdomain(self) -> None:
        for host in ("blocked-ai.example", "app.blocked-ai.example", "www.blocked-ai.example"):
            f = _flow(host, "/", b"", method="GET", content_type="text/html")
            _run(f)
            self.assertIsNotNone(f.response, f"{host} not locked")
            self.assertEqual(f.response.status_code, 403)

    def test_file_role_host_ignores_plain_chat(self) -> None:
        f = _flow("files.claudeusercontent.com", "/api/chat", _chat_body(f"call {NUMBER}"))
        _run(f)
        self.assertIsNone(f.response)
        self.assertEqual(EVALS, [])


if __name__ == "__main__":
    unittest.main()
