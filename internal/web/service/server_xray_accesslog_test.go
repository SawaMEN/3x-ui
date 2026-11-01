package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXrayAccessLogPreservesLargeEntriesAndNewestHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	accessPath := filepath.Join(dir, "access.log")
	config := `{"log":{"access":` + "\"" + accessPath + "\"" + `}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	largeDestination := strings.Repeat("a", 70*1024) + ".example"
	lines := []string{
		"2026/11/01 10:00:00.000001 from 127.0.0.1:1000 accepted first.example:443 [outbound]",
		"2026/11/01 10:00:01.000001 from 127.0.0.1:1001 accepted " + largeDestination + ":443 [outbound]",
	}
	if err := os.WriteFile(accessPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := (&ServerService{}).GetXrayLogs("10", "", "true", "true", "true", nil, nil)
	if len(got) != 2 {
		t.Fatalf("large access entry silently lost: found %d entries", len(got))
	}
	if got[0].ToAddress != "first.example:443" || got[1].ToAddress != largeDestination+":443" {
		t.Fatal("lost original access destination or chronology")
	}
}
