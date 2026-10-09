# Part of Gateway browser_ai_proxy — do not import directly.



def enforce_file_send_policy(
    *,
    platform: str,
    domain: str,
    host: str,
    client_ip: str,
    url: str,
    method: str,
    raw_text: str,
    content_type: str = "",
    file_name_hint: str = "",
    path: str = "",
) -> tuple[bool, str, str, int, bool]:
    """
    On chat Send with a real attached file (any monitored AI domain):
      - Block Upload ON  → block on Send only (file may attach in UI first)
      - Block Upload OFF → extract PDF/image/Office/voice text → Guard Rules
          BLOCK        → block Send
          REDACT       → log Redacted, allow Send (+ chat notice when possible)
          no match     → Allowed

    Multi-file + caption: EACH file is extracted + rule-checked separately; typed
    chat text is rule-checked separately. Any file BLOCK or caption BLOCK → block the
    whole Send on the site (ChatGPT/Gemini/…). Prompt Logs still show the truth per
    file: matching file = Blocked, non-matching = Allowed. Caption attached when present.

    Returns (should_block, block_message, redact_notice, files_processed, caption_consumed).
    """
    repeat_msg = file_send_block_matches(domain, raw_text or "")
    if repeat_msg:
        print(f"[Gateway Proxy] FILE SEND BLOCKED (repeat after rule hit) | {host}")
        return True, repeat_msg, "", 0, False

    has_attach = (
        chat_carries_attachment(raw_text)
        or messages_parts_carries_file(raw_text)
        or bool((file_name_hint or "").strip())
        or event_send_carries_binary_attach(raw_text)
    )
    cached_list = take_all_cached_uploads_for_send(domain, raw_text, allow_latest=False)
    expected_n = 0
    try:
        expected_n = _expected_send_attachment_count(raw_text or "")
    except Exception:
        expected_n = 0
    # CRITICAL multi-file fix: finding ONE file by id must NOT skip the rest.
    # ChatGPT/Gemini CDN often caches bytes without file_id keys; only the queue
    # has siblings. Always top-up from recent queue when Send has attachments.
    if has_attach or cached_list:
        more = take_all_cached_uploads_for_send(domain, raw_text, allow_latest=True)
        if more:
            seen = {str(e.get("cache_uid") or id(e)) for e in cached_list}
            for e in more:
                uid = str(e.get("cache_uid") or id(e))
                if uid not in seen:
                    cached_list.append(e)
                    seen.add(uid)
            try:
                cached_list = _dedupe_cached_uploads_by_bytes(cached_list)
            except Exception:
                pass
    if not cached_list and has_attach and is_chat_path(path or "", host, raw_text or ""):
        cached_list = take_recent_confident_caches_for_send(domain)
    # Claude/Gemini/Perplexity/DeepSeek: attach-time cache exists but Send omits file ids /
    # uses placeholder names — still bind recent uploads so predict + rules run.
    # Copilot long-lived WS: never consume cache unless this frame is a finished Send.
    if not cached_list and domain and _domain_has_pending_upload_cache(domain):
        if _is_persistent_chat_websocket(path) and not (
            _copilot_frame_is_user_send(raw_text or "")
            or _is_confident_chat_send(path, raw_text or "")
        ):
            pass
        else:
            cached_list = take_all_cached_uploads_for_send(domain, raw_text or "", allow_latest=True)
            if not cached_list:
                cached_list = take_recent_confident_caches_for_send(domain)
            if cached_list:
                has_attach = True

    # The typed prompt must not become its own attachment next to the real file.
    if cached_list and len(cached_list) > 1:
        caption_peek = ""
        try:
            caption_peek = extract_prompt_universal(
                (raw_text or "").encode("utf-8", errors="ignore"),
                content_type or "",
                host,
                url,
            ) or ""
        except Exception:
            caption_peek = ""
        cached_list = _drop_prompt_echo_uploads(cached_list, caption_peek)

    # ChatGPT often under-counts attachments (one wire token like composer_rendered
    # plus a single id). Never throw away sibling files from the same upload burst.
    # Only drop caches that are older than 3 minutes — a previous Send's leftovers.
    if cached_list and len(cached_list) > 1:
        newest = max(float(e.get("ts") or 0) for e in cached_list)
        clustered = [
            e for e in cached_list
            if newest - float(e.get("ts") or 0) <= 180.0
        ]
        if clustered:
            cached_list = clustered

    if not has_attach and not cached_list:
        return False, "", "", 0, False

    if has_attach:
        inlines = extract_all_inline_attachment_bytes(raw_text or "")
        if inlines and not cached_list:
            cached_list = []
            for idx, (inline_bytes, inline_ct, inline_name) in enumerate(inlines):
                is_img = _looks_like_image(inline_bytes, inline_ct, inline_name)
                suffix = f" {idx + 1}" if len(inlines) > 1 else ""
                fallback_name = f"Image{suffix}" if is_img else f"Document{suffix}"
                cached_list.append({
                    "file_name": inline_name or fallback_name,
                    "content_type": inline_ct or content_type,
                    "raw_bytes": inline_bytes,
                    "ts": time.time(),
                    "cache_uid": f"inline|{time.time():.6f}|{idx}|{len(inline_bytes)}",
                })
        elif inlines and cached_list and len(cached_list) < expected_n:
            cached_names = {(e.get("file_name") or "").strip().lower() for e in cached_list}
            for idx, (inline_bytes, inline_ct, inline_name) in enumerate(inlines):
                if inline_name.lower() not in cached_names:
                    cached_list.append({
                        "file_name": inline_name,
                        "content_type": inline_ct or content_type,
                        "raw_bytes": inline_bytes,
                        "ts": time.time(),
                        "cache_uid": f"inline|{time.time():.6f}|{idx}|{len(inline_bytes)}",
                    })
                    cached_names.add(inline_name.lower())
        elif not cached_list:
            inline_bytes, inline_ct, inline_name = extract_inline_attachment_bytes(raw_text or "")
            if inline_bytes:
                is_img = _looks_like_image(inline_bytes, inline_ct, inline_name)
                fallback_name = "Image" if is_img else "Document"
                cached_list = [{
                    "file_name": inline_name or fallback_name,
                    "content_type": inline_ct or content_type,
                    "raw_bytes": inline_bytes,
                    "ts": time.time(),
                    "cache_uid": f"inline|{time.time():.6f}",
                }]

    # Cache miss but Send clearly carries file/voice — never fail-open for Block Upload.
    if not cached_list and has_attach:
        get_control_settings()
        hint = (file_name_hint or "").strip() or extract_attachment_filename_from_send(raw_text or "") or ""
        if not _is_real_user_upload_name(hint):
            try:
                wait_bind_real_upload_names([], raw_text or "", domain, retries=6, delay=0.20)
            except Exception:
                pass
            try:
                hint = peek_pending_upload_name_for_domain(domain) or hint
            except Exception:
                pass
            if not hint:
                for n in extract_all_attachment_filenames_from_send(raw_text or ""):
                    if _is_real_user_upload_name(n):
                        hint = n
                        break
        if not hint:
            hint = "attachment"
        elif " -- $" in hint:
            hint = hint.split(" -- $", 1)[0].strip()
        if (
            hint == "attachment"
            and not _extract_file_ids_from_chat(raw_text or "")
            and not _extract_transcript_fields_from_json(raw_text or "")
        ):
            return False, "", "", 0, False
        is_voice = bool((
            chat_carries_attachment(raw_text) and _extract_transcript_fields_from_json(raw_text or "")
        ) or _looks_like_audio(b"", content_type, hint))
        if is_voice and (_is_fake_upload_name(hint) or hint.lower() in ("attachment", "file")):
            hint = "Voice Note"
        tag = "[VOICE UPLOAD]" if is_voice else "[FILE UPLOAD]"
        if controls_active("block_upload"):
            warn = (get_control_settings().get("upload_warning") or "").strip() or "File/voice uploads are blocked by admin policy."
            msg = warn
            dedupe_key = f"upload-send-block-all-nocache|{hint}"
            if not is_duplicate_event(domain, dedupe_key, ttl=BLOCK_DEDUPE_TTL, mark=False):
                print(f"[Gateway Proxy] FILE/VOICE SEND BLOCKED (no cache, Block Upload ON) | {client_ip} -> {host}")
                ok = post_upload_intercept(
                    platform=platform,
                    prompt=f"{tag} {hint} — Blocked (Block Upload)",
                    client_ip=client_ip,
                    domain=domain,
                    url=url,
                    method=method,
                    file_name=hint,
                    is_blocked=True,
                    blocked_reason="Block Upload",
                    scan_guard={"cache_miss": True},
                )
                if ok:
                    mark_duplicate_event(domain, dedupe_key)
            remember_file_send_block(
                domain,
                names=[hint],
                ids=_extract_file_ids_from_chat(raw_text or ""),
                message=msg,
            )
            return True, msg, "", 0, False

        # Voice/transcript or caption text still get regex+bot even without file bytes.
        transcript = _extract_transcript_fields_from_json(raw_text or "")
        caption = ""
        try:
            from_body = extract_prompt_universal((raw_text or "").encode("utf-8", errors="ignore"), content_type or "", host, url)
            if from_body and looks_like_user_prompt(from_body):
                caption = from_body.strip()
        except Exception:
            caption = ""
        scan_text = "\n".join(x for x in (transcript, caption) if x).strip()
        if scan_text:
            rule_hit, rule_name, rule_action = match_guard_rules_on_text(scan_text)
            rule_action = (rule_action or "").upper()
            if rule_action == "ALERT":
                rule_action = "WARN"
            plat = (platform or domain or "Browser AI").strip()
            dom = (domain or host or "").strip()
            if dom and (has_ai_bot_rules() or get_guard_rules()):
                try:
                    allowed, rt, action, _, _, eval_err = send_to_backend(
                        plat, dom, scan_text[:50_000], client_ip, url, method or "POST",
                        evaluation_only=True,
                        extracted_text=scan_text[:50_000],
                    )
                    is_rule_backend = (not allowed) or (action or "").upper() in ("BLOCK", "BLOCKED", "REDACT", "REDACTED", "WARN", "WARNED")
                    if is_rule_backend or not eval_err:
                        rule_hit, rule_name, rule_action = _merge_file_scan_backend(
                            rule_hit, rule_name, rule_action, allowed, rt or "AI Guard Bot Policy", action or ("Blocked" if not allowed else "Allowed"),
                        )
                        rule_action = (rule_action or "").upper()
                        if rule_action == "ALERT":
                            rule_action = "WARN"
                except Exception as e:
                    print(f"[Gateway Proxy] transcript/caption scan failed (allowed): {e}")
            cap_done = bool(caption)
            if rule_hit and rule_action == "BLOCK":
                msg = _security_reply_text(rule_name, "") or f"Blocked by Guard Rule ({rule_name})"
                dedupe_key = f"upload-send-block-nocache|{rule_name}|{hint}"
                if not is_duplicate_event(domain, dedupe_key, ttl=BLOCK_DEDUPE_TTL, mark=False):
                    print(f"[Gateway Proxy] FILE/VOICE SEND BLOCKED (transcript/caption) | {rule_name}")
                    ok = post_upload_intercept(
                        platform=platform,
                        prompt=f"{tag} {hint} — Blocked ({rule_name})",
                        client_ip=client_ip,
                        domain=domain,
                        url=url,
                        method=method,
                        file_name=hint,
                        is_blocked=True,
                        blocked_reason=rule_name,
                        extracted_text=scan_text[:50_000],
                        scan_guard={
                            "cache_miss": True,
                            "scan_rule_hit": True,
                            "scan_rule_name": rule_name,
                            "scan_rule_action": "BLOCK",
                        },
                    )
                    if ok:
                        mark_duplicate_event(domain, dedupe_key)
                remember_file_send_block(
                    domain,
                    names=[hint],
                    ids=_extract_file_ids_from_chat(raw_text or ""),
                    message=msg,
                )
                return True, msg, "", 1, cap_done
            if rule_hit and rule_action == "REDACT":
                notice = _warning_for_rule_name(rule_name) or "Gateway Guard redaction policy."
                dedupe_key = f"upload-send-redact-nocache|{rule_name}|{hint}"
                if not is_duplicate_event(domain, dedupe_key, ttl=BLOCK_DEDUPE_TTL, mark=False):
                    post_upload_intercept(
                        platform=platform,
                        prompt=f"{tag} {hint} — Redacted ({rule_name})",
                        client_ip=client_ip,
                        domain=domain,
                        url=url,
                        method=method,
                        file_name=hint,
                        extracted_text=scan_text[:50_000],
                        scan_guard={
                            "cache_miss": True,
                            "scan_rule_hit": True,
                            "scan_rule_name": rule_name,
                            "scan_rule_action": "REDACT",
                        },
                    )
                    mark_duplicate_event(domain, dedupe_key)
                return False, "", notice, 1, cap_done
            dedupe_key = f"upload-send-allow-nocache|{hint}|{scan_text[:40]}"
            if not is_duplicate_event(domain, dedupe_key, ttl=BLOCK_DEDUPE_TTL, mark=False):
                post_upload_intercept(
                    platform=platform,
                    prompt=f"{tag} {hint} — Allowed",
                    client_ip=client_ip,
                    domain=domain,
                    url=url,
                    method=method,
                    file_name=hint,
                    extracted_text=scan_text[:50_000],
                    scan_guard={
                        "cache_miss": True,
                        "scan_guard_decided": True,
                        "scan_guard_action": "Allowed",
                    },
                )
                mark_duplicate_event(domain, dedupe_key)
            return False, "", "", 1, cap_done

        # Cache miss + real attachment markers, but no caption/transcript text
        # (Claude/Gemini often attach-then-auto-read with empty user text).
        # Still log the file so Prompt Logs + rules are not silent.
        dedupe_key = f"upload-send-allow-nocache-empty|{hint}"
        if not is_duplicate_event(domain, dedupe_key, ttl=BLOCK_DEDUPE_TTL, mark=False):
            print(f"[Gateway Proxy] FILE/VOICE SEND (cache miss, no caption) | {hint} | {client_ip} -> {host}")
            ok = post_upload_intercept(
                platform=platform,
                prompt=f"{tag} {hint} — Allowed",
                client_ip=client_ip,
                domain=domain,
                url=url,
                method=method,
                file_name=hint,
                scan_guard={
                    "cache_miss": True,
                    "scan_guard_decided": True,
                    "scan_guard_action": "Allowed",
                    "empty_caption": True,
                },
            )
            if ok:
                mark_duplicate_event(domain, dedupe_key)
        return False, "", "", 1, False

    # Dedup by cache_uid ONLY — many ChatGPT/Claude images share name "attachment".
    deduped: list[dict] = []
    seen_uids: set[str] = set()
    for entry in cached_list:
        uid = str(entry.get("cache_uid") or id(entry))
        if uid in seen_uids:
            continue
        seen_uids.add(uid)
        deduped.append(entry)
    cached_list = deduped[:_UPLOAD_FILE_QUEUE_MAX]
    # Prefer real filenames from Send body over placeholders.
    cached_list = _bind_real_filenames_to_cached_uploads(cached_list, raw_text or "")

    caption = ""
    try:
        caption = _extract_file_send_user_caption(
            raw_text or "",
            content_type or "",
            host,
            url,
        )
    except Exception:
        caption = ""
    # Any Target: typed note often lives in composer traffic — join like Claude.
    if not (caption or "").strip():
        try:
            caption = peek_recent_composer_caption(domain, max_age=120.0) or ""
        except Exception:
            caption = ""
        if caption:
            print(
                f"[Gateway Proxy] FILE CAPTION from composer | {domain} | "
                f"{caption[:80]!r}"
            )
    if not (caption or "").strip():
        try:
            from_body = extract_prompt_universal(
                (raw_text or "").encode("utf-8", errors="ignore"),
                content_type or "",
                host,
                url,
            )
            if from_body and looks_like_user_prompt(from_body) and not _looks_like_filename_only(from_body):
                if not _looks_like_document_body_dump(from_body) and not _is_opaque_wire_blob(from_body):
                    caption = from_body.strip()
        except Exception:
            pass
    # ChatGPT often puts short captions like "anlayse this" only in parts[] strings.
    if not (caption or "").strip() and (raw_text or ""):
        for m in re.finditer(
            r'"parts"\s*:\s*\[[^\]]{0,80000}?"((?:[^"\\]|\\.){1,400})"',
            raw_text or "",
            re.I,
        ):
            try:
                t = json.loads(f'"{m.group(1)}"')
            except Exception:
                t = (m.group(1) or "").replace("\\n", "\n").replace('\\"', '"')
            t = (t or "").strip()
            if not t or len(t) > 500:
                continue
            if t.startswith(("file-", "sediment://", "file-service://", "http")):
                continue
            if "." in t and len(t) < 180 and re.search(r"\.[A-Za-z0-9]{2,5}$", t):
                continue
            if looks_like_user_prompt(t) and not _looks_like_filename_only(t):
                caption = t
                break

    if caption:
        c_clean = caption.strip()
        if (
            not looks_like_user_prompt(c_clean)
            or _is_chat_metadata_token(c_clean)
            or _UUID_LIKE_RE.search(c_clean)
            or (c_clean.startswith("$") and (len(c_clean) >= 12 or "-" in c_clean))
            or _looks_like_filename_only(c_clean)
            or _is_opaque_wire_blob(c_clean)
        ):
            caption = ""

    cached_list = _trim_phantom_upload_caches(
        cached_list,
        raw_text or "",
        caption or "",
    )
    cached_list = _dedupe_cached_uploads_by_bytes(cached_list)
    # Final hard drop: never process bare "attachment" when Send JSON has a real name.
    send_real_names = [
        n for n in extract_all_attachment_filenames_from_send(raw_text or "")
        if _is_real_user_upload_name(n)
    ]
    if send_real_names:
        # Send names MUST bind onto rows. If a row has a name not in send_real_names,
        # it was a stale guess/pending name and must yield to send_real_names.
        send_names_lower = {s.lower(): s for s in send_real_names}
        used: set[str] = set()
        for e in cached_list:
            cur = (e.get("file_name") or "").strip()
            if cur.lower() in send_names_lower and cur.lower() not in used:
                e["file_name"] = send_names_lower[cur.lower()]
                used.add(cur.lower())
        leftover = [n for n in send_real_names if n.lower() not in used]
        li = 0
        for e in cached_list:
            cur = (e.get("file_name") or "").strip()
            if cur.lower() in used:
                continue
            if li < len(leftover):
                e["file_name"] = leftover[li]
                used.add(leftover[li].lower())
                li += 1
    # Pending names → nameless upload caches.
    still_fake = [e for e in cached_list if not _is_real_user_upload_name((e.get("file_name") or "").strip())]
    if still_fake:
        try:
            pending_names = list_pending_upload_names_for_domain(domain)
        except Exception:
            pending_names = []
        already = {
            (e.get("file_name") or "").strip().lower()
            for e in cached_list
            if _is_real_user_upload_name((e.get("file_name") or "").strip())
        }
        leftover_p = [n for n in pending_names if n.lower() not in already]
        for i, e in enumerate(still_fake):
            if i < len(leftover_p):
                e["file_name"] = leftover_p[i]

    # ChatGPT/Gemini/Copilot: create-file name often lands a beat after bytes.
    # Wait + bind before any Prompt Log so we never write "attachment" on Send.
    try:
        cached_list = wait_bind_real_upload_names(cached_list, raw_text or "", domain, retries=8, delay=0.20)
    except Exception as e:
        print(f"[Gateway Proxy WARNING] FILE NAME wait-bind failed: {e}")

    get_control_settings()
    block_all = controls_active("block_upload")
    base_upload_msg = (get_control_settings().get("upload_warning") or "").strip() or "Upload block"

    # Prepare rows first (names/labels), then extract+local-regex in parallel for speed.
    prepared: list[dict] = []
    hint = (file_name_hint or "").strip()
    n_cached = len(cached_list)
    for i, cached in enumerate(cached_list):
        fname = (cached.get("file_name") or "").strip() or (hint if i == 0 else "") or ""
        # Resolve real name from file_id registry before inventing document-N.pdf
        fid = (cached.get("file_id") or "").strip()
        if fid and not _is_real_user_upload_name(fname):
            remembered = lookup_upload_filename(fid)
            if remembered:
                fname = remembered
                cached["file_name"] = remembered
        cached_bytes = cached.get("raw_bytes") or b""
        if not isinstance(cached_bytes, (bytes, bytearray)):
            cached_bytes = b""
        cached_ct = (cached.get("content_type") or content_type or "").strip()
        is_audio = _looks_like_audio(bytes(cached_bytes), cached_ct, fname)

        if is_audio:
            if _is_fake_upload_name(fname) or fname.lower() in ("attachment", "audio.bin"):
                suffix = f" {i + 1}" if n_cached > 1 else ""
                fname = f"Voice Note{suffix}"
        elif _looks_like_image(bytes(cached_bytes), cached_ct, fname):
            if _is_fake_upload_name(fname) or fname.lower() in ("attachment", "image.png", "image.jpg"):
                suffix = f" {i + 1}" if n_cached > 1 else ""
                fname = f"Image{suffix}"
        elif _is_fake_upload_name(fname) or not fname:
            send_real = _prefer_real_filenames(
                n for n in extract_all_attachment_filenames_from_send(raw_text or "")
                if _is_real_user_upload_name(n)
            )
            if i < len(send_real):
                fname = send_real[i]
            else:
                by_bytes = ""
                try:
                    by_bytes = lookup_upload_name_by_bytes(bytes(cached_bytes))
                except Exception:
                    by_bytes = ""
                if by_bytes:
                    fname = by_bytes
                else:
                    pending = ""
                    try:
                        pending = peek_pending_upload_name_for_domain(domain)
                    except Exception:
                        pending = ""
                    if pending:
                        fname = pending
                    else:
                        is_img = _looks_like_image(bytes(cached_bytes), cached_ct, fname)
                        suffix = f" {i + 1}" if n_cached > 1 else ""
                        fname = f"Image{suffix}" if is_img else f"Document{suffix}"
            cached["file_name"] = fname
            if _is_real_user_upload_name(fname) and cached_bytes:
                try:
                    remember_upload_name_by_bytes(bytes(cached_bytes), fname)
                except Exception:
                    pass
        display_label = _display_label_for_upload(fname, bytes(cached_bytes), cached_ct)
        if is_audio and display_label.lower() in ("attachment", "audio.bin"):
            suffix = f" {i + 1}" if n_cached > 1 else ""
            display_label = f"Voice Note{suffix}"
        elif _looks_like_image(bytes(cached_bytes), cached_ct, fname) and display_label.lower() in ("attachment", "image.png"):
            suffix = f" {i + 1}" if n_cached > 1 else ""
            display_label = f"Image{suffix}"
        elif display_label.lower() in ("attachment", "document.pdf", "unknown"):
            suffix = f" {i + 1}" if n_cached > 1 else ""
            display_label = f"Document{suffix}"
        prepared.append({
            "file_label": display_label,
            "store_name": fname,
            "cached_bytes": bytes(cached_bytes),
            "cached_ct": cached_ct,
            "cache_uid": str(cached.get("cache_uid") or id(cached)),
            "cached": cached,
        })

    # ALWAYS one row per distinct bytes (even when all have real names).
    unique_prep: list[dict] = []
    seen_fps: set[str] = set()
    seen_names: set[str] = set()
    for p in prepared:
        b = p.get("cached_bytes") or b""
        fp = _bytes_name_fingerprint(b)
        lbl = (p.get("store_name") or p.get("file_label") or "").strip()
        lbl_key = lbl.lower()
        if fp and fp in seen_fps:
            continue
        if lbl_key and _is_real_user_upload_name(lbl) and lbl_key in seen_names and (not b or len(b) < 96):
            continue
        if fp:
            seen_fps.add(fp)
        if lbl_key and _is_real_user_upload_name(lbl):
            seen_names.add(lbl_key)
        unique_prep.append(p)
    if unique_prep:
        prepared = unique_prep
        n_cached = len(prepared)

    def _scan_one(prep: dict) -> dict:
        scanned, local_hit, local_name, local_action, excerpt, upload_images, scan_evaluated, scan_eval_error = _scan_upload_for_rules(
            prep["cached_bytes"],
            prep["cached_ct"],
            raw_text or "",
            prep["store_name"],
            prep["cached"],
            platform=platform,
            domain=domain,
            client_ip=client_ip,
            url=url,
            method=method,
            # Local extract + regex only here — one optional AI-bot call below (fast path).
            skip_backend=True,
            extra_context="",
        )
        return {
            "file_label": prep["file_label"],
            "store_name": prep["store_name"],
            "cached_bytes": prep["cached_bytes"],
            "cached_ct": prep["cached_ct"],
            "scanned": scanned or "",
            "excerpt": excerpt or "",
            "upload_images": upload_images or [],
            "rule_hit": bool(local_hit),
            "rule_name": local_name or "",
            "rule_action": (local_action or "").upper(),
            "scan_evaluated": bool(scan_evaluated),
            "scan_eval_error": scan_eval_error or "",
            "cache_uid": prep["cache_uid"],
        }

    file_rows: list[dict] = []
    if len(prepared) <= 1:
        file_rows = [_scan_one(p) for p in prepared]
    else:
        from concurrent.futures import ThreadPoolExecutor, as_completed
        workers = min(4, len(prepared))
        with ThreadPoolExecutor(max_workers=workers) as pool:
            futs = {pool.submit(_scan_one, p): idx for idx, p in enumerate(prepared)}
            ordered: list[dict | None] = [None] * len(prepared)
            for fut in as_completed(futs):
                ordered[futs[fut]] = fut.result()
            file_rows = [r for r in ordered if r is not None]

    # Optional AI bot pass on combined extract (skip if regex-only).
    plat = (platform or domain or "Browser AI").strip()
    dom = (domain or host or "").strip()
    need_bot = bool(has_ai_bot_rules() and dom and not block_all)
    any_local_block = any(
        r.get("rule_hit") and (r.get("rule_action") or "").upper() == "BLOCK" for r in file_rows
    )
    if need_bot and not any_local_block:
        combined = "\n\n".join(
            x for x in (
                [(r.get("scanned") or "").strip() for r in file_rows] + [caption]
            ) if x
        ).strip()
        vision: list[str] = []
        for r in file_rows:
            for img in (r.get("upload_images") or [])[:2]:
                if img and img not in vision:
                    vision.append(img)
                if len(vision) >= 4:
                    break
        if combined or vision:
            try:
                allowed, rt, action, _, _, eval_err = send_to_backend(
                    plat,
                    dom,
                    combined[:40_000],
                    client_ip,
                    url,
                    method or "POST",
                    upload_images=vision or None,
                    evaluation_only=True,
                    extracted_text=combined[:40_000],
                )
                is_rule_backend = (not allowed) or (action or "").upper() in ("BLOCK", "BLOCKED", "REDACT", "REDACTED", "WARN", "WARNED")
                if is_rule_backend or not eval_err:
                    for r in file_rows:
                        hit, name, act = _merge_file_scan_backend(
                            bool(r.get("rule_hit")),
                            r.get("rule_name") or "",
                            (r.get("rule_action") or "").upper(),
                            allowed,
                            rt or "AI Guard Bot Policy",
                            action or ("Blocked" if not allowed else "Allowed"),
                        )
                        r["rule_hit"] = bool(hit)
                        r["rule_name"] = name or ""
                        r["rule_action"] = (act or "").upper()
                        r["scan_evaluated"] = True
            except Exception as e:
                print(f"[Gateway Proxy] combined file bot scan failed (allowed): {e}")

    cap_hit = False
    cap_name = ""
    cap_action = ""
    if caption:
        cap_hit, cap_name, cap_action = match_guard_rules_on_text(caption)
        cap_action = (cap_action or "").upper()
        if cap_action == "ALERT":
            cap_action = "WARN"
        # Caption AI bot only when bots configured and caption not already BLOCK.
        if (
            not (cap_hit and cap_action == "BLOCK")
            and need_bot
            and not any_local_block
        ):
            try:
                allowed, rt, action, _, _, eval_err = send_to_backend(
                    plat,
                    dom,
                    caption[:40_000],
                    client_ip,
                    url,
                    method or "POST",
                    evaluation_only=True,
                )
                is_rule_backend = (not allowed) or (action or "").upper() in ("BLOCK", "BLOCKED", "REDACT", "REDACTED", "WARN", "WARNED")
                if is_rule_backend or not eval_err:
                    cap_hit, cap_name, cap_action = _merge_file_scan_backend(
                        cap_hit, cap_name, cap_action, allowed, rt or "AI Guard Bot Policy", action or ("Blocked" if not allowed else "Allowed"),
                    )
                    cap_action = (cap_action or "").upper()
                    if cap_action == "ALERT":
                        cap_action = "WARN"
            except Exception as e:
                print(f"[Gateway Proxy] caption rule scan failed (allowed): {e}")

    any_file_blocked = any(
        r.get("rule_hit") and (r.get("rule_action") or "").upper() == "BLOCK"
        for r in file_rows
    )
    caption_blocked = bool(cap_hit and cap_action == "BLOCK")

    blocking_rule = ""
    for r in file_rows:
        if r.get("rule_hit") and (r.get("rule_action") or "").upper() == "BLOCK":
            blocking_rule = r.get("rule_name") or "Guard Rule (file content)"
            break
    if not blocking_rule and caption_blocked:
        blocking_rule = cap_name or "Guard Rule (prompt)"

    any_file_redacted = any(
        r.get("rule_hit") and (r.get("rule_action") or "").upper() in ("REDACT", "WARN")
        for r in file_rows
    )
    caption_redacted = bool(cap_hit and cap_action in ("REDACT", "WARN"))

    redact_rule = ""
    for r in file_rows:
        if r.get("rule_hit") and (r.get("rule_action") or "").upper() in ("REDACT", "WARN"):
            redact_rule = r.get("rule_name") or ""
            break
    if not redact_rule and caption_redacted:
        redact_rule = cap_name or ""

    should_block = False
    block_msg = ""
    redact_notice = ""

    # Any file/caption BLOCK → block whole Send. Per-file log stays truthful.
    if block_all or any_file_blocked or caption_blocked:
        should_block = True
        if block_all:
            block_msg = base_upload_msg
        else:
            rule_warn = _warning_for_rule_name(blocking_rule)
            left = (rule_warn or base_upload_msg).strip() or "Upload block"
            block_msg = f"{left} -- {blocking_rule}" if blocking_rule else left
    elif any_file_redacted or caption_redacted:
        redact_notice = _redact_notice_for_rule(redact_rule)

    n_files = len(file_rows)
    # Prompt Logs format: realname.pdf -- typed caption  (Claude-style, all Targets)
    if caption:
        c_clean = caption.strip()
        if (
            not looks_like_user_prompt(c_clean)
            or _is_chat_metadata_token(c_clean)
            or _UUID_LIKE_RE.search(c_clean)
            or (c_clean.startswith("$") and (len(c_clean) >= 12 or "-" in c_clean))
            or _looks_like_filename_only(c_clean)
            or _is_opaque_wire_blob(c_clean)
        ):
            caption = ""
    caption_bit = f" -- {caption}" if caption else ""

    for idx, row in enumerate(file_rows):
        if " -- $" in row.get("file_label", ""):
            row["file_label"] = row["file_label"].split(" -- $", 1)[0].strip()
        tag = _upload_log_tag(row["file_label"], row["cached_ct"], row["cached_bytes"])
        row_text = (row.get("scanned") or "").strip()
        row_imgs = row.get("upload_images") or []
        row_hit = bool(row.get("rule_hit"))
        row_act = (row.get("rule_action") or "").upper()
        row_name = (row.get("rule_name") or "").strip()
        row_eval = bool(row.get("scan_evaluated"))
        row_err = str(row.get("scan_eval_error") or "")

        this_scan_guard = _file_scan_guard_metadata(
            row_text,
            row_imgs,
            row_hit,
            row_name,
            row_act,
            scan_evaluated=row_eval,
            scan_eval_error=row_err,
        )
        this_scan_guard["multi_file_count"] = n_files
        this_scan_guard["multi_file_index"] = idx + 1
        if n_files > 1 and should_block:
            # Site blocked the whole Send even if this particular file row is Allowed.
            this_scan_guard["multi_file_send_blocked"] = True
            if blocking_rule:
                this_scan_guard["multi_file_send_block_rule"] = blocking_rule
        if caption:
            this_scan_guard["user_caption"] = caption[:500]

        if block_all:
            file_is_blocked = True
            file_status = "Blocked (Block Upload)"
            file_reason = "Block Upload"
            dedupe_type = "block-all"
        elif row_hit and row_act == "BLOCK":
            file_is_blocked = True
            file_status = f"Blocked ({row_name or 'policy'})"
            file_reason = row_name or "Guard Rule (file content)"
            dedupe_type = f"block|{file_reason}"
        elif row_hit and row_act == "REDACT":
            file_is_blocked = False
            file_status = f"Redacted ({row_name or 'policy'})"
            file_reason = ""
            dedupe_type = f"redact|{row_name}"
        elif row_hit and row_act in ("WARN", "ALERT"):
            file_is_blocked = False
            file_status = f"Warned ({row_name or 'policy'})"
            file_reason = ""
            dedupe_type = f"warn|{row_name}"
        else:
            file_is_blocked = False
            file_status = "Allowed"
            file_reason = ""
            dedupe_type = "allowed"
            this_scan_guard["scan_guard_decided"] = True
            this_scan_guard["scan_guard_action"] = "Allowed"
            this_scan_guard["scan_rule_triggered"] = ""

        if n_files > 1:
            prompt_log = f"{tag} {row['file_label']} ({idx + 1}/{n_files}){caption_bit} — {file_status}"
        else:
            prompt_log = f"{tag} {row['file_label']}{caption_bit} — {file_status}"

        dedupe_key = f"upload-send-{dedupe_type}|{row['cache_uid']}"
        if is_duplicate_event(domain, dedupe_key, ttl=BLOCK_DEDUPE_TTL, mark=False):
            continue

        print(
            f"[Gateway Proxy] FILE SEND LOG | {client_ip} -> {host} | {row['file_label']} | "
            f"verdict={file_status} (is_blocked={file_is_blocked}) | multi={idx + 1}/{n_files}"
        )

        def _post_row(
            *,
            _prompt_log=prompt_log,
            _store_name=row.get("store_name") or row["file_label"],
            _blocked=file_is_blocked,
            _reason=file_reason,
            _bytes=row["cached_bytes"],
            _ct=row["cached_ct"],
            _text=row_text or (caption if idx == 0 else ""),
            _imgs=row_imgs,
            _sg=this_scan_guard,
            _dedupe=dedupe_key,
            _label=row["file_label"],
        ) -> None:
            try:
                ok = post_upload_intercept(
                    platform=platform,
                    prompt=_prompt_log,
                    client_ip=client_ip,
                    domain=domain,
                    url=url,
                    method=method,
                    file_name=_store_name,
                    is_blocked=_blocked,
                    blocked_reason=_reason,
                    raw_bytes=_bytes,
                    content_type=_ct,
                    extracted_text=_text,
                    upload_images=_imgs,
                    scan_guard=_sg,
                )
                if ok:
                    mark_duplicate_event(domain, _dedupe)
                else:
                    print(f"[Gateway Proxy WARNING] File log failed to post | {_label}")
            except Exception as e:
                print(f"[Gateway Proxy WARNING] File log async failed | {_label}: {e}")

        # Do not block the chat Send on Prompt Log upload — fire-and-forget.
        threading.Thread(target=_post_row, daemon=True, name="gateway-file-log").start()

    if should_block:
        remember_file_send_block(
            domain,
            names=[
                (r.get("file_label") or r.get("file_name") or "").strip()
                for r in file_rows
            ],
            ids=_extract_file_ids_from_chat(raw_text or ""),
            message=block_msg,
        )
    return should_block, block_msg, redact_notice, n_files, bool(caption) or n_files > 0


def _extract_file_send_user_caption(
    raw_text: str,
    content_type: str = "",
    host: str = "",
    url: str = "",
) -> str:
    """
    Pull the short typed chat text that accompanies a file Send (Claude/Gemini/ChatGPT).
    Must not return PDF/doc dumps or filename-only tokens.
    """
    # ChatGPT multimodal JSON keys often get scraped as "captions" (e.g. asset_pointer).
    _WIRE_CAPTION_REJECT = frozenset({
        "asset_pointer", "content_type", "file_id", "mime_type", "multimodal_text",
        "text", "parts", "author", "role", "user", "assistant", "system", "tool",
        "image_asset_pointer", "audio_asset_pointer", "file", "files", "attachments",
        "sediment", "name", "size", "width", "height", "id", "type", "model",
    })

    candidates: list[str] = []

    def _accept(got: str) -> None:
        t = (got or "").strip()
        if not t or len(t) > 50_000:
            return
        low = t.lower()
        if low in _WIRE_CAPTION_REJECT:
            return
        if low.endswith("_pointer") or low.endswith("_id") or low.endswith("_type"):
            return
        if re.fullmatch(r"[a-z][a-z0-9_]{2,40}", low) and "_" in low:
            return  # snake_case schema keys, not typed chat
        if not looks_like_user_prompt(t):
            return
        if _is_chat_metadata_token(t):
            return
        if _UUID_LIKE_RE.search(t):
            return
        if t.startswith("$") and (re.search(r"[0-9a-fA-F]{4,}", t) or "-" in t):
            return
        if _looks_like_document_body_dump(t):
            return
        if _looks_like_filename_only(t):
            return
        if _is_google_wire_blob(t) or _is_opaque_wire_blob(t):
            return
        if t.startswith(("[null,", '[[["', '{"type":', '{"counters":', "[FILE UPLOAD", "[VOICE UPLOAD")):
            return
        # Prefer short captions; still allow longer typed instructions with a file.
        candidates.append(t)

    try:
        from_body = extract_prompt_universal(
            (raw_text or "").encode("utf-8", errors="ignore"),
            content_type or "",
            host,
            url,
        )
        if from_body:
            _accept(from_body)
    except Exception:
        pass

    body = raw_text or ""
    # Claude / Anthropic: {"type":"text","text":"..."}
    for m in re.finditer(
        r'"type"\s*:\s*"text"\s*,\s*"text"\s*:\s*"((?:[^"\\]|\\.)*)"',
        body,
    ):
        try:
            _accept(json.loads(f'"{m.group(1)}"'))
        except Exception:
            _accept(m.group(1).replace("\\n", "\n").replace('\\"', '"'))
    # Alternate order: "text":"...","type":"text"
    for m in re.finditer(
        r'"text"\s*:\s*"((?:[^"\\]|\\.)*)"\s*,\s*"type"\s*:\s*"text"',
        body,
    ):
        try:
            _accept(json.loads(f'"{m.group(1)}"'))
        except Exception:
            _accept(m.group(1).replace("\\n", "\n").replace('\\"', '"'))
    # Gemini / Google: parts[].text (skip huge blobs)
    for m in re.finditer(r'"text"\s*:\s*"((?:[^"\\]|\\.){1,2000})"', body):
        try:
            t = json.loads(f'"{m.group(1)}"')
        except Exception:
            t = m.group(1).replace("\\n", "\n").replace('\\"', '"')
        if len((t or "").strip()) <= 2000:
            _accept(t or "")
    # ChatGPT parts that are a bare number: "parts":[9080808782] or "parts":["9080"]
    for m in re.finditer(r'"parts"\s*:\s*\[\s*(-?\d+(?:\.\d+)?)', body):
        _accept(m.group(1))
    for m in re.finditer(r'"parts"\s*:\s*\[[^\]]{0,8000}?"(\d{1,32})"', body):
        _accept(m.group(1))
    # ChatGPT parts: "parts":["hello"]
    for m in re.finditer(
        r'"content_type"\s*:\s*"text"\s*,\s*"parts"\s*:\s*\[\s*"((?:[^"\\]|\\.)*)"',
        body,
    ):
        try:
            _accept(json.loads(f'"{m.group(1)}"'))
        except Exception:
            _accept(m.group(1))
    # ChatGPT reverse order: "parts":["hello"],"content_type":"text"
    for m in re.finditer(
        r'"parts"\s*:\s*\[\s*"((?:[^"\\]|\\.)*)"\s*\]\s*,\s*"content_type"\s*:\s*"text"',
        body,
    ):
        try:
            _accept(json.loads(f'"{m.group(1)}"'))
        except Exception:
            _accept(m.group(1))
    # ChatGPT multimodal_text: parts may mix file objects + a trailing string caption
    for m in re.finditer(
        r'"content_type"\s*:\s*"multimodal_text"\s*,\s*"parts"\s*:\s*\[([\s\S]{0,80000}?)\]',
        body,
        re.I,
    ):
        block = m.group(1) or ""
        for sm in re.finditer(r'"((?:[^"\\]|\\.){1,2000})"', block):
            try:
                t = json.loads(f'"{sm.group(1)}"')
            except Exception:
                t = sm.group(1).replace("\\n", "\n").replace('\\"', '"')
            t = (t or "").strip()
            # Skip file ids / pointers / mime junk / JSON key names
            if not t or t.startswith(("file-", "sediment://", "file-service://", "http")):
                continue
            if t.lower() in (
                "asset_pointer", "image_asset_pointer", "audio_asset_pointer",
                "content_type", "file_id", "mime_type", "multimodal_text",
            ):
                continue
            if "." in t and len(t) < 180 and re.search(r"\.[A-Za-z0-9]{2,5}$", t):
                continue  # filename-looking token
            if looks_like_user_prompt(t):
                _accept(t)
    # Loose ChatGPT author:user parts string after a file part
    for m in re.finditer(
        r'"author"\s*:\s*\{\s*"role"\s*:\s*"user"[\s\S]{0,4000}?"parts"\s*:\s*\[[\s\S]{0,20000}?"((?:[^"\\]|\\.){1,2000})"',
        body,
        re.I,
    ):
        try:
            t = json.loads(f'"{m.group(1)}"')
        except Exception:
            t = m.group(1)
        if looks_like_user_prompt(t or ""):
            _accept(t or "")
    # ChatGPT conversation create: top-level "messages" with string part after file_id
    for m in re.finditer(
        r'"role"\s*:\s*"user"[\s\S]{0,12000}?"parts"\s*:\s*\[[\s\S]{0,40000}?\]\s*,\s*"content_type"\s*:\s*"(?:text|multimodal_text)"',
        body,
        re.I,
    ):
        block = m.group(0) or ""
        for sm in re.finditer(r'"((?:[^"\\]|\\.){1,500})"', block):
            try:
                t = json.loads(f'"{sm.group(1)}"')
            except Exception:
                t = sm.group(1)
            t = (t or "").strip()
            if not t or len(t) > 500:
                continue
            if t.startswith(("file-", "sediment://", "file-service://", "http", "{", "[")):
                continue
            if "." in t and len(t) < 180 and re.search(r"\.[A-Za-z0-9]{2,5}$", t):
                continue
            if looks_like_user_prompt(t):
                _accept(t)
    # ChatGPT / Copilot: message string fields next to attachments
    for m in re.finditer(
        r'"(?:input_text|message|prompt|query|text|user_message|content)"\s*:\s*"((?:[^"\\]|\\.){1,2000})"',
        body,
        re.I,
    ):
        try:
            t = json.loads(f'"{m.group(1)}"')
        except Exception:
            t = m.group(1).replace("\\n", "\n").replace('\\"', '"')
        if looks_like_user_prompt(t or ""):
            _accept(t or "")
    # Copilot event / messages parts: {"text":"hiii"} inside event payloads
    for m in re.finditer(
        r'"role"\s*:\s*"user"[\s\S]{0,8000}?"(?:text|content)"\s*:\s*"((?:[^"\\]|\\.){1,2000})"',
        body,
        re.I,
    ):
        try:
            t = json.loads(f'"{m.group(1)}"')
        except Exception:
            t = m.group(1)
        if looks_like_user_prompt(t or ""):
            _accept(t or "")

    if not candidates:
        return ""
    # The first candidate is the extracted user message. A typed number or symbol
    # string must not lose to a longer field scraped from the same request.
    primary = candidates[0].strip()
    if primary.isdigit() or _is_typed_numeric_prompt(primary) or _is_symbol_prompt(primary):
        return primary
    for c in candidates:
        s = (c or "").strip()
        if s.isdigit() or _is_typed_numeric_prompt(s):
            return s
    if primary and looks_like_user_prompt(primary) and len(primary) <= 500 and not _looks_like_document_body_dump(primary):
        return primary
    short = [c for c in candidates if len(c) <= 500]
    pool = short or candidates
    pool.sort(key=lambda s: (len(s), candidates.index(s)))
    return pool[0].strip()


def _upload_log_tag(file_name: str = "", content_type: str = "", raw_bytes: bytes = b"") -> str:
    """Prompt Log prefix: VOICE vs FILE."""
    if _looks_like_audio(raw_bytes or b"", content_type, file_name):
        return "[VOICE UPLOAD]"
    fn = (file_name or "").lower()
    if fn.endswith((".wav", ".mp3", ".m4a", ".ogg", ".webm", ".flac", ".aac", ".opus", ".wma")):
        return "[VOICE UPLOAD]"
    return "[FILE UPLOAD]"


def post_upload_intercept(
    *,
    platform: str,
    prompt: str,
    client_ip: str,
    domain: str,
    url: str,
    method: str,
    file_name: str = "",
    is_blocked: bool = False,
    blocked_reason: str = "",
    raw_bytes: bytes | None = None,
    content_type: str = "",
    extracted_text: str = "",
    upload_images: list[str] | None = None,
    scan_guard: dict | None = None,
) -> bool:
    """
    Log file event on Send.
    Always posts filename + extracted text (permanent in Prompt Logs).
    Also attaches file bytes (when present, capped) so backend can temp-store
    for ~10 minutes of View/Download, then auto-delete the file only.
    """
    is_img = bool((content_type and "image/" in content_type.lower()) or _looks_like_image(raw_bytes or b"", content_type, file_name))
    is_aud = bool((content_type and "audio/" in content_type.lower()) or _looks_like_audio(raw_bytes or b"", content_type, file_name))
    fallback_name = "Voice Note" if is_aud else ("Image" if is_img else "Document")
    if not file_name or _is_fake_upload_name(file_name) or file_name.lower() in ("attachment", "document.pdf", "unknown"):
        file_name = fallback_name

    safe_name = (file_name or fallback_name).replace('"', "").replace("\r", "").replace("\n", "")
    if not safe_name:
        safe_name = fallback_name

    if "[FILE UPLOAD] attachment" in prompt:
        prompt = prompt.replace("[FILE UPLOAD] attachment", f"[FILE UPLOAD] {safe_name}")

    metadata = {
        "domain": domain,
        "url": url,
        "method": method,
        "is_blocked": bool(is_blocked),
        "upload_scan": True,
        "file_name": safe_name,
        **_agent_metadata_fields(),
    }
    if scan_guard:
        metadata.update(scan_guard)
    if blocked_reason:
        metadata["blocked_reason"] = blocked_reason
    ext = (extracted_text or "").strip()
    if ext:
        metadata["extracted_text"] = ext[:50_000]
    if upload_images:
        metadata["upload_images"] = upload_images[:10]

    ctype = (content_type or "application/octet-stream").strip() or "application/octet-stream"

    # Prefer clean payload (multipart unwrap / PDF island) for View storage
    file_payload = b""
    file_ctype = ctype
    file_label = safe_name
    try:
        if raw_bytes and len(raw_bytes) >= 1:
            payload, sniffed_ct, sniffed_name = extract_upload_file_payload(
                raw_bytes, content_type, safe_name,
            )
            tiny_txt = (safe_name or "").lower().endswith((".txt", ".md", ".csv", ".log"))
            if payload and (len(payload) >= 32 or (tiny_txt and len(payload) >= 1)):
                file_payload = payload
                if sniffed_ct:
                    file_ctype = sniffed_ct
                if sniffed_name and sniffed_name.lower() not in _FAKE_UPLOAD_NAMES:
                    file_label = sniffed_name.replace('"', "").replace("\r", "").replace("\n", "")
            else:
                # Raw body if it does not look like chat JSON metadata
                sample = raw_bytes[:64].lstrip()
                if sample[:1] not in (b"{", b"["):
                    file_payload = raw_bytes
    except Exception as e:
        print(f"[Gateway Proxy] upload payload prepare failed (log without file): {e}")
        file_payload = b""

    max_attach = 20 * 1024 * 1024
    if len(file_payload) > max_attach:
        print(
            f"[Gateway Proxy] upload file too large for temp View store "
            f"({len(file_payload)} bytes) — logging name+extract only"
        )
        file_payload = b""

    try:
        boundary = f"----Gateway{int(time.time() * 1000)}"
        parts: list[bytes] = []

        def add_field(name: str, value: str) -> None:
            parts.append(
                f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"\r\n\r\n{value}\r\n".encode("utf-8")
            )

        add_field("platform", platform)
        add_field("prompt", prompt)
        add_field("client_ip", client_ip)
        add_field("agent_id", GATEWAY_AGENT_ID or "")
        add_field("agent_hostname", GATEWAY_AGENT_HOSTNAME or "")
        add_field("agent_type", GATEWAY_AGENT_TYPE or "endpoint")
        add_field("file_name", file_label)
        add_field("content_type", file_ctype)
        add_field("metadata", json.dumps(metadata))
        if file_payload:
            # Backend reads form file field "file" for 10-minute temp View storage
            hdr = (
                f"--{boundary}\r\n"
                f"Content-Disposition: form-data; name=\"file\"; filename=\"{file_label}\"\r\n"
                f"Content-Type: {file_ctype}\r\n\r\n"
            ).encode("utf-8")
            parts.append(hdr)
            parts.append(file_payload)
            parts.append(b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode("utf-8"))
        body = b"".join(parts)
        req = urllib.request.Request(
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept-file",
            data=body,
            headers=_backend_headers({"Content-Type": f"multipart/form-data; boundary={boundary}"}),
            method="POST",
        )
        # Larger timeout when uploading file bytes (async log path — keep modest)
        timeout = 12 if file_payload else 4
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            if 200 <= getattr(resp, "status", 200) < 300:
                return True
    except Exception as e:
        print(f"[Gateway Proxy WARNING] intercept-file failed, falling back to JSON: {e}")

    try:
        payload_json = json.dumps({
            "platform": platform,
            "prompt": prompt,
            "client_ip": client_ip,
            **_agent_wire_fields(),
            "upload_images": upload_images or [],
            "metadata": metadata,
        }).encode("utf-8")
        req = urllib.request.Request(
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept",
            data=payload_json,
            headers=_backend_headers({"Content-Type": "application/json"}),
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=4) as resp:
            return 200 <= getattr(resp, "status", 200) < 300
    except Exception as e:
        print(f"[Gateway Proxy WARNING] upload intercept JSON failed: {e}")
        return False


def _extract_pdf_pypdf(data: bytes) -> str:
    """PDF text via pypdf library."""
    if not data:
        return ""
    if b"%PDF" not in data[:1024] and not data.startswith(b"%PDF"):
        start = data.find(b"%PDF")
        if start < 0:
            return ""
        data = data[start:]
    try:
        from pypdf import PdfReader

        reader = PdfReader(io.BytesIO(data), strict=False)
        parts: list[str] = []
        for page in reader.pages[:50]:
            try:
                t = page.extract_text() or ""
            except Exception:
                t = ""
            if t.strip():
                parts.append(t)
        return "\n".join(parts).strip()[:200_000]
    except Exception:
        return ""


def _extract_pdf_regex(data: bytes) -> str:
    """PDF text via regex / printable runs (no pypdf)."""
    if not data:
        return ""
    if b"%PDF" not in data[:1024] and not data.startswith(b"%PDF"):
        start = data.find(b"%PDF")
        if start < 0:
            return ""
        data = data[start:]
    try:
        raw = data.decode("latin-1", errors="ignore")
    except Exception:
        return ""
    chunks = re.findall(r"\((?:\\.|[^\\)]){3,}\)|\[[^\]]{3,}\]", raw)
    texts = []
    for c in chunks[:5000]:
        s = c.strip("()[]")
        s = s.replace("\\n", "\n").replace("\\r", "").replace("\\t", "\t")
        s = re.sub(r"\\[0-9]{3}", " ", s)
        s = re.sub(r"[^\x09\x0a\x0d\x20-\x7e\u00a0-\uffff]+", " ", s)
        if len(s.strip()) >= 3:
            texts.append(s.strip())
    for m in re.finditer(r"[ -~]{12,}", raw):
        texts.append(m.group(0))
    return "\n".join(texts)[:200_000]


def _normalize_pdf_bytes(data: bytes) -> bytes:
    """Return PDF payload bytes (direct or embedded in wrapper)."""
    if not data:
        return b""
    if data.startswith(b"%PDF-"):
        return data
    if b"%PDF" in data[:1024]:
        start = data.find(b"%PDF")
        if start >= 0:
            return data[start:]
    start = data.find(b"%PDF-")
    if start >= 0:
        return data[start:]
    return data


def _pdf_text_sufficient(text: str, min_chars: int = 40) -> bool:
    """True when PDF text layer looks usable (not empty/garbage)."""
    if not text:
        return False
    compact = re.sub(r"\s+", " ", text).strip()
    if len(compact) < min_chars:
        return False
    printable = sum(1 for ch in compact if ch.isprintable())
    return printable / max(1, len(compact)) >= 0.85


def _extract_pdf_embedded_images_ocr(data: bytes, max_images: int = 10) -> str:
    """OCR embedded raster images inside a PDF (common for scanned documents)."""
    parts: list[str] = []
    for b64 in _extract_pdf_images(data, max_images=max_images):
        try:
            raw = base64.b64decode(b64)
        except Exception:
            continue
        t = _extract_image_windows_ocr(raw)
        if not (t or "").strip():
            t = _extract_image_tesseract(raw)
        if (t or "").strip():
            parts.append(t.strip())
    return "\n\n".join(parts)


def _extract_pdf_pymupdf_ocr(data: bytes, max_pages: int = 10) -> str:
    """Render PDF pages to images and OCR (scanned PDFs without a text layer)."""
    data = _normalize_pdf_bytes(data)
    if not data:
        return ""
    try:
        try:
            import pymupdf as fitz
        except ImportError:
            import fitz
    except ImportError:
        return ""
    parts: list[str] = []
    doc = None
    try:
        doc = fitz.open(stream=data, filetype="pdf")
        for i, page in enumerate(doc):
            if i >= max_pages:
                break
            try:
                pix = page.get_pixmap(matrix=fitz.Matrix(2.0, 2.0), alpha=False)
                png = pix.tobytes("png")
            except Exception:
                continue
            t = _extract_image_windows_ocr(png)
            if not (t or "").strip():
                t = _extract_image_tesseract(png)
            if (t or "").strip():
                parts.append(t.strip())
    except Exception as e:
        print(f"[Gateway Proxy] PDF page OCR failed (allowed): {e}")
    finally:
        if doc is not None:
            try:
                doc.close()
            except Exception:
                pass
    return "\n\n".join(parts)


def _extract_pdf_ocr(data: bytes, max_pages: int = 10) -> str:
    """OCR fallback for scanned / low-text PDFs."""
    data = _normalize_pdf_bytes(data)
    if not data:
        return ""
    embedded = _extract_pdf_embedded_images_ocr(data, max_images=max_pages)
    embedded_compact = re.sub(r"\s+", "", embedded or "")
    if len(embedded_compact) >= 16:
        return embedded[:200_000]
    rendered = _extract_pdf_pymupdf_ocr(data, max_pages=max_pages)
    if rendered and embedded:
        return (embedded + "\n\n" + rendered)[:200_000]
    return (rendered or embedded or "")[:200_000]


def _extract_pdf_text_smart(data: bytes) -> str:
    """
    PDF text: fast text-layer extract first; OCR only when text layer is missing/weak.
    """
    data = _normalize_pdf_bytes(data)
    if not data:
        return ""
    text = _extract_pdf_pypdf(data)
    if _pdf_text_sufficient(text):
        return text[:200_000]
    regex_t = _extract_pdf_regex(data)
    if _pdf_text_sufficient(regex_t):
        return regex_t[:200_000]
    ocr_t = _extract_pdf_ocr(data)
    if ocr_t.strip():
        print(f"[Gateway Proxy] PDF OCR extracted {len(ocr_t.strip())} chars (scanned/low-text PDF)")
        return ocr_t[:200_000]
    return (text or regex_t or "")[:200_000]


def _extract_pdf_images(data: bytes, max_images: int = 10) -> list[str]:
    """Extract embedded images from PDF pages as base64 strings (in-memory only)."""
    if not data:
        return []
    if b"%PDF" not in data[:1024] and not data.startswith(b"%PDF"):
        start = data.find(b"%PDF")
        if start < 0:
            return []
        data = data[start:]
    out: list[str] = []
    try:
        from pypdf import PdfReader

        reader = PdfReader(io.BytesIO(data), strict=False)
        for page in reader.pages[:20]:
            if len(out) >= max_images:
                break
            images = getattr(page, "images", None)
            if not images:
                continue
            for img in images:
                if len(out) >= max_images:
                    break
                raw = getattr(img, "data", None)
                if raw and len(raw) >= 64:
                    out.append(base64.b64encode(raw).decode("ascii"))
    except Exception as e:
        print(f"[Gateway Proxy] pdf image extract failed (allowed): {e}")
    return out


def _extract_office_images(data: bytes, max_images: int = 10) -> list[str]:
    """Extract embedded images from docx/xlsx/pptx for LLaVA vision rules."""
    zdata = _office_zip_bytes(data)
    if not zdata:
        return []
    out: list[str] = []
    image_ext = (".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff")
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            for name in zf.namelist():
                if len(out) >= max_images:
                    break
                low = name.lower()
                if not (
                    low.startswith("word/media/")
                    or low.startswith("xl/media/")
                    or low.startswith("ppt/media/")
                ):
                    continue
                if not any(low.endswith(ext) for ext in image_ext):
                    continue
                raw = zf.read(name)
                if raw and len(raw) >= 64:
                    out.append(base64.b64encode(raw).decode("ascii"))
    except Exception as e:
        print(f"[Gateway Proxy] office image extract failed (allowed): {e}")
    return out


def _upload_images_for_vision(raw_bytes: bytes, content_type: str = "", file_name: str = "", max_images: int = 10) -> list[str]:
    """Build base64 image list from an upload for LLaVA vision rules."""
    if not raw_bytes:
        return []
    kind = _classify_upload_kind(raw_bytes, content_type, file_name)
    if kind == "image":
        return [base64.b64encode(raw_bytes).decode("ascii")]
    if kind == "pdf":
        return _extract_pdf_images(raw_bytes, max_images=max_images)
    if kind in ("docx", "xlsx", "pptx"):
        return _extract_office_images(raw_bytes, max_images=max_images)
    return []


def _xml_local(tag: str) -> str:
    if "}" in tag:
        return tag.rsplit("}", 1)[-1]
    return tag


def _office_zip_bytes(data: bytes) -> bytes | None:
    """Return ZIP payload for OOXML (docx/xlsx/pptx), including multipart-wrapped bodies."""
    if not data:
        return None
    if data[:2] == b"PK":
        return data
    # Multipart / prefix noise: locate ZIP local-file header
    idx = data.find(b"PK\x03\x04")
    if idx >= 0 and idx < len(data) - 30:
        return data[idx:]
    return None


def _extract_docx_text(data: bytes) -> str:
    """Extract paragraph text from .docx (OOXML ZIP) without third-party libs."""
    zdata = _office_zip_bytes(data)
    if not zdata:
        return ""
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            if "word/document.xml" not in zf.namelist():
                return ""
            xml = zf.read("word/document.xml")
    except Exception:
        return ""
    try:
        root = ET.fromstring(xml)
    except Exception:
        return ""
    parts: list[str] = []
    for el in root.iter():
        if _xml_local(el.tag) != "p":
            continue
        bits: list[str] = []
        for node in el.iter():
            loc = _xml_local(node.tag)
            if loc == "t" and node.text:
                bits.append(node.text)
            elif loc == "tab":
                bits.append("\t")
            elif loc in ("br", "cr"):
                bits.append("\n")
        line = "".join(bits).strip()
        if line:
            parts.append(line)
    if not parts:
        for el in root.iter():
            if _xml_local(el.tag) == "t" and (el.text or "").strip():
                parts.append(el.text.strip())
    joined = "\n".join(parts).strip()
    return joined[:200_000]


def _extract_xlsx_text(data: bytes) -> str:
    """Extract cell values from .xlsx (shared strings + sheet cells)."""
    zdata = _office_zip_bytes(data)
    if not zdata:
        return ""
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            names = zf.namelist()
            if not any(n.startswith("xl/") for n in names):
                return ""
            shared: list[str] = []
            if "xl/sharedStrings.xml" in names:
                try:
                    ss_root = ET.fromstring(zf.read("xl/sharedStrings.xml"))
                    for si in ss_root:
                        if _xml_local(si.tag) != "si":
                            continue
                        texts = []
                        for node in si.iter():
                            if _xml_local(node.tag) == "t" and node.text:
                                texts.append(node.text)
                        shared.append("".join(texts))
                except Exception:
                    shared = []

            sheet_names = sorted(
                n for n in names if re.match(r"xl/worksheets/sheet\d+\.xml$", n)
            )[:20]
            values: list[str] = []
            for sheet in sheet_names:
                try:
                    root = ET.fromstring(zf.read(sheet))
                except Exception:
                    continue
                for el in root.iter():
                    if _xml_local(el.tag) != "c":
                        continue
                    cell_type = (el.attrib.get("t") or "").lower()
                    v_el = None
                    is_el = None
                    for child in el:
                        loc = _xml_local(child.tag)
                        if loc == "v":
                            v_el = child
                        elif loc == "is":
                            is_el = child
                    if cell_type == "s" and v_el is not None and (v_el.text or "").strip() != "":
                        try:
                            idx = int(v_el.text)
                            if 0 <= idx < len(shared) and shared[idx].strip():
                                values.append(shared[idx].strip())
                        except Exception:
                            pass
                    elif cell_type == "inlineStr" and is_el is not None:
                        texts = []
                        for node in is_el.iter():
                            if _xml_local(node.tag) == "t" and node.text:
                                texts.append(node.text)
                        s = "".join(texts).strip()
                        if s:
                            values.append(s)
                    elif v_el is not None and (v_el.text or "").strip():
                        # numbers / booleans / formulas cached value
                        values.append(v_el.text.strip())
            # Also dump shared strings if sheets yielded little (rare sheet layouts)
            if len(values) < 3 and shared:
                values.extend(s.strip() for s in shared if s and s.strip())
    except Exception:
        return ""
    # Dedupe while preserving order (repeated headers OK once)
    seen: set[str] = set()
    out: list[str] = []
    for v in values:
        if v in seen:
            continue
        seen.add(v)
        out.append(v)
        if len(out) >= 20_000:
            break
    return "\n".join(out)[:200_000]


def _extract_pptx_text(data: bytes) -> str:
    """Extract text from .pptx slide XMLs."""
    zdata = _office_zip_bytes(data)
    if not zdata:
        return ""
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            slides = sorted(n for n in zf.namelist() if re.match(r"ppt/slides/slide\d+\.xml$", n))[:30]
            if not slides:
                return ""
            parts: list[str] = []
            for name in slides:
                try:
                    root = ET.fromstring(zf.read(name))
                except Exception:
                    continue
                for el in root.iter():
                    if _xml_local(el.tag) == "t" and (el.text or "").strip():
                        parts.append(el.text.strip())
    except Exception:
        return ""
    return "\n".join(parts)[:200_000]


def _looks_like_docx(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if "wordprocessingml" in ct or fn.endswith(".docx"):
        return True
    zdata = _office_zip_bytes(data)
    if not zdata:
        return False
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            return "word/document.xml" in zf.namelist()
    except Exception:
        return False


def _looks_like_xlsx(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if "spreadsheetml" in ct or fn.endswith(".xlsx") or fn.endswith(".xlsm"):
        return True
    zdata = _office_zip_bytes(data)
    if not zdata:
        return False
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            names = zf.namelist()
            return any(n.startswith("xl/") for n in names)
    except Exception:
        return False


def _looks_like_pptx(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if "presentationml" in ct or fn.endswith(".pptx"):
        return True
    zdata = _office_zip_bytes(data)
    if not zdata:
        return False
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            return any(n.startswith("ppt/slides/") for n in zf.namelist())
    except Exception:
        return False


def _extract_office_text(data: bytes, content_type: str = "", file_name: str = "") -> str:
    """Best-effort OOXML text (Word / Excel / PowerPoint)."""
    if _looks_like_docx(data, content_type, file_name):
        t = _extract_docx_text(data)
        if t:
            return t
    if _looks_like_xlsx(data, content_type, file_name):
        t = _extract_xlsx_text(data)
        if t:
            return t
    if _looks_like_pptx(data, content_type, file_name):
        t = _extract_pptx_text(data)
        if t:
            return t
    # Unknown ZIP: try all
    if data[:2] == b"PK" or b"PK\x03\x04" in data[:8192]:
        for fn in (_extract_docx_text, _extract_xlsx_text, _extract_pptx_text):
            try:
                t = fn(data)
            except Exception:
                t = ""
            if t:
                return t
    return ""


def _looks_like_ole(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    """True for legacy OLE Compound File binary Office (.doc/.xls/.ppt)."""
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    # Careful: str.endswith(".doc") is also true for ".docx"
    if fn.endswith((".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm")):
        return False
    if fn.endswith((".doc", ".xls", ".ppt", ".msg")):
        return True
    if any(x in ct for x in (
        "msword", "ms-excel", "ms-powerpoint", "application/vnd.ms-excel",
        "application/vnd.ms-powerpoint", "application/vnd.ms-office",
    )) and "openxmlformats" not in ct:
        # Content-type alone can lie (some clients send msword for docx) —
        # prefer magic bytes when present.
        if data and data[:2] == b"PK":
            return False
        if data and data[:8] == b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1":
            return True
        if data and len(data) >= 8:
            return data[:8] == b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
        return True
    return bool(data) and data[:8] == b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"


def _extract_binary_string_runs(data: bytes, *, min_chars: int = 4) -> str:
    """Harvest printable ASCII + UTF-16LE runs from binary office blobs (DLP-oriented)."""
    if not data:
        return ""
    # Cap work on huge uploads
    blob = data[: 12 * 1024 * 1024]
    parts: list[str] = []
    seen: set[str] = set()

    def add(s: str) -> None:
        s = re.sub(r"\s+", " ", (s or "").strip())
        if len(s) < min_chars:
            return
        key = s[:120].lower()
        if key in seen:
            return
        seen.add(key)
        parts.append(s)

    # ASCII printable runs
    for m in re.finditer(rb"[\x20-\x7e]{%d,}" % min_chars, blob):
        try:
            add(m.group(0).decode("ascii", errors="ignore"))
        except Exception:
            continue
        if len(parts) >= 4000:
            break

    # UTF-16LE printable runs (common in .doc/.xls/.ppt)
    i = 0
    n = len(blob)
    while i + (min_chars * 2) <= n and len(parts) < 5000:
        if blob[i + 1] != 0 or not (0x20 <= blob[i] <= 0x7E):
            i += 1
            continue
        chars: list[str] = []
        j = i
        while j + 1 < n and blob[j + 1] == 0 and 0x20 <= blob[j] <= 0x7E:
            chars.append(chr(blob[j]))
            j += 2
            if len(chars) >= 800:
                break
        if len(chars) >= min_chars:
            add("".join(chars))
            i = j
        else:
            i += 1

    # Skip OLE/CFB structural noise tokens
    noise = (
        "root entry", "workbook", "worddocument", "powerpoint document",
        "summaryinformation", "documentsummaryinformation", "compobj",
        "1table", "0table", "data", "current user", "pictures",
    )
    cleaned = []
    for p in parts:
        low = p.lower()
        if low in noise or low.startswith(("_____", "objinfo", "workbook")):
            continue
        if re.fullmatch(r"[0-9A-Fa-f]{8,}", p):
            continue
        cleaned.append(p)
    return "\n".join(cleaned)[:200_000]


def _extract_ole_office_text(data: bytes, content_type: str = "", file_name: str = "") -> str:
    """Best-effort text from legacy .doc / .xls / .ppt (OLE CFB) without third-party libs."""
    if not data:
        return ""
    try:
        if not _looks_like_ole(data, content_type, file_name) and data[:8] != b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1":
            fn = (file_name or "").lower()
            if not fn.endswith((".doc", ".xls", ".ppt")):
                return ""
        return _extract_binary_string_runs(data, min_chars=4)
    except Exception as e:
        print(f"[Gateway Proxy] OLE office extract failed (allowed): {e}")
        return ""


def _looks_like_rtf(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if fn.endswith(".rtf") or "rtf" in ct or "richtext" in ct:
        return True
    head = (data or b"")[:64].lstrip()
    return head.startswith(b"{\\rtf")


def _extract_rtf_text(data: bytes) -> str:
    """Strip RTF control words and keep readable text for Guard Rules."""
    if not data:
        return ""
    try:
        raw = data[: 4 * 1024 * 1024].decode("latin-1", errors="ignore")
    except Exception:
        return ""
    if "\\rtf" not in raw[:200].lower() and not raw.lstrip().startswith("{\\rtf"):
        return ""
    try:
        # Hex escapes \'hh
        def _hex_repl(m: re.Match) -> str:
            try:
                return chr(int(m.group(1), 16))
            except Exception:
                return ""

        text = re.sub(r"\\'([0-9a-fA-F]{2})", _hex_repl, raw)
        # Unicode \uN?
        def _u_repl(m: re.Match) -> str:
            try:
                n = int(m.group(1))
                if n < 0:
                    n = 65536 + n
                return chr(n)
            except Exception:
                return ""

        text = re.sub(r"\\u(-?\d+)\??", _u_repl, text)
        # Drop destinations like {\*\...}
        text = re.sub(r"\{\\\*[^}]*\}", " ", text)
        # Control words / symbols
        text = re.sub(r"\\[a-zA-Z]+\d* ?", " ", text)
        text = re.sub(r"\\[^a-zA-Z\s]", " ", text)
        text = text.replace("{", " ").replace("}", " ")
        text = re.sub(r"\s+", " ", text).strip()
        return text[:200_000]
    except Exception as e:
        print(f"[Gateway Proxy] RTF extract failed (allowed): {e}")
        return ""


def _looks_like_opendocument(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if fn.endswith((".odt", ".ods", ".odp")) or "opendocument" in ct:
        return True
    zdata = _office_zip_bytes(data) if data else None
    if not zdata:
        return False
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            names = set(zf.namelist())
            return "content.xml" in names and (
                "mimetype" in names or any(n.startswith("META-INF/") for n in names)
            )
    except Exception:
        return False


def _extract_opendocument_text(data: bytes) -> str:
    """Extract text from ODF (.odt/.ods/.odp) content.xml."""
    zdata = _office_zip_bytes(data)
    if not zdata:
        return ""
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            if "content.xml" not in zf.namelist():
                return ""
            xml = zf.read("content.xml")
    except Exception:
        return ""
    try:
        root = ET.fromstring(xml)
    except Exception:
        return ""
    parts: list[str] = []
    for el in root.iter():
        loc = _xml_local(el.tag)
        if loc in ("p", "h", "span", "a", "text", "s"):
            if el.text and el.text.strip():
                parts.append(el.text.strip())
            if el.tail and el.tail.strip():
                parts.append(el.tail.strip())
        elif el.text and el.text.strip() and loc not in ("script", "style", "binary-data"):
            # Spreadsheet cell values etc.
            if len(el.text.strip()) >= 2:
                parts.append(el.text.strip())
    # Dedupe adjacent
    out: list[str] = []
    prev = ""
    for p in parts:
        if p == prev:
            continue
        out.append(p)
        prev = p
    return "\n".join(out)[:200_000]


def _looks_like_html(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if fn.endswith((".html", ".htm", ".xhtml")) or "text/html" in ct or "xhtml" in ct:
        return True
    head = (data or b"")[:256].lstrip().lower()
    return head.startswith(b"<!doctype html") or head.startswith(b"<html") or b"<html" in head[:64]


def _extract_html_text(data: bytes) -> str:
    """Strip tags from HTML uploads so regex rules can scan page text."""
    if not data:
        return ""
    try:
        raw = data[: 4 * 1024 * 1024].decode("utf-8", errors="ignore")
        if not raw.strip():
            raw = data[: 4 * 1024 * 1024].decode("latin-1", errors="ignore")
    except Exception:
        return ""
    try:
        text = re.sub(r"(?is)<(script|style|noscript)[^>]*>.*?</\1>", " ", raw)
        text = re.sub(r"(?is)<!--.*?-->", " ", text)
        text = re.sub(r"(?s)<[^>]+>", " ", text)
        text = (
            text.replace("&nbsp;", " ")
            .replace("&amp;", "&")
            .replace("&lt;", "<")
            .replace("&gt;", ">")
            .replace("&quot;", '"')
            .replace("&#39;", "'")
        )
        text = re.sub(r"\s+", " ", text).strip()
        return text[:200_000]
    except Exception:
        return ""


def _looks_like_image(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if ct.startswith("image/"):
        return True
    if any(fn.endswith(ext) for ext in (".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff")):
        return True
    if not data:
        return False
    if data.startswith(b"\x89PNG\r\n\x1a\n") or data.startswith(b"\xff\xd8\xff") or data.startswith(b"GIF8"):
        return True
    if data.startswith(b"BM") and len(data) > 30:
        return True
    if len(data) >= 12 and data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return True
    return False


def _run_async(coro):
    """Run a coroutine even if mitmproxy already has an event loop."""
    try:
        asyncio.get_running_loop()
    except RuntimeError:
        return asyncio.run(coro)

    result: dict = {}
    exc: dict = {}

    def runner() -> None:
        try:
            result["v"] = asyncio.run(coro)
        except Exception as e:
            exc["e"] = e

    t = threading.Thread(target=runner, daemon=True)
    t.start()
    t.join(timeout=35)
    if t.is_alive():
        raise TimeoutError("ocr timed out")
    if "e" in exc:
        raise exc["e"]
    return result.get("v")
