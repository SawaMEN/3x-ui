package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// Each runtime has process-wide cursors: serialize retired and replacement panel
// jobs as well as journal replay. A new job recovers the old job's durable batch.
var mieruTrafficMu, mtprotoTrafficMu, externalTrafficMu sync.Mutex

type sidecarTrafficJournal struct{}

// A failed filesystem write stays owned by the process runtime, so replacing
// a panel job cannot discard it. Source locks serialize access and replay.
var unstoredSidecarTraffic sync.Map // journal directory -> *service.SidecarTrafficBatch

func sidecarJournalDir(source string) string {
	return filepath.Join(config.GetDBFolderPath(), source+"-traffic-journal")
}

type sidecarTrafficCommit func(service.SidecarTrafficBatch) (bool, bool, error)

func (j *sidecarTrafficJournal) replay(source string, svc *service.InboundService) (bool, bool, error) {
	return j.replayWith(source, svc.AddSidecarTrafficBatch)
}

func (j *sidecarTrafficJournal) replayWith(source string, apply sidecarTrafficCommit) (restart, disabled bool, err error) {
	key := sidecarJournalDir(source)
	if err := sidecartraffic.FlushPending(key); err != nil {
		return false, false, err
	}
	if pending, ok := unstoredSidecarTraffic.Load(key); ok {
		batch := pending.(*service.SidecarTrafficBatch)
		data, marshalErr := json.Marshal(batch)
		if marshalErr != nil {
			return false, false, marshalErr
		}
		if _, err = storeTrafficJournal(sidecarJournalDir(source), batch.ID, data); err != nil {
			return
		}
		unstoredSidecarTraffic.Delete(key)
	}
	entries, err := os.ReadDir(sidecarJournalDir(source))
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(sidecarJournalDir(source), entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return restart, disabled, readErr
		}
		var batch service.SidecarTrafficBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			return restart, disabled, err
		}
		if batch.ID+".json" != entry.Name() {
			return restart, disabled, fmt.Errorf("%s journal batch ID mismatch", source)
		}
		r, d, applyErr := apply(batch)
		restart, disabled = restart || r, disabled || d
		if applyErr != nil {
			return restart, disabled, applyErr
		}
		if err := os.Remove(path); err != nil {
			return restart, disabled, err
		}
		if err := syncJournalDir(sidecarJournalDir(source)); err != nil {
			return restart, disabled, err
		}
	}
	return
}

func (j *sidecarTrafficJournal) commit(source string, owners service.SidecarTrafficOwners, svc *service.InboundService, inbounds []*coremodel.Traffic, clients []*coremodel.ClientTraffic) (bool, bool, error) {
	return j.commitWith(source, owners, svc.AddSidecarTrafficBatch, inbounds, clients)
}

func (j *sidecarTrafficJournal) commitWith(source string, owners service.SidecarTrafficOwners, apply sidecarTrafficCommit, inbounds []*coremodel.Traffic, clients []*coremodel.ClientTraffic) (bool, bool, error) {
	batch := owners.Batch(source+"-"+uuid.NewString(), inbounds, clients)
	if len(batch.Inbounds) == 0 && len(batch.Clients) == 0 {
		return false, false, nil
	}
	// Retain this copy if the filesystem cannot accept the WAL. Do not consume a
	// newer sample until replay stores it; never write counters before the WAL.
	unstoredSidecarTraffic.Store(sidecarJournalDir(source), &batch)
	return j.replayWith(source, apply)
}
