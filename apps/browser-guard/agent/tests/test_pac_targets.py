from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path
from unittest import mock

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import agent_pac_content as pac  # noqa: E402


class LocalPacFromTargetsTest(unittest.TestCase):
    def test_any_admin_domain_form_is_routed(self) -> None:
        targets = {"targets": [
            {"domain": "https://www.ChatGPT.com/c/1", "monitored": True},
            {"domain": "*.deepseek.com", "monitored": True},
            {"domain": "copilot.microsoft.com.", "monitored": True},
            {"domain": "ab.chatgpt.com", "monitored": True},
            {"domain": "files.claudeusercontent.com", "monitored": True},
            {"domain": "locked.example", "monitored": False, "block_site": True},
            {"domain": "paused.example", "monitored": False},
        ]}
        with mock.patch.object(pac, "_http_get_text", return_value=json.dumps(targets)):
            body = pac.build_pac_from_targets("127.0.0.1:18103")
        self.assertIsNotNone(body)
        for host in ("chatgpt.com", "deepseek.com", "copilot.microsoft.com",
                     "files.claudeusercontent.com", "locked.example"):
            self.assertIn(f'"{host}"', body)
        self.assertNotIn('"ab.chatgpt.com"', body)
        self.assertNotIn('"paused.example"', body)
        host_list = body.split("var aiHosts = [", 1)[1].split("];", 1)[0]
        self.assertNotIn("*", host_list)


if __name__ == "__main__":
    unittest.main()
