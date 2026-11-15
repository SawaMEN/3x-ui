package runtime

import (
	"fmt"
	goruntime "runtime"
	"strings"
)

// Untrusted remote response bodies are useful to callers but must not be
// copied to persistent diagnostics, even through a wrapped error.
func (e *remoteHTTPError) DiagnosticSummary() string {
	return fmt.Sprintf("remote HTTP status %d", e.status)
}

func (e *remoteAPIError) DiagnosticSummary() string { return "remote API rejected operation" }

// Report a code operation rather than a URL: node base paths, client IDs,
// subscription tokens and DNS query values are not safe diagnostic fields.
func remoteDiagnosticOperation() string {
	var pcs [16]uintptr
	count := goruntime.Callers(2, pcs[:])
	frames := goruntime.CallersFrames(pcs[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "(*Remote).") {
			name := frame.Function[strings.LastIndex(frame.Function, ".")+1:]
			if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
				return name
			}
		}
		if !more {
			return "node_request"
		}
	}
}
