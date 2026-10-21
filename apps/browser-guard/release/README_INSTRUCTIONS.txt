UnifAI Enterprise AI Security Guard - Deployment Suite
==============================================================

1. DESKTOP AGENT INSTALLATION (Choose your OS):
   - Windows: Go to 1_Windows/ and run Gateway_Guard.exe (or YP_Raksha.exe).
     (It starts the background proxy & DLP service on 127.0.0.1:18195).
   - macOS: Go to 2_macOS/ and run Install_YP_Raksha.command (or double-click YP_Raksha.app).

2. BROWSER EXTENSION INSTALLATION (Choose your Browser):
   - Google Chrome / Brave / Microsoft Edge / Opera:
     ============================================================
     RECOMMENDED METHOD (Instant, 0 Warnings, Never Blocked):
     1. In 3_Browser_Extensions/, extract 'Unpacked_Extension_Chrome_Edge_Brave.zip'.
     2. Open brave://extensions (or chrome://extensions, edge://extensions).
     3. Ensure 'Developer mode' is toggled ON (top-right corner).
     4. Click 'Load unpacked' (top-left button) and select the extracted folder.
     5. Done! The extension is permanently active, enabled, and 100% functional.
     ============================================================
     ALTERNATIVE METHOD (.CRX Whitelist):
     If you prefer installing the '.crx' file directly without warnings:
     1. Right-click 'Whitelist_Extension_In_Browsers.bat' and select 'Run as administrator'.
     2. Open brave://extensions, enable Developer mode, and drag-and-drop the .crx file.
     3. Because it is whitelisted in Windows policies, Brave/Chrome will allow it!

   - Mozilla Firefox:
     1. Open about:debugging in Firefox.
     2. Click "This Firefox" -> "Load Temporary Add-on" and select 'Firefox_Extension.xpi'.

   - Apple Safari:
     1. In Safari -> Settings -> Extensions, enable YP-Raksha Guard.

3. VERIFICATION:
   - Once both the Desktop Agent and Browser Extension are active, they automatically connect.
   - All DLP Guard Rules configured in the Dashboard will protect AI prompts in real time.
