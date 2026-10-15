package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/outbound"
)

type subscriptionCheckResult struct {
	message string
	expires time.Time
}

var subscriptionCheckCache = struct {
	sync.Mutex
	entries map[string]subscriptionCheckResult
}{entries: map[string]subscriptionCheckResult{}}

// Check options against the installed kernel before a subscription member can
// enter production. Direct stubs resolve tag references without starting any
// tunnel or listener; dependency availability is checked separately afterwards.
func validateSubscriptionNativeMember(row map[string]any, core string) error {
	binary, err := filepath.Abs(singbox.GetBinaryPath())
	if err != nil {
		return err
	}
	info, err := os.Stat(binary)
	if err != nil {
		return nil
	} // Offline templates still use the static validators.
	cfg := singbox.NewConfig()
	cfg.Log = map[string]any{"level": "error", "timestamp": false}
	tag, _ := row["tag"].(string)
	direct := "__subscription_check_direct"
	for direct == tag {
		direct += "_"
	}
	cfg.Outbounds = []map[string]any{{"type": "direct", "tag": direct}}
	cfg.Route = map[string]any{"final": direct, "default_domain_resolver": "local"}
	if endpoint, handled, e := singbox.TranslatePanelEndpoint(row); handled {
		if e != nil {
			return e
		}
		cfg.Endpoints = []map[string]any{endpoint}
	} else {
		translated, e := NativeCore(core).translateOutbound(row)
		if e != nil {
			return e
		}
		cfg.Outbounds = append(cfg.Outbounds, translated)
	}
	seen := map[string]bool{direct: true, tag: true}
	for _, dependency := range outbound.OutboundDependencies(row) {
		if dependency == tag {
			return fmt.Errorf("подключение ссылается на себя")
		}
		if !seen[dependency] {
			cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "direct", "tag": dependency})
			seen[dependency] = true
		}
	}
	data, err := cfg.Marshal()
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%d:%d:%x", binary, info.Size(), info.ModTime().UnixNano(), sha256.Sum256(data))
	subscriptionCheckCache.Lock()
	cached, found := subscriptionCheckCache.entries[key]
	subscriptionCheckCache.Unlock()
	if found && time.Now().Before(cached.expires) {
		if cached.message != "" {
			return fmt.Errorf("%s", cached.message)
		}
		return nil
	}
	file, err := os.CreateTemp("", "x-ui-subscription-check-*.json")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "check", "-c", path)
	command.Dir = filepath.Dir(binary)
	output, checkErr := command.CombinedOutput()
	message := ""
	// Hiddify may represent an invalid profile as a blocked placeholder and
	// still exit successfully. Error-level diagnostics are also a rejection.
	diagnostics := strings.ToLower(string(output))
	invalidPlaceholder := strings.Contains(diagnostics, "error") || strings.Contains(diagnostics, "unknown outbound") || strings.Contains(diagnostics, "unknown field") || strings.Contains(diagnostics, "unsupported")
	if checkErr != nil || invalidPlaceholder {
		detail := strings.TrimSpace(string(output))
		// Kernel diagnostics must never expose imported authentication material.
		var redact func(any)
		redact = func(value any) {
			switch v := value.(type) {
			case map[string]any:
				for field, child := range v {
					lower := strings.ToLower(field)
					if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "key") || field == "psk" || field == "auth_str" {
						if text, ok := child.(string); ok && len(text) > 3 {
							detail = strings.ReplaceAll(detail, text, "[скрыто]")
						}
					}
					redact(child)
				}
			case []any:
				for _, child := range v {
					redact(child)
				}
			}
		}
		redact(row)
		detail = strings.TrimSpace(strings.SplitN(detail, "\n", 2)[0])
		if runes := []rune(detail); len(runes) > 400 {
			detail = string(runes[:400]) + "…"
		}
		if ctx.Err() != nil {
			detail = "проверка ядром превысила время ожидания"
		}
		if detail == "" {
			detail = "параметры отклонены ядром"
		}
		message = "Протокол или его настройки не поддерживаются установленным ядром: " + detail
	}
	subscriptionCheckCache.Lock()
	if len(subscriptionCheckCache.entries) >= 8192 {
		for old, result := range subscriptionCheckCache.entries {
			if time.Now().After(result.expires) {
				delete(subscriptionCheckCache.entries, old)
			}
		}
		if len(subscriptionCheckCache.entries) >= 8192 {
			for old := range subscriptionCheckCache.entries {
				delete(subscriptionCheckCache.entries, old)
				break
			}
		}
	}
	ttl := time.Hour
	if message != "" {
		ttl = 5 * time.Minute
	}
	subscriptionCheckCache.entries[key] = subscriptionCheckResult{message: message, expires: time.Now().Add(ttl)}
	subscriptionCheckCache.Unlock()
	if message != "" {
		return fmt.Errorf("%s", message)
	}
	return nil
}
