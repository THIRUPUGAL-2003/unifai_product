/* YP-Raksha Extension Content Script (Isolated World)
 * Manages communication between page hook, local agent (port 18195), and local storage cache.
 * Implements anti-tamper nonces and offline rule persistence.
 */
(function () {
  var base = (typeof GATEWAY_GUARD_BASE === "string" && GATEWAY_GUARD_BASE.trim())
    ? GATEWAY_GUARD_BASE.trim().replace(/\/+$/, "")
    : "http://127.0.0.1:18195";

  var guardNonce = "yp_" + Math.random().toString(36).slice(2) + Date.now().toString(36);

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
        var rules = (data && data.rules) || [];
        var exts = (data && data.restricted_extensions) || [];
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
        if (res && Array.isArray(res.cached_sites) && hostMatches(location.hostname, res.cached_sites)) {
          window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
          if (Array.isArray(res.cached_rules) && res.cached_rules.length) {
            window.postMessage({
              source: "gateway-guard",
              nonce: guardNonce,
              type: "sync_rules",
              rules: res.cached_rules,
              restricted_extensions: res.cached_exts || []
            }, "*");
          }
        }
      });
    }

    // 2. Fetch live configuration from local agent
    if (!base) return;
    fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var domains = (data && data.hosts) || [];
        if (chrome.storage && chrome.storage.local) {
          chrome.storage.local.set({ cached_sites: domains });
        }
        if (!hostMatches(location.hostname, domains)) return;
        window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "arm" }, "*");
        syncRules();
      })
      .catch(function () { });
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
    if (!base || !hasContent) {
      window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      return;
    }
    fetch(base + "/extension/check", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        host: msg.host || location.hostname,
        url: msg.url || location.href,
        prompt: msg.prompt || "",
        files: msg.files || [],
        upload_images: msg.upload_images || [],
        is_blocked: Boolean(msg.is_blocked),
        rule: msg.rule || ""
      })
    })
      .then(function (r) { return r.json(); })
      .then(function (decision) {
        window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: decision || { blocked: false } }, "*");
      })
      .catch(function () {
        window.postMessage({ source: "gateway-guard", nonce: guardNonce, type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      });
  });

  injectHook(armIfMonitored);
})();
