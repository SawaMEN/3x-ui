package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/recovery"
)

func CreateRecoveryBundle(ctx context.Context, password string) ([]byte, error) {
	b := recovery.Bundle{Format: 1, CreatedAt: time.Now().UTC(), PanelVersion: config.GetPanelVersion(), DatabaseType: config.GetDBKind()}
	b.CoreType, _ = (&SettingService{}).GetCoreType()
	versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	b.CoreVersion, _ = NativeCore(b.CoreType).CachedVersion(versionCtx)
	cancel()
	data, err := (&ServerService{}).GetDb()
	if err != nil {
		return nil, err
	}
	name := "database/backup.db"
	if b.DatabaseType == "postgres" {
		name = "database/backup.pgdump"
	}
	b.Add(name, config.GetDBPath(), data)
	keyPath := config.GetNodeTokenKeyFile()
	mode, err := nodetoken.ParseMode(config.GetNodeTokenEncryptionMode())
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(keyPath); err == nil {
		if _, err = (nodetoken.FileKeySource{Path: keyPath}).Load(); err != nil {
			return nil, err
		}
		raw, err := recoveryReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		b.Add("keys/node_token_key.json", keyPath, raw)
	} else if mode != nodetoken.ModeOff {
		ring, err := (nodetoken.EnvKeySource{Var: config.GetNodeTokenKeyEnv()}).Load()
		if err != nil {
			return nil, err
		}
		keys := map[string]string{}
		for id, key := range ring.Keys {
			keys[id] = base64.StdEncoding.EncodeToString(key[:])
		}
		raw, err := json.Marshal(map[string]any{"active": ring.ActiveID, "keys": keys})
		if err != nil {
			return nil, err
		}
		b.Add("keys/node_token_key.json", keyPath, raw)
	}
	paths := map[string]bool{}
	settings, err := (&SettingService{}).GetAllSetting()
	if err != nil {
		return nil, err
	}
	for _, p := range []string{settings.WebCertFile, settings.WebKeyFile, settings.SubCertFile, settings.SubKeyFile} {
		if p != "" {
			paths[p] = true
		}
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Where("node_id IS NULL").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				switch key {
				case "certificateFile", "keyFile", "certificatePath", "keyPath", "certificate", "privateKey":
					if path, ok := child.(string); ok && filepath.IsAbs(path) {
						paths[path] = true
					}
				}
				collect(child)
			}
		case []any:
			for _, child := range v {
				collect(child)
			}
		}
	}
	for _, ib := range inbounds {
		for _, raw := range []string{ib.Settings, ib.StreamSettings} {
			var obj any
			if json.Unmarshal([]byte(raw), &obj) == nil {
				collect(obj)
			}
		}
	}
	for path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := recoveryReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("required certificate %s: %w", path, err)
		}
		b.Add(fmt.Sprintf("certificates/%04d-%s", len(b.Files), filepath.Base(path)), path, raw)
	}
	environment := map[string]string{"NODE_TOKEN_ENCRYPTION": config.GetNodeTokenEncryptionMode(), "XUI_NODE_TOKEN_KEY_FILE": keyPath, "XUI_DB_TYPE": config.GetDBKind(), "XUI_DB_DSN": config.GetDBDSN()}
	raw, err := json.MarshalIndent(environment, "", "  ")
	if err != nil {
		return nil, err
	}
	b.Add("configuration/environment.json", "", raw)

	return recovery.Encode(b, password)
}

func recoveryReadFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, fmt.Errorf("not a regular recovery file smaller than 8 MiB")
	}
	return io.ReadAll(io.LimitReader(f, 8<<20))
}
