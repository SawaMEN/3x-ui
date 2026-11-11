package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"

	"github.com/SawaMEN/3x-ui/v3/internal/util/uripath"
)

func TestAuditDistEscapesConfiguredBasePath(t *testing.T) {
	basePath, err := uripath.Normalize(`/prefix"/><script>alert(1)</script>/`)
	if err != nil {
		t.Fatal(err)
	}
	previous := distFS
	SetDistFS(fstest.MapFS{"dist/login.html": {Data: []byte(`<html><head><script src="/assets/app.js"></script><link href="/assets/app.css"></head><body></body></html>`)}})
	t.Cleanup(func() { SetDistFS(previous) })
	router := gin.New()
	router.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("audit-dist-cookie-secret"))))
	router.GET("/", func(c *gin.Context) {
		c.Set("base_path", basePath)
		serveDistPage(c, "login.html")
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	doc, err := html.Parse(strings.NewReader(w.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	var scripts int
	var assets int
	var injectedBase string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "script" {
				scripts++
				if n.FirstChild != nil && strings.HasPrefix(n.FirstChild.Data, `window.X_UI_BASE_PATH=`) {
					raw := strings.TrimSuffix(strings.TrimPrefix(n.FirstChild.Data, `window.X_UI_BASE_PATH=`), ";")
					if err := json.Unmarshal([]byte(raw), &injectedBase); err != nil {
						t.Errorf("invalid injected JavaScript string: %v", err)
					}
				}
			}
			for _, attr := range n.Attr {
				if (attr.Key == "src" && attr.Val == basePath+"assets/app.js") || (attr.Key == "href" && attr.Val == basePath+"assets/app.css") {
					assets++
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	if scripts != 3 || assets != 2 || injectedBase != basePath {
		t.Fatalf("scripts=%d (want 3), intact assets=%d (want 2), injected base=%q (want %q)", scripts, assets, injectedBase, basePath)
	}
}
