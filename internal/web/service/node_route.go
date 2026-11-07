package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// Node-route tags are stable across core switches and never appear as manually
// edited outbounds. They identify a managed inbound on a remote node by DB ID.
const nodeRoutePrefix = "node-route-"

var nodeRouteProvisionMu sync.Mutex

type NodeRouteTarget struct {
	Tag       string `json:"tag"`
	InboundID int    `json:"inboundId"`
	NodeID    int    `json:"nodeId"`
	NodeName  string `json:"nodeName"`
	Remark    string `json:"remark"`
	Protocol  string `json:"protocol"`
	Port      int    `json:"port"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func nodeRouteTag(inboundID int) string {
	return fmt.Sprintf("%s%d", nodeRoutePrefix, inboundID)
}

func nodeRouteInboundID(tag string) (int, bool) {
	suffix, ok := strings.CutPrefix(tag, nodeRoutePrefix)
	if !ok || suffix == "" || strings.Trim(suffix, "0123456789") != "" || suffix[0] == '0' {
		return 0, false
	}
	id, err := strconv.Atoi(suffix)
	return id, err == nil && id > 0 && nodeRouteTag(id) == tag
}

func nodeRouteClientEmail(inboundID int) string {
	return fmt.Sprintf("node-route-%d@internal", inboundID)
}

func nodeRouteProtocolSupported(protocol, core string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "vless", "vmess", "trojan", "shadowsocks":
		return true
	case "hysteria", "tuic", "anytls", "naive", "snell", "shadowtls", "mieru":
		return core != "xray"
	default:
		return false
	}
}

// NodeRouteTargets exposes only labels and availability, never credentials.
func NodeRouteTargets(userID int, core string) ([]NodeRouteTarget, error) {
	inbounds, err := (&InboundService{}).GetInboundOptions(userID)
	if err != nil {
		return nil, err
	}
	nodes, err := (&NodeService{}).GetAll()
	if err != nil {
		return nil, err
	}
	byID := make(map[int]*model.Node, len(nodes))
	for _, n := range nodes {
		byID[n.Id] = n
	}
	result := make([]NodeRouteTarget, 0)
	for _, inbound := range inbounds {
		if inbound.NodeId == nil {
			continue
		}
		n := byID[*inbound.NodeId]
		if n == nil || n.Transitive {
			continue
		}
		target := NodeRouteTarget{
			Tag: nodeRouteTag(inbound.Id), InboundID: inbound.Id,
			NodeID: n.Id, NodeName: n.Name, Remark: inbound.Remark,
			Protocol: inbound.Protocol, Port: inbound.Port,
			Available: true,
		}
		switch {
		case !inbound.Enable:
			target.Available, target.Reason = false, "Подключение выключено"
		case !n.Enable:
			target.Available, target.Reason = false, "Нода выключена"
		case !nodeRouteProtocolSupported(inbound.Protocol, core):
			target.Available, target.Reason = false, "Протокол не поддерживается выбранным ядром"
		case n.Status != "online":
			target.Available, target.Reason = false, "Нода не в сети"
		}
		result = append(result, target)
	}
	return result, nil
}

func nodeRouteSource(inboundID int) (*model.Inbound, *model.Node, error) {
	if inboundID <= 0 {
		return nil, nil, fmt.Errorf("invalid node route inbound id")
	}
	inbound, err := (&InboundService{}).GetInbound(inboundID)
	if err != nil {
		return nil, nil, err
	}
	if inbound.NodeID == nil || !inbound.Enable {
		return nil, nil, fmt.Errorf("node route %d: remote inbound unavailable", inboundID)
	}
	var node model.Node
	if err := database.GetDB().First(&node, *inbound.NodeID).Error; err != nil {
		return nil, nil, err
	}
	if !node.Enable || node.Transitive || node.Address == "" {
		return nil, nil, fmt.Errorf("node route %d: node disabled or inaccessible", inboundID)
	}
	return inbound, &node, nil
}

// EnsureNodeRoute provisions one dedicated, unlimited service credential on the
// node's existing inbound. Never borrow a user's password or reveal it to UI.
func EnsureNodeRoute(userID, inboundID int, core string) (string, bool, error) {
	nodeRouteProvisionMu.Lock()
	defer nodeRouteProvisionMu.Unlock()
	inbound, node, err := nodeRouteSource(inboundID)
	if err != nil {
		return "", false, err
	}
	if inbound.UserId != userID {
		return "", false, fmt.Errorf("inbound is not available to this user")
	}
	if node.Status != "online" {
		return "", false, fmt.Errorf("node is offline")
	}
	if !nodeRouteProtocolSupported(string(inbound.Protocol), core) {
		return "", false, fmt.Errorf("inbound protocol %s is not supported by %s", inbound.Protocol, core)
	}
	email := nodeRouteClientEmail(inboundID)
	var existing model.ClientRecord
	err = database.GetDB().Where("email = ?", email).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, err
	}
	if err == nil && !existing.Enable {
		return "", false, fmt.Errorf("node route service client is disabled")
	}
	client := model.Client{
		Email: email, Enable: true, SubID: uuid.NewString(),
		Comment: "Managed node routing connection", Group: "_system",
	}
	if err == nil {
		client.SubID = existing.SubID
		if client.SubID == "" {
			return "", false, fmt.Errorf("node route service client is missing subId")
		}
	}
	needRestart, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: client, InboundIds: []int{inboundID},
	})
	if err != nil {
		return "", needRestart, fmt.Errorf("provision node route client: %w", err)
	}
	if _, err := resolveNodeRouteOutbound(inboundID, core); err != nil {
		return "", needRestart, err
	}
	return nodeRouteTag(inboundID), needRestart, nil
}

// resolveNodeRouteOutbound derives the wire client from the same share links
// used by subscriptions. Unsupported links fail closed rather than adding a
// direct fallback; credentials are never persisted in the editable template.
func resolveNodeRouteOutbound(inboundID int, core string) (map[string]any, error) {
	inbound, node, err := nodeRouteSource(inboundID)
	if err != nil {
		return nil, err
	}
	if !nodeRouteProtocolSupported(string(inbound.Protocol), core) {
		return nil, fmt.Errorf("node route %d: %s is unsupported by %s", inboundID, inbound.Protocol, core)
	}
	var client model.ClientRecord
	if err := database.GetDB().Where("email = ?", nodeRouteClientEmail(inboundID)).First(&client).Error; err != nil {
		return nil, fmt.Errorf("node route %d: service client missing: %w", inboundID, err)
	}
	if !client.Enable || client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli() || client.TotalGB > 0 {
		return nil, fmt.Errorf("node route %d: service client disabled, expired or limited", inboundID)
	}
	links, err := (&InboundService{}).GetScopedInboundLinks(node.Address, inbound, client.Email)
	if err != nil {
		return nil, fmt.Errorf("node route %d: no usable link: %w", inboundID, err)
	}
	var lastErr error
	for _, raw := range links {
		var outbound map[string]any
		if core == "xray" {
			var parsed *link.ParseResult
			parsed, lastErr = link.ParseLink(raw)
			if lastErr == nil && parsed != nil {
				outbound = map[string]any(parsed.Outbound)
				if outbound["protocol"] != string(inbound.Protocol) {
					lastErr = fmt.Errorf("link protocol mismatch")
					continue
				}
				outbound["tag"] = nodeRouteTag(inboundID)
				var data []byte
				data, lastErr = json.Marshal(outbound)
				if lastErr == nil {
					lastErr = xray.ValidateOutboundConfig(data)
				}
			}
		} else {
			outbound, lastErr = ParseNativeOutboundShareLink(raw)
			if lastErr == nil {
				outbound["tag"] = nodeRouteTag(inboundID)
			}
		}
		if lastErr == nil && outbound != nil {
			return outbound, nil
		}
	}
	return nil, fmt.Errorf("node route %d: no compatible connection for %s: %w", inboundID, core, lastErr)
}

// nodeRouteIDs finds only explicitly referenced managed destinations.
func nodeRouteIDs(rules []map[string]any, key string) []int {
	seen := make(map[int]bool)
	var ids []int
	for _, rule := range rules {
		if rule["enabled"] == false || rule["action"] == "reject" {
			continue
		}
		tag, _ := rule[key].(string)
		if id, ok := nodeRouteInboundID(tag); ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
