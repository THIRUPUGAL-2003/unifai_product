/* YP-Raksha Background Service Worker (Manifest V3)
 * Single source of policy updates: polls the local agent, writes the full policy
 * into chrome.storage.local (only keys that changed), and content scripts react
 * to storage changes. Tabs never poll the agent themselves, so websites that are
 * not Target Websites generate no traffic and receive no policy data.
 */
if (typeof importScripts === "function") {
  // Chrome service worker. Firefox loads these via manifest background.scripts.
  importScripts("endpoint.js", "guard_base.js");
}

var SYNC_MIN_INTERVAL_MS = 1000; // Reduced from 2000ms → faster first-sync after laptop boot
var _syncInFlight = false;
var _lastSyncAt = 0;

try {
  if (chrome.runtime && chrome.runtime.setUninstallURL) {
    chrome.runtime.setUninstallURL("http://127.0.0.1:18195/extension/uninstalled");
  }
} catch (_) {}

function updateBadge(_active) {
  if (chrome.action && chrome.action.setBadgeText) {
    // Keep badge text clear so no "ON" or warning overlays on pinned icon
    chrome.action.setBadgeText({ text: "" });
  }
}

function str(v) {
  return typeof v === "string" ? v : "";
}

// Full governance policy as stored for content scripts / page hook.
function policyFromRules(data) {
  var policy = {
    cached_rules: Array.isArray(data.rules) ? data.rules : [],
    block_upload: Boolean(data.block_upload),
    upload_warning: str(data.upload_warning),
    upload_target_websites: str(data.upload_target_websites),
    block_file_extension: Boolean(data.block_file_extension),
    file_extension_mode: data.file_extension_mode || "block_listed",
    file_extension_block_message: str(data.file_extension_block_message),
    restricted_file_extensions: str(data.restricted_file_extensions),
    allowed_file_extensions: str(data.allowed_file_extensions),
    file_extension_target_websites: str(data.file_extension_target_websites),
    block_clipboard: Boolean(data.block_clipboard),
    clipboard_target_websites: str(data.clipboard_target_websites),
    clipboard_block_message: str(data.clipboard_block_message),
    block_drag_drop: Boolean(data.block_drag_drop),
    drag_drop_target_websites: str(data.drag_drop_target_websites),
    drag_drop_block_message: str(data.drag_drop_block_message),
    block_download: Boolean(data.block_download),
    download_target_websites: str(data.download_target_websites),
    download_block_message: str(data.download_block_message),
    block_screenshot: Boolean(data.block_screenshot),
    screenshot_target_websites: str(data.screenshot_target_websites),
    screenshot_block_message: str(data.screenshot_block_message),
    governance_target_websites: str(data.governance_target_websites),
    governance_block_message: str(data.governance_block_message),
    enforce_corporate_domains: Boolean(data.enforce_corporate_domains),
    corporate_email_domains: str(data.corporate_email_domains),
    allowed_workspace_ids: str(data.allowed_workspace_ids),
    corporate_target_websites: str(data.corporate_target_websites),
    corporate_block_message: str(data.corporate_block_message),
    cached_tampered: Boolean(data.tampered)
  };
  if (Array.isArray(data.restricted_extensions)) {
    policy.cached_exts = data.restricted_extensions;
  }
  if (data.uninstall_key_configured !== undefined) {
    policy.uninstall_key_configured = Boolean(data.uninstall_key_configured);
    policy.uninstall_key_rotated_at = data.uninstall_key_rotated_at || "";
  }
  if (data.company_key_hash || data.common_key_hash) {
    var cHash = String(data.company_key_hash || data.common_key_hash);
    policy.company_key_hash = cHash;
    policy.common_key_hash = cHash;
  }
  if (data.agent_key_hash || data.daily_key_hash) {
    var dHash = String(data.agent_key_hash || data.daily_key_hash);
    policy.agent_key_hash = dHash;
    policy.daily_key_hash = dHash;
  }
  return policy;
}

function pushStorageToAgent() {
  try {
    chrome.storage.local.get(null, function (allData) {
      if (!allData || typeof allData !== "object" || Object.keys(allData).length === 0) return;
      resolveGuardBase(function (base) {
        if (!base) return;
        fetch(base + "/extension/sync_storage", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Browser-Guard-Extension": "1" },
          body: JSON.stringify(allData)
        }).catch(function () {});
      });
    });
  } catch (_) {}
}

// Writes only keys whose value changed, so storage.onChanged in tabs fires
// only for real policy/site updates instead of every poll.
function storeIfChanged(next, done) {
  var keys = Object.keys(next);
  chrome.storage.local.get(keys, function (prev) {
    prev = prev || {};
    var diff = {};
    var changed = false;
    keys.forEach(function (k) {
      if (JSON.stringify(prev[k]) !== JSON.stringify(next[k])) {
        diff[k] = next[k];
        changed = true;
      }
    });
    if (changed) {
      chrome.storage.local.set(diff, function () {
        pushStorageToAgent();
        if (done) done(true);
      });
    } else {
      if (done) done(false);
    }
  });
}

function fetchJson(url) {
  return fetch(url, {
    method: "GET",
    cache: "no-store",
    headers: { "X-Browser-Guard-Extension": "1" }
  }).then(function (r) {
    if (!r.ok) throw new Error("status " + r.status);
    return r.json();
  });
}

function isLoopbackHost(host) {
  return host === "localhost" || host === "127.0.0.1" || host === "0.0.0.0" || host === "::1" ||
    host.startsWith("127.") || host.endsWith(".localhost");
}

// Gateway domain only: the agent may report its own loopback address as
// server_host, which must never replace the real gateway domain.
function gatewayHost(value) {
  var host = normalizeDomainHost(value);
  return host && !isLoopbackHost(host) ? host : "";
}

// Older agent EXE builds return rules without governance fields: read them
// directly from the gateway server instead.
function withGovernanceFallback(data, serverHost) {
  if (data.block_clipboard !== undefined || data.block_upload !== undefined || !serverHost) {
    return Promise.resolve(data);
  }
  return fetchJson("https://" + serverHost + "/api/browser-ai/rules?for=agent")
    .then(function (gdata) {
      if (!gdata || !("block_clipboard" in gdata || "block_upload" in gdata)) return data;
      var merged = Object.assign({}, gdata, { rules: data.rules });
      if (Array.isArray(data.restricted_extensions)) merged.restricted_extensions = data.restricted_extensions;
      return merged;
    })
    .catch(function () { return data; });
}

var _syncQueued = false;

function syncBackgroundRules(done) {
  var now = Date.now();
  if (_syncInFlight || now - _lastSyncAt < SYNC_MIN_INTERVAL_MS) {
    // Trailing sync so a second change inside the throttle window is not lost.
    if (!_syncQueued) {
      _syncQueued = true;
      setTimeout(function () {
        _syncQueued = false;
        syncBackgroundRules();
      }, Math.max(200, SYNC_MIN_INTERVAL_MS - (now - _lastSyncAt)));
    }
    if (done) done();
    return;
  }
  _syncInFlight = true;
  _lastSyncAt = now;

  function finish() {
    _syncInFlight = false;
    if (done) done();
  }

  resolveGuardBase(function (base) {
    chrome.storage.local.get(["server_host"], function (saved) {
      var serverHost = gatewayHost(saved && saved.server_host) ||
        gatewayHost(typeof GATEWAY_SERVER_HOST === "string" ? GATEWAY_SERVER_HOST : "");

      var sitesTask = fetchJson(base + "/extension/sites")
        .then(function (data) {
          if (!data || !Array.isArray(data.hosts)) return;
          serverHost = gatewayHost(data.server_host) || serverHost;
          storeIfChanged({
            cached_sites: data.hosts,
            cached_blocked_sites: Array.isArray(data.blocked_hosts) ? data.blocked_hosts : [],
            server_host: serverHost
          });
        })
        .catch(function () { });

      var rulesTask = fetchJson(base + "/extension/rules")
        .then(function (data) {
          if (!data || !Array.isArray(data.rules)) throw new Error("rules");
          serverHost = gatewayHost(data.server_host) || serverHost;
          return withGovernanceFallback(data, serverHost);
        })
        .then(function (data) {
          storeIfChanged(policyFromRules(data));
          chrome.storage.local.set({ last_sync: Date.now() });
          updateBadge(true);
        })
        .catch(function () {
          updateBadge(false);
        });

      Promise.all([sitesTask, rulesTask]).then(finish, finish);
    });
  });
}

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

function domainFromUrl(value) {
  return normalizeDomainHost(value);
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

function matchesDomain(host, domain) {
  var h = normalizeDomainHost(host);
  var rawD = String(domain || "").toLowerCase().trim().replace(/^[*@.]+/g, "");
  if (!h || !rawD) return false;
  var prov = detectProviderKeyword(h);
  if (prov && rawD === prov) return true;
  if (rawD.indexOf("://") < 0) {
    rawD = rawD.indexOf(":") >= 0 && (rawD.match(/:/g) || []).length > 1 ? "https://[" + rawD + "]" : "https://" + rawD;
  }
  var d = normalizeDomainHost(rawD);
  if (!d) {
    return prov ? rawD === prov : false;
  }
  if (h === d || h.endsWith("." + d) || d.endsWith("." + h)) return true;
  if (!d.includes(".") && (h === d || h.startsWith(d + ".") || h.endsWith("." + d) || h.includes("." + d + "."))) return true;
  var r1 = getRootDomain(h);
  var r2 = getRootDomain(d);
  if (r1 && r2 && r1 === r2) return true;
  if (areRelatedAIHosts(h, d)) return true;
  return false;
}

function matchesDomainList(host, raw, monitoredSites) {
  var value = String(raw || "").trim();
  if (!value || value.toUpperCase() === "ALL" || value === "*") {
    return Array.isArray(monitoredSites) && monitoredSites.some(function (site) { return matchesDomain(host, site); });
  }
  if (value.toLowerCase() === "none") return false;
  return value.split(/[;,\s]+/).some(function (site) { return matchesDomain(host, site); });
}

function recordDownloadBlockedLog(sourceHost, item) {
  chrome.storage.local.get(["server_host"], function (stored) {
    var sHost = gatewayHost(stored && stored.server_host) ||
      gatewayHost(typeof GATEWAY_SERVER_HOST === "string" ? GATEWAY_SERVER_HOST : "");
    var siteHost = sourceHost || "website";
    var fileName = (item && (item.filename || (item.url ? item.url.split("/").pop().split("?")[0] : ""))) || "";
    var promptText = "--download [DOWNLOAD BLOCKED] -- " + siteHost + (fileName ? (" (" + fileName + ")") : "");
    var payload = {
      id: "dl_block_" + Date.now().toString(36),
      eval_id: "dl_block_" + Date.now().toString(36),
      host: sourceHost,
      url: item.url || ("https://" + sourceHost + "/"),
      prompt: promptText,
      files: [],
      upload_images: [],
      is_blocked: true,
      rule: "Download Policy",
      blocked_reason: "Download Policy",
      status: "Blocked (Download Policy)",
      browser: "Chrome"
    };
    resolveGuardBase(function (base) {
      if (base) {
        fetch(base + "/extension/check", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Browser-Guard-Extension": "1" },
          body: JSON.stringify(payload)
        }).catch(function () {
          if (sHost) {
            fetch("https://" + sHost + "/api/browser-ai/intercept", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(payload)
            }).catch(function () {});
          }
        });
      } else if (sHost) {
        fetch("https://" + sHost + "/api/browser-ai/intercept", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload)
        }).catch(function () {});
      }
    });
  });
}

function cancelDownloadIfGoverned(item, sourceUrl) {
  var sourceHost = domainFromUrl(sourceUrl);
  if (!sourceHost || !item || typeof item.id !== "number") return;
  chrome.storage.local.get(["block_download", "download_target_websites", "cached_sites", "download_block_message", "governance_block_message"], function (state) {
    if (!state || !state.block_download) return;
    var monitored = Array.isArray(state.cached_sites) && state.cached_sites.some(function (site) { return matchesDomain(sourceHost, site); });
    if (!monitored || !matchesDomainList(sourceHost, state.download_target_websites, state.cached_sites)) return;
    try {
      chrome.downloads.cancel(item.id, function () {
        if (chrome.runtime && chrome.runtime.lastError) return;
        recordDownloadBlockedLog(sourceHost, item);
        var dlMsg = (state.download_block_message || state.governance_block_message || "Downloading files, artifacts, and exports from this AI tool is restricted by corporate security policy.").trim();
        if (item.tabId >= 0 && chrome.tabs && chrome.tabs.sendMessage) {
          try {
            chrome.tabs.sendMessage(item.tabId, {
              type: "show_governance_notice",
              title: "Download Policy",
              message: dlMsg,
              action: "download"
            });
          } catch (_) {}
        }
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
        if (chrome.runtime && chrome.runtime.lastError) tab = null;
        checkFrom((tab && tab.url) || item.referrer || item.url);
      });
    } else {
      checkFrom(item.referrer || item.url);
    }
  });
}

// Content scripts on Target Websites ask for an immediate refresh (page load
// with empty cache, tab focus). This also wakes a suspended service worker.
if (chrome.runtime && chrome.runtime.onMessage) {
  chrome.runtime.onMessage.addListener(function (msg, sender, sendResponse) {
    if (!msg) return false;
    if (msg.type === "force_logout_cookies" || msg.type === "clear_site_cookies") {
      var dom = String(msg.domain || "").toLowerCase().replace(/^https?:\/\//, "").split("/")[0].split(":")[0];
      try {
        if (chrome.cookies && chrome.cookies.getAll) {
          chrome.cookies.getAll({}, function (allCookies) {
            if (!allCookies) {
              try { sendResponse({ ok: false }); } catch (_) {}
              return;
            }
            for (var ci = 0; ci < allCookies.length; ci++) {
              var c = allCookies[ci];
              var cDomain = (c.domain || "").toLowerCase().replace(/^\./, "");
              var matchesTarget = false;
              if (dom) {
                matchesTarget = (cDomain === dom || cDomain.endsWith("." + dom) || dom.endsWith("." + cDomain) || areRelatedAIHosts(cDomain, dom));
              } else {
                matchesTarget = cDomain.includes("claude.ai") || cDomain.includes("anthropic.com");
              }
              if (matchesTarget) {
                var protocol = c.secure ? "https:" : "http:";
                var url = protocol + "//" + c.domain.replace(/^\./, "") + c.path;
                try {
                  chrome.cookies.remove({ url: url, name: c.name });
                } catch (_) {}
              }
            }
            try { sendResponse({ ok: true }); } catch (_) {}
          });
          return true;
        }
      } catch (_) {}
      try { sendResponse({ ok: false }); } catch (_) {}
      return false;
    }
    if (msg.type === "guard_sync_now") {
      watchAgent();
      syncBackgroundRules(function () {
        try { sendResponse({ ok: true }); } catch (_) {}
      });
      return true;
    }
    return false;
  });
}

// Live link to the EXE. The background parks on /extension/wait; the EXE answers as
// soon as the website changes Target Websites, rules or controls (server pushes a
// "policy" event to the EXE), so the new policy lands in chrome.storage in ~1s.
// While the EXE is not running yet, it retries every second, so a freshly installed
// EXE and extension find each other within about a second in either install order.
var WATCH_WAIT_SECONDS = 20; // under the 30s MV3 service worker idle limit
var WATCH_RETRY_MS = 500;  // Reduced from 1000ms → faster EXE reconnect after laptop boot
var _watchRunning = false;
var _watchVersion = -1;

function fetchJsonWithTimeout(url, ms) {
  var ctrl = typeof AbortController !== "undefined" ? new AbortController() : null;
  var timer = ctrl ? setTimeout(function () { try { ctrl.abort(); } catch (_) {} }, ms) : null;
  return fetch(url, {
    method: "GET",
    cache: "no-store",
    headers: { "X-Browser-Guard-Extension": "1" },
    signal: ctrl ? ctrl.signal : undefined
  })
    .then(function (r) {
      if (timer) clearTimeout(timer);
      if (!r.ok) throw new Error("status " + r.status);
      return r.json();
    }, function (err) {
      if (timer) clearTimeout(timer);
      throw err;
    });
}

// Guard was uninstalled from the website: drop all cached policy so no page stays
// armed and no governance control keeps blocking. A reinstalled EXE restores it.
function clearPolicyOnUninstall() {
  var empty = policyFromRules({ rules: [] });
  empty.cached_exts = [];
  empty.cached_sites = [];
  empty.cached_blocked_sites = [];
  storeIfChanged(empty);
}

function watchAgent() {
  if (_watchRunning) return;
  _watchRunning = true;

  function next(delayMs) {
    setTimeout(loop, delayMs);
  }

  function loop() {
    // Any extension API call resets the service worker idle timer, keeping this loop alive.
    chrome.storage.local.get(["last_sync"], function () {
      resolveGuardBase(function (base) {
        var url = base + "/extension/wait?v=" + _watchVersion + "&t=" + WATCH_WAIT_SECONDS;
        fetchJsonWithTimeout(url, (WATCH_WAIT_SECONDS + 5) * 1000)
          .then(function (data) {
            if (data && data.uninstalling) {
              _watchVersion = -1;
              clearPolicyOnUninstall();
              next(WATCH_RETRY_MS);
              return;
            }
            var version = Number(data && data.policy_version);
            if (isFinite(version) && version !== _watchVersion) {
              _watchVersion = version;
              _lastSyncAt = 0;
              syncBackgroundRules();
            }
            next(0);
          })
          .catch(function () {
            // EXE not running (yet) or moved port: probe all candidate ports next time.
            _watchVersion = -1;
            if (typeof _guardResolved !== "undefined") _guardResolved = "";
            next(WATCH_RETRY_MS);
          });
      });
    });
  }

  loop();
}

function ensureSyncAlarm() {
  if (chrome.alarms && chrome.alarms.create) {
    chrome.alarms.create("guard-sync", { periodInMinutes: 0.5 });
  }
}

if (chrome.alarms && chrome.alarms.onAlarm) {
  chrome.alarms.onAlarm.addListener(function (alarm) {
    if (alarm && alarm.name === "guard-sync") {
      syncBackgroundRules();
      watchAgent();
    }
  });
}

chrome.runtime.onInstalled.addListener(function () {
  updateBadge(true);
  ensureSyncAlarm();
  pushStorageToAgent();
  syncBackgroundRules();
});

chrome.runtime.onStartup.addListener(function () {
  updateBadge(true);
  ensureSyncAlarm();
  pushStorageToAgent();
  syncBackgroundRules();
});

ensureSyncAlarm();
pushStorageToAgent();
setInterval(syncBackgroundRules, 10000);
watchAgent();

if (typeof chrome !== "undefined" && chrome.runtime && chrome.runtime.onSuspend) {
  chrome.runtime.onSuspend.addListener(function () {
    try {
      resolveGuardBase(function (base) {
        if (typeof navigator !== "undefined" && navigator.sendBeacon) {
          navigator.sendBeacon(base + "/extension/disconnected");
        } else {
          fetch(base + "/extension/disconnected", { method: "POST", keepalive: true }).catch(function () {});
        }
      });
    } catch (_) {}
  });
}
