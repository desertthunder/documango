package fonts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fontsource fakes the Fontsource API and jsDelivr. API responses are
// trimmed copies of real ones in testdata; font files are tiny fakes that
// start with the WOFF2 signature.
type fontsource struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	// bodies overrides the response for a path; a nil body is a 404.
	bodies map[string][]byte
}

func newFontsource(t *testing.T) *fontsource {
	t.Helper()
	f := &fontsource{bodies: map[string][]byte{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fontsource) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path)
	body, override := f.bodies[r.URL.Path]
	f.mu.Unlock()
	notFound := func() {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"status":404,"error":"Not Found. Font does not exist."}`)
	}
	switch p := r.URL.Path; {
	case override:
		if body == nil {
			notFound()
			return
		}
		w.Write(body)
	case p == "/v1/fonts":
		http.ServeFile(w, r, "testdata/fonts.json")
	case strings.HasPrefix(p, "/v1/fonts/"), strings.HasPrefix(p, "/v1/variable/"):
		name := strings.TrimPrefix(p, "/v1/fonts/") + ".json"
		if v, ok := strings.CutPrefix(p, "/v1/variable/"); ok {
			name = v + "-variable.json"
		}
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			notFound()
			return
		}
		w.Write(b)
	case strings.HasPrefix(p, "/fontsource/fonts/"):
		io.WriteString(w, woff2(p))
	case strings.HasPrefix(p, "/npm/") && strings.HasSuffix(p, "/LICENSE"):
		io.WriteString(w, "SIL Open Font License "+p)
	default:
		notFound()
	}
}

// woff2 is the fake font served at path.
func woff2(path string) string { return "wOF2" + path }

// set overrides the response for path; nil makes it a 404.
func (f *fontsource) set(path string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[path] = body
}

// take returns and clears the requested paths.
func (f *fontsource) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.requests
	f.requests = nil
	return r
}

func (f *fontsource) loader(t *testing.T) *Loader {
	return &Loader{
		CacheDir: filepath.Join(t.TempDir(), "fonts"), APIURL: f.URL + "/v1", CDNURL: f.URL,
		Logger: slog.New(slog.DiscardHandler),
	}
}

const interCSS = `@font-face{font-family:"Inter";font-style:normal;font-weight:100 900;font-display:swap;src:url(fonts/inter-latin-ext-wght-normal.woff2) format("woff2");unicode-range:U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF}
@font-face{font-family:"Inter";font-style:normal;font-weight:100 900;font-display:swap;src:url(fonts/inter-latin-wght-normal.woff2) format("woff2");unicode-range:U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD}
@font-face{font-family:"Inter";font-style:italic;font-weight:100 900;font-display:swap;src:url(fonts/inter-latin-ext-wght-italic.woff2) format("woff2");unicode-range:U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF}
@font-face{font-family:"Inter";font-style:italic;font-weight:100 900;font-display:swap;src:url(fonts/inter-latin-wght-italic.woff2) format("woff2");unicode-range:U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD}
:root{--font-sans:"Inter",var(--font-system-sans);}
`

func TestLoadVariable(t *testing.T) {
	fs := newFontsource(t)
	l := fs.loader(t)
	res, err := l.Load(context.Background(), "Inter", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.CSS != interCSS {
		t.Errorf("CSS:\n%s\nwant:\n%s", res.CSS, interCSS)
	}
	wantFiles := map[string]string{
		"_documango/fonts/inter-latin-ext-wght-normal.woff2": woff2("/fontsource/fonts/inter:vf@5.3.0/latin-ext-wght-normal.woff2"),
		"_documango/fonts/inter-latin-wght-normal.woff2":     woff2("/fontsource/fonts/inter:vf@5.3.0/latin-wght-normal.woff2"),
		"_documango/fonts/inter-latin-ext-wght-italic.woff2": woff2("/fontsource/fonts/inter:vf@5.3.0/latin-ext-wght-italic.woff2"),
		"_documango/fonts/inter-latin-wght-italic.woff2":     woff2("/fontsource/fonts/inter:vf@5.3.0/latin-wght-italic.woff2"),
		"_documango/fonts/LICENSE-inter.txt":                 "SIL Open Font License /npm/@fontsource-variable/inter@5.3.0/LICENSE",
	}
	checkFiles(t, res, wantFiles)
	if reqs := fs.take(); len(reqs) != 7 || reqs[0] != "/v1/fonts/inter" || reqs[1] != "/v1/variable/inter" {
		t.Errorf("requests %q", reqs)
	}

	// The second load is served from the cache, with no network.
	fs.Close()
	again, err := l.Load(context.Background(), "inter", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.CSS != interCSS || !maps.EqualFunc(again.Files, res.Files, func(a, b []byte) bool { return string(a) == string(b) }) {
		t.Error("cached load differs from the first")
	}
}

func checkFiles(t *testing.T, res Result, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for k, v := range res.Files {
		got[k] = string(v)
	}
	if !maps.Equal(got, want) {
		t.Errorf("files:\n%q\nwant:\n%q", got, want)
	}
}

func faceFiles(css string) []string {
	var out []string
	for line := range strings.SplitSeq(css, "\n") {
		if _, rest, ok := strings.Cut(line, "url(fonts/"); ok {
			name, _, _ := strings.Cut(rest, ")")
			weight := line[strings.Index(line, "font-weight:")+12:]
			weight, _, _ = strings.Cut(weight, ";")
			out = append(out, weight+" "+name)
		}
	}
	return out
}

func TestLoadStatic(t *testing.T) {
	fs := newFontsource(t)
	l := fs.loader(t)
	res, err := l.Load(context.Background(), "Lato", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Lato has no 600, so the browser uses 700 for semibold text.
	want := []string{
		"400 lato-latin-ext-400-normal.woff2", "400 lato-latin-400-normal.woff2",
		"700 lato-latin-ext-700-normal.woff2", "700 lato-latin-700-normal.woff2",
		"400 lato-latin-ext-400-italic.woff2", "400 lato-latin-400-italic.woff2",
		"700 lato-latin-ext-700-italic.woff2", "700 lato-latin-700-italic.woff2",
	}
	if got := faceFiles(res.CSS); !slices.Equal(got, want) {
		t.Errorf("faces %q\nwant %q", got, want)
	}
	if len(res.Files) != 9 || string(res.Files["_documango/fonts/LICENSE-lato.txt"]) != "SIL Open Font License /npm/@fontsource/lato@5.3.0/LICENSE" {
		t.Errorf("files %v", slices.Sorted(maps.Keys(res.Files)))
	}
	if !slices.Contains(fs.take(), "/fontsource/fonts/lato@5.3.0/latin-700-italic.woff2") {
		t.Error("static file not fetched from the pinned version")
	}

	// Code text uses fewer weights; the cache holds them all.
	res, err = l.Load(context.Background(), "", "", "Lato")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{
		"400 lato-latin-ext-400-normal.woff2", "400 lato-latin-400-normal.woff2",
		"700 lato-latin-ext-700-normal.woff2", "700 lato-latin-700-normal.woff2",
		"400 lato-latin-ext-400-italic.woff2", "400 lato-latin-400-italic.woff2",
	}
	if got := faceFiles(res.CSS); !slices.Equal(got, want) {
		t.Errorf("mono faces %q\nwant %q", got, want)
	}
	if !strings.HasSuffix(res.CSS, `:root{--font-mono:"Lato",var(--font-system-mono);}`+"\n") || len(res.Files) != 7 {
		t.Errorf("mono CSS %q, %d files", res.CSS, len(res.Files))
	}
	if reqs := fs.take(); len(reqs) != 0 {
		t.Errorf("mono load made requests %q", reqs)
	}
}

func TestLoadNearestWeight(t *testing.T) {
	fs := newFontsource(t)
	fs.set("/v1/fonts/lato", []byte(`{"id":"lato","family":"Lato","weights":[100,300,900],"styles":["normal"],"variable":false,"npmVersion":"5.3.0",
		"unicodeRange":{"latin":"U+0000-00FF"},"variants":{"100":{"normal":{"latin":{}}},"300":{"normal":{"latin":{}}},"900":{"normal":{"latin":{}}}}}`))
	res, err := fs.loader(t).Load(context.Background(), "Lato", "", "Lato")
	if err != nil {
		t.Fatal(err)
	}
	if got := faceFiles(res.CSS); !slices.Equal(got, []string{"300 lato-latin-300-normal.woff2"}) {
		t.Errorf("faces %q", got)
	}
}

func TestLoadRoles(t *testing.T) {
	fs := newFontsource(t)
	l := fs.loader(t)
	res, err := l.Load(context.Background(), "Inter", "Lato", "Noto Sans JP")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := `:root{--font-sans:"Inter",var(--font-system-sans);--font-heading:"Lato",var(--font-system-sans);--font-mono:"Noto Sans JP",var(--font-system-mono);}` + "\n"
	if !strings.HasSuffix(res.CSS, wantRoot) {
		t.Errorf("CSS ends %q", res.CSS[strings.LastIndex(res.CSS, ":root"):])
	}
	// Fontsource numbers the chunks of large fonts: "[0]" is file key 0.
	// Faces keep the API's order.
	jp := faceFiles(res.CSS)[len(faceFiles(res.CSS))-3:]
	if want := []string{"100 900 noto-sans-jp-0-wght-normal.woff2", "100 900 noto-sans-jp-1-wght-normal.woff2", "100 900 noto-sans-jp-latin-wght-normal.woff2"}; !slices.Equal(jp, want) {
		t.Errorf("CJK faces %q", jp)
	}
	if !strings.Contains(res.CSS, "noto-sans-jp-0-wght-normal.woff2) format(\"woff2\");unicode-range:U+25ee8,") {
		t.Error("CJK unicode-range missing")
	}
	if _, ok := res.Files["_documango/fonts/LICENSE-noto-sans-jp.txt"]; !ok {
		t.Error("no CJK license")
	}

	// A heading that matches the body inherits it through tokens.css.
	res, err = l.Load(context.Background(), "Inter", "inter", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.CSS, "--font-heading") || strings.Count(res.CSS, "@font-face") != 4 {
		t.Errorf("CSS %q", res.CSS)
	}

	res, err = l.Load(context.Background(), "", "", "")
	if err != nil || res.CSS != "" || len(res.Files) != 0 {
		t.Errorf("no fonts: %+v, %v", res, err)
	}
}

func TestLoadUnknown(t *testing.T) {
	fs := newFontsource(t)
	l := fs.loader(t)
	_, err := l.Load(context.Background(), "Intr", "", "")
	if err == nil || err.Error() != `unknown font "Intr"; did you mean "Inter", "Inder"?` || errors.Is(err, ErrUnavailable) {
		t.Errorf("error %v", err)
	}
	_, err = l.Load(context.Background(), "Zzzzzzzz", "", "")
	if err == nil || err.Error() != `unknown font "Zzzzzzzz"` {
		t.Errorf("error %v", err)
	}
	// Without the font list there are no suggestions.
	fs.set("/v1/fonts", nil)
	_, err = l.Load(context.Background(), "Intr", "", "")
	if err == nil || err.Error() != `unknown font "Intr"` {
		t.Errorf("error %v", err)
	}
	if _, err := l.Load(context.Background(), "", "", "--"); err == nil || !strings.Contains(err.Error(), "no letters or digits") {
		t.Errorf("error %v", err)
	}
}

func TestLoadUnavailable(t *testing.T) {
	fs := newFontsource(t)
	l := fs.loader(t)

	fs.set("/fontsource/fonts/inter:vf@5.3.0/latin-wght-normal.woff2", []byte("<html>captive portal</html>"))
	_, err := l.Load(context.Background(), "Inter", "", "")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "is not a WOFF2 file") {
		t.Errorf("bad signature: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(l.CacheDir, "inter", metaFile)); serr == nil {
		t.Error("meta.json written for a failed download")
	}

	fs.set("/fontsource/fonts/inter:vf@5.3.0/latin-wght-normal.woff2", []byte("wOF2"+strings.Repeat("x", maxFileSize)))
	if _, err := l.Load(context.Background(), "Inter", "", ""); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "larger than 8 MB") {
		t.Errorf("too large: %v", err)
	}

	fs.set("/v1/fonts/lato", []byte("{"))
	if _, err := l.Load(context.Background(), "Lato", "", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("bad JSON: %v", err)
	}
	fs.set("/v1/fonts/lato", []byte(`{"id":"lato","family":"Lato","npmVersion":"5.3.0","unicodeRange":{"latin":"U+0000-00FF; color: red"}}`))
	if _, err := l.Load(context.Background(), "Lato", "", ""); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "unexpected subset") {
		t.Errorf("bad range: %v", err)
	}
	fs.set("/v1/fonts/lato", []byte(`{"id":"other","family":"Lato","npmVersion":"5.3.0"}`))
	if _, err := l.Load(context.Background(), "Lato", "", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("wrong id: %v", err)
	}
	fs.set("/v1/variable/noto-sans-jp", []byte(`{"axes":{"wght":{"min":"900","max":"100"}}}`))
	if _, err := l.Load(context.Background(), "Noto Sans JP", "", ""); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "weight axis") {
		t.Errorf("bad axis: %v", err)
	}

	// A cache folder that cannot be created fails before any request.
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fs.take()
	offline := &Loader{CacheDir: blocked, APIURL: fs.URL + "/v1", CDNURL: fs.URL}
	if _, err := offline.Load(context.Background(), "Inter", "", ""); !errors.Is(err, ErrUnavailable) || len(fs.take()) != 0 {
		t.Errorf("blocked cache: %v", err)
	}

	fs.Close()
	if _, err := l.Load(context.Background(), "Lora", "", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("offline: %v", err)
	}
}

func TestLoadRepairsCache(t *testing.T) {
	fs := newFontsource(t)
	l := fs.loader(t)
	want, err := l.Load(context.Background(), "Inter", "", "")
	if err != nil {
		t.Fatal(err)
	}
	fs.take()
	dir := filepath.Join(l.CacheDir, "inter")

	// A damaged or missing file is downloaded again from its pinned URL.
	if err := os.WriteFile(filepath.Join(dir, "inter-latin-wght-normal.woff2"), []byte("wOF2 damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, licenseFile)); err != nil {
		t.Fatal(err)
	}
	got, err := l.Load(context.Background(), "Inter", "", "")
	if err != nil || got.CSS != want.CSS || string(got.Files["_documango/fonts/inter-latin-wght-normal.woff2"]) != string(want.Files["_documango/fonts/inter-latin-wght-normal.woff2"]) {
		t.Fatalf("repaired load: %v", err)
	}
	reqs := fs.take()
	slices.Sort(reqs)
	if want := []string{"/fontsource/fonts/inter:vf@5.3.0/latin-wght-normal.woff2", "/npm/@fontsource-variable/inter@5.3.0/LICENSE"}; !slices.Equal(reqs, want) {
		t.Errorf("repair requests %q", reqs)
	}

	// A folder without meta.json is an unfinished download and is ignored.
	if err := os.Remove(filepath.Join(dir, metaFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Load(context.Background(), "Inter", "", ""); err != nil {
		t.Fatal(err)
	}
	if reqs := fs.take(); len(reqs) != 7 {
		t.Errorf("partial cache: requests %q", reqs)
	}

	// So is a meta.json that does not parse or lists unsafe names.
	for _, m := range []string{"{", `{"family":"Inter","faces":[{"file":"a.woff2"}],"files":{"../x":{}}}`,
		`{"family":"Inter","faces":[{"file":"a.woff2"}],"files":{}}`, `{"family":"Inter","faces":[{"file":"a.woff2"}],"files":{"a.woff2":{}}}`} {
		if err := os.WriteFile(filepath.Join(dir, metaFile), []byte(m), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := l.Load(context.Background(), "Inter", "", ""); err != nil {
			t.Fatal(err)
		}
		if reqs := fs.take(); len(reqs) != 7 {
			t.Errorf("meta %s: requests %q", m, reqs)
		}
	}

	// Files cannot be repaired offline.
	if err := os.WriteFile(filepath.Join(dir, "inter-latin-wght-normal.woff2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fs.Close()
	if _, err := l.Load(context.Background(), "Inter", "", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("offline repair: %v", err)
	}
}

func TestLoadLatestFallback(t *testing.T) {
	fs := newFontsource(t)
	fs.set("/fontsource/fonts/inter:vf@5.3.0/latin-wght-italic.woff2", nil)
	l := fs.loader(t)
	res, err := l.Load(context.Background(), "Inter", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(res.Files["_documango/fonts/inter-latin-wght-italic.woff2"]); got != woff2("/fontsource/fonts/inter:vf@latest/latin-wght-italic.woff2") {
		t.Errorf("fallback file %q", got)
	}
	meta, err := os.ReadFile(filepath.Join(l.CacheDir, "inter", metaFile))
	if err != nil || !strings.Contains(string(meta), fs.URL+"/fontsource/fonts/inter:vf@latest/latin-wght-italic.woff2") {
		t.Errorf("meta.json does not record the fallback URL: %v", err)
	}
}

func TestLoadVariableWithoutWeightAxis(t *testing.T) {
	fs := newFontsource(t)
	fs.set("/v1/variable/inter", []byte(`{"family":"Inter","axes":{"opsz":{"min":"14","max":"32"}}}`))
	res, err := fs.loader(t).Load(context.Background(), "Inter", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// The trimmed fixture only lists weight 400.
	if got := faceFiles(res.CSS); !slices.Equal(got, []string{
		"400 inter-latin-ext-400-normal.woff2", "400 inter-latin-400-normal.woff2", "400 inter-latin-ext-400-italic.woff2", "400 inter-latin-400-italic.woff2",
	}) {
		t.Errorf("faces %q", got)
	}

	fs.set("/v1/variable/inter", nil)
	l := fs.loader(t)
	if _, err := l.Load(context.Background(), "Inter", "", ""); err != nil {
		t.Errorf("no variable record: %v", err)
	}
}

func TestID(t *testing.T) {
	for in, want := range map[string]string{
		"Inter": "inter", "JetBrains Mono": "jetbrains-mono", "jetbrains-mono": "jetbrains-mono",
		"  IBM Plex  Sans ": "ibm-plex-sans", "Noto Sans JP": "noto-sans-jp", "M PLUS 1p": "m-plus-1p", "--": "",
	} {
		if got := ID(in); got != want {
			t.Errorf("ID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCSSString(t *testing.T) {
	if got := cssString(`A "B" \C`); got != `"A \"B\" \\C"` {
		t.Errorf("cssString = %s", got)
	}
}
