package handlers

import (
	"archive/zip"
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/raksha/raksha/framework/logstore"
	"github.com/valyala/fasthttp"
)

func browserAISetupCandidates() map[string][]string {
	return map[string][]string{
		"Raksha_Guard_Setup.exe": {
			filepath.Join("apps", "browser-guard", "release", "Raksha_Guard_Setup.exe"),
			filepath.Join("release", "Raksha_Guard_Setup.exe"),
			"/app/release/Raksha_Guard_Setup.exe",
			"/app/apps/browser-guard/release/Raksha_Guard_Setup.exe",
		},
		// Portable EXE (latest PyInstaller build) — preferred when newer than Setup.exe.
		"Raksha_Guard.exe": {
			filepath.Join("apps", "browser-guard", "release", "Raksha_Guard.exe"),
			filepath.Join("apps", "browser-guard", "dist", "Raksha_Guard.exe"),
			filepath.Join("release", "Raksha_Guard.exe"),
			"/app/release/Raksha_Guard.exe",
			"/app/apps/browser-guard/release/Raksha_Guard.exe",
		},
		// macOS employee package (PyInstaller .app + install/uninstall scripts).
		"Raksha_Guard_macOS.zip": {
			filepath.Join("apps", "browser-guard", "release", "Raksha_Guard_macOS.zip"),
			filepath.Join("release", "Raksha_Guard_macOS.zip"),
			"/app/release/Raksha_Guard_macOS.zip",
			"/app/apps/browser-guard/release/Raksha_Guard_macOS.zip",
		},
		// macOS product installer package (.pkg setup wizard)
		"Raksha_Guard_Setup.pkg": {
			filepath.Join("apps", "browser-guard", "release", "Raksha_Guard_Setup.pkg"),
			filepath.Join("release", "Raksha_Guard_Setup.pkg"),
			"/app/release/Raksha_Guard_Setup.pkg",
			"/app/apps/browser-guard/release/Raksha_Guard_Setup.pkg",
		},
		"raksha_guard_config.json": {
			filepath.Join("apps", "browser-guard", "release", "raksha_guard_config.json"),
			filepath.Join("apps", "browser-guard", "config", "raksha_guard_config.json"),
			filepath.Join("release", "raksha_guard_config.json"),
			"/app/release/raksha_guard_config.json",
			"/app/apps/browser-guard/release/raksha_guard_config.json",
		},
		"INSTALL_WINDOWS.txt": {
			filepath.Join("apps", "browser-guard", "release", "INSTALL_WINDOWS.txt"),
			filepath.Join("release", "INSTALL_WINDOWS.txt"),
			"/app/release/INSTALL_WINDOWS.txt",
			"/app/apps/browser-guard/release/INSTALL_WINDOWS.txt",
		},
		"INSTALL_MACOS.txt": {
			filepath.Join("apps", "browser-guard", "release", "INSTALL_MACOS.txt"),
			filepath.Join("release", "INSTALL_MACOS.txt"),
			"/app/release/INSTALL_MACOS.txt",
			"/app/apps/browser-guard/release/INSTALL_MACOS.txt",
		},
		"UNINSTALL_MACOS.txt": {
			filepath.Join("apps", "browser-guard", "release", "UNINSTALL_MACOS.txt"),
			filepath.Join("release", "UNINSTALL_MACOS.txt"),
			"/app/release/UNINSTALL_MACOS.txt",
			"/app/apps/browser-guard/release/UNINSTALL_MACOS.txt",
		},
		"EMPLOYEE_README_MAC.txt": {
			filepath.Join("apps", "browser-guard", "release", "EMPLOYEE_README_MAC.txt"),
			filepath.Join("apps", "browser-guard", "installer", "EMPLOYEE_README_MAC.txt"),
			filepath.Join("release", "EMPLOYEE_README_MAC.txt"),
			"/app/release/EMPLOYEE_README_MAC.txt",
			"/app/apps/browser-guard/release/EMPLOYEE_README_MAC.txt",
		},
		"Install_Raksha_Guard.command": {
			filepath.Join("apps", "browser-guard", "release", "Install_Raksha_Guard.command"),
			filepath.Join("apps", "browser-guard", "installer", "Install_Raksha_Guard.command"),
			filepath.Join("release", "Install_Raksha_Guard.command"),
			"/app/release/Install_Raksha_Guard.command",
			"/app/apps/browser-guard/release/Install_Raksha_Guard.command",
		},
		"Uninstall_Raksha_Guard.command": {
			filepath.Join("apps", "browser-guard", "release", "Uninstall_Raksha_Guard.command"),
			filepath.Join("apps", "browser-guard", "installer", "Uninstall_Raksha_Guard.command"),
			filepath.Join("release", "Uninstall_Raksha_Guard.command"),
			"/app/release/Uninstall_Raksha_Guard.command",
			"/app/apps/browser-guard/release/Uninstall_Raksha_Guard.command",
		},
		"VERSION.txt": {
			filepath.Join("apps", "browser-guard", "release", "VERSION.txt"),
			filepath.Join("release", "VERSION.txt"),
			"/app/release/VERSION.txt",
			"/app/apps/browser-guard/release/VERSION.txt",
		},
		"Update_Raksha_Guard.ps1": {
			filepath.Join("apps", "browser-guard", "release", "Update_Raksha_Guard.ps1"),
			filepath.Join("release", "Update_Raksha_Guard.ps1"),
			"/app/release/Update_Raksha_Guard.ps1",
			"/app/apps/browser-guard/release/Update_Raksha_Guard.ps1",
		},
	}
}

func findFirstExisting(candidates []string) (string, bool) {
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func readGuardReleaseVersion() string {
	for _, p := range []string{
		filepath.Join("apps", "browser-guard", "release", "VERSION.txt"),
		filepath.Join("release", "VERSION.txt"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(data))
		if v != "" {
			return v
		}
	}
	// Fallback: parse AGENT_VERSION / AGENT_VERSION_BAKED from source when VERSION.txt missing.
	for _, agentPy := range []string{
		filepath.Join("apps", "browser-guard", "agent", "agent_config.py"),
		filepath.Join("apps", "browser-guard", "agent", "raksha_agent.py"),
	} {
		data, err := os.ReadFile(agentPy)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if (strings.HasPrefix(line, "AGENT_VERSION_BAKED") || strings.HasPrefix(line, "AGENT_VERSION")) && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					if v != "" {
						return v
					}
				}
			}
		}
	}
	return ""
}

// readGuardMacReleaseVersion returns the version baked into the shipped macOS .app.
// Windows-only publishes may bump VERSION.txt ahead of the Darwin binary — Mac downloads
// must advertise the .app plist version so fleets do not enter an infinite update loop.
func readGuardMacReleaseVersion() string {
	for _, p := range []string{
		filepath.Join("apps", "browser-guard", "release", "Raksha_Guard.app", "Contents", "Info.plist"),
		filepath.Join("release", "Raksha_Guard.app", "Contents", "Info.plist"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := string(data)
		marker := "<key>CFBundleShortVersionString</key>"
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		rest := text[idx+len(marker):]
		start := strings.Index(rest, "<string>")
		end := strings.Index(rest, "</string>")
		if start >= 0 && end > start {
			v := strings.TrimSpace(rest[start+len("<string>") : end])
			if v != "" {
				return v
			}
		}
	}
	// Sidecar written by package_macos.ps1
	for _, p := range []string{
		filepath.Join("apps", "browser-guard", "release", "Raksha_Guard.app", "Contents", "Resources", "VERSION.txt"),
		filepath.Join("release", "Raksha_Guard.app", "Contents", "Resources", "VERSION.txt"),
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(data))
		if v != "" {
			return v
		}
	}
	return readGuardReleaseVersion()
}

func fileModTime(path string) *time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	t := info.ModTime().UTC()
	return &t
}

func guardRebuildAvailable() bool {
	if _, ok := findFirstExisting([]string{
		filepath.Join("apps", "browser-guard", "scripts", "publish_fleet_packages.py"),
		filepath.Join("scripts", "publish_fleet_packages.py"),
	}); !ok {
		return false
	}
	for _, name := range []string{"python3", "python"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// guardPackageInfo describes the Guard packages this server currently serves to laptops.
func guardPackageInfo() map[string]any {
	info := map[string]any{
		"version":       readGuardReleaseVersion(),
		"mac_version":   readGuardMacReleaseVersion(),
		"windows_ready": false,
		"macos_ready":   false,
		"can_rebuild":   guardRebuildAvailable(),
	}
	if p, ok := findFirstExisting(browserAISetupCandidates()["Raksha_Guard_Setup.exe"]); ok {
		info["windows_ready"] = true
		info["windows_built_at"] = fileModTime(p)
	} else if p, ok := findFirstExisting(browserAISetupCandidates()["Raksha_Guard.exe"]); ok {
		info["windows_ready"] = true
		info["windows_built_at"] = fileModTime(p)
	}
	if p, ok := findFirstExisting(browserAISetupCandidates()["Raksha_Guard_macOS.zip"]); ok {
		info["macos_ready"] = true
		info["macos_built_at"] = fileModTime(p)
	}
	_, srcOK := guardProxySourceDir()
	_, agentOK := guardAgentSourceDir()
	info["proxy_source_available"] = srcOK && agentOK
	if meta := loadPublishedGuardProxyBundle(); meta != nil {
		info["proxy_bundle"] = meta
	}
	return info
}

// publishProxyBundleSummary publishes the Guard code hot-update bundle and describes the outcome.
func publishProxyBundleSummary() (map[string]any, string) {
	meta, err := publishGuardProxyBundle()
	if err != nil {
		return map[string]any{"error": err.Error()}, "Guard code update NOT published: " + err.Error() + "."
	}
	return map[string]any{
		"sha256":         meta.SHA256,
		"published_at":   meta.PublishedAt,
		"files":          meta.Files,
		"guard_versions": meta.GuardVersions,
	}, "Guard code " + meta.SHA256[:8] + " published — installed Guards on v" +
		strings.Join(meta.GuardVersions, " / v") + " switch to it within about a minute (no reinstall)."
}

func (h *BrowserAIHandler) getSetupInfo(ctx *fasthttp.RequestCtx) {
	SendJSON(ctx, guardPackageInfo())
}

func (h *BrowserAIHandler) downloadSetupPackage(ctx *fasthttp.RequestCtx) {
	type zipAsset struct {
		name string
		path string
	}

	platform := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("platform"))))
	if platform == "" {
		platform = strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("os"))))
	}
	path := string(ctx.Path())
	if strings.Contains(path, "download-windows") {
		platform = "windows"
	} else if strings.Contains(path, "download-mac") {
		platform = "mac"
	}

	setupPath, setupOK := findFirstExisting(browserAISetupCandidates()["Raksha_Guard_Setup.exe"])
	exePath, exeOK := findFirstExisting(browserAISetupCandidates()["Raksha_Guard.exe"])
	macZipPath, macZipOK := findFirstExisting(browserAISetupCandidates()["Raksha_Guard_macOS.zip"])
	winVer := readGuardReleaseVersion()
	macVer := readGuardMacReleaseVersion()

	// 1. MAC DEDICATED DOWNLOAD
	if platform == "mac" || platform == "macos" || platform == "darwin" {
		if !macZipOK {
			SendError(ctx, fasthttp.StatusNotFound, "No macOS Guard installer on server — add Raksha_Guard_macOS.zip under apps/browser-guard/release/")
			return
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetContentType("application/zip")
		ctx.Response.Header.Set("Content-Disposition", `attachment; filename="Raksha_Guard_macOS.zip"`)
		if macVer != "" {
			ctx.Response.Header.Set("X-Raksha-Guard-Version", macVer)
		}
		ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
			f, err := os.Open(macZipPath)
			if err != nil {
				return
			}
			defer f.Close()
			_, _ = io.Copy(w, f)
			_ = w.Flush()
		})
		return
	}

	// 2. WINDOWS DEDICATED DOWNLOAD
	if platform == "windows" || platform == "win" {
		if !setupOK && !exeOK {
			SendError(ctx, fasthttp.StatusNotFound, "No Windows Guard installer on server — add Raksha_Guard_Setup.exe or Raksha_Guard.exe under apps/browser-guard/release/")
			return
		}
		// Always ship BOTH when present: Setup (Inno install) + portable EXE.
		// INSTALL_WINDOWS.txt tells employees to prefer newer EXE if Setup is older.
		var winAssets []zipAsset
		if setupOK {
			winAssets = append(winAssets, zipAsset{name: "Raksha_Guard_Setup.exe", path: setupPath})
		}
		if exeOK {
			winAssets = append(winAssets, zipAsset{name: "Raksha_Guard.exe", path: exePath})
		}
		for _, name := range []string{"INSTALL_WINDOWS.txt", "VERSION.txt", "raksha_guard_config.json", "Update_Raksha_Guard.ps1"} {
			if p, ok := findFirstExisting(browserAISetupCandidates()[name]); ok {
				winAssets = append(winAssets, zipAsset{name: name, path: p})
			}
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetContentType("application/zip")
		ctx.Response.Header.Set("Content-Disposition", `attachment; filename="Raksha_Guard_Windows.zip"`)
		if winVer != "" {
			ctx.Response.Header.Set("X-Raksha-Guard-Version", winVer)
		}
		ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
			zw := zip.NewWriter(w)
			wroteVersion := false
			for _, asset := range winAssets {
				data, err := os.ReadFile(asset.path)
				if err != nil {
					continue
				}
				entry, err := zw.Create(asset.name)
				if err != nil {
					continue
				}
				_, _ = entry.Write(data)
				if asset.name == "VERSION.txt" {
					wroteVersion = true
				}
			}
			if !wroteVersion && winVer != "" {
				if entry, err := zw.Create("VERSION.txt"); err == nil {
					_, _ = entry.Write([]byte(winVer + "\n"))
				}
			}
			_ = zw.Close()
			_ = w.Flush()
		})
		return
	}

	// 3. COMBINED / LEGACY DOWNLOAD (when no platform specified)
	// Always include Setup + portable EXE when both exist (same as Windows ZIP).

	var assets []zipAsset
	if setupOK {
		assets = append(assets, zipAsset{name: "Raksha_Guard_Setup.exe", path: setupPath})
	}
	if exeOK {
		assets = append(assets, zipAsset{name: "Raksha_Guard.exe", path: exePath})
	}
	if macZipOK {
		assets = append(assets, zipAsset{name: "Raksha_Guard_macOS.zip", path: macZipPath})
	}
	for _, name := range []string{
		"INSTALL_WINDOWS.txt",
		"INSTALL_MACOS.txt",
		"UNINSTALL_MACOS.txt",
		"EMPLOYEE_README_MAC.txt",
		"Install_Raksha_Guard.command",
		"Uninstall_Raksha_Guard.command",
		"VERSION.txt",
		"raksha_guard_config.json",
	} {
		if path, ok := findFirstExisting(browserAISetupCandidates()[name]); ok {
			assets = append(assets, zipAsset{name: name, path: path})
		}
	}

	hasWindows := setupOK || exeOK
	hasMac := macZipOK
	if !hasWindows && !hasMac {
		SendError(ctx, fasthttp.StatusNotFound, "No Guard installer on server — add Raksha_Guard.exe / Raksha_Guard_Setup.exe and/or Raksha_Guard_macOS.zip under apps/browser-guard/release/")
		return
	}
	if len(assets) == 0 {
		SendError(ctx, fasthttp.StatusNotFound, "No Browser AI setup package files found on server")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/zip")
	ctx.Response.Header.Set("Content-Disposition", `attachment; filename="raksha-browser-ai-setup.zip"`)
	if winVer != "" {
		ctx.Response.Header.Set("X-Raksha-Guard-Version", winVer)
	}
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		zw := zip.NewWriter(w)
		wroteVersion := false
		for _, asset := range assets {
			data, err := os.ReadFile(asset.path)
			if err != nil {
				continue
			}
			entry, err := zw.Create(asset.name)
			if err != nil {
				continue
			}
			_, _ = entry.Write(data)
			if asset.name == "VERSION.txt" {
				wroteVersion = true
			}
		}
		// Always embed the current release version so employees see the expected number.
		if !wroteVersion && winVer != "" {
			if entry, err := zw.Create("VERSION.txt"); err == nil {
				_, _ = entry.Write([]byte(winVer + "\n"))
			}
		}
		_ = zw.Close()
		_ = w.Flush()
	})
}

// requireGuardAdmin allows only admin/sub_admin sessions when auth is enabled.
func (h *BrowserAIHandler) requireGuardAdmin(ctx *fasthttp.RequestCtx, forbiddenMsg string) bool {
	if h.configStore == nil {
		return true
	}
	authConfig, _ := h.configStore.GetAuthConfig(ctx)
	if authConfig == nil || !authConfig.IsEnabled {
		return true
	}
	token := sessionToken(ctx)
	if token == "" {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return false
	}
	session, err := h.configStore.GetSession(ctx, token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) || (session.Role != "admin" && session.Role != "sub_admin") {
		SendError(ctx, fasthttp.StatusForbidden, forbiddenMsg)
		return false
	}
	return true
}

// recordRebuild stores every Rebuild press in browser_guard_rebuild_logs and returns the row id.
func (h *BrowserAIHandler) recordRebuild(ctx *fasthttp.RequestCtx, started time.Time, status string, resp map[string]any, logTail string) string {
	h.ensureDB(ctx)
	if h.manager == nil {
		return ""
	}
	entry := &logstore.BrowserGuardRebuildLog{
		Status:      status,
		Log:         logTail,
		RequestedBy: auditInitiator(h.configStore, ctx),
		DurationMs:  time.Since(started).Milliseconds(),
	}
	entry.Mode, _ = resp["mode"].(string)
	entry.Version, _ = resp["version"].(string)
	entry.MacVersion, _ = resp["mac_version"].(string)
	entry.WindowsReady, _ = resp["windows_ready"].(bool)
	entry.MacosReady, _ = resp["macos_ready"].(bool)
	entry.Message, _ = resp["message"].(string)
	if b, ok := resp["proxy_bundle"].(map[string]any); ok {
		entry.BundleSHA, _ = b["sha256"].(string)
		entry.BundleFiles, _ = b["files"].(int)
		if v, ok := b["guard_versions"].([]string); ok {
			entry.GuardVersions = strings.Join(v, ",")
		}
		if e, ok := b["error"].(string); ok && e != "" && entry.Message == "" {
			entry.Message = e
		}
	}
	if err := h.manager.RecordGuardRebuild(ctx, entry); err != nil {
		if logger != nil {
			logger.Warn("rebuild history not saved: %v", err)
		}
		return ""
	}
	return entry.ID
}

func (h *BrowserAIHandler) getRebuildHistory(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to view Guard rebuild history") {
		return
	}
	h.ensureDB(ctx)
	if h.manager == nil {
		SendJSON(ctx, map[string]any{"history": []any{}})
		return
	}
	limit, _ := strconv.Atoi(string(ctx.QueryArgs().Peek("limit")))
	rows, err := h.manager.ListGuardRebuilds(ctx, limit)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"history": rows})
}

func (h *BrowserAIHandler) rebuildSetupPackages(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to rebuild Guard packages") {
		return
	}
	started := time.Now()

	_, winOK := findFirstExisting(append(
		browserAISetupCandidates()["Raksha_Guard_Setup.exe"],
		browserAISetupCandidates()["Raksha_Guard.exe"]...,
	))
	_, macOK := findFirstExisting(browserAISetupCandidates()["Raksha_Guard_macOS.zip"])

	publishScript, scriptOK := findFirstExisting([]string{
		filepath.Join("apps", "browser-guard", "scripts", "publish_fleet_packages.py"),
		filepath.Join("scripts", "publish_fleet_packages.py"),
	})
	pythonBin := ""
	for _, name := range []string{
		filepath.Join("apps", "browser-guard", ".venv-guard", "bin", "python"),
		"/opt/homebrew/bin/python3.12",
		"/opt/homebrew/bin/python3",
		"python3",
		"python",
	} {
		if strings.Contains(name, string(filepath.Separator)) || strings.HasPrefix(name, "/") {
			if info, err := os.Stat(name); err == nil && !info.IsDir() {
				pythonBin = name
				break
			}
		} else if p, err := exec.LookPath(name); err == nil {
			pythonBin = p
			break
		}
	}
	if !scriptOK || pythonBin == "" {
		// Docker runtime image ships prebuilt Guard binaries only (no Python / build tools):
		// publish the proxy code hot-update and report the installers being served.
		bundle, bundleMsg := publishProxyBundleSummary()
		resp := map[string]any{
			"status":        "success",
			"mode":          "prebuilt",
			"version":       readGuardReleaseVersion(),
			"mac_version":   readGuardMacReleaseVersion(),
			"windows_ready": winOK,
			"macos_ready":   macOK,
			"proxy_bundle":  bundle,
			"rebuilt_at":    time.Now().UTC().Format(time.RFC3339),
			"message": bundleMsg + " Installers served: v" + readGuardReleaseVersion() + ". " +
				"Only new Python packages or installer changes need a new version built on the build machine (Windows: scripts/publish_fleet_packages.py, macOS: make build-guard-mac).",
		}
		status := "success"
		if _, failed := bundle["error"]; failed {
			status = "failed"
		}
		resp["history_id"] = h.recordRebuild(ctx, started, status, resp, "")
		SendJSON(ctx, resp)
		return
	}

	// Prefer repo root (parent of apps/) so relative release paths resolve even if the
	// process cwd is not the product root (common under Docker / service wrappers).
	workDir := "."
	if abs, err := filepath.Abs(publishScript); err == nil {
		// .../apps/browser-guard/scripts/publish_fleet_packages.py → product root
		scriptsDir := filepath.Dir(abs)
		bgDir := filepath.Dir(scriptsDir)
		appsDir := filepath.Dir(bgDir)
		candidate := filepath.Dir(appsDir)
		if info, err := os.Stat(filepath.Join(candidate, "apps", "browser-guard")); err == nil && info.IsDir() {
			workDir = candidate
		}
	}

	// Full Windows PyInstaller+Inno can take several minutes.
	cmd := exec.Command(pythonBin, publishScript)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	logTail := strings.TrimSpace(string(out))
	if len(logTail) > 4000 {
		logTail = logTail[len(logTail)-4000:]
	}
	if err != nil {
		msg := "Guard package publish failed"
		if logTail != "" {
			msg = msg + ": " + logTail
		} else {
			msg = msg + ": " + err.Error()
		}
		h.recordRebuild(ctx, started, "failed", map[string]any{
			"mode":        "rebuilt",
			"version":     readGuardReleaseVersion(),
			"mac_version": readGuardMacReleaseVersion(),
			"message":     "Guard package publish failed: " + err.Error(),
		}, logTail)
		SendError(ctx, fasthttp.StatusInternalServerError, msg)
		return
	}

	releaseVer := readGuardReleaseVersion()
	macVer := readGuardMacReleaseVersion()
	serverDomain := os.Getenv("SERVER_DOMAIN")
	if serverDomain == "" {
		serverDomain = os.Getenv("RAKSHA_BACKEND_URL")
	}

	_, winOK = findFirstExisting(append(
		browserAISetupCandidates()["Raksha_Guard_Setup.exe"],
		browserAISetupCandidates()["Raksha_Guard.exe"]...,
	))
	_, macOK = findFirstExisting(browserAISetupCandidates()["Raksha_Guard_macOS.zip"])
	bundle, bundleMsg := publishProxyBundleSummary()

	resp := map[string]any{
		"status":        "success",
		"mode":          "rebuilt",
		"version":       releaseVer,
		"mac_version":   macVer,
		"windows_ready": winOK,
		"macos_ready":   macOK,
		"proxy_bundle":  bundle,
		"server_domain": serverDomain,
		"rebuilt_at":    time.Now().UTC().Format(time.RFC3339),
		"log":           logTail,
		"message": "Guard packages published for version " + releaseVer +
			". Windows and macOS employees with Guard installed (matching guard_secret) silent-update on the next auto-update check. " +
			bundleMsg,
	}
	status := "success"
	if _, failed := bundle["error"]; failed {
		status = "failed"
	}
	resp["history_id"] = h.recordRebuild(ctx, started, status, resp, logTail)
	SendJSON(ctx, resp)
}
