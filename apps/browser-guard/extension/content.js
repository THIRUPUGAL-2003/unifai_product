/* YP-Raksha Extension Content Script (Isolated World)
 * Manages communication between page hook, local agent (port 18195), and local storage cache.
 * Implements anti-tamper nonces and offline rule persistence.
 */
(function () {
  var base = (typeof GATEWAY_GUARD_BASE === "string" && GATEWAY_GUARD_BASE.trim())
    ? GATEWAY_GUARD_BASE.trim().replace(/\/+$/, "")
    : "http://127.0.0.1:18195";

  var guardNonce = "yp_" + Math.random().toString(36).slice(2) + Date.now().toString(36);

  function isInternalHost(host) {
    host = (host || "").toLowerCase().trim();
    if (host.indexOf("www.") === 0) host = host.slice(4);
    if (host.indexOf(":") >= 0) host = host.split(":")[0];
    if (!host || host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "0.0.0.0") return true;
    if (host.endsWith(".local") || host.endsWith(".internal") || host.endsWith(".lan") || host.endsWith(".home") || host.endsWith(".corp")) return true;
    if (/^10\.\d+\.\d+\.\d+$/.test(host)) return true;
    if (/^192\.168\.\d+\.\d+$/.test(host)) return true;
    if (/^172\.(1[6-9]|2\d|3[0-1])\.\d+\.\d+$/.test(host)) return true;
    if (/^127\.\d+\.\d+\.\d+$/.test(host)) return true;
    return false;
  }

  // STRICT BYPASS: Never monitor, inject, or intercept on internal portals / localhost / private networks
  if (isInternalHost(location.hostname)) {
    return;
  }

  function hostMatches(host, domains) {
    host = (host || "").toLowerCase();
    if (host.indexOf("www.") === 0) host = host.slice(4);
    for (var i = 0; i < domains.length; i++) {
      var d = String(domains[i] || "").toLowerCase();
      if (!d) continue;
      if (host === d || host.slice(-(d.length + 1)) === "." + d) return true;
    }
    return false;
  }

  function injectHook(done) {
    try {
      var s = document.createElement("script");
      s.src = chrome.runtime.getURL("page_hook.js");
      s.dataset.guardNonce = guardNonce;
      s.onload = function () { s.remove(); if (done) done(); };
      (document.documentElement || document.head || document.body).appendChild(s);
    } catch (e) {
      if (done) done();
    }
  }

  function syncRules() {
    if (!base) return;
    fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var rules = (data && Array.isArray(data.rules)) ? data.rules : [];
        var exts = (data && Array.isArray(data.restricted_extensions)) ? data.restricted_extensions : [];
        if (chrome.storage && chrome.storage.local) {
          chrome.storage.local.set({ cached_rules: rules, cached_exts: exts });
        }
        window.postMessage({
          source: "gateway-guard",
          nonce: guardNonce,
          type: "sync_rules",
          rules: rules,
          restricted_extensions: exts
        }, "*");
      })
      .catch(function () { });
  }

  function armIfMonitored() {
    // 1. Check local cache first for zero-latency startup
    if (chrome.storage && chrome.storage.local) {
      chrome.storage.local.get(["cached_rules", "cached_sites", "cached_exts"], function (res) {
        var sites = (res && Array.isArray(res.cached_sites)) ? res.cached_sites : [];
        if (hostMatches(location.hostname, sites)) {
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
          window.postMessage({
            source: "gateway-guard",
            nonce: guardNonce,
            type: "sync_rules",
            rules: Array.isArray(res.cached_rules) ? res.cached_rules : [],
            restricted_extensions: res.cached_exts || []
          }, "*");
        } else {
          // Domain is not monitored or was deleted
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
        }
      });
    }

    // 2. Fetch live configuration from local agent
    if (!base) return;
    fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var domains = (data && Array.isArray(data.hosts)) ? data.hosts : [];
        if (chrome.storage && chrome.storage.local) {
          chrome.storage.local.set({ cached_sites: domains });
        }
        if (!hostMatches(location.hostname, domains)) {
          // Domain removed from target websites: disarm immediately
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
          return;
        }
        window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
        syncRules();
      })
      .catch(function () { });
  }

  // Listen for storage changes across background worker and tabs
  if (chrome.storage && chrome.storage.onChanged) {
    chrome.storage.onChanged.addListener(function (changes, areaName) {
      if (areaName !== "local") return;
      if (changes.cached_rules) {
        var newRules = Array.isArray(changes.cached_rules.newValue) ? changes.cached_rules.newValue : [];
        window.postMessage({
          source: "gateway-guard",
          nonce: guardNonce,
          type: "sync_rules",
          rules: newRules,
          restricted_extensions: (changes.cached_exts && changes.cached_exts.newValue) || []
        }, "*");
      }
      if (changes.cached_sites) {
        var newSites = Array.isArray(changes.cached_sites.newValue) ? changes.cached_sites.newValue : [];
        if (hostMatches(location.hostname, newSites)) {
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
        } else {
          // Domain was deleted from target sites list: disarm immediately
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "disarm" }, "*");
        }
      }
    });
  }

  // Periodic rule refresh every 15 seconds and on tab focus
  setInterval(syncRules, 15000);
  window.addEventListener("focus", syncRules);

  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard-page") return;
    var msg = ev.data;
    // Verify anti-tamper nonce
    if (msg.nonce && msg.nonce !== guardNonce) return;

    var hasContent = Boolean(
      (msg.prompt && msg.prompt.trim()) ||
      (Array.isArray(msg.files) && msg.files.length) ||
      (Array.isArray(msg.upload_images) && msg.upload_images.length)
    );
    if (!base || !hasContent || isInternalHost(location.hostname) || isInternalHost(msg.host)) {
      window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      return;
    }

    var checkPayload = {
      host: msg.host || location.hostname,
      url: msg.url || location.href,
      prompt: msg.prompt || "",
      files: msg.files || [],
      upload_images: msg.upload_images || [],
      is_blocked: Boolean(msg.is_blocked),
      rule: msg.rule || ""
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

  injectHook(armIfMonitored);
})();
