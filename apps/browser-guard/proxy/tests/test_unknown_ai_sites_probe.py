#!/usr/bin/env python3
"""AI sites with no site-specific parser: admin adds the domain, Guard must still predict.

Bodies mirror the Send requests of each product (Poe, Meta AI, HuggingChat, Qwen,
Kimi, you.com, Character.ai, Pi, Duck.ai, Blackbox, ChatGLM, Doubao, Notion AI) plus
plain-text / query-string custom APIs. Telemetry on the same hosts must stay silent.
"""

from __future__ import annotations

import json
import re
import struct
import time
import unittest
import urllib.parse
from pathlib import Path

from mitmproxy.test import tflow, tutils

PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "unifai_proxy_parts"
PARTS = [
    "config_caches_rules.py",
    "helpers_prompts.py",
    "uploads_detect.py",
    "file_policy.py",
    "extract_office_backend.py",
    "responses_inject.py",
    "responses_addon.py",
]

TARGETS = [
    "poe.com", "meta.ai", "huggingface.co", "qwen.ai", "kimi.com", "you.com",
    "character.ai", "pi.ai", "duckduckgo.com", "blackbox.ai", "chatglm.cn",
    "doubao.com", "notion.so", "internal-llm.corp.example",
]

SSN = "123-45-6789"


def _load():
    ns: dict = {"__name__": "browser_ai_proxy_probe"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    ns["_apply_targets_from_data"]({
        "targets": [{"domain": d, "platform_name": d, "monitored": True} for d in TARGETS]
    })
    ns["_controls_fetched_at"] = time.time()
    return ns


NS = _load()
EVALS: list[tuple[str, str]] = []
NS["evaluate_prompt"] = lambda platform, domain, prompt, *a, **k: (
    EVALS.append((domain, prompt)) or NS["decide_prompt_locally"](prompt)
)
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = lambda *a, **k: True
ADDON = object.__new__(NS["BrowserAIInterceptor"])


def _reset() -> None:
    EVALS.clear()
    for name in ("_recent_prompts", "_recent_decisions", "_composer_draft", "_CLIENT_TARGET_STICKY", "_FILE_SEND_BLOCKS"):
        store = NS.get(name)
        if isinstance(store, dict):
            store.clear()
    pat = r"\b\d{3}-\d{2}-\d{4}\b"
    NS["_cached_rules"] = [{
        "name": "SSN", "pattern": pat, "regex": re.compile(pat, re.IGNORECASE),
        "action": "BLOCK", "severity": "HIGH", "warning_message": "SSN blocked",
    }]
    NS["_rules_fetched_at"] = time.time()
    NS["_rules_fetch_ok"] = True


def _flow(host, path, body, *, method="POST", ctype="application/json"):
    if isinstance(body, str):
        body = body.encode("utf-8")
    req = tutils.treq(
        host=host, port=443, scheme=b"https", method=method.encode(), path=path.encode(),
        headers=[(b"content-type", ctype.encode()), (b"host", host.encode())], content=body,
    )
    req.authority = host
    return tflow.tflow(req=req)


def _ws_flow(host, path, text):
    from mitmproxy import websocket
    from wsproto.frame_protocol import Opcode

    f = tflow.twebsocketflow(messages=False)
    f.request.host = host
    f.request.authority = host
    f.request.headers["host"] = host
    f.request.path = path
    f.websocket.messages.append(websocket.WebSocketMessage(Opcode.TEXT, True, text.encode("utf-8")))
    return f


def _multipart(fields: dict) -> tuple[bytes, str]:
    b = "----WebKitFormBoundaryProbe"
    out = b""
    for k, v in fields.items():
        out += f"--{b}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode()
    return out + f"--{b}--\r\n".encode(), f"multipart/form-data; boundary={b}"


def _connect_json(obj) -> bytes:
    raw = json.dumps(obj).encode()
    return b"\x00" + struct.pack(">I", len(raw)) + raw


def _sends(text: str):
    """(label, host, path, body, ctype, method)"""
    hf_body, hf_ct = _multipart({"data": json.dumps({"inputs": text, "id": "m1", "is_retry": False})})
    return [
        ("poe", "poe.com", "/api/gql_POST", json.dumps({
            "queryName": "sendMessageMutation",
            "variables": {"chatId": None, "bot": "capybara", "query": text, "source": {"sourceType": "chat_input"},
                          "withChatBreak": False, "attachments": []},
            "extensions": {"hash": "abc"},
        }), "application/json", "POST"),
        ("meta-ai", "www.meta.ai", "/api/graphql/", urllib.parse.urlencode({
            "fb_api_req_friendly_name": "useAbraSendMessageMutation",
            "variables": json.dumps({"message": {"sensitive_string_value": text}, "externalConversationId": "c1",
                                     "offlineThreadingId": "712", "entrypoint": "ABRA__CHAT__TEXT"}),
            "doc_id": "7783822248314888",
        }), "application/x-www-form-urlencoded", "POST"),
        ("huggingchat", "huggingface.co", "/chat/conversation/66a1b2", hf_body, hf_ct, "POST"),
        ("qwen", "chat.qwen.ai", "/api/v2/chat/completions?chat_id=q1", json.dumps({
            "stream": True, "incremental_output": True, "chat_id": "q1", "model": "qwen3-max",
            "messages": [{"role": "user", "content": text, "files": [], "chat_type": "t2t"}],
        }), "application/json", "POST"),
        ("kimi-rest", "www.kimi.com", "/api/chat/k1/completion/stream", json.dumps({
            "kimiplus_id": "kimi", "model": "k2", "use_search": False,
            "messages": [{"role": "user", "content": text}],
        }), "application/json", "POST"),
        ("kimi-connect", "www.kimi.com", "/apiv2/kimi.chat.v1.ChatService/Chat", _connect_json({
            "scenario": "SCENARIO_K2", "message": {"role": "user", "blocks": [{"text": {"content": text}}]},
        }), "application/connect+json", "POST"),
        ("you-get", "you.com", "/api/streamingSearch?" + urllib.parse.urlencode(
            {"q": text, "page": 1, "count": 10, "safeSearch": "Moderate", "chatId": "y1"}), b"", "", "GET"),
        ("pi", "pi.ai", "/api/chat", json.dumps({"text": text, "conversation": "p1"}), "application/json", "POST"),
        ("duck-ai", "duckduckgo.com", "/duckchat/v1/chat", json.dumps({
            "model": "gpt-4o-mini", "messages": [{"role": "user", "content": text}], "canUseTools": True,
        }), "application/json", "POST"),
        ("blackbox", "www.blackbox.ai", "/api/chat", json.dumps({
            "messages": [{"id": "b1", "content": text, "role": "user"}], "id": "b1",
            "previewToken": None, "userId": None, "codeModelMode": True,
        }), "application/json", "POST"),
        ("chatglm", "chatglm.cn", "/chatglm/backend-api/assistant/stream", json.dumps({
            "assistant_id": "65940acff94777010aa6b796", "conversation_id": "",
            "meta_data": {"is_test": False, "input_question_type": "xxxx"},
            "messages": [{"role": "user", "content": [{"type": "text", "text": text}]}],
        }), "application/json", "POST"),
        ("doubao", "www.doubao.com", "/samantha/chat/completion?aid=497858", json.dumps({
            "messages": [{"content": json.dumps({"text": text}), "content_type": 2001, "attachments": []}],
            "completion_option": {"is_regen": False, "with_suggest": True},
            "conversation_id": "0", "local_message_id": "d1",
        }), "application/json", "POST"),
        ("notion-ai", "www.notion.so", "/api/v3/runInferenceTranscript", json.dumps({
            "traceId": "t1", "spaceId": "s1",
            "transcript": [{"type": "config", "value": {"type": "workflow"}},
                           {"type": "user", "value": [[text]], "userId": "u1"}],
        }), "application/json", "POST"),
        ("custom-plain", "api.internal-llm.corp.example", "/generate", text, "text/plain", "POST"),
        ("custom-get", "internal-llm.corp.example", "/ask?" + urllib.parse.urlencode({"prompt": text}),
         b"", "", "GET"),
    ]


class UnknownAISitesProbe(unittest.TestCase):
    def setUp(self) -> None:
        _reset()

    def test_send_is_predicted_on_every_site(self) -> None:
        misses = []
        for label, host, path, body, ct, method in _sends("plan the quarterly offsite for my team"):
            _reset()
            ADDON.request(_flow(host, path, body, ctype=ct, method=method))
            if not any("quarterly offsite" in p for _, p in EVALS):
                misses.append((label, [p for _, p in EVALS]))
        self.assertEqual(misses, [], f"not predicted: {misses}")

    def test_ssn_is_blocked_on_every_site(self) -> None:
        misses = []
        for label, host, path, body, ct, method in _sends(f"my ssn is {SSN} please file taxes"):
            _reset()
            f = _flow(host, path, body, ctype=ct, method=method)
            ADDON.request(f)
            if f.response is None:
                misses.append((label, [p for _, p in EVALS]))
        self.assertEqual(misses, [], f"not blocked: {misses}")

    def test_logged_prompt_is_the_typed_text_not_the_json_wrapper(self) -> None:
        text = "plan the quarterly offsite for my team"
        wrapped = []
        for label, host, path, body, ct, method in _sends(text):
            _reset()
            ADDON.request(_flow(host, path, body, ctype=ct, method=method))
            if not any(p.strip() == text for _, p in EVALS):
                wrapped.append((label, [p for _, p in EVALS]))
        self.assertEqual(wrapped, [])

    def test_typed_json_keeps_every_field_for_rules(self) -> None:
        unwrap = NS["_unwrap_json_prompt"]
        self.assertEqual(unwrap('{"text": "hello there"}'), "hello there")
        typed = '{"text": "hello there", "note": "123-45-6789"}'
        self.assertEqual(unwrap(typed), typed)
        f = _flow("chat.qwen.ai", "/api/v2/chat/completions", json.dumps({
            "messages": [{"role": "user", "content": typed}]}))
        ADDON.request(f)
        self.assertIsNotNone(f.response, EVALS)

    def test_warn_rewrites_body_in_its_own_format(self) -> None:
        warning = "SSN is not allowed"
        misses = []
        for label, host, path, body, ct, method in _sends(f"my ssn is {SSN} please file taxes"):
            _reset()
            NS["_cached_rules"][0].update({"action": "WARN", "warning_message": warning})
            f = _flow(host, path, body, ctype=ct, method=method)
            ADDON.request(f)
            sent = f.request.content or b""
            text = sent.decode("utf-8", errors="ignore") if method == "POST" else f.request.url
            if f.response is not None:
                misses.append((label, "blocked"))
                continue
            try:
                if "connect+json" in ct:
                    self.assertEqual(struct.unpack(">I", sent[1:5])[0], len(sent) - 5)
                    json.loads(sent[5:])
                elif "json" in ct:
                    json.loads(text)
                elif "urlencoded" in ct:
                    json.loads(dict(urllib.parse.parse_qsl(text))["variables"])
                elif "multipart" in ct:
                    part = text.split('name="data"\r\n\r\n', 1)[1].split("\r\n--", 1)[0]
                    json.loads(part)
            except (ValueError, KeyError, IndexError, AssertionError) as e:
                misses.append((label, f"broken body: {e}"))
                continue
            if warning not in text.replace("\\u0020", " ") and warning not in urllib.parse.unquote_plus(text):
                misses.append((label, "warning missing"))
        self.assertEqual(misses, [], misses)

    def test_character_ai_websocket(self) -> None:
        frame = json.dumps({
            "command": "create_and_generate_turn", "request_id": "r1",
            "payload": {"num_candidates": 1, "character_id": "c1",
                        "turn": {"author": {"author_id": "u1", "is_human": True},
                                 "candidates": [{"raw_content": f"my ssn is {SSN}"}]}},
        })
        f = _ws_flow("neo.character.ai", "/ws/", frame)
        ADDON.websocket_message(f)
        self.assertTrue(f.websocket.messages[-1].dropped, EVALS)

    def test_telemetry_on_custom_hosts_is_silent(self) -> None:
        noise = [
            ("poe.com", "/api/gql_POST", json.dumps({"queryName": "ChatListPaginationQuery",
                                                    "variables": {"count": 25, "cursor": "0"}})),
            ("www.meta.ai", "/ajax/bz", json.dumps({"event": "page_view", "time": 1727600000})),
            ("chat.qwen.ai", "/api/v1/users/user/settings", json.dumps({"theme": "dark", "language": "en"})),
            ("www.kimi.com", "/api/event/track", json.dumps({"event": "click_send_button", "page": "chat"})),
            ("internal-llm.corp.example", "/metrics", json.dumps({"latency_ms": 312, "status": "ok"})),
        ]
        for host, path, body in noise:
            _reset()
            ADDON.request(_flow(host, path, body))
        self.assertEqual(EVALS, [])

    def test_only_listed_subdomain_is_covered(self) -> None:
        dt = NS["detect_target"]
        NS["_apply_targets_from_data"]({"targets": [
            {"domain": "chat.newai.example", "platform_name": "NewAI", "monitored": True}]})
        try:
            self.assertTrue(dt("chat.newai.example")[0])
            self.assertFalse(dt("api.newai.example")[0])
            NS["_apply_targets_from_data"]({"targets": [
                {"domain": "newai.example", "platform_name": "NewAI", "monitored": True}]})
            self.assertTrue(dt("api.newai.example")[0])
            self.assertTrue(dt("chat.newai.example")[0])
        finally:
            NS["_apply_targets_from_data"]({"targets": [
                {"domain": d, "platform_name": d, "monitored": True} for d in TARGETS]})


if __name__ == "__main__":
    unittest.main()
