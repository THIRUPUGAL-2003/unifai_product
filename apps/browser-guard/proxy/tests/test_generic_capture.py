#!/usr/bin/env python3
"""End-to-end capture harness: real mitmproxy flows -> addon -> fake backend.

Checks, for many AI site body shapes (known + unknown domains):
  * every chat prompt reaches /intercept (text, numbers, symbols, emoji, Tamil)
  * file uploads (single, multipart-many, separate uploads, no filename=, CDN PUT)
    reach /intercept-file with the right file name on Send
  * voice transcripts reach /intercept

Run:  python tests/test_generic_capture.py        (exit 1 on any failure)
"""
from __future__ import annotations

import io
import json
import os
import re
import sys
import threading
import time
import uuid
from contextlib import redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import quote

HERE = Path(__file__).resolve().parent
PARTS = HERE.parent / "gateway_proxy_parts"

# ── fake backend ─────────────────────────────────────────────────────────────
TARGETS: list[dict] = []
RECORDS: list[dict] = []
_rec_lock = threading.Lock()


def _multipart_fields(body: bytes, ctype: str) -> dict:
    m = re.search(r"boundary=([^;]+)", ctype or "")
    if not m:
        return {}
    b = ("--" + m.group(1).strip('"')).encode()
    out = {}
    for part in body.split(b):
        if b"\r\n\r\n" not in part:
            continue
        head, val = part.split(b"\r\n\r\n", 1)
        nm = re.search(rb'name="([^"]+)"', head)
        if not nm:
            continue
        name = nm.group(1).decode()
        if name == "file":
            out["_has_file_bytes"] = True
            continue
        out[name] = val.rstrip(b"\r\n").decode("utf-8", "replace")
    return out


class _Backend(BaseHTTPRequestHandler):
    def log_message(self, *a):  # silence
        pass

    def _json(self, obj, code=200):
        data = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        p = self.path.split("?", 1)[0]
        if p.endswith("/targets"):
            return self._json({"targets": TARGETS})
        if p.endswith("/rules"):
            return self._json({"rules": []})
        if p.endswith("/controls"):
            return self._json({"controls": {"enabled": True, "block_upload": False}})
        return self._json({})

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n) if n else b""
        p = self.path.split("?", 1)[0]
        rec = {"path": p}
        if p.endswith("/intercept-file"):
            rec.update(_multipart_fields(body, self.headers.get("Content-Type", "")))
        elif p.endswith("/intercept"):
            try:
                rec.update(json.loads(body.decode("utf-8")))
            except Exception:
                rec["raw"] = body[:2000].decode("utf-8", "replace")
        else:
            return self._json({"ok": True})
        with _rec_lock:
            RECORDS.append(rec)
        return self._json({"allowed": True, "action": "Allowed"})

    do_PUT = do_POST


def start_backend() -> str:
    srv = ThreadingHTTPServer(("127.0.0.1", 0), _Backend)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return f"http://127.0.0.1:{srv.server_address[1]}"


# ── load proxy parts into one namespace (same as browser_ai_proxy.py) ───────
def load_parts() -> dict:
    ns: dict = {"__name__": "gateway_proxy_under_test"}
    names = [
        ln.strip().lstrip("\ufeff")
        for ln in (PARTS / "MANIFEST.txt").read_text(encoding="utf-8-sig").splitlines()
        if ln.strip() and not ln.strip().startswith("#")
    ]
    for name in names:
        path = PARTS / name
        exec(compile(path.read_text(encoding="utf-8"), str(path), "exec"), ns)
    return ns


# ── flow builders ────────────────────────────────────────────────────────────
from mitmproxy.test import tflow, tutils  # noqa: E402
from mitmproxy import websocket  # noqa: E402
from mitmproxy import http as mhttp  # noqa: E402


def http_flow(host, path, body: bytes, ctype="application/json", method="POST", headers=None):
    hdrs = [(b"host", host.encode()), (b"user-agent", b"Mozilla/5.0 (Windows NT 10.0) Chrome/130")]
    if ctype:
        hdrs.append((b"content-type", ctype.encode()))
    for k, v in (headers or {}).items():
        hdrs.append((k.lower().encode(), v.encode()))
    req = tutils.treq(
        host=host, port=443, scheme=b"https", authority=host.encode(),
        path=path.encode(), method=method.encode(), headers=mhttp.Headers(hdrs),
        content=body,
    )
    f = tflow.tflow(req=req)
    f.response = None
    return f


def ws_flow(host, path, frame: str):
    f = tflow.twebsocketflow(messages=False)
    f.request.host = host
    f.request.authority = host
    f.request.path = path
    f.request.headers["host"] = host
    f.websocket.messages.append(websocket.WebSocketMessage(websocket.Opcode.TEXT, True, frame.encode()))
    return f


def multipart(fields: list[tuple]) -> tuple[bytes, str]:
    """fields: (name, value) or (name, filename|None, ctype, bytes)."""
    bnd = "----WebKitFormBoundary" + uuid.uuid4().hex[:16]
    out = []
    for f in fields:
        if len(f) == 2:
            out.append(f"--{bnd}\r\nContent-Disposition: form-data; name=\"{f[0]}\"\r\n\r\n{f[1]}\r\n".encode())
        else:
            name, fname, ct, data = f
            disp = f'form-data; name="{name}"' + (f'; filename="{fname}"' if fname is not None else "")
            out.append(f"--{bnd}\r\nContent-Disposition: {disp}\r\nContent-Type: {ct}\r\n\r\n".encode() + data + b"\r\n")
    out.append(f"--{bnd}--\r\n".encode())
    return b"".join(out), f"multipart/form-data; boundary={bnd}"


def U():
    return str(uuid.uuid4())


# ── sample file payloads ─────────────────────────────────────────────────────
PDF = (b"%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
       b"3 0 obj<</Type/Page/Parent 2 0 R/Contents 4 0 R>>endobj\n4 0 obj<</Length 44>>stream\nBT /F1 12 Tf (Salary 50000 confidential) Tj ET\n"
       b"endstream endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n")
TXT = b"Employee list\nRavi 9080578529\nconfidential project zeus\n" * 3
CSV = b"name,phone\nRavi,9080578529\nAnu,9876543210\n"
PNG = b"\x89PNG\r\n\x1a\n" + b"\x00\x00\x00\rIHDR" + b"\x00" * 200
WEBM = b"\x1aE\xdf\xa3" + b"\x00" * 600


# ── prompt body shapes (host, builder) ───────────────────────────────────────
def chatgpt(p):
    return "chatgpt.com", "/backend-api/f/conversation", json.dumps({
        "action": "next", "messages": [{"id": U(), "author": {"role": "user"}, "create_time": time.time(),
                                        "content": {"content_type": "text", "parts": [p]}, "metadata": {}}],
        "parent_message_id": "client-created-root", "model": "auto", "timezone_offset_min": -330,
        "conversation_mode": {"kind": "primary_assistant"}, "supports_buffering": True}, ensure_ascii=False)


def claude(p):
    return "claude.ai", f"/api/organizations/{U()}/chat_conversations/{U()}/completion", json.dumps({
        "prompt": p, "parent_message_uuid": U(), "timezone": "Asia/Calcutta", "locale": "en-US",
        "personalized_styles": [{"type": "default", "key": "Default", "name": "Normal"}],
        "tools": [], "attachments": [], "files": [], "sync_sources": [], "rendering_mode": "messages"}, ensure_ascii=False)


def gemini(p):
    inner = json.dumps([[p, 0, None, None, None, None, 0], ["en"], ["", "", ""]], ensure_ascii=False)
    freq = json.dumps([None, inner], ensure_ascii=False)
    body = "f.req=" + quote(freq) + "&at=AJvLN6M%3A1700000000000&"
    return ("gemini.google.com",
            "/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=boq_assistant&_reqid=1234&rt=c",
            body, "application/x-www-form-urlencoded;charset=UTF-8")


def deepseek(p):
    return "chat.deepseek.com", "/api/v0/chat/completion", json.dumps({
        "chat_session_id": U(), "parent_message_id": None, "prompt": p, "ref_file_ids": [],
        "thinking_enabled": False, "search_enabled": False}, ensure_ascii=False)


def mistral(p):
    return "chat.mistral.ai", "/api/chat", json.dumps({
        "chatId": U(), "mode": "append", "model": "mistral-large-latest",
        "messageInput": [{"type": "text", "text": p}], "messageId": U(), "features": ["beta-websearch"]}, ensure_ascii=False)


def perplexity(p):
    return "www.perplexity.ai", "/rest/sse/perplexity_ask", json.dumps({
        "params": {"attachments": [], "language": "en-US", "timezone": "Asia/Calcutta", "search_focus": "internet",
                   "sources": ["web"], "mode": "concise", "model_preference": "turbo", "frontend_uuid": U()},
        "query_str": p}, ensure_ascii=False)


def grok(p):
    return "grok.com", "/rest/app-chat/conversations/new", json.dumps({
        "temporary": False, "modelName": "grok-3", "message": p, "fileAttachments": [],
        "imageAttachments": [], "disableSearch": False}, ensure_ascii=False)


def poe(p):
    return "poe.com", "/api/gql_POST", json.dumps({
        "queryName": "sendMessageMutation", "variables": {"chatId": None, "bot": "Assistant", "query": p,
                                                          "source": {"sourceType": "chat_input"}, "clientNonce": U()},
        "extensions": {"hash": "abc123"}}, ensure_ascii=False)


def qwen(p):
    return "chat.qwen.ai", "/api/chat/completions", json.dumps({
        "stream": True, "model": "qwen-max", "chat_type": "t2t",
        "messages": [{"role": "user", "content": p, "chat_type": "t2t"}]}, ensure_ascii=False)


def custom_input(p):
    return "myai.example", "/api/run", json.dumps({"input": p, "session": "abc"}, ensure_ascii=False)


def custom_question(p):
    return "newchat.io", "/v2/turn", json.dumps({"question": p, "history": []}, ensure_ascii=False)


def custom_nested(p):
    return "bot.unknownai.dev", "/graphql", json.dumps({
        "operationName": "CreateTurn", "variables": {"input": {"conversationId": U(), "userMessage": {"body": p}}},
        "query": "mutation CreateTurn($input: TurnInput!) { createTurn(input: $input) { id } }"}, ensure_ascii=False)


def custom_textplain(p):
    return "plaintext.ai", "/send", p, "text/plain;charset=UTF-8"


def custom_form(p):
    return "formchat.ai", "/ask", "q=" + quote(p) + "&lang=en", "application/x-www-form-urlencoded"


def cross_domain(p):
    # Admin added only "crossai.example"; the chat API lives on another registrable domain.
    return ("api.crossai-cdn.net", "/v1/chat", json.dumps({"messages": [{"role": "user", "content": p}]}, ensure_ascii=False),
            "application/json", {"origin": "https://crossai.example", "referer": "https://crossai.example/c/1"})


PROMPT_SHAPES = [chatgpt, claude, gemini, deepseek, mistral, perplexity, grok, poe, qwen,
                 custom_input, custom_question, custom_nested, custom_textplain, custom_form, cross_domain]

PROMPTS = ["hello world", "9080578529", "!!!", "👍", "?", "hi-there", "let_me_go", "XYZ789",
           "thinking", "😀🔥", "₹500", "42", "a", "வணக்கம் நண்பா", "what is AI"]

WS_SHAPES = [
    ("copilot.microsoft.com", "/c/api/chat?api-version=2",
     lambda p: json.dumps({"event": "send", "conversationId": U(), "content": [{"type": "text", "text": p}], "mode": "chat"}, ensure_ascii=False)),
    ("wschat.example", "/socket",
     lambda p: json.dumps({"type": "user_message", "data": {"text": p}}, ensure_ascii=False)),
]

TARGET_DOMAINS = [
    "chatgpt.com", "claude.ai", "gemini.google.com", "chat.deepseek.com", "chat.mistral.ai", "perplexity.ai",
    "grok.com", "poe.com", "chat.qwen.ai", "myai.example", "newchat.io", "bot.unknownai.dev", "plaintext.ai",
    "formchat.ai", "crossai.example", "copilot.microsoft.com", "wschat.example",
    "files1.example", "files2.example", "files3.example", "files4.example", "files5.example", "files6.example",
    "files7.example", "voice1.example", "voice2.example",
]


# ── runner ───────────────────────────────────────────────────────────────────
def snapshot() -> int:
    with _rec_lock:
        return len(RECORDS)


def records_since(i: int) -> list[dict]:
    with _rec_lock:
        return list(RECORDS[i:])


def wait_for(pred, start: int, timeout=4.0) -> bool:
    end = time.time() + timeout
    while time.time() < end:
        if pred(records_since(start)):
            return True
        time.sleep(0.05)
    return pred(records_since(start))


def prompt_seen(expected: str):
    exp = expected.strip()
    return lambda recs: any((r.get("prompt") or "").strip() == exp for r in recs if r["path"].endswith("/intercept"))


def file_names(recs) -> set[str]:
    out = set()
    for r in recs:
        if r.get("file_name"):
            for n in str(r["file_name"]).split(","):
                out.add(n.strip())
        md = r.get("metadata")
        if isinstance(md, str):
            try:
                md = json.loads(md)
            except Exception:
                md = {}
        if isinstance(md, dict):
            for k in ("file_name", "filename", "file_names"):
                v = md.get(k)
                if isinstance(v, str):
                    out.update(x.strip() for x in v.split(","))
                elif isinstance(v, list):
                    out.update(str(x).strip() for x in v)
    return out


def files_seen(names: list[str]):
    return lambda recs: set(names) <= file_names(recs)


def run(addon, verbose: bool) -> list[tuple[str, bool, str]]:
    results = []

    def do_http(host, path, body, ctype="application/json", method="POST", headers=None):
        if isinstance(body, str):
            body = body.encode("utf-8")
        f = http_flow(host, path, body, ctype, method, headers)
        buf = io.StringIO()
        err = ""
        try:
            with redirect_stdout(buf):
                addon.request(f)
        except Exception as e:  # crash inside the hook = capture lost in production
            err = f"CRASH {type(e).__name__}: {e}"
        if verbose:
            sys.stderr.write(buf.getvalue())
        return err

    def do_ws(host, path, frame):
        f = ws_flow(host, path, frame)
        buf = io.StringIO()
        err = ""
        try:
            with redirect_stdout(buf):
                addon.websocket_message(f)
        except Exception as e:
            err = f"CRASH {type(e).__name__}: {e}"
        if verbose:
            sys.stderr.write(buf.getvalue())
        return err

    # 1. prompts over HTTP
    for shape in PROMPT_SHAPES:
        for p in PROMPTS:
            spec = shape(p)
            host, path, body = spec[0], spec[1], spec[2]
            ctype = spec[3] if len(spec) > 3 else "application/json"
            hdrs = spec[4] if len(spec) > 4 else None
            start = snapshot()
            err = do_http(host, path, body, ctype, headers=hdrs)
            ok = wait_for(prompt_seen(p), start, timeout=2.5)
            got = [r.get("prompt") for r in records_since(start) if r["path"].endswith("/intercept")]
            results.append((f"prompt {shape.__name__:16} {p!r}", ok, err or ("" if ok else f"got={got}")))

    # 2. prompts over WebSocket
    for host, path, build in WS_SHAPES:
        for p in PROMPTS:
            start = snapshot()
            err = do_ws(host, path, build(p))
            ok = wait_for(prompt_seen(p), start, timeout=2.5)
            got = [r.get("prompt") for r in records_since(start) if r["path"].endswith("/intercept")]
            results.append((f"ws     {host:16} {p!r}", ok, err or ("" if ok else f"got={got}")))

    # 3. files — upload then Send, name must reach /intercept-file
    def file_case(label, steps, names):
        start = snapshot()
        errs = [e for e in (s() for s in steps) if e]
        ok = wait_for(files_seen(names), start, timeout=5)
        got = sorted(file_names(records_since(start)))
        results.append((f"file   {label}", ok and not errs, "; ".join(errs) or ("" if ok else f"want={names} got={got}")))

    # 3a single multipart upload, then Send referencing file id
    b, ct = multipart([("file", "report_q3.pdf", "application/pdf", PDF)])
    file_case("single pdf (custom site)", [
        lambda: do_http("files1.example", "/api/upload", b, ct),
        lambda: do_http("files1.example", "/api/chat", json.dumps({"message": "summarize this", "files": ["file_1"]})),
    ], ["report_q3.pdf"])

    # 3b multi files in one multipart
    b, ct = multipart([("files", "staff.txt", "text/plain", TXT), ("files", "phones.csv", "text/csv", CSV)])
    file_case("2 files one multipart", [
        lambda: do_http("files2.example", "/api/upload", b, ct),
        lambda: do_http("files2.example", "/api/chat", json.dumps({"message": "compare", "attachments": [{"id": "a"}, {"id": "b"}]})),
    ], ["staff.txt", "phones.csv"])

    # 3c two separate uploads, one Send
    b1, ct1 = multipart([("file", "first.txt", "text/plain", TXT)])
    b2, ct2 = multipart([("file", "second.csv", "text/csv", CSV)])
    file_case("2 separate uploads", [
        lambda: do_http("files3.example", "/api/files", b1, ct1),
        lambda: do_http("files3.example", "/api/files", b2, ct2),
        lambda: do_http("files3.example", "/api/chat", json.dumps({"message": "read both", "attachments": [{"id": "x1"}, {"id": "x2"}]})),
    ], ["first.txt", "second.csv"])

    # 3d multipart without filename= (name in a sibling field)
    b, ct = multipart([("fileName", "budget_2026.pdf"), ("file", None, "application/pdf", PDF)])
    file_case("no filename= multipart", [
        lambda: do_http("files4.example", "/api/upload", b, ct),
        lambda: do_http("files4.example", "/api/chat", json.dumps({"message": "check budget", "files": ["f9"]})),
    ], ["budget_2026.pdf"])

    # 3e file + prompt in one POST (no separate upload)
    b, ct = multipart([("prompt", "is this contract safe"), ("file", "contract.pdf", "application/pdf", PDF)])
    file_case("file+prompt same POST", [
        lambda: do_http("files5.example", "/api/chat", b, ct),
    ], ["contract.pdf"])

    # 3f ChatGPT-style: create-file JSON, PUT raw bytes to CDN (other domain), Send with attachments
    fid = "file-" + uuid.uuid4().hex[:20]
    file_case("ChatGPT create+PUT+Send", [
        lambda: do_http("chatgpt.com", "/backend-api/files", json.dumps({"file_name": "payroll.pdf", "file_size": len(PDF), "use_case": "my_files"})),
        lambda: do_http("files.oaiusercontent.com", f"/{fid}?se=2026&sp=cw&sig=abc", PDF, "application/pdf", "PUT",
                        {"origin": "https://chatgpt.com", "referer": "https://chatgpt.com/", "x-ms-blob-type": "BlockBlob"}),
        lambda: do_http("chatgpt.com", "/backend-api/f/conversation", json.dumps({
            "action": "next", "messages": [{"id": U(), "author": {"role": "user"},
                                            "content": {"content_type": "multimodal_text", "parts": ["read file"]},
                                            "metadata": {"attachments": [{"id": fid, "name": "payroll.pdf", "size": len(PDF), "mime_type": "application/pdf"}]}}],
            "parent_message_id": "client-created-root", "model": "auto"})),
    ], ["payroll.pdf"])

    # 3g Claude-style upload then completion with files[]
    b, ct = multipart([("file", "memo.txt", "text/plain", TXT)])
    org = U()
    file_case("Claude upload+completion", [
        lambda: do_http("claude.ai", f"/api/{org}/upload", b, ct),
        lambda: do_http("claude.ai", f"/api/organizations/{org}/chat_conversations/{U()}/completion", json.dumps({
            "prompt": "explain memo", "parent_message_uuid": U(), "timezone": "Asia/Calcutta",
            "attachments": [], "files": [U()], "rendering_mode": "messages"})),
    ], ["memo.txt"])

    # 3h image + docx in one multipart on custom site
    DOCX = b"PK\x03\x04" + b"\x00" * 30 + b"word/document.xml" + b"\x00" * 600
    b, ct = multipart([("file", "photo.png", "image/png", PNG), ("file", "plan.docx", "application/octet-stream", DOCX)])
    file_case("png+docx multipart", [
        lambda: do_http("files6.example", "/upload", b, ct),
        lambda: do_http("files6.example", "/api/message", json.dumps({"text": "see attached", "attachments": [{"id": 1}, {"id": 2}]})),
    ], ["photo.png", "plan.docx"])

    # 3i three files, three uploads, Send — every file gets its own name
    bs = [multipart([("file", n, "text/plain", TXT)]) for n in ("one.txt", "two.txt", "three.txt")]
    file_case("3 uploads distinct names", [
        lambda: do_http("files7.example", "/api/upload", *bs[0]),
        lambda: do_http("files7.example", "/api/upload", *bs[1]),
        lambda: do_http("files7.example", "/api/upload", *bs[2]),
        lambda: do_http("files7.example", "/api/chat", json.dumps({"message": "merge", "files": ["1", "2", "3"]})),
    ], ["one.txt", "two.txt", "three.txt"])

    # 4. voice
    start = snapshot()
    err = do_http("voice1.example", "/api/voice", json.dumps({"transcript": "call ravi 9080578529 now", "lang": "en"}))
    ok = wait_for(prompt_seen("call ravi 9080578529 now"), start)
    results.append(("voice  JSON transcript", ok and not err, err or ("" if ok else str([r.get("prompt") for r in records_since(start)]))))

    start = snapshot()
    b, ct = multipart([("file", "blob", "audio/webm", WEBM)])
    e1 = do_http("voice2.example", "/api/transcribe", b, ct)
    e2 = do_http("voice2.example", "/api/chat", json.dumps({"message": "spoken words here"}))
    ok = wait_for(prompt_seen("spoken words here"), start)
    results.append(("voice  audio upload then Send", ok and not (e1 or e2), e1 or e2 or ("" if ok else str([r.get("prompt") for r in records_since(start)]))))

    return results


def unit_checks(ns) -> list[tuple[str, bool, str]]:
    out = []
    kind = ns.get("_classify_upload_kind")
    samples = {
        "heic": (b"\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic" + b"\x00" * 64, "IMG_1.heic", "image/heic"),
        "pdf_as_octet": (PDF, "upload", "application/octet-stream"),
        "png_mislabeled": (PNG, "pic", "application/octet-stream"),
        "m4a": (b"\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00M4A mp42isom" + b"\x00" * 64, "voice.m4a", "audio/mp4"),
    }
    expect = {"heic": "image", "pdf_as_octet": "pdf", "png_mislabeled": "image", "m4a": "audio"}
    if kind is None:
        return [("unit   _classify_upload_kind exists", False, "missing")]
    import inspect
    params = list(inspect.signature(kind).parameters)
    for k, (data, name, ct) in samples.items():
        try:
            kw = {}
            for p in params:
                pl = p.lower()
                if "byte" in pl or pl in ("data", "raw", "payload", "content"):
                    kw[p] = data
                elif "name" in pl:
                    kw[p] = name
                elif "type" in pl or pl in ("ct", "mime"):
                    kw[p] = ct
            got = str(kind(**kw))
        except Exception as e:
            got = f"CRASH {e}"
        ok = expect[k] in got.lower()
        out.append((f"unit   type {k}", ok, "" if ok else f"got={got}"))
    return out


def main() -> int:
    verbose = "-v" in sys.argv
    url = start_backend()
    os.environ["GATEWAY_BACKEND_URL"] = url
    os.environ.setdefault("GATEWAY_EVAL_TIMEOUT", "5")
    TARGETS.extend({"id": str(i), "domain": d, "monitored": True, "platform_name": d} for i, d in enumerate(TARGET_DOMAINS))
    buf = io.StringIO()
    with redirect_stdout(buf):
        ns = load_parts()
        ns["_apply_targets_from_data"]({"targets": TARGETS})
        ns["get_control_settings"](force_network=True)
        ns["get_guard_rules"](force_network=True)
        addon = ns["BrowserAIInterceptor"]()
    results = unit_checks(ns) + run(addon, verbose)
    fails = [r for r in results if not r[1]]
    for name, ok, why in results:
        if not ok or verbose:
            print(f"{'PASS' if ok else 'FAIL'}  {name}  {why}")
    print(f"\n{len(results) - len(fails)}/{len(results)} passed")
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
