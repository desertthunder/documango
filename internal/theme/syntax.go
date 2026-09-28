package theme

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
)

// tokenRules maps chroma token types to declarations following the base16
// styling guidelines. Types without an entry inherit from their parent
// sub-category or category.
var tokenRules = map[chroma.TokenType]string{
	chroma.Error: "color: var(--base08)",
	chroma.Other: "color: var(--base05)",

	chroma.Keyword:         "color: var(--base0E)",
	chroma.KeywordConstant: "color: var(--base09)",
	chroma.KeywordType:     "color: var(--base0A)",

	chroma.Name:              "color: var(--base05)",
	chroma.NameAttribute:     "color: var(--base0D)",
	chroma.NameBuiltin:       "color: var(--base0C)",
	chroma.NameBuiltinPseudo: "color: var(--base08)",
	chroma.NameClass:         "color: var(--base0A)",
	chroma.NameConstant:      "color: var(--base09)",
	chroma.NameDecorator:     "color: var(--base0C)",
	chroma.NameEntity:        "color: var(--base0C)",
	chroma.NameException:     "color: var(--base0A)",
	chroma.NameFunction:      "color: var(--base0D)",
	chroma.NameLabel:         "color: var(--base0A)",
	chroma.NameNamespace:     "color: var(--base0A)",
	chroma.NameProperty:      "color: var(--base08)",
	chroma.NameTag:           "color: var(--base08)",
	chroma.NameVariable:      "color: var(--base08)",

	chroma.Literal:        "color: var(--base09)",
	chroma.LiteralString:  "color: var(--base0B)",
	chroma.StringAffix:    "color: var(--base0E)",
	chroma.StringDoc:      "color: var(--base03); font-style: italic",
	chroma.StringEscape:   "color: var(--base0C)",
	chroma.StringInterpol: "color: var(--base0F)",
	chroma.StringRegex:    "color: var(--base0C)",
	chroma.StringSymbol:   "color: var(--base09)",
	chroma.LiteralNumber:  "color: var(--base09)",

	chroma.Operator:     "color: var(--base05)",
	chroma.OperatorWord: "color: var(--base0E)",
	chroma.Punctuation:  "color: var(--base05)",

	chroma.Comment:            "color: var(--base03); font-style: italic",
	chroma.CommentPreproc:     "color: var(--base0A)",
	chroma.CommentPreprocFile: "color: var(--base0B)",

	chroma.Generic:           "color: var(--base05)",
	chroma.GenericDeleted:    "color: var(--base08)",
	chroma.GenericEmph:       "font-style: italic",
	chroma.GenericError:      "color: var(--base08)",
	chroma.GenericHeading:    "color: var(--base0D); font-weight: bold",
	chroma.GenericInserted:   "color: var(--base0B)",
	chroma.GenericOutput:     "color: var(--base04)",
	chroma.GenericPrompt:     "color: var(--base03); font-weight: bold",
	chroma.GenericStrong:     "font-weight: bold",
	chroma.GenericSubheading: "color: var(--base0D)",
	chroma.GenericTraceback:  "color: var(--base08)",
	chroma.GenericUnderline:  "text-decoration: underline",

	chroma.Text: "color: var(--base05)",
}

const layoutCSS = `.bg { color: var(--base05); background-color: var(--base01); }
.chroma { color: var(--base05); background-color: var(--base01); }
.chroma .line { display: flex; }
.chroma .lntable { border-spacing: 0; padding: 0; margin: 0; border: 0; }
.chroma .lntd { vertical-align: top; padding: 0; margin: 0; border: 0; }
.chroma .lnlinks { outline: none; text-decoration: none; color: inherit; }
.chroma .ln { white-space: pre; user-select: none; margin-right: 0.4em; padding: 0 0.4em; color: var(--base03); }
.chroma .lnt { white-space: pre; user-select: none; margin-right: 0.4em; padding: 0 0.4em; color: var(--base03); }
.chroma .hl { background-color: var(--base02); }
`

// SyntaxCSS returns a stylesheet for HTML produced by chroma's html formatter
// with classes enabled and the default class prefix. Colors reference the
// --base00..--base0F custom properties from [Scheme.CSSVars], so the same
// stylesheet serves every scheme.
func SyntaxCSS() string {
	return syntaxCSS()
}

var syntaxCSS = sync.OnceValue(func() string {
	var types []chroma.TokenType
	for t, class := range chroma.StandardTypes {
		if class != "" && (t >= 0 || t == chroma.Error || t == chroma.Other) {
			types = append(types, t)
		}
	}
	slices.Sort(types)

	var b strings.Builder
	b.WriteString(layoutCSS)
	for _, t := range types {
		rule, ok := tokenRules[t]
		for p := t; !ok && p != 0; {
			p = p.Parent()
			rule, ok = tokenRules[p]
		}
		if ok {
			fmt.Fprintf(&b, ".chroma .%s { %s; }\n", chroma.StandardTypes[t], rule)
		}
	}
	return b.String()
})
