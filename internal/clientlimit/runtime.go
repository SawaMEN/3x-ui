package clientlimit

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

const (
	ListenPort     = 15879
	BridgePort     = 15880
	InternalPrefix = "__panel_speed_"
	burst          = 65536
)

type Route struct {
	ID, Email                string
	UploadMbps, DownloadMbps int
	Aliases                  []string
}

type Plan struct {
	Routes []Route
}

type (
	budget struct{ up, down *rate.Limiter }
	entry  struct {
		route  Route
		budget *budget
	}
)

// Runtime only listens on loopback and requires an unguessable process-local password.
type Runtime struct {
	mu             sync.Mutex
	password       string
	brokerPath     string
	brokerRoot     func() string
	brokerJSON     []byte
	listen, bridge string
	ln             net.Listener
	entries        map[string]entry
	conns          map[net.Conn]context.CancelFunc
}

func New(listen, bridge string) *Runtime {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(err)
	}
	return &Runtime{password: hex.EncodeToString(secret), listen: listen, bridge: bridge, entries: map[string]entry{}, conns: map[net.Conn]context.CancelFunc{}}
}

var Default = func() *Runtime {
	s := New(fmt.Sprintf("127.0.0.1:%d", ListenPort), fmt.Sprintf("127.0.0.1:%d", BridgePort))
	s.brokerRoot = config.GetBinFolderPath
	return s
}()

func (s *Runtime) Password() string { return s.password }

func limiter(mbps int) *rate.Limiter {
	if mbps == 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(float64(mbps)*125000), burst)
}

// Apply is staged before core startup; finish(false) restores the previous plan.
func (s *Runtime) Apply(plan Plan) (finish func(bool), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	opened := false
	if len(plan.Routes) > 0 && s.ln == nil {
		ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", s.listen)
		if err != nil {
			return nil, fmt.Errorf("client speed limiter: %w", err)
		}
		s.ln = ln
		opened = true
		go s.accept(ln)
	}
	old := s.entries
	budgets := map[string]*budget{}
	for _, e := range old {
		budgets[e.route.Email] = e.budget
	}
	next := make(map[string]entry, len(plan.Routes))
	for _, r := range plan.Routes {
		b := budgets[r.Email]
		if b == nil || !sameRate(b.up, r.UploadMbps) || !sameRate(b.down, r.DownloadMbps) {
			b = &budget{limiter(r.UploadMbps), limiter(r.DownloadMbps)}
			budgets[r.Email] = b
		}
		next[r.ID] = entry{r, b}
	}
	path, data, err := s.stageBroker(next)
	if err != nil {
		if opened {
			s.closeConnections()
			_ = s.ln.Close()
			s.ln = nil
		}
		return nil, err
	}
	oldPath, oldData := s.brokerPath, s.brokerJSON
	s.brokerPath, s.brokerJSON = path, data
	s.closeConnections()
	s.entries = next
	return func(commit bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !commit {
			s.closeConnections()
			s.entries = old
			s.brokerPath, s.brokerJSON = oldPath, oldData
			if oldPath != "" {
				if err := writeBroker(oldPath, oldData); err != nil {
					_ = os.Remove(oldPath)
				}
			} else if path != "" {
				_ = os.Remove(path)
				_ = os.Remove(filepath.Dir(path))
			}
		}
		if len(s.entries) == 0 && s.ln != nil {
			_ = s.ln.Close()
			s.ln = nil
		}
	}, nil
}

func sameRate(l *rate.Limiter, mbps int) bool {
	return l == nil && mbps == 0 || l != nil && l.Limit() == rate.Limit(float64(mbps)*125000)
}

func (s *Runtime) closeConnections() {
	for c, cancel := range s.conns {
		cancel()
		_ = c.Close()
	}
}

func (s *Runtime) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeConnections()
	if s.ln != nil {
		_ = s.ln.Close()
		s.ln = nil
	}
	s.entries = map[string]entry{}
	if s.brokerPath != "" {
		_ = os.Remove(s.brokerPath)
		_ = os.Remove(filepath.Dir(s.brokerPath))
	}
	s.brokerPath, s.brokerJSON = "", nil
}

func (s *Runtime) accept(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		if s.ln != ln || len(s.conns) >= 4096 {
			s.mu.Unlock()
			cancel()
			_ = c.Close()
			continue
		}
		s.conns[c] = cancel
		s.mu.Unlock()
		go func() {
			defer func() { cancel(); _ = c.Close(); s.mu.Lock(); delete(s.conns, c); s.mu.Unlock() }()
			s.serve(ctx, c)
		}()
	}
}

func readString(r io.Reader) ([]byte, error) {
	var n [1]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	b := make([]byte, int(n[0]))
	_, err := io.ReadFull(r, b)
	return b, err
}

func readAddress(r io.Reader, kind byte) ([]byte, error) {
	n := 0
	switch kind {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		name, err := readString(r)
		if err != nil {
			return nil, err
		}
		port := make([]byte, 2)
		_, err = io.ReadFull(r, port)
		return append(append([]byte{byte(len(name))}, name...), port...), err
	default:
		return nil, fmt.Errorf("invalid SOCKS address type %d", kind)
	}
	b := make([]byte, n+2)
	_, err := io.ReadFull(r, b)
	return b, err
}

func (s *Runtime) serve(ctx context.Context, c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	var hdr [2]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil || hdr[0] != 5 {
		return
	}
	methods := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	ok := false
	for _, m := range methods {
		if m == 2 {
			ok = true
		}
	}
	if !ok {
		_, _ = c.Write([]byte{5, 255})
		return
	}
	if _, err := c.Write([]byte{5, 2}); err != nil {
		return
	}
	var ver [1]byte
	if _, err := io.ReadFull(c, ver[:]); err != nil || ver[0] != 1 {
		return
	}
	user, err := readString(c)
	if err != nil {
		return
	}
	pass, err := readString(c)
	if err != nil {
		return
	}
	s.mu.Lock()
	e, exists := s.entries[string(user)]
	s.mu.Unlock()
	if !exists || subtle.ConstantTimeCompare(pass, []byte(s.password)) != 1 {
		_, _ = c.Write([]byte{1, 1})
		return
	}
	if _, err = c.Write([]byte{1, 0}); err != nil {
		return
	}
	var req [4]byte
	if _, err = io.ReadFull(c, req[:]); err != nil || req[0] != 5 || req[2] != 0 {
		return
	}
	addr, err := readAddress(c, req[3])
	if err != nil {
		return
	}
	if req[1] == 0xf0 {
		s.serveBudget(ctx, c, e.budget)
		return
	}
	if req[1] != 1 && req[1] != 3 {
		_, _ = c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	upstream, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", s.bridge)
	if err != nil {
		_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	stop := context.AfterFunc(ctx, func() { _ = upstream.Close() })
	defer stop()
	_ = upstream.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = upstream.Write([]byte{5, 1, 2}); err != nil {
		return
	}
	if _, err = io.ReadFull(upstream, hdr[:]); err != nil || hdr != [2]byte{5, 2} {
		return
	}
	auth := append([]byte{1, byte(len(e.route.ID))}, e.route.ID...)
	auth = append(auth, byte(len(s.password)))
	auth = append(auth, s.password...)
	if _, err = upstream.Write(auth); err != nil {
		return
	}
	if _, err = io.ReadFull(upstream, hdr[:]); err != nil || hdr != [2]byte{1, 0} {
		return
	}
	if _, err = upstream.Write(append(req[:], addr...)); err != nil {
		return
	}
	var reply [4]byte
	if _, err = io.ReadFull(upstream, reply[:]); err != nil || reply[0] != 5 {
		return
	}
	bind, err := readAddress(upstream, reply[3])
	if err != nil {
		return
	}
	if reply[1] != 0 {
		_, _ = c.Write(append(reply[:], bind...))
		return
	}
	_ = c.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	if req[1] == 3 {
		s.relayUDP(ctx, c, upstream, reply[3], bind, e.budget)
		return
	}
	if _, err = c.Write(append(reply[:], bind...)); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopRelay := context.AfterFunc(ctx, func() { _ = c.Close(); _ = upstream.Close() })
	defer stopRelay()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := copyLimited(ctx, upstream, c, e.budget.up); err != nil {
			cancel()
		}
		closeWrite(upstream)
	}()
	_ = copyLimited(ctx, c, upstream, e.budget.down)
	closeWrite(c)
	cancel()
	_ = c.Close()
	_ = upstream.Close()
	<-done
}

func closeWrite(c net.Conn) {
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}

func waitBudget(ctx context.Context, l *rate.Limiter, n int) error {
	if l == nil {
		return nil
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return l.WaitN(wait, n)
}

func copyLimited(ctx context.Context, dst io.Writer, src io.Reader, l *rate.Limiter) error {
	if l == nil {
		_, err := io.Copy(dst, src)
		return err
	}
	b := make([]byte, 32768)
	for {
		n, err := src.Read(b)
		if n > 0 {
			if e := waitBudget(ctx, l, n); e != nil {
				return e
			}
			if _, e := dst.Write(b[:n]); e != nil {
				return e
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *Runtime) relayUDP(ctx context.Context, c, upstream net.Conn, kind byte, bind []byte, b *budget) {
	if kind != 1 || len(bind) != 6 || (!net.IP(bind[:4]).IsLoopback() && !net.IP(bind[:4]).IsUnspecified()) {
		return
	}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(bind[4])<<8 | int(bind[5])}
	u, err := net.DialUDP("udp4", nil, remote)
	if err != nil {
		return
	}
	defer u.Close()
	local, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return
	}
	defer local.Close()
	port := local.LocalAddr().(*net.UDPAddr).Port
	if _, err = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(port >> 8), byte(port)}); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = local.Close(); _ = u.Close(); _ = upstream.Close(); _ = c.Close() })
	defer stop()
	var peerMu sync.Mutex
	var peer *net.UDPAddr
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); defer cancel(); _, _ = io.Copy(io.Discard, c) }()
	go func() { defer wg.Done(); defer cancel(); _, _ = io.Copy(io.Discard, upstream) }()
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 65536)
		for {
			n, err := u.Read(buf)
			if err != nil {
				return
			}
			if !validDatagram(buf[:n]) {
				continue
			}
			if waitBudget(ctx, b.down, n) != nil {
				return
			}
			peerMu.Lock()
			p := peer
			peerMu.Unlock()
			if p != nil {
				if _, err = local.WriteToUDP(buf[:n], p); err != nil {
					return
				}
			}
		}
	}()
	buf := make([]byte, 65536)
	for {
		n, p, err := local.ReadFromUDP(buf)
		if err != nil {
			break
		}
		if !p.IP.IsLoopback() || !validDatagram(buf[:n]) {
			continue
		}
		peerMu.Lock()
		if peer == nil {
			peer = p
		}
		match := peer.String() == p.String()
		peerMu.Unlock()
		if !match {
			continue
		}
		if waitBudget(ctx, b.up, n) != nil {
			break
		}
		if _, err = u.Write(buf[:n]); err != nil {
			break
		}
	}
	cancel()
	_ = local.Close()
	_ = u.Close()
	_ = c.Close()
	_ = upstream.Close()
	wg.Wait()
}

func validDatagram(b []byte) bool {
	if len(b) < 7 || b[0] != 0 || b[1] != 0 || b[2] != 0 {
		return false
	}
	switch b[3] {
	case 1:
		return len(b) >= 10
	case 4:
		return len(b) >= 22
	case 3:
		return len(b) >= int(b[4])+7 && b[4] > 0
	default:
		return false
	}
}

func (s *Runtime) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}
