package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSubscriptionDomainAliasesDoNotAllowPanelHost(t *testing.T) {
	initSubDB(t)
	for key, value := range map[string]string{"subPublicURL": "https://cdn.example.com", "subAllowedDomains": "extra.example.com"} {
		if err := database.GetDB().Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer()
	router := gin.New()
	router.Use(s.subscriptionDomainValidator("origin.example.com"))
	router.NoRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		host   string
		status int
	}{
		{"origin.example.com", 204},
		{"cdn.example.com", 204},
		{"extra.example.com", 204},
		{"CDN.EXAMPLE.COM.:443", 204},
		{"panel.example.com", 403},
		{"attacker.example.com", 403},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/sub/token", nil))
		if response.Code != tc.status {
			t.Fatalf("%s status = %d, want %d", tc.host, response.Code, tc.status)
		}
	}
}

func TestRestrictedSubscriptionRejectsListenerHost(t *testing.T) {
	initSubDB(t)
	for key, value := range map[string]string{
		"subPublicURL":      "https://first.example.com:8443",
		"subAllowedDomains": "second.example.com", "subRestrictDomains": "true",
	} {
		if err := database.GetDB().Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer()
	router := gin.New()
	router.Use(s.subscriptionDomainValidator("origin.example.com"))
	router.NoRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		host   string
		status int
	}{
		{"first.example.com:8443", 204},
		{"second.example.com:8443", 204},
		{"origin.example.com", 403},
		{"panel.example.com", 403},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/sub/token", nil))
		if response.Code != tc.status {
			t.Fatalf("%s = %d, want %d", tc.host, response.Code, tc.status)
		}
	}
}
