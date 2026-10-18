package runtime

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/tuic"
)

type LocalDeps struct {
	APIPort  func() int
	CoreType func() string
	// RequestConfigApply queues one validated apply of committed DB state.
	RequestConfigApply func(context.Context) error
	RestartCore        func(context.Context) error // Compatibility for existing embedders.
	SetNeedRestart     func()
}

type Local struct {
	deps         LocalDeps
	batchMu      sync.Mutex
	batches      int
	batchDirty   bool
	batchVersion uint64
}

func NewLocal(deps LocalDeps) *Local { return &Local{deps: deps} }
func (l *Local) Name() string        { return "local" }

func (l *Local) isNative() bool {
	return true
}

func (l *Local) applyCoreChange(ctx context.Context) error {
	if l.isNative() {
		l.batchMu.Lock()
		if l.batches > 0 {
			l.batchDirty = true
			l.batchMu.Unlock()
			return nil
		}
		l.batchMu.Unlock()
		return l.requestNativeApply(ctx)
	}
	if l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	return nil
}

func (l *Local) AddInbound(ctx context.Context, ib *model.Inbound) error {
	if ib.Protocol == model.Sudoku {
		// Sudoku is reconciled by its dedicated sidecar manager.
		return nil
	}
	if ib.Protocol == model.Mieru {
		inst, ok := mieru.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return mieru.GetManager().Ensure(inst)
	}
	if externalvpn.IsManaged(ib.Protocol) {
		inst, err := externalvpn.FromInbound(ib)
		if err != nil {
			return err
		}
		return externalvpn.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.MTProto {
		if mtproto.WebProxyInbound(ib) {
			mtproto.GetManager().Remove(ib.Id)
			return nil
		}
		inst, ok := mtproto.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return mtproto.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.AmneziaWG {
		inst, ok := amneziawg.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		err := amneziawgnet.GetManager().Ensure(amneziawgnet.Desired{Instance: inst, Options: amneziawgnet.DeviceOptions{HeaderProtectionKey: inst.Obfuscation.HeaderProtectionKey, ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition, RekeyAfterTime: inst.Obfuscation.RekeyAfterTime, RekeyTimeout: inst.Obfuscation.RekeyTimeout, RejectAfterTime: inst.Obfuscation.RejectAfterTime, KeepaliveTimeout: inst.Obfuscation.KeepaliveTimeout, MaxHandshakeAttempts: inst.Obfuscation.MaxHandshakeAttempts, RandomTrailers: inst.Obfuscation.RandomTrailers, DisableCookies: inst.Obfuscation.DisableCookies}})
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return err
	}
	if ib.Protocol == model.TUIC {
		if l.isNative() {
			return l.applyCoreChange(ctx)
		}
		inst, ok := tuic.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		err := tuic.GetManager().Ensure(inst)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return err
	}
	if l.isNative() {
		return l.applyCoreChange(ctx)
	}
	return l.applyCoreChange(ctx)
}

func (l *Local) DelInbound(ctx context.Context, ib *model.Inbound) error {
	if ib.Protocol == model.Sudoku {
		// The Sudoku reconcile job stops disabled/removed sidecars while keeping
		// credentials intact for a temporary disable.
		return nil
	}
	if ib.Protocol == model.Mieru {
		mieru.GetManager().Remove(ib.Id)
		return nil
	}
	if externalvpn.IsManaged(ib.Protocol) {
		externalvpn.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.MTProto {
		mtproto.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.AmneziaWG {
		amneziawgnet.GetManager().Remove(ib.Id)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	if ib.Protocol == model.TUIC {
		tuic.GetManager().Remove(ib.Id)
		if l.isNative() {
			return l.applyCoreChange(ctx)
		}
		return nil
	}
	if l.isNative() {
		return l.applyCoreChange(ctx)
	}
	return l.applyCoreChange(ctx)
}

func (l *Local) UpdateInbound(ctx context.Context, oldIb, newIb *model.Inbound) (err error) {
	finish := l.BeginConfigBatch()
	defer func() {
		// Sidecar transitions can add/remove an egress bridge even when the new
		// listener is disabled. The full native configuration must follow the DB.
		if l.isNative() && oldIb.Protocol != newIb.Protocol {
			err = errors.Join(err, l.applyCoreChange(ctx))
		}
		err = errors.Join(err, finish())
	}()
	if externalvpn.IsManaged(oldIb.Protocol) || externalvpn.IsManaged(newIb.Protocol) {
		if err := l.DelInbound(ctx, oldIb); err != nil {
			return err
		}
		if newIb.Enable {
			return l.AddInbound(ctx, newIb)
		}
		return nil
	}
	if l.isNative() && oldIb.Protocol != model.MTProto && oldIb.Protocol != model.AmneziaWG && oldIb.Protocol != model.Mieru && oldIb.Protocol != model.Sudoku && newIb.Protocol != model.MTProto && newIb.Protocol != model.AmneziaWG && newIb.Protocol != model.Mieru && newIb.Protocol != model.Sudoku {
		if oldIb.Protocol == model.TUIC {
			tuic.GetManager().Remove(oldIb.Id)
		}
		return l.applyCoreChange(ctx)
	}
	if oldIb.Protocol == model.Mieru || newIb.Protocol == model.Mieru {
		return l.updateMieruInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.MTProto || newIb.Protocol == model.MTProto {
		return l.updateMtprotoInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.AmneziaWG || newIb.Protocol == model.AmneziaWG {
		return l.updateAmneziaWGInbound(ctx, oldIb, newIb)
	}
	if oldIb.Protocol == model.TUIC || newIb.Protocol == model.TUIC {
		return l.updateTuicInbound(ctx, oldIb, newIb)
	}
	_ = l.DelInbound(ctx, oldIb)
	if !newIb.Enable {
		return nil
	}
	return l.AddInbound(ctx, newIb)
}

func (l *Local) updateMtprotoInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.MTProto && newIb.Protocol != model.MTProto {
		mtproto.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.MTProto {
		_ = l.DelInbound(ctx, oldIb)
	}
	if !newIb.Enable {
		mtproto.GetManager().Remove(newIb.Id)
		return nil
	}
	if mtproto.WebProxyInbound(newIb) {
		mtproto.GetManager().Remove(newIb.Id)
		return nil
	}
	inst, ok := mtproto.InstanceFromInbound(newIb)
	if !ok {
		mtproto.GetManager().Remove(newIb.Id)
		return nil
	}
	return mtproto.GetManager().Ensure(inst)
}

func (l *Local) updateAmneziaWGInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	if oldIb.Protocol == model.AmneziaWG && newIb.Protocol != model.AmneziaWG {
		amneziawgnet.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.AmneziaWG {
		_ = l.DelInbound(ctx, oldIb)
	}
	if !newIb.Enable {
		amneziawgnet.GetManager().Remove(newIb.Id)
		return nil
	}
	inst, ok := amneziawg.InstanceFromInbound(newIb)
	if !ok {
		amneziawgnet.GetManager().Remove(newIb.Id)
		return nil
	}
	return amneziawgnet.GetManager().Ensure(amneziawgnet.Desired{Instance: inst, Options: amneziawgnet.DeviceOptions{HeaderProtectionKey: inst.Obfuscation.HeaderProtectionKey, ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition, RekeyAfterTime: inst.Obfuscation.RekeyAfterTime, RekeyTimeout: inst.Obfuscation.RekeyTimeout, RejectAfterTime: inst.Obfuscation.RejectAfterTime, KeepaliveTimeout: inst.Obfuscation.KeepaliveTimeout, MaxHandshakeAttempts: inst.Obfuscation.MaxHandshakeAttempts, RandomTrailers: inst.Obfuscation.RandomTrailers, DisableCookies: inst.Obfuscation.DisableCookies}})
}

func (l *Local) updateTuicInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.TUIC && newIb.Protocol != model.TUIC {
		tuic.GetManager().Remove(oldIb.Id)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.TUIC {
		_ = l.DelInbound(ctx, oldIb)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
	}
	if oldIb.Protocol == model.TUIC && newIb.Protocol == model.TUIC && oldIb.Enable && newIb.Enable && oldIb.Tag != newIb.Tag && l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	if !newIb.Enable {
		tuic.GetManager().Remove(newIb.Id)
		if oldIb.Enable && l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	if !oldIb.Enable && newIb.Enable {
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
	}
	inst, ok := tuic.InstanceFromInbound(newIb)
	if !ok {
		tuic.GetManager().Remove(newIb.Id)
		return nil
	}
	return tuic.GetManager().Ensure(inst)
}

func (l *Local) updateMieruInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.Mieru && newIb.Protocol != model.Mieru {
		mieru.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.Mieru {
		_ = l.DelInbound(ctx, oldIb)
	}
	if !newIb.Enable {
		mieru.GetManager().Remove(newIb.Id)
		return nil
	}
	inst, ok := mieru.InstanceFromInbound(newIb)
	if !ok {
		mieru.GetManager().Remove(newIb.Id)
		return nil
	}
	return mieru.GetManager().Ensure(inst)
}

func (l *Local) AddUser(ctx context.Context, ib *model.Inbound, userMap map[string]any) error {
	if ib.Protocol == model.MTProto || ib.Protocol == model.AmneziaWG || ib.Protocol == model.Mieru || externalvpn.IsManaged(ib.Protocol) || ib.Protocol == model.Sudoku {
		return nil
	}
	return l.applyCoreChange(ctx)
}

func (l *Local) RemoveUser(ctx context.Context, ib *model.Inbound, email string) error {
	if ib.Protocol == model.MTProto || ib.Protocol == model.AmneziaWG || ib.Protocol == model.Mieru || externalvpn.IsManaged(ib.Protocol) || ib.Protocol == model.Sudoku {
		return nil
	}
	return l.applyCoreChange(ctx)
}

func (l *Local) AddClient(ctx context.Context, ib *model.Inbound, client model.Client) error {
	if !client.Enable {
		return nil
	}
	return l.AddUser(ctx, ib, map[string]any{"email": client.Email, "id": client.ID, "security": client.Security, "flow": client.Flow, "auth": client.Auth, "password": client.Password, "publicKey": client.PublicKey, "allowedIPs": client.AllowedIPs, "preSharedKey": client.PreSharedKey, "keepAlive": wgKeepAlive(client.KeepAliveSeconds())})
}

func (l *Local) DeleteUser(ctx context.Context, ib *model.Inbound, email string) error {
	if email == "" {
		return nil
	}
	if err := l.RemoveUser(ctx, ib, email); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	return nil
}
func (l *Local) DeleteClient(context.Context, string) error { return nil }
func (l *Local) UpdateUser(ctx context.Context, ib *model.Inbound, oldEmail string, payload model.Client) error {
	if ib.Protocol == model.Mieru || ib.Protocol == model.Sudoku || externalvpn.IsManaged(ib.Protocol) {
		return nil // Full state is reapplied by the protocol-specific reconciler.
	}
	if l.isNative() {
		// sing-box reads the client list from the database when regenerating
		// its config. Avoid the remove+add double restart used by Xray's API.
		return l.applyCoreChange(ctx)
	}
	if oldEmail != "" {
		if err := l.RemoveUser(ctx, ib, oldEmail); err != nil && !strings.Contains(err.Error(), "not found") {
			return err
		}
	}
	if !payload.Enable {
		return nil
	}
	return l.AddUser(ctx, ib, map[string]any{"email": payload.Email, "id": payload.ID, "security": payload.Security, "flow": payload.Flow, "auth": payload.Auth, "password": payload.Password, "publicKey": payload.PublicKey, "allowedIPs": payload.AllowedIPs, "preSharedKey": payload.PreSharedKey, "keepAlive": wgKeepAlive(payload.KeepAliveSeconds())})
}

func wgKeepAlive(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	return strconv.Itoa(seconds)
}

func (l *Local) RestartXray(ctx context.Context) error {
	return l.applyCoreChange(ctx)
}

func (l *Local) ResetClientTraffic(_ context.Context, _ *model.Inbound, _ string) error { return nil }

func (l *Local) ResetAllTraffics(_ context.Context) error { return nil }

func (l *Local) ResetInboundTraffic(_ context.Context, _ *model.Inbound) error { return nil }
