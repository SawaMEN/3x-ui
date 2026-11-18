package job

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveLogFileAndPruneByAge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	t.Setenv("XUI_BIN_FOLDER", dir)
	source := filepath.Join(dir, "active.log")
	if err := os.WriteFile(source, []byte("old session\nnew session\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := archiveLogFile(source, "sing-box", now); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil || len(data) != 0 {
		t.Fatalf("source not preserved/truncated: %q, %v", data, err)
	}
	archives, err := filepath.Glob(filepath.Join(logArchiveDir(), "sing-box-*.log.gz"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives=%v err=%v", archives, err)
	}
	f, err := os.Open(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "old session\nnew session\n" {
		t.Fatalf("archive content=%q err=%v", body, err)
	}
	pruneLogArchives(now, 7)
	if _, err := os.Stat(archives[0]); err != nil {
		t.Fatalf("fresh archive was removed: %v", err)
	}
	oldTime := now.Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(archives[0], oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(logArchiveDir(), "customer-data.log.gz")
	if err := os.WriteFile(unrelated, []byte("do not remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(unrelated, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	pruneLogArchives(now, 7)
	if _, err := os.Stat(archives[0]); !os.IsNotExist(err) {
		t.Fatalf("expired archive was not pruned: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestArchiveRejectsSymlinksAndDisabledPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	source := filepath.Join(dir, "real.log")
	if err := os.WriteFile(source, []byte("must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(source, link); err != nil {
		t.Skip(err)
	}
	if err := archiveLogFile(link, "xray-access", time.Now()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil || !strings.Contains(string(data), "must survive") {
		t.Fatalf("symlink target changed: %q err=%v", data, err)
	}
}
