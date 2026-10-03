# BrowserAIInterceptor — mitmproxy addon (loaded after responses_inject.py via MANIFEST).

from concurrent.futures import ThreadPoolExecutor

from mitmproxy.net import encoding as mitm_encoding

_RESPONSE_LEARN_MAX = 2 * 1024 * 1024
_RESPONSE_LEARN_META = "raksha_response_body"
# Cheap byte pre-filter: skip the regex/JSON walk unless the reply can carry a filename.
_RESPONSE_LEARN_HINTS = (
    b"file", b"attach", b"upload", b"original_name", b"originalname",
    b"display_name", b"displayname", b"document",
)
_NO_LEARN_CT_PREFIXES = (
    "text/event-stream", "text/html", "text/css", "image/", "video/", "audio/", "font/",
)


def _response_body_worth_learning(flow: http.HTTPFlow) -> bool:
    """Only JSON-ish upload/create replies carry {file_id, filename} for response()."""
    resp = flow.response
    if (flow.request.method or "").upper() not in ("POST", "PUT", "PATCH", "GET"):
        return False
    status = resp.status_code or 0
    if status >= 400 or status in (204, 304):
        return False
    ct = (resp.headers.get("content-type", "") or "").lower()
    if ct.startswith(_NO_LEARN_CT_PREFIXES) or "javascript" in ct or "ndjson" in ct or "grpc" in ct:
        return False
    length = (resp.headers.get("content-length", "") or "").strip()
    return not (length.isdigit() and int(length) > _RESPONSE_LEARN_MAX)


def _tee_response_body(flow: http.HTTPFlow):
    """mitmproxy stream callback: forward each chunk unchanged, keep a capped copy."""
    buf = bytearray()
    over = False

    def _tee(chunk: bytes) -> bytes:
        nonlocal over
        if chunk:
            if not over:
                if len(buf) + len(chunk) > _RESPONSE_LEARN_MAX:
                    over = True
                    buf.clear()
                else:
                    buf.extend(chunk)
        elif not over and buf:
            flow.metadata[_RESPONSE_LEARN_META] = bytes(buf)
            buf.clear()
        return chunk

    return _tee


class BrowserAIInterceptor:

    def __init__(self):
        print(f"[Raksha Proxy] Started. Backend: {RAKSHA_BACKEND_URL}")
        print(
            "[Raksha Proxy] Config refresh: background every 1s "
            "(targets/rules/controls) — request path is memory-only (instant Block/Monitor)."
        )
        _ensure_background_config_refresh()

    def _apply_http_prompt(
        self,
        flow: http.HTTPFlow,
        domain: str,
        platform: str,
        prompt: str,
        client_ip: str,
        raw_text: str,
    ) -> None:
        """Common predict + block/warn for any monitored domain (HTTP)."""
        host = flow.request.pretty_host
        prompt = (prompt or "").strip()
        if not prompt:
            return
        # Same-text browser double-fire: reuse decision (never silent skip).
        if is_duplicate_event(domain, prompt, ttl=DEDUPE_TTL, mark=False):
            self._apply_duplicate_http_prompt(flow, domain, platform, prompt, client_ip, raw_text)
            return

        print(f"[Raksha Proxy] Intercepted prompt | {client_ip} -> {platform} ({domain}) | {prompt[:80]!r}")

        allowed, rule_triggered, action, redacted_prompt, reply_text = evaluate_prompt_coalesced(
            platform=platform,
            domain=domain,
            prompt=prompt,
            client_ip=client_ip,
            url=flow.request.url,
            method=flow.request.method,
        )
        # Mark only after evaluate so a failed first attempt can retry.
        mark_duplicate_event(domain, prompt)
        clear_composer_state(domain)

        if not allowed:
            if (action or "").lower() in ("bot answered", "replied"):
                print(f"[Raksha Proxy] Reply Bot answered for {domain}")
            else:
                print(f"[Raksha Proxy] BLOCKED prompt to {domain} -> Rule: {rule_triggered}")
            make_blocked_response(flow, rule_triggered, host, reply_text=reply_text)
        elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != prompt:
            print(f"[Raksha Proxy] WARNED prompt to {domain} -> Rule: {rule_triggered} (prompt+warning forwarded)")
            try:
                if not inject_warned_into_request(flow, raw_text, prompt, redacted_prompt):
                    print(f"[Raksha Proxy Warning] WARN inject miss | {domain} | could not rewrite body")
            except Exception as e:
                print(f"[Raksha Proxy Warning] Failed to inject warning into request: {e}")

    def _apply_duplicate_http_prompt(
        self,
        flow: http.HTTPFlow,
        domain: str,
        platform: str,
        prompt: str,
        client_ip: str,
        raw_text: str,
    ) -> None:
        """ChatGPT/Claude often double-fire the same Send — never silent-allow the retry."""
        host = flow.request.pretty_host
        decision = get_remembered_guard_decision(domain, prompt)
        if decision is None:
            decision = evaluate_prompt_coalesced(
                platform=platform,
                domain=domain,
                prompt=prompt,
                client_ip=client_ip,
                url=flow.request.url,
                method=flow.request.method,
            )
        allowed, rule_triggered, action, redacted_prompt, reply_text = decision
        if not allowed:
            print(f"[Raksha Proxy] BLOCKED duplicate prompt to {domain} -> Rule: {rule_triggered}")
            make_blocked_response(flow, rule_triggered, host, reply_text=reply_text)
        elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != prompt:
            try:
                inject_warned_into_request(flow, raw_text, prompt, redacted_prompt)
            except Exception:
                pass

    def _file_send_maybe_block(
        self,
        flow: http.HTTPFlow,
        domain: str,
        platform: str,
        client_ip: str,
        raw_text: str,
        content_type: str,
        path: str,
    ) -> tuple[bool, int, bool]:
        """Scan attached/cached files on Send. Returns (blocked, files_processed, caption_consumed)."""
        should_block_file, file_block_msg, redact_notice, n_processed, caption_consumed = enforce_file_send_policy(
            platform=platform,
            domain=domain,
            host=flow.request.pretty_host,
            client_ip=client_ip,
            url=flow.request.url,
            method=flow.request.method,
            raw_text=raw_text,
            content_type=content_type,
            file_name_hint=(
                extract_filename_from_upload(flow, raw_text)
                if chat_carries_attachment(raw_text)
                else extract_attachment_filename_from_send(raw_text)
            ),
            path=path,
        )
        if should_block_file:
            make_blocked_response(
                flow, "Block Upload", flow.request.pretty_host, reply_text=file_block_msg,
            )
            return True, n_processed, caption_consumed
        if redact_notice:
            caption = extract_prompt_universal(
                flow.request.content or b"", content_type, host=flow.request.pretty_host, url=flow.request.url,
            ) or ""
            caption = (caption or "").strip()
            try:
                new_content = inject_file_redact_notice(raw_text, redact_notice, caption)
                if new_content:
                    flow.request.content = new_content.encode("utf-8")
                    print(f"[Raksha Proxy] FILE REDACT notice injected | {domain}")
                else:
                    print(f"[Raksha Proxy Warning] FILE REDACT inject miss | {domain}")
            except Exception as e:
                print(f"[Raksha Proxy Warning] FILE REDACT inject failed: {e}")
        return False, n_processed, caption_consumed

    def _detect_search_browser(self, flow: http.HTTPFlow) -> str:
        """Identify the employee browser from UA / Client Hints — any Chromium or Gecko browser."""
        user_agent = flow.request.headers.get("user-agent", "") or ""
        sec_ch_ua = (flow.request.headers.get("sec-ch-ua", "") or "").lower()
        ua = user_agent.lower()

        # Order matters: Edge/Opera/Brave/Vivaldi embed "chrome/" in UA.
        if (
            "edg/" in ua
            or "edga/" in ua
            or "edgios/" in ua
            or "microsoft edge" in sec_ch_ua
            or '"microsoft edge"' in sec_ch_ua
        ):
            return "Edge"
        if "opr/" in ua or "opera" in sec_ch_ua:
            return "Opera"
        if "brave" in sec_ch_ua or "brave/" in ua:
            return "Brave"
        if "vivaldi" in ua or "vivaldi" in sec_ch_ua:
            return "Vivaldi"
        if "firefox/" in ua or "fxios/" in ua:
            return "Firefox"
        if ("safari/" in ua or "version/" in ua) and "chrome" not in ua and "chromium" not in ua:
            return "Safari"
        if "chrome/" in ua or "crios/" in ua or "google chrome" in sec_ch_ua or "chromium" in sec_ch_ua:
            return "Chrome"
        return "Unknown"

    @staticmethod
    def _decode_bing_click_u(u_val: str) -> str:
        """Decode Bing/Edge result redirect `u=a1…` (base64url) to the destination URL."""
        import base64
        import urllib.parse

        raw = (u_val or "").strip()
        if not raw:
            return ""
        # Plain URL sometimes appears without a1 prefix
        if raw.startswith("http://") or raw.startswith("https://"):
            return raw
        # a1 / a1aHR0… — strip leading letter+digit marker then pad base64
        payload = raw
        if len(raw) > 2 and raw[0].isalpha() and raw[1].isdigit():
            payload = raw[2:]
        pad = "=" * ((4 - (len(payload) % 4)) % 4)
        for candidate in (payload + pad, payload):
            try:
                decoded = base64.urlsafe_b64decode(candidate.encode("ascii", errors="ignore")).decode(
                    "utf-8", errors="ignore"
                )
                decoded = (decoded or "").strip()
                if decoded.startswith("http://") or decoded.startswith("https://"):
                    return decoded
                # Sometimes nested once
                if decoded and not decoded.startswith("http"):
                    again = urllib.parse.unquote(decoded)
                    if again.startswith("http"):
                        return again
            except Exception:
                continue
        return ""

    @staticmethod
    def _is_committed_search_navigation(flow: http.HTTPFlow) -> bool:
        """True only for top-level address-bar / Enter navigations — not page widgets or prefetch.

        Edge/Bing SERP + MSN NTP fire dozens of subresource requests that carry `q=`
        (related searches, cards, suggest). Google mostly uses dedicated suggest paths;
        Bing reuses `/search?q=` so we must gate on Sec-Fetch document navigate.
        """
        method = (flow.request.method or "GET").upper()
        if method not in ("GET", "HEAD"):
            return False
        dest = (flow.request.headers.get("sec-fetch-dest") or "").lower().strip()
        mode = (flow.request.headers.get("sec-fetch-mode") or "").lower().strip()
        user = (flow.request.headers.get("sec-fetch-user") or "").strip()
        # Explicit user activation on a document navigation (typed + Enter / link open)
        if dest == "document" and mode in ("navigate", "nested-navigate"):
            return True
        # Legacy clients without Sec-Fetch — allow; modern Edge/Chrome always send them.
        if not dest and not mode:
            return True
        # sec-fetch-user: ? means user-initiated navigation even if dest is odd
        if user == "?/" or user == "?":
            if mode in ("navigate", "nested-navigate", ""):
                return True
        return False

    @staticmethod
    def _is_bing_committed_search_path(path_l: str) -> bool:
        """Paths that represent a real Bing/Edge search results page (not NTP/MSN cards)."""
        p = (path_l or "").split("?", 1)[0]
        if p in ("/search", "/images/search", "/videos/search", "/news/search", "/shop", "/maps"):
            return True
        if p.startswith("/images/search") or p.startswith("/videos/search") or p.startswith("/news/search"):
            return True
        if p.startswith("/maps") and ("/search" in p or p == "/maps"):
            return True
        return False

    @staticmethod
    def _is_search_engine_host(h: str) -> bool:
        """Search hosts the server PAC always routes to Guard (searchEnginePACRule) — Search Logs only."""
        if h.startswith("google.") or h.startswith("www.google."):
            return True
        if h in ("bing.com", "www.bing.com", "duckduckgo.com", "html.duckduckgo.com", "search.brave.com"):
            return True
        return h == "search.yahoo.com" or h.endswith(".search.yahoo.com")

    @staticmethod
    def _is_junk_search_query(q: str) -> bool:
        """Drop single-letter / page-scrap fragments that are not typed searches."""
        t = (q or "").strip()
        if not t:
            return True
        # Single token 1–2 chars (e.g. "H", "U", "O") — Edge widget crumbs
        if len(t) <= 2 and " " not in t:
            return True
        # Very short multi-token nonsense like "O Are Are"
        words = t.split()
        if len(words) >= 2 and all(len(w) <= 3 for w in words) and len(t) <= 12:
            # Allow normal short queries: "how old", "ai ml"
            if not any(len(w) >= 3 for w in words):
                return True
        return False

    def _maybe_record_search_engine(self, flow: http.HTTPFlow, host: str) -> None:
        """Capture search queries + result clicks for ANY browser (Chrome/Edge/Firefox/…).

        Engines: Google, Bing (incl. Edge/MSN), DuckDuckGo, Yahoo.
        Saves via POST /api/browser-ai/search-logs → Postgres.
        """
        try:
            import urllib.parse

            h_lower = (host or "").lower().strip(".")
            if not (self._is_search_engine_host(h_lower) or detect_target(h_lower)[0]):
                return
            engine = ""
            # Edge new-tab / MSN often fronts Bing — treat as Bing for Search Logs.
            if "google." in h_lower:
                engine = "Google"
            elif (
                "bing.com" in h_lower
                or h_lower.endswith("msn.com")
                or h_lower == "msn.com"
                or "edgeservices.bing" in h_lower
                or h_lower.endswith(".msn.com")
            ):
                engine = "Bing"
            elif "duckduckgo.com" in h_lower:
                engine = "DuckDuckGo"
            elif "search.brave.com" in h_lower or h_lower == "search.brave.com":
                engine = "Brave Search"
            elif "search.yahoo.com" in h_lower or h_lower.endswith("yahoo.com") or h_lower == "yahoo.com":
                engine = "Yahoo"
            else:
                return

            browser = self._detect_search_browser(flow)

            cookie = flow.request.headers.get("cookie", "") or ""
            is_incognito = False
            if flow.request.headers.get("x-edge-inprivate", ""):
                is_incognito = True
            elif not cookie or len(cookie.strip()) < 15:
                is_incognito = True
            elif engine == "Google" and ("SAPISID=" not in cookie and "SID=" not in cookie):
                is_incognito = True
            elif engine == "Bing" and ("MUID=" not in cookie and "_EDGE_S=" not in cookie and "USRLOC=" not in cookie):
                is_incognito = True

            path = flow.request.path or ""
            path_l = path.lower().split("?", 1)[0]
            query_str = flow.request.query or {}
            searched_query = ""
            clicked_url = ""
            clicked_title = ""

            def _q(*keys: str) -> str:
                for k in keys:
                    v = query_str.get(k, "")
                    if isinstance(v, (list, tuple)):
                        v = v[0] if v else ""
                    v = (v or "").strip()
                    if v:
                        return urllib.parse.unquote_plus(v)
                return ""

            if engine == "Google":
                # Skip autocomplete / suggest noise — keep committed /search and link redirects.
                if "/complete/" in path_l or path_l.endswith("/complete/search"):
                    return
                if not any(x in path_l for x in ("/gen_204", "/client_204", "/async/", "/csi", "/verify/")):
                    # Prefer real /search navigations; still allow classic / with q= for typed Enter.
                    is_search_path = (
                        path_l == "/search"
                        or path_l.startswith("/search")
                        or path_l in ("/", "/webhp")
                    )
                    if is_search_path and self._is_committed_search_navigation(flow):
                        raw_q = _q("q", "as_q", "query")
                        if raw_q and not raw_q.startswith("http") and not self._is_junk_search_query(raw_q):
                            searched_query = raw_q
                # Result link click: /url?url=… or /url?q=https://…
                if path_l.startswith("/url") or "/url?" in (flow.request.path or "").lower() or path_l == "/url":
                    target = _q("url", "q", "qurl")
                    if target.startswith("http"):
                        clicked_url = target
                        searched_query = ""  # click row — don't also store redirect junk as query
                # Image result click
                if not clicked_url and ("/imgres" in path_l or path_l.startswith("/imgres")):
                    target = _q("imgurl", "imgrefurl", "q")
                    if target.startswith("http"):
                        clicked_url = target

            elif engine == "Bing":
                # MSN / Edge NTP / edgeservices fire dozens of URLs with q= that are NOT searches.
                is_msn = (
                    h_lower.endswith("msn.com")
                    or h_lower == "msn.com"
                    or "edgeservices.bing" in h_lower
                    or h_lower.startswith("ntp.")
                )
                # Skip suggest / telemetry / related-card APIs
                if any(
                    path_l.startswith(p)
                    for p in (
                        "/as/",
                        "/suggestions/",
                        "/fd/ls",
                        "/notifications/",
                        "/api/",
                        "/hp/",
                        "/homepage",
                        "/rewards",
                        "/th/",
                        "/ts/",
                        "/rs/",
                        "/ans/",
                        "/entityexplore",
                        "/proactive",
                        "/sa/",
                        "/passport",
                        "/msnicons",
                    )
                ) or path_l in ("/th", "/homepage", "/hp"):
                    if not (path_l.startswith("/ck/") or "alink.aspx" in path_l or path_l.startswith("/aclick")):
                        return

                # Only log a search query from a real Bing SERP document navigation.
                # Never take `pq` (previous query) — it repeats on every related/widget hit.
                if (
                    not is_msn
                    and self._is_bing_committed_search_path(path_l)
                    and self._is_committed_search_navigation(flow)
                ):
                    raw_q = _q("q", "query")  # do not use pq
                    if raw_q and not raw_q.startswith("http") and not self._is_junk_search_query(raw_q):
                        searched_query = raw_q

                # Edge/Bing result click redirects (keep click rows; drop query noise)
                if (
                    path_l.startswith("/ck/")
                    or "alink.aspx" in path_l
                    or path_l.startswith("/aclick")
                    or path_l.startswith("/news/apiclick")
                    or "r.msn.com" in h_lower
                    or path_l.startswith("/cl/")
                ):
                    u_val = query_str.get("u", "") or query_str.get("url", "") or query_str.get("r", "")
                    if isinstance(u_val, (list, tuple)):
                        u_val = u_val[0] if u_val else ""
                    decoded = self._decode_bing_click_u(str(u_val or ""))
                    if not decoded:
                        decoded = _q("url", "r", "u")
                        if not (decoded.startswith("http")):
                            decoded = ""
                    if decoded.startswith("http"):
                        clicked_url = decoded
                        # Click row only — never attach SERP/widget q= as a new "search"
                        searched_query = ""

            elif engine == "DuckDuckGo":
                if path_l.startswith("/ac/"):
                    return
                if self._is_committed_search_navigation(flow):
                    raw_q = _q("q", "query")
                    if raw_q and not raw_q.startswith("http") and not self._is_junk_search_query(raw_q):
                        searched_query = raw_q
                if path_l.startswith("/l/") or path_l.startswith("/y.js"):
                    uddg = _q("uddg", "u")
                    if uddg.startswith("http"):
                        clicked_url = uddg
                        searched_query = ""

            elif engine == "Brave Search":
                if self._is_committed_search_navigation(flow):
                    raw_q = _q("q", "query")
                    if raw_q and not raw_q.startswith("http") and not self._is_junk_search_query(raw_q):
                        searched_query = raw_q
                # Brave often links out directly; capture redirect helpers when present
                target = _q("url", "u")
                if target.startswith("http") and ("/redirect" in path_l or path_l.startswith("/out")):
                    clicked_url = target
                    searched_query = ""

            elif engine == "Yahoo":
                if self._is_committed_search_navigation(flow):
                    raw_q = _q("p", "q", "query")
                    if raw_q and not raw_q.startswith("http") and not self._is_junk_search_query(raw_q):
                        searched_query = raw_q
                # Yahoo click redirects
                if "/RU=" in (flow.request.url or "") or path_l.startswith("/click") or "rds.yahoo" in h_lower:
                    ru = ""
                    full = flow.request.url or ""
                    if "/RU=" in full:
                        try:
                            part = full.split("/RU=", 1)[1]
                            part = part.split("/RK=", 1)[0].split("/RS=", 1)[0]
                            ru = urllib.parse.unquote(part)
                        except Exception:
                            ru = ""
                    if not ru:
                        ru = _q("RU", "url")
                    if ru.startswith("http"):
                        clicked_url = ru
                        searched_query = ""

            searched_query = (searched_query or "").strip()
            clicked_url = (clicked_url or "").strip()
            if not searched_query and not clicked_url:
                return

            # Drop obvious non-user noise queries
            if searched_query and len(searched_query) > 500:
                searched_query = searched_query[:500]
            if clicked_url and len(clicked_url) > 2000:
                clicked_url = clicked_url[:2000]

            if clicked_url:
                try:
                    p = urllib.parse.urlparse(clicked_url)
                    clicked_title = p.netloc or clicked_url[:40]
                except Exception:
                    clicked_title = clicked_url[:40]

            client_ip = get_client_ip(flow)
            # Include browser + IP so Edge is not dropped when Chrome searched the same term.
            event_key = f"{engine}:{browser}:{client_ip}:{searched_query}:{clicked_url}"
            if is_duplicate_event("search-engine", event_key, ttl=4):
                return
            mark_duplicate_event("search-engine", event_key)

            wire_fields = _agent_wire_fields() if "_agent_wire_fields" in globals() else {}
            payload_dict = {
                "engine": engine,
                "browser": browser,
                "is_incognito": is_incognito,
                "query": searched_query,
                "clicked_url": clicked_url,
                "clicked_title": clicked_title,
                "url": flow.request.url,
                "host": host,
                "client_ip": client_ip,
                **wire_fields,
            }

            import json
            import ssl
            import threading
            import urllib.request

            def _post():
                try:
                    ssl_ctx = ssl.create_default_context()
                    ssl_ctx.check_hostname = False
                    ssl_ctx.verify_mode = ssl.CERT_NONE
                    req_data = json.dumps(payload_dict).encode("utf-8")
                    r = urllib.request.Request(
                        f"{RAKSHA_BACKEND_URL}/api/browser-ai/search-logs",
                        data=req_data,
                        headers=_backend_headers({"Content-Type": "application/json"}),
                        method="POST",
                    )
                    with urllib.request.urlopen(r, context=ssl_ctx, timeout=8) as resp:
                        if getattr(resp, "status", 200) >= 400:
                            print(
                                f"[Raksha Proxy Warning] search-log HTTP {resp.status} → {RAKSHA_BACKEND_URL}"
                            )
                except Exception as e:
                    print(f"[Raksha Proxy Warning] Failed to send search log to {RAKSHA_BACKEND_URL}: {e}")

            threading.Thread(target=_post, daemon=True).start()
            print(
                f"[Raksha Proxy] SEARCH LOGGED | {engine} ({browser}"
                f"{' - INCOGNITO' if is_incognito else ''}) | "
                f"Query={searched_query!r} Click={clicked_url!r}"
            )

        except Exception as e:
            print(f"[Raksha Proxy Warning] Search engine parse error: {e}")

    # ── HTTP Request Interception ──────────────

    def request(self, flow: http.HTTPFlow) -> None:
        host = flow.request.pretty_host
        method = (flow.request.method or "").upper()

        # Search engine query / result interception.
        self._maybe_record_search_engine(flow, host)

        # CDN / noise hosts (cdn.*, static.*) often carry file uploads. Cache via Referer
        # BEFORE noise early-return — otherwise extract→rules never see the bytes.
        if method in ("POST", "PUT", "PATCH") and is_noise_host(host):
            path_n = flow.request.path
            raw_bytes_n = flow.request.content or b""
            content_type_n = flow.request.headers.get("content-type", "")
            try:
                raw_text_n = raw_bytes_n.decode("utf-8", errors="ignore")
            except Exception:
                raw_text_n = ""
            is_upload_n, upload_reason_n = detect_file_upload(flow, raw_text_n)
            if is_upload_n:
                fname_n = extract_filename_from_upload(flow, raw_text_n)
                bind = _resolve_upload_bind_domain(flow, host)
                # Block Upload must kill CDN attach too (ChatGPT/Gemini often use noise hosts).
                if bind and controls_active("block_upload"):
                    warn = (get_control_settings().get("upload_warning") or "").strip() or "File uploads are blocked by admin policy."
                    make_blocked_response(flow, "Block Upload", host, reply_text=warn)
                    return
                confident_n = is_confident_file_upload(
                    fname=fname_n,
                    content_type=content_type_n,
                    raw_bytes=raw_bytes_n,
                    raw_text=raw_text_n,
                    upload_reason=upload_reason_n or "",
                    host=host,
                    path=path_n,
                )
                payload_ok_n = False
                if not confident_n and bind and len(raw_bytes_n) >= 64:
                    p_n, _, _ = extract_upload_file_payload(raw_bytes_n, content_type_n, fname_n or "")
                    if p_n and len(p_n) >= 32:
                        payload_ok_n = True
                    elif raw_bytes_n.lstrip()[:1] not in (b"{", b"[") and len(raw_bytes_n) >= 256:
                        payload_ok_n = True
                if bind and (confident_n or payload_ok_n):
                    try:
                        ingest_upload_filenames_from_body(raw_text_n, bind)
                    except Exception:
                        pass
                    file_ids = _extract_file_ids_from_chat(raw_text_n)
                    cache_upload_file(
                        bind,
                        file_name=fname_n or "attachment",
                        raw_bytes=raw_bytes_n,
                        content_type=content_type_n,
                        upload_reason=upload_reason_n or "",
                        file_id=file_ids[0] if file_ids else "",
                    )
                    print(
                        f"[Raksha Proxy] FILE CACHED via noise CDN bind | upload_host={host} -> "
                        f"target={bind} | {fname_n or 'attachment'} | {len(raw_bytes_n)} bytes"
                    )
            return

        if is_noise_host(host):
            return

        # Full-site lock (admin: Block entire website) — all methods, all paths
        blocked, b_domain, b_platform = detect_site_block(host)
        if blocked:
            client_ip = get_client_ip(flow)
            if not is_duplicate_event(b_domain, "site-block", ttl=BLOCK_DEDUPE_TTL):
                print(f"[Raksha Proxy] SITE BLOCKED | {client_ip} -> {host} ({b_domain})")
                # Fire-and-forget: do NOT block the mitmproxy event loop with a
                # synchronous urlopen — backend latency stalls ALL proxy flows.
                import json as _json
                import threading as _thr
                _wire = _agent_wire_fields() if "_agent_wire_fields" in globals() else {}
                _meta = _agent_metadata_fields() if "_agent_metadata_fields" in globals() else {}
                _site_block_payload = _json.dumps({
                    "platform": b_platform,
                    "prompt": f"[SITE BLOCKED] Access denied to {b_domain}",
                    "client_ip": client_ip,
                    **_wire,
                    "metadata": {
                        "domain": b_domain,
                        "url": flow.request.url,
                        "method": flow.request.method,
                        "is_blocked": True,
                        "blocked_reason": "Block Entire Website",
                        **_meta,
                    },
                }).encode("utf-8")

                def _post_site_block(payload=_site_block_payload):
                    try:
                        req = urllib.request.Request(
                            f"{RAKSHA_BACKEND_URL}/api/browser-ai/intercept",
                            data=payload,
                            headers=_backend_headers({"Content-Type": "application/json"}),
                            method="POST",
                        )
                        urllib.request.urlopen(req, timeout=3)
                    except Exception:
                        pass

                _thr.Thread(target=_post_site_block, daemon=True).start()
            make_site_blocked_response(flow, b_domain, b_platform)
            return

        is_target, domain, platform = detect_target(host)
        path = flow.request.path
        client_ip = get_client_ip(flow)
        method = (flow.request.method or "").upper()

        # Do NOT brand-skip google.*/bing.*/yahoo.* here.
        # Search logged; non-targets continue to CDN bind.
        # Admin Target Websites on those hosts (clients6.google.com, notebooklm.google.com,
        # aistudio.google.com, bing Copilot, …) must still cache uploads + run Guard Rules.

        if is_target and domain:
            # Sticky bind for later CDN uploads that omit Referer (any admin-added domain).
            remember_client_target_domain(client_ip, domain)

        if not is_target:
            # File CDNs are often NOT the chat Target Website. Bind via Referer/Origin
            # to the admin-added domain so extract→rules still run on Send (any AI site).
            if method in ("POST", "PUT", "PATCH"):
                raw_bytes_nt = flow.request.content or b""
                content_type_nt = flow.request.headers.get("content-type", "")
                try:
                    raw_text_nt = raw_bytes_nt.decode("utf-8", errors="ignore")
                except Exception:
                    raw_text_nt = ""
                # Even tiny Google resumable START (no file bytes yet) carries the real name.
                try:
                    # Resolve the bind domain only when a real filename is on the wire —
                    # otherwise every analytics POST on the internet pays for it.
                    hdr_early = _filename_from_multipart_or_headers(
                        raw_bytes_nt, flow.request.headers, raw_text_nt,
                    )
                    if hdr_early and _is_real_user_upload_name(hdr_early):
                        bind_early = _resolve_upload_bind_domain(flow, host)
                        if bind_early:
                            remember_pending_upload_name_for_domain(bind_early, hdr_early)
                except Exception:
                    pass
                is_upload_nt, upload_reason_nt = detect_file_upload(flow, raw_text_nt)
                if is_upload_nt:
                    fname_nt = extract_filename_from_upload(flow, raw_text_nt)
                    bind = _resolve_upload_bind_domain(flow, host)
                    if remember_file_create_handshake(raw_text_nt, bind or "", raw_bytes_nt, path):
                        return
                    try:
                        ingest_upload_filenames_from_body(raw_text_nt, bind or "")
                    except Exception:
                        pass
                    # Google/ChatGPT CDN: capture name from headers even before bytes land.
                    try:
                        hdr_name = _filename_from_multipart_or_headers(
                            raw_bytes_nt, flow.request.headers, raw_text_nt,
                        )
                        if bind and hdr_name and _is_real_user_upload_name(hdr_name):
                            remember_pending_upload_name_for_domain(bind, hdr_name)
                            if not fname_nt or not _is_real_user_upload_name(fname_nt):
                                fname_nt = hdr_name
                    except Exception:
                        pass
                    # Block Upload also blocks non-target CDN attaches.
                    if bind and controls_active("block_upload"):
                        warn = (get_control_settings().get("upload_warning") or "").strip() or "File uploads are blocked by admin policy."
                        make_blocked_response(flow, "Block Upload", host, reply_text=warn)
                        return
                    confident_nt = is_confident_file_upload(
                        fname=fname_nt,
                        content_type=content_type_nt,
                        raw_bytes=raw_bytes_nt,
                        raw_text=raw_text_nt,
                        upload_reason=upload_reason_nt or "",
                        host=host,
                        path=path,
                    )
                    payload_ok_nt = False
                    if not confident_nt and bind and len(raw_bytes_nt) >= 64:
                        p_nt, _, _ = extract_upload_file_payload(
                            raw_bytes_nt, content_type_nt, fname_nt or ""
                        )
                        if p_nt and len(p_nt) >= 32:
                            payload_ok_nt = True
                        elif raw_bytes_nt.lstrip()[:1] not in (b"{", b"[") and len(raw_bytes_nt) >= 256:
                            payload_ok_nt = True
                    if bind and (confident_nt or payload_ok_nt):
                        file_ids = _extract_file_ids_from_chat(raw_text_nt)
                        cache_upload_file(
                            bind,
                            file_name=fname_nt or "attachment",
                            raw_bytes=raw_bytes_nt,
                            content_type=content_type_nt,
                            upload_reason=upload_reason_nt or "",
                            file_id=file_ids[0] if file_ids else "",
                        )
                        print(
                            f"[Raksha Proxy] FILE CACHED via Referer bind | upload_host={host} -> "
                            f"target={bind} | {fname_nt or 'attachment'} | {len(raw_bytes_nt)} bytes"
                        )
            return

        host_role = get_target_host_role(domain) if domain else ""
        # ui = static/CDN UI host — skip only for pure static GETs with no body
        if host_role == "ui" and method not in ("POST", "PUT") and not flow.request.content:
            return

        # Keep control settings warm
        get_control_settings()

        # ── Universal GET: query-string prompts on any monitored domain ──
        if method == "GET":
            if host_role == "file":
                return
            qs_prompt = extract_prompt_from_query_string(flow.request.url)
            path_q = (path or "").lower().split("?", 1)[0]
            if (
                qs_prompt
                and looks_like_user_prompt(qs_prompt)
                and not _is_typing_or_draft_path(path_q)
                and not any(m in path_q for m in ("suggest", "typeahead"))
                and (_is_confident_chat_send(path, "", b"") or _path_has_chat_marker(path_q))
            ):
                if is_duplicate_event(domain, qs_prompt, ttl=DEDUPE_TTL, mark=False):
                    self._apply_duplicate_http_prompt(flow, domain, platform, qs_prompt, client_ip, "")
                else:
                    self._apply_http_prompt(flow, domain, platform, qs_prompt, client_ip, "")
            return

        if method not in ("POST", "PUT", "PATCH"):
            return

        # File upload: block-all now, else cache for Send scan.
        raw_bytes = flow.request.content or b""
        content_type = flow.request.headers.get("content-type", "")

        try:
            raw_text = raw_bytes.decode("utf-8", errors="ignore")
        except Exception:
            raw_text = ""

        # Learn file_id → name for later nameless CDN uploads.
        try:
            ingest_upload_filenames_from_body(raw_text, domain or "")
        except Exception:
            pass
        # Gemini resumable START may have name header + empty body.
        try:
            hdr_name = _filename_from_multipart_or_headers(b"", flow.request.headers, raw_text or "")
            if hdr_name and _is_real_user_upload_name(hdr_name):
                remember_pending_upload_name_for_domain(domain, hdr_name)
        except Exception:
            pass

        # Gemini history/settings batchexecute must pass through BEFORE prompt extract.
        # Otherwise false extracts + block inject leave the sidebar spinning forever.
        if "batchexecute" in (path or "").lower() and not is_batchexecute_chat_submit(path, raw_text):
            return

        # Extract user text first. host_role=file skips pure chat (uploads still handled above/below).
        peek_prompt = extract_prompt_universal(
            raw_bytes, content_type, host=host, url=flow.request.url,
        )
        has_prompt = _should_intercept_extracted_prompt(
            peek_prompt, path, raw_text, domain, host=host, raw_bytes=raw_bytes,
        )
        attachment_send = _send_carries_attachment(raw_text)

        path_lower = (path or "").lower().split("?", 1)[0]
        is_upload_endpoint = _path_looks_like_upload(path_lower)
        is_upload, upload_reason = detect_file_upload(flow, raw_text)
        # Monitored domain: catch real file bodies even when URL path is unfamiliar
        # (Claude/Gemini/custom AIs often use opaque /api/.../uuid upload URLs).
        if not is_upload and not is_upload_endpoint and len(raw_bytes) >= 64:
            low_head = raw_bytes[: min(len(raw_bytes), 24 * 1024)].lower()
            ct_l = (content_type or "").lower()
            if (
                b"filename=" in low_head
                or b"filename*=" in low_head
                or raw_bytes[:5] == b"%PDF-"
                or (raw_bytes[:2] == b"PK" and len(raw_bytes) >= 512)
                or _looks_like_audio(raw_bytes, content_type, "")
                or _looks_like_image(raw_bytes, content_type, "")
                or ct_l.startswith(("audio/", "image/", "video/"))
                or "officedocument" in ct_l
                or "msword" in ct_l
            ):
                is_upload = True
                upload_reason = "binary/multipart body on monitored domain"
        # Voice Send as JSON with the site's STT transcript: the transcript IS the
        # user's message — predict now instead of caching the body as a file.
        voice_transcript = ""
        if (
            (is_upload or is_upload_endpoint or not (peek_prompt or "").strip())
            and raw_text.lstrip()[:1] in ("{", "[")
            and not is_event_sync_noise_content(raw_text)
        ):
            voice_transcript = (_extract_transcript_fields_from_json(raw_text) or "").strip()
        if voice_transcript:
            blocked, n_processed, _ = self._file_send_maybe_block(
                flow, domain, platform, client_ip, raw_text, content_type, path,
            )
            if blocked or n_processed > 0:
                return
            self._apply_http_prompt(flow, domain, platform, voice_transcript, client_ip, raw_text)
            return
        if is_upload or is_upload_endpoint:
            if remember_file_create_handshake(raw_text, domain or "", raw_bytes, path):
                return
            fname = extract_filename_from_upload(flow, raw_text)
            # If Block Upload control is actively enabled by admin:
            if controls_active("block_upload"):
                warn = (get_control_settings().get("upload_warning") or "").strip() or "File uploads are blocked by admin policy."
                make_blocked_response(flow, "Block Upload", host, reply_text=warn)
                return

            file_ids = _extract_file_ids_from_chat(raw_text)
            cache_upload_file(
                domain,
                file_name=fname or "attachment",
                raw_bytes=raw_bytes,
                content_type=content_type,
                upload_reason=upload_reason or "upload_endpoint",
                file_id=file_ids[0] if file_ids else "",
            )
            print(
                f"[Raksha Proxy] FILE CACHED (await Send — zero predict on upload) | {domain} | "
                f"{fname or 'attachment'} | {len(raw_bytes)} bytes | "
                f"{method} {host}{path.split('?', 1)[0][:90]}"
            )
            # Upload/attach = cache only. Type detect → extract → Guard Rules →
            # Block/Allow/Warn runs on chat Send (any Target Website).
            return

        # host_role=file: this host is upload/CDN only — skip pure chat DLP
        if host_role == "file" and not _file_policy_applies_on_send(
            path, raw_text, raw_bytes, domain=domain, host=host
        ) and not attachment_send:
            return

        # ── File Send: scan cached bytes; then still apply caption Guard Rules ──
        # Target chat submit with attachment markers or pending upload.
        if _file_policy_applies_on_send(path, raw_text, raw_bytes, domain=domain, host=host):
            blocked, n_processed, caption_consumed = self._file_send_maybe_block(
                flow, domain, platform, client_ip, raw_text, content_type, path,
            )
            if blocked:
                return
            if n_processed > 0:
                # File row(s) already logged with caption joined (Claude-style).
                return
            # Cache miss with attachment markers: fall through so prompt + rules still run.
            print(
                f"[Raksha Proxy] File Send markers without cache | {domain} | "
                "falling through to prompt evaluate (upload may have used another host)"
            )

        if is_event_sync_noise_content(raw_text):
            return

        # Only inspect real chat/prompt endpoints — ignore challenges & analytics
        if not is_chat_path(path, host, raw_text):
            # Universal fallback: if universal extractor already found a confirmed user prompt
            # on an admin target domain, do NOT drop it simply because the path wasn't in a list
            if not (has_prompt and peek_prompt and len(peek_prompt.strip()) >= 1 and not is_noise(path, raw_text)):
                return

        if not _is_confident_chat_send(path, raw_text, raw_bytes) and not attachment_send:
            # Universal fallback: if confirmed user prompt exists, treat as valid send
            if not (has_prompt and peek_prompt and len(peek_prompt.strip()) >= 1):
                return

        messages_parts_shaped = _looks_like_messages_parts_body(raw_text, raw_bytes)
        if not messages_parts_shaped:
            if is_noise(path):
                return
            if is_noise(path, raw_text):
                return
        elif "prepare" in (path or "").lower() or "autocomplet" in (path or "").lower():
            return

        # File attached + Send (fallback path when extract missed on first pass)
        if _file_policy_applies_on_send(path, raw_text, raw_bytes, domain=domain, host=host):
            blocked, n_processed, caption_consumed = self._file_send_maybe_block(
                flow, domain, platform, client_ip, raw_text, content_type, path,
            )
            if blocked:
                return
            if n_processed > 0:
                return
            # If no cache processed, continue so prompt evaluate can still block.

        prompt = extract_prompt_universal(raw_bytes, content_type, host=host, url=flow.request.url)
        if not prompt or len(prompt.strip()) < 1:
            if len(raw_bytes) > 8:
                print(f"[Raksha Proxy] No prompt extracted | {platform} ({domain}) path={path[:80]!r} bytes={len(raw_bytes)}")
            # Attachment-only send already logged above (real file markers only)
            if chat_carries_attachment(raw_text):
                return
            return
        # Same gate as early path — confident Send keeps number/symbol/short text.
        if not _should_intercept_extracted_prompt(
            prompt, path, raw_text, domain, host=host, raw_bytes=raw_bytes
        ):
            return
        # Skip duplicate FILE UPLOAD lines if extract_prompt somehow returned that
        if prompt.strip().startswith("[FILE UPLOAD"):
            return

        # ChatGPT/Perplexity: skip only in-progress draft bodies, not finished submits.
        if is_unsubmitted_chat_body(path, raw_text):
            return
        # Finished chat-shaped Sends: commit immediately (all Target domains).
        # Composer hold is ONLY for keystroke-as-HTTP sites (Grok/Copilot) — long waits
        # caused intermittent predict misses (hi / numbers / symbols / every AI).
        confident_send = _is_confident_chat_send(path, raw_text, raw_bytes)

        # Body-shape shortcut: if JSON body carries messages[]/prompt/query etc.
        # → treat as confident regardless of path recognition (fixes ChatGPT/Perplexity).
        if not confident_send and raw_text.lstrip().startswith(("{", "[")):
            try:
                _body_data = json.loads(raw_text)
                if _body_has_user_send_payload(_body_data):
                    confident_send = True
            except Exception:
                pass

        if not confident_send:
            if len(prompt.strip()) <= 15 or is_composer_typing_draft(domain, prompt):
                stable = wait_if_composer_unstable(domain, prompt)
                if stable is None:
                    # Supersede fallback: never silently drop — commit whatever is latest.
                    with _composer_lock:
                        _fallback = _composer_draft.get(domain)
                    prompt = (_fallback[0] or prompt).strip() if _fallback else prompt
                    if not prompt:
                        return
                else:
                    prompt = stable
        elif len(prompt.strip()) <= 48 and is_composer_typing_draft(domain, prompt):
            # ChatGPT/Gemini often mark keystroke bodies "confident" — still coalesce.
            stable = wait_if_composer_unstable(domain, prompt)
            if stable is None:
                with _composer_lock:
                    _fallback = _composer_draft.get(domain)
                prompt = (_fallback[0] or prompt).strip() if _fallback else prompt
                if not prompt:
                    return
            else:
                prompt = stable


        # Collapse browser double-fire — MUST still enforce the same guard decision
        # (silent return here previously let the 2nd request bypass BLOCK).
        if is_duplicate_event(domain, prompt, ttl=DEDUPE_TTL, mark=False):
            self._apply_duplicate_http_prompt(flow, domain, platform, prompt, client_ip, raw_text)
            return

        print(f"[Raksha Proxy] Intercepted prompt | {client_ip} -> {platform} ({domain}) | {prompt[:80]!r}")
        allowed, rule_triggered, action, redacted_prompt, reply_text = evaluate_prompt_coalesced(
            platform=platform,
            domain=domain,
            prompt=prompt,
            client_ip=client_ip,
            url=flow.request.url,
            method=flow.request.method,
        )
        mark_duplicate_event(domain, prompt)
        clear_composer_state(domain)

        if not allowed:
            if (action or "").lower() in ("bot answered", "replied"):
                print(f"[Raksha Proxy] Reply Bot answered for {domain}")
            else:
                print(f"[Raksha Proxy] BLOCKED prompt to {domain} -> Rule: {rule_triggered}")
            make_blocked_response(flow, rule_triggered, host, reply_text=reply_text)
        elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != prompt:
            print(f"[Raksha Proxy] WARNED prompt to {domain} -> Rule: {rule_triggered} (prompt+warning forwarded)")
            try:
                if not inject_warned_into_request(flow, raw_text, prompt, redacted_prompt):
                    print(f"[Raksha Proxy Warning] WARN inject miss | {domain} | could not rewrite body")
            except Exception as e:
                print(f"[Raksha Proxy Warning] Failed to inject warning into request: {e}")

    def responseheaders(self, flow: http.HTTPFlow) -> None:
        """Stream every server reply straight to the browser.

        Buffered, AI answers (SSE / chunked JSON) only appear after generation
        finishes. Replies response() may learn filenames from are copied (capped)
        while they stream.
        """
        resp = flow.response
        if resp is None or resp.stream or (resp.status_code or 0) == 101:
            return
        resp.stream = _tee_response_body(flow) if _response_body_worth_learning(flow) else True

    def response(self, flow: http.HTTPFlow) -> None:
        """Learn real filenames from upload/create responses on ANY Target / CDN.

        Many sites (ChatGPT, Gemini, Copilot, custom AIs) omit the name on the
        byte PUT and only return {id, filename} after. Request-side cache then
        stays 'attachment' unless we bind here.
        """
        try:
            host = flow.request.pretty_host
            method = (flow.request.method or "").upper()
            if method not in ("POST", "PUT", "PATCH", "GET"):
                return
            resp = flow.response
            if resp is None or (resp.status_code or 0) >= 400:
                return
            ct = (resp.headers.get("content-type", "") or "").lower()
            teed = flow.metadata.pop(_RESPONSE_LEARN_META, None)
            if teed is not None:
                enc = (resp.headers.get("content-encoding", "") or "").strip()
                raw = mitm_encoding.decode(teed, enc) if enc else teed
            elif resp.stream:
                return
            else:
                raw = resp.content or b""
            if not raw or len(raw) > _RESPONSE_LEARN_MAX:
                return
            _low = raw.lower()
            if not any(h in _low for h in _RESPONSE_LEARN_HINTS):
                return
            if not (
                "json" in ct
                or "text/" in ct
                or raw.lstrip()[:1] in (b"{", b"[")
            ):
                return
            try:
                text = raw.decode("utf-8", errors="ignore")
            except Exception:
                return
            if not text or len(text) < 8:
                return
            bind = ""
            ok, domain, _plat = detect_target(host)
            if ok and domain:
                bind = domain
            if not bind:
                bind = _resolve_upload_bind_domain(flow, host)
            if not bind:
                return
            ingest_upload_filenames_from_body(text, bind)
            # Pair request-side name (headers / multipart) with response file_id.
            try:
                req_bytes = flow.request.content or b""
                req_text = ""
                try:
                    req_text = req_bytes.decode("utf-8", errors="ignore")
                except Exception:
                    req_text = ""
                req_name = _filename_from_multipart_or_headers(
                    req_bytes, flow.request.headers, req_text,
                )
                if not req_name or not _is_real_user_upload_name(req_name):
                    req_names = [
                        n for n in extract_all_attachment_filenames_from_send(req_text)
                        if _is_real_user_upload_name(n)
                    ]
                    req_name = req_names[0] if req_names else ""
                resp_ids = _extract_file_ids_from_chat(text)
                if req_name and resp_ids:
                    remember_upload_filename(resp_ids[0], req_name)
                    remember_pending_upload_name_for_domain(bind, req_name)
                    rename_recent_nameless_caches(bind, req_name, resp_ids[0])
            except Exception:
                pass
        except Exception:
            return

    # ── WebSocket Message Interception ─────────

    def websocket_message(self, flow: http.HTTPFlow) -> None:
        if not flow.websocket or not flow.websocket.messages:
            return

        msg = flow.websocket.messages[-1]
        if not msg.from_client:
            return

        host = flow.request.pretty_host
        if is_noise_host(host):
            return
        blocked, b_domain, b_platform = detect_site_block(host)
        if blocked:
            msg.kill()
            print(f"[Raksha Proxy] SITE BLOCKED (websocket) -> {b_domain} ({b_platform})")
            return
        is_target, domain, platform = detect_target(host)
        if not is_target:
            return

        host_role = get_target_host_role(domain) if domain else ""
        if host_role == "file":
            return

        content = websocket_frame_text(msg)
        if not content or len(content.strip()) < 1:
            return

        ws_path = flow.request.path or ""
        if "raksha-reply" in ws_path.lower() or _is_raksha_inject_frame(content):
            return
        # History/settings batchexecute over WS — pass through before extract/inject.
        if "batchexecute" in ws_path.lower() and not is_batchexecute_chat_submit(ws_path, content):
            return

        client_ip = get_client_ip(flow)
        ws_bytes = content.encode("utf-8", errors="ignore")
        ws_has_prompt = False
        ws_prompt = extract_prompt_universal(
            ws_bytes, "application/json", host=host, url=flow.request.url,
        )
        ws_has_prompt = _should_intercept_extracted_prompt(
            ws_prompt, ws_path, content, domain, host=host, raw_bytes=ws_bytes,
        )
        if not ws_has_prompt and content.lstrip()[:1] in ("{", "["):
            ws_transcript = (_extract_transcript_fields_from_json(content) or "").strip()
            if ws_transcript:
                ws_prompt, ws_has_prompt = ws_transcript, True
        apply_files = _file_policy_applies_on_send(
            ws_path, content, ws_bytes, domain=domain, host=host,
        )
        if host_role == "file" and not apply_files and not ws_has_prompt:
            return
        if host_role == "file" and ws_has_prompt and not apply_files:
            # Pure chat frames on a file-role host — ignore
            return
        repeat_file_block = file_send_block_matches(domain, content)
        if repeat_file_block and _copilot_frame_is_user_send(content):
            _drop_websocket_outbound(msg)
            inject_websocket_reply(flow, host, repeat_file_block)
            return

        if apply_files:
            should_block_file, file_block_msg, _redact_notice, _n, _caption_consumed = enforce_file_send_policy(
                platform=platform,
                domain=domain,
                host=host,
                client_ip=client_ip,
                url=flow.request.url,
                method="WS",
                raw_text=content,
                content_type="application/json",
                path=ws_path,
            )
            if should_block_file:
                _drop_websocket_outbound(msg)
                inject_websocket_reply(flow, host, (file_block_msg or "").strip())
                return
            # File row logged — only allow a short user caption, never embedded doc text.
            if ws_has_prompt:
                pn = (ws_prompt or "").strip()
                if pn and len(pn) <= 320 and not _looks_like_document_body_dump(pn):
                    if not is_duplicate_event(domain, pn, ttl=DEDUPE_TTL, mark=False):
                        mark_duplicate_event(domain, ws_prompt)
                        allowed, rule_triggered, action, redacted_prompt, reply_text = evaluate_prompt_coalesced(
                            platform=platform,
                            domain=domain,
                            prompt=ws_prompt,
                            client_ip=client_ip,
                            url=flow.request.url,
                            method="WS",
                        )
                        if not allowed:
                            _drop_websocket_outbound(msg)
                            inject_websocket_reply(flow, host, (reply_text or "").strip())
                        elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != ws_prompt:
                            new_content = inject_warned_prompt(content, ws_prompt, redacted_prompt)
                            if new_content:
                                msg.text = new_content
                    else:
                        decision = get_remembered_guard_decision(domain, pn) or evaluate_prompt_coalesced(
                            platform=platform,
                            domain=domain,
                            prompt=ws_prompt,
                            client_ip=client_ip,
                            url=flow.request.url,
                            method="WS",
                        )
                        allowed, rule_triggered, action, redacted_prompt, reply_text = decision
                        if not allowed:
                            _drop_websocket_outbound(msg)
                            inject_websocket_reply(flow, host, (reply_text or "").strip())
                        elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != ws_prompt:
                            new_content = inject_warned_prompt(content, ws_prompt, redacted_prompt)
                            if new_content:
                                msg.text = new_content
            return

        # ── Universal WebSocket: domain-agnostic extract (same rule as HTTP) ──
        if ws_has_prompt:
            confident_ws = (
                _is_confident_chat_send(ws_path, content, ws_bytes)
                or is_event_send_chat_submit(ws_path, content)
            )
            # Finished Copilot/chat Send: do not debounce short prompts like "Emil id".
            if not confident_ws and (
                len(ws_prompt.strip()) <= 15 or is_composer_typing_draft(domain, ws_prompt)
            ):
                stable = wait_if_composer_unstable(domain, ws_prompt)
                if stable is None:
                    return
                ws_prompt = stable

            print(f"[Raksha Proxy] WebSocket prompt | {client_ip} -> {platform} ({domain}) | {ws_prompt[:80]!r}")
            mark_duplicate_event(domain, ws_prompt)
            allowed, rule_triggered, action, redacted_prompt, reply_text = evaluate_prompt_coalesced(
                platform=platform,
                domain=domain,
                prompt=ws_prompt,
                client_ip=client_ip,
                url=flow.request.url,
                method="WS",
            )
            if not allowed:
                _drop_websocket_outbound(msg)
                inject_websocket_reply(flow, host, (reply_text or "").strip())
            elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != ws_prompt:
                new_content = inject_warned_prompt(content, ws_prompt, redacted_prompt)
                if new_content:
                    msg.text = new_content
            return

        if is_event_sync_noise_content(content):
            return
        if not is_chat_path(ws_path, host, content):
            return

        ws_bytes_shaped = content.encode("utf-8", errors="ignore")
        messages_parts_shaped = _looks_like_messages_parts_body(content, ws_bytes_shaped)
        if not messages_parts_shaped and is_noise(ws_path, content):
            return

        # File attachment Send must hit Block Upload / file rules (finished Send only)
        if _file_policy_applies_on_send(
            ws_path, content, content.encode("utf-8", errors="ignore"), domain=domain, host=host,
        ):
            should_block_file, file_block_msg, _redact_notice, n_processed, caption_consumed = enforce_file_send_policy(
                platform=platform,
                domain=domain,
                host=host,
                client_ip=get_client_ip(flow),
                url=flow.request.url,
                method="WS",
                raw_text=content,
                content_type="application/json",
                path=flow.request.path or "",
            )
            if should_block_file:
                _drop_websocket_outbound(msg)
                inject_websocket_reply(flow, host, file_block_msg)
                return
            if n_processed > 0:
                return
            # Cache miss: fall through so typed text still gets rules.

        # Copilot/Edge image or file frames must not fall through as garbled text prompts.
        if (
            (event_send_carries_binary_attach(content) or chat_carries_attachment(content) or messages_parts_carries_file(content))
            and not (
                ws_has_prompt
                and ws_prompt
                and looks_like_user_prompt(ws_prompt)
                and not _looks_like_document_body_dump(ws_prompt)
                and not _looks_like_filename_only(ws_prompt)
            )
        ):
            return

        prompt = extract_prompt_universal(content.encode("utf-8"), "application/json", host=host, url=flow.request.url)
        if not prompt or prompt.strip() in ("{}", "[]", "ping", "pong"):
            return
        ws_bytes = content.encode("utf-8", errors="ignore")
        if not _should_intercept_extracted_prompt(
            prompt, flow.request.path, content, domain, host=host, raw_bytes=ws_bytes
        ):
            return

        if is_unsubmitted_chat_body(flow.request.path, content):
            return
        # Confident chat shapes: predict immediately (same as HTTP path).
        confident_ws = _is_confident_chat_send(flow.request.path, content, ws_bytes)
        if not confident_ws:
            if is_composer_typing_draft(domain, prompt):
                return
            stable = wait_if_composer_unstable(domain, prompt)
            if stable is None:
                return
            prompt = stable

        if is_duplicate_event(domain, prompt, ttl=DEDUPE_TTL, mark=False):
            decision = get_remembered_guard_decision(domain, prompt)
            if decision is None:
                decision = evaluate_prompt_coalesced(
                    platform=platform,
                    domain=domain,
                    prompt=prompt,
                    client_ip=get_client_ip(flow),
                    url=flow.request.url,
                    method="WS",
                )
            allowed, rule_triggered, action, redacted_prompt, reply_text = decision
            if not allowed:
                _drop_websocket_outbound(msg)
                inject_websocket_reply(flow, host, (reply_text or "").strip())
            elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != prompt:
                new_content = inject_warned_prompt(content, prompt, redacted_prompt)
                if new_content:
                    msg.text = new_content
            return

        client_ip = get_client_ip(flow)
        print(f"[Raksha Proxy] WebSocket prompt | {client_ip} -> {platform} ({domain}) | {prompt[:80]!r}")
        allowed, rule_triggered, action, redacted_prompt, reply_text = evaluate_prompt_coalesced(
            platform=platform,
            domain=domain,
            prompt=prompt,
            client_ip=client_ip,
            url=flow.request.url,
            method="WS",
        )
        mark_duplicate_event(domain, prompt)
        clear_composer_state(domain)

        if not allowed:
            if (action or "").lower() in ("bot answered", "replied"):
                print(f"[Raksha Proxy] Reply Bot answered via WebSocket for {domain}")
            else:
                print(f"[Raksha Proxy] BLOCKED WebSocket to {domain} -> Rule: {rule_triggered or action}")
            block_msg = (reply_text or "").strip()
            # Drop outbound turn (site AI never sees it), inject reply for ANY target site.
            _drop_websocket_outbound(msg)
            inject_websocket_reply(flow, host, block_msg)
        elif action in ("Warned", "Redacted") and redacted_prompt and redacted_prompt != prompt:
            print(f"[Raksha Proxy] WARNED WebSocket prompt to {domain} -> Rule: {rule_triggered}")
            new_content = inject_warned_prompt(content, prompt, redacted_prompt)
            if new_content:
                msg.text = new_content


# Prompt checks can wait on the backend / AI Guard Bot for seconds. Run them off
# mitmproxy's single event loop so only that one request waits, not every tab.
_HOOK_POOL = ThreadPoolExecutor(max_workers=64, thread_name_prefix="raksha-hook")


def _off_event_loop(fn):
    async def _hook(*args):
        global _EVENT_LOOP
        loop = asyncio.get_running_loop()
        _EVENT_LOOP = loop
        await loop.run_in_executor(_HOOK_POOL, fn, *args)

    return _hook


def _make_addon() -> BrowserAIInterceptor:
    addon = BrowserAIInterceptor()
    addon.request = _off_event_loop(addon.request)
    # response() runs regex/JSON walks over up to 2 MB — keep it off the event loop too.
    addon.response = _off_event_loop(addon.response)
    addon.websocket_message = _off_event_loop(addon.websocket_message)
    return addon


addons = [_make_addon()]