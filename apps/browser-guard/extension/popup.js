(function () {
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

  function fetchLive(fromClick) {
    resolveGuardBase(function (base) {
      var pending = 2;
      var failed = false;
      function finishOne(err) {
        if (err) failed = true;
        pending -= 1;
        if (!fromClick || pending > 0) return;
        syncText.textContent = failed ? "EXE not reachable" : "Rules Updated!";
        setTimeout(function () {
          syncText.textContent = "Sync Live Rules";
          syncBtn.disabled = false;
        }, 1500);
      }

      fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
        .then(function (r) {
          if (!r.ok) throw new Error("rules");
          return r.json();
        })
        .then(function (data) {
          if (!data || !Array.isArray(data.rules)) throw new Error("rules");
          rulesEl.textContent = data.rules.length;
          var host = base.replace(/^https?:\/\//, "");
          gatewayEl.textContent = "Connected (" + host + ")";
          if (chrome && chrome.storage && chrome.storage.local) {
            var stored = { cached_rules: data.rules };
            if (Array.isArray(data.restricted_extensions)) {
              stored.cached_exts = data.restricted_extensions;
            }
            chrome.storage.local.set(stored);
          }
          finishOne(false);
        })
        .catch(function () {
          gatewayEl.textContent = "Offline (Local Cache Active)";
          finishOne(true);
        });

      fetch(base + "/extension/sites", { method: "GET", cache: "no-store" })
        .then(function (r) {
          if (!r.ok) throw new Error("sites");
          return r.json();
        })
        .then(function (data) {
          if (!data || !Array.isArray(data.hosts)) throw new Error("sites");
          sitesEl.textContent = data.hosts.length > 0 ? (data.hosts.length + "+") : "0";
          if (chrome && chrome.storage && chrome.storage.local) {
            chrome.storage.local.set({ cached_sites: data.hosts });
          }
          finishOne(false);
        })
        .catch(function () { finishOne(true); });
    });
  }

  syncBtn.addEventListener("click", function () {
    syncText.textContent = "Syncing...";
    syncBtn.disabled = true;
    fetchLive(true);
  });

  loadCached();
  fetchLive();
})();
