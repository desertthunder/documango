package cli

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/desertthunder/documango/internal/theme"
)

const themesExample = `  # List every built-in theme
  documango themes

  # List only light themes
  documango themes --variant light

  # Use a theme when previewing
  documango --dark-theme dracula --light-theme github`

func (a *app) newThemesCmd() *cobra.Command {
	var variant string
	cmd := &cobra.Command{
		Use:   "themes",
		Short: "List the built-in themes",
		Long: `List the built-in colour themes you can pass to --dark-theme and --light-theme.

In a terminal each theme is shown with a colour sample. When the output is
piped, each line holds the name, variant, and display name separated by tabs.`,
		Example: themesExample,
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(*cobra.Command, []string) error {
			if variant != "" && variant != "dark" && variant != "light" {
				return &usageError{fmt.Errorf("invalid --variant %q: use dark or light", variant)}
			}
			schemes := slices.DeleteFunc(theme.Builtin(), func(s theme.Scheme) bool {
				return variant != "" && s.Variant != variant
			})
			if !a.stdoutTTY {
				for _, s := range schemes {
					fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", s.Slug, s.Variant, s.Name)
				}
				return nil
			}
			width := 0
			for _, s := range schemes {
				width = max(width, len(s.Slug))
			}
			slugStyle := boldStyle.Width(width + 2)
			variantStyle := dimStyle.Width(7)
			for _, s := range schemes {
				var swatch strings.Builder
				if a.stdout.Profile > colorprofile.NoTTY {
					for _, c := range s.Palette[8:] {
						swatch.WriteString(lipgloss.NewStyle().Background(lipgloss.Color(c)).Render("  "))
					}
					swatch.WriteString("  ")
				}
				fmt.Fprintf(a.stdout, "%s%s%s%s\n", swatch.String(), slugStyle.Render(s.Slug), variantStyle.Render(s.Variant), s.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&variant, "variant", "", "only list dark or light themes")
	return cmd
}
