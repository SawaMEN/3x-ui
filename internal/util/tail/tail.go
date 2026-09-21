package tail

import (
	"bytes"
	"io"
	"os"
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

	lines := make([]string, 0, min(maxLines, 256))
	var carry []byte
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
		if _, err := file.ReadAt(chunk, start); err != nil && err != io.EOF {
			return nil, err
		}

		buf := make([]byte, len(chunk)+len(carry))
		copy(buf, chunk)
		copy(buf[len(chunk):], carry)

		prefixEnd := len(buf)
		for prefixEnd > 0 && (maxLines <= 0 || len(lines) < maxLines) {
			newline := bytes.LastIndexByte(buf[:prefixEnd], '\n')
			if newline < 0 {
				break
			}
			line := bytes.TrimSuffix(buf[newline+1:prefixEnd], []byte{'\r'})
			if len(line) > 0 {
				lines = append(lines, string(line))
			}
			prefixEnd = newline
		}

		carry = append(carry[:0], buf[:prefixEnd]...)
		end = start
		remaining -= readSize
	}

	if len(carry) > 0 && (maxLines <= 0 || len(lines) < maxLines) {
		line := bytes.TrimSuffix(carry, []byte{'\r'})
		lines = append(lines, string(line))
	}

	return lines, nil
}
