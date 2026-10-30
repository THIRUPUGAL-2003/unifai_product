UnifAI Enterprise AI Security Guard - Deployment Suite
==============================================================

1. DESKTOP AGENT (Standalone Executable or Installer):
   - Windows: Go to windows/ and run YP_Raksha_Setup.exe (installer) or YP_Raksha.exe (portable).
     It connects to your UnifAI Security website and starts protection on 127.0.0.1:18195.
   - macOS: Go to mac/ and run Install_YP_Raksha.command (or extract YP_Raksha_macOS.zip).

2. BROWSER EXTENSION (Independent Installation):
   - Google Chrome / Brave / Microsoft Edge / Opera:
     ============================================================
     RECOMMENDED METHOD (Instant, 0 Warnings, Never Blocked):
     1. In extension/, the extension files are already unpacked and ready!
     2. Open brave://extensions (or chrome://extensions, edge://extensions).
     3. Ensure 'Developer mode' is toggled ON (top-right corner).
     4. Click 'Load unpacked' (top-left button) and select the 'extension' folder (or extension/YP_Raksha_Extension).
     5. Done! The extension is permanently active, enabled, and 100% functional.
     ============================================================
     ALTERNATIVE METHOD (.CRX Whitelist):
     1. Right-click 'Whitelist_Extension_In_Browsers.bat' and select 'Run as administrator'.
     2. Open brave://extensions, enable Developer mode, and drag-and-drop the .crx file.

   - Mozilla Firefox:
     1. Install the Mozilla-signed Firefox add-on package when it is included in this suite.
     2. Unsigned developer packages are only for temporary testing in a development Firefox build.

   - Apple Safari:
     1. In Safari -> Settings -> Extensions, enable YP-Raksha Guard.

3. HOW IT WORKS TOGETHER:
   - The Desktop Agent (YP_Raksha.exe) syncs DLP rules, prompt policies, and monitored domains from the website.
   - The Browser Extension connects directly to YP_Raksha.exe on 127.0.0.1:18195.
   - Both work seamlessly to predict, inspect, and enforce DLP policies on all AI chats and file uploads!

4. UNINSTALL PROTECTION:
   - YP_Raksha.exe cannot be deleted or uninstalled without authorization.
   - To uninstall, run 'Uninstall_YP_Raksha.bat' and enter your company uninstall key.
