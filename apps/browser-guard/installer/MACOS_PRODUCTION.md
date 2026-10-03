# macOS Guard production notes

- **Version:** 1.1.16 (`release/VERSION.txt`, agent `AGENT_VERSION`)
- **Backend:** from `.env` `SERVER_DOMAIN` (run `sync_config_from_env.py` / `make sync-guard-config`)
- **Ship to Macs:** `release/Raksha_Guard_Setup.pkg` (single installer wizard like Setup.exe) or `release/Raksha_Guard_macOS.zip`
- **Do not ship** stale `Raksha_Guard_*.pkg` with a lower version than `AGENT_VERSION`

## Build (on a Mac)

```bash
# from raksha_product repo root
make sync-guard-config
make build-guard-mac
# Or run direct script:
./apps/browser-guard/installer/build_macos.sh
```

## Employee update

- Prefer Browser AI → Setup → Download macOS ZIP or Setup PKG, or
- Run `release/Update_Raksha_Guard_macOS.command` (reads `backend_url` + `guard_secret` from config)

## Checklist

- [x] `.env` has `SERVER_DOMAIN` + real `RAKSHA_GUARD_SECRET`
- [x] sync + Mac rebuild completed (**required for binary**; Windows cannot produce a signed Mac binary)
- [x] `release/Raksha_Guard.app` Info.plist = 1.1.16
- [x] `Contents/Resources/VERSION.txt` present (= 1.1.16)
- [x] `release/Raksha_Guard_Setup.pkg` present (= 1.1.16)
- [x] Server `release/` updated so Setup download serves new ZIP / PKG
- [ ] Gatekeeper / notarize if shipping outside your org (see prior Mac signing docs)
