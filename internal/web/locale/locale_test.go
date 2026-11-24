package locale

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

func TestWebLocalizationIsIsolatedPerRequest(t *testing.T) {
	oldBundle, oldWeb := i18nBundle, LocalizerWeb
	defer func() { i18nBundle, LocalizerWeb = oldBundle, oldWeb }()
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	if err := bundle.AddMessages(language.MustParse("en-US"), &i18n.Message{ID: "test.error", Other: "Invalid domain"}); err != nil {
		t.Fatal(err)
	}
	if err := bundle.AddMessages(language.MustParse("ru-RU"), &i18n.Message{ID: "test.error", Other: "Неверный домен"}); err != nil {
		t.Fatal(err)
	}
	i18nBundle = bundle
	gin.SetMode(gin.TestMode)

	translate := func(lang string) func(I18nType, string, ...string) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.AddCookie(&http.Cookie{Name: "lang", Value: lang})
		LocalizerMiddleware()(c)
		f, ok := c.MustGet("I18n").(func(I18nType, string, ...string) string)
		if !ok {
			t.Fatal("web localization callback is missing")
		}
		return f
	}

	russian := translate("ru-RU")
	english := translate("en-US")
	if got := russian(Web, "test.error"); got != "Неверный домен" {
		t.Fatalf("Russian request switched language after another request: %q", got)
	}
	if got := english(Web, "test.error"); got != "Invalid domain" {
		t.Fatalf("English translation: %q", got)
	}
	if got := russian(Web, "test.error"); got != "Неверный домен" {
		t.Fatalf("Russian request is not isolated: %q", got)
	}
}

func TestTemplateDataIgnoresMalformedParameters(t *testing.T) {
	want := map[string]any{"name": "client"}
	got := createTemplateData([]string{"broken", "==bad", "name==client", "nameWithoutValue"})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed template values: got %#v, want %#v", got, want)
	}
}

type testLocaleSettings struct {
	err error
}

func (s testLocaleSettings) GetTgLang() (string, error) { return "en-US", s.err }

func TestInitLocalizerPreservesPublishedBundleOnFailure(t *testing.T) {
	oldBundle, oldWeb, oldBot := i18nBundle, LocalizerWeb, LocalizerBot
	t.Cleanup(func() {
		bundleMu.Lock()
		i18nBundle, LocalizerWeb, LocalizerBot = oldBundle, oldWeb, oldBot
		bundleMu.Unlock()
	})
	files := fstest.MapFS{
		"translation/en-US.json": {Data: []byte(`{"test.error":"working"}`)},
	}
	if err := InitLocalizer(files, testLocaleSettings{}); err != nil {
		t.Fatal(err)
	}
	previous := i18nBundle
	if err := InitLocalizer(files, testLocaleSettings{err: errors.New("settings unavailable")}); err == nil {
		t.Fatal("expected settings error")
	}
	if err := InitLocalizer(fstest.MapFS{}, testLocaleSettings{}); err == nil {
		t.Fatal("expected translation loading error")
	}
	if i18nBundle != previous || I18n(Web, "test.error") != "working" || I18n(Bot, "test.error") != "working" {
		t.Fatal("failed initialization replaced the working bundle")
	}
}
