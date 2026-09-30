"""Logging, single-instance guard, and resource path helpers."""

from __future__ import annotations

import os
import sys

from guard_platform import data_dir, ensure_single_instance as platform_ensure_single_instance


class _Tee:
    """File-like stdout/stderr. mitmdump calls isatty(); missing it kills the proxy."""

    closed = False
    errors = "replace"
    name = "<unifai-guard-log>"
    mode = "w"

    def __init__(self, stream, log_file):
        self._stream = stream
        self._log = log_file
        self.encoding = getattr(stream, "encoding", None) or "utf-8"

    def write(self, data):
        try:
            if self._stream is not None:
                self._stream.write(data)
                self._stream.flush()
        except Exception:
            pass
        try:
            self._log.write(data)
            self._log.flush()
        except Exception:
            pass
        return len(data) if data is not None else 0

    def flush(self):
        try:
            if self._stream is not None:
                self._stream.flush()
        except Exception:
            pass
        try:
            self._log.flush()
        except Exception:
            pass

    def isatty(self) -> bool:
        return False

    def readable(self) -> bool:
        return False

    def writable(self) -> bool:
        return True

    def seekable(self) -> bool:
        return False

    def fileno(self):
        raise OSError(9, "Tee has no fileno")

    def reconfigure(self, *args, **kwargs):
        fn = getattr(self._stream, "reconfigure", None)
        if callable(fn):
            return fn(*args, **kwargs)
        return None


def setup_file_logging() -> str:
    log_path = os.path.join(data_dir(), "unifai_guard.log")
    log_f = open(log_path, "a", encoding="utf-8", buffering=1)
    sys.stdout = _Tee(sys.stdout, log_f)
    sys.stderr = _Tee(sys.stderr, log_f)
    return log_path


def ensure_single_instance() -> bool:
    return platform_ensure_single_instance()


def get_resource_path(relative_path: str) -> str:
    if hasattr(sys, "_MEIPASS"):
        return os.path.join(sys._MEIPASS, relative_path)
    return os.path.join(os.path.dirname(os.path.abspath(__file__)), relative_path)
