import os

def update_page_hook():
    path = r"apps/browser-guard/extension/page_hook.js"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    # 1. Update handleSessionAuthDecision to log personal account blocks to Prompt Logs
    old_auth_decision = '''  function handleSessionAuthDecision(authRes) {
    if (!authRes) return;
    if (authRes.decision === "block") {
      sessionBlocked = true;
      sessionBlockedReason = authRes.message || "Personal accounts are blocked by corporate security policy.";
      renderCorporateBanner(true, sessionBlockedReason, detectedEmail);
    } else {
      sessionBlocked = false;
      sessionBlockedReason = "";
      renderCorporateBanner(false);
    }
  }'''

    new_auth_decision = '''  var _lastCorpAuthLoggedAt = 0;
  function handleSessionAuthDecision(authRes) {
    if (!authRes) return;
    if (authRes.decision === "block") {
      sessionBlocked = true;
      sessionBlockedReason = authRes.message || "Personal accounts are blocked by corporate security policy.";
      renderCorporateBanner(true, sessionBlockedReason, detectedEmail);
      var nowCorp = Date.now();
      if (nowCorp - _lastCorpAuthLoggedAt > 8000) {
        _lastCorpAuthLoggedAt = nowCorp;
        window.postMessage({
          source: "gateway-guard-page",
          nonce: guardNonce,
          id: "corp_block_" + Date.now().toString(36),
          prompt: "--corporate account [UNAUTHORIZED DOMAIN] User: " + (detectedEmail || "Personal Account"),
          host: location.hostname,
          url: location.href,
          is_blocked: true,
          rule: "Corporate AI Workspace Policy",
          blocked_reason: "Unauthorized Personal Account",
          status: "Blocked (Unauthorized Personal Account)"
        }, "*");
      }
    } else {
      sessionBlocked = false;
      sessionBlockedReason = "";
      renderCorporateBanner(false);
    }
  }'''

    if old_auth_decision in content:
        content = content.replace(old_auth_decision, new_auth_decision)
        print("Updated handleSessionAuthDecision!")
    else:
        print("WARNING: old_auth_decision not found!")

    # 2. Update scanDOMForSession to support Claude (__next_f), Gemini (Google Account aria-label), Copilot (MeControl)
    old_scan_dom = '''      if (typeof window.WIZ_global_data === "object" && window.WIZ_global_data !== null) {
        scanObjectForSession(window.WIZ_global_data, 0);
        try {
          var wStr = JSON.stringify(window.WIZ_global_data);
          var wm = wStr.match(emailRegex);
          if (wm && wm[0] && !wm[0].includes("google.com/")) {
            triggerSessionAuth(wm[0].toLowerCase(), "");
            return;
          }
        } catch (_) {}
      }'''

    new_scan_dom = '''      // 1. Google Account / Gemini Dedicated Scanner
      var gAccountEls = querySelectorAllDeep(
        "a[href*='accounts.google.com'], [aria-label*='Google Account'], [aria-label*='Google-Konto'], [aria-label*='Compte Google'], [aria-label*='Cuenta de Google']"
      );
      for (var gi = 0; gi < gAccountEls.length; gi++) {
        var gLabel = (gAccountEls[gi].getAttribute("aria-label") || "") + " " + (gAccountEls[gi].getAttribute("title") || "");
        var gParenMatch = gLabel.match(/\\(([^()@\\s]+@[^()@\\s]+)\\)/);
        if (gParenMatch && gParenMatch[1]) {
          var gEm = gParenMatch[1].toLowerCase().trim();
          if (!gEm.includes("google.com/") && !gEm.endsWith("@google.com")) {
            triggerSessionAuth(gEm, "");
            return;
          }
        }
        var gAnyMatch = gLabel.match(emailRegex);
        if (gAnyMatch && gAnyMatch[0]) {
          var gEm2 = gAnyMatch[0].toLowerCase().trim();
          if (!gEm2.includes("google.com/") && !gEm2.endsWith("@google.com")) {
            triggerSessionAuth(gEm2, "");
            return;
          }
        }
      }

      // 2. Microsoft Copilot MeControl & Account Widget Scanner
      var msEls = querySelectorAllDeep("#meControl, #mectrl_currentAccount_secondary, [aria-label*='Account manager'], [id*='mectrl']");
      for (var mi = 0; mi < msEls.length; mi++) {
        var msTxt = (msEls[mi].getAttribute("aria-label") || "") + " " + (msEls[mi].innerText || "") + " " + (msEls[mi].textContent || "");
        var msMatch = msTxt.match(emailRegex);
        if (msMatch && msMatch[0]) {
          var msEm = msMatch[0].toLowerCase().trim();
          if (!msEm.includes("microsoft.com/") && !msEm.includes("example.com")) {
            triggerSessionAuth(msEm, "");
            return;
          }
        }
      }

      // 3. Claude React Server Components Streaming Chunks (__next_f)
      if (Array.isArray(window.__next_f)) {
        for (var nfi = 0; nfi < window.__next_f.length; nfi++) {
          var nfChunk = window.__next_f[nfi];
          var nfStr = typeof nfChunk === "string" ? nfChunk : (Array.isArray(nfChunk) && typeof nfChunk[1] === "string" ? nfChunk[1] : "");
          if (nfStr && nfStr.includes("@")) {
            var nfMatches = nfStr.match(new RegExp(emailRegex.source, "g")) || [];
            for (var nfm = 0; nfm < nfMatches.length; nfm++) {
              var emCand = nfMatches[nfm].toLowerCase();
              if (!emCand.includes("sentry") && !emCand.includes("anthropic.com") && !emCand.includes("example.com") && !emCand.includes("w3.org") && !emCand.includes("schema.org") && !emCand.endsWith(".png") && !emCand.endsWith(".jpg")) {
                triggerSessionAuth(emCand, "");
                return;
              }
            }
          }
        }
      }

      if (typeof window.WIZ_global_data === "object" && window.WIZ_global_data !== null) {
        scanObjectForSession(window.WIZ_global_data, 0);
        try {
          var wStr = JSON.stringify(window.WIZ_global_data);
          var wmMatches = wStr.match(new RegExp(emailRegex.source, "g")) || [];
          for (var wi = 0; wi < wmMatches.length; wi++) {
            var cand = wmMatches[wi].toLowerCase();
            if (!cand.endsWith("@google.com") && !cand.includes("gserviceaccount.com") && !cand.includes("sentry") && !cand.includes("w3.org") && !cand.includes(".png")) {
              triggerSessionAuth(cand, "");
              return;
            }
          }
        } catch (_) {}
      }'''

    if old_scan_dom in content:
        content = content.replace(old_scan_dom, new_scan_dom)
        print("Updated scanDOMForSession!")
    else:
        print("WARNING: old_scan_dom not found!")

    # 3. Update handleCopyEvent to wipe clipboard data and remove all selection ranges
    old_copy_event = '''  function handleCopyEvent(ev) {
    if (!armed) return;
    if (clipboardPolicyActive()) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      if (ev.clipboardData) {
        try { ev.clipboardData.clearData(); } catch (_) {}
      }'''

    new_copy_event = '''  function handleCopyEvent(ev) {
    if (!armed) return;
    if (clipboardPolicyActive()) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      if (ev.clipboardData) {
        try {
          ev.clipboardData.clearData();
          ev.clipboardData.setData("text/plain", "");
          ev.clipboardData.setData("text/html", "");
        } catch (_) {}
      }
      try {
        var sel = window.getSelection();
        if (sel) sel.removeAllRanges();
      } catch (_) {}'''

    if old_copy_event in content:
        content = content.replace(old_copy_event, new_copy_event)
        print("Updated handleCopyEvent!")
    else:
        print("WARNING: old_copy_event not found!")

    # 4. Update copy button interceptor
    old_copy_btn = '''    var isCopyBtn =
      aria.includes("copy") ||
      title.includes("copy") ||
      testid.includes("copy") ||
      text === "copy" ||
      text === "copy code" ||
      text === "copy text" ||
      text === "copy response";'''

    new_copy_btn = '''    var tooltip = (el.getAttribute("data-tooltip") || el.getAttribute("data-tooltip-id") || el.getAttribute("mattooltip") || "").toLowerCase();
    var biId = (el.getAttribute("data-bi-id") || "").toLowerCase();
    var hasCopySvg = !!(el.querySelector && el.querySelector("svg[class*='copy' i], svg[aria-label*='copy' i], [class*='copy-icon' i], [class*='icon-copy' i]"));
    var isCopyBtn =
      aria.includes("copy") ||
      title.includes("copy") ||
      testid.includes("copy") ||
      tooltip.includes("copy") ||
      biId.includes("copy") ||
      hasCopySvg ||
      text === "copy" ||
      text === "copy code" ||
      text === "copy text" ||
      text === "copy response" ||
      text === "copy prompt" ||
      text === "copy to clipboard";'''

    if old_copy_btn in content:
        content = content.replace(old_copy_btn, new_copy_btn)
        print("Updated copy button interceptor!")
    else:
        print("WARNING: old_copy_btn not found!")

    # 5. In copy button click interceptor, also clear selection and clipboard
    old_copy_btn_block = '''    if (isCopyBtn) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();'''

    new_copy_btn_block = '''    if (isCopyBtn) {
      ev.preventDefault();
      ev.stopPropagation();
      ev.stopImmediatePropagation();
      try {
        if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
          navigator.clipboard.writeText("").catch(function () {});
        }
      } catch (_) {}
      try {
        var sel = window.getSelection();
        if (sel) sel.removeAllRanges();
      } catch (_) {}'''

    if old_copy_btn_block in content:
        content = content.replace(old_copy_btn_block, new_copy_btn_block)
        print("Updated copy button block actions!")
    else:
        print("WARNING: old_copy_btn_block not found!")

    # 6. Update fetch GET interceptor to detect Claude, Copilot, Gemini endpoints
    old_fetch_get = '''if (typeof url === "string" && (url.indexOf("/session") >= 0 || url.indexOf("/me") >= 0 || url.indexOf("/user") >= 0 || url.indexOf("/auth") >= 0 || url.indexOf("/account") >= 0 || url.indexOf("/profile") >= 0)) {'''

    new_fetch_get = '''if (typeof url === "string" && (
          url.indexOf("/session") >= 0 ||
          url.indexOf("/me") >= 0 ||
          url.indexOf("/user") >= 0 ||
          url.indexOf("/auth") >= 0 ||
          url.indexOf("/account") >= 0 ||
          url.indexOf("/profile") >= 0 ||
          url.indexOf("/organization") >= 0 ||
          url.indexOf("/org") >= 0 ||
          url.indexOf("/bootstrap") >= 0 ||
          url.indexOf("/userInfo") >= 0 ||
          url.indexOf("/current_user") >= 0
        )) {'''

    if old_fetch_get in content:
        content = content.replace(old_fetch_get, new_fetch_get)
        print("Updated fetch GET interceptor!")
    else:
        print("WARNING: old_fetch_get not found!")

    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    print("page_hook.js write finished.")

if __name__ == "__main__":
    update_page_hook()
