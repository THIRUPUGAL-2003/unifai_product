#!/usr/bin/env python3
"""Root wrapper for YP-Raksha unified build pipeline."""
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
TARGET = ROOT / "apps" / "browser-guard" / "build_all.py"

if __name__ == "__main__":
    if not TARGET.is_file():
        sys.exit(f"Missing {TARGET}")
    res = subprocess.run([sys.executable, str(TARGET)] + sys.argv[1:], cwd=str(ROOT))
    sys.exit(res.returncode)
