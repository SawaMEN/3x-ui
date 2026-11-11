package panel

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditAPITokenUnicodeNameLimit(t *testing.T) {
	setupAPITokenTestDB(t)
	svc := &ApiTokenService{}
	for _, char := range []string{"a", "я", "😀"} {
		name := strings.Repeat(char, 64)
		t.Run(char, func(t *testing.T) {
			created, err := svc.Create(name, model.ApiScopeMonitor, 0)
			if err != nil {
				t.Fatalf("create 64-character name: %v", err)
			}
			rotated, err := svc.RecreateByName(name, "")
			if err != nil {
				t.Fatalf("rotate 64-character name: %v", err)
			}
			if rotated.Name != name || rotated.Scope != model.ApiScopeMonitor || rotated.Token == created.Token {
				t.Fatalf("rotation changed metadata or retained credential: %+v", rotated)
			}
			if _, ok := svc.MatchToken(created.Token); ok {
				t.Fatal("old token still accepted")
			}
			if _, ok := svc.MatchToken(rotated.Token); !ok {
				t.Fatal("new token rejected")
			}
		})
		tooLong := name + char
		if _, err := svc.Create(tooLong, model.ApiScopeAdmin, 0); err == nil {
			t.Fatal("create accepted 65-character name")
		}
		if _, err := svc.RecreateByName(tooLong, ""); err == nil {
			t.Fatal("rotate accepted 65-character name")
		}
	}
}
