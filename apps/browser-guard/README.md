# Raksha Guard (Browser AI desktop agent)

Desktop agent for Target Websites, Prompt Logs, and Guard Bot — **Windows + macOS**.

## One-click build (recommended)

From repo root (`raksha_product/`), after `.env` has `SERVER_DOMAIN` (+ Guard secret):

| Command | Host | Output |
|---------|------|--------|
| `make sync-guard-config` | any | Sync configs/docs from `.env` |
| `make build-guard-windows` | **Windows** + Inno Setup 6 | `release/Raksha_Guard_Setup.exe`, ZIP, portable EXE |
| `make build-guard-mac` | **Mac** | `release/Raksha_Guard.app`, `Raksha_Guard_macOS.zip` |
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
| `release/Raksha_Guard_Setup.exe` | Inno installer (preferred for employees) |
| `release/Raksha_Guard.exe` | Portable / latest PyInstaller build |

Build on Windows: `make build-guard-windows` (or `installer\build_installer.bat`)

See `release/INSTALL_WINDOWS.txt`.

**Uninstall / turn OFF:** Windows Settings → Apps → Raksha Guard → Uninstall (company uninstall key).

## macOS setup

| Package | How |
|---------|-----|
| `release/Raksha_Guard_macOS.zip` | `.app` + Install / Uninstall `.command` scripts |
| `release/Raksha_Guard.app` | PyInstaller app bundle (after build) |

Build **on a Mac**:

```bash
make build-guard-mac
# optional:
make build-guard-mac-pkg
```

Or: `cd apps/browser-guard && ./installer/build_macos.sh`

Then copy `release/Raksha_Guard_macOS.zip` (and docs) onto the Raksha server under `apps/browser-guard/release/` and redeploy so **Download Setup ZIP** includes it.

See:

- `release/INSTALL_MACOS.txt` — install / turn ON
- `release/UNINSTALL_MACOS.txt` — turn OFF / uninstall
- `installer/EMPLOYEE_README_MAC.txt` — employee one-pager

**Uninstall / turn OFF:** double-click `Uninstall_Raksha_Guard.command` (same company uninstall key as Windows).

## Deploy to server

Copy Windows and/or macOS artifacts into `apps/browser-guard/release/` and redeploy so **Browser AI → Setup → Download Setup ZIP** ships them.
