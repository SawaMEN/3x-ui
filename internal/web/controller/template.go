package controller

import (
	"net/http"
	"strconv"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
	"github.com/gin-gonic/gin"
)

type TemplateController struct {
	service service.TemplateService
}

func NewTemplateController(g *gin.RouterGroup) *TemplateController {
	a := &TemplateController{}
	g.GET("/list", a.list)
	g.GET("/get/:id", a.get)
	g.POST("/sanitize", a.sanitize)
	g.POST("/save", a.save)
	g.POST("/del/:id", a.del)
	return a
}

func (a *TemplateController) list(c *gin.Context) {
	_ = session.GetLoginUser(c)
	limit := 20
	offset := 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}
	items, total, err := a.service.List(c.Query("kind"), c.Query("q"), limit, offset)
	if err != nil {
		jsonMsg(c, "failed to list templates", err)
		return
	}
	jsonObj(c, gin.H{"items": items, "total": total}, nil)
}

func (a *TemplateController) get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "invalid template id", err)
		return
	}
	item, err := a.service.Get(id)
	if err != nil {
		jsonMsg(c, "failed to get template", err)
		return
	}
	jsonObj(c, item, nil)
}

func (a *TemplateController) sanitize(c *gin.Context) {
	var in service.TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, "invalid template payload", err)
		return
	}
	result, err := a.service.Sanitize(in)
	if err != nil {
		jsonMsg(c, "failed to sanitize template", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *TemplateController) save(c *gin.Context) {
	var in service.TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, "invalid template payload", err)
		return
	}
	item, warnings, err := a.service.Save(in)
	if err != nil {
		jsonMsg(c, "failed to save template", err)
		return
	}
	jsonObj(c, gin.H{"template": item, "warnings": warnings}, nil)
}

func (a *TemplateController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "invalid template id", err)
		return
	}
	if err := a.service.Delete(id); err != nil {
		jsonMsg(c, "failed to delete template", err)
		return
	}
	jsonMsg(c, "template deleted", nil)
}

var _ = http.StatusOK
