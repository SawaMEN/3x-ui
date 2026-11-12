package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wirecodec"
)

func TestCoverCertificateExportRequiresAuthenticatedMaster(t *testing.T) {
	for _, tc := range []struct {
		authed bool
		scope  string
		push   string
	}{
		{false, "", ""},
		{false, "", "1"},
		{true, model.ApiScopeAdmin, "1"},
		{true, model.ApiScopeNodeSync, ""},
		{true, model.ApiScopeNodeAdmin, ""},
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"domain":"web.example.org"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Request.Header.Set(wirecodec.MasterPushHeader, tc.push)
		ctx.Set("api_authed", tc.authed)
		ctx.Set("api_token_scope", tc.scope)
		(&DomainController{}).exportCoverCertificate(ctx)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("export allowed for %+v: %d", tc, recorder.Code)
		}
	}
}

func TestCoverCertificateFleetCannotBeEnrolledByBearerToken(t *testing.T) {
	for _, scope := range []string{model.ApiScopeAdmin, model.ApiScopeNodeAdmin, model.ApiScopeNodeSync} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("api_authed", true)
		ctx.Set("api_token_scope", scope)
		(&DomainController{}).saveCertificateFleet(ctx)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("enrollment allowed for %s", scope)
		}
	}
}

func TestInboundCertificatePreparationRejectsBearerMutations(t *testing.T) {
	for _, scope := range []string{model.ApiScopeAdmin, model.ApiScopeNodeAdmin, model.ApiScopeNodeSync} {
		for _, handler := range []func(*gin.Context){(&DomainController{}).prepareInboundCertificate, (&DomainController{}).certificatePreparation} {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("api_authed", true)
			ctx.Set("api_token_scope", scope)
			handler(ctx)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("bearer token permitted certificate preparation: %s", scope)
			}
		}
	}
}
