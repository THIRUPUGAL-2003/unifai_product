#!/usr/bin/env python3
"""Dashboard rule -> every Target site: Block stops the Send, Warn/Redact rewrite it.

Rules are loaded through get_guard_rules() from a backend-shaped payload, so a pattern the
backend accepts (e.g. '(?i)' mid-expression) must be enforced by the Guard too.
"""

from __future__ import annotations

import json
import unittest
import urllib.parse

try:
    from .test_target_predict_e2e import (
        ADDON,
        EVALS,
        NS,
        _flow,
        _reset,
        _run,
        _send_bodies,
        _ws_flow,
    )
except ImportError:
    try:
        from test_target_predict_e2e import (
            ADDON,
            EVALS,
            NS,
            _flow,
            _reset,
            _run,
            _send_bodies,
            _ws_flow,
        )
    except ImportError:
        from tests.test_target_predict_e2e import (
            ADDON,
            EVALS,
            NS,
            _flow,
            _reset,
            _run,
            _send_bodies,
            _ws_flow,
        )

NUMBER = "7468486848"
RULE_PATTERN = r"\b\d{10}\b|(?i)[A-Z]{5}[0-9]{4}[A-Z]"
WARNING_TEXT = "Phone or PAN numbers are not allowed"


def _load_rule_from_backend(action: str) -> None:
    payload = {"rules": [{
        "name": "indian pan number", "rule_type": "regex", "active": True,
        "pattern": RULE_PATTERN, "action": action, "severity": "HIGH",
        "warning_message": WARNING_TEXT,
    }]}
    NS["GATEWAY_BACKEND_URL"] = "http://127.0.0.1:6000"
    NS["_fetch_json"] = lambda url, *a, **k: payload
    NS["get_guard_rules"](force_network=True)


def _copilot_ws(text: str):
    frame = json.dumps({"event": "send", "conversationId": "cv1", "mode": "chat",
                        "content": [{"type": "text", "text": text}]})
    return _ws_flow("copilot.microsoft.com", "/c/api/chat?api-version=2", frame)


class RuleActionsAllSitesTests(unittest.TestCase):
    def _prepare(self, action: str) -> None:
        _reset()
        _load_rule_from_backend(action)
        self.assertEqual(len(NS["_cached_rules"]), 1, "rule skipped: backend-valid regex failed to compile")

    def test_block_stops_send_on_every_site(self) -> None:
        misses = []
        for label, host, path, body, ctype in _send_bodies(f"call me on {NUMBER}"):
            self._prepare("BLOCK")
            f = _flow(host, path, body, content_type=ctype)
            _run(f)
            if f.response is None:
                misses.append((label, [p for _, p in EVALS]))
        self._prepare("BLOCK")
        ws = _copilot_ws(f"call me on {NUMBER}")
        NS["BrowserAIInterceptor"].websocket_message(ADDON, ws)
        if not ws.websocket.messages[-1].dropped:
            misses.append(("copilot-ws", [p for _, p in EVALS]))
        self.assertEqual(misses, [], f"not blocked: {misses}")

    def test_pan_number_blocks_via_mid_pattern_flag(self) -> None:
        self._prepare("BLOCK")
        label, host, path, body, ctype = _send_bodies("my pan is abcde1234f")[0]
        f = _flow(host, path, body, content_type=ctype)
        _run(f)
        self.assertIsNotNone(f.response, f"{label}: lowercase PAN not blocked; evals={EVALS}")

    def test_warn_and_redact_rewrite_send_on_every_site(self) -> None:
        """The AI must get the warning AND a body its parser still accepts."""
        for action in ("WARN", "REDACT"):
            misses = []
            for label, host, path, body, ctype in _send_bodies(f"call me on {NUMBER}"):
                self._prepare(action)
                f = _flow(host, path, body, content_type=ctype)
                _run(f)
                sent = (f.request.content or b"").decode("utf-8", errors="ignore")
                if f.response is not None:
                    misses.append((label, "blocked"))
                    continue
                try:
                    if "json" in ctype:
                        text = json.dumps(json.loads(sent), ensure_ascii=False)
                    else:
                        freq = dict(urllib.parse.parse_qsl(sent))["f.req"]
                        text = json.dumps(json.loads(json.loads(freq)[1]), ensure_ascii=False)
                except (ValueError, KeyError, IndexError, TypeError) as e:
                    misses.append((label, f"unparseable body: {e}"))
                    continue
                if WARNING_TEXT not in text:
                    misses.append((label, "warning missing"))
            self._prepare(action)
            ws = _copilot_ws(f"call me on {NUMBER}")
            NS["BrowserAIInterceptor"].websocket_message(ADDON, ws)
            msg = ws.websocket.messages[-1]
            try:
                ws_ok = not msg.dropped and WARNING_TEXT in json.dumps(json.loads(msg.text), ensure_ascii=False)
            except ValueError:
                ws_ok = False
            if not ws_ok:
                misses.append(("copilot-ws", "warning missing or invalid frame"))
            self.assertEqual(misses, [], f"{action} not applied on: {misses}")

    def test_clean_prompt_is_not_touched(self) -> None:
        misses = []
        for label, host, path, body, ctype in _send_bodies("write a short poem about rain"):
            self._prepare("BLOCK")
            f = _flow(host, path, body, content_type=ctype)
            original = f.request.content
            _run(f)
            if f.response is not None or f.request.content != original:
                misses.append(label)
        self.assertEqual(misses, [], f"clean prompt changed/blocked on: {misses}")


if __name__ == "__main__":
    unittest.main()
