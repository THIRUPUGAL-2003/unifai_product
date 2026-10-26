(function () {
  var badge = document.getElementById("status-badge");
  var badgeText = document.getElementById("badge-text");
  var heroCard = document.getElementById("hero-card");
  var heroTitle = document.getElementById("hero-title");
  var heroDesc = document.getElementById("hero-desc");

  function setConnected() {
    if (badge) badge.className = "status-badge";
    if (badgeText) badgeText.textContent = "ACTIVE";
    if (heroCard) heroCard.className = "hero-card";
    if (heroTitle) heroTitle.textContent = "Protected & Active";
    if (heroDesc) heroDesc.textContent = "Connected to YP-Raksha Desktop Agent";
  }

  function setTampered() {
    if (badge) badge.className = "status-badge tampered";
    if (badgeText) badgeText.textContent = "TAMPER ALERT";
    if (heroCard) heroCard.className = "hero-card tampered";
    if (heroTitle) heroTitle.textContent = "Tamper Detected";
    if (heroDesc) heroDesc.textContent = "Unauthorized modification attempted";
  }

  function setOffline() {
    if (typeof chrome !== "undefined" && chrome.storage && chrome.storage.local) {
      chrome.storage.local.get(["cached_rules", "cached_tampered"], function (res) {
        if (res && res.cached_tampered) {
          setTampered();
        } else if (res && Array.isArray(res.cached_rules) && res.cached_rules.length > 0) {
          if (badge) badge.className = "status-badge";
          if (badgeText) badgeText.textContent = "ACTIVE";
          if (heroCard) heroCard.className = "hero-card";
          if (heroTitle) heroTitle.textContent = "Protected";
          if (heroDesc) heroDesc.textContent = "Security Guard Active (Local Engine)";
        } else {
          if (badge) badge.className = "status-badge standby";
          if (badgeText) badgeText.textContent = "STANDBY";
          if (heroCard) heroCard.className = "hero-card standby";
          if (heroTitle) heroTitle.textContent = "Connecting to Agent";
          if (heroDesc) heroDesc.textContent = "Start YP_Raksha.exe to connect";
        }
      });
    } else {
      if (badge) badge.className = "status-badge standby";
      if (badgeText) badgeText.textContent = "STANDBY";
      if (heroCard) heroCard.className = "hero-card standby";
      if (heroTitle) heroTitle.textContent = "Connecting to Agent";
      if (heroDesc) heroDesc.textContent = "Start YP_Raksha.exe to connect";
    }
  }

  if (typeof resolveGuardBase === "function") {
    resolveGuardBase(function (base) {
      fetch(base + "/extension/rules", { method: "GET", cache: "no-store" })
        .then(function (r) {
          if (!r.ok) throw new Error("status");
          return r.json();
        })
        .then(function (data) {
          if (data && Array.isArray(data.rules)) {
            if (data.tampered) {
              setTampered();
            } else {
              setConnected();
            }
            if (chrome && chrome.storage && chrome.storage.local) {
              chrome.storage.local.set({ cached_rules: data.rules, cached_tampered: !!data.tampered });
            }
          } else {
            setOffline();
          }
        })
        .catch(function () {
          setOffline();
        });
    });
  } else {
    setOffline();
  }
})();
