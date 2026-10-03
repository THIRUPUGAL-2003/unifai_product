#!/bin/bash
# Double-click or run in Terminal to install Raksha Guard on this Mac.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
APP_SRC="$DIR/Raksha_Guard.app"
DEST="/Applications/Raksha_Guard.app"

echo "============================================================"
echo " Raksha Guard — macOS install"
echo "============================================================"

if [[ ! -d "$APP_SRC" ]]; then
  osascript -e 'display dialog "Raksha_Guard.app not found next to this installer.\nUnzip Raksha_Guard_macOS.zip fully, then run Install_Raksha_Guard.command again." with title "Raksha Guard" buttons {"OK"} default button 1 with icon stop' || true
  echo "ERROR: Missing $APP_SRC"
  exit 1
fi

# Clear quarantine so Gatekeeper does not block unsigned / first-run apps from ZIP
xattr -dr com.apple.quarantine "$APP_SRC" 2>/dev/null || true

# Stop existing running Guard instances and unload LaunchAgent
PLIST="$HOME/Library/LaunchAgents/com.raksha.guard.plist"
if [[ -f "$PLIST" ]]; then
  launchctl unload "$PLIST" 2>/dev/null || true
fi
pkill -f "Raksha_Guard.app/Contents/MacOS/Raksha_Guard" 2>/dev/null || true
pkill -f "/MacOS/Raksha_Guard" 2>/dev/null || true
sleep 1

# Try /Applications first; if permission denied, fallback to ~/Applications
echo "Copying to /Applications ..."
if [[ -d "$DEST" ]]; then
  rm -rf "$DEST" 2>/dev/null || true
fi

if ! cp -R "$APP_SRC" "$DEST" 2>/dev/null; then
  echo "/Applications write not permitted, falling back to $HOME/Applications ..."
  DEST="$HOME/Applications/Raksha_Guard.app"
  mkdir -p "$HOME/Applications"
  rm -rf "$DEST" 2>/dev/null || true
  cp -R "$APP_SRC" "$DEST"
fi

xattr -dr com.apple.quarantine "$DEST" 2>/dev/null || true
if [[ -f "$DEST/Contents/MacOS/Raksha_Guard" ]]; then
  chmod +x "$DEST/Contents/MacOS/Raksha_Guard"
fi

# Prefer config from package next to installer; place in both Resources and per-user data dir
if [[ -f "$DIR/raksha_guard_config.json" ]]; then
  mkdir -p "$DEST/Contents/Resources"
  cp "$DIR/raksha_guard_config.json" "$DEST/Contents/Resources/raksha_guard_config.json"

  USER_DATA="$HOME/Library/Application Support/Raksha/Guard"
  mkdir -p "$USER_DATA"
  cp "$DIR/raksha_guard_config.json" "$USER_DATA/raksha_guard_config.json"
fi

echo "Starting Guard at $DEST ..."
open "$DEST"

osascript -e 'display dialog "Raksha Guard installed successfully.\n\n1) Fully quit Safari/Chrome/Edge/Firefox\n2) Reopen browsers\n3) Visit a monitored AI site\n\nHealth: http://127.0.0.1:18195/\n\nTo turn OFF / uninstall: run Uninstall_Raksha_Guard.command" with title "Raksha Guard" buttons {"OK"} default button 1 with icon note' || true

echo "Done. Health check: open http://127.0.0.1:18195"
echo "Off / uninstall: double-click Uninstall_Raksha_Guard.command"

