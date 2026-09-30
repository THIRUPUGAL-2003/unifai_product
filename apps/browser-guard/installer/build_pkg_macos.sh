#!/usr/bin/env bash
# Build UnifAI Guard .pkg for macOS (product-style installer).
# Run on a Mac AFTER ./installer/build_macos.sh (needs release/UnifAI_Guard.app).
#
# Optional signing:
#   export MACOS_SIGN_IDENTITY="Developer ID Installer: Your Name (TEAMID)"
#   ./installer/build_pkg_macos.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]\ufeff' < release/VERSION.txt 2>/dev/null || echo "0.0.0")"
APP="release/UnifAI_Guard.app"
if [[ ! -d "$APP" ]]; then
  echo "ERROR: $APP missing — run ./installer/build_macos.sh first"
  exit 1
fi

STAGE="installer/pkg-root"
SCRIPTS="installer/pkg-scripts"
rm -rf "$STAGE" "$SCRIPTS"
mkdir -p "$STAGE/Applications" "$SCRIPTS"

cp -R "$APP" "$STAGE/Applications/UnifAI_Guard.app"
if [[ -f config/unifai_guard_config.json ]]; then
  mkdir -p "$STAGE/Applications/UnifAI_Guard.app/Contents/Resources"
  cp config/unifai_guard_config.json "$STAGE/Applications/UnifAI_Guard.app/Contents/Resources/unifai_guard_config.json"
fi

# postinstall: terminate old instance, clear quarantine, set permissions, open app for logged-in user
cat > "$SCRIPTS/postinstall" <<'EOF'
#!/bin/bash
set -euo pipefail
pkill -f "UnifAI_Guard.app/Contents/MacOS/UnifAI_Guard" 2>/dev/null || true
pkill -f "/MacOS/UnifAI_Guard" 2>/dev/null || true
sleep 1

APP="/Applications/UnifAI_Guard.app"
xattr -dr com.apple.quarantine "$APP" 2>/dev/null || true
if [[ -f "$APP/Contents/MacOS/UnifAI_Guard" ]]; then
  chmod +x "$APP/Contents/MacOS/UnifAI_Guard" 2>/dev/null || true
fi

CONSOLE_USER=$(stat -f "%Su" /dev/console 2>/dev/null || echo "")
if [[ -n "$CONSOLE_USER" && "$CONSOLE_USER" != "root" ]]; then
  USER_DATA="/Users/$CONSOLE_USER/Library/Application Support/UnifAI/Guard"
  mkdir -p "$USER_DATA" 2>/dev/null || true
  if [[ -f "$APP/Contents/Resources/unifai_guard_config.json" ]]; then
    cp -f "$APP/Contents/Resources/unifai_guard_config.json" "$USER_DATA/unifai_guard_config.json" 2>/dev/null || true
    chown -R "$CONSOLE_USER" "$USER_DATA" 2>/dev/null || true
  fi
  sudo -u "$CONSOLE_USER" open "$APP" || true
else
  open "$APP" || true
fi
exit 0
EOF
chmod +x "$SCRIPTS/postinstall"

PKG_OUT="release/UnifAI_Guard_${VERSION}.pkg"
PKG_SETUP="release/UnifAI_Guard_Setup.pkg"
COMPONENT="installer/UnifAI_Guard_component.pkg"
rm -f "$COMPONENT" "$PKG_OUT" "$PKG_SETUP"

pkgbuild \
  --root "$STAGE" \
  --scripts "$SCRIPTS" \
  --identifier "com.unifai.guard" \
  --version "$VERSION" \
  --install-location "/" \
  "$COMPONENT"

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  productbuild \
    --package "$COMPONENT" \
    --identifier "com.unifai.guard.pkg" \
    --version "$VERSION" \
    --sign "$MACOS_SIGN_IDENTITY" \
    "$PKG_OUT"
else
  productbuild \
    --package "$COMPONENT" \
    --identifier "com.unifai.guard.pkg" \
    --version "$VERSION" \
    "$PKG_OUT"
  echo "WARNING: unsigned pkg (set MACOS_SIGN_IDENTITY for Developer ID Installer sign)"
fi

cp -f "$PKG_OUT" "$PKG_SETUP"
rm -f "$COMPONENT"
echo "SUCCESS: $PKG_OUT"
echo "SUCCESS: $PKG_SETUP"
ls -lh "$PKG_OUT" "$PKG_SETUP"
