/* YP-Raksha Extension Content Script (Isolated World)
 * Manages communication between page hook, local agent (port 18195), and local storage cache.
 * Implements anti-tamper nonces and offline rule persistence.
 */
(function () {
  var base = (typeof GATEWAY_GUARD_BASE === "string" && GATEWAY_GUARD_BASE.trim())
    ? GATEWAY_GUARD_BASE.trim().replace(/\/+$/, "")
    : "http://127.0.0.1:18195";

  var nonceBytes = new Uint8Array(24);
  crypto.getRandomValues(nonceBytes);
  var guardNonce = "yp_" + Array.prototype.map.call(nonceBytes, function (b) { return b.toString(16).padStart(2, "0"); }).join("");

  function normalizeDomainHost(value) {
    var host = String(value || "").toLowerCase().trim();
    if (!host) return "";
    if (host.indexOf("://") >= 0) {
      try { host = new URL(host).hostname; } catch (e) { return ""; }
    } else {
      host = host.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0];
      if ((host.match(/:/g) || []).length === 1 && /:\d+$/.test(host)) {
        host = host.replace(/:\d+$/, "");
      }
    }
    if (host.charAt(0) === "[" && host.indexOf("]") >= 0) {
      host = host.slice(1, host.indexOf("]"));
    }
    host = host.replace(/^www\./, "").replace(/\.$/, "");
    return host;
  }

  function isLoopbackHost(host) {
    return host === "localhost" || host === "127.0.0.1" || host === "0.0.0.0" || host === "::1" ||
      /^127\./.test(host) || /\.localhost$/.test(host);
  }

  var serverHost = (typeof GATEWAY_SERVER_HOST === "string" && GATEWAY_SERVER_HOST.trim()) ? normalizeDomainHost(GATEWAY_SERVER_HOST) : "";
  function isServerHost(host) {
    if (!host) return false;
    host = normalizeDomainHost(host);
    if (host === "localhost" || host === "127.0.0.1" || host === "0.0.0.0" || host === "::1") return true;
    if (serverHost && (host === serverHost || host.endsWith("." + serverHost))) return true;
    return false;
  }

  // EARLY RETURN: Never run or hook inside our product dashboard, local UI, or gateway portal!
  if (isServerHost(location.hostname)) {
    return;
  }

  var monitoredSitesForPolicy = [];
  var clipboardPolicy = { enabled: false, targetSites: "", message: "", monitoredSites: [] };

  function renderBlockedPage(hostname) {
    try {
      window.stop();
    } catch (e) { }
    window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
    var displayHost = (hostname || location.hostname || "").toLowerCase().replace(/^www\./, "");
    var blockHtml = '<!DOCTYPE html>' +
      '<html lang="en">' +
      '<head>' +
      '  <meta charset="utf-8"/>' +
      '  <meta name="viewport" content="width=device-width, initial-scale=1"/>' +
      '  <title>Blocked by Gateway Guard</title>' +
      '  <style>' +
      '    * { box-sizing: border-box; margin: 0; padding: 0; }' +
      '    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b1220; color: #e2e8f0; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 20px; }' +
      '    .card { max-width: 540px; width: 100%; padding: 36px 32px; border: 1px solid #334155; border-radius: 14px; background: #111827; text-align: center; box-shadow: 0 20px 40px rgba(0,0,0,0.6); }' +
      '    .icon { width: 56px; height: 56px; margin: 0 auto 16px; background: rgba(239, 68, 68, 0.15); border: 2px solid #ef4444; border-radius: 50%; display: flex; align-items: center; justify-content: center; color: #ef4444; font-size: 26px; font-weight: bold; }' +
      '    h1 { margin: 0 0 12px; font-size: 22px; color: #f87171; font-weight: 700; }' +
      '    p { margin: 0 0 10px; line-height: 1.6; color: #cbd5e1; font-size: 14px; }' +
      '    code { background: #1e293b; color: #93c5fd; padding: 3px 8px; border-radius: 5px; font-size: 13px; font-weight: 600; word-break: break-all; }' +
      '    .footer { margin-top: 24px; padding-top: 16px; border-top: 1px solid #1e293b; font-size: 12px; color: #64748b; }' +
      '  </style>' +
      '</head>' +
      '<body>' +
      '  <div class="card">' +
      '    <div class="icon">✕</div>' +
      '    <h1>Website Blocked by Gateway Guard</h1>' +
      '    <p>Access to <code>' + displayHost + '</code> is blocked by your organization\'s security policy.</p>' +
      '    <div class="footer">UnifAI YP-Raksha Enterprise Security</div>' +
      '  </div>' +
      '</body>' +
      '</html>';

    try {
      if (document.documentElement) {
        document.documentElement.innerHTML = blockHtml;
      } else {
        document.write(blockHtml);
      }
    } catch (e) { }
  }

  var sitesMap = null;
  var lastSitesList = null;
  var blockedSitesMap = null;
  var lastBlockedSitesList = null;

  function buildDomainMap(domains) {
    var map = Object.create(null);
    if (!Array.isArray(domains)) return map;
    for (var i = 0; i < domains.length; i++) {
      var d = normalizeDomainHost(domains[i]);
      if (d) map[d] = true;
    }
    return map;
  }

  function matchDomainInMap(host, map, rawList) {
    host = normalizeDomainHost(host);
    if (!host) return false;

    if (map && map[host]) return true;

    // Fast O(1) parent subdomain traversal (e.g. sub.copilot.cloud.microsoft -> cloud.microsoft)
    var dot = host.indexOf(".");
    while (dot !== -1) {
      var parent = host.substring(dot + 1);
      if (map && map[parent]) return true;
      dot = host.indexOf(".", dot + 1);
    }

    if (Array.isArray(rawList)) {
      for (var j = 0; j < rawList.length; j++) {
        var dom = normalizeDomainHost(rawList[j]);
        if (!dom) continue;
        if (host === dom || host.slice(-(dom.length + 1)) === "." + dom) return true;
      }
    }
    return false;
  }

  // O(1) hash map matcher that scales to 10,000+ target websites with zero latency
  function hostMatches(host, domains) {
    if (!sitesMap || (Array.isArray(domains) && domains !== lastSitesList)) {
      lastSitesList = domains;
      sitesMap = buildDomainMap(domains);
    }
    return matchDomainInMap(host, sitesMap, domains);
  }

  function hostBlockedMatches(host, blockedDomains) {
    if (!blockedSitesMap || (Array.isArray(blockedDomains) && blockedDomains !== lastBlockedSitesList)) {
      lastBlockedSitesList = blockedDomains;
      blockedSitesMap = buildDomainMap(blockedDomains);
    }
    return matchDomainInMap(host, blockedSitesMap, blockedDomains);
  }

  function hostInConfiguredSites(host, rawSites) {
    var raw = String(rawSites || "").trim();
    if (!raw) return false;
    if (raw.toUpperCase() === "ALL" || raw === "*") return true;
    if (raw.toLowerCase() === "none") return false;
    var current = normalizeDomainHost(host);
    var parts = raw.split(/[;,\s]+/);
    for (var i = 0; i < parts.length; i++) {
      var item = parts[i].trim().replace(/^\*\./, "").replace(/^\./, "");
      if (!item) continue;
      item = normalizeDomainHost(item.indexOf("://") >= 0 ? item : item.indexOf(":") >= 0 && (item.match(/:/g) || []).length > 1 ? "https://[" + item + "]" : "https://" + item);
      if (item && (current === item || current.endsWith("." + item))) return true;
    }
    return false;
  }

  function clipboardPolicyApplies() {
    if (!clipboardPolicy.enabled) return false;
    // The governance selector only applies to registered monitored targets.
    // Keep this guard aligned with page_hook.js so a stale per-control target
    // cannot keep blocking a site after it has been removed from Target Websites.
    if (isServerHost(location.hostname)) return false;
    var raw = String(clipboardPolicy.targetSites || "").trim();
    if (raw && raw.toLowerCase() !== "all" && raw !== "*") {
      return hostInConfiguredSites(location.hostname, raw);
    }
    var monitoredSites = clipboardPolicy.monitoredSites || monitoredSitesForPolicy;
    if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
      return hostMatches(location.hostname, monitoredSites);
    }
    return true;
  }

  var clipboardNoticeAt = 0;
  function showClipboardBlockedNotice() {
    var now = Date.now();
    if (now - clipboardNoticeAt < 1000) return;
    clipboardNoticeAt = now;
    if (document.getElementById("yp-raksha-alert-card")) return;
    try {
      var old = document.getElementById("yp-raksha-isolated-clipboard-notice");
      if (old) old.remove();
      var host = document.createElement("div");
      host.id = "yp-raksha-isolated-clipboard-notice";
      var shadow = host.attachShadow ? host.attachShadow({ mode: "closed" }) : host;
      var box = document.createElement("div");
      box.style.cssText =
        "position:fixed;bottom:95px;left:50%;transform:translateX(-50%);max-width:460px;width:fit-content;min-width:280px;" +
        "z-index:2147483647;background:rgba(15,23,42,0.96);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);" +
        "color:#f9fafb;border:1px solid #ef4444;border-radius:12px;padding:9px 14px;box-shadow:0 10px 30px rgba(0,0,0,0.5);" +
        "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;transition:opacity 0.2s ease;pointer-events:none;";
      box.innerHTML =
        '<div style="display:flex;align-items:center;gap:6px;margin-bottom:3px;">' +
          '<span style="font-size:13px;line-height:1;">🛡️</span>' +
          '<strong style="font-size:11px;font-weight:700;letter-spacing:0.03em;text-transform:uppercase;color:#f87171;">Clipboard Paste Restricted</strong>' +
        '</div>' +
        '<div style="font-size:12.5px;line-height:1.4;color:#e2e8f0;font-weight:400;">' +
          (clipboardPolicy.message || "Pasting into AI inputs is restricted") +
        '</div>';
      shadow.appendChild(box);
      (document.documentElement || document).appendChild(host);
      setTimeout(function () {
        try {
          box.style.opacity = "0";
          setTimeout(function () { try { host.remove(); } catch (_) {} }, 200);
        } catch (e) {}
      }, 1000);
    } catch (e) {}
  }

  var _lastGovLogSig = "";
  var _lastGovLogTime = 0;
  function recordIsolatedGovernanceLog(promptText, ruleName) {
    var now = Date.now();
    var sig = promptText + "||" + ruleName;
    if (sig === _lastGovLogSig && (now - _lastGovLogTime < 2500)) return;
    _lastGovLogSig = sig;
    _lastGovLogTime = now;
    var checkPayload = {
      id: "gov_" + now.toString(36) + "_" + Math.random().toString(36).slice(2),
      eval_id: "gov_" + now.toString(36),
      host: location.hostname,
      url: location.href,
      prompt: promptText,
      files: [],
      upload_images: [],
      is_blocked: true,
      rule: ruleName,
      blocked_reason: ruleName,
      status: "Blocked (" + ruleName + ")",
      browser: detectCurrentBrowser()
    };
    withBase(function () {
      if (base) {
        fetch(base + "/extension/check", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Browser-Guard-Extension": "1" },
          body: JSON.stringify(checkPayload)
        }).catch(function () {
          if (serverHost) {
            fetch("https://" + serverHost + "/api/browser-ai/intercept", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(checkPayload)
            }).catch(function () {});
          }
        });
      } else if (serverHost) {
        fetch("https://" + serverHost + "/api/browser-ai/intercept", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(checkPayload)
        }).catch(function () {});
      }
    });
  }

  function blockClipboardEvent(ev) {
    if (!clipboardPolicyApplies()) return;
    if (document.getElementById("yp-blur-curtain") || document.getElementById("yp-screenshot-shield")) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      return;
    }
    // Only intercept PASTE events (prevent exfiltration into AI input boxes).
    // Allow COPY so users can seamlessly copy AI answers and code snippets without errors.
    var isPaste = ev.type && ev.type.indexOf("paste") >= 0;
    if (!isPaste) return;

    if (ev.clipboardData) {
      try { ev.clipboardData.clearData(); } catch (_) {}
    }
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation();
    showClipboardBlockedNotice();
    recordIsolatedGovernanceLog("--copy paste [CLIPBOARD PASTE]", "Clipboard Policy");
  }

  function handleClipboardKeys(e) {
    if (!clipboardPolicyApplies()) return;
    var isCtrlOrMeta = e.ctrlKey || e.metaKey;
    // Only intercept PASTE shortcuts (Ctrl+V, Shift+Insert).
    // Allow COPY shortcuts (Ctrl+C, Ctrl+Insert) so copying is fast, smooth, and error-free.
    var isPaste = (isCtrlOrMeta && (e.key === "v" || e.key === "V" || e.code === "KeyV")) ||
                  (e.shiftKey && (e.key === "Insert" || e.code === "Insert"));

    if (isPaste) {
      try {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      } catch (_) {}
      showClipboardBlockedNotice();
      recordIsolatedGovernanceLog("--copy paste [CLIPBOARD PASTE KEY]", "Clipboard Policy");
    }
  }

  ["paste", "beforepaste"].forEach(function (type) {
    window.addEventListener(type, blockClipboardEvent, true);
    document.addEventListener(type, blockClipboardEvent, true);
  });
  window.addEventListener("keydown", handleClipboardKeys, true);
  document.addEventListener("keydown", handleClipboardKeys, true);

  function applyClipboardPolicyState(state) {
    state = state || {};
    if (Array.isArray(state.cached_sites)) monitoredSitesForPolicy = state.cached_sites;
    clipboardPolicy = {
      enabled: typeof state.block_clipboard === "boolean" ? state.block_clipboard : clipboardPolicy.enabled,
      targetSites: typeof state.clipboard_target_websites === "string" ? state.clipboard_target_websites : clipboardPolicy.targetSites,
      message: typeof state.clipboard_block_message === "string"
        ? state.clipboard_block_message
        : (typeof state.governance_block_message === "string" ? state.governance_block_message : clipboardPolicy.message),
      monitoredSites: monitoredSitesForPolicy
    };
    applyScreenshotPolicyState(state);
    applyDragDropPolicyState(state);
  }

  var dragDropPolicy = { enabled: false, targetSites: "", message: "", monitoredSites: [] };
  function dragDropPolicyApplies() {
    if (!dragDropPolicy.enabled) return false;
    if (isServerHost(location.hostname)) return false;
    var raw = String(dragDropPolicy.targetSites || "").trim();
    if (raw && raw.toLowerCase() !== "all" && raw !== "*") {
      return hostInConfiguredSites(location.hostname, raw);
    }
    var monitoredSites = dragDropPolicy.monitoredSites || monitoredSitesForPolicy;
    if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
      return hostMatches(location.hostname, monitoredSites);
    }
    return true;
  }

  function applyDragDropPolicyState(state) {
    state = state || {};
    dragDropPolicy = {
      enabled: typeof state.block_drag_drop === "boolean" ? state.block_drag_drop : dragDropPolicy.enabled,
      targetSites: typeof state.drag_drop_target_websites === "string" ? state.drag_drop_target_websites : dragDropPolicy.targetSites,
      message: typeof state.drag_drop_block_message === "string"
        ? state.drag_drop_block_message
        : (typeof state.governance_block_message === "string" ? state.governance_block_message : dragDropPolicy.message),
      monitoredSites: monitoredSitesForPolicy
    };
  }

  var dragDropNoticeAt = 0;
  function showDragDropBlockedNotice() {
    var now = Date.now();
    if (now - dragDropNoticeAt < 1200) return;
    dragDropNoticeAt = now;
    if (document.getElementById("yp-raksha-alert-card")) return;
    try {
      var host = document.getElementById("yp-raksha-isolated-dragdrop-notice");
      if (!host) {
        host = document.createElement("div");
        host.id = "yp-raksha-isolated-dragdrop-notice";
        var shadow = host.attachShadow ? host.attachShadow({ mode: "closed" }) : host;
        var box = document.createElement("div");
        box.textContent = dragDropPolicy.message || "Drag and drop restricted";
        box.style.cssText = "position:fixed;top:20px;left:50%;transform:translateX(-50%);z-index:2147483647;background:#7f1d1d;color:white;padding:10px 22px;border-radius:10px;font:600 13px Segoe UI,Arial,sans-serif;box-shadow:0 8px 28px #0008;max-width:80vw;text-align:center;transition:opacity 0.2s ease;";
        shadow.appendChild(box);
        (document.documentElement || document).appendChild(host);
        setTimeout(function () {
          try {
            box.style.opacity = "0";
            setTimeout(function () { try { host.remove(); } catch (_) {} }, 200);
          } catch (e) {}
        }, 1000);
      }
    } catch (e) {}
  }

  function blockDragDropEvent(ev) {
    if (!dragDropPolicyApplies()) return;
    var droppedSummary = "";
    if (ev.dataTransfer) {
      if (ev.dataTransfer.files && ev.dataTransfer.files.length) {
        var names = [];
        for (var i = 0; i < ev.dataTransfer.files.length; i++) names.push(ev.dataTransfer.files[i].name);
        droppedSummary = "Files: " + names.join(", ");
      } else {
        try {
          var t = ev.dataTransfer.getData("text/plain");
          if (t) droppedSummary = "Text: " + t.trim().slice(0, 300);
        } catch (_) {}
      }
      ev.dataTransfer.dropEffect = "none";
      try { ev.dataTransfer.clearData(); } catch (_) {}
    }
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation();
    if (ev.type === "drop") {
      showDragDropBlockedNotice();
      recordIsolatedGovernanceLog("--drag drop [DRAG & DROP BLOCKED]" + (droppedSummary ? " " + droppedSummary : ""), "Drag & Drop Policy");
    }
  }

  ["dragenter", "dragover", "drop"].forEach(function (type) {
    window.addEventListener(type, blockDragDropEvent, true);
    document.addEventListener(type, blockDragDropEvent, true);
  });

  var screenshotPolicy = { enabled: false, targetSites: "", message: "", monitoredSites: [] };
  function screenshotPolicyApplies() {
    if (!screenshotPolicy.enabled) return false;
    if (isServerHost(location.hostname)) return false;
    var raw = String(screenshotPolicy.targetSites || "").trim();
    if (raw && raw.toLowerCase() !== "all" && raw !== "*") {
      return hostInConfiguredSites(location.hostname, raw);
    }
    var monitoredSites = screenshotPolicy.monitoredSites || monitoredSitesForPolicy;
    if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
      return hostMatches(location.hostname, monitoredSites);
    }
    return true;
  }

  function applyScreenshotPolicyState(state) {
    state = state || {};
    screenshotPolicy = {
      enabled: typeof state.block_screenshot === "boolean" ? state.block_screenshot : screenshotPolicy.enabled,
      targetSites: typeof state.screenshot_target_websites === "string" ? state.screenshot_target_websites : screenshotPolicy.targetSites,
      message: typeof state.screenshot_block_message === "string"
        ? state.screenshot_block_message
        : (typeof state.governance_block_message === "string" ? state.governance_block_message : screenshotPolicy.message),
      monitoredSites: monitoredSitesForPolicy
    };
  }

  function applyIsolatedCurtain() {
    if (!screenshotPolicyApplies()) return;
    var curtain = document.getElementById("yp-blur-curtain");
    if (!curtain) {
      curtain = document.createElement("div");
      curtain.id = "yp-blur-curtain";
      curtain.style.cssText = "position:fixed;inset:0;width:100vw;height:100vh;background:#0a0f1d;opacity:0.98;backdrop-filter:blur(30px);-webkit-backdrop-filter:blur(30px);z-index:2147483646;pointer-events:none;display:flex;align-items:center;justify-content:center;color:#f87171;font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:20px;font-weight:700;";
      curtain.innerHTML = '<div style="display:flex;align-items:center;gap:10px;"><span style="font-size:28px;">🛡️</span><span>SCREEN CAPTURE RESTRICTED</span></div>';
      (document.body || document.documentElement).appendChild(curtain);
    }
  }

  function removeIsolatedCurtain() {
    var curtain = document.getElementById("yp-blur-curtain");
    if (curtain) curtain.remove();
  }

  window.addEventListener("blur", applyIsolatedCurtain, true);
  window.addEventListener("focus", removeIsolatedCurtain, true);
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) applyIsolatedCurtain();
    else removeIsolatedCurtain();
  }, true);

  function handleIsolatedCaptureKeys(e) {
    if (!screenshotPolicyApplies()) return;
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
      // NOTE: Do NOT call navigator.clipboard.writeText("") here.
      // Doing so fires a clipboard event which triggers the clipboard
      // blocked banner simultaneously with the screenshot curtain.
      applyIsolatedCurtain();
      setTimeout(removeIsolatedCurtain, 2500);
    }
  }

  window.addEventListener("keydown", handleIsolatedCaptureKeys, true);
  document.addEventListener("keydown", handleIsolatedCaptureKeys, true);
  window.addEventListener("keyup", function (e) {
    if (!screenshotPolicyApplies()) return;
    if (e.key === "PrintScreen" || e.keyCode === 44 || e.code === "PrintScreen") {
      applyIsolatedCurtain();
      setTimeout(removeIsolatedCurtain, 2500);
    }
  }, true);

  if (chrome.storage && chrome.storage.local) {
    chrome.storage.local.get(["cached_sites", "block_clipboard", "clipboard_target_websites", "clipboard_block_message", "governance_block_message", "block_screenshot", "screenshot_target_websites", "screenshot_block_message"], applyClipboardPolicyState);
  }

  // Accurate client-side browser detector across Chrome, Edge, Safari, Firefox, Brave, Opera, Vivaldi
  function detectCurrentBrowser() {
    try {
      if (navigator.brave && typeof navigator.brave.isBrave === "function") return "Brave";
      var ua = navigator.userAgent || "";
      if (/edg\//i.test(ua)) return "Edge";
      if (/opr\/|opera/i.test(ua)) return "Opera";
      if (/firefox\/|fxios\//i.test(ua)) return "Firefox";
      if (/vivaldi/i.test(ua)) return "Vivaldi";
      if (/safari/i.test(ua) && !/chrome|chromium|crios/i.test(ua)) return "Safari";
      if (/chrome|chromium|crios/i.test(ua)) return "Chrome";
    } catch (e) { }
    return "Browser";
  }

  var hookInjected = false;
  function injectHook(done) {
    if (hookInjected) {
      if (done) done();
      return;
    }
    try {
      hookInjected = true;
      var s = document.createElement("script");
      s.src = chrome.runtime.getURL("page_hook.js");
      s.dataset.guardNonce = guardNonce;
      s.onload = function () { s.remove(); if (done) done(); };
      (document.documentElement || document.head || document.body).appendChild(s);
    } catch (e) {
      if (done) done();
    }
  }

  function withBase(run) {
    if (typeof resolveGuardBase === "function") {
      resolveGuardBase(function (found) {
        if (found) base = found;
        run();
      });
      return;
    }
    run();
  }

  // Policy keys written by background.js and forwarded to the page hook.
  var POLICY_KEYS = [
    "cached_rules", "cached_exts",
    "block_upload", "upload_warning", "upload_target_websites",
    "file_extension_mode", "restricted_file_extensions", "allowed_file_extensions",
    "block_file_extension", "file_extension_block_message", "file_extension_target_websites",
    "block_clipboard", "block_drag_drop", "block_download", "governance_target_websites", "governance_block_message",
    "clipboard_target_websites", "drag_drop_target_websites", "download_target_websites",
    "clipboard_block_message", "drag_drop_block_message", "download_block_message",
    "block_screenshot", "screenshot_block_message", "screenshot_target_websites"
  ];
  var SITE_KEYS = ["cached_sites", "cached_blocked_sites", "server_host"];
  var CLIPBOARD_KEYS = [
    "cached_sites", "block_clipboard", "clipboard_target_websites", "clipboard_block_message", "governance_block_message",
    "block_screenshot", "screenshot_target_websites", "screenshot_block_message",
    "block_drag_drop", "drag_drop_target_websites", "drag_drop_block_message"
  ];

  var armedHere = false;
  var pageBlocked = false;

  function policyMessage(res) {
    res = res || {};
    var msg = {
      source: "gateway-guard",
      nonce: guardNonce,
      type: "sync_rules",
      rules: Array.isArray(res.cached_rules) ? res.cached_rules : [],
      restricted_extensions: Array.isArray(res.cached_exts) ? res.cached_exts : [],
      file_extension_mode: res.file_extension_mode || "block_listed"
    };
    POLICY_KEYS.forEach(function (key) {
      if (key === "cached_rules" || key === "cached_exts" || key === "file_extension_mode") return;
      msg[key] = key.indexOf("block_") === 0 && key.slice(-8) !== "_message" ? Boolean(res[key]) : (res[key] || "");
    });
    return msg;
  }

  // Policy reaches the page only while this frame is an armed Target Website.
  function postPolicyToPage() {
    if (!armedHere || !chrome.storage || !chrome.storage.local) return;
    chrome.storage.local.get(POLICY_KEYS, function (res) {
      if (!armedHere) return;
      window.postMessage(policyMessage(res), "*");
    });
  }

  function disarmPage() {
    if (!armedHere) return;
    armedHere = false;
    window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
  }

  function evaluateSite(state) {
    state = state || {};
    // The agent may report its own loopback address; never let it replace the gateway domain.
    var storedHost = normalizeDomainHost(state.server_host);
    if (storedHost && !isLoopbackHost(storedHost)) serverHost = storedHost;
    if (isServerHost(location.hostname)) {
      disarmPage();
      return;
    }
    var blockedSites = Array.isArray(state.cached_blocked_sites) ? state.cached_blocked_sites : [];
    if (hostBlockedMatches(location.hostname, blockedSites)) {
      armedHere = false;
      if (!pageBlocked) {
        pageBlocked = true;
        renderBlockedPage(location.hostname);
      }
      return;
    }
    var sites = Array.isArray(state.cached_sites) ? state.cached_sites : [];
    var isTarget = hostMatches(location.hostname, sites) || clipboardPolicyApplies() || dragDropPolicyApplies() || screenshotPolicyApplies();
    if (!isTarget) {
      // Not a Target Website or Governance Policy site: stay passive.
      disarmPage();
      return;
    }
    if (armedHere) return;
    injectHook(function () {
      armedHere = true;
      window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
      postPolicyToPage();
      // Wakes a sleeping background worker so its live link to the EXE is running.
      requestBackgroundSync();
    });
  }

  var lastSyncRequestAt = 0;
  function requestBackgroundSync() {
    var now = Date.now();
    if (now - lastSyncRequestAt < 2000) return; // Reduced from 5000ms → faster policy pickup
    lastSyncRequestAt = now;
    try {
      chrome.runtime.sendMessage({ type: "guard_sync_now" }, function () {
        if (chrome.runtime.lastError) return;
      });
    } catch (e) { }
  }

  function armIfMonitored() {
    if (!chrome.storage || !chrome.storage.local) return;
    chrome.storage.local.get(SITE_KEYS.concat(CLIPBOARD_KEYS), function (res) {
      res = res || {};
      if (Array.isArray(res.cached_sites)) {
        monitoredSitesForPolicy = res.cached_sites;
        applyClipboardPolicyState(res);
      } else {
        // First run, nothing cached yet: background fills storage and onChanged arms us.
        requestBackgroundSync();
      }
      evaluateSite(res);
    });
  }

  // All updates arrive through storage written by background.js.
  if (chrome.storage && chrome.storage.onChanged) {
    chrome.storage.onChanged.addListener(function (changes, areaName) {
      if (areaName !== "local") return;
      var clipboardState = {};
      CLIPBOARD_KEYS.forEach(function (key) {
        if (changes[key]) clipboardState[key] = changes[key].newValue;
      });
      if (Object.keys(clipboardState).length) {
        clipboardState.cached_sites = clipboardState.cached_sites || monitoredSitesForPolicy;
        applyClipboardPolicyState(clipboardState);
      }
      var sitesChanged = SITE_KEYS.some(function (key) { return Boolean(changes[key]); });
      var clipChanged = CLIPBOARD_KEYS.some(function (key) { return Boolean(changes[key]); });
      if (sitesChanged || clipChanged) {
        var wasArmed = armedHere;
        chrome.storage.local.get(SITE_KEYS.concat(CLIPBOARD_KEYS), function (res) {
          evaluateSite(res);
          if (armedHere && POLICY_KEYS.some(function (key) { return Boolean(changes[key]); })) postPolicyToPage();
        });
        return;
      }
      if (armedHere && POLICY_KEYS.some(function (key) { return Boolean(changes[key]); })) {
        postPolicyToPage();
      }
    });
  }

  window.addEventListener("focus", function () {
    if (armedHere) requestBackgroundSync();
  });

  // On laptop wake/resume: tab becomes visible again → re-arm check immediately
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible") {
      // Re-evaluate arm status in case EXE restarted after sleep
      armIfMonitored();
    }
  });

  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard-page") return;
    var msg = ev.data;
    // Verify anti-tamper nonce
    if (!msg.nonce || msg.nonce !== guardNonce) return;

    if (msg.type === "authorize_ai_session") {
      var authPayload = {
        action: "authorize_ai_session",
        provider: msg.provider || "",
        email_domain: msg.email_domain || "",
        user_email: msg.user_email || "",
        workspace_id: msg.workspace_id || "",
        page_url: msg.page_url || location.href,
        policy_version: msg.policy_version || 1
      };
      function sendAuthResult(result) {
        window.postMessage({
          source: "gateway-guard",
          nonce: guardNonce,
          type: "session_authorization_result",
          id: msg.id,
          result: result || { decision: "block", reason_code: "GATEWAY_UNREACHABLE" }
        }, "*");
      }

      function fallbackGatewayDirect() {
        var endpoints = [];
        if (serverHost) {
          endpoints.unshift("https://" + serverHost + "/api/browser-ai/session/authorize");
        }
        var idx = 0;
        function tryNext() {
          if (idx >= endpoints.length) {
            sendAuthResult({
              decision: "block",
              reason_code: "GATEWAY_UNREACHABLE",
              message: "Gateway session verification unavailable; access is blocked until verification succeeds."
            });
            return;
          }
          var ep = endpoints[idx++];
          fetch(ep, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(authPayload)
          })
            .then(function (r) {
              if (!r.ok) throw new Error("status " + r.status);
              return r.json();
            })
            .then(function (res) {
              if (res && res.decision) {
                sendAuthResult(res);
              } else {
                tryNext();
              }
            })
            .catch(function () {
              tryNext();
            });
        }
        tryNext();
      }

      withBase(function () {
        fetch(base + "/extension/authorize_session", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Browser-Guard-Extension": "1" },
          body: JSON.stringify(authPayload)
        })
          .then(function (r) {
            if (!r.ok) throw new Error("agent endpoint " + r.status);
            return r.json();
          })
          .then(function (result) {
            if (result && result.decision) {
              sendAuthResult(result);
            } else {
              fallbackGatewayDirect();
            }
          })
          .catch(function () {
            fallbackGatewayDirect();
          });
      });
      return;
    }

    var hasContent = Boolean(
      (msg.prompt && msg.prompt.trim()) ||
      (Array.isArray(msg.files) && msg.files.length) ||
      (Array.isArray(msg.upload_images) && msg.upload_images.length)
    );
    if (!base || !hasContent) {
      window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      return;
    }

    var checkPayload = {
      id: msg.id || "",
      eval_id: msg.id || msg.eval_id || "",
      host: msg.host || location.hostname,
      url: msg.url || location.href,
      prompt: msg.prompt || "",
      files: msg.files || [],
      upload_images: msg.upload_images || [],
      is_blocked: Boolean(msg.is_blocked),
      rule: msg.rule || "",
      blocked_reason: msg.blocked_reason || msg.rule || "",
      browser: detectCurrentBrowser()
    };

    // Record prompt check in chrome.storage.local (middle-box persistence)
    if (chrome.storage && chrome.storage.local) {
      chrome.storage.local.set({
        last_prompt_check: {
          timestamp: Date.now(),
          host: checkPayload.host,
          prompt_preview: (checkPayload.prompt || "").slice(0, 100),
          file_count: checkPayload.files.length,
          image_count: checkPayload.upload_images.length
        }
      });
    }

    var reqHost = msg.host || location.hostname;
    function doCheck() {
      withBase(function () {
        fetch(base + "/extension/check", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Browser-Guard-Extension": "1" },
          body: JSON.stringify(checkPayload)
        })
          .then(function (r) { return r.json(); })
          .then(function (decision) {
            var dec = decision || { blocked: false };
            // Save EXE evaluation decision into chrome.storage.local
            if (chrome.storage && chrome.storage.local) {
              chrome.storage.local.set({
                last_decision: {
                  timestamp: Date.now(),
                  host: checkPayload.host,
                  blocked: Boolean(dec.blocked),
                  action: dec.action || (dec.blocked ? "Blocked" : "Allowed"),
                  rule: dec.rule || "",
                  message: dec.message || ""
                }
              });
            }
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: dec }, "*");
          })
          .catch(function () {
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
          });
      });
    }

    if (chrome.storage && chrome.storage.local) {
      chrome.storage.local.get(["cached_blocked_sites"], function (res) {
        var blockedList = (res && Array.isArray(res.cached_blocked_sites)) ? res.cached_blocked_sites : [];
        if (hostBlockedMatches(reqHost, blockedList)) {
          renderBlockedPage(reqHost);
          window.postMessage({
            source: "gateway-guard",
            nonce: guardNonce,
            type: "decision",
            id: msg.id,
            decision: {
              blocked: true,
              action: "Blocked",
              rule: "Site Blocked",
              message: "Access to this website is blocked by company security policy."
            }
          }, "*");
          return;
        }
        doCheck();
      });
    } else {
      doCheck();
    }
  });

  armIfMonitored();
})();
