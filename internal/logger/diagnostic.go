package logger

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	diagnosticPEM        = regexp.MustCompile(`(?s)-----BEGIN [^-\r\n]+-----.*?(?:-----END [^-\r\n]+-----|$)`)
	diagnosticJSONSecret = regexp.MustCompile(`(?i)("(?:password|passwd|token|apiToken|api_token|access_token|refresh_token|secret|privateKey(?:PEM)?|private_key(?:_pem)?|apiKey|api_key|dnsToken|authorization|cookie)"\s*:\s*)"(?:\\.|[^"\\])*"`)
	diagnosticHeader     = regexp.MustCompile(`(?im)\b(?:authorization|proxy-authorization|cookie|set-cookie)\s*:\s*[^\r\n]+`)
	diagnosticBearer     = regexp.MustCompile(`(?i)\bBearer[ \t]+[^\s,;"']+`)
	diagnosticSecret     = regexp.MustCompile(`(?i)\b(password|passwd|token|apiToken|api_token|access_token|refresh_token|secret|privateKey(?:PEM)?|private_key(?:_pem)?|apiKey|api_key|dnsToken)\b[ \t]*[:=][ \t]*(?:"(?:\\.|[^"\\])*"|'[^']*'|[^\s,;]+)`)
	diagnosticURL        = regexp.MustCompile(`https?://[^\s"'<>]+`)
	diagnosticPrivateURI = regexp.MustCompile(`(?i)\b(?:vless|vmess|trojan|ss|hysteria2?|hy2|tuic|wireguard|awg|amneziawg|vpn|tg|mtproto|naive(?:\+https|\+quic)?|anytls|shadowtls|mieru|snell|sudoku|fptn|trusttunnel|openflux|vetroff(?:-proxy)?|vk-turn-proxy|pingtunnel|masque(?:-client)?|socks5?|ssh|postgres(?:ql)?|mysql|mongodb(?:\+srv)?):\/\/[^\s"'<>]+`)
)

// SanitizeDiagnostic bounds single-line diagnostics and removes common secret
// formats. Request/response bodies and headers must still never be logged.
func SanitizeDiagnostic(value string) string {
	if len(value) > 64<<10 {
		value = value[:64<<10]
	}
	value = diagnosticPEM.ReplaceAllString(value, "[PEM redacted]")
	value = diagnosticJSONSecret.ReplaceAllString(value, `${1}"[redacted]"`)
	value = diagnosticHeader.ReplaceAllString(value, "[header redacted]")
	value = diagnosticBearer.ReplaceAllString(value, "Bearer [redacted]")
	value = diagnosticSecret.ReplaceAllString(value, `${1}=[redacted]`)
	value = diagnosticPrivateURI.ReplaceAllString(value, "[URI redacted]")
	value = diagnosticURL.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "[URL redacted]"
		}
		return parsed.Scheme + "://" + parsed.Host + "/[redacted]"
	})
	value = strings.Map(func(ch rune) rune {
		if ch < 32 || ch == 127 {
			return ' '
		}
		return ch
	}, value)
	if len(value) > 4096 {
		return strings.ToValidUTF8(value[:4096], "") + " [truncated]"
	}
	return value
}

// ErrorSummary lets untrusted transports supply a safe diagnostic without
// exposing their raw response body, URL, credentials or certificate material.
func ErrorSummary(err error) string { return errorSummary(err, 0) }

func errorSummary(err error, depth int) string {
	if depth >= 8 {
		return "additional nested errors truncated"
	}
	if err == nil {
		return ""
	}
	// Rollback failures often join multiple causes. Preserve each one rather
	// than allowing one HTTP error or cancellation to hide the others.
	for current, i := err, 0; current != nil && i < 8; current, i = errors.Unwrap(current), i+1 {
		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			causes := joined.Unwrap()
			parts := make([]string, 0, min(len(causes), 8))
			for _, cause := range causes[:min(len(causes), 8)] {
				if cause != nil {
					parts = append(parts, errorSummary(cause, depth+1))
				}
			}
			if len(causes) > 8 {
				parts = append(parts, "additional errors truncated")
			}
			return SanitizeDiagnostic(strings.Join(parts, "; "))
		}
	}
	var diagnostic interface{ DiagnosticSummary() string }
	if errors.As(err, &diagnostic) {
		return SanitizeDiagnostic(diagnostic.DiagnosticSummary())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "operation deadline exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "operation canceled"
	}
	for range 4 {
		var request *url.Error
		if !errors.As(err, &request) || request.Err == nil {
			break
		}
		err = request.Err
	}
	return SanitizeDiagnostic(err.Error())
}
