import os

def update_content_js():
    path = r"apps/browser-guard/extension/content.js"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    # 1. Update blockClipboardEvent to wipe clipboard and selection
    old_clip_block = '''    if (isCopy && ev.clipboardData) {
      try { ev.clipboardData.clearData(); } catch (_) {}
    }
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation();'''

    new_clip_block = '''    if (isCopy && ev.clipboardData) {
      try {
        ev.clipboardData.clearData();
        ev.clipboardData.setData("text/plain", "");
        ev.clipboardData.setData("text/html", "");
      } catch (_) {}
    }
    try {
      var sel = window.getSelection();
      if (sel) sel.removeAllRanges();
    } catch (_) {}
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation();'''

    if old_clip_block in content:
        content = content.replace(old_clip_block, new_clip_block)
        print("Updated blockClipboardEvent in content.js!")
    else:
        print("WARNING: old_clip_block not found in content.js!")

    # 2. Add copy button click listener after keydown listener
    old_key_listen = '''  window.addEventListener("keydown", handleClipboardKeys, true);
  document.addEventListener("keydown", handleClipboardKeys, true);'''

    new_key_listen = '''  window.addEventListener("keydown", handleClipboardKeys, true);
  document.addEventListener("keydown", handleClipboardKeys, true);

  // Intercept clicks on AI "Copy" and "Copy code" buttons in isolated world
  document.addEventListener("click", function (ev) {
    if (!clipboardPolicyApplies()) return;
    var target = (ev.composedPath && ev.composedPath()[0]) || ev.target;
    var el = (target && target.closest) ? target.closest("button, [role='button'], a, [data-tooltip], [data-testid]") : null;
    if (!el) return;
    var aria = (el.getAttribute("aria-label") || "").toLowerCase();
    var title = (el.getAttribute("title") || "").toLowerCase();
    var testid = (el.getAttribute("data-testid") || "").toLowerCase();
    var tooltip = (el.getAttribute("data-tooltip") || el.getAttribute("data-tooltip-id") || el.getAttribute("mattooltip") || "").toLowerCase();
    var biId = (el.getAttribute("data-bi-id") || "").toLowerCase();
    var text = (el.innerText || el.textContent || "").toLowerCase().trim();
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
      text === "copy to clipboard";
    if (isCopyBtn) {
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
      } catch (_) {}
      if (!document.getElementById("yp-raksha-alert-card")) {
        showClipboardBlockedNotice("Clipboard Copy Restricted", cleanNoticeMessage(clipboardPolicy.message, "Clipboard copying from this AI tool is restricted by corporate security policy."));
      }
      recordIsolatedGovernanceLog("--copy paste [CLIPBOARD COPY BUTTON]", "Clipboard Policy");
    }
  }, true);'''

    if old_key_listen in content:
        content = content.replace(old_key_listen, new_key_listen)
        print("Added copy button click listener to content.js!")
    else:
        print("WARNING: old_key_listen not found in content.js!")

    # 3. Add screenshot log to handleIsolatedCaptureKeys
    old_capture = '''    if (isPrtScn || isWinShiftS || isMacCapture || isPrint) {
      dismissAllClipboardNotices();
      try {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      } catch (_) {}
      applyIsolatedCurtain();
      setTimeout(removeIsolatedCurtain, 2500);
    }'''

    new_capture = '''    if (isPrtScn || isWinShiftS || isMacCapture || isPrint) {
      dismissAllClipboardNotices();
      try {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      } catch (_) {}
      applyIsolatedCurtain();
      recordIsolatedGovernanceLog("--screenshot [SCREEN CAPTURE RESTRICTED] Capture Shortcut Detected", "Screen Capture Policy");
      setTimeout(removeIsolatedCurtain, 2500);
    }'''

    if old_capture in content:
        content = content.replace(old_capture, new_capture)
        print("Updated handleIsolatedCaptureKeys in content.js!")
    else:
        print("WARNING: old_capture not found in content.js!")

    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    print("content.js write finished.")

if __name__ == "__main__":
    update_content_js()
