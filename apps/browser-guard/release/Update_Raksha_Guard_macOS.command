#!/bin/bash
# ==============================================================================
# Raksha Guard - macOS Auto Updater
# Double-click (or run in Terminal) to update Guard to latest version from server
# ==============================================================================

set -euo pipefail

# ── Colors ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; GRAY='\033[0;37m'; RESET='\033[0m'

# ── Config (reads from raksha_guard_config.json next to this script) ──────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/raksha_guard_config.json"
BACKEND_URL=""
GUARD_SECRET=""

if [ -f "$CONFIG_FILE" ]; then
    BACKEND_URL=$(python3 -c "import json; d=json.load(open(r'''$CONFIG_FILE''')); print((d.get('backend_url') or '').rstrip('/'))" 2>/dev/null || true)
    GUARD_SECRET=$(python3 -c "import json; d=json.load(open(r'''$CONFIG_FILE''')); print((d.get('guard_secret') or '').strip())" 2>/dev/null || true)
fi

if [ -z "$BACKEND_URL" ]; then
    echo -e "${YELLOW}Enter server URL (e.g. https://unifai.yespanchi.com):${RESET}"
    read -r BACKEND_URL
    BACKEND_URL="${BACKEND_URL%/}"
fi

CURL_AUTH=()
if [ -n "$GUARD_SECRET" ]; then
    CURL_AUTH=(-H "X-Raksha-Guard-Key: $GUARD_SECRET")
else
    echo -e "${YELLOW}WARNING: guard_secret missing in config — download may fail if server requires it.${RESET}"
fi

echo ""
echo -e "${CYAN}============================================================${RESET}"
echo -e "${CYAN}  Raksha Guard Auto-Updater (macOS)${RESET}"
echo -e "${CYAN}  Server: $BACKEND_URL${RESET}"
echo -e "${CYAN}============================================================${RESET}"
echo ""

# ── Step 1: Check current version ────────────────────────────────────────────
CURRENT_VERSION="unknown"
VERSION_FILE="$SCRIPT_DIR/VERSION.txt"
APP_VERSION_CANDIDATES=(
    "$HOME/Applications/Raksha_Guard.app/Contents/Resources/VERSION.txt"
    "/Applications/Raksha_Guard.app/Contents/Resources/VERSION.txt"
    "$HOME/Applications/Raksha_Guard.app/Contents/Info.plist"
    "/Applications/Raksha_Guard.app/Contents/Info.plist"
    "$VERSION_FILE"
)

for f in "${APP_VERSION_CANDIDATES[@]}"; do
    if [ -f "$f" ]; then
        if [[ "$f" == *.plist ]]; then
            CURRENT_VERSION=$(/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" "$f" 2>/dev/null || true)
        else
            CURRENT_VERSION=$(tr -d '[:space:]\ufeff' < "$f" || true)
        fi
        if [ -n "$CURRENT_VERSION" ]; then
            break
        fi
    fi
done
echo -e "${YELLOW}Current version: $CURRENT_VERSION${RESET}"

# ── Step 2: Check server version via HTTP header ──────────────────────────────
echo -e "${GRAY}Checking server for latest version...${RESET}"
SERVER_VERSION=$(curl -sI --max-time 15 "${CURL_AUTH[@]}" \
    "$BACKEND_URL/api/browser-ai/setup/download-mac.zip" \
    | grep -i "x-raksha-guard-version" \
    | awk '{print $2}' | tr -d '[:space:]\r' || echo "")

if [ -z "$SERVER_VERSION" ]; then
    echo -e "${YELLOW}Could not fetch server version - proceeding with download anyway.${RESET}"
    SERVER_VERSION="latest"
else
    echo -e "${GREEN}Server version : $SERVER_VERSION${RESET}"
fi

if [ "$CURRENT_VERSION" = "$SERVER_VERSION" ] && [ "$SERVER_VERSION" != "unknown" ] && [ "$SERVER_VERSION" != "latest" ]; then
    echo ""
    echo -e "${GREEN}Already up to date! ($CURRENT_VERSION)${RESET}"
    echo "Press Enter to exit..."
    read -r
    exit 0
fi

# ── Step 3: Stop running Guard ─────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}Stopping Raksha Guard...${RESET}"
pkill -f "Raksha_Guard" 2>/dev/null || true
sleep 2

# ── Step 4: Download latest macOS package ─────────────────────────────────────
TEMP_ZIP="/tmp/Raksha_Guard_Update_macOS.zip"
echo -e "${GRAY}Downloading latest macOS package from server...${RESET}"

if ! curl -L --max-time 180 --progress-bar "${CURL_AUTH[@]}" \
    "$BACKEND_URL/api/browser-ai/setup/download-mac.zip" \
    -o "$TEMP_ZIP"; then
    echo -e "${RED}ERROR: Download failed.${RESET}"
    echo -e "${YELLOW}Please download manually from Browser AI → Setup.${RESET}"
    echo "Press Enter to exit..."
    read -r
    exit 1
fi
echo -e "${GREEN}Download complete! Extracting...${RESET}"

# ── Step 5: Extract and Install ───────────────────────────────────────────────
TEMP_DIR="/tmp/Raksha_Guard_Update_macOS"
rm -rf "$TEMP_DIR"
mkdir -p "$TEMP_DIR"
unzip -q "$TEMP_ZIP" -d "$TEMP_DIR"
rm -f "$TEMP_ZIP"

echo -e "${YELLOW}Installing...${RESET}"

install_app_bundle() {
    local APP="$1"
    rm -rf "$HOME/Applications/Raksha_Guard.app" 2>/dev/null || true
    rm -rf "/Applications/Raksha_Guard.app" 2>/dev/null || true
    mkdir -p "$HOME/Applications"
    if cp -R "$APP" "$HOME/Applications/" 2>/dev/null; then
        DEST="$HOME/Applications/Raksha_Guard.app"
    else
        cp -R "$APP" "/Applications/"
        DEST="/Applications/Raksha_Guard.app"
    fi
    # Ensure VERSION.txt exists for next update check
    mkdir -p "$DEST/Contents/Resources"
    if [ "$SERVER_VERSION" != "latest" ] && [ -n "$SERVER_VERSION" ]; then
        printf '%s\n' "$SERVER_VERSION" > "$DEST/Contents/Resources/VERSION.txt"
    elif [ -f "$TEMP_DIR/VERSION.txt" ]; then
        cp "$TEMP_DIR/VERSION.txt" "$DEST/Contents/Resources/VERSION.txt"
    fi
    # Carry fleet config if present beside updater
    if [ -f "$CONFIG_FILE" ]; then
        cp "$CONFIG_FILE" "$DEST/Contents/Resources/raksha_guard_config.json" 2>/dev/null || true
    fi
    xattr -dr com.apple.quarantine "$DEST" 2>/dev/null || true
    echo -e "${GREEN}App installed at $DEST${RESET}"
}

if find "$TEMP_DIR" -name "*.pkg" | grep -q .; then
    PKG=$(find "$TEMP_DIR" -name "*.pkg" | head -1)
    echo "Found PKG installer: $PKG"
    sudo installer -pkg "$PKG" -target /
elif find "$TEMP_DIR" -name "Raksha_Guard.app" | grep -q .; then
    APP=$(find "$TEMP_DIR" -name "Raksha_Guard.app" | head -1)
    echo "Found .app bundle"
    install_app_bundle "$APP"
elif find "$TEMP_DIR" -name "Install_Raksha_Guard.command" | grep -q .; then
    INSTALL_CMD=$(find "$TEMP_DIR" -name "Install_Raksha_Guard.command" | head -1)
    chmod +x "$INSTALL_CMD"
    bash "$INSTALL_CMD"
else
    echo -e "${RED}ERROR: No installer found in downloaded package.${RESET}"
    echo "Contents: $(ls "$TEMP_DIR")"
    exit 1
fi

# ── Step 6: Cleanup ───────────────────────────────────────────────────────────
rm -rf "$TEMP_DIR"

# ── Step 7: Restart Guard ─────────────────────────────────────────────────────
echo ""
echo -e "${YELLOW}Starting Raksha Guard...${RESET}"

GUARD_APP=""
for loc in "$HOME/Applications/Raksha_Guard.app" "/Applications/Raksha_Guard.app"; do
    if [ -d "$loc" ]; then
        GUARD_APP="$loc"
        break
    fi
done

if [ -n "$GUARD_APP" ]; then
    open "$GUARD_APP"
    echo -e "${GREEN}Guard started!${RESET}"
else
    echo -e "${YELLOW}Guard app not found in standard locations — please start manually.${RESET}"
fi

echo ""
echo -e "${CYAN}============================================================${RESET}"
echo -e "${GREEN}  Update Complete! $CURRENT_VERSION → $SERVER_VERSION${RESET}"
echo -e "${CYAN}============================================================${RESET}"
echo ""
echo "Press Enter to exit..."
read -r
