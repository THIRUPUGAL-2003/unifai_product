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

  var serverHost = "";
  function isServerHost(host) {
    if (!host) return false;
    host = normalizeDomainHost(host);
    if (host === "localhost" || host === "127.0.0.1" || host === "0.0.0.0" || host === "::1") return true;
    if (host === "unifai.yespanchi.com" || host.endsWith(".unifai.yespanchi.com")) return true;
    if (serverHost) {
      var sHost = normalizeDomainHost(serverHost);
      if (host === sHost || host.endsWith("." + sHost)) return true;
    }
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
    var monitoredSites = clipboardPolicy.monitoredSites || monitoredSitesForPolicy;
    if (isServerHost(location.hostname) || !hostMatches(location.hostname, monitoredSites)) return false;
    var raw = String(clipboardPolicy.targetSites || "").trim();
    if (raw && raw.toLowerCase() !== "all" && raw !== "*") {
      return hostInConfiguredSites(location.hostname, raw);
    }
    return true;
  }

  var clipboardNoticeAt = 0;
  function showClipboardBlockedNotice() {
    var now = Date.now();
    if (now - clipboardNoticeAt < 1200) return;
    clipboardNoticeAt = now;
    try {
      var host = document.getElementById("yp-raksha-isolated-clipboard-notice");
      if (!host) {
        host = document.createElement("div");
        host.id = "yp-raksha-isolated-clipboard-notice";
        var shadow = host.attachShadow ? host.attachShadow({ mode: "closed" }) : host;
        var box = document.createElement("div");
        box.textContent = clipboardPolicy.message || "Copying and pasting are blocked by company security policy.";
        box.style.cssText = "position:fixed;top:20px;left:50%;transform:translateX(-50%);z-index:2147483647;background:#7f1d1d;color:white;padding:14px 22px;border-radius:10px;font:600 14px Segoe UI,Arial,sans-serif;box-shadow:0 8px 28px #0008;max-width:80vw;text-align:center;";
        shadow.appendChild(box);
        (document.documentElement || document).appendChild(host);
      }
      setTimeout(function () { try { host.remove(); } catch (e) {} }, 2600);
    } catch (e) {}
  }

  function blockClipboardEvent(ev) {
    if (!clipboardPolicyApplies()) return;
    ev.preventDefault();
    ev.stopImmediatePropagation();
    showClipboardBlockedNotice();
  }
  ["copy", "cut", "paste"].forEach(function (type) {
    window.addEventListener(type, blockClipboardEvent, true);
    document.addEventListener(type, blockClipboardEvent, true);
  });

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
  }

  if (chrome.storage && chrome.storage.local) {
    chrome.storage.local.get(["cached_sites", "block_clipboard", "clipboard_target_websites", "clipboard_block_message", "governance_block_message"], applyClipboardPolicyState);
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

  function syncRules() {
    withBase(function () {
      if (!base) return;
      fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
        .then(function (r) {
          if (!r.ok) throw new Error("rules");
          return r.json();
        })
        .then(function (data) {
          if (!data || !Array.isArray(data.rules)) return;
          var rules = data.rules;
          if (data.server_host) {
            serverHost = data.server_host;
          }
          var stored = {
            cached_rules: rules,
            server_host: serverHost,
            block_file_extension: Boolean(data.block_file_extension),
            file_extension_block_message: data.file_extension_block_message || "",
            file_extension_target_websites: data.file_extension_target_websites || "",
            block_upload: Boolean(data.block_upload),
            upload_warning: data.upload_warning || "",
            upload_target_websites: data.upload_target_websites || "",
            file_extension_mode: data.file_extension_mode || "block_listed",
            restricted_file_extensions: data.restricted_file_extensions || "",
            allowed_file_extensions: data.allowed_file_extensions || "",
            block_clipboard: Boolean(data.block_clipboard),
            block_drag_drop: Boolean(data.block_drag_drop),
            block_download: Boolean(data.block_download),
            governance_target_websites: data.governance_target_websites || "",
            governance_block_message: data.governance_block_message || "",
            clipboard_target_websites: data.clipboard_target_websites || "",
            drag_drop_target_websites: data.drag_drop_target_websites || "",
            download_target_websites: data.download_target_websites || "",
            clipboard_block_message: data.clipboard_block_message || "",
            drag_drop_block_message: data.drag_drop_block_message || "",
            download_block_message: data.download_block_message || "",
            block_screenshot: Boolean(data.block_screenshot),
            screenshot_block_message: data.screenshot_block_message || "",
            screenshot_target_websites: data.screenshot_target_websites || ""
          };
          var msg = {
            source: "gateway-guard",
            nonce: guardNonce,
            type: "sync_rules",
            rules: rules,
            block_upload: Boolean(data.block_upload),
            upload_warning: data.upload_warning || "",
            upload_target_websites: data.upload_target_websites || "",
            file_extension_mode: data.file_extension_mode || "block_listed",
            restricted_file_extensions: data.restricted_file_extensions || "",
            allowed_file_extensions: data.allowed_file_extensions || "",
            block_file_extension: Boolean(data.block_file_extension),
            file_extension_block_message: data.file_extension_block_message || "",
            file_extension_target_websites: data.file_extension_target_websites || "",
            block_clipboard: Boolean(data.block_clipboard),
            block_drag_drop: Boolean(data.block_drag_drop),
            block_download: Boolean(data.block_download),
            governance_target_websites: data.governance_target_websites || "",
            governance_block_message: data.governance_block_message || "",
            clipboard_target_websites: data.clipboard_target_websites || "",
            drag_drop_target_websites: data.drag_drop_target_websites || "",
            download_target_websites: data.download_target_websites || "",
            clipboard_block_message: data.clipboard_block_message || "",
            drag_drop_block_message: data.drag_drop_block_message || "",
            download_block_message: data.download_block_message || "",
            block_screenshot: Boolean(data.block_screenshot),
            screenshot_block_message: data.screenshot_block_message || "",
            screenshot_target_websites: data.screenshot_target_websites || ""
          };
          if (Array.isArray(data.restricted_extensions)) {
            stored.cached_exts = data.restricted_extensions;
            msg.restricted_extensions = data.restricted_extensions;
          }
          applyClipboardPolicyState(stored);
          if (data.uninstall_key_configured !== undefined) {
            stored.uninstall_key_configured = Boolean(data.uninstall_key_configured);
            stored.uninstall_key_rotated_at = data.uninstall_key_rotated_at || "";
          }
          if (chrome.storage && chrome.storage.local) {
            chrome.storage.local.set(stored);
          }
          window.postMessage(msg, "*");

          // Resilient fallback: if local agent EXE is an older build without governance fields, fetch directly from gateway
          if (data.block_clipboard === undefined && data.block_upload === undefined) {
            var gwUrls = [];
            if (serverHost) gwUrls.push("https://" + serverHost + "/api/browser-ai/rules?for=agent");
            var gwIdx = 0;
            function tryNextGw() {
              if (gwIdx >= gwUrls.length) return;
              fetch(gwUrls[gwIdx++], { method: "GET", cache: "no-store" })
                .then(function (gr) { return gr.json(); })
                .then(function (gdata) {
                  if (gdata && ("block_clipboard" in gdata || "block_upload" in gdata)) {
                    var governanceCache = {
                      block_clipboard: Boolean(gdata.block_clipboard),
                      clipboard_target_websites: gdata.clipboard_target_websites || "",
                      clipboard_block_message: gdata.clipboard_block_message || "",
                      governance_block_message: gdata.governance_block_message || "",
                      block_download: Boolean(gdata.block_download),
                      download_target_websites: gdata.download_target_websites || ""
                    };
                    applyClipboardPolicyState(Object.assign({ cached_sites: monitoredSitesForPolicy }, governanceCache));
                    if (chrome.storage && chrome.storage.local) chrome.storage.local.set(governanceCache);
                    var gmsg = {
                      source: "gateway-guard",
                      nonce: guardNonce,
                      type: "sync_rules",
                      rules: rules,
                      block_upload: Boolean(gdata.block_upload),
                      upload_warning: gdata.upload_warning || "",
                      upload_target_websites: gdata.upload_target_websites || "",
                      file_extension_mode: gdata.file_extension_mode || "block_listed",
                      restricted_file_extensions: gdata.restricted_file_extensions || "",
                      allowed_file_extensions: gdata.allowed_file_extensions || "",
                      block_file_extension: Boolean(gdata.block_file_extension),
                      file_extension_block_message: gdata.file_extension_block_message || "",
                      file_extension_target_websites: gdata.file_extension_target_websites || "",
                      block_clipboard: Boolean(gdata.block_clipboard),
                      block_drag_drop: Boolean(gdata.block_drag_drop),
                      block_download: Boolean(gdata.block_download),
                      governance_target_websites: gdata.governance_target_websites || "",
                      governance_block_message: gdata.governance_block_message || "",
                      clipboard_target_websites: gdata.clipboard_target_websites || "",
                      drag_drop_target_websites: gdata.drag_drop_target_websites || "",
                      download_target_websites: gdata.download_target_websites || "",
                      clipboard_block_message: gdata.clipboard_block_message || "",
                      drag_drop_block_message: gdata.drag_drop_block_message || "",
                      download_block_message: gdata.download_block_message || "",
                      block_screenshot: Boolean(gdata.block_screenshot),
                      screenshot_block_message: gdata.screenshot_block_message || "",
                      screenshot_target_websites: gdata.screenshot_target_websites || ""
                    };
                    if (chrome.storage && chrome.storage.local) {
                      chrome.storage.local.set(gmsg);
                    }
                    window.postMessage(gmsg, "*");
                  } else {
                    tryNextGw();
                  }
                })
                .catch(function () { tryNextGw(); });
            }
            tryNextGw();
          }
        })
        .catch(function () { });
    });
  }

  function armIfMonitored() {
    // 1. Check local cache first for zero-latency startup
    if (chrome.storage && chrome.storage.local) {
      chrome.storage.local.get([
        "cached_rules", "cached_sites", "cached_blocked_sites", "cached_exts", "server_host",
        "block_upload", "upload_warning", "upload_target_websites", "file_extension_mode", "restricted_file_extensions", "allowed_file_extensions",
        "block_file_extension", "file_extension_block_message", "file_extension_target_websites",
        "block_clipboard", "block_drag_drop", "block_download", "governance_target_websites", "governance_block_message",
        "clipboard_target_websites", "drag_drop_target_websites", "download_target_websites",
        "clipboard_block_message", "drag_drop_block_message", "download_block_message",
        "block_screenshot", "screenshot_block_message", "screenshot_target_websites"
      ], function (res) {
        if (Array.isArray(res && res.cached_sites)) {
          monitoredSitesForPolicy = res.cached_sites;
          applyClipboardPolicyState(res);
        }
        if (res && res.server_host) serverHost = res.server_host;
        if (isServerHost(location.hostname)) {
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
          return;
        }
        var blockedSites = (res && Array.isArray(res.cached_blocked_sites)) ? res.cached_blocked_sites : [];
        if (hostBlockedMatches(location.hostname, blockedSites)) {
          renderBlockedPage(location.hostname);
          return;
        }
        var sites = (res && Array.isArray(res.cached_sites)) ? res.cached_sites : [];
        if (hostMatches(location.hostname, sites)) {
          injectHook(function () {
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
            window.postMessage({
              source: "gateway-guard",
              nonce: guardNonce,
              type: "sync_rules",
              rules: Array.isArray(res.cached_rules) ? res.cached_rules : [],
              restricted_extensions: res.cached_exts || [],
              block_upload: Boolean(res && res.block_upload),
              upload_warning: (res && res.upload_warning) || "",
              upload_target_websites: (res && res.upload_target_websites) || "",
              file_extension_mode: (res && res.file_extension_mode) || "block_listed",
              restricted_file_extensions: (res && res.restricted_file_extensions) || "",
              allowed_file_extensions: (res && res.allowed_file_extensions) || "",
              block_file_extension: Boolean(res && res.block_file_extension),
              file_extension_block_message: (res && res.file_extension_block_message) || "",
              file_extension_target_websites: (res && res.file_extension_target_websites) || "",
              block_clipboard: Boolean(res && res.block_clipboard),
              block_drag_drop: Boolean(res && res.block_drag_drop),
              block_download: Boolean(res && res.block_download),
              governance_target_websites: (res && res.governance_target_websites) || "",
              governance_block_message: (res && res.governance_block_message) || "",
              clipboard_target_websites: (res && res.clipboard_target_websites) || "",
              drag_drop_target_websites: (res && res.drag_drop_target_websites) || "",
              download_target_websites: (res && res.download_target_websites) || "",
              clipboard_block_message: (res && res.clipboard_block_message) || "",
              drag_drop_block_message: (res && res.drag_drop_block_message) || "",
              download_block_message: (res && res.download_block_message) || "",
              block_screenshot: Boolean(res && res.block_screenshot),
              screenshot_block_message: (res && res.screenshot_block_message) || "",
              screenshot_target_websites: (res && res.screenshot_target_websites) || ""
            }, "*");
          });
        } else {
          // Domain is not monitored or was deleted: do not inject hook
          if (hookInjected) {
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
          }
        }
      });
    }

    // 2. Fetch live configuration from local agent
    withBase(function () {
      if (!base) return;
      fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
        .then(function (r) {
          if (!r.ok) throw new Error("sites");
          return r.json();
        })
        .then(function (data) {
          if (!data || !Array.isArray(data.hosts)) return;
          var domains = data.hosts || [];
          var blockedDomains = Array.isArray(data.blocked_hosts) ? data.blocked_hosts : [];
          if (data.server_host) {
            serverHost = data.server_host;
          }
          if (chrome.storage && chrome.storage.local) {
            chrome.storage.local.set({
              cached_sites: domains,
              cached_blocked_sites: blockedDomains,
              server_host: serverHost
            });
          }
          monitoredSitesForPolicy = domains;
          applyClipboardPolicyState({
            cached_sites: domains,
            block_clipboard: clipboardPolicy.enabled,
            clipboard_target_websites: clipboardPolicy.targetSites,
            clipboard_block_message: clipboardPolicy.message
          });
          if (hostBlockedMatches(location.hostname, blockedDomains)) {
            renderBlockedPage(location.hostname);
            return;
          }
          if (isServerHost(location.hostname) || !hostMatches(location.hostname, domains)) {
            // Server portal or domain not in target websites: disarm immediately
            if (hookInjected) {
              window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
            }
            return;
          }
          injectHook(function () {
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
            syncRules();
          });
        })
        .catch(function () { });
    });
  }

  // Listen for storage changes across background worker and tabs
  if (chrome.storage && chrome.storage.onChanged) {
    chrome.storage.onChanged.addListener(function (changes, areaName) {
      if (areaName !== "local") return;
      var clipboardState = {};
      ["cached_sites", "block_clipboard", "clipboard_target_websites", "clipboard_block_message", "governance_block_message"].forEach(function (key) {
        if (changes[key]) clipboardState[key] = changes[key].newValue;
      });
      if (Object.keys(clipboardState).length) {
        clipboardState.cached_sites = clipboardState.cached_sites || monitoredSitesForPolicy;
        applyClipboardPolicyState(clipboardState);
      }
      if (changes.cached_blocked_sites) {
        var newBlocked = Array.isArray(changes.cached_blocked_sites.newValue) ? changes.cached_blocked_sites.newValue : [];
        if (hostBlockedMatches(location.hostname, newBlocked)) {
          renderBlockedPage(location.hostname);
          return;
        }
      }
      if (changes.cached_rules) {
        var newRules = Array.isArray(changes.cached_rules.newValue) ? changes.cached_rules.newValue : [];
        var rulesMsg = {
          source: "gateway-guard",
          nonce: guardNonce,
          type: "sync_rules",
          rules: newRules
        };
        if (changes.cached_exts) {
          rulesMsg.restricted_extensions = changes.cached_exts.newValue || [];
        }
        window.postMessage(rulesMsg, "*");
      }
      if (changes.cached_sites) {
        var newSites = Array.isArray(changes.cached_sites.newValue) ? changes.cached_sites.newValue : [];
        if (hostMatches(location.hostname, newSites)) {
          injectHook(function () {
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
          });
        } else {
          // Domain was deleted from target sites list: disarm immediately
          if (hookInjected) {
            window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
          }
        }
      }
    });
  }

  // Periodic site refresh every 10 seconds and on tab focus
  setInterval(function () {
    armIfMonitored();
  }, 10000);
  window.addEventListener("focus", function () {
    armIfMonitored();
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
          headers: { "Content-Type": "application/json" },
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
          headers: { "Content-Type": "application/json" },
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
