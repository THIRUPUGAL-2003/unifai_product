/* Isolated world: asks the local Guard, then tells the page hook the decision. */
(function () {
  var base = (typeof GATEWAY_GUARD_BASE === "string" && GATEWAY_GUARD_BASE) || "";

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
        window.postMessage({
          source: "gateway-guard",
          type: "sync_rules",
          rules: rules,
          restricted_extensions: exts
        }, "*");
      })
      .catch(function () {});
  }

  function armIfMonitored() {
    if (!base) return;
    fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var domains = (data && data.hosts) || [];
        if (!hostMatches(location.hostname, domains)) return;
        window.postMessage({ source: "gateway-guard", type: "arm" }, "*");
        syncRules();
      })
      .catch(function () {});
  }

  // Periodic rule refresh every 15 seconds and on tab focus
  setInterval(syncRules, 15000);
  window.addEventListener("focus", syncRules);

  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard-page") return;
    var msg = ev.data;
    var hasContent = Boolean(
      (msg.prompt && msg.prompt.trim()) ||
      (Array.isArray(msg.files) && msg.files.length) ||
      (Array.isArray(msg.upload_images) && msg.upload_images.length)
    );
    if (!base || !hasContent) {
      window.postMessage({ source: "gateway-guard", type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
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
        window.postMessage({ source: "gateway-guard", type: "decision", id: msg.id, decision: decision || { blocked: false } }, "*");
      })
      .catch(function () {
        window.postMessage({ source: "gateway-guard", type: "decision", id: msg.id, decision: { blocked: false, passthrough: true } }, "*");
      });
  });

  injectHook(armIfMonitored);
})();
