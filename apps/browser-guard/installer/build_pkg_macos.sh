#!/usr/bin/env bash
# Build Raksha Guard .pkg for macOS (product-style installer).
# Run on a Mac AFTER ./installer/build_macos.sh (needs release/Raksha_Guard.app).
#
# Optional signing:
#   export MACOS_SIGN_IDENTITY="Developer ID Installer: Your Name (TEAMID)"
#   ./installer/build_pkg_macos.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]\ufeff' < release/VERSION.txt 2>/dev/null || echo "0.0.0")"
APP="release/Raksha_Guard.app"
if [[ ! -d "$APP" ]]; then
  echo "ERROR: $APP missing — run ./installer/build_macos.sh first"
  exit 1
fi

STAGE="installer/pkg-root"
SCRIPTS="installer/pkg-scripts"
chflags -R nouchg "$STAGE" 2>/dev/null || true
rm -rf "$STAGE" "$SCRIPTS"
mkdir -p "$STAGE/Applications" "$SCRIPTS"

cp -R "$APP" "$STAGE/Applications/Raksha_Guard.app"
if [[ -f config/raksha_guard_config.json ]]; then
  mkdir -p "$STAGE/Applications/Raksha_Guard.app/Contents/Resources"
  cp config/raksha_guard_config.json "$STAGE/Applications/Raksha_Guard.app/Contents/Resources/raksha_guard_config.json"
fi

# postinstall: terminate old instance, remove previous UnifAI build, clear quarantine,
# set permissions, apply server-fresh config, open app for logged-in user
cat > "$SCRIPTS/postinstall" <<'EOF'
#!/bin/bash
set -euo pipefail
pkill -f "Raksha_Guard.app/Contents/MacOS/Raksha_Guard" 2>/dev/null || true
pkill -f "/MacOS/Raksha_Guard" 2>/dev/null || true
pkill -f "UnifAI_Guard.app/Contents/MacOS/UnifAI_Guard" 2>/dev/null || true
pkill -f "/MacOS/UnifAI_Guard" 2>/dev/null || true
sleep 1

APP="/Applications/Raksha_Guard.app"
xattr -dr com.apple.quarantine "$APP" 2>/dev/null || true
if [[ -f "$APP/Contents/MacOS/Raksha_Guard" ]]; then
  chmod +x "$APP/Contents/MacOS/Raksha_Guard" 2>/dev/null || true
fi

# Browser AI → Setup → Build now ships a server-fresh config next to the .pkg; it wins over the compiled-in one.
PKG_SRC="${PACKAGE_PATH:-${1:-}}"
if [[ -n "$PKG_SRC" && -f "$(dirname "$PKG_SRC")/raksha_guard_config.json" ]]; then
  mkdir -p "$APP/Contents/Resources"
  cp -f "$(dirname "$PKG_SRC")/raksha_guard_config.json" "$APP/Contents/Resources/raksha_guard_config.json" 2>/dev/null || true
fi
rm -rf "/Applications/UnifAI_Guard.app" 2>/dev/null || true

CONSOLE_USER=$(stat -f "%Su" /dev/console 2>/dev/null || echo "")
if [[ -n "$CONSOLE_USER" && "$CONSOLE_USER" != "root" ]]; then
  USER_HOME=$(dscl . -read "/Users/$CONSOLE_USER" NFSHomeDirectory 2>/dev/null | awk '{print $2}')
  USER_HOME="${USER_HOME:-/Users/$CONSOLE_USER}"
  USER_UID=$(id -u "$CONSOLE_USER")

  # An earlier build shipped as UnifAI_Guard.app with its own LaunchAgent, browser policies and
  # data dir. Left running, both Guards fight over the PAC/proxy, so remove it, keeping its
  # data dir (device identity) for this build.
  OLD_PLIST="$USER_HOME/Library/LaunchAgents/com.unifai.guard.plist"
  if [[ -f "$OLD_PLIST" ]]; then
    launchctl bootout "gui/$USER_UID" "$OLD_PLIST" 2>/dev/null || true
    rm -f "$OLD_PLIST"
  fi
  sudo -u "$CONSOLE_USER" HOME="$USER_HOME" /bin/bash <<'MIGRATE' || true
SUPPORT="$HOME/Library/Application Support"
for ROOT in \
  "$SUPPORT/Google/Chrome/policies/managed" \
  "$SUPPORT/Google/Chrome Canary/policies/managed" \
  "$SUPPORT/Microsoft Edge/policies/managed" \
  "$SUPPORT/BraveSoftware/Brave-Browser/policies/managed" \
  "$SUPPORT/Chromium/policies/managed"
do
  rm -f "$ROOT/unifai_guard.json" 2>/dev/null || true
done
OLD_DATA="$SUPPORT/UnifAI/Guard"
NEW_DATA="$SUPPORT/Raksha/Guard"
if [[ -d "$OLD_DATA" ]]; then
  # proxy_bundle targets the old app and must not be reused.
  if [[ -d "$NEW_DATA" ]] || { mkdir -p "$NEW_DATA" && rsync -a --exclude proxy_bundle "$OLD_DATA/" "$NEW_DATA/"; }; then
    rm -rf "$OLD_DATA"
    rmdir "$SUPPORT/UnifAI" 2>/dev/null || true
  fi
fi
rm -rf "$HOME/Applications/UnifAI_Guard.app" 2>/dev/null || true
MIGRATE

  USER_DATA="$USER_HOME/Library/Application Support/Raksha/Guard"
  mkdir -p "$USER_DATA" 2>/dev/null || true
  if [[ -f "$APP/Contents/Resources/raksha_guard_config.json" ]]; then
    cp -f "$APP/Contents/Resources/raksha_guard_config.json" "$USER_DATA/raksha_guard_config.json" 2>/dev/null || true
    chown -R "$CONSOLE_USER" "$USER_DATA" 2>/dev/null || true
  fi
  sudo -u "$CONSOLE_USER" open "$APP" || true
else
  open "$APP" || true
fi
exit 0
EOF
chmod +x "$SCRIPTS/postinstall"

PKG_OUT="release/Raksha_Guard_${VERSION}.pkg"
PKG_SETUP="release/Raksha_Guard_Setup.pkg"
COMPONENT="installer/Raksha_Guard_component.pkg"
rm -f "$COMPONENT" "$PKG_OUT" "$PKG_SETUP"

pkgbuild \
  --root "$STAGE" \
  --scripts "$SCRIPTS" \
  --identifier "com.raksha.guard" \
  --version "$VERSION" \
  --install-location "/" \
  "$COMPONENT"

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  productbuild \
    --package "$COMPONENT" \
    --identifier "com.raksha.guard.pkg" \
    --version "$VERSION" \
    --sign "$MACOS_SIGN_IDENTITY" \
    "$PKG_OUT"
else
  productbuild \
    --package "$COMPONENT" \
    --identifier "com.raksha.guard.pkg" \
    --version "$VERSION" \
    "$PKG_OUT"
  echo "WARNING: unsigned pkg (set MACOS_SIGN_IDENTITY for Developer ID Installer sign)"
fi

cp -f "$PKG_OUT" "$PKG_SETUP"
rm -f "$COMPONENT"
echo "SUCCESS: $PKG_OUT"
echo "SUCCESS: $PKG_SETUP"
ls -lh "$PKG_OUT" "$PKG_SETUP"
