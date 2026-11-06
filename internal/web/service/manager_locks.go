package service

import (
	"fmt"
	"slices"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type managerResourceLock struct {
	mu   sync.Mutex
	refs int
}

var managerResourceLocks = struct {
	sync.Mutex
	keys map[string]*managerResourceLock
}{keys: map[string]*managerResourceLock{}}

func managerLockKeys(keys []string) func() {
	slices.Sort(keys)
	keys = slices.Compact(keys)
	locks := make([]*managerResourceLock, len(keys))
	managerResourceLocks.Lock()
	for i, key := range keys {
		lock := managerResourceLocks.keys[key]
		if lock == nil {
			lock = &managerResourceLock{}
			managerResourceLocks.keys[key] = lock
		}
		lock.refs++
		locks[i] = lock
	}
	managerResourceLocks.Unlock()
	for _, lock := range locks {
		lock.mu.Lock()
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].mu.Unlock()
		}
		managerResourceLocks.Lock()
		defer managerResourceLocks.Unlock()
		for i, key := range keys {
			locks[i].refs--
			if locks[i].refs == 0 {
				delete(managerResourceLocks.keys, key)
			}
		}
	}
}

func managerLockSubscription(sub *model.ManagerSubscription) func() {
	keys := []string{fmt.Sprintf("subscription/%d", sub.ID)}
	entries := append(slices.Clone(sub.Entries), sub.Published...)
	for _, g := range sub.Retired {
		entries = append(entries, g.Entries...)
	}
	for _, entry := range entries {
		if entry.Mode == "custom" {
			continue
		}
		node := 0
		if entry.NodeID != nil {
			node = *entry.NodeID
		} else if entry.TemplateID > 0 || entry.ActiveInboundID > 0 {
			var ib model.Inbound
			id := entry.ActiveInboundID
			if id == 0 {
				id = entry.TemplateID
			}
			if err := database.GetDB().Select("node_id").First(&ib, id).Error; err == nil && ib.NodeID != nil {
				node = *ib.NodeID
			}
		}
		keys = append(keys, fmt.Sprintf("node/%d", node))
		if entry.RelayID > 0 {
			keys = append(keys, fmt.Sprintf("relay/%d", entry.RelayID))
		}
	}
	return managerLockKeys(keys)
}
