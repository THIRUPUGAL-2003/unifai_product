#!/usr/bin/env python3
"""
Raksha Browser AI Live Proxy Interceptor & DLP Guardrail Addon for mitmproxy.

This file is the mitmproxy entrypoint (`-s browser_ai_proxy.py`).
Implementation is split across `raksha_proxy_parts/*.py` and loaded into ONE
shared module namespace (same behavior as the former monolith — no import cycles).

Parts (load order in MANIFEST.txt):
  config_caches_rules.py
  helpers_prompts.py
  uploads_detect.py
  file_policy.py
  extract_office_backend.py
  responses_inject.py
  responses_addon.py

Do not import part files directly.
"""

from __future__ import annotations

import sys
from pathlib import Path

_PARTS_DIR_NAME = "raksha_proxy_parts"


def _parts_dir() -> Path:
    here = Path(__file__).resolve().parent
    cand = here / _PARTS_DIR_NAME
    if cand.is_dir():
        return cand
    # PyInstaller: datas land in _MEIPASS
    meipass = getattr(sys, "_MEIPASS", None)
    if meipass:
        cand2 = Path(meipass) / _PARTS_DIR_NAME
        if cand2.is_dir():
            return cand2
    raise FileNotFoundError(
        f"Missing {_PARTS_DIR_NAME}/ next to browser_ai_proxy.py "
        f"(looked in {here}" + (f" and {meipass}" if meipass else "") + ")"
    )


def _load_parts() -> None:
    parts = _parts_dir()
    ns = globals()

    # 1. Prefer encrypted bundle if present (Production / Client build - zero plain text code)
    enc_path = parts / "raksha_proxy_parts.enc"
    if not enc_path.is_file():
        enc_path = parts.parent / "raksha_proxy_parts.enc"

    if enc_path.is_file():
        try:
            sys.path.insert(0, str(parts))
            import bundle_crypto
            code_map = bundle_crypto.decrypt_parts_bundle(enc_path)

            manifest = parts / "MANIFEST.txt"
            if manifest.is_file():
                order = [
                    ln.strip().lstrip("\ufeff")
                    for ln in manifest.read_text(encoding="utf-8-sig").splitlines()
                    if ln.strip().lstrip("\ufeff") and not ln.strip().startswith("#")
                ]
            else:
                order = sorted(code_map.keys())

            loaded_count = 0
            for name in order:
                if name in code_map:
                    exec(code_map[name], ns)
                    loaded_count += 1

            print(f"[Raksha Proxy] Secure in-memory bundle loaded: {loaded_count} encrypted parts active (zero disk leak).")
            return
        except Exception as e:
            if getattr(sys, "frozen", False):
                raise RuntimeError(f"Encrypted proxy bundle failed to load: {e}") from e
            print(f"[Raksha Proxy WARNING] Failed to load encrypted bundle: {e}, attempting source fallback...")

    if getattr(sys, "frozen", False):
        raise RuntimeError("Frozen Guard requires raksha_proxy_parts.enc; plain proxy sources are not loaded.")

    # 2. Source fallback (Development / unit testing)
    manifest = parts / "MANIFEST.txt"
    if manifest.is_file():
        names = [
            ln.strip().lstrip("\ufeff")
            for ln in manifest.read_text(encoding="utf-8-sig").splitlines()
            if ln.strip().lstrip("\ufeff")
        ]
    else:
        names = sorted(p.name for p in parts.glob("*.py") if not p.name.startswith("_") and p.name != "bundle_crypto.py")
    if not names:
        raise RuntimeError(f"No proxy parts found in {parts}")

    for name in names:
        path = parts / name
        if path.is_file():
            code = path.read_text(encoding="utf-8")
            exec(compile(code, str(path), "exec"), ns)


_load_parts()

# mitmproxy discovers this after parts load (BrowserAIInterceptor defined in part 06).
if "addons" not in globals() or not addons:  # type: ignore[name-defined]
    addons = [BrowserAIInterceptor()]  # type: ignore[name-defined]
