package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestImportSingBoxLinkHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/import", (&SettingController{}).importSingBoxLink)
	for _, tc := range []struct {
		body    string
		success bool
	}{
		{`{"link":"mierus://user:p%40ss@proxy.test?profile=test&port=2012-2022&protocol=TCP"}`, true},
		{`{"link":"mieru://bad"}`, false},
		{`{}`, false},
		{`not json`, false},
	} {
		req := httptest.NewRequest(http.MethodPost, "/import", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		var result struct {
			Success bool           `json:"success"`
			Obj     map[string]any `json:"obj"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Success != tc.success {
			t.Fatalf("%s: %s", tc.body, recorder.Body.String())
		}
		if tc.success && (result.Obj["type"] != "mieru" || result.Obj["password"] != "p@ss") {
			t.Fatal("lost credentials")
		}
	}
}
