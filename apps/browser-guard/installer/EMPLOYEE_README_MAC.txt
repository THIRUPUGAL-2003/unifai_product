UnifAI Guard — macOS Employee Install Guide
==========================================

Company server: (set SERVER_DOMAIN in .env — run sync_config_from_env.py)
(If IT gave you a different backend URL, use the config in this ZIP.)

INSTALL (turn ON)
-----------------
Preferred: if IT gave you UnifAI_Guard_*.pkg — double-click the .pkg and follow prompts.
  Allow any Keychain/admin prompts so the Guard certificate is trusted.

ZIP path:
1. Remove or disable conflicting web content filters if present
   (System Settings → Network → VPN & Filters → Filters)
2. Unzip UnifAI_Guard_macOS.zip — keep ALL files in one folder (do not move the .app separately)
3. Double-click Install_UnifAI_Guard.command
   - If macOS says it cannot be opened: Right-click → Open → Open
   - Or: System Settings → Privacy & Security → Open Anyway
4. Fully quit Safari / Chrome / Edge / Firefox / Brave (Cmd+Q), then reopen
5. Safari users: System Settings → iCloud → Private Relay → Off (required for corp intercept)
6. Open a monitored AI website and send a test prompt

Health check: http://127.0.0.1:18195/
  → proxy_port must be OK (not FAIL).
  → If FAIL: quit Guard (top-right menu bar icon), reopen /Applications/UnifAI_Guard.app, wait 10s, refresh.
Logs: ~/Library/Application Support/UnifAI/Guard/unifai_guard.log

Full install guide: INSTALL_MACOS.txt (included in this ZIP)

What it does
------------
- Connects to the company UnifAI backend for rules & target websites
- Runs a local proxy on this Mac only (127.0.0.1:18103)
- Sets system Auto Proxy URL (PAC) — same idea as Windows
- Disables Chromium HTTP/3 (QUIC) via managed policies when possible
- Starts again at login (LaunchAgent KeepAlive) — like Windows autostart
- Does NOT need Docker or direct database access

TURN OFF / UNINSTALL
--------------------
1. Double-click Uninstall_UnifAI_Guard.command (in this ZIP)
   - If blocked: Right-click → Open → Open
2. Enter the company uninstall key from IT (Browser AI → Setup)
3. Wait for the success dialog — Guard stops, proxy/PAC cleared, app removed
4. Fully quit and reopen browsers (Cmd+Q)

Full uninstall guide: UNINSTALL_MACOS.txt (included in this ZIP)

Same key rule as Windows. Leave blank only if IT disabled the key requirement.

Admin remote uninstall from Browser AI also turns Guard OFF on the next heartbeat
(no employee key) — same as Windows.
