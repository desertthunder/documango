package md

import (
	"fmt"
	"testing"
)

type testCase struct {
	Desc    string
	Content []byte
}

func TestFrontmatter(t *testing.T) {
	want := Frontmatter{
		Title:  "Some Title",
		Draft:  true,
		Layout: "base",
	}

	toml_fm := []byte(`+++
title = "Some Title"
draft = true
layout = "base"
+++
`)

	yaml_fm := []byte(`---
title: "Some Title"
draft: true
layout: "base"
---
	`)

	t.Run("Split Frontmatter", func(t *testing.T) {
		cases := []testCase{
			{Desc: "yaml", Content: yaml_fm},
			{Desc: "toml", Content: toml_fm},
		}

		for _, tc := range cases {
			t.Run(fmt.Sprintf("handles %v", tc.Desc), func(t *testing.T) {
				got, _, err := SplitFrontmatter(tc.Content)

				if err != nil {
					t.Errorf("splitting %v failed %v", tc.Desc, err.Error())
				}

				if got == nil {
					t.Errorf("failed to parse %v frontmatter", tc.Desc)
				}

				if got.Title != want.Title {
					t.Errorf("got %v, want %v", got.Title, want.Title)
				}

				if got.Draft != want.Draft {
					t.Errorf("got %v, want %v", got.Draft, want.Draft)
				}

				if got.Layout != want.Layout {
					t.Errorf("got %v, want %v", got.Layout, want.Layout)
				}
			})
		}

	})
}
