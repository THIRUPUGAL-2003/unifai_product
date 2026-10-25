#!/usr/bin/env bash
# Build Gateway Guard for macOS (.app + release ZIP). Run on a Mac.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Prefer Python 3.11+ (mitmproxy 10+). Override with: PYTHON=/path/to/python ./installer/build_macos.sh
if [[ -z "${PYTHON:-}" ]]; then
  if [[ -x "$ROOT/.venv-guard/bin/python" ]]; then
    PYTHON="$ROOT/.venv-guard/bin/python"
  else
    for c in python3.13 python3.12 python3.11 /opt/homebrew/bin/python3.13 /opt/homebrew/bin/python3.12 /opt/homebrew/bin/python3.11 /usr/local/bin/python3.12 /usr/local/bin/python3.11 python3; do
      if command -v "$c" >/dev/null 2>&1; then
        PYTHON="$(command -v "$c")"
        break
      fi
    done
  fi
fi
if [[ -z "${PYTHON:-}" ]]; then
  echo "ERROR: Python 3.11+ required (mitmproxy 10)."
  exit 1
fi

PY_VER="$("$PYTHON" -c "import sys; print('%d.%d' % sys.version_info[:2])")"
PY_MAJOR="$("$PYTHON" -c "import sys; print(sys.version_info[0])")"
PY_MINOR="$("$PYTHON" -c "import sys; print(sys.version_info[1])")"
if [[ "$PY_MAJOR" -lt 3 || ( "$PY_MAJOR" -eq 3 && "$PY_MINOR" -lt 11 ) ]]; then
  echo "ERROR: $PYTHON is $PY_VER - need Python 3.11+ for Guard packaging."
  echo "Install: brew install python@3.12"
  echo "Then:    PYTHON=/opt/homebrew/bin/python3.12 ./installer/build_macos.sh"
  exit 1
fi

VERSION="$(tr -d '[:space:]\ufeff' < release/VERSION.txt 2>/dev/null || true)"
VERSION="${VERSION//$'\xef\xbb\xbf'/}"
if [[ -z "${VERSION}" ]]; then
  VERSION="$("$PYTHON" -c "import re, pathlib; t=pathlib.Path('agent/agent_config.py').read_text(encoding='utf-8'); m=re.search(r'AGENT_VERSION\\s*=\\s*[\"\\']([^\"\\']+)[\"\\']', t); print(m.group(1) if m else '0.0.0')")"
fi

echo "============================================================"
echo " Gateway Guard macOS build  v${VERSION}"
echo " Python: $PYTHON ($PY_VER)"
echo "============================================================"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "ERROR: Build macOS Guard on a Mac (PyInstaller cannot cross-compile Darwin)."
  exit 1
fi

for f in agent/gateway_agent.py proxy/browser_ai_proxy.py config/gateway_guard_config.json; do
  if [[ ! -f "$f" ]]; then
    echo "Missing $f"
    exit 1
  fi
done

# Generate macOS .icns from gateway_guard.png if not present
if [[ ! -f "gateway_guard.icns" && -f "gateway_guard.png" ]] && command -v sips >/dev/null 2>&1 && command -v iconutil >/dev/null 2>&1; then
  echo "Generating gateway_guard.icns from gateway_guard.png..."
  ICONSET="gateway_guard.iconset"
  mkdir -p "$ICONSET"
  sips -z 16 16     gateway_guard.png --out "$ICONSET/icon_16x16.png" >/dev/null 2>&1 || true
  sips -z 32 32     gateway_guard.png --out "$ICONSET/icon_16x16@2x.png" >/dev/null 2>&1 || true
  sips -z 32 32     gateway_guard.png --out "$ICONSET/icon_32x32.png" >/dev/null 2>&1 || true
  sips -z 64 64     gateway_guard.png --out "$ICONSET/icon_32x32@2x.png" >/dev/null 2>&1 || true
  sips -z 128 128   gateway_guard.png --out "$ICONSET/icon_128x128.png" >/dev/null 2>&1 || true
  sips -z 256 256   gateway_guard.png --out "$ICONSET/icon_128x128@2x.png" >/dev/null 2>&1 || true
  sips -z 256 256   gateway_guard.png --out "$ICONSET/icon_256x256.png" >/dev/null 2>&1 || true
  sips -z 512 512   gateway_guard.png --out "$ICONSET/icon_256x256@2x.png" >/dev/null 2>&1 || true
  sips -z 512 512   gateway_guard.png --out "$ICONSET/icon_512x512.png" >/dev/null 2>&1 || true
  iconutil -c icns "$ICONSET" -o gateway_guard.icns 2>/dev/null || true
  rm -rf "$ICONSET"
fi

# Preflight: modular agent + proxy parts (post-split layout)
for f in \
  agent/agent_config.py \
  agent/agent_health.py \
  agent/agent_heartbeat.py \
  agent/agent_lifecycle.py \
  agent/agent_proxy_bundle.py \
  agent/guard_bootstrap.py \
  agent/guard_platform.py \
  proxy/gateway_proxy_parts/MANIFEST.txt \
  proxy/gateway_proxy_parts/responses_inject.py \
  proxy/gateway_proxy_parts/responses_addon.py
do
  if [[ ! -f "$f" ]]; then
    echo "Missing $f (Mac build needs latest split layout — sync repo from Windows first)"
    exit 1
  fi
done
while IFS= read -r part || [[ -n "$part" ]]; do
  part="${part#"${part%%[![:space:]]*}"}"
  part="${part%"${part##*[![:space:]]}"}"
  part="${part#$'\xef\xbb\xbf'}"
  [[ -z "$part" ]] && continue
  if [[ ! -f "proxy/gateway_proxy_parts/$part" ]]; then
    echo "Missing proxy part from MANIFEST: $part"
    exit 1
  fi
done < proxy/gateway_proxy_parts/MANIFEST.txt

# Keep Info.plist version in sync with VERSION / agent_config
SPEC="$ROOT/Gateway_Guard.macos.spec"
if [[ -f "$SPEC" ]]; then
  "$PYTHON" - "$SPEC" "$VERSION" <<'PY'
import pathlib, re, sys
spec, ver = pathlib.Path(sys.argv[1]), sys.argv[2]
text = spec.read_text(encoding="utf-8")
text2 = re.sub(
    r'("CFBundleShortVersionString":\s*")[^"]+(")',
    rf'\g<1>{ver}\g<2>',
    text,
)
text2 = re.sub(
    r'("CFBundleVersion":\s*")[^"]+(")',
    rf'\g<1>{ver}\g<2>',
    text2,
)
if text2 != text:
    spec.write_text(text2, encoding="utf-8")
    print(f"Updated {spec.name} bundle version → {ver}")
PY
fi

VENV="$ROOT/.venv-guard"
if [[ ! -d "$VENV" ]]; then
  echo "Creating venv at $VENV"
  "$PYTHON" -m venv "$VENV"
fi
# shellcheck disable=SC1091
source "$VENV/bin/activate"
PYTHON="$VENV/bin/python"

"$PYTHON" -m pip install -q --upgrade pip
"$PYTHON" -m pip install -q -r requirements-guard.txt

echo ""
echo "1) PyInstaller → Gateway_Guard.app"
"$PYTHON" installer/build_agent.py

APP_REL="release/Gateway_Guard.app"
if [[ ! -d "$APP_REL" ]]; then
  echo "ERROR: $APP_REL missing after build"
  exit 1
fi
if [[ -f "$APP_REL/Contents/MacOS/Gateway_Guard" ]]; then
  chmod +x "$APP_REL/Contents/MacOS/Gateway_Guard"
fi
printf '%s\n' "$VERSION" > "$APP_REL/Contents/Resources/VERSION.txt"
printf '%s\n' "$VERSION" > release/VERSION.txt

echo ""
echo "2) Sync config from .env / existing config"
"$PYTHON" scripts/sync_config_from_env.py || true

echo ""
echo "3) Build macOS Installer Package (.pkg / Setup Wizard)"
if [[ -f "$ROOT/installer/build_pkg_macos.sh" ]]; then
  "$ROOT/installer/build_pkg_macos.sh" || true
fi

echo ""
echo "4) Stage macOS employee package"
STAGE="installer/staging-mac"
chflags -R nouchg "$STAGE" 2>/dev/null || true
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R "$APP_REL" "$STAGE/Gateway_Guard.app"
cp config/gateway_guard_config.json "$STAGE/gateway_guard_config.json"
if [[ -f "release/Gateway_Guard_Setup.pkg" ]]; then
  cp "release/Gateway_Guard_Setup.pkg" "$STAGE/Gateway_Guard_Setup.pkg"
fi
if [[ -f release/EMPLOYEE_README_MAC.txt ]]; then
  cp release/EMPLOYEE_README_MAC.txt "$STAGE/EMPLOYEE_README_MAC.txt"
else
  cp installer/EMPLOYEE_README_MAC.txt "$STAGE/EMPLOYEE_README_MAC.txt"
fi
cp release/INSTALL_MACOS.txt "$STAGE/INSTALL_MACOS.txt"
cp release/UNINSTALL_MACOS.txt "$STAGE/UNINSTALL_MACOS.txt"
cp installer/Install_Gateway_Guard.command "$STAGE/Install_Gateway_Guard.command"
cp installer/Uninstall_Gateway_Guard.command "$STAGE/Uninstall_Gateway_Guard.command"
cp release/Update_Gateway_Guard_macOS.command "$STAGE/Update_Gateway_Guard_macOS.command"
# Strip Windows CRLF so Mac Terminal never hits: bad interpreter: /bin/bash^M
for _cmd in "$STAGE/Install_Gateway_Guard.command" "$STAGE/Uninstall_Gateway_Guard.command" "$STAGE/Update_Gateway_Guard_macOS.command"; do
  if [[ -f "$_cmd" ]]; then
    "$PYTHON" -c "from pathlib import Path; p=Path(r'''$_cmd'''); p.write_bytes(p.read_bytes().replace(b'\r\n', b'\n').replace(b'\r', b'\n'))"
  fi
done
chmod +x "$STAGE/Install_Gateway_Guard.command" "$STAGE/Uninstall_Gateway_Guard.command" "$STAGE/Update_Gateway_Guard_macOS.command"
if [[ -f "$STAGE/Gateway_Guard.app/Contents/MacOS/Gateway_Guard" ]]; then
  chmod +x "$STAGE/Gateway_Guard.app/Contents/MacOS/Gateway_Guard"
fi

mkdir -p "$STAGE/Gateway_Guard.app/Contents/Resources"
cp config/gateway_guard_config.json "$STAGE/Gateway_Guard.app/Contents/Resources/gateway_guard_config.json"
printf '%s\n' "$VERSION" > "$STAGE/Gateway_Guard.app/Contents/Resources/VERSION.txt"

echo ""
echo "5) ZIP for Download Setup package"
mkdir -p release
ZIP_OUT="$ROOT/release/Gateway_Guard_macOS.zip"
rm -f "$ZIP_OUT"
(
  cd "$STAGE"
  ZIP_FILES=(
    Gateway_Guard.app
    gateway_guard_config.json
    EMPLOYEE_README_MAC.txt
    INSTALL_MACOS.txt
    UNINSTALL_MACOS.txt
    Install_Gateway_Guard.command
    Uninstall_Gateway_Guard.command
    Update_Gateway_Guard_macOS.command
  )
  if [[ -f "Gateway_Guard_Setup.pkg" ]]; then
    ZIP_FILES=(Gateway_Guard_Setup.pkg "${ZIP_FILES[@]}")
  fi
  zip -r -y "$ZIP_OUT" "${ZIP_FILES[@]}"
)

printf '%s\n' "$VERSION" > release/VERSION.txt
rm -f release/MAC_ZIP_STALE.txt

cp -f installer/Install_Gateway_Guard.command release/Install_Gateway_Guard.command
cp -f installer/Uninstall_Gateway_Guard.command release/Uninstall_Gateway_Guard.command
chmod +x release/Install_Gateway_Guard.command release/Uninstall_Gateway_Guard.command release/Update_Gateway_Guard_macOS.command

echo ""
echo "============================================================"
echo " SUCCESS — Gateway Guard ${VERSION} (macOS)"
echo "  App:       release/Gateway_Guard.app"
echo "  Setup PKG: release/Gateway_Guard_Setup.pkg"
echo "  ZIP:       release/Gateway_Guard_macOS.zip"
echo "  Docs:      release/INSTALL_MACOS.txt  release/UNINSTALL_MACOS.txt"
echo "============================================================"
ls -la release/Gateway_Guard.app release/Gateway_Guard_Setup.pkg release/Gateway_Guard_macOS.zip release/INSTALL_MACOS.txt release/UNINSTALL_MACOS.txt
