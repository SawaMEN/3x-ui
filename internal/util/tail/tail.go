package tail

import (
	"bytes"
	"io"
	"os"
	"strings"
)

const (
	TailChunkSize    int64 = 64 << 10
	DefaultTailBytes int64 = 8 << 20
)

// ReadTailLines returns up to maxLines newest lines in newest-first order.
// It reads from the end of the file in bounded chunks instead of buffering the
// whole file, so callers can safely inspect large log files on small VDS hosts.
// At most maxBytes of file content is read.
func ReadTailLines(path string, maxLines int, maxBytes int64) ([]string, error) {
	if maxBytes <= 0 {
		return []string{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return []string{}, nil
	}

	remaining := maxBytes
	if remaining > info.Size() {
		remaining = info.Size()
	}

	// Non-positive maxLines means unlimited. A negative value must not
	// become a negative slice capacity (which would panic).
	capacity := 256
	if maxLines > 0 && maxLines < capacity {
		capacity = maxLines
	}
	lines := make([]string, 0, capacity)
	// Retain fragments newest first until a complete line is found. Joining
	// the growing carry on every chunk makes a single long record quadratic.
	var carry [][]byte
	carryBytes := 0
	lineText := func(prefix []byte) string {
		if len(carry) == 0 {
			return string(bytes.TrimSuffix(prefix, []byte{'\r'}))
		}
		var text strings.Builder
		text.Grow(len(prefix) + carryBytes)
		text.Write(prefix)
		for i := len(carry) - 1; i >= 0; i-- {
			text.Write(carry[i])
		}
		return strings.TrimSuffix(text.String(), "\r")
	}
	end := info.Size()

	for end > 0 && remaining > 0 && (maxLines <= 0 || len(lines) < maxLines) {
		readSize := TailChunkSize
		if readSize > remaining {
			readSize = remaining
		}
		if readSize > end {
			readSize = end
		}

		start := end - readSize
		chunk := make([]byte, int(readSize))
		if err := readTailChunk(file, chunk, start); err != nil {
			return nil, err
		}

		prefixEnd := len(chunk)
		for prefixEnd > 0 && (maxLines <= 0 || len(lines) < maxLines) {
			newline := bytes.LastIndexByte(chunk[:prefixEnd], '\n')
			if newline < 0 {
				break
			}
			line := lineText(chunk[newline+1 : prefixEnd])
			clear(carry)
			carry = carry[:0]
			carryBytes = 0
			if len(line) > 0 {
				lines = append(lines, line)
			}
			prefixEnd = newline
		}

		if prefixEnd > 0 {
			carry = append(carry, chunk[:prefixEnd])
			carryBytes += prefixEnd
		}
		end = start
		remaining -= readSize
	}

	// When the byte budget ends in the middle of a line, carry contains
	// only a truncated prefix. Return complete records only: otherwise the
	// dashboard can present an incomplete error or panic as a real log entry.
	// A line without a trailing newline is still valid when we reached the
	// beginning of the file.
	if end == 0 && carryBytes > 0 && (maxLines <= 0 || len(lines) < maxLines) {
		lines = append(lines, lineText(nil))
	}

	return lines, nil
}

// readTailChunk must never accept a short read as a complete log chunk.
// A rotating or concurrently truncated file can shrink after Stat; treating
// io.EOF as success would append zero-filled bytes as counterfeit log content.
func readTailChunk(reader io.ReaderAt, chunk []byte, offset int64) error {
	n, err := reader.ReadAt(chunk, offset)
	if n != len(chunk) {
		if err != nil && err != io.EOF {
			return err
		}
		return io.ErrUnexpectedEOF
	}
	return err
}

// RecentOutputBuffer retains only the last maxBytes of a command's stdout.
// Unlike bytes.Buffer it cannot grow unbounded when a journal entry is huge.
// It implements io.Writer so exec.Cmd can stream output directly into it.
type RecentOutputBuffer struct {
	data      []byte
	maxBytes  int
	head      int // Oldest byte when the fixed-size ring is full.
	truncated bool
}

func NewRecentOutputBuffer(maxBytes int) *RecentOutputBuffer {
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &RecentOutputBuffer{maxBytes: maxBytes}
}

func (b *RecentOutputBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	if b.maxBytes == 0 {
		b.truncated = true
		return n, nil
	}
	if n >= b.maxBytes {
		if len(b.data) > 0 || n > b.maxBytes {
			b.truncated = true
		}
		b.data = append(b.data[:0], p[n-b.maxBytes:]...)
		b.head = 0
		return n, nil
	}
	if len(b.data) < b.maxBytes {
		space := b.maxBytes - len(b.data)
		if n <= space {
			b.data = append(b.data, p...)
			return n, nil
		}
		b.data = append(b.data, p[:space]...)
		p = p[space:]
	}
	// Overwrite the oldest bytes in-place instead of shifting the whole
	// 8 MiB window on every small write. Total work stays O(input bytes).
	b.truncated = true
	first := min(len(p), b.maxBytes-b.head)
	copy(b.data[b.head:], p[:first])
	copy(b.data, p[first:])
	b.head = (b.head + len(p)) % b.maxBytes
	return n, nil
}

// CompleteBytes returns chronologically ordered output without a possibly
// truncated oldest record. The circular buffer is linearized only once,
// after the command has completed.
func (b *RecentOutputBuffer) CompleteBytes() []byte {
	var ordered []byte
	if b.head == 0 {
		ordered = b.data
	} else {
		ordered = make([]byte, len(b.data))
		copied := copy(ordered, b.data[b.head:])
		copy(ordered[copied:], b.data[:b.head])
	}
	if !b.truncated {
		return ordered
	}
	if i := bytes.IndexByte(ordered, '\n'); i >= 0 {
		return ordered[i+1:]
	}
	return nil
}

// Truncated reports whether earlier command output was discarded.
func (b *RecentOutputBuffer) Truncated() bool { return b.truncated }
