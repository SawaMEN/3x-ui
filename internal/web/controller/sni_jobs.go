package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

func init() {
	nodeSyncScopeAllow["/server/sni/jobs"] = map[string]struct{}{http.MethodPost: {}}
	nodeSyncScopeAllow["/server/sni/jobs/:id"] = map[string]struct{}{http.MethodGet: {}, http.MethodDelete: {}}
	nodeSyncScopeAllow["/server/sni/history"] = map[string]struct{}{http.MethodGet: {}}
}
func (a *ServerController) initSNIJobRouter(g *gin.RouterGroup) {
	g.POST("/sni/jobs", a.startSNIScan)
	g.GET("/sni/jobs/:id", a.sniScanStatus)
	g.DELETE("/sni/jobs/:id", a.sniScanStatus)
	g.GET("/sni/history", a.sniScanHistory)
}
func normalizeSNINodeTarget(c *gin.Context, id *int) (*int, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	if *id < 0 {
		return nil, errors.New("nodeId must be positive")
	}
	if scope, ok := c.Get("api_token_scope"); ok && scope == model.ApiScopeNodeSync {
		return nil, errors.New("node-sync SNI requests may target only the local server")
	}
	return id, nil
}
func sniNodeQuery(c *gin.Context) (*int, error) {
	raw := c.Query("nodeId")
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil, errors.New("invalid nodeId")
	}
	return normalizeSNINodeTarget(c, &id)
}
func forwardSNIJob(c *gin.Context, id *int, resource string, body any) bool {
	if id == nil {
		return false
	}
	manager := webruntime.GetManager()
	if manager == nil {
		jsonObj(c, nil, errors.New("runtime manager not initialized"))
		return true
	}
	runtime, err := manager.RuntimeFor(id)
	if err != nil {
		jsonObj(c, nil, err)
		return true
	}
	remote, ok := runtime.(webruntime.SNIJobRuntime)
	if !ok {
		jsonObj(c, nil, errors.New("selected node does not support SNI scan jobs; update the node"))
		return true
	}
	result, err := remote.RequestSNIScan(c.Request.Context(), c.Request.Method, resource, body)
	jsonObj(c, result, err)
	return true
}
func (a *ServerController) startSNIScan(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var request service.SNIScanRequest
	if err := c.ShouldBind(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	id, err := normalizeSNINodeTarget(c, request.NodeID)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	request.NodeID = nil // never forward a master-local node id to another panel
	if forwardSNIJob(c, id, "jobs", request) {
		return
	}
	result, err := service.StartSNIScan(request)
	jsonObj(c, result, err)
}
func (a *ServerController) sniScanStatus(c *gin.Context) {
	id, err := sniNodeQuery(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if forwardSNIJob(c, id, "jobs/"+c.Param("id"), nil) {
		return
	}
	result, err := service.SNIScanStatus(c.Param("id"), c.Request.Method == http.MethodDelete)
	jsonObj(c, result, err)
}
func (a *ServerController) sniScanHistory(c *gin.Context) {
	id, err := sniNodeQuery(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if forwardSNIJob(c, id, "history", nil) {
		return
	}
	result, err := service.SNIScanHistory()
	jsonObj(c, result, err)
}
