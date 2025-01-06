// Methods for handling TOML frontmatter
package view

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/BurntSushi/toml"
)

type Frontmatter struct {
	Title string `toml:"title"`
}

func SplitFrontmatter(content []byte) (*Frontmatter, []byte, error) {
	reader := bufio.NewReader(bytes.NewReader(content))

	start, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, nil, err
	}

	if !strings.HasPrefix(strings.TrimSpace(start), "+++") {
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

		if in_fm {
			fm.WriteString(line)
		} else {
			b.WriteString(line)
		}

		if err == io.EOF {
			break
		}
	}

	t := Frontmatter{}
	toml.Unmarshal(bytes.TrimSpace(fm.Bytes()), &t)

	return &t, bytes.TrimSpace(b.Bytes()), nil
}
