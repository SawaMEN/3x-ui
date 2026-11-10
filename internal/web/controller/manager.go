package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
)

type ManagerController struct{ service service.ManagerService }

func NewManagerController(g *gin.RouterGroup) *ManagerController {
	a := &ManagerController{}
	g = g.Group("/manager")
	g.GET("/state", a.state)
	g.GET("/subscriptions/:id", a.detail)
	g.POST("/subscriptions", a.save)
	g.POST("/subscriptions/bulk-shared", a.bulkShared)
	g.POST("/preflight", a.preflight)
	g.POST("/subscriptions/:id/rotate", a.rotate)
	g.POST("/subscriptions/:id/delete", a.remove)
	g.POST("/rotate", a.rotateAll)
	g.POST("/operations/:id/retry", a.retry)
	g.POST("/domains", a.domains)
	g.POST("/domains/cover-site", a.coverSite)
	g.POST("/relays", a.relay)
	g.POST("/relays/:id/install", a.installRelay)
	g.POST("/relays/:id/uninstall", a.uninstallRelay)
	g.POST("/relays/:id/delete", a.deleteRelay)
	return a
}

func (a *ManagerController) state(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	subPage, _ := strconv.Atoi(c.Query("subscriptionPage"))
	v, err := a.service.Snapshot(session.GetLoginUser(c).Id, page, c.Query("state"), c.Query("kind"), service.ManagerListOptions{Page: subPage, Search: c.Query("search"), Compact: true})
	jsonObj(c, v, err)
}

func managerBind(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := c.ShouldBindJSON(v); err != nil {
		c.JSON(400, gin.H{"success": false, "msg": err.Error()})
		return false
	}
	return true
}

func managerID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		c.JSON(400, gin.H{"success": false, "msg": "invalid ID"})
		return 0, false
	}
	return id, true
}

func (a *ManagerController) save(c *gin.Context) {
	var in model.ManagerSubscription
	if !managerBind(c, &in) {
		return
	}
	v, err := a.service.Save(session.GetLoginUser(c).Id, in)
	jsonObj(c, v, err)
}

func (a *ManagerController) bulkShared(c *gin.Context) {
	var input service.ManagerBulkSharedInput
	if !managerBind(c, &input) {
		return
	}
	result, err := a.service.BulkCreateShared(session.GetLoginUser(c).Id, input)
	jsonObj(c, result, err)
}

func (a *ManagerController) queue(c *gin.Context, kind string) {
	id, ok := managerID(c)
	if !ok {
		return
	}
	v, err := a.service.Enqueue(session.GetLoginUser(c).Id, id, kind)
	jsonObj(c, v, err)
}
func (a *ManagerController) rotate(c *gin.Context) { a.queue(c, "rotate") }
func (a *ManagerController) remove(c *gin.Context) { a.queue(c, "delete") }
func (a *ManagerController) rotateAll(c *gin.Context) {
	jsonObj(c, nil, a.service.EnqueueAll(session.GetLoginUser(c).Id))
}

func (a *ManagerController) retry(c *gin.Context) {
	id, ok := managerID(c)
	if !ok {
		return
	}
	v, err := a.service.Retry(session.GetLoginUser(c).Id, id)
	jsonObj(c, v, err)
}

func (a *ManagerController) domains(c *gin.Context) {
	var in struct {
		Domains string `json:"domains"`
		Replace bool   `json:"replace"`
	}
	if !managerBind(c, &in) {
		return
	}
	jsonObj(c, nil, a.service.ImportDomains(session.GetLoginUser(c).Id, in.Domains, in.Replace))
}

func (a *ManagerController) coverSite(c *gin.Context) {
	var input service.ManagerCoverSiteInput
	if !managerBind(c, &input) {
		return
	}
	result, err := a.service.ProvisionCoverSite(c.Request.Context(), session.GetLoginUser(c).Id, input)
	jsonObj(c, result, err)
}

func (a *ManagerController) relay(c *gin.Context) {
	var in service.ManagerRelayInput
	if !managerBind(c, &in) {
		return
	}
	v, err := a.service.SaveRelay(session.GetLoginUser(c).Id, in)
	jsonObj(c, v, err)
}

func (a *ManagerController) relayQueue(c *gin.Context, kind string) {
	id, ok := managerID(c)
	if !ok {
		return
	}
	v, err := a.service.EnqueueRelay(session.GetLoginUser(c).Id, id, kind)
	jsonObj(c, v, err)
}
func (a *ManagerController) installRelay(c *gin.Context)   { a.relayQueue(c, "relay-install") }
func (a *ManagerController) uninstallRelay(c *gin.Context) { a.relayQueue(c, "relay-uninstall") }
func (a *ManagerController) deleteRelay(c *gin.Context)    { a.relayQueue(c, "relay-delete") }
func ServeManagerSubscription(c *gin.Context) {
	service.ServeManagerSubscription(c.Writer, c.Request, c.Param("token"))
}

func (a *ManagerController) preflight(c *gin.Context) {
	var in model.ManagerSubscription
	if !managerBind(c, &in) {
		return
	}
	jsonObj(c, a.service.Preflight(c.Request.Context(), session.GetLoginUser(c).Id, in), nil)
}

func (a *ManagerController) detail(c *gin.Context) {
	id, ok := managerID(c)
	if !ok {
		return
	}
	var sub model.ManagerSubscription
	err := database.GetDB().Where("id = ? AND user_id = ?", id, session.GetLoginUser(c).Id).First(&sub).Error
	jsonObj(c, sub, err)
}
