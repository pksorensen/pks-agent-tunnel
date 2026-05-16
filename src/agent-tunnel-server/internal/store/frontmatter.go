package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// writeFrontmatter writes `--- yaml --- body` to path atomically.
func writeFrontmatter(path string, v any, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("yaml encode: %w", err)
	}
	enc.Close()
	buf.WriteString("---\n")
	if body != "" {
		buf.WriteString("\n")
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteString("\n")
		}
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// readFrontmatter unmarshals the YAML block at the top of path into v.
// Returns os.ErrNotExist if path doesn't exist.
func readFrontmatter(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return fmt.Errorf("%s: missing leading ---", path)
	}
	rest := raw[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return fmt.Errorf("%s: missing closing ---", path)
	}
	return yaml.Unmarshal(rest[:end], v)
}
