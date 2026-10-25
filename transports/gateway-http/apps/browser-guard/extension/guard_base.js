/* Finds the local Guard EXE. The packed endpoint.js URL is tried first,
 * then 18195 and the two fallback ports the EXE may bind. */
var GUARD_BASE_FALLBACK = "http://127.0.0.1:18195";
var _guardResolved = "";
var _guardResolvedAt = 0;
var _guardWaiters = null;

function guardBaseCandidates() {
  var list = [];
  function add(url) {
    url = String(url || "").trim().replace(/\/+$/, "");
    if (url && list.indexOf(url) < 0) list.push(url);
  }
  if (typeof GATEWAY_GUARD_BASE === "string") add(GATEWAY_GUARD_BASE);
  add(GUARD_BASE_FALLBACK);
  add("http://127.0.0.1:18196");
  add("http://127.0.0.1:18197");
  return list;
}

function rememberGuardBase(base) {
  _guardResolved = base;
  _guardResolvedAt = Date.now();
  try {
    if (typeof chrome !== "undefined" && chrome.storage && chrome.storage.local) {
      chrome.storage.local.set({ guard_base: base });
    }
  } catch (e) {}
}

function resolveGuardBase(done) {
  if (_guardResolved && Date.now() - _guardResolvedAt < 60000) {
    done(_guardResolved);
    return;
  }
  if (_guardWaiters) {
    _guardWaiters.push(done);
    return;
  }
  _guardWaiters = [done];
  var candidates = guardBaseCandidates();

  function finish(base, ok) {
    if (ok) rememberGuardBase(base);
    var waiters = _guardWaiters || [];
    _guardWaiters = null;
    for (var i = 0; i < waiters.length; i++) waiters[i](base);
  }

  function probe() {
    var i = 0;
    function next() {
      if (i >= candidates.length) {
        finish(candidates[0] || GUARD_BASE_FALLBACK, false);
        return;
      }
      var base = candidates[i++];
      var ctrl = typeof AbortController !== "undefined" ? new AbortController() : null;
      var timer = ctrl ? setTimeout(function () { try { ctrl.abort(); } catch (e) {} }, 1500) : null;
      fetch(base + "/extension/rules", {
        method: "GET",
        cache: "no-store",
        signal: ctrl ? ctrl.signal : undefined
      })
        .then(function (r) {
          if (timer) clearTimeout(timer);
          if (!r.ok) throw new Error("status");
          return r.json();
        })
        .then(function (data) {
          if (data && Array.isArray(data.rules)) finish(base, true);
          else next();
        })
        .catch(function () {
          if (timer) clearTimeout(timer);
          next();
        });
    }
    next();
  }

  try {
    if (typeof chrome !== "undefined" && chrome.storage && chrome.storage.local) {
      chrome.storage.local.get(["guard_base"], function (res) {
        var saved = res && res.guard_base ? String(res.guard_base) : "";
        saved = saved.trim().replace(/\/+$/, "");
        if (saved) {
          candidates = [saved].concat(candidates.filter(function (c) { return c !== saved; }));
        }
        probe();
      });
      return;
    }
  } catch (e) {}
  probe();
}
