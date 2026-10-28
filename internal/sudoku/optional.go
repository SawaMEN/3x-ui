package sudoku

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The speed-enabled component is shipped as an optional payload, never active
// until the operator presses Install. Its source revision is its build version.
func installOptional(ctx context.Context, binDir string) (string, bool, error) {
	sourceDir := filepath.Join(binDir, "optional", "sudoku")
	source := GetBinaryPath(sourceDir)
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return "", false, nil
	} else if err != nil {
		return "", true, err
	}
	if err := ctx.Err(); err != nil {
		return "", true, err
	}
	version := readInstalledVersion(sourceDir)
	if version == "" {
		return "", true, fmt.Errorf("optional Sudoku package has no version")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", true, err
	}
	manifestData, err := os.ReadFile(filepath.Join(sourceDir, "speed-components.json"))
	if err != nil {
		return "", true, err
	}
	var manifest struct {
		Sudoku struct {
			SHA256 string `json:"binary_sha256"`
		} `json:"sudoku"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", true, err
	}
	hash := sha256.Sum256(data)
	if manifest.Sudoku.SHA256 != hex.EncodeToString(hash[:]) {
		return "", true, fmt.Errorf("optional Sudoku checksum mismatch")
	}
	staged, err := os.CreateTemp(binDir, ".sudoku-install-*")
	if err != nil {
		return "", true, err
	}
	defer staged.Close()
	defer os.Remove(staged.Name())
	if _, err = staged.Write(data); err != nil {
		return "", true, err
	}
	if err = staged.Chmod(0o755); err != nil {
		return "", true, err
	}
	if err = staged.Sync(); err != nil {
		return "", true, err
	}
	if err = staged.Close(); err != nil {
		return "", true, err
	}
	oldVersion := readInstalledVersion(binDir)
	if err = writeInstalledVersion(binDir, version); err != nil {
		return "", true, err
	}
	target := GetBinaryPath(binDir)
	if err = os.Rename(staged.Name(), target); err != nil {
		if oldVersion != "" {
			_ = writeInstalledVersion(binDir, oldVersion)
		} else {
			_ = os.Remove(installedVersionPath(binDir))
		}
		return "", true, err
	}
	return target, true, nil
}
