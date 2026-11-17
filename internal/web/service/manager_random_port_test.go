package service

import (
	"context"
	"crypto/rand"
	"net"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type chosenManagerPortReader struct{ offset int }

func (r chosenManagerPortReader) Read(out []byte) (int, error) {
	clear(out)
	if len(out) >= 2 {
		out[len(out)-2] = byte(r.offset >> 8)
		out[len(out)-1] = byte(r.offset)
	}
	return len(out), nil
}

func TestManagerRejectsRandomPortAlreadyOwnedBySystem(t *testing.T) {
	s, _, sub := managerFixture(t)
	var listener net.Listener
	var port int
	for attempt := 0; attempt < 32; attempt++ {
		candidate, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port = candidate.Addr().(*net.TCPAddr).Port
		if port >= 10000 && port < 60000 {
			listener = candidate
			break
		}
		_ = candidate.Close()
	}
	if listener == nil {
		t.Skip("no ephemeral listener within the manager random port range")
	}
	defer listener.Close()

	previous := rand.Reader
	rand.Reader = chosenManagerPortReader{offset: port - 10000}
	defer func() { rand.Reader = previous }()

	op, err := s.Enqueue(1, sub.ID, "rotate")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.clone(context.Background(), op, sub, &sub.Entries[0], 0)
	if err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("manager accepted an occupied random port %d: %v", port, err)
	}
	var created int64
	if err := database.GetDB().Model(&model.Inbound{}).
		Where("user_id = ? AND tag LIKE ?", sub.UserID, "manager-"+sub.Token+"-%").
		Count(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("manager persisted %d inbounds with an occupied port", created)
	}
}
