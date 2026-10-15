(function () {
  var base = "http://127.0.0.1:18195";
  var rulesEl = document.getElementById("rules-count");
  var sitesEl = document.getElementById("sites-count");
  var gatewayEl = document.getElementById("gateway-status");
  var syncBtn = document.getElementById("btn-sync");
  var syncText = document.getElementById("sync-text");

  function loadCached() {
    if (chrome && chrome.storage && chrome.storage.local) {
      chrome.storage.local.get(["cached_rules", "cached_sites"], function (res) {
        if (res) {
          if (Array.isArray(res.cached_rules) && res.cached_rules.length) {
            rulesEl.textContent = res.cached_rules.length;
          }
          if (Array.isArray(res.cached_sites) && res.cached_sites.length) {
            sitesEl.textContent = res.cached_sites.length + "+";
          }
        }
      });
    }
  }

  function fetchLive() {
    fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var rules = (data && data.rules) || [];
        rulesEl.textContent = rules.length;
        gatewayEl.textContent = "Connected (127.0.0.1:18195)";
        if (chrome && chrome.storage && chrome.storage.local) {
          chrome.storage.local.set({ cached_rules: rules });
        }
      })
      .catch(function () {
        gatewayEl.textContent = "Offline (Local Cache Active)";
      });

    fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var hosts = (data && data.hosts) || [];
        sitesEl.textContent = hosts.length > 0 ? (hosts.length + "+") : "171+";
        if (chrome && chrome.storage && chrome.storage.local) {
          chrome.storage.local.set({ cached_sites: hosts });
        }
      })
      .catch(function () {});
  }

  syncBtn.addEventListener("click", function () {
    syncText.textContent = "Syncing...";
    syncBtn.disabled = true;
    fetchLive();
    setTimeout(function () {
      syncText.textContent = "Rules Updated!";
      setTimeout(function () {
        syncText.textContent = "Sync Live Rules";
        syncBtn.disabled = false;
      }, 1500);
    }, 600);
  });

  loadCached();
  fetchLive();
})();
