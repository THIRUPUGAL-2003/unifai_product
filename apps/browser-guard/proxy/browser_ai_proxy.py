#!/usr/bin/env python3
"""
UnifAI Browser AI Live Proxy Interceptor & DLP Guardrail Addon for mitmproxy.

This file is the mitmproxy entrypoint (`-s browser_ai_proxy.py`).
Implementation is split across `unifai_proxy_parts/*.py` and loaded into ONE
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

_PARTS_DIR_NAME = "unifai_proxy_parts"


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
    manifest = parts / "MANIFEST.txt"
    if manifest.is_file():
        names = [
            ln.strip().lstrip("\ufeff")
            for ln in manifest.read_text(encoding="utf-8-sig").splitlines()
            if ln.strip().lstrip("\ufeff")
        ]
    else:
        names = sorted(p.name for p in parts.glob("*.py") if not p.name.startswith("_"))
    if not names:
        raise RuntimeError(f"No proxy parts found in {parts}")

    ns = globals()
    for name in names:
        path = parts / name
        code = path.read_text(encoding="utf-8")
        exec(compile(code, str(path), "exec"), ns)


_load_parts()

# mitmproxy discovers this after parts load (BrowserAIInterceptor defined in part 06).
if "addons" not in globals() or not addons:  # type: ignore[name-defined]
    addons = [BrowserAIInterceptor()]  # type: ignore[name-defined]
