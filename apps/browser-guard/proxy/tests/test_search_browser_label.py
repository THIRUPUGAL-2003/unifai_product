"""Search Logs label every browser, including ones that do not exist yet."""

from __future__ import annotations

import ast
import unittest
from pathlib import Path

_SRC = Path(__file__).resolve().parents[1] / "gateway_proxy_parts" / "responses_addon.py"


def _load_label():
    tree = ast.parse(_SRC.read_text(encoding="utf-8"))
    body = []
    for node in tree.body:
        if isinstance(node, ast.ClassDef):
            break
        if isinstance(node, ast.FunctionDef) and node.name == "_search_browser_label":
            body.append(node)
        elif isinstance(node, ast.Assign):
            names = [getattr(t, "id", "") for t in node.targets]
            if "_UA_GENERIC" in names or "_CH_IGNORE" in names:
                body.append(node)
    ns: dict = {}
    exec(compile(ast.Module(body=body, type_ignores=[]), str(_SRC), "exec"), ns)
    return ns["_search_browser_label"]


class SearchBrowserLabelTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.label = _load_label()

    def test_known_and_future_browsers(self):
        cases = [
            ("Mozilla/5.0 Chrome/120.0.0.0 Safari/537.36", '"Google Chrome";v="120", "Chromium";v="120"', "Chrome"),
            ("Mozilla/5.0 Chrome/120.0 Edg/120.0", '"Microsoft Edge";v="120"', "Edge"),
            ("Mozilla/5.0 Firefox/121.0", "", "Firefox"),
            ("Mozilla/5.0 Version/17.0 Safari/605.1.15", "", "Safari"),
            ("Mozilla/5.0 Chrome/120 SamsungBrowser/25.0", "", "SamsungBrowser"),
            ("Mozilla/5.0 AppleWebKit/537 FooBrowser/3.1 Chrome/120 Safari/537", '"FooBrowser";v="3"', "FooBrowser"),
            ("SomeFutureAgent/1.0", "", "SomeFutureAgent"),
            ("", "", "Browser"),
        ]
        for ua, ch, want in cases:
            with self.subTest(want=want):
                self.assertEqual(SearchBrowserLabelTests.label(ua, ch), want)


if __name__ == "__main__":
    unittest.main()
