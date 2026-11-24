package controller

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/antidpi"
)

var antiDpiProbeMu sync.Mutex

func (a *SettingController) antiDpiProbe(c *gin.Context) {
	if !antiDpiProbeMu.TryLock() {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "msg": "A diagnostic is already running"})
		return
	}
	defer antiDpiProbeMu.Unlock()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var payload struct {
		Host string `json:"host"`
		Port int `json:"port"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonObj(c, nil, err)
		return
	}
	report, err := antidpi.Probe(c.Request.Context(), payload.Host, payload.Port)
	jsonObj(c, report, err)
}
