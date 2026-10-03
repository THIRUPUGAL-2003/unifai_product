#!/usr/bin/env python3
"""
Raksha Browser Guard - AI Domain Coverage Test Script
======================================================
Tests which AI domains are properly detected and whether
prompts / file uploads will be captured from them.

HOW TO RUN:
    Copy this file to:
    apps/browser-guard/proxy/test_ai_domain_coverage.py

    Then run:
    cd apps/browser-guard/proxy
    python test_ai_domain_coverage.py

Output:
    - Console table: PASS / FAIL / PARTIAL per domain
    - ai_domain_coverage_report.txt  (full report)
    - ai_domain_failures.txt         (only failed domains)
"""

import sys
import os
import json
import re
import datetime

# ─── Load proxy parts (same as browser_ai_proxy.py) ─────────────────────────

_HERE = os.path.dirname(os.path.abspath(__file__))
_PARTS_DIR = os.path.join(_HERE, "raksha_proxy_parts")


def _load_parts():
    manifest = os.path.join(_PARTS_DIR, "MANIFEST.txt")
    with open(manifest, encoding="utf-8-sig") as f:
        names = [ln.strip() for ln in f if ln.strip()]
    ns: dict = {}
    for name in names:
        path = os.path.join(_PARTS_DIR, name)
        with open(path, encoding="utf-8") as f:
            code = f.read()
        try:
            exec(compile(code, path, "exec"), ns)
        except Exception as e:
            print(f"  [WARN] Could not load {name}: {e}")
    return ns


print("Loading proxy parts...", end=" ", flush=True)
try:
    NS = _load_parts()
    detect_target            = NS["detect_target"]
    is_chat_path             = NS["is_chat_path"]
    _body_has_user_send_payload = NS["_body_has_user_send_payload"]
    is_confident_file_upload = NS["is_confident_file_upload"]
    print("OK")
except Exception as e:
    print(f"FAILED: {e}")
    sys.exit(1)

# ─── AI Domains to test ───────────────────────────────────────────────────────
# Format: (domain, platform_label, [chat_paths], [request_bodies])

AI_DOMAINS = [
    # ── Tier 1: Known AIs (must all PASS) ─────────────────────────────────
    ("chat.openai.com",       "ChatGPT",
     ["/backend-api/conversation"],
     ['{"messages":[{"role":"user","content":"hello"}],"model":"gpt-4o"}']),

    ("chatgpt.com",           "ChatGPT",
     ["/backend-api/conversation"],
     ['{"messages":[{"role":"user","content":"explain this"}]}']),

    ("claude.ai",             "Claude",
     ["/api/organizations/abc/chat_conversations/xyz/completion"],
     ['{"prompt":"hello claude","max_tokens":1024}']),

    ("gemini.google.com",     "Gemini",
     ["/_/BardChatUi/data/batchexecute"],
     ['f.req=%5B%5B%5B%22wXbdQc%22']),

    ("deepseek.com",          "DeepSeek",
     ["/api/v0/chat/completion"],
     ['{"messages":[{"role":"user","content":"hi"}],"stream":true}']),

    ("copilot.microsoft.com", "Copilot",
     ["/c/api/chat"],
     ['{"event":"send","conversationId":"abc","text":"hello"}']),

    # ── Tier 2: Popular AIs ────────────────────────────────────────────────
    ("www.perplexity.ai",     "Perplexity",
     ["/socket.io/"],
     ['{"query":"what is AI","search_focus":"internet"}']),

    ("grok.com",              "Grok",
     ["/api/grok/query"],
     ['{"variables":{"message":"hello grok","modelName":"grok-2"}}']),

    ("poe.com",               "Poe",
     ["/api/gql_POST"],
     ['{"query":"SendMessageMutation","variables":{"query":"hi"}}']),

    ("you.com",               "You.com",
     ["/api/streamingSearch"],
     ['{"q":"what is python","chat":[{"question":"hi","answer":"hello"}]}']),

    ("character.ai",          "Character.AI",
     ["/chat/streaming/"],
     ['{"character_external_id":"abc","text":"hello","history_external_id":"xyz"}']),

    ("pi.ai",                 "Pi AI",
     ["/api/chat"],
     ['{"text":"hello pi","conversation":"abc"}']),

    # ── Tier 3: Enterprise AIs ─────────────────────────────────────────────
    ("app.jasper.ai",         "Jasper",
     ["/v1/chat"],
     ['{"messages":[{"role":"user","content":"write me an email"}]}']),

    ("chat.mistral.ai",       "Mistral",
     ["/api/chat/completions"],
     ['{"messages":[{"role":"user","content":"bonjour"}],"model":"mistral-medium"}']),

    ("huggingface.co",        "HuggingFace",
     ["/chat/conversation/abc/message"],
     ['{"inputs":"explain transformers","parameters":{}}']),

    ("cohere.com",            "Cohere",
     ["/v1/chat"],
     ['{"message":"what is RAG","chat_history":[]}']),

    ("phind.com",             "Phind",
     ["/api/infer/followup"],
     ['{"userMessage":"how does mitmproxy work","questionPath":[]}']),

    # ── Tier 4: Copilot / Bing variants ────────────────────────────────────
    ("bing.com",              "Bing",
     ["/search"],
     ['{"query":"what is AI","conversationId":"abc"}']),

    ("github.com",            "GitHub Copilot",
     ["/github-copilot-chat"],
     ['{"messages":[{"role":"user","content":"fix this bug"}]}']),

    # ── Tier 5: Generic / Custom AIs ──────────────────────────────────────
    ("customai.company.com",  "Custom AI (OpenAI-compatible)",
     ["/v1/chat/completions"],
     ['{"messages":[{"role":"user","content":"test prompt"}],"stream":true}']),

    ("mybot.internal",        "Custom AI (simple prompt)",
     ["/ask"],
     ['{"prompt":"test question","session_id":"abc123"}']),

    ("corp-ai.example.com",   "Custom AI (input field)",
     ["/api/query"],
     ['{"input":"summarize this document","context":"enterprise"}']),

    ("writesonic.com",        "Writesonic",
     ["/api/v2/business/content/chatsonic"],
     ['{"enable_google_results":true,"message":"hello"}']),

    ("kagi.com",              "Kagi",
     ["/assistant"],
     ['{"query":"summarize this page"}']),

    ("inflection.ai",         "Inflection",
     ["/api/v1/completion"],
     ['{"prompt":"hello","model":"inflection-2"}']),
]

# ─── Test runner ──────────────────────────────────────────────────────────────

def test_domain(domain, platform, chat_paths, bodies):
    notes = []

    # 1. Domain detection
    ok, matched_domain, matched_platform = detect_target(domain)
    domain_detected = ok
    if not domain_detected:
        notes.append("Not in Target Websites (add it in Raksha dashboard)")

    # 2. Prompt capture via is_chat_path
    prompt_captured = False
    for path in chat_paths:
        for body in bodies:
            try:
                if is_chat_path(path, domain, body):
                    prompt_captured = True
                    break
            except Exception as e:
                notes.append(f"is_chat_path error: {e}")
        if prompt_captured:
            break

    # Fallback: generic body detection
    if not prompt_captured:
        for body in bodies:
            try:
                stripped = body.lstrip()
                if stripped.startswith("{") or stripped.startswith("["):
                    data = json.loads(body)
                    if _body_has_user_send_payload(data):
                        prompt_captured = True
                        notes.append("Generic body detection (path-independent)")
                        break
            except Exception:
                pass

    if prompt_captured:
        notes.append("Prompt capture: YES")
    else:
        notes.append("Prompt capture: NO - body format not recognized")

    # 3. File upload test (fake PDF)
    fake_pdf = b"%PDF-1.4 fake content " + b"x" * 200
    try:
        upload_detected = is_confident_file_upload(
            fname="testfile.pdf",
            content_type="application/pdf",
            raw_bytes=fake_pdf,
            raw_text="",
            upload_reason="test",
            host=domain,
            path="/upload",
        )
    except Exception as e:
        upload_detected = False
        notes.append(f"Upload test error: {e}")

    notes.append(f"File upload detection: {'YES' if upload_detected else 'NO'}")

    # Status
    if domain_detected and prompt_captured and upload_detected:
        status = "PASS"
    elif domain_detected and prompt_captured:
        status = "PARTIAL"
    elif not domain_detected:
        status = "NOT_ADDED"
    else:
        status = "FAIL"

    return {
        "domain": domain,
        "platform": platform,
        "domain_detected": domain_detected,
        "prompt_captured": prompt_captured,
        "upload_detected": upload_detected,
        "status": status,
        "notes": notes,
    }


# ─── Run all tests ────────────────────────────────────────────────────────────

print()
print("=" * 90)
print("  Raksha Browser Guard - AI Domain Coverage Test")
print(f"  {datetime.datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
print("=" * 90)
print()

STATUS_ICON = {
    "PASS":      "[PASS    ]",
    "PARTIAL":   "[PARTIAL ]",
    "NOT_ADDED": "[NOT ADD ]",
    "FAIL":      "[FAIL    ]",
}

HEADER = f"{'#':<4} {'Domain':<32} {'Platform':<26} {'Dom':^5} {'Prompt':^7} {'Upload':^7} {'Status':<12}"
print(HEADER)
print("-" * 90)

results = []
pass_count = partial_count = fail_count = not_added_count = 0

for i, args in enumerate(AI_DOMAINS, 1):
    r = test_domain(*args)
    results.append(r)

    d = "YES" if r["domain_detected"] else "NO"
    p = "YES" if r["prompt_captured"] else "NO"
    u = "YES" if r["upload_detected"] else "NO"
    st = STATUS_ICON[r["status"]]

    print(f"{i:<4} {r['domain']:<32} {r['platform']:<26} {d:^5} {p:^7} {u:^7} {st}")

    if r["status"] == "PASS":          pass_count += 1
    elif r["status"] == "PARTIAL":     partial_count += 1
    elif r["status"] == "NOT_ADDED":   not_added_count += 1
    else:                              fail_count += 1

print()
print("=" * 90)
print("  SUMMARY")
print("=" * 90)
print(f"  Total tested : {len(results)}")
print(f"  PASS         : {pass_count}  (domain + prompt + upload all work)")
print(f"  PARTIAL      : {partial_count}  (prompt works, upload detection needs upload path)")
print(f"  NOT_ADDED    : {not_added_count}  (domain not in Target Websites yet)")
print(f"  FAIL         : {fail_count}  (prompt NOT captured - needs fix)")
print()

# Detailed failures
failures = [r for r in results if r["status"] == "FAIL"]
if failures:
    print("DOMAINS NEEDING FIX:")
    print()
    for r in failures:
        print(f"  {r['domain']} ({r['platform']})")
        for note in r["notes"]:
            print(f"    -> {note}")
        print()

not_added = [r for r in results if r["status"] == "NOT_ADDED"]
if not_added:
    print("DOMAINS TO ADD IN TARGET WEBSITES:")
    for r in not_added:
        print(f"  - {r['domain']}   ({r['platform']})")
    print()

# ─── Write files ──────────────────────────────────────────────────────────────

report_path  = os.path.join(_HERE, "ai_domain_coverage_report.txt")
fail_path    = os.path.join(_HERE, "ai_domain_failures.txt")

with open(report_path, "w", encoding="utf-8") as f:
    f.write("Raksha AI Domain Coverage Report\n")
    f.write(f"Generated: {datetime.datetime.now()}\n")
    f.write("=" * 90 + "\n\n")
    f.write(f"{'#':<4} {'Domain':<32} {'Platform':<26} {'Dom':^5} {'Prompt':^7} {'Upload':^7} Status\n")
    f.write("-" * 90 + "\n")
    for i, r in enumerate(results, 1):
        f.write(
            f"{i:<4} {r['domain']:<32} {r['platform']:<26} "
            f"{'YES' if r['domain_detected'] else 'NO':^5} "
            f"{'YES' if r['prompt_captured'] else 'NO':^7} "
            f"{'YES' if r['upload_detected'] else 'NO':^7} "
            f"{r['status']}\n"
        )
    f.write(f"\nSummary: PASS={pass_count} PARTIAL={partial_count} NOT_ADDED={not_added_count} FAIL={fail_count}\n")
    f.write("\nDetailed Notes:\n")
    for r in results:
        f.write(f"\n[{r['status']}] {r['domain']} ({r['platform']})\n")
        for note in r["notes"]:
            f.write(f"  {note}\n")

with open(fail_path, "w", encoding="utf-8") as f:
    f.write("Domains with prompt capture FAIL:\n\n")
    for r in results:
        if r["status"] == "FAIL":
            f.write(f"{r['domain']} — {r['platform']}\n")
            for note in r["notes"]:
                f.write(f"  {note}\n")
            f.write("\n")

print(f"Reports saved:")
print(f"  {report_path}")
print(f"  {fail_path}")
print()
print("HOW TO FIX FAIL DOMAINS:")
print("  1. Add domain in Raksha dashboard -> Browser AI -> Target Websites")
print("  2. If prompt still not captured, check the AI site's POST request body format")
print("     and share it — a specific detection rule can be added.")
print("  3. gRPC/binary protocol AIs cannot be intercepted via HTTP proxy.")
print()
