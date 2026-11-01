package session

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupSessionTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("XUI_DB_JOURNAL_MODE", "")
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestLegacySessionMigrationFailsClosed(t *testing.T) {
	setupSessionTestDB(t)
	user := &model.User{Username: "migration-test", Password: "hash"}
	if err := database.GetDB().Create(user).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(sessions.Sessions(sessionCookieName, cookie.NewStore([]byte("01234567890123456789012345678901"))))
	router.GET("/legacy", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set(loginUserKey, user.Id)
		s.Set(loginEpochKey, user.LoginEpoch)
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
	})
	router.GET("/protected", func(c *gin.Context) {
		if GetLoginUser(c) != nil {
			c.Status(http.StatusNoContent)
			return
		}
		c.Status(http.StatusUnauthorized)
	})
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/legacy", nil))
	cb := database.GetDB().Callback().Create()
	if err := cb.Before("gorm:create").Register("test:session-write-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			tx.AddError(errors.New("session store unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cb.Remove("test:session-write-failure") })
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range login.Result().Cookies() {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when session registration fails", response.Code)
	}
}

func TestSetLoginUserStoresOnlyUserID(t *testing.T) {
	setupSessionTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions(sessionCookieName, cookie.NewStore([]byte("01234567890123456789012345678901"))))
	router.GET("/", func(c *gin.Context) {
		if err := SetLoginUser(c, &model.User{Id: 7, Username: "admin", Password: "hash"}); err != nil {
			t.Fatal(err)
		}
		got := sessions.Default(c).Get(loginUserKey)
		if got != 7 {
			t.Fatalf("stored session payload = %#v, want user id only", got)
		}
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestSessionUserIDSupportsLegacyUserPayload(t *testing.T) {
	id, ok := sessionUserID(model.User{Id: 11, Username: "admin", Password: "hash"})
	if !ok || id != 11 {
		t.Fatalf("legacy session payload resolved to (%d, %v), want (11, true)", id, ok)
	}
	id, ok = sessionUserID(&model.User{Id: 12, Username: "admin", Password: "hash"})
	if !ok || id != 12 {
		t.Fatalf("legacy pointer session payload resolved to (%d, %v), want (12, true)", id, ok)
	}
}

func TestRevokedSessionIsNotAuthenticated(t *testing.T) {
	setupSessionTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions(sessionCookieName, cookie.NewStore([]byte("01234567890123456789012345678901"))))
	router.GET("/login", func(c *gin.Context) {
		user := &model.User{Id: 7, Username: "admin", Password: "hash"}
		// The lookup performed by GetLoginUser requires the user row.
		if err := database.GetDB().Create(user).Error; err != nil {
			t.Fatal(err)
		}
		if err := SetLoginUser(c, user); err != nil {
			t.Fatal(err)
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/protected", func(c *gin.Context) {
		if IsLogin(c) {
			c.Status(http.StatusNoContent)
			return
		}
		c.Status(http.StatusUnauthorized)
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	router.ServeHTTP(first, firstReq)
	cookieHeader := first.Result().Header.Get("Set-Cookie")
	if cookieHeader == "" {
		t.Fatal("login response did not set a session cookie")
	}
	cookieHeader = strings.SplitN(cookieHeader, ";", 2)[0]

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Cookie", cookieHeader)
	before := httptest.NewRecorder()
	router.ServeHTTP(before, req)
	if before.Code != http.StatusNoContent {
		t.Fatalf("protected status before revoke = %d", before.Code)
	}

	var row model.UserSession
	if err := database.GetDB().First(&row).Error; err != nil {
		t.Fatalf("load session row: %v", err)
	}
	if err := RevokeUserSession(7, row.Id); err != nil {
		t.Fatalf("revoke session: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Cookie", cookieHeader)
	after := httptest.NewRecorder()
	router.ServeHTTP(after, req2)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("protected status after revoke = %d, want 401", after.Code)
	}
}
