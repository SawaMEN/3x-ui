package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap/zapcore"
)

type diagnosticZapCore struct{ fields []zapcore.Field }

// NewZapCore bridges certificate-library warnings/errors into the same
// persistent file and UI buffer as panel failures. Only diagnostic fields are
// retained; arbitrary structured request/response objects are never encoded.
func NewZapCore() zapcore.Core                              { return &diagnosticZapCore{} }
func (*diagnosticZapCore) Enabled(level zapcore.Level) bool { return level >= zapcore.WarnLevel }
func (c *diagnosticZapCore) With(fields []zapcore.Field) zapcore.Core {
	all := append([]zapcore.Field{}, c.fields...)
	return &diagnosticZapCore{fields: append(all, fields...)}
}

func (c *diagnosticZapCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *diagnosticZapCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	var detail strings.Builder
	appendField := func(field zapcore.Field) {
		if field.Type == zapcore.ErrorType {
			if err, ok := field.Interface.(error); ok {
				fmt.Fprintf(&detail, " %s=%q", SanitizeDiagnostic(field.Key), ErrorSummary(err))
			}
			return
		}
		switch field.Key {
		case "error":
			if err, ok := field.Interface.(error); ok {
				fmt.Fprintf(&detail, " error=%q", ErrorSummary(err))
			} else if field.Type == zapcore.StringType {
				fmt.Fprintf(&detail, " error=%q", SanitizeDiagnostic(field.String))
			}
		case "identifier", "domain":
			if field.Type == zapcore.StringType {
				fmt.Fprintf(&detail, " domain=%q", SanitizeDiagnostic(field.String))
			}
		}
	}
	for _, field := range c.fields {
		appendField(field)
	}
	for _, field := range fields {
		appendField(field)
	}
	line := fmt.Sprintf("Certificate library component=%q message=%q%s", SanitizeDiagnostic(entry.LoggerName), SanitizeDiagnostic(entry.Message), detail.String())
	if entry.Level >= zapcore.ErrorLevel {
		Error(line)
	} else {
		Warning(line)
	}
	return nil
}
func (*diagnosticZapCore) Sync() error { return nil }
