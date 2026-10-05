Raksha Guard — Employee Install Guide
=====================================

Company server: (set SERVER_DOMAIN in .env — run sync_config_from_env.py)
(If IT gave you a different backend URL, use the config in this ZIP.)

1. Extract the ZIP completely: Right-click Raksha_Guard_Windows.zip and choose "Extract All..."
   (Do NOT run Setup directly from inside the ZIP file)
2. In the extracted folder, run Raksha_Guard_Setup.exe
3. Keep "Start automatically at Windows login" checked
4. Finish — Guard starts in background (no black terminal)
5. Open ChatGPT / monitored AI sites as usual
6. If sites show certificate warnings, tell IT — CA trust must succeed
   (status file: %LOCALAPPDATA%\Raksha\Guard\ca_install_status.txt)

What it does
------------
- Connects to the company Raksha backend for rules & target websites
- Runs a local proxy on this PC only (127.0.0.1:18103)
- Does NOT need the Docker proxy container or direct database access

Logs (if IT asks)
-----------------
%LOCALAPPDATA%\Raksha\Guard\raksha_guard.log

Uninstall
---------
Windows Settings > Apps > Raksha Guard > Uninstall
(or Start Menu > Raksha Guard > Uninstall)

When prompted, enter the company uninstall key from IT
(Browser AI → Setup). Leave blank only if IT disabled the key requirement.

Antivirus / Windows Defender Alert
----------------------------------
If your Antivirus (Windows Defender, CrowdStrike, McAfee, Kaspersky, Norton, Bitdefender, etc.)
blocks installation, quarantines Raksha_Guard.exe, or prevents it from starting:
1. Open Windows Security or your company Antivirus app.
2. Under "Protection history" or "Quarantined threats", choose "Allow on device" or "Restore".
3. Add an Exclusion / Whitelist for:
   %LOCALAPPDATA%\Programs\Raksha\Guard\Raksha_Guard.exe
4. If your laptop is managed by company IT, contact your IT Administrator with this notice
   so they can approve Raksha Guard in the central endpoint security portal.

