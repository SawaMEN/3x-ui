package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type RoutingCatalogController struct{ service service.RoutingCatalogService }

func NewRoutingCatalogController(g *gin.RouterGroup) {
	a := &RoutingCatalogController{}
	g.GET("/categories", func(c *gin.Context) { jsonObj(c, a.service.Categories(), nil) })
	g.GET("/list", func(c *gin.Context) { rows, err := a.service.List(); jsonObj(c, rows, err) })
	g.GET("/:id/entries", a.entries)
	g.POST("/save", a.save)
	g.POST("/:id/refresh", a.refresh)
	g.POST("/:id/delete", a.delete)
}

func routingCatalogID(c *gin.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, errors.New("invalid routing list id")
	}
	return id, nil
}

func (a *RoutingCatalogController) entries(c *gin.Context) {
	id, err := routingCatalogID(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	entries, err := a.service.Entries(id)
	jsonObj(c, entries, err)
}

func (a *RoutingCatalogController) save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var in service.RoutingListInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonObj(c, nil, err)
		return
	}
	row, err := a.service.Save(c.Request.Context(), in)
	if err == nil && in.Id > 0 {
		service.MarkRoutingCatalogChanged()
	}
	jsonObj(c, row, err)
}

func (a *RoutingCatalogController) refresh(c *gin.Context) {
	id, err := routingCatalogID(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	changed, err := a.service.Refresh(c.Request.Context(), id)
	if changed {
		service.MarkRoutingCatalogChanged()
	}
	jsonObj(c, changed, err)
}

func (a *RoutingCatalogController) delete(c *gin.Context) {
	id, err := routingCatalogID(c)
	if err == nil {
		err = a.service.Delete(id)
	}
	jsonObj(c, nil, err)
}
