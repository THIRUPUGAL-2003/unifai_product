UnifAI Enterprise AI Security Guard - Deployment Suite
==============================================================

1. DESKTOP AGENT INSTALLATION (Choose your OS):
   - Windows: Go to 1_Windows/ and run Gateway_Guard.exe (or YP_Raksha.exe).
     (It starts the background proxy & DLP service on 127.0.0.1:18195).
   - macOS: Go to 2_macOS/ and run Install_YP_Raksha.command (or double-click YP_Raksha.app).

2. BROWSER EXTENSION INSTALLATION (Choose your Browser):
   - Google Chrome / Brave / Edge / Opera:
     Method A (Direct Drag & Drop):
       1. Open chrome://extensions (or brave://extensions, edge://extensions).
       2. Turn ON "Developer mode" (top-right).
       3. Drag and drop 'yp-raksha.crx' directly into the browser window.
     Method B (Unpacked Folder):
       1. Extract 'Unpacked_Extension_Chrome_Edge_Brave.zip'.
       2. Click 'Load unpacked' and select the extracted folder.
   - Mozilla Firefox:
     1. Open about:debugging in Firefox.
     2. Click "This Firefox" -> "Load Temporary Add-on" and select 'yp-raksha.xpi'.
   - Apple Safari:
     1. In Safari -> Settings -> Extensions, enable YP-Raksha Guard.

3. VERIFICATION:
   - Once both are running on your laptop, the browser extension automatically connects to the local agent.
   - All DLP Guard Rules configured in the UnifAI Dashboard will protect AI prompts in real time.
