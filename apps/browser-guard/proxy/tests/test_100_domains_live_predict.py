#!/usr/bin/env python3
"""Live-style predict: 100+ admin AI domains/subdomains + 10 regex rules.

Drives BrowserAIInterceptor.request() like a real Guard (prompt, file Send,
rule BLOCK). Writes apps/browser-guard/proxy/live_100_domain_predict_report.txt
"""

from __future__ import annotations

import json
import re
import time
import unittest
from pathlib import Path

from mitmproxy.test import tflow, tutils

PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "gateway_proxy_parts"
REPORT = PROXY_DIR / "live_100_domain_predict_report.txt"
PARTS = [
    "config_caches_rules.py",
    "helpers_prompts.py",
    "uploads_detect.py",
    "file_policy.py",
    "extract_office_backend.py",
    "responses_inject.py",
    "responses_addon.py",
]

# Parent AI / custom domains (admin Target Websites). Subdomains added below.
PARENTS: list[tuple[str, str]] = [
    ("chatgpt.com", "ChatGPT"),
    ("openai.com", "OpenAI"),
    ("claude.ai", "Claude"),
    ("anthropic.com", "Anthropic"),
    ("gemini.google.com", "Gemini"),
    ("deepseek.com", "DeepSeek"),
    ("chat.deepseek.com", "DeepSeek Chat"),
    ("perplexity.ai", "Perplexity"),
    ("copilot.microsoft.com", "Copilot"),
    ("bing.com", "Bing"),
    ("grok.com", "Grok"),
    ("x.ai", "xAI"),
    ("chat.mistral.ai", "Mistral"),
    ("mistral.ai", "Mistral AI"),
    ("poe.com", "Poe"),
    ("you.com", "You.com"),
    ("character.ai", "Character.AI"),
    ("pi.ai", "Pi"),
    ("jasper.ai", "Jasper"),
    ("writesonic.com", "Writesonic"),
    ("huggingface.co", "HuggingFace"),
    ("cohere.com", "Cohere"),
    ("phind.com", "Phind"),
    ("kagi.com", "Kagi"),
    ("inflection.ai", "Inflection"),
    ("github.com", "GitHub"),
    ("cursor.com", "Cursor"),
    ("notion.so", "Notion"),
    ("slack.com", "Slack"),
    ("ai.acme-internal.io", "Acme Custom AI"),
    ("corp-ai.example.com", "Corp AI"),
    ("mybot.internal", "MyBot"),
    ("llm.contoso.local", "Contoso LLM"),
    ("assistant.fabrikam.io", "Fabrikam Assistant"),
    ("chat.northwind.dev", "Northwind Chat"),
    ("ask.adventure-works.com", "Adventure Works Ask"),
    ("bot.tailspin.ai", "Tailspin Bot"),
    ("genai.wideworldimporters.net", "WWI GenAI"),
    ("copilot.litware.com", "Litware Copilot"),
    ("ai.alpine-ski.house", "Alpine AI"),
    ("chat.humongous.insurance", "Humongous Chat"),
    ("assistant.woodgrove.bank", "Woodgrove Assistant"),
    ("llm.proseware.org", "Proseware LLM"),
    ("ai.fourthcoffee.com", "Fourth Coffee AI"),
    ("bot.graphicdesigninstitute.edu", "GDI Bot"),
    ("ask.blueyonderairlines.com", "Blue Yonder Ask"),
    ("chat.citypower.utility", "City Power Chat"),
    ("ai.southridgevideo.com", "Southridge AI"),
    ("assistant.treyresearch.net", "Trey Research"),
    ("llm.wingtiptoys.com", "Wingtip LLM"),
]

SUB_PREFIXES = ("www", "app", "chat", "api", "cdn", "files", "upload", "assets", "ws")

SSN = "123-45-6789"


def _build_targets() -> list[tuple[str, str]]:
    rows: list[tuple[str, str]] = []
    seen: set[str] = set()
    for domain, plat in PARENTS:
        d = domain.lower().strip(".")
        if d and d not in seen:
            seen.add(d)
            rows.append((d, plat))
        for pref in SUB_PREFIXES:
            # Skip nonsense like chat.chat.deepseek.com when parent already starts with chat.
            host = f"{pref}.{d}"
            if host in seen:
                continue
            seen.add(host)
            rows.append((host, plat))
    return rows


TARGETS = _build_targets()
assert len(TARGETS) >= 100, f"need 100+ targets, got {len(TARGETS)}"


TEN_RULES = [
    ("SSN Rule", r"\b\d{3}-\d{2}-\d{4}\b", "BLOCK"),
    ("Phone Rule", r"\b(?:\+?91[- ]?)?[6-9]\d{9}\b", "WARN"),
    ("PAN Rule", r"\b[A-Z]{5}[0-9]{4}[A-Z]\b", "BLOCK"),
    ("Email Rule", r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b", "WARN"),
    ("Aadhaar Rule", r"\b\d{4}[-\s]?\d{4}[-\s]?\d{4}\b", "BLOCK"),
    ("AWS Key Rule", r"\bAKIA[0-9A-Z]{16}\b", "BLOCK"),
    ("Secret Token Rule", r"(?i)\bCONFIDENTIAL_INTERNAL_KEY_[A-Z0-9_]+\b", "BLOCK"),
    ("Credit Card Rule", r"\b(?:4\d{3}|5[1-5]\d{2}|3[47]\d{2})[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b", "BLOCK"),
    ("IBAN Rule", r"\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b", "WARN"),
    ("Password Leak Rule", r"(?i)\b(?:password|passwd|pwd)\s*[:=]\s*\S+", "BLOCK"),
]


def _load():
    ns: dict = {"__name__": "browser_ai_proxy_100_live"}
    for name in PARTS:
        path = PARTS_DIR / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    ns["_bg_config_refresh_started"] = True
    ns["_refresh_targets_from_backend"] = lambda: None
    return ns


NS = _load()
EVALS: list[tuple[str, str]] = []
UPLOAD_LOGS: list[dict] = []


def _fake_evaluate(platform, domain, prompt, client_ip, url, method):
    EVALS.append((domain, prompt))
    return NS["decide_prompt_locally"](prompt)


def _fake_upload_log(**kwargs):
    UPLOAD_LOGS.append(kwargs)
    return True


NS["evaluate_prompt"] = _fake_evaluate
NS["log_prompt_async"] = lambda *a, **k: None
NS["post_upload_intercept"] = _fake_upload_log
ADDON = object.__new__(NS["BrowserAIInterceptor"])


def _arm() -> None:
    NS["_apply_targets_from_data"]({
        "targets": [
            {"domain": d, "platform_name": p, "monitored": True} for d, p in TARGETS
        ]
    })
    rules = []
    for name, pat, action in TEN_RULES:
        rules.append({
            "name": name,
            "pattern": pat,
            "regex": re.compile(pat, re.IGNORECASE),
            "action": action,
            "severity": "HIGH",
            "warning_message": f"{name} hit",
        })
    NS["_cached_rules"] = rules
    NS["_rules_fetched_at"] = time.time()
    NS["_rules_fetch_ok"] = True
    NS["_controls_fetched_at"] = time.time()


def _reset() -> None:
    EVALS.clear()
    UPLOAD_LOGS.clear()
    for lock_name, store_names in (
        ("_FILE_ID_NAME_REGISTRY_LOCK", ["_FILE_ID_NAME_REGISTRY"]),
        ("_DOMAIN_PENDING_NAMES_LOCK", ["_DOMAIN_PENDING_NAMES"]),
        ("_CONTENT_HASH_NAME_LOCK", ["_CONTENT_HASH_NAME_REGISTRY"]),
        ("_UPLOAD_FILE_CACHE_LOCK", ["_UPLOAD_FILE_CACHE", "_UPLOAD_FILE_QUEUES"]),
        ("_CLIENT_TARGET_STICKY_LOCK", ["_CLIENT_TARGET_STICKY"]),
    ):
        with NS[lock_name]:
            for s in store_names:
                NS[s].clear()
    for name in ("_recent_prompts", "_recent_decisions", "_composer_draft", "_FILE_SEND_BLOCKS"):
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


def _multipart(filename: str, data: bytes) -> tuple[bytes, str]:
    boundary = "----WebKitFormBoundary100Live"
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\n"
        f"Content-Type: text/plain\r\n\r\n"
    ).encode() + data + f"\r\n--{boundary}--\r\n".encode()
    return body, f"multipart/form-data; boundary={boundary}"


def _chat_body(text: str) -> str:
    return json.dumps({
        "model": "auto",
        "stream": True,
        "messages": [{"role": "user", "content": text}],
        "prompt": text,
        "query": text,
        "question": text,
    })


class HundredDomainLivePredictTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        _arm()
        cls.rows: list[str] = []
        cls.fail_prompt: list[str] = []
        cls.fail_block: list[str] = []
        cls.fail_file: list[str] = []
        cls.fail_detect: list[str] = []

    @classmethod
    def tearDownClass(cls) -> None:
        total = len(TARGETS)
        ok_prompt = total - len(cls.fail_prompt)
        ok_block = total - len(cls.fail_block)
        ok_file = total - len(cls.fail_file)
        ok_detect = total - len(cls.fail_detect)
        lines = [
            "Gateway Guard — 100+ AI domain live-style predict report",
            f"Generated: {time.strftime('%Y-%m-%d %H:%M:%S')}",
            f"Targets: {total} (parents + subdomains)",
            f"Regex rules: {len(TEN_RULES)}",
            "",
            f"detect_target PASS: {ok_detect}/{total}",
            f"prompt predict PASS: {ok_prompt}/{total}",
            f"SSN rule BLOCK PASS: {ok_block}/{total}",
            f"file filename -- prompt PASS: {ok_file}/{total}",
            "",
        ]
        if cls.fail_detect:
            lines.append("DETECT FAIL:")
            lines.extend(f"  - {x}" for x in cls.fail_detect[:40])
            lines.append("")
        if cls.fail_prompt:
            lines.append("PROMPT FAIL:")
            lines.extend(f"  - {x}" for x in cls.fail_prompt[:40])
            lines.append("")
        if cls.fail_block:
            lines.append("BLOCK FAIL:")
            lines.extend(f"  - {x}" for x in cls.fail_block[:40])
            lines.append("")
        if cls.fail_file:
            lines.append("FILE FAIL:")
            lines.extend(f"  - {x}" for x in cls.fail_file[:40])
            lines.append("")
        lines.append("Sample PASS rows:")
        lines.extend(cls.rows[:25])
        REPORT.write_text("\n".join(lines) + "\n", encoding="utf-8")
        print("\n" + "\n".join(lines[:20]))
        print(f"\nFull report: {REPORT}")

    def test_00_target_count_and_ten_rules(self) -> None:
        self.assertGreaterEqual(len(TARGETS), 100)
        self.assertEqual(len(TEN_RULES), 10)
        self.assertEqual(len(NS["_cached_domains"]), len(TARGETS))
        self.assertEqual(len(NS["_cached_rules"]), 10)

    def test_01_every_domain_prompt_file_and_rules(self) -> None:
        for host, plat in TARGETS:
            _reset()
            _arm()

            ok, matched, got_plat = NS["detect_target"](host)
            if not ok:
                self.fail_detect.append(host)
                continue

            # 1) Clean prompt predict
            text = f"plan Q3 capacity for {plat}"
            f = _flow(host, "/v1/chat/completions", _chat_body(text))
            _run(f)
            if not any(text in p for _, p in EVALS):
                self.fail_prompt.append(f"{host} evals={EVALS!r}")
                continue

            # 2) SSN rule BLOCK on prompt Send
            _reset()
            _arm()
            ssn_text = f"employee ssn is {SSN} for payroll"
            f2 = _flow(host, "/v1/chat/completions", _chat_body(ssn_text))
            _run(f2)
            if f2.response is None:
                self.fail_block.append(f"{host} not blocked; evals={EVALS!r}")
                continue

            # 3) File upload → Send: filename -- caption + SSN block from file
            _reset()
            _arm()
            # Sticky bind for CDN-style follow-up (laptop 127.0.0.1)
            warm = _flow(host, "/v1/chat/completions", _chat_body("warmup"))
            _run(warm)
            EVALS.clear()
            UPLOAD_LOGS.clear()

            fname = "payroll.txt"
            data = f"tax record SSN {SSN}\n".encode() * 4
            ubody, mct = _multipart(fname, data)
            cdn_host = f"files.{host}" if not host.startswith("files.") else host
            # Prefer subdomain CDN if it was added; else upload on same host.
            up_host = cdn_host if NS["detect_target"](cdn_host)[0] else host
            up = _flow(
                up_host, "/api/upload", ubody, content_type=mct,
                headers={"Referer": f"https://{host}/chat"},
            )
            _run(up)
            caption = "review payroll file"
            send = _flow(host, "/v1/chat/completions", json.dumps({
                "prompt": caption,
                "messages": [{"role": "user", "content": caption}],
                "attachments": [{"file_name": fname}],
                "files": [fname],
            }))
            _run(send)
            for _ in range(30):
                if UPLOAD_LOGS:
                    break
                time.sleep(0.03)
            joined = " | ".join(str(u.get("prompt") or "") for u in UPLOAD_LOGS)
            if fname not in joined or f" -- {caption}" not in joined:
                # File path may block with cache on same host; accept block + filename in log
                if send.response is not None and fname in joined:
                    pass
                else:
                    self.fail_file.append(f"{host} log={joined!r} blocked={send.response is not None}")
                    continue
            if send.response is None:
                self.fail_file.append(f"{host} file SSN not blocked; log={joined!r}")
                continue

            self.rows.append(f"PASS {host} ({plat}) prompt+block+file")

        self.assertEqual(self.fail_detect, [], f"detect fails: {self.fail_detect[:10]}")
        self.assertEqual(self.fail_prompt, [], f"prompt fails: {self.fail_prompt[:10]}")
        self.assertEqual(self.fail_block, [], f"block fails: {self.fail_block[:10]}")
        self.assertEqual(self.fail_file, [], f"file fails: {self.fail_file[:10]}")


if __name__ == "__main__":
    unittest.main()
