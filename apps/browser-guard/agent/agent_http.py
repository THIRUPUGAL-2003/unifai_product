"""HTTP helpers for Raksha Guard backend API calls."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request

from agent_config import AGENT_VERSION, RAKSHA_GUARD_SECRET


def _guard_headers(extra: dict | None = None) -> dict:
    headers = {
        "Accept": "application/json",
        "User-Agent": f"Raksha-Guard/{AGENT_VERSION}",
    }
    secret = (RAKSHA_GUARD_SECRET or os.environ.get("RAKSHA_GUARD_SECRET") or "").strip()
    if secret:
        headers["X-Raksha-Guard-Key"] = secret
    if extra:
        headers.update(extra)
    return headers


def _http_json(method: str, url: str, payload: dict | None = None, timeout: int = 12) -> tuple[int, dict | None]:
    data = None
    headers = _guard_headers()
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8", errors="replace")
            try:
                return resp.status, json.loads(body) if body else {}
            except Exception:
                return resp.status, None
    except urllib.error.HTTPError as e:
        try:
            body = e.read().decode("utf-8", errors="replace")
            parsed = json.loads(body) if body else None
        except Exception:
            parsed = None
        return e.code, parsed
    except Exception as e:
        print(f"[Raksha Guard WARNING] HTTP {method} {url} failed: {e}")
        return 0, None


def _http_get_text(url: str, accept: str, timeout: int = 10) -> str | None:
    try:
        req = urllib.request.Request(url, headers=_guard_headers({"Accept": accept}))
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8", errors="replace")
            low = body.lstrip().lower()
            if low.startswith("<!doctype") or low.startswith("<html"):
                print(f"[Raksha Guard WARNING] Got HTML instead of API from {url} (backend missing Browser AI routes).")
                return None
            return body
    except Exception as e:
        print(f"[Raksha Guard WARNING] HTTP get failed {url}: {e}")
        return None
