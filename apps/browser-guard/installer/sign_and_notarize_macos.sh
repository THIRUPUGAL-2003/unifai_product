#!/usr/bin/env bash
# Sign + notarize UnifAI Guard .app and refresh the employee ZIP / optional .pkg.
# Requires Apple Developer ID + notary credentials.
#
# Required env:
#   MACOS_APP_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)"
#   APPLE_ID="you@example.com"
#   APPLE_TEAM_ID="TEAMID"
#   APPLE_APP_SPECIFIC_PASSWORD="xxxx-xxxx-xxxx-xxxx"   # or use keychain profile
#
# Optional:
#   NOTARY_PROFILE="unifai-notary"   # if you ran: xcrun notarytool store-credentials
#   MACOS_SIGN_IDENTITY="Developer ID Installer: ..."  # for .pkg
#
# Run on a Mac after ./installer/build_macos.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "ERROR: sign/notarize must run on macOS"
  exit 1
fi

APP="release/UnifAI_Guard.app"
ZIP="release/UnifAI_Guard_macOS.zip"
VERSION="$(tr -d '[:space:]\ufeff' < release/VERSION.txt 2>/dev/null || echo "0.0.0")"

if [[ ! -d "$APP" ]]; then
  echo "ERROR: $APP missing — run ./installer/build_macos.sh first"
  exit 1
fi
if [[ -z "${MACOS_APP_SIGN_IDENTITY:-}" ]]; then
  echo "ERROR: set MACOS_APP_SIGN_IDENTITY (Developer ID Application: ...)"
  exit 1
fi

echo "1) codesign .app (hardened runtime)"
codesign --force --deep --options runtime \
  --sign "$MACOS_APP_SIGN_IDENTITY" \
  "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

echo "2) zip for notarization"
NOTARY_ZIP="release/UnifAI_Guard_notarize.zip"
rm -f "$NOTARY_ZIP"
ditto -c -k --keepParent "$APP" "$NOTARY_ZIP"

echo "3) notarize"
if [[ -n "${NOTARY_PROFILE:-}" ]]; then
  xcrun notarytool submit "$NOTARY_ZIP" --keychain-profile "$NOTARY_PROFILE" --wait
else
  : "${APPLE_ID:?set APPLE_ID}"
  : "${APPLE_TEAM_ID:?set APPLE_TEAM_ID}"
  : "${APPLE_APP_SPECIFIC_PASSWORD:?set APPLE_APP_SPECIFIC_PASSWORD}"
  xcrun notarytool submit "$NOTARY_ZIP" \
    --apple-id "$APPLE_ID" \
    --team-id "$APPLE_TEAM_ID" \
    --password "$APPLE_APP_SPECIFIC_PASSWORD" \
    --wait
fi

echo "4) staple"
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"

echo "5) rebuild employee ZIP (signed+stapled app)"
STAGE="installer/staging-mac"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/UnifAI_Guard.app"
cp config/unifai_guard_config.json "$STAGE/unifai_guard_config.json"
cp installer/EMPLOYEE_README_MAC.txt "$STAGE/EMPLOYEE_README_MAC.txt" 2>/dev/null || true
cp release/INSTALL_MACOS.txt "$STAGE/INSTALL_MACOS.txt"
cp release/UNINSTALL_MACOS.txt "$STAGE/UNINSTALL_MACOS.txt"
cp installer/Install_UnifAI_Guard.command "$STAGE/Install_UnifAI_Guard.command"
cp installer/Uninstall_UnifAI_Guard.command "$STAGE/Uninstall_UnifAI_Guard.command"
chmod +x "$STAGE/Install_UnifAI_Guard.command" "$STAGE/Uninstall_UnifAI_Guard.command"
rm -f "$ZIP"
(
  cd "$STAGE"
  zip -r -y "$ZIP" \
    UnifAI_Guard.app \
    unifai_guard_config.json \
    EMPLOYEE_README_MAC.txt \
    INSTALL_MACOS.txt \
    UNINSTALL_MACOS.txt \
    Install_UnifAI_Guard.command \
    Uninstall_UnifAI_Guard.command
)

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  echo "6) build signed .pkg"
  ./installer/build_pkg_macos.sh
fi

rm -f "$NOTARY_ZIP"
echo "SUCCESS — signed+notarized v${VERSION}"
echo "  App: $APP"
echo "  ZIP: $ZIP"
