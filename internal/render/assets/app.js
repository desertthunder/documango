// documango page behaviour: theme toggle, navigation menu, search, and live
// reload. Everything degrades to a working static page without this script.
(function () {
  "use strict";

  var root = document.documentElement;
  var base = root.getAttribute("data-base") || "/";
  var themeKey = "documango-theme";

  // Theme toggle.
  var darkQuery = window.matchMedia("(prefers-color-scheme: dark)");
  var themeButton = document.querySelector(".theme-toggle");

  function currentTheme() {
    var t = root.getAttribute("data-theme");
    if (t === "light" || t === "dark") return t;
    return darkQuery.matches ? "dark" : "light";
  }

  function syncThemeButton() {
    if (!themeButton) return;
    var t = currentTheme();
    themeButton.setAttribute("data-current", t);
    themeButton.setAttribute("aria-label", t === "dark" ? "Switch to light theme" : "Switch to dark theme");
  }

  if (themeButton) {
    themeButton.addEventListener("click", function () {
      var next = currentTheme() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try {
        localStorage.setItem(themeKey, next);
      } catch (e) {
        // Storage may be unavailable; the choice then lasts for this page only.
      }
      syncThemeButton();
    });
    if (darkQuery.addEventListener) darkQuery.addEventListener("change", syncThemeButton);
    syncThemeButton();
  }

  // Navigation menu on small screens.
  var menuButton = document.querySelector(".menu-toggle");
  var sidebar = document.getElementById("sidebar");

  function setMenu(open) {
    if (!menuButton || !sidebar) return;
    menuButton.setAttribute("aria-expanded", String(open));
    menuButton.setAttribute("aria-label", open ? "Hide navigation" : "Show navigation");
    sidebar.classList.toggle("site-sidebar--open", open);
  }

  function menuOpen() {
    return !!menuButton && menuButton.getAttribute("aria-expanded") === "true";
  }

  if (menuButton && sidebar) {
    menuButton.addEventListener("click", function () {
      setMenu(!menuOpen());
      if (menuOpen()) {
        var link = sidebar.querySelector("a[aria-current]") || sidebar.querySelector("a");
        if (link) link.focus();
      }
    });
    document.addEventListener("click", function (e) {
      if (menuOpen() && !sidebar.contains(e.target) && !menuButton.contains(e.target)) setMenu(false);
    });
  }

  // Search.
  var input = document.getElementById("search-input");
  var list = document.getElementById("search-results");
  var status = document.getElementById("search-status");
  var form = input && input.form;
  var index = null;
  var loading = null;
  var results = [];
  var active = -1;
  var maxResults = 10;

  function loadIndex() {
    if (!loading) {
      loading = fetch(base + "_documango/search.json")
        .then(function (r) {
          if (!r.ok) throw new Error("HTTP " + r.status);
          return r.json();
        })
        .then(function (data) {
          index = data.map(function (entry) {
            return {
              entry: entry,
              title: entry.title.toLowerCase(),
              headings: entry.headings.join("\n").toLowerCase(),
              text: entry.text.toLowerCase()
            };
          });
        })
        .catch(function () {
          loading = null;
          status.textContent = "Search is unavailable.";
        });
    }
    return loading;
  }

  function search(query) {
    var terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (!terms.length || !index) return [];
    var hits = [];
    index.forEach(function (doc, order) {
      var score = 0;
      for (var i = 0; i < terms.length; i++) {
        var t = terms[i];
        var s = 0;
        if (doc.title.indexOf(t) >= 0) s += 10;
        if (doc.headings.indexOf(t) >= 0) s += 4;
        if (doc.text.indexOf(t) >= 0) s += 1;
        if (!s) return;
        score += s;
      }
      hits.push({ doc: doc, score: score, order: order, terms: terms });
    });
    hits.sort(function (a, b) {
      return b.score - a.score || a.order - b.order;
    });
    return hits.slice(0, maxResults);
  }

  function snippet(hit) {
    var text = hit.doc.entry.text;
    var lower = hit.doc.text;
    var at = -1;
    hit.terms.forEach(function (t) {
      var i = lower.indexOf(t);
      if (i >= 0 && (at < 0 || i < at)) at = i;
    });
    if (at < 0) return text.slice(0, 140) + (text.length > 140 ? "…" : "");
    var start = Math.max(0, at - 50);
    var end = Math.min(text.length, at + 110);
    if (start > 0) {
      var space = text.indexOf(" ", start);
      if (space >= 0 && space < at) start = space + 1;
    }
    return (start > 0 ? "…" : "") + text.slice(start, end) + (end < text.length ? "…" : "");
  }

  function setActive(i) {
    var options = list.children;
    if (active >= 0 && options[active]) options[active].setAttribute("aria-selected", "false");
    active = i;
    if (active >= 0 && options[active]) {
      options[active].setAttribute("aria-selected", "true");
      options[active].scrollIntoView({ block: "nearest" });
      input.setAttribute("aria-activedescendant", options[active].id);
    } else {
      input.removeAttribute("aria-activedescendant");
    }
  }

  function close() {
    list.hidden = true;
    input.setAttribute("aria-expanded", "false");
    setActive(-1);
  }

  function show() {
    var query = input.value.trim();
    list.replaceChildren();
    active = -1;
    input.removeAttribute("aria-activedescendant");
    if (!query) {
      results = [];
      status.textContent = "";
      close();
      return;
    }
    if (!index) {
      loadIndex().then(function () {
        if (index) show();
      });
      return;
    }
    results = search(query);
    results.forEach(function (hit, i) {
      var li = document.createElement("li");
      li.className = "search__option";
      li.id = "search-result-" + i;
      li.setAttribute("role", "option");
      li.setAttribute("aria-selected", "false");
      var a = document.createElement("a");
      a.className = "search__link";
      a.href = hit.doc.entry.url;
      a.tabIndex = -1;
      var title = document.createElement("span");
      title.className = "search__title";
      title.textContent = hit.doc.entry.title;
      var text = document.createElement("span");
      text.className = "search__snippet";
      text.textContent = snippet(hit);
      a.append(title, text);
      li.append(a);
      li.addEventListener("mousemove", function () {
        if (active !== i) setActive(i);
      });
      list.append(li);
    });
    if (!results.length) {
      var empty = document.createElement("li");
      empty.className = "search__empty";
      empty.textContent = "No results";
      list.append(empty);
    }
    list.hidden = false;
    input.setAttribute("aria-expanded", "true");
    status.textContent =
      results.length === 0 ? "No results" : results.length === 1 ? "1 result" : results.length + " results";
  }

  if (input && list && status && form) {
    input.addEventListener("focus", function () {
      loadIndex();
      if (input.value.trim()) show();
    });
    input.addEventListener("input", show);
    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        if (!results.length) return;
        e.preventDefault();
        var step = e.key === "ArrowDown" ? 1 : -1;
        setActive((active + step + results.length) % results.length);
      } else if (e.key === "Enter") {
        e.preventDefault();
        var hit = results[active >= 0 ? active : 0];
        if (hit) location.href = hit.doc.entry.url;
      } else if (e.key === "Escape") {
        e.preventDefault();
        if (!list.hidden) {
          close();
        } else if (input.value) {
          input.value = "";
          show();
        } else {
          input.blur();
        }
      }
    });
    form.addEventListener("submit", function (e) {
      e.preventDefault();
    });
    document.addEventListener("click", function (e) {
      if (!form.contains(e.target)) close();
    });
  }

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && menuOpen()) {
      setMenu(false);
      menuButton.focus();
      return;
    }
    if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey || !input) return;
    var t = e.target;
    if (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return;
    e.preventDefault();
    input.focus();
    input.select();
  });

  // Live reload from the development server.
  var events = root.getAttribute("data-livereload");
  if (events && window.EventSource) {
    var source = new EventSource(events);
    source.addEventListener("reload", function () {
      location.reload();
    });
    source.addEventListener("error", function (e) {
      // Connection errors arrive as plain events without data; only build
      // errors sent by the server carry a message.
      if (typeof e.data !== "string") return;
      var message;
      try {
        message = JSON.parse(e.data);
      } catch (err) {
        message = e.data;
      }
      var banner = document.querySelector(".reload-banner");
      if (!banner) {
        banner = document.createElement("div");
        banner.className = "reload-banner";
        banner.setAttribute("role", "alert");
        var text = document.createElement("p");
        text.className = "reload-banner__message";
        var dismiss = document.createElement("button");
        dismiss.type = "button";
        dismiss.className = "reload-banner__close";
        dismiss.textContent = "Dismiss";
        dismiss.addEventListener("click", function () {
          banner.remove();
        });
        banner.append(text, dismiss);
        document.body.append(banner);
      }
      banner.querySelector(".reload-banner__message").textContent = "Build failed: " + message;
    });
  }
})();
