package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUserKey       = "LOGIN_USER"
	loginEpochKey      = "LOGIN_EPOCH"
	loginSessionKey    = "LOGIN_SESSION_ID"
	apiAuthUserKey     = "api_auth_user"
	sessionCookieName  = "3x-ui"
	sessionTouchWindow = 2 * time.Minute
)

func init() {
	gob.Register(model.User{})
}

type SessionView struct {
	Id         int    `json:"id"`
	IpAddress  string `json:"ipAddress"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  int64  `json:"createdAt"`
	LastSeenAt int64  `json:"lastSeenAt"`
	RevokedAt  int64  `json:"revokedAt,omitempty"`
	IsCurrent  bool   `json:"isCurrent"`
}

func randomSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sessionHash(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])
}

func requestUserAgent(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("User-Agent"))
}

func getSessionIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.ClientIP())
}

func ensureSessionRecord(c *gin.Context, user *model.User, s sessions.Session) (string, error) {
	raw, _ := s.Get(loginSessionKey).(string)
	if raw == "" {
		var err error
		raw, err = randomSessionID()
		if err != nil {
			return "", err
		}
		s.Set(loginSessionKey, raw)
	}

	hash := sessionHash(raw)
	db := database.GetDB()
	var row model.UserSession
	err := db.Where("session_id_hash = ?", hash).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := time.Now()
		row = model.UserSession{
			UserId:        user.Id,
			SessionIdHash: hash,
			IpAddress:     getSessionIP(c),
			UserAgent:     requestUserAgent(c),
			CreatedAt:     now,
			LastSeenAt:    now,
		}
		if err := db.Create(&row).Error; err != nil {
			return "", err
		}
		return raw, nil
	}
	if err != nil {
		return "", err
	}
	if row.UserId != user.Id || row.RevokedAt != nil {
		raw, err := randomSessionID()
		if err != nil {
			return "", err
		}
		s.Set(loginSessionKey, raw)
		hash = sessionHash(raw)
		now := time.Now()
		row = model.UserSession{
			UserId:        user.Id,
			SessionIdHash: hash,
			IpAddress:     getSessionIP(c),
			UserAgent:     requestUserAgent(c),
			CreatedAt:     now,
			LastSeenAt:    now,
		}
		if err := db.Create(&row).Error; err != nil {
			return "", err
		}
		return raw, nil
	}
	now := time.Now()
	if now.Sub(row.LastSeenAt) >= sessionTouchWindow {
		if err := db.Model(&row).Updates(map[string]any{
			"last_seen_at": now,
			"ip_address":   getSessionIP(c),
			"user_agent":   requestUserAgent(c),
		}).Error; err != nil {
			return "", err
		}
	}
	return raw, nil
}

func SetLoginUser(c *gin.Context, user *model.User) error {
	if user == nil {
		return nil
	}
	s := sessions.Default(c)
	if _, err := ensureSessionRecord(c, user, s); err != nil {
		return err
	}
	s.Set(loginUserKey, user.Id)
	s.Set(loginEpochKey, user.LoginEpoch)
	return s.Save()
}

func SetAPIAuthUser(c *gin.Context, user *model.User) {
	if user == nil {
		return
	}
	c.Set(apiAuthUserKey, user)
}

func GetLoginUser(c *gin.Context) *model.User {
	if v, ok := c.Get(apiAuthUserKey); ok {
		if u, ok2 := v.(*model.User); ok2 {
			return u
		}
	}
	s := sessions.Default(c)
	obj := s.Get(loginUserKey)
	if obj == nil {
		return nil
	}
	userID, ok := sessionUserID(obj)
	if !ok {
		s.Delete(loginUserKey)
		s.Delete(loginEpochKey)
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to drop stale user payload:", err)
		}
		return nil
	}
	if legacyUserID, ok := legacySessionUserID(obj); ok {
		s.Set(loginUserKey, legacyUserID)
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to migrate legacy user payload:", err)
		}
	}
	user, err := getUserByID(userID)
	if err != nil {
		logger.Warning("session: failed to load user:", err)
		s.Delete(loginUserKey)
		s.Delete(loginEpochKey)
		if saveErr := s.Save(); saveErr != nil {
			logger.Warning("session: failed to drop missing user:", saveErr)
		}
		return nil
	}
	if !sessionEpochMatches(s.Get(loginEpochKey), user.LoginEpoch) {
		revokeAndClearSession(c, s, true)
		return nil
	}
	raw, _ := s.Get(loginSessionKey).(string)
	if raw == "" {
		if _, err := ensureSessionRecord(c, user, s); err != nil {
			logger.Warning("session: failed to register migrated session:", err)
			revokeAndClearSession(c, s, true)
			return nil
		}
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to persist migrated session:", err)
			revokeAndClearSession(c, s, false)
			return nil
		}
		return user
	}
	var row model.UserSession
	if err := database.GetDB().Where("session_id_hash = ?", sessionHash(raw)).First(&row).Error; err != nil ||
		row.UserId != user.Id || row.RevokedAt != nil {
		revokeAndClearSession(c, s, true)
		return nil
	}
	now := time.Now()
	if now.Sub(row.LastSeenAt) >= sessionTouchWindow {
		_ = database.GetDB().Model(&row).Updates(map[string]any{
			"last_seen_at": now,
			"ip_address":   getSessionIP(c),
			"user_agent":   requestUserAgent(c),
		}).Error
	}
	return user
}

func sessionEpochMatches(cookieVal any, userEpoch int64) bool {
	var got int64
	switch v := cookieVal.(type) {
	case nil:
	case int64:
		got = v
	case int:
		got = int64(v)
	case int32:
		got = int64(v)
	case float64:
		got = int64(v)
	default:
		return false
	}
	return got == userEpoch
}

func IsLogin(c *gin.Context) bool {
	return GetLoginUser(c) != nil
}

func sessionUserID(obj any) (int, bool) {
	switch v := obj.(type) {
	case int:
		return v, v > 0
	case int64:
		return int(v), v > 0
	case int32:
		return int(v), v > 0
	case float64:
		id := int(v)
		return id, v == float64(id) && id > 0
	case model.User:
		return v.Id, v.Id > 0
	case *model.User:
		if v == nil {
			return 0, false
		}
		return v.Id, v.Id > 0
	default:
		return 0, false
	}
}

func legacySessionUserID(obj any) (int, bool) {
	switch v := obj.(type) {
	case model.User:
		return v.Id, v.Id > 0
	case *model.User:
		if v == nil {
			return 0, false
		}
		return v.Id, v.Id > 0
	default:
		return 0, false
	}
}

func getUserByID(id int) (*model.User, error) {
	db := database.GetDB()
	if db == nil {
		return nil, http.ErrServerClosed
	}
	user := &model.User{}
	if err := db.Model(model.User{}).Where("id = ?", id).First(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func revokeAndClearSession(c *gin.Context, s sessions.Session, save bool) {
	raw, _ := s.Get(loginSessionKey).(string)
	if raw != "" {
		now := time.Now()
		_ = database.GetDB().Model(&model.UserSession{}).
			Where("session_id_hash = ?", sessionHash(raw)).
			Updates(map[string]any{"revoked_at": now, "last_seen_at": now}).Error
	}
	s.Delete(loginUserKey)
	s.Delete(loginEpochKey)
	s.Delete(loginSessionKey)
	if save {
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to persist revoked session:", err)
		}
	}
}

func ListUserSessions(userID int, currentSessionID string) ([]SessionView, error) {
	var rows []model.UserSession
	if err := database.GetDB().Where("user_id = ?", userID).Order("last_seen_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	currentHash := ""
	if currentSessionID != "" {
		currentHash = sessionHash(currentSessionID)
	}
	out := make([]SessionView, 0, len(rows))
	for _, row := range rows {
		view := SessionView{
			Id: row.Id, IpAddress: row.IpAddress, UserAgent: row.UserAgent,
			CreatedAt: row.CreatedAt.Unix(), LastSeenAt: row.LastSeenAt.Unix(),
			IsCurrent: currentHash != "" && currentHash == row.SessionIdHash,
		}
		if row.RevokedAt != nil {
			view.RevokedAt = row.RevokedAt.Unix()
		}
		out = append(out, view)
	}
	return out, nil
}

func RevokeUserSession(userID, sessionID int) error {
	now := time.Now()
	res := database.GetDB().Model(&model.UserSession{}).
		Where("user_id = ? AND id = ? AND revoked_at IS NULL", userID, sessionID).
		Updates(map[string]any{"revoked_at": now, "last_seen_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("session not found or already revoked")
	}
	return nil
}

func RevokeOtherUserSessions(userID int, currentSessionID string) error {
	query := database.GetDB().Model(&model.UserSession{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if currentSessionID != "" {
		query = query.Where("session_id_hash <> ?", sessionHash(currentSessionID))
	}
	return query.Updates(map[string]any{"revoked_at": time.Now()}).Error
}

func ClearSession(c *gin.Context) error {
	s := sessions.Default(c)
	revokeAndClearSession(c, s, false)
	cookiePath := c.GetString("base_path")
	if cookiePath == "" {
		cookiePath = "/"
	}
	secure := c.Request.TLS != nil
	s.Options(sessions.Options{
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	if err := s.Save(); err != nil {
		return err
	}
	if cookiePath != "/" {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
	return nil
}
