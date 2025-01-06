package view

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type Theme struct {
	System  string  `yaml:"system"`
	Name    string  `yaml:"name"`
	Author  string  `yaml:"author"`
	Variant string  `yaml:"variant"`
	Palette Palette `yaml:"palette"`
}

type Palette struct {
	Base00 string `yaml:"base00"`
	Base01 string `yaml:"base01"`
	Base02 string `yaml:"base02"`
	Base03 string `yaml:"base03"`
	Base04 string `yaml:"base04"`
	Base05 string `yaml:"base05"`
	Base06 string `yaml:"base06"`
	Base07 string `yaml:"base07"`
	Base08 string `yaml:"base08"`
	Base09 string `yaml:"base09"`
	Base0A string `yaml:"base0A"`
	Base0B string `yaml:"base0B"`
	Base0C string `yaml:"base0C"`
	Base0D string `yaml:"base0D"`
	Base0E string `yaml:"base0E"`
	Base0F string `yaml:"base0F"`
}

type themeCtx struct {
	Light *Theme
	Dark  *Theme
	Date  string
}

type styleCtx struct {
	ThemeSnippet string
}

// Unmarshal YAML file into a Theme struct
func ParseTheme(data []byte) (*Theme, error) {
	t := Theme{}
	err := yaml.Unmarshal(data, &t)
	if err != nil {
		return nil, fmt.Errorf("error parsing theme: %w", err)
	}
	return &t, nil
}

//go:embed themes/light/windows-nt.yml
var ExampleLightFile []byte

//go:embed themes/dark/oxocarbon-dark.yml
var ExampleDarkFile []byte

var ThemeCommand = &cli.Command{
	Name:   "theme",
	Usage:  "generate stylesheet",
	Action: Run,
}

func buildStack(errs []error, err error) []error {
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	return errs
}

func (t *themeCtx) buildStack(errs []error, err error, theme *Theme) []error {
	errs = buildStack(errs, err)
	if theme != nil {
		v := strings.ToLower(theme.Variant)
		if v == "dark" {
			t.Dark = theme
		} else {
			t.Light = theme
		}
	}

	return errs
}

func (t *themeCtx) withTime(layout ...string) *themeCtx {
	if layout != nil {
		t.Date = time.Now().Format(layout[0])
	} else {
		t.Date = time.Now().Format(time.RFC1123Z)
	}
	return t
}

func (s *styleCtx) with(t string) *styleCtx {
	s.ThemeSnippet = t
	return s
}

// function BuildTheme takes a theme slug to select a theme and then executes
// the theme variable & stylesheet templates. These are concatenated and then
// the contents are returns as a string
func BuildTheme(args ...string) string {
	theme_ctx := themeCtx{}
	style_ctx := styleCtx{}
	b := strings.Builder{}

	light_theme, err := ParseTheme(ExampleLightFile)
	errs := theme_ctx.buildStack([]error{}, err, light_theme)
	dark_theme, err := ParseTheme(ExampleDarkFile)
	errs = theme_ctx.buildStack(errs, err, dark_theme)

	if len(errs) == 2 {
		logger.Fatalf(
			"theme parsing failed \nLight: %v \nDark:%v",
			errs[0], errs[1],
		)
	}

	theme_template, err := template.ParseGlob("templates/_theme.css")
	if err != nil {
		logger.Fatalf("unable to read template dir %v", err)
	}

	if err = theme_template.Execute(&b, theme_ctx.withTime(time.Kitchen)); err != nil {
		logger.Fatalf("unable to execute template %v", err)
	}

	theme := b.String()
	b.Reset()

	style_template, err := template.ParseGlob("templates/_style.css")
	if err = style_template.Execute(&b, style_ctx.with(theme)); err != nil {
		logger.Fatalf("unable to execute template %v", err)
	}

	return b.String()
}

func Run(ctx context.Context, c *cli.Command) error {
	theme := BuildTheme()
	logger.Infof("generated stylesheet \n%v", theme)
	return nil
}
