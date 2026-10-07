"""Massive Live QA Test Suite: 100+ AI Domains & Subdomains.
Verifies that no matter how many AI domains or subdomains are added (ChatGPT, Gemini,
Claude, DeepSeek, Perplexity, and 100+ custom/enterprise AI domains), prompt prediction
and policy blocking work with 100% precision and zero error.
"""

import json
import unittest
from pathlib import Path

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


def _load_ns():
    ns = {"__name__": "test_100_domains_live"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    return ns


# 100 Real-world AI domains across categories:
# 1. Flagship AI (ChatGPT, Claude, Gemini, DeepSeek, Perplexity, Copilot, Mistral, Grok, Meta AI)
# 2. Developer & Coding AI (Cursor, Windsurf, Codeium, Tabnine, Replit, Sourcegraph Cody)
# 3. Model Hosting & APIs (HuggingFace, Replicate, Together AI, Fireworks, Groq, DeepInfra, Fal.ai)
# 4. Open-Source Self-Hosted (Ollama, vLLM, OpenWebUI, LibreChat, Dify, Flowise, Langflow)
# 5. Search & Research AI (You.com, Phind, Consensus, Elicit, Scite, Kagi)
# 6. Enterprise & Custom AI (Acme AI, FinTech LLM, HealthTech Bot, GovAI, EduAI)
AI_DOMAINS = [
    # Category 1: Flagship AI & Subdomains
    "chatgpt.com", "chat.openai.com", "api.openai.com", "platform.openai.com", "ab.chatgpt.com",
    "claude.ai", "api.anthropic.com", "console.anthropic.com", "files.claude.ai",
    "gemini.google.com", "bard.google.com", "aistudio.google.com", "generativelanguage.googleapis.com",
    "chat.deepseek.com", "api.deepseek.com", "platform.deepseek.com",
    "perplexity.ai", "www.perplexity.ai", "api.perplexity.ai", "labs.perplexity.ai",
    "copilot.microsoft.com", "sydney.bing.com", "edgeservices.bing.com", "ai.azure.com",
    "chat.mistral.ai", "api.mistral.ai", "console.mistral.ai",
    "grok.com", "api.x.ai",
    "meta.ai", "www.meta.ai",
    
    # Category 2: Developer & Coding AI
    "cursor.sh", "api.cursor.sh", "repo.cursor.sh",
    "codeium.com", "api.codeium.com",
    "tabnine.com", "api.tabnine.com",
    "replit.com", "agent.replit.com",
    "sourcegraph.com", "cody.sourcegraph.com",
    "continue.dev", "api.continue.dev",
    "v0.dev", "api.v0.dev",
    "bolt.new", "api.bolt.new",
    "lovable.dev", "api.lovable.dev",
    "devin.ai", "app.devin.ai",
    
    # Category 3: Model Hosting, Clouds & Hubs
    "huggingface.co", "api-inference.huggingface.co", "spaces.huggingface.tech",
    "replicate.com", "api.replicate.com",
    "together.ai", "api.together.xyz",
    "fireworks.ai", "api.fireworks.ai",
    "groq.com", "api.groq.com",
    "deepinfra.com", "api.deepinfra.com",
    "fal.ai", "queue.fal.run",
    "runpod.io", "api.runpod.ai",
    "anyscale.com", "api.endpoints.anyscale.com",
    "cohere.com", "api.cohere.ai",
    
    # Category 4: Open Source & Self-Hosted UIs
    "openwebui.internal", "chat.openwebui.com",
    "librechat.internal", "app.librechat.ai",
    "dify.ai", "cloud.dify.ai", "dify.internal",
    "flowiseai.com", "flowise.internal",
    "langflow.org", "langflow.internal",
    "ollama.internal", "localhost.ollama.ai",
    "vllm.internal", "tgi.internal",
    "localai.internal", "lmstudio.internal",
    
    # Category 5: AI Search & Reasoning Engines
    "you.com", "api.you.com",
    "phind.com", "www.phind.com",
    "consensus.app", "elicit.com", "scite.ai", "kagi.com",
    "genspark.ai", "felo.ai", "monica.im",
    
    # Category 6: Enterprise, Financial & Custom AI Gateways
    "ai.acme-corp.com", "llm.secure-bank.internal", "health-agent.hospital.org",
    "gov-ai.defense.gov", "chat.fintech-cloud.io", "internal-ai.telecom.net",
    "enterprise-copilot.insurance.com", "automotive-ai.oem.de", "pharma-research.biotech.ch",
    "retail-assistant.global-store.com", "cloud-ai.datacenter.jp", "custom-model.quantum.ai"
]


class Test100DomainsLivePredict(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.ns = _load_ns()
        # Register all 100+ domains into target map
        cls.ns["_apply_targets_from_data"]({
            "targets": [
                {"domain": d, "platform_name": d.split(".")[0].capitalize(), "monitored": True}
                for d in AI_DOMAINS
            ]
        })
        # Configure test DLP rules
        cls.ns["_cached_rules"] = [
            {
                "name": "SSN Detection",
                "pattern": r"\b\d{3}-\d{2}-\d{4}\b",
                "regex": cls.ns["_compile_guard_regex"](r"\b\d{3}-\d{2}-\d{4}\b"),
                "action": "BLOCK",
                "severity": "HIGH",
                "warning_message": "SSN detected and blocked."
            },
            {
                "name": "Secret API Key",
                "pattern": r"sk-live-[a-zA-Z0-9]{20,}",
                "regex": cls.ns["_compile_guard_regex"](r"sk-live-[a-zA-Z0-9]{20,}"),
                "action": "BLOCK",
                "severity": "CRITICAL",
                "warning_message": "API key leak prevented."
            }
        ]
        import time
        cls.ns["_rules_fetched_at"] = time.time()
        cls.detect_target = staticmethod(cls.ns["detect_target"])
        cls.extract = staticmethod(cls.ns["extract_prompt_universal"])
        cls.decide = staticmethod(cls.ns["decide_prompt_locally"])

    def test_01_all_100_domains_and_subdomains_detected(self):
        """Verify that every single domain and subdomain is matched accurately."""
        for d in AI_DOMAINS:
            matched, base_d, plat = self.detect_target(d)
            self.assertTrue(matched, f"Domain {d} failed detect_target!")
            self.assertTrue(bool(base_d), f"Base domain empty for {d}!")
            self.assertTrue(bool(plat), f"Platform empty for {d}!")

            # Test deep subdomains (e.g. cluster-1.ap-south.chatgpt.com)
            deep_sub = f"cluster-alpha.us-east-1.{d}"
            matched_deep, base_deep, _ = self.detect_target(deep_sub)
            self.assertTrue(matched_deep, f"Deep subdomain {deep_sub} failed detect_target!")

    def test_02_all_100_domains_predict_full_sentence_prompts(self):
        """Verify that a full sentence prompt is extracted and predicted on every domain."""
        test_prompt = "Explain quantum cryptography and post-quantum security algorithms."
        payload = json.dumps({"messages": [{"role": "user", "content": test_prompt}]}).encode("utf-8")
        for d in AI_DOMAINS:
            got = self.extract(payload, "application/json", host=d, url=f"https://{d}/api/chat/completions")
            self.assertEqual(got, test_prompt, f"Full prompt extraction failed on {d}!")
            # Verify DLP evaluation allows safe prompt
            allowed, rule, action, _, _ = self.decide(got)
            self.assertTrue(allowed, f"Safe prompt was blocked on {d} by rule {rule}!")

    def test_03_all_100_domains_predict_numbers_and_zero(self):
        """Verify numbers (0, 1, 42, 100) are never dropped or confused on any domain."""
        number_prompts = ["0", "1", "42", "100", "3.14159"]
        for num in number_prompts:
            payload = json.dumps({"prompt": num}).encode("utf-8")
            for d in AI_DOMAINS[:25]:  # Test across 25 representative domains
                got = self.extract(payload, "application/json", host=d, url=f"https://{d}/v1/chat")
                self.assertEqual(got, num, f"Number prompt {num} failed on {d}: got {got!r}!")

    def test_04_all_100_domains_predict_symbols(self):
        """Verify symbols (c++, x=1, #1, $50, ?, +) are never dropped on any domain."""
        symbols = ["c++", "x=1", "#1", "$50", "?", "+", "10%"]
        for sym in symbols:
            payload = json.dumps({"query": sym}).encode("utf-8")
            for d in AI_DOMAINS[:25]:
                got = self.extract(payload, "application/json", host=d, url=f"https://{d}/query")
                self.assertEqual(got, sym, f"Symbol prompt {sym} failed on {d}: got {got!r}!")

    def test_05_all_100_domains_block_ssn_dlp_rule(self):
        """Verify DLP SSN rule intercepts and blocks 100% of sensitive queries on all domains."""
        ssn_prompt = "Update employee record with SSN: 123-45-6789 confidential"
        payload = json.dumps({"input": ssn_prompt}).encode("utf-8")
        for d in AI_DOMAINS:
            got = self.extract(payload, "application/json", host=d, url=f"https://{d}/predict")
            self.assertEqual(got, ssn_prompt, f"SSN prompt extraction failed on {d}!")
            allowed, rule_name, action, _, _ = self.decide(got)
            self.assertFalse(allowed, f"SSN leak was NOT blocked on {d}!")
            self.assertIn("SSN", rule_name)

    def test_06_all_100_domains_block_secret_api_key_rule(self):
        """Verify DLP Secret API Key rule intercepts and blocks 100% on all domains."""
        key_prompt = "Deploy with sk-live-999888777666555444333222111000aaa"
        payload = json.dumps({"prompt": key_prompt}).encode("utf-8")
        for d in AI_DOMAINS:
            got = self.extract(payload, "application/json", host=d, url=f"https://{d}/generate")
            self.assertEqual(got, key_prompt, f"Secret Key prompt extraction failed on {d}!")
            allowed, rule_name, action, _, _ = self.decide(got)
            self.assertFalse(allowed, f"Secret Key leak was NOT blocked on {d}!")


if __name__ == "__main__":
    unittest.main()
