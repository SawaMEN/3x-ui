package database

import (
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/util/random"
)

func seedHostsFromExternalProxy() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "HostsFromExternalProxy") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if _, err := CreateHostsFromExternalProxy(tx, inbound.Id, inbound.StreamSettings); err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "HostsFromExternalProxy"}).Error
	})
}

func seedMtprotoCustomShareAddrToHosts() error {
	const seederName = "MtprotoCustomShareAddrToHosts"
	var count int64
	if err := db.Model(&model.HistoryOfSeeders{}).Where("seeder_name = ?", seederName).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var inbounds []model.Inbound
		if err := tx.Where("protocol = ? AND TRIM(COALESCE(share_addr_strategy, '')) = ?", string(model.MTProto), "custom").Find(&inbounds).Error; err != nil {
			return err
		}
		for _, inbound := range inbounds {
			if err := CreateHostFromMtprotoCustomShareAddr(tx, inbound.Id, inbound.ShareAddr); err != nil {
				return err
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Updates(map[string]any{
				"share_addr_strategy": "listen",
				"share_addr":          "",
			}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: seederName}).Error
	})
}

func seedWireguardPeersToClients() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "WireguardPeersToClients") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", string(model.WireGuard)).Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		usedEmails := map[string]struct{}{}
		var existingEmails []string
		if err := tx.Model(&model.ClientRecord{}).Pluck("email", &existingEmails).Error; err != nil {
			return err
		}
		for _, e := range existingEmails {
			usedEmails[e] = struct{}{}
		}

		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("WireguardPeersToClients: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			peers, ok := settings["peers"].([]any)
			if !ok || len(peers) == 0 {
				continue
			}

			var linkCount int64
			if err := tx.Model(&model.ClientInbound{}).Where("inbound_id = ?", inbound.Id).Count(&linkCount).Error; err != nil {
				return err
			}
			if linkCount > 0 {
				continue
			}

			clientObjs := make([]any, 0, len(peers))
			for i, raw := range peers {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				email := wireguardPeerEmail(inbound.Remark, obj, i, usedEmails)
				usedEmails[email] = struct{}{}
				obj["email"] = email
				if sub, _ := obj["subId"].(string); strings.TrimSpace(sub) == "" {
					obj["subId"] = random.NumLower(16)
				}
				if _, ok := obj["enable"]; !ok {
					obj["enable"] = true
				}

				blob, err := json.Marshal(obj)
				if err != nil {
					continue
				}
				var c model.Client
				if err := json.Unmarshal(blob, &c); err != nil {
					log.Printf("WireguardPeersToClients: skip peer in inbound %d: %v", inbound.Id, err)
					continue
				}
				c.Email = email

				incoming := c.ToRecord()
				var row model.ClientRecord
				err = tx.Where("email = ?", email).First(&row).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if err := tx.Create(incoming).Error; err != nil {
						return err
					}
					row = *incoming
				} else if err != nil {
					return err
				} else {
					model.MergeClientRecord(&row, incoming)
					if err := tx.Save(&row).Error; err != nil {
						return err
					}
				}

				link := model.ClientInbound{ClientId: row.Id, InboundId: inbound.Id}
				if err := tx.Where("client_id = ? AND inbound_id = ?", row.Id, inbound.Id).
					FirstOrCreate(&link).Error; err != nil {
					return err
				}

				clientObjs = append(clientObjs, obj)
			}

			delete(settings, "peers")
			settings["clients"] = clientObjs
			newSettings, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "WireguardPeersToClients"}).Error
	})
}

// seedMtprotoSecretsToClients converts each legacy single-secret mtproto inbound
// into a one-client inbound so MTProto joins the shared multi-client model: the
// inbound-level secret becomes the first client's FakeTLS secret, and a
// ClientRecord + client_inbounds link are created so per-client traffic, limits,
// and share links work exactly like every other protocol. One-time, self-gated
// on the "MtprotoSecretsToClients" seeder row. Mirrors seedWireguardPeersToClients.
func seedMtprotoSecretsToClients() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "MtprotoSecretsToClients") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", string(model.MTProto)).Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		usedEmails := map[string]struct{}{}
		var existingEmails []string
		if err := tx.Model(&model.ClientRecord{}).Pluck("email", &existingEmails).Error; err != nil {
			return err
		}
		for _, e := range existingEmails {
			usedEmails[e] = struct{}{}
		}

		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("MtprotoSecretsToClients: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			if clients, ok := settings["clients"].([]any); ok && len(clients) > 0 {
				continue
			}

			var linkCount int64
			if err := tx.Model(&model.ClientInbound{}).Where("inbound_id = ?", inbound.Id).Count(&linkCount).Error; err != nil {
				return err
			}
			if linkCount > 0 {
				continue
			}

			secret, _ := settings["secret"].(string)
			secret = strings.TrimSpace(secret)
			if secret == "" {
				domain, _ := settings["fakeTlsDomain"].(string)
				secret = model.GenerateFakeTLSSecret(strings.TrimSpace(domain))
			}

			email := mtprotoInboundClientEmail(inbound.Remark, usedEmails)
			usedEmails[email] = struct{}{}

			obj := map[string]any{
				"email":  email,
				"secret": secret,
				"enable": true,
				"subId":  random.NumLower(16),
			}
			c := model.Client{Email: email, Secret: secret, Enable: true, SubID: obj["subId"].(string)}

			incoming := c.ToRecord()
			var row model.ClientRecord
			err := tx.Where("email = ?", email).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(incoming).Error; err != nil {
					return err
				}
				row = *incoming
			} else if err != nil {
				return err
			} else {
				model.MergeClientRecord(&row, incoming)
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
			}

			link := model.ClientInbound{ClientId: row.Id, InboundId: inbound.Id}
			if err := tx.Where("client_id = ? AND inbound_id = ?", row.Id, inbound.Id).
				FirstOrCreate(&link).Error; err != nil {
				return err
			}

			delete(settings, "secret")
			settings["clients"] = []any{obj}
			newSettings, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "MtprotoSecretsToClients"}).Error
	})
}

func seedRandomSubscriptionPaths() error {
	settings := []model.Setting{
		{Key: "subPath", Value: "/" + random.NumLower(16) + "/"},
		{Key: "subJsonPath", Value: "/" + random.NumLower(16) + "/"},
		{Key: "subClashPath", Value: "/" + random.NumLower(16) + "/"},
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range settings {
			if err := tx.Where("key = ?", settings[i].Key).FirstOrCreate(&settings[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func runSeeders(isUsersEmpty bool) error {
	empty, err := isTableEmpty("history_of_seeders")
	if err != nil {
		log.Printf("Error checking if users table is empty: %v", err)
		return err
	}

	if empty && isUsersEmpty {
		seeders := []string{"UserPasswordHash", "ClientsTable", "InboundClientsArrayFix", "InboundClientTgIdFix2", "InboundClientSubIdFix", "FreedomFinalRulesReverseFix", "FreedomFinalRulesPrivateEgressBlock", "UppercaseFreedomFinalRulesFix", "InboundRealityFinalmaskTcpStrip", "ApiTokensHash", "LegacyProxySettingsCleanup", "OutboundRemovedKeysFix", "FreedomDomainStrategyFix", "DNSOutboundLegacyKeysFix", "DNSOutboundQTypeZeroFix", "HostVerifyPeerCertByNameBoolFix", "WireguardPeersToClients", "MtprotoSecretsToClients", "NodeInboundsAdopted", "HostMihomoX25519Reset", "NodeDownNotified", "ResetIpLimitNoFail2ban", "VKTurnAcceptClientKeysDefaultOn"}
		for _, name := range seeders {
			if err := db.Create(&model.HistoryOfSeeders{SeederName: name}).Error; err != nil {
				return err
			}
		}
		return seedApiTokens()
	}

	var seedersHistory []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &seedersHistory).Error; err != nil {
		log.Printf("Error fetching seeder history: %v", err)
		return err
	}

	if !slices.Contains(seedersHistory, "UserPasswordHash") && !isUsersEmpty {
		var users []model.User
		if err := db.Find(&users).Error; err != nil {
			log.Printf("Error fetching users for password migration: %v", err)
			return err
		}

		for _, user := range users {
			if crypto.IsHashed(user.Password) {
				continue
			}
			hashedPassword, err := crypto.HashPasswordAsBcrypt(user.Password)
			if err != nil {
				log.Printf("Error hashing password for user '%s': %v", user.Username, err)
				return err
			}
			if err := db.Model(&user).Update("password", hashedPassword).Error; err != nil {
				log.Printf("Error updating password for user '%s': %v", user.Username, err)
				return err
			}
		}

		hashSeeder := &model.HistoryOfSeeders{
			SeederName: "UserPasswordHash",
		}
		if err := db.Create(hashSeeder).Error; err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "ApiTokensTable") {
		if err := seedApiTokens(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "ApiTokensHash") {
		if err := hashExistingApiTokens(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "ClientsTable") {
		if err := seedClientsFromInboundJSON(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundClientsArrayFix") {
		if err := normalizeInboundClientsArray(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundClientTgIdFix2") {
		if err := normalizeInboundClientTgId(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundClientSubIdFix") {
		if err := normalizeInboundClientSubId(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "FreedomFinalRulesReverseFix") {
		if err := normalizeFreedomFinalRules(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "FreedomFinalRulesPrivateEgressBlock") {
		if err := hardenFreedomFinalRules(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "UppercaseFreedomFinalRulesFix") {
		if err := fixUppercaseFreedomFinalRules(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundRealityFinalmaskTcpStrip") {
		if err := stripRealityFinalmaskTcp(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "LegacyProxySettingsCleanup") {
		if err := clearLegacyProxySettings(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "OutboundRemovedKeysFix") {
		if err := migrateOutboundRemovedKeys(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "FreedomDomainStrategyFix") {
		if err := migrateFreedomDomainStrategy(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "DNSOutboundLegacyKeysFix") {
		if err := migrateDNSOutboundLegacyKeys(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "DNSOutboundQTypeZeroFix") {
		if err := migrateDNSOutboundQTypeZero(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "HostVerifyPeerCertByNameBoolFix") {
		if err := blankLegacyBoolVerifyPeerCertByName(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "NodeInboundsAdopted") {
		if err := seedNodeInboundsAdopted(); err != nil {
			return err
		}
	}

	if err := seedHostsFromExternalProxy(); err != nil {
		return err
	}

	if err := repairImportedHostTLS(); err != nil {
		return err
	}

	if err := seedMtprotoCustomShareAddrToHosts(); err != nil {
		return err
	}

	if err := resetIpLimitsWithoutFail2ban(); err != nil {
		return err
	}

	if err := seedWireguardPeersToClients(); err != nil {
		return err
	}

	if err := backfillEmptyHostGroupIds(); err != nil {
		return err
	}

	// Self-gated on the "MtprotoSecretsToClients" row.
	if err := seedMtprotoSecretsToClients(); err != nil {
		return err
	}

	// Self-gated on the "StripMtprotoInboundSecrets" row. Must run after the
	// seeder above so legacy single-secret inbounds are first converted to a
	// client (which preserves the secret) before the inbound-level copy is
	// dropped from every mtproto inbound.
	if err := stripMtprotoInboundSecrets(); err != nil {
		return err
	}

	if err := migrateVKTurnAcceptClientKeysDefaultOn(); err != nil {
		return err
	}

	// Idempotent, not seeder-gated: bad values can re-enter via a restored
	// backup, so re-check on every start.
	if !slices.Contains(seedersHistory, "HostMihomoX25519Reset") {
		if err := resetPreexistingMihomoX25519(); err != nil {
			return err
		}
	}
	if !slices.Contains(seedersHistory, "NodeDownNotified") {
		if err := seedNodeDownNotified(); err != nil {
			return err
		}
	}
	return normalizeSettingPaths()
}

// seedNodeInboundsAdopted keeps the pre-existing reconcile behavior for nodes
// that were already syncing before the inbounds_adopted_at gate was introduced.
func seedNodeInboundsAdopted() error {
	if err := db.Model(&model.Node{}).
		Where("inbounds_adopted_at = 0").
		Update("inbounds_adopted_at", time.Now().Unix()).Error; err != nil {
		return err
	}
	return db.Create(&model.HistoryOfSeeders{SeederName: "NodeInboundsAdopted"}).Error
}

func seedClientsFromInboundJSON() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		byEmail := map[string]*model.ClientRecord{}

		var existing []model.ClientRecord
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		for i := range existing {
			byEmail[existing[i].Email] = &existing[i]
		}

		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("ClientsTable seed: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			rawList, ok := settings["clients"].([]any)
			if !ok {
				continue
			}

			for _, raw := range rawList {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				normalizeClientJSONFields(obj)
				blob, err := json.Marshal(obj)
				if err != nil {
					continue
				}
				var c model.Client
				if err := json.Unmarshal(blob, &c); err != nil {
					log.Printf("ClientsTable seed: skip client in inbound %d (unmarshal failed): %v",
						inbound.Id, err)
					continue
				}
				email := strings.TrimSpace(c.Email)
				if email == "" {
					continue
				}
				incoming := c.ToRecord()

				row, dup := byEmail[email]
				if !dup {
					if err := tx.Create(incoming).Error; err != nil {
						return err
					}
					byEmail[email] = incoming
					row = incoming
				} else {
					conflicts := model.MergeClientRecord(row, incoming)
					for _, x := range conflicts {
						log.Printf("client merge: email=%s conflict on %s old=%v new=%v kept=%v",
							email, x.Field, x.Old, x.New, x.Kept)
					}
					if err := tx.Save(row).Error; err != nil {
						return err
					}
				}

				link := model.ClientInbound{
					ClientId:     row.Id,
					InboundId:    inbound.Id,
					FlowOverride: c.Flow,
				}
				if err := tx.Where("client_id = ? AND inbound_id = ?", row.Id, inbound.Id).
					FirstOrCreate(&link).Error; err != nil {
					return err
				}
			}
		}

		return tx.Create(&model.HistoryOfSeeders{SeederName: "ClientsTable"}).Error
	})
}

func seedApiTokens() error {
	empty, err := isTableEmpty("api_tokens")
	if err != nil {
		return err
	}
	if empty {
		var legacy model.Setting
		err := db.Model(model.Setting{}).Where("key = ?", "apiToken").First(&legacy).Error
		if err == nil && legacy.Value != "" {
			row := &model.ApiToken{
				Name:    "default",
				Token:   legacy.Value,
				Enabled: true,
			}
			if err := db.Create(row).Error; err != nil {
				log.Printf("Error migrating legacy apiToken: %v", err)
				return err
			}
		}
	}
	return db.Create(&model.HistoryOfSeeders{SeederName: "ApiTokensTable"}).Error
}

func hashExistingApiTokens() error {
	var rows []*model.ApiToken
	if err := db.Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		if crypto.IsSHA256Hex(r.Token) {
			continue
		}
		hashed := crypto.HashTokenSHA256(r.Token)
		if err := db.Model(model.ApiToken{}).Where("id = ?", r.Id).Update("token", hashed).Error; err != nil {
			log.Printf("Error hashing api token %d: %v", r.Id, err)
			return err
		}
	}
	return db.Create(&model.HistoryOfSeeders{SeederName: "ApiTokensHash"}).Error
}

// seedNodeDownNotified treats nodes already offline at upgrade as announced,
// so the first heartbeat after it does not re-send their node.down.
func seedNodeDownNotified() error {
	if err := db.Model(&model.Node{}).
		Where("status = ?", "offline").
		Update("down_notified", true).Error; err != nil {
		return err
	}
	return db.Create(&model.HistoryOfSeeders{SeederName: "NodeDownNotified"}).Error
}
