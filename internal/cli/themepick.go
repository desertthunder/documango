package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/desertthunder/documango/internal/theme"
)

const pickExample = `  # Pick a dark theme and preview the docs with it
  documango docs --dark-theme "$(documango themes pick --variant dark)"

  # Pick several light themes; the first one you select is the default
  documango build docs --light-theme "$(documango themes pick --variant light --multi)"`

func (a *app) newPickCmd(opts *themesOptions) *cobra.Command {
	var multi bool
	cmd := &cobra.Command{
		Use:   "pick",
		Short: "Choose themes interactively",
		Long: `Browse the themes in your terminal with a live preview, then print the name
of the one you choose. Type to filter, move with the arrow keys (or Ctrl+J and
Ctrl+K), press Enter to choose and Esc to cancel.

With --multi, Space selects or clears a theme and Enter prints the selected
names separated by commas, in the order you selected them. Only the chosen
names go to standard output, so the result can be passed straight to
--dark-theme or --light-theme.`,
		Example: pickExample,
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.variant != "" && opts.variant != "dark" && opts.variant != "light" {
				return &usageError{fmt.Errorf("invalid --variant %q: use dark or light", opts.variant)}
			}
			in, _ := a.env.Stdin.(*os.File)
			out, _ := a.env.Stderr.(*os.File)
			if in == nil || out == nil || !term.IsTerminal(in.Fd()) || !term.IsTerminal(out.Fd()) {
				return withHint(errors.New("themes pick needs an interactive terminal"),
					"Run 'documango themes' to list the themes instead.")
			}
			schemes, err := a.schemes(cmd.Context(), opts)
			if err != nil {
				return err
			}
			final, err := tea.NewProgram(newPicker(schemes, opts.variant, multi),
				tea.WithContext(cmd.Context()), tea.WithInput(in), tea.WithOutput(out),
				tea.WithEnvironment(a.env.Environ)).Run()
			if err != nil {
				return fmt.Errorf("theme picker: %w", err)
			}
			chosen := final.(*picker).chosen
			if len(chosen) == 0 {
				return errCancelled
			}
			fmt.Fprintln(a.stdout, strings.Join(chosen, ","))
			return nil
		},
	}
	cmd.Flags().BoolVar(&multi, "multi", false, "select several themes with Space and print them comma-separated")
	return cmd
}

// picker is the Bubble Tea model of themes pick.
type picker struct {
	schemes []theme.Scheme
	title   string
	multi   bool

	filter  textinput.Model
	matches []theme.Scheme
	cursor  int
	offset  int // index of the first visible match
	height  int // terminal rows; 0 until the size is known

	selected []string // slugs selected with --multi, in selection order
	chosen   []string // set when the user confirms
}

func newPicker(schemes []theme.Scheme, variant string, multi bool) *picker {
	title := "Pick a " + variant + " theme"
	if multi {
		title = "Pick " + variant + " themes"
	}
	title = strings.Join(strings.Fields(title), " ")
	filter := textinput.New()
	filter.Prompt = "> "
	filter.Placeholder = "type to filter"
	filter.Focus()
	return &picker{schemes: schemes, title: title, multi: multi, filter: filter, matches: schemes}
}

func (p *picker) Init() tea.Cmd { return nil }

// listRows is how many matches fit on screen.
func (p *picker) listRows() int {
	if p.height == 0 {
		return 12
	}
	return max(p.height-5, 3)
}

func (p *picker) move(delta int) {
	p.cursor = min(max(p.cursor+delta, 0), max(len(p.matches)-1, 0))
	rows := p.listRows()
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+rows {
		p.offset = p.cursor - rows + 1
	}
}

func (p *picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.height = msg.Height
		p.move(0)
		return p, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return p, tea.Quit
		case "enter":
			switch {
			case len(p.selected) > 0:
				p.chosen = p.selected
			case len(p.matches) > 0:
				p.chosen = []string{p.matches[p.cursor].Slug}
			default:
				return p, nil
			}
			return p, tea.Quit
		case "up", "ctrl+k", "ctrl+p":
			p.move(-1)
			return p, nil
		case "down", "ctrl+j", "ctrl+n":
			p.move(1)
			return p, nil
		case "pgup":
			p.move(-p.listRows())
			return p, nil
		case "pgdown":
			p.move(p.listRows())
			return p, nil
		case "space":
			if p.multi {
				if len(p.matches) > 0 {
					slug := p.matches[p.cursor].Slug
					if i := slices.Index(p.selected, slug); i >= 0 {
						p.selected = slices.Delete(p.selected, i, i+1)
					} else {
						p.selected = append(p.selected, slug)
					}
				}
				return p, nil
			}
		}
	}
	before := p.filter.Value()
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(msg)
	if query := p.filter.Value(); query != before {
		p.matches = filterSchemes(p.schemes, query)
		p.cursor, p.offset = 0, 0
	}
	return p, cmd
}

// filterSchemes returns the schemes whose slug or name match query, best
// first: prefix matches, then substring matches, then the rest whose letters
// appear in order.
func filterSchemes(schemes []theme.Scheme, query string) []theme.Scheme {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return schemes
	}
	var ranked [3][]theme.Scheme
	for _, s := range schemes {
		text := s.Slug + " " + strings.ToLower(s.Name)
		switch {
		case strings.HasPrefix(text, query):
			ranked[0] = append(ranked[0], s)
		case strings.Contains(text, query):
			ranked[1] = append(ranked[1], s)
		default:
			rest, ok := text, true
			for _, r := range query {
				i := strings.IndexRune(rest, r)
				if ok = i >= 0; !ok {
					break
				}
				rest = rest[i+utf8.RuneLen(r):]
			}
			if ok {
				ranked[2] = append(ranked[2], s)
			}
		}
	}
	return slices.Concat(ranked[0], ranked[1], ranked[2])
}

var (
	pickCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Cyan).Bold(true)
	pickCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
)

func (p *picker) View() tea.View {
	var list strings.Builder
	width := 0
	for _, s := range p.schemes {
		width = max(width, len(s.Slug))
	}
	slugStyle := lipgloss.NewStyle().Width(width + 2)
	end := min(p.offset+p.listRows(), len(p.matches))
	for i := p.offset; i < end; i++ {
		s := p.matches[i]
		marker := "  "
		if i == p.cursor {
			marker = pickCursorStyle.Render("› ")
		}
		if p.multi {
			if slices.Contains(p.selected, s.Slug) {
				marker += pickCheckStyle.Render("✓ ")
			} else {
				marker += "  "
			}
		}
		var swatch strings.Builder
		for _, c := range s.Palette[8:] {
			swatch.WriteString(lipgloss.NewStyle().Background(lipgloss.Color(c)).Render(" "))
		}
		slug := slugStyle.Render(s.Slug)
		if i == p.cursor {
			slug = slugStyle.Bold(true).Render(s.Slug)
		}
		fmt.Fprintf(&list, "%s%s  %s%s\n", marker, swatch.String(), slug, dimStyle.Render(s.Variant))
	}
	if len(p.matches) == 0 {
		list.WriteString(dimStyle.Render("  No themes match") + "\n")
	}

	body := strings.TrimSuffix(list.String(), "\n")
	if len(p.matches) > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, "    ", preview(p.matches[p.cursor]))
	}
	help := "↑/↓ move · enter choose · esc cancel"
	if p.multi {
		help = "↑/↓ move · space select · enter confirm · esc cancel"
	}
	count := fmt.Sprintf("  %d/%d", len(p.matches), len(p.schemes))
	if p.multi && len(p.selected) > 0 {
		count += fmt.Sprintf(" · %d selected", len(p.selected))
	}
	v := tea.NewView(boldStyle.Render(p.title) + dimStyle.Render(count) + "\n" +
		p.filter.View() + "\n" + body + "\n" + dimStyle.Render(help))
	v.AltScreen = true
	return v
}

// preview renders a small code sample in the colours of s.
func preview(s theme.Scheme) string {
	bg := lipgloss.Color(s.Palette[0])
	span := func(base int, text string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(s.Palette[base])).Render(text)
	}
	lines := [][]string{
		{span(3, "// "+s.Name)},
		{span(0xE, "func "), span(0xD, "greet"), span(5, "(name "), span(0xA, "string"), span(5, ") {")},
		{span(5, "    msg := "), span(0xB, `"Hello, "`), span(5, " + name")},
		{span(5, "    "), span(0xE, "if "), span(5, "len(msg) > "), span(9, "40"), span(5, " {")},
		{span(5, "        "), span(8, "panic"), span(5, "("), span(0xB, `"too long"`), span(5, ")")},
		{span(5, "    }")},
		{span(5, "    "), span(0xE, "return "), span(0xC, "strings"), span(5, ".TrimSpace(msg)")},
		{span(5, "}")},
	}
	const width = 36
	var b strings.Builder
	b.WriteString(boldStyle.Render(s.Name) + dimStyle.Render("  "+s.Variant) + "\n")
	if s.Author != "" {
		b.WriteString(dimStyle.Render(s.Author) + "\n")
	}
	pad := span(5, strings.Repeat(" ", width+2))
	b.WriteString(pad + "\n")
	for _, l := range lines {
		line := span(5, " ") + strings.Join(l, "")
		if w := lipgloss.Width(line); w < width+2 {
			line += span(5, strings.Repeat(" ", width+2-w))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(pad)
	return b.String()
}
