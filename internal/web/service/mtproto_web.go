package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
)

type mtprotoWebSettings struct {
	SharedIngress  *TelemtWebIngress     `json:"sharedIngress,omitempty"`
	Site           *TelemtWebSiteOptions `json:"site,omitempty"`
	Enabled        bool                  `json:"enabled"`
	MigrationDraft bool                  `json:"migrationDraft,omitempty"`
	Domain         string                `json:"domain"`
	BasePath       string                `json:"basePath"`
	Aliases        []string              `json:"aliases"`
	Frontend       string                `json:"frontend"`
	Carrier        string                `json:"carrier"`
	Carriers       []string              `json:"carriers"`
	RetryJitter    *TelemtWebRetryJitter `json:"retryJitter,omitempty"`
}

func telemtWebRetryJitterEqual(a, b *TelemtWebRetryJitter) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func mtprotoWebForInbound(ib *model.Inbound) (mtprotoWebSettings, error) {
	if ib == nil || ib.Protocol != model.MTProto {
		return mtprotoWebSettings{}, nil
	}
	var settings struct {
		WebProxy mtprotoWebSettings `json:"webProxy"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		return mtprotoWebSettings{}, err
	}
	return settings.WebProxy, nil
}

func mtprotoWebCandidates() ([]*model.Inbound, error) {
	return mtprotoWebCandidatesForNode(nil)
}

func mtprotoWebCandidatesForNode(nodeID *int) ([]*model.Inbound, error) {
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("mtproto web: database is unavailable")
	}
	var inbounds []*model.Inbound
	query := db.Where("protocol = ? AND enable = ?", model.MTProto, true)
	if nodeID == nil {
		query = query.Where("node_id IS NULL")
	} else {
		query = query.Where("node_id = ?", *nodeID)
	}
	if err := query.Find(&inbounds).Error; err != nil {
		return nil, err
	}
	selected := make([]*model.Inbound, 0, 1)
	for _, ib := range inbounds {
		opts, err := mtprotoWebForInbound(ib)
		if err != nil {
			return nil, err
		}
		if opts.Enabled {
			selected = append(selected, ib)
		}
	}
	return selected, nil
}

// ValidateMtprotoWebInbound ensures only one local HTTPS frontend owns port 443.
// This is checked before database mutations as well as by periodic reconciliation.
func ValidateMtprotoWebInbound(ib *model.Inbound) error {
	if ib == nil || ib.Protocol != model.MTProto {
		return nil
	}
	if err := mtproto.ValidateUniqueSecrets(ib); err != nil {
		return err
	}
	opts, err := mtprotoWebForInbound(ib)
	if err != nil || !opts.Enabled {
		return err
	}
	if opts.MigrationDraft {
		if ib.NodeID != nil || ib.Enable || opts.SharedIngress == nil || !opts.SharedIngress.Enabled {
			return errors.New("MTProto migration draft must be a disabled local WEB inbound with shared ingress")
		}
	} else if !ib.Enable {
		return nil
	}
	domain := normalizeTelemtWebDomain(opts.Domain)
	if domain == "" && ib.NodeID == nil {
		domain, err = (&SettingService{}).GetWebDomain()
		if err != nil {
			return err
		}
		domain = normalizeTelemtWebDomain(domain)
	}
	if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 {
		return errors.New("MTProto WEB Proxy requires a valid public domain")
	}
	if _, err := normalizeTelemtWebAliases(domain, opts.Aliases); err != nil {
		return err
	}
	normalizedSettings, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{SharedIngress: opts.SharedIngress, Frontend: opts.Frontend, Carrier: opts.Carrier, Carriers: opts.Carriers, Site: opts.Site, BasePath: &opts.BasePath, Aliases: &opts.Aliases, RetryJitter: opts.RetryJitter})
	if err != nil {
		return err
	}
	// Persist the effective endpoint so clients, API and subscriptions agree.
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(ib.Settings), &payload); err != nil {
		return err
	}
	var webPayload map[string]json.RawMessage
	if err := json.Unmarshal(payload["webProxy"], &webPayload); err != nil {
		return err
	}
	webPayload["domain"], err = json.Marshal(domain)
	if err != nil {
		return err
	}
	webPayload["basePath"], err = json.Marshal(*normalizedSettings.BasePath)
	if err != nil {
		return err
	}
	webPayload["aliases"], err = json.Marshal(*normalizedSettings.Aliases)
	if err != nil {
		return err
	}
	// Canonicalise required options for inbounds created by old UIs or API
	// clients. Otherwise the WEB form reopens with empty selectors.
	for name, value := range map[string]any{
		"frontend": normalizedSettings.Frontend,
		"carrier":  normalizedSettings.Carrier,
		"carriers": normalizedSettings.Carriers,
	} {
		webPayload[name], err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	if normalizedSettings.SharedIngress == nil {
		delete(webPayload, "sharedIngress")
	} else {
		webPayload["sharedIngress"], err = json.Marshal(normalizedSettings.SharedIngress)
		if err != nil {
			return err
		}
	}
	payload["webProxy"], err = json.Marshal(webPayload)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ib.Settings = string(encoded)
	ib.Port = 443
	ib.Listen = ""
	if opts.MigrationDraft {
		return nil
	}
	others, err := mtprotoWebCandidatesForNode(ib.NodeID)
	if err != nil {
		return err
	}
	for _, other := range others {
		if other.Id != ib.Id {
			return fmt.Errorf("MTProto WEB Proxy already belongs to inbound %d; only one HTTPS frontend per server is supported", other.Id)
		}
	}
	return nil
}

var (
	mtprotoWebSync       = make(chan struct{}, 1)
	mtprotoWebLastError  time.Time
	mtprotoWebFailedID   int
	mtprotoWebFailedCfg  [sha256.Size]byte
	mtprotoWebApplyError error
)

// ReconcileMtprotoWebProxy derives service state from MTProto inbounds.
// Ordinary MTProto sidecars remain managed independently by internal/mtproto.
// Legacy installations with an unowned WEB Proxy are never changed implicitly.
func ReconcileMtprotoWebProxy(ctx context.Context) error {
	return reconcileMtprotoWebProxy(ctx, readTelemtWebState, (TelemtService{}).GetConfig)
}

func reconcileMtprotoWebProxy(ctx context.Context, readState func() (TelemtWebProxyState, error), getConfig func() (TelemtConfig, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mtprotoWebSync <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-mtprotoWebSync }()
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	selected, err := mtprotoWebCandidates()
	if err != nil {
		return err
	}
	if len(selected) > 1 {
		return fmt.Errorf("mtproto web: %d enabled WEB inbounds; only one is supported", len(selected))
	}
	if len(selected) == 1 {
		selected[0], err = (&InboundService{}).buildInboundForLocalRuntime(database.GetDB(), selected[0])
		if err != nil {
			return err
		}
	}
	state, err := readState()
	if err != nil {
		return err
	}

	if len(selected) == 0 || !mtprotoWebHasUsers(selected[0]) {
		if journal, err := loadTelemtVPNMigrationForEnable(); err != nil {
			return err
		} else if journal != nil && journal.ManagedInboundID > 0 {
			if state.ManagedInboundID != 0 && state.ManagedInboundID != journal.ManagedInboundID {
				return errors.New("managed WEB migration owner mismatch")
			}
			if err := (TelemtService{}).restoreVPNMigrationLocked(); err != nil {
				return err
			}
			return stopMtprotoWebServiceIfUnused()
		}
		if state.ManagedInboundID == 0 {
			if err := refreshTelemtIngress(state); err != nil {
				return err
			}
			return telemtWebUpgradeFrontend(ctx, state)
		}
		if err := disableTelemtWebProxy(); err != nil {
			return err
		}
		if err := cleanupWebProxyBootUnit(); err != nil {
			return err
		}
		return stopMtprotoWebServiceIfUnused()
	}

	ib := selected[0]
	opts, err := mtprotoWebForInbound(ib)
	if err != nil {
		return err
	}
	domain := normalizeTelemtWebDomain(opts.Domain)
	if opts.Site == nil {
		opts.Site = &TelemtWebSiteOptions{Mode: "auto"}
	}
	normalized, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{SharedIngress: opts.SharedIngress, Frontend: opts.Frontend, Carrier: opts.Carrier, Carriers: opts.Carriers, Site: opts.Site, BasePath: &opts.BasePath, Aliases: &opts.Aliases, RetryJitter: opts.RetryJitter})
	if err != nil {
		return err
	}
	if state.Enabled && state.ManagedInboundID == 0 {
		return errors.New("mtproto web: legacy WEB Proxy is active; migrate or disable it before creating a managed WEB inbound")
	}
	if state.Enabled && state.ManagedInboundID != 0 && state.ManagedInboundID != ib.Id {
		// Release the old owner before applying the replacement.
		if err := disableTelemtWebProxy(); err != nil {
			return err
		}
		if err := cleanupWebProxyBootUnit(); err != nil {
			return err
		}
		state, err = readState()
		if err != nil {
			return err
		}
	}

	fingerprint := mtprotoWebFingerprint(ib)
	unchanged := state.Enabled && state.ManagedInboundID == ib.Id &&
		state.Domain == domain && state.BasePath == *normalized.BasePath && strings.Join(state.Aliases, ",") == strings.Join(*normalized.Aliases, ",") && telemtWebFrontend(state) == normalized.Frontend &&
		telemtWebCarrier(state) == normalized.Carrier &&
		strings.Join(state.Carriers, ",") == strings.Join(normalized.Carriers, ",") &&
		telemtWebSitesEqual(state.Site, normalized.Site) &&
		telemtWebRetryJitterEqual(state.RetryJitter, normalized.RetryJitter) &&
		telemtWebIngressEqual(state.SharedIngress, normalized.SharedIngress)
	// An older imported snapshot may still contain CSS/JS blocked by Telemt's
	// CSP. Reconcile it once into a new immutable snapshot after an upgrade.
	if unchanged && normalized.Site != nil && normalized.Site.Mode == "html" {
		unchanged = state.DecoyDir == telemtWebImportedSnapshotPath(telemtWebDecoyDir+"-sites", normalized.Site.HTML)
	}
	svc := TelemtService{}
	if unchanged {
		if err := refreshTelemtIngress(state); err != nil {
			return err
		}
	}
	if unchanged && state.UserFingerprint == fingerprint && svc.WebProxyTelemtActive() {
		if err := telemtWebUpgradeFrontend(ctx, state); err != nil {
			return err
		}
		if err := refreshMtprotoWebTrafficBinding(ctx); err != nil {
			return err
		}
		// Recover the HTTPS entry point even when only its process was stopped.
		if !telemtWebFrontendActive(state) {
			if err := telemtWebEnsureFrontendRunning(state); err != nil {
				return err
			}
		}
		// An active process can still return 502/404 or present the wrong TLS
		// virtual host. Do not report a healthy reconciliation in that case.
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return telemtWebWaitFrontendReady(probeCtx, state)
	}
	if unchanged {
		cfg, cfgErr := getConfig()
		if cfgErr != nil {
			return cfgErr
		}
		if saveErr := svc.saveWebUsersHotReloadLocked(ctx, cfg); saveErr != nil {
			return saveErr
		}
		state.UserFingerprint = fingerprint
		if err := writeTelemtWebState(state); err != nil {
			return err
		}
		if err := svc.EnsureWebProxyBackend(); err != nil {
			return err
		}
		if err := telemtWebUpgradeFrontend(ctx, state); err != nil {
			return err
		}
		if !telemtWebFrontendActive(state) {
			if err := telemtWebEnsureFrontendRunning(state); err != nil {
				return err
			}
		}
		return telemtWebWaitFrontendReady(ctx, state)
	}
	// Avoid repeatedly attempting certificate issuance on every periodic job
	// when DNS, ports, or certificate preconditions cannot be satisfied.
	desired := sha256.Sum256([]byte(ib.Settings))
	if mtprotoWebFailedID == ib.Id && mtprotoWebFailedCfg == desired && time.Since(mtprotoWebLastError) < time.Minute {
		return mtprotoWebApplyError
	}
	settings := SettingService{}
	cert, _ := settings.GetCertFile()
	key, _ := settings.GetKeyFile()
	previousState := state
	state.ManagedInboundID = ib.Id
	if state.BackendPort == 0 {
		port, portErr := mtproto.FreeLocalPort()
		if portErr != nil {
			return portErr
		}
		state.BackendPort = port
	}
	state.UserFingerprint = fingerprint
	if err := writeTelemtWebState(state); err != nil {
		return err
	}
	// An inbound is a complete desired configuration, unlike the partial
	// standalone enable API. Omission must clear previously enabled ingress.
	if normalized.SharedIngress == nil {
		normalized.SharedIngress = &TelemtWebIngress{Enabled: false}
	}
	if _, err := svc.enableWebProxyLocked(ctx, domain, cert, key, normalized); err != nil {
		mtprotoWebLastError, mtprotoWebFailedID, mtprotoWebFailedCfg = time.Now(), ib.Id, desired
		mtprotoWebApplyError = withTelemtRollbackError(err, writeTelemtWebState(previousState))
		return mtprotoWebApplyError
	}
	mtprotoWebLastError = time.Time{}
	mtprotoWebApplyError = nil
	// enableWebProxyLocked already persisted the managed inbound, rendered
	// its users, started the backend and verified HTTPS. Saving again would
	// restart the just-verified backend and invalidate that readiness check.
	return nil
}

// Stop the standalone WEB backend only when it has no independent/legacy
// credentials. Old proxy and subscription users must remain reachable.
func stopMtprotoWebServiceIfUnused() error {
	data, err := os.ReadFile(telemtConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var cfg struct {
		Access struct {
			Users map[string]string `toml:"users"`
		} `toml:"access"`
	}
	if len(data) != 0 {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return err
		}
	}
	for name := range cfg.Access.Users {
		if name != "xui" && name != telemtWebUser && !strings.HasPrefix(name, "mtweb_") {
			return nil
		}
	}
	if systemctl("is-active", "--quiet", telemtServiceName) == nil {
		if err := systemctl("stop", telemtServiceName); err != nil {
			return err
		}
	}
	if systemctl("is-enabled", "--quiet", telemtServiceName) == nil {
		return systemctl("disable", telemtServiceName)
	}
	return nil
}

func mtprotoWebHasUsers(ib *model.Inbound) bool {
	inst, ok := mtproto.InstanceFromInbound(ib)
	return ok && len(inst.Secrets) > 0
}

// mtprotoWebUsername is stable across editing or reordering clients. It also
// lets the normal traffic collector resolve the corresponding panel email.
func mtprotoWebUsername(inboundID int, email string) string {
	digest := sha256.Sum256([]byte(email))
	return fmt.Sprintf("mtweb_%d_%x", inboundID, digest[:8])
}

var mtprotoWebTraffic mtprotoWebTrafficTracker

func resetMtprotoWebClientQuota(inboundID int, email string) error {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	state, err := readTelemtWebState()
	if err != nil || !state.Enabled || state.ManagedInboundID != inboundID {
		return err
	}
	username := mtprotoWebUsername(inboundID, email)
	req, err := telemtAPIRequest(http.MethodPost, "/v1/users/"+url.PathEscape(username)+"/reset-quota")
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK || !result.OK {
		return fmt.Errorf("telemt WEB quota reset rejected: HTTP %d", response.StatusCode)
	}
	return nil
}

func mtprotoWebFingerprint(ib *model.Inbound) string {
	inst, ok := mtproto.InstanceFromInbound(ib)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(inst.Secrets))
	for _, user := range inst.Secrets {
		parts = append(parts, user.Name+":"+user.Secret+":"+fmt.Sprint(user.QuotaBytes)+":"+fmt.Sprint(user.ExpiresUnix)+":"+fmt.Sprint(user.LimitIP))
	}
	slices.Sort(parts)
	fingerprint := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", fingerprint)
}

// An explicitly saved, disabled local draft cannot listen on TCP 443.
// Ordinary disabled inbounds continue to reserve their configured ports.
func mtprotoWebMigrationDraft(ib *model.Inbound) bool {
	if ib == nil || ib.Enable || ib.NodeID != nil || ib.Protocol != model.MTProto {
		return false
	}
	opts, err := mtprotoWebForInbound(ib)
	return err == nil && opts.Enabled && opts.MigrationDraft && opts.SharedIngress != nil && opts.SharedIngress.Enabled
}
