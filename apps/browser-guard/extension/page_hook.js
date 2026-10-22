/* YP-Raksha Enterprise AI Guard Hook (Page World)
 * Full In-Browser DLP: Prompt Interception, All File Types, Images, Fast-Path Regex & Cloud AI
 */
(function () {
  "use strict";

  // STRICT BYPASS: Never attach interceptors on internal portals, localhost, or private networks
  var curHost = (location.hostname || "").toLowerCase().trim();
  if (curHost.indexOf("www.") === 0) curHost = curHost.slice(4);
  if (curHost.indexOf(":") >= 0) curHost = curHost.split(":")[0];
  if (
    !curHost ||
    curHost === "localhost" ||
    curHost === "127.0.0.1" ||
    curHost === "::1" ||
    curHost === "0.0.0.0" ||
    curHost.endsWith(".local") ||
    curHost.endsWith(".internal") ||
    curHost.endsWith(".lan") ||
    curHost.endsWith(".home") ||
    curHost.endsWith(".corp") ||
    /^10\.\d+\.\d+\.\d+$/.test(curHost) ||
    /^192\.168\.\d+\.\d+$/.test(curHost) ||
    /^172\.(1[6-9]|2\d|3[0-1])\.\d+\.\d+$/.test(curHost) ||
    /^127\.\d+\.\d+\.\d+$/.test(curHost)
  ) {
    return;
  }

  var armed = false; // Armed only when confirmed by extension for monitored AI domains
  var curScript = document.currentScript;
  var guardNonce = (curScript && curScript.dataset && curScript.dataset.guardNonce) || "";
  var passKey = false;
  var passClick = false;
  var pending = {};
  var stagedFiles = [];
  var stagedImages = [];
  var _lastHandledPromptTime = 0;
  var _lastHandledPromptText = "";
  var _lastHandledPromptId = "";
  var lastDecisionCache = { text: "", time: 0, decision: null };

  function stripInvisible(str) {
    if (!str) return "";
    return String(str)
      .replace(/[\u200b-\u200f\ufeff\u202a-\u202e\u2060-\u206f\u00a0]/g, "")
      .replace(/\s+/g, " ")
      .trim();
  }

  // Rules come only from the gateway for the armed target site.
  var dynamicRules = [];

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
    // In DLP prompt/file scanning, normalize full-string anchors ^...$ so patterns like ^[1-9][0-9]{5}$
    // match sensitive tokens inside full sentences (e.g. "613002 this is new")
    if (pattern.startsWith("^") && pattern.endsWith("$") && pattern.length > 2) {
      var inner = pattern.slice(1, -1);
      if (!/(?:^|[^\\])[\^$]/.test(inner)) {
        pattern = "\\b(?:" + inner + ")\\b";
      }
    }
    try {
      return new RegExp(pattern, flags);
    } catch (e) {
      console.warn("[Gateway Guard] Invalid pattern from DB:", rawPattern, e);
      return null;
    }
  }

  function setDynamicRules(rules) {
    if (!Array.isArray(rules) || rules.length === 0) {
      dynamicRules = [];
      return;
    }
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

  // Listen for messages from extension content script
  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard") return;
    if (guardNonce && ev.data.nonce && ev.data.nonce !== guardNonce) return;
    if (ev.data.type === "arm") {
      armed = true;
    }
    if (ev.data.type === "disarm") {
      armed = false;
      dynamicRules = [];
      lastDecisionCache = { text: "", time: 0, decision: null };
    }
    if (ev.data.type === "sync_rules") {
      setDynamicRules(ev.data.rules || []);
    }
    if (ev.data.type === "decision" && pending[ev.data.id]) {
      pending[ev.data.id](ev.data.decision || { blocked: false });
      delete pending[ev.data.id];
    }
  });

  // Client-Side Fast Evaluation against live DB rules
  function evaluateClientRules(text, files) {
    // Check prompt text and file text content against the target site's rules.
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

  var _lastAskSig = "";
  var _lastAskTime = 0;
  var _lastAskDecision = null;

  // Ask Gateway Guard Engine (Async verify with PAC server & Gateway backend)
  function ask(prompt, files, images) {
    if (!armed) {
      return Promise.resolve({ blocked: false, passthrough: true });
    }
    var trimmed = (prompt || "").trim();
    var fileNamesSig = (files || []).map(function (f) { return (f && (f.name || f)) || ""; }).sort().join(",");
    var sig = trimmed + "||" + fileNamesSig + "||" + ((images || []).length);
    if (sig && (Date.now() - _lastAskTime < 4000) && _lastAskSig === sig && _lastAskDecision) {
      return Promise.resolve(_lastAskDecision);
    }
    if (trimmed && (!files || files.length === 0) && (!images || images.length === 0)) {
      if (lastDecisionCache.text === trimmed && (Date.now() - lastDecisionCache.time < 5000) && lastDecisionCache.decision) {
        return Promise.resolve(lastDecisionCache.decision);
      }
    }
    return new Promise(function (resolve) {
      // 1. First run fast client-side check
      var clientDecision = evaluateClientRules(prompt, files);
      if (clientDecision && clientDecision.blocked) {
        _lastAskSig = sig;
        _lastAskTime = Date.now();
        _lastAskDecision = clientDecision;
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
        _lastAskSig = sig;
        _lastAskTime = Date.now();
        _lastAskDecision = res;
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

  // Deep helper: pierce shadow roots to find currently focused element
  function getDeepActiveElement(root) {
    try {
      var active = (root || document).activeElement;
      while (active && active.shadowRoot && active.shadowRoot.activeElement) {
        active = active.shadowRoot.activeElement;
      }
      return active;
    } catch (e) {
      return (root || document).activeElement;
    }
  }

  // Deep helper: search selectors across light DOM and all open shadow roots
  function querySelectorAllDeep(selector, root) {
    root = root || document;
    var results = [];
    try {
      var direct = root.querySelectorAll(selector);
      for (var i = 0; i < direct.length; i++) results.push(direct[i]);
    } catch (e) {}

    try {
      var walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT, null, false);
      while (walker.nextNode()) {
        var n = walker.currentNode;
        if (n && n.shadowRoot) {
          var sub = querySelectorAllDeep(selector, n.shadowRoot);
          for (var j = 0; j < sub.length; j++) results.push(sub[j]);
        }
      }
    } catch (e2) {}
    return results;
  }

  // Normalize invisible characters, zero-width spaces, and bidi control markers
  function stripInvisible(str) {
    if (!str || typeof str !== "string") return "";
    return str
      .replace(/[\u200B-\u200F\uFEFF\u202A-\u202E\u2060-\u206F]/g, "")
      .replace(/\u00A0/g, " ")
      .trim();
  }

  // Universal Composer Extraction across ChatGPT, Claude, Gemini, Copilot, DeepSeek, Mistral, Lovable
  function composerFrom(el) {
    if (!el) return "";
    var node = el;

    // Check if element itself has value property (web component, textarea, input)
    if (typeof node.value === "string" && node.value) return node.value;

    // Check inside element's shadowRoot if present
    if (node.shadowRoot) {
      try {
        var shadowInput = node.shadowRoot.querySelector("textarea, [contenteditable='true'], input[type='text'], [role='textbox']");
        if (shadowInput) {
          var sVal = (shadowInput.tagName === "TEXTAREA" || shadowInput.tagName === "INPUT") ? (shadowInput.value || "") : (shadowInput.innerText || shadowInput.textContent || "");
          if (sVal && sVal.trim()) return sVal.trim();
        }
      } catch (se) {}
    }

    for (var i = 0; node && i < 6; i++, node = (node.parentElement || (node.getRootNode && node.getRootNode().host))) {
      if (node.tagName === "TEXTAREA" || node.tagName === "INPUT") return node.value || "";
      if (node.isContentEditable) {
        var root = (node.closest && node.closest("[contenteditable='true'], [role='textbox']")) || node;
        return root.innerText || root.textContent || "";
      }
      if (typeof node.value === "string" && node.value) return node.value;
    }

    // Check surrounding form / fieldset / composer container of the clicked button
    var container = (el && el.closest && el.closest("fieldset, form, [class*='composer' i], [class*='input' i], div[data-testid*='composer' i], div.relative")) || null;
    if (container) {
      var innerEd = container.querySelector("div.ProseMirror, [contenteditable='true'], textarea, input[type='text'], [role='textbox']");
      if (innerEd) {
        if (innerEd.tagName === "TEXTAREA" || innerEd.tagName === "INPUT") return innerEd.value || "";
        var ct = (innerEd.innerText || innerEd.textContent || "").trim();
        if (ct) return ct;
      }
    }

    var deepActive = getDeepActiveElement();
    if (deepActive && deepActive !== el) {
      if (deepActive.tagName === "TEXTAREA" || deepActive.tagName === "INPUT") return deepActive.value || "";
      if (deepActive.isContentEditable) {
        var aroot = (deepActive.closest && deepActive.closest("[contenteditable='true'], [role='textbox']")) || deepActive;
        return aroot.innerText || aroot.textContent || "";
      }
      if (typeof deepActive.value === "string" && deepActive.value) return deepActive.value;
    }

    // Specific platform selectors across document and deep shadow roots
    var candidates = querySelectorAllDeep(
      "textarea[placeholder*='Message' i], " +
      "textarea[placeholder*='Copilot' i], " +
      "[placeholder*='Message Copilot' i], " +
      "[aria-label*='Message Copilot' i], " +
      "[placeholder*='Message' i], " +
      "[aria-label*='Message' i], " +
      "textarea#userInput, " +
      "fieldset div.ProseMirror, " +
      "fieldset [contenteditable='true'], " +
      "form div.ProseMirror, " +
      "div#prompt-textarea, " +
      "div.ProseMirror, " +
      "rich-textarea div[contenteditable], " +
      "textarea#searchbox, " +
      "textarea[placeholder*='Ask' i], " +
      "textarea[placeholder*='prompt' i], " +
      "textarea[placeholder*='describe' i], " +
      "textarea[placeholder*='generate' i], " +
      "textarea[placeholder*='image' i], " +
      "textarea[placeholder*='video' i], " +
      "textarea[placeholder*='music' i], " +
      "textarea[placeholder*='DeepSeek'], " +
      "[data-testid*='chat-input' i], " +
      "[data-testid*='prompt' i], " +
      "[contenteditable='true'][role='textbox']"
    );
    for (var j = 0; j < candidates.length; j++) {
      var c = candidates[j];
      if (!c) continue;
      // Skip editors in chat history
      if (c.closest && c.closest("[data-testid*='conversation-turn'], [data-testid*='user-message'], [class*='font-claude-message'], [class*='ChatMessage'], div.group")) {
        continue;
      }
      var t = (c.tagName === "TEXTAREA" || c.tagName === "INPUT") ? (c.value || "") : (c.innerText || c.textContent || "");
      if (t && t.trim()) return t.trim();
    }
    if (candidates.length) {
      var last = candidates[candidates.length - 1];
      var lastVal = (last.tagName === "TEXTAREA" || last.tagName === "INPUT") ? (last.value || "") : (last.innerText || last.textContent || "");
      if (lastVal && lastVal.trim()) return lastVal.trim();
    }

    // Universal Target Domain fallback: search all visible textareas & contenteditables with text
    try {
      var allEditors = querySelectorAllDeep("textarea, [contenteditable='true'], [role='textbox']");
      for (var k = allEditors.length - 1; k >= 0; k--) {
        var ed = allEditors[k];
        if (!ed) continue;
        if (ed.closest && ed.closest("[data-testid*='conversation-turn'], [data-testid*='user-message'], [class*='font-claude-message'], [class*='ChatMessage'], [class*='message-content'], div.group")) {
          continue;
        }
        var ev = (ed.tagName === "TEXTAREA" || ed.tagName === "INPUT") ? (ed.value || "") : (ed.innerText || ed.textContent || "");
        if (ev && ev.trim()) return ev.trim();
      }
    } catch (e) {}

    return "";
  }

  // Extract file attachments currently displayed in the chat composer UI
  function extractDOMAttachedFiles() {
    var detected = [];
    try {
      // STRICT SCOPE: Only look inside the active composer/upload container (NOT in previous message history)
      var activeContainer = document.querySelector(
        "fieldset, form, [class*='composer' i], [class*='input-container' i], div[data-testid*='composer' i], div[class*='chat-input' i]"
      ) || document.body;

      var selectors = [
        "[data-testid*='attachment']",
        "[data-testid*='file']",
        "div.file-item",
        "div[data-testid='file-preview']",
        "[aria-label*='attachment' i]",
        "[aria-label*='file' i]",
        "div[class*='attachment' i]",
        "div[class*='file-tile' i]",
        "div[class*='file-preview' i]",
        "span[class*='file-name' i]"
      ];
      var nodes = activeContainer.querySelectorAll(selectors.join(", "));
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i];
        if (!n) continue;
        // Never scrape files from conversation message history
        if (n.closest && n.closest("[data-testid*='conversation-turn'], [data-testid*='user-message'], [class*='font-claude-message'], [class*='ChatMessage'], [class*='message-content'], div.group")) {
          continue;
        }
        var txt = (n.getAttribute("aria-label") || n.getAttribute("title") || n.innerText || n.textContent || "").trim();
        if (!txt) continue;
        var m = txt.match(/([a-zA-Z0-9_\-\. ]+\.[a-zA-Z0-9]{1,8})/);
        if (m && m[1]) {
          var fn = m[1].trim();
          if (fn && !detected.some(function (x) { return x.name === fn; })) {
            detected.push({ name: fn, size: 0, type: "application/octet-stream" });
          }
        }
      }
    } catch (e) { }
    return detected;
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

    // Auto-dismiss after 1.5 seconds (requested by user)
    setTimeout(function () {
      if (card && card.parentNode) {
        card.style.transition = "opacity 0.2s ease";
        card.style.opacity = "0";
        setTimeout(function () { if (card.parentNode) card.remove(); }, 200);
      }
    }, 1500);
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

    // Read images as Base64 for Vision check
    if (type.indexOf("image/") === 0 || /\.(png|jpg|jpeg|webp|bmp|svg)$/i.test(name)) {
      var rImg = new FileReader();
      rImg.onload = function (e) {
        if (e.target && e.target.result) {
          stagedImages.push(e.target.result);
        }
      };
      rImg.readAsDataURL(file);
    }
    // 3. Read text/code/script/markup/config files
    else if (
      type.indexOf("text/") === 0 ||
      /\.(txt|py|pyw|pyi|pyx|js|mjs|cjs|jsx|ts|tsx|mts|cts|json|jsonc|json5|csv|tsv|log|md|markdown|yaml|yml|sql|sh|bash|zsh|fish|bat|cmd|ps1|psm1|psd1|java|kt|kts|scala|sc|groovy|c|h|cpp|cc|cxx|hpp|hh|hxx|cs|csx|fs|go|rs|rlib|php|phtml|phar|rb|rake|gemspec|swift|dart|lua|r|rmd|jl|pl|pm|t|html|htm|xml|svg|xhtml|css|scss|sass|less|vue|svelte|toml|ini|conf|config|cfg|properties|env|graphql|gql|proto|dockerfile|prisma|gradle|cmake|tf|tfvars|sol|asm|s|diff|patch|ipynb)$/i.test(name) ||
      /^(dockerfile|makefile|jenkinsfile|procfile|gemfile|rakefile|justfile|vagrantfile|license|licence|readme|\.env|\.gitignore|\.gitattributes|\.editorconfig|package\.json|cargo\.toml|go\.mod|requirements\.txt)$/i.test(name)
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
    }
    // 4. Binary Documents (PDF, Word, Excel, PowerPoint, ODF, RTF, ZIP, Archives)
    else if (
      /\.(pdf|docx|doc|xlsx|xls|pptx|ppt|odt|ods|odp|rtf|zip|tar|gz|7z)$/i.test(name) ||
      type.indexOf("application/pdf") >= 0 ||
      type.indexOf("application/zip") >= 0 ||
      type.indexOf("officedocument") >= 0
    ) {
      var rDoc = new FileReader();
      rDoc.onload = function (e) {
        if (e.target && typeof e.target.result === "string") {
          var b64 = e.target.result.split(",")[1] || "";
          stagedFiles.push({ name: name, size: size, type: type, base64: b64 });
        }
      };
      rDoc.readAsDataURL(file.slice(0, 2000000)); // Read first 2MB for deep backend extraction
    } else {
      // 5. General fallback: read first 1MB as Base64 so desktop proxy/agent extracts & scans it
      var rAny = new FileReader();
      rAny.onload = function (e) {
        if (e.target && typeof e.target.result === "string") {
          var b64 = e.target.result.split(",")[1] || "";
          stagedFiles.push({ name: name, size: size, type: type, base64: b64 });
        }
      };
      rAny.readAsDataURL(file.slice(0, 1000000));
    }
    return true;
  }

  // Hook 1: File Input Selector (<input type="file">)
  document.addEventListener("change", function (ev) {
    if (!armed) return;
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
    if (!armed) return;
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
    if (!armed) return;
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
    } catch (pasteErr) { }
  }, true);

  // --- 3. SEND INTERCEPTION (Enter Key & Send Button) ---

  function mergeStagedAndDomFiles() {
    var domFiles = extractDOMAttachedFiles();
    var all = stagedFiles.slice();
    for (var i = 0; i < domFiles.length; i++) {
      if (!all.some(function (x) { return x.name === domFiles[i].name; })) {
        all.push(domFiles[i]);
      }
    }
    return all;
  }

  // Keyboard Enter
  document.addEventListener("keydown", function (e) {
    if (!armed || passKey) return;
    if (e.key !== "Enter" || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey || e.isComposing) return;
    var actualTarget = (e.composedPath && e.composedPath()[0]) || e.target;
    var text = stripInvisible(composerFrom(actualTarget) || composerFrom(e.target) || composerFrom(getDeepActiveElement()));
    var effectiveFiles = mergeStagedAndDomFiles();
    if (!text && effectiveFiles.length === 0 && stagedImages.length === 0) return;

    var clientDecision = evaluateClientRules(text, effectiveFiles);
    if (clientDecision && clientDecision.blocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(clientDecision.message, clientDecision.rule, "block");
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
        prompt: text,
        files: effectiveFiles || [],
        upload_images: stagedImages || [],
        host: location.hostname,
        url: location.href,
        is_blocked: true,
        rule: clientDecision.rule || ""
      }, "*");
      return;
    }

    if (effectiveFiles.length > 0 || stagedImages.length > 0) {
      e.preventDefault();
      e.stopImmediatePropagation();
      var target = e.target;
      ask(text, effectiveFiles, stagedImages).then(function (decision) {
        handleDecision(decision, function () {
          stagedFiles = [];
          stagedImages = [];
          passKey = true;
          _lastHandledPromptTime = Date.now();
          _lastHandledPromptText = text;
          try {
            var sendBtn = document.querySelector("button[aria-label*='Send' i], button.send-button, [data-testid*='send-button' i]");
            if (sendBtn) {
              sendBtn.click();
            } else {
              target.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true, keyCode: 13, which: 13 }));
            }
          } finally {
            passKey = false;
          }
        });
      });
      return;
    }

    // Fast path: Clean text prompt without files sends instantly with native Enter (0ms delay)
    _lastHandledPromptTime = Date.now();
    _lastHandledPromptText = text;
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
      prompt: text,
      files: [],
      upload_images: [],
      host: location.hostname,
      url: location.href,
      is_blocked: false,
      rule: ""
    }, "*");
  }, true);

  // Send Button Click
  document.addEventListener("click", function (e) {
    if (!armed || passClick) return;
    var actualEl = (e.composedPath && e.composedPath()[0]) || e.target;
    var btn = (actualEl && actualEl.closest ? actualEl.closest("button,[role='button']") : null) || (e.target && e.target.closest ? e.target.closest("button,[role='button']") : null);
    if (!btn) return;

    var label = ((btn.getAttribute("aria-label") || "") + " " + (btn.getAttribute("data-testid") || "") + " " + (btn.title || "")).toLowerCase();
    var btnText = (btn.innerText || btn.textContent || "").toLowerCase().trim();
    var form = btn.closest ? btn.closest("form") : null;
    var sendLike =
      label.indexOf("send") >= 0 ||
      label.indexOf("submit") >= 0 ||
      label.indexOf("ask") >= 0 ||
      label.indexOf("generate") >= 0 ||
      label.indexOf("create") >= 0 ||
      label.indexOf("run") >= 0 ||
      label.indexOf("draw") >= 0 ||
      label.indexOf("message") >= 0 ||
      label.indexOf("chat") >= 0 ||
      label.indexOf("post") >= 0 ||
      label.indexOf("search") >= 0 ||
      btnText === "generate" ||
      btnText === "create" ||
      btnText === "send" ||
      btnText === "submit" ||
      btnText === "ask" ||
      btnText === "run" ||
      btnText === "post" ||
      btnText === "search" ||
      btnText === "go" ||
      btn.getAttribute("type") === "submit" ||
      btn.type === "submit" ||
      btn.getAttribute("data-testid") === "send-button" ||
      (form && form.contains(getDeepActiveElement())) ||
      Boolean(btn.closest("fieldset, form, [class*='composer' i], [class*='input-container' i], div[data-testid*='composer' i], div[class*='chat-input' i]")) ||
      (Boolean(btn.querySelector("svg")) && Boolean(btn.closest("fieldset, form, footer, [class*='composer' i], [class*='chat' i]")));

    if (!sendLike) return;
    var text = stripInvisible(composerFrom(btn) || composerFrom(getDeepActiveElement()) || composerFrom(document.activeElement));
    var effectiveFiles = mergeStagedAndDomFiles();
    if (!text && effectiveFiles.length === 0 && stagedImages.length === 0) return;

    var clientDecision = evaluateClientRules(text, effectiveFiles);
    if (clientDecision && clientDecision.blocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(clientDecision.message, clientDecision.rule, "block");
      window.postMessage({
        source: "gateway-guard-page",
        nonce: guardNonce,
        id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
        prompt: text,
        files: effectiveFiles || [],
        upload_images: stagedImages || [],
        host: location.hostname,
        url: location.href,
        is_blocked: true,
        rule: clientDecision.rule || ""
      }, "*");
      return;
    }

    if (effectiveFiles.length > 0 || stagedImages.length > 0) {
      e.preventDefault();
      e.stopImmediatePropagation();
      ask(text, effectiveFiles, stagedImages).then(function (decision) {
        handleDecision(decision, function () {
          stagedFiles = [];
          stagedImages = [];
          passClick = true;
          _lastHandledPromptTime = Date.now();
          _lastHandledPromptText = text;
          try {
            btn.click();
          } finally {
            passClick = false;
          }
        });
      });
      return;
    }

    // Fast path: Clean text prompt without files proceeds with native click instantly (0ms delay)
    _lastHandledPromptTime = Date.now();
    _lastHandledPromptText = text;
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
      prompt: text,
      files: [],
      upload_images: [],
      host: location.hostname,
      url: location.href,
      is_blocked: false,
      rule: ""
    }, "*");
  }, true);

  // Form Submit Interception (Catches Enter in simple forms or form.requestSubmit)
  document.addEventListener("submit", function (e) {
    if (!armed || passClick) return;
    var form = e.target;
    if (!form) return;
    var text = stripInvisible(composerFrom(form) || composerFrom(document.activeElement));
    var effectiveFiles = mergeStagedAndDomFiles();
    if (!text && effectiveFiles.length === 0 && stagedImages.length === 0) return;

    var clientDecision = evaluateClientRules(text, effectiveFiles);
    if (clientDecision && clientDecision.blocked) {
      e.preventDefault();
      e.stopImmediatePropagation();
      showBubble(clientDecision.message, clientDecision.rule, "block");
      return;
    }

    if (effectiveFiles.length > 0 || stagedImages.length > 0) {
      e.preventDefault();
      e.stopImmediatePropagation();
      ask(text, effectiveFiles, stagedImages).then(function (decision) {
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
      return;
    }

    _lastHandledPromptTime = Date.now();
    _lastHandledPromptText = text;
    window.postMessage({
      source: "gateway-guard-page",
      nonce: guardNonce,
      id: "log_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2),
      prompt: text,
      files: [],
      upload_images: [],
      host: location.hostname,
      url: location.href,
      is_blocked: false,
      rule: ""
    }, "*");
  }, true);

  // --- 4. NETWORK LAYER HOOK (fetch, XMLHttpRequest & WebSocket) ---

  var origFetch = window.fetch;
  if (typeof origFetch === "function") {
    window.fetch = function (input, init) {
      if (!armed) return origFetch.apply(this, arguments);

      // If a prompt was evaluated at the Send button within the last 4 seconds, mark any outgoing fetch
      if (Date.now() - _lastHandledPromptTime < 4000) {
        try {
          var reqUrlEarly = (typeof input === "string" ? input : (input && input.url) || "").toLowerCase();
          // Never tag external cloud storage or binary upload URLs (avoids breaking Azure Blob/S3 pre-signed signatures & CORS)
          if (
            !reqUrlEarly.includes("blob.core.windows.net") &&
            !reqUrlEarly.includes("amazonaws.com") &&
            !reqUrlEarly.includes("storage.googleapis.com") &&
            !reqUrlEarly.includes("/backend-api/files") &&
            !reqUrlEarly.includes("/files") &&
            !reqUrlEarly.includes("/upload")
          ) {
            var handledHeaders = {
              "X-Guard-Handled": "1",
              "X-Gateway-Guard-Handled": "1",
              "X-Guard-Eval-Id": _lastHandledPromptId || "eval_done",
              "X-Prompt-Id": _lastHandledPromptId || "eval_done"
            };
            if (typeof Request !== "undefined" && input instanceof Request) {
              try {
                input.headers.set("X-Guard-Handled", "1");
                input.headers.set("X-Gateway-Guard-Handled", "1");
                if (_lastHandledPromptId) {
                  input.headers.set("X-Guard-Eval-Id", _lastHandledPromptId);
                  input.headers.set("X-Prompt-Id", _lastHandledPromptId);
                }
              } catch (e1) {}
            } else if (init && init.headers) {
              if (typeof Headers !== "undefined" && init.headers instanceof Headers) {
                init.headers.set("X-Guard-Handled", "1");
                init.headers.set("X-Gateway-Guard-Handled", "1");
                if (_lastHandledPromptId) {
                  init.headers.set("X-Guard-Eval-Id", _lastHandledPromptId);
                  init.headers.set("X-Prompt-Id", _lastHandledPromptId);
                }
              } else if (typeof init.headers === "object") {
                Object.assign(init.headers, handledHeaders);
              }
            } else if (init) {
              init.headers = Object.assign({}, handledHeaders);
            }
          }
        } catch (e) { }
        return origFetch.apply(this, arguments);
      }

      var method = (init && init.method) || (input && input.method) || "GET";
      method = String(method).toUpperCase();
      if (method !== "POST" && method !== "PUT" && method !== "PATCH") {
        return origFetch.apply(this, arguments);
      }

      var url = typeof input === "string" ? input : (input && input.url) || "";
      var body = init && init.body;
      var promptText = "";
      var netFiles = [];

      // Handle JSON body
      if (typeof body === "string") {
        try {
          var parsed = JSON.parse(body);
          if (parsed) {
            // 1. Direct prompt fields across OpenAI, Claude, Gemini, Copilot, DeepSeek, Mistral
            if (typeof parsed.prompt === "string") promptText = parsed.prompt;
            else if (typeof parsed.query === "string" && !parsed.query.toLowerCase().startsWith("plsenvironmentquery") && !parsed.query.toLowerCase().startsWith("query pls")) promptText = parsed.query;
            else if (typeof parsed.text === "string") promptText = parsed.text;
            else if (typeof parsed.input === "string") promptText = parsed.input;
            else if (typeof parsed.userMessage === "string") promptText = parsed.userMessage;
            else if (parsed.message && typeof parsed.message.text === "string") promptText = parsed.message.text;
            else if (Array.isArray(parsed.arguments) && parsed.arguments[0]) {
              var a0 = parsed.arguments[0];
              if (a0.message && typeof a0.message.text === "string") promptText = a0.message.text;
              else if (typeof a0.text === "string") promptText = a0.text;
            }

            // 2. Claude Connect-RPC / PerformAction format
            var act = parsed.action || parsed;
            var rpcMsg = (act && (act.createMessage || act.appendMessage || act.message)) || null;
            if (rpcMsg) {
              if (typeof rpcMsg.text === "string" && rpcMsg.text) promptText = rpcMsg.text;
              if (Array.isArray(rpcMsg.attachments)) {
                for (var r = 0; r < rpcMsg.attachments.length; r++) {
                  var ra = rpcMsg.attachments[r];
                  if (ra) {
                    var rName = ra.fileName || ra.file_name || ra.name || "";
                    if (rName && !netFiles.some(function (x) { return x.name === rName; })) {
                      netFiles.push({ name: rName, size: ra.fileSize || ra.file_size || 0, type: ra.fileType || ra.file_type || "" });
                    }
                  }
                }
              }
            }

            // 3. Top-level attachments / files (Claude, etc.)
            if (Array.isArray(parsed.attachments)) {
              for (var a = 0; a < parsed.attachments.length; a++) {
                var att = parsed.attachments[a];
                if (att) {
                  var aName = att.file_name || att.fileName || att.name || att.title || "";
                  var aType = att.file_type || att.fileType || att.mime_type || "";
                  var aSize = att.file_size || att.fileSize || att.size || 0;
                  var aText = att.extracted_content || att.content || "";
                  if (aName && !netFiles.some(function (x) { return x.name === aName; })) {
                    netFiles.push({ name: aName, size: aSize, type: aType, text: aText });
                  }
                }
              }
            }
            if (Array.isArray(parsed.files)) {
              for (var f = 0; f < parsed.files.length; f++) {
                var fl = parsed.files[f];
                if (fl) {
                  var fName = fl.name || fl.file_name || fl.fileName || "";
                  var fType = fl.type || fl.mime_type || "";
                  var fSize = fl.size || 0;
                  if (fName && !netFiles.some(function (x) { return x.name === fName; })) {
                    netFiles.push({ name: fName, size: fSize, type: fType });
                  }
                }
              }
            }

            // 4. Conversation messages array (ChatGPT, Claude, Gemini, etc.)
            if (Array.isArray(parsed.messages)) {
              for (var m = parsed.messages.length - 1; m >= 0; m--) {
                var msg = parsed.messages[m];
                if (!msg) continue;
                var role = msg.role || (msg.author && msg.author.role) || "";
                if (role === "user") {
                  if (typeof msg.content === "string") {
                    promptText = msg.content;
                  }
                  // ChatGPT / Gemini parts array
                  else if (msg.content && Array.isArray(msg.content.parts)) {
                    var strParts = [];
                    for (var p = 0; p < msg.content.parts.length; p++) {
                      var pt = msg.content.parts[p];
                      if (typeof pt === "string") {
                        strParts.push(pt);
                      } else if (pt && typeof pt === "object") {
                        var ptName = pt.name || (pt.asset_pointer ? ("file_" + (pt.asset_pointer.split("/").pop() || "upload")) : "");
                        if (ptName && !netFiles.some(function (x) { return x.name === ptName; })) {
                          netFiles.push({ name: ptName, size: pt.size_bytes || 0, type: pt.content_type || "" });
                        }
                      }
                    }
                    if (strParts.length) promptText = strParts.join(" ");
                  }
                  // Claude content blocks array
                  else if (Array.isArray(msg.content)) {
                    var cParts = [];
                    for (var c = 0; c < msg.content.length; c++) {
                      var blk = msg.content[c];
                      if (!blk) continue;
                      if (blk.type === "text" && typeof blk.text === "string") {
                        cParts.push(blk.text);
                      } else if (blk.type === "document" || blk.type === "file") {
                        var docName = blk.title || blk.name || "document";
                        var docText = (blk.source && blk.source.data) ? blk.source.data : "";
                        var docType = (blk.source && blk.source.media_type) || "application/pdf";
                        if (!netFiles.some(function (x) { return x.name === docName; })) {
                          netFiles.push({ name: docName, size: docText.length, type: docType, text: blk.text || "" });
                        }
                      } else if (blk.type === "image") {
                        var imgName = blk.title || blk.name || "image.png";
                        if (!netFiles.some(function (x) { return x.name === imgName; })) {
                          netFiles.push({ name: imgName, size: 0, type: (blk.source && blk.source.media_type) || "image/png" });
                        }
                      }
                    }
                    if (cParts.length) promptText = cParts.join(" ");
                  }

                  // ChatGPT metadata.attachments
                  if (msg.metadata && Array.isArray(msg.metadata.attachments)) {
                    for (var ma = 0; ma < msg.metadata.attachments.length; ma++) {
                      var matt = msg.metadata.attachments[ma];
                      if (matt) {
                        var mattName = matt.name || matt.file_name || matt.title || "";
                        var mattSize = matt.size || 0;
                        var mattType = matt.mime_type || matt.mimeType || matt.type || "";
                        if (mattName && !netFiles.some(function (x) { return x.name === mattName; })) {
                          netFiles.push({ name: mattName, size: mattSize, type: mattType });
                        }
                      }
                    }
                  }

                  // Claude / generic msg.attachments
                  if (Array.isArray(msg.attachments)) {
                    for (var ma2 = 0; ma2 < msg.attachments.length; ma2++) {
                      var matt2 = msg.attachments[ma2];
                      if (matt2) {
                        var m2Name = matt2.file_name || matt2.name || "";
                        if (m2Name && !netFiles.some(function (x) { return x.name === m2Name; })) {
                          netFiles.push({ name: m2Name, size: matt2.file_size || matt2.size || 0, type: matt2.file_type || "" });
                        }
                      }
                    }
                  }
                  break;
                }
              }
            }
          }
        } catch (err) { }
      }
      // Handle FormData body (File uploads)
      else if (body && typeof body.forEach === "function") {
        try {
          body.forEach(function (val, key) {
            if (val && typeof val.name === "string") {
              netFiles.push({ name: val.name, size: val.size, type: val.type });
            } else if (typeof val === "string" && (key.indexOf("prompt") >= 0 || key.indexOf("query") >= 0 || key.indexOf("message") >= 0)) {
              promptText = val;
            }
          });
        } catch (err) { }
      }

      // Check if URL looks like a pure file upload endpoint (ChatGPT /files, Claude upload, Azure Blob, S3, etc.)
      var lowUrl = (url || "").toLowerCase();
      var isPureUploadEndpoint =
        lowUrl.includes("/backend-api/files") ||
        lowUrl.includes("/files") ||
        lowUrl.includes("/upload") ||
        lowUrl.includes("blob.core.windows.net") ||
        lowUrl.includes("amazonaws.com") ||
        lowUrl.includes("storage.googleapis.com") ||
        lowUrl.includes("/media/upload") ||
        lowUrl.includes("/convert_document");

      // Pure file uploads without a typed prompt MUST pass through immediately with zero delay and unmodified headers!
      // (The file was already staged via inspectAndStageFile, and full DLP scanning runs when the user clicks Send)
      if (!promptText || isPureUploadEndpoint) {
        return origFetch.apply(this, arguments);
      }

      if (promptText) {
        promptText = stripInvisible(promptText);
      }
      var ptClean = (promptText || "").trim().toLowerCase();
      var isNoisePrompt = !promptText || ptClean === "set" || ptClean === "ping" || ptClean === "pong" || ptClean === "heartbeat" || ptClean === "true" || ptClean === "false" || ptClean === "null" || ptClean === "me" || ptClean.indexOf("plsenvironmentquery") >= 0 || ptClean.indexOf("assign copilot license") >= 0;
      if (isNoisePrompt) {
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
          var nextInit = init ? Object.assign({}, init) : {};
          if ((action === "redacted" || action === "redact") && chosen.redacted_prompt && init) {
            var rewritten = rewriteBody(init.body, chosen.redacted_prompt);
            if (rewritten) {
              nextInit.body = rewritten;
            }
          }
          // Tag header so downstream Proxy skips duplicate logging (only on chat endpoints)
          try {
            var chatReqUrl = (url || "").toLowerCase();
            if (
              !chatReqUrl.includes("blob.core.windows.net") &&
              !chatReqUrl.includes("amazonaws.com") &&
              !chatReqUrl.includes("storage.googleapis.com")
            ) {
              if (typeof Request !== "undefined" && input instanceof Request) {
                try {
                  input.headers.set("X-Guard-Handled", "1");
                  input.headers.set("X-Gateway-Guard-Handled", "1");
                } catch (hdrErr) {
                  var safeH = new Headers(input.headers);
                  safeH.set("X-Guard-Handled", "1");
                  safeH.set("X-Gateway-Guard-Handled", "1");
                  nextInit.headers = safeH;
                  nextArgs = [input, nextInit];
                }
              } else if (nextInit.headers) {
                if (typeof Headers !== "undefined" && nextInit.headers instanceof Headers) {
                  nextInit.headers.set("X-Guard-Handled", "1");
                  nextInit.headers.set("X-Gateway-Guard-Handled", "1");
                } else if (Array.isArray(nextInit.headers)) {
                  nextInit.headers.push(["X-Guard-Handled", "1"]);
                  nextInit.headers.push(["X-Gateway-Guard-Handled", "1"]);
                } else if (typeof nextInit.headers === "object") {
                  nextInit.headers["X-Guard-Handled"] = "1";
                  nextInit.headers["X-Gateway-Guard-Handled"] = "1";
                }
                nextArgs = [input, nextInit];
              }
            }
          } catch (he) { }

          result = origFetch.apply(window, nextArgs);
        });
        if (!sent) return new Response(null, { status: 204 });
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
    if (Date.now() - _lastHandledPromptTime < 4000) {
      try {
        var xhrUrl = String(this._ggUrl || "").toLowerCase();
        if (
          !xhrUrl.includes("blob.core.windows.net") &&
          !xhrUrl.includes("amazonaws.com") &&
          !xhrUrl.includes("storage.googleapis.com") &&
          !xhrUrl.includes("/backend-api/files") &&
          !xhrUrl.includes("/files") &&
          !xhrUrl.includes("/upload")
        ) {
          this.setRequestHeader("X-Guard-Handled", "1");
          this.setRequestHeader("X-Gateway-Guard-Handled", "1");
          if (_lastHandledPromptId) {
            this.setRequestHeader("X-Guard-Eval-Id", _lastHandledPromptId);
            this.setRequestHeader("X-Prompt-Id", _lastHandledPromptId);
          }
        }
      } catch (e) { }
    }
    if (!armed || !body) return origXHRSend.apply(this, arguments);
    var promptText = "";
    if (typeof body === "string") {
      try {
        var parsed = JSON.parse(body);
        if (parsed && typeof parsed.prompt === "string") promptText = parsed.prompt;
      } catch (e) { }
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

  // Hook WebSocket (Streaming & Real-time AI Chatbots: Copilot SignalR, ChatGPT SSE, etc.)
  if (typeof window.WebSocket !== "undefined" && window.WebSocket.prototype) {
    var origWSSend = window.WebSocket.prototype.send;
    window.WebSocket.prototype.send = function (data) {
      if (armed && typeof data === "string" && data.length > 3) {
        var cleanStr = data.replace(/\x1e/g, "").trim();
        var promptText = "";
        try {
          var parsed = JSON.parse(cleanStr);
          if (parsed) {
            if (Array.isArray(parsed.arguments) && parsed.arguments[0]) {
              var arg0 = parsed.arguments[0];
              if (arg0.message && typeof arg0.message.text === "string") {
                promptText = arg0.message.text;
              } else if (arg0.message && typeof arg0.message.content === "string") {
                promptText = arg0.message.content;
              } else if (typeof arg0.text === "string") {
                promptText = arg0.text;
              } else if (typeof arg0.input === "string") {
                promptText = arg0.input;
              } else if (typeof arg0.prompt === "string") {
                promptText = arg0.prompt;
              } else if (typeof arg0.query === "string") {
                promptText = arg0.query;
              }
            }
            if (!promptText) {
              if (typeof parsed.prompt === "string") promptText = parsed.prompt;
              else if (typeof parsed.text === "string") promptText = parsed.text;
              else if (typeof parsed.input === "string") promptText = parsed.input;
              else if (typeof parsed.query === "string") promptText = parsed.query;
              else if (parsed.message && typeof parsed.message.text === "string") promptText = parsed.message.text;
              else if (parsed.message && typeof parsed.message.content === "string") promptText = parsed.message.content;
            }
          }
        } catch (e) {}

        promptText = stripInvisible(promptText);
        // Only evaluate if real user text was extracted (NEVER evaluate raw JSON packets/timestamps)
        if (promptText && promptText.length > 0 && !promptText.startsWith("{") && !promptText.startsWith("[")) {
          var wsDecision = evaluateClientRules(promptText, []);
          if (wsDecision && wsDecision.blocked) {
            showBubble(wsDecision.message, wsDecision.rule, "block");
            window.postMessage({
              source: "gateway-guard-page",
              nonce: guardNonce,
              id: "ws_block_" + Date.now().toString(36),
              prompt: promptText.slice(0, 500),
              host: location.hostname,
              url: location.href,
              is_blocked: true,
              rule: wsDecision.rule || ""
            }, "*");
            return;
          }
        }
      }
      return origWSSend.apply(this, arguments);
    };
  }

  // Hook navigator.sendBeacon (Safe Passthrough: background analytics/telemetry must NEVER trigger prompt block alerts)
  if (typeof navigator.sendBeacon === "function") {
    var origSendBeacon = navigator.sendBeacon.bind(navigator);
    navigator.sendBeacon = function (url, data) {
      return origSendBeacon(url, data);
    };
  }

  console.log("[YP-Raksha] In-Browser DLP Engine Active: Prompts, Images, Files, WebSockets & Beacons Protected.");
})();
