package recovery

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

const (
	magic   = "3XUI-RECOVERY-1\n"
	MaxSize = 512 << 20
)

type File struct {
	Path        string `json:"path"`
	RestorePath string `json:"restorePath"`
	SHA256      string `json:"sha256"`
	Data        []byte `json:"data"`
}
type Bundle struct {
	Format       int       `json:"format"`
	CreatedAt    time.Time `json:"createdAt"`
	PanelVersion string    `json:"panelVersion"`
	DatabaseType string    `json:"databaseType"`
	CoreType     string    `json:"coreType"`
	CoreVersion  string    `json:"coreVersion"`
	Files        []File    `json:"files"`
}

func (b *Bundle) Add(name, restorePath string, data []byte) {
	sum := sha256.Sum256(data)
	b.Files = append(b.Files, File{Path: name, RestorePath: restorePath, SHA256: hex.EncodeToString(sum[:]), Data: data})
}

func derive(password string, salt []byte) (cipher.AEAD, error) {
	if len(password) < 16 || len(password) > 4096 {
		return nil, errors.New("recovery password must contain 16–4096 bytes")
	}
	key, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func Encode(b Bundle, password string) ([]byte, error) {
	b.Format = 1
	if err := b.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxSize {
		return nil, errors.New("recovery bundle exceeds size limit")
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err = gz.Write(raw); err != nil {
		return nil, err
	}
	if err = gz.Close(); err != nil {
		return nil, err
	}
	clear(raw)
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	aead, err := derive(password, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	header := append([]byte(magic), salt...)
	header = append(header, nonce...)
	return aead.Seal(header, nonce, compressed.Bytes(), header), nil
}

func Decode(data []byte, password string) (Bundle, error) {
	var b Bundle
	headerSize := len(magic) + 16 + 12
	if len(data) > MaxSize || len(data) < headerSize+16 || !bytes.HasPrefix(data, []byte(magic)) {
		return b, errors.New("invalid recovery bundle")
	}
	aead, err := derive(password, data[len(magic):len(magic)+16])
	if err != nil {
		return b, err
	}
	plain, err := aead.Open(nil, data[headerSize-12:headerSize], data[headerSize:], data[:headerSize])
	if err != nil {
		return b, errors.New("incorrect recovery password or damaged bundle")
	}
	defer clear(plain)
	gz, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		return b, err
	}
	defer gz.Close()
	raw, err := io.ReadAll(io.LimitReader(gz, MaxSize+1))
	if err != nil {
		return b, err
	}
	defer clear(raw)
	if len(raw) > MaxSize {
		return b, errors.New("recovery bundle expands beyond size limit")
	}
	if err = json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	return b, b.Validate()
}

func (b Bundle) Validate() error {
	if b.Format != 1 {
		return errors.New("unsupported recovery format")
	}
	if b.DatabaseType != "sqlite" && b.DatabaseType != "postgres" {
		return errors.New("unknown recovery database type")
	}
	names := map[string]bool{}
	for _, file := range b.Files {
		name := filepath.ToSlash(file.Path)
		if name == "" || name == "." || strings.Contains(name, "\\") || strings.Contains(name, ":") || filepath.IsAbs(name) || name != filepath.ToSlash(filepath.Clean(name)) || name == ".." || strings.HasPrefix(name, "../") || name == "manifest.json" || names[name] {
			return errors.New("invalid or duplicate recovery path")
		}
		names[name] = true
		sum := sha256.Sum256(file.Data)
		if file.SHA256 != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("recovery checksum mismatch for %s", name)
		}
	}
	if (b.DatabaseType == "sqlite" && !names["database/backup.db"]) || (b.DatabaseType == "postgres" && !names["database/backup.pgdump"]) {
		return errors.New("recovery bundle has no database")
	}
	return nil
}

// Extract only writes into a new directory; original absolute paths are reviewable metadata.
func Extract(b Bundle, directory string) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if directory == "" {
		return errors.New("provide a new recovery output directory")
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return err
	}
	for _, file := range b.Files {
		target := filepath.Join(directory, file.Path)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(file.Data)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	for i := range b.Files {
		b.Files[i].Data = nil
	}
	manifest, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0o600)
}
