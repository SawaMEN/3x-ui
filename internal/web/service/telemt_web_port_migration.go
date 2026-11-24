package service

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

const (
	telemtPortMigrationKey    = "telemtIngressPortMigration"
	telemtPortMigrationWindow = 3 * time.Minute
)

// Only the two listener ports are changed. Keep a crash-safe journal in the
// same SQLite transaction; never store certificates, credentials or clients.
type TelemtPortMigration struct {
	ID              string    `json:"id"`
	OldPanel        int       `json:"oldPanel"`
	NewPanel        int       `json:"newPanel"`
	OldSubscription int       `json:"oldSubscription"`
	NewSubscription int       `json:"newSubscription"`
	Deadline        time.Time `json:"deadline"`
	Ready           bool      `json:"ready"`
}

type TelemtPortMigrationRequest struct {
	PanelPort                int `json:"panelPort"`
	SubscriptionPort         int `json:"subscriptionPort"`
	ExpectedPanelPort        int `json:"expectedPanelPort"`
	ExpectedSubscriptionPort int `json:"expectedSubscriptionPort"`
}

func loadTelemtPortMigration(db *gorm.DB) (*TelemtPortMigration, error) {
	var row model.Setting
	if err := db.Where("key = ?", telemtPortMigrationKey).First(&row).Error; err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var migration TelemtPortMigration
	if err := json.Unmarshal([]byte(row.Value), &migration); err != nil {
		return nil, err
	}
	if migration.ID == "" || migration.Deadline.IsZero() || migration.OldPanel < 1 || migration.NewPanel < 1 ||
		migration.OldPanel > 65535 || migration.NewPanel > 65535 || migration.OldSubscription < 0 ||
		migration.NewSubscription < 0 || migration.OldSubscription > 65535 || migration.NewSubscription > 65535 {
		return nil, errors.New("invalid WEB port migration journal")
	}
	return &migration, nil
}

func TelemtPendingPortMigration() (*TelemtPortMigration, error) {
	return loadTelemtPortMigration(database.GetDB())
}

func saveTelemtPortMigration(tx *gorm.DB, migration *TelemtPortMigration) error {
	data, err := json.Marshal(migration)
	if err != nil {
		return err
	}
	return saveSettingIn(tx, telemtPortMigrationKey, string(data))
}

func clearTelemtPortMigration(tx *gorm.DB) error {
	return tx.Where("key = ?", telemtPortMigrationKey).Delete(&model.Setting{}).Error
}

func telemtMigrationPort(tx *gorm.DB, key string) (int, error) {
	var row model.Setting
	if err := tx.Where("key = ?", key).First(&row).Error; err != nil && !database.IsNotFound(err) {
		return 0, err
	}
	return strconv.Atoi(effectiveSettingValue(key, row.Value))
}

func telemtMigrationMatches(tx *gorm.DB, m *TelemtPortMigration) (bool, error) {
	panel, err := telemtMigrationPort(tx, "webPort")
	if err != nil {
		return false, err
	}
	if panel != m.NewPanel {
		return false, nil
	}
	if m.OldSubscription == m.NewSubscription {
		return true, nil
	}
	subscription, err := telemtMigrationPort(tx, "subPort")
	return subscription == m.NewSubscription, err
}

// Stage an explicit, bounded migration. The existing bind address is kept so
// the administrator can reach the new port before enabling the shared entry.
// A rollback deadline protects against cloud firewalls or failed reconnects.
func (TelemtService) StagePortMigration(ctx context.Context, req TelemtPortMigrationRequest) (*TelemtPortMigration, error) {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	state, err := readTelemtWebState()
	if err != nil {
		return nil, err
	}
	if state.Enabled {
		return nil, errors.New("disable the existing WEB Proxy before staging a listener port migration")
	}
	service := &SettingService{}
	settings, err := service.GetAllSetting()
	if err != nil {
		return nil, err
	}
	effectivePanel := settings.WebPort
	portOverride, overridden, err := config.GetPortOverride()
	if err != nil {
		return nil, err
	}
	if overridden {
		effectivePanel = portOverride
	}
	if req.ExpectedPanelPort != effectivePanel || (settings.SubEnable && req.ExpectedSubscriptionPort != settings.SubPort) {
		return nil, errors.New("listener settings changed; refresh the setup preview")
	}
	m := &TelemtPortMigration{OldPanel: settings.WebPort, NewPanel: settings.WebPort, Deadline: time.Now().Add(telemtPortMigrationWindow)}
	if settings.SubEnable {
		m.OldSubscription, m.NewSubscription = settings.SubPort, settings.SubPort
	}
	if req.PanelPort != 0 {
		if overridden {
			return nil, errors.New("panel port is controlled by XUI_PORT; change the service environment instead")
		}
		if settings.WebPort != 443 {
			return nil, errors.New("automatic panel migration only frees TCP 443")
		}
		if _, err := tls.LoadX509KeyPair(settings.WebCertFile, settings.WebKeyFile); err != nil {
			return nil, errors.New("configure a valid panel HTTPS certificate and key before migrating")
		}
		m.NewPanel = req.PanelPort
	}
	if req.SubscriptionPort != 0 {
		if !settings.SubEnable || (settings.SubPort != 443 && settings.SubPort != effectivePanel) {
			return nil, errors.New("subscription listener does not require this migration")
		}
		m.NewSubscription = req.SubscriptionPort
	}
	if m.NewPanel == m.OldPanel && m.NewSubscription == m.OldSubscription {
		return nil, errors.New("no listener port changes requested")
	}
	finalPanelPort := effectivePanel
	if m.NewPanel != m.OldPanel {
		finalPanelPort = m.NewPanel
	}
	if settings.SubEnable && finalPanelPort == m.NewSubscription {
		return nil, errors.New("panel and subscriptions require different internal ports")
	}
	nativePort, err := telemtWebNativePort()
	if err != nil {
		return nil, err
	}
	// Check the requested, previewed port, not a silently substituted port.
	for _, item := range []struct {
		old, next int
		listen    string
	}{
		{m.OldPanel, m.NewPanel, settings.WebListen},
		{m.OldSubscription, m.NewSubscription, settings.SubListen},
	} {
		if item.old == item.next {
			continue
		}
		if item.next < 18080 || item.next > 18179 {
			return nil, errors.New("migration port must be in the suggested 18080–18179 range")
		}
		if _, err := telemtIngressFindPort(ctx, item.next, item.next, nativePort); err != nil {
			return nil, err
		}
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(item.listen, strconv.Itoa(item.next)))
		if err != nil {
			return nil, fmt.Errorf("new listener cannot bind: %w", err)
		}
		_ = listener.Close()
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	m.ID = hex.EncodeToString(nonce[:])
	err = runSerializedTx(func(tx *gorm.DB) error {
		if pending, err := loadTelemtPortMigration(tx); err != nil {
			return err
		} else if pending != nil {
			return errors.New("another port migration is pending")
		}
		panel, err := telemtMigrationPort(tx, "webPort")
		if err != nil || panel != m.OldPanel {
			return errors.New("panel port changed; refresh preview")
		}
		if settings.SubEnable {
			subscription, err := telemtMigrationPort(tx, "subPort")
			if err != nil || subscription != m.OldSubscription {
				return errors.New("subscription port changed; refresh preview")
			}
		}
		for _, port := range []int{req.PanelPort, req.SubscriptionPort} {
			if port == 0 {
				continue
			}
			candidate := &model.Inbound{Protocol: model.HTTP, Listen: "127.0.0.1", Port: port, Settings: "{}"}
			conflict, err := checkPortConflictTx(tx, candidate, 0)
			if err != nil {
				return err
			}
			if conflict != nil {
				return errors.New(conflict.String())
			}
		}
		if err := saveTelemtPortMigration(tx, m); err != nil {
			return err
		}
		if m.NewPanel != m.OldPanel {
			if err := saveSettingIn(tx, "webPort", strconv.Itoa(m.NewPanel)); err != nil {
				return err
			}
		}
		if m.NewSubscription != m.OldSubscription {
			return saveSettingIn(tx, "subPort", strconv.Itoa(m.NewSubscription))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Called by the panel lifecycle only after both listeners have started.
func MarkTelemtPortMigrationReady() error {
	return runSerializedTx(func(tx *gorm.DB) error {
		m, err := loadTelemtPortMigration(tx)
		if err != nil || m == nil {
			return err
		}
		if !time.Now().Before(m.Deadline) {
			return errors.New("port migration confirmation window expired")
		}
		ok, err := telemtMigrationMatches(tx, m)
		if err != nil || !ok {
			return errors.New("port migration settings no longer match")
		}
		m.Ready = true
		return saveTelemtPortMigration(tx, m)
	})
}

func ConfirmTelemtPortMigration(id string) error {
	return runSerializedTx(func(tx *gorm.DB) error {
		m, err := loadTelemtPortMigration(tx)
		if err != nil {
			return err
		}
		if m == nil || m.ID != id || !m.Ready || !time.Now().Before(m.Deadline) {
			return errors.New("migration is not ready or has expired")
		}
		ok, err := telemtMigrationMatches(tx, m)
		if err != nil || !ok {
			return errors.New("port migration settings no longer match")
		}
		return clearTelemtPortMigration(tx)
	})
}

// Roll back after a failed start, process crash or unconfirmed reconnect.
// Conditional writes do not clobber unrelated later changes. The caller
// restarts listeners after a true result.
func RollbackTelemtPortMigration(expiredOnly bool) (bool, error) {
	rolledBack := false
	err := runSerializedTx(func(tx *gorm.DB) error {
		m, err := loadTelemtPortMigration(tx)
		if err != nil || m == nil {
			return err
		}
		if expiredOnly && time.Now().Before(m.Deadline) {
			return nil
		}
		for _, item := range []struct {
			key       string
			old, next int
		}{
			{"webPort", m.OldPanel, m.NewPanel},
			{"subPort", m.OldSubscription, m.NewSubscription},
		} {
			if item.old == item.next {
				continue
			}
			current, err := telemtMigrationPort(tx, item.key)
			if err != nil {
				return err
			}
			if current == item.next {
				if err := saveSettingIn(tx, item.key, strconv.Itoa(item.old)); err != nil {
					return err
				}
				rolledBack = true
			}
		}
		return clearTelemtPortMigration(tx)
	})
	return rolledBack && err == nil, err
}
