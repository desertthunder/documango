package theme

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Defaults for the zero-valued fields of Catalog.
const (
	DefaultRef     = "spec-0.11"
	DefaultBaseURL = "https://api.github.com"
	DefaultMaxAge  = 7 * 24 * time.Hour
)

// stampFile records, inside a cache directory, when it was downloaded.
const stampFile = ".fetched"

// ErrUnavailable reports that the full scheme catalog could not be loaded.
// Catalog.All returns it alongside the embedded schemes, so callers can show
// it as a warning and carry on.
var ErrUnavailable = errors.New("full scheme catalog unavailable")

// Catalog provides every tinted-theming base16 scheme: the embedded ones plus
// the full set downloaded from GitHub and cached on disk. The zero value is
// ready to use.
type Catalog struct {
	// CacheDir holds one directory of schemes per Ref. It defaults to
	// os.UserCacheDir()/documango/schemes.
	CacheDir string
	// Ref is the tinted-theming/schemes git ref; it defaults to DefaultRef.
	Ref string
	// BaseURL is the GitHub API root; it defaults to DefaultBaseURL.
	BaseURL string
	// Client defaults to an http.Client with a two minute timeout.
	Client *http.Client
	// MaxAge is how long a download stays fresh; it defaults to DefaultMaxAge.
	MaxAge time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
	// Logger receives cache warnings; it defaults to slog.Default().
	Logger *slog.Logger
}

// All returns every scheme sorted by slug. Embedded schemes win over cached
// schemes with the same slug, so builds stay reproducible. The cache is
// downloaded when missing or older than MaxAge. If that download fails, a
// stale cache is used with a logged warning; without any cache All returns
// the embedded schemes and an error wrapping ErrUnavailable.
func (c *Catalog) All(ctx context.Context) ([]Scheme, error) {
	dir, err := c.cacheDir()
	if err != nil {
		return Builtin(), fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	cached, fetched, err := c.readCache(dir)
	if err == nil && c.now().Sub(fetched) < c.maxAge() {
		return merge(cached), nil
	}
	if ferr := c.refresh(ctx, dir); ferr != nil {
		if err == nil && len(cached) > 0 {
			c.logger().Warn("using stale theme cache", "dir", dir, "err", ferr)
			return merge(cached), nil
		}
		return Builtin(), fmt.Errorf("%w: %w", ErrUnavailable, ferr)
	}
	if cached, _, err = c.readCache(dir); err != nil {
		return Builtin(), fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return merge(cached), nil
}

// Load resolves nameOrPath like the package-level Load, but names that are
// not embedded are looked up in All, and suggestions for unknown names are
// drawn from every scheme.
func (c *Catalog) Load(ctx context.Context, nameOrPath string) (Scheme, error) {
	if isPath(nameOrPath) {
		return loadFile(nameOrPath)
	}
	slug := strings.ToLower(nameOrPath)
	if s, ok := Lookup(slug); ok {
		return s, nil
	}
	all, err := c.All(ctx)
	if s, ok := find(all, slug); ok {
		return s, nil
	}
	msg := unknownTheme(nameOrPath, all)
	if err != nil {
		return Scheme{}, fmt.Errorf("%s: %w", msg, err)
	}
	return Scheme{}, errors.New(msg)
}

// Download fetches the tinted-theming/schemes tarball at Ref and writes its
// LICENSE and the base16 schemes whose slug keep accepts into dir, which must
// exist. A nil keep accepts every scheme. It returns the slugs written.
// GITHUB_TOKEN, when set, authenticates the request; if GitHub rejects it the
// download is retried anonymously.
func (c *Catalog) Download(ctx context.Context, dir string, keep func(slug string) bool) ([]string, error) {
	u := strings.TrimSuffix(cmp.Or(c.BaseURL, DefaultBaseURL), "/") +
		"/repos/tinted-theming/schemes/tarball/" + url.PathEscape(c.ref())
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	token := os.Getenv("GITHUB_TOKEN")
	var resp *http.Response
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("download schemes: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if resp, err = client.Do(req); err != nil {
			return nil, fmt.Errorf("download schemes: %w", err)
		}
		if resp.StatusCode != http.StatusUnauthorized || token == "" {
			break
		}
		resp.Body.Close()
		c.logger().Warn("GITHUB_TOKEN rejected by GitHub; retrying without it")
		token = ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		hint := ""
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			hint = " (set GITHUB_TOKEN to raise the GitHub rate limit)"
		}
		return nil, fmt.Errorf("download schemes %s: %s%s", u, resp.Status, hint)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("open schemes tarball: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var slugs []string
	license := false
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read schemes tarball: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Entries sit under a single top-level directory named after the commit.
		_, rel, _ := strings.Cut(hdr.Name, "/")
		var name string
		switch slug := strings.TrimSuffix(path.Base(rel), ".yaml"); {
		case rel == "LICENSE":
			name, license = "LICENSE", true
		case path.Dir(rel) == "base16" && path.Ext(rel) == ".yaml" && slug != "" && (keep == nil || keep(slug)):
			name = path.Base(rel)
			slugs = append(slugs, slug)
		default:
			continue
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("create scheme file: %w", err)
		}
		_, err = io.Copy(f, tr)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}
	if len(slugs) == 0 {
		return nil, errors.New("no base16 schemes found in tarball")
	}
	if !license {
		return nil, errors.New("LICENSE not found in tarball")
	}
	slices.Sort(slugs)
	return slugs, nil
}

// refresh downloads the catalog into a staging directory beside dir and
// moves it into place. A first download renames the whole directory; later
// ones rename each file over its old copy, write the stamp last and then drop
// files that are gone upstream. Readers therefore only ever see complete
// files.
func (c *Catalog) refresh(ctx context.Context, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create theme cache: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+"-*")
	if err != nil {
		return fmt.Errorf("create theme cache: %w", err)
	}
	defer os.RemoveAll(stage)
	if _, err := c.Download(ctx, stage, nil); err != nil {
		return err
	}
	stamp := c.now().UTC().Format(time.RFC3339Nano)
	if err := os.WriteFile(filepath.Join(stage, stampFile), []byte(stamp), 0o644); err != nil {
		return fmt.Errorf("write theme cache stamp: %w", err)
	}
	if err := os.Rename(stage, dir); err == nil {
		return nil
	}

	staged, err := os.ReadDir(stage)
	if err != nil {
		return fmt.Errorf("read theme cache stage: %w", err)
	}
	keep := map[string]bool{stampFile: true}
	for _, e := range staged {
		if name := e.Name(); name != stampFile {
			keep[name] = true
			if err := os.Rename(filepath.Join(stage, name), filepath.Join(dir, name)); err != nil {
				return fmt.Errorf("update theme cache: %w", err)
			}
		}
	}
	if err := os.Rename(filepath.Join(stage, stampFile), filepath.Join(dir, stampFile)); err != nil {
		return fmt.Errorf("update theme cache: %w", err)
	}
	current, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read theme cache: %w", err)
	}
	for _, e := range current {
		if !keep[e.Name()] {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return fmt.Errorf("prune theme cache: %w", err)
			}
		}
	}
	return nil
}

// readCache parses the schemes cached in dir and returns them with the time
// they were downloaded; a missing stamp yields the zero time. Unparseable
// files are skipped with a warning. When a concurrent refresh swaps the
// directory mid-read, the scan starts over.
func (c *Catalog) readCache(dir string) ([]Scheme, time.Time, error) {
	var err error
	for range 3 {
		var entries []os.DirEntry
		if entries, err = os.ReadDir(dir); err != nil {
			return nil, time.Time{}, fmt.Errorf("read theme cache: %w", err)
		}
		var fetched time.Time
		if data, err := os.ReadFile(filepath.Join(dir, stampFile)); err == nil {
			fetched, _ = time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
		}
		var schemes []Scheme
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || filepath.Ext(name) != ".yaml" {
				continue
			}
			var data []byte
			if data, err = os.ReadFile(filepath.Join(dir, name)); err != nil {
				break
			}
			s, perr := Parse(data)
			if perr != nil {
				c.logger().Warn("skipping cached theme", "file", name, "err", perr)
				continue
			}
			schemes = append(schemes, withSlug(s, strings.TrimSuffix(name, ".yaml")))
		}
		if err == nil {
			return schemes, fetched, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			break
		}
	}
	return nil, time.Time{}, fmt.Errorf("read theme cache: %w", err)
}

func (c *Catalog) cacheDir() (string, error) {
	ref := c.ref()
	if ref == "." || ref == ".." {
		return "", fmt.Errorf("invalid scheme ref %q", ref)
	}
	root := c.CacheDir
	if root == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("locate theme cache: %w", err)
		}
		root = filepath.Join(base, "documango", "schemes")
	}
	return filepath.Join(root, url.PathEscape(ref)), nil
}

func (c *Catalog) ref() string { return cmp.Or(c.Ref, DefaultRef) }

func (c *Catalog) maxAge() time.Duration {
	if c.MaxAge > 0 {
		return c.MaxAge
	}
	return DefaultMaxAge
}

func (c *Catalog) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Catalog) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// merge combines cached schemes with the embedded ones, which take
// precedence, and sorts the result by slug.
func merge(cached []Scheme) []Scheme {
	out := Builtin()
	for _, s := range cached {
		if _, ok := Lookup(s.Slug); !ok {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Scheme) int { return strings.Compare(a.Slug, b.Slug) })
	return out
}
