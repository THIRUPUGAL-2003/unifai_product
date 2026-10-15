/* YP-Raksha Enterprise AI Guard Hook (Page World)
 * Full In-Browser DLP: Prompt Interception, All File Types, Images, Fast-Path Regex & Cloud AI
 */
(function () {
  "use strict";

  var armed = true; // Default armed, confirmed by extension
  var curScript = document.currentScript;
  var guardNonce = (curScript && curScript.dataset && curScript.dataset.guardNonce) || "";
  var passKey = false;
  var passClick = false;
  var pending = {};
  var stagedFiles = [];
  var stagedImages = [];

  // 1. Dynamic DLP Rules fetched live from PostgreSQL Database (No hardcoded static rules!)
  var dynamicRules = [];
  var restrictedExtensions = /\.(env|pem|key|cert|crt|id_rsa|id_ed25519|kdbx|sql|dump|bak|pfx|p12|ovpn|secret)$/i;

  function compileRegex(rawPattern) {
    if (!rawPattern || typeof rawPattern !== "string") return null;
    var pattern = rawPattern.trim();
    var flags = "";
    if (pattern.startsWith("(?i)")) {
      flags += "i";
      pattern = pattern.slice(4);
    }
    if (pattern.startsWith("(?m)")) {
      flags += "m";
      pattern = pattern.slice(4);
    }
    try {
      return new RegExp(pattern, flags);
    } catch (e) {
      console.warn("[Gateway Guard] Invalid pattern from DB:", rawPattern, e);
      return null;
    }
  }

  function setDynamicRules(rules, exts) {
    if (Array.isArray(rules)) {
      var compiled = [];
      for (var i = 0; i < rules.length; i++) {
        var r = rules[i];
        if (!r || !r.pattern) continue;
        var rx = compileRegex(r.pattern);
        if (rx) {
          compiled.push({
            id: r.id || "",
            name: r.name || "Custom Rule",
            action: (r.action || "BLOCK").toUpperCase(),
            msg: r.warning_message || ("Blocked by security policy: " + (r.name || "Restricted pattern")),
            re: rx
          });
        }
      }
      dynamicRules = compiled;
    }
    if (Array.isArray(exts) && exts.length > 0) {
      var escaped = exts.map(function (e) {
        return (e.startsWith(".") ? e.slice(1) : e).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      });
      try {
        restrictedExtensions = new RegExp("\\.(" + escaped.join("|") + ")$", "i");
      } catch (e) {}
    }
  }

  // Listen for messages from extension content script
  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard") return;
    if (guardNonce && ev.data.nonce && ev.data.nonce !== guardNonce) return;
    if (ev.data.type === "arm") armed = true;
    if (ev.data.type === "sync_rules") {
      setDynamicRules(ev.data.rules, ev.data.restricted_extensions);
    }
    if (ev.data.type === "decision" && pending[ev.data.id]) {
      pending[ev.data.id](ev.data.decision || { blocked: false });
      delete pending[ev.data.id];
    }
  });

  // Client-Side Fast Evaluation against live DB rules
  function evaluateClientRules(text, files) {
    // Check restricted file extensions
    if (Array.isArray(files)) {
      for (var i = 0; i < files.length; i++) {
        var fname = (files[i] && files[i].name) || "";
        if (restrictedExtensions.test(fname)) {
          return {
            blocked: true,
            message: "Uploading restricted credential/code file '" + fname + "' is blocked by company security policy.",
            rule: "Restricted File Extension"
          };
        }
      }
    }

    // Check prompt text and file text content against dynamic DB rules
    var fullContent = text || "";
    if (Array.isArray(files)) {
      for (var j = 0; j < files.length; j++) {
        if (files[j] && files[j].text) {
          fullContent += "\n" + files[j].text;
        }
      }
    }

    for (var k = 0; k < dynamicRules.length; k++) {
      var r = dynamicRules[k];
      if (r.re && r.re.test(fullContent)) {
        var act = String(r.action || "BLOCK").toUpperCase();
        if (act === "BLOCK" || act === "BLOCKED") {
          return {
            blocked: true,
            action: "Blocked",
            message: r.msg || ("Blocked by security rule: " + r.name),
            rule: r.name,
            matchedRule: r
          };
        }
        if (act === "REDACT" || act === "REDACTED") {
          var redacted = fullContent.replace(r.re, "[REDACTED]");
          return {
            blocked: false,
            action: "Redacted",
            message: r.msg || ("Confidential text was redacted (" + r.name + ")."),
            rule: r.name,
            redacted_prompt: redacted,
            matchedRule: r
          };
        }
        if (act === "WARN" || act === "WARNED") {
          return {
            blocked: false,
            action: "Warned",
            message: r.msg || ("Security Warning: " + r.name + " detected."),
            rule: r.name,
            matchedRule: r
          };
        }
      }
    }
    return null;
  }

  var lastDecisionCache = { text: "", time: 0, decision: null };

  // Ask Gateway Guard Engine (Async verify with PAC server & Gateway backend)
  function ask(prompt, files, images) {
    var trimmed = (prompt || "").trim();
    if (trimmed && (!files || files.length === 0) && (!images || images.length === 0)) {
      if (lastDecisionCache.text === trimmed && (Date.now() - lastDecisionCache.time < 5000) && lastDecisionCache.decision) {
        return Promise.resolve(lastDecisionCache.decision);
      }
    }
    return new Promise(function (resolve) {
      // 1. First run fast client-side check
      var clientDecision = evaluateClientRules(prompt, files);
      if (clientDecision && clientDecision.blocked) {
        if (trimmed) {
          lastDecisionCache = { text: trimmed, time: Date.now(), decision: clientDecision };
        }
        // Asynchronously notify backend so the blocked prompt is recorded in PostgreSQL DB & dashboard!
        window.postMessage({
          source: "gateway-guard-page",
          nonce: guardNonce,
          id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
          prompt: prompt,
          files: files || [],
          upload_images: images || [],
          host: location.hostname,
          url: location.href,
          is_blocked: true,
          rule: clientDecision.rule || ""
        }, "*");

        resolve(clientDecision);
        return;
      }

      // 2. Query Gateway Guard PAC and Cloud DLP
      var id = Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
      var timer = setTimeout(function () {
        delete pending[id];
        resolve(clientDecision || { blocked: false, passthrough: true });
      }, 5000);

      pending[id] = function (decision) {
        clearTimeout(timer);
        var res = decision || clientDecision || { blocked: false };
        if (trimmed && (!files || files.length === 0) && (!images || images.length === 0)) {
          lastDecisionCache = { text: trimmed, time: Date.now(), decision: res };
        }
        resolve(res);
      };

      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: id,
        prompt: prompt,
        files: files || [],
        upload_images: images || [],
        host: location.hostname,
        url: location.href,
        rule: clientDecision ? clientDecision.rule : ""
      }, "*");
    });
  }

  // Universal Composer Extraction across ChatGPT, Claude, Gemini, Copilot, DeepSeek, Perplexity
  function composerFrom(el) {
    if (!el) return "";
    var node = el;
    for (var i = 0; node && i < 6; i++, node = node.parentElement) {
      if (node.tagName === "TEXTAREA" || node.tagName === "INPUT") return node.value || "";
      if (node.isContentEditable) {
        var root = (node.closest && node.closest("[contenteditable='true'], [role='textbox']")) || node;
        return root.innerText || root.textContent || "";
      }
    }
    var active = document.activeElement;
    if (active && active !== el) {
      if (active.tagName === "TEXTAREA" || active.tagName === "INPUT") return active.value || "";
      if (active.isContentEditable) {
        var aroot = (active.closest && active.closest("[contenteditable='true'], [role='textbox']")) || active;
        return aroot.innerText || aroot.textContent || "";
      }
    }
    // Specific platform selectors
    var specific = document.querySelector(
      "div#prompt-textarea, " +
      "div.ProseMirror, " +
      "rich-textarea div[contenteditable], " +
      "textarea#searchbox, " +
      "textarea[placeholder*='message'], " +
      "textarea[placeholder*='Ask'], " +
      "textarea[placeholder*='DeepSeek'], " +
      "[data-testid='chat-input'], " +
      "[contenteditable='true'][role='textbox']"
    );
    if (specific) {
      if (specific.tagName === "TEXTAREA" || specific.tagName === "INPUT") return specific.value || "";
      return specific.innerText || specific.textContent || "";
    }
    return "";
  }

  // Set Composer text (for auto-redaction)
  function setComposerText(text) {
    var targets = [
      document.activeElement,
      document.querySelector("div#prompt-textarea"),
      document.querySelector("div.ProseMirror"),
      document.querySelector("rich-textarea div[contenteditable]"),
      document.querySelector("textarea")
    ];
    for (var i = 0; i < targets.length; i++) {
      var t = targets[i];
      if (!t) continue;
      if (t.tagName === "TEXTAREA" || t.tagName === "INPUT") {
        t.value = text;
        t.dispatchEvent(new Event("input", { bubbles: true }));
        return true;
      }
      if (t.isContentEditable) {
        t.innerText = text;
        t.dispatchEvent(new Event("input", { bubbles: true }));
        return true;
      }
    }
    return false;
  }

  // Sleek, compact In-Page YP-Raksha Alert Toast
  function showBubble(message, ruleName, kind) {
    var tone = kind === "warn" ? "warn" : (kind === "redact" ? "redact" : "block");
    var isWarn = (tone === "warn");
    var isRedact = (tone === "redact");
    var borderColor = isWarn ? "#f59e0b" : (isRedact ? "#38bdf8" : "#ef4444");
    var titleColor = isWarn ? "#fbbf24" : (isRedact ? "#38bdf8" : "#f87171");
    var titleText = isWarn ? "YP-Raksha Warning" : (isRedact ? "YP-Raksha Redacted" : "YP-Raksha Blocked");
    var iconText = isWarn ? "⚠️" : (isRedact ? "✂️" : "🛡️");
    var text = (message || (isWarn ? "Security Warning: Policy match detected." : (isRedact ? "Sensitive information was redacted." : "This request was blocked by YP-Raksha."))).trim();
    if (ruleName && text.indexOf(ruleName) < 0) {
      text = text + " (" + ruleName + ")";
    }
    var modalId = "yp-raksha-alert-card";
    var old = document.getElementById(modalId);
    if (old) old.remove();

    var card = document.createElement("div");
    card.id = modalId;
    card.setAttribute("role", "alertdialog");
    card.style.cssText =
      "all:initial;display:block;box-sizing:border-box;position:fixed;left:50%;bottom:95px;transform:translateX(-50%);max-width:460px;width:fit-content;min-width:280px;" +
      "z-index:2147483647;background:rgba(15,23,42,0.96);backdrop-filter:blur(12px);-webkit-backdrop-filter:blur(12px);" +
      "color:#f9fafb;border:1px solid " + borderColor + ";border-radius:12px;padding:9px 14px;box-shadow:0 10px 30px rgba(0,0,0,0.5);" +
      "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;animation:ypFadeIn 0.2s ease-out;";

    // Style animation
    if (!document.getElementById("yp-raksha-keyframes")) {
      var kf = document.createElement("style");
      kf.id = "yp-raksha-keyframes";
      kf.textContent = "@keyframes ypFadeIn{from{opacity:0;transform:translate(-50%,10px)}to{opacity:1;transform:translate(-50%,0)}}";
      (document.head || document.documentElement).appendChild(kf);
    }

    var header = document.createElement("div");
    header.style.cssText = "display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:3px;";

    var titleBox = document.createElement("div");
    titleBox.style.cssText = "display:flex;align-items:center;gap:6px;";

    var icon = document.createElement("span");
    icon.textContent = iconText;
    icon.style.cssText = "font-size:13px;line-height:1;";

    var title = document.createElement("strong");
    title.textContent = titleText;
    title.style.cssText = "font-size:11px;font-weight:700;letter-spacing:0.03em;text-transform:uppercase;color:" + titleColor + ";";

    titleBox.appendChild(icon);
    titleBox.appendChild(title);

    var closeBtn = document.createElement("button");
    closeBtn.textContent = "✕";
    closeBtn.style.cssText = "background:none;border:none;color:#9ca3af;font-size:12px;cursor:pointer;padding:0 2px;line-height:1;";
    closeBtn.onclick = function () { card.remove(); };

    header.appendChild(titleBox);
    header.appendChild(closeBtn);

    var body = document.createElement("div");
    body.style.cssText = "font-size:12.5px;line-height:1.4;color:#e2e8f0;font-weight:400;";
    body.textContent = text;

    card.appendChild(header);
    card.appendChild(body);

    (document.body || document.documentElement).appendChild(card);

    // Auto-dismiss after 5 seconds
    setTimeout(function () {
      if (card && card.parentNode) {
        card.style.transition = "opacity 0.3s ease";
        card.style.opacity = "0";
        setTimeout(function () { if (card.parentNode) card.remove(); }, 300);
      }
    }, 5000);
  }

  var lastWarnedTime = 0;
  var lastWarnedRule = "";

  function handleDecision(decision, proceed) {
    var action = String((decision && decision.action) || "").toLowerCase();
    var blocked = !!(decision && (decision.blocked || action === "blocked" || action === "block" || action === "bot answered" || action === "replied"));
    if (blocked) {
      showBubble(decision.message, decision.rule, "block");
      return;
    }
    if (action === "warned" || action === "warn" || action === "alert") {
      var now = Date.now();
      if (now - lastWarnedTime > 4000 || lastWarnedRule !== decision.rule) {
        lastWarnedTime = now;
        lastWarnedRule = decision.rule || "";
        showBubble(decision.message, decision.rule, "warn");
      }
      proceed(decision);
      return;
    }
    if (action === "redacted" || action === "redact") {
      if (decision.redacted_prompt) setComposerText(decision.redacted_prompt);
      showBubble(decision.message || "Sensitive text was redacted before sending.", decision.rule, "redact");
      proceed(decision);
      return;
    }
    proceed(decision || { blocked: false });
  }

  function rewriteBody(body, nextText) {
    if (typeof body !== "string" || !nextText) return null;
    try {
      var parsed = JSON.parse(body);
      var changed = false;
      if (parsed && typeof parsed.prompt === "string") { parsed.prompt = nextText; changed = true; }
      if (parsed && typeof parsed.query === "string") { parsed.query = nextText; changed = true; }
      if (parsed && typeof parsed.query_str === "string") { parsed.query_str = nextText; changed = true; }
      if (parsed && Array.isArray(parsed.messages)) {
        for (var m = parsed.messages.length - 1; m >= 0; m--) {
          var msg = parsed.messages[m];
          if (!msg) continue;
          var role = msg.role || (msg.author && msg.author.role) || "";
          if (role !== "user") continue;
          if (typeof msg.content === "string") { msg.content = nextText; changed = true; }
          else if (msg.content && Array.isArray(msg.content.parts)) { msg.content.parts = [nextText]; changed = true; }
          break;
        }
      }
      return changed ? JSON.stringify(parsed) : null;
    } catch (err) {
      return null;
    }
  }

  // --- 2. ALL FILE TYPES & IMAGE INTERCEPTION ENGINE ---

  function inspectAndStageFile(file) {
    if (!file) return;
    var name = file.name || "file";
    var size = file.size || 0;
    var type = file.type || "";

    // 1. Instant check file extension
    if (restrictedExtensions.test(name)) {
      showBubble("Uploading '" + name + "' is blocked by company security policy (restricted credential/code file).", "Restricted File Extension");
      return false;
    }

    // 2. Read images as Base64 for Vision check
    if (type.indexOf("image/") === 0 || /\.(png|jpg|jpeg|webp|bmp|svg)$/i.test(name)) {
      var rImg = new FileReader();
      rImg.onload = function (e) {
        if (e.target && e.target.result) {
          stagedImages.push(e.target.result);
        }
      };
      rImg.readAsDataURL(file);
    }
    // 3. Read text/code/csv/json/log files
    else if (
      type.indexOf("text/") === 0 ||
      /\.(txt|py|js|ts|json|csv|log|md|yaml|yml|sql|sh|java|cpp|c|html|xml)$/i.test(name)
    ) {
      var rText = new FileReader();
      rText.onload = function (e) {
        if (e.target && typeof e.target.result === "string") {
          var fContent = e.target.result;
          stagedFiles.push({ name: name, size: size, type: type, text: fContent });

          // Immediate DLP scan of file contents
          var fileDecision = evaluateClientRules(null, [{ name: name, text: fContent }]);
          if (fileDecision && fileDecision.blocked) {
            showBubble("File '" + name + "' contains confidential data: " + fileDecision.message, fileDecision.rule);
          }
        }
      };
      rText.readAsText(file.slice(0, 500000)); // Scan first 500KB
    } else {
      // General binary file (PDF, DOCX, XLSX, ZIP)
      stagedFiles.push({ name: name, size: size, type: type });
    }
    return true;
  }

  // Hook 1: File Input Selector (<input type="file">)
  document.addEventListener("change", function (ev) {
    if (ev.target && ev.target.tagName === "INPUT" && ev.target.type === "file") {
      var files = ev.target.files;
      if (files && files.length) {
        for (var i = 0; i < files.length; i++) {
          if (!inspectAndStageFile(files[i])) {
            ev.target.value = ""; // Clear file input on block
            ev.preventDefault();
            ev.stopImmediatePropagation();
            return;
          }
        }
      }
    }
  }, true);

  // Hook 2: Drag & Drop Files
  window.addEventListener("drop", function (ev) {
    if (ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files.length) {
      var files = ev.dataTransfer.files;
      for (var i = 0; i < files.length; i++) {
        if (!inspectAndStageFile(files[i])) {
          ev.preventDefault();
          ev.stopImmediatePropagation();
          return;
        }
      }
    }
  }, true);

  // Hook 3: Clipboard Paste (Ctrl+V Screenshots, Files & Sensitive Text Shielding)
  window.addEventListener("paste", function (ev) {
    var cd = ev.clipboardData;
    if (!cd) return;
    // 1. Files & Screenshots
    if (cd.files && cd.files.length) {
      for (var i = 0; i < cd.files.length; i++) {
        if (!inspectAndStageFile(cd.files[i])) {
          ev.preventDefault();
          ev.stopImmediatePropagation();
          return;
        }
      }
    }
    // 2. Sensitive Text Paste Shield (Blocks secret credentials directly on Ctrl+V paste)
    try {
      var pastedText = cd.getData("text/plain");
      if (pastedText && pastedText.length > 5) {
        var pasteDecision = evaluateClientRules(pastedText, []);
        if (pasteDecision && pasteDecision.blocked) {
          ev.preventDefault();
          ev.stopImmediatePropagation();
          showBubble("Pasting confidential data is blocked by company security policy: " + pasteDecision.message, pasteDecision.rule, "block");
          window.postMessage({
            source: "gateway-guard-page",
            nonce: guardNonce,
            id: "paste_log_" + Date.now().toString(36),
            prompt: pastedText.slice(0, 500),
            host: location.hostname,
            url: location.href,
            is_blocked: true,
            rule: pasteDecision.rule || ""
          }, "*");
          return;
        }
      }
    } catch (pasteErr) {}
  }, true);

  // --- 3. SEND INTERCEPTION (Enter Key & Send Button) ---

  // Keyboard Enter
  document.addEventListener("keydown", function (e) {
    if (!armed || passKey) return;
    if (e.key !== "Enter" || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey || e.isComposing) return;
    var text = composerFrom(e.target).trim();
    if (!text && stagedFiles.length === 0 && stagedImages.length === 0) return;

    e.preventDefault();
    e.stopImmediatePropagation();
    var target = e.target;

    ask(text, stagedFiles, stagedImages).then(function (decision) {
      handleDecision(decision, function () {
        stagedFiles = [];
        stagedImages = [];
        passKey = true;
        try {
          target.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true, keyCode: 13, which: 13 }));
        } finally {
          passKey = false;
        }
      });
    });
  }, true);

  // Send Button Click
  document.addEventListener("click", function (e) {
    if (!armed || passClick) return;
    var btn = e.target && e.target.closest ? e.target.closest("button,[role='button']") : null;
    if (!btn) return;

    var label = ((btn.getAttribute("aria-label") || "") + " " + (btn.getAttribute("data-testid") || "") + " " + (btn.title || "")).toLowerCase();
    var form = btn.closest ? btn.closest("form") : null;
    var sendLike =
      label.indexOf("send") >= 0 ||
      label.indexOf("submit") >= 0 ||
      label.indexOf("ask") >= 0 ||
      btn.getAttribute("data-testid") === "send-button" ||
      (form && form.contains(document.activeElement));

    if (!sendLike) return;
    var text = (composerFrom(btn) || composerFrom(document.activeElement)).trim();
    if (!text && stagedFiles.length === 0 && stagedImages.length === 0) return;

    e.preventDefault();
    e.stopImmediatePropagation();

    ask(text, stagedFiles, stagedImages).then(function (decision) {
      handleDecision(decision, function () {
        stagedFiles = [];
        stagedImages = [];
        passClick = true;
        try {
          btn.click();
        } finally {
          passClick = false;
        }
      });
    });
  }, true);

  // Form Submit Interception (Catches Enter in simple forms or form.requestSubmit)
  document.addEventListener("submit", function (e) {
    if (!armed || passClick) return;
    var form = e.target;
    if (!form) return;
    var text = (composerFrom(form) || composerFrom(document.activeElement)).trim();
    if (!text && stagedFiles.length === 0 && stagedImages.length === 0) return;

    e.preventDefault();
    e.stopImmediatePropagation();

    ask(text, stagedFiles, stagedImages).then(function (decision) {
      handleDecision(decision, function () {
        stagedFiles = [];
        stagedImages = [];
        passClick = true;
        try {
          if (typeof form.requestSubmit === "function") {
            form.requestSubmit();
          } else {
            form.submit();
          }
        } finally {
          passClick = false;
        }
      });
    });
  }, true);

  // --- 4. NETWORK LAYER HOOK (fetch, XMLHttpRequest & WebSocket) ---

  var origFetch = window.fetch;
  if (typeof origFetch === "function") {
    window.fetch = function (input, init) {
      if (!armed) return origFetch.apply(this, arguments);
      var url = typeof input === "string" ? input : (input && input.url) || "";
      var body = init && init.body;
      var promptText = "";
      var netFiles = [];

      // Handle JSON body
      if (typeof body === "string") {
        try {
          var parsed = JSON.parse(body);
          if (parsed && typeof parsed.prompt === "string") promptText = parsed.prompt;
          else if (parsed && Array.isArray(parsed.messages)) {
            for (var m = parsed.messages.length - 1; m >= 0; m--) {
              var msg = parsed.messages[m];
              if (msg && (msg.role === "user" || (msg.author && msg.author.role === "user"))) {
                if (typeof msg.content === "string") promptText = msg.content;
                else if (msg.content && Array.isArray(msg.content.parts)) promptText = msg.content.parts.join(" ");
                break;
              }
            }
          }
        } catch (err) {}
      }
      // Handle FormData body (File uploads)
      else if (body && typeof body.forEach === "function") {
        try {
          body.forEach(function (val, key) {
            if (val && typeof val.name === "string") {
              netFiles.push({ name: val.name, size: val.size, type: val.type });
            } else if (typeof val === "string" && key.indexOf("prompt") >= 0) {
              promptText = val;
            }
          });
        } catch (err) {}
      }

      if (!promptText && netFiles.length === 0) {
        return origFetch.apply(this, arguments);
      }

      var args = arguments;
      return ask(promptText, netFiles, []).then(function (decision) {
        var sent = false;
        var result = null;
        handleDecision(decision, function (chosen) {
          sent = true;
          var nextArgs = args;
          var action = String((chosen && chosen.action) || "").toLowerCase();
          if ((action === "redacted" || action === "redact") && chosen.redacted_prompt && init) {
            var rewritten = rewriteBody(init.body, chosen.redacted_prompt);
            if (rewritten) {
              var nextInit = {};
              for (var k in init) nextInit[k] = init[k];
              nextInit.body = rewritten;
              nextArgs = [input, nextInit];
            }
          }
          result = origFetch.apply(window, nextArgs);
        });
        if (!sent) return new Response("", { status: 204, headers: { "content-type": "text/plain" } });
        return result;
      });
    };
  }

  // Hook XMLHttpRequest (Legacy AJAX)
  var origXHRSend = XMLHttpRequest.prototype.send;
  var origXHROpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (method, url) {
    this._ggMethod = method;
    this._ggUrl = url;
    return origXHROpen.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function (body) {
    if (!armed || !body) return origXHRSend.apply(this, arguments);
    var promptText = "";
    if (typeof body === "string") {
      try {
        var parsed = JSON.parse(body);
        if (parsed && typeof parsed.prompt === "string") promptText = parsed.prompt;
      } catch (e) {}
    }
    if (promptText) {
      var clientDecision = evaluateClientRules(promptText, []);
      if (clientDecision && clientDecision.blocked) {
        showBubble(clientDecision.message, clientDecision.rule);
        return;
      }
    }
    return origXHRSend.apply(this, arguments);
  };

  // Hook WebSocket (Streaming & Real-time AI Chatbots)
  if (typeof window.WebSocket !== "undefined" && window.WebSocket.prototype) {
    var origWSSend = window.WebSocket.prototype.send;
    window.WebSocket.prototype.send = function (data) {
      if (armed && typeof data === "string" && data.length > 5) {
        var wsDecision = evaluateClientRules(data, []);
        if (wsDecision && wsDecision.blocked) {
          showBubble(wsDecision.message, wsDecision.rule, "block");
          return;
        }
      }
      return origWSSend.apply(this, arguments);
    };
  }

  // Hook navigator.sendBeacon (Telemetry & AI Background Ping Protection)
  if (typeof navigator.sendBeacon === "function") {
    var origSendBeacon = navigator.sendBeacon.bind(navigator);
    navigator.sendBeacon = function (url, data) {
      if (armed && typeof data === "string" && data.length > 5) {
        var beaconDecision = evaluateClientRules(data, []);
        if (beaconDecision && beaconDecision.blocked) {
          showBubble(beaconDecision.message, beaconDecision.rule, "block");
          return false;
        }
      }
      return origSendBeacon(url, data);
    };
  }

  console.log("[YP-Raksha] In-Browser DLP Engine Active: Prompts, Images, Files, WebSockets & Beacons Protected.");
})();
