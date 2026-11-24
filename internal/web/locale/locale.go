// Package locale provides internationalization (i18n) support for the 3x-ui web panel,
// including translation loading, localization, and middleware for web and bot interfaces.
package locale

import (
	"encoding/json"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var (
	bundleMu     sync.Mutex
	i18nBundle   *i18n.Bundle
	LocalizerWeb *i18n.Localizer
	LocalizerBot *i18n.Localizer
)

// I18nType represents the type of interface for internationalization.
type I18nType string

const (
	Bot I18nType = "bot" // Bot interface type
	Web I18nType = "web" // Web interface type
)

// SettingService interface defines methods for accessing locale settings.
type SettingService interface {
	GetTgLang() (string, error)
}

// InitLocalizer initializes the internationalization system with embedded translation files.
func InitLocalizer(i18nFS fs.FS, settingService SettingService) error {
	bundle := newTranslationBundle()
	if err := parseTranslationFiles(i18nFS, bundle); err != nil {
		return err
	}
	botLang, err := settingService.GetTgLang()
	if err != nil {
		return err
	}
	web := i18n.NewLocalizer(bundle, "en-US")
	bot := i18n.NewLocalizer(bundle, botLang)

	// Publish the complete bundle and its localizers together. Failed
	// initialization must leave the previous working translations intact.
	bundleMu.Lock()
	i18nBundle, LocalizerWeb, LocalizerBot = bundle, web, bot
	bundleMu.Unlock()
	return nil
}

// createTemplateData creates a template data map from parameters with optional separator.
func createTemplateData(params []string, separator ...string) map[string]any {
	sep := "=="
	if len(separator) > 0 {
		sep = separator[0]
	}

	templateData := make(map[string]any)
	for _, param := range params {
		parts := strings.SplitN(param, sep, 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		templateData[parts[0]] = parts[1]
	}

	return templateData
}

// I18n retrieves a localized message for the given key and type.
// It supports both bot and web contexts, with optional template parameters.
// Returns the localized message or an empty string if localization fails.
func I18n(i18nType I18nType, key string, params ...string) string {
	var localizer *i18n.Localizer

	bundleMu.Lock()
	switch i18nType {
	case "bot":
		localizer = LocalizerBot
	case "web":
		localizer = LocalizerWeb
	default:
		bundleMu.Unlock()
		logger.Errorf("Invalid type for I18n: %s", i18nType)
		return ""
	}

	bundleMu.Unlock()
	return localize(localizer, key, params...)
}

func localize(localizer *i18n.Localizer, key string, params ...string) string {
	if localizer == nil {
		// Fallback to key if localizer not ready; prevents nil panic on pages like sub
		return key
	}

	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: createTemplateData(params),
	})
	if err != nil {
		logger.Errorf("Failed to localize message: %v", err)
		return ""
	}

	return msg
}

// LocalizerFor returns a new Localizer for the given language tag using the global bundle.
func LocalizerFor(lang string) *i18n.Localizer {
	bundleMu.Lock()
	defer bundleMu.Unlock()
	if i18nBundle == nil {
		return nil
	}
	return i18n.NewLocalizer(i18nBundle, lang)
}

// I18nForLang retrieves a localized message for a specific language tag with optional params.
func I18nForLang(lang string, key string, params ...string) string {
	loc := LocalizerFor(lang)
	if loc == nil {
		return key
	}
	templateData := createTemplateData(params)
	msg, err := loc.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: templateData,
	})
	if err != nil {
		return key
	}
	return msg
}

func newTranslationBundle() *i18n.Bundle {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	return bundle
}

// LocalizerMiddleware returns a Gin middleware that sets up localization for web requests.
// It determines the user's language from cookies or Accept-Language header,
// creates a localizer instance, and stores it in the Gin context for use in handlers.
// Also provides the I18n function in the context for template rendering.
func LocalizerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Initialize and publish the bundle before concurrent requests use it.
		bundleMu.Lock()
		if i18nBundle == nil {
			i18nBundle = newTranslationBundle()
			// Try lazy-load from disk when running sub server without InitLocalizer
			if err := loadTranslationsFromDisk(i18nBundle); err != nil {
				logger.Warning("i18n lazy load failed:", err)
			}
			LocalizerWeb = i18n.NewLocalizer(i18nBundle, "en-US")
		}
		bundle := i18nBundle
		bundleMu.Unlock()
		var lang string

		if cookie, err := c.Request.Cookie("lang"); err == nil {
			lang = cookie.Value
		} else {
			lang = c.GetHeader("Accept-Language")
		}

		// Capture a localizer for each request. A shared LocalizerWeb pointer
		// can return another user's language under concurrent requests.
		localizer := i18n.NewLocalizer(bundle, lang)
		c.Set("localizer", localizer)
		c.Set("I18n", func(kind I18nType, key string, params ...string) string {
			if kind != Web {
				return I18n(kind, key, params...)
			}
			return localize(localizer, key, params...)
		})
		c.Next()
	}
}

// loadTranslationsFromDisk attempts to load translation files from "internal/web/translation" using the local filesystem.
func loadTranslationsFromDisk(bundle *i18n.Bundle) error {
	return parseTranslationFiles(os.DirFS("internal/web"), bundle)
}

// parseTranslationFiles parses embedded translation files and adds them to the i18n bundle.
func parseTranslationFiles(i18nFS fs.FS, i18nBundle *i18n.Bundle) error {
	return fs.WalkDir(i18nFS, "translation",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			data, err := fs.ReadFile(i18nFS, path)
			if err != nil {
				return err
			}

			_, err = i18nBundle.ParseMessageFileBytes(data, path)
			return err
		})
}
