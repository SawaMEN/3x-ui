package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// A slow node launch is bounded but can take ~25 seconds. The main HTTP
// server has a 30-second write deadline, so oversized batches must fail
// immediately instead of launching some updates and losing the HTTP response.
func TestNodeUpdatePanelRejectsOversizedBatch(t *testing.T) {
	newHostTestDB(t)
	a := &NodeController{}
	engine := gin.New()
	engine.POST("/panel/api/nodes/updatePanel", a.updatePanel)

	var ids []string
	for id := 1; id <= 33; id++ {
		ids = append(ids, strconv.Itoa(id))
	}
	body := `{"ids":[` + strings.Join(ids, ",") + `],"dev":false}`
	req := httptest.NewRequest(http.MethodPost, "/panel/api/nodes/updatePanel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status %d; body %s", w.Code, w.Body.String())
	}
	var result struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Success || !strings.Contains(w.Body.String(), "at most 32 nodes") {
		t.Fatalf("oversized update batch was not rejected: %s", w.Body.String())
	}
}
