package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/util/wirecodec"
)

func envelopeTestEngine(t *testing.T, onHandler func()) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ConfigEnvelopeMiddleware())
	engine.POST("/echo", func(c *gin.Context) {
		if onHandler != nil {
			onHandler()
		}
		b, _ := io.ReadAll(c.Request.Body)
		c.String(http.StatusOK, "%s", string(b))
	})
	return engine
}

func TestConfigEnvelope_DecompressesAndVerifies(t *testing.T) {
	engine := envelopeTestEngine(t, nil)

	orig := []byte(strings.Repeat("payload-", 200))
	packed := wirecodec.Compress(orig)
	req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(packed))
	req.Header.Set("Content-Encoding", wirecodec.EncodingZstd)
	req.Header.Set(wirecodec.HashHeader, wirecodec.Sha256Hex(orig))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != string(orig) {
		t.Fatal("handler did not receive the decompressed body")
	}
	if !strings.Contains(w.Header().Get(wirecodec.CapsHeader), wirecodec.CapZstd) {
		t.Fatal("response must advertise the zstd capability")
	}
}

func TestConfigEnvelope_RejectsHashMismatch(t *testing.T) {
	called := false
	engine := envelopeTestEngine(t, func() { called = true })

	body := []byte("the-real-config")
	req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(body))
	// Header claims a hash of a *different* body — a tampered/corrupted push.
	req.Header.Set(wirecodec.HashHeader, wirecodec.Sha256Hex([]byte("a-different-config")))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("a hash mismatch must be rejected 4xx, got %d", w.Code)
	}
	if called {
		t.Fatal("the handler must NOT be invoked on a hash mismatch")
	}
}

func TestConfigEnvelope_PlainPassesThrough(t *testing.T) {
	engine := envelopeTestEngine(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("plain=body"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "plain=body" {
		t.Fatalf("a request with no envelope headers must pass through unchanged: code=%d body=%q", w.Code, w.Body.String())
	}
}

type envelopeCountingReader struct {
	read      int
	remaining int
}

func (r *envelopeCountingReader) Read(buf []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(buf), r.remaining)
	for i := range buf[:n] {
		buf[i] = 'x'
	}
	r.read += n
	r.remaining -= n
	return n, nil
}

func TestConfigEnvelope_LimitsRawBodiesBeforeDecompression(t *testing.T) {
	cases := []struct {
		name     string
		encoding string
		hash     string
		limit    int
	}{
		{name: "plain-with-hash", hash: "test-sha256", limit: maxDecodedConfigBytes},
		{name: "zstd-before-decode", encoding: wirecodec.EncodingZstd, limit: maxEncodedConfigBytes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			engine := envelopeTestEngine(t, func() { called = true })
			source := &envelopeCountingReader{remaining: tc.limit + 1024}
			req := httptest.NewRequest(http.MethodPost, "/echo", io.NopCloser(source))
			req.Header.Set("Content-Encoding", tc.encoding)
			req.Header.Set(wirecodec.HashHeader, tc.hash)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("oversized envelope returned %d; want 413", recorder.Code)
			}
			if called {
				t.Fatal("oversized envelope reached the request handler")
			}
			if source.read != tc.limit+1 {
				t.Fatalf("envelope read %d bytes; want exactly %d", source.read, tc.limit+1)
			}
		})
	}
}
