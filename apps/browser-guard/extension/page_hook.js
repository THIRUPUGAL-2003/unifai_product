/* YP-Raksha Enterprise AI Guard Hook (Page World)
 * Full In-Browser DLP: Prompt Interception, All File Types, Images, Fast-Path Regex & Cloud AI
 */
(function () {
  "use strict";

  var armed = true; // Default armed, confirmed by extension
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
        if (r.action === "BLOCK" || r.action === "REDACT") {
          return {
            blocked: true,
            action: r.action,
            message: r.msg,
            rule: r.name,
            matchedRule: r
          };
        }
      }
    }
    return null;
  }

  // Ask Gateway Guard Engine (Async verify with PAC server & Gateway backend)
  function ask(prompt, files, images) {
    return new Promise(function (resolve) {
      // 1. First run fast client-side check
      var clientDecision = evaluateClientRules(prompt, files);
      if (clientDecision && clientDecision.blocked) {
        // Asynchronously notify backend so the blocked prompt is recorded in PostgreSQL DB & dashboard!
        window.postMessage({
          source: "gateway-guard-page",
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
        resolve({ blocked: false, passthrough: true });
      }, 5000);

      pending[id] = function (decision) {
        clearTimeout(timer);
        resolve(decision || { blocked: false });
      };

      window.postMessage({
        source: "gateway-guard-page",
        id: id,
        prompt: prompt,
        files: files || [],
        upload_images: images || [],
        host: location.hostname,
        url: location.href
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

  // Stunning In-Page YP-Raksha Warning Card
  function showBubble(message, ruleName, offendingText) {
    var text = (message || "This request was blocked by YP-Raksha.").trim();
    var modalId = "yp-raksha-alert-card";
    var old = document.getElementById(modalId);
    if (old) old.remove();

    var card = document.createElement("div");
    card.id = modalId;
    card.setAttribute("role", "alertdialog");
    card.style.cssText =
      "position:fixed;left:50%;bottom:80px;transform:translateX(-50%);max-width:680px;width:min(680px,calc(100% - 32px));" +
      "z-index:2147483647;background:rgba(17,24,39,0.96);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);" +
      "color:#f9fafb;border:1px solid #ef4444;border-radius:18px;padding:16px 20px;box-shadow:0 20px 45px rgba(0,0,0,0.45);" +
      "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;animation:ypFadeIn 0.25s ease-out;";

    // Style animation
    if (!document.getElementById("yp-raksha-keyframes")) {
      var kf = document.createElement("style");
      kf.id = "yp-raksha-keyframes";
      kf.textContent = "@keyframes ypFadeIn{from{opacity:0;transform:translate(-50%,16px)}to{opacity:1;transform:translate(-50%,0)}}";
      (document.head || document.documentElement).appendChild(kf);
    }

    var header = document.createElement("div");
    header.style.cssText = "display:flex;align-items:center;justify-content:space-between;margin-bottom:8px;";

    var titleBox = document.createElement("div");
    titleBox.style.cssText = "display:flex;align-items:center;gap:8px;";

    var icon = document.createElement("span");
    icon.textContent = "🛡️";
    icon.style.cssText = "font-size:18px;";

    var title = document.createElement("strong");
    title.textContent = "YP-Raksha Data Loss Prevention";
    title.style.cssText = "font-size:13px;font-weight:700;letter-spacing:0.02em;color:#f87171;";

    titleBox.appendChild(icon);
    titleBox.appendChild(title);

    var closeBtn = document.createElement("button");
    closeBtn.textContent = "✕";
    closeBtn.style.cssText = "background:none;border:none;color:#9ca3af;font-size:16px;cursor:pointer;padding:2px 6px;line-height:1;";
    closeBtn.onclick = function () { card.remove(); };

    header.appendChild(titleBox);
    header.appendChild(closeBtn);

    var body = document.createElement("div");
    body.style.cssText = "font-size:14px;line-height:1.5;color:#e5e7eb;";
    body.textContent = text;

    if (ruleName) {
      var badge = document.createElement("div");
      badge.style.cssText = "display:inline-block;background:#374151;color:#fbbf24;font-size:11px;font-weight:600;padding:3px 8px;border-radius:6px;margin-top:8px;";
      badge.textContent = "Rule Triggered: " + ruleName;
      body.appendChild(document.createElement("br"));
      body.appendChild(badge);
    }

    card.appendChild(header);
    card.appendChild(body);

    (document.body || document.documentElement).appendChild(card);
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

  // Hook 3: Clipboard Paste (Ctrl+V Screenshots & Files)
  window.addEventListener("paste", function (ev) {
    var cd = ev.clipboardData;
    if (!cd) return;
    if (cd.files && cd.files.length) {
      for (var i = 0; i < cd.files.length; i++) {
        if (!inspectAndStageFile(cd.files[i])) {
          ev.preventDefault();
          ev.stopImmediatePropagation();
          return;
        }
      }
    }
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
      if (decision && decision.blocked) {
        showBubble(decision.message, decision.rule);
        return;
      }
      stagedFiles = [];
      stagedImages = [];
      passKey = true;
      try {
        target.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true, keyCode: 13, which: 13 }));
      } finally {
        passKey = false;
      }
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
      if (decision && decision.blocked) {
        showBubble(decision.message, decision.rule);
        return;
      }
      stagedFiles = [];
      stagedImages = [];
      passClick = true;
      try {
        btn.click();
      } finally {
        passClick = false;
      }
    });
  }, true);

  // --- 4. NETWORK LAYER HOOK (fetch & XMLHttpRequest) ---

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
        if (decision && decision.blocked) {
          showBubble(decision.message, decision.rule);
          return new Response("", { status: 204, headers: { "content-type": "text/plain" } });
        }
        return origFetch.apply(window, args);
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

  console.log("[YP-Raksha] In-Browser DLP Engine Active: Prompts, Images, and All File Types Protected.");
})();
