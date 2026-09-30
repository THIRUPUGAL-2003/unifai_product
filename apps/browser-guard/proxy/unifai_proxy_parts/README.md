# UnifAI proxy parts

These files are **not** imported as a normal package.
`browser_ai_proxy.py` loads them in **MANIFEST.txt order** into one shared namespace
(same behavior as the old single-file monolith).

Files:
- config_caches_rules.py
- helpers_prompts.py
- uploads_detect.py
- file_policy.py
- extract_office_backend.py
- responses_inject.py
- responses_addon.py

Edit carefully — keep cross-part name references working.
Rebuild Guard after changes (`build_installer.bat` / `build_macos.sh`).
