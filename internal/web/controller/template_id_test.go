package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTemplateAndPresetRejectInvalidIDs(t *testing.T) {
	router := gin.New()
	NewTemplateController(router.Group("/templates"))
	NewRoutingPresetController(router.Group("/presets"))
	for _, group := range []string{"templates", "presets"} {
		for _, action := range []string{"get", "del"} {
			for _, id := range []string{"0", "-1", "invalid"} {
				path := "/" + group + "/" + action + "/" + id
				t.Run(path, func(t *testing.T) {
					method := http.MethodGet
					if action == "del" {
						method = http.MethodPost
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
					var result struct {
						Success bool `json:"success"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if result.Success {
						t.Fatalf("invalid ID reported success: %s", response.Body.String())
					}
				})
			}
		}
	}
}
