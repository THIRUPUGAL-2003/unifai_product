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
      try {
        host = new URL(host).hostname;
      } catch (_) {
        host = host.split("://", 2)[1] || host;
      }
    }
    host = host.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0];
    if ((host.match(/:/g) || []).length === 1 && /:\d+$/.test(host)) {
      host = host.replace(/:\d+$/, "");
    }
    if (host.charAt(0) === "[" && host.indexOf("]") >= 0) {
      host = host.slice(1, host.indexOf("]"));
    }
    return host.replace(/^[*@.]+/g, "").replace(/^www\./, "").replace(/^[*@.]+/g, "").replace(/\.+$/, "");
  }

  function isLoopbackHost(host) {
    return host === "localhost" || host === "127.0.0.1" || host === "0.0.0.0" || host === "::1" ||
      host.startsWith("127.") || host.endsWith(".localhost");
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

  function isExtensionValid() {
    try {
      if (typeof chrome === "undefined" || !chrome.runtime || !chrome.runtime.id) {
        return false;
      }
      return true;
    } catch (_) {
      return false;
    }
  }

  // Intercept and cleanly suppress harmless orphan context errors when extension is updated/reloaded
  if (typeof window !== "undefined") {
    window.addEventListener("error", function (e) {
      var msg = (e && (e.message || (e.error && e.error.message))) || "";
      if (typeof msg === "string" && msg.indexOf("Extension context invalidated") !== -1) {
        try {
          if (e.preventDefault) e.preventDefault();
          if (e.stopPropagation) e.stopPropagation();
          if (e.stopImmediatePropagation) e.stopImmediatePropagation();
        } catch (_) {}
        return true;
      }
    }, true);
    window.addEventListener("unhandledrejection", function (e) {
      var reason = (e && (e.reason && (e.reason.message || e.reason))) || "";
      if (typeof reason === "string" && reason.indexOf("Extension context invalidated") !== -1) {
        try {
          if (e.preventDefault) e.preventDefault();
        } catch (_) {}
      }
    }, true);
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
    if (matchDomainInMap(host, sitesMap, domains)) return true;
    if (Array.isArray(domains) && domains.length > 0) {
      return hostInConfiguredSites(host, domains.join(","));
    }
    return false;
  }

  function hostBlockedMatches(host, blockedDomains) {
    if (!blockedSitesMap || (Array.isArray(blockedDomains) && blockedDomains !== lastBlockedSitesList)) {
      lastBlockedSitesList = blockedDomains;
      blockedSitesMap = buildDomainMap(blockedDomains);
    }
    if (matchDomainInMap(host, blockedSitesMap, blockedDomains)) return true;
    if (Array.isArray(blockedDomains) && blockedDomains.length > 0) {
      return hostInConfiguredSites(host, blockedDomains.join(","));
    }
    return false;
  }

  function getRootDomain(host) {
    if (!host) return "";
    var clean = normalizeDomainHost(host).toLowerCase();
    var parts = clean.split(".");
    if (parts.length <= 2) return clean;
    var twoPartTlds = ["co.uk", "org.uk", "com.au", "net.au", "co.in", "co.jp", "com.br", "gov.in"];
    var lastTwo = parts.slice(-2).join(".");
    if (twoPartTlds.indexOf(lastTwo) !== -1 && parts.length > 2) {
      return parts.slice(-3).join(".");
    }
    return parts.slice(-2).join(".");
  }

  function areRelatedAIHosts(h1, h2) {
    if (!h1 || !h2) return false;
    h1 = normalizeDomainHost(h1);
    h2 = normalizeDomainHost(h2);
    if (h1 === h2) return true;
    var r1 = getRootDomain(h1);
    var r2 = getRootDomain(h2);
    if (r1 && r2 && r1 === r2) return true;

    var pairs = [
      ["chatgpt.com", "openai.com"],
      ["chat.openai.com", "chatgpt.com"],
      ["ab.chatgpt.com", "chatgpt.com"],
      ["claude.ai", "anthropic.com"],
      ["gemini.google.com", "aistudio.google.com"],
      ["gemini.google.com", "bard.google.com"],
      ["gemini.google.com", "google.com"],
      ["copilot.microsoft.com", "copilot.com"],
      ["copilot.microsoft.com", "copilot.cloud.microsoft"],
      ["copilot.microsoft.com", "bing.com"],
      ["copilot.microsoft.com", "githubcopilot.com"],
      ["deepseek.com", "chat.deepseek.com"],
      ["mistral.ai", "chat.mistral.ai"],
      ["perplexity.ai", "www.perplexity.ai"],
      ["grok.com", "x.ai"],
      ["lovable.dev", "lovable.app"],
      ["lovable.dev", "app.lovable.dev"],
      ["poe.com", "quora.com"]
    ];
    for (var i = 0; i < pairs.length; i++) {
      var p0 = pairs[i][0];
      var p1 = pairs[i][1];
      if ((h1 === p0 || h1.endsWith("." + p0)) && (h2 === p1 || h2.endsWith("." + p1))) return true;
      if ((h1 === p1 || h1.endsWith("." + p1)) && (h2 === p0 || h2.endsWith("." + p0))) return true;
    }
    return false;
  }

  function detectProviderKeyword(host) {
    var h = (host || "").toLowerCase();
    if (h.includes("chatgpt") || h.includes("openai")) return "chatgpt";
    if (h.includes("claude") || h.includes("anthropic")) return "claude";
    if (h.includes("copilot") || h.includes("bing.com")) return "copilot";
    if (h.includes("gemini") || h.includes("bard") || h.includes("aistudio")) return "gemini";
    if (h.includes("deepseek")) return "deepseek";
    if (h.includes("mistral")) return "mistral";
    if (h.includes("perplexity")) return "perplexity";
    if (h.includes("grok") || h.includes("x.ai")) return "grok";
    if (h.includes("lovable")) return "lovable";
    if (h.includes("poe.com")) return "poe";
    return "";
  }

  function hostInConfiguredSites(host, rawSites) {
    var raw = String(rawSites || "").trim();
    if (!raw) return false;
    if (raw.toUpperCase() === "ALL" || raw === "*") return true;
    if (raw.toLowerCase() === "none") return false;
    var current = normalizeDomainHost(host);
    var currentProv = detectProviderKeyword(current);
    var parts = raw.split(/[;,\s]+/);
    for (var i = 0; i < parts.length; i++) {
      var item = parts[i].trim().replace(/^\*\./, "").replace(/^\./, "");
      if (!item) continue;
      if (currentProv && item.toLowerCase() === currentProv) return true;
      var itemHost = normalizeDomainHost(item.indexOf("://") >= 0 ? item : item.indexOf(":") >= 0 && (item.match(/:/g) || []).length > 1 ? "https://[" + item + "]" : "https://" + item);
      if (!itemHost) {
        if (currentProv && item.toLowerCase() === currentProv) return true;
        continue;
      }
      if (current === itemHost || current.endsWith("." + itemHost) || itemHost.endsWith("." + current)) return true;
      if (!itemHost.includes(".") && current.includes(itemHost)) return true;
      var curR = getRootDomain(current);
      var itmR = getRootDomain(itemHost);
      if (curR && itmR && curR === itmR) return true;
      if (areRelatedAIHosts(current, itemHost)) return true;
    }
    return false;
  }

  function clipboardPolicyApplies() {
    if (!isExtensionValid()) return false;
    if (!clipboardPolicy.enabled) return false;
    if (isServerHost(location.hostname)) return false;
    var raw = String(clipboardPolicy.targetSites || "").trim();
    if (!raw || raw.toLowerCase() === "none") return false;
    if (raw.toUpperCase() === "ALL" || raw === "*") {
      var monitoredSites = clipboardPolicy.monitoredSites || monitoredSitesForPolicy;
      if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
        return hostMatches(location.hostname, monitoredSites);
      }
      return false;
    }
    return hostInConfiguredSites(location.hostname, raw);
  }

  function cleanNoticeMessage(msg, fallback) {
    var s = (msg || "").trim();
    if (!s) return fallback || "Action restricted by corporate security policy.";
    s = s.replace(/^\[?custom\]?\s*[:\-–—]?\s*/i, "");
    if (/^drag\s*(&|and)?\s*drop\s*block\s*active$/i.test(s) || /^drag\s*(&|and)?\s*drop\s*restricted$/i.test(s)) {
      return "File drag and drop is restricted by corporate security policy.";
    }
    if (/^clipboard\s*protection\s*active$/i.test(s) || /^copy\s*paste\s*not\s*allowed$/i.test(s)) {
      return "Clipboard copy-paste into AI tools is restricted by corporate security policy.";
    }
    if (/^download\s*block\s*active$/i.test(s)) {
      return "Downloading files, artifacts, and exports from this AI tool is restricted by corporate security policy.";
    }
    if (/^upload\s*block\s*active$/i.test(s) || /^file\s*upload\s*restricted$/i.test(s) || /^upload\s*restricted$/i.test(s)) {
      return "File uploads and attachments to this AI platform are blocked by corporate policy.";
    }
    if (/^screenshot\s*block\s*active$/i.test(s)) {
      return "Screen capture and recording is restricted on this AI workspace by corporate security policy.";
    }
    if (!s || /^block active$/i.test(s) || /^protection active$/i.test(s)) {
      return fallback || "Action restricted by corporate security policy.";
    }
    return s;
  }

  var clipboardNoticeAt = 0;
  function showClipboardBlockedNotice(customTitle, customMessage) {
    var now = Date.now();
    if (now - clipboardNoticeAt < 1000) return;
    clipboardNoticeAt = now;
    if (document.getElementById("yp-raksha-alert-card") || document.getElementById("yp-screenshot-shield") || document.getElementById("yp-blur-curtain")) return;
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
      var heading = customTitle || "Clipboard Restricted";
      var bodyMsg = customMessage || cleanNoticeMessage(clipboardPolicy.message, "Clipboard copy-paste into or from AI tools is restricted by corporate security policy.");
      box.innerHTML =
        '<div style="display:flex;align-items:center;gap:6px;margin-bottom:3px;">' +
          '<span style="font-size:13px;line-height:1;">🛡️</span>' +
          '<strong style="font-size:11px;font-weight:700;letter-spacing:0.03em;text-transform:uppercase;color:#f87171;">' + heading + '</strong>' +
        '</div>' +
        '<div style="font-size:12.5px;line-height:1.4;color:#e2e8f0;font-weight:400;">' +
          bodyMsg +
        '</div>';
      shadow.appendChild(box);
      (document.documentElement || document).appendChild(host);
      setTimeout(function () {
        try {
          box.style.opacity = "0";
          setTimeout(function () { try { host.remove(); } catch (_) {} }, 150);
        } catch (e) {}
      }, 3000);
    } catch (e) {}
  }

  var _lastGovLogSig = "";
  var _lastGovLogTime = 0;
  var _lastClipboardBlockLoggedAt = 0;
  var _lastDragDropBlockLoggedAt = 0;
  var _lastScreenshotBlockLoggedAt = 0;
  var _lastDownloadBlockLoggedAt = 0;
  function recordIsolatedGovernanceLog(promptText, ruleName) {
    if (!isExtensionValid()) return;
    var now = Date.now();
    if (ruleName === "Clipboard Policy" || (promptText && promptText.indexOf("--copy paste") === 0)) {
      if (now - _lastClipboardBlockLoggedAt < 2000) return;
      _lastClipboardBlockLoggedAt = now;
    }
    if (ruleName === "Drag & Drop Policy" || (promptText && promptText.indexOf("--drag drop") === 0)) {
      if (now - _lastDragDropBlockLoggedAt < 2000) return;
      _lastDragDropBlockLoggedAt = now;
    }
    if (ruleName === "Screen Capture Policy" || (promptText && (promptText.indexOf("--screenshot") === 0 || promptText.indexOf("[SCREEN CAPTURE") === 0))) {
      if (now - _lastScreenshotBlockLoggedAt < 2000) return;
      _lastScreenshotBlockLoggedAt = now;
    }
    if (ruleName === "Download Policy" || (promptText && promptText.indexOf("--download") === 0)) {
      if (now - _lastDownloadBlockLoggedAt < 2000) return;
      _lastDownloadBlockLoggedAt = now;
    }
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

  function enforceActiveElementRollback() {
    try {
      var activeEl = document.activeElement;
      if (!activeEl) return;
      var isInput = activeEl.tagName === "INPUT" || activeEl.tagName === "TEXTAREA";
      var isContentEditable = activeEl.isContentEditable || activeEl.getAttribute("contenteditable") === "true";
      if (!isInput && !isContentEditable) {
        var editableParent = activeEl.closest ? activeEl.closest("[contenteditable='true'], textarea, input, [role='textbox']") : null;
        if (editableParent) {
          activeEl = editableParent;
          isInput = activeEl.tagName === "INPUT" || activeEl.tagName === "TEXTAREA";
          isContentEditable = true;
        }
      }
      if (!isInput && !isContentEditable) return;

      var prevVal = isInput ? activeEl.value : activeEl.innerHTML;
      var prevText = isInput ? activeEl.value : (activeEl.innerText || activeEl.textContent || "");

      var delays = [0, 4, 16, 40, 100];
      for (var d = 0; d < delays.length; d++) {
        (function (delay) {
          setTimeout(function () {
            try {
              if (isInput) {
                if (activeEl.value !== prevVal) {
                  activeEl.value = prevVal;
                  activeEl.dispatchEvent(new Event("input", { bubbles: true }));
                }
              } else if (isContentEditable) {
                var currText = activeEl.innerText || activeEl.textContent || "";
                if (currText !== prevText) {
                  try { document.execCommand("undo", false, null); } catch (_) {}
                  var postUndoText = activeEl.innerText || activeEl.textContent || "";
                  if (postUndoText !== prevText) {
                    activeEl.innerHTML = prevVal;
                    try {
                      activeEl.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "deleteContentBackward" }));
                    } catch (_) {
                      activeEl.dispatchEvent(new Event("input", { bubbles: true }));
                    }
                  }
                }
              }
            } catch (_) {}
          }, delay);
        })(delays[d]);
      }
    } catch (_) {}
  }

  function blockClipboardEvent(ev) {
    if (!isExtensionValid()) return;
    if (!clipboardPolicyApplies()) return;
    if (document.getElementById("yp-blur-curtain") || document.getElementById("yp-screenshot-shield")) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      return;
    }
    var type = (ev.type || "").toLowerCase();
    var isPaste = type.indexOf("paste") >= 0;
    var isCopy = type.indexOf("copy") >= 0 || type.indexOf("cut") >= 0;
    if (!isPaste && !isCopy) return;

    if (isCopy && ev.clipboardData) {
      try {
        ev.clipboardData.clearData();
        ev.clipboardData.setData("text/plain", cleanNoticeMessage(clipboardPolicy.message, "[Clipboard copying from this AI tool is restricted by corporate security policy.]"));
        ev.clipboardData.setData("text/html", "");
      } catch (_) {}
    }
    try {
      var sel = window.getSelection();
      if (sel) {
        if (sel.removeAllRanges) sel.removeAllRanges();
        if (sel.empty) sel.empty();
      }
      if (document.activeElement && typeof document.activeElement.selectionStart === "number") {
        document.activeElement.selectionEnd = document.activeElement.selectionStart;
      }
    } catch (_) {}
    try {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
    } catch (_) {}
    if (isPaste) {
      enforceActiveElementRollback();
    }
    if (!document.getElementById("yp-raksha-alert-card")) {
      if (isCopy) {
        showClipboardBlockedNotice("Clipboard Copy Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard copying from this AI tool is restricted by corporate security policy."));
      } else {
        showClipboardBlockedNotice("Clipboard Paste Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard pasting into this AI tool is restricted by corporate security policy."));
      }
    }
    var logPrompt = isCopy ? "--copy paste [CLIPBOARD COPY]" : "--copy paste [CLIPBOARD PASTE]";
    recordIsolatedGovernanceLog(logPrompt, "Clipboard Policy");
  }

  function handleBeforeInputEvent(ev) {
    if (!isExtensionValid()) return;
    if (!clipboardPolicyApplies()) return;
    if (document.getElementById("yp-blur-curtain") || document.getElementById("yp-screenshot-shield")) return;
    var inputType = (ev.inputType || "").toLowerCase();
    var isPasteInput =
      inputType === "insertfrompaste" ||
      inputType === "insertfrompasteasquotation" ||
      inputType === "insertreplacementtext" ||
      inputType === "insertfromdrop" ||
      inputType === "insertfromyank" ||
      inputType.indexOf("paste") >= 0;

    if (isPasteInput) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      enforceActiveElementRollback();
      if (!document.getElementById("yp-raksha-alert-card")) {
        showClipboardBlockedNotice("Clipboard Paste Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard pasting into this AI tool is restricted by corporate security policy."));
      }
      recordIsolatedGovernanceLog("--copy paste [CLIPBOARD PASTE]", "Clipboard Policy");
    }
  }

  function handleClipboardKeys(e) {
    if (!isExtensionValid()) return;
    if (!clipboardPolicyApplies()) return;
    if (document.getElementById("yp-blur-curtain") || document.getElementById("yp-screenshot-shield")) return;
    var isCtrlOrMeta = e.ctrlKey || e.metaKey;
    var isPaste = (isCtrlOrMeta && (e.key === "v" || e.key === "V" || e.code === "KeyV")) ||
                  (e.shiftKey && (e.key === "Insert" || e.code === "Insert"));
    var isCopy = (isCtrlOrMeta && (e.key === "c" || e.key === "C" || e.code === "KeyC" || e.key === "x" || e.key === "X" || e.code === "KeyX")) ||
                 (isCtrlOrMeta && (e.key === "Insert" || e.code === "Insert"));

    if (isPaste || isCopy) {
      try {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      } catch (_) {}
      if (isPaste) {
        enforceActiveElementRollback();
      }
      if (isCopy) {
        try {
          var sel = window.getSelection();
          if (sel) {
            if (sel.removeAllRanges) sel.removeAllRanges();
            if (sel.empty) sel.empty();
          }
          if (document.activeElement && typeof document.activeElement.selectionStart === "number") {
            document.activeElement.selectionEnd = document.activeElement.selectionStart;
          }
        } catch (_) {}
      }
      if (!document.getElementById("yp-raksha-alert-card")) {
        if (isCopy) {
          showClipboardBlockedNotice("Clipboard Copy Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard copying from this AI tool is restricted by corporate security policy."));
        } else {
          showClipboardBlockedNotice("Clipboard Paste Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard pasting into this AI tool is restricted by corporate security policy."));
        }
      }
      var logPrompt = isCopy ? "--copy paste [CLIPBOARD COPY KEY]" : "--copy paste [CLIPBOARD PASTE KEY]";
      recordIsolatedGovernanceLog(logPrompt, "Clipboard Policy");
    }
  }

  // Prevent right-click copy on selected text
  function handleContextMenuEvent(ev) {
    if (!isExtensionValid()) return;
    if (!clipboardPolicyApplies()) return;
    try {
      var sel = window.getSelection();
      var selectedText = sel ? sel.toString().trim() : "";
      if (selectedText.length > 0) {
        if (ev && ev.preventDefault) ev.preventDefault();
        if (sel.removeAllRanges) sel.removeAllRanges();
        if (sel.empty) sel.empty();
        if (!document.getElementById("yp-raksha-alert-card")) {
          showClipboardBlockedNotice("Clipboard Copy Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard copying from this AI tool is restricted by corporate security policy."));
        }
        recordIsolatedGovernanceLog("--copy paste [CLIPBOARD COPY CONTEXT MENU]", "Clipboard Policy");
      }
    } catch (_) {}
  }

  // Prevent selection of AI output text when clipboard policy is active
  function handleSelectStartEvent(ev) {
    if (!isExtensionValid()) return;
    if (!clipboardPolicyApplies()) return;
    var t = ev.target;
    var tag = (t && t.tagName || "").toLowerCase();
    var isEditable = tag === "input" || tag === "textarea" || (t && t.isContentEditable) || (t && t.closest && t.closest("input, textarea, [contenteditable='true']"));
    if (!isEditable) {
      try {
        ev.preventDefault();
      } catch (_) {}
      try {
        var sel = window.getSelection();
        if (sel && sel.removeAllRanges) sel.removeAllRanges();
      } catch (_) {}
    }
  }

  ["paste", "beforepaste", "copy", "beforecopy", "cut", "beforecut"].forEach(function (type) {
    window.addEventListener(type, blockClipboardEvent, true);
    document.addEventListener(type, blockClipboardEvent, true);
  });
  window.addEventListener("beforeinput", handleBeforeInputEvent, true);
  document.addEventListener("beforeinput", handleBeforeInputEvent, true);
  window.addEventListener("keydown", handleClipboardKeys, true);
  document.addEventListener("keydown", handleClipboardKeys, true);
  window.addEventListener("contextmenu", handleContextMenuEvent, true);
  document.addEventListener("contextmenu", handleContextMenuEvent, true);
  document.addEventListener("selectstart", handleSelectStartEvent, true);

  // Intercept clicks, mousedown, pointerdown on AI "Copy" and "Copy code" buttons in isolated world
  function handleCopyButtonIsolated(ev) {
    if (!isExtensionValid()) return;
    if (!clipboardPolicyApplies()) return;
    var target = (ev.composedPath && ev.composedPath()[0]) || ev.target;
    var el = (target && target.closest) ? target.closest("button, [role='button'], a, [aria-label*='copy' i], [title*='copy' i], [data-testid*='copy' i], [class*='copy' i], [id*='copy' i], [data-tooltip*='copy' i]") : null;
    if (!el && target) {
      var curr = target;
      for (var depth = 0; curr && depth < 5; depth++) {
        var a = ((curr.getAttribute && curr.getAttribute("aria-label")) || "").toLowerCase();
        var ti = ((curr.getAttribute && curr.getAttribute("title")) || "").toLowerCase();
        var cl = String(curr.className || "").toLowerCase();
        if (a.indexOf("copy") >= 0 || ti.indexOf("copy") >= 0 || cl.indexOf("copy") >= 0) {
          el = curr;
          break;
        }
        curr = curr.parentElement;
      }
    }
    if (!el) return;
    var aria = (el.getAttribute("aria-label") || "").toLowerCase();
    var title = (el.getAttribute("title") || "").toLowerCase();
    var testid = (el.getAttribute("data-testid") || "").toLowerCase();
    var tooltip = (el.getAttribute("data-tooltip") || el.getAttribute("data-tooltip-id") || el.getAttribute("mattooltip") || "").toLowerCase();
    var biId = (el.getAttribute("data-bi-id") || "").toLowerCase();
    var className = String(el.className || "").toLowerCase();
    var text = (el.innerText || el.textContent || "").toLowerCase().trim();
    var hasCopySvg = !!(el.querySelector && el.querySelector("svg[class*='copy' i], svg[aria-label*='copy' i], [class*='copy-icon' i], [class*='icon-copy' i]"));
    var isCopyBtn =
      aria.includes("copy") ||
      title.includes("copy") ||
      testid.includes("copy") ||
      tooltip.includes("copy") ||
      biId.includes("copy") ||
      className.includes("copy") ||
      hasCopySvg ||
      text === "copy" ||
      text === "copy code" ||
      text === "copy text" ||
      text === "copy response" ||
      text === "copy prompt" ||
      text === "copy to clipboard" ||
      text.startsWith("copy ");
    if (isCopyBtn) {
      try {
        ev.preventDefault();
        ev.stopPropagation();
        ev.stopImmediatePropagation();
      } catch (_) {}
      try {
        if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
          navigator.clipboard.writeText("").catch(function () {});
        }
      } catch (_) {}
      try {
        var sel = window.getSelection();
        if (sel) {
          if (sel.removeAllRanges) sel.removeAllRanges();
          if (sel.empty) sel.empty();
        }
      } catch (_) {}
      if (!document.getElementById("yp-raksha-alert-card")) {
        showClipboardBlockedNotice("Clipboard Copy Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard copying from this AI tool is restricted by corporate security policy."));
      }
      recordIsolatedGovernanceLog("--copy paste [CLIPBOARD COPY BUTTON]", "Clipboard Policy");
    }
  }

  ["pointerdown", "mousedown", "click"].forEach(function (evt) {
    document.addEventListener(evt, handleCopyButtonIsolated, true);
    window.addEventListener(evt, handleCopyButtonIsolated, true);
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
    applyScreenshotPolicyState(state);
    applyDragDropPolicyState(state);
    applyCorporatePolicyState(state);
  }

  var corporatePolicy = { enabled: false, targetSites: "", emailDomains: "", blockMessage: "", monitoredSites: [] };
  function corporatePolicyApplies() {
    if (!isExtensionValid()) return false;
    if (!corporatePolicy.enabled) return false;
    if (isServerHost(location.hostname)) return false;
    var raw = String(corporatePolicy.targetSites || "").trim();
    if (!raw || raw.toLowerCase() === "none") {
      var monitoredSites = corporatePolicy.monitoredSites || monitoredSitesForPolicy;
      if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
        return hostMatches(location.hostname, monitoredSites);
      }
      return false;
    }
    if (raw.toUpperCase() === "ALL" || raw === "*") {
      var monitoredSites = corporatePolicy.monitoredSites || monitoredSitesForPolicy;
      if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
        return hostMatches(location.hostname, monitoredSites);
      }
      return false;
    }
    return hostInConfiguredSites(location.hostname, raw);
  }

  function applyCorporatePolicyState(state) {
    state = state || {};
    corporatePolicy = {
      enabled: typeof state.enforce_corporate_domains === "boolean" ? state.enforce_corporate_domains : corporatePolicy.enabled,
      targetSites: typeof state.corporate_target_websites === "string" ? state.corporate_target_websites : corporatePolicy.targetSites,
      emailDomains: typeof state.corporate_email_domains === "string" ? state.corporate_email_domains : corporatePolicy.emailDomains,
      blockMessage: typeof state.corporate_block_message === "string"
        ? state.corporate_block_message
        : (typeof state.governance_block_message === "string" ? state.governance_block_message : corporatePolicy.blockMessage),
      monitoredSites: monitoredSitesForPolicy
    };
  }

  var dragDropPolicy = { enabled: false, targetSites: "", message: "", monitoredSites: [] };
  function dragDropPolicyApplies() {
    if (!isExtensionValid()) return false;
    if (!dragDropPolicy.enabled) return false;
    if (isServerHost(location.hostname)) return false;
    var raw = String(dragDropPolicy.targetSites || "").trim();
    if (!raw || raw.toLowerCase() === "none") return false;
    if (raw.toUpperCase() === "ALL" || raw === "*") {
      var monitoredSites = dragDropPolicy.monitoredSites || monitoredSitesForPolicy;
      if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
        return hostMatches(location.hostname, monitoredSites);
      }
      return false;
    }
    return hostInConfiguredSites(location.hostname, raw);
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
    if (now - dragDropNoticeAt < 1000) return;
    dragDropNoticeAt = now;
    if (document.getElementById("yp-raksha-alert-card") || document.getElementById("yp-screenshot-shield") || document.getElementById("yp-blur-curtain")) return;
    try {
      var old = document.getElementById("yp-raksha-isolated-dragdrop-notice");
      if (old) old.remove();
      var host = document.createElement("div");
      host.id = "yp-raksha-isolated-dragdrop-notice";
      var shadow = host.attachShadow ? host.attachShadow({ mode: "closed" }) : host;
      var box = document.createElement("div");
      box.style.cssText =
        "position:fixed;bottom:95px;left:50%;transform:translateX(-50%);max-width:460px;width:fit-content;min-width:280px;" +
        "z-index:2147483647;background:rgba(15,23,42,0.96);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);" +
        "color:#f9fafb;border:1px solid #ef4444;border-radius:12px;padding:9px 14px;box-shadow:0 10px 30px rgba(0,0,0,0.5);" +
        "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;transition:opacity 0.2s ease;pointer-events:none;";
      var bodyMsg = cleanNoticeMessage(dragDropPolicy.message, "File drag and drop is restricted by corporate security policy.");
      box.innerHTML =
        '<div style="display:flex;align-items:center;gap:6px;margin-bottom:3px;">' +
          '<span style="font-size:13px;line-height:1;">🛡️</span>' +
          '<strong style="font-size:11px;font-weight:700;letter-spacing:0.03em;text-transform:uppercase;color:#f87171;">Drag & Drop Restricted</strong>' +
        '</div>' +
        '<div style="font-size:12.5px;line-height:1.4;color:#e2e8f0;font-weight:400;">' +
          bodyMsg +
        '</div>';
      shadow.appendChild(box);
      (document.documentElement || document).appendChild(host);
      setTimeout(function () {
        try {
          box.style.opacity = "0";
          setTimeout(function () { try { host.remove(); } catch (_) {} }, 150);
        } catch (e) {}
      }, 3000);
    } catch (e) {}
  }

  function blockDragDropEvent(ev) {
    if (!isExtensionValid()) return;
    if (!dragDropPolicyApplies()) return;
    var dt = ev.dataTransfer;
    var hasFiles = dt && dt.files && dt.files.length > 0;
    var hasItems = dt && dt.items && dt.items.length > 0;
    var droppedSummary = "";
    if (dt) {
      var names = [];
      if (dt.files && dt.files.length > 0) {
        for (var i = 0; i < dt.files.length; i++) {
          if (dt.files[i] && dt.files[i].name) names.push(dt.files[i].name);
        }
      }
      if (!names.length && dt.items && dt.items.length > 0) {
        for (var j = 0; j < dt.items.length; j++) {
          var item = dt.items[j];
          if (item && item.kind === "file") {
            try {
              var f = typeof item.getAsFile === "function" ? item.getAsFile() : null;
              if (f && f.name) names.push(f.name);
            } catch (_) {}
          }
        }
      }
      if (names.length) {
        droppedSummary = "Files: " + names.join(", ");
      } else {
        try {
          var t = dt.getData("text/plain") || dt.getData("text/uri-list");
          if (t) droppedSummary = "Text: " + t.trim().slice(0, 300);
        } catch (_) {}
      }
    }
    if (!hasFiles && !hasItems && !droppedSummary && ev.type !== "drop") return;
    if (dt) {
      dt.dropEffect = "none";
      try { dt.clearData(); } catch (_) {}
    }
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation();
    if (ev.type === "drop") {
      if (!document.getElementById("yp-raksha-alert-card")) {
        showDragDropBlockedNotice();
      }
      var sum = droppedSummary || (hasFiles ? "Files" : (hasItems ? "Items" : "Data"));
      recordIsolatedGovernanceLog("--drag drop [DRAG & DROP BLOCKED] " + sum, "Drag & Drop Policy");
    }
  }

  ["dragenter", "dragover", "drop"].forEach(function (type) {
    window.addEventListener(type, blockDragDropEvent, true);
    document.addEventListener(type, blockDragDropEvent, true);
  });

  var screenshotPolicy = { enabled: false, targetSites: "", message: "", monitoredSites: [] };
  function screenshotPolicyApplies() {
    if (!isExtensionValid()) return false;
    if (!screenshotPolicy.enabled) return false;
    if (isServerHost(location.hostname)) return false;
    var raw = String(screenshotPolicy.targetSites || "").trim();
    if (!raw || raw.toLowerCase() === "none") return false;
    if (raw.toUpperCase() === "ALL" || raw === "*") {
      var monitoredSites = screenshotPolicy.monitoredSites || monitoredSitesForPolicy;
      if (Array.isArray(monitoredSites) && monitoredSites.length > 0) {
        return hostMatches(location.hostname, monitoredSites);
      }
      return false;
    }
    return hostInConfiguredSites(location.hostname, raw);
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

  function dismissAllClipboardNotices() {
    _lastClipboardBlockLoggedAt = Date.now();
    _lastDragDropBlockLoggedAt = Date.now();
    try {
      var oldBubble = document.getElementById("yp-raksha-alert-card");
      if (oldBubble) oldBubble.remove();
    } catch (_) {}
    try {
      var clipNotice = document.getElementById("yp-raksha-isolated-clipboard-notice");
      if (clipNotice) clipNotice.remove();
    } catch (_) {}
    try {
      var dragNotice = document.getElementById("yp-raksha-isolated-dragdrop-notice");
      if (dragNotice) dragNotice.remove();
    } catch (_) {}
  }

  function checkDismissIsolatedNotice(e) {
    var clip = document.getElementById("yp-raksha-isolated-clipboard-notice");
    var drag = document.getElementById("yp-raksha-isolated-dragdrop-notice");
    if (!clip && !drag) return;
    var now = Date.now();
    // Do not dismiss fresh notices within 2.5s grace period
    if (now - clipboardNoticeAt < 2500 || now - dragDropNoticeAt < 2500) return;
    var target = (e && e.target) || document.activeElement;
    if (target) {
      var val = typeof target.value === "string" ? target.value.trim() : "";
      var text = (target.isContentEditable || (target.getAttribute && target.getAttribute("contenteditable") === "true"))
        ? (target.innerText || target.textContent || "").trim()
        : "";
      if (!val && !text) {
        dismissAllClipboardNotices();
      }
    }
  }

  ["input", "change"].forEach(function (evName) {
    document.addEventListener(evName, checkDismissIsolatedNotice, true);
    window.addEventListener(evName, checkDismissIsolatedNotice, true);
  });

  function applyIsolatedCurtain() {
    if (!isExtensionValid()) return;
    if (!screenshotPolicyApplies()) return;
    dismissAllClipboardNotices();
    var curtain = document.getElementById("yp-blur-curtain");
    if (!curtain) {
      curtain = document.createElement("div");
      curtain.id = "yp-blur-curtain";
      curtain.style.cssText = "position:fixed;inset:0;width:100vw;height:100vh;background:#0a0f1d;opacity:0.98;backdrop-filter:blur(30px);-webkit-backdrop-filter:blur(30px);z-index:2147483646;pointer-events:none;display:flex;align-items:center;justify-content:center;color:#f87171;font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:20px;font-weight:700;";
      var sMsg = cleanNoticeMessage(screenshotPolicy.message, "Screen capture and recording is restricted on this AI workspace by corporate security policy.");
      curtain.innerHTML =
        '<div style="display:flex;flex-direction:column;align-items:center;gap:10px;text-align:center;max-width:480px;padding:24px;">' +
          '<div style="display:flex;align-items:center;gap:10px;">' +
            '<span style="font-size:28px;">🛡️</span>' +
            '<span style="font-size:18px;letter-spacing:0.04em;">SCREEN CAPTURE RESTRICTED</span>' +
          '</div>' +
          '<div style="font-size:13.5px;color:#cbd5e1;font-weight:400;line-height:1.5;">' + sMsg + '</div>' +
        '</div>';
      (document.body || document.documentElement).appendChild(curtain);
    }
  }

  function removeIsolatedCurtain() {
    var curtain = document.getElementById("yp-blur-curtain");
    if (curtain) curtain.remove();
  }

  // Screen capture protection is triggered on capture keys (PrintScreen, Win+Shift+S, Mac capture shortcuts)
  // or screen recording API calls, NEVER on normal window blur, focus, or tab switching.

  function handleIsolatedCaptureKeys(e) {
    if (!isExtensionValid()) return;
    if (!screenshotPolicyApplies()) return;
    var isPrtScn = e.key === "PrintScreen" || e.keyCode === 44 || e.code === "PrintScreen";
    var isWinShiftS = (e.key === "s" || e.key === "S" || e.code === "KeyS") && e.shiftKey && (e.metaKey || e.altKey);
    var isMacCapture = e.metaKey && e.shiftKey && (e.key === "3" || e.key === "4" || e.key === "5" || e.code === "Digit3" || e.code === "Digit4" || e.code === "Digit5");
    var isPrint = (e.ctrlKey || e.metaKey) && (e.key === "p" || e.key === "P" || e.code === "KeyP");

    if (isPrtScn || isWinShiftS || isMacCapture || isPrint) {
      dismissAllClipboardNotices();
      try {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      } catch (_) {}
      applyIsolatedCurtain();
      recordIsolatedGovernanceLog("--screenshot [SCREEN CAPTURE RESTRICTED] Capture Shortcut Detected", "Screen Capture Policy");
      setTimeout(removeIsolatedCurtain, 2500);
    }
  }

  window.addEventListener("keydown", handleIsolatedCaptureKeys, true);
  document.addEventListener("keydown", handleIsolatedCaptureKeys, true);
  window.addEventListener("keyup", function (e) {
    if (!isExtensionValid()) return;
    if (!screenshotPolicyApplies()) return;
    if (e.key === "PrintScreen" || e.keyCode === 44 || e.code === "PrintScreen") {
      applyIsolatedCurtain();
      setTimeout(removeIsolatedCurtain, 2500);
    }
  }, true);

  if (isExtensionValid() && chrome.storage && chrome.storage.local) {
    try {
      chrome.storage.local.get([
        "cached_sites", "block_clipboard", "clipboard_target_websites", "clipboard_block_message", "governance_block_message",
        "block_screenshot", "screenshot_target_websites", "screenshot_block_message",
        "block_drag_drop", "drag_drop_target_websites", "drag_drop_block_message",
        "enforce_corporate_domains", "corporate_email_domains", "allowed_workspace_ids", "corporate_target_websites", "corporate_block_message",
        "governance_target_websites"
      ], function (res) {
        if (!isExtensionValid()) return;
        applyClipboardPolicyState(res);
      });
    } catch (_) {}
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
    if (!isExtensionValid()) {
      if (done) done();
      return;
    }
    if (hookInjected) {
      if (done) done();
      return;
    }
    try {
      var parent = document.documentElement || document.head || document.body;
      if (!parent) {
        if (document.readyState === "loading") {
          document.addEventListener("DOMContentLoaded", function () { ensureHookInjected(done); }, { once: true });
          return;
        }
        if (done) done();
        return;
      }
      hookInjected = true;
      var s = document.createElement("script");
      s.src = chrome.runtime.getURL("page_hook.js");
      s.dataset.guardNonce = guardNonce;
      s.onload = function () { s.remove(); if (done) done(); };
      parent.appendChild(s);
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
    "block_screenshot", "screenshot_block_message", "screenshot_target_websites",
    "enforce_corporate_domains", "corporate_email_domains", "allowed_workspace_ids",
    "corporate_target_websites", "corporate_block_message"
  ];
  var SITE_KEYS = ["cached_sites", "cached_blocked_sites", "server_host"];
  var CLIPBOARD_KEYS = [
    "cached_sites", "block_clipboard", "clipboard_target_websites", "clipboard_block_message", "governance_block_message",
    "block_screenshot", "screenshot_target_websites", "screenshot_block_message",
    "block_drag_drop", "drag_drop_target_websites", "drag_drop_block_message",
    "enforce_corporate_domains", "corporate_email_domains", "allowed_workspace_ids", "corporate_target_websites", "corporate_block_message",
    "governance_target_websites"
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
      if (key === "enforce_corporate_domains" || (key.indexOf("block_") === 0 && key.slice(-8) !== "_message")) {
        msg[key] = Boolean(res[key]);
      } else {
        msg[key] = res[key] || "";
      }
    });
    return msg;
  }

  // Policy reaches the page only while this frame is an armed Target Website.
  function postPolicyToPage() {
    if (!isExtensionValid() || !armedHere || !chrome.storage || !chrome.storage.local) return;
    try {
      chrome.storage.local.get(POLICY_KEYS, function (res) {
        if (!isExtensionValid() || !armedHere) return;
        window.postMessage(policyMessage(res), "*");
      });
    } catch (_) {}
  }

  function disarmPage() {
    if (!armedHere) return;
    armedHere = false;
    window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
  }

  function evaluateSite(state) {
    if (!isExtensionValid()) return;
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
    var isTarget = hostMatches(location.hostname, sites) || clipboardPolicyApplies() || dragDropPolicyApplies() || screenshotPolicyApplies() || corporatePolicyApplies();
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
    if (!isExtensionValid()) return;
    var now = Date.now();
    if (now - lastSyncRequestAt < 2000) return; // Reduced from 5000ms → faster policy pickup
    lastSyncRequestAt = now;
    try {
      chrome.runtime.sendMessage({ type: "guard_sync_now" }, function () {
        try {
          if (!isExtensionValid() || chrome.runtime.lastError) return;
        } catch (_) {}
      });
    } catch (e) { }
  }

  function armIfMonitored() {
    if (!isExtensionValid() || !chrome.storage || !chrome.storage.local) return;
    try {
      chrome.storage.local.get(SITE_KEYS.concat(CLIPBOARD_KEYS), function (res) {
        if (!isExtensionValid()) return;
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
    } catch (_) {}
  }

  // All updates arrive through storage written by background.js.
  if (isExtensionValid() && chrome.storage && chrome.storage.onChanged) {
    try {
      chrome.storage.onChanged.addListener(function (changes, areaName) {
        if (!isExtensionValid() || areaName !== "local") return;
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
          // site check

          try {
            chrome.storage.local.get(SITE_KEYS.concat(CLIPBOARD_KEYS), function (res) {
              if (!isExtensionValid()) return;
              evaluateSite(res);
              if (armedHere && POLICY_KEYS.some(function (key) { return Boolean(changes[key]); })) postPolicyToPage();
            });
          } catch (_) {}
          return;
        }
        if (armedHere && POLICY_KEYS.some(function (key) { return Boolean(changes[key]); })) {
          postPolicyToPage();
        }
      });
    } catch (_) {}
  }

  window.addEventListener("focus", function () {
    if (!isExtensionValid()) return;
    if (armedHere) requestBackgroundSync();
  });

  // On laptop wake/resume: tab becomes visible again → re-arm check immediately
  document.addEventListener("visibilitychange", function () {
    if (!isExtensionValid()) return;
    if (document.visibilityState === "visible") {
      // Re-evaluate arm status in case EXE restarted after sleep
      armIfMonitored();
    }
  });

  if (isExtensionValid() && chrome.runtime && chrome.runtime.onMessage) {
    chrome.runtime.onMessage.addListener(function (msg) {
      if (!isExtensionValid()) return;
      if (msg && msg.type === "show_governance_notice") {
        window.postMessage({
          source: "gateway-guard",
          nonce: guardNonce,
          type: "show_bubble",
          title: msg.title || "Governance Policy",
          message: msg.message || "Action restricted by corporate security policy."
        }, "*");
      }
    });
  }


  function isTelemetryOrNoise(text, url) {
    var u = String(url || (typeof location !== "undefined" ? location.href : "") || "").toLowerCase();
    if (u.includes("/uc/") || u.includes("/lat/") || u.includes("/telemetry") || u.includes("/analytics") || u.includes("/beacon") || u.includes("/ces/")) {
      return true;
    }
    if (!text) return false;
    var t = String(text).trim();
    if (!t) return false;
    var low = t.toLowerCase();
    if (low === "set" || low === "ping" || low === "pong" || low === "heartbeat" || low === "true" || low === "false" || low === "null" || low === "me" || low === "ready" || low === "ack" || low === "sync") {
      return true;
    }
    if (low.startsWith("{") && (
      low.includes('"app_name"') ||
      low.includes('"has_web_push') ||
      low.includes('"web_push_') ||
      low.includes('"is_dark_mode"') ||
      low.includes('"screen_height"') ||
      low.includes('"screen_width"') ||
      low.includes('"pixel_ratio"') ||
      low.includes('"notification_permission"') ||
      low.includes('"device_pixel_ratio"')
    )) {
      return true;
    }
    return false;
  }

  window.addEventListener("message", function (ev) {
    if (!isExtensionValid()) return;
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard-page") return;
    var msg = ev.data;
    // Verify anti-tamper nonce
    if (!msg.nonce || msg.nonce !== guardNonce) return;

    if (msg.type === "force_logout_cookies") {
      try {
        chrome.runtime.sendMessage({
          type: "force_logout_cookies",
          domain: msg.domain || location.hostname
        }, function () {});
      } catch (_) {}
      return;
    }

    if (msg.type === "authorize_ai_session") {
      function sendAuthResult(result) {
        window.postMessage({
          source: "gateway-guard",
          nonce: guardNonce,
          type: "session_authorization_result",
          id: msg.id,
          result: result || { decision: "allow", reason_code: "FAIL_OPEN_DISCONNECTED" }
        }, "*");
      }

      if (!corporatePolicy.enabled) {
        sendAuthResult({ decision: "allow", reason_code: "POLICY_NOT_ENFORCED" });
        return;
      }
      var targetSites = corporatePolicy.targetSites;
      if (!targetSites || targetSites.toLowerCase() === "none") {
        targetSites = "ALL";
      }
      if (targetSites && !hostInConfiguredSites(location.hostname, targetSites)) {
        sendAuthResult({ decision: "allow", reason_code: "TARGET_NOT_RESTRICTED" });
        return;
      }

      var authPayload = {
        action: "authorize_ai_session",
        provider: msg.provider || "",
        email_domain: msg.email_domain || "",
        user_email: msg.user_email || "",
        workspace_id: msg.workspace_id || "",
        page_url: msg.page_url || location.href,
        policy_version: msg.policy_version || 1
      };

      function fallbackGatewayDirect() {
        var endpoints = [];
        if (serverHost) {
          endpoints.unshift("https://" + serverHost + "/api/browser-ai/session/authorize");
        }
        var idx = 0;
        function tryNext() {
          if (idx >= endpoints.length) {
            sendAuthResult({
              decision: "allow",
              reason_code: "FAIL_OPEN_DISCONNECTED",
              message: "Gateway session verification unavailable; failing open to preserve browsing."
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

    if (isTelemetryOrNoise(msg.prompt, msg.url || location.href)) {
      window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      return;
    }

    var hasContent = Boolean(
      (msg.prompt && msg.prompt.trim()) ||
      (Array.isArray(msg.files) && msg.files.length) ||
      (Array.isArray(msg.upload_images) && msg.upload_images.length)
    );
    if (!hasContent) {
      window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      return;
    }

    if (msg.id && (
      String(msg.id).indexOf("paste_block_") === 0 ||
      String(msg.id).indexOf("paste_beforeinput_") === 0 ||
      String(msg.id).indexOf("paste_api_block_") === 0 ||
      String(msg.id).indexOf("clip_key_block_") === 0 ||
      String(msg.id).indexOf("copy_block_") === 0 ||
      String(msg.id).indexOf("copy_btn_block_") === 0 ||
      String(msg.id).indexOf("copy_api_block_") === 0
    )) {
      var nowClip = Date.now();
      if (nowClip - _lastClipboardBlockLoggedAt < 2000) {
        return;
      }
      _lastClipboardBlockLoggedAt = nowClip;
    }

    if (msg.id && (String(msg.id).indexOf("drag_block_") === 0 || String(msg.id).indexOf("upload_drop_block_") === 0)) {
      var nowDrag = Date.now();
      if (nowDrag - _lastDragDropBlockLoggedAt < 2000) {
        return;
      }
      _lastDragDropBlockLoggedAt = nowDrag;
    }

    if (msg.id && String(msg.id).indexOf("screenshot_block_") === 0) {
      var nowSs = Date.now();
      if (nowSs - _lastScreenshotBlockLoggedAt < 2000) {
        return;
      }
      _lastScreenshotBlockLoggedAt = nowSs;
    }

    if (msg.id && String(msg.id).indexOf("dl_block_") === 0) {
      var nowDl = Date.now();
      if (nowDl - _lastDownloadBlockLoggedAt < 2000) {
        return;
      }
      _lastDownloadBlockLoggedAt = nowDl;
    }

    var checkPayload = {
      id: msg.id || "",
      eval_id: msg.id || msg.eval_id || "",
      host: msg.host || location.hostname,
      url: msg.url || location.href,
      prompt: msg.prompt || "",
      original_prompt: msg.original_prompt || "",
      action: msg.action || "",
      files: msg.files || [],
      upload_images: msg.upload_images || [],
      is_blocked: Boolean(msg.is_blocked),
      rule: msg.rule || "",
      blocked_reason: msg.blocked_reason || msg.rule || "",
      eval_only: Boolean(msg.eval_only),
      browser: detectCurrentBrowser()
    };

    // Record prompt check in chrome.storage.local (middle-box persistence)
    if (isExtensionValid() && chrome.storage && chrome.storage.local) {
      try {
        chrome.storage.local.set({
          last_prompt_check: {
            timestamp: Date.now(),
            host: checkPayload.host,
            prompt_preview: (checkPayload.prompt || "").slice(0, 100),
            file_count: checkPayload.files.length,
            image_count: checkPayload.upload_images.length
          }
        });
      } catch (_) {}
    }

    var reqHost = msg.host || location.hostname;
    function doCheck() {
      withBase(function () {
        if (base) {
          var hasFiles = (checkPayload.files && checkPayload.files.length > 0) || (checkPayload.upload_images && checkPayload.upload_images.length > 0);
          var checkTimeout = hasFiles ? 15000 : 2500;
          var ctrl = typeof AbortController !== "undefined" ? new AbortController() : null;
          var tId = ctrl ? setTimeout(function () { try { ctrl.abort(); } catch (_) {} }, checkTimeout) : null;
          fetch(base + "/extension/check", {
            method: "POST",
            headers: { "Content-Type": "application/json", "X-Browser-Guard-Extension": "1" },
            body: JSON.stringify(checkPayload),
            signal: ctrl ? ctrl.signal : undefined
          })
            .then(function (r) {
              if (tId) clearTimeout(tId);
              return r.json();
            })
            .then(function (decision) {
              var dec = decision || { blocked: false };
              // Save EXE evaluation decision into chrome.storage.local
              if (isExtensionValid() && chrome.storage && chrome.storage.local) {
                try {
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
                } catch (_) {}
              }
              window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: dec }, "*");
            })
            .catch(function () {
              if (serverHost) {
                fetch("https://" + serverHost + "/api/browser-ai/intercept", {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify(checkPayload)
                }).catch(function () {});
              }
              window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
            });
        } else if (serverHost) {
          fetch("https://" + serverHost + "/api/browser-ai/intercept", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(checkPayload)
          }).catch(function () {});
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
        } else {
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
        }
      });
    }

    if (isExtensionValid() && chrome.storage && chrome.storage.local) {
      try {
        chrome.storage.local.get(["cached_blocked_sites"], function (res) {
          if (!isExtensionValid()) {
            doCheck();
            return;
          }
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
      } catch (_) {
        doCheck();
      }
    } else {
      doCheck();
    }
  });

  armIfMonitored();
  if (typeof setInterval !== "undefined") {
    setInterval(function () {
      if (!isExtensionValid()) {
        try {
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
        } catch (_) {}
      }
    }, 1500);
  }
})();
