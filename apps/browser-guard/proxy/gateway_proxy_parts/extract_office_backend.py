# Part of Gateway browser_ai_proxy — do not import directly.


def _run_async(coro):
    """Run an async coroutine from synchronous context (even within mitmproxy loop)."""
    import asyncio
    import threading

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


async def _windows_ocr_pil(img) -> str:
    """Windows.Media.Ocr (built into Windows 10+) — no Tesseract install required."""
    from winrt.windows.globalization import Language
    from winrt.windows.graphics.imaging import BitmapPixelFormat, SoftwareBitmap
    from winrt.windows.media.ocr import OcrEngine
    from winrt.windows.storage.streams import DataWriter

    if img.mode != "RGBA":
        img = img.convert("RGBA")
    # Cap size for OCR latency / memory
    max_side = 2000
    w, h = img.size
    if w < 8 or h < 8:
        return ""
    if max(w, h) > max_side:
        scale = max_side / float(max(w, h))
        img = img.resize((max(8, int(w * scale)), max(8, int(h * scale))))

    engine = OcrEngine.try_create_from_user_profile_languages()
    if engine is None:
        for tag in ("en", "en-US", "en-GB"):
            try:
                if OcrEngine.is_language_supported(Language(tag)):
                    engine = OcrEngine.try_create_from_language(Language(tag))
                    if engine is not None:
                        break
            except Exception:
                continue
    if engine is None:
        return ""

    writer = DataWriter()
    writer.write_bytes(bytearray(img.tobytes()))
    bitmap = SoftwareBitmap.create_copy_from_buffer(
        writer.detach_buffer(),
        BitmapPixelFormat.RGBA8,
        img.width,
        img.height,
    )
    result = await engine.recognize_async(bitmap)
    text = (getattr(result, "text", None) or "").strip()
    return text


def _extract_image_windows_ocr(data: bytes) -> str:
    """OCR via Windows.Media.Ocr (built into Windows 10+)."""
    if not data or len(data) < 32:
        return ""
    if len(data) > 15 * 1024 * 1024:
        data = data[: 15 * 1024 * 1024]
    try:
        from PIL import Image
    except Exception:
        return ""
    try:
        img = Image.open(io.BytesIO(data))
        img.load()
        if getattr(img, "n_frames", 1) > 1:
            img.seek(0)
        img = img.convert("RGB")
    except Exception:
        return ""
    try:
        text = _run_async(_windows_ocr_pil(img)) or ""
        return str(text).strip()[:200_000]
    except Exception as e:
        print(f"[Gateway Proxy] Windows OCR unavailable: {e}")
        return ""


def _extract_image_tesseract(data: bytes) -> str:
    """OCR via pytesseract when installed on the laptop."""
    if not data or len(data) < 32:
        return ""
    try:
        from PIL import Image
        import pytesseract  # type: ignore
    except Exception:
        return ""
    try:
        img = Image.open(io.BytesIO(data))
        img.load()
        if getattr(img, "n_frames", 1) > 1:
            img.seek(0)
        img = img.convert("RGB")
        text = (pytesseract.image_to_string(img) or "").strip()
        return text[:200_000]
    except Exception:
        return ""


def _looks_like_video(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    """True for video uploads (mp4/mov/webm/mkv/avi) — scan via audio-track STT when possible."""
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if ct.startswith("video/"):
        return True
    if ct.startswith("audio/"):
        return False
    if fn.endswith((".mp4", ".mov", ".mkv", ".avi", ".m4v", ".wmv", ".mpeg", ".mpg")):
        return True
    if not data or len(data) < 12:
        return fn.endswith(".webm")
    head = data[:32]
    if head[4:8] == b"ftyp":
        brand = head[8:12]
        # ISO BMFF video brands (m4a still handled as audio later).
        if brand in (b"isom", b"iso2", b"mp41", b"mp42", b"avc1", b"dash", b"M4V ", b"qt  "):
            return True
    if head[:4] == b"RIFF" and len(data) >= 12 and data[8:12] == b"AVI ":
        return True
    if head[:4] == b"\x1aE\xdf\xa3":
        # Matroska/WebM: video only when a video track codec is present (mic recordings are audio-only).
        return bool(re.search(rb"V_(?:VP[89]|AV1|MPEG|MS/|THEORA|UNCOMPRESSED)", data[:65536]))
    return False


def _looks_like_audio(data: bytes, content_type: str = "", file_name: str = "") -> bool:
    """True for voice notes / audio file uploads (wav/mp3/m4a/ogg/webm/aac/flac)."""
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    if ct.startswith("video/") or fn.endswith((".mp4", ".mov", ".mkv", ".avi", ".m4v", ".wmv")):
        return False
    if ct.startswith("audio/") or "audio" in ct.split(";")[0]:
        return True
    if fn.endswith((".wav", ".mp3", ".m4a", ".aac", ".ogg", ".oga", ".opus", ".webm", ".flac", ".wma")):
        return True
    if not data or len(data) < 12:
        return False
    head = data[:16]
    if head[:4] == b"RIFF" and data[8:12] == b"WAVE":
        return True
    if head[:3] == b"ID3" or head[:2] == b"\xff\xfb" or head[:2] == b"\xff\xf3":
        return True  # mp3
    if head[:4] == b"fLaC" or head[:4] == b"OggS":
        return True
    if head[:4] == b"\x1a\x45\xdf\xa3":  # EBML (WebM Opus browser voice recording)
        return True
    if head[4:8] == b"ftyp":  # m4a/mp4 container
        return True
    return False


def _extract_transcript_fields_from_json(raw_text: str) -> str:
    """Many AI sites STT locally and send transcript JSON with the voice blob."""
    if not raw_text or len(raw_text) < 8:
        return ""
    low = raw_text.lower()
    if not any(
        k in low
        for k in (
            "transcript", "transcription", "spoken", "voice", "dictation",
            "asr", "speech_to_text", "speechtext", "audio_transcript", "audio_text",
        )
    ):
        return ""
    try:
        data = json.loads(raw_text)
    except Exception:
        # Regex fallback for nested / escaped transcripts
        parts = []
        for pat in (
            r'"(?:transcript|transcription|spoken_text|speech_text|dictation|asr_text|audio_transcript|audio_text|voice_text)"\s*:\s*"((?:\\.|[^"\\])*)"',
            r'"(?:transcript|transcription)"\s*:\s*"((?:\\.|[^"\\])*)"',
        ):
            for m in re.finditer(pat, raw_text, re.I):
                try:
                    val = json.loads(f'"{m.group(1)}"')
                except Exception:
                    val = m.group(1).replace("\\n", "\n").replace('\\"', '"')
                val = (val or "").strip()
                if val and looks_like_user_prompt(val):
                    parts.append(val)
        return "\n".join(parts)[:200_000]

    found: list[str] = []

    def walk(obj, depth=0):
        if depth > 8 or len(found) >= 5:
            return
        if isinstance(obj, dict):
            for k, v in obj.items():
                kl = str(k).lower()
                if kl in (
                    "transcript", "transcription", "spoken_text", "speech_text",
                    "dictation", "asr_text", "voice_text", "recognized_text",
                    "audio_transcript", "spoken", "audio_text", "spoken_query", "voice_prompt",
                ) and isinstance(v, str) and v.strip():
                    if looks_like_user_prompt(v.strip()):
                        found.append(v.strip())
                else:
                    walk(v, depth + 1)
        elif isinstance(obj, list):
            for item in obj[:40]:
                walk(item, depth + 1)

    walk(data)
    return "\n".join(found)[:200_000]


def _audio_to_wav_path(data: bytes, content_type: str = "", file_name: str = "") -> str | None:
    """Write audio to a temp path; convert to WAV when possible for Windows STT."""
    import tempfile
    import os

    if not data or len(data) < 64:
        return None
    if len(data) > 25 * 1024 * 1024:
        data = data[: 25 * 1024 * 1024]

    fn = (file_name or "").lower()
    ct = (content_type or "").lower()
    suffix = ".bin"
    for ext in (".wav", ".mp3", ".m4a", ".ogg", ".webm", ".flac", ".aac", ".wma", ".opus"):
        if fn.endswith(ext) or ext.replace(".", "") in ct:
            suffix = ext
            break
    if data[:4] == b"RIFF" and data[8:12] == b"WAVE":
        suffix = ".wav"

    fd, path = tempfile.mkstemp(prefix="gateway_voice_", suffix=suffix)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)

    if path.lower().endswith(".wav"):
        return path

    # Prefer ffmpeg on PATH (common on IT images) to produce PCM WAV for System.Speech
    wav_path = path + ".wav"
    try:
        import shutil
        ffmpeg = shutil.which("ffmpeg")
        if ffmpeg:
            import subprocess
            proc = subprocess.run(
                [ffmpeg, "-y", "-i", path, "-ac", "1", "-ar", "16000", wav_path],
                capture_output=True,
                timeout=15,
            )
            if proc.returncode == 0 and os.path.isfile(wav_path) and os.path.getsize(wav_path) > 64:
                try:
                    os.remove(path)
                except Exception:
                    pass
                return wav_path
    except Exception as e:
        print(f"[Gateway Proxy] ffmpeg voice convert skipped: {e}")

    # Optional pydub (if installed + ffmpeg)
    try:
        from pydub import AudioSegment  # type: ignore
        fmt = suffix.lstrip(".")
        if fmt == "mp3":
            seg = AudioSegment.from_mp3(path)
        elif fmt in ("ogg", "oga", "opus"):
            seg = AudioSegment.from_ogg(path)
        elif fmt == "wav":
            seg = AudioSegment.from_wav(path)
        else:
            seg = AudioSegment.from_file(path)
        seg = seg.set_channels(1).set_frame_rate(16000)
        seg.export(wav_path, format="wav")
        if os.path.isfile(wav_path) and os.path.getsize(wav_path) > 64:
            try:
                os.remove(path)
            except Exception:
                pass
            return wav_path
    except Exception:
        pass

    return path  # may still be useful for whisper


def _windows_system_speech_stt(wav_path: str) -> str:
    """Offline STT via Windows System.Speech (dictation grammar) for WAV files."""
    import subprocess
    import tempfile
    import os

    if not wav_path or not os.path.isfile(wav_path):
        return ""
    # Escape for PowerShell single-quoted string
    safe = wav_path.replace("'", "''")
    ps = f"""
Add-Type -AssemblyName System.Speech
$engine = New-Object System.Speech.Recognition.SpeechRecognitionEngine
try {{
  $engine.SetInputToWaveFile('{safe}')
  $engine.LoadGrammar((New-Object System.Speech.Recognition.DictationGrammar))
  $result = $engine.Recognize()
  if ($result -ne $null) {{ $result.Text }}
}} finally {{
  $engine.Dispose()
}}
"""
    try:
        proc = subprocess.run(
            ["powershell", "-NoProfile", "-NonInteractive", "-Command", ps],
            capture_output=True,
            timeout=18,
            text=True,
            encoding="utf-8",
            errors="ignore",
        )
        text = (proc.stdout or "").strip()
        if text and looks_like_user_prompt(text):
            return text[:200_000]
    except Exception as e:
        print(f"[Gateway Proxy] Windows System.Speech STT failed: {e}")
    return ""


def _whisper_stt(path: str) -> str:
    """Optional local Whisper (openai-whisper or faster-whisper) if installed on the machine."""
    if not path:
        return ""
    # faster-whisper
    try:
        from faster_whisper import WhisperModel  # type: ignore

        model = WhisperModel("tiny", device="cpu", compute_type="int8")
        segments, _info = model.transcribe(path, beam_size=1)
        parts = [s.text.strip() for s in segments if getattr(s, "text", None)]
        text = " ".join(parts).strip()
        if text:
            return text[:200_000]
    except Exception:
        pass
    # openai-whisper
    try:
        import whisper  # type: ignore

        model = whisper.load_model("tiny")
        result = model.transcribe(path)
        text = (result.get("text") or "").strip()
        if text:
            return text[:200_000]
    except Exception:
        pass
    # speech_recognition + whisper backend
    try:
        import speech_recognition as sr  # type: ignore

        r = sr.Recognizer()
        with sr.AudioFile(path) as source:
            audio = r.record(source)
        if hasattr(r, "recognize_whisper"):
            text = (r.recognize_whisper(audio, model="tiny") or "").strip()
            if text:
                return text[:200_000]
    except Exception:
        pass
    return ""


def _extract_audio_text(data: bytes, content_type: str = "", file_name: str = "") -> str:
    """
    Speech-to-text for voice / audio uploads so Guard Rules can scan spoken content.
    Order: site-provided transcript fields are handled separately; here we STT the bytes.
    1) Windows System.Speech on WAV (built-in)
    2) Optional Whisper / faster-whisper if installed
    """
    import os

    if not data or len(data) < 64:
        return ""
    path = None
    try:
        path = _audio_to_wav_path(data, content_type, file_name)
        if not path:
            return ""
        # Prefer WAV for System.Speech
        wav = path if path.lower().endswith(".wav") else None
        if wav:
            text = _windows_system_speech_stt(wav)
            if text:
                print(f"[Gateway Proxy] Voice STT (Windows) | {len(text)} chars | {file_name or 'audio'}")
                return text
        text = _whisper_stt(path)
        if text:
            print(f"[Gateway Proxy] Voice STT (Whisper) | {len(text)} chars | {file_name or 'audio'}")
            return text
        if wav is None and path.lower().endswith((".wav",)):
            text = _windows_system_speech_stt(path)
            if text:
                return text
    except Exception as e:
        print(f"[Gateway Proxy] Voice STT error: {e}")
    finally:
        if path:
            try:
                os.remove(path)
            except Exception:
                pass
            try:
                if path.endswith(".wav") is False and os.path.isfile(path + ".wav"):
                    os.remove(path + ".wav")
            except Exception:
                pass
    return ""


def _extract_plain_text_bytes(data: bytes) -> str:
    if not data:
        return ""
    # Skip obvious binary without text
    if data.startswith(b"\x89PNG") or data.startswith(b"\xff\xd8\xff") or data.startswith(b"GIF8"):
        return ""
    if data[:2] == b"PK" or data[:5] == b"%PDF-":
        return ""
    if data[:8] == b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1":
        return ""
    sample = data[: 4 * 1024 * 1024]
    text = ""
    try:
        if sample.startswith(b"\xff\xfe") or sample.startswith(b"\xfe\xff"):
            text = sample.decode("utf-16", errors="ignore")
        elif len(sample) >= 4 and sample[1:2] == b"\x00" and sample[3:4] == b"\x00":
            text = sample.decode("utf-16-le", errors="ignore")
        else:
            text = sample.decode("utf-8")
    except Exception:
        try:
            text = sample.decode("latin-1", errors="ignore")
        except Exception:
            return ""
    # Heuristic: enough printable ratio (sample 4KB for ultra-fast check on large files)
    if not text or not text.strip():
        return ""
    sample_chk = text[:4096]
    printable = sum(1 for ch in sample_chk if ch.isprintable() or ch in "\r\n\t")
    total_len = max(1, len(sample_chk))
    if len(sample_chk) >= 8 and (printable / total_len) < 0.65:
        return ""
    if len(sample_chk) < 8 and (printable / total_len) < 0.8:
        return ""
    return text[:200_000]


def _extract_binary_strings(data: bytes, min_len: int = 4, max_chars: int = 200_000) -> str:
    """Extract printable ASCII / UTF-8 string runs from binary files (models, binaries, databases).

    Enables Guard Rules to inspect strings, keys, secrets, numbers, and tokens inside binary files.
    """
    if not data:
        return ""
    runs = re.findall(rb"[\x20-\x7E\t\r\n]{%d,}" % min_len, data[: 8 * 1024 * 1024])
    if not runs:
        return ""
    extracted: list[str] = []
    total = 0
    for r in runs:
        s = r.decode("latin-1", errors="ignore").strip()
        if s and len(s) >= min_len:
            extracted.append(s)
            total += len(s) + 1
            if total >= max_chars:
                break
    return "\n".join(extracted)[:max_chars]


_PLAIN_TEXT_EXTS = frozenset({
    # 1. Documents & Office text
    ".txt", ".text", ".md", ".markdown", ".mdown", ".tex", ".latex", ".rtf",
    ".csv", ".tsv", ".fb2",
    # 2. Programming and Source Code
    ".py", ".pyw", ".pyi", ".pyx", ".js", ".mjs", ".cjs",
    ".ts", ".tsx", ".mts", ".cts", ".html", ".htm", ".css", ".sass", ".scss",
    ".less", ".java", ".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx",
    ".cs", ".csx", ".fs", ".fsi", ".fsx", ".go", ".rs", ".rlib", ".php",
    ".phtml", ".phar", ".rb", ".rake", ".gemspec", ".swift", ".kt", ".kts",
    ".dart", ".r", ".rmd", ".m", ".mlx", ".jl", ".scala", ".sc",
    ".pl", ".pm", ".t", ".lua", ".sh", ".bash", ".zsh", ".fish", ".bat",
    ".cmd", ".ps1", ".psm1", ".psd1", ".asm", ".s", ".f", ".for", ".f90",
    ".f95", ".cbl", ".cob", ".cpy", ".mm", ".ex", ".exs", ".erl", ".hrl",
    ".hs", ".lhs", ".clj", ".cljs", ".cljc", ".edn", ".lisp", ".lsp", ".cl",
    ".pro", ".sol", ".sql", ".graphql", ".gql", ".proto", ".wat", ".ipynb",
    # 3. Web Development, Configuration and Project Files
    ".json", ".jsonl", ".ndjson", ".xml", ".yaml", ".yml", ".toml", ".ini",
    ".conf", ".config", ".cfg", ".env", ".properties", ".editorconfig", ".map",
    ".vue", ".svelte", ".aspx", ".ascx", ".cshtml", ".razor", ".manifest",
    ".webmanifest", ".tf", ".tfvars", ".tfstate", ".j2", ".jinja", ".jinja2",
    ".cmake", ".gradle", ".pom", ".sln", ".slnx", ".csproj", ".vcxproj",
    ".fsproj", ".lock", ".diff", ".patch", ".log",
    # 4. Vector Graphics text
    ".svg",
    # 6. Subtitles & Captions
    ".srt", ".vtt", ".ass", ".ssa", ".ttml", ".dfxp",
    # 8. Database scripts & exports
    ".sql", ".dump",
    # 9. AI config, prompt templates & tokenizers
    ".prompt", ".jinja", ".jinja2", ".j2", ".dvc",
    # 11. Security, Certificates & Cryptographic text
    ".pem", ".crt", ".cer", ".cert", ".csr", ".key", ".pub", ".asc", ".sig",
    ".sha256", ".sha512", ".md5", ".sha1", ".spdx",
    # 12. Email, Messaging & Contacts
    ".eml", ".vcf", ".ics", ".ical",
    # 14. CAD, 3D Models & Hardware text
    ".obj", ".gcode", ".nc", ".tap", ".scad", ".dxf", ".step", ".stp",
    ".iges", ".igs", ".kicad_sch", ".kicad_pcb", ".gbr", ".ger", ".gtl",
    ".gbl", ".brd", ".sch",
    # 15. GIS & Maps text
    ".geojson", ".kml", ".gpx", ".tab", ".mif", ".mid", ".topojson", ".wkt",
    # 16. OS & System text
    ".reg", ".service", ".socket", ".timer", ".cron",
    # 18. Scientific & Research text
    ".bib", ".ris", ".rdf", ".fa", ".fasta", ".fna", ".fq", ".fastq",
    ".sam", ".pdb", ".cif", ".qmd",
    # 19. E-commerce & Business data
    ".edi", ".x12",
    # 20. Shaders
    ".hlsl", ".glsl", ".shader",
    # 22. Localization & Misc text
    ".po", ".pot", ".mo", ".xliff", ".xlf", ".strings", ".stringsdict",
    ".arb", ".rc", ".res",
})

_STANDALONE_FILENAMES = frozenset({
    "dockerfile", ".dockerignore", "makefile", "jenkinsfile", "procfile",
    "gemfile", "rakefile", "justfile", "brewfile", "vagrantfile", "license",
    "license.txt", "licence", "readme", "readme.txt", "readme.md", "changelog",
    "contributing", "codeowners", "notice", ".gitignore", ".gitattributes",
    ".editorconfig", ".env", ".env.local", ".env.production", ".env.development",
    ".npmrc", ".pypirc", ".bashrc", ".zshrc", ".profile", ".nvmrc", ".node-version",
    "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
    "requirements.txt", "pyproject.toml", "setup.py", "setup.cfg", "cargo.toml",
    "cargo.lock", "go.mod", "go.sum", "pom.xml", "cmakelists.txt", "config",
    "authorized_keys", "known_hosts", "id_rsa", "id_ed25519", "robots.txt",
    "sitemap.xml", "manifest.json", "site.webmanifest", "compose.yaml",
    "compose.yml", "docker-compose.yml", "chart.yaml", "values.yaml",
    "mlmodel", "config.json", "tokenizer.json", "tokenizer_config.json",
    "vocab.json", "merges.txt",
})

_BINARY_DATA_EXTS = frozenset({
    ".exe", ".dll", ".sys", ".cpl", ".scr", ".msp", ".msix", ".appx",
    ".appxbundle", ".so", ".dylib", ".kext", ".dex", ".odex", ".vdex", ".elf",
    ".bin", ".hex", ".fw", ".obj", ".o", ".pdb", ".dsym", ".core", ".dmp",
    ".wasm", ".pt", ".pth", ".jit", ".tflite", ".keras", ".weights.h5", ".onnx",
    ".blob", ".engine", ".plan", ".mlmodel", ".mlpackage", ".gguf", ".ggml",
    ".safetensors", ".spm", ".ckpt", ".checkpoint", ".npz", ".npy", ".ubj",
    ".model", ".cbm", ".faiss", ".index", ".ann", ".lance", ".onnx_data",
    ".db", ".sqlite", ".sqlite3", ".dump", ".backup", ".mdb", ".accdb", ".bak",
    ".trn", ".exp", ".dat", ".duckdb", ".ldb", ".sst", ".parquet", ".orc",
    ".avro", ".feather", ".arrow", ".arrows", ".h5", ".hdf5", ".nc", ".pkl",
    ".pickle", ".joblib", ".pb", ".protobuf", ".msgpack", ".mpk", ".bson",
    ".cbor", ".sas7bdat", ".sas7bcat", ".xpt", ".sav", ".zsav", ".por",
    ".dta", ".wal", ".ibd", ".hc", ".p12", ".pfx", ".der", ".jks", ".keystore",
    ".pk8", ".p7b", ".p7c", ".p7s",
})


def _classify_upload_kind(data: bytes, content_type: str = "", file_name: str = "") -> str:
    """Classify upload bytes so we use one extractor per file type (not all at once)."""
    ct = (content_type or "").lower()
    fn = (file_name or "").lower()
    base = fn.replace("\\", "/").rsplit("/", 1)[-1].strip()
    ext = ("." + base.rsplit(".", 1)[-1]) if "." in base else ""

    # Fast check for standalone text config / project files
    if base in _STANDALONE_FILENAMES:
        return "plain"
    if ext in _PLAIN_TEXT_EXTS:
        return "plain"

    if not data:
        if "pdf" in ct or ext == ".pdf":
            return "pdf"
        if "image/" in ct or ext in (".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp"):
            return "image"
        if "audio/" in ct or ext in (".wav", ".mp3", ".m4a", ".ogg", ".webm", ".flac", ".aac"):
            return "audio"
        if "video/" in ct or ext in (".mp4", ".mov", ".avi", ".mkv", ".webm"):
            return "video"
        if "word" in ct or ext in (".docx", ".doc"):
            return "docx"
        if "excel" in ct or "spreadsheet" in ct or ext in (".xlsx", ".xls", ".csv"):
            return "xlsx"
        if "presentation" in ct or "powerpoint" in ct or ext in (".pptx", ".ppt"):
            return "pptx"
        if ext in (".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".gz", ".bz2", ".xz", ".zst"):
            return "tar"
        if ext in (".zip", ".rar", ".7z", ".cab", ".iso", ".dmg", ".apk", ".deb", ".rpm"):
            return "zip"
        if ext in _BINARY_DATA_EXTS:
            return "binary"
        return "unknown"

    if "pdf" in ct or ext == ".pdf" or data[:5] == b"%PDF-" or b"%PDF-" in data[:4096]:
        return "pdf"
    if _looks_like_image(data, content_type, file_name):
        return "image"
    if _looks_like_video(data, content_type, file_name):
        return "video"
    if _looks_like_audio(data, content_type, file_name):
        return "audio"
    if ext == ".rtf" or _looks_like_rtf(data, content_type, file_name):
        return "rtf"
    if ext in (".html", ".htm", ".xhtml") or _looks_like_html(data, content_type, file_name):
        return "html"
    if _looks_like_docx(data, content_type, file_name) or ext in (".docx", ".doc", ".dot", ".dotx", ".dotm"):
        return "docx"
    if _looks_like_xlsx(data, content_type, file_name) or ext in (".xlsx", ".xls", ".xlsm", ".xlsb", ".xlt", ".xltx", ".xltm"):
        return "xlsx"
    if _looks_like_pptx(data, content_type, file_name) or ext in (".pptx", ".ppt", ".pptm", ".pot", ".potx", ".potm", ".key"):
        return "pptx"
    if _looks_like_opendocument(data, content_type, file_name) or ext in (".odt", ".ods", ".odp"):
        return "odf"
    if _looks_like_ole(data, content_type, file_name):
        return "ole"
    if ext in (
        ".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".gz", ".gzip", ".bz2", ".xz", ".zst",
    ) or data[:2] == b"\x1f\x8b" or (len(data) >= 262 and data[257:262] == b"ustar") or data[:3] == b"BZh" or data[:6] == b"\xfd7zXZ\x00":
        return "tar"
    if any(x in ct for x in ("text/", "csv", "json", "xml", "yaml", "javascript", "typescript", "x-python", "x-sh")):
        return "plain"
    if data[:2] == b"PK" or b"PK\x03\x04" in data[:8192]:
        # Unknown OOXML / ODF zip — sniff inner layout
        zdata = _office_zip_bytes(data)
        if zdata:
            try:
                with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
                    names = zf.namelist()
                    if "word/document.xml" in names:
                        return "docx"
                    if any(n.startswith("xl/") for n in names):
                        return "xlsx"
                    if any(n.startswith("ppt/slides/") for n in names):
                        return "pptx"
                    if "content.xml" in names:
                        return "odf"
            except Exception:
                pass
        return "zip"
    if ext in _BINARY_DATA_EXTS:
        return "binary"
    return "unknown"


def _try_file_extract_chain(
    data: bytes,
    content_type: str,
    file_name: str,
    kind: str,
    steps: list[tuple[str, callable]],
) -> str:
    """Run extractors in order; first non-empty result wins. Logs fallback usage."""
    for i, (label, fn) in enumerate(steps):
        try:
            t = (fn() or "").strip()
            if t:
                if i > 0:
                    print(
                        f"[Gateway Proxy] file extract fallback OK ({label}) | "
                        f"{file_name or kind} | {len(t)} chars"
                    )
                return t[:200_000]
        except Exception as e:
            print(f"[Gateway Proxy] file extract try failed ({label}): {e}")
    return ""


def _office_extract_steps(data: bytes, primary: str) -> list[tuple[str, callable]]:
    """Office OOXML: primary type first, then other Office parsers, then generic ZIP."""
    order = ["docx", "xlsx", "pptx"]
    if primary in order:
        order.remove(primary)
        order.insert(0, primary)
    fns = {
        "docx": ("docx-xml", lambda: _extract_docx_text(data)),
        "xlsx": ("xlsx-xml", lambda: _extract_xlsx_text(data)),
        "pptx": ("pptx-xml", lambda: _extract_pptx_text(data)),
    }
    steps = [fns[k] for k in order if k in fns]
    steps.append(("office-zip-all", lambda: _extract_office_text(data, content_type="", file_name="")))
    return steps


def _extract_zip_archive_members_text(
    data: bytes,
    *,
    depth: int = 0,
    max_depth: int = 2,
    max_files: int = 40,
    max_chars: int = 250_000,
) -> str:
    """Unpack a real .zip and extract text from each inner file (PDF/Office/image/plain/nested zip).

    Used so Guard Rules + predict see resumes.zip contents, not only the outer archive name.
    """
    if not data or depth > max_depth:
        return ""
    zdata = data
    if data[:2] != b"PK":
        zdata = _office_zip_bytes(data) or data
    if not zdata or zdata[:2] != b"PK":
        return ""
    chunks: list[str] = []
    total = 0
    n_files = 0
    try:
        with zipfile.ZipFile(io.BytesIO(zdata)) as zf:
            for info in zf.infolist():
                if n_files >= max_files or total >= max_chars:
                    break
                name = (info.filename or "").replace("\\", "/")
                base = name.rsplit("/", 1)[-1]
                low = name.lower()
                if info.is_dir():
                    continue
                if not base or base.startswith("."):
                    continue
                if "__macosx" in low or low.endswith("/.ds_store") or low.endswith(".ds_store"):
                    continue
                if info.file_size <= 0 or info.file_size > 25 * 1024 * 1024:
                    continue
                try:
                    inner = zf.read(info)
                except Exception:
                    continue
                if not inner or len(inner) < 8:
                    continue
                n_files += 1
                try:
                    inner_kind = _classify_upload_kind(inner, "", base)
                except Exception:
                    inner_kind = "unknown"
                # Nested real archives only — OOXML (docx/xlsx/pptx) also starts with PK.
                if inner_kind == "zip" or low.endswith(".zip"):
                    if depth >= max_depth:
                        continue
                    nested = _extract_zip_archive_members_text(
                        inner, depth=depth + 1, max_depth=max_depth,
                        max_files=max(8, max_files - n_files),
                        max_chars=max_chars - total,
                    )
                    if nested:
                        piece = f"[ZIP:{base}]\n{nested}"
                        chunks.append(piece)
                        total += len(piece)
                    continue
                try:
                    t = _extract_text_from_file_bytes(inner, "", base)
                except Exception:
                    t = ""
                if t and t.strip():
                    piece = f"[FILE:{base}]\n{t.strip()}"
                    chunks.append(piece)
                    total += len(piece)
    except Exception as e:
        print(f"[Gateway Proxy] zip member extract failed (allowed): {e}")
        return ""
    if not chunks:
        return ""
    out = "\n\n".join(chunks)
    print(
        f"[Gateway Proxy] ZIP archive extract | members_text={len(chunks)} files~{n_files} | {len(out)} chars"
    )
    return out[:max_chars]


def _extract_tar_archive_members_text(
    data: bytes,
    *,
    depth: int = 0,
    max_depth: int = 2,
    max_files: int = 40,
    max_chars: int = 250_000,
) -> str:
    """Unpack a .tar, .tar.gz, .tgz, .tar.bz2, .tar.xz or standalone .gz and extract text from each member."""
    if not data or depth > max_depth:
        return ""
    chunks: list[str] = []
    total = 0
    n_files = 0
    import tarfile, io
    try:
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:*") as tf:
            for member in tf.getmembers():
                if n_files >= max_files or total >= max_chars:
                    break
                if not member.isfile():
                    continue
                name = (member.name or "").replace("\\", "/")
                base = name.rsplit("/", 1)[-1]
                low = name.lower()
                if not base or base.startswith("."):
                    continue
                if "__macosx" in low or "ds_store" in low:
                    continue
                if member.size <= 0 or member.size > 25 * 1024 * 1024:
                    continue
                try:
                    f = tf.extractfile(member)
                    if not f:
                        continue
                    inner = f.read(25 * 1024 * 1024)
                except Exception:
                    continue
                if not inner or len(inner) < 2:
                    continue
                n_files += 1
                try:
                    inner_kind = _classify_upload_kind(inner, "", base)
                except Exception:
                    inner_kind = "unknown"
                if inner_kind in ("zip", "tar") or low.endswith((".zip", ".tar", ".tar.gz", ".tgz")):
                    if depth < max_depth:
                        nested = ""
                        if inner_kind == "zip" or low.endswith(".zip"):
                            nested = _extract_zip_archive_members_text(inner, depth=depth + 1, max_depth=max_depth)
                        else:
                            nested = _extract_tar_archive_members_text(inner, depth=depth + 1, max_depth=max_depth)
                        if nested:
                            piece = f"[ARCHIVE:{base}]\n{nested}"
                            chunks.append(piece)
                            total += len(piece)
                    continue
                try:
                    t = _extract_text_from_file_bytes(inner, "", base)
                except Exception:
                    t = ""
                if t and t.strip():
                    piece = f"[FILE:{base}]\n{t.strip()}"
                    chunks.append(piece)
                    total += len(piece)
    except Exception as e:
        # If it's a standalone .gz or .bz2 file (not tar archive), decompress directly
        if data.startswith(b"\x1f\x8b"):
            try:
                import gzip
                decomp = gzip.decompress(data)
                t = _extract_text_from_file_bytes(decomp, "", "")
                if t and t.strip():
                    return t[:max_chars]
            except Exception:
                pass
        elif data.startswith(b"BZh"):
            try:
                import bz2
                decomp = bz2.decompress(data)
                t = _extract_text_from_file_bytes(decomp, "", "")
                if t and t.strip():
                    return t[:max_chars]
            except Exception:
                pass
    if not chunks:
        if data.startswith(b"\x1f\x8b"):
            try:
                import gzip
                decomp = gzip.decompress(data)
                t = _extract_text_from_file_bytes(decomp, "", "")
                if t and t.strip():
                    return t[:max_chars]
            except Exception:
                pass
        return ""
    out = "\n\n".join(chunks)
    print(
        f"[Gateway Proxy] TAR archive extract | members_text={len(chunks)} files~{n_files} | {len(out)} chars"
    )
    return out[:max_chars]


def _extract_text_from_file_bytes(data: bytes, content_type: str = "", file_name: str = "") -> str:
    """Extract scannable text — primary method per file type, then fallbacks until one succeeds."""
    if not data:
        return ""
    kind = _classify_upload_kind(data, content_type, file_name)
    ct = content_type
    fn = file_name
    # Shared fallbacks used by several kinds + unknown
    common_fallbacks = [
        ("rtf", lambda: _extract_rtf_text(data)),
        ("html", lambda: _extract_html_text(data)),
        ("odf", lambda: _extract_opendocument_text(data)),
        ("ole-office", lambda: _extract_ole_office_text(data, ct, fn)),
        ("docx-xml", lambda: _extract_docx_text(data)),
        ("xlsx-xml", lambda: _extract_xlsx_text(data)),
        ("pptx-xml", lambda: _extract_pptx_text(data)),
        ("plain-decode", lambda: _extract_plain_text_bytes(data)),
        ("strings-extract", lambda: _extract_binary_strings(data)),
    ]
    try:
        if kind == "pdf":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("pdf-smart", lambda: _extract_pdf_text_smart(data)),
                ("pdf-pypdf", lambda: _extract_pdf_pypdf(data)),
                ("pdf-ocr", lambda: _extract_pdf_ocr(data)),
                ("pdf-regex", lambda: _extract_pdf_regex(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "docx":
            steps = _office_extract_steps(data, "docx")
            steps.append(("ole-office", lambda: _extract_ole_office_text(data, ct, fn)))
            steps.append(("plain-decode", lambda: _extract_plain_text_bytes(data)))
            return _try_file_extract_chain(data, ct, fn, kind, steps)

        if kind == "xlsx":
            steps = _office_extract_steps(data, "xlsx")
            steps.append(("ole-office", lambda: _extract_ole_office_text(data, ct, fn)))
            steps.append(("plain-decode", lambda: _extract_plain_text_bytes(data)))
            return _try_file_extract_chain(data, ct, fn, kind, steps)

        if kind == "pptx":
            steps = _office_extract_steps(data, "pptx")
            steps.append(("ole-office", lambda: _extract_ole_office_text(data, ct, fn)))
            steps.append(("plain-decode", lambda: _extract_plain_text_bytes(data)))
            return _try_file_extract_chain(data, ct, fn, kind, steps)

        if kind == "ole":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("ole-office", lambda: _extract_ole_office_text(data, ct, fn)),
                ("docx-xml", lambda: _extract_docx_text(data)),
                ("xlsx-xml", lambda: _extract_xlsx_text(data)),
                ("pptx-xml", lambda: _extract_pptx_text(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "rtf":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("rtf", lambda: _extract_rtf_text(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "odf":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("odf", lambda: _extract_opendocument_text(data)),
                ("docx-xml", lambda: _extract_docx_text(data)),
                ("xlsx-xml", lambda: _extract_xlsx_text(data)),
                ("pptx-xml", lambda: _extract_pptx_text(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "html":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("html", lambda: _extract_html_text(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "image":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("image-windows-ocr", lambda: _extract_image_windows_ocr(data)),
                ("image-tesseract", lambda: _extract_image_tesseract(data)),
                ("pdf-pypdf", lambda: _extract_pdf_pypdf(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "audio":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("audio-stt", lambda: _extract_audio_text(data, ct, fn)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "video":
            # Pull spoken track via ffmpeg→WAV→STT (same pipeline as voice).
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("video-audio-stt", lambda: _extract_audio_text(data, ct, fn)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        if kind == "plain":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("plain-utf8", lambda: _extract_plain_text_bytes(data)),
                ("strings-extract", lambda: _extract_binary_strings(data)),
                ("html", lambda: _extract_html_text(data)),
                ("rtf", lambda: _extract_rtf_text(data)),
                ("pdf-pypdf", lambda: _extract_pdf_pypdf(data)),
                ("docx-xml", lambda: _extract_docx_text(data)),
            ])

        if kind == "binary":
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("strings-extract", lambda: _extract_binary_strings(data)),
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
                ("pdf-pypdf", lambda: _extract_pdf_pypdf(data)),
            ])

        if kind in ("zip", "tar"):
            # Real archives (resumes.zip, code.tar.gz): unpack members → PDF/Office/image/plain → rules.
            return _try_file_extract_chain(data, ct, fn, kind, [
                ("zip-members", lambda: _extract_zip_archive_members_text(data)),
                ("tar-members", lambda: _extract_tar_archive_members_text(data)),
                ("odf", lambda: _extract_opendocument_text(data)),
            ] + _office_extract_steps(data, "docx") + [
                ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ])

        # Unknown: try every sensible extractor in order (never crash the proxy)
        return _try_file_extract_chain(data, ct, fn, kind or "unknown", [
            ("pdf-smart", lambda: _extract_pdf_text_smart(data)),
            ("pdf-pypdf", lambda: _extract_pdf_pypdf(data)),
            ("pdf-regex", lambda: _extract_pdf_regex(data)),
            ("zip-members", lambda: _extract_zip_archive_members_text(data)),
            ("tar-members", lambda: _extract_tar_archive_members_text(data)),
            ("rtf", lambda: _extract_rtf_text(data)),
            ("html", lambda: _extract_html_text(data)),
            ("odf", lambda: _extract_opendocument_text(data)),
            ("ole-office", lambda: _extract_ole_office_text(data, ct, fn)),
            ("docx-xml", lambda: _extract_docx_text(data)),
            ("xlsx-xml", lambda: _extract_xlsx_text(data)),
            ("pptx-xml", lambda: _extract_pptx_text(data)),
            ("image-windows-ocr", lambda: _extract_image_windows_ocr(data)),
            ("image-tesseract", lambda: _extract_image_tesseract(data)),
            ("audio-stt", lambda: _extract_audio_text(data, ct, fn)),
            ("plain-decode", lambda: _extract_plain_text_bytes(data)),
            ("strings-extract", lambda: _extract_binary_strings(data)),
        ])
    except Exception as e:
        print(f"[Gateway Proxy] file extract ({kind}) failed — allowed: {e}")
        # Last-resort safe extract so regex still has a chance
        try:
            for label, fn_step in common_fallbacks:
                try:
                    t = (fn_step() or "").strip()
                    if t:
                        print(f"[Gateway Proxy] file extract emergency OK ({label}) | {fn or kind}")
                        return t[:200_000]
                except Exception:
                    continue
        except Exception:
            pass
    return ""


def extract_upload_text_for_rules(
    raw_bytes: bytes,
    content_type: str = "",
    raw_text: str = "",
    file_name: str = "",
) -> str:
    """
    Pull text from an upload body so Guard Rules can scan file contents
    (PDF, Word, Excel, PowerPoint, ODF, RTF, HTML, image OCR, voice STT,
    ZIP member files, plain text, multipart).
    Uses one extractor per detected file type with safe fallbacks.
    """
    try:
        parts: list[str] = []
        ct = (content_type or "").lower()
        data = raw_bytes or b""
        fname = (file_name or "").strip()

        # Site often sends STT transcript alongside the voice blob
        transcript = _extract_transcript_fields_from_json(raw_text or "")
        if transcript:
            parts.append(transcript)

        def _tiny_text_ok(blob: bytes, ct_s: str, name_s: str) -> bool:
            if not blob:
                return False
            nl = (name_s or "").lower()
            cl = (ct_s or "").lower()
            if nl.endswith((".txt", ".md", ".csv", ".json", ".xml", ".html", ".log", ".rtf")):
                return True
            if cl.startswith("text/") or "json" in cl or "csv" in cl:
                return True
            if 1 <= len(blob) < 32:
                try:
                    blob.decode("utf-8")
                    return all(b >= 9 or b in (10, 13) for b in blob)
                except Exception:
                    return False
            return False

        # Prefer clean file bytes from multipart / wrappers when present
        payload, sniffed_ct, sniffed_name = extract_upload_file_payload(data, content_type, fname)
        if payload and (len(payload) >= 32 or _tiny_text_ok(payload, sniffed_ct or content_type, sniffed_name or fname)):
            t = _extract_text_from_file_bytes(payload, sniffed_ct or content_type, sniffed_name or fname)
            if t:
                parts.append(t)

        # Direct scan of full body (non-multipart or when payload extract missed)
        if not parts or (transcript and len(parts) == 1):
            t = _extract_text_from_file_bytes(data, content_type, fname)
            if t and t not in parts:
                parts.append(t)

        # Multipart islands: one extractor per part by file type
        if "multipart" in ct or b"filename=" in data[:12000] or b"webkitformboundary" in data[:4000].lower():
            for chunk in re.split(rb"\r\n--[^\r\n]+", data):
                if len(chunk) < 20:
                    continue
                body = chunk
                part_name = ""
                if b"\r\n\r\n" in chunk:
                    header, body = chunk.split(b"\r\n\r\n", 1)
                    try:
                        hdr = header.decode("utf-8", errors="ignore")
                    except Exception:
                        hdr = ""
                    m = re.search(r'filename\*?=(?:UTF-8\'\')?"?([^";\r\n]+)"?', hdr, re.I)
                    if m:
                        part_name = m.group(1).strip()
                t = _extract_text_from_file_bytes(body, content_type, part_name or fname)
                if t and len(t) > 8 and t not in parts:
                    parts.append(t)

        # Plain text bodies only — never treat ChatGPT/API JSON metadata as file content.
        # Tiny .txt files ("Emil id") are often <20 bytes — still scan them.
        if raw_text and not parts:
            stripped = raw_text.lstrip()
            tiny_name = (fname or "").lower().endswith((".txt", ".md", ".csv", ".log"))
            if (
                (len(raw_text) > 20 or tiny_name or len(raw_text.strip()) >= 1)
                and stripped[:1] not in ("{", "[")
                and "filename=" not in raw_text[:2000].lower()
                and raw_text.count("\x00") == 0
            ):
                if len(raw_text) > 20 or tiny_name or (1 <= len(data) < 64):
                    parts.append(raw_text[:200_000])

        # Deduplicate while preserving order
        seen = set()
        out = []
        for p in parts:
            key = p[:200]
            if key in seen:
                continue
            seen.add(key)
            out.append(p)
        return "\n\n".join(out)[:100_000]
    except Exception as e:
        print(f"[Gateway Proxy] extract_upload_text_for_rules failed (allowed): {e}")
        return ""


def match_guard_rules_on_text(text: str) -> tuple[bool, str, str]:
    """
    Apply active Guard Rules to arbitrary text (prompt, file extract, or audio STT).
    Same matching semantics as typed prompts (admin regex only).
    Returns (matched, rule_name, action).
    """
    if not text or len(text.strip()) < 1:
        return False, "", ""
    # Fast path: scan a bounded window (rules rarely need 100k+ of PDF noise)
    scan = text if len(text) <= 80_000 else text[:80_000]
    rules = sorted(
        get_guard_rules(),
        key=lambda r: 0 if (r.get("action") or "BLOCK").upper() in ("BLOCK",) else 1,
    )
    for r in rules:
        try:
            if not rule_matches_prompt(r, scan):
                continue
            return True, r.get("name", "Guard Rule"), (r.get("action") or "BLOCK").upper()
        except Exception:
            continue
    return False, "", ""


def extract_rest_sse_ask_prompt(content: str) -> str | None:
    """Extract user query from Perplexity /rest/sse, /rest/thread, entrypoint JSON or SSE lines."""
    if not content:
        return None
    text = content.strip()

    # SSE stream chunks: data: {"query_str":"..."}
    if "data:" in text and not text.lstrip().startswith("{"):
        candidates: list[str] = []
        for line in text.splitlines():
            line = line.strip()
            if not line.startswith("data:"):
                continue
            chunk = line[5:].strip()
            if not chunk or chunk in ("[DONE]", "done"):
                continue
            if chunk.startswith("{"):
                nested = extract_rest_sse_ask_prompt(chunk)
                if nested:
                    candidates.append(nested)
        picked = _pick_best_user_text(candidates)
        if picked:
            return picked

    if not text.startswith("{"):
        return None
    try:
        data = json.loads(text)
    except Exception:
        return None
    if not isinstance(data, dict):
        return None

    def _pick(val) -> str | None:
        if isinstance(val, (str, int, float)):
            got = _clean_prompt_text(str(val))
            if got:
                return got
        return None

    qs = data.get("query_str")
    if isinstance(qs, str) and qs.strip():
        got = _clean_prompt_text(qs)
        if got:
            return got

    for key in (
        "query", "user_query", "last_query", "search_query", "user_message",
        "message", "text", "input", "follow_up_input", "user_text", "dsl_query",
        "q", "prompt", "utterance",
    ):
        got = _pick(data.get(key))
        if got:
            return got

    params = data.get("params")
    if isinstance(params, dict):
        for key in ("query_str", "dsl_query", "query", "user_query", "message", "text"):
            got = _pick(params.get(key))
            if got:
                return got

    for wrapper in ("data", "payload", "body", "request", "entry"):
        inner = data.get(wrapper)
        if isinstance(inner, dict):
            try:
                nested = extract_rest_sse_ask_prompt(json.dumps(inner))
            except Exception:
                nested = None
            if nested:
                return nested

    msgs = data.get("messages")
    if isinstance(msgs, list):
        for msg in reversed(msgs):
            if not isinstance(msg, dict):
                continue
            for key in ("query_str", "query", "text", "content", "message", "user_message"):
                got = _pick(msg.get(key))
                if got:
                    return got

    return None


def extract_batchexecute_prompt(content: str) -> str:
    """
    Extract ONLY the user-typed prompt from Gemini/Bard requests.

    Real chat submits go to BardFrontendService/StreamGenerate or chat RPCs:
      f.req=[null,"[[\\"what is python\\\\n\\",0,null,...], ...]"]
    Background/telemetry batchexecute RPCs (ESY5D, L5adhe, VxUbXb, aPya6c, etc.) are ignored.
    """
    decoded = urllib.parse.unquote(content)
    req_str = decoded
    if "f.req=" in decoded:
        try:
            parsed = urllib.parse.parse_qs(decoded, keep_blank_values=False)
            req_str = parsed.get("f.req", [""])[0]
        except Exception:
            idx = decoded.find("f.req=")
            req_str = decoded[idx + 6:]
            if "&" in req_str:
                req_str = req_str.split("&", 1)[0]
            req_str = urllib.parse.unquote(req_str)

    if not req_str:
        return ""

    def _normalize_prompt(s: str) -> str:
        """Collapse whitespace and unescape JSON escapes."""
        if not s:
            return ""
        if "\\n" in s or "\\t" in s or '\\"' in s or "\\\\" in s:
            s = (
                s.replace("\\\\", "\0")
                .replace("\\n", "\n")
                .replace("\\t", "\t")
                .replace('\\"', '"')
                .replace("\0", "\\")
            )
        return re.sub(r"\s+", " ", s).strip()

    def _is_valid_user_prompt(s: str) -> bool:
        if not s:
            return False
        s = _normalize_prompt(s)
        if not looks_like_user_prompt(s):
            return False
        if _is_ai_chrome_url(s):
            return False
        # Drop locale crumbs; keep greetings like "hi".
        if s.lower() in BATCHEXECUTE_LOCALE_JUNK or re.fullmatch(r"[a-z]{2}-[A-Za-z]{2,3}", s):
            return False
        if re.fullmatch(r"[a-z]{2}-[A-Z]{2,3}", s):
            return False
        if re.fullmatch(r"en", s, re.IGNORECASE):
            return False
        # Reject dot-tokens like "z.fdeb774424ec3df1"
        if re.match(r"^[a-z]\.[a-f0-9]{8,}", s, re.IGNORECASE):
            return False
        # Hex hashes with letters — not digit-only user input
        if " " not in s and re.fullmatch(r"[0-9a-fA-F]{10,64}", s) and re.search(r"[a-fA-F]", s):
            return False
        # Reject tokens starting with c_, r_, v_, rc_, f_, z_, A0vx, %., or bare _session ids
        if s.startswith(("c_", "r_", "v_", "rc_", "f_", "z_", "req0_", "_", "A0vx", "%.")):
            return False
        if re.search(r":[0-9]{10,16}$", s) and len(s) >= 20:
            return False
        if "[null,[[" in s:
            return False
        low = s.lower()
        if any(bad in low for bad in (
            "bard activity", "generic", "batchexecute", "wrb.fr",
            "assistant.lamda", "bardfrontendservice", "google account",
            "model_metadata", "conversation_turn", "workspace_id", "count=", "&ofs="
        )):
            return False
        return len(s) >= 1

    def _pick_user_prompt(cands: list[str]) -> str:
        good = []
        for raw in cands:
            cand = _normalize_prompt(raw)
            if _is_valid_user_prompt(cand) and not _is_google_wire_blob(cand):
                good.append(cand)
        if not good:
            return ""
        # Prefer real sentences over a leftover token in the same payload.
        good.sort(key=lambda s: (1 if (" " in s or "\n" in s) else 0, len(s)), reverse=True)
        return good[0]

    def _from_stream_inner(inner, trusted: bool = False) -> str:
        """StreamGenerate: typed prompt is ONLY the first [prompt, 0, ...] slot — not locale."""
        if not isinstance(inner, list) or not inner:
            return ""

        cands: list[str] = []
        if len(inner) > 0 and isinstance(inner[0], list):
            first = inner[0]
            if (
                len(first) > 0
                and isinstance(first[0], list)
                and len(first[0]) > 1
                and isinstance(first[0][0], str)
                and first[0][1] == 0
            ):
                cands.append(first[0][0])
            elif len(first) > 1 and isinstance(first[0], str) and first[1] == 0:
                cands.append(first[0])
        if trusted and cands:
            slot = cands[0].strip()
            if slot and _struct_value_ok(slot) and not _is_google_wire_blob(slot):
                return slot
        return _pick_user_prompt(cands)

    try:
        data = json.loads(req_str)
        # 1. StreamGenerate payload: [null, "<json_string>", ...]
        if isinstance(data, list) and len(data) >= 2 and isinstance(data[1], str):
            try:
                inner = json.loads(data[1])
                got = _from_stream_inner(inner, trusted=True)
                if got:
                    return got
            except Exception:
                pass

        # 2. batchexecute: Check recognized chat RPCs first, then fallback to any other item
        if isinstance(data, list):
            items_to_check = []
            for rpc in data:
                item = None
                if isinstance(rpc, list) and rpc and isinstance(rpc[0], list):
                    item = rpc[0]
                elif isinstance(rpc, list) and len(rpc) > 1 and isinstance(rpc[1], str):
                    item = rpc
                if item and len(item) >= 2:
                    items_to_check.append(item)
            # Priority 1: Known Gemini RPC IDs
            for item in items_to_check:
                if str(item[0]) in GEMINI_CHAT_RPCS:
                    payload_str = item[1]
                    if isinstance(payload_str, str) and payload_str not in ("", "[]", "[[]]"):
                        try:
                            payload = json.loads(payload_str)
                            if isinstance(payload, list):
                                got = _from_stream_inner(payload)
                                if got:
                                    return got
                                if len(payload) >= 2 and isinstance(payload[1], str):
                                    try:
                                        sub = json.loads(payload[1])
                                        got = _from_stream_inner(sub)
                                        if got:
                                            return got
                                    except Exception:
                                        pass
                        except Exception:
                            pass
            # Priority 2: Fallback to any other RPC item in data (resilient to Google rotations)
            for item in items_to_check:
                if str(item[0]) not in GEMINI_CHAT_RPCS:
                    payload_str = item[1]
                    if isinstance(payload_str, str) and payload_str not in ("", "[]", "[[]]"):
                        try:
                            payload = json.loads(payload_str)
                            if isinstance(payload, list):
                                got = _from_stream_inner(payload)
                                if got:
                                    return got
                                if len(payload) >= 2 and isinstance(payload[1], str):
                                    try:
                                        sub = json.loads(payload[1])
                                        got = _from_stream_inner(sub)
                                        if got:
                                            return got
                                    except Exception:
                                        pass
                        except Exception:
                            pass
    except Exception:
        pass

    # 3. Slot regex for StreamGenerate-shaped prompts.
    # Google rotates batchexecute RPC ids often — do not require a hard-coded id list.
    for pat in (
        r'\[\s*\[\s*"((?:[^"\\]|\\.)+?)"\s*,\s*0\s*,',
        r'\\"((?:[^"\\]|\\.)+?)\\"\s*,\s*0\s*,',
    ):
        m = re.search(pat, req_str)
        if m:
            cand = _normalize_prompt(m.group(1))
            if _is_valid_user_prompt(cand) and not _is_google_wire_blob(cand):
                return cand

    return ""


def _printable_runs(raw: bytes) -> list[str]:
    """Pull UTF-8 / ASCII strings out of protobuf or mixed ChatGPT bodies."""
    if not raw:
        return []
    text = raw.decode("utf-8", errors="ignore")
    chunks: list[str] = []
    for m in re.finditer(r"[\x20-\x7e\u00a0-\uffff]{3,4000}", text):
        s = (m.group(0) or "").strip()
        if s and len(s) >= 3:
            chunks.append(s)
    return chunks


def extract_messages_parts_prompt(text: str, raw: bytes) -> str | None:
    """ChatGPT web: JSON parts, nested author.content, or protobuf string fields."""
    blob = text or ""
    raw_blob = (raw or b"").decode("utf-8", errors="ignore")
    search_blob = blob if len(blob) >= len(raw_blob) else raw_blob
    if not search_blob and blob:
        search_blob = blob

    # Prefer parts[] / input_text from embedded conversation JSON.
    parts_got = _extract_messages_parts_prompt(search_blob)
    if parts_got and (messages_parts_carries_file(search_blob) or chat_carries_attachment(search_blob)):
        if _looks_like_document_body_dump(parts_got) or len(parts_got) > 320:
            parts_got = None
    if parts_got:
        return parts_got

    candidates: list[str] = []
    if blob.lstrip().startswith(("{", "[")):
        try:
            got = _extract_from_json(json.loads(blob))
            if got and not _is_chat_metadata_token(got):
                candidates.append(got)
        except Exception:
            pass
    for pat in (
        r'"prompt"\s*:\s*"((?:[^"\\]|\\.)*)"',
    ):
        for m in re.finditer(pat, search_blob):
            raw_val = m.group(1) or ""
            if "\\" in raw_val:
                try:
                    import json
                    unescaped = json.loads(f'"{raw_val}"')
                except Exception:
                    unescaped = raw_val.replace('\\n', '\n').replace('\\r', '\r').replace('\\t', '\t').replace('\\"', '"').replace('\\\\', '\\')
            else:
                unescaped = raw_val
            cand = _clean_prompt_text(unescaped)
            if cand and not _is_chat_metadata_token(cand):
                candidates.append(cand)
    for s in _printable_runs(raw or b""):
        if len(s) > 400:
            continue
        if s.startswith("{") or s.startswith("["):
            continue
        if any(x in s.lower() for x in ("text/event-stream", "authorization", "mozilla/")):
            continue
        candidates.append(s)
    return _pick_best_user_text(candidates)


def _extract_prompt_from_multipart(raw: str) -> str | None:
    """Pull user text from multipart form fields (name=prompt/query/...) — not file parts."""
    if not raw or "content-disposition" not in raw.lower():
        return None
    keys = "|".join(re.escape(k) for k in _UNIVERSAL_PROMPT_KEYS)
    pat = (
        rf'Content-Disposition:\s*form-data;\s*name="({keys})"'
        rf'(?![^\r\n]*filename)[^\r\n]*(?:\r\n[^\r\n]+)*\r\n\r\n(.*?)(?:\r\n--|\Z)'
    )
    try:
        for m in re.finditer(pat, raw, re.I | re.S):
            val = (m.group(2) or "").strip()
            if not val or val.startswith("------"):
                continue
            if looks_like_user_prompt(val) and not _is_opaque_wire_blob(val):
                got = _clean_prompt_text(val)
                if got:
                    return got
    except Exception:
        pass
    return None


def extract_prompt(body_bytes: bytes, content_type: str = "", host: str = "") -> str | None:
    """
    Extract ONLY the exact user-typed prompt text from a request body.
    Never returns raw JSON / API metadata / Cloudflare challenge blobs.
    """
    if not body_bytes:
        return None

    try:
        text = body_bytes.decode("utf-8", errors="ignore")
        ct = (content_type or "").lower()
        if not text.strip():
            if _looks_like_messages_parts_body("", body_bytes):
                return extract_messages_parts_prompt(text, body_bytes)
            return None

        # Multipart: extract prompt fields; never treat the raw boundary blob as chat text.
        if "multipart/form-data" in ct or "webkitformboundary" in text[:200].lower() or text.lstrip().startswith("------"):
            return _extract_prompt_from_multipart(text)

        # Body-shape parsers (any admin-monitored domain — no hostname lists).
        if "f.req=" in text or "req0___data__" in text or text.startswith("f.req="):
            batchexecute_prompt = extract_batchexecute_prompt(text)
            if batchexecute_prompt:
                return _clean_prompt_text(batchexecute_prompt)

        if (
            "\x1e" in text
            or is_event_send_chat_submit("", text)
            or '"event":"send"' in text
            or '"event": "send"' in text
            or '"target":"chat"' in text
            or '"target": "chat"' in text
        ):
            event_send_prompt = extract_event_send_prompt(text)
            if event_send_prompt:
                return _clean_prompt_text(event_send_prompt)

        rest_sse_ask_prompt = extract_rest_sse_ask_prompt(text)
        if rest_sse_ask_prompt:
            return rest_sse_ask_prompt

        if _looks_like_messages_parts_body(text, body_bytes):
            messages_parts_got = extract_messages_parts_prompt(text, body_bytes)
            if messages_parts_got:
                return messages_parts_got
            # Fall through — some ChatGPT wires look like conversation JSON but need
            # generic message/content walk (otherwise Claude works and ChatGPT misses).

        # URL-encoded form bodies (Copilot / misc — not Gemini)
        if "application/x-www-form-urlencoded" in ct or (
            "%" in text and not text.lstrip().startswith(("{", "["))
        ) or text.startswith(("count=", "at=", "soc-app=", "req0_", "req1_")):
            try:
                form = urllib.parse.parse_qs(urllib.parse.unquote(text), keep_blank_values=False)
                for key in (
                    "prompt", "query", "query_str", "text", "message",
                    "rawUserQuery", "input", "q", "user_query", "instruction",
                    "inputs", "utterance", "content", "user_input", "question",
                ):
                    vals = form.get(key) or form.get(key.lower())
                    if vals and isinstance(vals[0], str):
                        got = _clean_prompt_text(vals[0])
                        if got:
                            return got
                if form.get("f.req"):
                    batchexecute_prompt = extract_batchexecute_prompt("f.req=" + form["f.req"][0])
                    return _clean_prompt_text(batchexecute_prompt) if batchexecute_prompt else None
            except Exception:
                pass
            return None  # Form data must never fall through to plain text!

        # Structured JSON chat payloads (any platform or custom website)
        if "json" in ct or text.lstrip().startswith(("{", "[")) or "\x1e" in text:
            data = _loads_json_maybe_signalr(text)
            if data is not None:
                got = _extract_from_json(data)
                if got:
                    return got
                got = _deep_extract_from_json(data)
                if got:
                    return got

        # Plain text payloads (only if body itself is a direct user sentence/code)
        cl = text.strip()
        if cl and not cl.startswith(("{", "[")) and looks_like_user_prompt(cl):
            return _clean_prompt_text(cl)

        return None

    except Exception:
        return None


def get_client_ip(flow: http.HTTPFlow) -> str:
    try:
        return flow.client_conn.peername[0]
    except Exception:
        return "127.0.0.1"


def send_to_backend(platform: str, domain: str, prompt: str, client_ip: str, url: str, method: str, upload_images: list[str] | None = None, evaluation_only: bool = False, extracted_text: str = "") -> tuple[bool, str, str, str, str, str]:
    """
    Send intercepted prompt to Gateway backend /api/browser-ai/intercept.
    Backend handles guard rule matching and returns allowed/blocked decision.
    Returns (allowed, rule_triggered, action, redacted_prompt, reply_text, eval_error)
    """
    if not GATEWAY_BACKEND_URL:
        return True, "", "Allowed", prompt, "", "backend url not configured"
    try:
        metadata = {
            "domain": domain,
            "url": url,
            "method": method,
            **_agent_metadata_fields(),
            "evaluation_only": bool(evaluation_only),
        }
        ext = (extracted_text or "").strip()
        if ext:
            metadata["extracted_text"] = ext[:50_000]
        # Do NOT set upload_scan for evaluation_only — that flag is for file audit logs only
        # and would skip AI Guard Bot if the eval_only early-return ever changed.
        payload = json.dumps({
            "platform": platform,
            "prompt": prompt,
            "client_ip": client_ip,
            **_agent_wire_fields(),
            "upload_images": upload_images or [],
            "metadata": metadata,
        }).encode("utf-8")

        req = urllib.request.Request(
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept",
            data=payload,
            headers=_backend_headers({"Content-Type": "application/json"}),
            method="POST"
        )

        # AI Guard Bot may call an LLM — keep under browser request timeouts.
        # evaluation_only: prefer fast defaults for Send.
        try:
            default_to = "25" if evaluation_only and not upload_images else "35"
            eval_timeout = float(
                os.getenv("GATEWAY_EVAL_TIMEOUT") or os.getenv("GATEWAY_EVAL_TIMEOUT", default_to) or default_to
            )
        except Exception:
            eval_timeout = 25.0 if evaluation_only else 35.0
        eval_timeout = max(5.0, min(eval_timeout, 45.0 if evaluation_only else 95.0))
        with urllib.request.urlopen(req, timeout=eval_timeout) as response:
            if response.status == 200:
                res_data = json.loads(response.read().decode("utf-8"))
                allowed = res_data.get("allowed", True)
                rule_triggered = res_data.get("rule_triggered", "")
                action = res_data.get("action", "Allowed")
                redacted_prompt = res_data.get("forward_prompt") or res_data.get("redacted_prompt", prompt)
                # If backend says Redacted but returned the raw prompt, append notice locally.
                if (action or "") in ("Redacted", "Warned") and redacted_prompt == prompt:
                    redacted_prompt = _redacted_forward(prompt, res_data.get("warning_message", ""))
                reply_text = (res_data.get("reply_text") or "").strip()
                eval_error = (res_data.get("eval_error") or "").strip()
                verdict = (res_data.get("security_verdict") or "").strip().lower()
                if not eval_error and verdict == "eval_failed":
                    eval_error = (res_data.get("security_message") or "AI Guard Bot evaluation failed").strip()
                if eval_error:
                    print(f"[Gateway Proxy] AI Guard Bot eval failed | {eval_error}")
                return allowed, rule_triggered, action, redacted_prompt, reply_text, eval_error
    except Exception as e:
        print(f"[Gateway Proxy] send_to_backend failed: {e}")

    # Fallback: apply admin guard rules locally if backend is down
    rules = sorted(
        get_guard_rules(),
        key=lambda r: 0 if (r.get("action") or "BLOCK").upper() == "BLOCK" else 1,
    )
    for r in rules:
        if not rule_matches_prompt(r, prompt):
            continue
        rule_action = (r.get("action") or "BLOCK").upper()
        if rule_action == "ALERT":
            rule_action = "WARN"
        if rule_action == "BLOCK":
            return False, r["name"], "Blocked", prompt, _security_reply_text(r["name"], r.get("warning_message", "")), ""
        if rule_action == "REDACT":
            return True, r["name"], "Redacted", _redacted_forward(prompt, r.get("warning_message", ""), r), "", ""
        if rule_action == "WARN":
            return True, r["name"], "Warned", _warning_forward(prompt, r.get("warning_message", "")), "", ""

    # Backend / evaluator miss — regex already ran locally.
    # Default fail-CLOSED when AI Guard Bots are configured (do not silently allow).
    # Opt-in fail-open only via GATEWAY_FAIL_OPEN=1.
    backend_miss = "backend unreachable or AI Guard Bot could not evaluate"
    if _fail_open():
        if not evaluation_only:
            log_prompt_async(platform, domain, prompt, client_ip, url, method)
        return True, "", "Allowed", prompt, "", backend_miss
    if has_ai_bot_rules():
        return (
            False,
            "AI Guard Bot Unavailable",
            "Blocked",
            prompt,
            "Gateway Guard could not reach the AI security evaluator. Prompt blocked for safety.",
            backend_miss,
        )
    return (
        False,
        "Backend Unreachable",
        "Blocked",
        prompt,
        "Gateway Guard cannot reach the security backend. Prompt blocked for safety.",
        backend_miss,
    )
