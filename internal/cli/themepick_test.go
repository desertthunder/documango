package cli

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/desertthunder/documango/internal/theme"
)

func key(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

// typeText sends each rune of s as a key press.
func typeText(p *picker, s string) {
	for _, r := range s {
		p.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func slugs(schemes []theme.Scheme) []string {
	out := make([]string, len(schemes))
	for i, s := range schemes {
		out[i] = s.Slug
	}
	return out
}

func testSchemes() []theme.Scheme {
	var out []theme.Scheme
	for _, slug := range []string{"dracula", "gruvbox-dark-medium", "nord", "tomorrow-night", "tomorrow"} {
		s, ok := theme.Lookup(slug)
		if !ok {
			panic("missing built-in theme " + slug)
		}
		out = append(out, s)
	}
	return out
}

func TestFilterSchemes(t *testing.T) {
	schemes := testSchemes()
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", []string{"dracula", "gruvbox-dark-medium", "nord", "tomorrow-night", "tomorrow"}},
		{"NIGHT", []string{"tomorrow-night"}},
		{"tom", []string{"tomorrow-night", "tomorrow"}},
		{"dark", []string{"gruvbox-dark-medium"}},
		{"gdm", []string{"gruvbox-dark-medium"}},
		{"or", []string{"nord", "tomorrow-night", "tomorrow", "gruvbox-dark-medium"}},
		{"zzz", []string{}},
	} {
		if got := slugs(filterSchemes(schemes, tc.query)); !slices.Equal(got, tc.want) {
			t.Errorf("filter %q = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestPickerSingle(t *testing.T) {
	p := newPicker(testSchemes(), "dark", false)
	if p.Init() != nil {
		t.Error("Init returned a command")
	}
	view := p.View()
	for _, w := range []string{"Pick a dark theme", "5/5", "› ", "dracula", "func ", "esc cancel"} {
		if !strings.Contains(view.Content, w) {
			t.Errorf("view lacks %q:\n%s", w, view.Content)
		}
	}
	if !view.AltScreen {
		t.Error("picker does not use the alternate screen")
	}

	p.Update(key(tea.KeyDown, 0))
	p.Update(key('j', tea.ModCtrl))
	p.Update(key('k', tea.ModCtrl))
	p.Update(key(tea.KeyUp, 0))
	p.Update(key(tea.KeyUp, 0)) // stays on the first row
	p.Update(key('n', tea.ModCtrl))
	if p.cursor != 1 {
		t.Errorf("cursor = %d, want 1", p.cursor)
	}
	typeText(p, "tom")
	if p.cursor != 0 || !slices.Equal(slugs(p.matches), []string{"tomorrow-night", "tomorrow"}) {
		t.Fatalf("after typing: cursor %d, matches %v", p.cursor, slugs(p.matches))
	}
	p.Update(key(tea.KeyPgDown, 0))
	if p.cursor != 1 {
		t.Errorf("pgdown: cursor = %d, want 1", p.cursor)
	}
	p.Update(key(tea.KeyPgUp, 0))
	p.Update(key(tea.KeyDown, 0))
	if !strings.Contains(p.View().Content, "// Tomorrow\x1b") {
		t.Errorf("preview does not show the highlighted theme:\n%s", p.View().Content)
	}
	_, cmd := p.Update(key(tea.KeyEnter, 0))
	if cmd == nil || !slices.Equal(p.chosen, []string{"tomorrow"}) {
		t.Errorf("enter: chosen %v, quit %v", p.chosen, cmd != nil)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("enter does not quit")
	}
}

func TestPickerNoMatch(t *testing.T) {
	p := newPicker(testSchemes(), "", false)
	if !strings.Contains(p.View().Content, "Pick a theme") {
		t.Errorf("title: %q", p.View().Content)
	}
	typeText(p, "zzz")
	if !strings.Contains(p.View().Content, "No themes match") {
		t.Errorf("view lacks empty state:\n%s", p.View().Content)
	}
	if _, cmd := p.Update(key(tea.KeyEnter, 0)); cmd != nil || p.chosen != nil {
		t.Error("enter with no matches chose something")
	}
	p.Update(key(tea.KeyBackspace, 0))
	p.Update(key(tea.KeyBackspace, 0))
	p.Update(key(tea.KeyBackspace, 0))
	if len(p.matches) != 5 {
		t.Errorf("clearing the filter left %d matches", len(p.matches))
	}
}

func TestPickerMulti(t *testing.T) {
	p := newPicker(testSchemes(), "light", true)
	if !strings.Contains(p.View().Content, "Pick light themes") || !strings.Contains(p.View().Content, "space select") {
		t.Errorf("multi view:\n%s", p.View().Content)
	}
	space := tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	p.Update(key(tea.KeyDown, 0))
	p.Update(key(tea.KeyDown, 0))
	p.Update(space) // nord
	p.Update(key(tea.KeyUp, 0))
	p.Update(key(tea.KeyUp, 0))
	p.Update(space) // dracula
	p.Update(key(tea.KeyDown, 0))
	p.Update(space) // gruvbox
	p.Update(space) // and clear it again
	if p.filter.Value() != "" {
		t.Errorf("space typed into the filter: %q", p.filter.Value())
	}
	if v := p.View().Content; !strings.Contains(v, "2 selected") || strings.Count(v, "✓") != 2 {
		t.Errorf("view does not show the selection:\n%s", v)
	}
	p.Update(key(tea.KeyEnter, 0))
	if !slices.Equal(p.chosen, []string{"nord", "dracula"}) {
		t.Errorf("chosen = %v, want selection order", p.chosen)
	}

	// With nothing selected, enter picks the highlighted theme.
	p = newPicker(testSchemes(), "", true)
	typeText(p, "zzz")
	p.Update(space)
	p.Update(key(tea.KeyEnter, 0))
	if p.chosen != nil {
		t.Errorf("chosen = %v with no matches", p.chosen)
	}
	p = newPicker(testSchemes(), "", true)
	p.Update(key(tea.KeyEnter, 0))
	if !slices.Equal(p.chosen, []string{"dracula"}) {
		t.Errorf("chosen = %v, want the highlighted theme", p.chosen)
	}
}

func TestPickerCancel(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{key(tea.KeyEscape, 0), key('c', tea.ModCtrl)} {
		p := newPicker(testSchemes(), "", true)
		p.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		_, cmd := p.Update(k)
		if cmd == nil || p.chosen != nil {
			t.Errorf("%s: chosen %v, quit %v", k, p.chosen, cmd != nil)
		}
	}
}

func TestPickerScroll(t *testing.T) {
	schemes := theme.Builtin()
	p := newPicker(schemes, "", false)
	p.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	if p.listRows() != 5 {
		t.Fatalf("listRows = %d, want 5", p.listRows())
	}
	for range 7 {
		p.Update(key(tea.KeyDown, 0))
	}
	if p.cursor != 7 || p.offset != 3 {
		t.Errorf("cursor %d offset %d, want 7 and 3", p.cursor, p.offset)
	}
	v := p.View().Content
	if strings.Contains(v, schemes[0].Slug+" ") || !strings.Contains(v, schemes[7].Slug) {
		t.Errorf("view does not scroll:\n%s", v)
	}
	p.Update(key(tea.KeyPgUp, 0))
	if p.cursor != 2 || p.offset != 2 {
		t.Errorf("pgup: cursor %d offset %d, want 2 and 2", p.cursor, p.offset)
	}
}
