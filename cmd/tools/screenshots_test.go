package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func shotNames(s []shot) []string {
	var names []string
	for _, x := range s {
		names = append(names, x.name)
	}
	return names
}

func TestParseScreenshots(t *testing.T) {
	var stderr bytes.Buffer
	opts, ok := parseScreenshots(nil, &stderr)
	if !ok {
		t.Fatalf("defaults rejected: %s", stderr.String())
	}
	if opts.out != ".github/assets" || opts.docs != "docs" || len(opts.shots) != len(shots) {
		t.Errorf("defaults = %+v", opts)
	}

	opts, ok = parseScreenshots([]string{"-out", "x", "-docs", "d", "-only", "site-light,cli-help"}, &stderr)
	if !ok {
		t.Fatalf("flags rejected: %s", stderr.String())
	}
	if got, want := shotNames(opts.shots), []string{"cli-help", "site-light"}; !slices.Equal(got, want) || opts.out != "x" || opts.docs != "d" {
		t.Errorf("got %v %+v, want shots %v", got, opts, want)
	}

	for _, args := range [][]string{{"-only", "nope"}, {"extra"}, {"-bogus"}} {
		stderr.Reset()
		if _, ok := parseScreenshots(args, &stderr); ok {
			t.Errorf("%v accepted", args)
		}
		if stderr.Len() == 0 {
			t.Errorf("%v: no message", args)
		}
	}
}

func TestShotList(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range shots {
		if seen[s.name] {
			t.Errorf("duplicate shot %s", s.name)
		}
		seen[s.name] = true
		if (s.command == "") == (s.path == "") {
			t.Errorf("%s: set exactly one of command and path", s.name)
		}
		if s.path != "" && (s.width == 0 || s.height == 0) {
			t.Errorf("%s: missing viewport", s.name)
		}
	}
}

func TestShotScript(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	script := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(script, []byte(shotScript("echo hi --there")), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "\033[90m$\033[0m echo hi --there\nhi --there\n"; string(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestFreezeArgs(t *testing.T) {
	args := freezeArgs("cli-help.sh", "/tmp/cli-help.svg")
	flag := func(name string) string {
		i := slices.Index(args, name)
		if i < 0 || i+1 >= len(args) {
			t.Fatalf("%s missing from %v", name, args)
		}
		return args[i+1]
	}
	if got := flag("--execute"); got != "sh cli-help.sh" {
		t.Errorf("--execute = %q", got)
	}
	if got := flag("--output"); got != "/tmp/cli-help.svg" {
		t.Errorf("--output = %q", got)
	}
	if got := flag("--width"); got != "1480" {
		t.Errorf("--width = %q", got)
	}
	if !slices.Contains(args, "--window") {
		t.Error("no window controls")
	}
}

func TestShotEnv(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	env := shotEnv([]string{"HOME=/h", "NO_COLOR=1", "TERM=dumb", "PATH=/old"}, "/b", "/c")
	for _, want := range []string{"HOME=/h", "PATH=/b" + string(os.PathListSeparator) + "/usr/bin", "TERM=xterm-256color", "CLICOLOR_FORCE=1", "DOCUMANGO_CACHE_DIR=/c"} {
		if !slices.Contains(env, want) {
			t.Errorf("env lacks %s: %v", want, env)
		}
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "NO_COLOR=") || kv == "TERM=dumb" || kv == "PATH=/old" {
			t.Errorf("env keeps %s", kv)
		}
	}
}

func TestServingLine(t *testing.T) {
	m := servingLine.FindStringSubmatch("Serving documango at http://127.0.0.1:54321/")
	if m == nil || m[1] != "http://127.0.0.1:54321/" {
		t.Errorf("match = %v", m)
	}
}

func TestFrameHTML(t *testing.T) {
	img := dataURL("image/png", []byte("png"))
	tests := []struct {
		name  string
		f     frame
		want  []string
		avoid []string
	}{
		{"browser dark", frame{Kind: "browser", Image: img, Width: 1280, Dark: true, URL: "docs.example.com/guide/markdown/"},
			[]string{`class="bar"`, "docs.example.com/guide/markdown/", "width: 1280px", "--bar: #2b2d31", `src="data:image/png;base64,cG5n"`}, []string{`class="phone"`}},
		{"browser light", frame{Kind: "browser", Image: img, Width: 1280, URL: "docs.example.com/"},
			[]string{`class="bar"`, "--bar: #ececec"}, []string{"#2b2d31"}},
		{"phone", frame{Kind: "phone", Image: img, Width: 390, Dark: true},
			[]string{`class="phone"`, "width: 390px"}, []string{`class="bar"`}},
		{"terminal", frame{Kind: "terminal", Image: img, Width: 1480, Dark: true},
			[]string{`<div class="window"><img`}, []string{`class="bar"`, `class="phone"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := frameHTML(tt.f)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, a := range tt.avoid {
				if strings.Contains(html, a) {
					t.Errorf("unexpected %q", a)
				}
			}
		})
	}
}
