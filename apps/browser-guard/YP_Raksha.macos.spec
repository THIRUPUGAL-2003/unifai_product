# -*- mode: python ; coding: utf-8 -*-
# Build on macOS:  pyinstaller YP_Raksha.macos.spec
# Output: dist/YP_Raksha.app
import shutil
from pathlib import Path

from PyInstaller.utils.hooks import collect_all

ROOT = Path(SPECPATH).resolve()
AGENT = ROOT / "agent" / "guard_bootstrap.py"
PROXY_PARTS = ROOT / "proxy" / "gateway_proxy_parts"
CONFIG = ROOT / "config" / "gateway_guard_config.json"
CODE_ENC = ROOT / "proxy" / "gateway_guard_code.enc"
STUB_DIR = ROOT / "installer" / "frozen_stub"
STUB_DIR.mkdir(parents=True, exist_ok=True)
shutil.copyfile(ROOT / "installer" / "proxy_stub.py", STUB_DIR / "browser_ai_proxy.py")

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
    (str(ROOT / "extension"), "extension"),
]
binaries = []
hiddenimports = [
    "pypdf",
    "PIL",
    "PIL.Image",
    "bundle_crypto",
    "shlex", "runpy", "csv", "difflib", "glob", "fnmatch", "queue", "secrets", "hmac",
    "gzip", "tarfile", "plistlib", "sqlite3", "xml.etree.ElementTree", "http.server",
    "concurrent.futures", "fcntl",
]

for pkg in ("pypdf", "PIL", "mitmproxy", "mitmproxy_macos"):
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
    excludes=["winrt", "mitmproxy_windows", *AGENT_MODULES],
    noarchive=False,
    optimize=2,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    [],
    exclude_binaries=True,
    name="YP_Raksha",
    debug=False,
    bootloader_ignore_signals=False,
    strip=True,
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
    name="YP_Raksha",
)

mac_icon = str(ROOT / "yp_raksha.icns") if (ROOT / "yp_raksha.icns").is_file() else (str(ROOT / "gateway_guard.icns") if (ROOT / "gateway_guard.icns").is_file() else None)

app = BUNDLE(
    coll,
    name="YP_Raksha.app",
    icon=mac_icon,
    bundle_identifier="com.yespanchi.raksha",
    info_plist={
        "CFBundleDisplayName": "YP Raksha",
        "CFBundleName": "YP Raksha",
        "CFBundleShortVersionString": "1.1.29",
        "CFBundleVersion": "1.1.29",
        "LSBackgroundOnly": False,
        "NSHighResolutionCapable": True,
    },
)
