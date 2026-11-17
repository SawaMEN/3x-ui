package clientlimit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit/brokerclient"
)

const (
	BrokerEnvironment = brokerclient.Environment
	BrokerVersion     = brokerclient.Version
)

func (s *Runtime) Environment(parent []string) []string {
	env := make([]string, 0, len(parent)+1)
	for _, value := range parent {
		key, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(key, BrokerEnvironment) {
			env = append(env, value)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.brokerPath != "" {
		env = append(env, BrokerEnvironment+"="+s.brokerPath)
	}

	return env
}

func CheckBrokerBinary(ctx context.Context, binary string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, binary, "--xui-speed-broker-version").Output()
	if err != nil || strings.TrimSpace(string(data)) != BrokerVersion {
		return fmt.Errorf("%s does not support per-client speed limiting; install the panel's speed-enabled component build", binary)
	}
	return nil
}

func (s *Runtime) serveBudget(ctx context.Context, c net.Conn, b *budget) {
	if _, err := c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	frames := make(chan [5]byte)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer cancel()
		for {
			var frame [5]byte
			if _, err := io.ReadFull(c, frame[:]); err != nil {
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-frames:
			n := binary.BigEndian.Uint32(frame[1:])
			if frame[0] > 1 || n == 0 || n > burst {
				return
			}
			limiter := b.up
			if frame[0] == 1 {
				limiter = b.down
			}
			if err := waitBudget(ctx, limiter, int(n)); err != nil {
				return
			}
			if _, err := c.Write([]byte{0}); err != nil {
				return
			}
		}
	}
}

func (s *Runtime) Revision() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := sha256.Sum256(s.brokerJSON)
	return hex.EncodeToString(digest[:])
}

func (s *Runtime) stageBroker(entries map[string]entry) (string, []byte, error) {
	users := map[string]string{}
	for id, e := range entries {
		for _, alias := range e.route.Aliases {
			if old := users[alias]; old == "" || id < old {
				users[alias] = id
			}
		}
	}
	if len(users) == 0 && s.brokerPath == "" {
		return "", nil, nil
	}
	address := s.listen
	if s.ln != nil {
		address = s.ln.Addr().String()
	}
	data, err := json.Marshal(brokerclient.Config{Address: address, Password: s.password, Users: users})
	if err != nil {
		return "", nil, err
	}
	if bytes.Equal(data, s.brokerJSON) {
		if info, err := os.Stat(s.brokerPath); err == nil && info.Mode().IsRegular() {
			return s.brokerPath, data, nil
		}
	}
	path := s.brokerPath
	if path == "" {
		root := ""
		if s.brokerRoot != nil {
			root, err = filepath.Abs(s.brokerRoot())
			if err != nil {
				return "", nil, err
			}
			if err = os.MkdirAll(root, 0o750); err != nil {
				return "", nil, err
			}
		}
		dir, err := os.MkdirTemp(root, ".client-speed-*")
		if err != nil {
			return "", nil, err
		}
		path = filepath.Join(dir, "credentials.json")
	}
	if err := writeBroker(path, data); err != nil {
		if s.brokerPath == "" {
			_ = os.Remove(filepath.Dir(path))
		}
		return "", nil, err
	}
	return path, data, nil
}

func writeBroker(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".broker-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (s *Runtime) Requires(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		for _, alias := range e.route.Aliases {
			if strings.HasPrefix(alias, prefix) {
				return true
			}
		}
	}
	return false
}

func (s *Runtime) HasAlias(alias string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		for _, value := range e.route.Aliases {
			if value == alias {
				return true
			}
		}
	}
	return false
}
