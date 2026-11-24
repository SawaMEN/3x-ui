package controller

import (
	"net/http"
	"sync"
	"time"

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
		Port int    `json:"port"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonObj(c, nil, err)
		return
	}
	report, err := antidpi.Probe(c.Request.Context(), payload.Host, payload.Port)
	jsonObj(c, report, err)
}

var lastSiberianCheck time.Time

func (a *SettingController) antiDpiSiberian(c *gin.Context) {
	if !antiDpiProbeMu.TryLock() {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "msg": "A diagnostic is already running"})
		return
	}
	defer antiDpiProbeMu.Unlock()
	if time.Since(lastSiberianCheck) < time.Minute {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "msg": "Wait one minute between active diagnostics"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var payload struct {
		antidpi.SiberianOptions
		Confirmed bool `json:"confirmed"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonObj(c, nil, err)
		return
	}
	if !payload.Confirmed {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "Explicit confirmation is required"})
		return
	}
	lastSiberianCheck = time.Now()
	report, err := antidpi.CheckSiberian(c.Request.Context(), payload.SiberianOptions)
	jsonObj(c, report, err)
}
