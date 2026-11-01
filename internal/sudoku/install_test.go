package sudoku

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestValidPrivateKey(t *testing.T) {
	if ValidPrivateKey("zz") {
		t.Fatal("expected invalid key")
	}
	valid64 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !ValidPrivateKey(valid64) {
		t.Fatal("expected master key to be valid")
	}
	if !ValidPrivateKey(valid64 + valid64) {
		t.Fatal("expected split key to be valid")
	}
}

func TestExtractBinaryAcceptsOfficialSudokuName(t *testing.T) {
	t.Parallel()

	const payload = "sudoku-binary"
	t.Run("tar.gz", func(t *testing.T) {
		archivePath := filepath.Join(t.TempDir(), "sudoku-linux-amd64.tar.gz")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tarWriter := tar.NewWriter(gz)
		if err := tarWriter.WriteHeader(&tar.Header{Name: "sudoku", Mode: 0o755, Size: int64(len(payload))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, payload); err != nil {
			t.Fatal(err)
		}
		if err := tarWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}

		extracted, err := extractBinary(archivePath, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		checkExtractedBinary(t, extracted, payload)
	})

	t.Run("zip", func(t *testing.T) {
		archivePath := filepath.Join(t.TempDir(), "sudoku-windows-amd64.zip")
		var buf bytes.Buffer
		zipWriter := zip.NewWriter(&buf)
		entry, err := zipWriter.Create("package/sudoku.exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, payload); err != nil {
			t.Fatal(err)
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}

		extracted, err := extractBinary(archivePath, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		checkExtractedBinary(t, extracted, payload)
	})
}

func checkExtractedBinary(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("extracted binary = %q, want %q", got, want)
	}
}
