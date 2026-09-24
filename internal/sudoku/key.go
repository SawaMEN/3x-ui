package sudoku

import (
    "context"
    "fmt"
    "os/exec"
    "regexp"
    "strings"
    "time"
)

var hexKeyRE = regexp.MustCompile(`^[0-9a-fA-F]{64}(?:[0-9a-fA-F]{64})?$`)

func ValidPrivateKey(key string) bool {
    return hexKeyRE.MatchString(strings.TrimSpace(key))
}

func GenerateMasterKey(ctx context.Context, binary string) (publicKey, privateKey string, err error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    out, err := exec.CommandContext(ctx, binary, "-keygen").CombinedOutput()
    if err != nil {
        return "", "", fmt.Errorf("Sudoku keygen failed: %w: %s", err, strings.TrimSpace(string(out)))
    }
    publicKey = findLabeledValue(string(out), "Master Public Key:")
    privateKey = findLabeledValue(string(out), "Master Private Key:")
    if !ValidPrivateKey(privateKey) || !ValidPrivateKey(publicKey) {
        return "", "", fmt.Errorf("Sudoku keygen returned invalid master keys")
    }
    return publicKey, privateKey, nil
}

func GenerateSplitKey(ctx context.Context, binary, masterPrivate string) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    out, err := exec.CommandContext(ctx, binary, "-keygen", "-more", masterPrivate).CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("Sudoku split-key generation failed: %w: %s", err, strings.TrimSpace(string(out)))
    }
    key := findLabeledValue(string(out), "Split Private Key:")
    if !ValidPrivateKey(key) {
        return "", fmt.Errorf("Sudoku keygen returned invalid split key")
    }
    return key, nil
}

func findLabeledValue(output, label string) string {
    for _, line := range strings.Split(output, "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, label) {
            return strings.TrimSpace(strings.TrimPrefix(line, label))
        }
    }
    return ""
}
