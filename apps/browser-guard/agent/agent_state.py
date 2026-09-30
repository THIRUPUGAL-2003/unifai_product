"""Shared mutable runtime state for UnifAI Guard PAC and health reporting."""

from __future__ import annotations

import threading

from agent_config import PAC_HTTP_HOST, PAC_HTTP_PORT

_LAST_PAC_BUST = ""
_HEALTH_LOCK = threading.Lock()
_LAST_HEALTH: dict = {}
_PAC_HTTP_URL = f"http://{PAC_HTTP_HOST}:{PAC_HTTP_PORT}/proxy.pac"
