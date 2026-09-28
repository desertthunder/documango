//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

// latinRange is the unicode-range of Inter's latin subset on Fontsource.
const latinRange = "U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD"

// seedInter writes Inter's latin variable font into a documango font cache
// in cache, so the site gets the font without the network. The font and its
// license in testdata come from Fontsource (@fontsource-variable/inter 5.3.0).
func seedInter(t *testing.T, cache string) {
	t.Helper()
	dir := filepath.Join(cache, "fonts", "inter")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]any{}
	for name, src := range map[string]string{
		"inter-latin-wght-normal.woff2": "testdata/inter-latin-wght-normal.woff2",
		"LICENSE.txt":                   "testdata/LICENSE-inter.txt",
	} {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		files[name] = map[string]string{"sha256": hex.EncodeToString(sum[:])}
	}
	meta, _ := json.Marshal(map[string]any{
		"id": "inter", "family": "Inter", "version": "5.3.0", "files": files,
		"faces": []map[string]any{{"style": "normal", "weight": []int{100, 900}, "unicodeRange": latinRange, "file": "inter-latin-wght-normal.woff2"}},
	})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFonts(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	seedInter(t, cache)
	config := filepath.Join(t.TempDir(), "documango.toml")
	if err := os.WriteFile(config, []byte("[fonts]\nbody = \"Inter\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := serve(t, []string{"DOCUMANGO_CACHE_DIR=" + cache}, "--config", config)
	siteURL, err := url.Parse(site.URL)
	check(t, err, "parse site URL")

	page := newPage(t)
	errs := watchErrors(page)
	var mu sync.Mutex
	var requests []string
	page.OnRequest(func(r playwright.Request) {
		mu.Lock()
		requests = append(requests, r.URL())
		mu.Unlock()
	})
	site.open(t, page, "/guide/install/")

	loaded, err := page.Evaluate(`async () => {
		await document.fonts.ready;
		const inter = [...document.fonts].filter(f => f.family.replace(/"/g, "") === "Inter");
		return document.fonts.check('16px "Inter"') && inter.length > 0 && inter.every(f => f.status === "loaded");
	}`)
	check(t, err, "wait for fonts")
	if loaded != true {
		t.Error("Inter is not loaded")
	}
	family, err := page.Locator("body").Evaluate("el => getComputedStyle(el).fontFamily", nil)
	check(t, err, "body font-family")
	if f, _ := family.(string); !strings.HasPrefix(f, `"Inter"`) && !strings.HasPrefix(f, "Inter") {
		t.Errorf("body font-family = %q", family)
	}

	mu.Lock()
	defer mu.Unlock()
	var woff2 bool
	for _, r := range requests {
		u, err := url.Parse(r)
		check(t, err, "parse request URL")
		if u.Host != siteURL.Host {
			t.Errorf("request to another host: %s", r)
		}
		woff2 = woff2 || strings.HasSuffix(u.Path, "/_documango/fonts/inter-latin-wght-normal.woff2")
	}
	if !woff2 {
		t.Errorf("font not requested from the site; requests: %q", requests)
	}
	if e := errs(); len(e) > 0 {
		t.Errorf("page errors:\n%s", strings.Join(e, "\n"))
	}
}
