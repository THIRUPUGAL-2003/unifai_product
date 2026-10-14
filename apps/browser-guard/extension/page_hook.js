/* Page world: stop a blocked Send before the site starts its stream, and show the warning. */
(function () {
  var armed = false;
  var passKey = false;
  var passClick = false;
  var pending = {};

  window.addEventListener("message", function (ev) {
    if (ev.source !== window || !ev.data || ev.data.source !== "gateway-guard") return;
    if (ev.data.type === "arm") armed = true;
    if (ev.data.type === "decision" && pending[ev.data.id]) {
      pending[ev.data.id](ev.data.decision || { blocked: false });
      delete pending[ev.data.id];
    }
  });

  function ask(prompt) {
    return new Promise(function (resolve) {
      var id = Date.now().toString(36) + "_" + Math.random().toString(36).slice(2);
      var timer = setTimeout(function () {
        delete pending[id];
        resolve({ blocked: false, passthrough: true });
      }, 8000);
      pending[id] = function (decision) {
        clearTimeout(timer);
        resolve(decision || { blocked: false });
      };
      window.postMessage({
        source: "gateway-guard-page",
        id: id,
        prompt: prompt,
        host: location.hostname,
        url: location.href
      }, "*");
    });
  }

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
    var container = (el.closest && (el.closest("form") || el.closest("[class*='composer'], [class*='chat'], [class*='input'], [class*='prompt'], main"))) || document;
    var target = container.querySelector("textarea, [contenteditable='true'], [role='textbox']");
    if (target) {
      if (target.tagName === "TEXTAREA" || target.tagName === "INPUT") return target.value || "";
      if (target.isContentEditable) {
        var troot = (target.closest && target.closest("[contenteditable='true'], [role='textbox']")) || target;
        return troot.innerText || troot.textContent || "";
      }
    }
    return "";
  }

  function showBubble(message) {
    var text = (message || "This request was blocked by YP-Raksha.").trim();
    var box = document.getElementById("gateway-guard-reply");
    if (!box) {
      box = document.createElement("div");
      box.id = "gateway-guard-reply";
      box.setAttribute("role", "status");
      box.style.cssText = "position:fixed;left:50%;bottom:92px;transform:translateX(-50%);max-width:680px;width:min(680px,calc(100% - 32px));z-index:2147483646;background:#ffffff;color:#111827;border:1px solid #e5e7eb;border-radius:18px;padding:12px 16px;box-shadow:0 10px 30px rgba(0,0,0,.18);font:14px/1.45 Segoe UI,sans-serif";
      (document.documentElement || document.body).appendChild(box);
    }
    box.textContent = "";
    var label = document.createElement("div");
    label.textContent = "YP-Raksha";
    label.style.cssText = "font-size:11px;font-weight:700;letter-spacing:.02em;color:#b45309;margin-bottom:4px";
    var body = document.createElement("div");
    body.textContent = text;
    box.appendChild(label);
    box.appendChild(body);
  }

  function chatUrl(url) {
    var u = String(url || "").toLowerCase();
    return u.indexOf("completion") >= 0 || u.indexOf("conversation") >= 0 || u.indexOf("streamgenerate") >= 0 ||
      u.indexOf("perplexity_ask") >= 0 || u.indexOf("/api/chat") >= 0 || u.indexOf("app-chat") >= 0 ||
      u.indexOf("generatecontent") >= 0 || u.indexOf("/messages") >= 0 || u.indexOf("/chat") >= 0 ||
      u.indexOf("backend-api") >= 0;
  }

  function promptFromBody(body) {
    if (!body || typeof body !== "string") return "";
    var raw = body.length > 200000 ? body.slice(0, 200000) : body;
    try {
      var data = JSON.parse(raw);
      if (data && typeof data.prompt === "string") return data.prompt.trim();
      if (data && typeof data.query_str === "string") return data.query_str.trim();
      if (data && typeof data.message === "string" && data.message.length < 8000) return data.message.trim();
      if (data && Array.isArray(data.messageInput)) {
        var joined = data.messageInput.map(function (x) { return x && x.text ? x.text : ""; }).join("\n").trim();
        if (joined) return joined;
      }
      if (data && Array.isArray(data.messages)) {
        for (var i = data.messages.length - 1; i >= 0; i--) {
          var m = data.messages[i];
          if (!m) continue;
          var role = (m.author && m.author.role) || m.role || "";
          if (role && String(role).toLowerCase() !== "user") continue;
          var parts = m.content && m.content.parts;
          if (Array.isArray(parts) && typeof parts[0] === "string") return parts[0].trim();
          if (typeof m.content === "string") return m.content.trim();
        }
      }
    } catch (e) {}
    return "";
  }

  document.addEventListener("keydown", function (e) {
    if (!armed || passKey) return;
    if (e.key !== "Enter" || e.shiftKey || e.altKey || e.ctrlKey || e.metaKey || e.isComposing) return;
    var text = composerFrom(e.target).trim();
    if (!text || text.length > 8000) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    var target = e.target;
    ask(text).then(function (decision) {
      if (decision && decision.blocked) {
        showBubble(decision.message || "This request was blocked by YP-Raksha.");
        return;
      }
      passKey = true;
      try {
        target.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true, cancelable: true, keyCode: 13, which: 13 }));
      } finally {
        passKey = false;
      }
    });
  }, true);

  document.addEventListener("click", function (e) {
    if (!armed || passClick) return;
    var btn = e.target && e.target.closest ? e.target.closest("button,[role='button']") : null;
    if (!btn) return;
    var label = ((btn.getAttribute("aria-label") || "") + " " + (btn.getAttribute("data-testid") || "") + " " + (btn.title || "")).toLowerCase();
    var form = btn.closest ? btn.closest("form") : null;
    var sendLike = label.indexOf("send") >= 0 || label.indexOf("submit") >= 0 || (form && form.contains(document.activeElement));
    if (!sendLike) return;
    var text = (composerFrom(btn) || composerFrom(document.activeElement)).trim();
    if (!text || text.length > 8000) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    ask(text).then(function (decision) {
      if (decision && decision.blocked) {
        showBubble(decision.message || "This request was blocked by YP-Raksha.");
        return;
      }
      passClick = true;
      try {
        btn.click();
      } finally {
        passClick = false;
      }
    });
  }, true);

  var origFetch = window.fetch;
  if (typeof origFetch === "function") {
    window.fetch = function (input, init) {
      if (!armed) return origFetch.apply(this, arguments);
      var url = typeof input === "string" ? input : (input && input.url) || "";
      var body = init && typeof init.body === "string" ? init.body : "";
      if (!body && input && typeof input !== "string") {
        try { body = ""; } catch (e) { body = ""; }
      }
      var prompt = chatUrl(url) ? promptFromBody(body) : "";
      if (!prompt) return origFetch.apply(this, arguments);
      var args = arguments;
      return ask(prompt).then(function (decision) {
        if (decision && decision.blocked) {
          showBubble(decision.message || "This request was blocked by YP-Raksha.");
          return new Response("", { status: 204, headers: { "content-type": "text/plain" } });
        }
        return origFetch.apply(window, args);
      });
    };
  }
})();
