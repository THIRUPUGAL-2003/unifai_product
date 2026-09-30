# Part of UnifAI browser_ai_proxy — loaded via browser_ai_proxy.py into one shared namespace.
# Do not import this file directly.
import asyncio


# Same rule/send must not paint 3 chat bubbles (Claude multi-request / multi-file).
_BLOCK_UI_DEDUPE_TTL = 15.0


def _security_reply_text(rule_triggered: str, warning_message: str = "") -> str:
    """Admin block message first; otherwise a clear default that still names the rule."""
    w = (warning_message or "").strip()
    if w:
        return w
    name = (rule_triggered or "").strip()
    if name:
        return f"Blocked by UnifAI Guard ({name})."
    return "This request was blocked by UnifAI Guard."


def _block_ui_dedupe_key(host: str, msg: str) -> tuple[str, str]:
    """Domain + normalized message key for one-visible-bubble-per-send."""
    host_l = (host or "").strip().lower()
    domain = ""
    try:
        _is_t, dom, _plat = detect_target(host_l)
        domain = (dom or "").strip().lower()
    except Exception:
        domain = ""
    if not domain:
        # Strip www. for stable key even when targets cache is empty in unit tests.
        domain = host_l[4:] if host_l.startswith("www.") else host_l
        domain = domain or "unknown"
    norm = re.sub(r"\s+", " ", (msg or "").strip().lower())[:240]
    return domain, f"block-ui|{norm}"


def _silent_block_response(flow: http.HTTPFlow, common_headers: dict, *, path: str, raw_body: str, accept: str) -> None:
    """Kill a duplicate blocked request without adding another in-chat bubble."""
    path_l = (path or "").lower()
    raw_low = (raw_body or "").lower()
    accept_l = (accept or "").lower()
    # Claude / Anthropic: empty turn (no text delta).
    if _is_anthropic_messages_api_shape(path_l, raw_body or ""):
        silent = (
            'event: message_start\n'
            'data: {"type":"message_start","message":{"id":"msg_unifai_block_dup","type":"message",'
            '"role":"assistant","content":[],"model":"unifai-guard","stop_reason":null}}\n\n'
            'event: message_delta\n'
            'data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},'
            '"usage":{"output_tokens":0}}\n\n'
            'event: message_stop\n'
            'data: {"type":"message_stop"}\n\n'
        )
        flow.response = http.Response.make(
            200,
            silent.encode("utf-8"),
            {**common_headers, "Content-Type": "text/event-stream; charset=utf-8", "X-Accel-Buffering": "no"},
        )
        return
    # ChatGPT-shaped / generic SSE: DONE with no assistant content.
    if (
        "text/event-stream" in accept_l
        or "event-stream" in accept_l
        or "/stream" in path_l
        or '"stream":true' in raw_low.replace(" ", "")
        or '"stream": true' in raw_low
        or "conversation" in path_l
    ):
        flow.response = http.Response.make(
            200,
            b"data: [DONE]\n\n",
            {**common_headers, "Content-Type": "text/event-stream; charset=utf-8"},
        )
        return
    flow.response = http.Response.make(
        200,
        b'{"id":"unifai-block-dup","object":"chat.completion","choices":[]}',
        {**common_headers, "Content-Type": "application/json; charset=utf-8"},
    )


def _warning_for_rule_name(rule_name: str) -> str:
    name = (rule_name or "").strip()
    if not name:
        return ""
    get_guard_rules()
    entry = _cached_rule_catalog.get(name)
    if entry:
        return (entry.get("warning_message") or "").strip()
    for r in _cached_rules:
        if (r.get("name") or "").strip() == name:
            return (r.get("warning_message") or "").strip()
    return ""


def inject_file_redact_notice(raw_text: str, notice: str, user_caption: str = "") -> str | None:
    """Append redaction notice to chat Send body. Log keeps original; browser gets notice."""
    notice = (notice or "").strip()
    if not notice or not raw_text:
        return None
    caption = (user_caption or "").strip()
    if caption:
        return inject_warned_prompt(raw_text, caption, _redacted_forward(caption, notice.replace("[UNIFAI REDACTED]", "").strip()))
    if notice in raw_text:
        return raw_text
    escaped = json.dumps(notice)[1:-1]
    if '"content":' in raw_text or '"text":' in raw_text:
        for needle in ('"content":"', '"text":"', '"parts":["'):
            if needle in raw_text:
                return raw_text.replace(needle, needle + escaped + "\\n\\n", 1)
    if raw_text.lstrip()[:1] in ("{", "["):
        return None
    return raw_text + "\n" + notice


def _ws_frames_event_send(reply: str) -> list[bytes]:
    frames = [
        json.dumps({"event": "received"}, ensure_ascii=False).encode("utf-8"),
        json.dumps({"event": "startMessage", "messageId": "unifai-reply"}, ensure_ascii=False).encode("utf-8"),
    ]
    step = 400
    for i in range(0, len(reply), step):
        frames.append(
            json.dumps({"event": "appendText", "text": reply[i : i + step]}, ensure_ascii=False).encode("utf-8")
        )
    frames.append(json.dumps({"event": "done"}, ensure_ascii=False).encode("utf-8"))
    return frames


def _ws_frames_openai(reply: str) -> list[bytes]:
    """ChatGPT / OpenAI-compatible / DeepSeek / many chat UIs."""
    return [
        json.dumps({
            "id": "unifai-reply",
            "object": "chat.completion.chunk",
            "choices": [{"index": 0, "delta": {"role": "assistant", "content": reply}, "finish_reason": None}],
        }, ensure_ascii=False).encode("utf-8"),
        json.dumps({
            "id": "unifai-reply",
            "object": "chat.completion.chunk",
            "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
        }, ensure_ascii=False).encode("utf-8"),
        b"[DONE]",
        json.dumps({
            "message": {
                "id": "unifai-reply",
                "author": {"role": "assistant"},
                "content": {"content_type": "text", "parts": [reply]},
                "status": "finished_successfully",
            },
            "error": None,
        }, ensure_ascii=False).encode("utf-8"),
    ]


def _ws_frames_rest_sse(reply: str) -> list[bytes]:
    return [
        json.dumps({"text": reply}, ensure_ascii=False).encode("utf-8"),
        json.dumps({"status": "completed", "text": reply, "final": True}, ensure_ascii=False).encode("utf-8"),
        json.dumps({"event": "done"}, ensure_ascii=False).encode("utf-8"),
    ]


def _ws_frames_signalr(reply: str) -> list[bytes]:
    """
    Microsoft Copilot / Bing Sydney / ASP.NET SignalR Hub protocol frames.
    SignalR strictly requires 0x1E record separator at the end of every message,
    and a type: 2 (completion item with AdaptiveCard) and type: 3 (invocation ack)
    to terminate the client-side loading spinner and render the block message.
    """
    signalr_sep = b"\x1e"
    # Type 1: Streaming update / AdaptiveCard message
    type1_update = {
        "type": 1,
        "target": "update",
        "arguments": [{
            "cursor": {"j": 0, "p": 0},
            "messages": [{
                "text": reply,
                "author": "bot",
                "messageType": "Chat",
                "contentOrigin": "DeepLeo",
                "adaptiveCards": [{
                    "type": "AdaptiveCard",
                    "version": "1.5",
                    "body": [{
                        "type": "TextBlock",
                        "text": reply,
                        "wrap": True,
                    }],
                }],
            }],
        }],
    }
    # Type 1: Append text target (Copilot web/app variations)
    type1_append = {
        "type": 1,
        "target": "append",
        "arguments": [{
            "text": reply,
            "author": "bot",
        }],
    }
    # Type 2: Completion item (Sydney / Copilot message complete)
    type2_complete = {
        "type": 2,
        "item": {
            "result": {
                "value": "Success",
                "message": reply,
            },
            "messages": [{
                "text": reply,
                "author": "bot",
                "messageType": "Chat",
                "contentOrigin": "DeepLeo",
                "adaptiveCards": [{
                    "type": "AdaptiveCard",
                    "version": "1.5",
                    "body": [{
                        "type": "TextBlock",
                        "text": reply,
                        "wrap": True,
                    }],
                }],
            }],
        },
    }
    # Type 3: Invocation complete - stops the spinning wheel / loading indicator
    type3_invocation0 = {"type": 3, "invocationId": "0"}
    type3_invocation1 = {"type": 3, "invocationId": "1"}

    return [
        json.dumps(type1_update, ensure_ascii=False).encode("utf-8") + signalr_sep,
        json.dumps(type1_append, ensure_ascii=False).encode("utf-8") + signalr_sep,
        json.dumps(type2_complete, ensure_ascii=False).encode("utf-8") + signalr_sep,
        json.dumps(type3_invocation0, ensure_ascii=False).encode("utf-8") + signalr_sep,
        json.dumps(type3_invocation1, ensure_ascii=False).encode("utf-8") + signalr_sep,
    ]


def _ws_frames_universal(reply: str) -> list[bytes]:
    """
    Multi-shape burst for unknown Target Websites.
    Clients ignore frames they don't understand; one matching shape is enough.
    """
    frames: list[bytes] = []
    frames.extend(_ws_frames_event_send(reply))
    frames.extend(_ws_frames_openai(reply))
    frames.extend(_ws_frames_rest_sse(reply))
    frames.append(json.dumps({
        "type": "message",
        "role": "assistant",
        "content": reply,
        "text": reply,
        "message": {"role": "assistant", "content": reply, "text": reply},
    }, ensure_ascii=False).encode("utf-8"))
    frames.extend(_ws_frames_signalr(reply))
    return frames


def _drop_websocket_outbound(msg) -> None:
    """Stop a client WebSocket Send from reaching the site. Empty payload if drop is late."""
    try:
        msg.drop()
    except Exception:
        try:
            msg.kill()
        except Exception:
            pass
    try:
        if hasattr(msg, "content"):
            msg.content = b""
        if hasattr(msg, "text"):
            msg.text = ""
    except Exception:
        pass


_EVENT_LOOP = None  # mitmproxy loop; set by the off-loop hook wrapper in responses_addon.py


def _run_on_event_loop(fn) -> None:
    """mitmproxy inject commands are not thread-safe — hand them to the proxy loop."""
    loop = _EVENT_LOOP
    try:
        running = asyncio.get_running_loop()
    except RuntimeError:
        running = None
    if loop is not None and running is not loop and not loop.is_closed():
        loop.call_soon_threadsafe(fn)
    else:
        fn()


def inject_websocket_reply(flow: http.HTTPFlow, host: str, reply_text: str) -> None:
    """
    Push an in-chat assistant reply over WebSocket for ANY monitored Target Website.
    Uses universal frame shapes — prioritizing SignalR if Copilot/Sydney.
    """
    reply = (reply_text or "").strip()
    if not reply or not flow.websocket:
        return

    try:
        from mitmproxy import ctx
    except Exception as e:
        print(f"[UnifAI Proxy Warning] WS inject unavailable: {e}")
        return

    host_l = (host or "").lower()
    path_l = (getattr(flow.request, "path", "") or "").lower()
    # SignalR records end with \x1e (handshake included); blocked frames are already emptied.
    is_signalr = any(x in path_l for x in ("signalr", "chathub")) or any(
        "\x1e" in (websocket_frame_text(m) or "")
        for m in list(flow.websocket.messages or [])[-40:]
    )

    if is_signalr:
        frames = _ws_frames_signalr(reply) + _ws_frames_universal(reply)
    else:
        frames = _ws_frames_universal(reply)

    def _send() -> None:
        ok = 0
        for frame in frames:
            try:
                ctx.master.commands.call("inject.websocket", flow, True, frame, True)
                ok += 1
            except Exception as e:
                print(f"[UnifAI Proxy Warning] WS inject frame failed: {e}")
                break
        print(f"[UnifAI Proxy] Injected WebSocket reply -> {host_l} ({ok}/{len(frames)} frames)")

    _run_on_event_loop(_send)


def make_blocked_response(flow: http.HTTPFlow, rule_triggered: str, host: str, reply_text: str = "") -> None:
    """
    Inject a clean in-chat security reply (HTTP 200) so the website shows a
    professional violation message instead of "Network Error".
    Formats are tailored per platform.

    Same Send often hits Guard 2–3 times (Claude multi-file / parallel requests).
    Only the first inject shows the block text; later ones stay silent so the
    bubble does not repeat and then vanish.
    """
    path = (flow.request.path or "").lower()
    host_l = (host or "").lower()
    accept = (flow.request.headers.get("Accept", "") or "").lower()
    msg = (reply_text or "").strip()
    if not msg:
        msg = "This request was blocked by UnifAI Guard."
    if "evaluation failed" in msg.lower():
        msg = "This request was blocked by UnifAI Guard."
    msg_json = json.dumps(msg)
    msg_escaped = (
        msg.replace("\\", "\\\\")
        .replace('"', '\\"')
        .replace("\n", "\\n")
    )

    # Mirror request origin dynamically for valid CORS (Origin: * + Credentials: true causes browser CORS failure / 'Internet Error')
    req_origin = flow.request.headers.get("Origin", "") or f"https://{flow.request.pretty_host}"
    req_headers = flow.request.headers.get("Access-Control-Request-Headers", "*")
    common_headers = {
        "Access-Control-Allow-Origin": req_origin,
        "Access-Control-Allow-Credentials": "true",
        "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS, PATCH",
        "Access-Control-Allow-Headers": req_headers,
        "Cache-Control": "no-cache",
        "Vary": "Origin",
    }

    raw_body = (flow.request.content or b"").decode("utf-8", errors="ignore")

    # One visible block bubble per Send (Claude was painting 3 for multi-file).
    path_l_early = (path or "").lower().split("?", 1)[0]
    is_upload_ep = _path_looks_like_upload(path_l_early) or any(
        x in path_l_early for x in ("/files", "/upload", "/attachments")
    )
    if not is_upload_ep:
        dedupe_domain, dedupe_key = _block_ui_dedupe_key(host_l, msg)
        if is_duplicate_event(dedupe_domain, dedupe_key, ttl=_BLOCK_UI_DEDUPE_TTL, mark=False):
            _silent_block_response(
                flow, common_headers, path=path, raw_body=raw_body, accept=accept,
            )
            print(f"[UnifAI Proxy] BLOCK UI deduped (silent) | {dedupe_domain} | {msg[:80]!r}")
            return
        mark_duplicate_event(dedupe_domain, dedupe_key)

    # Dedicated upload-endpoint response: return proper JSON error so SPA shows block message instead of network error
    path_l = path_l_early
    if is_upload_ep:
        upload_err_obj = {
            "error": {
                "message": msg,
                "type": "unifai_guard_blocked",
                "code": "upload_blocked",
                "rule": rule_triggered,
            },
            "detail": msg,
            "message": msg,
            "status": "blocked",
            "blocked": True,
        }
        flow.response = http.Response.make(
            400,
            json.dumps(upload_err_obj, ensure_ascii=False).encode("utf-8"),
            {**common_headers, "Content-Type": "application/json; charset=utf-8"},
        )
        return

    # Wire-format routing: detect from REQUEST SHAPE (path/body/Accept) — not hostname lists.
    messages_parts_body = None
    try:
        parsed = json.loads(raw_body or "")
        if isinstance(parsed, dict) and isinstance(parsed.get("messages"), list):
            if "conversation_id" in parsed or "parent_message_id" in parsed:
                messages_parts_body = parsed
            elif any(isinstance(m, dict) and isinstance(m.get("author"), dict) for m in parsed["messages"]):
                messages_parts_body = parsed
    except Exception:
        messages_parts_body = None

    # ── ChatGPT-shaped conversation APIs (body shape, any monitored domain) ──
    if messages_parts_body is not None:
        user_msg_id = ""
        conv_id = None
        req_data = messages_parts_body if isinstance(messages_parts_body, dict) else {}
        if not req_data:
            try:
                req_data = json.loads(flow.request.content.decode("utf-8", errors="ignore"))
            except Exception:
                req_data = {}
        if not isinstance(req_data, dict):
            req_data = {}
        conv_id = req_data.get("conversation_id")
        user_msg_id = req_data.get("parent_message_id") or ""
        msgs = req_data.get("messages") or []
        if isinstance(msgs, list):
            for m in reversed(msgs):
                if not isinstance(m, dict):
                    continue
                author = m.get("author") if isinstance(m.get("author"), dict) else {}
                if (author.get("role") or "").lower() == "user" and m.get("id"):
                    user_msg_id = m.get("id")
                    break

        import uuid
        reply_msg_id = str(uuid.uuid4())
        now_ts = time.time()

        messages_parts_resp_obj = {
            "message": {
                "id": reply_msg_id,
                "author": {"role": "assistant", "name": None, "metadata": {}},
                "create_time": now_ts,
                "update_time": None,
                "content": {"content_type": "text", "parts": [msg]},
                "status": "finished_successfully",
                "end_turn": True,
                "weight": 1.0,
                "metadata": {
                    "finish_details": {"type": "stop"},
                    "is_complete": True,
                    "model_slug": "gpt-4o",
                    "parent_id": user_msg_id or None,
                },
                "recipient": "all",
            },
            "conversation_id": conv_id,
            "error": None,
        }
        sse_payload = f"data: {json.dumps(messages_parts_resp_obj)}\n\ndata: [DONE]\n\n"
        flow.response = http.Response.make(
            200,
            sse_payload.encode("utf-8"),
            {**common_headers, "Content-Type": "text/event-stream; charset=utf-8"},
        )
        return

    # ── Claude / Anthropic chat APIs (path/body shape) ──
    # Single content_block only — do NOT also emit legacy `completion` (that painted
    # the same block text twice, and with 3 parallel Sends looked like 3 bubbles).
    if _is_anthropic_messages_api_shape(path, raw_body):
        anthropic_sse = (
            'event: message_start\n'
            'data: {"type":"message_start","message":{"id":"msg_unifai_block","type":"message",'
            '"role":"assistant","content":[],"model":"unifai-guard","stop_reason":null}}\n\n'
            'event: content_block_start\n'
            'data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}\n\n'
            'event: content_block_delta\n'
            f'data: {{"type":"content_block_delta","index":0,"delta":{{"type":"text_delta","text":{msg_json}}}}}\n\n'
            'event: content_block_stop\n'
            'data: {"type":"content_block_stop","index":0}\n\n'
            'event: message_delta\n'
            'data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}\n\n'
            'event: message_stop\n'
            'data: {"type":"message_stop"}\n\n'
        )
        flow.response = http.Response.make(
            200,
            anthropic_sse.encode("utf-8"),
            {
                **common_headers,
                "Content-Type": "text/event-stream; charset=utf-8",
                "X-Accel-Buffering": "no",
            },
        )
        return

    # ── Microsoft Copilot / Bing (request shape) ──
    if is_event_send_chat_submit(path, raw_body):
        copilot_card = {
            "type": "AdaptiveCard",
            "version": "1.5",
            "body": [{"type": "TextBlock", "text": msg, "wrap": True}],
        }
        signalr_complete = {
            "type": 2,
            "item": {
                "result": {"value": "Success"},
                "messages": [{
                    "text": msg,
                    "author": "bot",
                    "messageType": "Chat",
                    "contentOrigin": "DeepLeo",
                    "adaptiveCards": [copilot_card],
                }],
            },
        }
        signalr_ack = {"type": 3, "invocationId": "0"}

        event_send_sse = (
            f"data: {json.dumps(signalr_complete, ensure_ascii=False)}\x1e\n\n"
            f"data: {json.dumps(signalr_ack, ensure_ascii=False)}\x1e\n\n"
            f'data: {{"choices":[{{"index":0,"delta":{{"role":"assistant","content":{msg_json}}},"finish_reason":null}}]}}\n\n'
            'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\n'
            "data: [DONE]\n\n"
        )
        # Also provide a Graph-style message payload some Copilot UIs accept
        event_send_json = json.dumps({
            "result": {"value": "Success"},
            "item": {
                "result": {"value": "Success"},
                "messages": [{
                    "text": msg,
                    "author": "bot",
                    "messageType": "Chat",
                    "contentOrigin": "DeepLeo",
                    "adaptiveCards": [copilot_card],
                }],
            },
            "message": {"text": msg, "role": "assistant", "content": msg},
            "messages": [{"text": msg, "author": "bot", "role": "assistant", "content": msg}],
            "text": msg,
            "error": None,
            "unifai_blocked": True,
        }, ensure_ascii=False)
        body = event_send_sse if "event-stream" in accept or "stream" in path else event_send_json
        ctype = (
            "text/event-stream; charset=utf-8"
            if body == event_send_sse
            else "application/json; charset=utf-8"
        )
        flow.response = http.Response.make(
            200,
            body.encode("utf-8"),
            {**common_headers, "Content-Type": ctype},
        )
        return

    # ── Perplexity (request shape) ──
    if is_rest_sse_ask_submit(path, raw_body):
        pplx = (
            f'event: message\ndata: {{"text":{msg_json}}}\n\n'
            f'data: {{"status":"completed","text":{msg_json},"final":true}}\n\n'
            "data: [DONE]\n\n"
        )
        flow.response = http.Response.make(
            200,
            pplx.encode("utf-8"),
            {**common_headers, "Content-Type": "text/event-stream; charset=utf-8"},
        )
        return

    # ── Gemini / Bard (request shape) ──
    # Only inject on real chat submits. History/settings batchexecute share f.req=
    # — fake wrb.fr there leaves the sidebar spinning forever.
    if is_batchexecute_chat_submit(path, raw_body):
        # Dynamically detect actual RPC ID from raw_body (wXbdQc, vyAQhe, hR32Ce, StreamGenerate)
        detected_rpc = "StreamGenerate"
        for rpc_cand in ("wXbdQc", "vyAQhe", "hR32Ce", "StreamGenerate", "GenerateContent"):
            if rpc_cand in (raw_body or ""):
                detected_rpc = rpc_cand
                break

        payload_data = [None, [None, None, None, [[msg]]]]
        payload_str = json.dumps(payload_data, ensure_ascii=False)

        envelopes = [
            ["wrb.fr", detected_rpc, payload_str, None, None, None, "generic"],
        ]
        if detected_rpc != "StreamGenerate":
            envelopes.append(
                ["wrb.fr", "StreamGenerate", payload_str, None, None, None, "generic"]
            )
        envelopes.extend([
            ["di", 34],
            ["af.httprm", 34, "-unifai-", 1],
        ])

        batchexecute_body = ")]}'\n" + json.dumps(envelopes, ensure_ascii=False) + "\n"
        flow.response = http.Response.make(
            200,
            batchexecute_body.encode("utf-8"),
            {**common_headers, "Content-Type": "application/json; charset=utf-8"},
        )
        return

    # ── OpenAI-compatible SSE (Accept/path/body stream flag — any admin-added domain) ──
    raw_low = (raw_body or "").lower()
    wants_stream = (
        "event-stream" in accept
        or "chat/completions" in path
        or "/stream" in path
        or path.endswith("/stream")
        or "stream" in path and ("completion" in path or "chat" in path or "generate" in path)
        or '"stream":true' in raw_low.replace(" ", "")
        or '"stream": true' in raw_low
    )
    if wants_stream or "completion" in path:
        openai_sse = (
            'data: {"id":"unifai-reply","object":"chat.completion.chunk","choices":'
            '[{"index":0,"delta":{"role":"assistant","content":'
            f"{msg_json}"
            '},"finish_reason":null}]}\n\n'
            'data: {"id":"unifai-reply","object":"chat.completion.chunk","choices":'
            '[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\n'
            "data: [DONE]\n\n"
        )
        flow.response = http.Response.make(
            200,
            openai_sse.encode("utf-8"),
            {**common_headers, "Content-Type": "text/event-stream; charset=utf-8"},
        )
        return

    # ── Fallback for ANY other Target Website ──
    # Many UIs (Abacus, Poe, custom chat apps) ignore plain OpenAI JSON and keep spinning.
    # Emit a multi-shape body + SSE twin so at least one field the SPA reads shows the block message.
    multi = {
        "id": "unifai-security-block",
        "object": "chat.completion",
        "choices": [{
            "index": 0,
            "message": {"role": "assistant", "content": msg},
            "delta": {"role": "assistant", "content": msg},
            "text": msg,
            "finish_reason": "stop",
        }],
        "message": {
            "id": "unifai-security-block",
            "role": "assistant",
            "author": {"role": "assistant"},
            "content": msg,
            "text": msg,
            "parts": [msg],
            "content_type": "text",
        },
        "messages": [{"role": "assistant", "content": msg, "text": msg}],
        "text": msg,
        "content": msg,
        "answer": msg,
        "reply": msg,
        "response": msg,
        "output": msg,
        "result": {
            "content": [{"type": "text", "text": msg}],
            "message": msg,
            "text": msg,
        },
        "data": {"message": msg, "text": msg, "content": msg, "answer": msg},
        "error": None,
        "unifai": {
            "blocked": True,
            "rule": rule_triggered,
            "message": msg,
        },
        # Some SPAs surface server "detail" / "error_message" even on HTTP 200.
        "detail": msg,
        "error_message": msg,
        "status": "ok",
        "success": True,
    }
    # Prefer SSE when the path looks chatty — stops infinite "thinking" loaders on unknown sites.
    chatty_path = any(
        x in path
        for x in (
            "chat", "message", "completion", "generate", "ask", "prompt",
            "conversation", "thread", "query", "agent", "llm", "ai/",
        )
    )
    if chatty_path:
        sse_lines = (
            f"data: {json.dumps({'text': msg, 'message': msg, 'content': msg, 'role': 'assistant'}, ensure_ascii=False)}\n\n"
            f"data: {json.dumps({'choices': [{'delta': {'content': msg}, 'finish_reason': None}]}, ensure_ascii=False)}\n\n"
            f"data: {json.dumps({'choices': [{'delta': {}, 'finish_reason': 'stop'}]}, ensure_ascii=False)}\n\n"
            f"data: {json.dumps(multi, ensure_ascii=False)}\n\n"
            "data: [DONE]\n\n"
        )
        flow.response = http.Response.make(
            200,
            sse_lines.encode("utf-8"),
            {**common_headers, "Content-Type": "text/event-stream; charset=utf-8"},
        )
        return

    flow.response = http.Response.make(
        200,
        json.dumps(multi, ensure_ascii=False).encode("utf-8"),
        {**common_headers, "Content-Type": "application/json; charset=utf-8"},
    )


def _replace_prompt_in_json_tree(node, original: str, warned: str):
    """Returns (node, changed). Recurses into strings that are themselves JSON (Gemini f.req)."""
    if isinstance(node, str):
        s = node.strip()
        if len(s) >= 2 and s[0] in "[{" and s[-1] in "]}":
            try:
                inner = json.loads(s)
            except ValueError:
                inner = None
            if inner is not None:
                inner, changed = _replace_prompt_in_json_tree(inner, original, warned)
                if changed:
                    return json.dumps(inner, ensure_ascii=False, separators=(",", ":")), True
        if original in node:
            return node.replace(original, warned), True
        return node, False
    if isinstance(node, list):
        changed = False
        for i, item in enumerate(node):
            node[i], c = _replace_prompt_in_json_tree(item, original, warned)
            changed = changed or c
        return node, changed
    if isinstance(node, dict):
        changed = False
        for k in list(node):
            node[k], c = _replace_prompt_in_json_tree(node[k], original, warned)
            changed = changed or c
        return node, changed
    return node, False


def _inject_warned_json(raw_text: str, original: str, warned: str) -> str | None:
    # SignalR (Copilot/Bing) frames are JSON records separated by \x1e.
    sep = "\x1e" if "\x1e" in raw_text else ""
    records = raw_text.split(sep) if sep else [raw_text]
    changed_any = False
    out = []
    for rec in records:
        if not rec.strip():
            out.append(rec)
            continue
        try:
            obj = json.loads(rec)
        except ValueError:
            return None
        obj, changed = _replace_prompt_in_json_tree(obj, original, warned)
        changed_any = changed_any or changed
        out.append(json.dumps(obj, ensure_ascii=False, separators=(",", ":")) if changed else rec)
    return sep.join(out) if changed_any else None


def _inject_warned_form(raw_text: str, original: str, warned: str) -> str | None:
    pairs = urllib.parse.parse_qsl(raw_text, keep_blank_values=True)
    if not pairs:
        return None
    changed_any = False
    new_pairs = []
    for k, v in pairs:
        nv = inject_warned_prompt(v, original, warned) if v else None
        if nv is not None:
            changed_any = True
            new_pairs.append((k, nv))
        else:
            new_pairs.append((k, v))
    return urllib.parse.urlencode(new_pairs) if changed_any else None


def inject_warned_prompt(raw_text: str, original: str, warned: str) -> str | None:
    """Rewrite a Send body (JSON, nested JSON strings, form f.req, SignalR frames) so the AI
    receives `warned` instead of `original`. The result stays valid for the site's parser."""
    if not raw_text or not original or not warned or original == warned:
        return None
    head = raw_text.lstrip()[:1]
    if head in ("{", "["):
        rewritten = _inject_warned_json(raw_text, original, warned)
        if rewritten is not None:
            return rewritten
    elif "=" in raw_text and original not in raw_text:
        rewritten = _inject_warned_form(raw_text, original, warned)
        if rewritten is not None:
            return rewritten

    # Partial / non-standard JSON: swap the JSON-escaped form so the string stays valid.
    for ensure_ascii in (False, True):
        ov = json.dumps(original, ensure_ascii=ensure_ascii)[1:-1]
        wv = json.dumps(warned, ensure_ascii=ensure_ascii)[1:-1]
        if ov != original and ov in raw_text:
            return raw_text.replace(ov, wv, 1)
    if head not in ("{", "[") and original in raw_text:
        return raw_text.replace(original, warned, 1)
    if original in raw_text:
        return raw_text.replace(original, json.dumps(warned, ensure_ascii=False)[1:-1], 1)
    return None


def _split_length_prefixed_frames(data: bytes) -> list[tuple[int, bytes]] | None:
    """Connect / gRPC-web envelopes: 1 flag byte + 4-byte big-endian length + payload, repeated."""
    frames: list[tuple[int, bytes]] = []
    i = 0
    while i < len(data):
        if len(data) - i < 5:
            return None
        flag = data[i]
        n = int.from_bytes(data[i + 1:i + 5], "big")
        if flag not in (0x00, 0x01, 0x02, 0x03, 0x80, 0x81) or i + 5 + n > len(data):
            return None
        frames.append((flag, data[i + 5:i + 5 + n]))
        i += 5 + n
    return frames or None


def _inject_warned_framed(data: bytes, original: str, warned: str) -> bytes | None:
    frames = _split_length_prefixed_frames(data)
    if not frames:
        return None
    out = bytearray()
    changed = False
    for flag, payload in frames:
        body = payload
        if flag == 0x00 and payload.lstrip()[:1] in (b"{", b"["):
            try:
                rewritten = inject_warned_prompt(payload.decode("utf-8"), original, warned)
            except UnicodeDecodeError:
                rewritten = None
            if rewritten is not None:
                body = rewritten.encode("utf-8")
                changed = True
        out += bytes([flag]) + len(body).to_bytes(4, "big") + body
    return bytes(out) if changed else None


def _inject_warned_multipart(data: bytes, content_type: str, original: str, warned: str) -> bytes | None:
    """Rewrite only text fields; file parts keep their exact bytes."""
    m = re.search(r'boundary="?([^";]+)"?', content_type or "", re.IGNORECASE)
    if m:
        boundary = m.group(1).strip().encode("latin-1", errors="ignore")
    else:
        first = data.split(b"\r\n", 1)[0]
        if not first.startswith(b"--"):
            return None
        boundary = first[2:]
    delim = b"--" + boundary
    chunks = data.split(delim)
    out = [chunks[0]]
    changed = False
    for chunk in chunks[1:]:
        head_end = chunk.find(b"\r\n\r\n")
        if head_end == -1 or chunk.startswith(b"--") or b"filename=" in chunk[:head_end].lower():
            out.append(chunk)
            continue
        rest = chunk[head_end + 4:]
        trail = b"\r\n" if rest.endswith(b"\r\n") else b""
        try:
            text = (rest[:-2] if trail else rest).decode("utf-8")
        except UnicodeDecodeError:
            out.append(chunk)
            continue
        rewritten = inject_warned_prompt(text, original, warned)
        if rewritten is None:
            out.append(chunk)
            continue
        changed = True
        out.append(chunk[:head_end + 4] + rewritten.encode("utf-8") + trail)
    return delim.join(out) if changed else None


def inject_warned_into_request(flow, raw_text: str, original: str, warned: str) -> bool:
    """Forward `warned` instead of `original` in whatever shape the Send used. True when rewritten."""
    req = flow.request
    data = req.content or b""
    if not data:
        pairs = list(req.query.items(multi=True))
        if not any(original in v for _, v in pairs):
            return False
        req.query = [(k, v.replace(original, warned)) for k, v in pairs]
        return True
    ct = req.headers.get("content-type", "")
    new = _inject_warned_framed(data, original, warned)
    if new is None and ("multipart" in ct.lower() or data.startswith(b"--")) and b"content-disposition" in data[:4096].lower():
        new = _inject_warned_multipart(data, ct, original, warned)
        if new is None:
            return False
    if new is None:
        text = inject_warned_prompt(raw_text, original, warned)
        new = text.encode("utf-8") if text is not None else None
    if new is None:
        return False
    req.content = new
    return True


# ─────────────────────────────────────────────
# mitmproxy Addon Class
# ─────────────────────────────────────────────
