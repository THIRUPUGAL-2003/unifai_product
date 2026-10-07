# -*- mode: python ; coding: utf-8 -*-
from pathlib import Path

from PyInstaller.utils.hooks import collect_all

ROOT = Path(SPECPATH).resolve()
# guard_bootstrap is the entry so Rebuild & Publish can hot-swap the agent modules below.
AGENT = ROOT / "agent" / "guard_bootstrap.py"
PROXY = ROOT / "proxy" / "browser_ai_proxy.py"
PROXY_PARTS = ROOT / "proxy" / "gateway_proxy_parts"
CONFIG = ROOT / "config" / "gateway_guard_config.json"

# Auto-encrypt proxy engine bundle before packaging (zero plain-text code leak)
import sys
sys.path.insert(0, str(PROXY_PARTS))
import bundle_crypto
bundle_crypto.encrypt_parts_bundle(PROXY_PARTS, PROXY_PARTS / "gateway_proxy_parts.enc")

datas = [
    (str(PROXY), "."),
    (str(PROXY_PARTS / "gateway_proxy_parts.enc"), "gateway_proxy_parts"),
    (str(PROXY_PARTS / "bundle_crypto.py"), "gateway_proxy_parts"),
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
    "agent_autoupdate",
    "agent_config",
    "agent_http",
    "agent_logging",
    "agent_certs",
    "agent_proxy_engine",
    "agent_proxy_bundle",
    "agent_state",
    "agent_pac_content",
    "agent_pac_server",
    "agent_browser_policy",
    "agent_pac_orchestration",
    "agent_identity",
    "agent_health",
    "agent_heartbeat",
    "agent_lifecycle",
    "guard_platform",
    "guard_bootstrap",
    "gateway_agent",
    # Stdlib headroom so hot-updated Guard code can use these without a new EXE.
    "shlex", "runpy", "csv", "difflib", "glob", "fnmatch", "queue", "secrets", "hmac",
    "gzip", "tarfile", "plistlib", "sqlite3", "xml.etree.ElementTree", "http.server",
    "concurrent.futures", "ctypes.wintypes", "winreg",
]

for pkg in ("pypdf", "PIL", "winrt", "mitmproxy", "mitmproxy_windows"):
    tmp_ret = collect_all(pkg)
    datas += tmp_ret[0]
    binaries += tmp_ret[1]
    hiddenimports += tmp_ret[2]

a = Analysis(
    [str(AGENT)],
    pathex=[str(ROOT / "agent")],
    binaries=binaries,
    datas=datas,
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
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
