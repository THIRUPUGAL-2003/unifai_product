# -*- mode: python ; coding: utf-8 -*-
# Build on macOS only:  pyinstaller Raksha_Guard.macos.spec
# Output: dist/Raksha_Guard.app
from pathlib import Path

from PyInstaller.utils.hooks import collect_all

ROOT = Path(SPECPATH).resolve()
# guard_bootstrap is the entry so Rebuild & Publish can hot-swap the agent modules below.
AGENT = ROOT / "agent" / "guard_bootstrap.py"
PROXY = ROOT / "proxy" / "browser_ai_proxy.py"
PROXY_PARTS = ROOT / "proxy" / "raksha_proxy_parts"
CONFIG = ROOT / "config" / "raksha_guard_config.json"

datas = [
    (str(PROXY), "."),
    (str(PROXY_PARTS), "raksha_proxy_parts"),
    (str(CONFIG), "."),
]
binaries = []
# Explicit agent modules (split from raksha_agent.py) — safe if Analysis misses a lazy import.
hiddenimports = [
    "pypdf",
    "PIL",
    "PIL.Image",
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
    "raksha_agent",
    "agent_autoupdate",
    # Stdlib headroom so hot-updated Guard code can use these without a new .app.
    "shlex", "runpy", "csv", "difflib", "glob", "fnmatch", "queue", "secrets", "hmac",
    "gzip", "tarfile", "plistlib", "sqlite3", "xml.etree.ElementTree", "http.server",
    "concurrent.futures", "fcntl",
]

for pkg in ("pypdf", "PIL", "mitmproxy", "mitmproxy_macos"):
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
    excludes=["winrt", "mitmproxy_windows"],
    noarchive=False,
    optimize=0,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    [],
    exclude_binaries=True,
    name="Raksha_Guard",
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=False,
    console=False,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
)

coll = COLLECT(
    exe,
    a.binaries,
    a.datas,
    strip=False,
    upx=False,
    upx_exclude=[],
    name="Raksha_Guard",
)

mac_icon = str(ROOT / "raksha_guard.icns") if (ROOT / "raksha_guard.icns").is_file() else None

app = BUNDLE(
    coll,
    name="Raksha_Guard.app",
    icon=mac_icon,
    bundle_identifier="com.raksha.guard",
    info_plist={
        "CFBundleDisplayName": "Raksha Guard",
        "CFBundleName": "Raksha Guard",
        "CFBundleShortVersionString": "1.1.15",
        "CFBundleVersion": "1.1.15",
        "LSBackgroundOnly": False,
        "NSHighResolutionCapable": True,
    },
)
