"""mitmproxy entry shipped in the EXE.

The Guard's proxy loader and agent code live in gateway_guard_code.enc.
This stub only decrypts that blob in memory. It contains no rules.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path


def _code_enc() -> Path:
    code_dir = os.environ.get("GATEWAY_GUARD_CODE_DIR") or ""
    if code_dir:
        hot = Path(code_dir) / "gateway_guard_code.enc"
        if hot.is_file():
            return hot
    meipass = getattr(sys, "_MEIPASS", None)
    if meipass:
        packed = Path(meipass) / "gateway_guard_code.enc"
        if packed.is_file():
            return packed
    here = Path(__file__).resolve().parent / "gateway_guard_code.enc"
    if here.is_file():
        return here
    raise FileNotFoundError("gateway_guard_code.enc")


try:
    import bundle_crypto
except ImportError:
    sys.path.insert(0, str(Path(__file__).resolve().parent / "gateway_proxy_parts"))
    import bundle_crypto

_code = bundle_crypto.decrypt_parts_bundle(_code_enc())["browser_ai_proxy.py"]
exec(_code, globals())
