# Part of Gateway browser_ai_proxy — do not import directly.
import hashlib



_FAKE_UPLOAD_NAMES = frozenset({
    "", "attachment", "attachment.txt", "attachment.bin", "blob", "blob.txt",
    "document", "document.txt", "document.bin", "image",
    "null", "undefined", "unknown", "screenshot", "voice",
})

# Generic media stems are no longer blacklisted — if a user uploads screenshot.png, photo.jpg,
# or presentation.pptx, that is their real filename and must be preserved as-is.
_GENERIC_VENDOR_MEDIA_STEMS = frozenset()

# Only match anonymous numbered attachment/document/image placeholders synthesized by the proxy itself
_GENERATED_UPLOAD_NAME_RE = re.compile(
    r"^(?:attachment|document|image)(?:[-\s]\d+)?$",
    re.I,
)

_UPLOAD_NAME_EXTS = (
    ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".xlsm", ".pptx", ".ppt",
    ".odt", ".ods", ".odp", ".rtf", ".html", ".htm", ".xml",
    ".txt", ".csv", ".json", ".md", ".log",
    ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff",
    ".zip", ".tar", ".gz", ".7z", ".rar",
    ".wav", ".mp3", ".m4a", ".webm", ".ogg", ".flac", ".aac", ".opus", ".wma",
    ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".go", ".rs", ".rb", ".php",
    ".c", ".cpp", ".h", ".cs", ".swift", ".kt", ".sql", ".sh", ".ps1",
    ".css", ".scss", ".vue", ".dart", ".lua", ".toml", ".env", ".ipynb",
    ".yaml", ".yml", ".ini", ".cfg", ".conf", ".svg", ".ico", ".heic", ".avif",
    ".pages", ".numbers", ".key", ".epub", ".mobi", ".apk", ".dmg", ".iso",
)

# ChatGPT wire sometimes leaks site host / chat tab titles as "filenames".
_AI_SITE_LABEL_RE = re.compile(
    r"^(?:www\.)?(?:chatgpt|chat\.openai|openai|claude|anthropic|gemini|bard|"
    r"copilot|perplexity|grok|x\.ai|deepseek|poe|you\.com)(?:\.(?:com|ai|google\.com))?$",
    re.I,
)
_HOSTNAME_LABEL_RE = re.compile(
    r"^(?:[a-z0-9-]+\.)+(?:com|ai|org|net|io|dev|app|co|info|me|gg)(?:\.[a-z]{2})?$",
    re.I,
)

# ChatGPT page/CDN JSON leaks script names (analytics.js) onto cached PDFs.
_WIRE_JUNK_NAME_EXTS = frozenset({
    ".map", ".woff", ".woff2", ".ttf", ".eot", ".ico",
})
_WIRE_JUNK_STEM_RE = re.compile(
    r"(?:^|[-_.])("
    r"analytics|gtag|gtm|googletag|pixel|tracking|beacon|rum|"
    r"webpack|chunk|polyfill|hot-update|vendorbundle|sentry|"
    r"datadog|newrelic|segment|mixpanel|hotjar|clarity|intercom"
    r")(?:[-_.]|$)",
    re.I,
)
_WIRE_JUNK_EXACT = frozenset({
    "analytics.js", "gtag.js", "gtm.js", "ga.js", "fbevents.js",
    "tag.js", "sdk.js", "beacon.js", "rum.js", "vendor.js",
    "chunk.js", "runtime.js", "polyfill.js", "hot-update.js",
})


def _is_generic_vendor_media_label(name: str) -> bool:
    """True only for completely empty or nameless placeholders."""
    n = (name or "").strip().lower()
    return not n or n in _FAKE_UPLOAD_NAMES


def _is_fake_upload_name(name: str) -> bool:
    n = (name or "").strip().lower()
    if not n or n in _FAKE_UPLOAD_NAMES:
        return True
    if _GENERATED_UPLOAD_NAME_RE.fullmatch(n):
        return True
    if _is_generic_vendor_media_label(n):
        return True
    return False


def _is_wire_junk_filename(name: str) -> bool:
    """True for ChatGPT/Gemini page-asset names that must never become Prompt Log labels."""
    n = (name or "").strip().replace("\\", "/").rsplit("/", 1)[-1].lower()
    if not n:
        return True
    if n in _WIRE_JUNK_EXACT or n in _FAKE_UPLOAD_NAMES:
        return True
    stem, ext = (n.rsplit(".", 1) + [""])[:2] if "." in n else (n, "")
    ext = f".{ext}" if ext else ""
    if ext in _WIRE_JUNK_NAME_EXTS:
        return True
    if ext in {".js", ".mjs", ".css", ".html", ".htm"} and _WIRE_JUNK_STEM_RE.search(stem):
        return True
    return False


def _name_fits_upload_bytes(name: str, raw: bytes = b"", content_type: str = "") -> bool:
    """False when a leaked script name is stamped onto a PDF/image/office body."""
    n = (name or "").strip().lower().rsplit("/", 1)[-1]
    if not n or _is_wire_junk_filename(n):
        return False
    data = raw or b""
    ct = (content_type or "").lower()
    ext = ""
    if "." in n:
        ext = "." + n.rsplit(".", 1)[-1]
    is_pdf = data[:5] == b"%PDF-" or "pdf" in ct
    is_img = (
        (len(data) >= 3 and data[0] == 0xFF and data[1] == 0xD8 and data[2] == 0xFF)
        or data.startswith(b"\x89PNG\r\n\x1a\n")
        or data.startswith(b"GIF87a") or data.startswith(b"GIF89a")
        or "image/" in ct
    )
    is_office = data[:2] == b"PK"
    if ext in {".js", ".mjs", ".css", ".map", ".html", ".htm", ".woff", ".woff2"}:
        if is_pdf or is_img or is_office:
            return False
    if is_pdf and ext in {".png", ".jpg", ".jpeg", ".gif", ".webp", ".mp3", ".wav"}:
        return False
    if is_office and ext in {".pdf", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".mp3", ".wav", ".txt"}:
        return False
    if is_img and ext in {".pdf", ".docx", ".xlsx", ".pptx", ".zip"}:
        return False
    return True


def _upload_name_quality(name: str, raw: bytes = b"", content_type: str = "") -> int:
    """Higher is a better user-picked name. 0 = fake/empty (do not bind)."""
    n = (name or "").strip()
    if not n or not _is_real_user_upload_name(n):
        return 0
    if raw and not _name_fits_upload_bytes(n, raw, content_type):
        return 0
    score = 2 if _has_any_file_extension(n) else 1
    # Document-style names (04-Feature-Buttons.pdf) beat generic leftovers.
    if re.search(r"[\d()]", n):
        score += 2
    if n.lower().endswith((".pdf", ".docx", ".xlsx", ".pptx", ".png", ".jpg", ".jpeg", ".zip")):
        score += 1
    return score


def _has_any_file_extension(name: str) -> bool:
    """True for ANY real file extension (pdf/docx/json/png/zip/code/… — not only a few)."""
    base = (name or "").strip().rsplit("/", 1)[-1].rsplit("\\", 1)[-1]
    if not base:
        return False
    # Single-dot dotfiles (.env, .gitignore)
    if base.startswith(".") and base.count(".") == 1:
        return base.lower() in {".env", ".gitignore", ".dockerignore", ".npmrc", ".editorconfig"}
    if "." not in base:
        return False
    ext = "." + base.rsplit(".", 1)[-1].lower()
    # "5.3" is a version, not a file extension. Real extensions start with a letter.
    if not re.fullmatch(r"\.[a-z][a-z0-9]{0,7}", ext):
        return False
    if ext in _UPLOAD_NAME_EXTS:
        return True
    return True


def _is_snake_case_wire_filename(name: str) -> bool:
    """True for ChatGPT JSON keys leaked as names: composer_rendered, asset_pointer.

    A real upload has an extension (UnifAI_Competitive_Position_and_Gaps.docx) or a
    human label (Gateway Product (1)). All-lowercase snake_case with no extension
    is a wire field, not the file the user picked.
    """
    n = (name or "").strip()
    if not n or _has_any_file_extension(n):
        return False
    return bool(re.fullmatch(r"[a-z][a-z0-9]*(_[a-z0-9]+)+", n))


def _looks_like_site_or_tab_label_not_file(name: str) -> bool:
    """Reject hostnames / AI site labels that ChatGPT sometimes leaks as file names."""
    n = (name or "").strip()
    if not n:
        return True
    compact = n.replace(" ", "")
    if _AI_SITE_LABEL_RE.fullmatch(compact) or _HOSTNAME_LABEL_RE.fullmatch(compact):
        return True
    # Extensionless chat-tab / product titles — not user file picks.
    # Keep Gemini-style extensionless names that include digits/parens (e.g. 'Gateway Product (1)').
    if not _has_any_file_extension(n):
        if _AI_SITE_LABEL_RE.fullmatch(n.strip()):
            return True
        words = [w for w in re.split(r"\s+", n.strip()) if w]
        # Single brand/chat token ("Gateway", "Greeting") with no digits/parens.
        if (
            len(words) == 1
            and not re.search(r"[\d()_\-./\\]", n)
            and re.fullmatch(r"[A-Za-z][A-Za-z0-9]{0,48}", words[0] or "")
        ):
            return True
        if (
            2 <= len(words) <= 6
            and not re.search(r"[\d()_\-./\\]", n)
            and all(re.fullmatch(r"[A-Za-z]+", w) for w in words)
        ):
            return True
    return False


def _json_name_field_ok(name: str, field: str = "") -> bool:
    """True when a JSON name/title value is safe as a user filename.

    ChatGPT often puts chat titles / product labels in bare name/title fields.
    Prefer file_name/filename; only accept name/title when file-like.
    """
    n = (name or "").strip()
    if not n or not _is_real_user_upload_name(n):
        return False
    field_l = (field or "").lower()
    if field_l in ("name", "title", "label", "model"):
        # Chat title / model picker. The original upload name is file_name or filename=.
        return _has_any_file_extension(n)
    return True


def _prefer_real_filenames(names: list[str]) -> list[str]:
    """Keep every real file name; prefer ANY extensioned name first, then extensionless."""
    real = [n for n in names if _is_real_user_upload_name(n)]
    with_ext = [n for n in real if _has_any_file_extension(n)]
    without = [n for n in real if not _has_any_file_extension(n)]
    # Stable de-dupe
    out: list[str] = []
    seen: set[str] = set()
    for n in with_ext + without:
        key = n.lower()
        if key in seen:
            continue
        seen.add(key)
        out.append(n)
    return out


def _is_real_user_upload_name(name: str) -> bool:
    """True when filename looks like a user-picked name (not Guard/site placeholder).

    Applies to ALL file types (office, images, audio, zip, code, config, …).
    Gemini/Google often send display names WITHOUT an extension (e.g. 'Gateway Product (1)').
    Those are still real user names — do not treat as fake — unless they look like
    a site host or chat-tab title leaked from ChatGPT wire.
    """
    n = (name or "").strip()
    if not n or _is_fake_upload_name(n):
        return False
    if _is_wire_junk_filename(n):
        return False
    if _looks_like_site_or_tab_label_not_file(n):
        return False
    # ChatGPT wire keys (composer_rendered) are not user-picked file names.
    if _is_snake_case_wire_filename(n):
        return False
    # Reject opaque ids mistaken for names
    if re.fullmatch(r"(?:file-)?[A-Za-z0-9_-]{20,}", n):
        return False
    if re.fullmatch(r"[0-9a-f]{8,}(?:-[0-9a-f]{4,})+", n, re.I):
        return False
    return True


def _ensure_name_has_extension(name: str, raw: bytes = b"", content_type: str = "") -> str:
    """If a real display name has no extension, append one from magic/content-type."""
    n = (name or "").strip()
    if not n:
        return n
    if "." in n.rsplit("/", 1)[-1]:
        return n
    kind = ""
    try:
        kind = _classify_upload_kind(raw or b"", content_type, n)
    except Exception:
        kind = ""
    ext = {
        "pdf": ".pdf",
        "zip": ".zip",
        "image": ".png",
        "audio": ".m4a",
        "video": ".mp4",
        "docx": ".docx",
        "xlsx": ".xlsx",
        "pptx": ".pptx",
        "plain": ".txt",
    }.get(kind, "")
    if not ext and (raw or b"")[:5] == b"%PDF-":
        ext = ".pdf"
    return (n + ext) if ext else n


def _name_from_pdf_metadata(raw: bytes) -> str:
    """Last-resort label from PDF Title / metadata when the wire omits filename.

    PDF /Title is a *document* title (often a product brand like "Gateway"), NOT the
    user's picked filesystem name (01-User-Manual.pdf). Only accept metadata that
    already looks like a real filename with an extension.
    """
    if not raw or raw[:5] != b"%PDF-":
        return ""

    def _accept(raw_title: str) -> str:
        got = _sanitize_upload_filename((raw_title or "").strip())
        if not got or not _has_any_file_extension(got):
            return ""
        if not _is_real_user_upload_name(got):
            return ""
        return got

    try:
        from pypdf import PdfReader
        reader = PdfReader(io.BytesIO(raw), strict=False)
        meta = getattr(reader, "metadata", None) or {}
        for key in ("/Title", "Title", "/Subject"):
            try:
                val = meta.get(key) if hasattr(meta, "get") else None
            except Exception:
                val = None
            if not val:
                try:
                    val = getattr(meta, key.lstrip("/").lower(), None)
                except Exception:
                    val = None
            accepted = _accept(str(val or "").strip())
            if accepted:
                return accepted
    except Exception:
        pass
    # Lightweight /Title (....) scan without full parse
    try:
        sample = raw[: min(len(raw), 256 * 1024)]
        m = re.search(rb"/Title\s*\(([^\)]{3,120})\)", sample)
        if m:
            accepted = _accept(m.group(1).decode("latin-1", errors="ignore"))
            if accepted:
                return accepted
    except Exception:
        pass
    return ""


def _sanitize_upload_filename(name: str) -> str:
    name = urllib.parse.unquote((name or "").strip().strip("\"'"))
    name = name.replace("\\", "/").rsplit("/", 1)[-1].strip()
    if not name or _is_fake_upload_name(name):
        return ""
    if _is_wire_junk_filename(name):
        return ""
    # Reject path traversal / absurd lengths
    if ".." in name or len(name) > 180:
        return ""
    return name


def _is_default_generated_upload_name(name: str) -> bool:
    """True for Guard-invented placeholders (document.pdf / attachment / image.png)."""
    n = (name or "").strip()
    if not n:
        return True
    if _is_fake_upload_name(n):
        return True
    if _GENERATED_UPLOAD_NAME_RE.fullmatch(n.lower()):
        return True
    return False


def _needs_real_upload_filename(name: str) -> bool:
    """True when Prompt Logs would show a useless default instead of the user file name."""
    n = (name or "").strip()
    if not n:
        return True
    if _is_default_generated_upload_name(n):
        return True
    return not _is_real_user_upload_name(n)


def _worth_binding_upload_bytes(stored: bytes) -> bool:
    """Only consume pending real names when we have real file bytes (not tiny probes)."""
    data = stored or b""
    if len(data) < 64:
        return False
    if data[:5] == b"%PDF-" or data[:2] == b"PK":
        return True
    if len(data) >= 3 and data[0] == 0xFF and data[1] == 0xD8 and data[2] == 0xFF:
        return True
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return True
    if data.startswith(b"GIF87a") or data.startswith(b"GIF89a"):
        return True
    return len(data) >= 256


def _filename_from_url(url_or_path: str) -> str:
    """ChatGPT/Gemini signed URLs sometimes carry ?filename= / path basename."""
    raw = (url_or_path or "").strip()
    if not raw:
        return ""
    try:
        parsed = urllib.parse.urlparse(raw if "://" in raw else ("https://x.invalid" + (raw if raw.startswith("/") else "/" + raw)))
        qs = urllib.parse.parse_qs(parsed.query or "")
        for key in (
            "filename", "file_name", "fileName", "original_filename", "originalFilename",
            "display_name", "displayName", "name", "file",
        ):
            for val in (qs.get(key) or []):
                got = _sanitize_upload_filename(val)
                if got and _is_real_user_upload_name(got):
                    return got
        seg = urllib.parse.unquote((parsed.path or "").rsplit("/", 1)[-1])
        if seg and "." in seg and not seg.startswith("."):
            got = _sanitize_upload_filename(seg)
            if got and _is_real_user_upload_name(got) and _has_any_file_extension(got):
                return got
    except Exception:
        return ""
    return ""


def _filename_from_multipart_or_headers(raw: bytes = b"", headers=None, raw_text: str = "") -> str:
    """Pull a real user filename from multipart bytes, headers, or JSON text."""
    if headers is not None:
        try:
            cd = headers.get("content-disposition", "") or ""
        except Exception:
            cd = ""
        if "filename" in cd.lower():
            m = re.search(r"filename\*=(?:UTF-8''|utf-8'')([^;\r\n]+)|filename\*?=(?:UTF-8''|utf-8'')?\"?([^\";\r\n]+)\"?", cd, re.I)
            if m:
                got = _sanitize_upload_filename(m.group(1) or m.group(2) or "")
                if got:
                    return got
        for hk in (
            "x-file-name", "x-goog-upload-file-name", "x-goog-upload-header-content-disposition",
            "x-filename", "x-upload-filename",
            "x-ms-file-name", "openai-file-name", "file-name", "x-amz-meta-filename",
            "x-amz-meta-file-name", "x-amz-meta-name",
            # Extra vendor headers seen on ChatGPT/Gemini CDNs
            "x-amz-meta-original-filename", "x-original-file-name", "x-original-filename",
        ):
            try:
                val = headers.get(hk)
            except Exception:
                val = None
            if val:
                # Content-Disposition: attachment; filename="foo.pdf"
                raw_val = str(val)
                if "filename" in raw_val.lower():
                    m = re.search(
                        r'filename\*=(?:UTF-8\'\'|utf-8\'\')([^;\r\n]+)|filename\*?=(?:UTF-8\'\')?\"?([^\";\r\n]+)\"?',
                        raw_val,
                        re.I,
                    )
                    if m:
                        got = _sanitize_upload_filename(m.group(1) or m.group(2) or "")
                        if got:
                            return got
                got = _sanitize_upload_filename(raw_val)
                if got:
                    return got

    blob = raw or b""
    if blob:
        # Only trust filename= inside a multipart part header (Content-Disposition ...
        # blank line). Scanning the whole body picked up code/text INSIDE the uploaded
        # file (e.g. `b"filename=" in low_head or b"...` -> "in low_head or b.pdf").
        sample = blob[: min(len(blob), 96 * 1024)]
        # A multipart body always opens with a "--boundary" line; anything else is file
        # content (text/code/pdf) and must never be scanned for a filename.
        is_multipart_body = sample.lstrip()[:2] == b"--"
        for hdr_m in (
            re.finditer(rb"content-disposition:[^\r\n]*(?:\r?\n[ \t][^\r\n]*)*", sample[:16384], re.I)
            if is_multipart_body else ()
        ):
            hdr = hdr_m.group(0)
            for pat in (
                rb'filename\*=(?:UTF-8\'\'|utf-8\'\')([^;\r\n]+)',
                rb'filename="([^"]+)"',
                rb"filename='([^']+)'",
                rb'filename=([^;\r\n\s]+)',
            ):
                m = re.search(pat, hdr, re.I)
                if m:
                    try:
                        raw_name = m.group(1).decode("utf-8", errors="ignore")
                    except Exception:
                        raw_name = m.group(1).decode("latin-1", errors="ignore")
                    got = _sanitize_upload_filename(raw_name)
                    if got:
                        return got

    text = raw_text or ""
    if text:
        for pat in (
            r'["\'](?:file_name|fileName|filename|original_name|originalName|original_filename|originalFilename|display_name|displayName|documentName|document_name)["\']\s*:\s*["\']([^"\']+)["\']',
            # filename=... only on a Content-Disposition line (not arbitrary text/code)
            r'content-disposition:[^\r\n]*?filename\s*=\s*["\']([^"\';\r\n]+)["\']',
        ):
            m = re.search(pat, text[:20000], re.I)
            if m:
                got = _sanitize_upload_filename(m.group(1))
                if got:
                    return got
    return ""


def _default_name_from_bytes(raw: bytes, content_type: str = "", idx: int = 0, total_count: int = 1) -> str:
    """Fallback label only when the wire completely omits any user filename.
    Never invents fake filenames like document.pdf or spreadsheet.xlsx.
    Defaults cleanly to Document, Image, or Voice Note.
    """
    kind = ""
    try:
        kind = _classify_upload_kind(raw or b"", content_type, "")
    except Exception:
        kind = ""
    suffix = f" {idx + 1}" if (idx > 0 or total_count > 1) else ""
    if kind == "audio" or (raw and raw[:4] == b"RIFF"):
        return f"Voice Note{suffix}"
    if kind == "image" or (raw and (raw[:3] == b"\xff\xd8\xff" or raw[:8] == b"\x89PNG\r\n\x1a\n" or raw[:4] == b"GIF8")):
        return f"image{suffix}.png" if suffix else "image.png"
    return f"document{suffix}.pdf" if suffix else "document.pdf"


def _list_zip_member_basenames(data: bytes, max_names: int = 24) -> list[str]:
    if not data:
        return []
    zdata = data if data[:2] == b"PK" else (_office_zip_bytes(data) or b"")
    if not zdata or zdata[:2] != b"PK":
        return []
    out: list[str] = []
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            for info in zf.infolist():
                if len(out) >= max_names:
                    break
                if info.is_dir():
                    continue
                name = (info.filename or "").replace("\\", "/")
                base = name.rsplit("/", 1)[-1]
                low = name.lower()
                if not base or base.startswith("."):
                    continue
                if "__macosx" in low or low.endswith(".ds_store"):
                    continue
                out.append(base)
    except Exception:
        return []
    return out


def _extract_names_from_one_text(raw_text: str) -> list[str]:
    """All real filenames referenced on a chat Send (multi-file).

    Returns names for EVERY file type (not only pdf/pptx/json). Filters out
    ChatGPT site hosts / chat-tab titles that leak into title/name fields.
    """
    if not raw_text:
        return []
    found: list[str] = []
    seen: set[str] = set()

    def _add(got: str, *, field: str = "") -> None:
        name = _sanitize_upload_filename(got)
        if not name:
            return
        field_l = (field or "").lower()
        # Bare UI "title"/"name" without a file-like shape is almost never a real upload.
        if field_l in ("title", "name") and not _json_name_field_ok(name, field_l):
            return
        if not _is_real_user_upload_name(name):
            return
        key = name.lower()
        if key in seen:
            return
        seen.add(key)
        found.append(name)

    patterns = (
        (r'["\'](file_name|fileName|filename|original_name|originalName|original_filename|originalFilename|display_name|displayName)["\']\s*:\s*["\']([^"\']+)["\']', True),
        (r'["\'](name)["\']\s*:\s*["\']([^"\']+\.[A-Za-z][A-Za-z0-9]{0,7})["\']', True),
        (r'["\'](title)["\']\s*:\s*["\']([^"\']+\.[A-Za-z][A-Za-z0-9]{0,7})["\']', True),
    )
    for pat, _ in patterns:
        for m in re.finditer(pat, raw_text, re.I):
            _add(m.group(2), field=m.group(1))
    # Attachment objects: pull name fields inside attachments/files/parts arrays
    for arr_pat in (
        r'"(?:attachments|files|parts|documents|content|fileattachments|imageattachments|file_list|filelist|uploadedfiles|uploaded_files|assets|sources|media|docs|messageinput)"\s*:\s*\[([\s\S]{0,80000}?)\]',
    ):
        for am in re.finditer(arr_pat, raw_text, re.I):
            block = am.group(1) or ""
            for m in re.finditer(
                r'["\'](file_name|fileName|filename|original_name|originalName|original_filename|originalFilename|display_name|displayName|document_name|documentName|doc_name|docName|name|title|file|path|label)["\']\s*:\s*["\']([^"\']+)["\']',
                block,
                re.I,
            ):
                _add(m.group(2), field=m.group(1))
    return _prefer_real_filenames(found)


_POSITIONAL_NAME_RE = re.compile(
    r"^[^\\/\r\n\"]{1,150}\.(?:pdf|docx?|xlsx?|xlsm|pptx?|csv|tsv|txt|md|rtf|json|zip|7z|rar|"
    r"png|jpe?g|gif|webp|bmp|tiff?|heic|svg|mp3|wav|m4a|aac|ogg|flac|webm|mp4|mov|py|ipynb)$",
    re.I,
)
_NESTED_NAME_KEYS = frozenset({
    "file_name", "filename", "original_name", "originalname", "original_filename",
    "originalfilename", "display_name", "displayname", "name", "title",
})


def _nested_json_send_names(raw_text: str) -> list[str]:
    """File names hidden inside JSON-in-JSON / form-encoded wires (Gemini batchexecute).

    STRUCTURAL parse only: a string is descended into only if it is itself valid JSON.
    Free text (the user's typed/pasted prompt, even one that mentions "filename": "x.pdf")
    never parses as JSON, so it can never produce a phantom name.
    """
    body = (raw_text or "").strip()
    if not body or len(body) > 2_000_000:
        return []
    roots: list = []
    if body[0] in "[{":
        try:
            roots.append(json.loads(body))
        except Exception:
            pass
    elif "=" in body[:64]:
        try:
            for _k, v in urllib.parse.parse_qsl(body, keep_blank_values=False):
                vv = (v or "").strip()
                if vv[:1] in "[{":
                    try:
                        roots.append(json.loads(vv))
                    except Exception:
                        continue
        except Exception:
            return []
    out: list[str] = []
    seen: set[str] = set()

    def _add(name: str, field: str = "") -> None:
        got = _sanitize_upload_filename(name or "")
        if not got or not _is_real_user_upload_name(got):
            return
        if field and not _json_name_field_ok(got, field):
            return
        if got.lower() not in seen:
            seen.add(got.lower())
            out.append(got)

    def _walk(obj, nested: bool, depth: int) -> None:
        if depth > 10:
            return
        if isinstance(obj, dict):
            for k, v in obj.items():
                if nested and isinstance(v, str) and str(k).lower() in _NESTED_NAME_KEYS:
                    _add(v, str(k))
                _walk(v, nested, depth + 1)
        elif isinstance(obj, list):
            for v in obj[:400]:
                if nested and isinstance(v, str) and _POSITIONAL_NAME_RE.match(v.strip()):
                    _add(v.strip())
                _walk(v, nested, depth + 1)
        elif isinstance(obj, str):
            t = obj.strip()
            if len(t) >= 2 and t[0] in "[{" and t[-1] in "]}":
                try:
                    inner = json.loads(t)
                except Exception:
                    return
                _walk(inner, True, depth + 1)

    for root in roots:
        _walk(root, False, 0)
    return out


def extract_all_attachment_filenames_from_send(raw_text: str) -> list[str]:
    """All real filenames referenced on a chat Send — any wire shape, any AI site."""
    if not raw_text:
        return []
    names = _extract_names_from_one_text(raw_text)
    if names:
        return names
    # Nothing on the plain wire (ChatGPT-style keys): try nested / form-encoded wires.
    try:
        nested = _prefer_real_filenames(_nested_json_send_names(raw_text))
        if nested:
            return nested
    except Exception:
        pass
    # Protobuf / Connect-RPC payloads (Claude Web PerformAction, etc.)
    try:
        raw_b = raw_text.encode("latin-1", errors="ignore") if isinstance(raw_text, str) else raw_text
        if len(raw_b) >= 8:
            pb_strs = extract_protobuf_strings(raw_b)
            if not pb_strs and isinstance(raw_text, str):
                pb_strs = extract_protobuf_strings(raw_text.encode("utf-8", errors="surrogateescape"))
            if pb_strs:
                pb_names = []
                for s in pb_strs:
                    s_clean = (s or "").strip()
                    if _POSITIONAL_NAME_RE.match(s_clean) and _is_real_user_upload_name(s_clean):
                        if s_clean.lower() not in [n.lower() for n in pb_names]:
                            pb_names.append(s_clean)
                if pb_names:
                    return _prefer_real_filenames(pb_names)
    except Exception:
        pass
    # Regex fallback for positional filenames in raw string (e.g. from decoded RPC / binary bodies)
    try:
        raw_str = raw_text if isinstance(raw_text, str) else raw_text.decode("utf-8", errors="ignore")
        if any(ext in raw_str.lower() for ext in (".pdf", ".docx", ".xlsx", ".pptx", ".csv", ".txt", ".png", ".jpg")):
            direct_matches = []
            for m in re.finditer(r'[\w\.-]+\.(?:pdf|docx?|xlsx?|xlsm|pptx?|csv|tsv|txt|md|rtf|json|zip|png|jpe?g|gif|webp|mp3|wav|m4a|py)', raw_str, re.I):
                cand = m.group(0).strip()
                if _is_real_user_upload_name(cand) and cand.lower() not in [x.lower() for x in direct_matches]:
                    direct_matches.append(cand)
            if direct_matches:
                return _prefer_real_filenames(direct_matches)
    except Exception:
        pass
    return []


# file_id → real filename (ChatGPT often nameless at upload).
_FILE_ID_NAME_REGISTRY: dict[str, str] = {}
_FILE_ID_NAME_REGISTRY_LOCK = threading.Lock()
_FILE_ID_NAME_REGISTRY_MAX = 400


def remember_upload_filename(file_id: str, name: str) -> None:
    """Remember a real filename for a file_id so later nameless CDN uploads can bind."""
    fid = (file_id or "").strip()
    got = _sanitize_upload_filename(name or "")
    if not fid or not got or not _is_real_user_upload_name(got):
        return
    keys = {fid}
    if fid.startswith("file-"):
        keys.add(fid[5:])
    else:
        keys.add("file-" + fid)
    with _FILE_ID_NAME_REGISTRY_LOCK:
        for k in keys:
            _FILE_ID_NAME_REGISTRY[k] = got
        while len(_FILE_ID_NAME_REGISTRY) > _FILE_ID_NAME_REGISTRY_MAX:
            _FILE_ID_NAME_REGISTRY.pop(next(iter(_FILE_ID_NAME_REGISTRY)), None)


def lookup_upload_filename(file_id: str) -> str:
    fid = (file_id or "").strip()
    if not fid:
        return ""
    with _FILE_ID_NAME_REGISTRY_LOCK:
        got = _FILE_ID_NAME_REGISTRY.get(fid) or ""
        if not got and fid.startswith("file-"):
            got = _FILE_ID_NAME_REGISTRY.get(fid[5:]) or ""
        if not got and not fid.startswith("file-"):
            got = _FILE_ID_NAME_REGISTRY.get("file-" + fid) or ""
    return got if _is_real_user_upload_name(got) else ""


# Per-domain pending real names (ChatGPT create-file → nameless CDN PUT).
_DOMAIN_PENDING_NAMES: dict[str, list[str]] = {}
_DOMAIN_PENDING_NAMES_LOCK = threading.Lock()
_DOMAIN_PENDING_NAMES_MAX = 40
_DOMAIN_PENDING_TTL = 120.0  # seconds — a name not consumed by then is stale
_DOMAIN_PENDING_TS: dict[tuple[str, str], float] = {}


def _purge_pending_names_locked(now: float | None = None) -> None:
    """Drop expired pending names. Caller must hold _DOMAIN_PENDING_NAMES_LOCK."""
    now = now if now is not None else time.time()
    for d in list(_DOMAIN_PENDING_NAMES.keys()):
        keep = [
            n for n in (_DOMAIN_PENDING_NAMES.get(d) or [])
            if now - _DOMAIN_PENDING_TS.get((d, (n or "").lower()), now) <= _DOMAIN_PENDING_TTL
        ]
        if keep:
            _DOMAIN_PENDING_NAMES[d] = keep
        else:
            _DOMAIN_PENDING_NAMES.pop(d, None)
    for k in list(_DOMAIN_PENDING_TS.keys()):
        if now - _DOMAIN_PENDING_TS[k] > _DOMAIN_PENDING_TTL:
            _DOMAIN_PENDING_TS.pop(k, None)


def _upload_name_already_cached(domain: str, name: str, *, except_uid: str = "") -> bool:
    """True when this real filename is already on a different cached upload."""
    want = (name or "").strip().lower()
    if not want or not domain:
        return False
    skip = (except_uid or "").strip()
    aliases = []
    try:
        aliases = upload_domain_aliases(domain) or [(domain or "").strip().lower()]
    except Exception:
        aliases = [(domain or "").strip().lower()]
    aliases = [a for a in aliases if a]
    with _UPLOAD_FILE_CACHE_LOCK:
        seen: set[str] = set()
        for alias in aliases:
            for entry in list(_UPLOAD_FILE_QUEUES.get(_upload_queue_key(alias), []) or []):
                uid = str(entry.get("cache_uid") or id(entry))
                if uid in seen or (skip and uid == skip):
                    continue
                seen.add(uid)
                cur = (entry.get("file_name") or "").strip().lower()
                if cur == want and _is_real_user_upload_name(entry.get("file_name") or ""):
                    return True
            latest = _UPLOAD_FILE_CACHE.get(f"{alias}|latest")
            if latest:
                uid = str(latest.get("cache_uid") or id(latest))
                if uid not in seen and not (skip and uid == skip):
                    cur = (latest.get("file_name") or "").strip().lower()
                    if cur == want and _is_real_user_upload_name(latest.get("file_name") or ""):
                        return True
    return False


def remember_pending_upload_name_for_domain(domain: str, name: str) -> None:
    """Queue a real filename for this Target so the next nameless CDN cache can bind it."""
    d = (domain or "").strip().lower()
    got = _sanitize_upload_filename(name or "")
    if not d or not got or not _is_real_user_upload_name(got):
        return
    with _DOMAIN_PENDING_NAMES_LOCK:
        _purge_pending_names_locked()
        q = _DOMAIN_PENDING_NAMES.setdefault(d, [])
        if any((x or "").strip().lower() == got.lower() for x in q):
            return
        q.append(got)
        _DOMAIN_PENDING_TS[(d, got.lower())] = time.time()
        if len(q) > _DOMAIN_PENDING_NAMES_MAX:
            _DOMAIN_PENDING_NAMES[d] = q[-_DOMAIN_PENDING_NAMES_MAX:]


def peek_pending_upload_name_for_domain(domain: str) -> str:
    """Latest pending real name for domain (do not consume)."""
    d = (domain or "").strip().lower()
    if not d:
        return ""
    with _DOMAIN_PENDING_NAMES_LOCK:
        _purge_pending_names_locked()
        q = _DOMAIN_PENDING_NAMES.get(d) or []
        if not q:
            # Try domain alias roots
            for key, qq in _DOMAIN_PENDING_NAMES.items():
                if key.endswith(d) or d.endswith(key):
                    if qq:
                        got = qq[-1]
                        return got if _is_real_user_upload_name(got) else ""
            return ""
        got = q[-1]
    return got if _is_real_user_upload_name(got) else ""


def take_pending_upload_name_for_domain(domain: str) -> str:
    """Consume one pending real name for this domain (FIFO).

    Skip names already shown on a cached file so File A's name cannot label File B.
    """
    d = (domain or "").strip().lower()
    if not d:
        return ""
    got = ""
    with _DOMAIN_PENDING_NAMES_LOCK:
        _purge_pending_names_locked()

        def _pop_from(key: str) -> str:
            qq = _DOMAIN_PENDING_NAMES.get(key) or []
            if not qq:
                return ""
            val = qq.pop(0)
            if not qq:
                _DOMAIN_PENDING_NAMES.pop(key, None)
            else:
                _DOMAIN_PENDING_NAMES[key] = qq
            return val

        while True:
            val = _pop_from(d)
            if not val:
                for key in list(_DOMAIN_PENDING_NAMES.keys()):
                    if key != d and (key.endswith(d) or d.endswith(key)):
                        val = _pop_from(key)
                        if val:
                            break
            if not val:
                break
            if _is_real_user_upload_name(val):
                got = val
                break
    # Iterative fallback — avoid RecursionError if _upload_name_already_cached
    # keeps returning True for every pending name (max 10 attempts).
    _attempts = 0
    while got and _upload_name_already_cached(d, got) and _attempts < 10:
        _attempts += 1
        with _DOMAIN_PENDING_NAMES_LOCK:
            _purge_pending_names_locked()
            _next_val = ""
            qq = _DOMAIN_PENDING_NAMES.get(d) or []
            if qq:
                _next_val = qq.pop(0)
                if not qq:
                    _DOMAIN_PENDING_NAMES.pop(d, None)
                else:
                    _DOMAIN_PENDING_NAMES[d] = qq
        if not _next_val or not _is_real_user_upload_name(_next_val):
            got = ""
            break
        got = _next_val
    return got if _is_real_user_upload_name(got) and not _upload_name_already_cached(d, got) else ""


def list_pending_upload_names_for_domain(domain: str) -> list[str]:
    """Copy of pending real names (do not consume) — used to label multi-file Sends."""
    d = (domain or "").strip().lower()
    if not d:
        return []
    out: list[str] = []
    with _DOMAIN_PENDING_NAMES_LOCK:
        _purge_pending_names_locked()
        keys = [d]
        for key in _DOMAIN_PENDING_NAMES:
            if key != d and (key.endswith(d) or d.endswith(key)):
                keys.append(key)
        seen: set[str] = set()
        for key in keys:
            for n in _DOMAIN_PENDING_NAMES.get(key) or []:
                if n and _is_real_user_upload_name(n) and n.lower() not in seen:
                    seen.add(n.lower())
                    out.append(n)
    return out


def rename_recent_nameless_caches(domain: str, name: str, file_id: str = "") -> int:
    """Stamp a learned real name onto recent nameless cached uploads (any Target).

    ChatGPT/Gemini often cache bytes first (CDN PUT) and only later return
    {id, filename} on the create-file response. Without this, Prompt Logs stay
    'attachment' even after the real name is on the wire.
    """
    got = _sanitize_upload_filename(name or "")
    if not got or not _is_real_user_upload_name(got):
        return 0
    fid = (file_id or "").strip()
    aliases = upload_domain_aliases(domain) or [((domain or "").strip().lower())]
    aliases = [a for a in aliases if a]
    if not aliases:
        return 0
    changed = 0
    with _UPLOAD_FILE_CACHE_LOCK:
        _purge_upload_file_cache()
        candidates: list[dict] = []
        seen: set[str] = set()
        for alias in aliases:
            for entry in list(_UPLOAD_FILE_QUEUES.get(_upload_queue_key(alias), [])):
                uid = str(entry.get("cache_uid") or id(entry))
                if uid in seen:
                    continue
                seen.add(uid)
                candidates.append(entry)
            latest = _UPLOAD_FILE_CACHE.get(f"{alias}|latest")
            if latest:
                uid = str(latest.get("cache_uid") or id(latest))
                if uid not in seen:
                    seen.add(uid)
                    candidates.append(latest)
        # Prefer file_id match; else oldest nameless real-byte cache.
        targets: list[dict] = []
        if fid:
            for e in candidates:
                efid = (e.get("file_id") or "").strip()
                if efid and (efid == fid or efid == fid[5:] or ("file-" + efid) == fid):
                    targets.append(e)
        if not targets:
            nameless = [
                e for e in candidates
                if not _is_real_user_upload_name((e.get("file_name") or "").strip())
                and isinstance(e.get("raw_bytes"), (bytes, bytearray))
                and len(e.get("raw_bytes") or b"") >= 64
            ]
            nameless.sort(key=lambda e: float(e.get("ts") or 0))
            if nameless:
                targets = [nameless[0]]
        already = {
            (e.get("file_name") or "").strip().lower()
            for e in candidates
            if _is_real_user_upload_name((e.get("file_name") or "").strip())
        }
        for entry in targets:
            cur = (entry.get("file_name") or "").strip()
            raw = entry.get("raw_bytes") or b""
            if not isinstance(raw, (bytes, bytearray)):
                raw = b""
            raw_b = bytes(raw)
            ct = entry.get("content_type") or ""
            uid = str(entry.get("cache_uid") or id(entry))
            if got.lower() in already:
                # Name already belongs to another file — only keep if this IS that file.
                same = cur.lower() == got.lower()
                if not same:
                    continue
            if not _name_fits_upload_bytes(got, raw_b, ct):
                continue
            if _upload_name_quality(cur, raw_b, ct) >= _upload_name_quality(got, raw_b, ct):
                continue
            entry["file_name"] = _ensure_name_has_extension(got, raw_b, ct)
            if fid and not (entry.get("file_id") or "").strip():
                entry["file_id"] = fid
            if raw:
                remember_upload_name_by_bytes(bytes(raw), entry["file_name"])
            if fid:
                remember_upload_filename(fid, entry["file_name"])
            changed += 1
            already.add(entry["file_name"].lower())
            print(
                f"[Gateway Proxy] FILE NAME retro-bound | {domain} | "
                f"{cur or 'attachment'} -> {entry['file_name']}"
            )
    return changed


# Content hash → filename for nameless re-uploads.
_CONTENT_HASH_NAME_REGISTRY: dict[str, str] = {}
_CONTENT_HASH_NAME_LOCK = threading.Lock()
_CONTENT_HASH_NAME_MAX = 400


def _bytes_name_fingerprint(raw: bytes) -> str:
    """Full-content fingerprint (head/tail-only hashing merged different files)."""
    if not raw:
        return ""
    return f"{len(raw)}|{hashlib.blake2b(raw, digest_size=16).hexdigest()}"


def remember_upload_name_by_bytes(raw: bytes, name: str) -> None:
    got = _sanitize_upload_filename(name or "")
    if not got or not _is_real_user_upload_name(got):
        return
    if raw and not _name_fits_upload_bytes(got, raw or b"", ""):
        return
    key = _bytes_name_fingerprint(raw or b"")
    if not key:
        return
    with _CONTENT_HASH_NAME_LOCK:
        _CONTENT_HASH_NAME_REGISTRY[key] = got
        while len(_CONTENT_HASH_NAME_REGISTRY) > _CONTENT_HASH_NAME_MAX:
            _CONTENT_HASH_NAME_REGISTRY.pop(next(iter(_CONTENT_HASH_NAME_REGISTRY)), None)


def lookup_upload_name_by_bytes(raw: bytes) -> str:
    key = _bytes_name_fingerprint(raw or b"")
    if not key:
        return ""
    with _CONTENT_HASH_NAME_LOCK:
        got = _CONTENT_HASH_NAME_REGISTRY.get(key) or ""
    return got if _is_real_user_upload_name(got) else ""


def ingest_upload_filenames_from_body(raw_text: str, domain: str = "") -> None:
    """Learn file_id→name pairs from any Target's JSON (request OR response)."""
    if not raw_text or len(raw_text) < 8:
        return
    id_map = extract_file_id_name_map(raw_text)
    for fid, name in id_map.items():
        remember_upload_filename(fid, name)
        if domain:
            remember_pending_upload_name_for_domain(domain, name)
            try:
                rename_recent_nameless_caches(domain, name, fid)
            except Exception:
                pass
    # Standalone create-file payloads / display names without an id
    for m in re.finditer(
        r'["\'](?:file_name|fileName|filename|original_name|originalName|original_filename|originalFilename|display_name|displayName)["\']\s*:\s*["\']([^"\']+)["\']',
        raw_text,
        re.I,
    ):
        got = _sanitize_upload_filename(m.group(1))
        if got and _is_real_user_upload_name(got):
            remember_upload_filename(f"__pending__:{got.lower()}", got)
            if domain:
                remember_pending_upload_name_for_domain(domain, got)
                # No file_id: do NOT stamp the oldest nameless cache — that
                # reused File A's name on File B. Handshake pending is enough.
    # Deep JSON walk — ChatGPT/Gemini nest {id, filename} under data/file/attachment.
    stripped = (raw_text or "").lstrip()
    if stripped[:1] in ("{", "["):
        try:
            data = json.loads(raw_text)
        except Exception:
            data = None
        if data is not None:
            for fid, name in _walk_json_file_id_names(data).items():
                remember_upload_filename(fid, name)
                if domain:
                    remember_pending_upload_name_for_domain(domain, name)
                    try:
                        rename_recent_nameless_caches(domain, name, fid)
                    except Exception:
                        pass


def _walk_json_file_id_names(obj, out: dict[str, str] | None = None, depth: int = 0) -> dict[str, str]:
    """Recursively collect id↔filename pairs from any vendor JSON shape."""
    if out is None:
        out = {}
    if depth > 12 or obj is None:
        return out
    if isinstance(obj, dict):
        fid = ""
        for k in (
            "file_id", "fileId", "file_uuid", "fileUuid", "attachment_id",
            "attachmentId", "docId", "doc_id", "document_id", "documentId",
            "document_url", "documentUrl", "file_url", "fileUrl",
            "upload_id", "uploadId", "blob_id", "blobId", "asset_id", "assetId",
            "media_id", "mediaId", "storage_id", "storageId", "source_id", "sourceId",
            "kb_id", "dataset_id", "id",
        ):
            v = obj.get(k)
            if isinstance(v, str) and len(v.strip()) >= 6:
                fid = v.strip()
                break
        name = ""
        name_q = 0
        name_field = ""
        for k in (
            "filename", "file_name", "fileName", "original_filename", "originalFilename",
            "original_name", "originalName", "display_name", "displayName",
            "document_name", "documentName", "doc_name", "docName", "name", "title",
            "file", "path", "label",
        ):
            v = obj.get(k)
            if isinstance(v, str) and v.strip():
                cand = _sanitize_upload_filename(v)
                if not cand:
                    continue
                # ChatGPT puts chat/product titles in bare name/title — ignore those.
                if not _json_name_field_ok(cand, k):
                    continue
                q = _upload_name_quality(cand)
                # Prefer explicit file_name/filename keys over bare name/title.
                if k.lower() in {
                    "filename", "file_name",
                    "original_filename", "originalfilename",
                    "original_name", "originalname",
                    "display_name", "displayname",
                }:
                    q += 2
                if q > name_q:
                    name = cand
                    name_q = q
                    name_field = k
        if fid and name and _json_name_field_ok(name, name_field):
            prev = out.get(fid)
            if not prev or _upload_name_quality(name) > _upload_name_quality(prev):
                out[fid] = name
                if fid.startswith("file-"):
                    out[fid[5:]] = name
                else:
                    out["file-" + fid] = name
        for v in obj.values():
            if isinstance(v, (dict, list)):
                _walk_json_file_id_names(v, out, depth + 1)
    elif isinstance(obj, list):
        for v in obj[:400]:
            if isinstance(v, (dict, list)):
                _walk_json_file_id_names(v, out, depth + 1)
    return out


def extract_file_id_name_map(raw_text: str) -> dict[str, str]:
    """Map ChatGPT/Claude file_id → real filename when both appear near each other."""
    out: dict[str, str] = {}
    if not raw_text:
        return out

    def _put(fid: str, name: str) -> None:
        fid = (fid or "").strip()
        name = _sanitize_upload_filename(name or "")
        if not fid or not name or not _is_real_user_upload_name(name):
            return
        # Skip opaque ChatGPT ids mistaken for names
        if fid.lower() in ("file", "id", "name", "type", "role", "user"):
            return
        # Prefer file-like names; reject bare product/chat labels even if they slipped through.
        if not _has_any_file_extension(name) and not re.search(r"[\d()]", name):
            return
        prev = out.get(fid)
        # First real filename for this id wins. A later .pdf must not replace
        # notes.txt just because ".pdf" scores higher (that swapped names).
        if prev and _has_any_file_extension(prev):
            return
        if prev and _upload_name_quality(prev) >= _upload_name_quality(name):
            return
        out[fid] = name
        if fid.startswith("file-"):
            out[fid[5:]] = name
        else:
            out["file-" + fid] = name

    # Wider window — id/name may be far apart in JSON.
    # Prefer explicit file_* keys; bare name/title only when value has an extension.
    for m in re.finditer(
        r'(?:file_id|fileId|id)\s*"?\s*:\s*"((?:file-)?[A-Za-z0-9_-]{6,})"[^}]{0,800}?'
        r'(?:file_name|fileName|filename|original_name|originalName|display_name|displayName)\s*"?\s*:\s*"([^"]+)"',
        raw_text,
        re.I,
    ):
        _put(m.group(1), m.group(2))
    for m in re.finditer(
        r'(?:file_name|fileName|filename|original_name|originalName|display_name|displayName)\s*"?\s*:\s*"([^"]+)"[^}]{0,800}?'
        r'(?:file_id|fileId|id)\s*"?\s*:\s*"((?:file-)?[A-Za-z0-9_-]{6,})"',
        raw_text,
        re.I,
    ):
        _put(m.group(2), m.group(1))
    for m in re.finditer(
        r'(?:file_id|fileId|id)\s*"?\s*:\s*"((?:file-)?[A-Za-z0-9_-]{6,})"[^}]{0,800}?'
        r'(?:name|title)\s*"?\s*:\s*"([^"]+\.[A-Za-z0-9]{1,8})"',
        raw_text,
        re.I,
    ):
        _put(m.group(1), m.group(2))
    for m in re.finditer(
        r'(?:name|title)\s*"?\s*:\s*"([^"]+\.[A-Za-z0-9]{1,8})"[^}]{0,800}?'
        r'(?:file_id|fileId|id)\s*"?\s*:\s*"((?:file-)?[A-Za-z0-9_-]{6,})"',
        raw_text,
        re.I,
    ):
        _put(m.group(2), m.group(1))
    # ChatGPT file-service:// / sediment:// pointers next to a filename
    for m in re.finditer(
        r'(?:file-service://file-|sediment://file-)([A-Za-z0-9_-]{6,})[^\n]{0,2500}?'
        r'(?:file_name|fileName|filename)\s*"?\s*:\s*"([^"]+)"',
        raw_text,
        re.I,
    ):
        _put("file-" + m.group(1), m.group(2))
        _put(m.group(1), m.group(2))
    for m in re.finditer(
        r'(?:file_name|fileName|filename)\s*"?\s*:\s*"([^"]+)"[^\n]{0,2500}?'
        r'(?:file-service://file-|sediment://file-)([A-Za-z0-9_-]{6,})',
        raw_text,
        re.I,
    ):
        _put("file-" + m.group(2), m.group(1))
        _put(m.group(2), m.group(1))
    # Structured JSON wins over regex (avoids grabbing nearby "name":"screenshot").
    stripped = (raw_text or "").lstrip()
    if stripped[:1] in "{[":
        try:
            data = json.loads(raw_text)
        except Exception:
            data = None
        if data is not None:
            for fid, name in _walk_json_file_id_names(data).items():
                _put(fid, name)
    return out


def _upload_content_fingerprint(entry: dict) -> str:
    """Stable-ish fingerprint so the same PDF cached as real name + document.pdf merges.

    Distinct ChatGPT file_ids must never collapse together — even when extract_pdf
    truncates similar headers — or multi-file Prompt Logs lose names (Mac/Claude style).
    """
    raw = entry.get("raw_bytes") or b""
    if not isinstance(raw, (bytes, bytearray)):
        raw = b""
    raw = bytes(raw)
    fid = (entry.get("file_id") or "").strip().lower()
    if not raw:
        return f"empty|{fid or id(entry)}"
    body_fp = entry.get("_body_fp")
    if not body_fp or not str(body_fp).startswith(f"{len(raw)}|"):
        body_fp = _bytes_name_fingerprint(raw)
        entry["_body_fp"] = body_fp
    if fid:
        return f"id|{fid}|{body_fp}"
    return body_fp


def _prefer_named_upload(a: dict, b: dict) -> dict:
    """Keep the entry with the more user-real filename."""
    an = (a.get("file_name") or "").strip()
    bn = (b.get("file_name") or "").strip()
    a_real = _is_real_user_upload_name(an)
    b_real = _is_real_user_upload_name(bn)
    if a_real and not b_real:
        return a
    if b_real and not a_real:
        return b
    ar = a.get("raw_bytes") or b""
    br = b.get("raw_bytes") or b""
    if isinstance(ar, (bytes, bytearray)) and isinstance(br, (bytes, bytearray)):
        if len(br) > len(ar) + 64:
            return b
        if len(ar) > len(br) + 64:
            return a
    if len(an) >= len(bn):
        return a
    return b


def _dedupe_cached_uploads_by_bytes(cached_list: list[dict]) -> list[dict]:
    """One row per distinct file bytes — ChatGPT often caches the same PDF as name + attachment."""
    if not cached_list or len(cached_list) <= 1:
        return cached_list
    by_fp: dict[str, dict] = {}
    no_fp: list[dict] = []
    for e in cached_list:
        fp = _upload_content_fingerprint(e)
        if not fp or fp.startswith("empty|"):
            raw = e.get("raw_bytes") or b""
            if isinstance(raw, (bytes, bytearray)) and len(raw) < 96:
                continue
            no_fp.append(e)
            continue
        prev = by_fp.get(fp)
        by_fp[fp] = e if prev is None else _prefer_named_upload(prev, e)
    out = list(by_fp.values())
    if out:
        return out
    return no_fp or cached_list


_UPLOAD_BURST_WINDOW = 60.0   # files of ONE Send are uploaded within this many seconds
_UPLOAD_SEND_MAX_FILES = 128   # one Send may attach 100+ files; do not drop the rest


def _has_known_file_magic(data: bytes) -> bool:
    d = data[:16] if data else b""
    return bool(
        d[:5] == b"%PDF-" or d[:2] == b"PK" or d[:3] == b"\xff\xd8\xff"
        or d.startswith((b"\x89PNG", b"GIF8", b"RIFF", b"ID3", b"OggS", b"\x1f\x8b",
                         b"\xd0\xcf\x11\xe0", b"7z\xbc\xaf", b"Rar!", b"fLaC"))
        or d[4:8] == b"ftyp" or d[:4] == b"\x1aE\xdf\xa3"
    )


def _looks_binary_blob(data: bytes) -> bool:
    head = (data or b"")[:1024]
    if not head:
        return False
    if b"\x00" in head:
        return True
    bad = sum(1 for b in head if b < 9 or (13 < b < 32) or b > 126)
    return bad / len(head) > 0.30


def _upload_evidence(e: dict) -> int:
    """3 = real user name, 2 = known file magic, 1 = readable text file, 0 = opaque blob."""
    raw = e.get("raw_bytes") or b""
    raw = bytes(raw) if isinstance(raw, (bytes, bytearray)) else b""
    if _is_real_user_upload_name((e.get("file_name") or "").strip()):
        return 3
    if len(raw) >= 12 and _has_known_file_magic(raw):
        return 2
    if len(raw) >= 256 and not _looks_binary_blob(raw) and raw.lstrip()[:1] not in (b"{", b"[", b"<"):
        return 1
    return 0


def _rank_uploads(cached_list: list[dict], keep: int) -> list[dict]:
    """Strongest evidence first (then newest); return at most `keep`, in upload order."""
    ranked = sorted(
        cached_list,
        key=lambda e: (_upload_evidence(e), float(e.get("ts") or 0)),
        reverse=True,
    )[: max(1, keep)]
    return sorted(ranked, key=lambda e: float(e.get("ts") or 0))


def _drop_stale_and_partial_uploads(cached_list: list[dict]) -> list[dict]:
    """Keep only the newest upload burst; drop headerless binary continuation chunks.

    Without this every abandoned attach / chunk since the last Send was logged as its
    own 'attachment (k/22)' row on the next Send.
    """
    if not cached_list or len(cached_list) <= 1:
        return cached_list
    items = sorted(cached_list, key=lambda e: float(e.get("ts") or 0))
    newest = float(items[-1].get("ts") or 0)
    burst = [e for e in items if newest - float(e.get("ts") or 0) <= _UPLOAD_BURST_WINDOW]

    def _raw(e: dict) -> bytes:
        r = e.get("raw_bytes") or b""
        return bytes(r) if isinstance(r, (bytes, bytearray)) else b""

    with_magic = [e for e in burst if _has_known_file_magic(_raw(e))]
    if with_magic:
        keep = [
            e for e in burst
            if _has_known_file_magic(_raw(e))
            or _is_real_user_upload_name((e.get("file_name") or "").strip())
            or not _looks_binary_blob(_raw(e))     # plain-text files have no magic
        ]
    else:
        keep = burst
    keep = keep[-_UPLOAD_SEND_MAX_FILES:]
    if len(keep) != len(cached_list):
        print(
            f"[Gateway Proxy] FILE CACHE trimmed {len(cached_list)} -> {len(keep)} "
            f"(stale/partial dropped) | dropped="
            + ", ".join(
                f"{(e.get('file_name') or 'attachment')}:{len(_raw(e))}B:{_raw(e)[:4]!r}"
                for e in items if e not in keep
            )[:600]
        )
    return keep


def _cache_text(entry: dict) -> str:
    raw = entry.get("raw_bytes") or b""
    if not isinstance(raw, (bytes, bytearray)):
        return ""
    return bytes(raw).decode("utf-8", errors="ignore").strip()


def _is_prompt_echo_upload(entry: dict, caption: str) -> bool:
    """True when this cache row is the typed prompt, not a file.

    A real PDF/image/office body is never an echo. A row whose text is exactly
    the caption, or only "File name: …", is the prompt being logged as a second attachment.
    """
    raw = entry.get("raw_bytes") or b""
    if isinstance(raw, (bytes, bytearray)) and _has_known_file_magic(bytes(raw)):
        return False
    text = _cache_text(entry)
    if not text:
        return False
    if text.lower().startswith("file name:") and len(text) < 400:
        return True
    cap = (caption or "").strip()
    return bool(cap) and text == cap


def _drop_prompt_echo_uploads(cached_list: list[dict], caption: str) -> list[dict]:
    """Keep every real file. Drop a sibling row that is only the typed prompt."""
    if len(cached_list) <= 1:
        return cached_list
    echoes = [e for e in cached_list if _is_prompt_echo_upload(e, caption)]
    if not echoes or len(echoes) == len(cached_list):
        return cached_list
    kept = [e for e in cached_list if e not in echoes]
    return kept or cached_list


def _trim_phantom_upload_caches(
    cached_list: list[dict],
    raw_text: str = "",
    caption: str = "",
) -> list[dict]:
    """Drop caption phantoms / duplicate caches on ANY Target Website."""
    if not cached_list:
        return cached_list

    cached_list = _dedupe_cached_uploads_by_bytes(cached_list)
    cached_list = _drop_stale_and_partial_uploads(cached_list)

    # A Send that references NO file (no name/id/attachment object anywhere in the decoded
    # body) must not swallow opaque background blobs as 'attachment' rows (typed 'helo'
    # -> 4 phantom rows). Real files still bind: they carry a name or a known file magic.
    try:
        _ref_names = [
            n for n in extract_all_attachment_filenames_from_send(raw_text or "")
            if _is_real_user_upload_name(n)
        ]
        _ref_count = max(len(_ref_names), _expected_send_attachment_count(raw_text or ""))
    except Exception:
        _ref_count = 0
    if _ref_count == 0:
        strong = [e for e in cached_list if _upload_evidence(e) >= 2]
        if len(strong) != len(cached_list):
            print(
                f"[Gateway Proxy] FILE CACHE phantom blobs ignored: {len(cached_list) - len(strong)} "
                "(no file reference in Send, no file magic/name)"
            )
        cached_list = strong
        if not cached_list:
            return []
    if len(cached_list) <= 1:
        return cached_list

    send_names = [
        n for n in extract_all_attachment_filenames_from_send(raw_text or "")
        if _is_real_user_upload_name(n)
    ]
    expected = 0
    try:
        expected = _expected_send_attachment_count(raw_text or "")
    except Exception:
        expected = len(send_names)
    if len(cached_list) <= 1:
        return cached_list

    realish = [e for e in cached_list if _is_real_user_upload_name((e.get("file_name") or "").strip())]
    fakeish = [e for e in cached_list if not _is_real_user_upload_name((e.get("file_name") or "").strip())]

    # Multi-file Send: never drop nameless siblings just because one row already
    # has a real name — that was causing only 1/N files to predict.
    if expected > 1 and len(cached_list) <= max(expected + 2, expected):
        keep = [
            e for e in cached_list
            if len(e.get("raw_bytes") or b"") >= 64
            or _is_real_user_upload_name((e.get("file_name") or "").strip())
        ]
        if keep:
            keep = _drop_prompt_echo_uploads(keep, caption or "")
            return _dedupe_cached_uploads_by_bytes(keep)

    # Drop tiny caption/metadata phantoms next to a real file. Keep unnamed
    # rows that still look like real documents (multi-file: 1 named + 2 nameless).
    if realish and fakeish:
        tiny_named = [e for e in realish if len(e.get("raw_bytes") or b"") < 256]
        solid_named = [e for e in realish if e not in tiny_named]
        keep_fake = [
            e for e in fakeish
            if len(e.get("raw_bytes") or b"") >= 256
        ]
        # Gemini StreamGenerate logs two rows: a tiny "File name: X" field plus the
        # real bytes labeled "attachment". Move the real name onto the file and drop
        # the name-only phantom so one Send is one file.
        if tiny_named and keep_fake and not solid_named:
            for i, e in enumerate(keep_fake):
                if i < len(tiny_named):
                    moved = (tiny_named[i].get("file_name") or "").strip()
                    if moved and not _is_real_user_upload_name((e.get("file_name") or "").strip()):
                        e["file_name"] = moved
            return _dedupe_cached_uploads_by_bytes(keep_fake)
        if keep_fake:
            keep_fake = [e for e in keep_fake if not _is_prompt_echo_upload(e, caption or "")]
            tiny_named = [e for e in tiny_named if not _is_prompt_echo_upload(e, caption or "")]
            return _dedupe_cached_uploads_by_bytes(solid_named + tiny_named + keep_fake)
        if expected > 1:
            return _dedupe_cached_uploads_by_bytes(realish + fakeish)
        return _dedupe_cached_uploads_by_bytes(realish)

    if send_names or expected > 1:
        target = max(len(send_names), expected, 1)
        uniq_real: list[dict] = []
        seen_fp: set[str] = set()
        for e in realish:
            # Same display name must not drop a different file (5 uploads → 4 rows).
            fp = _upload_content_fingerprint(e) or str(e.get("cache_uid") or id(e))
            if fp in seen_fp:
                continue
            seen_fp.add(fp)
            uniq_real.append(e)
        if len(uniq_real) >= target:
            # Prefer exact target count but never collapse multi distinct bytes.
            if len(uniq_real) == target:
                return uniq_real
            return _dedupe_cached_uploads_by_bytes(uniq_real)[: max(target, len(uniq_real))]
        if uniq_real:
            need = max(target - len(uniq_real), 0)
            return uniq_real + (_rank_uploads(fakeish, need) if (need and fakeish) else [])
        # Nameless rows: the Send references `target` files — keep the `target` strongest,
        # not all of them (that produced attachment (1/7) ... (7/7) for one Send).
        return _rank_uploads(_dedupe_cached_uploads_by_bytes(cached_list), target)

    if (caption or "").strip() and len(cached_list) > 1 and expected <= 1:
        # Only drop small items if they are fake/nameless placeholders (attachment/document),
        # NEVER drop items with real filenames (budget.pdf, notes.csv) or strong evidence!
        real_or_strong = [
            e for e in cached_list
            if _is_real_user_upload_name((e.get("file_name") or "").strip())
            or _upload_evidence(e) >= 2
            or len(e.get("raw_bytes") or b"") >= 256
        ]
        if real_or_strong:
            return _dedupe_cached_uploads_by_bytes(real_or_strong)
    return cached_list


def _bind_real_filenames_to_cached_uploads(cached_list: list[dict], raw_text: str) -> list[dict]:
    """Attach real names from Send body / registries onto cached uploads (any Target)."""
    if not cached_list:
        return cached_list
    id_map = extract_file_id_name_map(raw_text or "")
    send_names = _prefer_real_filenames(
        n for n in extract_all_attachment_filenames_from_send(raw_text or "")
        if _is_real_user_upload_name(n)
    )
    used_names: set[str] = set()
    # If File A's name was stolen onto File B, keep it on the first distinct-bytes
    # row and clear the duplicate so we can bind B's real name.
    seen_name_uid: dict[str, str] = {}
    for entry in cached_list:
        cur = (entry.get("file_name") or "").strip()
        if not _is_real_user_upload_name(cur) or _needs_real_upload_filename(cur):
            continue
        key = cur.lower()
        uid = str(entry.get("cache_uid") or id(entry))
        if key in seen_name_uid and seen_name_uid[key] != uid:
            # Two different files must keep their own names. Blanking the
            # second one let the next file's name get assigned here.
            continue
        seen_name_uid[key] = uid
        used_names.add(key)
    # Domain pending FIFO (do not peek the same latest name onto every row).
    pending_pool: list[str] = []
    try:
        domains = []
        for e in cached_list:
            d = (e.get("domain") or "").strip()
            if d and d not in domains:
                domains.append(d)
        for d in domains:
            for n in list_pending_upload_names_for_domain(d):
                if n and n.lower() not in {x.lower() for x in pending_pool}:
                    pending_pool.append(n)
    except Exception:
        pending_pool = []
    pending_i = 0

    for entry in cached_list:
        cur = (entry.get("file_name") or "").strip()
        fid = (entry.get("file_id") or "").strip()
        raw = entry.get("raw_bytes") or b""
        if not isinstance(raw, (bytes, bytearray)):
            raw = b""
        _id_in_send = bool(fid) and (
            fid in id_map
            or (fid.startswith("file-") and fid[5:] in id_map)
            or (not fid.startswith("file-") and ("file-" + fid) in id_map)
        )
        # The file_id -> name written in THIS Send beats the bytes-hash memory
        # (same content re-uploaded under a new name must take the new name).
        if _needs_real_upload_filename(cur) and raw and not _id_in_send:
            by_bytes = lookup_upload_name_by_bytes(bytes(raw))
            if by_bytes and by_bytes.lower() not in used_names:
                entry["file_name"] = by_bytes
                cur = by_bytes
        if (
            fid and fid in id_map and _is_real_user_upload_name(id_map[fid])
            and _needs_real_upload_filename(cur)
        ):
            incoming = id_map[fid]
            if _name_fits_upload_bytes(incoming, bytes(raw), entry.get("content_type") or "") and (
                _needs_real_upload_filename(cur)
                or _upload_name_quality(incoming, bytes(raw), entry.get("content_type") or "")
                > _upload_name_quality(cur, bytes(raw), entry.get("content_type") or "")
            ):
                entry["file_name"] = incoming
                cur = incoming
                if raw:
                    remember_upload_name_by_bytes(bytes(raw), incoming)
            used_names.add((entry.get("file_name") or cur).lower())
            continue
        if (
            fid.startswith("file-") and fid[5:] in id_map and _is_real_user_upload_name(id_map[fid[5:]])
            and _needs_real_upload_filename(cur)
        ):
            incoming = id_map[fid[5:]]
            if _name_fits_upload_bytes(incoming, bytes(raw), entry.get("content_type") or "") and (
                _needs_real_upload_filename(cur)
                or _upload_name_quality(incoming, bytes(raw), entry.get("content_type") or "")
                > _upload_name_quality(cur, bytes(raw), entry.get("content_type") or "")
            ):
                entry["file_name"] = incoming
                cur = incoming
                if raw:
                    remember_upload_name_by_bytes(bytes(raw), incoming)
            used_names.add((entry.get("file_name") or cur).lower())
            continue
        if fid and _needs_real_upload_filename(cur):
            remembered = lookup_upload_filename(fid)
            if remembered:
                entry["file_name"] = remembered
                cur = remembered
                if raw:
                    remember_upload_name_by_bytes(bytes(raw), remembered)
        if _needs_real_upload_filename(cur):
            while pending_i < len(pending_pool) and pending_pool[pending_i].lower() in used_names:
                pending_i += 1
            if pending_i < len(pending_pool):
                pending = pending_pool[pending_i]
                pending_i += 1
                if pending.lower() in used_names:
                    continue
                entry["file_name"] = pending
                cur = pending
                if raw:
                    remember_upload_name_by_bytes(bytes(raw), pending)
                try:
                    take_pending_upload_name_for_domain(entry.get("domain") or "")
                except Exception:
                    pass
        if _is_real_user_upload_name(cur) and not _needs_real_upload_filename(cur):
            used_names.add(cur.lower())
            if raw:
                remember_upload_name_by_bytes(bytes(raw), cur)

    unused = [n for n in send_names if n.lower() not in used_names]
    ui = 0
    for i, entry in enumerate(cached_list):
        cur = (entry.get("file_name") or "").strip()
        if _is_real_user_upload_name(cur) and not _needs_real_upload_filename(cur):
            continue
        if ui < len(unused):
            entry["file_name"] = unused[ui]
            raw = entry.get("raw_bytes") or b""
            if isinstance(raw, (bytes, bytearray)) and raw:
                remember_upload_name_by_bytes(bytes(raw), unused[ui])
            used_names.add(unused[ui].lower())
            ui += 1
            continue
        if _is_fake_upload_name(cur) or not cur:
            entry["file_name"] = cur or f"attachment-{i + 1}"
    return _dedupe_cached_uploads_by_bytes(cached_list)


def _display_label_for_upload(name: str, raw: bytes, content_type: str = "") -> str:
    """Human label for Prompt Logs — real name + ZIP member preview when useful."""
    label = (name or "").strip() or "attachment"
    # Never show ChatGPT paste placeholders (screenshot) or brand/PDF-title leaks.
    if _is_fake_upload_name(label) or not _is_real_user_upload_name(label):
        label = _default_name_from_bytes(raw or b"", content_type) or "attachment"
    kind = ""
    try:
        kind = _classify_upload_kind(raw or b"", content_type, label)
    except Exception:
        kind = ""
    if kind == "zip" or label.lower().endswith(".zip"):
        members = _list_zip_member_basenames(raw or b"")
        if members:
            preview = ", ".join(members[:4])
            more = f" +{len(members) - 4} more" if len(members) > 4 else ""
            return f"{label} [{len(members)} files: {preview}{more}]"
    return label


def is_confident_file_upload(
    *,
    fname: str,
    content_type: str,
    raw_bytes: bytes,
    raw_text: str,
    upload_reason: str = "",
    host: str = "",
    path: str = "",
) -> bool:
    """True only for a real user file pick/upload — not chat/telemetry noise.

    Used to CACHE bytes. Prompt Log + Block happen only later on chat Send.
    """
    name = (fname or "").strip()
    name_l = name.lower()
    ct = (content_type or "").lower()
    data = raw_bytes or b""
    reason = (upload_reason or "").lower()
    raw = raw_text or ""
    host_l = (host or "").lower()
    path_l = (path or "").lower().split("?", 1)[0]

    # Monitored upload paths: cache real bytes, skip tiny JSON.
    if detect_target(host_l)[0] and _path_looks_like_upload(path_l) and len(data) >= 32:
        if name and name_l not in _FAKE_UPLOAD_NAMES:
            return True
        if any(x in ct for x in (
            "octet-stream", "multipart", "pdf", "image/", "audio/", "video/",
            "msword", "officedocument",
        )):
            return True
        if data[:5] == b"%PDF-" or (len(data) >= 2 and data[:2] == b"PK"):
            return True
        if b"filename=" in data[:16000].lower() or b"filename*=" in data[:16000].lower():
            return True
        if len(data) >= 2048:
            return True
        # JSON file/voice wrappers on monitored upload paths (Claude/Gemini/DeepSeek/…).
        if data.lstrip()[:1] in (b"{", b"["):
            j_bytes, _, _ = _extract_bytes_from_json_upload(data, name)
            if j_bytes and len(j_bytes) >= 32:
                return True

    # Any host: JSON body that already carries extractable file/voice bytes.
    if data.lstrip()[:1] in (b"{", b"[") and len(data) >= 120:
        j_bytes, _, _ = _extract_bytes_from_json_upload(data, name)
        if j_bytes and len(j_bytes) >= 32:
            return True

    # Google resumable finalize/upload with bytes
    if "google resumable" in reason and len(data) >= 64:
        if any(x in reason for x in ("finalize", "upload", "append")):
            return True
        return False

    # Real filename with extension (not placeholder names)
    if name and name_l not in _FAKE_UPLOAD_NAMES and "." in name:
        ext = name_l.rsplit(".", 1)[-1]
        if 1 <= len(ext) <= 5 and ext.isalnum():
            return True

    # Magic bytes / known file shapes (strong evidence)
    if len(data) >= 64:
        if data[:5] == b"%PDF-" or b"%PDF-" in data[:4096]:
            return True
        if _looks_like_image(data, ct, name):
            return True
        if _looks_like_audio(data, ct, name):
            return True
        if data[:2] == b"PK" and len(data) >= 1024:
            # OOXML / zip — only if upload-ish path or office content-type
            if "officedocument" in ct or "zip" in ct or any(
                x in path_l for x in ("/upload", "/files", "/attachment", "/convert")
            ):
                return True

    # Multipart with non-empty real filename=
    if "filename=" in raw.lower() or "filename*=" in raw.lower():
        m = re.search(r'filename\*?=(?:UTF-8\'\')?["\']?([^"\';\r\n]+)["\']?', raw[:16000], re.I)
        if m:
            got = (m.group(1) or "").strip().strip('"\'')
            if got and got.lower() not in _FAKE_UPLOAD_NAMES:
                return True

    # Binary content-type alone is NOT enough (media-sync noise).
    # Require upload path + meaningful size + binary type.
    uploadish = _path_looks_like_upload(path_l) or "/media/" in path_l
    if uploadish and len(data) >= 2048 and any(
        x in ct
        for x in (
            "octet-stream", "application/pdf", "image/", "audio/", "video/",
            "msword", "officedocument",
        )
    ):
        return True

    return False


def extract_filename_from_upload(flow: http.HTTPFlow, raw_text: str = "") -> str:
    """Extract uploaded file name from headers, JSON body, multipart body, or URL path."""
    raw_bytes = b""
    try:
        raw_bytes = flow.request.content or b""
    except Exception:
        raw_bytes = b""
    got = _filename_from_multipart_or_headers(raw_bytes, flow.request.headers, raw_text or "")
    if got:
        return got

    # Query / signed URL filename (ChatGPT file-service, Gemini fife)
    try:
        url_name = _filename_from_url(getattr(flow.request, "url", "") or flow.request.path or "")
        if url_name:
            return url_name
    except Exception:
        pass

    # Path parameter if it ends with a file extension
    path_clean = (flow.request.path or "").split("?", 1)[0]
    last_seg = path_clean.rsplit("/", 1)[-1]
    if "." in last_seg:
        name = _sanitize_upload_filename(urllib.parse.unquote(last_seg))
        if name and any(name.lower().endswith(ext) for ext in _UPLOAD_NAME_EXTS):
            return name

    return ""


def extract_pdf_bytes(raw: bytes) -> bytes | None:
    """Return PDF payload if present in raw upload body (direct or multipart)."""
    if not raw:
        return None
    if raw.startswith(b"%PDF-"):
        data = raw
    else:
        idx = raw.find(b"%PDF-")
        if idx < 0 or idx > 64 * 1024:
            return None
        data = raw[idx:]
    eof = data.rfind(b"%%EOF")
    if eof >= 0:
        end = eof + 5
        while end < len(data) and data[end] in (10, 13):
            end += 1
        data = data[:end]
    if len(data) > 20 * 1024 * 1024:
        return None
    return data


def _sniff_upload_content_type(data: bytes, file_name: str = "", hint: str = "") -> str:
    hint = (hint or "").split(";")[0].strip().lower()
    if hint and hint not in ("application/octet-stream", "binary/octet-stream"):
        return hint
    if data.startswith(b"%PDF-") or b"%PDF-" in data[:4096]:
        return "application/pdf"
    if len(data) >= 3 and data[0] == 0xFF and data[1] == 0xD8 and data[2] == 0xFF:
        return "image/jpeg"
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "image/png"
    if data.startswith(b"GIF87a") or data.startswith(b"GIF89a"):
        return "image/gif"
    if len(data) >= 12 and data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return "image/webp"
    if data[:2] == b"PK":
        low = (file_name or "").lower()
        if low.endswith(".docx"):
            return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
        if low.endswith(".xlsx"):
            return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        if low.endswith(".pptx"):
            return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
        return "application/zip"
    ext = os.path.splitext(file_name or "")[1].lower()
    return {
        ".pdf": "application/pdf",
        ".png": "image/png",
        ".jpg": "image/jpeg",
        ".jpeg": "image/jpeg",
        ".gif": "image/gif",
        ".webp": "image/webp",
        ".txt": "text/plain",
        ".csv": "text/csv",
        ".json": "application/json",
    }.get(ext, "application/octet-stream")


def _extract_bytes_from_json_upload(raw: bytes, file_name: str = "") -> tuple[bytes | None, str, str]:
    """Pull real file/voice bytes from JSON wrappers (Claude/Gemini/DeepSeek/etc).

    ChatGPT often sends multipart/octet-stream; other AIs wrap base64 in JSON.
    Tiny metadata handshakes (file_id only, no payload) return (None, "", name).
    """
    if not raw:
        return None, "", file_name or "attachment"
    stripped = raw.lstrip()
    if stripped[:1] not in (b"{", b"[") or raw[:2] == b"PK":
        return None, "", file_name or "attachment"
    if len(raw) < 120:
        return None, "", file_name or "attachment"

    try:
        text = raw.decode("utf-8", errors="ignore")
    except Exception:
        return None, "", file_name or "attachment"

    name = _sanitize_upload_filename(file_name) or (file_name or "").strip() or "attachment"
    for m in re.finditer(
        r'"(?:file_name|fileName|filename|original_name|originalName|display_name|displayName)"\s*:\s*"([^"]{1,240})"',
        text,
        re.I,
    ):
        got = _sanitize_upload_filename(m.group(1))
        if got and _is_real_user_upload_name(got):
            name = got
            break
    # Bare "name" only when it looks like a real filename (has extension).
    if _is_fake_upload_name(name) or not _is_real_user_upload_name(name):
        for m in re.finditer(
            r'"(?:name|title)"\s*:\s*"([^"]+\.[A-Za-z0-9]{1,8})"',
            text,
            re.I,
        ):
            got = _sanitize_upload_filename(m.group(1))
            if got and _is_real_user_upload_name(got):
                name = got
                break

    inlines = extract_all_inline_attachment_bytes(text)
    if inlines:
        data, mime, inline_name = inlines[0]
        if data and len(data) >= 32:
            out_name = name
            if _is_fake_upload_name(out_name) and inline_name and not _is_fake_upload_name(inline_name):
                out_name = inline_name
            ctype = mime or _sniff_upload_content_type(data, out_name)
            return data, ctype, out_name or "attachment"

    m_data_url = re.search(
        r'data:([a-zA-Z0-9.+/-]+);base64,([A-Za-z0-9+/=\s]{80,})',
        text,
        re.I,
    )
    if m_data_url:
        import base64
        mime = (m_data_url.group(1) or "").strip()
        blob = (m_data_url.group(2) or "").replace("\n", "").replace("\r", "").replace(" ", "")
        try:
            data = base64.b64decode(blob, validate=False)
        except Exception:
            data = b""
        if len(data) >= 32:
            ctype = mime or _sniff_upload_content_type(data, name)
            return data, ctype, name or "attachment"

    return None, "", name


def iter_multipart_named_files(raw: bytes) -> list[tuple[str, bytes]]:
    """Each filename= part when one request carries two or more files (Claude batch upload)."""
    if not raw or raw.lower().count(b"filename=") < 2:
        return []
    out: list[tuple[str, bytes]] = []
    low = raw.lower()
    start = 0
    while True:
        idx = low.find(b"filename=", start)
        if idx < 0:
            break
        rest = raw[idx:]
        hdr_end = rest.find(b"\r\n\r\n")
        sep_len = 4
        if hdr_end < 0:
            hdr_end = rest.find(b"\n\n")
            sep_len = 2
        if hdr_end < 0:
            break
        hdr = rest[:hdr_end].decode("utf-8", errors="ignore")
        hm = re.search(r'filename\*?=(?:UTF-8\'\')?["\']?([^"\';\r\n]+)["\']?', hdr, re.I)
        name = _sanitize_upload_filename(hm.group(1)) if hm else ""
        body = rest[hdr_end + sep_len :]
        end = len(body)
        for i in range(max(0, len(body) - 2)):
            if body[i] == 10 and body[i + 1] == 45 and body[i + 2] == 45:
                end = i - 1 if i > 0 and body[i - 1] == 13 else i
                break
        part = body[:end]
        if name and part and _is_real_user_upload_name(name):
            out.append((name, part))
        start = idx + len(b"filename=")
    return out if len(out) >= 2 else []


def extract_upload_file_payload(raw: bytes, content_type: str = "", file_name: str = "") -> tuple[bytes | None, str, str]:
    """Best-effort file bytes from upload body. Returns (bytes, content_type, name)."""
    if not raw:
        return None, "", file_name or "attachment"
    name = _sanitize_upload_filename(file_name) or (file_name or "").strip() or "attachment"
    # Prefer multipart / header filename when caller only had a placeholder.
    mp_name = _filename_from_multipart_or_headers(raw, None, "")
    if mp_name and _is_fake_upload_name(name):
        name = mp_name
    elif mp_name and not _is_fake_upload_name(mp_name):
        name = mp_name

    pdf = extract_pdf_bytes(raw)
    if pdf:
        # Do NOT invent document.pdf here — that became the Prompt Log label for
        # Gemini/ChatGPT when the real name arrives later on Send. Keep placeholder.
        if not (name or "").lower().endswith(".pdf") and _is_real_user_upload_name(name):
            name = f"{name}.pdf"
        elif _is_fake_upload_name(name) or not name:
            name = "attachment"
        return pdf, "application/pdf", name

    # JSON base64 file wrappers (Claude/Gemini/DeepSeek).
    # Extract real payload when present; only skip tiny metadata handshakes.
    stripped = raw.lstrip()
    if stripped[:1] in (b"{", b"[") and raw[:2] != b"PK":
        j_bytes, j_ct, j_name = _extract_bytes_from_json_upload(raw, name)
        if j_bytes:
            return j_bytes, j_ct or content_type or "application/octet-stream", j_name or name
        return None, "", name

    # Multipart: extract part after filename=
    low_prefix = raw[: min(len(raw), 64 * 1024)].lower()
    if b"filename=" in low_prefix or b"webkitformboundary" in low_prefix or b"multipart" in (content_type or "").lower().encode():
        idx = low_prefix.find(b"filename=")
        if idx >= 0:
            rest = raw[idx:]
            # Capture filename= value from this part header
            try:
                hdr_end = rest.find(b"\r\n\r\n")
                hdr = rest[: hdr_end if hdr_end >= 0 else 800].decode("utf-8", errors="ignore")
                hm = re.search(r'filename\*?=(?:UTF-8\'\')?["\']?([^"\';\r\n]+)["\']?', hdr, re.I)
                if hm:
                    got = _sanitize_upload_filename(hm.group(1))
                    if got:
                        name = got
            except Exception:
                pass
            sep = b"\r\n\r\n"
            si = rest.find(sep)
            if si < 0:
                sep = b"\n\n"
                si = rest.find(sep)
            if si >= 0:
                body = rest[si + len(sep) :]
                end = len(body)
                for i in range(len(body) - 2):
                    if body[i] == 10 and body[i + 1] == 45 and body[i + 2] == 45:  # \n--
                        end = i - 1 if i > 0 and body[i - 1] == 13 else i
                        break
                part = body[:end]
                if part:
                    pdf2 = extract_pdf_bytes(part)
                    if pdf2:
                        if not name.lower().endswith(".pdf"):
                            if _is_fake_upload_name(name):
                                # Keep placeholder — Send/registry can still bind the real name.
                                name = name or "attachment"
                            else:
                                name = f"{name}.pdf"
                        return pdf2, "application/pdf", name
                    ctype = _sniff_upload_content_type(part, name, content_type)
                    if len(part) > 20 * 1024 * 1024:
                        part = part[: 20 * 1024 * 1024]
                    # Do not invent document.pdf here — that becomes a fake "prompt" in logs.
                    return part, ctype, name or "attachment"

    # Direct binary body (resumable / octet-stream / plain files)
    if len(raw) >= 32 or (len(raw) >= 1 and (_has_any_file_extension(name) or (content_type or "").lower().startswith("text/"))):
        ctype = _sniff_upload_content_type(raw, name, content_type)
        # Skip tiny JSON metadata
        stripped = raw.lstrip()
        if stripped[:1] in (b"{", b"[") and len(raw) < 50_000 and ctype == "application/octet-stream":
            return None, "", name
        data = raw if len(raw) <= 20 * 1024 * 1024 else raw[: 20 * 1024 * 1024]
        # Keep placeholder — inventing document.pdf creates fake Prompt Log rows
        # that never get replaced by the real name from create-file / Send.
        if _is_fake_upload_name(name):
            name = "attachment"
        return data, ctype, name
    return None, "", name


def _purge_upload_file_cache(now: float | None = None) -> None:
    now = now if now is not None else time.time()
    dead = [k for k, v in _UPLOAD_FILE_CACHE.items() if now - float(v.get("ts") or 0) > _UPLOAD_FILE_CACHE_TTL]
    for k in dead:
        _UPLOAD_FILE_CACHE.pop(k, None)
    while len(_UPLOAD_FILE_CACHE) > _UPLOAD_FILE_CACHE_MAX:
        oldest = min(_UPLOAD_FILE_CACHE.items(), key=lambda kv: float(kv[1].get("ts") or 0))
        _UPLOAD_FILE_CACHE.pop(oldest[0], None)
    _purge_upload_queues(now)


def _upload_queue_key(alias: str) -> str:
    return f"{alias}::__queue__"


def _purge_upload_queues(now: float | None = None) -> None:
    now = now if now is not None else time.time()
    for qkey in list(_UPLOAD_FILE_QUEUES.keys()):
        q = _UPLOAD_FILE_QUEUES.get(qkey) or []
        alive = [e for e in q if now - float(e.get("ts") or 0) <= _UPLOAD_FILE_CACHE_TTL]
        if alive:
            _UPLOAD_FILE_QUEUES[qkey] = alive[-_UPLOAD_FILE_QUEUE_MAX:]
        else:
            _UPLOAD_FILE_QUEUES.pop(qkey, None)


def _remove_cache_keys_for_entry(entry: dict, aliases: list[str]) -> None:
    fname = (entry.get("file_name") or "attachment").lower()
    fid = entry.get("file_id") or ""
    uid = entry.get("cache_uid")
    for alias in aliases:
        _UPLOAD_FILE_CACHE.pop(f"{alias}|latest", None)
        _UPLOAD_FILE_CACHE.pop(f"{alias}|name|{fname}", None)
        if fid:
            _UPLOAD_FILE_CACHE.pop(f"{alias}|id|{fid}", None)
            _UPLOAD_FILE_CACHE.pop(f"{alias}|id|file-{fid}", None)
        if uid:
            qkey = _upload_queue_key(alias)
            if qkey in _UPLOAD_FILE_QUEUES:
                _UPLOAD_FILE_QUEUES[qkey] = [
                    e for e in _UPLOAD_FILE_QUEUES[qkey] if e.get("cache_uid") != uid
                ]


def upload_domain_aliases(domain: str) -> list[str]:
    """All admin-added Target Websites in the same family as `domain`.

    Built from /api/browser-ai/targets (platform_name + parent_id + subdomain).
    Upload may hit host A; chat Send hits host B — cache is shared across the family.
    """
    get_target_domains()
    d = _normalize_domain(domain or "")
    if not d:
        return []

    fam = _cached_families.get(d)
    if fam:
        return sorted(fam)

    # Longest-suffix match by walking labels: O(labels), not O(#targets).
    labels = d.split(".")
    for i in range(1, len(labels)):
        fam = _cached_families.get(".".join(labels[i:]))
        if fam:
            return sorted(fam)

    # Single monitored domain with no siblings in Target Websites.
    return [d]


def _is_file_metadata_handshake(raw_bytes: bytes = b"", raw_text: str = "", path: str = "") -> bool:
    """ChatGPT / Gemini / Copilot create-file JSON — real name, no document bytes."""
    raw = raw_bytes or b""
    text = raw_text or ""
    if not text and raw:
        try:
            text = raw.decode("utf-8", errors="ignore")
        except Exception:
            text = ""
    stripped = raw.lstrip() if raw else (text.encode("utf-8", errors="ignore") if text else b"")
    if stripped[:1] not in (b"{", b"["):
        return False
    if len(raw) >= 64 * 1024:
        return False
    if raw[:5] == b"%PDF-" or raw[:2] == b"PK":
        return False
    low = (text or "").lower()
    if not low:
        return False
    # Never treat a chat Send as create-file (Send also embeds file_name + file_size).
    if any(
        k in low
        for k in (
            '"conversation_id"', '"parent_message_id"', '"messages"',
            '"contents"', '"action":"next"', '"author"',
        )
    ):
        return False
    if "base64" in low and len(raw) > 4000:
        return False
    has_name = bool(re.search(
        r'"(?:file_name|filename|fileName|original_name|originalName|original_filename|'
        r'originalFilename|display_name|displayName|documentName|document_name)"\s*:\s*"[^"]+"',
        low,
    ))
    has_meta = any(
        k in low
        for k in (
            "file_size", "filesize", "use_case", "upload_url", "reset_rate_limits",
            "timezone_offset", "num_bytes", '"purpose"',
            "mimetype", "mime_type", "sessionname", "uploadtype", "upload_type",
            "byte_size", "bytesize", "content_length",
        )
    )
    path_l = (path or "").lower()
    return bool(
        has_name
        and (
            has_meta
            or "/files" in path_l
            or "/upload" in path_l
            or "/fife" in path_l
            or "/file" in path_l
        )
    )


def remember_file_create_handshake(raw_text: str, domain: str, raw_bytes: bytes = b"", path: str = "") -> bool:
    """Learn real filename from create-file JSON. Never cache/log this step."""
    if not _is_file_metadata_handshake(raw_bytes or b"", raw_text or "", path):
        return False
    if domain:
        ingest_upload_filenames_from_body(raw_text or "", domain)
        for n in extract_all_attachment_filenames_from_send(raw_text or ""):
            remember_pending_upload_name_for_domain(domain, n)
            print(f"[Gateway Proxy] FILE NAME from create-file | {domain} | {n} (await Send)")
    return True


def _is_finished_user_file_send(
    path: str,
    raw_text: str,
    raw_bytes: bytes = b"",
    host: str = "",
) -> bool:
    """True only for a finished chat Send that may carry a file — not attach/create-file."""
    path_l = (path or "").lower().split("?", 1)[0]
    body = raw_text or ""
    if _path_has_ignore_pattern(path_l) or is_noise(path_l, body):
        return False
    if _path_looks_like_upload(path_l):
        return False
    if _is_typing_or_draft_path(path_l, body):
        return False
    if _is_gateway_inject_frame(body):
        return False
    if _is_file_metadata_handshake(raw_bytes or b"", body, path):
        return False
    if _is_persistent_chat_websocket(path):
        return _copilot_frame_is_user_send(body)
    if _copilot_frame_is_user_send(body):
        return True
    if is_batchexecute_chat_submit(path, body):
        return True
    if is_event_send_chat_submit(path, body):
        return True
    if not _is_confident_chat_send(path, body, raw_bytes or b""):
        return False
    data = None
    stripped = body.lstrip()
    if stripped[:1] in "{[":
        try:
            data = json.loads(body)
        except Exception:
            data = None
    if isinstance(data, dict) and _body_has_user_send_payload(data):
        return True
    if isinstance(data, list) and any(
        isinstance(item, dict) and _body_has_user_send_payload(item) for item in data
    ):
        return True
    # File-only Send (no caption): user role + file pointer on a conversation path.
    if chat_carries_attachment(body) or messages_parts_carries_file(body):
        low = body.lower()
        userish = bool(re.search(
            r'"author"\s*:\s*\{[^}]{0,240}"role"\s*:\s*"user"'
            r'|"role"\s*:\s*"user"'
            r'|"author"\s*:\s*"user"'
            r'|"messageinput"'
            r'|"chatid"'
            r'|"chat_id"'
            r'|"conversation_id"'
            r'|"conversationid"'
            r'|"session_id"'
            r'|"sessionid"'
            r'|"inputs"'
            r'|"model"'
            r'|"parent_message_uuid"'
            r'|"parentmessageuuid"'
            r'|"rendering_mode"'
            r'|"prompt"\s*:\s*"',
            low,
        ))
        if userish and (
            _is_messages_conversation_path(path_l)
            or _path_has_chat_marker(path_l)
            or is_chat_path(path, host, body)
        ):
            return True
    return False


def wait_bind_real_upload_names(
    cached_list: list[dict],
    raw_text: str,
    domain: str,
    *,
    retries: int = 8,
    delay: float = 0.20,
) -> list[dict]:
    """On Send, wait briefly for create-file {id, filename} then stamp nameless caches."""
    if not cached_list:
        return cached_list
    try:
        ingest_upload_filenames_from_body(raw_text or "", domain or "")
    except Exception:
        pass

    def _named(entry: dict) -> bool:
        cur = (entry.get("file_name") or "").strip()
        return _is_real_user_upload_name(cur) and not _needs_real_upload_filename(cur)

    def _stamp(entry: dict, got: str) -> None:
        raw = entry.get("raw_bytes") or b""
        if not isinstance(raw, (bytes, bytearray)):
            raw = b""
        new_name = _ensure_name_has_extension(got, bytes(raw), entry.get("content_type") or "")
        old = (entry.get("file_name") or "").strip() or "attachment"
        entry["file_name"] = new_name
        fid = (entry.get("file_id") or "").strip()
        if fid:
            remember_upload_filename(fid, new_name)
        if raw:
            remember_upload_name_by_bytes(bytes(raw), new_name)
        print(f"[Gateway Proxy] FILE NAME bound on Send | {domain} | {old} -> {new_name}")

    def _try_bind() -> None:
        try:
            _bind_real_filenames_to_cached_uploads(cached_list, raw_text or "")
        except Exception:
            pass
        send_names = [
            n for n in extract_all_attachment_filenames_from_send(raw_text or "")
            if _is_real_user_upload_name(n)
        ]
        pending = []
        try:
            pending = list_pending_upload_names_for_domain(domain)
        except Exception:
            pending = []
        unused = [n for n in (send_names + pending) if _is_real_user_upload_name(n)]
        used = {(e.get("file_name") or "").strip().lower() for e in cached_list if _named(e)}
        leftover = [n for n in unused if n.lower() not in used]
        li = 0
        for entry in cached_list:
            if _named(entry):
                continue
            fid = (entry.get("file_id") or "").strip()
            got = lookup_upload_filename(fid) if fid else ""
            if not got:
                raw = entry.get("raw_bytes") or b""
                if isinstance(raw, (bytes, bytearray)) and raw:
                    got = lookup_upload_name_by_bytes(bytes(raw))
            if not got and li < len(leftover):
                got = leftover[li]
                li += 1
            if not got:
                got = peek_pending_upload_name_for_domain(domain)
            if got and _is_real_user_upload_name(got):
                _stamp(entry, got)

    _try_bind()
    if all(_named(e) for e in cached_list):
        return cached_list
    # Send body already names the files (Claude/Gemini JSON) — one short retry only.
    # Long waits made Prompt Logs feel stuck even when extract already finished.
    send_has_names = any(
        _is_real_user_upload_name(n)
        for n in extract_all_attachment_filenames_from_send(raw_text or "")
    )
    max_retries = 2 if send_has_names else max(1, int(retries))
    sleep_s = 0.08 if send_has_names else max(0.05, float(delay))
    for _ in range(max_retries):
        time.sleep(sleep_s)
        try:
            ingest_upload_filenames_from_body(raw_text or "", domain or "")
        except Exception:
            pass
        _try_bind()
        if all(_named(e) for e in cached_list):
            break
    return cached_list


def cache_upload_file(
    domain: str,
    *,
    file_name: str,
    raw_bytes: bytes,
    content_type: str,
    upload_reason: str,
    rule_hit: bool = False,
    rule_name: str = "",
    rule_action: str = "",
    file_id: str = "",
) -> None:
    """Remember file at upload-time; enforcement/log waits for chat Send."""
    try:
        handshake_text = (raw_bytes or b"").decode("utf-8", errors="ignore")
    except Exception:
        handshake_text = ""
    if remember_file_create_handshake(handshake_text, domain, raw_bytes or b"", ""):
        return
    payload, ctype, name = extract_upload_file_payload(raw_bytes or b"", content_type, file_name)
    # Keep body if extract fails (View after block).
    # Skip tiny JSON metadata handshakes (file_id create) that are not the document.
    # Claude/Gemini/etc base64 JSON is extracted above and cached as real bytes.
    fallback = raw_bytes or b""
    if not payload and fallback.lstrip()[:1] in (b"{", b"["):
        j_bytes, j_ct, j_name = _extract_bytes_from_json_upload(fallback, file_name)
        if j_bytes:
            payload, ctype, name = j_bytes, j_ct or content_type, j_name or name
        else:
            return
    stored = payload if payload else fallback
    if not stored:
        return
    if len(stored) > 20 * 1024 * 1024:
        stored = stored[: 20 * 1024 * 1024]
    final_ct = ctype or content_type or "application/octet-stream"
    final_name = (name or file_name or "").strip() or "attachment"
    # Prefer remembered real name (ChatGPT often uploads bytes with only file_id).
    if file_id:
        remembered = lookup_upload_filename(file_id)
        if remembered:
            final_name = remembered
    if _is_real_user_upload_name(file_name or ""):
        final_name = _ensure_name_has_extension((file_name or "").strip(), stored, final_ct)
        if file_id:
            remember_upload_filename(file_id, final_name)
    elif _is_real_user_upload_name(name or ""):
        final_name = _ensure_name_has_extension((name or "").strip(), stored, final_ct)
        if file_id:
            remember_upload_filename(file_id, final_name)
    # Fallback: pending → hash → PDF title. Keep extensionless Gemini names.
    # Pending MUST win over by_bytes: ChatGPT multi-file create-file queues one
    # real name per upload; by_bytes reuse was stealing the first PDF's name for all.
    # Only CONSUME pending when bytes look like a real file (tiny probes must not steal names).
    if _is_fake_upload_name(final_name) or not _is_real_user_upload_name(final_name):
        sniffed = _sniff_upload_content_type(stored, final_name, final_ct)
        final_ct = sniffed or final_ct
        pending = ""
        if _worth_binding_upload_bytes(stored):
            pending = take_pending_upload_name_for_domain(domain)
            if pending and _upload_name_already_cached(domain, pending):
                pending = ""
        if pending and _worth_binding_upload_bytes(stored):
            final_name = _ensure_name_has_extension(pending, stored, final_ct)
            if file_id:
                remember_upload_filename(file_id, final_name)
        else:
            by_bytes = lookup_upload_name_by_bytes(stored)
            if by_bytes:
                final_name = by_bytes
            else:
                pdf_name = _name_from_pdf_metadata(stored)
                if pdf_name:
                    final_name = pdf_name
                else:
                    final_name = _default_name_from_bytes(stored, final_ct) or "attachment"
    elif "." not in final_name.rsplit("/", 1)[-1]:
        final_name = _ensure_name_has_extension(final_name, stored, final_ct)
        if _is_real_user_upload_name(final_name):
            remember_upload_name_by_bytes(stored, final_name)
            if file_id:
                remember_upload_filename(file_id, final_name)
            # Never re-queue a name after the file is cached — that let File A
            # steal File B's Prompt Log label.
    entry = {
        "ts": time.time(),
        "domain": domain,
        "file_name": final_name,
        "content_type": final_ct,
        "raw_bytes": stored,
        "upload_reason": upload_reason or "",
        "rule_hit": bool(rule_hit),
        "rule_name": rule_name or "",
        "rule_action": (rule_action or "").upper(),
        "file_id": file_id or "",
    }
    fname_key = (entry["file_name"] or "attachment").lower()
    entry["cache_uid"] = f"{entry['ts']:.6f}|{fname_key}|{len(stored)}"
    keys: list[str] = []
    aliases = upload_domain_aliases(domain)
    for alias in aliases:
        if file_id:
            keys.append(f"{alias}|id|{file_id}")
        keys.append(f"{alias}|name|{fname_key}")
        keys.append(f"{alias}|latest")
    with _UPLOAD_FILE_CACHE_LOCK:
        _purge_upload_file_cache()
        for k in keys:
            _UPLOAD_FILE_CACHE[k] = entry
        for alias in aliases:
            qkey = _upload_queue_key(alias)
            q = _UPLOAD_FILE_QUEUES.setdefault(qkey, [])
            q.append(entry)
            if len(q) > _UPLOAD_FILE_QUEUE_MAX:
                _UPLOAD_FILE_QUEUES[qkey] = q[-_UPLOAD_FILE_QUEUE_MAX:]
    print(
        f"[Gateway Proxy] FILE CACHED (await Send) | {domain} | {entry['file_name']} | "
        f"{len(entry['raw_bytes'])} bytes | ct={final_ct} | reason={(upload_reason or '')[:60]} | "
        f"magic={bytes(entry['raw_bytes'][:6])!r} | evidence={_upload_evidence(entry)} | "
        f"rule_hit={rule_hit} | aliases={len(keys)}"
    )
    if _is_real_user_upload_name(entry["file_name"]):
        # Queue after the cache row exists so a second file cannot take this name.
        remember_pending_upload_name_for_domain(domain, entry["file_name"])


def _extract_file_ids_from_chat(raw_text: str) -> list[str]:
    ids: list[str] = []
    if not raw_text:
        return ids
    # 1. String arrays: "files": ["uuid-1", "uuid-2"], "file_uuids": [...]
    for m in re.finditer(
        r'"(?:files|file_uuids|fileuuids|file_ids|fileids|attachment_ids|attachmentids|document_ids|documentids|documents|uploads)"\s*:\s*\[([\s\S]{0,10000}?)\]',
        raw_text,
        re.I,
    ):
        chunk = m.group(1) or ""
        for sm in re.finditer(r'["\']([a-zA-Z0-9_\-\.]{1,128})["\']', chunk):
            val = sm.group(1).strip()
            if not _POSITIONAL_NAME_RE.match(val):
                ids.append(val)
    for pat in (
        r'"file_id"\s*:\s*"([^"]+)"',
        r'"fileId"\s*:\s*"([^"]+)"',
        r'"file_uuid"\s*:\s*"([^"]+)"',
        r'"fileUuid"\s*:\s*"([^"]+)"',
        r'"docId"\s*:\s*"([^"]+)"',
        r'"doc_id"\s*:\s*"([^"]+)"',
        r'"document_id"\s*:\s*"([^"]+)"',
        r'"documentId"\s*:\s*"([^"]+)"',
        r'"document_url"\s*:\s*"([^"]+)"',
        r'"documentUrl"\s*:\s*"([^"]+)"',
        r'"file_url"\s*:\s*"([^"]+)"',
        r'"fileUrl"\s*:\s*"([^"]+)"',
        r'"attachment_id"\s*:\s*"([^"]+)"',
        r'"attachmentId"\s*:\s*"([^"]+)"',
        r'"attachmentIds"\s*:\s*\[\s*"([^"]+)"',
        r'"upload_id"\s*:\s*"([^"]+)"',
        r'"uploadId"\s*:\s*"([^"]+)"',
        r'"blob_id"\s*:\s*"([^"]+)"',
        r'"blobId"\s*:\s*"([^"]+)"',
        r'"asset_id"\s*:\s*"([^"]+)"',
        r'"assetId"\s*:\s*"([^"]+)"',
        r'"media_id"\s*:\s*"([^"]+)"',
        r'"mediaId"\s*:\s*"([^"]+)"',
        r'"source_id"\s*:\s*"([^"]+)"',
        r'"sourceId"\s*:\s*"([^"]+)"',
        r'"storage_id"\s*:\s*"([^"]+)"',
        r'"storageId"\s*:\s*"([^"]+)"',
        r'/c/api/attachments/([^"?\s]+)',
        r'file-service://file-([a-zA-Z0-9_-]+)',
        r'asset_pointer"\s*:\s*"[^"]*file-([a-zA-Z0-9_-]+)',
        r'"id"\s*:\s*"(file-[a-zA-Z0-9_-]+)"',
    ):
        for m in re.finditer(pat, raw_text):
            ids.append(m.group(1))
    out: list[str] = []
    seen: set[str] = set()
    for i in ids:
        if i not in seen:
            seen.add(i)
            out.append(i)
    return out


def _expected_send_attachment_count(raw_text: str) -> int:
    """How many distinct files this Send references (ChatGPT/Gemini/Claude multi-upload)."""
    body = raw_text or ""
    if not body:
        return 0
    ids = _extract_file_ids_from_chat(body)
    names = [
        n for n in extract_all_attachment_filenames_from_send(body)
        if _is_real_user_upload_name(n)
    ]
    # Non-empty attachments / files / documents object or string arrays
    n_objs = 0
    for m in re.finditer(
        r'"(?:attachments|files|documents|uploadedFiles|uploaded_files|media|fileattachments|imageattachments|file_list|filelist|assets|sources|docs|content)"\s*:\s*\[(.*?)\]',
        body,
        re.I | re.S,
    ):
        chunk = m.group(1) or ""
        if not chunk.strip():
            continue
        n_braces = len(re.findall(r'\{', chunk))
        n_strings = len(re.findall(r'["\'][a-zA-Z0-9_\-\.]{1,}["\']', chunk)) if not n_braces else 0
        n_objs = max(n_objs, n_braces, n_strings)
    n_content_blocks = len(re.findall(r'"type"\s*:\s*"(?:document|image|file|audio)"', body, re.I))
    return max(len(ids), len(names), n_objs, n_content_blocks, 0)


def chat_carries_attachment(raw_text: str) -> bool:
    """True only when THIS chat Send actually references a real attached file.

    Claude/Copilot text-only bodies often contain keys like "document", "attachments":[]
    or "media_type" — those must NOT count as an upload (that caused Prompt Log =
    "[FILE UPLOAD] attachment" for normal typed prompts).
    """
    if not raw_text:
        return False
    low = raw_text.lower()

    # WebRTC SDP is not a file attachment
    if 'name="sdp"' in low or "webrtc-datachannel" in low or "\nv=0\r\n" in low or "/realtime" in low:
        return False

    # Empty arrays / nulls are not attachments
    for empty_arr in (
        '"attachments"', '"files"', '"documents"', '"fileattachments"', '"imageattachments"',
        '"file_list"', '"filelist"', '"uploadedfiles"', '"uploaded_files"', '"assets"',
        '"sources"', '"media"', '"docs"',
    ):
        if re.search(rf'{empty_arr}\s*:\s*\[\s*\]', low):
            low = re.sub(rf'{empty_arr}\s*:\s*\[\s*\]', " ", low)

    # Strong URI / pointer evidence
    if any(
        x in low
        for x in (
            "file-service://",
            "attachment://",
            "asset_pointer",
            "converted_file",
            "convert_document",
            "/c/api/attachments",
            '"messagetype":"image"',
            "input_file",
            "input_image",
            '"attachmentids"',
            '"referencedattachments"',
            '"hiddenattachments"',
            '"parttype":"file"',
            '"contentorigin":"upload"',
            "document_url",
            "documenturl",
            "file_url",
            "fileurl",
            "download_url",
            "downloadurl",
        )
    ):
        return True

    # In-browser extracted document content (Claude Web)
    if "extracted_content" in low:
        return True

    # Non-empty attachment / files arrays (objects or string IDs)
    if re.search(r'"(?:attachments|files|documents|fileattachments|imageattachments|file_list|filelist|uploadedfiles|uploaded_files|file_uuids|file_ids|attachment_ids|assets|sources|media|docs|messageinput)"\s*:\s*\[\s*(?:\{|["\'][a-zA-Z0-9_\-\.]{4,})', low):
        return True

    # Any confirmed file_ids from payload
    if _extract_file_ids_from_chat(raw_text):
        return True

    # Real IDs with non-empty values (not null / "")
    id_patterns = (
        r'"file_id"\s*:\s*"(?!null)[^"]+"',
        r'"fileid"\s*:\s*"(?!null)[^"]+"',
        r'"file_uuid"\s*:\s*"(?!null)[^"]+"',
        r'"fileuuid"\s*:\s*"(?!null)[^"]+"',
        r'"docid"\s*:\s*"(?!null)[^"]+"',
        r'"doc_id"\s*:\s*"(?!null)[^"]+"',
        r'"document_id"\s*:\s*"(?!null)[^"]+"',
        r'"documentid"\s*:\s*"(?!null)[^"]+"',
        r'"attachment_id"\s*:\s*"(?!null)[^"]+"',
        r'"attachmentid"\s*:\s*"(?!null)[^"]+"',
        r'"file_uri"\s*:\s*"(?!null)[^"]+"',
        r'"fileuri"\s*:\s*"(?!null)[^"]+"',
        r'"image_url"\s*:\s*"(?!null)https?[^"]+"',
        r'"imageurl"\s*:\s*"(?!null)https?[^"]+"',
        r'"document_url"\s*:\s*"(?!null)https?[^"]+"',
        r'"documenturl"\s*:\s*"(?!null)https?[^"]+"',
        r'"file_url"\s*:\s*"(?!null)https?[^"]+"',
        r'"fileurl"\s*:\s*"(?!null)https?[^"]+"',
        r'"upload_id"\s*:\s*"(?!null)[^"]+"',
        r'"uploadid"\s*:\s*"(?!null)[^"]+"',
        r'"blob_id"\s*:\s*"(?!null)[^"]+"',
        r'"blobid"\s*:\s*"(?!null)[^"]+"',
        r'"asset_id"\s*:\s*"(?!null)[^"]+"',
        r'"assetid"\s*:\s*"(?!null)[^"]+"',
        r'"storage_id"\s*:\s*"(?!null)[^"]+"',
        r'"storageid"\s*:\s*"(?!null)[^"]+"',
        r'"source_id"\s*:\s*"(?!null)[^"]+"',
        r'"sourceid"\s*:\s*"(?!null)[^"]+"',
        r'"media_id"\s*:\s*"(?!null)[^"]+"',
        r'"mediaid"\s*:\s*"(?!null)[^"]+"',
    )
    if any(re.search(p, low) for p in id_patterns):
        return True

    # Inline binary / base64 file payloads (Gemini / multimodal)
    if re.search(r'"(?:inline_data|inlinedata|filedata)"\s*:\s*\{', low):
        return True
    if re.search(r'"media_type"\s*:\s*"(?:application/|image/|audio/|video/)', low):
        # Claude document blocks often use media_type + data together
        if '"data"' in low or "base64" in low or "extracted_content" in low:
            return True

    # Filename with common document/image/audio extension in this send
    if re.search(
        r'"(?:file_name|filename|fileName|name|title)"\s*:\s*"[^"]+\.(?:pdf|docx?|xlsx?|pptx?|png|jpe?g|gif|webp|txt|csv|zip|mp3|wav|m4a|aac|ogg|webm|flac|mp4|mov)"',
        low,
    ):
        return True

    # Claude / Mistral / generic content blocks with real payload
    if re.search(r'"type"\s*:\s*"(?:document|image|file|input_image|input_file|audio|input_audio|voice)"', low):
        if any(
            x in low
            for x in (
                '"source"', '"data"', "base64", "file_uuid", "file_id", "fileid",
                "document_id", "documentid", "document_url", "documenturl",
                "file_url", "fileurl", '"url"', '"uri"', "application/pdf",
                "image/png", "image/jpeg", "extracted_content",
                "audio/", "audio/wav", "audio/mpeg", "audio/webm",
            )
        ):
            return True

    # DeepSeek / Perplexity / generic file lists with uuid or url (non-empty)
    if re.search(r'"(?:file_uuid|fileUuid|file_id|fileId|documentId|document_id|docId|doc_id)"\s*:\s*"(?!null)[^"]{4,}"', low):
        return True
    if re.search(r'"files"\s*:\s*\[[\s\S]{0,4000}?"(?:url|uri|path|name)"\s*:\s*"(?!null)[^"]+"', low):
        return True

    # Voice / audio attachment markers. Flag keys count only with a real value:
    # ChatGPT sends "dictation": false on every typed message.
    if any(x in low for x in ("audio/webm", "audio/wav", "audio/mpeg", "audio/mp4")):
        return True
    if re.search(
        r'"(?:input_audio|audio_url|voice_mode|voice|speech_to_text|dictation)"\s*:\s*'
        r'(?!\s*(?:false|null|0(?![\d.])|""|\[\s*\]|\{\s*\}))',
        low,
    ):
        return True
    if re.search(r'"(?:transcript|transcription)"\s*:\s*"(?!null)[^"]{2,}"', low):
        # Transcript alone is enough to treat as voice content for rule scan on Send
        if any(x in low for x in ("audio", "voice", "speech", "dictation", "file_id", "attachment")):
            return True

    # Cloud file URI refs on chat submits (not empty placeholders)
    if re.search(r'"(?:file_data|filedata|file_uri|fileuri)"\s*:\s*\{', low):
        return True
    if re.search(r'https?://[^"\']+/(?:file/|open\?id=)', low):
        if re.search(r'"(?:file_data|filedata|file_uri|fileuri|uri|url)"\s*:', low):
            return True

    # ChatGPT / OpenAI file pointers
    if messages_parts_carries_file(raw_text):
        return True
    if re.search(r'"id"\s*:\s*"file-[a-zA-Z0-9_-]+"', low):
        return True
    if re.search(r'"mime_type"\s*:\s*"(?:application/|image/|audio/|video/)', low):
        if '"attachments"' in low or '"files"' in low or '"content_type"' in low:
            return True

    # Copilot / Sydney image or file payloads (base64 in send frame)
    if event_send_carries_binary_attach(raw_text):
        return True

    if extract_all_attachment_filenames_from_send(raw_text):
        return True

    return False


def event_send_carries_binary_attach(raw_text: str) -> bool:
    """Copilot/Edge image or file sends often embed base64 instead of file_id."""
    if not raw_text:
        return False
    low = raw_text.lower()
    if any(
        x in low
        for x in (
            '"messagetype":"image"',
            '"messagetype": "image"',
            '"inputimage"',
            '"input_image"',
            '"imageurl"',
            '"image_url"',
            '"binarydata"',
            '"binary_data"',
        )
    ):
        return True
    if re.search(r'"(?:data|image|bytes|content)"\s*:\s*"[A-Za-z0-9+/=\s]{500,}"', raw_text):
        return True
    if re.search(r'"type"\s*:\s*"(?:image|input_image|input_file|file|document)"', low):
        if re.search(r'"(?:data|source|url)"\s*:\s*', low):
            return True
    return False


def extract_attachment_filename_from_send(raw_text: str) -> str:
    """Best-effort filename from a chat Send JSON body."""
    names = extract_all_attachment_filenames_from_send(raw_text or "")
    return names[0] if names else ""


def extract_inline_attachment_bytes(raw_text: str) -> tuple[bytes, str, str]:
    """Pull the first inline base64 file/image from a chat Send body."""
    all_inlines = extract_all_inline_attachment_bytes(raw_text)
    if not all_inlines:
        return b"", "", ""
    return all_inlines[0]


def extract_all_inline_attachment_bytes(raw_text: str) -> list[tuple[bytes, str, str]]:
    """Pull ALL inline base64 file/image payloads (Gemini multi-image Send / Claude attachments)."""
    if not raw_text:
        return []
    import base64

    out: list[tuple[bytes, str, str]] = []
    seen: set[str] = set()
    fname_base = extract_attachment_filename_from_send(raw_text) or "attachment"

    # 1. Claude in-browser extracted_content inside attachments array:
    # attachments: [{"file_name": "x.csv", "extracted_content": "...", "file_type": "text/csv"}]
    for m in re.finditer(
        r'\{[^{}]*?"(?:extracted_content|extractedContent)"\s*:\s*"((?:[^"\\]|\\.)+)"[^{}]*?\}',
        raw_text,
    ):
        block = m.group(0)
        try:
            content_str = json.loads(f'"{m.group(1)}"')
        except Exception:
            content_str = m.group(1).replace("\\n", "\n").replace('\\"', '"')
        if not content_str or len(content_str.strip()) < 2:
            continue
        nm = re.search(r'"(?:file_name|fileName|filename|name|title)"\s*:\s*"([^"]+)"', block)
        fname = _sanitize_upload_filename(nm.group(1).strip()) if nm else ""
        if not fname or not _is_real_user_upload_name(fname):
            fname = f"document_{len(out) + 1}.txt"
        tm = re.search(r'"(?:file_type|fileType|content_type|contentType|mime_type|media_type)"\s*:\s*"([^"]+)"', block)
        ctype = tm.group(1).strip() if tm else "text/plain"
        doc_bytes = content_str.encode("utf-8")
        sig = f"{fname.lower()}|{len(doc_bytes)}"
        if sig not in seen:
            seen.add(sig)
            out.append((doc_bytes, ctype, fname))

    # 2. Claude / Anthropic source base64 blocks:
    # {"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": "..."}, "title": "doc.pdf"}
    for m in re.finditer(
        r'\{[^{}]*?"source"\s*:\s*\{[^}]*?"(?:data|bytes)"\s*:\s*"([A-Za-z0-9+/=\s\\]{60,})"[^}]*?\}[^{}]*?\}',
        raw_text,
    ):
        block = m.group(0)
        blob = m.group(1).replace("\\n", "").replace("\\r", "").replace(" ", "")
        try:
            data = base64.b64decode(blob, validate=False)
        except Exception:
            continue
        if len(data) < 32:
            continue
        mm = re.search(r'"(?:media_type|mime_type|mediaType)"\s*:\s*"([^"]+)"', block)
        mime = mm.group(1).strip() if mm else ""
        tm = re.search(r'"(?:title|name|file_name|filename)"\s*:\s*"([^"]+)"', block)
        fname = _sanitize_upload_filename(tm.group(1).strip()) if tm else ""
        if not mime:
            if data[:5] == b"%PDF-": mime = "application/pdf"
            elif data[:8] == b"\x89PNG\r\n\x1a\n": mime = "image/png"
            elif data[:3] == b"\xff\xd8\xff": mime = "image/jpeg"
            else: mime = "application/octet-stream"
        ext = {"application/pdf": "pdf", "image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}.get(mime, "bin")
        if not fname or not _is_real_user_upload_name(fname):
            is_img = mime.startswith("image/")
            fname = f"image_{len(out)+1}.{ext}" if is_img else f"document_{len(out)+1}.{ext}"
        sig = f"{fname.lower()}|{len(data)}"
        if sig not in seen:
            seen.add(sig)
            out.append((data, mime, fname))

    # Prefer structured inline_data / fileData blocks, then generic large base64 fields.
    patterns = (
        r'"(?:inline_data|inlineData|fileData|file_data)"\s*:\s*\{[^}]{0,400}?"(?:mime_type|mimeType|media_type)"\s*:\s*"([^"]+)"[^}]{0,400}?"(?:data|bytes)"\s*:\s*"([A-Za-z0-9+/=\s\\]{80,})"',
        r'"(?:data|bytes|content|image|binary|base64)"\s*:\s*"([A-Za-z0-9+/=\s\\]{200,})"',
    )
    for pi, pat in enumerate(patterns):
        for mi, m in enumerate(re.finditer(pat, raw_text, re.I | re.DOTALL)):
            if pi == 0:
                mime = (m.group(1) or "").strip()
                blob = (m.group(2) or "")
            else:
                mime = ""
                blob = (m.group(1) or "")
                m_mime = re.search(
                    r'"(?:mime_type|mimeType|media_type|content_type)"\s*:\s*"([^"]+)"',
                    raw_text[max(0, m.start() - 180): m.start() + 40],
                    re.I,
                )
                if m_mime:
                    mime = (m_mime.group(1) or "").strip()
            blob = blob.replace("\\n", "").replace("\\r", "").replace(" ", "")
            if len(blob) < 80:
                continue
            # Dedup identical payloads (same image referenced twice).
            sig = blob[:64] + f"|{len(blob)}"
            if sig in seen:
                continue
            try:
                data = base64.b64decode(blob, validate=False)
            except Exception:
                continue
            if len(data) < 32:
                continue
            seen.add(sig)
            if not mime:
                if data[:5] == b"%PDF-":
                    mime = "application/pdf"
                elif data[:2] == b"PK":
                    mime = "application/vnd.openxmlformats-officedocument"
                elif data[:3] == b"\xff\xd8\xff":
                    mime = "image/jpeg"
                elif data[:8] == b"\x89PNG\r\n\x1a\n":
                    mime = "image/png"
                elif data[:4] == b"RIFF" and data[8:12] == b"WEBP":
                    mime = "image/webp"
                else:
                    mime = "application/octet-stream"
            ext = {
                "image/jpeg": "jpg",
                "image/png": "png",
                "image/webp": "webp",
                "image/gif": "gif",
                "application/pdf": "pdf",
            }.get(mime.lower().split(";")[0].strip(), "bin")
            fname = fname_base if len(out) == 0 and fname_base not in ("attachment", "file", "") else f"image_{len(out) + 1}.{ext}"
            if len(out) == 0 and "." not in fname_base and fname_base not in ("attachment", "file", ""):
                fname = f"{fname_base}.{ext}"
            elif len(out) == 0 and fname_base in ("attachment", "file", ""):
                fname = f"image_1.{ext}"
            out.append((data[:20 * 1024 * 1024], mime, fname))
            if len(out) >= _UPLOAD_FILE_QUEUE_MAX:
                return out
        if out and pi == 0:
            # Structured blocks found — don't also re-scan generic fields (duplicates).
            return out
    if not out:
        # Protobuf / Connect-RPC payloads (Claude Web): document body text embedded in protobuf
        try:
            names = [
                n for n in extract_all_attachment_filenames_from_send(raw_text)
                if _is_real_user_upload_name(n)
            ]
            raw_b = raw_text.encode("latin-1", errors="ignore") if isinstance(raw_text, str) else raw_text
            if len(raw_b) >= 8:
                pb_strs = extract_protobuf_strings(raw_b)
                if not pb_strs and isinstance(raw_text, str):
                    pb_strs = extract_protobuf_strings(raw_text.encode("utf-8", errors="surrogateescape"))
                docs = []
                for s in pb_strs:
                    s_clean = (s or "").strip()
                    if _looks_like_document_body_dump(s_clean):
                        docs.append(s_clean)
                for i, s_clean in enumerate(docs):
                    suffix = f" {i + 1}" if len(docs) > 1 else ""
                    one_name = names[i] if i < len(names) else f"Document{suffix}"
                    doc_bytes = s_clean.encode("utf-8")
                    ext = one_name.rsplit(".", 1)[-1].lower() if "." in one_name else "txt"
                    ct = "text/plain" if ext in ("txt", "md", "csv", "json", "py") else "application/octet-stream"
                    out.append((doc_bytes, ct, one_name))
        except Exception:
            pass
    return out


def _domain_has_pending_upload_cache(domain: str) -> bool:
    """True when a recent upload is cached for this Target Website family (peek only)."""
    aliases = upload_domain_aliases(domain)
    if not aliases:
        d = _normalize_domain(domain or "")
        aliases = [d] if d else []
    now = time.time()
    with _UPLOAD_FILE_CACHE_LOCK:
        _purge_upload_file_cache()
        for alias in aliases:
            latest = _UPLOAD_FILE_CACHE.get(f"{alias}|latest")
            if latest and (now - float(latest.get("ts") or 0)) <= _UPLOAD_LATEST_MATCH_TTL:
                return True
            for entry in _UPLOAD_FILE_QUEUES.get(_upload_queue_key(alias), []) or []:
                if (now - float(entry.get("ts") or 0)) <= _UPLOAD_LATEST_MATCH_TTL:
                    return True
    return False


def _host_from_url_header(raw: str) -> str:
    s = (raw or "").strip()
    if not s:
        return ""
    try:
        if "://" not in s:
            s = "https://" + s
        return _normalize_domain(urllib.parse.urlparse(s).hostname or "")
    except Exception:
        return ""


_DOMAIN_SET_MEMO: dict = {"key": None, "set": frozenset()}


def _monitored_domain_set():
    """Set view of _cached_domains so suffix lookups stay O(1) with 10k+ targets."""
    src = _cached_domains
    if isinstance(src, (set, frozenset, dict)):
        return src
    key = (id(src), len(src))
    if _DOMAIN_SET_MEMO["key"] != key:
        _DOMAIN_SET_MEMO["set"] = frozenset(src)
        _DOMAIN_SET_MEMO["key"] = key
    return _DOMAIN_SET_MEMO["set"]


def _resolve_upload_bind_domain(flow: http.HTTPFlow, upload_host: str) -> str:
    """Map a file-CDN / non-chat host upload to an admin Target Website.

    No product hostname hardcoding — uses Referer/Origin (chat page) or parent
    Target Website that covers this host as a subdomain. Falls back to sticky
    last-monitored domain for this client (CDN uploads often omit Referer).
    """
    # Prefer the page the employee is chatting on.
    for hdr in ("Referer", "Origin", "referer", "origin"):
        ref_host = _host_from_url_header(flow.request.headers.get(hdr, "") or "")
        if not ref_host:
            continue
        ok, domain, _plat = detect_target(ref_host)
        if ok and domain:
            try:
                remember_client_target_domain(get_client_ip(flow), domain)
            except Exception:
                pass
            return domain

    # Upload host itself might be a monitored target or child of one.
    ok, domain, _plat = detect_target(upload_host)
    if ok and domain:
        try:
            remember_client_target_domain(get_client_ip(flow), domain)
        except Exception:
            pass
        return domain

    # Child of a monitored parent (admin added parent only).
    get_target_domains()
    h = _normalize_domain(upload_host or "")
    best = ""
    if h:
        dom_set = _monitored_domain_set()
        labels = h.split(".")
        for i in range(len(labels)):
            cand = ".".join(labels[i:])
            if cand in dom_set:
                best = cand
                break
    if best:
        try:
            remember_client_target_domain(get_client_ip(flow), best)
        except Exception:
            pass
        return best

    # Sticky: last admin Target Website this client used (any added domain).
    try:
        sticky = lookup_client_target_domain(get_client_ip(flow))
        if sticky:
            return sticky
    except Exception:
        pass
    return ""


def remember_client_target_domain(client_ip: str, domain: str) -> None:
    """Remember which Target Website this client is actively using (for CDN bind).

    Endpoint Guard (laptop) peers are almost always 127.0.0.1 / ::1 — must keep
    sticky so Claude/Gemini CDN uploads without Referer still bind to the chat
    Target Website the employee just used.
    """
    ip = (client_ip or "").strip()
    d = _normalize_domain(domain or "")
    if not ip or not d or ip in ("-", "unknown"):
        return
    with _CLIENT_TARGET_STICKY_LOCK:
        _CLIENT_TARGET_STICKY[ip] = (d, time.time())
        if len(_CLIENT_TARGET_STICKY) > 2000:
            now = time.time()
            dead = [k for k, (_dd, ts) in _CLIENT_TARGET_STICKY.items() if now - ts > _CLIENT_TARGET_STICKY_TTL]
            for k in dead:
                _CLIENT_TARGET_STICKY.pop(k, None)


def lookup_client_target_domain(client_ip: str) -> str:
    """Return sticky Target Website for this client, or ''."""
    ip = (client_ip or "").strip()
    if not ip:
        return ""
    with _CLIENT_TARGET_STICKY_LOCK:
        item = _CLIENT_TARGET_STICKY.get(ip)
        if not item:
            return ""
        domain, ts = item
        if time.time() - float(ts) > _CLIENT_TARGET_STICKY_TTL:
            _CLIENT_TARGET_STICKY.pop(ip, None)
            return ""
        return domain or ""


_FILE_SEND_BLOCK_LOCK = threading.Lock()
_FILE_SEND_BLOCKS: dict[str, dict] = {}
_FILE_SEND_BLOCK_TTL = 90.0


def remember_file_send_block(
    domain: str,
    *,
    names: list[str] | tuple[str, ...] = (),
    ids: list[str] | tuple[str, ...] = (),
    message: str = "",
) -> None:
    """Remember a file-rule BLOCK so a second Copilot socket cannot retry the same Send."""
    d = (domain or "").strip().lower()
    if not d:
        return
    with _FILE_SEND_BLOCK_LOCK:
        prev = _FILE_SEND_BLOCKS.get(d) or {}
        nset = set(prev.get("names") or [])
        iset = set(prev.get("ids") or [])
        for n in names:
            nn = (n or "").strip().lower()
            if nn and not _is_fake_upload_name(nn):
                nset.add(nn)
        for i in ids:
            ii = str(i or "").strip().lower()
            if ii:
                iset.add(ii)
        _FILE_SEND_BLOCKS[d] = {
            "ts": time.time(),
            "names": nset,
            "ids": iset,
            "msg": (message or prev.get("msg") or "").strip(),
        }


def file_send_block_matches(domain: str, raw_text: str) -> str:
    """Block message if this Send retries files that just failed a Guard rule."""
    d = (domain or "").strip().lower()
    if not d:
        return ""
    now = time.time()
    with _FILE_SEND_BLOCK_LOCK:
        rec = _FILE_SEND_BLOCKS.get(d)
        if not rec:
            return ""
        if now - float(rec.get("ts") or 0) > _FILE_SEND_BLOCK_TTL:
            _FILE_SEND_BLOCKS.pop(d, None)
            return ""
        body = (raw_text or "").lower()
        if not body:
            return ""
        for fid in rec.get("ids") or []:
            if fid and fid in body:
                return rec.get("msg") or "This request was blocked by Gateway Guard."
        for name in rec.get("names") or []:
            if name and len(name) >= 4 and name in body:
                return rec.get("msg") or "This request was blocked by Gateway Guard."
    return ""


def _file_policy_applies_on_send(
    path: str,
    raw_text: str,
    raw_bytes: bytes = b"",
    *,
    domain: str = "",
    host: str = "",
) -> bool:
    """File extract + Guard Rules on finished chat Send — ANY admin-monitored domain.

    Not ChatGPT-only: known platform shapes OR attachment markers OR pending upload
    cache for this Target Website family.
    """
    path_l = (path or "").lower().split("?", 1)[0]
    if _path_has_ignore_pattern(path_l) or is_noise(path_l, raw_text or ""):
        return False
    if "/realtime" in path_l:
        return False
    if _is_gateway_inject_frame(raw_text or ""):
        return False
    if _is_typing_or_draft_path(path_l, raw_text or ""):
        return False
    # Upload URLs are not chat Sends — skip file policy.
    if _path_looks_like_upload(path_l):
        return False

    body = raw_text or ""
    has_file = (
        _send_carries_attachment(body)
        or messages_parts_carries_file(body)
        or chat_carries_attachment(body)
        or event_send_carries_binary_attach(body)
    )
    confident = _is_confident_chat_send(path, raw_text, raw_bytes) or _copilot_frame_is_user_send(body)
    chatish = (
        confident
        or is_chat_path(path, host, body)
        or _path_has_chat_marker(path)
    )
    # Long-lived Copilot WS URL is always /c/api/chat — path chatish is not a Send.
    if _is_persistent_chat_websocket(path) and not confident:
        chatish = False

    # Pure file-API picks (/files, /upload, …) wait for a later chat Send.
    # Exception: custom AIs that POST file+prompt on the same upload URL.
    if _path_looks_like_upload(path) and not (confident or (has_file and chatish)):
        return False

    finished = _is_finished_user_file_send(path, raw_text, raw_bytes, host)

    # Attachment markers → scan only on a finished Send. Attach/create-file wait.
    if has_file and chatish:
        return finished

    # Pending upload cache: ONLY on a finished user Send — never on intermediate
    # ChatGPT JSON calls or telemetry (those used to consume cache as "attachment" and log a
    # phantom Blocked row 1–2s before the real named Send).
    if domain and _domain_has_pending_upload_cache(domain):
        if not chatish:
            return False
        if finished:
            return True
        peek = ""
        try:
            peek = extract_prompt_universal(
                (raw_text or "").encode("utf-8", errors="ignore") if isinstance(raw_text, str) else (raw_bytes or b""),
                "",
                host or "",
                "",
            ) or ""
        except Exception:
            peek = ""
        has_user_text = bool(peek and looks_like_user_prompt(peek))
        if confident and has_user_text and not _is_persistent_chat_websocket(path):
            return True
        return False
    return False


def take_all_cached_uploads_for_send(
    domain: str,
    raw_text: str = "",
    *,
    allow_latest: bool = False,
) -> list[dict]:
    """Pop all matching cached uploads for this chat Send (one log row per file on Send)."""
    aliases = upload_domain_aliases(domain)
    found: list[dict] = []
    seen_uids: set[str] = set()

    def add_entry(entry: dict) -> None:
        uid = entry.get("cache_uid") or str(id(entry))
        if uid in seen_uids:
            return
        # Prefer registry / pending name over cached "attachment" placeholder.
        # Do NOT peek the same latest pending onto every row (multi-file bug).
        cur = (entry.get("file_name") or "").strip()
        fid = (entry.get("file_id") or "").strip()
        if _needs_real_upload_filename(cur):
            remembered = lookup_upload_filename(fid) if fid else ""
            if not remembered:
                raw = entry.get("raw_bytes") or b""
                if isinstance(raw, (bytes, bytearray)) and raw:
                    remembered = lookup_upload_name_by_bytes(bytes(raw))
            if remembered:
                entry["file_name"] = remembered
        seen_uids.add(uid)
        found.append(entry)

    with _UPLOAD_FILE_CACHE_LOCK:
        _purge_upload_file_cache()
        for fid in _extract_file_ids_from_chat(raw_text):
            for alias in aliases:
                for key in (f"{alias}|id|{fid}", f"{alias}|id|file-{fid}"):
                    if key in _UPLOAD_FILE_CACHE:
                        entry = _UPLOAD_FILE_CACHE.pop(key)
                        add_entry(entry)
                        _remove_cache_keys_for_entry(entry, aliases)
        for m in re.finditer(
            r'["\'](?:file_name|fileName|filename|name|title)["\']\s*:\s*["\']([^"\']+)["\']',
            raw_text or "",
        ):
            name = m.group(1).strip()
            name_l = name.lower()
            if (
                not name_l
                or name_l in ("attachment", "file", "document", "untitled")
                or _is_fake_upload_name(name)
                or not _is_real_user_upload_name(name)
            ):
                continue
            for alias in aliases:
                key = f"{alias}|name|{name_l}"
                if key in _UPLOAD_FILE_CACHE:
                    entry = _UPLOAD_FILE_CACHE.pop(key)
                    add_entry(entry)
                    _remove_cache_keys_for_entry(entry, aliases)
        if allow_latest:
            now = time.time()
            for alias in aliases:
                qkey = _upload_queue_key(alias)
                for entry in list(_UPLOAD_FILE_QUEUES.get(qkey, [])):
                    age = now - float(entry.get("ts") or 0)
                    if age <= _UPLOAD_LATEST_MATCH_TTL:
                        add_entry(entry)
                        _remove_cache_keys_for_entry(entry, aliases)
                if qkey in _UPLOAD_FILE_QUEUES:
                    _UPLOAD_FILE_QUEUES[qkey] = [
                        e for e in _UPLOAD_FILE_QUEUES[qkey]
                        if e.get("cache_uid") not in seen_uids
                    ]
                latest = _UPLOAD_FILE_CACHE.get(f"{alias}|latest")
                if latest and (now - float(latest.get("ts") or 0)) <= _UPLOAD_LATEST_MATCH_TTL:
                    add_entry(latest)
                    _remove_cache_keys_for_entry(latest, aliases)
    return _dedupe_cached_uploads_by_bytes(found)


def take_recent_confident_caches_for_send(domain: str) -> list[dict]:
    """Bind recent real uploads on Send when attachment markers are weak."""
    aliases = upload_domain_aliases(domain)
    now = time.time()
    out: list[dict] = []
    seen: set[str] = set()
    with _UPLOAD_FILE_CACHE_LOCK:
        _purge_upload_file_cache()
        for alias in aliases:
            candidates: list[dict] = list(_UPLOAD_FILE_QUEUES.get(_upload_queue_key(alias), []))
            latest = _UPLOAD_FILE_CACHE.get(f"{alias}|latest")
            if latest:
                candidates.append(latest)
            for entry in candidates:
                uid = entry.get("cache_uid") or str(id(entry))
                if uid in seen:
                    continue
                age = now - float(entry.get("ts") or 0)
                if age > _UPLOAD_LATEST_MATCH_TTL:
                    continue
                name = (entry.get("file_name") or "").strip()
                raw = entry.get("raw_bytes") or b""
                # ChatGPT often caches as "attachment" — still bind real bytes on Send.
                if not isinstance(raw, (bytes, bytearray)) or len(raw) < 32:
                    if not name or _is_fake_upload_name(name):
                        continue
                if not is_confident_file_upload(
                    fname=name if not _is_fake_upload_name(name) else "file.bin",
                    content_type=(entry.get("content_type") or ""),
                    raw_bytes=raw if isinstance(raw, (bytes, bytearray)) else b"",
                    raw_text="",
                    upload_reason=(entry.get("upload_reason") or ""),
                    host=alias,
                    path="/files",
                ):
                    continue
                seen.add(uid)
                out.append(entry)
                _remove_cache_keys_for_entry(entry, aliases)
    return _dedupe_cached_uploads_by_bytes(out)


def _scan_upload_for_rules(
    raw_bytes: bytes,
    content_type: str,
    raw_text: str,
    file_label: str,
    cached: dict | None = None,
    *,
    platform: str = "",
    domain: str = "",
    client_ip: str = "",
    url: str = "",
    method: str = "",
    skip_backend: bool = False,
    extra_context: str = "",
) -> tuple[str, bool, str, str, str, list[str], bool, str]:
    """Scan file bytes on Send only — extract by type, then apply Guard Rules (+ AI bot if configured).
    Returns (..., scan_evaluated, scan_eval_error).
    skip_backend=True: extract + local regex only (multi-file combined eval).
    extra_context: caption/other text merged into local regex + backend evaluate."""
    scanned = ""
    rule_hit = False
    rule_name = ""
    rule_action = ""
    excerpt = ""
    upload_images: list[str] = []
    scan_evaluated = False
    scan_eval_error = ""
    try:
        if raw_bytes:
            try:
                scanned = extract_upload_text_for_rules(raw_bytes, content_type, raw_text or "", file_label) or ""
            except Exception as e:
                print(f"[Gateway Proxy] extract_upload_text_for_rules failed (allowed): {e}")
                scanned = ""
        try:
            upload_images = _upload_images_for_vision(raw_bytes or b"", content_type, file_label) if raw_bytes else []
        except Exception as e:
            print(f"[Gateway Proxy] upload vision images failed (allowed): {e}")
            upload_images = []
        eval_blob = "\n\n".join(
            x for x in ((extra_context or "").strip(), (scanned or "").strip()) if x
        ).strip()
        if eval_blob:
            excerpt = re.sub(r"\s+", " ", eval_blob).strip()[:180]
            try:
                rule_hit, rule_name, rule_action = match_guard_rules_on_text(eval_blob)
            except Exception as e:
                print(f"[Gateway Proxy] local file regex failed (allowed): {e}")
                rule_hit, rule_name, rule_action = False, "", ""
            rule_action = (rule_action or "").upper()
            if rule_action == "ALERT":
                rule_action = "WARN"

        # Check rule match on filename itself (e.g. passwords*.txt, sensitive.pdf, secret_*.docx)
        for fn_candidate in (file_label, (cached or {}).get("file_name", "")):
            fn_cand = (fn_candidate or "").strip()
            if not fn_cand or _is_fake_upload_name(fn_cand):
                continue
            try:
                fn_hit, fn_name, fn_action = match_guard_rules_on_text(fn_cand)
                if fn_hit:
                    fn_action = (fn_action or "").upper()
                    if fn_action == "ALERT":
                        fn_action = "WARN"
                    if fn_action == "BLOCK" or not rule_hit:
                        rule_hit, rule_name, rule_action = True, fn_name, fn_action
                    if rule_action == "BLOCK":
                        break
            except Exception as e:
                pass

        if not excerpt and file_label and not _is_fake_upload_name(file_label):
            excerpt = file_label.strip()[:180]
        # Backend regex/bot on extract or vision images.
        has_regex = bool(get_guard_rules())
        plat = (platform or domain or "Browser AI").strip()
        dom = (domain or "").strip()
        run_backend = bool(
            (not skip_backend)
            and dom
            and (eval_blob or upload_images)
            and (has_ai_bot_rules() or has_regex)
            and not (rule_hit and rule_action == "BLOCK")
        )
        # Local BLOCK already decided — no backend call needed (gated above via run_backend).
        if rule_hit and rule_action == "BLOCK":
            scan_evaluated = True
        elif eval_blob and has_regex and not run_backend:
            # Regex-only rules already applied locally — treat as evaluated (no HTTP round-trip).
            scan_evaluated = True
        if run_backend:
            try:
                _eval_text = (eval_blob or scanned or "")[:50_000]
                allowed, rt, action, _, _, eval_err = send_to_backend(
                    plat, dom, _eval_text, client_ip, url, method or "POST",
                    upload_images=upload_images,
                    evaluation_only=True,
                    extracted_text=_eval_text,
                )
                is_rule_backend = (not allowed) or (action or "").upper() in ("BLOCK", "BLOCKED", "REDACT", "REDACTED", "WARN", "WARNED")
                if is_rule_backend or not eval_err:
                    scan_evaluated = True
                    rule_hit, rule_name, rule_action = _merge_file_scan_backend(
                        rule_hit, rule_name, rule_action, allowed, rt or "AI Guard Bot Policy", action or ("Blocked" if not allowed else "Allowed"),
                    )
                else:
                    scan_eval_error = str(eval_err).strip()
                    scan_evaluated = False
                    print(f"[Gateway Proxy] AI bot file scan eval_error (will re-check on log): {scan_eval_error}")
            except Exception as e:
                scan_evaluated = False
                scan_eval_error = str(e).strip()[:300] or "backend file scan failed"
                print(f"[Gateway Proxy] AI bot file scan failed (allowed): {e}")
    except Exception as e:
        print(f"[Gateway Proxy] file rule scan failed (allowed): {e}")
        # Keep any partial extract/local regex already computed — do not wipe on late errors.
        if not scanned:
            rule_hit = False
            rule_name = ""
            rule_action = ""
            excerpt = ""
            upload_images = []
            scan_evaluated = False
            scan_eval_error = ""
    return scanned, rule_hit, rule_name, rule_action, excerpt, upload_images, scan_evaluated, scan_eval_error