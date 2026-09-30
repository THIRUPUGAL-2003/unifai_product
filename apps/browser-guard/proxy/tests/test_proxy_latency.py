#!/usr/bin/env python3
"""Guard must not add latency: AI answers stream through, and a slow prompt
check (backend / AI Guard Bot) holds only its own request — never other tabs."""

from __future__ import annotations

import asyncio
import gzip
import json
import threading
import time
import unittest

import sys
from pathlib import Path

_TEST_DIR = str(Path(__file__).resolve().parent)
if _TEST_DIR not in sys.path:
    sys.path.insert(0, _TEST_DIR)

from mitmproxy import http

from test_target_predict_e2e import NS, _flow, _install_rules, _reset, _send_bodies


def _with_response(flow, content_type: str, headers: dict | None = None):
    hdrs = {"content-type": content_type, **(headers or {})}
    flow.response = http.Response.make(200, b"", hdrs)
    flow.response.content = None
    return flow


class ResponseStreamingTests(unittest.TestCase):
    def setUp(self) -> None:
        self.addon = NS["addons"][0]

    def test_ai_answers_stream_immediately(self) -> None:
        for host, path, ctype in (
            ("chatgpt.com", "/backend-api/f/conversation", "text/event-stream; charset=utf-8"),
            ("claude.ai", "/api/organizations/o/chat_conversations/c/completion", "text/event-stream"),
            ("www.perplexity.ai", "/rest/sse/perplexity_ask", "text/event-stream"),
            ("chat.deepseek.com", "/api/v0/chat/completion", "text/event-stream"),
            ("grok.com", "/rest/app-chat/conversations/new", "application/x-ndjson"),
        ):
            f = _with_response(_flow(host, path, "{}"), ctype)
            self.addon.responseheaders(f)
            self.assertIs(f.response.stream, True, host)

    def test_chunked_json_streams_chunk_by_chunk(self) -> None:
        f = _with_response(_flow("gemini.google.com", "/_/BardChatUi/data/StreamGenerate", "{}"), "application/json")
        self.addon.responseheaders(f)
        tee = f.response.stream
        self.assertTrue(callable(tee))
        self.assertEqual(tee(b'[["wrb.fr","part 1"]]'), b'[["wrb.fr","part 1"]]')
        self.assertEqual(tee(b'[["wrb.fr","part 2"]]'), b'[["wrb.fr","part 2"]]')
        self.assertEqual(tee(b""), b"")

    def test_streamed_upload_reply_still_learns_filename(self) -> None:
        seen: list[str] = []
        orig = NS["ingest_upload_filenames_from_body"]
        NS["ingest_upload_filenames_from_body"] = lambda text, bind: seen.append(text)
        try:
            f = _with_response(
                _flow("ai.acme-internal.io", "/api/files", "{}"),
                "application/json", {"content-encoding": "gzip"},
            )
            self.addon.responseheaders(f)
            body = gzip.compress(json.dumps({"file_id": "file-abc123", "filename": "payroll.txt"}).encode())
            tee = f.response.stream
            for i in range(0, len(body), 7):
                tee(body[i:i + 7])
            tee(b"")
            NS["BrowserAIInterceptor"].response(self.addon, f)
        finally:
            NS["ingest_upload_filenames_from_body"] = orig
        self.assertTrue(any("payroll.txt" in t for t in seen), seen)

    def test_oversized_reply_is_not_copied(self) -> None:
        f = _with_response(_flow("ai.acme-internal.io", "/api/big", "{}"), "application/json")
        self.addon.responseheaders(f)
        tee = f.response.stream
        chunk = b"x" * (1024 * 1024)
        for _ in range(3):
            self.assertEqual(tee(chunk), chunk)
        tee(b"")
        self.assertNotIn(NS["_RESPONSE_LEARN_META"], f.metadata)


class NonBlockingCheckTests(unittest.TestCase):
    def setUp(self) -> None:
        _reset()
        _install_rules()
        self.orig_eval = NS["evaluate_prompt"]

    def tearDown(self) -> None:
        NS["evaluate_prompt"] = self.orig_eval
        NS["_EVENT_LOOP"] = None

    def test_slow_prompt_check_does_not_stall_other_requests(self) -> None:
        def slow_eval(platform, domain, prompt, client_ip, url, method):
            time.sleep(0.8)
            return True, "", "Allowed", prompt, ""

        NS["evaluate_prompt"] = slow_eval
        _, host, path, body, ctype = _send_bodies("summarize the board meeting notes")[0]
        slow_flow = _flow(host, path, body, content_type=ctype)
        other_flow = _flow("example.com", "/", b"", method="GET")
        addon = NS["addons"][0]

        async def scenario():
            slow = asyncio.ensure_future(addon.request(slow_flow))
            await asyncio.sleep(0.1)
            t0 = time.monotonic()
            await addon.request(other_flow)
            await asyncio.sleep(0.01)
            other_elapsed = time.monotonic() - t0
            slow_still_running = not slow.done()
            await slow
            return other_elapsed, slow_still_running

        other_elapsed, slow_still_running = asyncio.run(scenario())
        self.assertTrue(slow_still_running, "slow check finished too early for this test to prove anything")
        self.assertLess(other_elapsed, 0.4, f"other tab waited {other_elapsed:.2f}s behind a prompt check")

    def test_parallel_duplicate_sends_evaluate_once(self) -> None:
        calls: list[str] = []

        def slow_eval(platform, domain, prompt, client_ip, url, method):
            calls.append(prompt)
            time.sleep(0.3)
            return False, "SSN Rule", "Blocked", prompt, "blocked"

        NS["evaluate_prompt"] = slow_eval
        results: list[tuple] = []

        def worker():
            results.append(NS["evaluate_prompt_coalesced"](
                "ChatGPT", "chatgpt.com", "same prompt twice", "1.2.3.4", "https://chatgpt.com/", "POST",
            ))

        threads = [threading.Thread(target=worker) for _ in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join(5)
        self.assertEqual(len(calls), 1, calls)
        self.assertEqual(len(results), 4)
        self.assertTrue(all(r[0] is False for r in results), results)

    def test_websocket_inject_runs_on_proxy_loop(self) -> None:
        ran_on: list[int] = []

        async def scenario():
            NS["_EVENT_LOOP"] = asyncio.get_running_loop()
            loop_thread = threading.get_ident()
            await asyncio.get_running_loop().run_in_executor(
                None, NS["_run_on_event_loop"], lambda: ran_on.append(threading.get_ident()),
            )
            await asyncio.sleep(0.05)
            return loop_thread

        loop_thread = asyncio.run(scenario())
        self.assertEqual(ran_on, [loop_thread])


if __name__ == "__main__":
    unittest.main()
