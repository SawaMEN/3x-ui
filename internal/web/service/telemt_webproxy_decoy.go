package service

import (
	_ "embed"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

//go:embed telemt_web_decoy.html
var telemtWebDecoyHTML string

func writeTelemtWebDecoy(domain, dir string) error {
	if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 {
		return errors.New("invalid WEB site domain")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	index := filepath.Join(dir, "index.html")
	if info, err := os.Lstat(index); err == nil {
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("WEB site %s must be a non-empty regular file", index)
		}
		_, err = os.ReadFile(index)
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".index-*.html")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	content := strings.ReplaceAll(telemtWebDecoyHTML, "{{DOMAIN}}", html.EscapeString(domain))
	_, writeErr := tmp.WriteString(content)
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	// Publish a complete page without overwriting a concurrently installed site.
	if err := os.Link(tmp.Name(), index); err != nil {
		return fmt.Errorf("publish WEB site: %w", err)
	}
	return nil
}
