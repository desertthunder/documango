//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

// TestScreenshots saves full-page screenshots for people to review. They are
// not compared against anything.
func TestScreenshots(t *testing.T) {
	t.Parallel()
	site := serve(t, nil, "--dark-theme", "tomorrow-night,github-dark", "--light-theme", "tomorrow,catppuccin-latte")
	dir := screenshotDir(t)
	desktop := func(scheme *playwright.ColorScheme) playwright.BrowserNewContextOptions {
		return playwright.BrowserNewContextOptions{Viewport: &playwright.Size{Width: 1280, Height: 800}, ColorScheme: scheme}
	}
	phone := mobile
	phone.ColorScheme = playwright.ColorSchemeLight

	shots := []struct {
		name string
		opts playwright.BrowserNewContextOptions
		act  func(playwright.Page) error
	}{
		{"desktop-light", desktop(playwright.ColorSchemeLight), nil},
		{"desktop-dark", desktop(playwright.ColorSchemeDark), nil},
		{"mobile", phone, nil},
		{"mobile-menu-open", phone, func(p playwright.Page) error {
			if err := p.Locator(".menu-toggle").Click(); err != nil {
				return err
			}
			return expect.Locator(p.Locator("#sidebar")).ToBeVisible()
		}},
		{"mobile-search-open", phone, func(p playwright.Page) error {
			if err := p.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Search", Exact: playwright.Bool(true)}).Click(); err != nil {
				return err
			}
			if err := p.Locator("#search-input").PressSequentially("acme"); err != nil {
				return err
			}
			return expect.Locator(p.GetByRole("listbox", playwright.PageGetByRoleOptions{Name: "Search results"})).ToBeVisible()
		}},
		{"search-open", desktop(playwright.ColorSchemeLight), func(p playwright.Page) error {
			if err := p.Locator("#search-input").PressSequentially("acme"); err != nil {
				return err
			}
			return expect.Locator(p.GetByRole("listbox", playwright.PageGetByRoleOptions{Name: "Search results"})).ToBeVisible()
		}},
	}
	for _, s := range shots {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			page := newPage(t, s.opts)
			site.open(t, page, "/guide/install/")
			if s.act != nil {
				check(t, s.act(page), s.name)
			}
			path := filepath.Join(dir, s.name+".png")
			_, err := page.Screenshot(playwright.PageScreenshotOptions{
				Path: playwright.String(path), FullPage: playwright.Bool(true), Animations: playwright.ScreenshotAnimationsDisabled,
			})
			check(t, err, "screenshot")
			t.Logf("saved %s", path)
		})
	}
}
