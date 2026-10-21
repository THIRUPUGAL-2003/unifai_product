/* YP-Raksha Background Service Worker (Manifest V3)
 * Manages badge state, periodic background sync, and offline rule persistence
 */
importScripts("endpoint.js", "guard_base.js");

function updateBadge(active) {
  if (chrome.action && chrome.action.setBadgeText) {
    // Keep badge text clear so no "ON" or warning overlays on pinned icon
    chrome.action.setBadgeText({ text: "" });
  }
}

function syncBackgroundRules() {
  resolveGuardBase(function (base) {
    fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
      .then(function (r) {
        if (!r.ok) throw new Error("rules");
        return r.json();
      })
      .then(function (data) {
        if (!data || !Array.isArray(data.rules)) return;
        var stored = {
          cached_rules: data.rules,
          last_sync: Date.now()
        };
        if (Array.isArray(data.restricted_extensions)) {
          stored.cached_exts = data.restricted_extensions;
        }
        chrome.storage.local.set(stored);
        updateBadge(true);
      })
      .catch(function () {
        updateBadge(false);
      });

    fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
      .then(function (r) {
        if (!r.ok) throw new Error("sites");
        return r.json();
      })
      .then(function (data) {
        if (data && Array.isArray(data.hosts)) {
          chrome.storage.local.set({ cached_sites: data.hosts });
        }
      })
      .catch(function () { });
  });
}

function ensureSyncAlarm() {
  if (chrome.alarms && chrome.alarms.create) {
    chrome.alarms.create("guard-sync", { periodInMinutes: 0.5 });
  }
}

if (chrome.alarms && chrome.alarms.onAlarm) {
  chrome.alarms.onAlarm.addListener(function (alarm) {
    if (alarm && alarm.name === "guard-sync") syncBackgroundRules();
  });
}

chrome.runtime.onInstalled.addListener(function () {
  updateBadge(true);
  ensureSyncAlarm();
  syncBackgroundRules();
});

chrome.runtime.onStartup.addListener(function () {
  updateBadge(true);
  ensureSyncAlarm();
  syncBackgroundRules();
});

ensureSyncAlarm();
setInterval(syncBackgroundRules, 30000);
