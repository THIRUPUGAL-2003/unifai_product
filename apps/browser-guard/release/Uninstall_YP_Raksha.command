#!/bin/bash
# Double-click or run in Terminal to turn OFF + uninstall Gateway Guard on this Mac.
# Asks for the company uninstall key (same as Windows), then clears PAC / LaunchAgent / app.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

find_guard_bin() {
  local candidates=(
    "/Applications/Gateway_Guard.app/Contents/MacOS/Gateway_Guard"
    "$DIR/Gateway_Guard.app/Contents/MacOS/Gateway_Guard"
    "$HOME/Applications/Gateway_Guard.app/Contents/MacOS/Gateway_Guard"
  )
  local c
  for c in "${candidates[@]}"; do
    if [[ -x "$c" ]]; then
      echo "$c"
      return 0
    fi
  done
  return 1
}

echo "============================================================"
echo " Gateway Guard — macOS uninstall / turn OFF"
echo "============================================================"
echo "This will:"
echo "  1) Verify company uninstall key with Gateway backend"
echo "  2) Clear system Auto Proxy (PAC) + browser helper state"
echo "  3) Remove LaunchAgent (no more start at login)"
echo "  4) Stop Guard process and delete Gateway_Guard.app"
echo ""

BIN="$(find_guard_bin || true)"
if [[ -z "${BIN}" ]]; then
  osascript -e 'display dialog "Gateway_Guard.app not found.\nWill still try to clear LaunchAgent and leftover proxy settings if possible." with title "Gateway Guard" buttons {"OK"} default button 1 with icon caution' || true
fi

CODE=0
if [[ -n "${BIN}" ]]; then
  set +e
  if [[ -n "${1:-}" ]]; then
    "$BIN" --uninstall "$1"
  else
    "$BIN" --uninstall-prompt
  fi
  CODE=$?
  set -e
else
  # No binary — still clear local autostart / try to kill leftovers
  CODE=0
fi

if [[ "$CODE" -eq 3 ]]; then
  echo "Cancelled by user."
  osascript -e 'display dialog "Uninstall cancelled." with title "Gateway Guard" buttons {"OK"} default button 1' || true
  exit 3
fi
if [[ "$CODE" -eq 2 ]]; then
  echo "Invalid uninstall key."
  osascript -e 'display dialog "Invalid company uninstall key.\nGet the key from Browser AI → Setup (IT)." with title "Gateway Guard" buttons {"OK"} default button 1 with icon stop' || true
  exit 2
fi
if [[ "$CODE" -ne 0 ]]; then
  echo "Uninstall rejected (exit $CODE). PAC left on."
  osascript -e "display dialog \"Uninstall failed (code $CODE).\nBackend may be unreachable or key rejected.\nProxy settings were NOT cleared.\" with title \"Gateway Guard\" buttons {\"OK\"} default button 1 with icon stop" || true
  exit "$CODE"
fi

# Stop any running instance
PLIST="$HOME/Library/LaunchAgents/com.gateway.guard.plist"
if [[ -f "$PLIST" ]]; then
  launchctl unload "$PLIST" 2>/dev/null || true
  rm -f "$PLIST"
fi
pkill -f "Gateway_Guard.app/Contents/MacOS/Gateway_Guard" 2>/dev/null || true
pkill -f "/MacOS/Gateway_Guard" 2>/dev/null || true
sleep 1

# Remove installed app copies
for APP in \
  "/Applications/Gateway_Guard.app" \
  "$HOME/Applications/Gateway_Guard.app" \
  "$DIR/Gateway_Guard.app"
do
  if [[ -d "$APP" ]]; then
    echo "Removing $APP"
    chflags -R nouchg "$APP" 2>/dev/null || true
    rm -rf "$APP"
  fi
done

# Optional: keep logs for IT — do NOT wipe data dir by default
# Data left at: ~/Library/Application Support/Gateway/Guard

osascript -e 'display dialog "Gateway Guard is OFF and uninstalled.\n\nFully quit and reopen browsers.\nLogs (if IT needs them) remain under:\n~/Library/Application Support/Gateway/Guard" with title "Gateway Guard" buttons {"OK"} default button 1 with icon note' || true

echo "Done. Guard stopped, PAC cleared, app removed."
echo "Optional cleanup: rm -rf \"$HOME/Library/Application Support/Gateway/Guard\""
