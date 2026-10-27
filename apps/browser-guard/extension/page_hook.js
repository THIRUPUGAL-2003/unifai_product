/* YP-Raksha Enterprise AI Guard Hook (Page World)
 * Full In-Browser DLP: Prompt Interception, All File Types, Images, Fast-Path Regex & Cloud AI
 */
(function () {
  function isProductHost(host) {
    if (!host) return false;
    var h = String(host || "").toLowerCase().trim();
    if (h.indexOf("://") >= 0) { try { h = new URL(h).hostname; } catch (e) { return false; } }
    else { h = h.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0]; if ((h.match(/:/g) || []).length === 1 && /:\d+$/.test(h)) h = h.replace(/:\d+$/, ""); }
    if (h.charAt(0) === "[" && h.indexOf("]") >= 0) h = h.slice(1, h.indexOf("]"));
    h = h.replace(/^www\./, "").replace(/\.$/, "");
    if (h === "localhost" || h === "127.0.0.1" || h === "0.0.0.0" || h === "::1") return true;
    var targetServer = (typeof GATEWAY_SERVER_HOST === "string" && GATEWAY_SERVER_HOST.trim())
      ? GATEWAY_SERVER_HOST.toLowerCase().trim().replace(/^www\./, "").replace(/\.$/, "")
      : "";
    if (targetServer && (h === targetServer || h.endsWith("." + targetServer))) return true;
    return false;
  }
  if (isProductHost(location.hostname)) {
    return;
  }

  var armed = false; // Armed only when confirmed by extension for monitored AI domains
  var curScript = document.currentScript;
  var guardNonce = (curScript && curScript.dataset && curScript.dataset.guardNonce) || "";
  var passKey = false;
  var passClick = false;
  var pending = {};
  var stagedFiles = [];
  var stagedFilesCache = {};
  var stagedImages = [];
  var lastCreatedFileName = "";

  function pushStagedFile(item) {
    if (!item || !item.name) return;
    var now = Date.now();
    item.timestamp = now;
    var key = String(item.name).toLowerCase().trim();
    var existing = stagedFilesCache[key] || stagedFilesCache[item.name];
    if (existing) {
      if (!item.text && existing.text) item.text = existing.text;
      if (!item.base64 && existing.base64) item.base64 = existing.base64;
      if (!item.size && existing.size) item.size = existing.size;
      if (!item.type && existing.type) item.type = existing.type;
    }
    stagedFilesCache[key] = item;
    stagedFilesCache[item.name] = item;
    for (var s = 0; s < stagedFiles.length; s++) {
      if (stagedFiles[s].name === item.name || String(stagedFiles[s].name).toLowerCase().trim() === key) {
        stagedFiles[s] = item;
        return;
      }
    }
    stagedFiles.push(item);
  }

  function getAllFreshStagedFiles() {
    var now = Date.now();
    var map = {};
    for (var s = 0; s < stagedFiles.length; s++) {
      var item = stagedFiles[s];
      if (item && item.name && (now - (item.timestamp || 0) < 180000)) {
        map[String(item.name).toLowerCase().trim()] = item;
      }
    }
    var keys = Object.keys(stagedFilesCache);
    for (var k = 0; k < keys.length; k++) {
      var entry = stagedFilesCache[keys[k]];
      if (entry && entry.name && (now - (entry.timestamp || 0) < 180000)) {
        var ckey = String(entry.name).toLowerCase().trim();
        if (!map[ckey]) {
          map[ckey] = entry;
        } else if ((!map[ckey].base64 && !map[ckey].text) && (entry.base64 || entry.text)) {
          map[ckey] = entry;
        }
      }
    }
    var result = [];
    var mkeys = Object.keys(map);
    for (var m = 0; m < mkeys.length; m++) {
      result.push(map[mkeys[m]]);
    }
    return result;
  }

  function getCachedStagedFile(name) {
    if (!name) return null;
    var now = Date.now();
    var clean = String(name).toLowerCase().trim();
    if (stagedFilesCache[clean] && (now - (stagedFilesCache[clean].timestamp || 0) < 180000)) {
      return stagedFilesCache[clean];
    }
    if (stagedFilesCache[name] && (now - (stagedFilesCache[name].timestamp || 0) < 180000)) {
      return stagedFilesCache[name];
    }
    var keys = Object.keys(stagedFilesCache);
    for (var k = 0; k < keys.length; k++) {
      var entry = stagedFilesCache[keys[k]];
      if (entry && (now - (entry.timestamp || 0) < 180000)) {
        var eName = String(entry.name || "").toLowerCase().trim();
        if (clean === eName || clean.endsWith(eName) || eName.endsWith(clean)) {
          return entry;
        }
      }
    }
    for (var s = 0; s < stagedFiles.length; s++) {
      if (stagedFiles[s].name === name || String(stagedFiles[s].name).toLowerCase().trim() === clean) {
        return stagedFiles[s];
      }
    }
    var freshEntries = [];
    for (var f = 0; f < keys.length; f++) {
      var fe = stagedFilesCache[keys[f]];
      if (fe && (now - (fe.timestamp || 0) < 90000)) {
        if (!freshEntries.some(function (x) { return x === fe; })) {
          freshEntries.push(fe);
        }
      }
    }
    if (freshEntries.length === 1) {
      return freshEntries[0];
    }
    return null;
  }
  var _lastHandledPromptTime = 0;
  var _lastHandledPromptText = "";
  var _lastHandledPromptId = "";
  var lastDecisionCache = { text: "", time: 0, decision: null };

  function stripInvisible(str) {
    if (!str) return "";
    return String(str)
      .replace(/[\u200b-\u200f\ufeff\u202a-\u202e\u2060-\u206f\u00a0]/g, "")
      .replace(/\s+/g, " ")
      .trim();
  }

  // Rules come only from the gateway for the armed target site.
  var dynamicRules = [];

  function compileRegex(rawPattern) {
    if (!rawPattern || typeof rawPattern !== "string") return null;
    var pattern = rawPattern.trim();
    var flags = "i";
    while (pattern.startsWith("(?i)") || pattern.startsWith("(?m)")) {
      if (pattern.startsWith("(?i)")) {
        if (flags.indexOf("i") === -1) flags += "i";
        pattern = pattern.slice(4);
      }
      if (pattern.startsWith("(?m)")) {
        if (flags.indexOf("m") === -1) flags += "m";
        pattern = pattern.slice(4);
      }
    }
    // In DLP prompt/file scanning, normalize full-string anchors ^...$ so patterns like ^[1-9][0-9]{5}$
    // match sensitive tokens inside full sentences (e.g. "613002 this is new")
    if (pattern.startsWith("^") && pattern.endsWith("$") && pattern.length > 2) {
      var inner = pattern.slice(1, -1);
      if (!/(?:^|[^\\])[\^$]/.test(inner)) {
        pattern = "\\b(?:" + inner + ")\\b";
      }
    }
    try {
      return new RegExp(pattern, flags);
    } catch (e) {
      console.warn("[Gateway Guard] Invalid pattern from DB:", rawPattern, e);
      return null;
    }
  }

  function setDynamicRules(rules) {
    if (!Array.isArray(rules) || rules.length === 0) {
      dynamicRules = [];
      return;
    }
    var compiled = [];
    for (var i = 0; i < rules.length; i++) {
      var r = rules[i];
      if (!r || !r.pattern) continue;
      var rx = compileRegex(r.pattern);
      if (rx) {
        compiled.push({
          id: r.id || "",
          name: r.name || "Custom Rule",
          action: (r.action || "BLOCK").toUpperCase(),
          msg: r.warning_message || ("Blocked by security policy: " + (r.name || "Restricted pattern")),
          re: rx
        });
      }
    }
    dynamicRules = compiled;
  }

  // Listen for messages from extension content script
  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard") return;
    if (!guardNonce || ev.data.nonce !== guardNonce) return;
    if (ev.data.type === "arm") {
      if (isProductOrServerHost(location.hostname)) {
        armed = false;
        return;
      }
      armed = true;
    }
    if (ev.data.type === "disarm") {
      armed = false;
      dynamicRules = [];
      lastDecisionCache = { text: "", time: 0, decision: null };
    }
    if (ev.data.type === "sync_rules") {
      lastCheckedKey = "";
      setTimeout(function () {
        if (armed) {
          scanStorageForSession();
          scanDOMForSession();
        }
      }, 300);
      setDynamicRules(ev.data.rules || []);
      if (typeof ev.data.block_upload !== "undefined") {
        govBlockUpload = Boolean(ev.data.block_upload);
      }
      if (typeof ev.data.upload_warning === "string") {
        govUploadWarning = ev.data.upload_warning;
      }
      if (typeof ev.data.upload_target_websites === "string") {
        govUploadTargetWebsites = ev.data.upload_target_websites;
      }
      if (typeof ev.data.file_extension_mode === "string") {
        govFileExtensionMode = ev.data.file_extension_mode;
      }
      if (typeof ev.data.restricted_file_extensions === "string") {
        govRestrictedExtensions = parseExtensionList(ev.data.restricted_file_extensions);
      } else if (Array.isArray(ev.data.restricted_extensions)) {
        govRestrictedExtensions = ev.data.restricted_extensions.map(function (x) { return String(x).toLowerCase().replace(/^\./, "").trim(); });
      }
      if (typeof ev.data.allowed_file_extensions === "string") {
        govAllowedExtensions = parseExtensionList(ev.data.allowed_file_extensions);
      }
      if (typeof ev.data.block_file_extension !== "undefined") {
        govBlockFileExtension = Boolean(ev.data.block_file_extension);
      }
      if (typeof ev.data.file_extension_block_message === "string") {
        govFileExtensionBlockMessage = ev.data.file_extension_block_message;
      }
      if (typeof ev.data.file_extension_target_websites === "string") {
        govFileExtensionTargetWebsites = ev.data.file_extension_target_websites;
      }
      if (typeof ev.data.block_clipboard !== "undefined") {
        govBlockClipboard = Boolean(ev.data.block_clipboard);
      }
      if (typeof ev.data.block_drag_drop !== "undefined") {
        govBlockDragDrop = Boolean(ev.data.block_drag_drop);
      }
      if (typeof ev.data.block_download !== "undefined") {
        govBlockDownload = Boolean(ev.data.block_download);
      }
      if (typeof ev.data.governance_target_websites === "string") {
        govTargetWebsites = ev.data.governance_target_websites;
      }
      if (typeof ev.data.governance_block_message === "string") {
        govBlockMessage = ev.data.governance_block_message;
      }
      if (typeof ev.data.clipboard_target_websites === "string") {
        govClipboardTargetWebsites = ev.data.clipboard_target_websites;
      }
      if (typeof ev.data.drag_drop_target_websites === "string") {
        govDragDropTargetWebsites = ev.data.drag_drop_target_websites;
      }
      if (typeof ev.data.download_target_websites === "string") {
        govDownloadTargetWebsites = ev.data.download_target_websites;
      }
      if (typeof ev.data.clipboard_block_message === "string") {
        govClipboardBlockMessage = ev.data.clipboard_block_message;
      }
      if (typeof ev.data.drag_drop_block_message === "string") {
        govDragDropBlockMessage = ev.data.drag_drop_block_message;
      }
      if (typeof ev.data.download_block_message === "string") {
        govDownloadBlockMessage = ev.data.download_block_message;
      }
      if (typeof ev.data.block_screenshot !== "undefined") {
        govBlockScreenshot = Boolean(ev.data.block_screenshot);
        applyScreenshotProtection();
      }
      if (typeof ev.data.screenshot_block_message === "string") {
        govScreenshotBlockMessage = ev.data.screenshot_block_message;
      }
      if (typeof ev.data.screenshot_target_websites === "string") {
        govScreenshotTargetWebsites = ev.data.screenshot_target_websites;
      }
    }
    if (ev.data.type === "session_authorization_result") {
      handleSessionAuthDecision(ev.data.result || {});
    }
    if (ev.data.type === "decision" && pending[ev.data.id]) {
      pending[ev.data.id](ev.data.decision || { blocked: false });
      delete pending[ev.data.id];
    }
  });

  // --- Data Governance Controls (Uploads, File Types, Clipboard, Drag & Drop, Downloads, Screenshots) ---
  var govBlockUpload = false;
  var govUploadWarning = "";
  var govUploadTargetWebsites = "";
  var govBlockFileExtension = false;
  var govFileExtensionMode = "block_listed";
  var govRestrictedExtensions = [];
  var govAllowedExtensions = [];
  var govFileExtensionBlockMessage = "";
  var govFileExtensionTargetWebsites = "";
  var govBlockClipboard = false;
  var govClipboardTargetWebsites = "";
  var govBlockDragDrop = false;
  var govDragDropTargetWebsites = "";
  var govBlockDownload = false;
  var govDownloadTargetWebsites = "";
  var govTargetWebsites = "";
  var govBlockMessage = "";
  var govClipboardBlockMessage = "";
  var govDragDropBlockMessage = "";
  var govDownloadBlockMessage = "";
  var govBlockScreenshot = false;
  var govScreenshotBlockMessage = "";
  var govScreenshotTargetWebsites = "";

  function parseExtensionList(str) {
    if (!str || typeof str !== "string") return [];
    var parts = str.toLowerCase().split(/[,\s;]+/);
    var list = [];
    for (var i = 0; i < parts.length; i++) {
      var item = parts[i].replace(/^\./, "").trim();
      if (item && list.indexOf(item) === -1) {
        list.push(item);
      }
    }
    return list;
  }

  function normalizeDomainHost(value) {
    var host = String(value || "").toLowerCase().trim();
    if (!host) return "";
    if (host.indexOf("://") >= 0) {
      try { host = new URL(host).hostname; } catch (e) { return ""; }
    } else {
      host = host.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0];
      if ((host.match(/:/g) || []).length === 1 && /:\d+$/.test(host)) host = host.replace(/:\d+$/, "");
    }
    if (host.charAt(0) === "[" && host.indexOf("]") >= 0) host = host.slice(1, host.indexOf("]"));
    return host.replace(/^www\./, "").replace(/\.$/, "");
  }

  function isProductOrServerHost(host) {
    if (!host) return false;
    var h = normalizeDomainHost(host);
    if (h === "localhost" || h === "127.0.0.1" || h === "0.0.0.0" || h === "::1") return true;
    var targetServer = (typeof GATEWAY_SERVER_HOST === "string" && GATEWAY_SERVER_HOST.trim())
      ? normalizeDomainHost(GATEWAY_SERVER_HOST)
      : "";
    if (targetServer && (h === targetServer || h.endsWith("." + targetServer))) return true;
    return false;
  }

  function isControlActiveForSite(configuredSites) {
    if (isProductOrServerHost(location.hostname)) return false;
    var raw = (configuredSites && typeof configuredSites === "string") ? configuredSites.trim() : "";
    if (!raw || raw.toUpperCase() === "ALL" || raw === "*") {
      return true;
    }
    if (raw.toLowerCase() === "none") return false;
    var currentHost = normalizeDomainHost(location.hostname);
    var parts = raw.toLowerCase().split(/[,\s;]+/);
    for (var i = 0; i < parts.length; i++) {
      var item = parts[i].trim().replace(/^\*\./, "").replace(/^\./, "");
      if (!item) continue;
      if (item.indexOf("://") < 0) {
        item = item.indexOf(":") >= 0 && (item.match(/:/g) || []).length > 1 ? "https://[" + item + "]" : "https://" + item;
      }
      item = normalizeDomainHost(item);
      if (item && (currentHost === item || currentHost.endsWith("." + item))) {
        return true;
      }
    }
    return false;
  }

  function isGovernanceActiveForSite() {
    return isControlActiveForSite(govTargetWebsites);
  }

  var MIME_TO_EXT_MAP = {
    "application/x-msdownload": "exe",
    "application/x-dosexec": "exe",
    "application/x-executable": "exe",
    "application/x-msdos-program": "exe",
    "application/x-sh": "sh",
    "application/x-bat": "bat",
    "application/zip": "zip",
    "application/x-zip-compressed": "zip",
    "application/x-zip": "zip",
    "application/x-7z-compressed": "7z",
    "application/x-tar": "tar",
    "application/gzip": "gz",
    "application/x-gzip": "gz",
    "application/x-bzip2": "bz2",
    "application/vnd.rar": "rar",
    "application/x-rar-compressed": "rar",
    "application/pdf": "pdf",
    "application/sql": "sql",
    "application/x-sql": "sql",
    "text/x-sql": "sql",
    "text/sql": "sql",
    "application/x-pem-file": "pem",
    "application/x-x509-ca-cert": "pem",
    "application/pkix-cert": "pem",
    "application/json": "json",
    "text/csv": "csv",
    "text/plain": "txt"
  };

  function sniffMagicFileType(bytesOrB64, textContent) {
    if (typeof bytesOrB64 === "string" && bytesOrB64.length > 4) {
      if (bytesOrB64.startsWith("TVqQ")) return "exe"; // MZ header
      if (bytesOrB64.startsWith("UEsDB") || bytesOrB64.startsWith("UEsBA")) return "zip"; // PK zip header
      if (bytesOrB64.startsWith("N3r8")) return "7z"; // 7z header
      if (bytesOrB64.startsWith("H4sI")) return "gz"; // gzip header
      if (bytesOrB64.startsWith("UmFy")) return "rar"; // Rar header
      if (bytesOrB64.startsWith("f0VMR")) return "elf"; // ELF binary
      if (bytesOrB64.startsWith("JVBER")) return "pdf"; // %PDF-
    }
    if (typeof textContent === "string" && textContent.length > 5) {
      var s = textContent.trim();
      if (s.indexOf("-----BEGIN ") === 0 || s.indexOf("-----BEGIN RSA PRIVATE KEY") >= 0 || s.indexOf("-----BEGIN PRIVATE KEY") >= 0 || s.indexOf("-----BEGIN CERTIFICATE") >= 0) {
        return "pem";
      }
      if (/^(?:--|\/\*|CREATE\s+|INSERT\s+INTO|DROP\s+TABLE|ALTER\s+TABLE|USE\s+|SELECT\s+.+\s+FROM|LOCK\s+TABLES)\b/i.test(s)) {
        return "sql";
      }
      if (/(?:API_KEY|SECRET|PASSWORD|DATABASE_URL|AWS_ACCESS_KEY_ID)\s*=\s*[^\r\n]+/i.test(s)) {
        return "env";
      }
    }
    return "";
  }

  function checkFileUploadAllowed(file, sniffedType) {
    if (!file) return true;
    var fileName = file.name || "";
    var fileType = String(file.type || "").toLowerCase().trim();
    // Check 1: Master upload block - applies to any target domain if govUploadTargetWebsites is empty/all or matches
    if (govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
      showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
      return false;
    }
    // Check 2: File extension and true file type restrictions (anti-spoofing)
    var fileExtTarget = govFileExtensionTargetWebsites || govUploadTargetWebsites;
    if (govBlockFileExtension && isControlActiveForSite(fileExtTarget)) {
      var lowerFileName = String(fileName).toLowerCase();
      var mimeExt = MIME_TO_EXT_MAP[fileType] || "";
      var effectiveType = sniffedType || mimeExt || "";
      var hasAllowed = govAllowedExtensions && govAllowedExtensions.length > 0;

      if (govFileExtensionMode === "allow_listed") {
        var isAllowed = hasAllowed && govAllowedExtensions.some(function (a) {
          var ext = String(a).replace(/^\./, "").toLowerCase().trim();
          return ext && (lowerFileName.endsWith("." + ext) || mimeExt === ext || sniffedType === ext);
        });
        if (!isAllowed) {
          var msg = govFileExtensionBlockMessage || (hasAllowed
            ? ("File type for '" + fileName + "' is not permitted. Only approved file types (" + govAllowedExtensions.map(function (e) { return "." + e; }).join(", ") + ") may be uploaded.")
            : "No file types are currently permitted by corporate policy.");
          showBubble(msg, "File Extension Policy", "block");
          return false;
        }
      } else {
        var blockedExtension = govRestrictedExtensions && govRestrictedExtensions.find(function (b) {
          var ext = String(b).replace(/^\./, "").toLowerCase().trim();
          return ext && (lowerFileName.endsWith("." + ext) || mimeExt === ext || sniffedType === ext);
        });
        if (blockedExtension) {
          var cleanBlockedExtension = String(blockedExtension).replace(/^\./, "").toLowerCase().trim();
          var isRenamed = sniffedType && sniffedType === cleanBlockedExtension && !lowerFileName.endsWith("." + cleanBlockedExtension);
          var msg2 = govFileExtensionBlockMessage || (isRenamed
            ? ("File '" + fileName + "' matches restricted file type '." + cleanBlockedExtension + "' (file renaming detected). Upload blocked by security policy.")
            : ("Uploading '." + cleanBlockedExtension + "' files is restricted by corporate DLP policy."));
          showBubble(msg2, "File Extension Policy", "block");
          return false;
        }
      }
    }
    return true;
  }

  // --- Anti-Screenshot & Screen Capture Protection ---
  var _screenshotShieldEl = null;
  function triggerScreenshotAlert() {
    if (!armed || !govBlockScreenshot || !isControlActiveForSite(govScreenshotTargetWebsites)) return;

    // NOTE: Do NOT call navigator.clipboard.writeText("") here.
    // That fires a clipboard event which triggers the clipboard blocked banner
    // simultaneously with the screenshot shield — two banners collide on screen.
    // The full-screen overlay already blacks out the content, no clipboard clear needed.

    // Remove any existing clipboard notice so it doesn't overlap the screenshot shield
    try {
      var clipNotice = document.getElementById("yp-raksha-isolated-clipboard-notice");
      if (clipNotice) clipNotice.remove();
    } catch (_) {}

    // 2. Display full-screen Privacy Shield overlay
    if (!_screenshotShieldEl) {
      _screenshotShieldEl = document.createElement("div");
      _screenshotShieldEl.id = "yp-screenshot-shield";
      _screenshotShieldEl.style.cssText =
        "position:fixed;inset:0;z-index:2147483647;background:rgba(10,15,29,0.98);backdrop-filter:blur(40px);" +
        "-webkit-backdrop-filter:blur(40px);display:flex;flex-direction:column;align-items:center;justify-content:center;" +
        "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#f8fafc;padding:24px;text-align:center;" +
        "transition:opacity 0.25s ease-out;opacity:0;pointer-events:none;";
      _screenshotShieldEl.innerHTML =
        '<div style="width:72px;height:72px;border-radius:50%;background:rgba(239,68,68,0.18);border:2px solid #ef4444;display:flex;align-items:center;justify-content:center;font-size:36px;margin-bottom:16px;">🛡️</div>' +
        '<h2 style="font-size:22px;font-weight:700;letter-spacing:0.04em;text-transform:uppercase;color:#f87171;margin:0 0 8px 0;">CAPTURE SHORTCUT DETECTED</h2>' +
        '<p style="font-size:14px;color:#cbd5e1;max-width:440px;line-height:1.5;margin:0 0 16px 0;">' +
        (govScreenshotBlockMessage || "A browser print or common screenshot shortcut was detected. Operating-system capture tools cannot be reliably blocked from a webpage.") +
        '</p>' +
        '<div style="font-size:11px;color:#94a3b8;border:1px solid rgba(148,163,184,0.3);padding:4px 12px;border-radius:6px;background:rgba(30,41,59,0.6);">' +
        'Monitored &amp; Protected by YP-Raksha Browser Guard' +
        '</div>';
      (document.body || document.documentElement).appendChild(_screenshotShieldEl);
    }
    _screenshotShieldEl.style.opacity = "1";
    _screenshotShieldEl.style.pointerEvents = "all";

    // Dismiss any existing toast to keep only the main full-screen shield
    var oldBubble = document.getElementById("yp-raksha-alert-card");
    if (oldBubble) oldBubble.remove();

    // Fade out after 2.5 seconds
    setTimeout(function () {
      if (_screenshotShieldEl) {
        _screenshotShieldEl.style.opacity = "0";
        _screenshotShieldEl.style.pointerEvents = "none";
      }
    }, 2500);
  }

  function applyScreenshotProtection() {
    var printStyleId = "yp-print-shield-style";
    var existingPrintStyle = document.getElementById(printStyleId);
    if (govBlockScreenshot && isControlActiveForSite(govScreenshotTargetWebsites)) {
      if (!existingPrintStyle) {
        var style = document.createElement("style");
        style.id = printStyleId;
        style.textContent = "@media print { body, html { display: none !important; visibility: hidden !important; } }";
        (document.head || document.documentElement).appendChild(style);
      }
    } else {
      if (existingPrintStyle) existingPrintStyle.remove();
      var bs = document.getElementById("yp-blur-shield-style");
      if (bs) bs.remove();
    }
  }

  // Defeat OS Snipping tools (Win+Shift+S, Lightshot, ShareX) via Window Blur & Opaque Curtain Shield:
  // When an OS screenshot tool or snip overlay opens, the browser window immediately
  // loses focus (blur event). Showing an opaque curtain ensures the capture grabs only a blackout screen.
  var _blurShieldStyleId = "yp-blur-shield-style";
  var _blurCurtainId = "yp-blur-curtain";

  function applyScreenshotBlur() {
    if (!armed || !govBlockScreenshot || !isControlActiveForSite(govScreenshotTargetWebsites)) return;
    if (!document.getElementById(_blurShieldStyleId)) {
      var bs = document.createElement("style");
      bs.id = _blurShieldStyleId;
      bs.textContent = "body, html { filter: blur(35px) !important; -webkit-filter: blur(35px) !important; user-select: none !important; }";
      (document.head || document.documentElement).appendChild(bs);
    }
    if (!document.getElementById(_blurCurtainId)) {
      var curtain = document.createElement("div");
      curtain.id = _blurCurtainId;
      curtain.style.cssText =
        "position:fixed;inset:0;width:100vw;height:100vh;background:#0a0f1d;opacity:0.98;backdrop-filter:blur(30px);" +
        "-webkit-backdrop-filter:blur(30px);z-index:2147483646;pointer-events:none;display:flex;align-items:center;" +
        "justify-content:center;color:#f87171;font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:20px;font-weight:700;";
      curtain.innerHTML = '<div style="display:flex;align-items:center;gap:10px;"><span style="font-size:28px;">🛡️</span><span>SCREEN CAPTURE RESTRICTED</span></div>';
      (document.body || document.documentElement).appendChild(curtain);
    }
  }

  function removeScreenshotBlur() {
    var bs = document.getElementById(_blurShieldStyleId);
    if (bs) bs.remove();
    var curtain = document.getElementById(_blurCurtainId);
    if (curtain) curtain.remove();
  }

  window.addEventListener("blur", applyScreenshotBlur, true);
  window.addEventListener("focus", removeScreenshotBlur, true);
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) applyScreenshotBlur();
    else removeScreenshotBlur();
  }, true);

  // Intercept Screen Recording & Screen Sharing API (navigator.mediaDevices.getDisplayMedia)
  try {
    if (navigator.mediaDevices && typeof navigator.mediaDevices.getDisplayMedia === "function") {
      var _origGetDisplayMedia = navigator.mediaDevices.getDisplayMedia.bind(navigator.mediaDevices);
      navigator.mediaDevices.getDisplayMedia = function () {
        if (armed && govBlockScreenshot && isControlActiveForSite(govScreenshotTargetWebsites)) {
          triggerScreenshotAlert();
          return Promise.reject(new DOMException("Screen capture and recording are blocked by corporate security policy.", "NotAllowedError"));
        }
        return _origGetDisplayMedia.apply(this, arguments);
      };
    }
  } catch (_) {}

  // Intercept PrintScreen key, Snipping tool shortcuts (Win+Shift+S), Mac Cmd+Shift+3/4
  function handleCaptureKey(e) {
    if (!armed || !govBlockScreenshot || !isControlActiveForSite(govScreenshotTargetWebsites)) return;
    var isPrtScn = e.key === "PrintScreen" || e.keyCode === 44 || e.code === "PrintScreen";
    var isWinShiftS = (e.key === "s" || e.key === "S" || e.code === "KeyS") && e.shiftKey && (e.metaKey || e.altKey);
    var isMacCapture = e.metaKey && e.shiftKey && (e.key === "3" || e.key === "4" || e.key === "5" || e.code === "Digit3" || e.code === "Digit4" || e.code === "Digit5");
    var isPrint = (e.ctrlKey || e.metaKey) && (e.key === "p" || e.key === "P" || e.code === "KeyP");

    if (isPrtScn || isWinShiftS || isMacCapture || isPrint) {
      try {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      } catch (_) {}
      triggerScreenshotAlert();
    }
  }

  window.addEventListener("keydown", handleCaptureKey, true);
  document.addEventListener("keydown", handleCaptureKey, true);
  window.addEventListener("keyup", function (e) {
    if (!armed || !govBlockScreenshot || !isControlActiveForSite(govScreenshotTargetWebsites)) return;
    if (e.key === "PrintScreen" || e.keyCode === 44 || e.code === "PrintScreen") {
      triggerScreenshotAlert();
    }
  }, true);

  // --- Corporate AI Domain & Workspace Authorization Engine ---
  var sessionBlocked = false;
  var sessionBlockedReason = "";
  var detectedEmail = "";
  var detectedWorkspace = "";
  var lastCheckedKey = "";

  function handleSessionAuthDecision(authRes) {
    if (!authRes) return;
    if (authRes.decision === "block") {
      sessionBlocked = true;
      sessionBlockedReason = authRes.message || "Personal accounts are blocked by corporate security policy.";
      renderCorporateBanner(true, sessionBlockedReason, detectedEmail);
    } else {
      sessionBlocked = false;
      sessionBlockedReason = "";
      renderCorporateBanner(false);
    }
  }

  function performAutoLogout() {
    try {
      sessionStorage.clear();
      for (var i = localStorage.length - 1; i >= 0; i--) {
        var k = localStorage.key(i) || "";
        var kLow = k.toLowerCase();
        if (kLow.includes("token") || kLow.includes("auth") || kLow.includes("session") || kLow.includes("user") || kLow.includes("oidc") || kLow.includes("profile")) {
          localStorage.removeItem(k);
        }
      }
    } catch (_) {}
    var h = (location.hostname || "").toLowerCase();
    if (h.includes("chatgpt.com") || h.includes("openai.com")) {
      location.href = "https://chatgpt.com/api/auth/signout";
    } else if (h.includes("claude.ai")) {
      location.href = "https://claude.ai/api/auth/logout";
    } else if (h.includes("gemini.google.com") || h.includes("aistudio.google.com")) {
      location.href = "https://accounts.google.com/AccountChooser?continue=" + encodeURIComponent(location.href);
    } else if (h.includes("deepseek.com")) {
      location.href = "https://chat.deepseek.com/sign_in";
    } else if (h.includes("copilot.microsoft.com")) {
      location.href = "https://login.microsoftonline.com/common/oauth2/v2.0/logout";
    } else if (h.includes("grok.com") || h.includes("x.ai")) {
      location.href = "https://grok.com/sign-in";
    } else if (h.includes("perplexity.ai")) {
      location.href = "https://www.perplexity.ai/api/auth/signout";
    } else if (h.includes("mistral.ai")) {
      location.href = "https://chat.mistral.ai/auth/login";
    } else if (h.includes("poe.com")) {
      location.href = "https://poe.com/login";
    } else if (h.includes("lovable.dev")) {
      location.href = "https://lovable.dev/login";
    } else {
      location.reload();
    }
  }

  function renderCorporateBanner(isBlocked, message, email) {
    var existing = document.getElementById("yp-corp-policy-banner");
    if (!isBlocked) {
      if (existing) existing.remove();
      return;
    }
    if (existing) {
      var msgEl = existing.querySelector(".yp-corp-msg");
      if (msgEl && message) msgEl.textContent = message;
      var emailEl = existing.querySelector(".yp-corp-email");
      if (emailEl && email) emailEl.textContent = email;
      return;
    }

    if (!document.getElementById("yp-corp-keyframes")) {
      var kf = document.createElement("style");
      kf.id = "yp-corp-keyframes";
      kf.textContent = "@keyframes ypFadeInModal{from{opacity:0;transform:scale(0.92)}to{opacity:1;transform:scale(1)}}" +
        "@keyframes ypFadeInOverlay{from{opacity:0}to{opacity:1}}";
      (document.head || document.documentElement).appendChild(kf);
    }

    // Centered modal overlay container
    var overlay = document.createElement("div");
    overlay.id = "yp-corp-policy-banner";
    overlay.style.cssText = "all:initial;position:fixed;top:0;left:0;right:0;bottom:0;width:100vw;height:100vh;" +
      "background:rgba(2,6,23,0.72);backdrop-filter:blur(8px);-webkit-backdrop-filter:blur(8px);z-index:2147483647;" +
      "display:flex;align-items:center;justify-content:center;padding:20px;box-sizing:border-box;pointer-events:auto;" +
      "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;animation:ypFadeInOverlay 0.25s ease-out;";

    var emailTag = email
      ? '<span class="yp-corp-email" style="font-size:12px;background:#1e293b;color:#fca5a5;padding:3px 10px;border-radius:20px;border:1px solid rgba(239,68,68,0.35);font-family:ui-monospace,SFMono-Regular,Consolas,monospace;word-break:break-all;">' + email + '</span>'
      : '';

    overlay.innerHTML =
      '<div style="max-width:520px;width:100%;background:linear-gradient(160deg, #0f172a 0%, #1e293b 100%);color:#f8fafc;' +
      'border:2px solid #ef4444;border-radius:18px;padding:32px 28px;box-shadow:0 30px 60px -12px rgba(0,0,0,0.85), 0 0 35px -5px rgba(239,68,68,0.3);' +
      'box-sizing:border-box;text-align:center;display:flex;flex-direction:column;align-items:center;gap:14px;animation:ypFadeInModal 0.3s cubic-bezier(0.16, 1, 0.3, 1);">' +
      '  <div style="background:rgba(239,68,68,0.16);border:2px solid #ef4444;color:#ef4444;border-radius:50%;width:52px;height:52px;display:flex;align-items:center;justify-content:center;font-size:24px;font-weight:bold;box-shadow:0 0 20px rgba(239,68,68,0.25);margin-bottom:2px;">✕</div>' +
      '  <div style="display:flex;flex-direction:column;align-items:center;gap:6px;width:100%;">' +
      '    <strong style="font-size:14px;letter-spacing:0.06em;text-transform:uppercase;color:#f87171;font-weight:800;">Corporate Policy Restriction</strong>' +
      emailTag +
      '  </div>' +
      '  <p class="yp-corp-msg" style="font-size:13.5px;color:#cbd5e1;line-height:1.55;margin:4px 0 8px 0;max-width:440px;">' +
      (message || "Personal accounts are restricted by corporate security policy. Please log in with your company account.") +
      '  </p>' +
      '  <button id="yp-btn-signout" style="background:#ef4444;color:#ffffff;border:none;border-radius:10px;padding:12px 32px;font-size:13px;font-weight:700;cursor:pointer;box-shadow:0 4px 14px rgba(239,68,68,0.4);transition:all 0.2s ease;letter-spacing:0.02em;">Sign Out</button>' +
      '  <div id="yp-corp-timer-notice" style="font-size:11.5px;color:#94a3b8;margin-top:2px;">Auto signing out in <span id="yp-corp-countdown" style="color:#f87171;font-weight:700;">8</span>s...</div>' +
      '</div>';

    (document.body || document.documentElement).appendChild(overlay);

    var btnSignout = document.getElementById("yp-btn-signout");
    if (btnSignout) {
      btnSignout.addEventListener("click", function () {
        performAutoLogout();
      });
      btnSignout.addEventListener("mouseenter", function () {
        btnSignout.style.background = "#dc2626";
        btnSignout.style.transform = "translateY(-1px)";
      });
      btnSignout.addEventListener("mouseleave", function () {
        btnSignout.style.background = "#ef4444";
        btnSignout.style.transform = "translateY(0)";
      });
    }

    var secondsLeft = 8;
    var timerEl = document.getElementById("yp-corp-countdown");
    var countdownInterval = setInterval(function () {
      secondsLeft--;
      if (timerEl) timerEl.textContent = String(secondsLeft);
      if (secondsLeft <= 0) {
        clearInterval(countdownInterval);
        if (sessionBlocked) {
          performAutoLogout();
        }
      }
    }, 1000);
  }

  function detectProviderName() {
    var h = (location.hostname || "").toLowerCase();
    if (h.includes("chatgpt.com") || h.includes("openai.com")) return "chatgpt";
    if (h.includes("claude.ai") || h.includes("anthropic.com")) return "claude";
    if (h.includes("copilot.microsoft.com") || h.includes("bing.com")) return "copilot";
    if (h.includes("gemini.google.com") || h.includes("bard.google.com") || h.includes("aistudio.google.com")) return "gemini";
    if (h.includes("grok.com") || h.includes("x.ai")) return "grok";
    if (h.includes("deepseek.com")) return "deepseek";
    if (h.includes("mistral.ai")) return "mistral";
    if (h.includes("perplexity.ai")) return "perplexity";
    if (h.includes("poe.com")) return "poe";
    if (h.includes("lovable.dev")) return "lovable";
    var parts = h.replace(/^www\./, "").split(".");
    return parts[0] || "ai";
  }

  var lastCheckedAt = 0;
  function triggerSessionAuth(email, workspaceId, force) {
    if (!email) return;
    email = String(email).trim().toLowerCase();
    var domain = email.indexOf("@") >= 0 ? email.split("@")[1].trim() : email;
    var ws = workspaceId ? String(workspaceId).trim() : "";
    var key = email + "::" + ws;
    var now = Date.now();
    // Re-verify periodically (every 15 min max) or immediately if email/workspace changed
    if (!force && key === lastCheckedKey && (now - lastCheckedAt < 900000)) return;
    lastCheckedKey = key;
    lastCheckedAt = now;
    detectedEmail = email;
    detectedWorkspace = ws;

    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      type: "authorize_ai_session",
      id: "auth_" + Date.now().toString(36),
      provider: detectProviderName(),
      email_domain: domain,
      user_email: email,
      workspace_id: ws,
      page_url: location.href,
      policy_version: 1
    }, "*");
  }

  // Universal deep object search for email and workspace keys across all auth formats
  function scanObjectForSession(obj, depth) {
    if (!obj || typeof obj !== "object" || (depth || 0) > 5) return;
    try {
      var candidate = obj.email || obj.email_address || obj.user_email || obj.upn || obj.mail ||
        obj.userPrincipalName || obj.preferred_username || obj.accountEmail || obj.account_email ||
        obj.primaryEmail || obj.primary_email;
      if (typeof candidate === "string" && candidate.includes("@")) {
        var ws = obj.workspace_id || obj.org_id || obj.organization_id || obj.account_id || obj.accountId || obj.tenant_id || "";
        triggerSessionAuth(candidate, ws);
        return;
      }
      if (typeof obj.sub === "string" && obj.sub.includes("@")) {
        triggerSessionAuth(obj.sub, obj.workspace_id || obj.org_id || "");
        return;
      }
      if (typeof obj.username === "string" && obj.username.includes("@")) {
        triggerSessionAuth(obj.username, obj.workspace_id || obj.org_id || "");
        return;
      }
      if (typeof obj.login === "string" && obj.login.includes("@")) {
        triggerSessionAuth(obj.login, obj.workspace_id || obj.org_id || "");
        return;
      }
      for (var k in obj) {
        if (Object.prototype.hasOwnProperty.call(obj, k) && typeof obj[k] === "object" && obj[k] !== null) {
          scanObjectForSession(obj[k], (depth || 0) + 1);
        }
      }
    } catch (e) {}
  }

  // Scan LocalStorage and SessionStorage for JWT or user profiles
  function scanStorageForSession() {
    try {
      var emailRegex = /\b[A-Za-z0-9._%+-]+@([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b/;
      var storages = [localStorage, sessionStorage];
      for (var s = 0; s < storages.length; s++) {
        var st = storages[s];
        if (!st) continue;
        for (var i = 0; i < st.length; i++) {
          var k = st.key(i);
          if (!k) continue;
          var val = st.getItem(k);
          if (!val || typeof val !== "string") continue;
          if (val.startsWith("{") && val.endsWith("}")) {
            try {
              scanObjectForSession(JSON.parse(val), 0);
            } catch (je) {}
          }
          if (val.startsWith("ey") && val.includes(".")) {
            try {
              var parts = val.split(".");
              if (parts.length >= 2) {
                var payload = JSON.parse(atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")));
                scanObjectForSession(payload, 0);
              }
            } catch (jte) {}
          }
          if (val.includes("@") && val.length < 8000) {
            var emMatch = val.match(emailRegex);
            if (emMatch && emMatch[0]) {
              var em = emMatch[0].toLowerCase();
              if (!em.includes("sentry") && !em.includes("example.com") && !em.includes("w3.org") && !em.includes("schema.org") && !em.includes("datadoghq") && !em.includes(".png") && !em.includes(".jpg")) {
                triggerSessionAuth(em, "");
              }
            }
          }
        }
      }
    } catch (e) {}
  }

  // Scan DOM for visible user email (profile avatars, Google Account chips, user menus)
  function scanDOMForSession() {
    try {
      var emailRegex = /\b[A-Za-z0-9._%+-]+@([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b/;
      var elements = querySelectorAllDeep("[aria-label*='@'], [title*='@'], [alt*='@'], [data-identifier*='@'], [data-email*='@'], [data-user*='@'], [data-testid*='profile'], [data-testid*='user'], [data-testid*='account'], a[href*='accounts.google.com'], img[alt*='Google Account']");
      for (var i = 0; i < elements.length; i++) {
        var el = elements[i];
        if (!el) continue;
        var txt = (el.getAttribute("aria-label") || "") + " " + (el.getAttribute("title") || "") + " " + (el.getAttribute("alt") || "") + " " + (el.getAttribute("data-identifier") || "") + " " + (el.getAttribute("data-email") || "") + " " + (el.innerText || "");
        var m = txt.match(emailRegex);
        if (m && m[0]) {
          var em = m[0].toLowerCase();
          if (!em.includes("sentry") && !em.includes("example.com") && !em.includes("w3.org") && !em.includes("schema.org")) {
            triggerSessionAuth(em, "");
            return;
          }
        }
      }
      if (typeof window.__NEXT_DATA__ === "object" && window.__NEXT_DATA__ !== null) {
        scanObjectForSession(window.__NEXT_DATA__, 0);
      }
      if (typeof window.__remixContext === "object" && window.__remixContext !== null) {
        scanObjectForSession(window.__remixContext, 0);
      }
      if (typeof window.__INITIAL_STATE__ === "object" && window.__INITIAL_STATE__ !== null) {
        scanObjectForSession(window.__INITIAL_STATE__, 0);
      }
      if (typeof window.WIZ_global_data === "object" && window.WIZ_global_data !== null) {
        scanObjectForSession(window.WIZ_global_data, 0);
        try {
          var wStr = JSON.stringify(window.WIZ_global_data);
          var wm = wStr.match(emailRegex);
          if (wm && wm[0] && !wm[0].includes("google.com/")) {
            triggerSessionAuth(wm[0].toLowerCase(), "");
            return;
          }
        } catch (_) {}
      }
      if (typeof window._gaUserInfo === "object" && window._gaUserInfo !== null) {
        scanObjectForSession(window._gaUserInfo, 0);
      }
    } catch (e) {}
  }

  // Client-Side Fast Evaluation against live DB rules
  function evaluateClientRules(text, files) {
    // Check prompt text and file text content against the target site's rules.
    var fullContent = text || "";
    if (Array.isArray(files)) {
      for (var j = 0; j < files.length; j++) {
        if (files[j] && files[j].text) {
          fullContent += "\n" + files[j].text;
        }
      }
    }

    var redactRules = [];
    var warnDecision = null;

    for (var k = 0; k < dynamicRules.length; k++) {
      var r = dynamicRules[k];
      if (r && r.re) {
        r.re.lastIndex = 0;
        if (r.re.test(fullContent)) {
          var act = String(r.action || "BLOCK").toUpperCase();
          if (act === "BLOCK" || act === "BLOCKED") {
            return {
              blocked: true,
              action: "Blocked",
              message: r.msg || ("Blocked by security rule: " + r.name),
              rule: r.name,
              matchedRule: r
            };
          }
          if (act === "REDACT" || act === "REDACTED") {
            redactRules.push(r);
          } else if (act === "WARN" || act === "WARNED") {
            if (!warnDecision) {
              warnDecision = {
                blocked: false,
                action: "Warned",
                message: r.msg || ("Security Warning: " + r.name + " detected."),
                rule: r.name,
                matchedRule: r
              };
            }
          }
        }
      }
    }

    if (redactRules.length > 0) {
      var redactedText = text || "";
      var ruleNames = [];
      for (var ri = 0; ri < redactRules.length; ri++) {
        var rr = redactRules[ri];
        ruleNames.push(rr.name);
        var flags = rr.re.flags || "";
        if (flags.indexOf("g") === -1) flags += "g";
        try {
          var reG = new RegExp(rr.re.source, flags);
          redactedText = redactedText.replace(reG, "[REDACTED]");
        } catch (eG) {
          redactedText = redactedText.replace(rr.re, "[REDACTED]");
        }
      }
      return {
        blocked: false,
        action: "Redacted",
        message: "Confidential text was redacted (" + ruleNames.join(", ") + ").",
        rule: ruleNames.join(", "),
        redacted_prompt: redactedText,
        matchedRules: redactRules
      };
    }

    if (warnDecision) {
      return warnDecision;
    }

    return null;
  }

  var _lastAskSig = "";
  var _lastAskTime = 0;
  var _lastAskDecision = null;

  // Ask Gateway Guard Engine (Async verify with PAC server & Gateway backend)
  function ask(prompt, files, images) {
    if (!armed) {
      return Promise.resolve({ blocked: false, passthrough: true });
    }
    if (sessionBlocked) {
      return Promise.resolve({
        blocked: true,
        action: "Blocked",
        message: sessionBlockedReason || "Personal accounts are restricted by corporate security policy. Please log in with your company account.",
        rule: "Corporate AI Account Enforcement"
      });
    }
    var trimmed = (prompt || "").trim();
    var fileNamesSig = (files || []).map(function (f) { return (f && (f.name || f)) || ""; }).sort().join(",");
    var sig = trimmed + "||" + fileNamesSig + "||" + ((images || []).length);
    if (sig && (Date.now() - _lastAskTime < 4000) && _lastAskSig === sig && _lastAskDecision) {
      return Promise.resolve(_lastAskDecision);
    }
    if (trimmed && (!files || files.length === 0) && (!images || images.length === 0)) {
      if (lastDecisionCache.text === trimmed && (Date.now() - lastDecisionCache.time < 5000) && lastDecisionCache.decision) {
        return Promise.resolve(lastDecisionCache.decision);
      }
    }
    return new Promise(function (resolve) {
      // 1. First run fast client-side check
      var clientDecision = evaluateClientRules(prompt, files);
      if (clientDecision && clientDecision.blocked) {
        _lastAskSig = sig;
        _lastAskTime = Date.now();
        _lastAskDecision = clientDecision;
        if (trimmed) {
          lastDecisionCache = { text: trimmed, time: Date.now(), decision: clientDecision };
        }
        // Asynchronously notify backend so the blocked prompt is recorded in PostgreSQL DB & dashboard!
        window.postMessage({
          source: "gateway-guard-page",
          nonce: guardNonce,
          id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
          prompt: prompt,
          files: files || [],
          upload_images: images || [],
          host: location.hostname,
          url: location.href,
          is_blocked: true,
          rule: clientDecision.rule || ""
        }, "*");

        resolve(clientDecision);
        return;
      }

      var cAct = String((clientDecision && clientDecision.action) || "").toLowerCase();
      if (cAct === "redacted" || cAct === "redact") {
        _lastAskSig = sig;
        _lastAskTime = Date.now();
        _lastAskDecision = clientDecision;
        if (trimmed) {
          lastDecisionCache = { text: trimmed, time: Date.now(), decision: clientDecision };
        }
        window.postMessage({
          source: "gateway-guard-page",
          nonce: guardNonce,
          id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
          prompt: clientDecision.redacted_prompt || prompt,
          files: files || [],
          upload_images: images || [],
          host: location.hostname,
          url: location.href,
          is_blocked: false,
          action: "Redacted",
          rule: clientDecision.rule || ""
        }, "*");

        resolve(clientDecision);
        return;
      }

      // 2. Query Gateway Guard PAC and Cloud DLP
      var id = Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
      var timer = setTimeout(function () {
        delete pending[id];
        resolve(clientDecision || { blocked: false, passthrough: true });
      }, 5000);

      pending[id] = function (decision) {
        clearTimeout(timer);
        var res = decision || clientDecision || { blocked: false };
        _lastAskSig = sig;
        _lastAskTime = Date.now();
        _lastAskDecision = res;
        _lastHandledPromptId = id;
        _lastHandledPromptTime = Date.now();
        _lastHandledPromptText = trimmed;
        if (trimmed && (!files || files.length === 0) && (!images || images.length === 0)) {
          lastDecisionCache = { text: trimmed, time: Date.now(), decision: res };
        }
        resolve(res);
      };

      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: id,
        prompt: prompt,
        files: files || [],
        upload_images: images || [],
        host: location.hostname,
        url: location.href,
        rule: clientDecision ? clientDecision.rule : ""
      }, "*");
    });
  }

  // Deep helper: pierce shadow roots to find currently focused element
  function getDeepActiveElement(root) {
    try {
      var active = (root || document).activeElement;
      while (active && active.shadowRoot && active.shadowRoot.activeElement) {
        active = active.shadowRoot.activeElement;
      }
      return active;
    } catch (e) {
      return (root || document).activeElement;
    }
  }

  // Deep helper: search selectors across light DOM and all open shadow roots
  function querySelectorAllDeep(selector, root) {
    root = root || document;
    var results = [];
    try {
      var direct = root.querySelectorAll(selector);
      for (var i = 0; i < direct.length; i++) results.push(direct[i]);
    } catch (e) {}

    try {
      var walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT, null, false);
      while (walker.nextNode()) {
        var n = walker.currentNode;
        if (n && n.shadowRoot) {
          var sub = querySelectorAllDeep(selector, n.shadowRoot);
          for (var j = 0; j < sub.length; j++) results.push(sub[j]);
        }
      }
    } catch (e2) {}
    return results;
  }

  // Normalize invisible characters, zero-width spaces, and bidi control markers
  function stripInvisible(str) {
    if (!str || typeof str !== "string") return "";
    return str
      .replace(/[\u200B-\u200F\uFEFF\u202A-\u202E\u2060-\u206F]/g, "")
      .replace(/\u00A0/g, " ")
      .trim();
  }

  // Universal Composer Extraction across ChatGPT, Claude, Gemini, Copilot, DeepSeek, Mistral, Lovable
  function composerFrom(el) {
    if (!el) return "";
    var node = el;

    // Check if element itself has value property (web component, textarea, input)
    if (typeof node.value === "string" && node.value) return node.value;

    // Check inside element's shadowRoot if present
    if (node.shadowRoot) {
      try {
        var shadowInput = node.shadowRoot.querySelector("textarea, [contenteditable='true'], input[type='text'], [role='textbox']");
        if (shadowInput) {
          var sVal = (shadowInput.tagName === "TEXTAREA" || shadowInput.tagName === "INPUT") ? (shadowInput.value || "") : (shadowInput.innerText || shadowInput.textContent || "");
          if (sVal && sVal.trim()) return sVal.trim();
        }
      } catch (se) {}
    }

    for (var i = 0; node && i < 6; i++, node = (node.parentElement || (node.getRootNode && node.getRootNode().host))) {
      if (node.tagName === "TEXTAREA" || node.tagName === "INPUT") return node.value || "";
      if (node.isContentEditable) {
        var root = (node.closest && node.closest("[contenteditable='true'], [role='textbox']")) || node;
        return root.innerText || root.textContent || "";
      }
      if (typeof node.value === "string" && node.value) return node.value;
    }

    // Check surrounding form / fieldset / composer container of the clicked button
    var container = (el && el.closest && el.closest("fieldset, form, [class*='composer' i], [class*='input' i], div[data-testid*='composer' i], div.relative")) || null;
    if (container) {
      var innerEd = container.querySelector("div.ProseMirror, [contenteditable='true'], textarea, input[type='text'], [role='textbox']");
      if (innerEd) {
        if (innerEd.tagName === "TEXTAREA" || innerEd.tagName === "INPUT") return innerEd.value || "";
        var ct = (innerEd.innerText || innerEd.textContent || "").trim();
        if (ct) return ct;
      }
    }

    var deepActive = getDeepActiveElement();
    if (deepActive && deepActive !== el) {
      if (deepActive.tagName === "TEXTAREA" || deepActive.tagName === "INPUT") return deepActive.value || "";
      if (deepActive.isContentEditable) {
        var aroot = (deepActive.closest && deepActive.closest("[contenteditable='true'], [role='textbox']")) || deepActive;
        return aroot.innerText || aroot.textContent || "";
      }
      if (typeof deepActive.value === "string" && deepActive.value) return deepActive.value;
    }

    // Specific platform selectors across document and deep shadow roots
    var candidates = querySelectorAllDeep(
      "textarea[placeholder*='Message' i], " +
      "textarea[placeholder*='Copilot' i], " +
      "[placeholder*='Message Copilot' i], " +
      "[aria-label*='Message Copilot' i], " +
      "[placeholder*='Ask Copilot' i], " +
      "[aria-label*='Ask Copilot' i], " +
      "[placeholder*='Ask me anything' i], " +
      "[aria-label*='Ask me anything' i], " +
      "[placeholder*='Message' i], " +
      "[aria-label*='Message' i], " +
      "textarea#userInput, " +
      "textarea[id*='userInput' i], " +
      "textarea#searchbox, " +
      "textarea[id*='chat' i], " +
      "copilot-chat-container textarea, " +
      "copilot-text-input textarea, " +
      "copilot-composer textarea, " +
      "[data-testid*='copilot' i], " +
      "[aria-label*='copilot' i] textarea, " +
      "[aria-label*='copilot' i] [contenteditable='true'], " +
      "cib-text-input textarea, " +
      "cib-serp textarea, " +
      "fieldset div.ProseMirror, " +
      "fieldset [contenteditable='true'], " +
      "form div.ProseMirror, " +
      "div#prompt-textarea, " +
      "div.ProseMirror, " +
      "rich-textarea div[contenteditable], " +
      "textarea[placeholder*='Ask' i], " +
      "textarea[placeholder*='prompt' i], " +
      "textarea[placeholder*='describe' i], " +
      "textarea[placeholder*='generate' i], " +
      "textarea[placeholder*='image' i], " +
      "textarea[placeholder*='video' i], " +
      "textarea[placeholder*='music' i], " +
      "textarea[placeholder*='DeepSeek'], " +
      "[data-testid*='chat-input' i], " +
      "[data-testid*='composer' i], " +
      "[data-testid*='prompt' i], " +
      "[contenteditable='true'][role='textbox']"
    );
    for (var j = 0; j < candidates.length; j++) {
      var c = candidates[j];
      if (!c) continue;
      // Skip editors in chat history
      if (c.closest && c.closest("[data-testid*='conversation-turn'], [data-testid*='user-message'], [class*='font-claude-message'], [class*='ChatMessage'], div.group")) {
        continue;
      }
      var t = (c.tagName === "TEXTAREA" || c.tagName === "INPUT") ? (c.value || "") : (c.innerText || c.textContent || "");
      if (t && t.trim()) return t.trim();
    }
    if (candidates.length) {
      var last = candidates[candidates.length - 1];
      var lastVal = (last.tagName === "TEXTAREA" || last.tagName === "INPUT") ? (last.value || "") : (last.innerText || last.textContent || "");
      if (lastVal && lastVal.trim()) return lastVal.trim();
    }

    // Universal Target Domain fallback: search all visible textareas & contenteditables with text
    try {
      var allEditors = querySelectorAllDeep("textarea, [contenteditable='true'], [role='textbox']");
      for (var k = allEditors.length - 1; k >= 0; k--) {
        var ed = allEditors[k];
        if (!ed) continue;
        if (ed.closest && ed.closest("[data-testid*='conversation-turn'], [data-testid*='user-message'], [class*='font-claude-message'], [class*='ChatMessage'], [class*='message-content'], div.group")) {
          continue;
        }
        var ev = (ed.tagName === "TEXTAREA" || ed.tagName === "INPUT") ? (ed.value || "") : (ed.innerText || ed.textContent || "");
        if (ev && ev.trim()) return ev.trim();
      }
    } catch (e) {}

    return "";
  }

  // Extract file attachments currently displayed in the chat composer UI
  function extractDOMAttachedFiles() {
    var detected = [];
    try {
      // STRICT SCOPE: Only look inside the active composer/upload container (NOT in previous message history)
      var activeContainer = document.querySelector(
        "fieldset, form, [class*='composer' i], [class*='input-container' i], div[data-testid*='composer' i], div[class*='chat-input' i]"
      ) || document.body;

      var selectors = [
        "[data-testid*='attachment']",
        "[data-testid*='file']",
        "div.file-item",
        "div[data-testid='file-preview']",
        "[aria-label*='attachment' i]",
        "[aria-label*='file' i]",
        "div[class*='attachment' i]",
        "div[class*='file-tile' i]",
        "div[class*='file-preview' i]",
        "span[class*='file-name' i]",
        "[class*='upload-preview' i]",
        "[class*='uploaded-file' i]"
      ];
      var nodes = querySelectorAllDeep(selectors.join(", "));
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i];
        if (!n) continue;
        // Never scrape files from conversation message history
        if (n.closest && n.closest("[data-testid*='conversation-turn'], [data-testid*='user-message'], [class*='font-claude-message'], [class*='ChatMessage'], [class*='message-content'], div.group")) {
          continue;
        }
        var txt = (n.getAttribute("aria-label") || n.getAttribute("title") || n.innerText || n.textContent || "").trim();
        if (!txt) continue;
        var m = txt.match(/([a-zA-Z0-9_\-\. ]+\.[a-zA-Z0-9]{1,8})/);
        if (m && m[1]) {
          var fn = m[1].trim();
          if (fn && !detected.some(function (x) { return x.name === fn; })) {
            detected.push({ name: fn, size: 0, type: "application/octet-stream" });
          }
        }
      }
    } catch (e) { }
    return detected;
  }

  // Set Composer text (for auto-redaction) across ChatGPT, Claude, Gemini, Copilot, Perplexity, etc.
  function setComposerText(text) {
    if (typeof text !== "string") return false;
    var composer = getDeepActiveElement() || document.activeElement;
    var candidates = [
      composer,
      document.querySelector("div#prompt-textarea"),
      document.querySelector("div.ProseMirror"),
      document.querySelector("rich-textarea div[contenteditable]"),
      document.querySelector("div[contenteditable='true']"),
      document.querySelector("textarea#prompt-textarea"),
      document.querySelector("textarea[data-id]"),
      document.querySelector("textarea")
    ];

    var target = null;
    for (var i = 0; i < candidates.length; i++) {
      var c = candidates[i];
      if (c && (c.tagName === "TEXTAREA" || c.tagName === "INPUT" || c.isContentEditable || c.getAttribute("contenteditable") === "true")) {
        target = c;
        break;
      }
    }
    if (!target) return false;

    try { target.focus(); } catch (e) {}

    // 1. Textarea and Input elements (support React synthetic event system via native setter)
    if (target.tagName === "TEXTAREA" || target.tagName === "INPUT") {
      try {
        var proto = target.tagName === "TEXTAREA" ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
        var nativeSetter = Object.getOwnPropertyDescriptor(proto, "value");
        if (nativeSetter && nativeSetter.set) {
          nativeSetter.set.call(target, text);
        } else {
          target.value = text;
        }
      } catch (ex) {
        target.value = text;
      }
      target.dispatchEvent(new Event("input", { bubbles: true }));
      target.dispatchEvent(new Event("change", { bubbles: true }));
      return true;
    }

    // 2. Rich contenteditable editors (ProseMirror, Lexical, DraftJS, Gemini rich-textarea)
    if (target.isContentEditable || target.getAttribute("contenteditable") === "true") {
      try {
        var sel = window.getSelection();
        if (sel) {
          var range = document.createRange();
          range.selectNodeContents(target);
          sel.removeAllRanges();
          sel.addRange(range);
        }
      } catch (eSel) {}

      var inserted = false;
      try {
        inserted = document.execCommand("insertText", false, text);
      } catch (eCmd) {
        inserted = false;
      }

      if (!inserted) {
        try {
          target.innerText = text;
        } catch (eTxt) {
          target.textContent = text;
        }
      }

      try {
        target.dispatchEvent(new InputEvent("input", {
          bubbles: true,
          cancelable: true,
          inputType: "insertText",
          data: text
        }));
      } catch (eInp) {
        target.dispatchEvent(new Event("input", { bubbles: true }));
      }
      target.dispatchEvent(new Event("change", { bubbles: true }));
      return true;
    }

    return false;
  }

  // Sleek, compact In-Page YP-Raksha Alert Toast
  function showBubble(message, ruleName, kind) {
    var tone = kind === "warn" ? "warn" : (kind === "redact" ? "redact" : "block");
    var isWarn = (tone === "warn");
    var isRedact = (tone === "redact");
    var borderColor = isWarn ? "#f59e0b" : (isRedact ? "#38bdf8" : "#ef4444");
    var titleColor = isWarn ? "#fbbf24" : (isRedact ? "#38bdf8" : "#f87171");
    var titleText = isWarn ? "YP-Raksha Warning" : (isRedact ? "YP-Raksha Redacted" : "YP-Raksha Blocked");
    var iconText = isWarn ? "⚠️" : (isRedact ? "✂️" : "🛡️");
    var text = (message || (isWarn ? "Security Warning: Policy match detected." : (isRedact ? "Sensitive information was redacted." : "This request was blocked by YP-Raksha."))).trim();
    var modalId = "yp-raksha-alert-card";
    var old = document.getElementById(modalId);
    if (old) old.remove();

    var card = document.createElement("div");
    card.id = modalId;
    card.setAttribute("role", "alertdialog");
    card.style.cssText =
      "all:initial;display:block;box-sizing:border-box;position:fixed;left:50%;bottom:95px;transform:translateX(-50%);max-width:460px;width:fit-content;min-width:280px;" +
      "z-index:2147483647;background:rgba(15,23,42,0.96);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);" +
      "color:#f9fafb;border:1px solid " + borderColor + ";border-radius:12px;padding:9px 14px;box-shadow:0 10px 30px rgba(0,0,0,0.5);" +
      "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;animation:ypFadeIn 0.2s ease-out;";

    // Style animation
    if (!document.getElementById("yp-raksha-keyframes")) {
      var kf = document.createElement("style");
      kf.id = "yp-raksha-keyframes";
      kf.textContent = "@keyframes ypFadeIn{from{opacity:0;transform:translate(-50%,10px)}to{opacity:1;transform:translate(-50%,0)}}";
      (document.head || document.documentElement).appendChild(kf);
    }

    var header = document.createElement("div");
    header.style.cssText = "display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:3px;";

    var titleBox = document.createElement("div");
    titleBox.style.cssText = "display:flex;align-items:center;gap:6px;";

    var icon = document.createElement("span");
    icon.textContent = iconText;
    icon.style.cssText = "font-size:13px;line-height:1;";

    var title = document.createElement("strong");
    title.textContent = titleText;
    title.style.cssText = "font-size:11px;font-weight:700;letter-spacing:0.03em;text-transform:uppercase;color:" + titleColor + ";";

    titleBox.appendChild(icon);
    titleBox.appendChild(title);

    var closeBtn = document.createElement("button");
    closeBtn.textContent = "✕";
    closeBtn.style.cssText = "background:none;border:none;color:#9ca3af;font-size:12px;cursor:pointer;padding:0 2px;line-height:1;";
    closeBtn.onclick = function () { card.remove(); };

    header.appendChild(titleBox);
    header.appendChild(closeBtn);

    var body = document.createElement("div");
    body.style.cssText = "font-size:12.5px;line-height:1.4;color:#e2e8f0;font-weight:400;";
    body.textContent = text;

    card.appendChild(header);
    card.appendChild(body);

    (document.body || document.documentElement).appendChild(card);

    // Auto-dismiss after 1 second (User requirement: 1 sec la vantu oirum)
    setTimeout(function () {
      if (card && card.parentNode) {
        card.style.transition = "opacity 0.2s ease";
        card.style.opacity = "0";
        setTimeout(function () { if (card.parentNode) card.remove(); }, 200);
      }
    }, 1000);
  }

  var lastWarnedTime = 0;
  var lastWarnedRule = "";

  function handleDecision(decision, proceed) {
    var action = String((decision && decision.action) || "").toLowerCase();
    var blocked = !!(decision && (decision.blocked || action === "blocked" || action === "block" || action === "bot answered" || action === "replied"));
    if (blocked) {
      showBubble(decision.message, decision.rule, "block");
      return;
    }
    if (action === "warned" || action === "warn" || action === "alert") {
      var now = Date.now();
      if (now - lastWarnedTime > 4000 || lastWarnedRule !== decision.rule) {
        lastWarnedTime = now;
        lastWarnedRule = decision.rule || "";
        showBubble(decision.message, decision.rule, "warn");
      }
      proceed(decision);
      return;
    }
    if (action === "redacted" || action === "redact") {
      if (decision.redacted_prompt) setComposerText(decision.redacted_prompt);
      showBubble(decision.message || "Sensitive text was redacted before sending.", decision.rule, "redact");
      proceed(decision);
      return;
    }
    proceed(decision || { blocked: false });
  }

  function rewriteBody(body, nextText, origText) {
    if (typeof body !== "string" || !nextText) return null;
    try {
      var parsed = JSON.parse(body);
      var changed = false;

      // Recursive walker to redact string properties matching origText or known prompt fields
      function walk(obj) {
        if (!obj || typeof obj !== "object") return;
        for (var k in obj) {
          if (!Object.prototype.hasOwnProperty.call(obj, k)) continue;
          var v = obj[k];
          if (typeof v === "string") {
            if (origText && v.indexOf(origText) >= 0) {
              obj[k] = v.split(origText).join(nextText);
              changed = true;
            } else if (k === "prompt" || k === "query" || k === "query_str" || k === "text" || k === "input" || k === "inputs" || k === "message") {
              obj[k] = nextText;
              changed = true;
            }
          } else if (typeof v === "object" && v !== null) {
            walk(v);
          }
        }
      }

      // 1. ChatGPT / OpenAI style: messages array
      if (parsed && Array.isArray(parsed.messages)) {
        for (var m = parsed.messages.length - 1; m >= 0; m--) {
          var msg = parsed.messages[m];
          if (!msg) continue;
          var role = msg.role || (msg.author && msg.author.role) || "";
          if (role === "user" || !role) {
            if (typeof msg.content === "string") {
              msg.content = nextText;
              changed = true;
            } else if (msg.content && Array.isArray(msg.content.parts)) {
              msg.content.parts = [nextText];
              changed = true;
            } else if (Array.isArray(msg.content)) {
              for (var c = 0; c < msg.content.length; c++) {
                if (msg.content[c] && typeof msg.content[c].text === "string") {
                  msg.content[c].text = nextText;
                  changed = true;
                }
              }
            }
            break;
          }
        }
      }

      // 2. Gemini / Vertex style: contents array
      if (parsed && Array.isArray(parsed.contents)) {
        for (var cIdx = parsed.contents.length - 1; cIdx >= 0; cIdx--) {
          var item = parsed.contents[cIdx];
          if (item && Array.isArray(item.parts)) {
            for (var p = 0; p < item.parts.length; p++) {
              if (item.parts[p] && typeof item.parts[p].text === "string") {
                item.parts[p].text = nextText;
                changed = true;
              }
            }
            break;
          }
        }
      }

      // 3. Copilot / SignalR style: arguments array
      if (parsed && Array.isArray(parsed.arguments) && parsed.arguments[0]) {
        var arg0 = parsed.arguments[0];
        if (arg0.message && typeof arg0.message.text === "string") { arg0.message.text = nextText; changed = true; }
        else if (arg0.message && typeof arg0.message.content === "string") { arg0.message.content = nextText; changed = true; }
        else if (typeof arg0.text === "string") { arg0.text = nextText; changed = true; }
        else if (typeof arg0.input === "string") { arg0.input = nextText; changed = true; }
      }

      // 4. Standard root prompt/query fields
      if (parsed && typeof parsed.prompt === "string") { parsed.prompt = nextText; changed = true; }
      if (parsed && typeof parsed.query === "string") { parsed.query = nextText; changed = true; }
      if (parsed && typeof parsed.query_str === "string") { parsed.query_str = nextText; changed = true; }
      if (parsed && typeof parsed.text === "string") { parsed.text = nextText; changed = true; }
      if (parsed && typeof parsed.input === "string") { parsed.input = nextText; changed = true; }

      if (!changed) {
        walk(parsed);
      }

      return changed ? JSON.stringify(parsed) : null;
    } catch (err) {
      if (origText && body.indexOf(origText) >= 0) {
        return body.split(origText).join(nextText);
      }
      return null;
    }
  }

  // --- 2. ALL FILE TYPES & IMAGE INTERCEPTION ENGINE ---

  function inspectAndStageFile(file, optName) {
    if (!file) return false;
    if (!checkFileUploadAllowed(file)) {
      return false;
    }
    var name = optName || file.name || "file";
    var size = file.size || 0;
    var type = file.type || "";

    // IMMEDIATELY stage file metadata (name, type, size) so EXE always receives it even if async reader is in-flight
    pushStagedFile({ name: name, size: size, type: type, text: "", base64: "", pending: true });

    // Read images as Base64 for Vision check
    if (type.indexOf("image/") === 0 || /\.(png|jpg|jpeg|webp|bmp|svg)$/i.test(name)) {
      var rImg = new FileReader();
      rImg.onload = function (e) {
        if (e.target && e.target.result) {
          var b64Img = (e.target.result.split(",")[1] || "");
          var sniffed = sniffMagicFileType(b64Img, "");
          if (sniffed && sniffed !== "png" && sniffed !== "pdf" && !checkFileUploadAllowed(file, sniffed)) {
            return;
          }
          stagedImages.push(e.target.result);
        }
      };
      rImg.readAsDataURL(file);
    }
    // 3. Read text/code/script/markup/config files
    else if (
      type.indexOf("text/") === 0 ||
      /\.(txt|py|pyw|pyi|pyx|js|mjs|cjs|jsx|ts|tsx|mts|cts|json|jsonc|json5|csv|tsv|log|md|markdown|yaml|yml|sql|sh|bash|zsh|fish|bat|cmd|ps1|psm1|psd1|java|kt|kts|scala|sc|groovy|c|h|cpp|cc|cxx|hpp|hh|hxx|cs|csx|fs|go|rs|rlib|php|phtml|phar|rb|rake|gemspec|swift|dart|lua|r|rmd|jl|pl|pm|t|html|htm|xml|svg|xhtml|css|scss|sass|less|vue|svelte|toml|ini|conf|config|cfg|properties|env|graphql|gql|proto|dockerfile|prisma|gradle|cmake|tf|tfvars|sol|asm|s|diff|patch|ipynb)$/i.test(name) ||
      /^(dockerfile|makefile|jenkinsfile|procfile|gemfile|rakefile|justfile|vagrantfile|license|licence|readme|\.env|\.gitignore|\.gitattributes|\.editorconfig|package\.json|cargo\.toml|go\.mod|requirements\.txt)$/i.test(name)
    ) {
      var rText = new FileReader();
      rText.onload = function (e) {
        if (e.target && typeof e.target.result === "string") {
          var fContent = e.target.result;
          var sniffed = sniffMagicFileType("", fContent);
          if (sniffed && !checkFileUploadAllowed(file, sniffed)) {
            return;
          }
          pushStagedFile({ name: name, size: size, type: type, text: fContent });

          // Immediate DLP scan of file contents
          var fileDecision = evaluateClientRules(null, [{ name: name, text: fContent }]);
          if (fileDecision && fileDecision.blocked) {
            showBubble("File '" + name + "' contains confidential data: " + fileDecision.message, fileDecision.rule);
          }
        }
      };
      rText.readAsText(file.slice(0, 1000000)); // Scan first 1MB of text
    }
    // 4. Binary Documents (PDF, Word, Excel, PowerPoint, ODF, RTF, ZIP, Archives)
    else if (
      /\.(pdf|docx|doc|xlsx|xls|pptx|ppt|odt|ods|odp|rtf|zip|tar|gz|7z)$/i.test(name) ||
      type.indexOf("application/pdf") >= 0 ||
      type.indexOf("application/zip") >= 0 ||
      type.indexOf("officedocument") >= 0
    ) {
      var rDoc = new FileReader();
      rDoc.onload = function (e) {
        if (e.target && typeof e.target.result === "string") {
          var b64 = e.target.result.split(",")[1] || "";
          var sniffed = sniffMagicFileType(b64, "");
          if (sniffed && !checkFileUploadAllowed(file, sniffed)) {
            return;
          }
          pushStagedFile({ name: name, size: size, type: type, base64: b64 });
        }
      };
      rDoc.readAsDataURL(file.slice(0, 5000000)); // Read first 5MB for deep backend extraction
    } else {
      // 5. General fallback: read first 3MB as Base64 so desktop proxy/agent extracts & scans it
      var rAny = new FileReader();
      rAny.onload = function (e) {
        if (e.target && typeof e.target.result === "string") {
          var b64 = e.target.result.split(",")[1] || "";
          var sniffed = sniffMagicFileType(b64, "");
          if (sniffed && !checkFileUploadAllowed(file, sniffed)) {
            return;
          }
          pushStagedFile({ name: name, size: size, type: type, base64: b64 });
        }
      };
      rAny.readAsDataURL(file.slice(0, 3000000));
    }
    return true;
  }

  // Hook 1: File Input Selector (<input type="file">) and Attach Button Click Blocking
  try {
    var origInputClick = HTMLInputElement.prototype.click;
    HTMLInputElement.prototype.click = function () {
      if (armed && this.type === "file" && govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        return;
      }
      return origInputClick.apply(this, arguments);
    };
  } catch (e) {}

  document.addEventListener("click", function (ev) {
    if (!armed || !govBlockUpload || !isControlActiveForSite(govUploadTargetWebsites)) return;
    var target = (ev.composedPath && ev.composedPath()[0]) || ev.target;
    if (!target) return;
    if (target.tagName === "INPUT" && target.type === "file") {
      ev.preventDefault();
      ev.stopImmediatePropagation();
      showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
      return;
    }
    var btn = target.closest("button, [role='button'], label");
    if (btn) {
      var aria = (btn.getAttribute("aria-label") || "").toLowerCase();
      var title = (btn.getAttribute("title") || "").toLowerCase();
      var testid = (btn.getAttribute("data-testid") || "").toLowerCase();
      var isAttachBtn =
        aria.includes("attach") || aria.includes("upload") || aria.includes("add file") || aria.includes("add photo") ||
        title.includes("attach") || title.includes("upload") ||
        testid.includes("attach") || testid.includes("upload") || testid.includes("file-upload");
      if (isAttachBtn) {
        ev.preventDefault();
        ev.stopImmediatePropagation();
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        return;
      }
    }
  }, true);

  document.addEventListener("change", function (ev) {
    if (!armed) return;
    if (ev.target && ev.target.tagName === "INPUT" && ev.target.type === "file") {
      if (govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
        ev.target.value = "";
        ev.preventDefault();
        ev.stopImmediatePropagation();
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        return;
      }
      var files = ev.target.files;
      if (files && files.length) {
        for (var i = 0; i < files.length; i++) {
          if (!inspectAndStageFile(files[i])) {
            ev.target.value = ""; // Clear file input on block
            ev.preventDefault();
            ev.stopImmediatePropagation();
            return;
          }
        }
      }
    }
  }, true);

  // Hook 2: Drag & Drop Files and Ingestion
  function handleDragOverOrEnter(ev) {
    if (!armed) return;
    var uploadActive = govBlockUpload && isControlActiveForSite(govUploadTargetWebsites);
    var dragDropActive = govBlockDragDrop && isControlActiveForSite(govDragDropTargetWebsites);
    if (uploadActive || dragDropActive) {
      if (ev.dataTransfer) {
        ev.dataTransfer.dropEffect = "none";
      }
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
    }
  }

  function handleDropBlock(ev) {
    if (!armed) return;
    var uploadActive = govBlockUpload && isControlActiveForSite(govUploadTargetWebsites);
    var dragDropActive = govBlockDragDrop && isControlActiveForSite(govDragDropTargetWebsites);
    if (dragDropActive) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      var droppedSummary = "";
      if (ev.dataTransfer) {
        if (ev.dataTransfer.files && ev.dataTransfer.files.length) {
          var fNames = [];
          for (var fi = 0; fi < ev.dataTransfer.files.length; fi++) fNames.push(ev.dataTransfer.files[fi].name);
          droppedSummary = "Files: " + fNames.join(", ");
        } else {
          try {
            var dtText = ev.dataTransfer.getData("text/plain") || "";
            if (dtText) droppedSummary = "Text: " + dtText.trim().slice(0, 300);
          } catch (_) {}
        }
        ev.dataTransfer.dropEffect = "none";
        try { ev.dataTransfer.clearData(); } catch (_) {}
      }
      showBubble(govDragDropBlockMessage || govBlockMessage || "Drag-and-drop file and data ingestion is blocked by corporate data governance policy.", "Data Governance Policy", "block");
      var promptLogText = "--drag drop [DRAG & DROP BLOCKED]";
      if (droppedSummary) {
        promptLogText += " " + droppedSummary;
      }
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: "drag_block_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
        prompt: promptLogText,
        host: location.hostname,
        url: location.href,
        is_blocked: true,
        rule: "Drag & Drop Policy",
        blocked_reason: "Drag & Drop Policy",
        status: "Blocked (Drag & Drop Policy)"
      }, "*");
      return;
    }
    if (uploadActive) {
      if (ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files.length) {
        ev.preventDefault();
        ev.stopPropagation();
        ev.stopImmediatePropagation();
        var upNames = [];
        for (var ufi = 0; ufi < ev.dataTransfer.files.length; ufi++) upNames.push(ev.dataTransfer.files[ufi].name);
        if (ev.dataTransfer) {
          ev.dataTransfer.dropEffect = "none";
          try { ev.dataTransfer.clearData(); } catch (_) {}
        }
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        window.postMessage({
          source: "gateway-guard-page",
          nonce: guardNonce,
          id: "upload_drop_block_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
          prompt: "--drag drop [DRAG & DROP BLOCKED] Upload: " + (upNames.join(", ") || "Files"),
          host: location.hostname,
          url: location.href,
          is_blocked: true,
          rule: "File Upload Policy",
          blocked_reason: "File Upload Policy",
          status: "Blocked (File Upload Policy)"
        }, "*");
        return;
      }
    }
    if (ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files.length) {
      var files = ev.dataTransfer.files;
      for (var i = 0; i < files.length; i++) {
        if (!inspectAndStageFile(files[i])) {
          ev.preventDefault();
          ev.stopPropagation();
          ev.stopImmediatePropagation();
          return;
        }
      }
    }
  }

  window.addEventListener("dragenter", handleDragOverOrEnter, true);
  document.addEventListener("dragenter", handleDragOverOrEnter, true);
  window.addEventListener("dragover", handleDragOverOrEnter, true);
  document.addEventListener("dragover", handleDragOverOrEnter, true);
  window.addEventListener("drop", handleDropBlock, true);
  document.addEventListener("drop", handleDropBlock, true);

  // Hook 3: Clipboard Paste (Ctrl+V Screenshots, Files & Sensitive Text Shielding)
  window.addEventListener("paste", function (ev) {
    if (!armed) return;
    if (govBlockClipboard && isControlActiveForSite(govClipboardTargetWebsites)) {
      ev.preventDefault();
      ev.stopImmediatePropagation();
      // Do not show clipboard bubble when screenshot shield is already visible —
      // the two banners must never appear at the same time.
      var screenshotShieldActive = !!(document.getElementById("yp-screenshot-shield") || document.getElementById("yp-blur-curtain"));
      var pastedSnippet = "";
      try {
        if (ev.clipboardData) pastedSnippet = ev.clipboardData.getData("text/plain") || "";
      } catch (e) {}
      if (!screenshotShieldActive) {
        showBubble(govClipboardBlockMessage || govBlockMessage || "Copy paste not allowed", "Clipboard Policy", "block");
      }
      var promptLogText = "--copy paste [CLIPBOARD PASTE]";
      if (pastedSnippet && pastedSnippet.trim()) {
        promptLogText += " " + pastedSnippet.trim().slice(0, 1000);
      }
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: "paste_block_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
        prompt: promptLogText,
        host: location.hostname,
        url: location.href,
        is_blocked: true,
        rule: "Clipboard Policy",
        blocked_reason: "Clipboard Policy",
        status: "Blocked (Clipboard Policy)"
      }, "*");
      return;
    }
    var cd = ev.clipboardData;
    if (!cd) return;

    // Master upload block on pasting files / images / screenshots
    if (govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
      if (cd.files && cd.files.length > 0) {
        ev.preventDefault();
        ev.stopImmediatePropagation();
        showBubble(govUploadWarning || "File uploads and image pasting are blocked by corporate policy.", "File Upload Policy", "block");
        return;
      }
      if (cd.items && cd.items.length) {
        for (var pki = 0; pki < cd.items.length; pki++) {
          if (cd.items[pki].kind === "file") {
            ev.preventDefault();
            ev.stopImmediatePropagation();
            showBubble(govUploadWarning || "File uploads and image pasting are blocked by corporate policy.", "File Upload Policy", "block");
            return;
          }
        }
      }
    }

    // 1. Files & Screenshots inspection
    if (cd.files && cd.files.length) {
      for (var i = 0; i < cd.files.length; i++) {
        if (!inspectAndStageFile(cd.files[i])) {
          ev.preventDefault();
          ev.stopImmediatePropagation();
          return;
        }
      }
    }
    // 2. Sensitive Text Paste Shield (Blocks secret credentials directly on Ctrl+V paste)
    try {
      var pastedText = cd.getData("text/plain");
      if (pastedText && pastedText.length > 5) {
        var pasteDecision = evaluateClientRules(pastedText, []);
        if (pasteDecision && pasteDecision.blocked) {
          ev.preventDefault();
          ev.stopImmediatePropagation();
          showBubble("Pasting confidential data is blocked by company security policy: " + pasteDecision.message, pasteDecision.rule, "block");
          window.postMessage({
            source: "gateway-guard-page",
            nonce: guardNonce,
            id: "paste_log_" + Date.now().toString(36),
            prompt: "--copy paste [CLIPBOARD PASTE] " + pastedText.slice(0, 500),
            host: location.hostname,
            url: location.href,
            is_blocked: true,
            rule: pasteDecision.rule || "Clipboard Policy",
            blocked_reason: pasteDecision.rule || "Clipboard Policy",
            status: "Blocked (Clipboard Policy)"
          }, "*");
          return;
        }
      }
    } catch (pasteErr) { }
  }, true);

  window.addEventListener("copy", function (ev) {
    if (!armed || !govBlockClipboard || !isControlActiveForSite(govClipboardTargetWebsites)) return;
    ev.preventDefault();
    ev.stopImmediatePropagation();
    // Do not show clipboard bubble when screenshot shield is already visible.
    var screenshotShieldActive = !!(document.getElementById("yp-screenshot-shield") || document.getElementById("yp-blur-curtain"));
    var copiedSnippet = "";
    try {
      var sel = window.getSelection ? window.getSelection().toString() : "";
      if (sel) copiedSnippet = sel;
    } catch (e) {}
    if (!screenshotShieldActive) {
      showBubble(govClipboardBlockMessage || govBlockMessage || "Copy paste not allowed", "Clipboard Policy", "block");
    }
    var promptLogText = "--copy paste [CLIPBOARD COPY]";
    if (copiedSnippet && copiedSnippet.trim()) {
      promptLogText += " " + copiedSnippet.trim().slice(0, 1000);
    }
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: "copy_block_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
      prompt: promptLogText,
      host: location.hostname,
      url: location.href,
      is_blocked: true,
      rule: "Clipboard Policy",
      blocked_reason: "Clipboard Policy",
      status: "Blocked (Clipboard Policy)"
    }, "*");
  }, true);

  window.addEventListener("cut", function (ev) {
    if (!armed || !govBlockClipboard || !isControlActiveForSite(govClipboardTargetWebsites)) return;
    ev.preventDefault();
    ev.stopImmediatePropagation();
    // Do not show clipboard bubble when screenshot shield is already visible.
    var screenshotShieldActive = !!(document.getElementById("yp-screenshot-shield") || document.getElementById("yp-blur-curtain"));
    var cutSnippet = "";
    try {
      var sel = window.getSelection ? window.getSelection().toString() : "";
      if (sel) cutSnippet = sel;
    } catch (e) {}
    if (!screenshotShieldActive) {
      showBubble(govClipboardBlockMessage || govBlockMessage || "Copy paste not allowed", "Clipboard Policy", "block");
    }
    var promptLogText = "--copy paste [CLIPBOARD CUT]";
    if (cutSnippet && cutSnippet.trim()) {
      promptLogText += " " + cutSnippet.trim().slice(0, 1000);
    }
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: "cut_block_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
      prompt: promptLogText,
      host: location.hostname,
      url: location.href,
      is_blocked: true,
      rule: "Clipboard Policy",
      blocked_reason: "Clipboard Policy",
      status: "Blocked (Clipboard Policy)"
    }, "*");
  }, true);

  // Cover direct page Clipboard API and execCommand calls as well as DOM events.
  // The isolated-world content script independently blocks trusted copy/paste events.
  function clipboardPolicyActive() {
    return armed && govBlockClipboard && isControlActiveForSite(govClipboardTargetWebsites);
  }
  try {
    var clipboardApi = navigator.clipboard;
    if (clipboardApi) {
      ["read", "readText", "write", "writeText"].forEach(function (method) {
        var original = clipboardApi[method];
        if (typeof original !== "function") return;
        try {
          clipboardApi[method] = function () {
            if (clipboardPolicyActive()) {
              showBubble(govClipboardBlockMessage || govBlockMessage || "Clipboard access is blocked by corporate policy.", "Clipboard Policy", "block");
              // Safely resolve with empty content so Chrome does not throw an Uncaught (in promise) NotAllowedError
              if (method === "readText") return Promise.resolve("");
              if (method === "read") return Promise.resolve([]);
              return Promise.resolve();
            }
            return original.apply(clipboardApi, arguments);
          };
        } catch (e) {}
      });
    }
  } catch (e) {}
  try {
    var originalExecCommand = document.execCommand;
    if (typeof originalExecCommand === "function") {
      document.execCommand = function (command) {
        var normalized = String(command || "").toLowerCase();
        if (clipboardPolicyActive() && (normalized === "copy" || normalized === "cut" || normalized === "paste")) {
          showBubble(govClipboardBlockMessage || govBlockMessage || "Clipboard access is blocked by corporate policy.", "Clipboard Policy", "block");
          return false;
        }
        return originalExecCommand.apply(document, arguments);
      };
    }
  } catch (e) {}

  // Hook 4: File Downloads & Exports Restriction
  document.addEventListener("click", function (ev) {
    if (!armed || !govBlockDownload || !isControlActiveForSite(govDownloadTargetWebsites)) return;
    var target = (ev.composedPath && ev.composedPath()[0]) || ev.target;
    var el = (target && target.closest) ? target.closest("a, button, [role='button']") : null;
    if (!el) return;
    var href = (el.getAttribute("href") || "").toLowerCase();
    var downloadAttr = el.getAttribute("download");
    var label = (el.getAttribute("aria-label") || "").toLowerCase();
    var title = (el.getAttribute("title") || "").toLowerCase();
    var text = (el.innerText || el.textContent || "").toLowerCase();

    var isDownload = (downloadAttr !== null) ||
      Boolean(href && href.match(/\.(pdf|csv|xlsx|docx|txt|zip|tar|gz|json|py|js|html)(\?.*)?$/i)) ||
      Boolean(label && (label.includes("download") || label.includes("export") || label.includes("save as"))) ||
      Boolean(title && (title.includes("download") || title.includes("export"))) ||
      Boolean(text && text.length < 35 && (text.includes("download") || text.includes("export") || text.includes("save code") || text.includes("export chat")));

    if (isDownload) {
      ev.preventDefault();
      ev.stopImmediatePropagation();
      showBubble(govDownloadBlockMessage || govBlockMessage || "Downloading files, artifacts, and exports from this AI tool is restricted by corporate security policy.", "Download Policy", "block");
    }
  }, true);

  try {
    var origAnchorClick = HTMLAnchorElement.prototype.click;
    HTMLAnchorElement.prototype.click = function () {
      if (armed && govBlockDownload && isControlActiveForSite(govDownloadTargetWebsites)) {
        var href = (this.getAttribute("href") || "").toLowerCase();
        var hasDl = this.hasAttribute("download");
        if (hasDl || href.startsWith("blob:") || href.startsWith("data:") || href.match(/\.(pdf|csv|xlsx|docx|zip|txt|json)/i)) {
          showBubble(govDownloadBlockMessage || govBlockMessage || "File download was blocked by corporate data governance policy.", "Download Policy", "block");
          return;
        }
      }
      return origAnchorClick.apply(this, arguments);
    };
  } catch (e) {}

  // --- 3. SEND INTERCEPTION (Enter Key & Send Button) ---

  function mergeStagedAndDomFiles() {
    var domFiles = extractDOMAttachedFiles();
    var all = getAllFreshStagedFiles();
    for (var i = 0; i < domFiles.length; i++) {
      var df = domFiles[i];
      var cached = getCachedStagedFile(df.name);
      if (cached) {
        if (!df.text && cached.text) df.text = cached.text;
        if (!df.base64 && cached.base64) df.base64 = cached.base64;
        if (!df.size && cached.size) df.size = cached.size;
        if (!df.type && cached.type) df.type = cached.type;
      }
      var exists = false;
      for (var a0 = 0; a0 < all.length; a0++) {
        if (all[a0].name === df.name || String(all[a0].name).toLowerCase().trim() === String(df.name).toLowerCase().trim()) {
          exists = true;
          break;
        }
      }
      if (!exists) {
        all.push(df);
      }
    }
    for (var a = 0; a < all.length; a++) {
      if (!all[a].text && !all[a].base64) {
        var match = getCachedStagedFile(all[a].name);
        if (match) {
          if (match.text) all[a].text = match.text;
          if (match.base64) all[a].base64 = match.base64;
          if (match.size) all[a].size = match.size;
          if (match.type) all[a].type = match.type;
        }
      }
    }
    return all;
  }

  // Keyboard Enter
  document.addEventListener("keydown", function (e) {
    if (!armed || passKey) return;
    if (e.key !== "Enter" || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey || e.isComposing) return;

    if (sessionBlocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(sessionBlockedReason || "Corporate policy restricts personal accounts. Please sign in with your corporate account.", "Corporate Policy Block", "block");
      return;
    }
    var actualTarget = (e.composedPath && e.composedPath()[0]) || e.target;
    var text = stripInvisible(composerFrom(actualTarget) || composerFrom(e.target) || composerFrom(getDeepActiveElement()));
    var effectiveFiles = mergeStagedAndDomFiles();
    if (!text && effectiveFiles.length === 0 && stagedImages.length === 0) return;
    if (text && _lastHandledPromptText === text && (Date.now() - _lastHandledPromptTime < 1500)) {
      return;
    }

    if (govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
      if (effectiveFiles.length > 0 || stagedImages.length > 0) {
        e.preventDefault();
        e.stopImmediatePropagation();
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        return;
      }
    }

    for (var fi = 0; fi < effectiveFiles.length; fi++) {
      if (!checkFileUploadAllowed(effectiveFiles[fi])) {
        e.preventDefault();
        e.stopImmediatePropagation();
        return;
      }
    }

    var clientDecision = evaluateClientRules(text, effectiveFiles);
    if (clientDecision && clientDecision.blocked) {
      _lastHandledPromptTime = Date.now();
      _lastHandledPromptText = text;
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(clientDecision.message, clientDecision.rule, "block");
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
        prompt: text,
        files: effectiveFiles || [],
        upload_images: stagedImages || [],
        host: location.hostname,
        url: location.href,
        is_blocked: true,
        action: "Blocked",
        rule: clientDecision.rule || ""
      }, "*");
      return;
    }

    var clientAction = String((clientDecision && clientDecision.action) || "").toLowerCase();
    if (clientAction === "redacted" || clientAction === "redact") {
      e.preventDefault();
      e.stopImmediatePropagation();
      var redactedText = clientDecision.redacted_prompt || text;
      setComposerText(redactedText);
      showBubble(clientDecision.message || "Sensitive text was redacted before sending.", clientDecision.rule, "redact");

      var redactLogId = "redact_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: redactLogId,
        prompt: redactedText,
        original_prompt: text,
        files: effectiveFiles || [],
        upload_images: stagedImages || [],
        host: location.hostname,
        url: location.href,
        is_blocked: false,
        action: "Redacted",
        rule: clientDecision.rule || ""
      }, "*");

      var enterTarget = e.target;
      setTimeout(function () {
        passKey = true;
        try {
          var sendBtns = querySelectorAllDeep(
            "button[aria-label*='Send' i], button[aria-label*='Submit' i], button[aria-label*='Ask' i], button.send-button, [data-testid*='send-button' i], [data-testid*='submit' i], cib-serp button, button[title*='Send' i], button[title*='Submit' i], button[type='submit']"
          );
          var sendBtn = sendBtns.length ? sendBtns[sendBtns.length - 1] : null;
          if (sendBtn) {
            passClick = true;
            try {
              sendBtn.click();
            } finally {
              passClick = false;
            }
          } else if (enterTarget) {
            enterTarget.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true, keyCode: 13, which: 13 }));
          }
        } finally {
          passKey = false;
        }
      }, 50);
      return;
    }

    if (clientAction === "warned" || clientAction === "warn") {
      var now = Date.now();
      if (now - lastWarnedTime > 4000 || lastWarnedRule !== clientDecision.rule) {
        lastWarnedTime = now;
        lastWarnedRule = clientDecision.rule || "";
        showBubble(clientDecision.message, clientDecision.rule, "warn");
      }
    }

    if (effectiveFiles.length > 0 || stagedImages.length > 0) {
      e.preventDefault();
      e.stopImmediatePropagation();
      var target = e.target;
      ask(text, effectiveFiles, stagedImages).then(function (decision) {
        handleDecision(decision, function () {
          setTimeout(function () { stagedFiles = []; stagedImages = []; }, 15000);
          passKey = true;
          var pId = "eval_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
          _lastHandledPromptId = pId;
          _lastHandledPromptTime = Date.now();
          _lastHandledPromptText = text;
          try {
            var sendBtns = querySelectorAllDeep(
              "button[aria-label*='Send' i], button[aria-label*='Submit' i], button[aria-label*='Ask' i], button.send-button, [data-testid*='send-button' i], [data-testid*='submit' i], cib-serp button, button[title*='Send' i], button[title*='Submit' i], button[type='submit']"
            );
            var sendBtn = sendBtns.length ? sendBtns[sendBtns.length - 1] : null;
            if (sendBtn) {
              sendBtn.click();
            } else {
              target.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true, keyCode: 13, which: 13 }));
            }
          } finally {
            passKey = false;
          }
        });
      });
      return;
    }

    // Fast path: Clean text prompt without files sends instantly with native Enter (0ms delay)
    var fastId = "eval_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
    _lastHandledPromptId = fastId;
    _lastHandledPromptTime = Date.now();
    _lastHandledPromptText = text;
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: fastId,
      eval_id: fastId,
      prompt: text,
      files: [],
      upload_images: [],
      host: location.hostname,
      url: location.href,
      is_blocked: false,
      action: (clientAction === "warned" || clientAction === "warn") ? "Warned" : "Allowed",
      rule: (clientAction === "warned" || clientAction === "warn") ? (clientDecision.rule || "") : ""
    }, "*");
  }, true);

  // Send Button Click
  document.addEventListener("click", function (e) {
    if (!armed || passClick) return;
    var actualEl = (e.composedPath && e.composedPath()[0]) || e.target;
    var btn = (actualEl && actualEl.closest ? actualEl.closest("button,[role='button']") : null) || (e.target && e.target.closest ? e.target.closest("button,[role='button']") : null);
    if (!btn) return;

    var label = ((btn.getAttribute("aria-label") || "") + " " + (btn.getAttribute("data-testid") || "") + " " + (btn.title || "")).toLowerCase();
    var btnText = (btn.innerText || btn.textContent || "").toLowerCase().trim();
    var form = btn.closest ? btn.closest("form") : null;
    var sendLike =
      label.indexOf("send") >= 0 ||
      label.indexOf("submit") >= 0 ||
      label.indexOf("ask") >= 0 ||
      label.indexOf("generate") >= 0 ||
      label.indexOf("create") >= 0 ||
      label.indexOf("run") >= 0 ||
      label.indexOf("draw") >= 0 ||
      label.indexOf("message") >= 0 ||
      label.indexOf("chat") >= 0 ||
      label.indexOf("post") >= 0 ||
      label.indexOf("search") >= 0 ||
      btnText === "generate" ||
      btnText === "create" ||
      btnText === "send" ||
      btnText === "submit" ||
      btnText === "ask" ||
      btnText === "run" ||
      btnText === "post" ||
      btnText === "search" ||
      btnText === "go" ||
      btn.getAttribute("type") === "submit" ||
      btn.type === "submit" ||
      btn.getAttribute("data-testid") === "send-button" ||
      (form && form.contains(getDeepActiveElement())) ||
      Boolean(btn.closest("fieldset, form, [class*='composer' i], [class*='input-container' i], div[data-testid*='composer' i], div[class*='chat-input' i]")) ||
      (Boolean(btn.querySelector("svg")) && Boolean(btn.closest("fieldset, form, footer, [class*='composer' i], [class*='chat' i]")));

    if (!sendLike) return;

    if (sessionBlocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(sessionBlockedReason || "Corporate policy restricts personal accounts. Please sign in with your corporate account.", "Corporate Policy Block", "block");
      return;
    }
    var text = stripInvisible(composerFrom(btn) || composerFrom(getDeepActiveElement()) || composerFrom(document.activeElement));
    var effectiveFiles = mergeStagedAndDomFiles();
    if (!text && effectiveFiles.length === 0 && stagedImages.length === 0) return;
    if (text && _lastHandledPromptText === text && (Date.now() - _lastHandledPromptTime < 1500)) {
      return;
    }

    if (govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
      if (effectiveFiles.length > 0 || stagedImages.length > 0) {
        e.preventDefault();
        e.stopImmediatePropagation();
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        return;
      }
    }

    for (var fi2 = 0; fi2 < effectiveFiles.length; fi2++) {
      if (!checkFileUploadAllowed(effectiveFiles[fi2])) {
        e.preventDefault();
        e.stopImmediatePropagation();
        return;
      }
    }

    var clientDecision = evaluateClientRules(text, effectiveFiles);
    if (clientDecision && clientDecision.blocked) {
      _lastHandledPromptTime = Date.now();
      _lastHandledPromptText = text;
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(clientDecision.message, clientDecision.rule, "block");
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
        prompt: text,
        files: effectiveFiles || [],
        upload_images: stagedImages || [],
        host: location.hostname,
        url: location.href,
        is_blocked: true,
        action: "Blocked",
        rule: clientDecision.rule || ""
      }, "*");
      return;
    }

    var clientAction = String((clientDecision && clientDecision.action) || "").toLowerCase();
    if (clientAction === "redacted" || clientAction === "redact") {
      e.preventDefault();
      e.stopImmediatePropagation();
      var redactedText = clientDecision.redacted_prompt || text;
      setComposerText(redactedText);
      showBubble(clientDecision.message || "Sensitive text was redacted before sending.", clientDecision.rule, "redact");

      var redactClickId = "redact_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: redactClickId,
        prompt: redactedText,
        original_prompt: text,
        files: effectiveFiles || [],
        upload_images: stagedImages || [],
        host: location.hostname,
        url: location.href,
        is_blocked: false,
        action: "Redacted",
        rule: clientDecision.rule || ""
      }, "*");

      setTimeout(function () {
        passClick = true;
        try {
          btn.click();
        } finally {
          passClick = false;
        }
      }, 50);
      return;
    }

    if (clientAction === "warned" || clientAction === "warn") {
      var now2 = Date.now();
      if (now2 - lastWarnedTime > 4000 || lastWarnedRule !== clientDecision.rule) {
        lastWarnedTime = now2;
        lastWarnedRule = clientDecision.rule || "";
        showBubble(clientDecision.message, clientDecision.rule, "warn");
      }
    }

    if (effectiveFiles.length > 0 || stagedImages.length > 0) {
      e.preventDefault();
      e.stopImmediatePropagation();
      ask(text, effectiveFiles, stagedImages).then(function (decision) {
        handleDecision(decision, function () {
          setTimeout(function () { stagedFiles = []; stagedImages = []; }, 15000);
          passClick = true;
          var clickId = "eval_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
          _lastHandledPromptId = clickId;
          _lastHandledPromptTime = Date.now();
          _lastHandledPromptText = text;
          try {
            btn.click();
          } finally {
            passClick = false;
          }
        });
      });
      return;
    }

    // Fast path: Clean text prompt without files proceeds with native click instantly (0ms delay)
    var fastClickId = "eval_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
    _lastHandledPromptId = fastClickId;
    _lastHandledPromptTime = Date.now();
    _lastHandledPromptText = text;
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: fastClickId,
      eval_id: fastClickId,
      prompt: text,
      files: [],
      upload_images: [],
      host: location.hostname,
      url: location.href,
      is_blocked: false,
      action: (clientAction === "warned" || clientAction === "warn") ? "Warned" : "Allowed",
      rule: (clientAction === "warned" || clientAction === "warn") ? (clientDecision.rule || "") : ""
    }, "*");
  }, true);

  // Form Submit Interception (Catches Enter in simple forms or form.requestSubmit)
  document.addEventListener("submit", function (e) {
    if (!armed || passClick) return;
    var form = e.target;
    if (!form) return;

    if (sessionBlocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(sessionBlockedReason || "Corporate policy restricts personal accounts. Please sign in with your corporate account.", "Corporate Policy Block", "block");
      return;
    }
    var text = stripInvisible(composerFrom(form) || composerFrom(document.activeElement));
    var effectiveFiles = mergeStagedAndDomFiles();
    if (!text && effectiveFiles.length === 0 && stagedImages.length === 0) return;

    var clientDecision = evaluateClientRules(text, effectiveFiles);
    if (clientDecision && clientDecision.blocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(clientDecision.message, clientDecision.rule, "block");
      return;
    }

    if (effectiveFiles.length > 0 || stagedImages.length > 0) {
      e.preventDefault();
      e.stopImmediatePropagation();
      ask(text, effectiveFiles, stagedImages).then(function (decision) {
        handleDecision(decision, function () {
          setTimeout(function () { stagedFiles = []; stagedImages = []; }, 15000);
          passClick = true;
          try {
            if (form && typeof form.requestSubmit === "function") {
              form.requestSubmit();
            } else if (form && typeof form.submit === "function") {
              form.submit();
            }
          } catch (subErr) {
            console.warn("[Gateway Guard] form submit replay error:", subErr);
          } finally {
            passClick = false;
          }
        });
      });
      return;
    }

    _lastHandledPromptTime = Date.now();
    _lastHandledPromptText = text;
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
      prompt: text,
      files: [],
      upload_images: [],
      host: location.hostname,
      url: location.href,
      is_blocked: false,
      rule: ""
    }, "*");
  }, true);

  // Helper: Return clean platform-native SSE or structured JSON block response
  function createBlockedFetchResponse(decision, reqUrl) {
    var blockMsg = (decision && decision.message) || "Blocked by security policy.";
    var u = String(reqUrl || (typeof location !== "undefined" ? location.href : "") || "").toLowerCase();
    var isChatGpt = u.includes("chatgpt.com") || u.includes("openai.com") || u.includes("/conversation") || u.includes("backend-api");
    var isClaude = u.includes("claude.ai") || u.includes("anthropic.com");

    if (isChatGpt) {
      var msgId = "block_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
      var sseBody =
        "data: " + JSON.stringify({
          message: {
            id: msgId,
            author: { role: "assistant", name: null, metadata: {} },
            create_time: Math.floor(Date.now() / 1000),
            update_time: null,
            content: { content_type: "text", parts: [blockMsg] },
            status: "finished_successfully",
            end_turn: true,
            weight: 1.0,
            metadata: { finish_details: { type: "stop" }, model_slug: "gpt-4" },
            recipient: "all"
          },
          conversation_id: null,
          error: null
        }) + "\n\n" +
        "data: [DONE]\n\n";

      return new Response(sseBody, {
        status: 200,
        headers: {
          "Content-Type": "text/event-stream; charset=utf-8",
          "Cache-Control": "no-cache",
          "X-Accel-Buffering": "no"
        }
      });
    }

    if (isClaude) {
      var cSse =
        'event: message_start\ndata: {"type":"message_start","message":{"id":"msg_blk_' + Date.now().toString(36) + '","type":"message","role":"assistant","content":[],"model":"claude","stop_reason":null}}\n\n' +
        'event: content_block_start\ndata: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}\n\n' +
        'event: content_block_delta\ndata: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":' + JSON.stringify(blockMsg) + '}}\n\n' +
        'event: content_block_stop\ndata: {"type":"content_block_stop","index":0}\n\n' +
        'event: message_delta\ndata: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}\n\n' +
        'event: message_stop\ndata: {"type":"message_stop"}\n\n';

      return new Response(cSse, {
        status: 200,
        headers: {
          "Content-Type": "text/event-stream; charset=utf-8",
          "Cache-Control": "no-cache"
        }
      });
    }

    var genSse =
      "data: " + JSON.stringify({
        text: blockMsg,
        message: blockMsg,
        content: blockMsg,
        role: "assistant",
        choices: [{ delta: { content: blockMsg }, finish_reason: "stop" }]
      }) + "\n\n" +
      "data: [DONE]\n\n";

    return new Response(genSse, {
      status: 200,
      headers: {
        "Content-Type": "text/event-stream; charset=utf-8",
        "Cache-Control": "no-cache"
      }
    });
  }

  // --- 4. NETWORK LAYER HOOK (fetch, XMLHttpRequest & WebSocket) ---

  var origFetch = window.fetch;
  if (typeof origFetch === "function") {
    window.fetch = function (input, init) {
      if (!armed) return origFetch.apply(this, arguments);

      var method = (init && init.method) || (input && input.method) || "GET";
      method = String(method).toUpperCase();
      var url = typeof input === "string" ? input : (input && input.url) || "";

      if (method !== "POST" && method !== "PUT" && method !== "PATCH") {
        var pGet = origFetch.apply(this, arguments);
        if (typeof url === "string" && (url.indexOf("/session") >= 0 || url.indexOf("/me") >= 0 || url.indexOf("/user") >= 0 || url.indexOf("/auth") >= 0 || url.indexOf("/account") >= 0 || url.indexOf("/profile") >= 0)) {
          pGet.then(function (r) {
            try {
              if (r && r.ok && typeof r.clone === "function") {
                r.clone().json().then(function (data) {
                  if (data && typeof data === "object") {
                    scanObjectForSession(data, 0);
                  }
                }).catch(function () {});
              }
            } catch (_) {}
          }).catch(function () {});
        }
        return pGet;
      }
      var body = init && init.body;
      var promptText = "";
      var netFiles = [];

      // Handle JSON body
      if (typeof body === "string") {
        try {
          var parsed = JSON.parse(body);
          if (parsed) {
            if (typeof parsed.file_name === "string" && parsed.file_name) {
              lastCreatedFileName = parsed.file_name;
            }
            // 1. Direct prompt fields across OpenAI, Claude, Gemini, Copilot, DeepSeek, Mistral
            if (typeof parsed.prompt === "string") promptText = parsed.prompt;
            else if (typeof parsed.query === "string" && !parsed.query.toLowerCase().startsWith("plsenvironmentquery") && !parsed.query.toLowerCase().startsWith("query pls")) promptText = parsed.query;
            else if (typeof parsed.text === "string") promptText = parsed.text;
            else if (typeof parsed.input === "string") promptText = parsed.input;
            else if (typeof parsed.userMessage === "string") promptText = parsed.userMessage;
            else if (parsed.message && typeof parsed.message.text === "string") promptText = parsed.message.text;
            else if (Array.isArray(parsed.arguments) && parsed.arguments[0]) {
              var a0 = parsed.arguments[0];
              if (a0.message && typeof a0.message.text === "string") promptText = a0.message.text;
              else if (typeof a0.text === "string") promptText = a0.text;
            }

            // 2. Claude Connect-RPC / PerformAction format
            var act = parsed.action || parsed;
            var rpcMsg = (act && (act.createMessage || act.appendMessage || act.message)) || null;
            if (rpcMsg) {
              if (typeof rpcMsg.text === "string" && rpcMsg.text) promptText = rpcMsg.text;
              if (Array.isArray(rpcMsg.attachments)) {
                for (var r = 0; r < rpcMsg.attachments.length; r++) {
                  var ra = rpcMsg.attachments[r];
                  if (ra) {
                    var rName = ra.fileName || ra.file_name || ra.name || "";
                    if (rName && !netFiles.some(function (x) { return x.name === rName; })) {
                      netFiles.push({ name: rName, size: ra.fileSize || ra.file_size || 0, type: ra.fileType || ra.file_type || "" });
                    }
                  }
                }
              }
            }

            // 3. Top-level attachments / files (Claude, etc.)
            if (Array.isArray(parsed.attachments)) {
              for (var a = 0; a < parsed.attachments.length; a++) {
                var att = parsed.attachments[a];
                if (att) {
                  var aName = att.file_name || att.fileName || att.name || att.title || "";
                  var aType = att.file_type || att.fileType || att.mime_type || "";
                  var aSize = att.file_size || att.fileSize || att.size || 0;
                  var aText = att.extracted_content || att.content || "";
                  if (aName && !netFiles.some(function (x) { return x.name === aName; })) {
                    netFiles.push({ name: aName, size: aSize, type: aType, text: aText });
                  }
                }
              }
            }
            if (Array.isArray(parsed.files)) {
              for (var f = 0; f < parsed.files.length; f++) {
                var fl = parsed.files[f];
                if (fl) {
                  var fName = fl.name || fl.file_name || fl.fileName || "";
                  var fType = fl.type || fl.mime_type || "";
                  var fSize = fl.size || 0;
                  if (fName && !netFiles.some(function (x) { return x.name === fName; })) {
                    netFiles.push({ name: fName, size: fSize, type: fType });
                  }
                }
              }
            }

            // 4. Conversation messages array (ChatGPT, Claude, Gemini, etc.)
            if (Array.isArray(parsed.messages)) {
              for (var m = parsed.messages.length - 1; m >= 0; m--) {
                var msg = parsed.messages[m];
                if (!msg) continue;
                var role = msg.role || (msg.author && msg.author.role) || "";
                if (role === "user") {
                  if (typeof msg.content === "string") {
                    promptText = msg.content;
                  }
                  // ChatGPT / Gemini parts array
                  else if (msg.content && Array.isArray(msg.content.parts)) {
                    var strParts = [];
                    for (var p = 0; p < msg.content.parts.length; p++) {
                      var pt = msg.content.parts[p];
                      if (typeof pt === "string") {
                        strParts.push(pt);
                      } else if (pt && typeof pt === "object") {
                        var ptName = pt.name || (pt.asset_pointer ? ("file_" + (pt.asset_pointer.split("/").pop() || "upload")) : "");
                        if (ptName && !netFiles.some(function (x) { return x.name === ptName; })) {
                          netFiles.push({ name: ptName, size: pt.size_bytes || 0, type: pt.content_type || "" });
                        }
                      }
                    }
                    if (strParts.length) promptText = strParts.join(" ");
                  }
                  // Claude content blocks array
                  else if (Array.isArray(msg.content)) {
                    var cParts = [];
                    for (var c = 0; c < msg.content.length; c++) {
                      var blk = msg.content[c];
                      if (!blk) continue;
                      if (blk.type === "text" && typeof blk.text === "string") {
                        cParts.push(blk.text);
                      } else if (blk.type === "document" || blk.type === "file") {
                        var docName = blk.title || blk.name || "document";
                        var docText = (blk.source && blk.source.data) ? blk.source.data : "";
                        var docType = (blk.source && blk.source.media_type) || "application/pdf";
                        if (!netFiles.some(function (x) { return x.name === docName; })) {
                          netFiles.push({ name: docName, size: docText.length, type: docType, text: blk.text || "" });
                        }
                      } else if (blk.type === "image") {
                        var imgName = blk.title || blk.name || "image.png";
                        if (!netFiles.some(function (x) { return x.name === imgName; })) {
                          netFiles.push({ name: imgName, size: 0, type: (blk.source && blk.source.media_type) || "image/png" });
                        }
                      }
                    }
                    if (cParts.length) promptText = cParts.join(" ");
                  }

                  // ChatGPT metadata.attachments
                  if (msg.metadata && Array.isArray(msg.metadata.attachments)) {
                    for (var ma = 0; ma < msg.metadata.attachments.length; ma++) {
                      var matt = msg.metadata.attachments[ma];
                      if (matt) {
                        var mattName = matt.name || matt.file_name || matt.title || "";
                        var mattSize = matt.size || 0;
                        var mattType = matt.mime_type || matt.mimeType || matt.type || "";
                        if (mattName && !netFiles.some(function (x) { return x.name === mattName; })) {
                          netFiles.push({ name: mattName, size: mattSize, type: mattType });
                        }
                      }
                    }
                  }

                  // Claude / generic msg.attachments
                  if (Array.isArray(msg.attachments)) {
                    for (var ma2 = 0; ma2 < msg.attachments.length; ma2++) {
                      var matt2 = msg.attachments[ma2];
                      if (matt2) {
                        var m2Name = matt2.file_name || matt2.name || "";
                        if (m2Name && !netFiles.some(function (x) { return x.name === m2Name; })) {
                          netFiles.push({ name: m2Name, size: matt2.file_size || matt2.size || 0, type: matt2.file_type || "" });
                        }
                      }
                    }
                  }
                  break;
                }
              }
            }
          }
        } catch (err) { }
      }
      // Handle FormData body (File uploads)
      else if (body && typeof body.forEach === "function") {
        try {
          var formUploadBlocked = false;
          body.forEach(function (val, key) {
            if (val && typeof val.name === "string") {
              if (!checkFileUploadAllowed(val)) {
                formUploadBlocked = true;
              }
              inspectAndStageFile(val);
              netFiles.push({ name: val.name, size: val.size, type: val.type });
            } else if (typeof val === "string" && (key.indexOf("prompt") >= 0 || key.indexOf("query") >= 0 || key.indexOf("message") >= 0)) {
              promptText = val;
            }
          });
          if (formUploadBlocked) {
            return Promise.resolve(new Response(null, { status: 204 }));
          }
        } catch (err) { }
      }
      // Handle Blob / File body (e.g. direct PUT / POST to Azure Blob, CDN, or file storage)
      else if (body && (body instanceof Blob || (typeof File !== "undefined" && body instanceof File))) {
        try {
          var fBlobName = body.name || lastCreatedFileName || "upload_attachment";
          inspectAndStageFile(body, fBlobName);
          netFiles.push({ name: fBlobName, size: body.size, type: body.type });
        } catch (_) {}
      }

      // Check if URL looks like a pure file upload endpoint (ChatGPT /files, Claude upload, Azure Blob, S3, etc.)
      var lowUrl = (url || "").toLowerCase();
      var isPureUploadEndpoint =
        lowUrl.includes("/backend-api/files") ||
        lowUrl.includes("/files") ||
        lowUrl.includes("/upload") ||
        lowUrl.includes("blob.core.windows.net") ||
        lowUrl.includes("amazonaws.com") ||
        lowUrl.includes("storage.googleapis.com") ||
        lowUrl.includes("/media/upload") ||
        lowUrl.includes("/convert_document") ||
        lowUrl.includes("/attachments");

      // Check if master upload block is active on this site
      if (govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
        if (isPureUploadEndpoint || (body && typeof body.forEach === "function") || (netFiles && netFiles.length > 0)) {
          showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
          return Promise.reject(new TypeError("File uploads and attachments to this AI platform are blocked by corporate policy."));
        }
      }

      // Pure file uploads without a typed prompt MUST pass through immediately with zero delay and unmodified headers!
      // (The file was already staged via inspectAndStageFile, and full DLP scanning runs when the user clicks Send)
      if (!promptText || isPureUploadEndpoint) {
        var rawPromise = origFetch.apply(this, arguments);
        try {
          if (rawPromise && typeof rawPromise.then === "function") {
            rawPromise.then(function (res) {
              try {
                var ct = (res && res.headers && res.headers.get("content-type")) || "";
                if (ct.includes("json")) {
                  res.clone().json().then(function (data) {
                    scanObjectForSession(data, 0);
                  }).catch(function () {});
                }
              } catch (err) {}
            }).catch(function () {});
          }
        } catch (e) {}
        return rawPromise;
      }

      if (promptText) {
        promptText = stripInvisible(promptText);
      }
      var ptClean = (promptText || "").trim().toLowerCase();
      var isNoisePrompt = !promptText || ptClean === "set" || ptClean === "ping" || ptClean === "pong" || ptClean === "heartbeat" || ptClean === "true" || ptClean === "false" || ptClean === "null" || ptClean === "me" || ptClean.indexOf("plsenvironmentquery") >= 0 || ptClean.indexOf("assign copilot license") >= 0;
      if (isNoisePrompt) {
        return origFetch.apply(this, arguments);
      }

      var args = arguments;
      var targetUrl = url;

      // Robustly enrich netFiles with cached base64 and text content
      var now_ts = Date.now();
      var freshStaged = getAllFreshStagedFiles();
      for (var nf = 0; nf < netFiles.length; nf++) {
        var nFile = netFiles[nf];
        var cachedFile = getCachedStagedFile(nFile.name);
        if (cachedFile) {
          if (!nFile.text && cachedFile.text) nFile.text = cachedFile.text;
          if (!nFile.base64 && cachedFile.base64) nFile.base64 = cachedFile.base64;
          if (!nFile.size && cachedFile.size) nFile.size = cachedFile.size;
          if (!nFile.type && cachedFile.type) nFile.type = cachedFile.type;
        }
      }
      // Positionally match any netFiles still lacking content to fresh staged files (handles ChatGPT asset_pointer file-xxx)
      for (var fi2 = 0; fi2 < netFiles.length && fi2 < freshStaged.length; fi2++) {
        if (!netFiles[fi2].text && !netFiles[fi2].base64) {
          var sf2 = freshStaged[fi2];
          if (sf2.text) netFiles[fi2].text = sf2.text;
          if (sf2.base64) netFiles[fi2].base64 = sf2.base64;
          if (!netFiles[fi2].size && sf2.size) netFiles[fi2].size = sf2.size;
          if (!netFiles[fi2].type && sf2.type) netFiles[fi2].type = sf2.type;
          if (sf2.name && /^file[-_][a-zA-Z0-9]+$/.test(netFiles[fi2].name || "")) {
            netFiles[fi2].name = sf2.name;
          }
        }
      }
      if (netFiles.length === 0 && freshStaged.length > 0) {
        netFiles = freshStaged.slice();
      }
      if (netFiles.length === 0) {
        var domFound = mergeStagedAndDomFiles();
        if (domFound.length > 0) {
          netFiles = domFound;
        }
      }

      return ask(promptText, netFiles, []).then(function (decision) {
        var sent = false;
        var result = null;
        handleDecision(decision, function (chosen) {
          sent = true;
          var nextArgs = args;
          var action = String((chosen && chosen.action) || "").toLowerCase();
          var nextInit = init ? Object.assign({}, init) : {};
          if ((action === "redacted" || action === "redact") && chosen.redacted_prompt && init) {
            var rewritten = rewriteBody(init.body, chosen.redacted_prompt, promptText);
            if (rewritten) {
              nextInit.body = rewritten;
            }
          }
          if (nextInit && nextInit.body && nextInit.body !== (init && init.body)) {
            var modifiedArgs = Array.prototype.slice.call(args);
            if (modifiedArgs.length >= 2) {
              modifiedArgs[1] = nextInit;
              result = origFetch.apply(window, modifiedArgs);
            } else {
              result = origFetch.call(window, targetUrl, nextInit);
            }
          } else {
            result = origFetch.apply(window, nextArgs);
          }
        });
        if (!sent) return createBlockedFetchResponse(decision, targetUrl);
        return result;
      });
    };
  }

  // Hook XMLHttpRequest (Legacy AJAX)
  var origXHRSend = XMLHttpRequest.prototype.send;
  var origXHROpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (method, url) {
    this._ggMethod = method;
    this._ggUrl = url;
    try {
      var lowU = String(url || "").toLowerCase();
      if (lowU.includes("session") || lowU.includes("account") || lowU.includes("user") || lowU.includes("auth") || lowU.includes("me") || lowU.includes("organization") || lowU.includes("profile")) {
        this.addEventListener("load", function () {
          try {
            if (this.responseText && this.responseText.startsWith("{")) {
              scanObjectForSession(JSON.parse(this.responseText), 0);
            }
          } catch (xe) {}
        });
      }
    } catch (e) {}
    return origXHROpen.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function (body) {
    if (!armed || !body) return origXHRSend.apply(this, arguments);

    var xhrUrl = String(this._ggUrl || "").toLowerCase();
    var isXhrUpload =
      xhrUrl.includes("/backend-api/files") ||
      xhrUrl.includes("/files") ||
      xhrUrl.includes("/upload") ||
      xhrUrl.includes("blob.core.windows.net") ||
      xhrUrl.includes("amazonaws.com") ||
      xhrUrl.includes("storage.googleapis.com") ||
      xhrUrl.includes("/media/upload") ||
      xhrUrl.includes("/convert_document") ||
      xhrUrl.includes("/attachments");

    if (armed && govBlockUpload && isControlActiveForSite(govUploadTargetWebsites)) {
      if (isXhrUpload || (body && (typeof FormData !== "undefined" && body instanceof FormData || typeof Blob !== "undefined" && body instanceof Blob || typeof File !== "undefined" && body instanceof File || typeof body.forEach === "function"))) {
        showBubble(govUploadWarning || "File uploads and attachments to this AI platform are blocked by corporate policy.", "File Upload Policy", "block");
        try {
          this.abort();
        } catch (e) {}
        return;
      }
    }

    var promptText = "";
    if (typeof body === "string") {
      try {
        var parsed = JSON.parse(body);
        if (parsed && typeof parsed.prompt === "string") promptText = parsed.prompt;
      } catch (e) { }
    }
    if (promptText) {
      var clientDecision = evaluateClientRules(promptText, []);
      if (clientDecision && clientDecision.blocked) {
        showBubble(clientDecision.message, clientDecision.rule, "block");
        return;
      }
      var cAct = String((clientDecision && clientDecision.action) || "").toLowerCase();
      if ((cAct === "redacted" || cAct === "redact") && clientDecision.redacted_prompt) {
        var rewrittenXhr = rewriteBody(body, clientDecision.redacted_prompt, promptText);
        if (rewrittenXhr) {
          showBubble(clientDecision.message || "Sensitive text was redacted before sending.", clientDecision.rule, "redact");
          return origXHRSend.call(this, rewrittenXhr);
        }
      }
    }
    return origXHRSend.apply(this, arguments);
  };

  // Hook WebSocket (Streaming & Real-time AI Chatbots: Copilot SignalR, ChatGPT SSE, etc.)
  if (typeof window.WebSocket !== "undefined" && window.WebSocket.prototype) {
    var origWSSend = window.WebSocket.prototype.send;
    window.WebSocket.prototype.send = function (data) {
      if (armed && typeof data === "string" && data.length > 3) {
        var cleanStr = data.replace(/\x1e/g, "").trim();
        var promptText = "";
        try {
          var parsed = JSON.parse(cleanStr);
          if (parsed) {
            if (Array.isArray(parsed.arguments) && parsed.arguments[0]) {
              var arg0 = parsed.arguments[0];
              if (arg0.message && typeof arg0.message.text === "string") {
                promptText = arg0.message.text;
              } else if (arg0.message && typeof arg0.message.content === "string") {
                promptText = arg0.message.content;
              } else if (typeof arg0.text === "string") {
                promptText = arg0.text;
              } else if (typeof arg0.input === "string") {
                promptText = arg0.input;
              } else if (typeof arg0.prompt === "string") {
                promptText = arg0.prompt;
              } else if (typeof arg0.query === "string") {
                promptText = arg0.query;
              }
            }
            if (!promptText) {
              if (typeof parsed.prompt === "string") promptText = parsed.prompt;
              else if (typeof parsed.text === "string") promptText = parsed.text;
              else if (typeof parsed.input === "string") promptText = parsed.input;
              else if (typeof parsed.query === "string") promptText = parsed.query;
              else if (parsed.message && typeof parsed.message.text === "string") promptText = parsed.message.text;
              else if (parsed.message && typeof parsed.message.content === "string") promptText = parsed.message.content;
            }
          }
        } catch (e) {}

        promptText = stripInvisible(promptText);
        // Only evaluate if real user text was extracted (NEVER evaluate raw JSON packets/timestamps)
        if (promptText && promptText.length > 0 && !promptText.startsWith("{") && !promptText.startsWith("[")) {
          var wsDecision = evaluateClientRules(promptText, []);
          if (wsDecision && wsDecision.blocked) {
            showBubble(wsDecision.message, wsDecision.rule, "block");
            window.postMessage({
              source: "gateway-guard-page",
              nonce: guardNonce,
              id: "ws_block_" + Date.now().toString(36),
              prompt: promptText.slice(0, 500),
              host: location.hostname,
              url: location.href,
              is_blocked: true,
              rule: wsDecision.rule || ""
            }, "*");
            return;
          }
          var wsAct = String((wsDecision && wsDecision.action) || "").toLowerCase();
          if ((wsAct === "redacted" || wsAct === "redact") && wsDecision.redacted_prompt) {
            showBubble(wsDecision.message || "Sensitive text was redacted before sending.", wsDecision.rule, "redact");
            var rewrittenWs = rewriteBody(cleanStr, wsDecision.redacted_prompt, promptText);
            if (rewrittenWs) {
              var finalWsData = data.endsWith("\x1e") ? (rewrittenWs + "\x1e") : rewrittenWs;
              return origWSSend.call(this, finalWsData);
            }
          }
          // Coordinate with extension and desktop proxy: record allowed WebSocket prompt so proxy skips duplicate logging
          window.postMessage({
            source: "gateway-guard-page",
            nonce: guardNonce,
            id: "ws_allow_" + Date.now().toString(36),
            prompt: promptText.slice(0, 500),
            host: location.hostname,
            url: location.href,
            is_blocked: false,
            rule: (wsDecision && wsDecision.rule) || ""
          }, "*");
        }
      }
      return origWSSend.apply(this, arguments);
    };
  }

  // Hook navigator.sendBeacon (Safe Passthrough: background analytics/telemetry must NEVER trigger prompt block alerts)
  if (typeof navigator.sendBeacon === "function") {
    var origSendBeacon = navigator.sendBeacon.bind(navigator);
    navigator.sendBeacon = function (url, data) {
      return origSendBeacon(url, data);
    };
  }

  // Periodic, load, and focus session inspection
  [400, 1200, 2500].forEach(function (ms) {
    setTimeout(function () {
      if (armed) {
        scanStorageForSession();
        scanDOMForSession();
      }
    }, ms);
  });
  window.addEventListener("focus", function () {
    if (armed) {
      scanStorageForSession();
      scanDOMForSession();
    }
  });
  setInterval(function () {
    if (armed) {
      scanStorageForSession();
      scanDOMForSession();
    }
  }, 5000);

  if (typeof window !== "undefined") {
    window.__yp_test = {
      evaluateClientRules: evaluateClientRules,
      rewriteBody: rewriteBody,
      setComposerText: setComposerText,
      setDynamicRules: setDynamicRules
    };
  }

  console.log("[YP-Raksha] In-Browser DLP Engine Active: Prompts, Images, Files, WebSockets & Corporate AI Sessions Protected.");
})();

