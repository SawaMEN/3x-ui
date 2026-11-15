package logger

import (
	"fmt"
	"io"
)

// A broken log file must not silently discard the original diagnostic. The
// fallback is stderr (and therefore the service journal), never this logger.
type diagnosticFileWriter struct {
	target   io.Writer
	fallback io.Writer
}

func (w diagnosticFileWriter) Write(record []byte) (int, error) {
	n, err := w.target.Write(record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err != nil {
		diagnostic := "panel log file write failed: " + ErrorSummary(err)
		addToBuffer("ERROR", diagnostic)
		_, _ = fmt.Fprintln(w.fallback, diagnostic)
		_, _ = w.fallback.Write(record)
	}
	return n, err
}
