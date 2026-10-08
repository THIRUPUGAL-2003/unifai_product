# -*- mode: python ; coding: utf-8 -*-
import shutil
from pathlib import Path

from PyInstaller.utils.hooks import collect_all

ROOT = Path(SPECPATH).resolve()
# guard_bootstrap is the only first-party module frozen as bytecode.
# Agent modules and the proxy loader are sealed in gateway_guard_code.enc.
AGENT = ROOT / "agent" / "guard_bootstrap.py"
PROXY_PARTS = ROOT / "proxy" / "gateway_proxy_parts"
CONFIG = ROOT / "config" / "gateway_guard_config.json"
CODE_ENC = ROOT / "proxy" / "gateway_guard_code.enc"
STUB_DIR = ROOT / "installer" / "frozen_stub"
STUB_DIR.mkdir(parents=True, exist_ok=True)
shutil.copyfile(ROOT / "installer" / "proxy_stub.py", STUB_DIR / "browser_ai_proxy.py")

# Auto-encrypt Guard code before packaging (zero plain-text product code)
import sys
sys.path.insert(0, str(PROXY_PARTS))
import bundle_crypto
bundle_crypto.encrypt_parts_bundle(PROXY_PARTS, PROXY_PARTS / "gateway_proxy_parts.enc")
bundle_crypto.encrypt_guard_code_bundle(ROOT, CODE_ENC)

AGENT_MODULES = [
    p.stem for p in (ROOT / "agent").glob("*.py")
    if p.name != "guard_bootstrap.py" and not p.name.startswith("_")
]

datas = [
    (str(STUB_DIR / "browser_ai_proxy.py"), "."),
    (str(CODE_ENC), "."),
    (str(PROXY_PARTS / "gateway_proxy_parts.enc"), "gateway_proxy_parts"),
    (str(PROXY_PARTS / "MANIFEST.txt"), "gateway_proxy_parts"),
    (str(CONFIG), "."),
]
binaries = []
hiddenimports = [
    "pypdf",
    "PIL",
    "PIL.Image",
    "winrt",
    "winrt.windows.media.ocr",
    "winrt.windows.globalization",
    "winrt.windows.graphics.imaging",
    "winrt.windows.storage.streams",
    "bundle_crypto",
    # Stdlib headroom so hot-updated Guard code can use these without a new EXE.
    "shlex", "runpy", "csv", "difflib", "glob", "fnmatch", "queue", "secrets", "hmac",
    "gzip", "tarfile", "plistlib", "sqlite3", "xml.etree.ElementTree", "http.server",
    "concurrent.futures", "ctypes.wintypes", "winreg",
]

for pkg in ("pypdf", "PIL", "winrt", "mitmproxy", "mitmproxy_windows"):
    tmp_ret = collect_all(pkg)
    datas += [item for item in tmp_ret[0] if not str(item[0]).lower().endswith((".py", ".pyi"))]
    binaries += tmp_ret[1]
    hiddenimports += tmp_ret[2]

a = Analysis(
    [str(AGENT)],
    pathex=[str(ROOT / "agent"), str(PROXY_PARTS)],
    binaries=binaries,
    datas=datas,
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=AGENT_MODULES,
    noarchive=False,
    optimize=2,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name="Gateway_Guard",
    debug=False,
    bootloader_ignore_signals=False,
    strip=True,
    upx=False,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=False,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
    icon=str(ROOT / "gateway_guard.ico"),
)
