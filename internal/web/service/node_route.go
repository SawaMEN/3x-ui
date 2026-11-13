package service

import (
	"context"
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
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
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
	PortLabel string `json:"portLabel,omitempty"`
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
	case "vless", "vmess", "trojan", "shadowsocks", "vetroff", "sudoku":
		// VetroffProxy and Sudoku use managed SOCKS sidecars on both cores.
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
	// InboundOptions deliberately omits credentials/settings. Read only the
	// Mieru port settings here so the routing picker shows actual listeners.
	var mieruRows []struct {
		Id       int
		Port     int
		Settings string
	}
	if err := database.GetDB().Table("inbounds").
		Select("id, port, settings").
		Where("user_id = ? AND protocol = ? AND node_id IS NOT NULL", userID, model.Mieru).
		Scan(&mieruRows).Error; err != nil {
		return nil, err
	}
	mieruPlans := make(map[int]mieruProbePlan, len(mieruRows))
	for _, row := range mieruRows {
		mieruPlans[row.Id] = nodeRouteMieruProbePlan(&model.Inbound{
			Protocol: model.Mieru, Port: row.Port, Settings: row.Settings,
		})
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
		if inbound.Protocol == string(model.Mieru) {
			target.PortLabel = strings.Join(mieruPlans[inbound.Id].portLabels, ", ")
		}
		switch {
		case !inbound.Enable:
			target.Available, target.Reason = false, "Подключение выключено"
		case !n.Enable:
			target.Available, target.Reason = false, "Нода выключена"
		case inbound.Protocol == string(model.Mieru) && target.PortLabel == "":
			target.Available, target.Reason = false, "У Mieru нет настроенных TCP/UDP-портов"
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
	if inbound.Protocol == model.Mieru && len(nodeRouteMieruProbePlan(inbound).portLabels) == 0 {
		return "", false, fmt.Errorf("node route %d: Mieru has no configured TCP/UDP listeners", inboundID)
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
	// The inbound and its service credentials live on another panel. A successful
	// local database write is not proof that the remote core can authenticate
	// the cascade. Synchronize the current complete inbound before publishing
	// its managed outbound to the local routing template.
	if err := syncNodeRouteInbound(inbound, node); err != nil {
		return "", needRestart, fmt.Errorf("node route %d: remote inbound sync: %w", inboundID, err)
	}
	if _, err := resolveNodeRouteOutbound(inboundID, core); err != nil {
		return "", needRestart, err
	}
	return nodeRouteTag(inboundID), needRestart, nil
}

// nodeRouteRemoteInboundExists matches only the inbound's tag on its own node.
// Central panel tags may carry a n<ID>- prefix not present on the node.
func nodeRouteRemoteInboundExists(inboundTag string, nodeID int, options []runtime.RemoteInboundOption) bool {
	bare := strings.TrimPrefix(inboundTag, nodeTagPrefix(&nodeID))
	for _, option := range options {
		if option.Tag == inboundTag || option.Tag == bare {
			return true
		}
	}
	return false
}

// syncNodeRouteInbound closes the gap between provisioning the dedicated
// cascade identity in the central database and applying it on the destination
// node. It pushes only the selected inbound, never the node's entire template.
func syncNodeRouteInbound(inbound *model.Inbound, node *model.Node) error {
	manager := runtime.GetManager()
	if manager == nil {
		return fmt.Errorf("runtime manager not initialized")
	}
	remote, err := manager.RemoteFor(node)
	if err != nil {
		return err
	}
	latest, err := (&InboundService{}).GetInbound(inbound.Id)
	if err != nil {
		return err
	}
	payload, err := (&InboundService{}).buildInboundForNodePush(database.GetDB(), latest)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	options, err := remote.ListInboundOptions(ctx)
	if err != nil {
		return fmt.Errorf("read destination node inbounds: %w", err)
	}
	exists := nodeRouteRemoteInboundExists(payload.Tag, node.Id, options)
	// This is an explicit prepare/repair action. The cached fingerprint only
	// describes our last push, not the destination's current client list (which
	// can change after a restore or a direct edit on that panel). Always re-send
	// the credential before restarting, even when the inbound tag still exists.
	remote.ForgetPushedInbound(payload.Tag)
	_, err = remote.ReconcileInbound(ctx, payload, exists)
	if err != nil {
		return fmt.Errorf("apply cascade credential to destination node: %w", err)
	}
	// Fingerprint reconciliation only proves that the node accepted its
	// configuration, not that the running sing-box instance loaded the service
	// credential. This explicit ensure action must activate the listener even
	// when the client is already linked and the serialized settings match.
	// It also repairs managed cascades created before the activation fix.
	// Restart can involve a full sing-box configuration reload. Give it
	// a separate deadline instead of sharing the time spent listing and
	// reconciling the remote inbound.
	activateCtx, activateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer activateCancel()
	if err := remote.RestartXray(activateCtx); err != nil {
		return fmt.Errorf("activate destination node core for managed cascade: %w", err)
	}
	return nil
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
		if inbound.Protocol == model.VetroffProxy || inbound.Protocol == model.Sudoku {
			scheme := string(inbound.Protocol) + "://"
			if !strings.HasPrefix(raw, scheme) {
				lastErr = fmt.Errorf("not a %s connection link", inbound.Protocol)
				continue
			}
			outbound = map[string]any{
				"tag": nodeRouteTag(inboundID), "protocol": string(inbound.Protocol),
				"settings": map[string]any{"link": raw},
			}
			lastErr = externalvpn.ValidateAdditionalOutbound(outbound)
		} else if core == "xray" {
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

// nodeRouteIDs finds explicitly referenced managed destinations. Native
// logical rules may nest their destination; ignoring them silently drops
// the required outbound while leaving the routing rule intact.
func nodeRouteIDs(rules []map[string]any, key string) []int {
	seen := make(map[int]bool)
	var ids []int
	var visit func(map[string]any)
	visit = func(rule map[string]any) {
		if rule["enabled"] == false || rule["action"] == "reject" {
			return
		}
		if tag, _ := rule[key].(string); tag != "" {
			if id, ok := nodeRouteInboundID(tag); ok && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if key != "outbound" {
			return
		}
		switch children := rule["rules"].(type) {
		case []any:
			for _, child := range children {
				if nested, ok := child.(map[string]any); ok {
					visit(nested)
				}
			}
		case []map[string]any:
			for _, child := range children {
				visit(child)
			}
		}
	}
	for _, rule := range rules {
		visit(rule)
	}
	return ids
}

// The native final route is a real destination even when there are no field
// rules. Include it so final=node-route-N never produces an unresolved tag.
func nativeNodeRouteIDs(rules []map[string]any, final string) []int {
	ids := nodeRouteIDs(rules, "outbound")
	if id, ok := nodeRouteInboundID(final); ok {
		for _, existing := range ids {
			if existing == id {
				return ids
			}
		}
		ids = append(ids, id)
	}
	return ids
}
