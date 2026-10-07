#!/usr/bin/env python3
"""End-to-end: any admin-added Target Website gets prompt / file / voice predicted.

Drives BrowserAIInterceptor.request() with realistic Send bodies for the big AI
sites plus a custom domain nobody hard-coded, and asserts the Guard evaluated
the user's text (and blocked it when a rule matched).
"""

from __future__ import annotations

import json
import re
import time
import unittest
import urllib.parse
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

TARGETS = [
    ("chatgpt.com", "ChatGPT"),
    ("gemini.google.com", "Gemini"),
    ("claude.ai", "Claude"),
    ("perplexity.ai", "Perplexity"),
    ("chat.deepseek.com", "DeepSeek"),
    ("grok.com", "Grok"),
    ("copilot.microsoft.com", "Copilot"),
    ("chat.mistral.ai", "Mistral"),
    ("ai.acme-internal.io", "Acme Custom AI"),
]

SSN = "123-45-6789"


def _load():
    ns: dict = {"__name__": "browser_ai_proxy_e2e"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    # Module load already started the background refresher. A later test that
    # stubs _fetch_json with a rules payload would otherwise wipe this cache.
    ns["_refresh_targets_from_backend"] = lambda: None
    ns["_apply_targets_from_data"]({
        "targets": [
            {"domain": d, "platform_name": p, "monitored": True} for d, p in TARGETS
        ]
    })
    ns["_controls_fetched_at"] = time.time()
    return ns


NS = _load()
EVALS: list[tuple[str, str]] = []
UPLOAD_LOGS: list[dict] = []


def _fake_evaluate(platform, domain, prompt, client_ip, url, method):
    EVALS.append((domain, prompt))
    return NS["decide_prompt_locally"](prompt)


def _fake_upload_log(*args, **kwargs):
    UPLOAD_LOGS.append(kwargs or {"args": args})
    return True


NS["evaluate_prompt"] = _fake_evaluate
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = _fake_upload_log


def _arm_targets() -> None:
    """Put the suite target list back. Other tests share this namespace and replace it."""
    NS["GATEWAY_BACKEND_URL"] = ""
    NS["_apply_targets_from_data"]({
        "targets": [
            {"domain": d, "platform_name": p, "monitored": True} for d, p in TARGETS
        ]
    })


def _install_rules() -> None:
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


def _reset() -> None:
    EVALS.clear()
    UPLOAD_LOGS.clear()
    for lock_name, store_names in (
        ("_FILE_ID_NAME_REGISTRY_LOCK", ["_FILE_ID_NAME_REGISTRY"]),
        ("_DOMAIN_PENDING_NAMES_LOCK", ["_DOMAIN_PENDING_NAMES"]),
        ("_CONTENT_HASH_NAME_LOCK", ["_CONTENT_HASH_NAME_REGISTRY"]),
        ("_UPLOAD_FILE_CACHE_LOCK", ["_UPLOAD_FILE_CACHE", "_UPLOAD_FILE_QUEUES"]),
    ):
        with NS[lock_name]:
            for s in store_names:
                NS[s].clear()
    for name in ("_recent_prompts", "_recent_decisions", "_composer_draft", "_CLIENT_TARGET_STICKY", "_FILE_SEND_BLOCKS"):
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


def _blocked(flow) -> bool:
    return flow.response is not None


ADDON = object.__new__(NS["BrowserAIInterceptor"])


def _send_bodies(text: str) -> list[tuple[str, str, str, bytes | str, str]]:
    """(label, host, path, body, content_type) — real Send shapes per site."""
    gemini_inner = json.dumps([[text, 0, None, None, None, None, 0], ["en"], ["", "", ""]])
    gemini_body = urllib.parse.urlencode({"f.req": json.dumps([None, gemini_inner]), "at": "AJvLN6M:1727"})
    return [
        ("chatgpt", "chatgpt.com", "/backend-api/f/conversation", json.dumps({
            "action": "next",
            "messages": [{
                "id": "aaa1", "author": {"role": "user"},
                "content": {"content_type": "text", "parts": [text]}, "metadata": {},
            }],
            "parent_message_id": "client-created-root", "model": "auto",
            "timezone_offset_min": -330, "conversation_mode": {"kind": "primary_assistant"},
        }), "application/json"),
        ("gemini", "gemini.google.com",
         "/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=boq_x&_reqid=123&rt=c",
         gemini_body, "application/x-www-form-urlencoded;charset=UTF-8"),
        ("claude", "claude.ai",
         "/api/organizations/0f1e/chat_conversations/9a8b/completion", json.dumps({
            "prompt": text, "parent_message_uuid": "00000000-0000-4000-8000-000000000000",
            "timezone": "Asia/Kolkata", "attachments": [], "files": [], "rendering_mode": "messages",
         }), "application/json"),
        ("perplexity", "www.perplexity.ai", "/rest/sse/perplexity_ask", json.dumps({
            "params": {"version": "2.18", "source": "default", "mode": "concise", "language": "en-US"},
            "query_str": text,
        }), "application/json"),
        ("deepseek", "chat.deepseek.com", "/api/v0/chat/completion", json.dumps({
            "chat_session_id": "c1", "parent_message_id": None, "prompt": text,
            "ref_file_ids": [], "thinking_enabled": False, "search_enabled": False,
        }), "application/json"),
        ("grok", "grok.com", "/rest/app-chat/conversations/new", json.dumps({
            "temporary": False, "modelName": "grok-3", "message": text,
            "fileAttachments": [], "imageAttachments": [],
        }), "application/json"),
        ("mistral", "chat.mistral.ai", "/api/chat", json.dumps({
            "chatId": "m1", "mode": "append", "model": "mistral-large",
            "messageInput": [{"type": "text", "text": text}],
        }), "application/json"),
        ("custom-openai-shape", "ai.acme-internal.io", "/v1/chat/completions", json.dumps({
            "model": "acme-1", "stream": True,
            "messages": [{"role": "user", "content": text}],
        }), "application/json"),
        ("custom-question", "ai.acme-internal.io", "/api/ask", json.dumps({
            "question": text, "session": "s1",
        }), "application/json"),
        ("custom-input", "ai.acme-internal.io", "/graphql", json.dumps({
            "operationName": "SendMessage",
            "variables": {"input": {"text": text, "threadId": "t1"}},
            "query": "mutation SendMessage($input: SendInput!) { send(input: $input) { id } }",
        }), "application/json"),
    ]


class TargetPromptPredictTests(unittest.TestCase):
    def setUp(self) -> None:
        _reset()
        _arm_targets()
        _install_rules()

    def test_detect_target_matches_any_admin_domain_and_subdomains(self) -> None:
        dt = NS["detect_target"]
        self.assertTrue(dt("chatgpt.com")[0])
        self.assertTrue(dt("www.perplexity.ai")[0])
        self.assertTrue(dt("api.ai.acme-internal.io")[0])
        self.assertFalse(dt("acme-internal.io")[0])
        self.assertFalse(dt("example.com")[0])

    def test_every_site_send_is_predicted(self) -> None:
        misses = []
        for label, host, path, body, ctype in _send_bodies("plan the quarterly offsite for my team"):
            _reset()
            _install_rules()
            _run(_flow(host, path, body, content_type=ctype))
            prompts = [p for _, p in EVALS]
            if not any("quarterly offsite" in p for p in prompts):
                misses.append((label, prompts))
        self.assertEqual(misses, [], f"not predicted: {misses}")

    def test_every_site_send_with_ssn_is_blocked(self) -> None:
        misses = []
        for label, host, path, body, ctype in _send_bodies(f"my ssn is {SSN} please file taxes"):
            _reset()
            _install_rules()
            f = _flow(host, path, body, content_type=ctype)
            _run(f)
            if not _blocked(f):
                misses.append((label, [p for _, p in EVALS]))
        self.assertEqual(misses, [], f"not blocked: {misses}")

    def test_short_prompts_are_predicted(self) -> None:
        misses = []
        for text in ("hi", "42", "?"):
            for label, host, path, body, ctype in _send_bodies(text):
                _reset()
                _install_rules()
                _run(_flow(host, path, body, content_type=ctype))
                if not any(p.strip() == text for _, p in EVALS):
                    misses.append((text, label, [p for _, p in EVALS]))
        self.assertEqual(misses, [], f"short prompt misses: {misses}")

    def test_chatgpt_2026_body_with_dictation_false_is_predicted(self) -> None:
        for text in ("hii", "hiiii"):
            _reset()
            _install_rules()
            body = json.dumps({
                "action": "next",
                "messages": [{
                    "id": "5c6d", "author": {"role": "user"},
                    "content": {"content_type": "text", "parts": [text]},
                    "metadata": {"selected_github_repos": [], "serialization_metadata": {"custom_symbol_offsets": []},
                                 "dictation": False},
                }],
                "parent_message_id": "client-created-root", "model": "auto",
                "timezone_offset_min": -330, "conversation_mode": {"kind": "primary_assistant"},
                "enable_message_followups": True, "supports_buffering": True,
                "supported_encodings": ["v1"], "client_contextual_info": {"is_dark_mode": True},
                "paragen_cot_summary_display_override": "allow", "force_parallel_switch": "auto",
            })
            _run(_flow("chatgpt.com", "/backend-api/f/conversation", body))
            self.assertTrue(any(p.strip() == text for _, p in EVALS), f"{text!r} not predicted: {EVALS}")

    def test_typed_word_dictation_is_not_an_attachment(self) -> None:
        carries = NS["chat_carries_attachment"]
        self.assertFalse(carries('{"parts":["hi"],"metadata":{"dictation":false}}'))
        self.assertFalse(carries('{"parts":["hi"],"voice_mode":null,"input_audio":""}'))
        self.assertTrue(carries('{"parts":["hi"],"metadata":{"dictation":true}}'))
        self.assertTrue(carries('{"input_audio":{"data":"UklGR","format":"wav"}}'))


def _multipart(field: str, filename: str, ctype: str, data: bytes) -> tuple[bytes, str]:
    boundary = "----WebKitFormBoundaryE2E"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


class TargetFileVoicePredictTests(unittest.TestCase):
    def setUp(self) -> None:
        _reset()
        _arm_targets()
        _install_rules()

    def _upload_then_send(self, host: str, upload_path: str, filename: str, ctype: str, data: bytes,
                          send_path: str, send_body: str):
        body, mct = _multipart("file", filename, ctype, data)
        up = _flow(host, upload_path, body, content_type=mct)
        _run(up)
        send = _flow(host, send_path, send_body)
        _run(send)
        return up, send

    def test_custom_domain_text_file_with_ssn_blocks_on_send(self) -> None:
        up, send = self._upload_then_send(
            "ai.acme-internal.io", "/api/files/7c1d", "payroll.txt", "text/plain",
            f"employee ssn {SSN}\n".encode() * 4,
            "/v1/chat/completions",
            json.dumps({"messages": [{"role": "user", "content": "summarize the attached file"}],
                        "attachments": [{"name": "payroll.txt", "type": "file"}]}),
        )
        self.assertIsNone(up.response, "upload itself must not be blocked (cache only)")
        self.assertTrue(_blocked(send), f"file SSN not blocked on send; evals={EVALS} logs={len(UPLOAD_LOGS)}")

    def test_claude_text_file_with_ssn_blocks_on_send(self) -> None:
        up, send = self._upload_then_send(
            "claude.ai", "/api/0f1e/upload", "hr.txt", "text/plain",
            f"ssn: {SSN}\n".encode() * 4,
            "/api/organizations/0f1e/chat_conversations/9a8b/completion",
            json.dumps({"prompt": "what is in this file", "attachments": [],
                        "files": ["file-uuid-1"], "rendering_mode": "messages"}),
        )
        self.assertTrue(_blocked(send), f"claude file SSN not blocked; evals={EVALS}")

    def test_mistral_text_file_with_ssn_blocks_on_send(self) -> None:
        up, send = self._upload_then_send(
            "chat.mistral.ai", "/api/files", "payroll.txt", "text/plain",
            f"employee ssn {SSN}\n".encode() * 4,
            "/api/chat",
            json.dumps({
                "chatId": "m1", "mode": "append", "model": "mistral-large",
                "messageInput": [
                    {"type": "text", "text": "summarize this file"},
                    {"type": "document_url", "document_url": "https://chat.mistral.ai/files/doc1.pdf"}
                ]
            }),
        )
        self.assertIsNone(up.response, "upload itself must not be blocked (cache only)")
        self.assertTrue(_blocked(send), f"mistral file SSN not blocked on send; evals={EVALS} logs={len(UPLOAD_LOGS)}")

    def test_voice_transcript_with_ssn_blocks_on_any_target(self) -> None:
        for host, path in (("ai.acme-internal.io", "/api/voice/send"), ("chatgpt.com", "/backend-api/f/conversation")):
            _reset()
            _install_rules()
            body = json.dumps({
                "transcript": f"my social security number is {SSN}",
                "audio": {"format": "webm", "duration_ms": 2100},
                "attachments": [{"type": "audio", "name": "voice-message.webm"}],
            })
            f = _flow(host, path, body)
            _run(f)
            self.assertTrue(_blocked(f), f"{host}: voice transcript not blocked; evals={EVALS}")

    def test_voice_recording_upload_is_logged_on_send(self) -> None:
        webm = b"\x1a\x45\xdf\xa3" + b"\x42\x82\x84webm" + b"\x00" * 400
        body, mct = _multipart("file", "voice-message.webm", "audio/webm", webm)
        up = _flow("ai.acme-internal.io", "/api/uploads/9f", body, content_type=mct)
        _run(up)
        self.assertIsNone(up.response)
        send = _flow("ai.acme-internal.io", "/v1/chat/completions", json.dumps({
            "messages": [{"role": "user", "content": "reply to this voice note"}],
            "attachments": [{"name": "voice-message.webm", "type": "audio"}],
        }))
        _run(send)
        self.assertTrue(UPLOAD_LOGS or EVALS, "voice upload + send was silent (no log, no predict)")


def _ws_flow(host: str, path: str, text: str):
    from mitmproxy import websocket
    from wsproto.frame_protocol import Opcode

    f = tflow.twebsocketflow(messages=False)
    f.request.host = host
    f.request.authority = host
    f.request.headers["host"] = host
    f.request.path = path
    f.websocket.messages.append(websocket.WebSocketMessage(Opcode.TEXT, True, text.encode("utf-8")))
    return f


class TargetWebSocketPredictTests(unittest.TestCase):
    def setUp(self) -> None:
        _reset()
        _arm_targets()
        _install_rules()

    def _copilot_frame(self, text: str) -> str:
        return json.dumps({
            "event": "send", "conversationId": "cv1", "mode": "chat",
            "content": [{"type": "text", "text": text}],
        })

    def test_copilot_ws_prompt_is_predicted(self) -> None:
        f = _ws_flow("copilot.microsoft.com", "/c/api/chat?api-version=2", self._copilot_frame("draft a leave letter"))
        NS["BrowserAIInterceptor"].websocket_message(ADDON, f)
        self.assertTrue(any("leave letter" in p for _, p in EVALS), EVALS)

    def test_copilot_ws_ssn_is_dropped(self) -> None:
        f = _ws_flow("copilot.microsoft.com", "/c/api/chat?api-version=2", self._copilot_frame(f"ssn {SSN} fill form"))
        NS["BrowserAIInterceptor"].websocket_message(ADDON, f)
        self.assertTrue(f.websocket.messages[-1].dropped, EVALS)

    def _copilot_upload_then_send(self, filename: str, data: bytes, caption: str):
        body, mct = _multipart("file", filename, "text/plain", data)
        up = _flow("copilot.microsoft.com", "/c/api/attachments", body, content_type=mct)
        _run(up)
        frame = json.dumps({
            "event": "send", "conversationId": "cv1", "mode": "chat", "context": {},
            "content": [
                {"type": "file", "fileName": filename,
                 "url": "https://copilot.microsoft.com/c/api/attachments/5f0c2a"},
                {"type": "text", "text": caption},
            ],
        })
        ws = _ws_flow("copilot.microsoft.com", "/c/api/chat?api-version=2", frame)
        NS["BrowserAIInterceptor"].websocket_message(ADDON, ws)
        return up, ws

    def test_copilot_file_with_ssn_is_dropped_on_send(self) -> None:
        up, ws = self._copilot_upload_then_send("payroll.txt", f"employee ssn {SSN}\n".encode() * 4, "summarize this")
        self.assertIsNone(up.response, "upload itself must only be cached")
        self.assertTrue(ws.websocket.messages[-1].dropped, f"copilot file SSN not blocked; evals={EVALS}")

    def test_copilot_clean_file_send_goes_through_and_is_logged(self) -> None:
        up, ws = self._copilot_upload_then_send("notes.txt", b"team lunch on friday at noon\n" * 4, "summarize this")
        self.assertIsNone(up.response)
        self.assertFalse(ws.websocket.messages[-1].dropped, EVALS)
        self.assertTrue(UPLOAD_LOGS or EVALS, "copilot file send was silent")

    def test_copilot_block_upload_control_stops_attach(self) -> None:
        NS["_control_cache"] = {"block_upload": True}
        orig = NS["controls_active"]
        NS["controls_active"] = lambda name: name == "block_upload"
        try:
            body, mct = _multipart("file", "notes.txt", "text/plain", b"hello world\n" * 8)
            up = _flow("copilot.microsoft.com", "/c/api/attachments", body, content_type=mct)
            _run(up)
            self.assertIsNotNone(up.response, "Block Upload did not stop Copilot attach")
        finally:
            NS["controls_active"] = orig

    def test_copilot_ws_warn_keeps_frame_valid(self) -> None:
        NS["_cached_rules"][0]["action"] = "WARN"
        f = _ws_flow("copilot.microsoft.com", "/c/api/chat?api-version=2", self._copilot_frame(f"ssn {SSN} fill form"))
        NS["BrowserAIInterceptor"].websocket_message(ADDON, f)
        msg = f.websocket.messages[-1]
        self.assertFalse(msg.dropped)
        sent = json.loads(msg.text)
        self.assertIn("SSN blocked", json.dumps(sent))

    def test_custom_ws_voice_transcript_is_dropped(self) -> None:
        frame = json.dumps({"type": "voice", "transcript": f"my ssn is {SSN}", "final": True})
        f = _ws_flow("ai.acme-internal.io", "/ws/realtime", frame)
        NS["BrowserAIInterceptor"].websocket_message(ADDON, f)
        self.assertTrue(f.websocket.messages[-1].dropped, EVALS)


if __name__ == "__main__":
    unittest.main()
