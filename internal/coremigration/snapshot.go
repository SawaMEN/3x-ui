package coremigration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

type savedFile struct {
	Path   string `json:"path"`
	Backup string `json:"backup,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	Exists bool   `json:"exists"`
}
type manifest struct {
	DatabaseID          string      `json:"databaseId"`
	Schema              int         `json:"schema"`
	Revision            string      `json:"revision"`
	CoreType            string      `json:"coreType"`
	DBKind              string      `json:"dbKind"`
	Phase               string      `json:"phase"`
	PostgresSHA256      string      `json:"postgresSha256,omitempty"`
	PostgresFingerprint string      `json:"postgresFingerprint,omitempty"`
	PostgresTables      []string    `json:"postgresTables,omitempty"`
	Files               []savedFile `json:"files"`
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".core-migration-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func saveManifest(dir string, m *manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "manifest.json"), data, 0o600)
}

func snapshotFiles(dir string, paths []string) ([]savedFile, error) {
	result := []savedFile{}
	for i, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		s := savedFile{Path: absolute}
		info, err := os.Lstat(absolute)
		if os.IsNotExist(err) {
			result = append(result, s)
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("migration snapshot requires a regular file: %s", absolute)
		}
		s.Exists, s.Mode, s.Backup = true, uint32(info.Mode().Perm()), fmt.Sprintf("files/%03d", i)
		digest, err := atomicCopy(filepath.Join(dir, s.Backup), absolute, 0o600)
		if err != nil {
			return nil, err
		}
		s.SHA256 = digest
		result = append(result, s)
	}
	return result, nil
}

func verifyFiles(dir string, files []savedFile) error {
	for _, f := range files {
		if !f.Exists {
			continue
		}
		// Backup names are never arbitrary paths supplied by a manifest.
		if filepath.Dir(f.Backup) != "files" || filepath.Base(f.Backup) == "." {
			return fmt.Errorf("invalid snapshot entry")
		}
		hash, err := hashFile(filepath.Join(dir, f.Backup))
		if err != nil {
			return err
		}
		if hash != f.SHA256 {
			return fmt.Errorf("migration backup checksum mismatch: %s", f.Path)
		}
	}
	return nil
}

func restoreFiles(dir string, files []savedFile) error {
	if err := verifyFiles(dir, files); err != nil {
		return err
	}
	for _, f := range files {
		if !f.Exists {
			if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if _, err := atomicCopy(f.Path, filepath.Join(dir, f.Backup), os.FileMode(f.Mode)); err != nil {
			return err
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func atomicCopy(dst, src string, mode os.FileMode) (string, error) {
	input, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	output, err := os.CreateTemp(filepath.Dir(dst), ".core-migration-*")
	if err != nil {
		return "", err
	}
	name := output.Name()
	defer os.Remove(name)
	if err := output.Chmod(mode); err != nil {
		output.Close()
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, hash), input); err != nil {
		output.Close()
		return "", err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(name, dst); err != nil {
		return "", err
	}
	parent, err := os.Open(filepath.Dir(dst))
	if err != nil {
		return "", err
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyPostgres(dir string, m *manifest) error {
	if m.DBKind != "postgres" {
		return nil
	}
	hash, err := hashFile(filepath.Join(dir, "database.dump"))
	if err != nil {
		return err
	}
	if hash != m.PostgresSHA256 || hash == "" {
		return fmt.Errorf("PostgreSQL backup checksum mismatch")
	}
	return nil
}

// A prepared backup must still describe the stopped SQLite database. Otherwise
// rollback would discard users/traffic written after --backup-only.
func verifySQLiteSource(m *manifest) error {
	if m.DBKind != "sqlite" {
		return nil
	}
	db, err := filepath.Abs(config.GetDBPath())
	if err != nil {
		return err
	}
	for _, f := range m.Files {
		if f.Path != db && f.Path != db+"-wal" && f.Path != db+"-shm" && f.Path != db+"-journal" {
			continue
		}
		_, err := os.Lstat(f.Path)
		if os.IsNotExist(err) && !f.Exists {
			continue
		}
		if err != nil {
			return fmt.Errorf("SQLite database changed since backup; use a new backup directory: %w", err)
		}
		if !f.Exists {
			return fmt.Errorf("SQLite database changed since backup; use a new backup directory")
		}
		hash, err := hashFile(f.Path)
		if err != nil {
			return err
		}
		if hash != f.SHA256 {
			return fmt.Errorf("SQLite database changed since backup; use a new backup directory")
		}
	}
	return nil
}
