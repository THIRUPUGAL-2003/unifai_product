# UnifAI Guard — one-click build entrypoints
# Delegates to existing installer scripts (do not remove those).
#
# From repo root:
#   make sync-guard-config
#   make build-guard-windows    # Windows host + Inno Setup 6
#   make build-guard-mac        # macOS host
#   make build-guard-mac-pkg    # macOS .pkg (after mac build)
#
# Or from this directory:
#   make -C apps/browser-guard build-guard-windows

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
REPO_ROOT := $(abspath $(ROOT)/../..)
PYTHON ?= python3
UNAME_S := $(shell uname -s 2>/dev/null || echo unknown)

.PHONY: help sync-guard-config build-guard-windows build-guard-mac build-guard-mac-pkg build-guard-agent

help: ## Show Guard build targets
	@echo "UnifAI Guard build"
	@echo ""
	@echo "  make sync-guard-config       Sync configs/docs from repo .env"
	@echo "  make build-guard-windows     Windows: sync + EXE + Setup + ZIP (run on Windows)"
	@echo "  make build-guard-mac         macOS: sync + .app + ZIP (run on a Mac)"
	@echo "  make build-guard-mac-pkg     macOS: .pkg (run after build-guard-mac)"
	@echo "  make build-guard-agent       PyInstaller only for current OS"
	@echo ""
	@echo "Requires: SERVER_DOMAIN (and Guard secret) in $(REPO_ROOT)/.env"
	@echo "Windows also needs: Inno Setup 6 (ISCC.exe)"
	@echo "Artifacts land in: $(ROOT)/release/"

sync-guard-config: ## Sync Guard JSON/docs/ISS from repo .env
	@echo "==> sync_config_from_env.py"
	@cd "$(ROOT)" && (command -v $(PYTHON) >/dev/null 2>&1 && $(PYTHON) scripts/sync_config_from_env.py || python scripts/sync_config_from_env.py)

build-guard-windows: ## Full Windows installer pipeline
	@ok=0; \
	case "$(UNAME_S)" in MINGW*|MSYS*|CYGWIN*) ok=1 ;; esac; \
	if [ "$${OS:-}" = "Windows_NT" ]; then ok=1; fi; \
	if [ "$$ok" != "1" ]; then \
		echo "ERROR: build-guard-windows must run on a Windows machine (Inno Setup + PyInstaller)."; \
		echo "  On Windows (Git Bash / PowerShell): make build-guard-windows"; \
		echo "  Or: apps\\browser-guard\\installer\\build_installer.bat"; \
		exit 1; \
	fi
	@echo "==> build_installer.bat (includes sync)"
	@cd "$(ROOT)/installer" && cmd.exe /c build_installer.bat

build-guard-mac: ## Full macOS .app + ZIP pipeline
	@if [ "$(UNAME_S)" != "Darwin" ]; then \
		echo "ERROR: build-guard-mac must run on a Mac (PyInstaller cannot cross-compile Darwin)."; \
		echo "  On Mac: make build-guard-mac"; \
		echo "  Or: ./apps/browser-guard/installer/build_macos.sh"; \
		exit 1; \
	fi
	@echo "==> build_macos.sh (includes sync)"
	@cd "$(ROOT)" && ./installer/build_macos.sh

build-guard-mac-pkg: ## macOS product .pkg (needs release/UnifAI_Guard.app)
	@if [ "$(UNAME_S)" != "Darwin" ]; then \
		echo "ERROR: build-guard-mac-pkg must run on a Mac."; \
		exit 1; \
	fi
	@echo "==> build_pkg_macos.sh"
	@cd "$(ROOT)" && ./installer/build_pkg_macos.sh

build-guard-agent: ## PyInstaller agent only (current OS; syncs via build_agent.py)
	@echo "==> build_agent.py"
	@cd "$(ROOT)" && (command -v $(PYTHON) >/dev/null 2>&1 && $(PYTHON) installer/build_agent.py || python installer/build_agent.py)
