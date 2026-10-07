# Part of Gateway browser_ai_proxy
import re
import json
import os
import urllib.parse

def _is_anthropic_messages_api_shape(path: str, body: str) -> bool:
    """Detect Claude / Anthropic chat submit from request path or JSON body — not hostname."""
    path_l = (path or "").lower()
    if _path_has_ignore_pattern(path_l):
        return False
    if any(x in path_l for x in ("/v1/messages", "chat_conversations", "append_message", "/completion", "performaction")):
        return True
    if not body or not body.lstrip().startswith("{"):
        return False
    try:
        data = json.loads(body)
    except Exception:
        return False
    if not isinstance(data, dict):
        return False
    if "max_tokens" in data and isinstance(data.get("messages"), list):
        return True
    if isinstance(data.get("prompt"), str) and str(data.get("prompt", "")).strip():
        return True
    return False


def _is_gateway_inject_frame(body: str) -> bool:
    """Frames Guard itself injected — never treat as a user Send."""
    b = (body or "").lower()
    return "gateway-reply" in b or '"messageid":"gateway-reply"' in b


def _is_persistent_chat_websocket(path: str) -> bool:
    """Long-lived chat sockets whose URL stays the same for pings AND Sends."""
    path_l = (path or "").lower()
    return any(
        m in path_l
        for m in (
            "/c/api/chat",
            "chathub",
            "sydney",
            "chatoverstream",
            "turing/conversation",
        )
    )


def _copilot_frame_is_user_send(body: str) -> bool:
    """True only for a finished Copilot/Bing/Edge user Send frame — not attach-ack or ping."""
    if not body or not str(body).strip():
        return False
    if _is_gateway_inject_frame(body):
        return False
    if is_event_sync_noise_content(body):
        return False
    bl = body.lower()
    if '"event":"send"' in bl or '"event": "send"' in bl:
        return True
    if '"target":"chat"' in bl or '"target": "chat"' in bl:
        return True
    if ('"type":4' in bl or '"type": 4' in bl) and "chat" in bl:
        return True
    data = _loads_json_maybe_signalr(body)
    if isinstance(data, dict) and _body_has_user_send_payload(data):
        return True
    if isinstance(data, list) and any(
        isinstance(item, dict) and _body_has_user_send_payload(item) for item in data
    ):
        return True
    # File-only Send (empty caption) still has a chat envelope + attachment refs.
    if chat_carries_attachment(body) and any(
        k in bl
        for k in (
            '"messagetype":"chat"',
            '"role":"user"',
            '"author":"user"',
            '"conversationid"',
            '"conversation_id"',
        )
    ):
        return True
    return False


def is_event_sync_noise_content(content: str) -> bool:
    """Copilot SignalR / Sydney frames that are not a user chat submit."""
    if not content:
        return True
    cl = content.lower()
    if '"target":"metrics"' in cl or '"target": "metrics"' in cl:
        return True
    if '"type":6' in cl or '"type": 6' in cl:
        return True
    if '"event":"typing"' in cl or '"event": "typing"' in cl:
        return True
    if '"event":"ping"' in cl or '"event": "ping"' in cl:
        return True
    if "messagetype\":\"internal" in cl or "messagetype\": \"internal" in cl:
        return True
    if "\x1e" in content or '"target":' in cl or '"arguments":' in cl:
        if '"event":"send"' not in cl and '"event": "send"' not in cl:
            if '"target":"chat"' not in cl and '"target": "chat"' not in cl:
                if ('"type":4' not in cl and '"type": 4' not in cl) or "chat" not in cl:
                    if len(content) > 40 and _is_opaque_wire_blob(content.strip()):
                        return True
    return False


def _parse_signalr_frames(text: str) -> list:
    """Split SignalR JSON frames (0x1e record separator)."""
    out = []
    for part in re.split(r"\x1e", text or ""):
        part = part.strip()
        if not part or part[0] not in "{[":
            continue
        try:
            out.append(json.loads(part))
        except Exception:
            continue
    return out


def _loads_json_maybe_signalr(text: str):
    """Parse JSON, including Copilot/Bing SignalR frames terminated by 0x1e."""
    if not text or not isinstance(text, str):
        return None
    raw = text.strip()
    if not raw:
        return None
    try:
        return json.loads(raw)
    except Exception:
        pass
    if "\x1e" not in raw:
        return None
    frames = _parse_signalr_frames(raw)
    for frame in reversed(frames):
        if isinstance(frame, (dict, list)):
            return frame
    try:
        return json.loads(raw.replace("\x1e", "").strip())
    except Exception:
        return None


def websocket_frame_text(msg) -> str:
    """Decode a mitmproxy WebSocketMessage (text frame or binary UTF-8/JSON)."""
    text = ""
    try:
        text = getattr(msg, "text", None) or ""
    except Exception:
        text = ""
    if isinstance(text, str) and text.strip():
        return text
    content = getattr(msg, "content", None) or b""
    if isinstance(content, str):
        return content
    if isinstance(content, (bytes, bytearray)) and content:
        try:
            return bytes(content).decode("utf-8")
        except Exception:
            return bytes(content).decode("utf-8", errors="ignore")
    return text if isinstance(text, str) else ""


def extract_event_send_prompt(content: str) -> str:
    """Extract user-typed text from Copilot / Bing Sydney / Edge / M365 SignalR payloads."""

    def _pick_text(val: str) -> str:
        if not val or not isinstance(val, str):
            return ""
        got = val.strip()
        if not got or _is_opaque_wire_blob(got):
            return ""
        if looks_like_user_prompt(got):
            return got
        return ""

    def _from_content_field(val) -> str:
        if isinstance(val, str):
            return _pick_text(val)
        if isinstance(val, list):
            got = _parts_to_text(val)
            return got.strip() if got and looks_like_user_prompt(got) else ""
        if isinstance(val, dict):
            if isinstance(val.get("text"), str):
                got = _pick_text(val.get("text") or "")
                if got:
                    return got
            got = _parts_to_text(val.get("parts") or [val])
            return got.strip() if got and looks_like_user_prompt(got) else ""
        return ""

    def _from_message_dict(msg: dict) -> str:
        if not isinstance(msg, dict):
            return ""
        author = str(msg.get("author") or msg.get("role") or "").lower()
        if author and author not in ("user", "human", "customer", "client", "sender"):
            return ""
        got = _from_content_field(msg.get("content"))
        if got:
            return got
        for key in ("text", "hiddenText", "rawText", "input", "query", "prompt", "utterance"):
            got = _pick_text(msg.get(key) or "")
            if got:
                return got
        return ""

    def _from_obj(obj) -> str:
        if isinstance(obj, str):
            return _pick_text(obj)
        if not isinstance(obj, dict):
            return ""

        # SignalR StreamInvocation: type 4, target chat
        if obj.get("type") == 4 and str(obj.get("target", "")).lower() == "chat":
            for arg in obj.get("arguments") or []:
                if not isinstance(arg, dict):
                    continue
                got = _from_message_dict(arg.get("message") or {})
                if got:
                    return got
                got = _from_content_field(arg.get("content"))
                if got:
                    return got
                for key in ("text", "query", "prompt", "rawUserQuery", "utterance", "userMessage"):
                    got = _pick_text(arg.get(key) or "")
                    if got:
                        return got

        event = str(obj.get("event", "")).lower()
        if event in ("send", "message", "chat"):
            got = _from_message_dict(obj.get("message") or {})
            if got:
                return got
            got = _from_content_field(obj.get("content"))
            if got:
                return got
            for key in ("text", "query", "prompt", "rawUserQuery", "utterance", "userMessage"):
                got = _pick_text(obj.get(key) or "")
                if got:
                    return got
            return ""

        got = _from_message_dict(obj.get("message") or {})
        if got:
            return got
        got = _from_content_field(obj.get("content"))
        if got:
            return got
        for key in ("text", "query", "prompt", "rawUserQuery", "utterance", "userMessage", "input"):
            got = _pick_text(obj.get(key) or "")
            if got:
                return got
        for nest in ("arguments", "params", "payload", "data", "body", "request"):
            nested = obj.get(nest)
            if isinstance(nested, list):
                for item in nested:
                    got = _from_obj(item)
                    if got:
                        return got
            elif isinstance(nested, dict):
                got = _from_obj(nested)
                if got:
                    return got
        return ""

    if not content:
        return ""

    if "\x1e" in content:
        for frame in reversed(_parse_signalr_frames(content)):
            got = _from_obj(frame)
            if got:
                return got

    try:
        data = json.loads(content)
        got = _from_obj(data)
        if got:
            return got
    except Exception:
        pass

    for line in (content or "").splitlines():
        line = line.strip()
        if not line or line[0] not in "{[":
            continue
        try:
            got = _from_obj(json.loads(line))
            if got:
                return got
        except Exception:
            continue
    return ""


def _is_ai_chrome_url(text: str) -> bool:
    """True when the string is a Gemini/Bard/ChatGPT page URL, not typed chat."""
    t = (text or "").strip()
    if not re.match(r"^https?://", t, re.I):
        return False
    try:
        u = urllib.parse.urlparse(t)
    except Exception:
        return False
    host = (u.hostname or "").lower()
    path = (u.path or "").lower()
    query = (u.query or "").lower()
    # Page/navigation URLs on an admin-monitored host — not typed chat text.
    if detect_target(host)[0] and (path in ("", "/", "/app") or "hl=" in query):
        return True
    return False


_FILE_EXTENSION_RE = re.compile(
    r"\.(?:pdf|docx?|xlsx?|pptx?|csv|txt|png|jpe?g|gif|webp|zip|rar|7z|"
    r"mp3|mp4|wav|m4a|mov|avi|json|xml|html?|md|rtf|odt|ods|ppt)(?:\s|$)",
    re.IGNORECASE,
)


def _looks_like_filename_only(text: str) -> bool:
    """True when extracted text is only an attachment filename — not user chat."""
    t = (text or "").strip()
    if not t or " " in t or "\n" in t or len(t) > 240:
        return False
    # Numbers (integers or decimals like 3.14, 100.5) are valid prompts, not filenames
    try:
        float(t)
        return False
    except ValueError:
        pass
    if not re.search(r"\.[a-z0-9]{2,8}$", t, re.IGNORECASE):
        return False
    if _FILE_EXTENSION_RE.search(t):
        return True
    return bool(re.fullmatch(r"[\w\-.]+\.[a-z0-9]{2,8}", t, re.IGNORECASE))


def _send_carries_attachment(raw_text: str) -> bool:
    """True when this chat Send references an uploaded/attached file."""
    return bool(
        chat_carries_attachment(raw_text)
        or messages_parts_carries_file(raw_text)
        or bool(extract_attachment_filename_from_send(raw_text))
    )


def looks_like_user_prompt(text: str) -> bool:
    """
    Accept ANY user-typed prompt: 1 char to 100k+, any language (Tamil, Arabic,
    Chinese, Hindi, Japanese, Korean, English, any Unicode), numbers, symbols, code.
    Reject ONLY true protocol junk: RPC tokens, base64 blobs, multipart headers.
    """
    if not text or not isinstance(text, str):
        return False
    t = text.strip()
    if len(t) < 1:
        return False

    # ── Non-Latin / Unicode scripts → ALWAYS a user prompt ──────────────────
    # Tamil, Arabic, Chinese, Hindi, Japanese, Korean, Russian, Greek, Hebrew,
    # Thai, Devanagari, Bengali, Telugu, Kannada, Malayalam, Gujarati, Punjabi,
    # and ALL other non-ASCII Unicode scripts. Never filter by language.
    if any(ord(c) > 127 for c in t[:80]):
        # Only reject actual mojibake (replacement chars ≥8% of content)
        if "\ufffd" in t and t.count("\ufffd") / max(len(t), 1) >= 0.08:
            return False
        # Reject raw multipart headers even in non-ASCII
        low_head = t[:80].lower()
        if "webkitformboundary" in low_head or "content-disposition: form-data" in t[:200].lower():
            return False
        return True  # All other non-ASCII → always a real user prompt

    # Never treat multipart / raw HTTP file bodies as chat prompts
    low_head = t[:80].lower()
    if (
        "webkitformboundary" in low_head
        or t.startswith("------")
        or "content-disposition: form-data" in t[:500].lower()
        or "multipart/form-data" in t[:200].lower()
    ):
        return False
    if _is_ai_chrome_url(t):
        return False
    if _is_google_wire_blob(t):
        return False
    if (
        t.startswith(("[null,", '[[["', "[[[", "[[null,", '["[["', '["[', '["contrib', '["/contrib'))
        or "f.req=" in t
        or "/contrib service" in t
        or (t.startswith('{"type":"action"') and "_dd" in t)
        or (t.startswith("{") and '"_dd":' in t)
        or bool(re.match(r'^\d+(?:/[^,]*,\s*)?[\[\{]', t))
    ):
        return False
    if any(rpc in t for rpc in ("xyhAld", "umJEY", "k06x8e", "wrb.fr", "batchexecute", "GmailHttp")):
        return False
    if _is_opaque_wire_blob(t):
        return False
    if _is_internal_wire_text(t):
        return False
    # Claude Connect-RPC / protobuf crumbs must never become Prompt Logs mid-send.
    if _is_claude_wire_noise(t):
        return False
    if "what would you like to do with this file?" in t.lower():
        return False
    if t.startswith("gAAAA") or '"p":"gAAAA' in t:
        return False
    if t.startswith("[FILE UPLOAD") or t.startswith("[FILE DOWNLOAD") or t.startswith("[FILE CONTENT") or t.startswith("[SITE BLOCKED"):
        return True
    if _looks_like_filename_only(t):
        return False
    # Only drop lone path separators — keep user symbols like # @ ! ? $ %
    if len(t) == 1 and t in "/.\\|":
        return False
    # Isolated single ASCII letters (e.g. 'b', 'r', 'x') are protobuf varints/tags or wire tokens, not user chat prompts.
    # Preserve digits ('0'-'9'), user symbols ('?', '!', '#', '$', '%'), and non-ASCII characters (e.g. Chinese/Japanese kanji).
    if len(t) == 1 and t.isalpha() and ord(t) < 128:
        return False

    # Reject raw urlencoded wire parameters or batch execute bodies
    if any(wire in t for wire in ("count=", "&ofs=", "req0___data__", "f.req=", "soc-app=", "soc-platform=", "___data__=")):
        return False

    # Reject raw GraphQL operations
    if t.startswith(("mutation ", "mutation{", "query {", "subscription ", "subscription{")):
        return False

    low = t.lower()
    if low in BATCHEXECUTE_LOCALE_JUNK or re.fullmatch(r"[a-z]{2}-[a-z]{2,3}", low):
        return False
    # Locale strings or locale tags with trailing wire noise (e.g. 'en-US', 'en-USz qBudp', 'fr-FR', 'zh-CN')
    if re.match(r"^[a-z]{2}[-_][a-z]{2,4}", low):
        return False
    if low in _CONTROL_PLANE_PROMPT_TOKENS:
        return False
    if low in {
        "batchexecute", "wrb.fr",
        "bard activity enabled", "activity enabled", "streamgenerate",
        "co.in", "com.au", "co.uk", "com.br", "co.jp", "co.kr",
    }:
        return False
    if "bard activity" in low and len(t) < 30:
        return False
    # Domain / public-suffix crumbs that Gemini embeds in wire payloads (not typed chat)
    # Keep decimals like 3.14 (not filenames).
    if re.fullmatch(r"[a-z0-9]{1,8}\.(?:co\.)?[a-z]{2,3}", low):
        # Reject TLD suffixes; keep numeric decimals.
        suffix = low.rsplit(".", 1)[-1]
        if suffix.isalpha():
            return False

    # Filter tokens and RPC IDs when text has no spaces.
    # Digit-only text is a valid user prompt (IDs, math, OTPs). Do not drop it.
    if " " not in t:
        # UUIDs or React Server Action tokens (e.g. $a74604b4-54f3-43da-8962-990f7883a6ad, $542aef20-... or 74604b4-54f3-...)
        if re.fullmatch(r"(\$a?|\$)?[0-9a-fA-F]{4,16}(?:-[0-9a-fA-F]{4,16}){2,6}", t, re.IGNORECASE):
            return False
        if t.startswith(("$a", "$@", "$F", "$L", "$")) and len(t) >= 16 and "-" in t:
            return False
        # Gemini session / client tokens: _05Zravx, _a1B2c3d4
        if re.fullmatch(r"_[0-9A-Za-z]{4,24}", t):
            return False
        if t.startswith("_") and 5 <= len(t) <= 32 and re.fullmatch(r"[0-9A-Za-z_]+", t):
            return False
        # Google conversation/response tokens: r_653a..., c_44a8..., v_7f45..., rc_...
        if re.fullmatch(r"[rcv][_\.][0-9a-fA-F]{6,}", t, re.IGNORECASE):
            return False
        # Hex hashes that contain a-f (not digit-only numbers the user typed)
        if len(t) >= 12 and re.fullmatch(r"[0-9a-fA-F]{12,64}", t) and re.search(r"[a-fA-F]", t):
            return False
        # Google batchexecute RPC ids.
        if 4 <= len(t) <= 8 and re.fullmatch(r"[A-Za-z0-9]+", t):
            # Mixed-case RPC id e.g. ESY5D, VxUbXb — keep all-lower words (hi, hello, tamil…)
            if any(ch.isupper() for ch in t) and any(ch.islower() for ch in t):
                return False
            if sum(1 for ch in t if ch.isupper()) >= 2 and any(ch.isdigit() for ch in t):
                return False

    return True


# Extra product hosts are not auto-applied. Admin must add them in Target Websites.

def detect_target(host: str) -> tuple[bool, str, str]:
    """Check if host matches any monitored domain. Returns (is_target, domain, platform)."""
    domains_map = get_target_domains()
    h = (host or "").lower().strip(".")
    if "://" in h:
        h = h.split("://", 1)[1]
    h = h.split("/", 1)[0].split("?", 1)[0]
    if ":" in h:
        h = h.rsplit(":", 1)[0]
    if h.startswith("www."):
        h = h[4:]
    if not h:
        return False, "", ""
    best_domain = ""
    best_platform = ""
    best_len = -1
    for domain, platform in domains_map.items():
        d = (domain or "").lower().strip(".")
        if "://" in d:
            d = d.split("://", 1)[1]
        d = d.split("/", 1)[0].split("?", 1)[0]
        if ":" in d:
            d = d.rsplit(":", 1)[0]
        if d.startswith("www."):
            d = d[4:]
        if not d:
            continue
        if h == d or h.endswith("." + d):
            if len(d) > best_len:
                best_len = len(d)
                best_domain = domain
                best_platform = platform
    if best_domain:
        return True, best_domain, best_platform
    return False, "", ""


def _is_internal_wire_text(text: str) -> bool:
    """Internal RPC ids, pubsub actions, and wire fragments — not user-typed chat."""
    t = (text or "").strip()
    if not t:
        return True
    if _is_typed_numeric_prompt(t):
        return False
    low = t.lower()
    if low in {
        "turn exchange complete", "fetch socket url", "ping", "pong",
        "heartbeat", "keepalive", "keep alive", "connection established",
        "stream complete", "message complete", "typing", "presence",
    }:
        return True
    # pubsub.fetch-socket-url, client.create, etc.
    if " " not in t and re.fullmatch(r"[a-z][a-z0-9_.-]*(?:\.[a-z][a-z0-9_.-]+)+", low):
        return True
    if " " not in t and re.fullmatch(r"[a-z]+(?:_[a-z0-9]+){2,}", low):
        return True
    # Short expressions (symbols, code, math operators, tags): c++, x=1, #1, i++, a:=1, $50, 10%, fn(), etc.
    if len(t) <= 14 and " " not in t:
        _allowed_syms = set("+-*/=<>!&|^%$#@?:;~_().[]{}'\",\\`")
        if all(c.isalnum() or c in _allowed_syms for c in t):
            # Only flag high-entropy base64/hex hashes without operators
            if len(t) >= 6 and not any(c in "+-*/=<>!&|^%$#@?:;~_().[]{}'\"`" for c in t):
                vowels = sum(1 for c in t.lower() if c in "aeiouy")
                if vowels == 0 and sum(1 for c in t if c.isupper()) >= 2:
                    return True
            return False
    return False


_TURN_NEST_KEYS = ("transcript", "turn", "turns", "candidates", "blocks", "items")


def _flatten_text_leaves(node, depth: int = 0) -> list[str]:
    if depth > 6:
        return []
    if isinstance(node, str):
        return [node] if node.strip() else []
    if isinstance(node, list):
        out: list[str] = []
        for item in node:
            out.extend(_flatten_text_leaves(item, depth + 1))
        return out
    return []


def _user_typed_value(data) -> str | None:
    """{"type": "user", "value": [["hi"]]} turn records (Notion AI style transcripts)."""
    if not isinstance(data, dict):
        return None
    if str(data.get("type") or "").lower() not in ("user", "human"):
        return None
    for key in ("value", "text", "content"):
        chunks = _flatten_text_leaves(data.get(key))
        if chunks:
            return _clean_prompt_text(" ".join(chunks))
    return None


def _body_has_user_send_payload(data) -> bool:
    """True when JSON body carries an explicit user message/query — not sync/telemetry.

    Used for ANY admin-added Target domain (known products + future custom AIs).
    """
    if not isinstance(data, dict):
        return False
    if "_dd" in data or "format_version" in data:
        return False
    event = str(data.get("event") or data.get("type") or "").lower()
    if event in (
        "ping", "pong", "typing", "presence", "heartbeat", "metrics", "internal",
        "rgstr", "telemetry", "analytics", "rum", "activity", "beacon", "stat", "stats",
    ):
        return False
    if str(data.get("command") or data.get("action") or "").lower() in (
        "ping", "pong", "metrics", "telemetry", "rgstr", "activity",
    ):
        return False

    # GraphQL stringified or dictionary variables (Grok, Poe, custom AI GraphQL APIs)
    variables = data.get("variables")
    if isinstance(variables, str) and variables.strip().startswith("{"):
        try:
            variables = json.loads(variables)
        except Exception:
            pass
    if isinstance(variables, dict):
        if _body_has_user_send_payload(variables):
            return True

    # Nested operation objects common in modern AIs (incl. Copilot SignalR arguments, Mistral messageInput)
    for nest_key in (
        "request", "input", "payload", "body", "args", "data", "params", "arguments",
        "messageInput", "message_input", "message", "messages", "content", "contents",
        "docs", "files", "attachments", "uploadedFiles", "sources", "file_list",
    ):
        sub = data.get(nest_key)
        if isinstance(sub, dict) and _body_has_user_send_payload(sub):
            return True
        if isinstance(sub, list):
            for item in sub:
                if isinstance(item, dict) and _body_has_user_send_payload(item):
                    return True

    if _user_typed_value(data):
        return True
    for turn_key in _TURN_NEST_KEYS:
        sub = data.get(turn_key)
        if isinstance(sub, dict) and _body_has_user_send_payload(sub):
            return True
        if isinstance(sub, list) and sub and isinstance(sub[-1], dict) and _body_has_user_send_payload(sub[-1]):
            return True

    msgs = data.get("messages")
    if isinstance(msgs, list):
        for msg in reversed(msgs):
            if isinstance(msg, dict) and _extract_from_message_obj(msg):
                return True
    contents = data.get("contents")
    if isinstance(contents, list):
        for msg in reversed(contents):
            if isinstance(msg, dict) and _extract_from_message_obj(msg):
                return True
    # Nested OpenAI/Claude-style content.parts / content.text
    content = data.get("content")
    if isinstance(content, dict):
        parts = content.get("parts")
        if isinstance(parts, list) and any(
            isinstance(p, str) and p.strip() for p in parts
        ):
            return True
        for ck in ("text", "input_text", "message"):
            cv = content.get(ck)
            if isinstance(cv, str) and cv.strip():
                return True
    if isinstance(content, list):
        for block in content:
            if isinstance(block, str) and block.strip() and looks_like_user_prompt(block.strip()):
                return True
            if isinstance(block, dict):
                tv = block.get("text") or block.get("input_text")
                if isinstance(tv, str) and tv.strip() and looks_like_user_prompt(tv.strip()):
                    return True
    if isinstance(content, str) and content.strip() and looks_like_user_prompt(content.strip()):
        return True
    for key in _UNIVERSAL_PROMPT_KEYS:
        val = data.get(key)
        if isinstance(val, str) and val.strip() and looks_like_user_prompt(val.strip()):
            return True
        if isinstance(val, (int, float)) and str(val).strip():
            return True
    return False


def _is_clear_chat_submit(path: str, host: str, raw_text: str, raw_bytes: bytes = b"") -> bool:
    """Finished chat Send on an admin-monitored host — not typing/telemetry/CDN."""
    path_l = (path or "").lower().split("?", 1)[0]
    body = raw_text or ""
    if _path_has_ignore_pattern(path_l):
        return False
    if path_l and NOISE_EXTENSIONS.search(path_l):
        return False
    if "prepare" in path_l or "autocomplet" in path_l or "implicit_hint" in path_l:
        return False
    if is_unsubmitted_chat_body(path, body):
        return False
    if is_event_sync_noise_content(body):
        return False
    if (
        is_rest_sse_ask_submit(path, body)
        or is_batchexecute_chat_submit(path, body)
        or is_event_send_chat_submit(path, body)
        or _is_anthropic_messages_api_shape(path, body)
    ):
        return True
    if _is_messages_conversation_path(path_l) or _looks_like_messages_parts_body(body, raw_bytes):
        return True
    if _path_has_chat_marker(path_l):
        # Bare /batchexecute is noise; only StreamGenerate submits.
        if "batchexecute" in path_l and not is_batchexecute_chat_submit(path, body):
            return False
        if is_noise(path, body):
            return False
        return True
    if body.lstrip().startswith(("{", "[")) or "\x1e" in body:
        data = _loads_json_maybe_signalr(body)
        if isinstance(data, dict) and _body_has_user_send_payload(data):
            return True
        if isinstance(data, list) and any(
            isinstance(item, dict) and _body_has_user_send_payload(item) for item in data
        ):
            return True
    return False


def _is_confident_chat_send(path: str, raw_text: str, raw_bytes: bytes = b"") -> bool:
    """True when request is very likely a finished user Send (platform body shapes).

    Covers known products AND future admin-added AIs that POST JSON with a
    clear user message / parts[] payload (no product hostname hardcoding).
    """
    path_l = (path or "").lower().split("?", 1)[0]
    if _path_has_ignore_pattern(path_l) or is_noise(path_l, raw_text or ""):
        return False
    if _path_looks_like_upload(path_l):
        return False
    if _is_clear_chat_submit(path, "", raw_text, raw_bytes):
        return True
    body = raw_text or ""
    if _looks_like_messages_parts_body(body, raw_bytes) and (
        _is_messages_conversation_path(path_l) or _path_has_chat_marker(path_l)
    ):
        return True
    if is_rest_sse_ask_submit(path, body):
        return True
    if _is_anthropic_messages_api_shape(path, body):
        return True
    # Unknown / new AI: JSON user-send payload = finished Send (any path, any domain).
    if body.lstrip().startswith(("{", "[")) or "\x1e" in body:
        data = _loads_json_maybe_signalr(body)
        if isinstance(data, dict) and _body_has_user_send_payload(data):
            return True
        if isinstance(data, list) and any(isinstance(item, dict) and _body_has_user_send_payload(item) for item in data):
            return True

    # Form, XML, NDJSON user-send shapes
    if "=" in body and any(k in body for k in ("prompt=", "query=", "message=", "text=", "input=")):
        return True
    if body.lstrip().startswith("<") and any(k in body for k in ("<prompt", "<query", "<question", "<message", "<text", "<input")):
        return True
    if "\n" in body and '"user"' in body and any(k in body for k in ('"content"', '"text"', '"message"', '"prompt"')):
        return True
    return False


_CHAT_METADATA_JUNK = frozenset({
    "user", "assistant", "system", "auto", "text", "message", "role", "content",
    "parts", "author", "metadata", "recipient", "client", "server", "ping", "pong",
    "null", "undefined", "true", "false", "default", "model", "parent", "child",
    "chatgpt", "gpt-4", "gpt-4o", "gpt-3.5", "o1", "o3", "thinking", "standard",
    "claude", "haiku", "sonnet", "opus", "claude-3-5-sonnet", "claude-3-5-haiku", "claude-3-7-sonnet",
})

_TIMEZONE_RE = re.compile(
    r"^(?:Africa|America|Antarctica|Arctic|Asia|Atlantic|Australia|Europe|Indian|Pacific|UTC|GMT)(?:/[\w_-]+)+$",
    re.IGNORECASE,
)
_SESSION_ID_RE = re.compile(r"^(?:sess|session|sid|device|req|msg)_[0-9a-zA-Z_-]+$", re.IGNORECASE)
_TIMESTAMP_SEQ_RE = re.compile(r"^\d{10,}(?:-\d+)?$")
_UUID_LIKE_RE = re.compile(r"(\$a?|\$)?[0-9a-fA-F]{4,16}(?:-[0-9a-fA-F]{4,16}){2,6}", re.I)


def _is_chat_metadata_token(text: str) -> bool:
    t = (text or "").strip()
    t_l = t.lower()
    if not t:
        return True
    if t_l in _CHAT_METADATA_JUNK:
        return True
    if re.fullmatch(r"gpt[-\d\.]+[a-z]*", t_l) or re.fullmatch(r"claude[-\d\.]+[a-z]*", t_l):
        return True
    if any(ord(c) < 32 and c not in "\n\r\t" for c in t):
        return True
    if len(t) == 1 and t in "/.\\|":
        return True
    if _TIMEZONE_RE.match(t):
        return True
    if _SESSION_ID_RE.match(t):
        return True
    if _TIMESTAMP_SEQ_RE.match(t):
        return True
    if _UUID_LIKE_RE.search(t):
        return True
    return False


def _pick_best_user_text(candidates: list[str]) -> str | None:
    """Choose the best user-typed string from protobuf/JSON fragments (any domain)."""
    best = None
    best_score = -1
    for s in reversed(candidates):
        if not s:
            continue
        got = _clean_prompt_text(s)
        if not got or not looks_like_user_prompt(got):
            continue
        if _is_opaque_wire_blob(got) or _is_internal_wire_text(got) or _is_chat_metadata_token(got):
            continue
        if _looks_like_document_body_dump(got) or _looks_like_filename_only(got):
            continue
        if len(got) == 1 and got.isalpha() and ord(got) < 128:
            continue
        if re.match(r"^[a-z]{2}[-_][A-Za-z]{2,4}", got, re.I):
            continue
        if re.fullmatch(r"(\$a?|\$)?[0-9a-fA-F]{4,16}(?:-[0-9a-fA-F]{4,16}){2,6}", got, re.I):
            continue
        # User prompts are typically concise (1 to 500 chars).
        score = min(len(got), 300)
        if 1 <= len(got) <= 300:
            score += 60
        if " " in got:
            score += 24
        if got.isdigit():
            score += 20
        if 2 <= len(got) <= 4 and got.isalpha():
            score += 12
        if re.search(r"[a-zA-Z]", got) and re.search(r"\d", got):
            score += 4
        # Conversational prompt keywords
        if any(w in got.lower() for w in ("please", "summarize", "explain", "what", "how", "why", "write", "analyze", "review", "check", "help", "create", "can you", "find", "compare")):
            score += 80
        if score > best_score:
            best_score = score
            best = got
    return best


def _extract_messages_parts_prompt(blob: str) -> str | None:
    """Last ChatGPT/OpenAI parts[] slot — the finished user Send text."""
    if not blob:
        return None
    last: str | None = None
    patterns = (
        r'"parts"\s*:\s*\[\s*"((?:[^"\\]|\\.)*)"',
        r'"content"\s*:\s*\{\s*"content_type"\s*:\s*"text"\s*,\s*"parts"\s*:\s*\[\s*"((?:[^"\\]|\\.)*)"',
        r'"input_text"\s*:\s*"((?:[^"\\]|\\.)*)"',
    )
    for pat in patterns:
        for m in re.finditer(pat, blob):
            raw = m.group(1)
            cand = _clean_prompt_text(
                raw.encode("utf-8").decode("unicode_escape", errors="ignore")
                if "\\" in raw
                else raw
            )
            if (
                cand
                and looks_like_user_prompt(cand)
                and not _is_opaque_wire_blob(cand)
                and not _is_internal_wire_text(cand)
                and not _is_chat_metadata_token(cand)
            ):
                last = cand
    return last


def _is_typing_or_draft_path(path: str, raw_text: str = "") -> bool:
    """Autocomplete / prepare / in-progress only — not a finished Send."""
    path_l = (path or "").lower()
    if path_l.endswith("/prepare") or "/prepare" in path_l:
        return True
    if "autocomplet" in path_l or "implicit_hint" in path_l:
        return True
    if "partial_query" in (raw_text or "") and '"query_str"' not in (raw_text or ""):
        return True
    return is_unsubmitted_chat_body(path, raw_text)


def _is_static_asset_request(path: str) -> bool:
    """Static CDN assets — never chat submits."""
    path_l = (path or "").lower().split("?", 1)[0]
    return bool(path_l and NOISE_EXTENSIONS.search(path_l))


def _should_intercept_extracted_prompt(
    prompt: str | None,
    path: str,
    raw_text: str,
    domain: str,
    host: str = "",
    raw_bytes: bytes = b"",
) -> bool:
    """Finished chat Send with user text → predict. Telemetry/sync must not.

    Confident Send (Claude/ChatGPT/Gemini/… shape): accept ANY non-empty typed
    text — letters, digits, symbols, 1 char or 1000+ — unless clear protocol junk.
    """
    if not prompt or not isinstance(prompt, str):
        return False
    text = prompt.strip()
    if len(text) < 1:
        return False

    # ── Non-ASCII / Unicode scripts (Tamil, Arabic, Chinese, Hindi, etc.) ────
    # Non-Latin prompts are always real user text.
    if any(ord(c) > 127 for c in text[:80]):
        if text.startswith("[FILE UPLOAD"):
            return False
        if _is_typing_or_draft_path(path, raw_text):
            return False
        if is_noise(path, raw_text):
            return False
        return True

    if text.startswith("[FILE UPLOAD"):
        return False
    if _is_typing_or_draft_path(path, raw_text):
        return False
    if is_noise(path, raw_text):
        return False
    # File Send: keep short captions only in Prompt Logs (never document body dumps).
    if _send_carries_attachment(raw_text) or (domain and _domain_has_pending_upload_cache(domain)):
        if _looks_like_document_body_dump(text) or len(text) > 320:
            return False

    confident = _is_confident_chat_send(path, raw_text, raw_bytes)

    if confident:
        # Document body dump must NEVER become a user prompt when an upload exists
        if _looks_like_document_body_dump(text):
            return False
        # Exact user Send — do not drop number/symbol/short text via wire heuristics.
        if _is_clear_protocol_junk(text) or _is_google_wire_blob(text) or _is_opaque_wire_blob(text):
            return False
        # ChatGPT attach JSON often exposes "document.pdf" as content — that is the
        # filename, not a typed prompt. File row is logged via post_upload_intercept.
        if _looks_like_filename_only(text):
            return False
        # Duplicate peek is still a real Send — caller reuses remembered decision.
        # Never return False here or the prompt silently vanishes from Prompt Logs.
        return True

    # Non-confident paths keep stricter filters (avoid telemetry false positives).
    if not looks_like_user_prompt(text):
        return False
    if _is_opaque_wire_blob(text):
        return False
    if _looks_like_binary_or_wire_garbage(text) and not _is_digit_heavy_user_text(text):
        return False
    if _is_ide_non_chat_noise(text, domain=domain) or _is_ide_non_chat_noise(text, domain=host):
        return False
    if _is_internal_wire_text(text) and not _is_digit_heavy_user_text(text):
        return False
    if _looks_like_filename_only(text):
        return False
    if not (
        is_chat_path(path, host, raw_text)
        or _path_has_chat_marker(path)
        or _is_clear_chat_submit(path, host or "", raw_text, raw_bytes)
    ):
        return False
    # Allow 1-char typed Sends (?, !, #, a, …) when body/path already looks like chat.
    if len(text) < 2 and not text.isdigit() and not _is_typed_numeric_prompt(text):
        if not (
            is_chat_path(path, host, raw_text)
            or _path_has_chat_marker(path)
            or _is_clear_chat_submit(path, host or "", raw_text, raw_bytes)
        ):
            return False
    body = (raw_text or "").lstrip()
    if body.startswith(("{", "[")) and not _looks_like_messages_parts_body(body, raw_bytes):
        if not _path_has_chat_marker(path) and not is_chat_path(path, host, raw_text):
            return False
    # Never silent-drop duplicates here — caller reuses remembered Guard decision
    # (returning False previously let the 2nd browser fire bypass Prompt Logs).
    return True


def is_noise(path: str, content: str = "") -> bool:
    """Filter telemetry, analytics, static assets and background noise."""
    path_lower = path.lower().split("?", 1)[0]

    if NOISE_EXTENSIONS.search(path_lower):
        return True

    if _path_has_ignore_pattern(path_lower):
        return True

    # Partial typing / autocomplete — not a submitted prompt
    if path_lower.endswith("/prepare") or "/prepare" in path_lower or "partial_query" in (content or ""):
        return True
    if "autocomplet" in path_lower or "implicit_hint" in path_lower:
        return True

    if content:
        # Datadog RUM actions / telemetry (Claude / web monitoring)
        if '"_dd":' in content or ('"format_version":' in content and '"action"' in content):
            return True
        if content.strip().startswith(('{"type":"action"', '{"type": "action"')) and "_dd" in content:
            return True
        if content.startswith('{"counters":') or content.startswith('{"view":') or content.startswith('{"events":'):
            return True
        if '{"prepare_token":' in content or '"prepare_token"' in content:
            return True
        # ChatGPT encrypted sentinel / challenge blobs — not user text
        if '"p":"gAAAA' in content or content.strip().startswith('{"p":"gAAAA'):
            return True
        if '"requested_default_model"' in content and '"messages"' not in content:
            return True
        if 'AttributionReporting' in content or 'googletagmanager' in content:
            return True
        if 'columnNumber' in content and 'lineNumber' in content and 'sourceFile' in content:
            return True
        if 'com.google.android.gms' in content:
            return True
        if '"presence"' in content and '"messages"' not in content and '"parts"' not in content:
            return True
        if content.startswith('{"id":') and '"command":' in content and '"messages"' not in content:
            return True
        # Protobuf chat submit — do not drop as noise.
        path_l = (path or "").lower()
        chat_submit = (
            _is_messages_conversation_path(path_l)
            or is_rest_sse_ask_submit(path_l, content)
            or is_batchexecute_chat_submit(path_l, content)
            or is_event_send_chat_submit(path_l, content)
            or _path_has_chat_marker(path_l)
        )
        if (
            len(content) > 0
            and ord(content[0]) < 32
            and ord(content[0]) not in (10, 13)
            and not chat_submit
        ):
            return True
        # Gemini / form chat bodies are valid (f.req=...)
        cl = content.lstrip()
        if cl.startswith("f.req=") or "f.req=" in cl[:120]:
            return False
        # Copilot websocket send events
        if '"event":"send"' in content or '"event": "send"' in content:
            return False
        # Copilot SignalR / sync noise — not user prompts
        if ("chathub" in path_l or "sydney" in path_l or "\x1e" in content or '"target":' in cl) and is_event_sync_noise_content(content):
            return True
        # Cloudflare challenge bodies (non-JSON)
        if not cl.startswith(("{", "[")) and not looks_like_user_prompt(content[:200]):
            if len(content) > 40 and content.count(" ") < 2 and not any(ch.isdigit() for ch in content):
                return True

    return False


def _clean_prompt_text(text: str) -> str | None:
    """Normalize extracted text; reject empty / challenge blobs."""
    if not text or not isinstance(text, str):
        return None
    cleaned = text.strip()
    if len(cleaned) < 1:
        return None
    if cleaned.startswith("gAAAA") or '"p":"gAAAA' in cleaned:
        return None
    if cleaned.lower() in ("null", "undefined"):
        return None
    if not looks_like_user_prompt(cleaned):
        return None
    return cleaned


def is_duplicate_event(domain: str, event_key: str, ttl: float = DEDUPE_TTL, *, mark: bool = True) -> bool:
    """Return True if the same event was already logged for this domain recently.

    mark=False only peeks (does not stamp) — use before a backend write, then
    call mark_duplicate_event after success so failed posts can retry.
    """
    now = time.time()
    key = f"{domain}|{event_key.strip().lower()}"
    with _dedupe_lock:
        expired = [k for k, ts in _recent_prompts.items() if now - ts > max(DEDUPE_TTL, BLOCK_DEDUPE_TTL)]
        for k in expired:
            _recent_prompts.pop(k, None)

        prev = _recent_prompts.get(key)
        if prev is not None and (now - prev) <= ttl:
            return True
        if mark:
            _recent_prompts[key] = now
    return False


def mark_duplicate_event(domain: str, event_key: str) -> None:
    """Stamp dedupe key after a successful backend log."""
    key = f"{domain}|{(event_key or '').strip().lower()}"
    with _dedupe_lock:
        _recent_prompts[key] = time.time()


def messages_parts_carries_file(raw_text: str) -> bool:
    """ChatGPT multimodal sends use content_type:file / file_id — not always attachments[]."""
    if not raw_text:
        return False
    low = raw_text.lower()
    if re.search(r'"content_type"\s*:\s*"file"', low):
        return True
    if "sediment://" in low or "file-service://" in low:
        return True
    if re.search(r'"file_id"\s*:\s*"file-[a-zA-Z0-9_-]+"', low):
        return True
    if re.search(r'"content_type"\s*:\s*"multimodal_text"', low):
        if re.search(
            r'"(?:file_id|asset_pointer|file-service://|sediment://|mime_type|file_name|filename)"',
            low,
        ):
            return True
        if re.search(
            r'"name"\s*:\s*"[^"]+\.(?:pdf|docx?|xlsx?|pptx?|png|jpe?g|gif|webp|csv|txt|zip)"',
            low,
        ):
            return True
    if re.search(
        r'"parts"\s*:\s*\[[\s\S]{0,4000}?"content_type"\s*:\s*"file"',
        low,
    ):
        return True
    return False


def detect_messages_parts_file_upload(
    host: str,
    path: str,
    method: str,
    content_type: str,
    body_len: int,
    raw: bytes,
) -> tuple[bool, str]:
    """Catch file uploads on admin-monitored domains (path/body — no product hostname list)."""
    if not detect_target(host or "")[0]:
        return False, ""
    if (method or "").upper() not in ("POST", "PUT", "PATCH"):
        return False, ""
    path_l = (path or "").lower().split("?", 1)[0]
    if "/realtime" in path_l:
        return False, ""
    ct = (content_type or "").lower()
    data = raw or b""
    if b"webrtc-datachannel" in data or b'name="sdp"' in data:
        return False, ""

    if any(x in path_l for x in _GENERIC_UPLOAD_PATH_MARKERS):
        if body_len >= 8:
            return True, f"File API ({path_l[:80]})"
    if "/backend-api/" in path_l:
        if any(x in path_l for x in ("/sentinel/", "/prepare", "/autocomplet", "/me", "/settings")):
            return False, ""
        if body_len >= 64 and (
            any(p in ct for p in UPLOAD_CONTENT_TYPES)
            or data[:5] == b"%PDF-"
            or (len(data) >= 2 and data[:2] == b"PK")
            or b"filename=" in data[:16000].lower()
        ):
            return True, f"Binary upload ({path_l[:80]})"
    return False, ""


def _path_looks_like_upload(path: str) -> bool:
    """True for generic file-upload URL paths on any monitored domain."""
    p = (path or "").lower().split("?", 1)[0]
    if not p:
        return False
    if any(m in p for m in _GENERIC_UPLOAD_PATH_MARKERS):
        return True
    return any(x in p for x in ("/files", "/file/", "/upload", "/attachments", "/attachment", "/media/upload", "/convert_document", "/documents", "/document", "/storage", "/import", "/blob", "/blobs"))


def is_unsubmitted_chat_body(path: str, body: str) -> bool:
    """True only for prepare/draft/in-progress — finished Send must predict."""
    path_l = (path or "").lower()
    if "/prepare" in path_l or path_l.endswith("prepare") or "autocomplet" in path_l:
        return True
    if "partial_query" in (body or "") and "query_str" not in (body or ""):
        return True
    if "aPya6c" in (body or ""):
        return True
    if not body:
        return False
    try:
        data = json.loads(body)
    except Exception:
        return False
    if not isinstance(data, dict):
        return False
    for m in data.get("messages") or []:
        if not isinstance(m, dict):
            continue
        status = str(m.get("status") or "").lower()
        if status in ("in_progress", "unfinished", "draft"):
            return True
        meta = m.get("metadata") if isinstance(m.get("metadata"), dict) else {}
        if meta.get("is_complete") is False:
            return True
    return False


def _composer_hold_seconds(text: str) -> float:
    n = len((text or "").strip())
    if n <= 3:
        return COMPOSER_STABILITY_HOLD_TINY
    if n <= 12:
        return COMPOSER_STABILITY_HOLD_SHORT
    return COMPOSER_STABILITY_HOLD


def _composer_related(a: str, b: str) -> bool:
    """True when one string is a typing prefix/extension of the other.

    Prefix-only — do NOT treat unrelated Sends (hi vs how are u) as related
    via fuzzy first-3-char matching (that caused real prompts to be dropped).
    """
    if not a or not b:
        return False
    if a == b:
        return True
    if a.startswith(b) or b.startswith(a):
        return True
    return False


def note_composer_observation(domain: str, prompt: str) -> None:
    """Observe phase: remember latest composer text for this Target family key."""
    text = (prompt or "").strip()
    if not text or not domain:
        return
    with _composer_lock:
        _composer_draft[domain] = (text, time.time())


def is_composer_typing_draft(domain: str, prompt: str) -> bool:
    """True when this fragment is clearly mid-edit vs the latest observation."""
    text = (prompt or "").strip()
    if not text or not domain:
        return False
    now = time.time()
    with _composer_lock:
        prev = _composer_draft.get(domain)
        _composer_draft[domain] = (text, now)
    if not prev:
        return False
    prev_text, prev_ts = prev
    elapsed = now - float(prev_ts)
    if elapsed >= COMPOSER_PREFIX_WINDOW:
        return False
    if text == prev_text:
        return False
    # Shorter than latest related text → stale keystroke request (skip).
    if prev_text.startswith(text) and len(prev_text) > len(text):
        return True
    # We just grew from prev within window — still typing; commit waits for quiet.
    if text.startswith(prev_text) and 1 <= (len(text) - len(prev_text)) <= COMPOSER_DRAFT_MAX_GROW:
        return True
    if prev_text.startswith(text) and 1 <= (len(prev_text) - len(text)) <= COMPOSER_DRAFT_MAX_GROW:
        return True
    return False


def wait_if_composer_unstable(domain: str, prompt: str) -> str | None:
    """Commit phase for ALL monitored domains.

    Many AI sites (Grok, Copilot, …) POST every keystroke as a chat-shaped request.
    We OBSERVE those, but COMMIT predict only after the composer text is quiet
    for an adaptive hold — so "h" then "hi" becomes one predict: "hi".

    Returns final text to evaluate, or None if this request was superseded.
    """
    text = (prompt or "").strip()
    if not text or not domain:
        return text or None

    note_composer_observation(domain, text)

    # Very long pastes / finished essays: still brief check for supersede, then go.
    hold = _composer_hold_seconds(text)
    if len(text) > COMPOSER_HOLD_MAX_LEN:
        hold = min(hold, 0.25)

    deadline = time.time() + hold
    while time.time() < deadline:
        time.sleep(0.05)
        with _composer_lock:
            cur = _composer_draft.get(domain)
        if not cur:
            break
        cur_text, cur_ts = cur
        cur_text = (cur_text or "").strip()
        if not cur_text:
            break

        if cur_text != text:
            if _composer_related(cur_text, text):
                if len(cur_text) > len(text):
                    # Longer typing won — follow it on THIS request (do not silent-drop;
                    # the longer request may never arrive if the browser aborted).
                    print(
                        f"[Gateway Proxy] Composer superseded -> commit longer | {domain!r} | "
                        f"{text[:40]!r} -> {cur_text[:40]!r}"
                    )
                    text = cur_text
                    hold = _composer_hold_seconds(text)
                    deadline = max(deadline, time.time() + hold * 0.65)
                    continue
                # We grew (or matched longer stored) — follow the latest string.
                text = cur_text
                hold = _composer_hold_seconds(text)
                deadline = max(deadline, time.time() + hold * 0.65)
                continue
            # Unrelated new prompt — stop waiting; evaluate what we have.
            break

        # Same text: commit once it has been quiet long enough.
        quiet = time.time() - float(cur_ts)
        need = _composer_hold_seconds(text)
        if quiet >= need * 0.85 or quiet >= COMPOSER_DRAFT_TTL:
            break

    with _composer_lock:
        cur = _composer_draft.get(domain)
    if cur:
        cur_text = (cur[0] or "").strip()
        if cur_text and cur_text != text and _composer_related(cur_text, text):
            text = cur_text if len(cur_text) >= len(text) else text

    # Final guard: if live composer is a longer related prefix within PREFIX_WINDOW,
    # Prefer longer text (avoid silent Prompt Log misses).
    with _composer_lock:
        cur = _composer_draft.get(domain)
    if cur:
        cur_text, cur_ts = cur
        cur_text = (cur_text or "").strip()
        if (
            cur_text
            and text != cur_text
            and cur_text.startswith(text)
            and len(cur_text) > len(text)
            and (time.time() - float(cur_ts)) <= COMPOSER_PREFIX_WINDOW
        ):
            print(
                f"[Gateway Proxy] Composer prefix -> commit longer | {domain!r} | "
                f"{text[:40]!r} -> {cur_text[:40]!r}"
            )
            text = cur_text

    return text


def clear_composer_state(domain: str) -> None:
    if domain:
        with _composer_lock:
            _composer_draft.pop(domain, None)


def peek_recent_composer_caption(domain: str, max_age: float = 15.0) -> str:
    """
    Latest typed composer text for this Target family.
    Used when file Send body omits the caption (ChatGPT/Copilot/Perplexity often
    separate the typed note from the attachment wire) so Prompt Logs can still
    show Claude-style: filename -- text.
    """
    if not domain:
        return ""
    with _composer_lock:
        prev = _composer_draft.get(domain)
    if not prev:
        return ""
    text, ts = prev
    try:
        if (time.time() - float(ts)) > float(max_age):
            return ""
    except Exception:
        return ""
    t = (text or "").strip()
    if not t or len(t) > 2000:
        return ""
    if not looks_like_user_prompt(t):
        return ""
    if _looks_like_document_body_dump(t) or _looks_like_filename_only(t):
        return ""
    if _is_google_wire_blob(t) or _is_opaque_wire_blob(t):
        return ""
    return t


def _parts_to_text(parts) -> str | None:
    """Join ChatGPT/Claude-style content parts into plain user text (skip file/document blocks)."""
    if parts is None:
        return None
    if isinstance(parts, (str, int, float)):
        return _clean_prompt_text(str(parts))
    if not isinstance(parts, list):
        return None
    chunks = []
    for part in parts:
        if isinstance(part, (str, int, float)):
            chunks.append(str(part))
        elif isinstance(part, dict):
            if _is_file_content_part(part):
                continue
            ct = str(part.get("content_type") or "").lower()
            if ct and ct not in ("text", "input_text", "multimodal_text"):
                if ct in ("file", "image", "audio", "video", "document"):
                    continue
            if isinstance(part.get("text"), (str, int, float)):
                chunks.append(str(part["text"]))
            elif part.get("type") in ("text", "input_text") and isinstance(part.get("text"), (str, int, float)):
                chunks.append(str(part["text"]))
            elif "parts" in part and ct in ("", "text", "multimodal_text"):
                nested = _parts_to_text(part.get("parts"))
                if nested:
                    chunks.append(nested)
    return _clean_prompt_text(" ".join(chunks)) if chunks else None


def _extract_from_message_obj(msg: dict) -> str | None:
    """Pull user text from a single message object (OpenAI / ChatGPT / Claude / Copilot / generic shapes)."""
    if not isinstance(msg, dict):
        return None

    role = msg.get("role")
    if role is None and isinstance(msg.get("author"), dict):
        role = msg["author"].get("role")
    if role and str(role).lower() not in ("user", "human", "customer", "client", "sender"):
        return None

    content = msg.get("content")
    if isinstance(content, (str, int, float)):
        return _clean_prompt_text(str(content))
    if isinstance(content, dict):
        if _is_file_content_part(content):
            return None
        # ChatGPT web: {"content_type":"text","parts":["hello"]}
        if "parts" in content:
            return _parts_to_text(content.get("parts"))
        if isinstance(content.get("text"), (str, int, float)):
            return _clean_prompt_text(str(content["text"]))
    if isinstance(content, list):
        text_chunks: list[str] = []
        for block in content:
            if isinstance(block, dict) and _is_file_content_part(block):
                continue
            got = _parts_to_text([block]) if isinstance(block, dict) else _parts_to_text(block)
            if got:
                text_chunks.append(got)
        return _clean_prompt_text(" ".join(text_chunks)) if text_chunks else None

    # Copilot / Graph / Bing / generic fields
    for key in (
        "text", "prompt", "query", "query_str", "rawUserQuery",
        "utterance", "userMessage", "input", "question", "user_input", "inputs",
    ):
        val = msg.get(key)
        if isinstance(val, (str, int, float)):
            sval = str(val)
            if _is_opaque_wire_blob(sval):
                continue
            got = _clean_prompt_text(sval)
            if got:
                return got
    return None


def _extract_from_json(data) -> str | None:
    """Walk known and custom chat API shapes across ANY domain to extract the submitted user prompt."""
    if isinstance(data, list):
        # Gemini StreamGenerate is [null, "<nested json>", requestId, ...].
        # Never treat trailing numeric / token slots as the user prompt.
        if data and (data[0] is None or (len(data) >= 2 and isinstance(data[1], str) and data[1][:1] in ("[", "{"))):
            dumped = json.dumps(data, ensure_ascii=False)
            got = extract_batchexecute_prompt(dumped)
            if got:
                return _clean_prompt_text(got)
            return None
        # Prefer last user message in an array of messages
        for item in reversed(data):
            got = _extract_from_message_obj(item) if isinstance(item, dict) else None
            if got:
                return got
            if isinstance(item, str):
                # Raw array slots are often request ids; never save mixed-case RPC tokens.
                if _is_opaque_wire_blob(item):
                    continue
                got = _clean_prompt_text(item)
                if got:
                    return got
        return None

    if not isinstance(data, dict):
        return None

    # OpenAI-style messages[].content.parts user text.
    if isinstance(data.get("messages"), list):
        for msg in reversed(data["messages"]):
            got = _extract_from_message_obj(msg)
            if got:
                return got

    # Claude / Anthropic: messages or prompt
    if isinstance(data.get("prompt"), (str, int, float)):
        got = _clean_prompt_text(str(data["prompt"]))
        if got:
            return got

    # Gemini API: contents[].parts[].text
    if isinstance(data.get("contents"), list) and data["contents"]:
        last = data["contents"][-1]
        if isinstance(last, dict):
            role = str(last.get("role", "user")).lower()
            if role in ("user", "human", ""):
                got = _parts_to_text(last.get("parts"))
                if got:
                    return got

    # Perplexity / Copilot / Bing / generic query fields across ANY Target Website
    for key in (
        "query", "query_str", "prompt", "input", "input_text", "inputs", "text",
        "message", "question", "user_input", "last_query", "user_query",
        "rawUserQuery", "utterance", "userMessage", "content", "instruction",
        "search_query", "q",
    ):
        val = data.get(key)
        if isinstance(val, (str, int, float)):
            sval = str(val)
            if _is_opaque_wire_blob(sval):
                continue
            got = _clean_prompt_text(sval)
            if got:
                return got
        elif isinstance(val, list):
            got = _parts_to_text(val)
            if got:
                return got
        elif isinstance(val, dict):
            got = _extract_from_message_obj(val)
            if got:
                return got
            for nk in ("query", "text", "prompt", "question", "rawUserQuery", "content", "input", "message"):
                if isinstance(val.get(nk), (str, int, float)):
                    sval = str(val[nk])
                    if _is_opaque_wire_blob(sval):
                        continue
                    got = _clean_prompt_text(sval)
                    if got:
                        return got

    # Copilot: use message.text (content may be encrypted).
    if str(data.get("event", "")).lower() in ("send", "message", "chat"):
        msg = data.get("message")
        if isinstance(msg, dict):
            got = _extract_from_message_obj(msg)
            if got:
                return got
        for key in ("parts", "attachments", "input"):
            val = data.get(key)
            if isinstance(val, list):
                got = _parts_to_text(val)
                if got:
                    return got
            if isinstance(val, dict):
                got = _extract_from_message_obj(val)
                if got:
                    return got
            if isinstance(val, (str, int, float)):
                sval = str(val)
                if _is_opaque_wire_blob(sval):
                    continue
                got = _clean_prompt_text(sval)
                if got:
                    return got
        val = data.get("content")
        if isinstance(val, (str, int, float)):
            sval = str(val)
            if not _is_opaque_wire_blob(sval):
                got = _clean_prompt_text(sval)
                if got:
                    return got

    # Nested: { "params": { "query": "..." } }, { "payload": { ... } }, etc.
    for nest_key in ("params", "data", "payload", "body", "request", "arguments", "input", "options"):
        nested = data.get(nest_key)
        if isinstance(nested, (dict, list)):
            got = _extract_from_json(nested)
            if got:
                return got

    return None


def _deep_extract_from_json(data, depth: int = 0, max_depth: int = 10) -> str | None:
    """Recursive domain-agnostic JSON walk — finds user text in any nested shape."""
    if depth > max_depth:
        return None
    if isinstance(data, dict):
        role = str(data.get("role") or data.get("author") or data.get("sender") or "").lower()
        if role in ("system", "assistant", "bot", "model", "ai"):
            return None
        if str(data.get("type") or "").lower() in ("assistant", "bot", "model", "ai"):
            return None
        got = _user_typed_value(data)
        if got:
            return got
        if role in ("user", "human", "customer", "client", "sender", ""):
            got = _extract_from_message_obj(data)
            if got:
                return got
        for key in _UNIVERSAL_PROMPT_KEYS:
            val = data.get(key)
            if isinstance(val, (str, int, float)):
                if isinstance(val, (int, float, bool)):
                    if key in ("code", "data", "payload", "body", "operation", "entry", "status", "type", "id", "index", "count", "version", "step"):
                        continue
                    if val in (0, 1) and key not in ("prompt", "text", "query", "user_input", "message"):
                        continue
                sval = str(val).strip()
                # Unpack nested JSON string values.
                if sval.startswith(("{", "[")):
                    try:
                        unpacked = json.loads(sval)
                        if isinstance(unpacked, (dict, list)):
                            got = _deep_extract_from_json(unpacked, depth + 1, max_depth)
                            if got:
                                return got
                    except Exception:
                        pass
                # For direct known-key hits: only reject true protocol wire blobs,
                # NOT language-based filters. Any user text in a named prompt field
                # must be accepted regardless of length or language.
                if not _is_opaque_wire_blob(sval) and not _is_clear_protocol_junk(sval):
                    got = _clean_prompt_text(sval)
                    if got:
                        return got
                    # For floats/ints that clean to empty string, return raw stripped value
                    if isinstance(val, (int, float)):
                        raw = str(val).strip()
                        if raw and raw not in ("0", "1"):
                            return raw
            elif isinstance(val, list):
                got = _parts_to_text(val)
                if got:
                    return got
            elif isinstance(val, dict):
                got = _deep_extract_from_json(val, depth + 1, max_depth)
                if got:
                    return got
        for nest_key in (
            "params", "data", "payload", "body", "request", "arguments",
            "input", "inputs", "options", "message", "messages",
            "conversation", "chat", "turns", "history", "context", "query",
            *_TURN_NEST_KEYS,
        ):
            nested = data.get(nest_key)
            if isinstance(nested, (dict, list)):
                got = _deep_extract_from_json(nested, depth + 1, max_depth)
                if got:
                    return got
    elif isinstance(data, list):
        for item in reversed(data):
            if isinstance(item, (dict, list)):
                got = _deep_extract_from_json(item, depth + 1, max_depth)
                if got:
                    return got
        # Also check direct string items in arrays (Gradio, HuggingFace, prompt lists)
        for item in data:
            if isinstance(item, str):
                sval = item.strip()
                if (
                    len(sval) >= 2
                    and looks_like_user_prompt(sval)
                    and not _is_opaque_wire_blob(sval)
                    and not _is_clear_protocol_junk(sval)
                ):
                    got = _clean_prompt_text(sval)
                    if got:
                        return got
        return None


def _prompt_from_json_string(sval: str) -> str | None:
    """Form / multipart field whose value is site JSON (GraphQL variables, HuggingChat data)."""
    s = (sval or "").strip()
    if not s.startswith(("{", "[")):
        return None
    try:
        data = json.loads(s)
    except Exception:
        return None
    if not isinstance(data, (dict, list)):
        return None
    return _deep_extract_from_json(data) or _extract_from_json(data)


_UUID_LEAF = re.compile(r"(\$a?|\$)?[0-9a-fA-F]{4,16}(?:-[0-9a-fA-F]{4,16}){2,6}", re.I)
_ID_LEAF = re.compile(r"[A-Za-z_][A-Za-z0-9_.-]{0,40}")


def _json_scalar_leaves(node, depth: int = 0) -> list[str]:
    if depth > 8:
        return []
    if isinstance(node, bool) or node is None:
        return []
    if isinstance(node, (str, int, float)):
        return [str(node)]
    out: list[str] = []
    items = node.values() if isinstance(node, dict) else node if isinstance(node, list) else []
    for item in items:
        out.extend(_json_scalar_leaves(item, depth + 1))
    return out


def _is_ignorable_json_leaf(s: str) -> bool:
    s = (s or "").strip()
    if not s:
        return True
    if _UUID_LEAF.fullmatch(s):
        return True
    if s.isdigit():
        return len(s) <= 3
    return bool(_ID_LEAF.fullmatch(s)) and not re.search(r"\d{3,}", s)


def _unwrap_json_prompt(text: str | None) -> str | None:
    """'{"text": "hi"}' → 'hi' when the wrapper holds nothing else a DLP rule could match."""
    t = (text or "").strip()
    if not t.startswith(("{", "[")):
        return text
    try:
        data = json.loads(t)
    except Exception:
        return text
    if not isinstance(data, (dict, list)):
        return text
    inner = _deep_extract_from_json(data) or _extract_from_json(data)
    if not inner or inner.strip() == t:
        return text
    inner_s = inner.strip()
    others = [leaf for leaf in _json_scalar_leaves(data) if leaf.strip() and leaf.strip() not in inner_s]
    if any(not _is_ignorable_json_leaf(leaf) for leaf in others):
        return text
    return inner


def _regex_extract_prompt_from_text(text: str) -> str | None:
    """Last-resort: pull known JSON keys from raw text without full parse.
    Also handles single-quoted JSON variants and key: value (no-quote) formats."""
    if not text or len(text) < 2:
        return None
    best = None
    best_len = 0
    for key in _UNIVERSAL_PROMPT_KEYS:
        # Standard double-quoted JSON
        pat = rf'"{re.escape(key)}"\s*:\s*"((?:[^"\\]|\\.)*)"'
        for m in re.finditer(pat, text):
            cand = _clean_prompt_text(m.group(1).replace("\\n", "\n").replace('\\"', '"'))
            if cand and looks_like_user_prompt(cand) and not _is_opaque_wire_blob(cand):
                if len(cand) > best_len:
                    best = cand
                    best_len = len(cand)
        # Numeric values (e.g. "count": 42)
        pat2 = rf'"{re.escape(key)}"\s*:\s*(\d[\d.]*)'
        for m in re.finditer(pat2, text):
            cand = _clean_prompt_text(m.group(1))
            if cand and looks_like_user_prompt(cand):
                if len(cand) > best_len:
                    best = cand
                    best_len = len(cand)
    return best


# ─────────────────────────────────────────────
# Multi-Format Extractors (URL-encoded / Multipart / GraphQL / XML / NDJSON)
# ─────────────────────────────────────────────

def _extract_from_urlencoded(text: str) -> str | None:
    """Extract prompt from application/x-www-form-urlencoded bodies.

    Example: query=hello+world&lang=en&model=gpt4
    Covers: many custom AI APIs, enterprise chatbots, form-POST AI tools.
    """
    if not text:
        return None
    try:
        qs = urllib.parse.parse_qs(text, keep_blank_values=False)
        best = None
        best_len = 0
        for key in _UNIVERSAL_PROMPT_KEYS:
            vals = qs.get(key) or qs.get(key.lower()) or qs.get(key.upper())
            if vals:
                for v in vals:
                    got = _prompt_from_json_string(v) or _clean_prompt_text(v)
                    if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                        if len(got) > best_len:
                            best = got
                            best_len = len(got)
        if best:
            return best
        # Handle f.req separately — never treat raw batchexecute envelope as user text!
        if "f.req" in qs:
            for v in qs["f.req"]:
                batchexecute_prompt = extract_batchexecute_prompt("f.req=" + v)
                if batchexecute_prompt and looks_like_user_prompt(batchexecute_prompt) and not _is_opaque_wire_blob(batchexecute_prompt):
                    return _clean_prompt_text(batchexecute_prompt)
        # Fallback: long user-like text (skip wire tokens).
        for k, vals in qs.items():
            if k in ("f.req", "req0___data__", "___data__", "soc-app", "soc-platform", "at", "f.sid"):
                continue
            for v in vals:
                got = _prompt_from_json_string(v) or _clean_prompt_text(v)
                if got and len(got) >= 3 and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                    if len(got) > best_len:
                        best = got
                        best_len = len(got)
        return best
    except Exception:
        return None


def _extract_from_multipart(text: str) -> str | None:
    """Extract user prompt text from multipart/form-data bodies.

    Handles: file + text caption sends (ChatGPT drag-drop, custom AI with file attachment).
    Skips binary file parts; returns only the text field content.
    """
    if not text:
        return None
    try:
        best = None
        best_len = 0
        # Find boundary parts
        parts = re.split(r'--[A-Za-z0-9\-_]{10,}', text)
        for part in parts:
            if not part.strip() or part.strip() == '--':
                continue
            # Skip file parts (Content-Type: application/... or image/... or has filename=)
            header_end = part.find('\r\n\r\n') if '\r\n\r\n' in part else part.find('\n\n')
            if header_end == -1:
                continue
            header = part[:header_end].lower()
            if 'filename=' in header:
                continue
            # Skip non-text content types
            if re.search(r'content-type:\s*(?:application/(?!json)|image/|audio/|video/|binary)', header):
                continue
            # Extract name= field to check against known keys
            name_m = re.search(r'name="?([^"\r\n;]+)"?', header)
            field_name = name_m.group(1).lower().strip() if name_m else ""
            body_part = part[header_end:].strip('\r\n ')
            if not body_part:
                continue
            # If field name matches a known prompt key — accept directly
            if field_name in _UNIVERSAL_PROMPT_KEYS or field_name in {k.lower() for k in _UNIVERSAL_PROMPT_KEYS}:
                got = _prompt_from_json_string(body_part) or _clean_prompt_text(body_part)
                if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                    if len(got) > best_len:
                        best = got
                        best_len = len(got)
            else:
                # Generic text field — filter carefully
                got = _prompt_from_json_string(body_part) or _clean_prompt_text(body_part)
                if got and len(got) >= 3 and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                    if ' ' in got or len(got) >= 10:  # needs some substance
                        if len(got) > best_len:
                            best = got
                            best_len = len(got)
        return best
    except Exception:
        return None


def _extract_from_graphql(data) -> str | None:
    """Extract user prompt from GraphQL request bodies.

    Handles:
      {"query": "...", "variables": {"input": "hello"}}
      {"operationName": "SendMessage", "variables": {"message": "hello"}}
    Covers: GraphQL-based AI APIs (Poe, some enterprise AI, custom frontends).
    """
    if not isinstance(data, dict):
        return None
    try:
        # Check GraphQL variables for known prompt keys.
        variables = data.get("variables") or data.get("input") or {}
        if isinstance(variables, str) and variables.strip().startswith("{"):
            try:
                variables = json.loads(variables)
            except Exception:
                pass
        if isinstance(variables, dict):
            best = None
            best_len = 0
            for key in _UNIVERSAL_PROMPT_KEYS:
                val = variables.get(key) or variables.get(key.lower())
                if isinstance(val, (str, int, float)):
                    got = _clean_prompt_text(str(val))
                    if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                        if len(got) > best_len:
                            best = got
                            best_len = len(got)
                elif isinstance(val, dict):
                    got = _deep_extract_from_json(val)
                    if got and len(got) > best_len:
                        best = got
                        best_len = len(got)
            if best:
                return best
            # Recursive walk inside variables
            got = _deep_extract_from_json(variables)
            if got:
                return got

        # Nested input objects
        for nest_key in ("input", "request", "args", "params", "data"):
            nested = data.get(nest_key)
            if isinstance(nested, dict):
                for key in _UNIVERSAL_PROMPT_KEYS:
                    val = nested.get(key)
                    if isinstance(val, (str, int, float)):
                        got = _clean_prompt_text(str(val))
                        if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                            return got
    except Exception:
        pass
    return None


def _extract_from_xml(text: str) -> str | None:
    """Extract user prompt from XML / SOAP request bodies.

    Handles:
      <query>hello world</query>
      <message><text>hello</text></message>
      SOAP envelopes wrapping AI chat requests.
    Covers: Enterprise AI APIs using SOAP/REST XML, legacy systems.
    """
    if not text or not text.lstrip().startswith('<'):
        return None
    try:
        clean_xml = re.sub(r'<(/?)(\w+):', r'<\1', text.strip())
        root = ET.fromstring(clean_xml)
        prompt_tag_names = {k.lower() for k in _UNIVERSAL_PROMPT_KEYS}
        best = None
        best_len = 0

        def _walk(node):
            nonlocal best, best_len
            tag = re.sub(r'\{[^}]+\}', '', node.tag or '').split(':')[-1].lower()
            if tag in prompt_tag_names:
                val = (node.text or '').strip()
                if val:
                    got = _clean_prompt_text(val)
                    if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                        if len(got) > best_len:
                            best = got
                            best_len = len(got)
            for child in node:
                _walk(child)

        _walk(root)
        if best:
            return best
    except Exception as _xml_ex:
        print(f"[Gateway Proxy DEBUG] XML prompt extract failed ({type(_xml_ex).__name__}): {_xml_ex}")

    prompt_tags = r'question|query|prompt|text|message|input|user_input|instruction'
    pat = rf'<(?:\w+:)?({prompt_tags})[^>]*>([^<]+)</(?:\w+:)?\1>'
    m = re.search(pat, text, re.IGNORECASE)
    if m:
        val = m.group(2).strip()
        got = _clean_prompt_text(val)
        if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
            return got

    return None


def _extract_from_ndjson(text: str) -> str | None:
    """Extract user prompt from NDJSON / JSON Lines bodies.

    Handles:
      {"role": "user", "content": "hello"}\\n{"role": "system", ...}
      Streaming request formats that send one JSON object per line.
    Covers: Some LLM servers (TGI, vLLM streaming input), custom AI streaming.
    """
    if not text:
        return None
    best = None
    best_len = 0
    try:
        for line in text.splitlines():
            line = line.strip()
            if not line or line[0] not in '{[':
                continue
            try:
                obj = json.loads(line)
                role = str(obj.get("role") or obj.get("author") or obj.get("sender") or "").lower() if isinstance(obj, dict) else ""
                if role in ("system", "assistant", "bot", "model", "ai"):
                    continue
                got = _deep_extract_from_json(obj)
                if got and looks_like_user_prompt(got) and not _is_opaque_wire_blob(got):
                    if role in ("user", "human", "customer", "client"):
                        return got
                    if len(got) > best_len:
                        best = got
                        best_len = len(got)
            except Exception:
                continue
    except Exception as _ndjson_ex:
        print(f"[Gateway Proxy DEBUG] NDJSON prompt extract failed ({type(_ndjson_ex).__name__}): {_ndjson_ex}")
    return best


def extract_prompt_from_query_string(url: str) -> str | None:
    """Extract prompt from GET query params on any monitored domain."""
    try:
        parsed = urllib.parse.urlparse(url or "")
        qs = urllib.parse.parse_qs(parsed.query, keep_blank_values=False)
        for key in _UNIVERSAL_PROMPT_KEYS:
            vals = qs.get(key) or qs.get(key.lower())
            if vals and isinstance(vals[0], str):
                raw_val = vals[0]
                if "%" in raw_val:
                    raw_val = urllib.parse.unquote(raw_val)
                got = _prompt_from_json_string(raw_val) or _clean_prompt_text(raw_val)
                if got and looks_like_user_prompt(got):
                    return got
        for k, vals in qs.items():
            for raw_val in vals:
                if "%" in raw_val:
                    raw_val = urllib.parse.unquote(raw_val)
                if raw_val.strip().startswith(("{", "[")):
                    got = _prompt_from_json_string(raw_val)
                    if got and looks_like_user_prompt(got):
                        return got
    except Exception:
        pass
    return None


def extract_protobuf_strings(data: bytes) -> list[str]:
    """Parse arbitrary protobuf message and extract all length-delimited (wire_type 2) strings."""
    if not data or len(data) < 2:
        return []
    strings: list[str] = []
    idx = 0
    n = len(data)
    while idx < n:
        tag = 0
        shift = 0
        while idx < n:
            b = data[idx]
            idx += 1
            tag |= (b & 0x7F) << shift
            if not (b & 0x80):
                break
            shift += 7
        wire_type = tag & 0x07
        field_num = tag >> 3
        if field_num == 0:
            break
        if wire_type == 0:  # varint
            while idx < n and (data[idx] & 0x80):
                idx += 1
            idx += 1
        elif wire_type == 1:  # 64-bit
            idx += 8
        elif wire_type == 2:  # length-delimited (string / bytes / submessage)
            length = 0
            shift = 0
            while idx < n:
                b = data[idx]
                idx += 1
                length |= (b & 0x7F) << shift
                if not (b & 0x80):
                    break
                shift += 7
            if idx + length > n or length < 0:
                break
            chunk = data[idx : idx + length]
            idx += length
            try:
                s = chunk.decode("utf-8")
                if any(c.isprintable() for c in s) and not any(ord(c) < 32 and c not in "\n\r\t" for c in s):
                    strings.append(s)
            except Exception:
                pass
            if length >= 2:
                strings.extend(extract_protobuf_strings(chunk))
        elif wire_type == 5:  # 32-bit
            idx += 4
        else:
            break
    return strings


def _is_claude_wire_noise(s: str) -> bool:
    t = (s or "").strip()
    if not t:
        return True
    if _is_chat_metadata_token(t):
        return True
    if any(x in t for x in ("anthropic.connect.", "ConversationService", "PerformAction", "ReportViewing", "GetConversation", "ListConversations", "RecordAction")):
        return True
    if t.startswith("type.googleapis.com/") or t.startswith("anthropic.connect."):
        return True
    # Lone model identifiers (e.g. claude-3-5-sonnet-20241022)
    if re.fullmatch(r"claude-[0-9a-z\.\-]+", t, re.IGNORECASE) or re.fullmatch(r"anthropic\.[0-9a-z\.\-]+", t, re.IGNORECASE):
        return True
    # Claude internal entity IDs: org_..., chat_..., msg_...
    if re.fullmatch(r"(?:org|chat|msg|user)_[0-9a-zA-Z]{12,}", t):
        return True
    # Standard role and MIME tokens (numbers like '0', '1', '42' are real user inputs, never noise)
    if t.lower() in ("text", "text/plain", "user", "assistant", "human", "model", "application/json", "application/connect+proto", "true", "false", "null", "undefined"):
        return True
    # Single non-digit characters (protobuf field tags, varint wire bytes like 'b', 'r')
    if len(t) == 1 and t.isalpha() and ord(t) < 128:
        return True
    # Short alphabetic tokens that are not common English words (e.g. "zz").
    # Keep user symbols/numbers: #1, c++, x=1, ?, +, $50 — those are real prompts.
    if len(t) <= 2 and t.isalpha() and t.lower() not in (
        "hi", "ok", "no", "go", "me", "we", "he", "it", "is", "in", "on", "at",
        "to", "by", "if", "my", "or", "up", "so", "do", "am", "an", "as",
    ):
        return True
    # Client locale tags with or without trailing wire characters (e.g. 'en-US', 'en-USz qBudp')
    if re.match(r"^[a-z]{2}[-_][A-Za-z]{2,4}", t, re.IGNORECASE):
        return True
    # Claude RPC actions, event types, UI view tokens
    if t.lower() in (
        "performaction", "reportviewing", "getconversation", "listconversations",
        "recordaction", "view", "click", "select", "focus", "blur", "scroll",
        "change", "input", "submit", "ack", "sync",
    ):
        return True
    # Filenames embedded in protobuf (not user chat prompts).
    # Do NOT treat full sentences that merely end with .pdf/.txt as filenames —
    # e.g. "Please review quarterly_report.pdf" is a real Claude caption.
    if _looks_like_filename_only(t):
        return True
    if " " not in t and "\n" not in t and _POSITIONAL_NAME_RE.match(t):
        return True
    # Document body dumps from attached files
    if _looks_like_document_body_dump(t):
        return True
    # Lone index brackets [0], [1]
    if re.fullmatch(r"\[\d+\]", t):
        return True
    # Only drop known RPC enum tokens like ACTION_TYPE_..., not user prompts
    if t.isupper() and t.startswith(("ACTION_", "RPC_", "SERVICE_", "EVENT_", "STATUS_", "TYPE_")) and len(t) < 40:
        return True
    if _is_internal_wire_text(t) or _is_opaque_wire_blob(t):
        return True
    return False


def _filter_and_pick_claude_prompt(candidates: list[str]) -> str | None:
    valid = []
    for s in candidates:
        s_clean = (s or "").strip()
        if (
            not s_clean
            or _is_claude_wire_noise(s_clean)
            or _looks_like_document_body_dump(s_clean)
            or _looks_like_filename_only(s_clean)
            or not looks_like_user_prompt(s_clean)
        ):
            continue
        valid.append(s_clean)
    if not valid:
        return None
    # When a document is attached, protobuf payload contains both the extracted document
    # and the user-typed prompt. Prioritize non-dump strings (the actual user prompt).
    non_dumps = [c for c in valid if not _looks_like_document_body_dump(c)]
    if non_dumps:
        return _pick_best_user_text(non_dumps) or non_dumps[-1]
    return _pick_best_user_text(valid) or valid[-1]


def extract_connect_rpc_prompt(body_bytes: bytes, content_type: str = "", host: str = "", url: str = "") -> str | None:
    """Extract prompt from Connect-RPC / gRPC-Web / Protobuf payloads (e.g. Claude Web PerformAction)."""
    if not body_bytes or len(body_bytes) < 4:
        return None
    url_l = (url or "").lower()
    if _path_has_ignore_pattern(url_l):
        return None
    ct_l = (content_type or "").lower()

    is_rpc = (
        "claudeai-rpc" in url_l
        or "anthropic." in url_l
        or "performaction" in url_l
        or "connect" in ct_l
        or "grpc" in ct_l
        or "proto" in ct_l
        or (len(body_bytes) >= 5 and body_bytes[0] in (0, 1) and 0 < int.from_bytes(body_bytes[1:5], "big") <= len(body_bytes) - 5)
    )
    if not is_rpc and not ("claude" in (host or "").lower()):
        return None

    data = body_bytes
    if "grpc-web-text" in ct_l or (data[:2] == b"AA" and len(data) % 4 == 0):
        try:
            import base64
            data = base64.b64decode(data)
        except Exception:
            pass

    payloads = []
    idx = 0
    while idx + 5 <= len(data):
        flag = data[idx]
        if flag not in (0, 1, 2):
            break
        frame_len = int.from_bytes(data[idx+1:idx+5], "big")
        if frame_len <= 0 or idx + 5 + frame_len > len(data):
            break
        frame_data = data[idx+5 : idx+5+frame_len]
        idx += 5 + frame_len
        if flag == 1:
            try:
                import gzip
                frame_data = gzip.decompress(frame_data)
            except Exception:
                try:
                    import zlib
                    frame_data = zlib.decompress(frame_data, 16 + zlib.MAX_WBITS)
                except Exception:
                    pass
        payloads.append(frame_data)

    if not payloads:
        payloads = [data]

    for p in payloads:
        p_strip = p.strip()
        if p_strip.startswith((b"{", b"[")):
            try:
                j = json.loads(p.decode("utf-8", errors="ignore"))
                got = _deep_extract_from_json(j) or _extract_from_json(j)
                if got and looks_like_user_prompt(got) and not _is_claude_wire_noise(got):
                    return got
            except Exception:
                pass

        pb_strings = extract_protobuf_strings(p)
        if pb_strings:
            cand = _filter_and_pick_claude_prompt(pb_strings)
            if cand:
                return cand

        runs = _printable_runs(p)
        if runs:
            cand = _filter_and_pick_claude_prompt(runs)
            if cand:
                return cand

    return None


# ─────────────────────────────────────────────
# Specialized AI Protocol & Wire Decoders
# ─────────────────────────────────────────────

def _printable_runs(raw: bytes) -> list[str]:
    """Pull UTF-8 / ASCII strings out of protobuf or mixed binary bodies."""
    if not raw:
        return []
    text = raw.decode("utf-8", errors="ignore")
    chunks: list[str] = []
    for m in re.finditer(r"[\x20-\x7e\u00a0-\uffff]{3,4000}", text):
        s = (m.group(0) or "").strip()
        if s and len(s) >= 3:
            chunks.append(s)
    return chunks


def _extract_from_socketio(text: str) -> str | None:
    """Extract user prompt from Socket.IO / Engine.IO packet streams (v2, v3, v4).
    
    Covers: OpenWebUI, LibreChat, Botpress, Flowise, Rasa, custom WebSocket bots.
    Packets:
      42["chat", {"message": "hello"}]
      42/custom_ns,["event", {"content": "hello"}]
      420["prompt", "hello world"]
      43[{"reply": "..."}]
    """
    if not text:
        return None
    stripped = text.strip()
    m = re.match(r'^\d+(?:/[^,]*,\s*)?', stripped)
    if not m:
        return None
    payload_str = stripped[m.end():].strip()
    if not payload_str.startswith(("[", "{")):
        return None
    try:
        data = json.loads(payload_str)
    except Exception:
        return None
    if isinstance(data, list):
        args = data[1:] if len(data) > 1 else data
        for arg in args:
            if isinstance(arg, (dict, list)):
                got = _deep_extract_from_json(arg)
                if got:
                    return got
            elif isinstance(arg, str):
                s = arg.strip()
                if len(s) >= 2 and looks_like_user_prompt(s) and not _is_opaque_wire_blob(s) and not _is_clear_protocol_junk(s):
                    got = _clean_prompt_text(s)
                    if got:
                        return got
    elif isinstance(data, dict):
        return _deep_extract_from_json(data)
    return None


def _extract_from_mcp_or_jsonrpc(data: dict) -> str | None:
    """Extract prompt from Model Context Protocol (MCP) and JSON-RPC 2.0 payloads.
    
    Covers: Claude Desktop MCP, Cursor MCP, Zed, Continue, Sourcegraph Cody, OpenAI Tools.
    Handles:
      {"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "...", "arguments": {"query": "..."}}}
      {"jsonrpc": "2.0", "method": "prompts/get", "params": {"name": "...", "arguments": {"user_input": "..."}}}
      {"jsonrpc": "2.0", "method": "sampling/createMessage", "params": {"messages": [...]}}
    """
    if not isinstance(data, dict):
        return None
    if "jsonrpc" not in data and "method" not in data:
        return None
    params = data.get("params")
    if isinstance(params, str) and params.strip().startswith("{"):
        try:
            params = json.loads(params)
        except Exception:
            pass
    if isinstance(params, dict):
        args = params.get("arguments") or params.get("args")
        if isinstance(args, str) and args.strip().startswith("{"):
            try:
                args = json.loads(args)
            except Exception:
                pass
        if isinstance(args, dict):
            got = _deep_extract_from_json(args)
            if got:
                return got
            for val in args.values():
                if isinstance(val, str) and len(val.strip()) >= 2 and looks_like_user_prompt(val.strip()):
                    return _clean_prompt_text(val.strip())
        if "messages" in params or "message" in params:
            got = _deep_extract_from_json(params)
            if got:
                return got
        return _deep_extract_from_json(params)
    return None


def _extract_from_gradio(data: dict) -> str | None:
    """Extract prompt from Gradio / Hugging Face Spaces API calls.
    
    Covers: /api/predict, /gradio_api/call, /run/predict, HF Spaces, SD WebUI.
    Payload:
      {"data": ["how to train a model", 0.7, 50, true], "fn_index": 0}
    """
    if not isinstance(data, dict) or "data" not in data or not isinstance(data["data"], list):
        return None
    items = data["data"]
    candidates = []
    for item in items:
        if isinstance(item, str):
            s = item.strip()
            if (
                len(s) >= 2
                and looks_like_user_prompt(s)
                and not _is_opaque_wire_blob(s)
                and not _is_clear_protocol_junk(s)
            ):
                cand = _clean_prompt_text(s)
                if cand:
                    candidates.append(cand)
        elif isinstance(item, dict):
            got = _deep_extract_from_json(item)
            if got:
                candidates.append(got)
    if candidates:
        return _pick_best_user_text(candidates) or candidates[0]
    return None


def _extract_from_streamlit(data: dict) -> str | None:
    """Extract prompt from Streamlit session WebSocket rerun packets.
    
    Covers: Streamlit AI apps, ChatBot UI components, st.chat_input.
    Payload:
      {"type": "backMsg", "rerun": {"clientState": {"widgetStates": [{"stringValue": "..."}]}}}
      {"widgetStates": [{"id": "...", "stringValue": "user prompt"}]}
    """
    if not isinstance(data, dict):
        return None
    widget_states = None
    if "widgetStates" in data:
        widget_states = data["widgetStates"]
    elif "rerun" in data and isinstance(data["rerun"], dict):
        cs = data["rerun"].get("clientState")
        if isinstance(cs, dict) and "widgetStates" in cs:
            widget_states = cs["widgetStates"]
    elif "clientState" in data and isinstance(data["clientState"], dict):
        widget_states = data["clientState"].get("widgetStates")
        
    if isinstance(widget_states, list):
        for w in widget_states:
            if isinstance(w, dict):
                for val_key in ("stringValue", "string_value", "value", "prompt", "text", "query"):
                    v = w.get(val_key)
                    if isinstance(v, str) and len(v.strip()) >= 1 and looks_like_user_prompt(v.strip()):
                        return _clean_prompt_text(v.strip())
    return None


def _extract_from_msgpack(body_bytes: bytes) -> str | None:
    """Extract prompt from MessagePack (msgpack) binary format.
    
    Covers: Python/FastAPI/Node AI backends, Celery AI pipelines, Neovim LLMs,
            application/x-msgpack, application/msgpack.
    """
    if not body_bytes or len(body_bytes) < 3:
        return None
    first = body_bytes[0]
    is_msgpack_shape = (0x80 <= first <= 0x8f) or (0x90 <= first <= 0x9f) or first in (0xde, 0xdf, 0xdc, 0xdd)
    if not is_msgpack_shape:
        return None
    try:
        import msgpack
        unpacked = msgpack.unpackb(body_bytes, raw=False, strict_map_key=False)
        if isinstance(unpacked, (dict, list)):
            return _deep_extract_from_json(unpacked)
    except Exception:
        pass
    return None


def _extract_from_base64_payload(text: str) -> str | None:
    """Extract prompt from Base64 data URLs or Base64 wrapped strings.
    
    Covers: data:application/json;base64,... or pure base64-encoded request bodies.
    """
    if not text:
        return None
    s = text.strip()
    payload = None
    if s.startswith("data:") and ";base64," in s:
        payload = s.split(";base64,", 1)[1].strip()
    elif len(s) >= 16 and len(s) % 4 == 0 and re.match(r'^[A-Za-z0-9+/]+={0,2}$', s):
        payload = s
    if not payload:
        return None
    try:
        import base64
        decoded = base64.b64decode(payload)
        d_text = decoded.decode("utf-8", errors="ignore").strip()
        if d_text.startswith(("{", "[")):
            data = json.loads(d_text)
            if isinstance(data, (dict, list)):
                return _deep_extract_from_json(data)
        elif len(d_text) >= 2 and looks_like_user_prompt(d_text) and not _is_opaque_wire_blob(d_text):
            return _clean_prompt_text(d_text)
    except Exception:
        pass
    return None


def _extract_from_binary_stream_heuristics(body_bytes: bytes) -> str | None:
    """Universal string scanner fallback for ANY unknown binary protocol worldwide.
    
    Covers: CBOR, BSON, Cap'n Proto, Thrift, Avro, proprietary TCP/WebSocket AI binary wire.
    Scans printable UTF-8 chunks, filters binary headers, UUIDs, type names, and wire noise.
    """
    if not body_bytes or len(body_bytes) < 4:
        return None
    runs = _printable_runs(body_bytes)
    if not runs:
        return None
    candidates = []
    for r in runs:
        s = r.strip()
        if (
            len(s) >= 2
            and looks_like_user_prompt(s)
            and not _is_claude_wire_noise(s)
            and not _is_google_wire_blob(s)
            and not _is_opaque_wire_blob(s)
            and not _is_clear_protocol_junk(s)
            and not _looks_like_document_body_dump(s)
            and not _looks_like_filename_only(s)
        ):
            candidates.append(s)
    if candidates:
        return _pick_best_user_text(candidates) or candidates[-1]
    return None


def extract_prompt_universal(body_bytes: bytes, content_type: str = "", host: str = "", url: str = "") -> str | None:
    return _unwrap_json_prompt(_extract_prompt_universal_raw(body_bytes, content_type, host, url))


def _extract_prompt_universal_raw(body_bytes: bytes, content_type: str = "", host: str = "", url: str = "") -> str | None:
    """Universal prompt extraction for ANY admin Target Website — any protocol, any format worldwide.

    16 Comprehensive Layers in Priority Order:
      0. Connect-RPC / gRPC-Web / Protobuf (Claude Web PerformAction, Buf Connect, Envoy)
      1. SignalR / ASP.NET Core Hubs (Copilot, Bing Chat, Azure OpenAI Studio 0x1E)
      2. Socket.IO / Engine.IO v2, v3, v4 (OpenWebUI, LibreChat, Botpress, Flowise 42[...])
      3. Model Context Protocol (MCP) & JSON-RPC 2.0 (Claude MCP, Cursor, Zed tools/call)
      4. Platform-Specific Parsers (ChatGPT, Claude, Gemini batchexecute, Perplexity, DeepSeek, Copilot)
      5. Gradio & Hugging Face Spaces (/api/predict, /gradio_api/call, {"data": [...]})
      6. Streamlit AI Session (backMsg, widgetStates, stringValue)
      7. Base64 & Data URLs (data:application/json;base64, pure base64 payloads)
      8. URL Query String (GET params, nested JSON strings, %2520 unquote)
      9. Deep Recursive JSON (12 Levels, all _UNIVERSAL_PROMPT_KEYS, list scanning)
      10. GraphQL Queries & Mutations (Meta AI, Poe, GraphQL Apollo variables/inputs)
      11. Binary Serialization (MessagePack msgpack.unpackb)
      12. URL-Encoded Form (application/x-www-form-urlencoded, %25 unquoting)
      13. Multipart Form-Data (boundary parsing, text fields & upload captions)
      14. XML / SOAP / XHTML Envelopes (<prompt>, <query>, <message>)
      15. NDJSON / JSON Lines / JSON-Seq (RFC 7464, Ollama, vLLM, TGI streaming)
      16. Plain Text & Universal Binary Heuristic Fallback (Raw text, regex, UTF-8 scanner)
    """
    # ── Layer 8: URL query string (check FIRST for GET requests) ────────────
    if url:
        got = extract_prompt_from_query_string(url)
        if got:
            return got

    if not body_bytes:
        return None

    ct = (content_type or "").lower()

    # ── Layer 0: Connect-RPC / gRPC-Web / Protobuf (Claude Web PerformAction, etc.) ──
    rpc_prompt = extract_connect_rpc_prompt(body_bytes, content_type, host, url)
    if rpc_prompt:
        return rpc_prompt

    # ── Layer 11 (Early Binary): MessagePack (msgpack) ─────────────────────
    if "msgpack" in ct or (len(body_bytes) >= 3 and ((0x80 <= body_bytes[0] <= 0x8f) or (0x90 <= body_bytes[0] <= 0x9f) or body_bytes[0] in (0xde, 0xdf, 0xdc, 0xdd))):
        mp_prompt = _extract_from_msgpack(body_bytes)
        if mp_prompt:
            return mp_prompt

    try:
        text = body_bytes.decode("utf-8", errors="ignore")
    except Exception:
        text = ""

    if not text.strip():
        # Binary payload that failed UTF-8 decode — try Layer 16 binary heuristic
        return _extract_from_binary_stream_heuristics(body_bytes)

    stripped = text.lstrip()

    # ── Layer 1: SignalR / ASP.NET Hubs (\x1e record separator) ───────────
    if "\x1e" in text:
        event_got = extract_event_send_prompt(text)
        if event_got:
            return _clean_prompt_text(event_got)

    # ── Layer 2: Socket.IO / Engine.IO (42[...], 420[...], 42/ns,[...]) ────
    if re.match(r'^\d+(?:/[^,]*,\s*)?[\[\{]', stripped):
        sio_prompt = _extract_from_socketio(stripped)
        if sio_prompt:
            return sio_prompt

    # ── Layer 5: Gradio / Hugging Face Spaces (check before generic JSON walk) ─
    if stripped.startswith("{") and '"data"' in stripped:
        try:
            g_data = json.loads(text)
            if isinstance(g_data, dict) and "data" in g_data and isinstance(g_data["data"], list):
                g_got = _extract_from_gradio(g_data)
                if g_got:
                    return g_got
        except Exception:
            pass

    # ── Layer 3: Model Context Protocol (MCP) & JSON-RPC 2.0 ───────────────
    if stripped.startswith("{") and ('"jsonrpc"' in stripped or '"method"' in stripped):
        try:
            mcp_data = json.loads(text)
            if isinstance(mcp_data, dict):
                mcp_got = _extract_from_mcp_or_jsonrpc(mcp_data)
                if mcp_got:
                    return mcp_got
        except Exception:
            pass

    # ── Layer 6: Streamlit AI Session ──────────────────────────────────────
    if stripped.startswith("{") and ('"backMsg"' in stripped or '"widgetStates"' in stripped):
        try:
            st_data = json.loads(text)
            if isinstance(st_data, dict):
                st_got = _extract_from_streamlit(st_data)
                if st_got:
                    return st_got
        except Exception:
            pass

    # ── Layer 4: Platform-specific parsers (most accurate for known shapes) ─
    got = extract_prompt(body_bytes, content_type, host=host)
    if got:
        got_s = (got or "").strip()
        if (
            got_s
            and not got_s.startswith("<")
            and "webkitformboundary" not in got_s.lower()[:80]
            and not _is_clear_protocol_junk(got_s)
        ):
            return got

    # ── Layer 7: Base64 & Data URLs ─────────────────────────────────────────
    if stripped.startswith("data:") or (len(stripped) >= 16 and re.match(r'^[A-Za-z0-9+/=]{16,}$', stripped.strip())):
        b64_prompt = _extract_from_base64_payload(stripped)
        if b64_prompt:
            return b64_prompt

    # ── Layer 9, 3, 5, 6, 10: JSON — deep recursive walk & sub-protocols ───
    if stripped.startswith(("{", "[")):
        try:
            data = _loads_json_maybe_signalr(text)
            if data is not None:
                if isinstance(data, dict):
                    # Layer 3: Model Context Protocol (MCP) & JSON-RPC
                    mcp_got = _extract_from_mcp_or_jsonrpc(data)
                    if mcp_got:
                        return mcp_got
                    # Layer 5: Gradio / Hugging Face Spaces
                    gradio_got = _extract_from_gradio(data)
                    if gradio_got:
                        return gradio_got
                    # Layer 6: Streamlit AI Session
                    st_got = _extract_from_streamlit(data)
                    if st_got:
                        return st_got
                    # Layer 10: GraphQL variables
                    if "variables" in data:
                        got = _extract_from_graphql(data)
                        if got:
                            return got

                # Layer 9: Deep recursive walk (12 levels, all _UNIVERSAL_PROMPT_KEYS)
                got = _deep_extract_from_json(data)
                if got:
                    return got
                if isinstance(data, dict):
                    got = _extract_from_graphql(data)
                    if got:
                        return got
                    got = _extract_from_json(data)
                    if got:
                        return got
        except Exception as _json_ex:
            print(f"[Gateway Proxy DEBUG] prompt JSON extract failed ({type(_json_ex).__name__}): {_json_ex}")

    # ── Layer 12: URL-encoded form body ────────────────────────────────────
    if (
        "urlencoded" in ct
        or "form" in ct
        or ("=" in text and "&" in text and not stripped.startswith(("{", "[", "<")))
    ):
        norm_text = urllib.parse.unquote(text) if "%25" in text else text
        got = _extract_from_urlencoded(norm_text)
        if got:
            return got

    # ── Layer 13: Multipart form-data ───────────────────────────────────────
    if "multipart" in ct or "boundary" in ct or "webkitformboundary" in text[:400].lower():
        got = _extract_from_multipart(text)
        if got:
            return got

    # ── Layer 14: XML / SOAP ────────────────────────────────────────────────
    if stripped.startswith("<") or "xml" in ct or "soap" in ct:
        got = _extract_from_xml(text)
        if got:
            return got

    # ── Layer 15: NDJSON / JSON Lines / JSON-Seq ───────────────────────────
    if "\n" in text.strip() and not stripped.startswith(("{", "[", "<")):
        got = _extract_from_ndjson(text)
        if got:
            return got
    elif "\n" in text.strip() and stripped.startswith(("{", "[")):
        got = _extract_from_ndjson(text)
        if got:
            return got

    # ── Layer 16: Plain text body ───────────────────────────────────────────
    if (
        "text/plain" in ct
        or "text/xml" in ct
        or (
            not stripped.startswith(("{", "[", "<"))
            and "=" not in text[:30]
            and len(text.strip()) >= 2
        )
    ):
        cleaned = _clean_prompt_text(text.strip())
        if (
            cleaned
            and len(cleaned) >= 2
            and len(cleaned) <= 200_000
            and looks_like_user_prompt(cleaned)
            and not _is_opaque_wire_blob(cleaned)
        ):
            return cleaned

    # ── Regex last resort ──────────────────────────────────────────────────
    regex_cand = _regex_extract_prompt_from_text(text)
    if regex_cand:
        return regex_cand

    # ── Universal Binary Heuristic Fallback (CBOR, BSON, unknown wire) ─────
    return _extract_from_binary_stream_heuristics(body_bytes)



def detect_file_upload(flow: http.HTTPFlow, raw_content: str) -> tuple[bool, str]:
    """Detect real file upload attempts — not chat JSON / + menu / Gemini prompt posts."""
    headers = flow.request.headers
    content_type = (headers.get("content-type", "") or "").lower()
    path = (flow.request.path or "").lower()
    method = (flow.request.method or "").upper()
    host = (flow.request.pretty_host or "").lower()
    body_len = len(flow.request.content or b"")
    path_only = path.split("?", 1)[0]
    raw = raw_content or ""

    # WebRTC SDP is transport, not a file upload.
    if "/realtime" in path_only or "webrtc-datachannel" in raw.lower() or 'name="sdp"' in raw.lower() or "\nv=0\r\n" in raw:
        return False, ""

    # ChatGPT / OpenAI CDN — catch before chat-path exclusions swallow file POSTs
    messages_parts_up, messages_parts_reason = detect_messages_parts_file_upload(host, path, method, content_type, body_len, flow.request.content or b"")
    if messages_parts_up:
        return True, messages_parts_reason

    # ── Never treat real chat Send as a file upload ──
    if is_batchexecute_chat_submit(path, raw) or "f.req=" in raw[:500]:
        return False, ""
    if is_rest_sse_ask_submit(path, raw):
        return False, ""
    # Claude / ChatGPT / generic chat completion paths (JSON text prompts)
    chat_path_markers = (
        "/conversation", "/completion", "/completions", "/append_message",
        "/chat_conversations", "/backend-api/f/conversation", "/v1/messages",
        "/streamgenerate", "/generatecontent", "/chat/completions",
    )
    if any(m in path_only for m in chat_path_markers):
        # Only if this request is clearly a binary/multipart file body
        if "multipart/form-data" not in content_type and "octet-stream" not in content_type:
            return False, ""
        if "filename=" not in raw.lower() and "filename*=" not in raw.lower():
            if body_len < 8192:
                return False, ""

    # Google resumable: only real byte transfer / finalize — not session "start"/"query"
    goog_cmd = (headers.get("x-goog-upload-command", "") or "").lower()
    if goog_cmd:
        if any(x in goog_cmd for x in ("upload", "finalize", "append")) and body_len >= 64:
            return True, f"Google Resumable Upload ({goog_cmd})"
        return False, ""  # start / query / cancel = not a file
    for hk, hv in headers.items():
        hk_l = (hk or "").lower()
        hv_l = (hv or "").lower()
        if hk_l.startswith("x-goog-upload") and body_len >= 2048:
            if "start" in hv_l or "query" in hv_l or "cancel" in hv_l:
                continue
            return True, f"Google Resumable Upload ({hk}={hv_l[:40]})"

    # Real multipart with an actual filename= part (picker-open empty multipart ≠ upload)
    if "multipart/form-data" in content_type or "webkitformboundary" in raw[:300].lower():
        if "filename=" in raw or "filename*=" in raw.lower():
            # Empty filename="" is not a real file
            if re.search(r'filename\*?=(?:UTF-8\'\')?["\'][^"\']+', raw[:12000], re.I):
                empty = re.search(r'filename\*?=(?:UTF-8\'\')?["\']["\']', raw[:12000], re.I)
                named = re.search(r'filename\*?=(?:UTF-8\'\')?["\']([^"\']+)["\']', raw[:12000], re.I)
                if named and (named.group(1) or "").strip():
                    return True, f"File content-type ({(content_type or 'multipart').split(';')[0]})"
                if empty and not named:
                    return False, ""
                return True, f"File content-type ({(content_type or 'multipart').split(';')[0]})"
        return False, ""

    # Upload URL paths — require binary / large non-JSON (tiny JSON handshake ≠ upload)
    path_looks_upload = _path_looks_like_upload(path_only)
    for ep in UPLOAD_ENDPOINTS:
        if ep in path_only:
            path_looks_upload = True
            break
    if path_looks_upload:
        if any(x in path_only for x in ("/files/library", "/files/process", "/files/download", "/files/list")):
            return False, ""
        is_json_body = "json" in content_type or raw.lstrip()[:1] in ("{", "[")
        if "multipart/form-data" in content_type or "octet-stream" in content_type:
            if body_len >= 64:
                return True, f"File Upload Endpoint ({path_only[:80]})"
        if not is_json_body and body_len >= 1024:
            return True, f"File Upload Endpoint ({path_only[:80]})"
        _json_file_keys = (
            '"file_name"', '"fileName"', '"filename"', '"mime_type"', '"mimeType"',
            '"bytes"', '"fileData"', '"file_data"', '"inline_data"', '"inlineData"',
            '"media_type"', '"mediaType"', "base64", '"content":', '"data":',
            '"source"', '"document"', '"attachment"',
        )
        if is_json_body and body_len >= 512 and any(k in raw for k in _json_file_keys):
            return True, f"File Upload Endpoint JSON ({path_only[:80]})"
        if is_json_body and body_len >= 80 and any(
            k in raw for k in ('"file_name"', '"fileName"', '"filename"', '"mime_type"', '"mimeType"', '"bytes"')
        ):
            return True, f"File Upload Endpoint JSON ({path_only[:80]})"
        return False, ""

    # Binary content-type on non-chat hosts — require real size + magic / upload header
    chat_submit = is_chat_path(path, host, raw)
    if not chat_submit:
        for prefix in UPLOAD_CONTENT_TYPES:
            if prefix in content_type and body_len >= 64:
                # Skip generic application/json mistaken as upload
                if prefix in ("application/pdf", "image/", "audio/", "video/", "application/octet-stream",
                              "application/msword", "application/vnd.", "text/csv", "text/tab-separated-values", "application/csv"):
                    return True, f"File content-type ({content_type.split(';')[0]})"

    # JSON attachment heuristics: NEVER on chat submit
    if raw and not chat_submit and body_len >= 2048:
        strong = any(
            k in raw
            for k in (
                '"file_name"', '"fileName"', '"mime_type"', '"mimeType"',
                '"fileData"', '"inline_data"', '"inlineData"',
                "application/vnd.openxmlformats",
            )
        )
        if strong and (
            "filename=" in raw.lower()
            or body_len >= 8192
            or any(x in raw for x in ('"bytes"', "base64", "octet-stream"))
        ):
            return True, "File Attachment Payload in Request"

    return False, ""
