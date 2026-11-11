package service

import (
    "bytes"
    "context"
    "crypto/sha256"
    "crypto/tls"
    "crypto/x509"
    "encoding/hex"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/SawaMEN/3x-ui/v3/internal/config"
    "github.com/SawaMEN/3x-ui/v3/internal/database"
    "github.com/SawaMEN/3x-ui/v3/internal/database/model"
    "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

const maxDistributedPEMBytes = 64 << 10

var certificateDistributionMu sync.Mutex

// DistributedTLSCertificate is the public, path-only acknowledgement. Neither
// the certificate PEM nor its private key is returned to the panel browser.
type DistributedTLSCertificate struct {
    NodeID    int    `json:"nodeId,omitempty"`
    Domain    string `json:"domain"`
    CertFile  string `json:"certFile"`
    KeyFile   string `json:"keyFile"`
    ExpiresAt int64  `json:"expiresAt"`
}

func distributionDomain(value string) (string, error) {
    name, err := managerDomain(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), "."))
    if err != nil || strings.Contains(name, "*") || name == "" {
        return "", errors.New("a valid owned DNS hostname is required")
    }
    return name, nil
}

func validateDistributedPair(name string, certificatePEM, privateKeyPEM []byte) (int64, error) {
    if len(certificatePEM) == 0 || len(privateKeyPEM) == 0 ||
        len(certificatePEM) > maxDistributedPEMBytes || len(privateKeyPEM) > maxDistributedPEMBytes {
        return 0, errors.New("certificate or private key exceeds allowed size")
    }
    pair, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
    if err != nil || len(pair.Certificate) == 0 {
        return 0, errors.New("certificate and private key do not form a valid pair")
    }
    leaf, err := x509.ParseCertificate(pair.Certificate[0])
    if err != nil || leaf.VerifyHostname(name) != nil {
        return 0, errors.New("certificate does not cover the requested hostname")
    }
    now := time.Now()
    if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
        return 0, errors.New("certificate is not currently valid")
    }
    roots, err := x509.SystemCertPool()
    if err != nil {
        return 0, errors.New("system CA store is unavailable")
    }
    intermediates := x509.NewCertPool()
    for _, der := range pair.Certificate[1:] {
        intermediate, err := x509.ParseCertificate(der)
        if err != nil {
            return 0, errors.New("invalid intermediate certificate")
        }
        intermediates.AddCert(intermediate)
    }
    if _, err := leaf.Verify(x509.VerifyOptions{
        DNSName: name, Roots: roots, Intermediates: intermediates,
        KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
    }); err != nil {
        return 0, errors.New("certificate chain cannot be verified against trusted server CAs")
    }
    return leaf.NotAfter.Unix(), nil
}

func readDistributedSource(path string) ([]byte, error) {
    info, err := os.Stat(path)
    if err != nil { return nil, errors.New("managed certificate file is unavailable") }
    if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxDistributedPEMBytes {
        return nil, errors.New("managed certificate file size is invalid")
    }
    contents, err := os.ReadFile(path)
    if err != nil || len(contents) > maxDistributedPEMBytes {
        return nil, errors.New("managed certificate file cannot be read")
    }
    return contents, nil
}

// DistributeTLSCertificate sends only a locally registered certificate for an
// owned hostname. A node with HTTP or skip-verification is refused because
// forwarding a private key over such transport would expose the TLS identity.
func DistributeTLSCertificate(ctx context.Context, nodeID int, domain string) (DistributedTLSCertificate, error) {
    var empty DistributedTLSCertificate
    name, err := distributionDomain(domain)
    if err != nil { return empty, err }
    if nodeID <= 0 || database.GetDB() == nil { return empty, errors.New("select a directly managed node") }

    var node model.Node
    if err := database.GetDB().First(&node, nodeID).Error; err != nil { return empty, err }
    if !node.Enable || node.Transitive || node.Scheme != "https" || node.TlsVerifyMode == "skip" {
        return empty, errors.New("node must be enabled, direct, and use verified HTTPS (verify, pin or mTLS)")
    }

    sites, err := ManagedSNIDomains(ctx, false)
    if err != nil { return empty, err }
    var source *ManagedSNIDomain
    for i := range sites {
        if sites[i].Name == name && sites[i].CertFile != "" && sites[i].KeyFile != "" &&
            sites[i].Source != "remote" && sites[i].Source != "distributed" {
            source = &sites[i]
            break
        }
    }
    if source == nil { return empty, errors.New("main node has no valid managed certificate for this SNI") }
    cert, err := readDistributedSource(source.CertFile)
    if err != nil { return empty, err }
    key, err := readDistributedSource(source.KeyFile)
    if err != nil { return empty, err }
    expiry, err := validateDistributedPair(name, cert, key)
    if err != nil { return empty, err }

    manager := runtime.GetManager()
    if manager == nil { return empty, errors.New("node manager is unavailable") }
    remote, err := manager.RemoteFor(&node)
    if err != nil { return empty, err }
    installed, err := remote.InstallTLSCertificate(ctx, runtime.TLSCertificateInstall{
        Domain: name, CertificatePEM: string(cert), PrivateKeyPEM: string(key),
    })
    if err != nil { return empty, fmt.Errorf("node certificate delivery failed: %w", err) }
    if installed.ExpiresAt != expiry { return empty, errors.New("node returned unexpected certificate expiration") }
    return DistributedTLSCertificate{
        NodeID: nodeID, Domain: name, CertFile: installed.CertFile,
        KeyFile: installed.KeyFile, ExpiresAt: expiry,
    }, nil
}

func distributedTLSRoot() string {
    return filepath.Join(config.GetDBFolderPath(), "distributed-tls")
}

func distributedTLSNameDir(name string) string {
    sum := sha256.Sum256([]byte(name))
    return filepath.Join(distributedTLSRoot(), hex.EncodeToString(sum[:]))
}

func privateDistributionDirectory(path string) error {
    if err := os.MkdirAll(path, 0700); err != nil { return err }
    info, err := os.Lstat(path)
    if err != nil { return err }
    if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
        return errors.New("TLS destination is not a private directory")
    }
    return os.Chmod(path, 0700)
}

// InstallDistributedTLSCertificate stores the pair as an immutable version.
// Switching the current symlink is atomic: an existing inbound sees either
// the old complete pair or the new complete pair, never mixed PEM files.
func InstallDistributedTLSCertificate(payload runtime.TLSCertificateInstall) (DistributedTLSCertificate, error) {
    var empty DistributedTLSCertificate
    name, err := distributionDomain(payload.Domain)
    if err != nil || name != payload.Domain { return empty, errors.New("invalid certificate hostname") }
    certificatePEM, privateKeyPEM := []byte(payload.CertificatePEM), []byte(payload.PrivateKeyPEM)
    expiry, err := validateDistributedPair(name, certificatePEM, privateKeyPEM)
    if err != nil { return empty, err }

    certificateDistributionMu.Lock()
    defer certificateDistributionMu.Unlock()
    root, hostDir := distributedTLSRoot(), distributedTLSNameDir(name)
    versions := filepath.Join(hostDir, "versions")
    for _, directory := range []string{root, hostDir, versions} {
        if err := privateDistributionDirectory(directory); err != nil { return empty, err }
    }

    sum := sha256.Sum256(append(append([]byte{}, certificatePEM...), privateKeyPEM...))
    versionName := hex.EncodeToString(sum[:])
    versionDir := filepath.Join(versions, versionName)
    if _, statErr := os.Lstat(versionDir); errors.Is(statErr, os.ErrNotExist) {
        stage, err := os.MkdirTemp(versions, ".install-")
        if err != nil { return empty, err }
        defer os.RemoveAll(stage)
        if err := os.Chmod(stage, 0700); err != nil { return empty, err }
        if err := os.WriteFile(filepath.Join(stage, "fullchain.pem"), certificatePEM, 0600); err != nil { return empty, err }
        if err := os.WriteFile(filepath.Join(stage, "privkey.pem"), privateKeyPEM, 0600); err != nil { return empty, err }
        if err := os.Rename(stage, versionDir); err != nil {
            if _, statErr := os.Stat(versionDir); statErr != nil { return empty, err }
        }
    } else if statErr != nil { return empty, statErr }
    // Verify an existing content-addressed version before linking it.
    storedCert, err := readDistributedSource(filepath.Join(versionDir, "fullchain.pem"))
    if err != nil { return empty, err }
    storedKey, err := readDistributedSource(filepath.Join(versionDir, "privkey.pem"))
    if err != nil { return empty, err }
    if !bytes.Equal(storedCert, certificatePEM) || !bytes.Equal(storedKey, privateKeyPEM) {
        return empty, errors.New("stored certificate version failed integrity check")
    }

    // The host name marker is data only; paths always use a SHA-256 component.
    if err := os.WriteFile(filepath.Join(hostDir, "hostname"), []byte(name), 0600); err != nil { return empty, err }
    next := filepath.Join(hostDir, ".current-next")
    if err := os.Remove(next); err != nil && !errors.Is(err, os.ErrNotExist) { return empty, err }
    if err := os.Symlink(filepath.Join("versions", versionName), next); err != nil { return empty, err }
    if err := os.Rename(next, filepath.Join(hostDir, "current")); err != nil {
        _ = os.Remove(next)
        return empty, err
    }
    return DistributedTLSCertificate{
        Domain: name, CertFile: filepath.Join(hostDir, "current", "fullchain.pem"),
        KeyFile: filepath.Join(hostDir, "current", "privkey.pem"), ExpiresAt: expiry,
    }, nil
}

// LocalDistributedTLSCertificates returns previously delivered, locally
// validated paths after restart. No private key contents enter the inventory.
func LocalDistributedTLSCertificates() []DistributedTLSCertificate {
    entries, err := os.ReadDir(distributedTLSRoot())
    if err != nil { return nil }
    out := make([]DistributedTLSCertificate, 0)
    for _, entry := range entries {
        if len(out) >= 128 { break }
        if !entry.IsDir() || len(entry.Name()) != 64 { continue }
        folder := filepath.Join(distributedTLSRoot(), entry.Name())
        marker, err := os.ReadFile(filepath.Join(folder, "hostname"))
        if err != nil || len(marker) > 253 { continue }
        name, err := distributionDomain(string(marker))
        if err != nil || folder != distributedTLSNameDir(name) { continue }
        certFile := filepath.Join(folder, "current", "fullchain.pem")
        keyFile := filepath.Join(folder, "current", "privkey.pem")
        cert, err := readDistributedSource(certFile)
        if err != nil { continue }
        key, err := readDistributedSource(keyFile)
        if err != nil { continue }
        expiry, err := validateDistributedPair(name, cert, key)
        if err != nil { continue }
        out = append(out, DistributedTLSCertificate{
            Domain: name, CertFile: certFile, KeyFile: keyFile, ExpiresAt: expiry,
        })
    }
    return out
}
