from __future__ import annotations

import sys
import threading
import unittest
from pathlib import Path
from unittest import mock

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import agent_heartbeat as hb  # noqa: E402


class CommandWaitLoopTest(unittest.TestCase):
    def test_uninstall_event_removes_guard_immediately(self) -> None:
        stop = threading.Event()
        with mock.patch.object(hb, "_http_json", return_value=(200, {"event": "uninstall"})), \
                mock.patch.object(hb, "send_heartbeat", return_value={"command": "uninstall"}), \
                mock.patch.object(hb, "apply_admin_uninstall") as uninstall:
            hb.command_wait_loop("agent-1", stop)
        uninstall.assert_called_once_with("agent-1")

    def test_rebuild_event_sends_heartbeat_now(self) -> None:
        stop = threading.Event()
        responses = iter([(200, {"event": "rebuild"})])

        def fake_http(*_args, **_kwargs):
            try:
                return next(responses)
            except StopIteration:
                stop.set()
                return 200, {"event": ""}

        with mock.patch.object(hb, "_http_json", side_effect=fake_http), \
                mock.patch.object(hb, "send_heartbeat", return_value={"command": ""}) as beat, \
                mock.patch.object(hb, "apply_admin_uninstall") as uninstall, \
                mock.patch.object(hb.random, "uniform", return_value=0.0):
            hb.command_wait_loop("agent-1", stop)
        beat.assert_called_once()
        uninstall.assert_not_called()

    def test_old_server_without_route_backs_off(self) -> None:
        stop = threading.Event()
        waits: list[float] = []

        def fake_wait(seconds: float) -> bool:
            waits.append(seconds)
            stop.set()
            return True

        with mock.patch.object(hb, "_http_json", return_value=(404, None)), \
                mock.patch.object(stop, "wait", side_effect=fake_wait):
            hb.command_wait_loop("agent-1", stop)
        self.assertEqual(waits, [600])


if __name__ == "__main__":
    unittest.main()
