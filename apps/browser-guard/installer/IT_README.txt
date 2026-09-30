
UnifAI Guard — Company Release Notes
====================================

Employee files to distribute:
  Windows: release\UnifAI_Guard_Setup.exe   (Inno Setup installer)
           release\UnifAI_Guard_Windows.zip  (ZIP for web download — contains Setup.exe + EXE + docs)
  macOS:   release\UnifAI_Guard_macOS.zip   (ZIP for web download — contains .app + install scripts)
           release\UnifAI_Guard_*.pkg        (optional signed pkg — build with build_pkg_macos.sh)

Configured backend:
  (from .env SERVER_DOMAIN — run: python apps/browser-guard/scripts/sync_config_from_env.py)

Hybrid (laptop + network, ONE Browser AI dashboard):
  See HYBRID_DEPLOY.txt in this folder (parent apps\browser-guard).
  Laptop EXE/.app and docker network proxy share the same rules + Prompt Logs.

BEFORE company rollout — verify server APIs return JSON (not HTML):
  ${SERVER_DOMAIN}/health
  ${SERVER_DOMAIN}/api/browser-ai/targets
  ${SERVER_DOMAIN}/api/browser-ai/rules
  ${SERVER_DOMAIN}/api/browser-ai/proxy.pac?proxy=127.0.0.1:18103
  Network PAC example:
  ${SERVER_DOMAIN}/api/browser-ai/pac?proxy=proxy.company.local:8087

If /api/browser-ai/* returns the UnifAI web page HTML, deploy the latest
backend that includes Browser AI routes, then re-test.

Rebuild installers:
  Windows: installer\build_installer.bat
    → Produces: release\UnifAI_Guard_Setup.exe + release\UnifAI_Guard_Windows.zip
  macOS:   ./installer/build_macos.sh   (must run on a Mac, Python 3.11+)
    → Produces: release\UnifAI_Guard_macOS.zip + release\UnifAI_Guard.app
  macOS pkg (optional):
    ./installer/build_pkg_macos.sh  (or sign_and_notarize_macos.sh for Developer ID)
    → Produces: release\UnifAI_Guard_VERSION.pkg

Uninstall / turn OFF
--------------------
  Windows: Settings → Apps → UnifAI Guard → Uninstall (company key)
  macOS:   Uninstall_UnifAI_Guard.command (same company key) — see UNINSTALL_MACOS.txt

Packaging structure
-------------------
dist\UnifAI_Guard.exe            Raw standalone Windows agent build
dist\UnifAI_Guard.app            Raw macOS app (after Mac build)
installer\UnifAI_Guard.iss       Inno Setup source
installer\build_installer.bat    Rebuilds Windows staging + setup EXE + Windows ZIP
installer\build_macos.sh         Rebuilds Mac .app + UnifAI_Guard_macOS.zip
installer\build_pkg_macos.sh     Builds Mac .pkg from existing .app
installer\EMPLOYEE_README.txt    Windows employee readme
installer\EMPLOYEE_README_MAC.txt Mac employee readme
installer\staging\               Temporary/generated Windows build staging
release\UnifAI_Guard_Setup.exe   Final Windows employee installer (Inno Setup)
release\UnifAI_Guard_Windows.zip Final Windows employee download package (web ZIP)
release\UnifAI_Guard_macOS.zip   Final Mac employee package
release\VERSION.txt              Current build version
