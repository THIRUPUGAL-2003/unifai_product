/* YP-Raksha Background Service Worker (Manifest V3)
 * Manages badge state, periodic background sync, and offline rule persistence
 */

var base = "http://127.0.0.1:18195";

function updateBadge(active) {
  if (chrome.action && chrome.action.setBadgeText) {
    chrome.action.setBadgeText({ text: active ? "ON" : "OFF" });
    chrome.action.setBadgeBackgroundColor({ color: active ? "#10b981" : "#ef4444" });
  }
}

function syncBackgroundRules() {
  fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
    .then(function (r) { return r.json(); })
    .then(function (data) {
      if (data && Array.isArray(data.rules)) {
        chrome.storage.local.set({
          cached_rules: data.rules,
          cached_exts: data.restricted_extensions || [],
          last_sync: Date.now()
        });
        updateBadge(true);
      }
    })
    .catch(function () {
      // Local agent unreachable; keep existing cache
    });

  fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
    .then(function (r) { return r.json(); })
    .then(function (data) {
      if (data && Array.isArray(data.hosts)) {
        chrome.storage.local.set({ cached_sites: data.hosts });
      }
    })
    .catch(function () {});
}

chrome.runtime.onInstalled.addListener(function () {
  updateBadge(true);
  syncBackgroundRules();
});

chrome.runtime.onStartup.addListener(function () {
  updateBadge(true);
  syncBackgroundRules();
});

// Periodic sync every 30 seconds
setInterval(syncBackgroundRules, 30000);
