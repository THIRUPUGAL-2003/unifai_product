UnifAI Enterprise AI Security Guard - Deployment Suite
==============================================================

1. DESKTOP AGENT (Standalone Executable - Zero Setup Wizard):
   - Windows: Go to 1_Windows/ and run YP_Raksha.exe.
     It runs directly in the background (no setup wizard, no installer errors).
     It connects to your UnifAI Security website and starts protection on 127.0.0.1:18195.
   - macOS: Go to 2_macOS/ and run Install_YP_Raksha.command (or double-click YP_Raksha.app).

2. BROWSER EXTENSION (Independent Installation):
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
     1. Right-click 'Whitelist_Extension_In_Browsers.bat' and select 'Run as administrator'.
     2. Open brave://extensions, enable Developer mode, and drag-and-drop the .crx file.

   - Mozilla Firefox:
     1. Open about:debugging in Firefox.
     2. Click "This Firefox" -> "Load Temporary Add-on" and select 'Firefox_Extension.xpi'.

   - Apple Safari:
     1. In Safari -> Settings -> Extensions, enable YP-Raksha Guard.

3. HOW IT WORKS TOGETHER:
   - The Desktop Agent (YP_Raksha.exe) syncs DLP rules, prompt policies, and monitored domains from the website.
   - The Browser Extension connects directly to YP_Raksha.exe on 127.0.0.1:18195.
   - Both work seamlessly to predict, inspect, and enforce DLP policies on all AI chats and file uploads!

4. UNINSTALL PROTECTION:
   - YP_Raksha.exe cannot be deleted or uninstalled without authorization.
   - To uninstall, run 'Uninstall_YP_Raksha.bat' and enter your company uninstall key.
