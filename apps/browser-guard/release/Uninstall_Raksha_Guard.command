#!/bin/bash
# Double-click or run in Terminal to turn OFF + uninstall Raksha Guard on this Mac.
# Asks for the company uninstall key (same as Windows), then clears PAC / LaunchAgent / app.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

find_guard_bin() {
  local candidates=(
    "/Applications/Raksha_Guard.app/Contents/MacOS/Raksha_Guard"
    "$DIR/Raksha_Guard.app/Contents/MacOS/Raksha_Guard"
    "$HOME/Applications/Raksha_Guard.app/Contents/MacOS/Raksha_Guard"
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
echo " Raksha Guard — macOS uninstall / turn OFF"
echo "============================================================"
echo "This will:"
echo "  1) Verify company uninstall key with Raksha backend"
echo "  2) Clear system Auto Proxy (PAC) + browser helper state"
echo "  3) Remove LaunchAgent (no more start at login)"
echo "  4) Stop Guard process and delete Raksha_Guard.app"
echo ""

BIN="$(find_guard_bin || true)"

CODE=0
if [[ -n "${BIN}" ]]; then
  # Same flow as Windows Inno: --uninstall-prompt (key dialog via osascript)
  set +e
  export RAKSHA_WRAPPER_UI=1
  "$BIN" --uninstall-prompt
  CODE=$?
  set -e
else
  # Binary not in standard path: still require key verification with backend
  BACKEND_URL="https://unifai.yespanchi.com"
  CFG="$HOME/Library/Application Support/Raksha/Guard/raksha_guard_config.json"
  if [[ -f "$CFG" ]]; then
    B_URL=$(grep -o '"backend_url": *"[^"]*"' "$CFG" | cut -d'"' -f4 || true)
    if [[ -n "$B_URL" ]]; then BACKEND_URL="$B_URL"; fi
  fi
  ID_FILE="$HOME/Library/Application Support/Raksha/Guard/agent_id.txt"
  AGENT_ID=""
  if [[ -f "$ID_FILE" ]]; then AGENT_ID="$(cat "$ID_FILE" | tr -d ' \r\n')"; fi

  KEY=$(osascript -e '
  try
    set r to display dialog "Enter company or device uninstall key provided by your administrator to uninstall Raksha Guard:" default answer "" with title "Raksha Guard Uninstall" buttons {"Cancel", "Uninstall"} default button "Uninstall"
    return text returned of r
  on error
    return "__CANCEL__"
  end try' 2>/dev/null || echo "__CANCEL__")

  if [[ "$KEY" == "__CANCEL__" ]]; then
    CODE=3
  elif [[ -z "$KEY" ]]; then
    CODE=2
  else
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BACKEND_URL}/api/browser-ai/agents/uninstall" -H "Content-Type: application/json" -d "{\"agent_id\":\"$AGENT_ID\",\"key\":\"$KEY\"}" 2>/dev/null || echo "0")
    if [[ "$HTTP_CODE" == "200" ]]; then
      CODE=0
    elif [[ "$HTTP_CODE" == "403" ]]; then
      CODE=2
    else
      CODE=1
    fi
  fi
fi

if [[ "$CODE" -eq 3 ]]; then
  echo "Cancelled by user."
  osascript -e 'display dialog "Uninstall cancelled." with title "Raksha Guard" buttons {"OK"} default button 1' || true
  exit 3
fi
if [[ "$CODE" -eq 2 ]]; then
  echo "Invalid uninstall key."
  osascript -e 'display dialog "Invalid company uninstall key.\nGet the key from Browser AI → Setup (IT)." with title "Raksha Guard" buttons {"OK"} default button 1 with icon stop' || true
  exit 2
fi
if [[ "$CODE" -ne 0 ]]; then
  echo "Uninstall rejected (exit $CODE). PAC left on."
  osascript -e "display dialog \"Uninstall failed (code $CODE).\nBackend may be unreachable or key rejected.\nProxy settings were NOT cleared.\" with title \"Raksha Guard\" buttons {\"OK\"} default button 1 with icon stop" || true
  exit "$CODE"
fi

# Stop any running instance
PLIST="$HOME/Library/LaunchAgents/com.raksha.guard.plist"
if [[ -f "$PLIST" ]]; then
  launchctl unload "$PLIST" 2>/dev/null || true
  rm -f "$PLIST"
fi
pkill -f "Raksha_Guard.app/Contents/MacOS/Raksha_Guard" 2>/dev/null || true
pkill -f "/MacOS/Raksha_Guard" 2>/dev/null || true
sleep 1

# Remove installed app copies
for APP in \
  "/Applications/Raksha_Guard.app" \
  "$HOME/Applications/Raksha_Guard.app" \
  "$DIR/Raksha_Guard.app"
do
  if [[ -d "$APP" ]]; then
    echo "Removing $APP"
    rm -rf "$APP"
  fi
done

# Clear system Auto Proxy (PAC) state on all macOS network services
while IFS= read -r SERVICE; do
  if [[ -n "$SERVICE" && ! "$SERVICE" =~ ^\* ]]; then
    networksetup -setautoproxystate "$SERVICE" off 2>/dev/null || true
  fi
done < <(networksetup -listallnetworkservices 2>/dev/null | tail -n +2)

# Remove browser policies
SUPPORT="$HOME/Library/Application Support"
for ROOT in \
  "$SUPPORT/Google/Chrome/policies/managed" \
  "$SUPPORT/Google/Chrome Canary/policies/managed" \
  "$SUPPORT/Microsoft Edge/policies/managed" \
  "$SUPPORT/BraveSoftware/Brave-Browser/policies/managed" \
  "$SUPPORT/Chromium/policies/managed"
do
  rm -f "$ROOT/raksha_guard.json" 2>/dev/null || true
done

# Clean up application data directory
rm -rf "$HOME/Library/Application Support/Raksha" 2>/dev/null || true

osascript -e 'display dialog "Raksha Guard is OFF and successfully uninstalled.\n\nBrowser protection has been disabled and files removed.\nPlease fully quit and reopen your browsers." with title "Raksha Guard" buttons {"OK"} default button 1 with icon note' || true

echo "Done. Guard stopped, PAC proxy cleared, app and data removed."
