/*
Package view handles the generation of the site's rendered
markdown files.

From the filesystem, a markdown is file is loaded into a markup struct
that contains an abstract syntax tree (struct ast) and the source code.

The ast is generated by walking recursively through the documents treesitter
grammar and then using the range of each element to extract its string
contents. Take a look at `examples/test-ast.json` for an example of the
generated ast.
*/
package syntax

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/desertthunder/documango/cmd/libs/logs"

	ts "github.com/smacker/go-tree-sitter"
	ts_md "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
)

var logger = logs.CreateConsoleLogger("[view]")

// type markup contains attributes about a
// markdown file in the configured markup dir
type markup struct {
	// source code
	src []byte
	ast ast
}

// type ast represents the abstract syntax tree created by parsing the
type ast struct {
	DocType  string    `json:"docType"`
	Children []astNode `json:"children"`
}

// type astNode refers to a node on the abstract syntax tree
type astNode struct {
	ElType   string    `json:"element"`
	Depth    int       `json:"depth"`
	Content  string    `json:"contents"`
	Children []astNode `json:"children"`
}

// type Document is the HTML file created from the loaded template
type Document struct {
	// The name of the template
	Name     string
	Template *template.Template
}

// function setErrorTemplate creates an html element with the
// error message to send to the client.
//
// TODO: change to html attr in markup
func (m *markup) setErrorTemplate(err error) {
	m.src = []byte(fmt.Sprintf("<p>Something went wrong %v</p>", err.Error()))
}

// function loadMarkup is the Markup struct constructor. It takes a
// reference to a markdown file and stores its contents in the src attr.
func loadMarkup(f *os.File) *markup {
	m := markup{}
	info, err := f.Stat()
	if err != nil {
		logger.Errorf("unable to get file info %v %v", f.Name(), err.Error())
		m.setErrorTemplate(err)
		return &m
	}

	b := make([]byte, info.Size())
	_, err = f.Read(b)
	if err != nil {
		logger.Errorf("unable to read markup %v %v", f.Name(), err.Error())
		m.setErrorTemplate(err)

		return &m
	}

	m.src = b
	return &m
}

// function printNode is a debugging helper for tree construction
func printNode(n *ts.Node, depth int, name string, content string) {
	if logger.GetLevel() > -4 || n == nil {
		return
	}

	prefix := strings.Repeat("  ", depth)
	if name != "" {
		prefix += name + ": "
	}

	if content != "" {
		logger.Debugf("Content: %v", content)
	}

	logger.Debugf("%s%s [%d-%d]\n", prefix, n.Type(), n.StartByte(), n.EndByte())
}

// function visitNode recursively walks across the syntax tree
// generated by treesitter
func (m *markup) visitNode(n *ts.Node, depth int, parent *astNode) {
	printNode(n, depth, n.Type(), n.Content(m.src))

	ast_n := astNode{
		ElType:   n.Type(),
		Children: []astNode{},
		Depth:    int(n.ChildCount()),
		Content:  n.Content(m.src),
	}

	for i := range n.ChildCount() {
		child := n.Child(int(i))
		if child != nil {
			m.visitNode(child, depth+int(i), &ast_n)
		}
	}

	if ast_n.ElType != "" {
		logger.Info(ast_n.ElType)
		if parent != nil {
			parent.Children = append(parent.Children, ast_n)
		} else {
			m.ast.Children = append(m.ast.Children, ast_n)
		}
	}
}

// function createAst instantiates a parser based on the treesitter
// markdown grammar and then recursively traverses the document.
func (m *markup) createAst() {
	p := ts.NewParser()
	p.SetLanguage(ts_md.GetLanguage())

	tree, _ := p.ParseCtx(context.Background(), nil, m.src)
	root := tree.RootNode()

	logger.Debugf("%v elements in %v", root.ChildCount(), root.Type())

	m.ast = ast{DocType: root.Type(), Children: []astNode{}}
	m.visitNode(root, 0, nil)
}

// function attachMarkup takes an HTML template document and executes
// the template to create an in-memory string.
//
// TODO: implement this
func (m markup) attachMarkup(d Document) string {
	logger.Debugf("rendering markup as...\n%v", d.Name)
	return d.Name
}
