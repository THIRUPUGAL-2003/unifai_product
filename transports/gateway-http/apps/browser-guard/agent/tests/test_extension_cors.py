from __future__ import annotations

import sys
import unittest
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AGENT_DIR))

import agent_pac_server as pac  # noqa: E402


class ExtensionCorsTest(unittest.TestCase):
    def test_private_network_opt_in(self) -> None:
        handler = object.__new__(pac._PACRequestHandler)
        sent = []
        handler.send_header = lambda key, value: sent.append((key, value))
        handler._cors()
        self.assertIn(("Access-Control-Allow-Origin", "*"), sent)
        self.assertIn(("Access-Control-Allow-Private-Network", "true"), sent)


if __name__ == "__main__":
    unittest.main()
