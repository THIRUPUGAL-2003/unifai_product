from __future__ import annotations

import json
import sys
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import agent_heartbeat as hb  # noqa: E402
import agent_pac_server as pac  # noqa: E402
import agent_policy_sync as sync  # noqa: E402


def _reset() -> None:
    sync._version = 0
    sync._fingerprint = ""
    sync._uninstalling = False
    sync._refresh_nudge.clear()
    sync.pac_nudge.clear()


class PolicyVersionTest(unittest.TestCase):
    def setUp(self) -> None:
        _reset()

    def test_version_bumps_only_on_real_change(self) -> None:
        self.assertTrue(sync.note_policy([["chatgpt.com"], [], [], {}]))
        self.assertFalse(sync.note_policy([["chatgpt.com"], [], [], {}]))
        self.assertTrue(sync.note_policy([["chatgpt.com", "claude.ai"], [], [], {}]))
        self.assertEqual(sync.current_version(), 2)

    def test_waiter_wakes_within_a_second_of_change(self) -> None:
        sync.note_policy(["a"])
        since = sync.current_version()
        got: list[tuple[int, bool]] = []
        t = threading.Thread(target=lambda: got.append(sync.wait_for_change(since, 10)))
        t.start()
        time.sleep(0.1)
        started = time.monotonic()
        sync.note_policy(["b"])
        t.join(2)
        self.assertLess(time.monotonic() - started, 1.0)
        self.assertEqual(got, [(since + 1, False)])

    def test_uninstall_releases_waiters(self) -> None:
        got: list[tuple[int, bool]] = []
        t = threading.Thread(target=lambda: got.append(sync.wait_for_change(sync.current_version(), 10)))
        t.start()
        time.sleep(0.1)
        sync.signal_uninstalling()
        t.join(2)
        self.assertTrue(got and got[0][1])

    def test_refresh_now_reads_backend_and_bumps(self) -> None:
        import agent_extension as ext
        with mock.patch.object(ext, "monitored_hosts", return_value=["chatgpt.com"]), \
                mock.patch.object(ext, "blocked_hosts", return_value=[]), \
                mock.patch.object(ext, "dynamic_rules", return_value=[{"pattern": "x"}]), \
                mock.patch.object(ext, "get_governance_controls", return_value={"block_upload": True}), \
                mock.patch.object(ext, "invalidate_policy_caches") as invalidate:
            self.assertEqual(sync.refresh_now(), 1)
            self.assertEqual(sync.refresh_now(), 1)
        self.assertEqual(invalidate.call_count, 2)


class PolicyEventTest(unittest.TestCase):
    def setUp(self) -> None:
        _reset()

    def test_policy_event_nudges_refresh_and_pac_without_heartbeat(self) -> None:
        stop = threading.Event()
        responses = iter([(200, {"event": "policy"})])

        def fake_http(*_args, **_kwargs):
            try:
                return next(responses)
            except StopIteration:
                stop.set()
                return 200, {"event": ""}

        with mock.patch.object(hb, "_http_json", side_effect=fake_http), \
                mock.patch.object(hb, "send_heartbeat") as beat, \
                mock.patch.object(hb.random, "uniform", return_value=0.0):
            hb.command_wait_loop("agent-1", stop)
        beat.assert_not_called()
        self.assertTrue(sync._refresh_nudge.is_set())
        self.assertTrue(sync.pac_nudge.is_set())


class WaitEndpointTest(unittest.TestCase):
    def setUp(self) -> None:
        _reset()

    def _get(self, path: str) -> dict:
        response: list[dict] = []
        handler = type("HandlerStub", (), {})()
        handler.path = path
        handler._json = lambda code, payload: response.append(payload)
        pac._PACRequestHandler.do_GET(handler)
        return response[0]

    def test_wait_returns_new_version_after_change(self) -> None:
        sync.note_policy(["a"])
        got: list[dict] = []
        t = threading.Thread(target=lambda: got.append(self._get("/extension/wait?v=1&t=10")))
        t.start()
        time.sleep(0.2)
        sync.note_policy(["b"])
        t.join(3)
        self.assertEqual(got, [{"policy_version": 2, "uninstalling": False}])

    def test_wait_returns_immediately_for_stale_version(self) -> None:
        sync.note_policy(["a"])
        started = time.monotonic()
        self.assertEqual(self._get("/extension/wait?v=-1&t=10")["policy_version"], 1)
        self.assertLess(time.monotonic() - started, 1.0)


if __name__ == "__main__":
    unittest.main()
