package controller

import (
	"errors"
	"strconv"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
	"github.com/gin-gonic/gin"
)

type ProxyPresetController struct {
	service service.ProxyPresetService
}

func NewProxyPresetController(g *gin.RouterGroup) *ProxyPresetController {
	a := &ProxyPresetController{}
	g.GET("/list", a.list)
	g.GET("/get/:id", a.get)
	g.POST("/save", a.save)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	g.GET("/assignment/:groupId", a.assignment)
	g.POST("/assign/:groupId", a.assign)
	g.POST("/unassign/:groupId", a.unassign)
	return a
}

func (a *ProxyPresetController) userID(c *gin.Context) int {
	u := session.GetLoginUser(c)
	if u == nil {
		return 0
	}
	return u.Id
}

func parsePositivePresetID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "invalid preset id", errors.New("id must be a positive integer"))
		return 0, false
	}
	return id, true
}

func (a *ProxyPresetController) list(c *gin.Context) {
	items, err := a.service.List(a.userID(c))
	jsonObj(c, items, err)
}

func (a *ProxyPresetController) get(c *gin.Context) {
	id, ok := parsePositivePresetID(c)
	if !ok {
		return
	}
	item, err := a.service.Get(a.userID(c), id)
	jsonObj(c, item, err)
}

func (a *ProxyPresetController) save(c *gin.Context) {
	var in service.ProxyPresetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, "invalid proxy preset payload", err)
		return
	}
	item, err := a.service.Save(a.userID(c), in)
	jsonObj(c, item, err)
}

func (a *ProxyPresetController) update(c *gin.Context) {
	id, ok := parsePositivePresetID(c)
	if !ok {
		return
	}
	var in service.ProxyPresetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, "invalid proxy preset payload", err)
		return
	}
	item, err := a.service.Update(a.userID(c), id, in)
	jsonObj(c, item, err)
}

func (a *ProxyPresetController) del(c *gin.Context) {
	id, ok := parsePositivePresetID(c)
	if !ok {
		return
	}
	err := a.service.Delete(a.userID(c), id)
	jsonMsg(c, "proxy preset deleted", err)
}

func (a *ProxyPresetController) assignment(c *gin.Context) {
	item, err := a.service.Assignment(a.userID(c), c.Param("groupId"))
	jsonObj(c, item, err)
}

func (a *ProxyPresetController) assign(c *gin.Context) {
	var body struct {
		PresetId int `json:"presetId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "invalid proxy preset assignment", err)
		return
	}
	if body.PresetId <= 0 {
		jsonMsg(c, "invalid proxy preset assignment", errors.New("presetId must be a positive integer"))
		return
	}
	item, err := a.service.Assign(a.userID(c), c.Param("groupId"), body.PresetId)
	jsonObj(c, item, err)
}

func (a *ProxyPresetController) unassign(c *gin.Context) {
	err := a.service.Unassign(a.userID(c), c.Param("groupId"))
	jsonMsg(c, "proxy preset unassigned", err)
}
