#!/bin/bash
# Double-click or run in Terminal to install YP Raksha on this Mac.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
APP_SRC="$DIR/YP_Raksha.app"
DEST="/Applications/YP_Raksha.app"

echo "============================================================"
echo " YP Raksha — macOS install"
echo "============================================================"

if [[ ! -d "$APP_SRC" ]]; then
  if [[ -d "$DIR/Gateway_Guard.app" ]]; then
    APP_SRC="$DIR/Gateway_Guard.app"
  else
    osascript -e 'display dialog "YP_Raksha.app not found next to this installer.\nUnzip YP_Raksha_macOS.zip fully, then run Install_YP_Raksha.command again." with title "YP Raksha" buttons {"OK"} default button 1 with icon stop' || true
    echo "ERROR: Missing $APP_SRC"
    exit 1
  fi
fi

# Clean previous legacy builds (UnifAI Guard / Gateway Guard)
remove_previous_build() {
  local OLD_PLIST="$HOME/Library/LaunchAgents/com.unifai.guard.plist"
  if [[ -f "$OLD_PLIST" ]]; then
    launchctl unload "$OLD_PLIST" 2>/dev/null || true
    rm -f "$OLD_PLIST"
  fi
  local OLD_PLIST2="$HOME/Library/LaunchAgents/com.gateway.guard.plist"
  if [[ -f "$OLD_PLIST2" ]]; then
    launchctl unload "$OLD_PLIST2" 2>/dev/null || true
    rm -f "$OLD_PLIST2"
  fi
  pkill -f "UnifAI_Guard.app/Contents/MacOS/UnifAI_Guard" 2>/dev/null || true
  pkill -f "/MacOS/UnifAI_Guard" 2>/dev/null || true
  pkill -f "Gateway_Guard.app/Contents/MacOS/Gateway_Guard" 2>/dev/null || true
  pkill -f "/MacOS/Gateway_Guard" 2>/dev/null || true

  local OLD_APP
  for OLD_APP in "/Applications/UnifAI_Guard.app" "$HOME/Applications/UnifAI_Guard.app" "/Applications/Gateway_Guard.app" "$HOME/Applications/Gateway_Guard.app"; do
    if [[ -d "$OLD_APP" ]]; then
      echo "Removing previous build $OLD_APP"
      rm -rf "$OLD_APP" 2>/dev/null || true
    fi
  done
}

# Clear quarantine so Gatekeeper does not block unsigned / first-run apps from ZIP
xattr -dr com.apple.quarantine "$APP_SRC" 2>/dev/null || true

remove_previous_build

# Stop existing running YP Raksha instances and unload LaunchAgent
PLIST="$HOME/Library/LaunchAgents/com.yespanchi.raksha.plist"
if [[ -f "$PLIST" ]]; then
  launchctl unload "$PLIST" 2>/dev/null || true
fi
pkill -f "YP_Raksha" 2>/dev/null || true
sleep 1

# Try /Applications first; if permission denied, fallback to ~/Applications
echo "Copying to /Applications ..."
if [[ -d "$DEST" ]]; then
  chflags -R nouchg "$DEST" 2>/dev/null || true
  rm -rf "$DEST" 2>/dev/null || true
fi

if ! cp -R "$APP_SRC" "$DEST" 2>/dev/null; then
  echo "/Applications write not permitted, falling back to $HOME/Applications ..."
  DEST="$HOME/Applications/YP_Raksha.app"
  mkdir -p "$HOME/Applications"
  chflags -R nouchg "$DEST" 2>/dev/null || true
  rm -rf "$DEST" 2>/dev/null || true
  cp -R "$APP_SRC" "$DEST"
fi

xattr -dr com.apple.quarantine "$DEST" 2>/dev/null || true
if [[ -f "$DEST/Contents/MacOS/YP_Raksha" ]]; then
  chmod +x "$DEST/Contents/MacOS/YP_Raksha"
elif [[ -f "$DEST/Contents/MacOS/Gateway_Guard" ]]; then
  chmod +x "$DEST/Contents/MacOS/Gateway_Guard"
fi

# Prefer config from package next to installer; place in both Resources and per-user data dir
if [[ -f "$DIR/gateway_guard_config.json" ]]; then
  mkdir -p "$DEST/Contents/Resources"
  cp "$DIR/gateway_guard_config.json" "$DEST/Contents/Resources/gateway_guard_config.json"

  USER_DATA="$HOME/Library/Application Support/Gateway/Guard"
  mkdir -p "$USER_DATA"
  cp "$DIR/gateway_guard_config.json" "$USER_DATA/gateway_guard_config.json"
fi

echo "Starting YP Raksha at $DEST ..."
open "$DEST"

osascript -e 'display dialog "YP Raksha installed successfully.\n\n1) Fully quit Safari/Chrome/Edge/Firefox\n2) Reopen browsers\n3) Visit a monitored AI site\n\nHealth: http://127.0.0.1:18195/\n\nTo turn OFF / uninstall: run Uninstall_YP_Raksha.command" with title "YP Raksha" buttons {"OK"} default button 1 with icon note' || true

echo "Done. Health check: open http://127.0.0.1:18195"
echo "Off / uninstall: double-click Uninstall_YP_Raksha.command"
