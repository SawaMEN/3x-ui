// Package sidecartraffic transfers consumed runtime counters to the panel's WAL
// without calling quota/runtime maintenance while a manager holds its lock.
package sidecartraffic

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type Delta struct {
	ID   int   `json:"id"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}
type Batch struct {
	ID       string  `json:"id"`
	Inbounds []Delta `json:"inbounds"`
	Clients  []Delta `json:"clients"`
}
type Sample struct {
	Email    string
	Up, Down int64
	Active   bool
}

// Owner pins database identities and the WAL directory before a runtime starts.
// The manager serializes its use; no database lookups occur during handoff.
type Owner struct {
	source, dir string
	inboundID   int
	clients     map[string]int
	routed      bool
}

func Capture(source string, id int, emails []string, routed bool) (*Owner, error) {
	if source != "mieru" && source != "mtproto" && source != "externalvpn" {
		return nil, errors.New("unsupported traffic source")
	}
	db := database.GetDB()
	// Managers can also run standalone, outside a panel database.
	if db == nil {
		return nil, nil
	}
	owner := &Owner{source: source, dir: filepath.Join(config.GetDBFolderPath(), source+"-traffic-journal"), inboundID: id, clients: map[string]int{}, routed: routed}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inbound model.Inbound
		err := tx.Select("id").Where("id = ? AND node_id IS NULL", id).Take(&inbound).Error
		if err != nil {
			return err
		}
		for start := 0; start < len(emails); start += 900 {
			var rows []coremodel.ClientTraffic
			end := min(start+900, len(emails))
			if err := tx.Select("id", "email").Where("email IN ?", emails[start:end]).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				owner.clients[row.Email] = row.Id
			}
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("capture sidecar traffic identities failed")
	}
	return owner, nil
}

type pendingWrite struct {
	dir, id string
	data    []byte
}

var pending sync.Map // file path -> immutable pendingWrite, owned by the process

// FlushPending retries failed file writes even after the original manager or
// panel job is gone. It does not touch database accounting or runtime state.
func FlushPending(dir string) error {
	var result error
	pending.Range(func(key, value any) bool {
		item := value.(pendingWrite)
		if item.dir != dir {
			return true
		}
		if _, err := StoreFile(item.dir, item.id, item.data); err != nil {
			result = errors.Join(result, err)
		} else {
			pending.Delete(key)
		}
		return true
	})
	return result
}

// Store accepts ownership into a process-wide retry queue before touching the
// filesystem. Ordinary CRUD can stop disabled users even during a disk outage;
// queued bytes survive a panel replacement, but not a crash before fsync.
func (o *Owner) Store(samples []Sample) (accepted bool, err error) {
	if o == nil {
		return true, nil
	}
	batch := Batch{ID: o.source + "-" + uuid.NewString()}
	up, down := int64(0), int64(0)
	for _, sample := range samples {
		u, d := max(sample.Up, 0), max(sample.Down, 0)
		if u == 0 && d == 0 && !sample.Active {
			continue
		}
		if id := o.clients[sample.Email]; id > 0 {
			batch.Clients = append(batch.Clients, Delta{ID: id, Up: u, Down: d})
		}
		up = saturate(up, u)
		down = saturate(down, d)
	}
	if !o.routed && (up > 0 || down > 0) {
		batch.Inbounds = []Delta{{ID: o.inboundID, Up: up, Down: down}}
	}
	if len(batch.Inbounds) == 0 && len(batch.Clients) == 0 {
		return true, nil
	}
	data, err := json.Marshal(batch)
	if err != nil {
		return false, err
	}
	key := filepath.Join(o.dir, batch.ID+".json")
	pending.Store(key, pendingWrite{dir: o.dir, id: batch.ID, data: data})
	_, err = StoreFile(o.dir, batch.ID, data)
	if err == nil {
		pending.Delete(key)
	}
	return true, err
}

func saturate(a, b int64) int64 {
	if b >= database.TrafficMax-a {
		return database.TrafficMax
	}
	return a + b
}

func SyncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func StoreFile(dir, id string, data []byte) (string, error) {
	if id == "" || filepath.Base(id) != id || id == "." || id == ".." {
		return "", errors.New("invalid traffic journal ID")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := SyncDir(filepath.Dir(dir)); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".pending-")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, SyncDir(dir)
}
