package frontmatter

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		want  Meta
		body  string
		extra map[string]any
	}{
		{
			name: "no front matter",
			src:  "# Hello\n\ntext\n",
			body: "# Hello\n\ntext\n",
		},
		{
			name: "empty input",
			src:  "",
			body: "",
		},
		{
			name: "bom stripped without front matter",
			src:  "\xef\xbb\xbf# Hello\n",
			body: "# Hello\n",
		},
		{
			name: "all known fields",
			src:  "---\ntitle: Install\ndescription: How to install\norder: 3\ndraft: true\n---\n# Body\n",
			want: Meta{Title: "Install", Description: "How to install", Order: 3, Draft: true},
			body: "# Body\n",
		},
		{
			name:  "unknown keys kept in extra",
			src:   "---\ntitle: T\nauthor: jane\ntags: [a, b]\n---\nbody",
			want:  Meta{Title: "T"},
			body:  "body",
			extra: map[string]any{"author": "jane", "tags": []any{"a", "b"}},
		},
		{
			name: "bom before front matter",
			src:  "\xef\xbb\xbf---\ntitle: BOM\n---\nbody\n",
			want: Meta{Title: "BOM"},
			body: "body\n",
		},
		{
			name: "crlf and trailing spaces on delimiters",
			src:  "---  \r\ntitle: CRLF\r\n---\t\r\nbody\r\n",
			want: Meta{Title: "CRLF"},
			body: "body\r\n",
		},
		{
			name: "dots close the block",
			src:  "---\ntitle: Dots\n...\nbody\n",
			want: Meta{Title: "Dots"},
			body: "body\n",
		},
		{
			name: "empty block",
			src:  "---\n---\nbody\n",
			body: "body\n",
		},
		{
			name: "whitespace-only block",
			src:  "---\n\n  \n---\nbody\n",
			body: "body\n",
		},
		{
			name: "closing delimiter at end of file without newline",
			src:  "---\ntitle: EOF\n---",
			want: Meta{Title: "EOF"},
			body: "",
		},
		{
			name: "thematic break later in document is not front matter",
			src:  "# Title\n\n---\n\ntitle: nope\n---\n",
			body: "# Title\n\n---\n\ntitle: nope\n---\n",
		},
		{
			name: "opening line with extra text is not front matter",
			src:  "--- not yaml\ntitle: x\n---\n",
			body: "--- not yaml\ntitle: x\n---\n",
		},
		{
			name: "four dashes is not front matter",
			src:  "----\ntitle: x\n----\n",
			body: "----\ntitle: x\n----\n",
		},
		{
			name: "body keeps later thematic breaks",
			src:  "---\ntitle: A\n---\npara\n\n---\n\nmore\n",
			want: Meta{Title: "A"},
			body: "para\n\n---\n\nmore\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta, body, err := Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("Parse: unexpected error: %v", err)
			}
			if string(body) != tt.body {
				t.Errorf("body = %q, want %q", body, tt.body)
			}
			if meta.Title != tt.want.Title || meta.Description != tt.want.Description ||
				meta.Order != tt.want.Order || meta.Draft != tt.want.Draft {
				t.Errorf("meta = %+v, want %+v", meta, tt.want)
			}
			if len(meta.Extra) != len(tt.extra) {
				t.Fatalf("extra = %#v, want %#v", meta.Extra, tt.extra)
			}
			for k, v := range tt.extra {
				if got := meta.Extra[k]; !equalAny(got, v) {
					t.Errorf("extra[%q] = %#v, want %#v", k, got, v)
				}
			}
		})
	}
}

func equalAny(a, b any) bool {
	as, aok := a.([]any)
	bs, bok := b.([]any)
	if aok && bok {
		if len(as) != len(bs) {
			return false
		}
		for i := range as {
			if as[i] != bs[i] {
				return false
			}
		}
		return true
	}
	return a == b
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		{name: "unclosed", src: "---\ntitle: x\nbody\n", wantErr: "unclosed front matter"},
		{name: "unclosed only opener", src: "---\n", wantErr: "unclosed front matter"},
		{name: "invalid yaml", src: "---\ntitle: [unterminated\n---\n", wantErr: "front matter"},
		{name: "list instead of mapping", src: "---\n- a\n- b\n---\n", wantErr: "front matter"},
		{name: "scalar instead of mapping", src: "---\njust text\n---\n", wantErr: "front matter"},
		{name: "wrong field type", src: "---\norder: first\n---\n", wantErr: "front matter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Parse([]byte(tt.src))
			if err == nil {
				t.Fatal("Parse: expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}
