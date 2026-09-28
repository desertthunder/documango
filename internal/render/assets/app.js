// documango page behaviour: theme toggle and color scheme menu, navigation
// menu, search, and live reload. Everything degrades to a working static page without this script.
(function () {
  "use strict";

  var root = document.documentElement;
  var base = root.getAttribute("data-base") || "/";
  var themeKey = "documango-theme";

  // Theme toggle and color scheme menu.
  var darkQuery = window.matchMedia("(prefers-color-scheme: dark)");
  var themeButton = document.querySelector(".theme-toggle");
  var schemeMenu = document.querySelector(".scheme-menu");
  var schemeSelect = schemeMenu && schemeMenu.querySelector("select");

  function currentTheme() {
    var t = root.getAttribute("data-theme");
    if (t === "light" || t === "dark") return t;
    return darkQuery.matches ? "dark" : "light";
  }

  function store(key, value) {
    try {
      if (value) localStorage.setItem(key, value);
      else localStorage.removeItem(key);
    } catch (e) {
      // Storage may be unavailable; the choice then lasts for this page only.
    }
  }

  // Fills the scheme menu with the current mode's schemes. Each mode's choice
  // lives in data-dark-scheme or data-light-scheme; the default has none.
  function syncSchemeMenu() {
    if (!schemeSelect) return;
    var mode = currentTheme();
    var options = schemeMenu.querySelector('template[data-mode="' + mode + '"]');
    schemeSelect.replaceChildren(options.content.cloneNode(true));
    schemeMenu.hidden = schemeSelect.options.length < 2;
    var attr = "data-" + mode + "-scheme";
    schemeSelect.value = root.getAttribute(attr) || "";
    if (schemeSelect.selectedIndex <= 0) {
      schemeSelect.selectedIndex = 0;
      root.removeAttribute(attr);
    }
  }

  function syncThemeButton() {
    syncSchemeMenu();
    if (!themeButton) return;
    var t = currentTheme();
    themeButton.setAttribute("data-current", t);
    themeButton.setAttribute("aria-label", t === "dark" ? "Switch to light theme" : "Switch to dark theme");
  }

  if (themeButton) {
    themeButton.addEventListener("click", function () {
      var next = currentTheme() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      store(themeKey, next);
      syncThemeButton();
    });
  }
  if (schemeSelect) {
    schemeSelect.addEventListener("change", function () {
      var mode = currentTheme();
      var slug = schemeSelect.selectedIndex > 0 ? schemeSelect.value : "";
      if (slug) root.setAttribute("data-" + mode + "-scheme", slug);
      else root.removeAttribute("data-" + mode + "-scheme");
      store("documango-" + mode + "-scheme", slug);
    });
  }
  if (darkQuery.addEventListener) darkQuery.addEventListener("change", syncThemeButton);
  syncThemeButton();

  // Navigation menu on small screens. While it is open, everything but the
  // header and the menu itself is inert.
  var menuButton = document.querySelector(".menu-toggle");
  var sidebar = document.getElementById("sidebar");
  var narrowQuery = window.matchMedia("(max-width: 48rem)");

  function menuOpen() {
    return !!menuButton && menuButton.getAttribute("aria-expanded") === "true";
  }

  function setMenu(open) {
    if (!menuButton || !sidebar) return;
    menuButton.setAttribute("aria-expanded", String(open));
    menuButton.setAttribute("aria-label", open ? "Hide navigation" : "Show navigation");
    sidebar.classList.toggle("site-sidebar--open", open);
    var header = menuButton.closest("header");
    var others = Array.prototype.filter.call(document.body.children, function (el) {
      return el !== header && !el.contains(sidebar);
    });
    others = others.concat(
      Array.prototype.filter.call(sidebar.parentElement.children, function (el) {
        return el !== sidebar;
      })
    );
    others.forEach(function (el) {
      el.inert = open;
    });
  }

  if (menuButton && sidebar) {
    menuButton.addEventListener("click", function () {
      setMenu(!menuOpen());
      if (menuOpen()) {
        var link = sidebar.querySelector("a[aria-current]") || sidebar.querySelector("a, summary");
        if (link) link.focus();
      }
    });
    document.addEventListener("click", function (e) {
      if (menuOpen() && !sidebar.contains(e.target) && !menuButton.contains(e.target)) setMenu(false);
    });
    var onResize = function () {
      if (!narrowQuery.matches && menuOpen()) setMenu(false);
    };
    if (narrowQuery.addEventListener) narrowQuery.addEventListener("change", onResize);
  }

  // Search. A Pagefind bundle at the site root is used when present;
  // otherwise the built-in index in search.json.
  var input = document.getElementById("search-input");
  var list = document.getElementById("search-results");
  var status = document.getElementById("search-status");
  var form = input && input.form;
  var index = null;
  var loading = null;
  var engine = null;
  var pagefind = null;
  var results = [];
  var active = -1;
  var maxResults = 10;
  var pending = 0;
  var timer = 0;

  function loadEngine() {
    if (!engine) {
      engine = import(base + "pagefind/pagefind.js")
        .then(function (pf) {
          return Promise.resolve(pf.options({ baseUrl: base })).then(function () {
            pagefind = pf;
            if (pf.init) pf.init();
          });
        })
        .catch(loadIndex);
    }
    return engine;
  }

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
    return hits.slice(0, maxResults).map(function (hit) {
      return { url: hit.doc.entry.url, title: hit.doc.entry.title, snippet: document.createTextNode(snippet(hit)) };
    });
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

  // Copies the text and <mark> highlights of a Pagefind excerpt, which is
  // HTML, into a fragment. The markup is parsed in an inert document and no
  // other elements or attributes are kept.
  function excerpt(html) {
    var out = document.createDocumentFragment();
    function copy(from, to) {
      from.childNodes.forEach(function (node) {
        if (node.nodeType === Node.TEXT_NODE) {
          to.append(node.textContent);
        } else if (node.nodeType === Node.ELEMENT_NODE && node.tagName === "MARK") {
          var mark = document.createElement("mark");
          mark.textContent = node.textContent;
          to.append(mark);
        } else if (node.nodeType === Node.ELEMENT_NODE) {
          copy(node, to);
        }
      });
    }
    copy(new DOMParser().parseFromString(html || "", "text/html").body, out);
    return out;
  }

  function searchPagefind(query) {
    return pagefind.search(query).then(function (found) {
      return Promise.all(
        ((found && found.results) || []).slice(0, maxResults).map(function (r) {
          return r.data();
        })
      ).then(function (pages) {
        return pages.map(function (page) {
          return { url: page.url, title: (page.meta && page.meta.title) || page.url, snippet: excerpt(page.excerpt) };
        });
      });
    });
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

  function render(items) {
    list.replaceChildren();
    active = -1;
    input.removeAttribute("aria-activedescendant");
    results = items;
    results.forEach(function (item, i) {
      var li = document.createElement("li");
      li.className = "search__option";
      li.id = "search-result-" + i;
      li.setAttribute("role", "option");
      li.setAttribute("aria-selected", "false");
      var a = document.createElement("a");
      a.className = "search__link";
      a.href = item.url;
      a.tabIndex = -1;
      var title = document.createElement("span");
      title.className = "search__title";
      title.textContent = item.title;
      var text = document.createElement("span");
      text.className = "search__snippet";
      text.append(item.snippet);
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

  function show() {
    var query = input.value.trim();
    var run = ++pending;
    clearTimeout(timer);
    if (!query) {
      list.replaceChildren();
      results = [];
      status.textContent = "";
      close();
      return;
    }
    loadEngine().then(function () {
      if (run !== pending) return;
      if (!pagefind) {
        loadIndex().then(function () {
          if (run === pending && index) render(search(query));
        });
        return;
      }
      timer = setTimeout(function () {
        searchPagefind(query)
          .then(function (items) {
            if (run === pending) render(items);
          })
          .catch(function () {
            if (run === pending) status.textContent = "Search is unavailable.";
          });
      }, 150);
    });
  }

  if (input && list && status && form) {
    input.addEventListener("focus", function () {
      loadEngine();
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
        var item = results[active >= 0 ? active : 0];
        if (item) location.href = item.url;
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
