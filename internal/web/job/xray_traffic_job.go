package job

import (
	"context"
	"encoding/json"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/websocket"

	"github.com/valyala/fasthttp"
)

// XrayTrafficJob collects and processes traffic statistics from Xray, updating the database and optionally informing external APIs.
type XrayTrafficJob struct {
	settingService service.SettingService
	inboundService service.InboundService
}

// clientStatsSnapshotMaxClients caps how many client_traffics rows the job
// ships as a full websocket snapshot per poll (same spirit as the
// controller's broadcastInboundsUpdateClientLimit). Above it, a snapshot
// would blow past the hub's payload cap and be dropped wholesale, so the job
// broadcasts only this poll's active rows and the UI leans on its 5s REST
// refetch for the rest.
const clientStatsSnapshotMaxClients = 5000

// splitMovedClientTraffics keeps the rows that actually moved bytes this poll,
// alongside the active-email list and set derived from the same pass.
//
// Xray reports a row for every known email whether or not it transferred
// anything, so on a large panel nearly every delta is zero. The database writes
// and the external-API inform consume the full slice before this point; the
// WebSocket frame only feeds the dashboard's live speed column, where an absent
// row and a zero row render identically. Broadcasting just the movers keeps that
// frame from growing with the client count — at 5k clients it was carrying about
// a megabyte of zeros every five seconds.
func splitMovedClientTraffics(clientTraffics []*coremodel.ClientTraffic) ([]*coremodel.ClientTraffic, []string, map[string]bool) {
	moved := make([]*coremodel.ClientTraffic, 0, len(clientTraffics))
	emails := make([]string, 0, len(clientTraffics))
	active := make(map[string]bool, len(clientTraffics))
	for _, ct := range clientTraffics {
		if ct == nil || ct.Up+ct.Down <= 0 {
			continue
		}
		moved = append(moved, ct)
		emails = append(emails, ct.Email)
		active[ct.Email] = true
	}
	return moved, emails, active
}

const externalInformTimeout = 3 * time.Second

var externalInformClient = &fasthttp.Client{
	ReadTimeout:         externalInformTimeout,
	WriteTimeout:        externalInformTimeout,
	MaxIdleConnDuration: time.Minute,
	MaxConnsPerHost:     4,
}

// NewXrayTrafficJob creates a new traffic collection job instance.
func NewXrayTrafficJob() *XrayTrafficJob {
	return new(XrayTrafficJob)
}

// runNonXrayTraffic keeps the clients page live when Xray is not the active
// traffic source. Sing-box contributes its connection-presence signal here,
// while byte counters come from the shared client_traffics table so sidecars
// and any other collectors are included automatically.
func (j *XrayTrafficJob) runNonXrayTraffic() {
	core, coreErr := j.settingService.GetCoreType()
	if coreErr != nil {
		logger.Debug("get core type for traffic presence failed:", coreErr)
	}
	if coreErr == nil && service.IsNativeCore(core) {
		singBoxService := service.SelectedNativeCore()
		if !singBoxService.IsRunning() {
			// A selected but missing/stopped sing-box has no live connections.
			// Clear the previous snapshot instead of polling an unavailable API
			// every five seconds and leaving stale clients shown as online.
			j.inboundService.RefreshLocalOnlineClients(nil, nil)
		} else {
			trafficCtx, trafficCancel := context.WithTimeout(context.Background(), 4*time.Second)
			if err := singBoxService.PollTraffic(trafficCtx); err != nil {
				logger.Warning("collect sing-box traffic failed:", err)
			}
			trafficCancel()
			service.EnsureOnlinePresenceTracker()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			online, activeInbounds, err := singBoxService.OnlinePresence(ctx)
			cancel()
			if err != nil {
				logger.Debug("get online users from sing-box api failed:", err)
				j.inboundService.RefreshLocalOnlineClients(nil, nil)
			} else {
				emails := make([]string, 0, len(online))
				for email := range online {
					if email != "" {
						emails = append(emails, email)
					}
				}
				if len(emails) > 0 {
					if err := j.inboundService.BumpClientsLastOnline(emails); err != nil {
						logger.Warning("bump last online for sing-box clients failed:", err)
					}
				}
				j.inboundService.RefreshLocalOnlineClients(emails, activeInbounds)
			}
		}
	}

	if !websocket.HasClients() {
		service.ResetClientTrafficLiveSampler()
		return
	}

	liveTraffics, liveStats, err := j.inboundService.SampleClientTrafficDeltas()
	if err != nil {
		logger.Warning("sample unified client traffic failed:", err)
		liveTraffics = []*coremodel.ClientTraffic{}
		liveStats = nil
	}
	onlineClients := j.inboundService.GetOnlineClients()
	if onlineClients == nil {
		onlineClients = []string{}
	}
	websocket.BroadcastTraffic(map[string]any{
		"clientTraffics": liveTraffics,
		"onlineClients":  onlineClients,
		"onlineByGuid":   j.inboundService.GetOnlineClientsByGuid(),
		"activeInbounds": j.inboundService.GetActiveInboundsByGuid(),
	})
	if len(liveStats) > 0 {
		websocket.BroadcastClientStats(map[string]any{
			"snapshot": false,
			"clients":  liveStats,
		})
	}
}

// Run collects traffic statistics from Xray, updates the database, and pushes
// real-time updates over WebSocket using compact delta payloads — no REST
// fallback, scales to 10k–20k+ clients per inbound.
func (j *XrayTrafficJob) Run() {
	j.runNonXrayTraffic()
}

func (j *XrayTrafficJob) informTrafficToExternalAPI(inboundTraffics []*coremodel.Traffic, clientTraffics []*coremodel.ClientTraffic) {
	if len(inboundTraffics) == 0 && len(clientTraffics) == 0 {
		return
	}
	informURL, err := j.settingService.GetExternalTrafficInformURI()
	if err != nil {
		logger.Warning("get ExternalTrafficInformURI failed:", err)
		return
	}
	informURL, err = service.SanitizePublicHTTPURL(informURL, false)
	if err != nil {
		logger.Warning("ExternalTrafficInformURI blocked:", err)
		return
	}
	requestBody, err := json.Marshal(map[string]any{"clientTraffics": clientTraffics, "inboundTraffics": inboundTraffics})
	if err != nil {
		logger.Warning("parse client/inbound traffic failed:", err)
		return
	}
	request := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(request)
	request.Header.SetMethod("POST")
	request.Header.SetContentType("application/json; charset=UTF-8")
	request.SetBody(requestBody)
	request.SetRequestURI(informURL)
	request.Header.SetConnectionClose()
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := externalInformClient.DoTimeout(request, response, externalInformTimeout); err != nil {
		logger.Warning("POST ExternalTrafficInformURI failed:", err)
	}
}
