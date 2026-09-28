// Package pagefind locates or installs the Pagefind binary and uses it to build
// a full-text search bundle for an in-memory site.
package pagefind

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Version is the Pagefind release documango downloads.
const Version = "1.5.2"

// EnvBinary names the environment variable that points at a pagefind binary,
// overriding PATH lookup and the download.
const EnvBinary = "DOCUMANGO_PAGEFIND"

// DefaultBaseURL is the release download location for [Version].
const DefaultBaseURL = "https://github.com/Pagefind/pagefind/releases/download/v" + Version

// maxArchiveSize caps how much of a release archive is read into memory.
const maxArchiveSize = 64 << 20

// checksums pins the SHA-256 of each release archive by target triple.
var checksums = map[string]string{
	"aarch64-apple-darwin":       "7286f394a349bd37677d44a65a20078a02b1747da0b0814d83403bf86be17abe",
	"x86_64-apple-darwin":        "26f51b4ba921897142338fb13b836420696e251fe197b1782ea7981de311156d",
	"aarch64-unknown-linux-musl": "f50ec608bcbf431cebd84e0efa3a5b041ee63df2ad81f138023e0cfd2f509424",
	"x86_64-unknown-linux-musl":  "afb824a9e7f64905a934900481cea5be679c03975e527329e0e5e6cc70f5feda",
	"aarch64-pc-windows-msvc":    "c4d869293a9cd5c14c2dff7e7f568b27a5f4acb1124a2097b638cffc0d82b840",
	"x86_64-pc-windows-msvc":     "fab125d5e8e2d3481ffe7d36dec6e101f54a6581cd51cf5e2e09220d4bc78e9c",
}

// Finder locates or installs the pagefind binary. The zero value is ready to use.
type Finder struct {
	// CacheDir holds the downloaded binary. Defaults to
	// os.UserCacheDir()/documango/pagefind/<Version>.
	CacheDir string
	// Getenv reads environment variables. Defaults to os.Getenv.
	Getenv func(string) string
	// LookPath searches PATH. Defaults to exec.LookPath.
	LookPath func(string) (string, error)
	// Client downloads the release. Defaults to a client with a 5 minute timeout.
	Client *http.Client
	// BaseURL is the release download base. Defaults to DefaultBaseURL.
	BaseURL string
	// Logger reports downloads. Defaults to slog.Default().
	Logger *slog.Logger

	// goos, goarch and checksums override the platform and pinned sums in tests.
	goos, goarch string
	checksums    map[string]string
}

// Find returns the path to a pagefind binary. It checks, in order,
// $DOCUMANGO_PAGEFIND, pagefind on PATH, and the cache directory, and
// otherwise downloads, verifies and installs the pinned release into the cache.
func (f *Finder) Find(ctx context.Context) (string, error) {
	getenv := f.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if bin := getenv(EnvBinary); bin != "" {
		info, err := os.Stat(bin)
		if err != nil {
			return "", fmt.Errorf("pagefind: %s: %w", EnvBinary, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("pagefind: %s: %s is a directory", EnvBinary, bin)
		}
		return bin, nil
	}

	lookPath := f.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if bin, err := lookPath("pagefind"); err == nil {
		return bin, nil
	}

	goos, goarch := f.goos, f.goarch
	if goos == "" {
		goos, goarch = runtime.GOOS, runtime.GOARCH
	}
	target, err := Target(goos, goarch)
	if err != nil {
		return "", err
	}

	dir := f.CacheDir
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("pagefind: locate cache dir: %w", err)
		}
		dir = filepath.Join(base, "documango", "pagefind", Version)
	}
	name := "pagefind"
	if goos == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	if info, err := os.Stat(bin); err == nil && info.Mode().IsRegular() {
		return bin, nil
	}

	exe, err := f.download(ctx, target, name)
	if err != nil {
		return "", err
	}
	if err := install(dir, bin, exe); err != nil {
		return "", err
	}
	return bin, nil
}

// Target maps a GOOS/GOARCH pair to a Pagefind release target triple.
func Target(goos, goarch string) (string, error) {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	suffix := map[string]string{
		"darwin":  "apple-darwin",
		"linux":   "unknown-linux-musl",
		"windows": "pc-windows-msvc",
	}[goos]
	if arch == "" || suffix == "" {
		return "", fmt.Errorf("pagefind: no prebuilt binary for %s/%s; install pagefind %s and set %s to its path",
			goos, goarch, Version, EnvBinary)
	}
	return arch + "-" + suffix, nil
}

// download fetches the release archive for target, verifies its pinned
// checksum, and returns the contents of the binary entry called name.
func (f *Finder) download(ctx context.Context, target, name string) ([]byte, error) {
	sums := f.checksums
	if sums == nil {
		sums = checksums
	}
	want, ok := sums[target]
	if !ok {
		return nil, fmt.Errorf("pagefind: no pinned checksum for %s", target)
	}
	base := f.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := fmt.Sprintf("%s/pagefind-v%s-%s.tar.gz", strings.TrimSuffix(base, "/"), Version, target)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("pagefind: build request: %w", err)
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	logger := f.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info(fmt.Sprintf("downloading pagefind %s (~5 MB)", Version), "url", url)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pagefind: download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pagefind: download %s: %s", url, resp.Status)
	}
	archive, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveSize))
	if err != nil {
		return nil, fmt.Errorf("pagefind: download %s: %w", url, err)
	}

	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("pagefind: checksum mismatch for %s: got %s, want %s", url, got, want)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("pagefind: read archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("pagefind: archive %s has no %s entry", url, name)
		}
		if err != nil {
			return nil, fmt.Errorf("pagefind: read archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == name {
			exe, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("pagefind: extract %s: %w", name, err)
			}
			return exe, nil
		}
	}
}

// install writes exe to bin atomically so concurrent readers never observe a
// partial file.
func install(dir, bin string, exe []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("pagefind: create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".pagefind-*")
	if err != nil {
		return fmt.Errorf("pagefind: create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	_, err = tmp.Write(exe)
	if err == nil {
		err = tmp.Chmod(0o755)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("pagefind: write %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), bin); err != nil {
		// Another process may have installed it first (and on Windows a
		// running binary cannot be replaced).
		if _, serr := os.Stat(bin); serr == nil {
			return nil
		}
		return fmt.Errorf("pagefind: install %s: %w", bin, err)
	}
	return nil
}
