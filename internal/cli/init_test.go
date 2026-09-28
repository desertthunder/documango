package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"
)

// scaffoldConfig is the part of the generated config the tests check.
type scaffoldConfig struct {
	Title string `toml:"title" yaml:"title"`
	Theme struct {
		Dark  string `toml:"dark" yaml:"dark"`
		Light string `toml:"light" yaml:"light"`
	} `toml:"theme" yaml:"theme"`
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "acme", "docs")
	r := run(t, nil, "init", dir)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want empty", r.stdout)
	}
	for _, f := range []string{"documango.toml", "index.md", "guide/index.md", "guide/writing.md"} {
		if !strings.Contains(r.stderr, filepath.Join(dir, filepath.FromSlash(f))) {
			t.Errorf("stderr lacks %s:\n%s", f, r.stderr)
		}
	}
	for _, w := range []string{"documango " + dir, "documango build " + dir} {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("stderr lacks next step %q:\n%s", w, r.stderr)
		}
	}

	if home := readFile(t, filepath.Join(dir, "index.md")); !strings.Contains(home, "# Acme\n") || !strings.Contains(home, "(guide/writing.md)") {
		t.Errorf("index.md:\n%s", home)
	}
	writing := readFile(t, filepath.Join(dir, "guide", "writing.md"))
	for _, w := range []string{"---\ntitle: ", "> [!TIP]", "```sh", "](../index.md)"} {
		if !strings.Contains(writing, w) {
			t.Errorf("guide/writing.md lacks %q", w)
		}
	}

	var cfg scaffoldConfig
	if _, err := toml.DecodeFile(filepath.Join(dir, "documango.toml"), &cfg); err != nil {
		t.Fatalf("documango.toml: %v", err)
	}
	if cfg.Title != "Acme" || cfg.Theme.Dark != "tomorrow-night" || cfg.Theme.Light != "tomorrow" {
		t.Errorf("documango.toml = %+v", cfg)
	}

	out := filepath.Join(t.TempDir(), "site")
	r = run(t, nil, "build", dir, "-o", out, "--search", "builtin")
	if r.code != 0 || !strings.HasPrefix(r.stderr, "Built 3 pages to ") {
		t.Fatalf("build: exit %d, stderr %q", r.code, r.stderr)
	}
	for _, f := range []string{"index.html", "guide/index.html", "guide/writing/index.html"} {
		if !exists(filepath.Join(out, f)) {
			t.Errorf("build lacks %s", f)
		}
	}
}

func TestInitYAML(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site")
	title := `Say "hi" & <go>: it's here`
	if r := run(t, nil, "init", dir, "--format", "yaml", "--title", title, "-q"); r.code != 0 || r.stderr != "" || r.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if exists(filepath.Join(dir, "documango.toml")) {
		t.Error("yaml format wrote documango.toml")
	}
	var cfg scaffoldConfig
	if err := yaml.Unmarshal([]byte(readFile(t, filepath.Join(dir, "documango.yaml"))), &cfg); err != nil {
		t.Fatalf("documango.yaml: %v", err)
	}
	if cfg.Title != title || cfg.Theme.Dark != "tomorrow-night" || cfg.Theme.Light != "tomorrow" {
		t.Errorf("documango.yaml = %+v", cfg)
	}
	if home := readFile(t, filepath.Join(dir, "index.md")); !strings.Contains(home, "# "+title+"\n") {
		t.Errorf("index.md:\n%s", home)
	}
}

func TestInitTOMLTitleEscaping(t *testing.T) {
	dir := t.TempDir()
	title := `Say "hi" & \ <go>`
	if r := run(t, nil, "init", dir, "--title", title); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	var cfg scaffoldConfig
	if _, err := toml.DecodeFile(filepath.Join(dir, "documango.toml"), &cfg); err != nil || cfg.Title != title {
		t.Errorf("title = %q, err %v", cfg.Title, err)
	}
}

func TestInitDefaultDir(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "my_cool-project")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	r := run(t, nil, "init")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if !strings.Contains(r.stderr, filepath.Join("docs", "index.md")) || strings.Contains(r.stderr, tmp) {
		t.Errorf("paths are not relative to the current folder:\n%s", r.stderr)
	}
	if home := readFile(t, filepath.Join(tmp, "docs", "index.md")); !strings.Contains(home, "# My Cool Project\n") {
		t.Errorf("index.md:\n%s", home)
	}
}

func TestInferTitle(t *testing.T) {
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	for _, tc := range []struct{ dir, want string }{
		{filepath.Join(root, "code", "acme", "docs"), "Acme"},
		{filepath.Join(root, "code", "acme-widgets", "Doc"), "Acme Widgets"},
		{filepath.Join(root, "code", "field_notes"), "Field Notes"},
		{filepath.Join(root, "code", "gRPC.gateway"), "GRPC Gateway"},
		{filepath.Join(root, "docs"), "Documentation"},
		{root, "Documentation"},
	} {
		if got := inferTitle(tc.dir); got != tc.want {
			t.Errorf("inferTitle(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestInitNonEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes.txt"), "mine")
	r := run(t, nil, "init", dir)
	if r.code != 1 || !strings.Contains(r.stderr, "is not empty") || !strings.Contains(r.stderr, "Hint: ") || !strings.Contains(r.stderr, "--force") {
		t.Errorf("exit %d, stderr %q", r.code, r.stderr)
	}
	if exists(filepath.Join(dir, "index.md")) {
		t.Error("init wrote into a non-empty folder")
	}
}

func TestInitConflicts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.md"), "# Mine\n")
	writeFile(t, filepath.Join(dir, "guide", "writing.md"), "# Also mine\n")

	r := run(t, nil, "init", dir)
	if r.code != 1 {
		t.Errorf("exit %d, want 1", r.code)
	}
	for _, w := range []string{"index.md", filepath.Join("guide", "writing.md"), "--force"} {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("stderr %q lacks %q", r.stderr, w)
		}
	}
	if exists(filepath.Join(dir, "documango.toml")) || exists(filepath.Join(dir, "guide", "index.md")) {
		t.Error("init wrote files despite conflicts")
	}

	r = run(t, nil, "init", dir, "--force")
	if r.code != 0 {
		t.Fatalf("--force: exit %d, stderr %q", r.code, r.stderr)
	}
	if got := readFile(t, filepath.Join(dir, "index.md")); got != "# Mine\n" {
		t.Errorf("--force overwrote index.md: %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "guide", "writing.md")); got != "# Also mine\n" {
		t.Errorf("--force overwrote guide/writing.md: %q", got)
	}
	if !exists(filepath.Join(dir, "documango.toml")) || !exists(filepath.Join(dir, "guide", "index.md")) {
		t.Error("--force did not add the missing files")
	}
	if !regexp.MustCompile(`(?s)Skipped.*index\.md.*writing\.md`).MatchString(r.stderr) {
		t.Errorf("--force does not report skipped files:\n%s", r.stderr)
	}
}

func TestInitErrors(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "file.md")
	writeFile(t, file, "# x")

	r := run(t, nil, "init", file)
	if r.code != 1 || !strings.Contains(r.stderr, "is not a directory") {
		t.Errorf("file: exit %d, stderr %q", r.code, r.stderr)
	}

	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{"init", filepath.Join(tmp, "a"), "--format", "json"}, `invalid --format "json": use toml or yaml`},
		{[]string{"init", "a", "b"}, "accepts at most 1 arg"},
	} {
		r := run(t, nil, tc.args...)
		if r.code != 2 || !strings.Contains(r.stderr, tc.msg) || !strings.Contains(r.stderr, "Run 'documango init --help' for usage.") {
			t.Errorf("%v: exit %d, stderr %q", tc.args, r.code, r.stderr)
		}
	}
	if exists(filepath.Join(tmp, "a")) {
		t.Error("init with a bad --format created the folder")
	}
}

func TestInitHelp(t *testing.T) {
	r := run(t, nil, "init", "--help")
	for _, w := range []string{"Examples:", "--title", "--format", "--force", "documango init"} {
		if !strings.Contains(r.stdout, w) {
			t.Errorf("help lacks %q:\n%s", w, r.stdout)
		}
	}
}
