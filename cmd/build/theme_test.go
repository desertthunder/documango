package build

import "testing"

func TestTheme(t *testing.T) {
	invalid_yaml := []byte(`
system = "base16"
slug = "catppuccin-mocha"
name = "Catppuccin Mocha"
author = "https://github.com/catppuccin/catppuccin"
variant = "dark"

[palette]
base00 = "#1e1e2e"
base01 = "#181825"
base02 = "#313244"
base03 = "#45475a"
base04 = "#585b70"
base05 = "#cdd6f4"
base06 = "#f5e0dc"
base07 = "#b4befe"
base08 = "#f38ba8"
base09 = "#fab387"
base0A = "#f9e2af"
base0B = "#a6e3a1"
base0C = "#94e2d5"
base0D = "#89b4fa"
base0E = "#cba6f7"
base0F = "#f2cdcd"`)

	valid_yaml := []byte(`
system: "base16"
name: "vice"
author: "Thomas Leon Highbaugh thighbaugh@zoho.com"
variant: "dark"
palette:
  base00: "#17191E"
  base01: "#22262d"
  base02: "#3c3f4c"
  base03: "#383a47"
  base04: "#555e70"
  base05: "#8b9cbe"
  base06: "#B2BFD9"
  base07: "#f4f4f7"
  base08: "#ff29a8"
  base09: "#85ffe0"
  base0A: "#f0ffaa"
  base0B: "#0badff"
  base0C: "#8265ff"
  base0D: "#00eaff"
  base0E: "#00f6d9"
  base0F: "#ff3d81"`)

	t.Run("ParseTheme", func(t *testing.T) {
		t.Run("can't parse toml", func(t *testing.T) {
			_, err := ParseTheme(invalid_yaml)

			if err == nil {
				t.Error("parse theme is a wrapper around yaml and toml should cause failure")
			}
		})

		t.Run("should parse a valid base16 yaml file", func(t *testing.T) {
			theme, err := ParseTheme(valid_yaml)

			if err != nil && theme == nil {
				t.Error("parse theme should parse a yaml spec for a base16 theme")
			}
		})
	})
}
