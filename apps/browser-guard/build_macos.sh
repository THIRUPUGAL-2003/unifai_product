#!/usr/bin/env bash
# Build YP-Raksha macOS Standalone .app and Package
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

echo "============================================================"
echo " 1/2: Syncing configuration from root .env..."
echo "============================================================"
python3 scripts/sync_config_from_env.py

echo ""
echo "============================================================"
echo " 2/2: Building macOS Package..."
echo "============================================================"
bash installer/build_macos.sh

echo ""
echo "============================================================"
echo " SUCCESS: macOS package ready at release/YP_Raksha_macOS.zip"
echo "============================================================"
