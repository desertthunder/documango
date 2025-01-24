// package md handles markdown and frontmatter (TOML & YAML) parsing
package md

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Frontmatter struct {
	Title  string `toml:"title" yaml:"title"`
	Layout string `toml:"layout" yaml:"layout"`
	Draft  bool   `toml:"draft" yaml:"draft"`
}

type MD struct {
	Frontmatter Frontmatter
	Content     interface{}
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
