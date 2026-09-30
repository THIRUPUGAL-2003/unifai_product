# UnifAI Guard (Browser AI desktop agent)

Desktop agent for Target Websites, Prompt Logs, and Guard Bot — **Windows + macOS**.

## One-click build (recommended)

From repo root (`unifai_product/`), after `.env` has `SERVER_DOMAIN` (+ Guard secret):

| Command | Host | Output |
|---------|------|--------|
| `make sync-guard-config` | any | Sync configs/docs from `.env` |
| `make build-guard-windows` | **Windows** + Inno Setup 6 | `release/UnifAI_Guard_Setup.exe`, ZIP, portable EXE |
| `make build-guard-mac` | **Mac** | `release/UnifAI_Guard.app`, `UnifAI_Guard_macOS.zip` |
| `make build-guard-mac-pkg` | **Mac** (after mac build) | `.pkg` installer |
| `make help-guard` | any | List Guard targets |

Same targets work via `make -C apps/browser-guard …`.

**No `make` on Windows?** Use the same pipeline directly:

```bat
apps\browser-guard\installer\build_installer.bat
```

Underlying scripts are unchanged: `installer/build_installer.bat`, `installer/build_macos.sh`, `scripts/sync_config_from_env.py`.

## Windows setup

| Package | How |
|---------|-----|
| `release/UnifAI_Guard_Setup.exe` | Inno installer (preferred for employees) |
| `release/UnifAI_Guard.exe` | Portable / latest PyInstaller build |

Build on Windows: `make build-guard-windows` (or `installer\build_installer.bat`)

See `release/INSTALL_WINDOWS.txt`.

**Uninstall / turn OFF:** Windows Settings → Apps → UnifAI Guard → Uninstall (company uninstall key).

## macOS setup

| Package | How |
|---------|-----|
| `release/UnifAI_Guard_macOS.zip` | `.app` + Install / Uninstall `.command` scripts |
| `release/UnifAI_Guard.app` | PyInstaller app bundle (after build) |

Build **on a Mac**:

```bash
make build-guard-mac
# optional:
make build-guard-mac-pkg
```

Or: `cd apps/browser-guard && ./installer/build_macos.sh`

Then copy `release/UnifAI_Guard_macOS.zip` (and docs) onto the UnifAI server under `apps/browser-guard/release/` and redeploy so **Download Setup ZIP** includes it.

See:

- `release/INSTALL_MACOS.txt` — install / turn ON
- `release/UNINSTALL_MACOS.txt` — turn OFF / uninstall
- `installer/EMPLOYEE_README_MAC.txt` — employee one-pager

**Uninstall / turn OFF:** double-click `Uninstall_UnifAI_Guard.command` (same company uninstall key as Windows).

## Deploy to server

Copy Windows and/or macOS artifacts into `apps/browser-guard/release/` and redeploy so **Browser AI → Setup → Download Setup ZIP** ships them.
