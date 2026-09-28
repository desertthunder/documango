// Package fonts downloads web fonts from Fontsource into a local cache and
// turns them into self-hosted files and @font-face rules for a site. Sites
// built with them load fonts from their own origin only.
package fonts

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/desertthunder/documango/internal/suggest"
)

// Default endpoints.
const (
	DefaultAPIURL = "https://api.fontsource.org/v1"
	DefaultCDNURL = "https://cdn.jsdelivr.net"
)

// OutDir is where font files go in a built site, relative to its root.
const OutDir = "_documango/fonts"

// ErrUnavailable reports that fonts could not be downloaded or cached, for
// example because the machine is offline.
var ErrUnavailable = errors.New("fonts unavailable")

// Fallback stacks, as defined in the site's tokens.css.
const (
	sansStack = "var(--font-system-sans)"
	monoStack = "var(--font-system-mono)"
)

const (
	maxFileSize   = 8 << 20
	maxJSONSize   = 16 << 20
	downloadLimit = 6
	metaFile      = "meta.json"
	licenseFile   = "LICENSE.txt"
)

// Loader downloads fonts and keeps them in a cache. The zero value is ready
// to use.
type Loader struct {
	// CacheDir holds one folder per font. Defaults to
	// os.UserCacheDir()/documango/fonts.
	CacheDir string
	// APIURL is the Fontsource API base. Defaults to DefaultAPIURL.
	APIURL string
	// CDNURL is the jsDelivr base the font and license files come from.
	// Defaults to DefaultCDNURL.
	CDNURL string
	// Client makes the requests. Defaults to a client with a 2 minute timeout.
	Client *http.Client
	// Logger reports downloads. Defaults to slog.Default().
	Logger *slog.Logger
}

// Result is the stylesheet and files that add fonts to a site.
type Result struct {
	// CSS holds the @font-face rules and the :root font variables. Its URLs
	// are relative to _documango/style.css.
	CSS string
	// Files are the font and license files, keyed by output path.
	Files map[string][]byte
}

// weights are the weights, by style, that each role of a font uses.
type weights map[string][]int

var (
	textWeights = weights{"normal": {400, 600, 700}, "italic": {400, 700}}
	monoWeights = weights{"normal": {400, 700}, "italic": {400}}
)

// Load returns the fonts for the body, heading and code text. Each is a
// family name such as "JetBrains Mono" or a Fontsource ID such as
// "jetbrains-mono"; "" keeps the system fonts, and heading defaults to body.
// Fonts are downloaded once and then read from the cache. Errors from the
// network or the cache wrap ErrUnavailable.
func (l *Loader) Load(ctx context.Context, body, heading, mono string) (Result, error) {
	heading = cmp.Or(heading, body)
	roles := []struct {
		name, variable, stack string
		weights               weights
	}{
		{body, "--font-sans", sansStack, textWeights},
		{heading, "--font-heading", sansStack, textWeights},
		{mono, "--font-mono", monoStack, monoWeights},
	}
	res := Result{Files: map[string][]byte{}}
	var order []string
	metas := map[string]*meta{}
	need := map[string]weights{}
	var vars strings.Builder
	for _, r := range roles {
		if r.name == "" {
			continue
		}
		id := ID(r.name)
		if id == "" {
			return Result{}, fmt.Errorf("font %q has no letters or digits", r.name)
		}
		m, ok := metas[id]
		if !ok {
			var err error
			if m, err = l.load(ctx, id, r.name); err != nil {
				return Result{}, err
			}
			metas[id] = m
			order = append(order, id)
			need[id] = weights{}
		}
		for style, ws := range r.weights {
			need[id][style] = append(need[id][style], ws...)
		}
		// The heading defaults to the body font through tokens.css.
		if r.variable == "--font-heading" && ID(heading) == ID(body) {
			continue
		}
		fmt.Fprintf(&vars, "%s:%s,%s;", r.variable, cssString(m.Family), r.stack)
	}
	if len(order) == 0 {
		return Result{}, nil
	}

	var css strings.Builder
	for _, id := range order {
		m := metas[id]
		dir, _ := l.dir(id)
		faces := m.faces(need[id])
		for _, f := range faces {
			fmt.Fprintf(&css, "@font-face{font-family:%s;font-style:%s;font-weight:%s;font-display:swap;src:url(fonts/%s) format(\"woff2\");unicode-range:%s}\n",
				cssString(m.Family), f.Style, f.weight(), f.File, f.Range)
		}
		files := []string{licenseFile}
		for _, f := range faces {
			files = append(files, f.File)
		}
		for _, name := range files {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return Result{}, fmt.Errorf("%w: read font %s: %v", ErrUnavailable, name, err)
			}
			out := OutDir + "/" + name
			if name == licenseFile {
				out = OutDir + "/LICENSE-" + id + ".txt"
			}
			res.Files[out] = b
		}
	}
	css.WriteString(":root{" + vars.String() + "}\n")
	res.CSS = css.String()
	return res, nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// ID returns the Fontsource ID of a family name: lowercase, with every run of
// other characters than letters and digits turned into one "-".
// "JetBrains Mono" becomes "jetbrains-mono".
func ID(family string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(family), "-"), "-")
}

// meta describes a cached font. It is written last, so a folder without it
// is an unfinished download.
type meta struct {
	ID      string          `json:"id"`
	Family  string          `json:"family"`
	Version string          `json:"version"`
	Faces   []face          `json:"faces"`
	Files   map[string]file `json:"files"`
}

// face is one @font-face rule.
type face struct {
	Style string `json:"style"`
	// Weight is the range of weights; both ends are equal for a static font.
	Weight [2]int `json:"weight"`
	Range  string `json:"unicodeRange"`
	File   string `json:"file"`
}

func (f face) weight() string {
	if f.Weight[0] == f.Weight[1] {
		return strconv.Itoa(f.Weight[0])
	}
	return fmt.Sprintf("%d %d", f.Weight[0], f.Weight[1])
}

// file is a cached file and where it came from.
type file struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// faces returns the faces of m that match need. A variable face covers
// every weight. When nothing matches, every face is returned.
func (m *meta) faces(need weights) []face {
	var out []face
	for _, f := range m.Faces {
		if f.Weight[0] != f.Weight[1] || slices.Contains(need[f.Style], f.Weight[0]) {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return m.Faces
	}
	return out
}

// load returns the cached font id, downloading it when needed.
func (l *Loader) load(ctx context.Context, id, name string) (*meta, error) {
	dir, err := l.dir(id)
	if err != nil {
		return nil, err
	}
	if m, err := l.readCache(ctx, dir); err == nil {
		return m, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: create font cache: %v", ErrUnavailable, err)
	}
	return l.download(ctx, dir, id, name)
}

func (l *Loader) dir(id string) (string, error) {
	root := l.CacheDir
	if root == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("%w: locate cache dir: %v", ErrUnavailable, err)
		}
		root = filepath.Join(base, "documango", "fonts")
	}
	return filepath.Join(root, id), nil
}

// readCache reads the font cached in dir, downloading again any file that is
// missing or corrupt from the URL it came from. It returns an error wrapping
// os.ErrNotExist when dir holds no complete download.
func (l *Loader) readCache(ctx context.Context, dir string) (*meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return nil, os.ErrNotExist
	}
	var m meta
	if err := json.Unmarshal(data, &m); err != nil || m.Family == "" || len(m.Faces) == 0 {
		return nil, os.ErrNotExist
	}
	var stale []string
	for name, f := range m.Files {
		if !validName.MatchString(name) {
			return nil, os.ErrNotExist
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || hash(b) != f.SHA256 {
			stale = append(stale, name)
		}
	}
	for _, f := range m.Faces {
		if _, ok := m.Files[f.File]; !ok {
			return nil, os.ErrNotExist
		}
	}
	if _, ok := m.Files[licenseFile]; !ok {
		return nil, os.ErrNotExist
	}
	if len(stale) == 0 {
		return &m, nil
	}
	slices.Sort(stale)
	l.logger().Info(fmt.Sprintf("downloading %d damaged font files of %s again", len(stale), m.Family))
	jobs := make([]job, len(stale))
	for i, name := range stale {
		jobs[i] = job{name: name, url: m.Files[name].URL, font: name != licenseFile}
	}
	if _, err := l.fetchAll(ctx, dir, jobs); err != nil {
		return nil, err
	}
	return &m, nil
}

// fontInfo is the part of a Fontsource font record that is used.
type fontInfo struct {
	ID         string                                           `json:"id"`
	Family     string                                           `json:"family"`
	Weights    []int                                            `json:"weights"`
	Styles     []string                                         `json:"styles"`
	Variable   bool                                             `json:"variable"`
	NPMVersion string                                           `json:"npmVersion"`
	Ranges     ranges                                           `json:"unicodeRange"`
	Variants   map[string]map[string]map[string]json.RawMessage `json:"variants"`
}

// ranges is the unicodeRange object in its original order, which is kept in
// the stylesheet: for overlapping ranges, browsers prefer the last face.
type ranges []struct{ Key, Range string }

func (r *ranges) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return errors.New("unicodeRange is not an object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var v string
		if err := dec.Decode(&v); err != nil {
			return err
		}
		*r = append(*r, struct{ Key, Range string }{t.(string), v})
	}
	return nil
}

var (
	validName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
	validVersion = regexp.MustCompile(`^[0-9A-Za-z.-]+$`)
	validRange   = regexp.MustCompile(`^[Uu]\+[0-9A-Fa-f?]+(-[0-9A-Fa-f]+)?(,\s*[Uu]\+[0-9A-Fa-f?]+(-[0-9A-Fa-f]+)?)*$`)
)

// job is one file to download into the cache.
type job struct {
	name, url string
	// fallback is tried when url is not found.
	fallback string
	// font marks a WOFF2 file, which must start with its signature.
	font bool
}

// download fetches the font id from Fontsource into dir.
func (l *Loader) download(ctx context.Context, dir, id, name string) (*meta, error) {
	api := strings.TrimSuffix(cmp.Or(l.APIURL, DefaultAPIURL), "/")
	cdn := strings.TrimSuffix(cmp.Or(l.CDNURL, DefaultCDNURL), "/")

	var info fontInfo
	status, err := l.getJSON(ctx, api+"/fonts/"+id, &info)
	if status == http.StatusNotFound {
		return nil, l.unknown(ctx, api, name)
	}
	if err != nil {
		return nil, err
	}
	if info.ID != id || info.Family == "" || !validVersion.MatchString(info.NPMVersion) {
		return nil, fmt.Errorf("%w: font %s: unexpected response from %s", ErrUnavailable, id, api)
	}

	var subsets []struct{ key, rng string }
	for _, r := range info.Ranges {
		key := strings.Trim(r.Key, "[]")
		if !validName.MatchString(key) || !validRange.MatchString(r.Range) {
			return nil, fmt.Errorf("%w: font %s: unexpected subset %q", ErrUnavailable, id, r.Key)
		}
		subsets = append(subsets, struct{ key, rng string }{key, r.Range})
	}

	m := &meta{ID: id, Family: info.Family, Version: info.NPMVersion, Files: map[string]file{}}
	var jobs []job
	styles := []string{"normal"}
	if slices.Contains(info.Styles, "italic") {
		styles = append(styles, "italic")
	}

	wght, err := l.variableWeight(ctx, api, &info)
	if err != nil {
		return nil, err
	}
	pkg := "@fontsource/"
	if wght != [2]int{} {
		pkg = "@fontsource-variable/"
		for _, style := range styles {
			for _, s := range subsets {
				path := fmt.Sprintf("%s-wght-%s.woff2", s.key, style)
				f := face{Style: style, Weight: wght, Range: s.rng, File: id + "-" + path}
				m.Faces = append(m.Faces, f)
				jobs = append(jobs, job{
					name: f.File, font: true,
					url:      fmt.Sprintf("%s/fontsource/fonts/%s:vf@%s/%s", cdn, id, info.NPMVersion, path),
					fallback: fmt.Sprintf("%s/fontsource/fonts/%s:vf@latest/%s", cdn, id, path),
				})
			}
		}
	} else {
		for _, style := range styles {
			for _, w := range staticWeights(&info, style) {
				for _, s := range subsets {
					if _, ok := info.Variants[strconv.Itoa(w)][style][s.key]; !ok {
						continue
					}
					path := fmt.Sprintf("%s-%d-%s.woff2", s.key, w, style)
					f := face{Style: style, Weight: [2]int{w, w}, Range: s.rng, File: id + "-" + path}
					m.Faces = append(m.Faces, f)
					jobs = append(jobs, job{name: f.File, font: true,
						url: fmt.Sprintf("%s/fontsource/fonts/%s@%s/%s", cdn, id, info.NPMVersion, path)})
				}
			}
		}
	}
	if len(m.Faces) == 0 {
		return nil, fmt.Errorf("font %q has no files to download", info.Family)
	}
	jobs = append(jobs, job{name: licenseFile, url: fmt.Sprintf("%s/npm/%s%s@%s/LICENSE", cdn, pkg, id, info.NPMVersion)})

	log := l.logger()
	log.Info(fmt.Sprintf("downloading font %s from Fontsource (%d files)", info.Family, len(jobs)))
	start := time.Now()
	files, err := l.fetchAll(ctx, dir, jobs)
	if err != nil {
		return nil, err
	}
	size := 0
	for name, f := range files {
		m.Files[name] = f.file
		size += f.size
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeFile(dir, metaFile, data); err != nil {
		return nil, err
	}
	log.Info(fmt.Sprintf("downloaded font %s (%.1f MB) in %s", info.Family, float64(size)/(1<<20), time.Since(start).Round(time.Millisecond)),
		"cache", dir)
	return m, nil
}

// variableWeight returns the weight axis of a variable font, or zero when
// info is static or has no weight axis.
func (l *Loader) variableWeight(ctx context.Context, api string, info *fontInfo) ([2]int, error) {
	if !info.Variable {
		return [2]int{}, nil
	}
	var v struct {
		Axes map[string]struct{ Min, Max string } `json:"axes"`
	}
	status, err := l.getJSON(ctx, api+"/variable/"+info.ID, &v)
	if status == http.StatusNotFound {
		return [2]int{}, nil
	}
	if err != nil {
		return [2]int{}, err
	}
	axis, ok := v.Axes["wght"]
	if !ok {
		return [2]int{}, nil
	}
	lo, err1 := strconv.ParseFloat(axis.Min, 64)
	hi, err2 := strconv.ParseFloat(axis.Max, 64)
	if err1 != nil || err2 != nil || lo < 1 || hi > 1000 || lo >= hi {
		return [2]int{}, fmt.Errorf("%w: font %s: unexpected weight axis %s to %s", ErrUnavailable, info.ID, axis.Min, axis.Max)
	}
	return [2]int{int(lo), int(hi)}, nil
}

// staticWeights returns the weights of style that the site uses and the font
// has, else the one nearest to 400.
func staticWeights(info *fontInfo, style string) []int {
	var have []int
	for _, w := range info.Weights {
		if _, ok := info.Variants[strconv.Itoa(w)][style]; ok {
			have = append(have, w)
		}
	}
	var out []int
	for _, w := range textWeights[style] {
		if slices.Contains(have, w) {
			out = append(out, w)
		}
	}
	if len(out) > 0 || len(have) == 0 {
		return out
	}
	nearest := slices.MinFunc(have, func(a, b int) int { return cmp.Compare(abs(a-400), abs(b-400)) })
	return []int{nearest}
}

func abs(n int) int { return max(n, -n) }

// unknown describes a font that Fontsource does not have, suggesting close
// family names when the font list can be fetched.
func (l *Loader) unknown(ctx context.Context, api, name string) error {
	err := fmt.Errorf("unknown font %q", name)
	var list []struct{ ID, Family string }
	if _, lerr := l.getJSON(ctx, api+"/fonts", &list); lerr != nil {
		return err
	}
	ids := make([]string, len(list))
	families := make(map[string]string, len(list))
	for i, f := range list {
		ids[i] = f.ID
		families[f.ID] = f.Family
	}
	hints := suggest.Closest(ID(name), ids, 3)
	if len(hints) == 0 {
		return err
	}
	for i, id := range hints {
		hints[i] = strconv.Quote(families[id])
	}
	return fmt.Errorf("%w; did you mean %s?", err, strings.Join(hints, ", "))
}

// fetched is a file written to the cache.
type fetched struct {
	file
	size int
}

// fetchAll downloads jobs into dir, a few at a time.
func (l *Loader) fetchAll(ctx context.Context, dir string, jobs []job) (map[string]fetched, error) {
	out := make([]fetched, len(jobs))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(downloadLimit)
	for i, j := range jobs {
		g.Go(func() error {
			url := j.url
			b, status, err := l.get(ctx, url, maxFileSize)
			if status == http.StatusNotFound && j.fallback != "" {
				url = j.fallback
				b, _, err = l.get(ctx, url, maxFileSize)
			}
			if err != nil {
				return err
			}
			if j.font && !bytes.HasPrefix(b, []byte("wOF2")) {
				return fmt.Errorf("%w: %s is not a WOFF2 file", ErrUnavailable, url)
			}
			if err := writeFile(dir, j.name, b); err != nil {
				return err
			}
			out[i] = fetched{file{URL: url, SHA256: hash(b)}, len(b)}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	files := make(map[string]fetched, len(jobs))
	for i, j := range jobs {
		files[j.name] = out[i]
	}
	return files, nil
}

// getJSON decodes the JSON at url into v. It returns the response status,
// or 0 when there was none.
func (l *Loader) getJSON(ctx context.Context, url string, v any) (int, error) {
	b, status, err := l.get(ctx, url, maxJSONSize)
	if err != nil {
		return status, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return status, fmt.Errorf("%w: decode %s: %v", ErrUnavailable, url, err)
	}
	return status, nil
}

// get returns the body at url, failing on any status but 200 or a body
// larger than limit.
func (l *Loader) get(ctx context.Context, url string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	client := l.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("%w: GET %s: %s", ErrUnavailable, url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: GET %s: %v", ErrUnavailable, url, err)
	}
	if int64(len(b)) > limit {
		return nil, resp.StatusCode, fmt.Errorf("%w: GET %s: larger than %d MB", ErrUnavailable, url, limit>>20)
	}
	return b, resp.StatusCode, nil
}

// writeFile writes data to dir/name atomically, so a reader never sees a
// partial file.
func writeFile(dir, name string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("%w: write font cache: %v", ErrUnavailable, err)
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		return fmt.Errorf("%w: write font cache: %v", ErrUnavailable, err)
	}
	return nil
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// cssString quotes a family name for CSS.
func cssString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(s) + `"`
}

func (l *Loader) logger() *slog.Logger {
	if l.Logger != nil {
		return l.Logger
	}
	return slog.Default()
}
