#!/bin/bash

echo "===================================================="
echo "🚀 Starting UnifAI & AI Guard (system-wide)"
echo "===================================================="

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
REPO_ROOT="$( cd "$SCRIPT_DIR/../.." && pwd )"
PROXY_SCRIPT="$REPO_ROOT/apps/browser-guard/proxy/browser_ai_proxy.py"
cd "$REPO_ROOT"

# Load .env — ports/domain from .env only (no hardcoded fallbacks).
if [[ -f "$REPO_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source <(grep -E '^(UNIFAI_PROXY_ADDR|PROXY_PORT|PAC_HTTP_PORT|SERVER_DOMAIN|APP_PORT|MITM_WEB_PORT)=' "$REPO_ROOT/.env" | sed 's/\r$//')
  set +a
fi
PROXY_HOST_PORT="${UNIFAI_PROXY_ADDR:?set UNIFAI_PROXY_ADDR in .env}"
PROXY_PORT_ONLY="${PROXY_HOST_PORT##*:}"
MITM_WEB_PORT="${MITM_WEB_PORT:?set MITM_WEB_PORT in .env}"
APP_PORT="${APP_PORT:?set APP_PORT in .env}"

# 0. Check if mitmproxy/mitmweb is installed
if ! command -v mitmweb &> /dev/null; then
    echo "❌ Error: mitmweb is not installed or not in PATH."
    echo "   Please install mitmproxy (e.g. pip install mitmproxy or winget install mitmproxy)"
    exit 1
fi

WIN_USERPROFILE=$(powershell.exe -NoProfile -Command '[Environment]::GetFolderPath("UserProfile")' | tr -d '\r')
MITM_CERT_CER="${WIN_USERPROFILE}\\.mitmproxy\\mitmproxy-ca-cert.cer"

# 1. Trust mitmproxy CA in Current User store (required for HTTPS MITM)
echo "1. Ensuring mitmproxy CA is trusted (Current User)..."
if [[ -f "$HOME/.mitmproxy/mitmproxy-ca-cert.cer" ]]; then
    certutil.exe -user -addstore Root "$MITM_CERT_CER" >/dev/null 2>&1 || true
    echo "   CA trust OK (Chrome / Edge / Brave)"
else
    echo "   ⚠️  CA not found yet — will be created when mitmweb starts."
    echo "   Re-run ./start_app.sh once after first start if HTTPS sites warn."
fi

# 2. Start mitmweb
echo "2. Starting Proxy Interceptor (Proxy Port: ${PROXY_PORT_ONLY}, Web UI Port: ${MITM_WEB_PORT})..."
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "
Get-CimInstance Win32_Process |
  Where-Object { \$_.CommandLine -match 'mitmweb|mitmdump' } |
  ForEach-Object { Stop-Process -Id \$_.ProcessId -Force -ErrorAction SilentlyContinue }
" 2>/dev/null
sleep 1

mitmweb -p "${PROXY_PORT_ONLY}" --web-host 127.0.0.1 --web-port "${MITM_WEB_PORT}" \
  -s "$PROXY_SCRIPT" \
  --set block_global=false \
  --set connection_strategy=lazy \
  > "$SCRIPT_DIR/mitmweb.log" 2>&1 &
PROXY_PID=$!
echo "   Proxy running (PID: $PROXY_PID) on ${PROXY_HOST_PORT}"
sleep 3

# Re-try CA install after mitmweb may have created ~/.mitmproxy
if [[ -f "$HOME/.mitmproxy/mitmproxy-ca-cert.cer" ]]; then
    certutil.exe -user -addstore Root "$MITM_CERT_CER" >/dev/null 2>&1 || true
fi

# 3. Force Windows SYSTEM proxy (not file:// PAC — Chrome ignores file PAC)
echo "3. Enabling Windows system proxy → ${PROXY_HOST_PORT} ..."
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "
\$path = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
Remove-ItemProperty -Path \$path -Name AutoConfigURL -ErrorAction SilentlyContinue
Set-ItemProperty -Path \$path -Name ProxyEnable -Value 1
Set-ItemProperty -Path \$path -Name ProxyServer -Value '${PROXY_HOST_PORT}'
Set-ItemProperty -Path \$path -Name ProxyOverride -Value 'localhost;127.0.0.1;*.local;<local>'
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public class WinInetFlush {
  [DllImport(\"wininet.dll\", SetLastError=true)]
  public static extern bool InternetSetOption(IntPtr hInternet, int dwOption, IntPtr lpBuffer, int dwBufferLength);
}
'@
[void][WinInetFlush]::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0)
[void][WinInetFlush]::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0)
netsh winhttp import proxy source=ie | Out-Null
Write-Host '   ProxyEnable=1 ProxyServer=${PROXY_HOST_PORT}'
"

# 4. Verify proxy accepts CONNECT (otherwise guard cannot see HTTPS)
echo "4. Verifying proxy path..."
VERIFY=$(powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "
try {
  \$proxy = New-Object System.Net.WebProxy('http://${PROXY_HOST_PORT}', \$true)
  \$wc = New-Object System.Net.WebClient
  \$wc.Proxy = \$proxy
  \$null = \$wc.DownloadString('http://mitm.it')
  'OK'
} catch {
  'FAIL: ' + \$_.Exception.Message
}
" | tr -d '\r')
echo "   Proxy check: $VERIFY"

# 5. Open dashboard in default browser (bypasses proxy via ProxyOverride)
echo "5. Opening Dashboard & Proxy Monitor..."
cmd.exe //c start "" "http://localhost:${APP_PORT}/workspace/browser-ai" >/dev/null 2>&1 &
cmd.exe //c start "" "http://127.0.0.1:${MITM_WEB_PORT}" >/dev/null 2>&1 &

echo "===================================================="
echo "✅ AI Guard is ON for this laptop"
echo "   Dashboard: http://localhost:${APP_PORT}/workspace/browser-ai"
echo "   Proxy:     ${PROXY_HOST_PORT}"
echo "   Proxy UI:  http://127.0.0.1:${MITM_WEB_PORT}  ← must show flows when you browse"
echo ""
echo "⚠️  REQUIRED: Fully quit Chrome/Edge (all windows), then reopen."
echo "   Already-open browsers keep the old (no-proxy) settings."
echo ""
echo "   Test: open a monitored Target Website, send a prompt — Proxy UI should"
echo "   show that site's flows, and Prompt Logs should increase."
echo ""
echo "   When done:  ./stop_app.sh"
echo "===================================================="
