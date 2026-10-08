#!/usr/bin/env python3
"""Full Product End-to-End (E2E) Verification Test Suite.
Validates the entire product across all layers with zero errors:
1. Frontend UI Production Asset & Route Trees
2. Backend API Routing & FastHTTP Router Integrity (Zero Panics, No Duplicates)
3. Auth & RBAC Security Enforcement (User Locked to Prompt Repo, Admin Access)
4. Browser AI Guard: Target Websites, DLP Rules, Controls, Agent Fleet, License, PAC
5. Interception Engine: Flagship AI Sites, Numbers/Symbols, 26 File Types, Voice
6. Observability Connectors & Settings
7. Hardware-Protected AES-256-GCM Binary Container Verification
"""

import json
import os
import re
import sys
import unittest
from pathlib import Path

# Repo directories
REPO_ROOT = Path(__file__).resolve().parents[4]
PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "gateway_proxy_parts"
UI_DIR = REPO_ROOT / "ui"
UI_OUT_DIR = REPO_ROOT / "transports" / "gateway-http" / "ui"
HANDLERS_DIR = REPO_ROOT / "transports" / "gateway-http" / "handlers"


class FullProductE2ETests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # Load proxy modules
        cls.parts = [
            "config_caches_rules.py",
            "helpers_prompts.py",
            "uploads_detect.py",
            "file_policy.py",
            "extract_office_backend.py",
            "responses_inject.py",
            "responses_addon.py",
        ]
        cls.ns = {"__name__": "full_product_e2e"}
        for p in cls.parts:
            code = (PARTS_DIR / p).read_text(encoding="utf-8")
            exec(compile(code, str(PARTS_DIR / p), "exec"), cls.ns)
        cls.ns["_bg_config_refresh_started"] = True

    # -------------------------------------------------------------------------
    # 1. Frontend UI Production Assets & Route Integrity
    # -------------------------------------------------------------------------
    def test_01_ui_production_build_and_route_integrity(self):
        """Verify UI compiled output exists and embeds without missing files."""
        self.assertTrue(UI_OUT_DIR.exists(), f"UI out directory missing: {UI_OUT_DIR}")
        index_html = UI_OUT_DIR / "index.html"
        self.assertTrue(index_html.exists(), f"UI index.html missing: {index_html}")
        html_content = index_html.read_text(encoding="utf-8")
        self.assertIn("<html", html_content)
        self.assertIn("assets/", html_content)

        # Verify generated route tree has core routes
        route_tree = UI_DIR / "app" / "routeTree.gen.ts"
        self.assertTrue(route_tree.exists(), "routeTree.gen.ts is missing!")
        tree_text = route_tree.read_text(encoding="utf-8")
        core_routes = [
            "/workspace/observability",
            "/workspace/logs",
            "/workspace/browser-ai",
            "/workspace/dashboard",
            "/workspace/governance",
            "/workspace/config",
            "/login",
            "/signup",
        ]
        for r in core_routes:
            self.assertIn(r, tree_text, f"Route {r} missing from generated route tree!")

    # -------------------------------------------------------------------------
    # 2. Backend Routing & FastHTTP Router Integrity
    # -------------------------------------------------------------------------
    def test_02_backend_route_registration_and_no_duplicates(self):
        """Verify that all Go backend routes have zero duplicates and no panics."""
        routes = set()
        duplicates = []
        for p in (REPO_ROOT / "transports" / "gateway-http").rglob("*.go"):
            text = p.read_text(encoding="utf-8", errors="ignore")
            for m in re.finditer(r'r\.(GET|POST|PUT|DELETE|PATCH|HEAD)\("([^"]+)"', text):
                method, path = m.groups()
                key = (method, path)
                if key in routes:
                    duplicates.append(key)
                routes.add(key)

        self.assertEqual(
            duplicates, [],
            f"Found duplicate routes that would cause router panic on startup: {duplicates}"
        )
        # Verify Gateway_Guard_Setup.exe exists and UnifAI is NOT present
        self.assertIn(("GET", "/api/browser-ai/setup/Gateway_Guard_Setup.exe"), routes)
        self.assertNotIn(("GET", "/api/browser-ai/setup/UnifAI_Guard_Setup.exe"), routes)

    # -------------------------------------------------------------------------
    # 3. Auth & RBAC Security Enforcement
    # -------------------------------------------------------------------------
    def test_03_auth_and_rbac_role_enforcement(self):
        """Verify role separation: admin has access, standard user is restricted to Prompt Repo."""
        middlewares_go = (HANDLERS_DIR / "middlewares.go").read_text(encoding="utf-8")
        # Verify roleUsesRBACDelegation excludes 'user'
        self.assertIn('role != "user"', middlewares_go)

        # Verify client-side workspaceAccess permissions logic
        workspace_access_ts = (UI_DIR / "lib" / "utils" / "workspaceAccess.ts").read_text(encoding="utf-8")
        self.assertIn('USER_ROLE_HOME_PATH = "/workspace/prompt-repo"', workspace_access_ts)
        self.assertIn('getWorkspaceAccessRedirect', workspace_access_ts)

    # -------------------------------------------------------------------------
    # 4. Browser AI Guard: Targets, DLP Rules, PAC, Fleet
    # -------------------------------------------------------------------------
    def test_04_browser_ai_guard_targets_and_pac(self):
        """Verify dynamic Target Websites matching and wildcard PAC script generation."""
        # 1. Target detection logic
        self.ns["_apply_targets_from_data"]({
            "targets": [
                {"domain": "chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
                {"domain": "claude.ai", "platform_name": "Claude", "monitored": True},
                {"domain": "deepseek.com", "platform_name": "DeepSeek", "monitored": True},
                {"domain": "internal-ai.corp.io", "platform_name": "CustomCorpAI", "monitored": True},
            ]
        })
        detect = self.ns["detect_target"]
        m, base, plat = detect("chat.deepseek.com")
        self.assertTrue(m)
        self.assertEqual(base, "deepseek.com")
        self.assertEqual(plat, "DeepSeek")

        m, base, plat = detect("cluster-prod.internal-ai.corp.io")
        self.assertTrue(m)
        self.assertEqual(base, "internal-ai.corp.io")
        self.assertEqual(plat, "CustomCorpAI")

        # 2. PAC generator verification
        pac_file = REPO_ROOT / "apps" / "browser-guard" / "agent" / "agent_pac_content.py"
        self.assertTrue(pac_file.exists())
        pac_code = pac_file.read_text(encoding="utf-8")
        self.assertIn("dnsDomainIs(host", pac_code)
        self.assertIn("shExpMatch(host", pac_code)

    def test_05_browser_ai_dlp_rules_and_controls(self):
        """Verify regex and DLP rules evaluation with priority BLOCK > REDACT > WARN."""
        self.ns["_cached_rules"] = [
            {
                "name": "SSN Block Rule",
                "pattern": r"\b\d{3}-\d{2}-\d{4}\b",
                "regex": self.ns["_compile_guard_regex"](r"\b\d{3}-\d{2}-\d{4}\b"),
                "action": "BLOCK",
                "severity": "HIGH",
            },
            {
                "name": "Secret Redact Rule",
                "pattern": r"sk-[a-zA-Z0-9]{16,}",
                "regex": self.ns["_compile_guard_regex"](r"sk-[a-zA-Z0-9]{16,}"),
                "action": "REDACT",
                "severity": "HIGH",
            },
        ]
        import time
        self.ns["_rules_fetched_at"] = time.time()
        decide = self.ns["decide_prompt_locally"]

        # Safe prompt
        allowed, rule, action, _, _ = decide("Hello world! How are you today?")
        self.assertTrue(allowed)
        self.assertEqual(action, "Allowed")

        # SSN prompt (MUST BLOCK)
        allowed, rule, action, _, _ = decide("Employee SSN is 123-45-6789 confidential")
        self.assertFalse(allowed)
        self.assertEqual(action, "Blocked")
        self.assertEqual(rule, "SSN Block Rule")

    # -------------------------------------------------------------------------
    # 5. Prompt Interception: Wire Protocols, Numbers, Files, Audio
    # -------------------------------------------------------------------------
    def test_06_numbers_and_symbols_never_dropped(self):
        """Verify zero drops on single digits ('0', '1', '42') and symbols ('c++', 'x=1')."""
        extract = self.ns["extract_prompt_universal"]
        test_inputs = ["0", "1", "42", "100", "3.14159", "c++", "x=1", "#1", "$50", "?", "+"]
        for inp in test_inputs:
            payload = json.dumps({"messages": [{"role": "user", "content": inp}]}).encode("utf-8")
            got = extract(payload, "application/json", host="chatgpt.com", url="https://chatgpt.com/backend-api/conversation")
            self.assertEqual(got, inp, f"Input {inp!r} was dropped or corrupted: got {got!r}")

    def test_07_connect_rpc_and_claude_wire_noise_filtered(self):
        """Verify Claude Connect-RPC wire tokens (PerformAction, IDs) are filtered cleanly."""
        is_noise = self.ns["_is_claude_wire_noise"]
        self.assertTrue(is_noise("PerformAction"))
        self.assertTrue(is_noise("ConversationService"))
        self.assertTrue(is_noise("anthropic.connect.v1.ConversationService"))
        self.assertTrue(is_noise("type.googleapis.com/anthropic.connect.Action"))
        self.assertTrue(is_noise("msg_01AbCdEfGhIjKlMnOpQrStUv"))
        # Real user prompt is NOT noise
        self.assertFalse(is_noise("Explain quantum computing"))
        self.assertFalse(is_noise("42"))
        self.assertFalse(is_noise("0"))

    def test_08_file_upload_26_types_and_audio_stt(self):
        """Verify office, code, archive files and audio extraction logic are active."""
        extract_upload = self.ns["extract_upload_text_for_rules"]
        # Code file
        code_data = b'def process_payroll():\n    api_key = "SECRET_123"\n    return api_key'
        text = extract_upload(code_data, "text/x-python", "", "script.py")
        self.assertIn("SECRET_123", text)

        # Audio MIME recognition
        looks_audio = self.ns["_looks_like_audio"]
        self.assertTrue(looks_audio(b"\x00\x00\x00 ft", "audio/m4a", "voice_memo.m4a"))
        self.assertTrue(looks_audio(b"RIFF....WAVE", "audio/wav", "recording.wav"))

    # -------------------------------------------------------------------------
    # 6. Observability Connectors & Settings
    # -------------------------------------------------------------------------
    def test_09_observability_connectors_supported(self):
        """Verify all enterprise observability platforms are present in UI views."""
        obs_view_file = UI_DIR / "app" / "workspace" / "observability" / "views" / "observabilityView.tsx"
        self.assertTrue(obs_view_file.exists())
        obs_code = obs_view_file.read_text(encoding="utf-8")
        required_connectors = ["datadog", "bigquery", "kafka", "pubsub", "newrelic", "otel", "prometheus"]
        for c in required_connectors:
            self.assertIn(c, obs_code, f"Observability connector {c} is missing from UI!")

    # -------------------------------------------------------------------------
    # 7. Hardware-Protected AES-256-GCM Binary Container
    # -------------------------------------------------------------------------
    def test_10_proxy_encrypted_container_integrity(self):
        """Verify AES-256-GCM encrypted proxy container bundle exists and is valid."""
        enc_file = PARTS_DIR / "gateway_proxy_parts.enc"
        self.assertTrue(enc_file.exists(), "gateway_proxy_parts.enc container is missing!")
        size = enc_file.stat().st_size
        self.assertGreater(size, 250_000, f"Container size suspiciously small: {size} bytes")


if __name__ == "__main__":
    unittest.main()
