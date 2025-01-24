// package md handles markdown and frontmatter (TOML & YAML) parsing
package md

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/desertthunder/documango/internal/utils"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"gopkg.in/yaml.v3"
)

//go:embed content
var SampleContentDir embed.FS

type Frontmatter struct {
	Title  string `toml:"title" yaml:"title"`
	Layout string `toml:"layout" yaml:"layout"`
	Draft  bool   `toml:"draft" yaml:"draft"`
}

type MD struct {
	FilePath    string
	Frontmatter *Frontmatter
	Content     []byte
}

func SplitFrontmatter(content []byte) (*Frontmatter, []byte, error) {
	reader := bufio.NewReader(bytes.NewReader(content))
	start, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, nil, err
	}

	is_toml := strings.HasPrefix(strings.TrimSpace(start), "+++")
	is_yaml := strings.HasPrefix(strings.TrimSpace(start), "---")
	if !is_toml && !is_yaml {
		return nil, content, nil
	}

	fm := *bytes.NewBuffer([]byte{})
	b := *bytes.NewBuffer([]byte{})
	in_fm := true

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, nil, err
		}

		if strings.HasPrefix(strings.TrimSpace(line), "+++") {
			in_fm = false
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "---") {
			in_fm = false
			continue
		}

		if in_fm {
			fm.WriteString(line)
		} else {
			b.WriteString(line)
		}

		if err == io.EOF {
			break
		}
	}

	t := Frontmatter{Draft: false}

	if is_toml {
		toml.Unmarshal(bytes.TrimSpace(fm.Bytes()), &t)
	} else if is_yaml {
		yaml.Unmarshal(bytes.TrimSpace(fm.Bytes()), &t)
	}

	return &t, bytes.TrimSpace(b.Bytes()), nil
}

func OpenContentFile(fp string) (*MD, error) {
	var md MD
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, fmt.Errorf("unable to read data from %v: %v", fp, err.Error())
	}

	frontmatter, content, err := SplitFrontmatter(data)

	md = MD{
		FilePath:    fp,
		Frontmatter: frontmatter,
		Content:     content,
	}

	if err != nil {
		err = fmt.Errorf("unable to read content %v", err)
		return &md, err
	}

	return &md, nil
}

// ReadContentDirectory recursively calls constructors on a
// provided directory and creates pointers to views
func ReadContentDirectory(dir string, tdir string) ([]*MD, error) {
	entries, err := os.ReadDir(dir)
	mdFiles := []*MD{}
	if err != nil && os.IsNotExist(err) {
		fp := "README.md"
		data, _ := SampleContentDir.ReadFile(fp)
		frontmatter, content, _ := SplitFrontmatter(data)

		mdFiles = append(mdFiles, &MD{
			FilePath:    fp,
			Frontmatter: frontmatter,
			Content:     content,
		})

		return mdFiles, errors.New("using sample file for content")
	} else if err != nil {
		return mdFiles, fmt.Errorf("unable to create views for directory %v: %v", dir, err.Error())
	}

	for _, entry := range entries {
		fpath := fmt.Sprintf("%v/%v", dir, entry.Name())
		if entry.IsDir() {
			nestedMD, err := ReadContentDirectory(fpath, tdir)
			if err != nil && len(nestedMD) == 0 {
				return []*MD{}, err
			}

			mdFiles = slices.Concat(mdFiles, nestedMD)
			continue
		}

		if utils.IsNotMarkdown(entry.Name()) {
			continue
		}

		mdFile, err := OpenContentFile(fpath)
		if err != nil {
			return []*MD{}, err
		}

		if mdFile == nil ||
			(mdFile.Frontmatter != nil && mdFile.Frontmatter.Draft) {
			continue
		}

		mdFiles = append(mdFiles, mdFile)

	}

	return mdFiles, nil
}

func (m MD) HTML() []byte {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock)
	renderer := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags | html.HrefTargetBlank})
	doc := p.Parse(m.Content)
	return markdown.Render(doc, renderer)
}
