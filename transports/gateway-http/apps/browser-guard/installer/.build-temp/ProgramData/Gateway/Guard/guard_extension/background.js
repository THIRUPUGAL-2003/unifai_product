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
          last_sync: Date.now(),
          block_upload: Boolean(data.block_upload),
          upload_warning: data.upload_warning || "",
          upload_target_websites: data.upload_target_websites || "",
          block_file_extension: Boolean(data.block_file_extension),
          file_extension_mode: data.file_extension_mode || "block_listed",
          restricted_file_extensions: data.restricted_file_extensions || "",
          allowed_file_extensions: data.allowed_file_extensions || "",
          file_extension_target_websites: data.file_extension_target_websites || "",
          block_clipboard: Boolean(data.block_clipboard),
          clipboard_target_websites: data.clipboard_target_websites || "",
          clipboard_block_message: data.clipboard_block_message || "",
          block_drag_drop: Boolean(data.block_drag_drop),
          drag_drop_target_websites: data.drag_drop_target_websites || "",
          block_download: Boolean(data.block_download),
          download_target_websites: data.download_target_websites || "",
          governance_target_websites: data.governance_target_websites || "",
          governance_block_message: data.governance_block_message || ""
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

function normalizeDomainHost(value) {
  var host = String(value || "").toLowerCase().trim();
  if (!host) return "";
  if (host.indexOf("://") >= 0) {
    try { host = new URL(host).hostname; } catch (_) { return ""; }
  } else {
    host = host.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0];
    if ((host.match(/:/g) || []).length === 1 && /:\d+$/.test(host)) host = host.replace(/:\d+$/, "");
  }
  if (host.charAt(0) === "[" && host.indexOf("]") >= 0) host = host.slice(1, host.indexOf("]"));
  return host.replace(/^www\./, "").replace(/\.$/, "");
}

function domainFromUrl(value) {
  return normalizeDomainHost(value);
}

function matchesDomain(host, domain) {
  var h = normalizeDomainHost(host);
  var d = String(domain || "").toLowerCase().trim().replace(/^\*\./, "").replace(/^\./, "");
  if (!h || !d) return false;
  if (d.indexOf("://") < 0) d = d.indexOf(":") >= 0 && (d.match(/:/g) || []).length > 1 ? "https://[" + d + "]" : "https://" + d;
  d = normalizeDomainHost(d);
  return h === d || h.endsWith("." + d);
}

function matchesDomainList(host, raw, monitoredSites) {
  var value = String(raw || "").trim();
  if (!value || value.toUpperCase() === "ALL" || value === "*") {
    return Array.isArray(monitoredSites) && monitoredSites.some(function (site) { return matchesDomain(host, site); });
  }
  if (value.toLowerCase() === "none") return false;
  return value.split(/[;,\s]+/).some(function (site) { return matchesDomain(host, site); });
}

function cancelDownloadIfGoverned(item, sourceUrl) {
  var sourceHost = domainFromUrl(sourceUrl);
  if (!sourceHost || !item || typeof item.id !== "number") return;
  chrome.storage.local.get(["block_download", "download_target_websites", "cached_sites"], function (state) {
    if (!state || !state.block_download) return;
    var monitored = Array.isArray(state.cached_sites) && state.cached_sites.some(function (site) { return matchesDomain(sourceHost, site); });
    if (!monitored || !matchesDomainList(sourceHost, state.download_target_websites, state.cached_sites)) return;
    try {
      chrome.downloads.cancel(item.id, function () {
        if (chrome.runtime && chrome.runtime.lastError) return;
      });
    } catch (_) {}
  });
}

if (chrome.downloads && chrome.downloads.onCreated) {
  chrome.downloads.onCreated.addListener(function (item) {
    if (!item) return;
    function checkFrom(url) {
      cancelDownloadIfGoverned(item, url || item.referrer || item.url);
    }
    if (item.tabId >= 0 && chrome.tabs && chrome.tabs.get) {
      chrome.tabs.get(item.tabId, function (tab) {
        checkFrom((tab && tab.url) || item.referrer || item.url);
      });
    } else {
      checkFrom(item.referrer || item.url);
    }
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
setInterval(syncBackgroundRules, 10000);
