package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/desertthunder/documango/internal/theme"
)

const themesExample = `  # List every theme
  documango themes

  # List only light themes
  documango themes --variant light

  # List the built-in themes without going online
  documango themes --offline

  # Use a theme when previewing
  documango --dark-theme dracula --light-theme github`

// themesOptions are the flags shared by themes and themes pick.
type themesOptions struct {
	variant string
	offline bool
}

// schemes returns the themes to show: the built-in ones plus, unless
// offline, the full downloaded list, filtered by variant.
func (a *app) schemes(ctx context.Context, o *themesOptions) ([]theme.Scheme, error) {
	if o.variant != "" && o.variant != "dark" && o.variant != "light" {
		return nil, &usageError{fmt.Errorf("invalid --variant %q: use dark or light", o.variant)}
	}
	schemes := theme.Builtin()
	if !o.offline {
		var err error
		if schemes, err = a.catalog().All(ctx); err != nil {
			a.logger().Warn("showing the built-in themes only: the full theme list could not be downloaded", "err", err)
		}
	}
	return slices.DeleteFunc(schemes, func(s theme.Scheme) bool {
		return o.variant != "" && s.Variant != o.variant
	}), nil
}

func (a *app) newThemesCmd() *cobra.Command {
	opts := &themesOptions{}
	cmd := &cobra.Command{
		Use:   "themes",
		Short: "List the colour themes",
		Long: `List the colour themes you can pass to --dark-theme and --light-theme: the
built-in ones plus every base16 scheme from tinted-theming, which is downloaded
once and cached. Use --offline to list only the built-in themes.

In a terminal each theme is shown with a colour sample. When the output is
piped, each line holds the name, variant, display name and source (builtin
or download) separated by tabs.`,
		Example: themesExample,
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			schemes, err := a.schemes(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if !a.stdoutTTY {
				for _, s := range schemes {
					source := "download"
					if _, ok := theme.Lookup(s.Slug); ok {
						source = "builtin"
					}
					fmt.Fprintf(a.stdout, "%s\t%s\t%s\t%s\n", s.Slug, s.Variant, s.Name, source)
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
				tag := ""
				if _, ok := theme.Lookup(s.Slug); ok {
					tag = dimStyle.Render("  built-in")
				}
				fmt.Fprintf(a.stdout, "%s%s%s%s%s\n", swatch.String(), slugStyle.Render(s.Slug), variantStyle.Render(s.Variant), s.Name, tag)
			}
			return nil
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringVar(&opts.variant, "variant", "", "only show dark or light themes")
	pf.BoolVar(&opts.offline, "offline", false, "only show the built-in themes, without going online")
	cmd.AddCommand(a.newPickCmd(opts))
	return cmd
}
