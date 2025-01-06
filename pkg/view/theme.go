package view

import (
	"fmt"

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

// Unmarshal YAML file into a Theme struct
func ParseTheme(data []byte) (*Theme, error) {
	t := Theme{}
	err := yaml.Unmarshal(data, &t)
	if err != nil {
		return nil, fmt.Errorf("error parsing theme: %w", err)
	}
	return &t, nil
}
