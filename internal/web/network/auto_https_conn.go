// Package network provides network utilities for the 3x-ui web panel,
// including automatic HTTP to HTTPS redirection functionality.
package network

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
)

const maxRedirectRequestBytes = 2048

// AutoHttpsConn intercepts an initial HTTP request and redirects it to HTTPS.
// TLS traffic is passed through to the server.
type AutoHttpsConn struct {
	net.Conn

	readMu sync.Mutex
	reader io.Reader
}

func NewAutoHttpsConn(conn net.Conn) net.Conn {
	return &AutoHttpsConn{Conn: conn}
}

func (c *AutoHttpsConn) initialReader() io.Reader {
	reader := bufio.NewReaderSize(c.Conn, maxRedirectRequestBytes)
	first, err := reader.Peek(1)
	if err != nil || first[0] < '!' || first[0] >= 0x7f {
		// TLS record types start with a control byte.
		return reader
	}

	// A plaintext request can span multiple reads. Bound the amount inspected
	// and replay what was read if it turns out not to be a redirectable request.
	var captured bytes.Buffer
	request, err := http.ReadRequest(bufio.NewReader(io.TeeReader(io.LimitReader(reader, maxRedirectRequestBytes), &captured)))
	if err != nil || request.Host == "" || request.URL.Path == "*" {
		return io.MultiReader(&captured, reader)
	}
	location := url.URL{
		Scheme:     "https",
		Host:       request.Host,
		Path:       request.URL.Path,
		RawPath:    request.URL.RawPath,
		RawQuery:   request.URL.RawQuery,
		ForceQuery: request.URL.ForceQuery,
	}
	response := http.Response{
		StatusCode:    http.StatusTemporaryRedirect,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Location": {location.String()}},
		ContentLength: 0,
		Close:         true,
	}
	_ = response.Write(c.Conn)
	_ = c.Close()
	return bytes.NewReader(nil)
}

// Read redirects a plaintext HTTP request or reads from the TLS connection.
func (c *AutoHttpsConn) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.reader == nil {
		c.reader = c.initialReader()
	}
	return c.reader.Read(buf)
}
