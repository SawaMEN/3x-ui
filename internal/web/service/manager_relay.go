package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type ManagerRelayInput struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	SSHPort    int    `json:"sshPort"`
	Username   string `json:"username"`
	HostKey    string `json:"hostKey"`
	Credential string `json:"credential"`
	KeyAuth    bool   `json:"keyAuth"`
	NodeID     int    `json:"nodeId"`
}

func (s *ManagerService) SaveRelay(owner int, in ManagerRelayInput) (*model.ManagerRelay, error) {
	managerMu.Lock()
	defer managerMu.Unlock()
	db := database.GetDB()
	if in.Name == "" || len(in.Name) > 256 || in.SSHPort < 1 || in.SSHPort > 65535 || in.Username == "" || len(in.Credential) > 65536 {
		return nil, fmt.Errorf("invalid relay settings")
	}
	if net.ParseIP(in.Address) == nil {
		if _, err := managerDomain(in.Address); err != nil {
			return nil, err
		}
	}
	if !strings.HasPrefix(in.HostKey, "SHA256:") || len(in.HostKey) != 50 {
		return nil, fmt.Errorf("provide the SSH SHA256 host key fingerprint from the server console")
	}
	var node model.Node
	if err := db.First(&node, in.NodeID).Error; err != nil {
		return nil, err
	}
	r := model.ManagerRelay{UserID: owner, Name: in.Name, Address: in.Address, SSHPort: in.SSHPort, Username: in.Username, HostKey: in.HostKey, KeyAuth: in.KeyAuth, NodeID: in.NodeID}
	err := db.Transaction(func(tx *gorm.DB) error {
		if in.ID > 0 {
			if err := tx.Where("id = ? AND user_id = ?", in.ID, owner).First(&r).Error; err != nil {
				return err
			}
			if r.Installed {
				return fmt.Errorf("uninstall relay before editing")
			}
			var n int64
			if err := tx.Model(&model.ManagerOperation{}).Where("relay_id = ? AND state IN ?", in.ID, []string{"queued", "running"}).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				return fmt.Errorf("relay has a pending operation")
			}
			r.Name, r.Address, r.SSHPort, r.Username, r.HostKey, r.NodeID = in.Name, in.Address, in.SSHPort, in.Username, in.HostKey, in.NodeID
			if in.Credential != "" {
				r.KeyAuth = in.KeyAuth
			}
		} else {
			if in.Credential == "" {
				return fmt.Errorf("SSH credential is required")
			}
			if err := tx.Create(&r).Error; err != nil {
				return err
			}
		}
		if in.Credential != "" {
			enc, err := nodetoken.Encrypt(-r.ID, in.Credential)
			if err != nil {
				return err
			}
			r.Credential = enc
		}
		return tx.Save(&r).Error
	})
	if err == nil {
		for _, host := range []string{in.Address, node.Address} {
			if name, e := managerDomain(host); e == nil {
				if e := s.ImportDomains(owner, name, false); e != nil {
					return nil, e
				}
			}
		}
	}
	return &r, err
}

func (s *ManagerService) EnqueueRelay(owner, id int, kind string) (*model.ManagerOperation, error) {
	managerMu.Lock()
	defer managerMu.Unlock()
	if kind != "relay-install" && kind != "relay-uninstall" && kind != "relay-delete" {
		return nil, fmt.Errorf("invalid operation")
	}
	db := database.GetDB()
	var r model.ManagerRelay
	if err := db.Where("id = ? AND user_id = ?", id, owner).First(&r).Error; err != nil {
		return nil, err
	}
	key := fmt.Sprintf("relay/%d", id)
	var old model.ManagerOperation
	if err := db.Where("active_key = ?", key).First(&old).Error; err == nil {
		return &old, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	op := &model.ManagerOperation{UserID: owner, RelayID: id, Kind: kind, State: "queued", ActiveKey: &key, Total: 1}
	return op, db.Create(op).Error
}

func (s *ManagerService) executeRelay(ctx context.Context, op *model.ManagerOperation) error {
	db := database.GetDB()
	var r model.ManagerRelay
	if err := db.Where("id = ? AND user_id = ?", op.RelayID, op.UserID).First(&r).Error; err != nil {
		return err
	}
	install := op.Kind == "relay-install"
	if !install {
		var subs []model.ManagerSubscription
		if err := db.Where("user_id = ?", op.UserID).Find(&subs).Error; err != nil {
			return err
		}
		for _, sub := range subs {
			for _, e := range append(slices.Clone(sub.Entries), sub.Published...) {
				if e.RelayID == r.ID {
					return fmt.Errorf("remove relay from subscriptions and regenerate before uninstalling")
				}
			}
		}
	}
	if op.Kind == "relay-delete" && !r.Installed {
		var n int64
		if err := db.Model(&model.ManagerOperation{}).Where("relay_id = ? AND kind = ? AND state IN ?", r.ID, "relay-install", []string{"failed", "interrupted"}).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return db.Delete(&r).Error
		}
	}
	if err := managerRetry(ctx, op, func() error { return s.applyRelay(ctx, r, install, nil) }); err != nil {
		return err
	}
	r.Installed = install
	if err := db.Save(&r).Error; err != nil {
		return err
	}
	op.Completed = 1
	if op.Kind == "relay-delete" {
		return db.Delete(&r).Error
	}
	return nil
}

func (s *ManagerService) updateRelayPorts(ctx context.Context, entries []model.ManagerEntry) error {
	ids := map[int]bool{}
	for _, e := range entries {
		if e.RelayID > 0 {
			ids[e.RelayID] = true
		}
	}
	for id := range ids {
		var r model.ManagerRelay
		if err := database.GetDB().First(&r, id).Error; err != nil {
			return err
		}
		if !r.Installed {
			return fmt.Errorf("relay is not installed")
		}
		if err := s.applyRelay(ctx, r, true, entries); err != nil {
			return err
		}
	}
	return nil
}

func (s *ManagerService) applyRelay(ctx context.Context, r model.ManagerRelay, install bool, staging []model.ManagerEntry, excluded ...int) error {
	db := database.GetDB()
	var node model.Node
	if err := db.First(&node, r.NodeID).Error; err != nil {
		return err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", node.Address)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("node has no IP")
	}
	ip := ips[0].Unmap()
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsPrivate() && !node.AllowPrivateAddress {
		return fmt.Errorf("node destination is not allowed")
	}
	entries := slices.Clone(staging)
	var subs []model.ManagerSubscription
	if err := db.Where("user_id = ?", r.UserID).Find(&subs).Error; err != nil {
		return err
	}
	for _, sub := range subs {
		if slices.Contains(excluded, sub.ID) {
			continue
		}
		entries = append(entries, sub.Published...)
	}
	ports := []int{}
	for _, e := range entries {
		if e.RelayID == r.ID && e.ActiveInboundID > 0 {
			var ib model.Inbound
			if err := db.First(&ib, e.ActiveInboundID).Error; err != nil {
				return err
			}
			ports = append(ports, ib.Port)
		}
	}
	slices.Sort(ports)
	ports = slices.Compact(ports)
	script, err := managerRelayScript(r.ID, ip, ports, install)
	if err != nil {
		return err
	}
	return managerSSH(ctx, r, script)
}

func managerRelayScript(id int, ip netip.Addr, ports []int, install bool) (string, error) {
	if id < 1 || !ip.IsValid() {
		return "", fmt.Errorf("invalid relay destination")
	}
	cmd, sysctl := "iptables", "net.ipv4.ip_forward=1"
	if ip.Is6() {
		cmd, sysctl = "ip6tables", "net.ipv6.conf.all.forwarding=1"
	}
	chain := fmt.Sprintf("XMGR_%d", id)
	path := fmt.Sprintf("/etc/3x-ui-manager/relay-%d.sh", id)
	unit := fmt.Sprintf("3x-ui-manager-relay-%d.service", id)
	hooks := []struct{ table, hook, chain string }{{"nat", "PREROUTING", chain + "D"}, {"nat", "POSTROUTING", chain + "S"}, {"filter", "FORWARD", chain}}
	var b strings.Builder
	b.WriteString("set -eu\ncommand -v " + cmd + " >/dev/null\ncommand -v systemctl >/dev/null\n")
	if !install {
		fmt.Fprintf(&b, "systemctl disable --now %s 2>/dev/null || true\n", unit)
		for _, h := range hooks {
			fmt.Fprintf(&b, "while %s -w -t %s -C %s -j %s 2>/dev/null; do %s -w -t %s -D %s -j %s; done\n", cmd, h.table, h.hook, h.chain, cmd, h.table, h.hook, h.chain)
			fmt.Fprintf(&b, "if %s -w -t %s -L %s >/dev/null 2>&1; then %s -w -t %s -F %s; %s -w -t %s -X %s; fi\n", cmd, h.table, h.chain, cmd, h.table, h.chain, cmd, h.table, h.chain)
		}
		fmt.Fprintf(&b, "rm -f %s /etc/systemd/system/%s\nsystemctl daemon-reload\n", path, unit)
		return b.String(), nil
	}
	var rules strings.Builder
	rules.WriteString("#!/bin/sh\nset -eu\nsysctl -w " + sysctl + " >/dev/null\n")
	for _, h := range hooks {
		fmt.Fprintf(&rules, "%s -w -t %s -N %s 2>/dev/null || %s -w -t %s -L %s >/dev/null\n%s -w -t %s -F %s\n", cmd, h.table, h.chain, cmd, h.table, h.chain, cmd, h.table, h.chain)
	}
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid port")
		}
		for _, proto := range []string{"tcp", "udp"} {
			fmt.Fprintf(&rules, "%s -w -t nat -A %sD -p %s -m addrtype --dst-type LOCAL --dport %d -j DNAT --to-destination %s\n", cmd, chain, proto, port, ip)
			fmt.Fprintf(&rules, "%s -w -t nat -A %sS -p %s -d %s --dport %d -m conntrack --ctstate DNAT -j MASQUERADE\n", cmd, chain, proto, ip, port)
			fmt.Fprintf(&rules, "%s -w -A %s -p %s -d %s --dport %d -j ACCEPT\n%s -w -A %s -p %s -s %s --sport %d -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT\n", cmd, chain, proto, ip, port, cmd, chain, proto, ip, port)
		}
	}
	for _, h := range hooks {
		fmt.Fprintf(&rules, "%s -w -t %s -C %s -j %s 2>/dev/null || %s -w -t %s -I %s 1 -j %s\n", cmd, h.table, h.hook, h.chain, cmd, h.table, h.hook, h.chain)
	}
	fmt.Fprintf(&b, "install -d -m 700 /etc/3x-ui-manager\ncat > %s <<'XUI_RULES'\n%sXUI_RULES\nchmod 700 %s\n", path, rules.String(), path)
	fmt.Fprintf(&b, "cat > /etc/systemd/system/%s <<'XUI_UNIT'\n[Unit]\nDescription=3x-ui manager relay %d\nAfter=network-online.target\n[Service]\nType=oneshot\nExecStart=%s\nRemainAfterExit=yes\n[Install]\nWantedBy=multi-user.target\nXUI_UNIT\nsystemctl daemon-reload\nsystemctl enable %s\nsystemctl restart %s\n", unit, id, path, unit, unit)
	return b.String(), nil
}

func managerSSH(parent context.Context, r model.ManagerRelay, script string) error {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	secret, err := nodetoken.Decrypt(-r.ID, r.Credential)
	if err != nil {
		return err
	}
	var auth ssh.AuthMethod
	if r.KeyAuth {
		key, err := ssh.ParsePrivateKey([]byte(secret))
		if err != nil {
			return fmt.Errorf("invalid private key")
		}
		auth = ssh.PublicKeys(key)
	} else {
		auth = ssh.Password(secret)
	}
	cfg := &ssh.ClientConfig{User: r.Username, Auth: []ssh.AuthMethod{auth}, Timeout: 15 * time.Second, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if subtle.ConstantTimeCompare([]byte(ssh.FingerprintSHA256(key)), []byte(r.HostKey)) != 1 {
			return fmt.Errorf("SSH host key mismatch")
		}
		return nil
	}}
	addr := net.JoinHostPort(r.Address, strconv.Itoa(r.SSHPort))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	c, ch, req, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return fmt.Errorf("SSH authentication or host key verification failed: %w", err)
	}
	client := ssh.NewClient(c, ch, req)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(script)
	session.Stdout = io.Discard
	session.Stderr = io.Discard
	if err := session.Run("sh -s"); err != nil {
		return fmt.Errorf("relay command failed: %w", err)
	}
	return nil
}
