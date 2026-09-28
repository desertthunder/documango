//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/desertthunder/documango/internal/pagefind"
)

var pages = []struct{ path, title, heading string }{
	{"/", "Acme Docs", "Acme Docs"},
	{"/reference/cli/", "CLI · Acme Docs", "CLI"},
	{"/reference/api/", "API · Acme Docs", "API"},
	{"/guide/", "Guide · Acme Docs", "Guide"},
	{"/guide/install/", "Install · Acme Docs", "Install"},
	{"/guide/configure/", "Configure · Acme Docs", "Configure"},
}

func TestPagesLoadCleanly(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t)
	errs := watchErrors(page)
	for _, p := range pages {
		site.open(t, page, p.path)
		check(t, expect.Page(page).ToHaveTitle(p.title), p.path+" title")
		check(t, expect.Locator(page.Locator("h1")).ToHaveText(p.heading), p.path+" h1")
		current := page.Locator(`[aria-current="page"]`)
		check(t, expect.Locator(current).ToHaveCount(1), p.path+" one aria-current link")
		check(t, expect.Locator(current).ToHaveAttribute("href", p.path), p.path+" aria-current link")
	}
	if e := errs(); len(e) > 0 {
		t.Errorf("page errors:\n%s", strings.Join(e, "\n"))
	}
}

func TestTableOfContents(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t, playwright.BrowserNewContextOptions{Viewport: &playwright.Size{Width: 1280, Height: 500}})
	site.open(t, page, "/guide/install/")

	links, err := page.GetByRole("navigation", playwright.PageGetByRoleOptions{Name: "On this page"}).GetByRole("link").All()
	check(t, err, "list toc links")
	if len(links) != 5 {
		t.Fatalf("toc has %d links, want 5", len(links))
	}
	last, err := links[len(links)-1].GetAttribute("href")
	check(t, err, "toc href")
	check(t, expect.Locator(page.Locator(last)).Not().ToBeInViewport(), "last heading starts below the fold")

	for _, link := range links {
		href, err := link.GetAttribute("href")
		check(t, err, "toc href")
		check(t, link.Click(), "click "+href)
		check(t, expect.Page(page).ToHaveURL(regexp.MustCompile(regexp.QuoteMeta(href)+"$")), "url after "+href)
		check(t, expect.Locator(page.Locator(href)).ToBeInViewport(), "heading "+href+" in view")
	}
}

func TestPager(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t)
	site.open(t, page, "/")
	check(t, expect.Locator(page.Locator("a[rel=prev]")).ToHaveCount(0), "no previous on home")

	for _, p := range pages[1:] {
		check(t, page.Locator("a[rel=next]").Click(), "click next towards "+p.path)
		check(t, expect.Page(page).ToHaveURL(site.URL+strings.TrimPrefix(p.path, "/")), "next")
		check(t, expect.Locator(page.Locator("h1")).ToHaveText(p.heading), "h1 after next")
	}
	check(t, expect.Locator(page.Locator("a[rel=next]")).ToHaveCount(0), "no next on last page")

	check(t, page.Locator("a[rel=prev]").Click(), "click previous")
	check(t, expect.Page(page).ToHaveURL(site.URL+strings.TrimPrefix(pages[len(pages)-2].path, "/")), "previous")
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t)
	resp, err := page.Goto(site.URL + "no/such/page/")
	check(t, err, "open missing page")
	if resp.Status() != 404 {
		t.Errorf("status = %d, want 404", resp.Status())
	}
	check(t, expect.Locator(page.Locator("h1")).ToHaveText("Page not found"), "404 heading")
	check(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Go to the home page"}).Click(), "follow home link")
	check(t, expect.Page(page).ToHaveURL(site.URL), "home after 404")
}

func TestColorSchemeFollowsSystem(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	for _, tt := range []struct {
		scheme *playwright.ColorScheme
		slug   string
	}{
		{playwright.ColorSchemeLight, "tomorrow"},
		{playwright.ColorSchemeDark, "tomorrow-night"},
	} {
		t.Run(tt.slug, func(t *testing.T) {
			t.Parallel()
			page := newPage(t, playwright.BrowserNewContextOptions{ColorScheme: tt.scheme})
			site.open(t, page, "/")
			check(t, expect.Locator(page.Locator("body")).ToHaveCSS("background-color", background(t, tt.slug)), "background")
		})
	}
}

func TestThemeToggle(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t, playwright.BrowserNewContextOptions{ColorScheme: playwright.ColorSchemeLight})
	site.open(t, page, "/")
	body := page.Locator("body")
	toggle := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Switch to dark theme"})

	check(t, toggle.Click(), "switch to dark")
	check(t, expect.Locator(body).ToHaveCSS("background-color", background(t, "tomorrow-night")), "dark background")

	_, err := page.Reload()
	check(t, err, "reload")
	check(t, expect.Locator(page.Locator("html")).ToHaveAttribute("data-theme", "dark"), "theme kept")
	check(t, expect.Locator(body).ToHaveCSS("background-color", background(t, "tomorrow-night")), "dark background after reload")

	check(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Switch to light theme"}).Click(), "switch to light")
	check(t, expect.Locator(body).ToHaveCSS("background-color", background(t, "tomorrow")), "light background")
}

func TestSchemeMenu(t *testing.T) {
	t.Parallel()
	site := serve(t, nil, "--dark-theme", "tomorrow-night,github-dark", "--light-theme", "tomorrow,catppuccin-latte")
	page := newPage(t, playwright.BrowserNewContextOptions{ColorScheme: playwright.ColorSchemeDark})
	site.open(t, page, "/")
	body := page.Locator("body")
	menu := page.GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Color scheme"})

	_, err := menu.SelectOption(playwright.SelectOptionValues{Values: &[]string{"github-dark"}})
	check(t, err, "choose github-dark")
	check(t, expect.Locator(body).ToHaveCSS("background-color", background(t, "github-dark")), "github-dark background")

	_, err = page.Reload()
	check(t, err, "reload")
	check(t, expect.Locator(menu).ToHaveValue("github-dark"), "menu keeps choice")
	check(t, expect.Locator(body).ToHaveCSS("background-color", background(t, "github-dark")), "background after reload")

	// Without app.js, only the inline script in <head> can restore the
	// choice, so it applies before the first paint.
	check(t, page.Route("**/_documango/app.js", func(r playwright.Route) { _ = r.Abort() }), "block app.js")
	_, err = page.Reload()
	check(t, err, "reload without app.js")
	check(t, expect.Locator(page.Locator("html")).ToHaveAttribute("data-dark-scheme", "github-dark"), "scheme attribute")
	check(t, expect.Locator(body).ToHaveCSS("background-color", background(t, "github-dark")), "background without app.js")
	check(t, expect.Locator(page.Locator(".scheme-menu")).ToBeHidden(), "menu stays hidden without app.js")
}

func TestSidebarSections(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t)
	site.open(t, page, "/guide/install/")

	section := func(title string) playwright.Locator {
		return page.Locator("details.sidebar-nav__section").Filter(playwright.LocatorFilterOptions{
			Has: page.Locator("summary", playwright.PageLocatorOptions{HasText: title}),
		})
	}
	guide, reference := section("Guide"), section("Reference")
	check(t, expect.Locator(guide).ToHaveJSProperty("open", true), "current section open")
	check(t, expect.Locator(reference).ToHaveJSProperty("open", false), "other section closed")

	summary := reference.Locator("summary")
	check(t, summary.Focus(), "focus Reference")
	check(t, summary.Press("Enter"), "Enter")
	check(t, expect.Locator(reference).ToHaveJSProperty("open", true), "Enter opens")
	check(t, expect.Locator(reference.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "CLI"})).ToBeVisible(), "section links shown")
	check(t, summary.Press(" "), "Space")
	check(t, expect.Locator(reference).ToHaveJSProperty("open", false), "Space closes")
}

func TestSearch(t *testing.T) {
	t.Parallel()
	t.Run("builtin", func(t *testing.T) {
		t.Parallel()
		testSearch(t, serve(t, nil), "**/_documango/search.json")
	})
	t.Run("pagefind", func(t *testing.T) {
		t.Parallel()
		bin := os.Getenv(pagefind.EnvBinary)
		if bin == "" {
			if cache, err := os.UserCacheDir(); err == nil {
				bin = filepath.Join(cache, "documango", "pagefind", pagefind.Version, "pagefind")
			}
		}
		if _, err := os.Stat(bin); bin == "" || err != nil {
			t.Skipf("pagefind not found; set %s or run documango once with --search pagefind to cache it", pagefind.EnvBinary)
		}
		testSearch(t, serve(t, []string{pagefind.EnvBinary + "=" + bin}, "--search", "pagefind"), "**/pagefind/pagefind.js")
	})
}

// testSearch runs the search flow; index is the file the engine loads.
func testSearch(t *testing.T, site *served, index string) {
	page := newPage(t)
	site.open(t, page, "/")
	input := page.GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Search"})
	results := page.GetByRole("listbox", playwright.PageGetByRoleOptions{Name: "Search results"})
	status := page.Locator("#search-status")

	resp, err := page.ExpectResponse(index, func() error { return page.Keyboard().Press("/") })
	check(t, err, "press / and load "+index)
	if !resp.Ok() {
		t.Fatalf("%s: status %d", resp.URL(), resp.Status())
	}
	check(t, expect.Locator(input).ToBeFocused(), "/ focuses search")

	check(t, input.PressSequentially("zeppelin"), "type query")
	check(t, expect.Locator(results).ToBeVisible(), "results shown")
	check(t, expect.Locator(input).ToHaveAttribute("aria-expanded", "true"), "expanded")
	check(t, expect.Locator(results.GetByRole("option")).ToHaveCount(1), "one result")
	check(t, expect.Locator(results.GetByRole("option")).ToContainText("Install"), "result title")
	check(t, expect.Locator(status).ToHaveText("1 result"), "live count")

	check(t, input.Press("Escape"), "Escape")
	check(t, expect.Locator(results).ToBeHidden(), "Escape closes results")
	check(t, expect.Locator(input).ToHaveAttribute("aria-expanded", "false"), "collapsed")

	check(t, input.Blur(), "blur")
	check(t, input.Focus(), "refocus")
	check(t, expect.Locator(results).ToBeVisible(), "results shown again")
	check(t, input.Press("ArrowDown"), "ArrowDown")
	option := results.GetByRole("option").First()
	check(t, expect.Locator(option).ToHaveAttribute("aria-selected", "true"), "option selected")
	check(t, expect.Locator(input).ToHaveAttribute("aria-activedescendant", "search-result-0"), "active descendant")
	check(t, input.Press("Enter"), "Enter")
	check(t, expect.Page(page).ToHaveURL(site.URL+"guide/install/"), "Enter opens the result")
}

func TestMobileMenu(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t, mobile)
	site.open(t, page, "/guide/install/")
	button := page.Locator(".menu-toggle")
	sidebar := page.Locator("#sidebar")
	main := page.Locator("#content")

	check(t, expect.Locator(sidebar).ToBeHidden(), "sidebar starts hidden")
	check(t, button.Click(), "open menu")
	check(t, expect.Locator(button).ToHaveAttribute("aria-expanded", "true"), "expanded")
	check(t, expect.Locator(sidebar).ToBeVisible(), "sidebar shown")
	check(t, expect.Locator(main).ToHaveJSProperty("inert", true), "main inert")
	check(t, expect.Locator(page.Locator(".skip-link")).ToHaveJSProperty("inert", true), "skip link inert")
	check(t, expect.Locator(sidebar.Locator(`a[aria-current="page"]`)).ToBeFocused(), "focus on current page link")

	check(t, page.Keyboard().Press("Escape"), "Escape")
	check(t, expect.Locator(button).ToHaveAttribute("aria-expanded", "false"), "collapsed")
	check(t, expect.Locator(button).ToBeFocused(), "focus back on the button")
	check(t, expect.Locator(main).ToHaveJSProperty("inert", false), "main no longer inert")
	check(t, expect.Locator(sidebar).ToBeHidden(), "sidebar hidden again")
}

func TestMobileNoHorizontalOverflow(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t, mobile)
	for _, path := range []string{"/", "/guide/install/"} {
		site.open(t, page, path)
		got, err := page.Evaluate(`() => [document.documentElement.scrollWidth, window.innerWidth]`)
		check(t, err, "measure "+path)
		w := got.([]any)
		if scroll, inner := w[0].(int), w[1].(int); scroll > inner {
			t.Errorf("%s: scrollWidth %d > innerWidth %d", path, scroll, inner)
		}
	}
}

func TestLiveReload(t *testing.T) {
	t.Parallel()
	site := serve(t, nil)
	page := newPage(t)
	events := "**" + eventsSuffix
	_, err := page.ExpectResponse(events, func() error {
		_, err := page.Goto(site.URL + "guide/configure/")
		return err
	})
	check(t, err, "open page with live reload")

	const good = "---\ntitle: Configure\norder: 2\n---\n%s\n"
	_, err = page.ExpectResponse(events, func() error {
		site.write(t, "guide/configure.md", strings.Replace(good, "%s", "Settings now live in acme.toml.", 1))
		return nil
	})
	check(t, err, "reload after edit")
	check(t, expect.Locator(page.GetByText("Settings now live in acme.toml.")).ToBeVisible(), "edited text")

	site.write(t, "guide/configure.md", "---\ntitle: [unclosed\n---\nBroken.\n")
	banner := page.GetByRole("alert")
	check(t, expect.Locator(banner).ToContainText("Build failed"), "error banner")

	_, err = page.ExpectResponse(events, func() error {
		site.write(t, "guide/configure.md", strings.Replace(good, "%s", "Settings are fixed again.", 1))
		return nil
	})
	check(t, err, "reload after fix")
	check(t, expect.Locator(page.GetByText("Settings are fixed again.")).ToBeVisible(), "fixed text")
	check(t, expect.Locator(banner).ToHaveCount(0), "banner gone")
}

func TestAccessibility(t *testing.T) {
	t.Parallel()
	site := serve(t, nil, "--dark-theme", "tomorrow-night,github-dark")
	page := newPage(t)
	paths := []string{"/no-such-page/"}
	for _, p := range pages {
		paths = append(paths, p.path)
	}
	for _, path := range paths {
		if _, err := page.Goto(site.URL + strings.TrimPrefix(path, "/")); err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		check(t, expect.Locator(page.GetByRole("main")).ToHaveCount(1), path+" one main")

		check(t, page.Keyboard().Press("Tab"), "Tab")
		skip := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Skip to content"})
		check(t, expect.Locator(skip).ToBeFocused(), path+" skip link first")
		check(t, page.Keyboard().Press("Enter"), "follow skip link")
		check(t, expect.Locator(page.GetByRole("main")).ToBeFocused(), path+" skip link focuses main")

		check(t, expect.Locator(page.Locator("img:not([alt]), img[alt='']")).ToHaveCount(0), path+" images have alt text")
		if path == "/" {
			check(t, expect.Locator(page.GetByRole("img", playwright.PageGetByRoleOptions{Name: "Acme logo"})).ToBeVisible(), "home logo")
		}
		buttons, err := page.GetByRole("button").All()
		check(t, err, "list buttons")
		for _, b := range buttons {
			check(t, expect.Locator(b).ToHaveAccessibleName(regexp.MustCompile(`\S`)), path+" button name")
		}
		controls, err := page.GetByRole("combobox").All()
		check(t, err, "list comboboxes")
		for _, c := range controls {
			check(t, expect.Locator(c).ToHaveAccessibleName(regexp.MustCompile(`\S`)), path+" combobox name")
		}
	}
}

// serveConfigured serves the fixture with a config file that sets a favicon,
// a logo and two header links.
func serveConfigured(t *testing.T) *served {
	t.Helper()
	config := filepath.Join(t.TempDir(), "documango.toml")
	body := `title = "Acme Documentation for Operators"
favicon = "logo.svg"
logo = "logo.svg"

[[links]]
title = "GitHub"
url = "https://github.com/acme/acme"

[[links]]
title = "Changelog"
url = "/guide/"
`
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return serve(t, nil, "--config", config)
}

func TestHeaderLinksAndLogo(t *testing.T) {
	t.Parallel()
	site := serveConfigured(t)
	page := newPage(t)
	errs := watchErrors(page)
	site.open(t, page, "/guide/install/")

	links := page.GetByRole("navigation", playwright.PageGetByRoleOptions{Name: "Site"})
	check(t, expect.Locator(links).ToBeVisible(), "header links visible")
	github := links.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "GitHub"})
	check(t, expect.Locator(github).ToHaveAttribute("rel", "noopener"), "external link rel")
	check(t, expect.Locator(links.Locator("[target]")).ToHaveCount(0), "links open in the same tab")
	check(t, links.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Changelog"}).Click(), "follow internal link")
	check(t, expect.Page(page).ToHaveURL(site.URL+"guide/"), "internal link")

	logo := page.Locator(".site-header__logo")
	check(t, expect.Locator(logo).ToBeVisible(), "logo visible")
	box, err := logo.BoundingBox()
	check(t, err, "logo box")
	header, err := page.Locator(".site-header").BoundingBox()
	check(t, err, "header box")
	if box.Height <= 0 || box.Height >= header.Height {
		t.Errorf("logo height %v, header height %v", box.Height, header.Height)
	}
	check(t, expect.Locator(page.Locator(".site-header__title")).ToHaveAccessibleName("Acme Documentation for Operators"), "title link name")

	icon := page.Locator(`link[rel="icon"]`)
	check(t, expect.Locator(icon).ToHaveAttribute("type", "image/svg+xml"), "favicon type")
	status, err := page.Evaluate(`() => fetch(document.querySelector('link[rel="icon"]').href).then(r => r.status)`)
	check(t, err, "fetch favicon")
	if status != 200 {
		t.Errorf("favicon status %v", status)
	}
	if e := errs(); len(e) > 0 {
		t.Errorf("page errors:\n%s", strings.Join(e, "\n"))
	}
}

func TestMobileHeaderLinks(t *testing.T) {
	t.Parallel()
	site := serveConfigured(t)
	page := newPage(t, mobile)
	site.open(t, page, "/guide/install/")

	check(t, expect.Locator(page.Locator(".site-header .site-links")).ToBeHidden(), "header links hidden")
	check(t, expect.Locator(page.Locator(".site-header__logo")).ToBeVisible(), "logo visible")
	got, err := page.Evaluate(`() => [document.documentElement.scrollWidth, window.innerWidth]`)
	check(t, err, "measure page")
	if w := got.([]any); w[0].(int) > w[1].(int) {
		t.Errorf("scrollWidth %d > innerWidth %d", w[0], w[1])
	}

	check(t, page.Locator(".menu-toggle").Click(), "open menu")
	links := page.Locator("#sidebar").GetByRole("navigation", playwright.LocatorGetByRoleOptions{Name: "Site"})
	github := links.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "GitHub"})
	check(t, expect.Locator(github).ToBeVisible(), "links in the menu")
	check(t, expect.Locator(github).ToHaveAttribute("rel", "noopener"), "menu link rel")
}
