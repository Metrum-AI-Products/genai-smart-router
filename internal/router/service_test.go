package router

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnthropicIngressUnaryHappyPath(t *testing.T) {
	var upstreamAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_1",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "hello through router"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
		})
	}))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, "secret-provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "claude-code-test")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-provider-key") {
		t.Fatal("response leaked provider key")
	}
	if upstreamAuth != "Bearer secret-provider-key" {
		t.Fatalf("upstream auth not injected, got %q", upstreamAuth)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["type"] != "message" {
		t.Fatalf("not an anthropic message response: %#v", body)
	}
}

func TestAuthRejectsUnknownTokenBeforeUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer bogus")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatal("upstream was called for unauthorized request")
	}
	if _, tokenID, err := svc.authenticate("", ""); err == nil || tokenID != "missing-token" {
		t.Fatalf("missing token id=%q err=%v", tokenID, err)
	}
	if _, tokenID, err := svc.authenticate("Bearer bogus-secret-token", ""); err == nil || tokenID != "invalid-token" {
		t.Fatalf("invalid token id=%q err=%v", tokenID, err)
	}
}

func TestAuthRejectsInactiveCallerKeyAfterTokenMatch(t *testing.T) {
	for _, tt := range []struct {
		status string
		code   string
	}{
		{status: "disabled", code: "key-disabled"},
		{status: "suspended", code: "key-suspended"},
		{status: "expired", code: "key-expired"},
		{status: "rotated", code: "key-rotated"},
	} {
		t.Run(tt.status, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("upstream must not be called for inactive key")
			}))
			defer upstream.Close()
			cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
			cfg.Users = []UserConfig{{ID: "alice"}}
			cfg.Projects = []ProjectConfig{{ID: "metrum-insights"}}
			cfg.ProjectMemberships = []ProjectMembershipConfig{{UserID: "alice", Project: "metrum-insights", Role: "developer"}}
			cfg.Callers[0].OwnerUser = "alice"
			cfg.Callers[0].Status = tt.status
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()

			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), tt.code) {
				t.Fatalf("status=%d body=%s, want %s", rr.Code, rr.Body.String(), tt.code)
			}
		})
	}
}

func TestAuthAcceptsXAPIKeyForAnthropicStyleClients(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_x_api_key",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "x-api-key ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("X-API-Key", testToken)
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "x-api-key ok") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestAdminBasicAuthCheckDisabledIsNotPublic(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/check", nil)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("disabled admin auth sent challenge: %q", rr.Header().Get("WWW-Authenticate"))
	}
}

func TestAdminBasicAuthCheckChallengesAndAuthorizes(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled:           true,
		Realm:             "Unit Test Admin",
		AllowInsecureHTTP: true,
		Users: []AdminBasicAuthUser{{
			Username:        "admin",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:admin",
			Domain:          "local/test",
			Permissions:     []string{"admin:auth:read"},
		}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	for _, tt := range []struct {
		name       string
		username   string
		password   string
		setAuth    bool
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "bad username", username: "operator", password: "yell-yell-yum", setAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "bad password", username: "admin", password: "wrong", setAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "valid", username: "admin", password: "yell-yell-yum", setAuth: true, wantStatus: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/auth/check", nil)
			if tt.setAuth {
				req.SetBasicAuth(tt.username, tt.password)
			}
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("missing no-store cache header: %#v", rr.Header())
			}
			if tt.wantStatus == http.StatusUnauthorized && !strings.Contains(rr.Header().Get("WWW-Authenticate"), `Basic realm="Unit Test Admin"`) {
				t.Fatalf("missing Basic challenge: %#v", rr.Header())
			}
			if strings.Contains(rr.Body.String(), "yell-yell-yum") || strings.Contains(rr.Body.String(), hash) {
				t.Fatalf("admin auth response exposed credential material: %s", rr.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body["subject"] != "basic:admin" || body["domain"] != "local/test" || body["source"] != "basic" {
					t.Fatalf("unexpected subject response: %#v", body)
				}
			}
		})
	}
}

func TestAdminBasicAuthCheckRequiresPermissionAndHTTPS(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled: true,
		Realm:   "Unit Test Admin",
		Users: []AdminBasicAuthUser{{
			Username:        "admin",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:admin",
			Domain:          "local/test",
		}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	httpReq := httptest.NewRequest(http.MethodGet, "/admin/auth/check", nil)
	httpReq.SetBasicAuth("admin", "yell-yell-yum")
	httpRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(httpRR, httpReq)
	if httpRR.Code != http.StatusUnauthorized {
		t.Fatalf("plain http status=%d body=%s", httpRR.Code, httpRR.Body.String())
	}

	httpsReq := httptest.NewRequest(http.MethodGet, "/admin/auth/check", nil)
	httpsReq.Header.Set("X-Forwarded-Proto", "https")
	httpsReq.SetBasicAuth("admin", "yell-yell-yum")
	httpsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(httpsRR, httpsReq)
	if httpsRR.Code != http.StatusUnauthorized {
		t.Fatalf("untrusted forwarded proto status=%d body=%s", httpsRR.Code, httpsRR.Body.String())
	}

	cfg.Server.AdminAuth.Basic.TrustedProxyCIDRs = []string{"192.0.2.0/24"}
	svcTrusted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svcTrusted.Close()
	trustedReq := httptest.NewRequest(http.MethodGet, "/admin/auth/check", nil)
	trustedReq.Header.Set("X-Forwarded-Proto", "https")
	trustedReq.SetBasicAuth("admin", "yell-yell-yum")
	trustedRR := httptest.NewRecorder()
	svcTrusted.Handler().ServeHTTP(trustedRR, trustedReq)
	if trustedRR.Code != http.StatusForbidden {
		t.Fatalf("missing permission status=%d body=%s", trustedRR.Code, trustedRR.Body.String())
	}
	if !strings.Contains(trustedRR.Body.String(), "admin-forbidden") {
		t.Fatalf("missing admin-forbidden body: %s", trustedRR.Body.String())
	}
}

func TestAdminBasicAuthDoesNotChangeProxyBearerAuth(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_admin_basic_proxy",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", mustBcryptHash(t, "yell-yell-yum"))
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled:           true,
		Realm:             "Unit Test Admin",
		AllowInsecureHTTP: true,
		Users: []AdminBasicAuthUser{{
			Username:        "admin",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:admin",
			Domain:          "local/test",
			Permissions:     []string{"admin:auth:read"},
		}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminOIDCLoginCallbackMeAndLogout(t *testing.T) {
	issuer := newFakeOIDCIssuer(t)
	defer issuer.Close()
	svc := newTestOIDCAdminService(t, issuer, []string{
		"p, user:alice@example.com, example/prod, admin:reports, read",
	})
	defer svc.Close()

	login := httptest.NewRequest(http.MethodGet, "/admin/auth/login", nil)
	loginRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(loginRR, login)
	if loginRR.Code != http.StatusFound {
		t.Fatalf("login status=%d body=%s", loginRR.Code, loginRR.Body.String())
	}
	location := loginRR.Header().Get("Location")
	authURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse auth redirect: %v", err)
	}
	q := authURL.Query()
	if q.Get("state") == "" || q.Get("nonce") == "" || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("login redirect missing OIDC state/nonce/PKCE fields: %s", location)
	}
	issuer.nextNonce.Store(q.Get("nonce"))

	callback := httptest.NewRequest(http.MethodGet, "/admin/auth/callback?state="+url.QueryEscape(q.Get("state"))+"&code=ok", nil)
	callbackRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(callbackRR, callback)
	if callbackRR.Code != http.StatusFound {
		t.Fatalf("callback status=%d body=%s", callbackRR.Code, callbackRR.Body.String())
	}
	cookies := callbackRR.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("callback cookies=%d, want 1", len(cookies))
	}
	sessionCookie := cookies[0]
	if sessionCookie.Name != "test_admin_session" || !sessionCookie.HttpOnly || sessionCookie.Secure || sessionCookie.Path != "/admin" || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie attributes: %#v", sessionCookie)
	}
	if strings.Contains(callbackRR.Body.String(), "id_token") || strings.Contains(callbackRR.Body.String(), "client-secret") {
		t.Fatalf("callback exposed token or secret material: %s", callbackRR.Body.String())
	}

	me := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	me.AddCookie(sessionCookie)
	meRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(meRR, me)
	if meRR.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", meRR.Code, meRR.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(meRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["source"] != "oidc_session" || body["subject"] != "user:alice@example.com" || body["domain"] != "example/prod" || body["email"] != "alice@example.com" {
		t.Fatalf("unexpected me response: %#v", body)
	}
	if strings.Contains(meRR.Body.String(), "id_token") || strings.Contains(meRR.Body.String(), "client-secret") {
		t.Fatalf("me response exposed token or secret material: %s", meRR.Body.String())
	}

	logout := httptest.NewRequest(http.MethodPost, "/admin/auth/logout", nil)
	logout.AddCookie(sessionCookie)
	logoutRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(logoutRR, logout)
	if logoutRR.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", logoutRR.Code, logoutRR.Body.String())
	}

	meAfterLogout := httptest.NewRequest(http.MethodGet, "/admin/auth/me", nil)
	meAfterLogout.AddCookie(sessionCookie)
	meAfterLogoutRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(meAfterLogoutRR, meAfterLogout)
	if meAfterLogoutRR.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status=%d body=%s", meAfterLogoutRR.Code, meAfterLogoutRR.Body.String())
	}
}

func TestAdminOIDCCallbackRejectsBadStateAndDomain(t *testing.T) {
	issuer := newFakeOIDCIssuer(t)
	defer issuer.Close()
	svc := newTestOIDCAdminService(t, issuer, nil)
	defer svc.Close()

	badState := httptest.NewRequest(http.MethodGet, "/admin/auth/callback?state=bad&code=ok", nil)
	badStateRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(badStateRR, badState)
	if badStateRR.Code != http.StatusUnauthorized {
		t.Fatalf("bad state status=%d body=%s", badStateRR.Code, badStateRR.Body.String())
	}

	login := httptest.NewRequest(http.MethodGet, "/admin/auth/login", nil)
	loginRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(loginRR, login)
	authURL, err := url.Parse(loginRR.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	issuer.nextNonce.Store(authURL.Query().Get("nonce"))
	issuer.nextEmail.Store("mallory@other.example")
	badDomain := httptest.NewRequest(http.MethodGet, "/admin/auth/callback?state="+url.QueryEscape(authURL.Query().Get("state"))+"&code=ok", nil)
	badDomainRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(badDomainRR, badDomain)
	if badDomainRR.Code != http.StatusUnauthorized {
		t.Fatalf("bad domain status=%d body=%s", badDomainRR.Code, badDomainRR.Body.String())
	}
	if len(badDomainRR.Result().Cookies()) != 0 {
		t.Fatalf("bad domain callback set cookies: %#v", badDomainRR.Result().Cookies())
	}
}

func TestAdminOIDCStateStoreBoundsPendingLogins(t *testing.T) {
	now := time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
	store := &adminOIDCStateStore{states: map[string]adminOIDCLoginState{}}
	for i := 0; i < adminOIDCMaxPendingLogin; i++ {
		ok := store.put(adminOIDCLoginState{
			state:     fmt.Sprintf("state-%d", i),
			nonce:     "nonce",
			expiresAt: now.Add(adminOIDCLoginTTL),
		}, now)
		if !ok {
			t.Fatalf("state %d was rejected before cap", i)
		}
	}
	if ok := store.put(adminOIDCLoginState{state: "overflow", nonce: "nonce", expiresAt: now.Add(adminOIDCLoginTTL)}, now); ok {
		t.Fatal("overflow state was accepted")
	}
	if ok := store.put(adminOIDCLoginState{state: "after-expiry", nonce: "nonce", expiresAt: now.Add(2 * adminOIDCLoginTTL)}, now.Add(adminOIDCLoginTTL+time.Second)); !ok {
		t.Fatal("state after pruning expired entries was rejected")
	}
}

func TestAdminOIDCSessionAuthorizesReportsWithCasbin(t *testing.T) {
	issuer := newFakeOIDCIssuer(t)
	defer issuer.Close()
	svc := newTestOIDCAdminService(t, issuer, []string{
		"p, user:alice@example.com, example/prod, admin:reports, read",
	})
	defer svc.Close()
	cookie := loginTestOIDCAdmin(t, svc, issuer)

	report := httptest.NewRequest(http.MethodGet, "/admin/reports/", nil)
	report.AddCookie(cookie)
	reportRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(reportRR, report)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("authorized report status=%d body=%s", reportRR.Code, reportRR.Body.String())
	}

	noPolicyIssuer := newFakeOIDCIssuer(t)
	defer noPolicyIssuer.Close()
	noPolicySvc := newTestOIDCAdminService(t, noPolicyIssuer, []string{
		"p, user:bob@example.com, example/prod, admin:reports, read",
	})
	defer noPolicySvc.Close()
	noPolicyCookie := loginTestOIDCAdmin(t, noPolicySvc, noPolicyIssuer)
	forbidden := httptest.NewRequest(http.MethodGet, "/admin/reports/", nil)
	forbidden.AddCookie(noPolicyCookie)
	forbiddenRR := httptest.NewRecorder()
	noPolicySvc.Handler().ServeHTTP(forbiddenRR, forbidden)
	if forbiddenRR.Code != http.StatusForbidden || !strings.Contains(forbiddenRR.Body.String(), "reports-forbidden") {
		t.Fatalf("forbidden report status=%d body=%s", forbiddenRR.Code, forbiddenRR.Body.String())
	}
}

func TestAdminReportsRequireBasicAndCasbinAuthorization(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_admin_reports",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 5, "total_tokens": 13},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled:           true,
		Realm:             "Unit Test Admin",
		AllowInsecureHTTP: true,
		Users: []AdminBasicAuthUser{{
			Username:        "admin",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:admin",
			Domain:          "local/test",
		}, {
			Username:        "reader",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:reader",
			Domain:          "local/test",
		}},
	}
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{
		Enabled: true,
		Policy: []string{
			"g, basic:admin, reports_admin, local/test",
			"p, reports_admin, local/test, admin:reports, read|export|drilldown",
			"p, basic:reader, local/test, admin:reports, read",
		},
	}
	cfg.Server.AdminReports = AdminReportsConfig{Enabled: true, DefaultSince: "24h", MaxRange: "31d", MaxRows: 1, ExportMarkdown: true}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	for i := 0; i < 2; i++ {
		modelReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi `+strconv.Itoa(i)+`"}]}`))
		modelReq.Header.Set("Authorization", "Bearer "+testToken)
		modelRR := httptest.NewRecorder()
		svc.Handler().ServeHTTP(modelRR, modelReq)
		if modelRR.Code != http.StatusOK {
			t.Fatalf("model status=%d body=%s", modelRR.Code, modelRR.Body.String())
		}
	}
	errType := "upstream-timeout"
	ttfbMS := int64(125)
	upstreamMS := int64(30100)
	downstreamMS := int64(250)
	upstreamTPS := 42.5
	downstreamTPS := 39.25
	svc.usage.Emit(logRecord{
		TS:                           time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		RequestID:                    "admin-report-synthetic-expensive",
		CallerID:                     "alice",
		CallerUser:                   "alice",
		CallerProject:                "metrum-insights",
		CallerEnvironment:            "test",
		CallerIP:                     "203.0.113.10",
		TokenID:                      "rtr_alice_test",
		Client:                       "codex-cli",
		InboundDialect:               "openai-responses",
		RequestedModel:               "default",
		ResolvedGroup:                "default",
		Strategy:                     "weighted",
		TargetProvider:               "mock",
		TargetModel:                  "mock-model",
		TargetDialect:                "openai",
		Stream:                       true,
		Cache:                        "hit",
		Status:                       504,
		Attempts:                     2,
		FallbackUsed:                 true,
		LatencyMS:                    30150,
		TTFBMS:                       &ttfbMS,
		UpstreamMS:                   &upstreamMS,
		DownstreamMS:                 &downstreamMS,
		UpstreamOutputTPS:            &upstreamTPS,
		DownstreamOutputTPS:          &downstreamTPS,
		Usage:                        Usage{InputTokens: 1_000_000, OutputTokens: 500_000, TotalTokens: 1_500_000},
		InputHasImage:                true,
		InputImageCount:              1,
		InputImageTokens:             1234,
		PIIFilterApplied:             true,
		PIIFilterMode:                "redact",
		PIIFilterReplacements:        2,
		PIIFilterRuleCount:           1,
		InputCostUSD:                 1.25,
		OutputCostUSD:                1.25,
		TotalCostUSD:                 2.50,
		UpstreamReportedTotalCostUSD: 2.75,
		CacheEnabled:                 true,
		CacheItems:                   7,
		CacheBytes:                   4096,
		CacheMaxBytes:                8192,
		CacheOccupancyPct:            50,
		QuotaState:                   "soft_limit",
		KeyState:                     "ok",
		Error:                        &errType,
		ErrorClass:                   "timeout",
		ErrorMessage:                 "redacted provider-key should not be returned",
	})

	unauth := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary?since=24h", nil)
	unauthRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(unauthRR, unauth)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d body=%s", unauthRR.Code, unauthRR.Body.String())
	}

	ordinary := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary?since=24h", nil)
	ordinary.Header.Set("Authorization", "Bearer "+testToken)
	ordinaryRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(ordinaryRR, ordinary)
	if ordinaryRR.Code != http.StatusForbidden || !strings.Contains(ordinaryRR.Body.String(), "reports-forbidden") {
		t.Fatalf("ordinary status=%d body=%s", ordinaryRR.Code, ordinaryRR.Body.String())
	}

	summary := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary?since=24h", nil)
	summary.SetBasicAuth("admin", "yell-yell-yum")
	summaryRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(summaryRR, summary)
	if summaryRR.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", summaryRR.Code, summaryRR.Body.String())
	}
	if summaryRR.Header().Get("Cache-Control") != "no-store" || !strings.Contains(summaryRR.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatalf("missing admin report security headers: %#v", summaryRR.Header())
	}
	var body map[string]any
	if err := json.Unmarshal(summaryRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["summary"].(map[string]any)["requests"].(float64) < 2 {
		t.Fatalf("summary did not include request: %#v", body)
	}
	charts := body["charts"].([]any)
	if len(charts) == 0 {
		t.Fatalf("summary missing chart contract: %#v", body)
	}
	firstChart := charts[0].(map[string]any)
	for _, key := range []string{"chart_id", "title", "x_axis", "y_axis", "series", "generated_at", "from", "to", "filters"} {
		if _, ok := firstChart[key]; !ok {
			t.Fatalf("chart missing %s: %#v", key, firstChart)
		}
	}
	xAxis := firstChart["x_axis"].(map[string]any)
	yAxis := firstChart["y_axis"].(map[string]any)
	if xAxis["label"] == "" || xAxis["type"] == "" || yAxis["label"] == "" || yAxis["unit"] == "" {
		t.Fatalf("chart axes missing labels/units: %#v %#v", xAxis, yAxis)
	}
	chartSeries := firstChart["series"].([]any)
	if len(chartSeries) == 0 {
		t.Fatalf("chart missing series: %#v", firstChart)
	}
	series0 := chartSeries[0].(map[string]any)
	for _, key := range []string{"name", "unit", "color_key", "points"} {
		if _, ok := series0[key]; !ok {
			t.Fatalf("chart series missing %s: %#v", key, series0)
		}
	}
	points := series0["points"].([]any)
	if len(points) == 0 {
		t.Fatalf("chart series missing scalar points: %#v", series0)
	}
	point0 := points[0].(map[string]any)
	if point0["x"] == "" {
		t.Fatalf("chart point missing x: %#v", point0)
	}
	if _, ok := point0["y"].(float64); !ok {
		t.Fatalf("chart point y is not numeric: %#v", point0)
	}
	for _, forbidden := range []string{"token_sha256", "provider-key", testToken, "messages"} {
		if strings.Contains(summaryRR.Body.String(), forbidden) {
			t.Fatalf("summary leaked %q: %s", forbidden, summaryRR.Body.String())
		}
	}

	savings := httptest.NewRequest(http.MethodGet, "/admin/reports/api/savings?since=24h&baseline=gpt-5.5", nil)
	savings.SetBasicAuth("admin", "yell-yell-yum")
	savingsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(savingsRR, savings)
	if savingsRR.Code != http.StatusOK {
		t.Fatalf("savings status=%d body=%s", savingsRR.Code, savingsRR.Body.String())
	}
	var savingsBody map[string]any
	if err := json.Unmarshal(savingsRR.Body.Bytes(), &savingsBody); err != nil {
		t.Fatal(err)
	}
	baseline := savingsBody["baseline"].(map[string]any)
	if baseline["baseline_id"] != "gpt-5.5" || baseline["pricing_source"] == "" || baseline["pricing_updated_at"] == "" {
		t.Fatalf("savings baseline missing source metadata: %#v", baseline)
	}
	savingsSummary := savingsBody["summary"].(map[string]any)
	if savingsSummary["baseline_cost_usd"].(float64) <= 0 {
		t.Fatalf("savings baseline cost was not calculated from stored tokens: %#v", savingsSummary)
	}
	if len(savingsBody["charts"].([]any)) == 0 || len(savingsBody["byGroup"].([]any)) == 0 {
		t.Fatalf("savings missing charts or group rows: %#v", savingsBody)
	}
	for _, forbidden := range []string{"token_sha256", "provider-key", testToken, "messages"} {
		if strings.Contains(savingsRR.Body.String(), forbidden) {
			t.Fatalf("savings leaked %q: %s", forbidden, savingsRR.Body.String())
		}
	}

	reportPaths := []string{
		"/admin/reports/api/overview?since=24h",
		"/admin/reports/api/savings-by-user?since=24h&baseline=custom&baseline_input_price_per_million_usd=4&baseline_output_price_per_million_usd=8",
		"/admin/reports/api/savings-by-key?since=24h&baseline=custom&baseline_input_price_per_million_usd=4&baseline_output_price_per_million_usd=8",
		"/admin/reports/api/savings-by-group?since=24h&baseline=custom&baseline_input_price_per_million_usd=4&baseline_output_price_per_million_usd=8",
		"/admin/reports/api/model-groups-by-user?since=24h",
		"/admin/reports/api/usage-by-key?since=24h",
		"/admin/reports/api/provider-model-mix?since=24h",
		"/admin/reports/api/latency-throughput?since=24h",
		"/admin/reports/api/errors-fallbacks?since=24h",
		"/admin/reports/api/cache?since=24h",
		"/admin/reports/api/quotas-budgets?since=24h",
		"/admin/reports/api/routing-decisions?since=24h",
		"/admin/reports/api/expensive-requests?since=24h&limit=1",
		"/admin/reports/api/client-breakdown?since=24h",
		"/admin/reports/api/project-chargeback?since=24h",
		"/admin/reports/api/capability-usage?since=24h&limit=10",
		"/admin/reports/api/anomalies?since=24h",
	}
	for _, path := range reportPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.SetBasicAuth("admin", "yell-yell-yum")
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		body := mustJSONMap(t, rr.Body.String())
		if body["period"] == nil || body["summary"] == nil || body["generatedUtc"] == "" {
			t.Fatalf("%s missing common report shape: %#v", path, body)
		}
		if strings.Contains(path, "expensive-requests") {
			requestRows := body["requests"].([]any)
			if len(requestRows) != 1 || requestRows[0].(map[string]any)["requestId"] != "admin-report-synthetic-expensive" {
				t.Fatalf("%s did not return bounded cost-sorted requests: %#v", path, body)
			}
		} else if !strings.Contains(path, "overview") {
			rows := body["rows"].([]any)
			if len(rows) == 0 || len(rows) > 1 {
				t.Fatalf("%s rows len=%d, want bounded nonempty rows: %#v", path, len(rows), body)
			}
			row := rows[0].(map[string]any)
			if row["key"] == "" || row["requests"].(float64) <= 0 {
				t.Fatalf("%s missing scalar row key/request count: %#v", path, row)
			}
			if strings.Contains(path, "savings-by") && row["savingsUsd"] == nil {
				t.Fatalf("%s missing savings scalar fields: %#v", path, row)
			}
			if strings.Contains(path, "anomalies") {
				for _, key := range []string{"baseline", "baselineCostUsd", "savingsUsd", "savingsPct"} {
					if _, ok := row[key]; ok {
						t.Fatalf("%s exposed non-baseline savings field %q: %#v", path, key, row)
					}
				}
				if strings.HasPrefix(fmt.Sprint(row["key"]), "key-active") {
					t.Fatalf("%s treated active key state as anomalous: %#v", path, row)
				}
			}
			if strings.Contains(path, "latency-throughput") && row["avgUpstreamTokensPerSec"].(float64) <= 0 {
				t.Fatalf("%s missing throughput scalar fields: %#v", path, row)
			}
			if strings.Contains(path, "capability-usage") {
				if row["secondaryKey"] == "" {
					t.Fatalf("%s missing capability secondary group: %#v", path, row)
				}
			}
		}
		for _, forbidden := range []string{"token_sha256", "provider-key", testToken, "messages"} {
			if strings.Contains(rr.Body.String(), forbidden) {
				t.Fatalf("%s leaked %q: %s", path, forbidden, rr.Body.String())
			}
		}
	}

	forbiddenScalar := httptest.NewRequest(http.MethodGet, "/admin/reports/api/provider-model-mix?since=24h", nil)
	forbiddenScalar.Header.Set("Authorization", "Bearer "+testToken)
	forbiddenScalarRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(forbiddenScalarRR, forbiddenScalar)
	if forbiddenScalarRR.Code != http.StatusForbidden || !strings.Contains(forbiddenScalarRR.Body.String(), "reports-forbidden") {
		t.Fatalf("ordinary scalar report status=%d body=%s", forbiddenScalarRR.Code, forbiddenScalarRR.Body.String())
	}

	custom := httptest.NewRequest(http.MethodGet, "/admin/reports/api/savings?since=24h&baseline=custom&baseline_input_price_per_million_usd=1&baseline_output_price_per_million_usd=2", nil)
	custom.SetBasicAuth("admin", "yell-yell-yum")
	customRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(customRR, custom)
	if customRR.Code != http.StatusOK || !strings.Contains(customRR.Body.String(), `"custom":true`) {
		t.Fatalf("custom savings status=%d body=%s", customRR.Code, customRR.Body.String())
	}

	badCustom := httptest.NewRequest(http.MethodGet, "/admin/reports/api/savings?since=24h&baseline=custom&baseline_input_price_per_million_usd=-1&baseline_output_price_per_million_usd=2", nil)
	badCustom.SetBasicAuth("admin", "yell-yell-yum")
	badCustomRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(badCustomRR, badCustom)
	if badCustomRR.Code != http.StatusBadRequest {
		t.Fatalf("bad custom savings status=%d body=%s", badCustomRR.Code, badCustomRR.Body.String())
	}

	requests := body["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("summary request rows len=%d, want max_rows cap 1: %#v", len(requests), body)
	}
	requestID := requests[0].(map[string]any)["requestId"].(string)
	readerSummary := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary?since=24h", nil)
	readerSummary.SetBasicAuth("reader", "yell-yell-yum")
	readerSummaryRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(readerSummaryRR, readerSummary)
	if readerSummaryRR.Code != http.StatusOK {
		t.Fatalf("reader summary status=%d body=%s", readerSummaryRR.Code, readerSummaryRR.Body.String())
	}
	readerDetail := httptest.NewRequest(http.MethodGet, "/admin/reports/api/request/"+url.PathEscape(requestID), nil)
	readerDetail.SetBasicAuth("reader", "yell-yell-yum")
	readerDetailRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(readerDetailRR, readerDetail)
	if readerDetailRR.Code != http.StatusForbidden || !strings.Contains(readerDetailRR.Body.String(), "reports-forbidden") {
		t.Fatalf("reader detail status=%d body=%s", readerDetailRR.Code, readerDetailRR.Body.String())
	}

	detail := httptest.NewRequest(http.MethodGet, "/admin/reports/api/request/"+url.PathEscape(requestID), nil)
	detail.SetBasicAuth("admin", "yell-yell-yum")
	detailRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(detailRR, detail)
	if detailRR.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRR.Code, detailRR.Body.String())
	}
	var detailBody map[string]any
	if err := json.Unmarshal(detailRR.Body.Bytes(), &detailBody); err != nil {
		t.Fatal(err)
	}
	attempts := detailBody["attempts"].([]any)
	if len(attempts) == 0 || attempts[0].(map[string]any)["attemptIndex"] == nil {
		t.Fatalf("detail missing safe attempt DTO fields: %#v", detailBody)
	}
	for _, forbidden := range []string{"TokenSHA256", "token_sha256", "provider-key", testToken, "messages"} {
		if strings.Contains(detailRR.Body.String(), forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, detailRR.Body.String())
		}
	}

	ui := httptest.NewRequest(http.MethodGet, "/admin/reports/", nil)
	ui.SetBasicAuth("admin", "yell-yell-yum")
	uiRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(uiRR, ui)
	uiBody := uiRR.Body.String()
	for _, want := range []string{"Metrum Smart Router Admin Reports", "static/metrum_logo_white_new.png", `id="themeToggle"`} {
		if !strings.Contains(uiBody, want) {
			t.Fatalf("ui missing %q: status=%d body=%s", want, uiRR.Code, uiBody)
		}
	}
	if uiRR.Code != http.StatusOK || strings.Contains(uiBody, "https://") {
		t.Fatalf("ui status=%d body=%s", uiRR.Code, uiRR.Body.String())
	}

	asset := httptest.NewRequest(http.MethodGet, "/admin/reports/static/chart.umd.js", nil)
	asset.SetBasicAuth("admin", "yell-yell-yum")
	assetRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(assetRR, asset)
	if assetRR.Code != http.StatusOK || !strings.Contains(assetRR.Body.String(), "window.Chart") {
		t.Fatalf("asset status=%d body=%s", assetRR.Code, assetRR.Body.String())
	}

	css := httptest.NewRequest(http.MethodGet, "/admin/reports/static/admin.css", nil)
	css.SetBasicAuth("admin", "yell-yell-yum")
	cssRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(cssRR, css)
	if cssRR.Code != http.StatusOK || !strings.Contains(cssRR.Body.String(), "--metrum-purple: #cc28af") || strings.Contains(cssRR.Body.String(), "#1f6feb") {
		t.Fatalf("css status=%d body=%s", cssRR.Code, cssRR.Body.String())
	}

	js := httptest.NewRequest(http.MethodGet, "/admin/reports/static/admin.js", nil)
	js.SetBasicAuth("admin", "yell-yell-yum")
	jsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(jsRR, js)
	if jsRR.Code != http.StatusOK || !strings.Contains(jsRR.Body.String(), "metrum-admin-reports-theme") || !strings.Contains(jsRR.Body.String(), "localStorage") {
		t.Fatalf("js status=%d body=%s", jsRR.Code, jsRR.Body.String())
	}
	for _, want := range []string{"formatUnit", "renderChartSpecs", "color_key", "api/savings", "savingsRows"} {
		if !strings.Contains(jsRR.Body.String(), want) {
			t.Fatalf("js missing chart contract helper %q: %s", want, jsRR.Body.String())
		}
	}

	logo := httptest.NewRequest(http.MethodGet, "/admin/reports/static/metrum_logo_white_new.png", nil)
	logo.SetBasicAuth("admin", "yell-yell-yum")
	logoRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(logoRR, logo)
	if logoRR.Code != http.StatusOK || logoRR.Body.Len() == 0 {
		t.Fatalf("logo status=%d len=%d", logoRR.Code, logoRR.Body.Len())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/admin/reports/export.md?since=24h", nil)
	exportReq.SetBasicAuth("admin", "yell-yell-yum")
	exportRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(exportRR, exportReq)
	if exportRR.Code != http.StatusOK || !strings.Contains(exportRR.Body.String(), "# Smart LLM Router Usage Report") {
		t.Fatalf("export status=%d body=%s", exportRR.Code, exportRR.Body.String())
	}
}

type fakeOIDCIssuer struct {
	server    *httptest.Server
	key       *rsa.PrivateKey
	nextNonce atomic.Value
	nextEmail atomic.Value
}

func newFakeOIDCIssuer(t *testing.T) *fakeOIDCIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := &fakeOIDCIssuer{key: key}
	issuer.nextEmail.Store("alice@example.com")
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                issuer.server.URL,
			"authorization_endpoint":                issuer.server.URL + "/authorize",
			"token_endpoint":                        issuer.server.URL + "/token",
			"jwks_uri":                              issuer.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]any{issuer.jwk()}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") == "" || r.Form.Get("code_verifier") == "" {
			t.Fatalf("unexpected token request form: %#v", r.Form)
		}
		nonce, _ := issuer.nextNonce.Load().(string)
		email, _ := issuer.nextEmail.Load().(string)
		idToken := issuer.signIDToken(t, map[string]any{
			"iss":            issuer.server.URL,
			"sub":            "stable-subject-1",
			"aud":            "test-client-id",
			"exp":            time.Now().Add(time.Hour).Unix(),
			"iat":            time.Now().Add(-time.Minute).Unix(),
			"nonce":          nonce,
			"email":          email,
			"email_verified": true,
			"groups":         []string{"admins", "finance"},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "opaque-access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})
	issuer.server = httptest.NewServer(mux)
	return issuer
}

func (i *fakeOIDCIssuer) Close() {
	i.server.Close()
}

func (i *fakeOIDCIssuer) URL() string {
	return i.server.URL
}

func (i *fakeOIDCIssuer) jwk() map[string]any {
	n := base64.RawURLEncoding.EncodeToString(i.key.PublicKey.N.Bytes())
	e := big.NewInt(int64(i.key.PublicKey.E)).Bytes()
	return map[string]any{
		"kty": "RSA",
		"use": "sig",
		"kid": "test-key",
		"alg": "RS256",
		"n":   n,
		"e":   base64.RawURLEncoding.EncodeToString(e),
	}
}

func (i *fakeOIDCIssuer) signIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"typ": "JWT", "alg": "RS256", "kid": "test-key"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := encodedHeader + "." + encodedClaims
	sum := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, i.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func newTestOIDCAdminService(t *testing.T, issuer *fakeOIDCIssuer, policy []string) *Service {
	t.Helper()
	t.Setenv("TEST_OIDC_CLIENT_ID", "test-client-id")
	t.Setenv("TEST_OIDC_CLIENT_SECRET", "client-secret")
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.AdminAuth.OIDC = AdminOIDCConfig{
		Enabled:         true,
		IssuerURL:       issuer.URL(),
		ClientIDEnv:     "TEST_OIDC_CLIENT_ID",
		ClientSecretEnv: "TEST_OIDC_CLIENT_SECRET",
		RedirectURL:     "http://localhost/admin/auth/callback",
		AllowedDomains:  []string{"example.com"},
		GroupsClaim:     "groups",
		EmailClaim:      "email",
		SubjectClaim:    "email",
		Domain:          "example/prod",
	}
	cfg.Server.AdminAuth.Sessions = AdminSessionConfig{
		CookieName:    "test_admin_session",
		TTL:           time.Hour,
		SecureCookies: testBoolPtr(false),
		SameSite:      "lax",
	}
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: policy}
	if len(policy) == 0 {
		cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: []string{"p, user:nobody@example.com, example/prod, admin:reports, read"}}
	}
	cfg.Server.AdminReports.Enabled = true
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func loginTestOIDCAdmin(t *testing.T, svc *Service, issuer *fakeOIDCIssuer) *http.Cookie {
	t.Helper()
	login := httptest.NewRequest(http.MethodGet, "/admin/auth/login", nil)
	loginRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(loginRR, login)
	if loginRR.Code != http.StatusFound {
		t.Fatalf("login status=%d body=%s", loginRR.Code, loginRR.Body.String())
	}
	authURL, err := url.Parse(loginRR.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	issuer.nextNonce.Store(authURL.Query().Get("nonce"))
	callback := httptest.NewRequest(http.MethodGet, "/admin/auth/callback?state="+url.QueryEscape(authURL.Query().Get("state"))+"&code=ok", nil)
	callbackRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(callbackRR, callback)
	if callbackRR.Code != http.StatusFound {
		t.Fatalf("callback status=%d body=%s", callbackRR.Code, callbackRR.Body.String())
	}
	cookies := callbackRR.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("callback cookies=%d", len(cookies))
	}
	return cookies[0]
}

func testBoolPtr(v bool) *bool {
	return &v
}

func TestAdminCapabilityUsageReportsEverySignal(t *testing.T) {
	rows := buildAdminScalarReportResponse(
		adminReportFilters{
			From:  time.Now().UTC().Add(-time.Hour),
			To:    time.Now().UTC(),
			Limit: 20,
		},
		[]usageRow{{
			RequestID:        "req_capability",
			TS:               time.Now().UTC(),
			ResolvedGroup:    "default",
			TargetDialect:    "openai-responses",
			Stream:           true,
			Cache:            "hit",
			InputHasImage:    true,
			InputImageCount:  1,
			InputImageTokens: 12,
			PIIFilterApplied: true,
			InputTokens:      10,
			OutputTokens:     5,
			TotalTokens:      15,
			Status:           200,
			Attempts:         1,
		}},
		adminScalarEndpointSpec{Report: "capability-usage", Dimension: "capability", Secondary: "model_group", Sort: "requests"},
		adminSavingsBaselineDTO{},
	).Rows
	got := map[string]bool{}
	for _, row := range rows {
		got[row.Key] = true
	}
	for _, want := range []string{"image-input", "streaming", "pii-filtered", "cacheable", "dialect:openai-responses"} {
		if !got[want] {
			t.Fatalf("missing capability %q in %#v", want, rows)
		}
	}
}

func TestAdminAnomalyKeysTreatActiveKeyStateAsNormal(t *testing.T) {
	spec := adminScalarEndpointSpec{Secondary: "provider_model"}
	keys := adminAnomalyKeys(usageRow{
		TargetProvider: "mock",
		TargetModel:    "mock-model",
		KeyState:       "active",
	}, spec)
	if len(keys) != 0 {
		t.Fatalf("active key state produced anomalies: %#v", keys)
	}

	keys = adminAnomalyKeys(usageRow{
		TargetProvider: "mock",
		TargetModel:    "mock-model",
		KeyState:       "disabled",
	}, spec)
	if len(keys) != 1 || keys[0].Key != "key-disabled" {
		t.Fatalf("disabled key state did not produce key-disabled anomaly: %#v", keys)
	}
}

func TestAdminReportsRejectWithoutCasbinPolicy(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled:           true,
		AllowInsecureHTTP: true,
		Users: []AdminBasicAuthUser{{
			Username:        "admin",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:admin",
			Domain:          "local/test",
		}},
	}
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, Policy: []string{"p, other, local/test, admin:reports, read"}}
	cfg.Server.AdminReports = AdminReportsConfig{Enabled: true, DefaultSince: "24h", MaxRange: "31d", MaxRows: 100}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary", nil)
	req.SetBasicAuth("admin", "yell-yell-yum")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "reports-forbidden") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminSecurityReportsPersistSafeAccessEvents(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "up_security",
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.ClientIP = ClientIPConfig{TrustedProxyCIDRs: []string{"192.0.2.0/24"}, HeaderOrder: []string{"X-Forwarded-For", "X-Real-IP"}}
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled:           true,
		Realm:             "Unit Test Admin",
		AllowInsecureHTTP: true,
		Users: []AdminBasicAuthUser{{
			Username:        "admin",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:admin",
			Domain:          "local/test",
		}, {
			Username:        "reader",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:reader",
			Domain:          "local/test",
		}},
	}
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{
		Enabled: true,
		Policy: []string{
			"g, basic:admin, reports_admin, local/test",
			"p, basic:reader, local/test, admin:security_reports, read",
			"p, reports_admin, local/test, admin:reports, read|export|drilldown",
			"p, reports_admin, local/test, admin:security_reports, read|export",
		},
	}
	cfg.Server.AdminReports = AdminReportsConfig{Enabled: true, DefaultSince: "24h", MaxRange: "31d", MaxRows: 50, ExportMarkdown: true, Security: AdminSecurityReportsConfig{Enabled: true, RetentionDays: 30}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.usage.EmitSecurityAccessEvent(securityAccessEvent{
		TS:         time.Now().UTC().Add(-60 * 24 * time.Hour),
		RequestID:  "req_old_security",
		EventType:  "api_auth_failed",
		Surface:    "v1_chat_completions",
		StatusCode: http.StatusUnauthorized,
		Outcome:    "unauthorized",
		ReasonCode: "invalid-token",
		IPAddress:  "198.51.100.1",
	})

	invalid := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	invalid.RemoteAddr = "192.0.2.10:1234"
	invalid.Header.Set("X-Forwarded-For", "203.0.113.55, 192.0.2.10")
	invalid.Header.Set("Authorization", "Bearer invalid-secret-token")
	invalidRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(invalidRR, invalid)
	if invalidRR.Code != http.StatusUnauthorized {
		t.Fatalf("invalid status=%d body=%s", invalidRR.Code, invalidRR.Body.String())
	}

	okReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	okReq.Header.Set("Authorization", "Bearer "+testToken)
	okRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(okRR, okReq)
	if okRR.Code != http.StatusOK {
		t.Fatalf("ok status=%d body=%s", okRR.Code, okRR.Body.String())
	}
	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	usageReq.Header.Set("Authorization", "Bearer "+testToken)
	usageRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(usageRR, usageReq)
	if usageRR.Code != http.StatusOK {
		t.Fatalf("usage status=%d body=%s", usageRR.Code, usageRR.Body.String())
	}

	ordinary := httptest.NewRequest(http.MethodGet, "/admin/reports/api/security/events?since=24h", nil)
	ordinary.Header.Set("Authorization", "Bearer "+testToken)
	ordinaryRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(ordinaryRR, ordinary)
	if ordinaryRR.Code != http.StatusForbidden || !strings.Contains(ordinaryRR.Body.String(), "reports-forbidden") {
		t.Fatalf("ordinary security status=%d body=%s", ordinaryRR.Code, ordinaryRR.Body.String())
	}

	report := httptest.NewRequest(http.MethodGet, "/admin/reports/api/security/events?since=24h&limit=50", nil)
	report.SetBasicAuth("admin", "yell-yell-yum")
	reportRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(reportRR, report)
	if reportRR.Code != http.StatusOK {
		t.Fatalf("security report status=%d body=%s", reportRR.Code, reportRR.Body.String())
	}
	body := mustJSONMap(t, reportRR.Body.String())
	rows := body["rows"].([]any)
	if len(rows) == 0 {
		t.Fatalf("security report has no rows: %#v", body)
	}
	foundInvalid := false
	foundAllowed := false
	foundUsage := false
	for _, raw := range rows {
		row := raw.(map[string]any)
		if row["requestId"] == "req_old_security" {
			t.Fatalf("old security event was not purged: %#v", row)
		}
		if row["reason"] == "invalid-token" {
			foundInvalid = true
			if row["ipAddress"] != "203.0.113.55" || row["ipSource"] != "x-forwarded-for" || row["trustedProxyApplied"] != true {
				t.Fatalf("invalid-token row missing trusted proxy IP metadata: %#v", row)
			}
		}
		if row["outcome"] == "allowed" && row["surface"] == "v1_chat_completions" {
			foundAllowed = true
			if row["inputTokens"].(float64) <= 0 || row["outputTokens"].(float64) <= 0 {
				t.Fatalf("allowed row missing token split: %#v", row)
			}
		}
		if row["surface"] == "v1_usage" {
			foundUsage = true
			if row["method"] != http.MethodGet || row["path"] != "/v1/usage" {
				t.Fatalf("usage row missing method/path: %#v", row)
			}
		}
	}
	if !foundInvalid || !foundAllowed || !foundUsage {
		t.Fatalf("missing expected security rows invalid=%v allowed=%v usage=%v rows=%#v", foundInvalid, foundAllowed, foundUsage, rows)
	}
	readerCSV := httptest.NewRequest(http.MethodGet, "/admin/reports/security/export.csv?since=24h", nil)
	readerCSV.SetBasicAuth("reader", "yell-yell-yum")
	readerCSVRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(readerCSVRR, readerCSV)
	if readerCSVRR.Code != http.StatusForbidden || !strings.Contains(readerCSVRR.Body.String(), "reports-forbidden") {
		t.Fatalf("reader csv status=%d body=%s", readerCSVRR.Code, readerCSVRR.Body.String())
	}
	csvReq := httptest.NewRequest(http.MethodGet, "/admin/reports/security/export.csv?since=24h", nil)
	csvReq.SetBasicAuth("admin", "yell-yell-yum")
	csvRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(csvRR, csvReq)
	if csvRR.Code != http.StatusOK || !strings.Contains(csvRR.Body.String(), "input_tokens,output_tokens,total_tokens") {
		t.Fatalf("csv status=%d body=%s", csvRR.Code, csvRR.Body.String())
	}
	allEvents, err := svc.usage.securityAccessEvents(SecurityReportOptions{From: time.Now().UTC().Add(-365 * 24 * time.Hour), To: time.Now().UTC().Add(time.Hour), Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range allEvents {
		if event.RequestID == "req_old_security" {
			t.Fatalf("old security event was not purged from store: %#v", event)
		}
	}
	for _, forbidden := range []string{"invalid-secret-token", testToken, "token_sha256", "provider-key", "messages"} {
		if strings.Contains(reportRR.Body.String(), forbidden) {
			t.Fatalf("security report leaked %q: %s", forbidden, reportRR.Body.String())
		}
	}
}

func TestModelsEndpointIncludesCodexModelsField(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["data"].([]any); !ok {
		t.Fatalf("missing OpenAI data field: %#v", body)
	}
	for _, forbidden := range []string{"version", "commit", "build_date", "go_version", "goos", "goarch"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("/v1/models leaked router build field %q: %#v", forbidden, body)
		}
	}
	if models, ok := body["models"].([]any); !ok || len(models) == 0 {
		t.Fatalf("missing Codex models field: %#v", body)
	} else if first, ok := models[0].(map[string]any); !ok || first["slug"] == "" || first["display_name"] == "" || first["base_instructions"] == "" || first["context_window"] == nil || first["max_context_window"] == nil || first["supported_reasoning_levels"] == nil || first["shell_type"] == "" || first["supported_in_api"] != true {
		t.Fatalf("missing Codex model compatibility fields: %#v", body)
	}
}

func TestVersionAndHealthEndpointsExposeBuildInfo(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	for _, path := range []string{"/version", "/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s json: %v", path, err)
		}
		for _, key := range []string{"version", "commit", "build_date"} {
			if body[key] == "" || body[key] == nil {
				t.Fatalf("%s missing %s: %#v", path, key, body)
			}
		}
		if path == "/version" {
			for _, key := range []string{"go_version", "goos", "goarch"} {
				if body[key] == "" || body[key] == nil {
					t.Fatalf("%s missing %s: %#v", path, key, body)
				}
			}
		} else if body["ok"] != true {
			t.Fatalf("%s ok field=%#v body=%#v", path, body["ok"], body)
		}
	}
}

func TestEmbeddedDocsRootRedirectsToDocs(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "/docs/" {
		t.Fatalf("location=%q", got)
	}
}

func TestEmbeddedDocsAreServedUnderDocs(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "GenAI Smart Router") {
		t.Fatalf("root did not serve docs HTML: %s", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	for _, header := range []string{"X-Smart-LLMRouter-Version", "X-Smart-LLMRouter-Build-Date"} {
		if rr.Header().Get(header) == "" {
			t.Fatalf("missing docs version header %s", header)
		}
	}
	if got := rr.Header().Get("X-Smart-LLMRouter-Commit"); got != "" {
		t.Fatalf("docs response exposed source-control commit header: %q", got)
	}
}

func TestEmbeddedDocsServeExtensionlessDocusaurusPages(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/docs/solution-brief", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Solution Brief") {
		t.Fatalf("extensionless doc page did not serve page HTML: %s", rr.Body.String())
	}
}

func TestEmbeddedDocsFallbackDoesNotMaskAPIRoutes(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()

	for _, path := range []string{"/v1/unknown", "/v1", "/metrics/extra"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()

		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "Metrum Smart LLM Router Docs") {
			t.Fatalf("%s unexpectedly served docs fallback", path)
		}
	}
}

func TestModelsEndpointMarksAgentToolsSmokeAsToolCapable(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "tool-model", Dialect: "openai-responses", ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}}}}
	cfg.Callers[0].Allow = []string{"agent-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	models := body["models"].([]any)
	first := models[0].(map[string]any)
	if first["supports_parallel_tool_calls"] != true {
		t.Fatalf("agent-tools-smoke not marked tool capable: %#v", first)
	}
	tools := first["experimental_supported_tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("agent-tools-smoke missing supported tools: %#v", first)
	}
}

func TestModelsEndpointReportsVisionInputModalities(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "text-model", InputModalities: []string{"text"}},
		{Provider: "mock", Model: "vision-model", InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"}},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	models := body["models"].([]any)
	first := models[0].(map[string]any)
	if first["supports_image_detail_original"] != true {
		t.Fatalf("vision support not advertised: %#v", first)
	}
	modalities := first["input_modalities"].([]any)
	hasImage := false
	for _, modality := range modalities {
		hasImage = hasImage || modality == "image"
	}
	if !hasImage {
		t.Fatalf("input_modalities missing image: %#v", first)
	}
	for _, modality := range modalities {
		if modality != "text" && modality != "image" {
			t.Fatalf("public input_modalities exposed client-incompatible modality %q: %#v", modality, first)
		}
	}
}

func TestImageRequestsFilterToVisionTargetsAndBypassCache(t *testing.T) {
	var gotModel string
	var gotImageURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = stringValue(body["model"])
		msgs := body["messages"].([]any)
		content := msgs[0].(map[string]any)["content"].([]any)
		img := content[1].(map[string]any)["image_url"].(map[string]any)
		gotImageURL = stringValue(img["url"])
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_vision",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Rite Aid"},
			}},
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 4,
				"total_tokens":      104,
				"prompt_tokens_details": map[string]any{
					"image_tokens": 64,
				},
				"cost": 0.00456,
				"cost_details": map[string]any{
					"upstream_inference_prompt_cost":      0.003,
					"upstream_inference_completions_cost": 0.00156,
				},
			},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "text-model", InputModalities: []string{"text"}},
		{
			Provider:                           "mock",
			Model:                              "vision-model",
			InputModalities:                    []string{"text", "image"},
			InputPricePerMillionUSD:            2,
			OutputPricePerMillionUSD:           8,
			ImageInputPricePerMillionTokensUSD: 10,
			ImageInputPricePerImageUSD:         0.001,
		},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "default",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Read the receipt."},
				{"type": "image_url", "image_url": {"url": "`+receiptImageURL+`"}}
			]
		}]
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	svc.Close()

	if gotModel != "vision-model" || gotImageURL != receiptImageURL {
		t.Fatalf("upstream model/image=%q/%q", gotModel, gotImageURL)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	logText := string(raw)
	for _, want := range []string{
		`"target_model":"vision-model"`,
		`"input_has_image":true`,
		`"input_image_count":1`,
		`"input_image_tokens":64`,
		`"image_input_price_per_million_tokens_usd":10`,
		`"image_input_price_per_image_usd":0.001`,
		`"image_cost_usd":0.00164`,
		`"upstream_reported_total_cost_usd":0.00456`,
		`"cache":"bypass"`,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("vision log missing %s: %s", want, raw)
		}
	}
	if !strings.Contains(logText, `"input_cost_usd":0.000072`) || !strings.Contains(logText, `"output_cost_usd":0.000032`) || !strings.Contains(logText, `"total_cost_usd":0.001744`) {
		t.Fatalf("vision log missing target/image/cache fields: %s", raw)
	}
}

func TestOpenAIResponsesToolPassthroughPreservesToolsAndRawOutput(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "resp_upstream_tool",
			"object": "response",
			"status": "requires_action",
			"model":  "gpt-tool",
			"output": []map[string]any{{
				"id":        "call_1",
				"type":      "function_call",
				"name":      "shell",
				"call_id":   "call_1",
				"arguments": `{"cmd":"cat > /app/solver.py"}`,
				"status":    "completed",
			}},
			"usage": map[string]any{"input_tokens": 11, "output_tokens": 7, "total_tokens": 18},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["openai"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "openai", Model: "gpt-tool", ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}}}}
	cfg.Callers[0].Allow = []string{"agent-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"agent-tools-smoke",
		"input":"write solver",
		"stream":true,
		"tools":[{"type":"function","name":"shell","description":"run shell","parameters":{"type":"object"}}]
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "codex-test")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "gpt-tool" {
		t.Fatalf("upstream model=%q", upstreamBody["model"])
	}
	if upstreamBody["stream"] != false {
		t.Fatalf("upstream stream=%#v, want false", upstreamBody["stream"])
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%q, want event stream; body=%s", ct, rr.Body.String())
	}
	events := parseSSEEvents(t, rr.Body.String())
	itemDone, ok := events["response.output_item.done"]
	if !ok {
		t.Fatalf("tool output SSE missing: %#v", events)
	}
	item := itemDone["item"].(map[string]any)
	if item["type"] != "function_call" || item["call_id"] != "call_1" {
		t.Fatalf("function call not preserved in SSE: %#v", item)
	}
	completed, ok := events["response.completed"]
	if !ok {
		t.Fatalf("completed SSE missing: %#v", events)
	}
	response := completed["response"].(map[string]any)
	if response["id"] != "resp_upstream_tool" || response["status"] != "requires_action" {
		t.Fatalf("raw response not preserved in completed event: %#v", response)
	}
}

func TestOpenAIChatToolPassthroughPreservesToolsAndStreamsToolCalls(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_tool",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "chat-tool",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]any{{
						"id":   "call_weather",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"location":"San Francisco"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 17, "completion_tokens": 5, "total_tokens": 22},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["openai_chat"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["warp-agent-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "openai_chat",
		Model:       "chat-tool",
		ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
	}}}
	cfg.Callers[0].Allow = []string{"warp-agent-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"warp-agent-smoke",
		"stream":true,
		"messages":[{"role":"user","content":"weather"}],
		"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object","properties":{"location":{"type":"string"}}}}}],
		"tool_choice":"auto",
		"parallel_tool_calls":true
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "OpenAI/Go 3.15.0")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "chat-tool" {
		t.Fatalf("upstream model=%q", upstreamBody["model"])
	}
	if upstreamBody["stream"] != false {
		t.Fatalf("upstream stream=%#v, want false", upstreamBody["stream"])
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
	if upstreamBody["tool_choice"] != "auto" || upstreamBody["parallel_tool_calls"] != true {
		t.Fatalf("tool fields not preserved upstream: %#v", upstreamBody)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%q, want event stream; body=%s", ct, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"tool_calls"`) ||
		!strings.Contains(body, `"get_weather"`) ||
		!strings.Contains(body, `"finish_reason":"tool_calls"`) ||
		!strings.Contains(body, "data: [DONE]") {
		t.Fatalf("tool call stream not preserved:\n%s", body)
	}
}

func TestOpenAIChatToolRequestsRequireExplicitToolSupport(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "chat-maybe-tools"}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := &IRRequest{Tools: []map[string]any{{"type": "function"}}, Messages: []IRMessage{{Role: "user", Content: "hi"}}}
	if got := svc.targetsForRequest(cfg.Models["default"].Targets, req, "openai-chat"); len(got) != 0 {
		t.Fatalf("openai-chat target without explicit tool metadata was eligible: %#v", got)
	}
}

func TestOpenAIChatStructuredOutputPassthrough(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_structured",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "provider-structured-chat",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": `{"ticket_id":"INC-1234","priority":"high"}`},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["structured_chat"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["structured-chat-test"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "structured_chat",
		Model:       "provider-structured-chat",
		ToolSupport: ToolSupport{OpenAIChat: []string{"structured_outputs"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "structured-chat-test")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	requestBody := `{
	  "model":"structured-chat-test",
	  "messages":[{"role":"user","content":"Extract INC-1234 high"}],
	  "response_format":{
	    "type":"json_schema",
	    "json_schema":{
	      "name":"ticket_extract",
	      "strict":true,
	      "schema":{
	        "type":"object",
	        "properties":{
	          "ticket_id":{"type":"string"},
	          "priority":{"type":"string","enum":["low","medium","high"]}
	        },
	        "required":["ticket_id","priority"],
	        "additionalProperties":false
	      }
	    }
	  }
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "provider-structured-chat" {
		t.Fatalf("upstream model=%#v, want provider model; body=%#v", upstreamBody["model"], upstreamBody)
	}
	expectedRequest := mustJSONMap(t, requestBody)
	assertJSONEquivalent(t, "response_format", upstreamBody["response_format"], expectedRequest["response_format"])
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	choices := response["choices"].([]any)
	if response["object"] != "chat.completion" || stringValue(response["id"]) == "" || len(choices) != 1 {
		t.Fatalf("not an OpenAI Chat-compatible response: %#v", response)
	}
	message := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != `{"ticket_id":"INC-1234","priority":"high"}` {
		t.Fatalf("structured response content not preserved: %#v", response)
	}
	rawLog, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ticket_id", "priority", "INC-1234", "ticket_extract"} {
		if strings.Contains(string(rawLog), forbidden) {
			t.Fatalf("request log leaked structured-output schema or prompt text %q: %s", forbidden, rawLog)
		}
	}
}

func TestOpenAIResponsesStructuredOutputPassthrough(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_structured",
			"object":      "response",
			"status":      "completed",
			"model":       "provider-structured-responses",
			"output_text": `{"ticket_id":"INC-1234","priority":"high"}`,
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": `{"ticket_id":"INC-1234","priority":"high"}`,
				}},
			}},
			"usage": map[string]any{"input_tokens": 13, "output_tokens": 8, "total_tokens": 21},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["structured_responses"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["structured-responses-test"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "structured_responses",
		Model:       "provider-structured-responses",
		ToolSupport: ToolSupport{OpenAIResponses: []string{"structured_outputs"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "structured-responses-test")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	requestBody := `{
	  "model":"structured-responses-test",
	  "input":"Extract INC-1234 high",
	  "text":{
	    "format":{
	      "type":"json_schema",
	      "name":"ticket_extract",
	      "strict":true,
	      "schema":{
	        "type":"object",
	        "properties":{
	          "ticket_id":{"type":"string"},
	          "priority":{"type":"string","enum":["low","medium","high"]}
	        },
	        "required":["ticket_id","priority"],
	        "additionalProperties":false
	      }
	    }
	  }
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(requestBody))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "provider-structured-responses" {
		t.Fatalf("upstream model=%#v, want provider model; body=%#v", upstreamBody["model"], upstreamBody)
	}
	expectedRequest := mustJSONMap(t, requestBody)
	assertJSONEquivalent(t, "text.format", upstreamBody["text"], expectedRequest["text"])
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["object"] != "response" || response["status"] != "completed" {
		t.Fatalf("not a Responses-compatible response: %#v", response)
	}
	usage := response["usage"].(map[string]any)
	if usage["total_tokens"] != float64(21) {
		t.Fatalf("usage not preserved: %#v", usage)
	}
}

func TestStructuredOutputRequestsRequireExplicitSupport(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"id": "unexpected"})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["mock"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["structured-chat-test"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "mock",
		Model:       "unsupported-structured-chat",
		ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "structured-chat-test")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"structured-chat-test","messages":[{"role":"user","content":"Extract INC-1234 high"}],"response_format":{"type":"json_schema","json_schema":{"name":"ticket_extract","schema":{"type":"object","properties":{"ticket_id":{"type":"string"}}}}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("unsupported structured-output request reached upstream %d times", calls.Load())
	}
	if !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) ||
		!strings.Contains(rr.Body.String(), `structured_outputs`) {
		t.Fatalf("error details missing structured-output requirement: %s", rr.Body.String())
	}
	rawLog, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawLog), `"error":"no-eligible-target"`) ||
		!strings.Contains(string(rawLog), `structured_outputs`) {
		t.Fatalf("request log missing safe scalar error metadata: %s", rawLog)
	}
	for _, forbidden := range []string{"ticket_id", "INC-1234", "ticket_extract"} {
		if strings.Contains(string(rawLog), forbidden) {
			t.Fatalf("request log leaked structured-output schema or prompt text %q: %s", forbidden, rawLog)
		}
	}
}

func TestToolAndStructuredOutputRequestsRequireBothCapabilities(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	req := &IRRequest{
		Messages: []IRMessage{{Role: "user", Content: "Extract INC-1234 high"}},
		Tools:    []map[string]any{{"type": "function"}},
		Raw: map[string]any{
			"response_format": map[string]any{"type": "json_schema"},
		},
	}
	targets := []Target{
		{Provider: "mock", Model: "tools-only", ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}}},
		{Provider: "mock", Model: "structured-only", ToolSupport: ToolSupport{OpenAIChat: []string{"structured_outputs"}}},
		{Provider: "mock", Model: "tools-and-structured", ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice", "structured_outputs"}}},
		{Provider: "mock", Model: "neither"},
	}
	got := svc.targetsForRequest(targets, req, "openai-chat")
	if len(got) != 1 || got[0].Model != "tools-and-structured" {
		t.Fatalf("eligible targets=%#v, want only tools-and-structured", got)
	}
}

func TestOpenAIChatMaxCompletionTokensSkipsTargetsThatDoNotHonorCaps(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_max_completion_tokens",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "cap-safe-chat",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "x"},
				"finish_reason": "length",
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 1, "total_tokens": 8},
		})
	}))
	defer upstream.Close()

	honorsMaxTokens := true
	ignoresMaxTokens := false
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["ignored_caps"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Provider["safe_caps"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["chat"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "ignored_caps", Model: "cap-unsafe-chat", InputModalities: []string{"text"}, OutputModalities: []string{"text"}, HonorsMaxTokens: &ignoresMaxTokens},
		{Provider: "safe_caps", Model: "cap-safe-chat", InputModalities: []string{"text"}, OutputModalities: []string{"text"}, HonorsMaxTokens: &honorsMaxTokens},
	}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "chat")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"chat","max_completion_tokens":1,"messages":[{"role":"user","content":"write a long essay"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamBody["model"]; got != "cap-safe-chat" {
		t.Fatalf("upstream model=%#v, want cap-safe-chat; body=%#v", got, upstreamBody)
	}
	if got := upstreamBody["max_completion_tokens"]; got != float64(1) {
		t.Fatalf("upstream max_completion_tokens=%#v, want 1; body=%#v", got, upstreamBody)
	}
	if _, ok := upstreamBody["max_tokens"]; ok {
		t.Fatalf("upstream max_tokens should not be set when max_completion_tokens was used; body=%#v", upstreamBody)
	}
}

func TestOpenAIChatToolPassthroughMaxCompletionTokensFiltersCapUnsafeTargets(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_tool_cap",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "cap-safe-tool",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]any{{
						"id":   "call_echo",
						"type": "function",
						"function": map[string]any{
							"name":      "echo",
							"arguments": `{"text":"hi"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 17, "completion_tokens": 1, "total_tokens": 18},
		})
	}))
	defer upstream.Close()

	honorsMaxTokens := true
	ignoresMaxTokens := false
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["ignored_caps"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Provider["safe_caps"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["warp-agent-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "ignored_caps", Model: "cap-unsafe-tool", HonorsMaxTokens: &ignoresMaxTokens, ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}}},
		{Provider: "safe_caps", Model: "cap-safe-tool", HonorsMaxTokens: &honorsMaxTokens, ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}}},
	}}
	cfg.Callers[0].Allow = []string{"warp-agent-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"warp-agent-smoke",
		"stream":true,
		"max_completion_tokens":1,
		"messages":[{"role":"user","content":"echo"}],
		"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object","properties":{"text":{"type":"string"}}}}}],
		"tool_choice":"auto"
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamBody["model"]; got != "cap-safe-tool" {
		t.Fatalf("upstream model=%#v, want cap-safe-tool; body=%#v", got, upstreamBody)
	}
	if got := upstreamBody["max_completion_tokens"]; got != float64(1) {
		t.Fatalf("upstream max_completion_tokens=%#v, want 1; body=%#v", got, upstreamBody)
	}
	if _, ok := upstreamBody["max_tokens"]; ok {
		t.Fatalf("upstream max_tokens should not be set when max_completion_tokens was used; body=%#v", upstreamBody)
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
}

func TestAnthropicToolPassthroughPreservesToolsAndStreamsToolUse(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            "msg_tool_1",
			"type":          "message",
			"role":          "assistant",
			"model":         "claude-tool",
			"stop_reason":   "tool_use",
			"stop_sequence": nil,
			"content": []map[string]any{{
				"type":  "tool_use",
				"id":    "toolu_1",
				"name":  "Bash",
				"input": map[string]any{"command": "cat > /app/solver.py"},
			}},
			"usage": map[string]any{"input_tokens": 13, "output_tokens": 9},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Provider["anthropic_passthrough"] = ProviderConfig{BaseURL: upstream.URL, Dialect: "anthropic", APIKey: "provider-key"}
	cfg.Models["claude-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "anthropic_passthrough", Model: "claude-tool"}}}
	cfg.Callers[0].Allow = []string{"claude-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-tools-smoke",
		"max_tokens":256,
		"stream":true,
		"messages":[{"role":"user","content":"write solver"}],
		"tools":[{"name":"Bash","description":"run shell","input_schema":{"type":"object"}}]
	}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "claude-code-test")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamBody["model"] != "claude-tool" {
		t.Fatalf("upstream model=%q", upstreamBody["model"])
	}
	if upstreamBody["stream"] != false {
		t.Fatalf("upstream stream=%#v, want false", upstreamBody["stream"])
	}
	if upstreamBody["max_tokens"] != float64(256) {
		t.Fatalf("upstream max_tokens=%#v, want 256", upstreamBody["max_tokens"])
	}
	if tools, ok := upstreamBody["tools"].([]any); !ok || len(tools) != 1 {
		t.Fatalf("tools not preserved upstream: %#v", upstreamBody)
	}
	events := parseSSEEvents(t, rr.Body.String())
	start, ok := events["content_block_start"]
	if !ok {
		t.Fatalf("content block start missing: %#v", events)
	}
	block := start["content_block"].(map[string]any)
	if block["type"] != "tool_use" || block["name"] != "Bash" || block["id"] != "toolu_1" {
		t.Fatalf("tool_use block not preserved: %#v", block)
	}
	delta, ok := events["content_block_delta"]
	if !ok {
		t.Fatalf("content block delta missing: %#v", events)
	}
	inputDelta := delta["delta"].(map[string]any)
	if inputDelta["type"] != "input_json_delta" {
		t.Fatalf("tool input delta not emitted: %#v", inputDelta)
	}
	messageDelta := events["message_delta"]
	stop := messageDelta["delta"].(map[string]any)
	if stop["stop_reason"] != "tool_use" {
		t.Fatalf("stop reason not preserved: %#v", stop)
	}
}

func TestToolRequestsBypassCache(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_tool_cache",
			"object":      "response",
			"status":      "completed",
			"model":       "gpt-tool",
			"output_text": "ok",
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": "ok",
				}},
			}},
			"usage": map[string]any{"input_tokens": 3, "output_tokens": 1, "total_tokens": 4},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.Cache.Enabled = true
	cfg.Provider["openai"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "openai", Model: "gpt-tool", ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}}}}
	cfg.Callers[0].Allow = []string{"agent-tools-smoke"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"agent-tools-smoke","input":"write file","tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}]}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("tool requests should bypass cache; upstream calls=%d", calls.Load())
	}
}

func TestOpenAIChatStructuredOutputRequiresMatchingTargetSupport(t *testing.T) {
	var gotModel string
	var gotResponseFormat map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		gotResponseFormat, _ = body["response_format"].(map[string]any)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chat_structured",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": `{"ticket_id":"INC-1234","priority":"high"}`},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 9, "completion_tokens": 7, "total_tokens": 16},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["structured-chat"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "chat-plain", ToolSupport: ToolSupport{OpenAIChat: []string{"tools"}}},
		{Provider: "mock", Model: "chat-structured", ToolSupport: ToolSupport{OpenAIChat: []string{"structured_outputs"}}},
	}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "structured-chat")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
		"model":"structured-chat",
		"messages":[{"role":"user","content":"Extract the ticket id and priority from INC-1234 high"}],
		"response_format":{"type":"json_schema","json_schema":{"name":"ticket_extract","strict":true,"schema":{"type":"object","properties":{"ticket_id":{"type":"string"},"priority":{"type":"string","enum":["low","medium","high"]}},"required":["ticket_id","priority"],"additionalProperties":false}}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "chat-structured" {
		t.Fatalf("selected model %q, want structured target", gotModel)
	}
	if gotResponseFormat["type"] != "json_schema" {
		t.Fatalf("response_format not forwarded: %#v", gotResponseFormat)
	}
	jsonSchema, _ := gotResponseFormat["json_schema"].(map[string]any)
	if jsonSchema["name"] != "ticket_extract" || jsonSchema["strict"] != true {
		t.Fatalf("schema payload not preserved: %#v", gotResponseFormat)
	}
}

func TestOpenAIResponsesStructuredOutputRequiresMatchingTargetSupport(t *testing.T) {
	var gotModel string
	var gotText map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		gotText, _ = body["text"].(map[string]any)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_structured",
			"object":      "response",
			"status":      "completed",
			"model":       gotModel,
			"output_text": `{"ticket_id":"INC-1234","priority":"high"}`,
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": `{"ticket_id":"INC-1234","priority":"high"}`,
				}},
			}},
			"usage": map[string]any{"input_tokens": 8, "output_tokens": 7, "total_tokens": 15},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["responses"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["structured-responses"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "responses", Model: "responses-plain", ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}},
		{Provider: "responses", Model: "responses-structured", ToolSupport: ToolSupport{OpenAIResponses: []string{"structured_outputs"}}},
	}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "structured-responses")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
		"model":"structured-responses",
		"input":"Extract the ticket id and priority from INC-1234 high",
		"text":{"format":{"type":"json_schema","name":"ticket_extract","strict":true,"schema":{"type":"object","properties":{"ticket_id":{"type":"string"},"priority":{"type":"string","enum":["low","medium","high"]}},"required":["ticket_id","priority"],"additionalProperties":false}}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "responses-structured" {
		t.Fatalf("selected model %q, want structured target", gotModel)
	}
	format, _ := gotText["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "ticket_extract" || format["strict"] != true {
		t.Fatalf("text.format not preserved: %#v", gotText)
	}
}

func TestOpenAIChatToolsAndStructuredOutputRequireBothCapabilities(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chat_tool_structured",
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":       "call_1",
						"type":     "function",
						"function": map[string]any{"name": "pick", "arguments": `{"ticket_id":"INC-1234"}`},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["tool-structured-chat"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "tools-only", ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice"}}},
		{Provider: "mock", Model: "structured-only", ToolSupport: ToolSupport{OpenAIChat: []string{"structured_outputs"}}},
		{Provider: "mock", Model: "tools-and-structured", ToolSupport: ToolSupport{OpenAIChat: []string{"tools", "tool_choice", "structured_outputs"}}},
		{Provider: "mock", Model: "neither"},
	}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "tool-structured-chat")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
		"model":"tool-structured-chat",
		"messages":[{"role":"user","content":"Pick the ticket"}],
		"tools":[{"type":"function","function":{"name":"pick","parameters":{"type":"object","properties":{"ticket_id":{"type":"string"}}}}}],
		"tool_choice":"auto",
		"response_format":{"type":"json_schema","json_schema":{"name":"ticket_extract","strict":true,"schema":{"type":"object","properties":{"ticket_id":{"type":"string"}},"required":["ticket_id"],"additionalProperties":false}}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "tools-and-structured" {
		t.Fatalf("selected model %q, want target with tools and structured outputs", gotModel)
	}
}

func TestStructuredOutputNoEligibleTargetReturnsRequirement(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["plain-chat"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "mock",
		Model:       "plain-chat",
		ToolSupport: ToolSupport{OpenAIChat: []string{"tools"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "plain-chat")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"plain-chat","messages":[{"role":"user","content":"Extract INC-1234"}],"response_format":{"type":"json_schema","json_schema":{"name":"ticket_extract","strict":true,"schema":{"type":"object","properties":{"ticket_id":{"type":"string"}},"required":["ticket_id"],"additionalProperties":false}}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream was called %d time(s)", calls.Load())
	}
	if !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) ||
		!strings.Contains(rr.Body.String(), `"structured_outputs"`) {
		t.Fatalf("structured output requirement missing from body=%s", rr.Body.String())
	}
}

func TestPlainTextFormatDoesNotRequireStructuredOutputSupport(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_text_format",
			"object":      "response",
			"status":      "completed",
			"model":       gotModel,
			"output_text": "plain text ok",
			"usage":       map[string]any{"input_tokens": 3, "output_tokens": 3, "total_tokens": 6},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["responses"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["plain-text-format"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "responses",
		Model:       "responses-plain",
		ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "plain-text-format")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"plain-text-format","input":"Reply plainly.","text":{"format":{"type":"text"}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "responses-plain" {
		t.Fatalf("selected model %q, want plain responses target", gotModel)
	}
}

func TestStructuredOutputRequestsBypassCache(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chat_structured_cache",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": `{"ticket_id":"INC-1234"}`},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.Cache.Enabled = true
	cfg.Models["structured-cache"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:    "mock",
		Model:       "structured-cache",
		ToolSupport: ToolSupport{OpenAIChat: []string{"structured_outputs"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "structured-cache")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"structured-cache","messages":[{"role":"user","content":"Extract INC-1234"}],"response_format":{"type":"json_schema","json_schema":{"name":"ticket_extract","strict":true,"schema":{"type":"object","properties":{"ticket_id":{"type":"string"}},"required":["ticket_id"],"additionalProperties":false}}}}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("structured output requests should bypass cache; upstream calls=%d", calls.Load())
	}
}

func parseSSEEvents(t *testing.T, body string) map[string]map[string]any {
	t.Helper()
	events := map[string]map[string]any{}
	for _, frame := range strings.Split(body, "\n\n") {
		frame = strings.TrimSpace(frame)
		if frame == "" {
			continue
		}
		var event string
		var dataLines []string
		for _, line := range strings.Split(frame, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "[DONE]" {
					continue
				}
				dataLines = append(dataLines, data)
			}
		}
		if event == "" || len(dataLines) == 0 {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &payload); err != nil {
			t.Fatalf("invalid JSON payload for SSE event %q: %v\nframe:\n%s", event, err, frame)
		}
		events[event] = payload
	}
	return events
}

func TestCallerAllowListRestrictsModelGroups(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_allow",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "allowed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["big-coder"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "big-model"}}}
	cfg.Models["high"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "high-model"}}}
	cfg.Callers[0].Allow = []string{"default"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	allowed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	allowed.Header.Set("Authorization", "Bearer "+testToken)
	allowedRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(allowedRR, allowed)
	if allowedRR.Code != http.StatusOK {
		t.Fatalf("allowed status=%d body=%s", allowedRR.Code, allowedRR.Body.String())
	}

	for _, model := range []string{"big-coder", "high"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", model, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "model-not-allowed") {
			t.Fatalf("%s missing model-not-allowed: %s", model, rr.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("disallowed model groups should not call upstream, calls=%d", calls.Load())
	}
}

func TestModelsEndpointOnlyListsAllowedModelGroups(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Models["big-coder"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "big-model"}}}
	cfg.Models["high"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "high-model"}}}
	cfg.Callers[0].Allow = []string{"default"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, model := range body.Data {
		got = append(got, model.ID)
	}
	if strings.Join(got, ",") != "default" {
		t.Fatalf("models=%v, want only default", got)
	}
}

func TestCacheHitAcrossDialectsAndTargetIsolation(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_cache",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "cached answer"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": int(n), "total_tokens": int(n) + 1},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		return rr
	}

	post("/v1/chat/completions", `{"model":"default","messages":[{"role":"user","content":"same"}]}`)
	post("/v1/messages", `{"model":"default","messages":[{"role":"user","content":"same"}]}`)
	if calls.Load() != 1 {
		t.Fatalf("expected cross-dialect cache hit, upstream calls=%d", calls.Load())
	}
	post("/v1/messages", `{"model":"other","messages":[{"role":"user","content":"same"}]}`)
	if calls.Load() != 2 {
		t.Fatalf("expected separate cache entry for different target, upstream calls=%d", calls.Load())
	}
}

func TestCacheHitSanitizesProviderIDAndRawPayload(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":              "provider-secret-response-id",
			"provider_secret": "raw-provider-metadata",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "cached sanitized"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": int(n), "total_tokens": int(n) + 3},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	post := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"id":"caller-specific-id","model":"default","messages":[{"role":"user","content":"same cache prompt"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		raw := rr.Body.String()
		if strings.Contains(raw, "provider-secret-response-id") || strings.Contains(raw, "raw-provider-metadata") {
			t.Fatalf("provider raw data leaked: %s", raw)
		}
		return body
	}

	first := post()
	second := post()
	if calls.Load() != 1 {
		t.Fatalf("expected cache hit, upstream calls=%d", calls.Load())
	}
	if first["id"] == "" || second["id"] == "" || first["id"] == second["id"] {
		t.Fatalf("expected fresh router IDs, first=%q second=%q", first["id"], second["id"])
	}
	if !strings.HasPrefix(first["id"].(string), "resp_") || !strings.HasPrefix(second["id"].(string), "resp_") {
		t.Fatalf("expected router response IDs, first=%q second=%q", first["id"], second["id"])
	}
}

func TestCacheKeyIgnoresRawRequestID(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "provider-id",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "same"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	for _, id := range []string{"caller-id-1", "caller-id-2"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"id":"`+id+`","model":"default","messages":[{"role":"user","content":"raw id ignored"}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected second request to hit cache despite different raw id, calls=%d", calls.Load())
	}
}

func TestCacheTTLAndLRUEviction(t *testing.T) {
	resp := &IRResponse{ID: "provider-id", Model: "m", Text: "cached", Usage: Usage{TotalTokens: 1}, Raw: map[string]any{"id": "provider-id"}}
	short := newCache(CacheConfig{Enabled: true, MaxBytes: 1024, DefaultTTL: time.Nanosecond})
	short.Put("a", resp)
	time.Sleep(time.Millisecond)
	if _, ok := short.Get("a"); ok {
		t.Fatal("expected expired cache entry to miss")
	}

	lru := newCache(CacheConfig{Enabled: true, MaxBytes: 150, DefaultTTL: time.Minute})
	lru.Put("a", &IRResponse{Model: "m", Text: strings.Repeat("a", 40)})
	lru.Put("b", &IRResponse{Model: "m", Text: strings.Repeat("b", 40)})
	if _, ok := lru.Get("a"); ok {
		t.Fatal("expected oldest entry to be evicted")
	}
	if _, ok := lru.Get("b"); !ok {
		t.Fatal("expected newest entry to remain")
	}
}

func TestCacheHitDoesNotConsumeLifetimeQuota(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "quota-cache",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "quota cache"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Callers[0].Key.LifetimeTokens = 10
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	post := func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":4}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	post()
	post()
	if calls.Load() != 1 {
		t.Fatalf("expected second request from cache, calls=%d", calls.Load())
	}
	usage := svc.quota.Usage(svc.quota.callers["alice"])
	keyUsage := usage["key"].(map[string]any)
	if keyUsage["lifetime_tokens"] != int64(5) {
		t.Fatalf("expected only upstream request to count against lifetime quota: %#v", keyUsage)
	}
}

func TestDailyQuotaAdmissionReservesRequestedMaxTokens(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"id": "unexpected"})
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Quota.Day.Tokens = 20
	cfg.Callers[0].Quota.Month.Tokens = 1000000
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "quota-exhausted") {
		t.Fatalf("body=%s, want quota-exhausted", rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream calls=%d, want 0", calls.Load())
	}
}

func TestMonthlyQuotaAdmissionReservesRequestedMaxOutputTokens(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"id": "unexpected"})
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Quota.Day.Tokens = 1000000
	cfg.Callers[0].Quota.Month.Tokens = 20
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"default","input":"hi","max_output_tokens":100}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "quota-exhausted") {
		t.Fatalf("body=%s, want quota-exhausted", rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream calls=%d, want 0", calls.Load())
	}
}

func TestLifetimeAdmissionReservesRequestedMaxCompletionTokens(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"id": "unexpected"})
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Key.LifetimeTokens = 20
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":100}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "key-exhausted") {
		t.Fatalf("body=%s, want key-exhausted", rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream calls=%d, want 0", calls.Load())
	}
}

func TestConcurrentReservationsPreventAggregateTokenOvershoot(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "held",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "held"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Rate.Concurrent = 4
	cfg.Callers[0].Quota.Day.Tokens = 50
	cfg.Callers[0].Quota.Month.Tokens = 1000000
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	firstDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":40}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		firstDone <- rr.Code
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first request did not reach upstream")
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi again"}],"max_tokens":40}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	close(release)
	if code := <-firstDone; code != http.StatusOK {
		t.Fatalf("first status=%d", code)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want 1", calls.Load())
	}
}

func TestConcurrentLifetimeReservationRejectionDoesNotDisableKey(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Key.LifetimeTokens = 100
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	caller := svc.quota.callers["alice"]

	ad := svc.quota.Admit(caller, 0)
	if !ad.OK {
		t.Fatalf("admit failed: %#v", ad)
	}
	defer svc.quota.Release(caller)
	resAd := svc.quota.ReserveTokens(caller, 100)
	if !resAd.OK {
		t.Fatalf("initial reserve failed: %#v", resAd)
	}
	rejected := svc.quota.ReserveTokens(caller, 1)
	if rejected.OK || rejected.Status != http.StatusForbidden || rejected.Reason != "key-exhausted" {
		t.Fatalf("second reserve=%#v, want key-exhausted rejection", rejected)
	}
	svc.quota.RecordTokens(caller, resAd.Reservation, Usage{TotalTokens: 5})
	again := svc.quota.ReserveTokens(caller, 10)
	if !again.OK {
		t.Fatalf("key was disabled by reservation-only exhaustion: %#v", again)
	}
	svc.quota.ReleaseReservation(caller, again.Reservation)
}

func TestFailureReleasesTokenReservation(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "temporary", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Callers[0].Quota.Day.Tokens = 50
	cfg.Callers[0].Quota.Month.Tokens = 1000000
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}],"max_tokens":40}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		return rr.Code
	}
	if code := post(); code != http.StatusBadGateway {
		t.Fatalf("first status=%d", code)
	}
	if code := post(); code != http.StatusBadGateway {
		t.Fatalf("second status=%d", code)
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls=%d, want 2", calls.Load())
	}
}

func TestLifetimeKeyExhaustionReturns403AndPersists(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_quota",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "quota"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 9, "total_tokens": 10},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Callers[0].Key.LifetimeTokens = 10
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", rr.Code, rr.Body.String())
	}
	svc.Close()

	svc2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc2.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"again"}]}`))
	req2.Header.Set("Authorization", "Bearer "+testToken)
	rr2 := httptest.NewRecorder()
	svc2.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Fatalf("second status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if !strings.Contains(rr2.Body.String(), "key-exhausted") {
		t.Fatalf("missing key-exhausted: %s", rr2.Body.String())
	}
}

func TestCountTokensEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"count these tokens"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]int
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["input_tokens"] <= 0 {
		t.Fatalf("bad token estimate: %#v", body)
	}
}

func TestUsageAndLogsIncludeCallerMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_meta",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "metadata"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	group := cfg.Models["default"]
	group.Targets[0].InputPricePerMillionUSD = 2.5
	group.Targets[0].OutputPricePerMillionUSD = 7.5
	group.Targets[0].PricingSource = "https://example.test/pricing"
	group.Targets[0].PricingUpdatedAt = "2026-06-17"
	cfg.Models["default"] = group
	cfg.Server.ClientIP.TrustedProxyCIDRs = []string{"192.0.2.0/24"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	usageReq.Header.Set("Authorization", "Bearer "+testToken)
	usageRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(usageRR, usageReq)
	if usageRR.Code != http.StatusOK {
		t.Fatalf("usage status=%d body=%s", usageRR.Code, usageRR.Body.String())
	}
	var usage map[string]any
	if err := json.Unmarshal(usageRR.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if usage["caller_user"] != "alice" || usage["caller_project"] != "metrum-insights" || usage["caller_environment"] != "test" {
		t.Fatalf("usage metadata missing: %#v", usage)
	}
	svc.Close()

	raw, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"caller_user":"alice"`) || !strings.Contains(string(raw), `"caller_project":"metrum-insights"`) || !strings.Contains(string(raw), `"caller_environment":"test"`) {
		t.Fatalf("log metadata missing: %s", raw)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var rec logRecord
	for _, line := range lines {
		var candidate logRecord
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatal(err)
		}
		if candidate.TargetModel == "mock-model" {
			rec = candidate
			break
		}
	}
	if rec.RequestID == "" {
		t.Fatalf("chat completion record not found: %s", raw)
	}
	if rec.UpstreamMS == nil || *rec.UpstreamMS <= 0 {
		t.Fatalf("upstream duration missing: %#v", rec.UpstreamMS)
	}
	if rec.DownstreamMS == nil || *rec.DownstreamMS <= 0 {
		t.Fatalf("downstream duration missing: %#v", rec.DownstreamMS)
	}
	if rec.UpstreamOutputTPS == nil || rec.DownstreamOutputTPS == nil {
		t.Fatalf("throughput missing: upstream=%#v downstream=%#v", rec.UpstreamOutputTPS, rec.DownstreamOutputTPS)
	}
	if !rec.CacheEnabled || rec.CacheMaxBytes <= 0 {
		t.Fatalf("cache snapshot missing: enabled=%v max=%d", rec.CacheEnabled, rec.CacheMaxBytes)
	}
	if rec.CallerIP != "203.0.113.10" {
		t.Fatalf("caller ip = %q", rec.CallerIP)
	}
	if rec.InputPricePerMillionUSD != 2.5 || rec.OutputPricePerMillionUSD != 7.5 ||
		rec.InputCostUSD != 0.000005 || rec.OutputCostUSD != 0.0000225 || rec.TotalCostUSD != 0.0000275 ||
		rec.PricingSource != "https://example.test/pricing" || rec.PricingUpdatedAt != "2026-06-17" {
		t.Fatalf("cost metadata missing from log record: %#v", rec)
	}
}

func TestMetricsEndpointRequiresMetricsAdminAndExportsGlobalLabels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_metrics",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "metrics"},
			}},
			"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 6, "total_tokens": 10},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	bobToken := "rtr_bob_test_token"
	bobSum := sha256.Sum256([]byte(bobToken))
	adminToken := "rtr_metrics_admin_test_token"
	adminSum := sha256.Sum256([]byte(adminToken))
	cfg.Callers = append(cfg.Callers,
		CallerConfig{
			ID:          "bob",
			User:        "bob",
			Project:     "openfang-daily-reports",
			Environment: "test",
			TokenSHA256: hex.EncodeToString(bobSum[:]),
			TokenID:     "rtr_bob_test",
			Allow:       []string{"default"},
			Rate:        RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
		},
		CallerConfig{
			ID:           "metrics-admin",
			User:         "ops",
			Project:      "observability",
			Environment:  "test",
			TokenSHA256:  hex.EncodeToString(adminSum[:]),
			TokenID:      "rtr_metrics_admin_test",
			Allow:        []string{"default"},
			MetricsAdmin: true,
			Rate:         RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
		},
	)
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	unauth := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	unauthRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(unauthRR, unauth)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("unauth metrics status=%d body=%s", unauthRR.Code, unauthRR.Body.String())
	}

	for _, token := range []string{testToken, bobToken} {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}

	for _, token := range []string{testToken, bobToken} {
		metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		metricsReq.Header.Set("Authorization", "Bearer "+token)
		metricsRR := httptest.NewRecorder()
		svc.Handler().ServeHTTP(metricsRR, metricsReq)
		if metricsRR.Code != http.StatusForbidden {
			t.Fatalf("non-admin metrics status=%d body=%s", metricsRR.Code, metricsRR.Body.String())
		}
		body := metricsRR.Body.String()
		if !strings.Contains(body, "metrics-forbidden") {
			t.Fatalf("non-admin metrics missing metrics-forbidden: %s", body)
		}
		if strings.Contains(body, "caller_user") || strings.Contains(body, "rtr_") || strings.Contains(body, "smart_llmrouter_") {
			t.Fatalf("non-admin metrics leaked metrics data: %s", body)
		}
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer "+adminToken)
	metricsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(metricsRR, metricsReq)
	if metricsRR.Code != http.StatusOK {
		t.Fatalf("admin metrics status=%d body=%s", metricsRR.Code, metricsRR.Body.String())
	}
	body := metricsRR.Body.String()
	for _, want := range []string{
		`smart_llmrouter_requests_total`,
		`caller_id="alice"`,
		`caller_user="alice"`,
		`caller_project="metrum-insights"`,
		`caller_environment="test"`,
		`token_id="rtr_alice_test"`,
		`caller_id="bob"`,
		`caller_user="bob"`,
		`caller_project="openfang-daily-reports"`,
		`token_id="rtr_bob_test"`,
		`model_group="default"`,
		`target_provider="mock"`,
		`target_model="mock-model"`,
		`smart_llmrouter_tokens_total`,
		`smart_llmrouter_cache_bypass_total`,
		`smart_llmrouter_cache_entries`,
		`smart_llmrouter_upstream_output_tokens_per_second_sum`,
		`smart_llmrouter_downstream_output_tokens_per_second_sum`,
		`smart_llmrouter_build_info`,
		`version="`,
		`build_date="`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestCasbinAuthorizationForMetricsAndReports(t *testing.T) {
	hash := mustBcryptHash(t, "yell-yell-yum")
	t.Setenv("SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST", hash)
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "authz-policy.csv")
	if err := os.WriteFile(policyFile, []byte(strings.Join([]string{
		"# caller policy loaded from file",
		"g, caller:alice, metrics_admin, metrum-insights/test",
		"p, metrics_admin, metrum-insights/test, metrics, read",
		"g, basic:reports, reports_admin, local/test",
		"p, reports_admin, local/test, admin:reports, read|export|drilldown",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Server.AdminAuth.Basic = AdminBasicAuthConfig{
		Enabled:           true,
		AllowInsecureHTTP: true,
		Users: []AdminBasicAuthUser{{
			Username:        "reports",
			PasswordHashEnv: "SMART_ROUTER_ADMIN_PASSWORD_HASH_TEST",
			Subject:         "basic:reports",
			Domain:          "local/test",
		}},
	}
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, PolicyFile: policyFile}
	cfg.Server.AdminReports = AdminReportsConfig{Enabled: true, DefaultSince: "24h", MaxRange: "31d", MaxRows: 100}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer "+testToken)
	metricsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(metricsRR, metricsReq)
	if metricsRR.Code != http.StatusOK {
		t.Fatalf("policy metrics status=%d body=%s", metricsRR.Code, metricsRR.Body.String())
	}
	if !strings.Contains(metricsRR.Body.String(), "smart_llmrouter_build_info") {
		t.Fatalf("policy metrics missing metrics output: %s", metricsRR.Body.String())
	}

	reportsReq := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary?since=24h", nil)
	reportsReq.SetBasicAuth("reports", "yell-yell-yum")
	reportsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(reportsRR, reportsReq)
	if reportsRR.Code != http.StatusOK {
		t.Fatalf("reports status=%d body=%s", reportsRR.Code, reportsRR.Body.String())
	}

	ordinaryReports := httptest.NewRequest(http.MethodGet, "/admin/reports/api/summary?since=24h", nil)
	ordinaryReports.Header.Set("Authorization", "Bearer "+testToken)
	ordinaryReportsRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(ordinaryReportsRR, ordinaryReports)
	if ordinaryReportsRR.Code != http.StatusForbidden || !strings.Contains(ordinaryReportsRR.Body.String(), "reports-forbidden") {
		t.Fatalf("caller report status=%d body=%s", ordinaryReportsRR.Code, ordinaryReportsRR.Body.String())
	}
}

func TestCasbinAuthorizationRejectsMalformedPolicyFile(t *testing.T) {
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "authz-policy.csv")
	if err := os.WriteFile(policyFile, []byte("p, reports_admin, local/test, admin:reports\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{Enabled: true, PolicyFile: policyFile}
	svc, err := New(cfg)
	if err == nil {
		svc.Close()
		t.Fatal("expected malformed policy file to fail service startup")
	}
	if !strings.Contains(err.Error(), `p lines require subject, domain, object, action`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnthropicMaxTokensForwardedToAnthropicUpstream(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "msg_max_tokens",
			"type":        "message",
			"role":        "assistant",
			"model":       "anthropic-vision",
			"stop_reason": "max_tokens",
			"content":     []map[string]any{{"type": "text", "text": "x"}},
			"usage":       map[string]any{"input_tokens": 7, "output_tokens": 1},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["anthropic_vision"] = ProviderConfig{BaseURL: upstream.URL, Dialect: "anthropic", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:         "anthropic_vision",
		Model:            "anthropic-vision",
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"vision","max_tokens":1,"messages":[{"role":"user","content":"write a long essay"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamBody["max_tokens"]; got != float64(1) {
		t.Fatalf("upstream max_tokens=%#v, want 1; body=%#v", got, upstreamBody)
	}
}

func TestAnthropicMaxTokensForwardedToOpenAIChatVisionUpstream(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_max_tokens",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "chat-vision",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "x"},
				"finish_reason": "length",
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 1, "total_tokens": 8},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["chat_vision"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:         "chat_vision",
		Model:            "chat-vision",
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"vision","max_tokens":1,"messages":[{"role":"user","content":"write a long essay"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamBody["max_tokens"]; got != float64(1) {
		t.Fatalf("upstream max_tokens=%#v, want 1; body=%#v", got, upstreamBody)
	}
}

func TestExplicitMaxTokensSkipsTargetsThatDoNotHonorCaps(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_max_tokens",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "cap-safe-vision",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "x"},
				"finish_reason": "length",
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 1, "total_tokens": 8},
		})
	}))
	defer upstream.Close()

	honorsMaxTokens := true
	ignoresMaxTokens := false
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["ignored_caps"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Provider["safe_caps"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "ignored_caps", Model: "cap-unsafe-vision", InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"}, HonorsMaxTokens: &ignoresMaxTokens},
		{Provider: "safe_caps", Model: "cap-safe-vision", InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"}, HonorsMaxTokens: &honorsMaxTokens},
	}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"vision","max_tokens":1,"messages":[{"role":"user","content":"write a long essay"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamBody["model"]; got != "cap-safe-vision" {
		t.Fatalf("upstream model=%#v, want cap-safe-vision; body=%#v", got, upstreamBody)
	}
	if got := upstreamBody["max_tokens"]; got != float64(1) {
		t.Fatalf("upstream max_tokens=%#v, want 1; body=%#v", got, upstreamBody)
	}
}

func TestResponsesMaxOutputTokensSkipsTargetsThatDoNotHonorCaps(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_max_output_tokens",
			"object":  "chat.completion",
			"created": 1710000000,
			"model":   "cap-safe-vision",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "x"},
				"finish_reason": "length",
			}},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 1, "total_tokens": 8},
		})
	}))
	defer upstream.Close()

	honorsMaxTokens := true
	ignoresMaxTokens := false
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["ignored_caps"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Provider["safe_caps"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "ignored_caps", Model: "cap-unsafe-vision", InputModalities: []string{"text"}, OutputModalities: []string{"text"}, HonorsMaxTokens: &ignoresMaxTokens},
		{Provider: "safe_caps", Model: "cap-safe-vision", InputModalities: []string{"text"}, OutputModalities: []string{"text"}, HonorsMaxTokens: &honorsMaxTokens},
	}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"vision","max_output_tokens":1,"input":"write a long essay"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := upstreamBody["model"]; got != "cap-safe-vision" {
		t.Fatalf("upstream model=%#v, want cap-safe-vision; body=%#v", got, upstreamBody)
	}
	if got := upstreamBody["max_tokens"]; got != float64(1) {
		t.Fatalf("upstream max_tokens=%#v, want 1; body=%#v", got, upstreamBody)
	}
}

func TestNoEligibleTargetMentionsMaxTokensWhenCapUnsafeTargetsSkipped(t *testing.T) {
	ignoresMaxTokens := false
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Provider["ignored_caps"] = ProviderConfig{BaseURL: "http://127.0.0.1:1/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:         "ignored_caps",
		Model:            "cap-unsafe-vision",
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		HonorsMaxTokens:  &ignoresMaxTokens,
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"vision","max_tokens":1,"messages":[{"role":"user","content":"write a long essay"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "max_tokens") {
		t.Fatalf("body=%s, want max_tokens requirement", rr.Body.String())
	}
}

func TestReplicateProviderAdapter(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "pred_1",
			"status": "succeeded",
			"output": []any{"replicate ", "answer"},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["replicate"] = ProviderConfig{BaseURL: upstream.URL, Dialect: "replicate", APIKey: "replicate-key"}
	cfg.Models["replicate"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "replicate", Model: "owner/model-name"}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "replicate")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"replicate","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/v1/models/owner/model-name/predictions" {
		t.Fatalf("unexpected replicate path %s", gotPath)
	}
	if gotAuth != "Bearer replicate-key" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	input := gotBody["input"].(map[string]any)
	if !strings.Contains(input["prompt"].(string), "hi") {
		t.Fatalf("prompt not mapped: %#v", input)
	}
	if !strings.Contains(rr.Body.String(), "replicate answer") {
		t.Fatalf("response not mapped: %s", rr.Body.String())
	}
}

func TestTypeScriptRoutingStrategy(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
type Target = {
  provider: string;
  model: string;
  tier?: string;
  weight: number;
  keyConfigured: boolean;
};

type RouteContext = {
  text: string;
  targets: Target[];
};

export function route(ctx: RouteContext) {
  const eligible = ctx.targets
    .map((target, index) => ({ target, index }))
    .filter((entry) => entry.target.keyConfigured && entry.target.weight > 0);

  if (eligible.length === 0) {
    return { targetIndex: 0, classLabel: "prompt-size:no-eligible-targets" };
  }

  const preferredTier = ctx.text.length > 8000 ? "heavy" : "cheap";
  const preferred = eligible.find((entry) => entry.target.tier === preferredTier) || eligible[0];

  return {
    targetIndex: preferred.index,
    fallbackIndexes: eligible
      .filter((entry) => entry.index !== preferred.index)
      .map((entry) => entry.index),
    classLabel: "prompt-size:" + preferredTier,
  };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["scripted"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 70},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 30},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "scripted")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	dec, err := svc.pick("scripted", cfg.Models["scripted"], &IRRequest{
		Model:    "scripted",
		Messages: []IRMessage{{Role: "user", Content: "short question"}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err != nil {
		t.Fatalf("short script pick: %v", err)
	}
	if dec.Target.Model != "cheap-model" {
		t.Fatalf("short script pick selected %q", dec.Target.Model)
	}
	if dec.ClassLabel == nil || *dec.ClassLabel != "prompt-size:cheap" {
		t.Fatalf("short script class label=%v", dec.ClassLabel)
	}
	if len(dec.Fallbacks) == 0 || dec.Fallbacks[0].Model != "heavy-model" {
		t.Fatalf("short script fallbacks=%#v", dec.Fallbacks)
	}

	dec, err = svc.pick("scripted", cfg.Models["scripted"], &IRRequest{
		Model:    "scripted",
		Messages: []IRMessage{{Role: "user", Content: strings.Repeat("large prompt ", 900)}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err != nil {
		t.Fatalf("long script pick: %v", err)
	}
	if dec.Target.Model != "heavy-model" {
		t.Fatalf("long script pick selected %q", dec.Target.Model)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"scripted","messages":[{"role":"user","content":"`+strings.Repeat("large prompt ", 900)+`"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "heavy-model" {
		t.Fatalf("script selected model %q", gotModel)
	}
}

func TestTypeScriptRoutingContextRawIsRedactedForNonToolChat(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_pii_raw_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script redacted raw"},
			}},
			"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 3, "total_tokens": 7},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
type RouteContext = {
  request: {
    raw?: unknown;
  };
};

export function route(ctx: RouteContext) {
  const raw = JSON.stringify(ctx.request.raw || {});
  if (raw.includes("jane.doe@example.com")) {
    throw new Error("script raw context leaked original PII");
  }
  if (!raw.includes("[EMAIL_1]")) {
    throw new Error("script raw context missing redacted placeholder: " + raw);
  }
  return { targetIndex: 0, classLabel: "script-pii-redacted" };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["scripted-pii"] = ModelGroup{
		Strategy:  "script",
		Script:    scriptPath,
		PIIFilter: testPIIFilterConfig("redact_only"),
		Targets:   []Target{{Provider: "mock", Model: "script-pii-model"}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "scripted-pii")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"scripted-pii","messages":[{"role":"user","content":"Email jane.doe@example.com"}],"max_tokens":16}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "script-pii-model" {
		t.Fatalf("script selected model %q", gotModel)
	}
}

func TestTypeScriptRoutingCanUseCallerTokenRegex(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_key_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script key routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
type Ctx = { caller?: { tokenId: string; user: string; project: string }; targets: Array<{ model: string }> };
export function route(ctx: Ctx) {
  if (/^rtr_alice_/.test(ctx.caller?.tokenId || "") && /^metrum-/.test(ctx.caller?.project || "")) {
    return { targetIndex: 1, classLabel: "key-regex:" + ctx.caller?.user };
  }
  return { targetIndex: 0, classLabel: "key-regex:fallback" };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["keyed"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "default-key-model"},
			{Provider: "mock", Model: "alice-key-model"},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "keyed")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"keyed","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "alice-key-model" {
		t.Fatalf("script selected model %q", gotModel)
	}
}

func TestTypeScriptPIIPolicyExampleRoutesSensitiveWithoutLeakingPII(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "examples", "typescript-pii-policy", "router.ts")

	cfg := testConfig(t, "http://example.invalid", "provider-key", t.TempDir())
	cfg.Models["pii-aware"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "normal-model", DisplayName: "Normal target", Tier: "normal", Weight: 90},
			{Provider: "mock", Model: "private-model", DisplayName: "Private sensitive target", Tier: "private", Weight: 10},
			{Provider: "mock", Model: "sensitive-fallback-model", DisplayName: "Sensitive fallback target", Tier: "sensitive", Weight: 5},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "pii-aware")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	piiText := "Please summarize the account note for Jane Patient. Email jane.patient@example.com and SSN 123-45-6789 are in the record."
	dec, err := svc.pick("pii-aware", cfg.Models["pii-aware"], &IRRequest{
		Model:    "pii-aware",
		Messages: []IRMessage{{Role: "user", Content: piiText}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err != nil {
		t.Fatalf("pii script pick: %v", err)
	}
	if dec.Target.Model != "private-model" {
		t.Fatalf("pii script pick selected %q", dec.Target.Model)
	}
	if len(dec.Fallbacks) != 1 || dec.Fallbacks[0].Model != "sensitive-fallback-model" {
		t.Fatalf("pii script fallbacks=%#v, want only sensitive fallback target", dec.Fallbacks)
	}
	if dec.ClassLabel == nil || *dec.ClassLabel != "pii-detected:sensitive-route" {
		t.Fatalf("pii script class label=%v", dec.ClassLabel)
	}
	rawDecision, err := json.Marshal(dec)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"jane.patient@example.com", "123-45-6789", "Jane Patient"} {
		if strings.Contains(string(rawDecision), forbidden) {
			t.Fatalf("script decision leaked raw pii %q: %s", forbidden, rawDecision)
		}
	}

	dec, err = svc.pick("pii-aware", cfg.Models["pii-aware"], &IRRequest{
		Model:    "pii-aware",
		Messages: []IRMessage{{Role: "user", Content: "Summarize this public release note in one sentence."}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err != nil {
		t.Fatalf("non-pii script pick: %v", err)
	}
	if dec.Target.Model != "normal-model" {
		t.Fatalf("non-pii script pick selected %q", dec.Target.Model)
	}
	if len(dec.Fallbacks) != 2 {
		t.Fatalf("non-pii script fallbacks=%#v, want remaining eligible targets", dec.Fallbacks)
	}
	if dec.ClassLabel == nil || *dec.ClassLabel != "pii-detected:none" {
		t.Fatalf("non-pii script class label=%v", dec.ClassLabel)
	}
}

func TestTypeScriptPIIPolicyExampleFailsClosedWithoutSensitiveTarget(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "examples", "typescript-pii-policy", "router.ts")

	cfg := testConfig(t, "http://example.invalid", "provider-key", t.TempDir())
	cfg.Models["pii-aware"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		Targets: []Target{
			{Provider: "mock", Model: "normal-model", DisplayName: "Normal target", Tier: "normal", Weight: 90},
			{Provider: "mock", Model: "public-fallback-model", DisplayName: "Public fallback target", Tier: "normal", Weight: 10},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "pii-aware")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	_, err = svc.pick("pii-aware", cfg.Models["pii-aware"], &IRRequest{
		Model:    "pii-aware",
		Messages: []IRMessage{{Role: "user", Content: "Contact Jane Patient at jane.patient@example.com."}},
	}, "openai-chat", svc.quota.callers["alice"], "rtr_alice_test")
	if err == nil {
		t.Fatal("pii script pick succeeded without a sensitive target")
	}
	if !strings.Contains(err.Error(), "pii-detected:no-sensitive-target") {
		t.Fatalf("pii script error=%v, want fail-closed no-sensitive-target error", err)
	}
}

func TestTypeScriptRoutingCanUseBundledImportsAndAllowedHTTP(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "script_http_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "script http routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer policy-secret" {
			t.Fatalf("missing policy auth header %q", r.Header.Get("Authorization"))
		}
		writeJSON(w, http.StatusOK, map[string]any{"tier": "heavy"})
	}))
	defer policy.Close()
	policyURL, err := url.Parse(policy.URL)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "policy.ts"), []byte(`
export function chooseTier(text: string) {
  return text.includes("force") ? "heavy" : "cheap";
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
import { chooseTier } from "./policy";

type RouteContext = {
  text: string;
  targets: Array<{ tier?: string; keyConfigured: boolean; weight: number }>;
};

export function route(ctx: RouteContext) {
  const response = router.fetchJSON("`+policy.URL+`/route", {
    method: "POST",
    body: { hint: chooseTier(ctx.text) },
  });
  const tier = response.ok ? response.body.tier : chooseTier(ctx.text);
  const targetIndex = ctx.targets.findIndex((target) =>
    target.keyConfigured && target.weight > 0 && target.tier === tier
  );
  return { targetIndex: targetIndex >= 0 ? targetIndex : 0, classLabel: "external-policy:" + tier };
}
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["script-http"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		ScriptHTTP: ScriptHTTPConfig{
			Enabled:          true,
			AllowHosts:       []string{policyURL.Hostname()},
			TimeoutMS:        500,
			MaxResponseBytes: 4096,
			Headers:          map[string]string{"Authorization": "Bearer policy-secret"},
		},
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 50},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 50},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "script-http")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"script-http","messages":[{"role":"user","content":"force external policy"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "heavy-model" {
		t.Fatalf("script selected model %q", gotModel)
	}
}

func TestTypeScriptRoutingRejectsHTTPHostOutsideAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when script policy fails")
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tier": "heavy"})
	}))
	defer policy.Close()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
export function route() {
  router.fetchJSON("`+policy.URL+`/route");
  return { targetIndex: 0 };
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["script-http-blocked"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		ScriptHTTP: ScriptHTTPConfig{
			Enabled:    true,
			AllowHosts: []string{"policy.internal.example"},
			TimeoutMS:  500,
		},
		Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "script-http-blocked")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"script-http-blocked","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "routing-failed") {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
}

func TestTypeScriptRoutingRejectsHTTPRedirectOutsideAllowlist(t *testing.T) {
	for _, tt := range []struct {
		name        string
		redirectTo  func(targetURL string) string
		wantBlocked string
	}{
		{
			name:        "loopback IP",
			redirectTo:  func(targetURL string) string { return targetURL + "/secret" },
			wantBlocked: "host 127.0.0.1 is not allowed",
		},
		{
			name:        "public host",
			redirectTo:  func(string) string { return "https://example.com/secret" },
			wantBlocked: "host example.com is not allowed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var redirectedReached atomic.Bool
			redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectedReached.Store(true)
				writeJSON(w, http.StatusOK, map[string]any{"tier": "heavy"})
			}))
			defer redirected.Close()
			policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, tt.redirectTo(redirected.URL), http.StatusFound)
			}))
			defer policy.Close()
			policyURL := testURLWithHostname(t, policy.URL, "localhost")

			dir := t.TempDir()
			scriptPath := filepath.Join(dir, "router.ts")
			if err := os.WriteFile(scriptPath, []byte(`
export function route() {
  router.fetchJSON("`+policyURL+`/route");
  return { targetIndex: 0 };
}
`), 0600); err != nil {
				t.Fatal(err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("upstream should not be called when script policy redirect is blocked")
			}))
			defer upstream.Close()
			cfg := testConfig(t, upstream.URL, "provider-key", dir)
			cfg.Models["script-http-redirect-blocked"] = ModelGroup{
				Strategy: "script",
				Script:   scriptPath,
				ScriptHTTP: ScriptHTTPConfig{
					Enabled:    true,
					AllowHosts: []string{"localhost"},
					TimeoutMS:  500,
				},
				Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
			}
			cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "script-http-redirect-blocked")
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"script-http-redirect-blocked","messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadGateway {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "routing-failed") {
				t.Fatalf("unexpected body=%s", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "/secret") {
				t.Fatalf("redirect error leaked URL path: %s", rr.Body.String())
			}
			if redirectedReached.Load() {
				t.Fatal("redirected server was reached")
			}
		})
	}
}

func TestTypeScriptRoutingAllowsHTTPRedirectToAllowedHost(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "script_http_redirect_allowed",
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/route" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0})
	}))
	defer policy.Close()
	policyURL := testURLWithHostname(t, policy.URL, "localhost")

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "router.ts")
	if err := os.WriteFile(scriptPath, []byte(`
export function route() {
  const response = router.fetchJSON("`+policyURL+`/route");
  return { targetIndex: response.body.targetIndex };
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["script-http-redirect-allowed"] = ModelGroup{
		Strategy: "script",
		Script:   scriptPath,
		ScriptHTTP: ScriptHTTPConfig{
			Enabled:    true,
			AllowHosts: []string{"localhost"},
			TimeoutMS:  500,
		},
		Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "script-http-redirect-allowed")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"script-http-redirect-allowed","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "cheap-model" {
		t.Fatalf("selected model %q", gotModel)
	}
}

func TestExternalRoutingPolicyStrategy(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "external_policy_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "external policy routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()

	var policyPayload map[string]any
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer policy-secret" {
			t.Fatalf("missing policy auth header %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&policyPayload); err != nil {
			t.Fatal(err)
		}
		text, _ := policyPayload["text"].(string)
		if len(text) > 8000 {
			writeJSON(w, http.StatusOK, map[string]any{
				"targetIndex":     1,
				"fallbackIndexes": []int{0},
				"classLabel":      "external-policy:heavy",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0, "classLabel": "external-policy:cheap"})
	}))
	defer policy.Close()
	policyURL, err := url.Parse(policy.URL)
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["external-policy"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:              policy.URL + "/route",
			AllowHosts:       []string{policyURL.Hostname()},
			TimeoutMS:        500,
			MaxResponseBytes: 4096,
			Headers:          map[string]string{"Authorization": "Bearer policy-secret"},
		},
		Targets: []Target{
			{Provider: "mock", Model: "cheap-model", Tier: "cheap", Weight: 70, InputPricePerMillionUSD: 0.1, OutputPricePerMillionUSD: 0.2},
			{Provider: "mock", Model: "heavy-model", Tier: "heavy", Weight: 30, InputPricePerMillionUSD: 1.0, OutputPricePerMillionUSD: 2.0},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "external-policy")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"external-policy","messages":[{"role":"user","content":"`+strings.Repeat("large prompt ", 900)+`"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "heavy-model" {
		t.Fatalf("external policy selected model %q", gotModel)
	}
	if policyPayload["group"] != "external-policy" {
		t.Fatalf("policy payload missing group: %#v", policyPayload)
	}
	targets, _ := policyPayload["targets"].([]any)
	if len(targets) != 2 {
		t.Fatalf("policy payload targets=%#v", policyPayload["targets"])
	}
	firstTarget, _ := targets[0].(map[string]any)
	if firstTarget["inputPricePerMillionUsd"] != 0.1 || firstTarget["keyConfigured"] != true {
		t.Fatalf("policy target metadata missing: %#v", firstTarget)
	}
	caller, _ := policyPayload["caller"].(map[string]any)
	if caller["tokenId"] == "" || caller["user"] != "alice" {
		t.Fatalf("policy caller metadata missing: %#v", caller)
	}
	rawPayload, _ := json.Marshal(policyPayload)
	if strings.Contains(string(rawPayload), testToken) || strings.Contains(string(rawPayload), "provider-key") || strings.Contains(string(rawPayload), cfg.Callers[0].TokenSHA256) {
		t.Fatalf("policy payload leaked secret material: %s", rawPayload)
	}
}

func TestExternalRoutingPolicyPayloadRedactsPIIForAllDialects(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		dialect string
		body    string
	}{
		{
			name:    "openai-chat",
			path:    "/v1/chat/completions",
			dialect: "openai-chat",
			body:    `{"model":"external-policy-pii","messages":[{"role":"user","content":"Email jane.doe@example.com"}],"max_tokens":16}`,
		},
		{
			name:    "openai-responses",
			path:    "/v1/responses",
			dialect: "openai-responses",
			body:    `{"model":"external-policy-pii","input":"Email jane.doe@example.com","max_output_tokens":16}`,
		},
		{
			name:    "anthropic",
			path:    "/v1/messages",
			dialect: "anthropic",
			body:    `{"model":"external-policy-pii","messages":[{"role":"user","content":"Email jane.doe@example.com"}],"max_tokens":16}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch tc.dialect {
				case "anthropic":
					writeJSON(w, http.StatusOK, map[string]any{
						"id":          "msg_external_pii",
						"type":        "message",
						"content":     []map[string]any{{"type": "text", "text": "ok"}},
						"stop_reason": "end_turn",
						"usage":       map[string]any{"input_tokens": 4, "output_tokens": 1},
					})
				case "openai-responses":
					writeJSON(w, http.StatusOK, map[string]any{
						"id":          "resp_external_pii",
						"output_text": "ok",
						"usage":       map[string]any{"input_tokens": 4, "output_tokens": 1, "total_tokens": 5},
					})
				default:
					writeJSON(w, http.StatusOK, map[string]any{
						"id": "chat_external_pii",
						"choices": []map[string]any{{
							"message": map[string]any{"role": "assistant", "content": "ok"},
						}},
						"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 1, "total_tokens": 5},
					})
				}
			}))
			defer upstream.Close()

			var policyPayload map[string]any
			policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&policyPayload); err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(policyPayload)
				if strings.Contains(string(raw), "jane.doe@example.com") {
					t.Fatalf("external policy payload leaked raw PII: %s", raw)
				}
				if !strings.Contains(string(raw), "[EMAIL_1]") {
					t.Fatalf("external policy payload missing redacted placeholder: %s", raw)
				}
				writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0, "classLabel": "external-policy-pii-redacted"})
			}))
			defer policy.Close()
			policyURL, err := url.Parse(policy.URL)
			if err != nil {
				t.Fatal(err)
			}

			cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
			cfg.Provider["mock"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: tc.dialect, APIKey: "provider-key"}
			cfg.Models["external-policy-pii"] = ModelGroup{
				Strategy:  "external",
				PIIFilter: testPIIFilterConfig("redact_only"),
				ExternalPolicy: ExternalPolicyConfig{
					URL:        policy.URL + "/route",
					AllowHosts: []string{policyURL.Hostname()},
					TimeoutMS:  500,
				},
				Targets: []Target{{Provider: "mock", Model: "policy-pii-model"}},
			}
			cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "external-policy-pii")
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if policyPayload == nil {
				t.Fatal("external policy was not called")
			}
		})
	}
}

func TestExternalRoutingPolicyRejectsHTTPRedirectOutsideAllowlist(t *testing.T) {
	for _, tt := range []struct {
		name        string
		redirectTo  func(targetURL string) string
		wantBlocked string
	}{
		{
			name:        "loopback IP",
			redirectTo:  func(targetURL string) string { return targetURL + "/secret" },
			wantBlocked: "host 127.0.0.1 is not allowed",
		},
		{
			name:        "public host",
			redirectTo:  func(string) string { return "https://example.com/secret" },
			wantBlocked: "host example.com is not allowed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var redirectedReached atomic.Bool
			redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectedReached.Store(true)
				writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0})
			}))
			defer redirected.Close()
			policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, tt.redirectTo(redirected.URL), http.StatusFound)
			}))
			defer policy.Close()
			policyURL := testURLWithHostname(t, policy.URL, "localhost")

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("upstream should not be called when external policy redirect is blocked")
			}))
			defer upstream.Close()
			cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
			cfg.Models["external-policy-redirect-blocked"] = ModelGroup{
				Strategy: "external",
				ExternalPolicy: ExternalPolicyConfig{
					URL:        policyURL + "/route",
					AllowHosts: []string{"localhost"},
					TimeoutMS:  500,
				},
				Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
			}
			cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "external-policy-redirect-blocked")
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"external-policy-redirect-blocked","messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadGateway {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "routing-policy-error") || !strings.Contains(rr.Body.String(), tt.wantBlocked) {
				t.Fatalf("unexpected body=%s", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "/secret") {
				t.Fatalf("redirect error leaked URL path: %s", rr.Body.String())
			}
			if redirectedReached.Load() {
				t.Fatal("redirected server was reached")
			}
		})
	}
}

func TestExternalRoutingPolicyAllowsHTTPRedirectToAllowedHost(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "external_policy_redirect_allowed",
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/route" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 0})
	}))
	defer policy.Close()
	policyURL := testURLWithHostname(t, policy.URL, "localhost")

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["external-policy-redirect-allowed"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:        policyURL + "/route",
			AllowHosts: []string{"localhost"},
			TimeoutMS:  500,
		},
		Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "external-policy-redirect-allowed")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"external-policy-redirect-allowed","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "cheap-model" {
		t.Fatalf("selected model %q", gotModel)
	}
}

func TestExternalRoutingPolicyInvalidDecisionFailsClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when external policy returns invalid decision")
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"targetIndex": 99})
	}))
	defer policy.Close()
	policyURL, err := url.Parse(policy.URL)
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["external-policy"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:        policy.URL + "/route",
			AllowHosts: []string{policyURL.Hostname()},
			TimeoutMS:  500,
		},
		Targets: []Target{{Provider: "mock", Model: "cheap-model", Weight: 1}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "external-policy")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"external-policy","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "routing-policy-error") {
		t.Fatalf("missing routing-policy-error: %s", rr.Body.String())
	}
}

func TestModelGroupContractFiltersWeightedTargetsAndRecordsUsage(t *testing.T) {
	var selectedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		selectedModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chatcmpl_contract",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	enabled := true
	cfg.Server.UsageDB = UsageDBConfig{Enable: &enabled, Path: filepath.Join(dir, "usage.sqlite")}
	minScore := 0.9
	cfg.Models["contracted"] = ModelGroup{
		Strategy: "weighted",
		Contract: &ModelGroupContract{
			IntendedWorkloads:  []string{"support-chat"},
			SupportedAPIShapes: []string{"openai_chat"},
			QualityFloor: ContractQualityFloor{
				RequireTags:             []string{"validated"},
				MinEvalQualityScore:     &minScore,
				AllowedValidationStatus: []string{"passed"},
			},
			Reporting: ContractReporting{ExposeWorkloadLabels: true},
		},
		Targets: []Target{
			{Provider: "mock", Model: "low-quality", Weight: 100, Tags: []string{"validated"}, Validation: &TargetValidation{Status: "passed", Workload: "support-chat", ValidatedAt: "2026-06-24", QualityScore: 0.7, PassRate: 1, Harness: "unit"}},
			{Provider: "mock", Model: "validated", Weight: 1, Tags: []string{"validated"}, Validation: &TargetValidation{Status: "passed", Workload: "support-chat", ValidatedAt: "2026-06-24", QualityScore: 0.95, PassRate: 1, Harness: "unit"}},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "contracted")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"contracted","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if selectedModel != "validated" {
		t.Fatalf("selected model %q, want validated", selectedModel)
	}
	var records []usageRecord
	if err := svc.usage.db.Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("usage rows=%d, want 1", len(records))
	}
	row, err := rowFromUsageRecord(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if !row.ContractPresent || row.ContractBucket != "passed" || row.ContractWorkload != "support-chat" {
		t.Fatalf("contract usage metadata missing: %#v", row)
	}
	if row.TargetValidationStatus != "passed" || row.TargetValidationWorkload != "support-chat" || row.TargetValidationAgeBucket == "" {
		t.Fatalf("validation usage metadata missing: %#v", row)
	}
}

func TestModelGroupContractNoEligibleTargetStaysGroupLocal(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Fatal("upstream should not be called when requested group contract has no eligible target")
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["support-chat"] = ModelGroup{
		Strategy: "static",
		Contract: &ModelGroupContract{
			SupportedAPIShapes: []string{"openai_chat"},
			RequiredCaps:       ContractRequiredCapabilities{InputModalities: []string{"text"}},
		},
		Targets: []Target{{Provider: "mock", Model: "support-text", InputModalities: []string{"text"}}},
	}
	cfg.Models["receipt-ocr"] = ModelGroup{
		Strategy: "static",
		Contract: &ModelGroupContract{
			SupportedAPIShapes: []string{"openai_chat"},
			RequiredCaps:       ContractRequiredCapabilities{InputModalities: []string{"image"}},
		},
		Targets: []Target{{Provider: "mock", Model: "ocr-image", InputModalities: []string{"text", "image"}}},
	}
	cfg.Callers[0].Allow = []string{"support-chat"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"support-chat","messages":[{"role":"user","content":[{"type":"text","text":"read it"},{"type":"image_url","image_url":{"url":"https://example.test/receipt.png"}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls != 0 {
		t.Fatalf("upstream calls=%d, want 0", calls)
	}
	if strings.Contains(rr.Body.String(), "receipt-ocr") || strings.Contains(rr.Body.String(), "ocr-image") {
		t.Fatalf("no-eligible-target leaked other group details: %s", rr.Body.String())
	}
}

func TestModelGroupContractQualityFloorReasonBuckets(t *testing.T) {
	minScore := 0.9
	contract := &ModelGroupContract{
		SupportedAPIShapes: []string{"openai_chat"},
		QualityFloor: ContractQualityFloor{
			RequireTags:             []string{"validated"},
			MinEvalQualityScore:     &minScore,
			MaxEvalAgeDays:          30,
			AllowedValidationStatus: []string{"passed"},
		},
	}
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name   string
		target Target
		want   string
	}{
		{
			name:   "passes",
			target: Target{Provider: "mock", Model: "ok", Tags: []string{"validated"}, Validation: &TargetValidation{Status: "passed", Workload: "support", ValidatedAt: "2026-06-24", QualityScore: 0.95}},
		},
		{
			name:   "low score",
			target: Target{Provider: "mock", Model: "low", Tags: []string{"validated"}, Validation: &TargetValidation{Status: "passed", Workload: "support", ValidatedAt: "2026-06-24", QualityScore: 0.5}},
			want:   "contract-quality-floor",
		},
		{
			name:   "stale",
			target: Target{Provider: "mock", Model: "stale", Tags: []string{"validated"}, Validation: &TargetValidation{Status: "passed", Workload: "support", ValidatedAt: "2026-04-01", QualityScore: 0.95}},
			want:   "contract-validation-expired",
		},
		{
			name:   "missing validation",
			target: Target{Provider: "mock", Model: "missing", Tags: []string{"validated"}},
			want:   "contract-no-validated-target",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := targetPassesContract(contract, tt.target, "openai-chat", "openai-chat", &IRRequest{}, dynamicStats{}, now)
			if got != tt.want {
				t.Fatalf("reason=%q, want %q", got, tt.want)
			}
		})
	}
}

func testURLWithHostname(t *testing.T, rawURL, hostname string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if port := u.Port(); port != "" {
		u.Host = hostname + ":" + port
	} else {
		u.Host = hostname
	}
	return u.String()
}

func TestExternalRoutingPolicyCanFallbackOnError(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "external_policy_fallback_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "fallback routed"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "policy down", http.StatusServiceUnavailable)
	}))
	defer policy.Close()
	policyURL, err := url.Parse(policy.URL)
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["external-policy"] = ModelGroup{
		Strategy: "external",
		ExternalPolicy: ExternalPolicyConfig{
			URL:        policy.URL + "/route",
			AllowHosts: []string{policyURL.Hostname()},
			TimeoutMS:  500,
			OnError:    "fallback",
		},
		Targets: []Target{
			{Provider: "mock", Model: "fallback-model", Weight: 1},
			{Provider: "mock", Model: "second-model", Weight: 1},
		},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "external-policy")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"external-policy","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "fallback-model" {
		t.Fatalf("fallback selected model %q", gotModel)
	}
}

func TestPIIFilterRedactsOpenAIChatAndRestoresResponse(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(upstreamBody)
		if strings.Contains(string(raw), "jane.doe@example.com") || strings.Contains(string(raw), "415-555-0199") {
			t.Fatalf("upstream received raw PII: %s", raw)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "pii_chat_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Use [EMAIL_1] and [PHONE_1]."},
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 5, "total_tokens": 13},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["default"] = ModelGroup{
		Strategy:  "static",
		PIIFilter: testPIIFilterConfig("redact_and_restore"),
		Targets:   []Target{{Provider: "mock", Model: "mock-model"}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"Email jane.doe@example.com or call 415-555-0199."}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "jane.doe@example.com") || !strings.Contains(rr.Body.String(), "415-555-0199") {
		t.Fatalf("response did not restore placeholders: %s", rr.Body.String())
	}
	msgs := upstreamBody["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].(string)
	if !strings.Contains(content, "[EMAIL_1]") || !strings.Contains(content, "[PHONE_1]") {
		t.Fatalf("upstream content=%q, want placeholders", content)
	}
	logRaw, err := os.ReadFile(cfg.Server.Logging.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logRaw), "jane.doe@example.com") || strings.Contains(string(logRaw), "415-555-0199") {
		t.Fatalf("log leaked raw PII: %s", logRaw)
	}
	if !strings.Contains(string(logRaw), `"pii_filter_applied":true`) || !strings.Contains(string(logRaw), `"pii_filter_replacements":2`) {
		t.Fatalf("log missing pii filter metadata: %s", logRaw)
	}
}

func TestPIIFilterReplacementLimitDoesNotLetRawMirrorStarveNormalizedRequest(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(upstreamBody)
		if strings.Contains(string(raw), "jane.doe@example.com") {
			t.Fatalf("upstream received raw PII after raw mirror consumed budget: %s", raw)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "pii_limit_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Use [EMAIL_1]."},
			}},
			"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 3, "total_tokens": 7},
		})
	}))
	defer upstream.Close()

	filter := testPIIFilterConfig("redact_only")
	filter.MaxReplacementsPerRequest = 1
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{
		Strategy:  "static",
		PIIFilter: filter,
		Targets:   []Target{{Provider: "mock", Model: "mock-model"}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"Email jane.doe@example.com"}],"max_tokens":16}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	msgs := upstreamBody["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].(string)
	if content != "Email [EMAIL_1]" {
		t.Fatalf("upstream content=%q, want normalized placeholder", content)
	}
}

func TestPIIFilterCacheStoresRedactedResponseNotRestoredPII(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "pii_cache_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Contact [EMAIL_1]."},
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 5, "total_tokens": 13},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.Cache.DefaultTTL = time.Minute
	cfg.Models["default"] = ModelGroup{
		Strategy:  "static",
		PIIFilter: testPIIFilterConfig("redact_and_restore"),
		Targets:   []Target{{Provider: "mock", Model: "mock-model"}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	post := func(email string) string {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"Email `+email+`."}]}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}
	first := post("alice@example.com")
	second := post("bob@example.com")
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d, want cache hit on second request", calls.Load())
	}
	if !strings.Contains(first, "alice@example.com") || strings.Contains(first, "bob@example.com") {
		t.Fatalf("first response=%s", first)
	}
	if !strings.Contains(second, "bob@example.com") || strings.Contains(second, "alice@example.com") {
		t.Fatalf("second response=%s", second)
	}
}

func TestPIIFilterFailOnMatchRejectsBeforeUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{
		Strategy:  "static",
		PIIFilter: testPIIFilterConfig("fail_on_match"),
		Targets:   []Target{{Provider: "mock", Model: "mock-model"}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"SSN 123-45-6789"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "pii-filter-blocked") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("upstream calls=%d, want 0", calls.Load())
	}
}

func TestPIIFilterRedactsResponsesAndAnthropicMessages(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		dialect      string
		body         string
		wantField    string
		responseBody map[string]any
	}{
		{
			name:      "responses",
			path:      "/v1/responses",
			dialect:   "openai-responses",
			body:      `{"model":"default","input":"Email jane.doe@example.com","max_output_tokens":16}`,
			wantField: "input",
			responseBody: map[string]any{
				"id":          "resp_pii",
				"output_text": "Received [EMAIL_1].",
				"usage":       map[string]any{"input_tokens": 4, "output_tokens": 3, "total_tokens": 7},
			},
		},
		{
			name:      "anthropic",
			path:      "/v1/messages",
			dialect:   "anthropic",
			body:      `{"model":"default","messages":[{"role":"user","content":"Email jane.doe@example.com"}],"max_tokens":16}`,
			wantField: "messages",
			responseBody: map[string]any{
				"id":          "msg_pii",
				"type":        "message",
				"content":     []map[string]any{{"type": "text", "text": "Received [EMAIL_1]."}},
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 4, "output_tokens": 3},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamBody map[string]any
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(upstreamBody)
				if strings.Contains(string(raw), "jane.doe@example.com") {
					t.Fatalf("upstream received raw PII: %s", raw)
				}
				writeJSON(w, http.StatusOK, tc.responseBody)
			}))
			defer upstream.Close()
			cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
			cfg.Provider["mock"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: tc.dialect, APIKey: "provider-key"}
			cfg.Models["default"] = ModelGroup{
				Strategy:  "static",
				PIIFilter: testPIIFilterConfig("redact_and_restore"),
				Targets:   []Target{{Provider: "mock", Model: "mock-model"}},
			}
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "jane.doe@example.com") {
				t.Fatalf("response did not restore placeholder: %s", rr.Body.String())
			}
			raw, _ := json.Marshal(upstreamBody[tc.wantField])
			if !strings.Contains(string(raw), "[EMAIL_1]") {
				t.Fatalf("upstream %s=%s, want placeholder", tc.wantField, raw)
			}
		})
	}
}

func TestPIIFilterRedactsOpenAIChatToolResultPassthrough(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(upstreamBody)
		if strings.Contains(string(raw), "jane.doe@example.com") {
			t.Fatalf("upstream received raw tool-result PII: %s", raw)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "pii_tool_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Tool result had [EMAIL_1]."},
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 5, "total_tokens": 13},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["default"] = ModelGroup{
		Strategy:  "static",
		PIIFilter: testPIIFilterConfig("redact_and_restore"),
		Targets: []Target{{
			Provider:    "mock",
			Model:       "mock-model",
			ToolSupport: ToolSupport{OpenAIChat: []string{"function"}},
		}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
	  "model":"default",
	  "messages":[
	    {"role":"user","content":"Use the tool result."},
	    {"role":"tool","tool_call_id":"call_123","content":"Customer email jane.doe@example.com"}
	  ],
	  "tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],
	  "tool_choice":"auto"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "jane.doe@example.com") {
		t.Fatalf("response did not restore placeholder: %s", rr.Body.String())
	}
	msgs := upstreamBody["messages"].([]any)
	toolMsg := msgs[1].(map[string]any)
	if toolMsg["tool_call_id"] != "call_123" {
		t.Fatalf("tool_call_id changed: %#v", toolMsg)
	}
	if content := toolMsg["content"].(string); !strings.Contains(content, "[EMAIL_1]") || strings.Contains(content, "jane.doe@example.com") {
		t.Fatalf("tool content=%q, want redacted placeholder", content)
	}
}

func TestPIIFilterRedactOnlyPreservesPlaceholdersAndImages(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(upstreamBody)
		if strings.Contains(string(raw), "jane.doe@example.com") {
			t.Fatalf("upstream received raw PII: %s", raw)
		}
		if !strings.Contains(string(raw), "https://example.com/receipt.png") {
			t.Fatalf("upstream did not preserve image URL: %s", raw)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "pii_image_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Image processed for [EMAIL_1]."},
			}},
			"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Models["default"] = ModelGroup{
		Strategy:  "static",
		PIIFilter: testPIIFilterConfig("redact_only"),
		Targets: []Target{{
			Provider:        "mock",
			Model:           "mock-model",
			InputModalities: []string{"text", "image"},
		}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
	  "model":"default",
	  "messages":[{
	    "role":"user",
	    "content":[
	      {"type":"text","text":"Receipt for jane.doe@example.com"},
	      {"type":"image_url","image_url":{"url":"https://example.com/receipt.png"}}
	    ]
	  }],
	  "max_tokens":64,
	  "stream":false
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "jane.doe@example.com") || !strings.Contains(rr.Body.String(), "[EMAIL_1]") {
		t.Fatalf("redact_only response=%s, want placeholder without original", rr.Body.String())
	}
	msgs := upstreamBody["messages"].([]any)
	parts := msgs[0].(map[string]any)["content"].([]any)
	textPart := parts[0].(map[string]any)
	imagePart := parts[1].(map[string]any)
	if text := textPart["text"].(string); !strings.Contains(text, "[EMAIL_1]") || strings.Contains(text, "jane.doe@example.com") {
		t.Fatalf("text part=%q, want redacted placeholder", text)
	}
	imageURL := imagePart["image_url"].(map[string]any)["url"].(string)
	if imageURL != "https://example.com/receipt.png" {
		t.Fatalf("image URL changed: %q", imageURL)
	}
}

func TestPIIFilterImageURLsRedactedOnlyWhenEnabled(t *testing.T) {
	tests := []struct {
		name        string
		imageURLs   bool
		wantURLPart string
		blockURL    string
	}{
		{
			name:        "disabled",
			imageURLs:   false,
			wantURLPart: "jane.doe@example.com",
			blockURL:    "[EMAIL_1]",
		},
		{
			name:        "enabled",
			imageURLs:   true,
			wantURLPart: "[EMAIL_1]",
			blockURL:    "jane.doe@example.com",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamBody map[string]any
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
					t.Fatal(err)
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"id": "pii_image_url_1",
					"choices": []map[string]any{{
						"message": map[string]any{"role": "assistant", "content": "ok"},
					}},
					"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 1, "total_tokens": 9},
				})
			}))
			defer upstream.Close()

			filter := testPIIFilterConfig("redact_only")
			filter.ApplyTo.ImageURLs = tc.imageURLs
			cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
			cfg.Models["default"] = ModelGroup{
				Strategy:  "static",
				PIIFilter: filter,
				Targets: []Target{{
					Provider:        "mock",
					Model:           "mock-model",
					InputModalities: []string{"text", "image"},
				}},
			}
			svc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()

			body := `{
			  "model":"default",
			  "messages":[{
			    "role":"user",
			    "content":[
			      {"type":"text","text":"Read this image."},
			      {"type":"image_url","image_url":{"url":"https://example.com/jane.doe@example.com/receipt.png"}}
			    ]
			  }],
			  "max_tokens":64,
			  "stream":false
			}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			msgs := upstreamBody["messages"].([]any)
			parts := msgs[0].(map[string]any)["content"].([]any)
			imagePart := parts[1].(map[string]any)
			imageURL := imagePart["image_url"].(map[string]any)["url"].(string)
			if !strings.Contains(imageURL, tc.wantURLPart) || strings.Contains(imageURL, tc.blockURL) {
				t.Fatalf("image URL=%q, want %q and not %q", imageURL, tc.wantURLPart, tc.blockURL)
			}
		})
	}
}

func TestScriptTargetsIncludeProviderMetadataWithoutRawKeys(t *testing.T) {
	targets := []Target{{
		Provider:    "mock",
		ModelRef:    "small",
		Model:       "mock-small",
		Weight:      7,
		Tier:        "cheap",
		RPM:         12,
		Cost:        3,
		Dialect:     "openai-responses",
		DisplayName: "Mock Small",
	}}
	providers := map[string]ProviderConfig{
		"mock": {
			BaseURL:   "https://mock.example/v1",
			Dialect:   "openai-chat",
			APIKey:    "raw-secret-key",
			APIKeyEnv: "MOCK_API_KEY",
			KeyID:     "mock-key",
		},
	}
	scriptTargets := buildScriptTargets(targets, providers)
	if len(scriptTargets) != 1 {
		t.Fatalf("script target count=%d", len(scriptTargets))
	}
	got := scriptTargets[0]
	if got.Model != "mock-small" || got.ModelRef != "small" || got.BaseURL != "https://mock.example/v1" || got.Dialect != "openai-responses" {
		t.Fatalf("metadata not populated: %#v", got)
	}
	if got.Weight != 7 || got.KeyID != "mock-key" || got.APIKeyEnv != "MOCK_API_KEY" || !got.KeyConfigured {
		t.Fatalf("key/weight metadata not populated: %#v", got)
	}
	raw, err := json.Marshal(scriptTargets)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "raw-secret-key") {
		t.Fatalf("raw API key leaked into script context: %s", raw)
	}
}

func TestScriptCallerMetadataExcludesSecrets(t *testing.T) {
	sum := sha256.Sum256([]byte(testToken))
	caller := &callerRuntime{cfg: CallerConfig{
		ID:          "alice",
		OwnerUser:   "Alice",
		Project:     "Metrum Insights",
		Environment: "Prod",
		TokenSHA256: hex.EncodeToString(sum[:]),
		TokenID:     "rtr_metrum_alice_metrum-insights_prod_key1",
		Allow:       []string{"default"},
	}, keyStatus: "active", membershipRole: "developer"}

	scriptCaller := buildScriptCaller(caller, caller.cfg.TokenID)
	if scriptCaller == nil || scriptCaller.TokenID != caller.cfg.TokenID || scriptCaller.User != "alice" || scriptCaller.OwnerUser != "alice" || scriptCaller.Username != "alice" || scriptCaller.Project != "metrum-insights" || scriptCaller.Environment != "prod" || scriptCaller.MembershipRole != "developer" || scriptCaller.KeyStatus != "active" {
		t.Fatalf("caller metadata not populated: %#v", scriptCaller)
	}
	raw, err := json.Marshal(scriptCaller)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testToken) || strings.Contains(string(raw), caller.cfg.TokenSHA256) {
		t.Fatalf("caller secret leaked into script context: %s", raw)
	}
}

func TestModelRefTargetUsesResolvedExternalModel(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "model_ref_1",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "resolved"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{
		BaseURL: upstream.URL + "/v1",
		Dialect: "openai",
		APIKey:  "provider-key",
		Models: map[string]ProviderModel{
			"small": {Model: "mock-small"},
		},
	}
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "small"}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotModel != "mock-small" {
		t.Fatalf("upstream model=%q", gotModel)
	}
}

func TestToolRequestsRequireMatchingToolSupportMetadata(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{
		BaseURL: "http://127.0.0.1:1/v1",
		Dialect: "openai-responses",
		APIKey:  "provider-key",
		Models: map[string]ProviderModel{
			"chat-tools-only": {
				Model: "chat-tools-only",
				ToolSupport: ToolSupport{
					OpenAIChat: []string{"tools", "tool_choice"},
				},
			},
			"responses-tools": {
				Model: "responses-tools",
				ToolSupport: ToolSupport{
					OpenAIResponses: []string{"function"},
				},
			},
		},
	}
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "chat-tools-only", ToolOnly: true}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.supportedToolsForGroup("default"); len(got) != 0 {
		t.Fatalf("advertised tools for incompatible metadata: %#v", got)
	}
	svc.Close()

	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", ModelRef: "responses-tools", ToolOnly: true}}}
	svc, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if got := svc.supportedToolsForGroup("default"); len(got) == 0 {
		t.Fatal("expected tools for matching responses metadata")
	}
}

func TestOpenRouterAnthropicSkinUsesBearerAuthAndMessagesPath(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey, gotVersion, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-API-Key")
		gotVersion = r.Header.Get("Anthropic-Version")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            "msg_openrouter",
			"type":          "message",
			"role":          "assistant",
			"model":         gotModel,
			"content":       []map[string]any{{"type": "text", "text": "openrouter anthropic ok"}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["openrouter_anthropic"] = ProviderConfig{
		BaseURL:    upstream.URL + "/api",
		Dialect:    "anthropic",
		AuthScheme: "bearer",
		APIKey:     "openrouter-key",
		Models: map[string]ProviderModel{
			"qwen3-coder-30b-nitro": {
				Model:            "qwen/qwen3-coder-30b-a3b-instruct:nitro",
				Weight:           1,
				InputModalities:  []string{"text", "image"},
				OutputModalities: []string{"text"},
				ToolSupport:      ToolSupport{AnthropicMessages: []string{"client_tools"}},
			},
		},
	}
	cfg.Models["openrouter-anthropic"] = ModelGroup{
		Strategy: "static",
		Targets:  []Target{{Provider: "openrouter_anthropic", ModelRef: "qwen3-coder-30b-nitro"}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "openrouter-anthropic")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"openrouter-anthropic","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/messages" {
		t.Fatalf("unexpected path %s", gotPath)
	}
	if gotAuth != "Bearer openrouter-key" || gotAPIKey != "" {
		t.Fatalf("unexpected auth headers Authorization=%q X-API-Key=%q", gotAuth, gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("missing Anthropic-Version, got %q", gotVersion)
	}
	if gotModel != "qwen/qwen3-coder-30b-a3b-instruct:nitro" {
		t.Fatalf("upstream model=%q", gotModel)
	}
}

func TestAnthropicToolPassthroughAppliesDefaultThinking(t *testing.T) {
	var gotThinking map[string]any
	var gotToolChoice any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotThinking, _ = body["thinking"].(map[string]any)
		gotToolChoice = body["tool_choice"]
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "msg_kimi",
			"type":        "message",
			"role":        "assistant",
			"model":       body["model"],
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["kimi_anthropic"] = ProviderConfig{BaseURL: upstream.URL + "/anthropic", Dialect: "anthropic", AuthScheme: "bearer", APIKey: "kimi-key"}
	cfg.Models["kimi-tools"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:        "kimi_anthropic",
		Model:           "kimi-k2.7-code",
		ToolOnly:        true,
		DefaultThinking: map[string]any{"type": "enabled", "budget_tokens": 512},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "kimi-tools")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"kimi-tools","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"echo","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"echo"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotThinking["type"] != "enabled" || gotThinking["budget_tokens"].(float64) != 512 {
		t.Fatalf("thinking=%#v, want enabled budget 512", gotThinking)
	}
	if gotToolChoice != nil {
		t.Fatalf("tool_choice=%#v, want omitted for Kimi thinking compatibility", gotToolChoice)
	}
}

func TestResponsesToolPassthroughCanUseMiniMaxTarget(t *testing.T) {
	var gotPath, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_minimax",
			"object":      "response",
			"status":      "completed",
			"model":       gotModel,
			"output_text": "ok",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["minimax"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "minimax-key"}
	cfg.Models["agent-tools-smoke"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "minimax", Model: "MiniMax-M3", Dialect: "openai-responses", ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "agent-tools-smoke")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"agent-tools-smoke","input":"hi","tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/v1/responses" || gotModel != "MiniMax-M3" {
		t.Fatalf("path/model=%s/%s, want /v1/responses MiniMax-M3", gotPath, gotModel)
	}
}

func TestResponsesToolPassthroughRequiresExplicitTargetSupport(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		writeJSON(w, http.StatusOK, map[string]any{"id": "unexpected"})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["responses"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-responses", APIKey: "provider-key"}
	cfg.Models["responses-no-tools"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "responses", Model: "responses-plain"}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "responses-no-tools")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"responses-no-tools","input":"hi","tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) {
		t.Fatalf("body=%s, want no-eligible-target", rr.Body.String())
	}
	if upstreamCalled {
		t.Fatal("upstream called for Responses tool request without explicit tool support")
	}
}

func TestResponsesToolPassthroughCanUseOpenRouterResponsesTarget(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "resp_openrouter",
			"object":      "response",
			"status":      "completed",
			"model":       gotModel,
			"output_text": "ok",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["openrouter_responses"] = ProviderConfig{
		BaseURL: upstream.URL + "/api/v1",
		Dialect: "openai-responses",
		APIKey:  "openrouter-key",
		Models: map[string]ProviderModel{
			"qwen3-coder-30b-nitro": {Model: "qwen/qwen3-coder-30b-a3b-instruct:nitro", Weight: 1, ToolSupport: ToolSupport{OpenAIResponses: []string{"function"}}},
		},
	}
	cfg.Models["agent-tools-smoke-openrouter"] = ModelGroup{
		Strategy: "static",
		Targets:  []Target{{Provider: "openrouter_responses", ModelRef: "qwen3-coder-30b-nitro", Dialect: "openai-responses"}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "agent-tools-smoke-openrouter")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"agent-tools-smoke-openrouter","input":"hi","tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/responses" || gotModel != "qwen/qwen3-coder-30b-a3b-instruct:nitro" {
		t.Fatalf("path/model=%s/%s, want /api/v1/responses qwen/qwen3-coder-30b-a3b-instruct:nitro", gotPath, gotModel)
	}
	if gotAuth != "Bearer openrouter-key" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
}

func TestAnthropicToolPassthroughCanUseOpenRouterAnthropicTarget(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey, gotVersion, gotModel string
	var gotContent []any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-API-Key")
		gotVersion = r.Header.Get("Anthropic-Version")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = body["model"].(string)
		if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			if msg, ok := messages[0].(map[string]any); ok {
				gotContent, _ = msg["content"].([]any)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          "msg_openrouter_tool",
			"type":        "message",
			"role":        "assistant",
			"model":       gotModel,
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "unused", t.TempDir())
	cfg.Provider["openrouter_anthropic"] = ProviderConfig{
		BaseURL:    upstream.URL + "/api",
		Dialect:    "anthropic",
		AuthScheme: "bearer",
		APIKey:     "openrouter-key",
		Models: map[string]ProviderModel{
			"qwen3-coder-30b-nitro": {Model: "qwen/qwen3-coder-30b-a3b-instruct:nitro", Weight: 1},
		},
	}
	cfg.Models["claude-tools-smoke-openrouter"] = ModelGroup{
		Strategy: "static",
		Targets: []Target{{
			Provider:         "openrouter_anthropic",
			ModelRef:         "qwen3-coder-30b-nitro",
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text"},
			ToolSupport:      ToolSupport{AnthropicMessages: []string{"client_tools"}},
		}},
	}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "claude-tools-smoke-openrouter")
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"claude-tools-smoke-openrouter","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"` + receiptImageURL + `"}}]}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/messages" || gotModel != "qwen/qwen3-coder-30b-a3b-instruct:nitro" {
		t.Fatalf("path/model=%s/%s, want /api/v1/messages qwen/qwen3-coder-30b-a3b-instruct:nitro", gotPath, gotModel)
	}
	if gotAuth != "Bearer openrouter-key" || gotAPIKey != "" {
		t.Fatalf("unexpected auth headers Authorization=%q X-API-Key=%q", gotAuth, gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("missing Anthropic-Version, got %q", gotVersion)
	}
	if len(gotContent) != 2 {
		t.Fatalf("forwarded content=%#v, want text and image blocks", gotContent)
	}
	imageBlock, _ := gotContent[1].(map[string]any)
	source, _ := imageBlock["source"].(map[string]any)
	if imageBlock["type"] != "image" || source["type"] != "url" || source["url"] != receiptImageURL {
		t.Fatalf("forwarded image block=%#v", imageBlock)
	}
}

func TestNoEligibleTargetReturnsActionableError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when no target supports the request shape")
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Provider["mock"] = ProviderConfig{BaseURL: upstream.URL + "/v1", Dialect: "openai-chat", APIKey: "provider-key"}
	cfg.Models["vision"] = ModelGroup{Strategy: "static", Targets: []Target{{
		Provider:        "mock",
		Model:           "vision-chat-only",
		InputModalities: []string{"text", "image"},
		ToolSupport:     ToolSupport{OpenAIChat: []string{"tools"}},
	}}}
	cfg.Callers[0].Allow = append(cfg.Callers[0].Allow, "vision")

	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{
		"model":"vision",
		"max_tokens":512,
		"tools":[{"name":"noop","description":"No-op","input_schema":{"type":"object","properties":{}}}],
		"messages":[{"role":"user","content":[
			{"type":"text","text":"Read the image."},
			{"type":"image","source":{"type":"url","url":"` + receiptImageURL + `"}}
		]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) ||
		!strings.Contains(rr.Body.String(), `anthropic_tool_passthrough`) ||
		!strings.Contains(rr.Body.String(), `image`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
}

func TestUpstreamFailureReturnsActionableError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary outage", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"upstream-failed"`) ||
		!strings.Contains(rr.Body.String(), `"attempts":1`) ||
		!strings.Contains(rr.Body.String(), `"provider":"mock"`) ||
		!strings.Contains(rr.Body.String(), `upstream status 503`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
}

func TestUpstreamStatusClassifiesQuotaRateBillingFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantClass string
		wantRetry bool
		forbidden []string
	}{
		{
			name:      "openrouter credits exhausted",
			status:    http.StatusTooManyRequests,
			body:      `{"error":{"message":"This request requires more credits than your account balance allows for account acct_live_secret","code":429}}`,
			wantClass: "upstream_quota_exhausted",
			wantRetry: true,
			forbidden: []string{"acct_live_secret", "account balance allows"},
		},
		{
			name:      "insufficient balance",
			status:    http.StatusPaymentRequired,
			body:      `{"error":{"message":"insufficient balance for provider account provider_account_123"}}`,
			wantClass: "upstream_quota_exhausted",
			wantRetry: true,
			forbidden: []string{"provider_account_123", "insufficient balance"},
		},
		{
			name:      "quota exceeded",
			status:    http.StatusForbidden,
			body:      `{"error":{"type":"quota_exceeded","message":"monthly quota exceeded"}}`,
			wantClass: "upstream_quota_exhausted",
			wantRetry: true,
			forbidden: []string{"monthly quota exceeded"},
		},
		{
			name:      "billing disabled",
			status:    http.StatusForbidden,
			body:      `{"error":{"code":"billing_disabled","message":"billing disabled for customer cust_secret"}}`,
			wantClass: "upstream_quota_exhausted",
			wantRetry: true,
			forbidden: []string{"cust_secret", "billing disabled for customer"},
		},
		{
			name:      "ordinary rate limit",
			status:    http.StatusTooManyRequests,
			body:      `{"error":{"message":"rate limit exceeded"}}`,
			wantClass: "upstream_rate_limited",
			wantRetry: true,
		},
		{
			name:      "ordinary upstream 5xx",
			status:    http.StatusBadGateway,
			body:      `{"error":{"message":"temporary outage"}}`,
			wantClass: "upstream_status_5xx",
			wantRetry: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyUpstreamStatus(tc.status, []byte(tc.body))
			if got.Class != tc.wantClass || got.Retryable != tc.wantRetry || got.StatusCode != tc.status {
				t.Fatalf("classified=%#v, want class=%s retry=%v status=%d", got, tc.wantClass, tc.wantRetry, tc.status)
			}
			if tc.wantClass == "upstream_quota_exhausted" {
				if !strings.Contains(got.Message, "quota, credits, or billing") {
					t.Fatalf("message=%q, want actionable quota/billing text", got.Message)
				}
				for _, forbidden := range tc.forbidden {
					if strings.Contains(got.Message, forbidden) {
						t.Fatalf("message leaked %q: %s", forbidden, got.Message)
					}
				}
			}
		})
	}
}

func TestUpstreamQuotaExhaustionReturnsSanitizedErrorAcrossSurfaces(t *testing.T) {
	const (
		accountID   = "acct_provider_secret_123"
		providerKey = "sk-provider-secret-1234567890"
		tokenHash   = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	)
	cases := []struct {
		name string
		path string
		body string
	}{
		{
			name: "chat",
			path: "/v1/chat/completions",
			body: `{"model":"default","messages":[{"role":"user","content":"hi"}]}`,
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: `{"model":"default","input":"hi","max_output_tokens":16}`,
		},
		{
			name: "messages",
			path: "/v1/messages",
			body: `{"model":"default","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusPaymentRequired, map[string]any{
					"error": map[string]any{
						"message":          "insufficient credits for provider account " + accountID,
						"provider_api_key": providerKey,
						"token_hash":       tokenHash,
						"raw_body":         "caller prompt should not appear",
					},
				})
			}))
			defer upstream.Close()

			svc := newTestService(t, upstream.URL, "provider-key")
			defer svc.Close()

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+testToken)
			rr := httptest.NewRecorder()
			svc.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			for _, want := range []string{`"type":"upstream-quota-exhausted"`, `"request_id"`, "quota, credits, or billing"} {
				if !strings.Contains(body, want) {
					t.Fatalf("body missing %q: %s", want, body)
				}
			}
			for _, forbidden := range []string{accountID, providerKey, tokenHash, "caller prompt should not appear", "insufficient credits for provider account"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("body leaked %q: %s", forbidden, body)
				}
			}

			var attempts []requestAttemptRecord
			if err := svc.usage.db.Find(&attempts).Error; err != nil {
				t.Fatal(err)
			}
			if len(attempts) != 1 || attempts[0].ErrorClass != "upstream_quota_exhausted" || !attempts[0].Retryable {
				t.Fatalf("unexpected attempts: %#v", attempts)
			}
			for _, forbidden := range []string{accountID, providerKey, tokenHash, "caller prompt should not appear"} {
				if strings.Contains(attempts[0].ErrorMessage, forbidden) {
					t.Fatalf("attempt leaked %q: %#v", forbidden, attempts[0])
				}
			}
			var errors []requestErrorRecord
			if err := svc.usage.db.Find(&errors).Error; err != nil {
				t.Fatal(err)
			}
			if len(errors) != 1 || errors[0].ErrorType != "upstream-quota-exhausted" || errors[0].Status != http.StatusServiceUnavailable || !errors[0].Retryable {
				t.Fatalf("unexpected error rows: %#v", errors)
			}
		})
	}
}

func TestUpstreamQuotaExhaustionFallsBackAndRecordsAttempt(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch stringValue(body["model"]) {
		case "credit-empty":
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": map[string]any{
					"message":    "OpenRouter credits exhausted for account acct_live_secret",
					"account_id": "acct_live_secret",
				},
			})
		case "healthy-model":
			writeJSON(w, http.StatusOK, map[string]any{
				"id": "up_fallback_success",
				"choices": []map[string]any{{
					"message":       map[string]any{"role": "assistant", "content": "fallback ok"},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
		default:
			t.Fatalf("unexpected upstream model %v", body["model"])
		}
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{
		{Provider: "mock", Model: "credit-empty"},
		{Provider: "mock", Model: "healthy-model"},
	}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "fallback ok") || strings.Contains(rr.Body.String(), "acct_live_secret") {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}

	var attempts []requestAttemptRecord
	if err := svc.usage.db.Order("attempt_index ASC").Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempt rows=%d: %#v", len(attempts), attempts)
	}
	if attempts[0].ErrorClass != "upstream_quota_exhausted" || attempts[0].FallbackReason != "upstream_quota_exhausted" || !attempts[0].Retryable || attempts[0].Selected {
		t.Fatalf("unexpected failed attempt: %#v", attempts[0])
	}
	if strings.Contains(attempts[0].ErrorMessage, "acct_live_secret") || strings.Contains(attempts[0].ErrorMessage, "OpenRouter credits exhausted") {
		t.Fatalf("failed attempt leaked upstream body: %#v", attempts[0])
	}
	if !attempts[1].Selected || attempts[1].ErrorClass != "" || attempts[1].Model != "healthy-model" {
		t.Fatalf("unexpected fallback attempt: %#v", attempts[1])
	}
	var rows []usageRecord
	if err := svc.usage.db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != http.StatusOK || !rows[0].FallbackUsed || rows[0].Attempts != 2 || rows[0].Error != "" {
		t.Fatalf("unexpected usage rows: %#v", rows)
	}
	var errors []requestErrorRecord
	if err := svc.usage.db.Find(&errors).Error; err != nil {
		t.Fatal(err)
	}
	if len(errors) != 0 {
		t.Fatalf("unexpected error rows for fallback success: %#v", errors)
	}
}

func TestUpstreamFailureResponseDoesNotLabelMixedAttemptsAsQuota(t *testing.T) {
	rc := &requestContext{rec: logRecord{AttemptsDetail: []attemptLogRecord{
		{ErrorClass: "upstream_timeout"},
		{ErrorClass: "upstream_quota_exhausted"},
	}}}
	code, status := upstreamFailureResponse(upstreamError{Class: "upstream_quota_exhausted"}, rc)
	if code != "upstream-failed" || status != http.StatusBadGateway {
		t.Fatalf("mixed attempts response code=%s status=%d", code, status)
	}

	rc = &requestContext{rec: logRecord{AttemptsDetail: []attemptLogRecord{
		{ErrorClass: "upstream_quota_exhausted"},
		{ErrorClass: "upstream_quota_exhausted"},
	}}}
	code, status = upstreamFailureResponse(upstreamError{Class: "upstream_quota_exhausted"}, rc)
	if code != "upstream-quota-exhausted" || status != http.StatusServiceUnavailable {
		t.Fatalf("quota attempts response code=%s status=%d", code, status)
	}
}

func TestDiagnosticsSanitizeUpstreamErrorBeforePersistence(t *testing.T) {
	const (
		echoedPrompt = "prompt-like user text: summarize confidential launch notes"
		bearerToken  = "Bearer fake-provider-bearer-token-1234567890"
		providerKey  = "sk-fake-provider-key-1234567890"
		tokenHash    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		nestedDetail = "nested upstream body detail"
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"message": "provider echoed request context",
				"details": map[string]any{
					"prompt":           echoedPrompt,
					"authorization":    bearerToken,
					"provider_api_key": providerKey,
					"token_hash":       tokenHash,
					"nested": map[string]any{
						"body": nestedDetail,
					},
				},
			},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.Diagnostics.StoreSanitizedUpstreamError = true
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"please do not persist this prompt"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var attempts []requestAttemptRecord
	if err := svc.usage.db.Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	var traces []requestTraceEventRecord
	if err := svc.usage.db.Find(&traces).Error; err != nil {
		t.Fatal(err)
	}
	var errors []requestErrorRecord
	if err := svc.usage.db.Find(&errors).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || len(errors) != 1 {
		t.Fatalf("diagnostic rows attempts=%d errors=%d", len(attempts), len(errors))
	}
	foundFailedTrace := false
	for _, trace := range traces {
		if trace.Event == "upstream_attempt_failed" {
			foundFailedTrace = true
			if !strings.Contains(trace.Message, "upstream error body redacted") {
				t.Fatalf("failed trace message was not redacted: %q", trace.Message)
			}
		}
	}
	if !foundFailedTrace {
		t.Fatalf("upstream_attempt_failed trace not found: %#v", traces)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	diagnosticsText := string(raw)
	for _, attempt := range attempts {
		diagnosticsText += "\n" + attempt.ErrorMessage
	}
	for _, trace := range traces {
		diagnosticsText += "\n" + trace.Message
	}
	for _, rec := range errors {
		diagnosticsText += "\n" + rec.ErrorMessage
	}
	for _, forbidden := range []string{echoedPrompt, bearerToken, providerKey, tokenHash, nestedDetail, "please do not persist this prompt"} {
		if strings.Contains(diagnosticsText, forbidden) {
			t.Fatalf("diagnostics leaked %q in: %s", forbidden, diagnosticsText)
		}
	}
	for _, want := range []string{`"trace_events"`, `"attempts_detail"`, "upstream error body redacted"} {
		if !strings.Contains(diagnosticsText, want) {
			t.Fatalf("sanitized diagnostics missing %q in: %s", want, diagnosticsText)
		}
	}
}

func TestDiagnosticsSanitizeTruncatedUpstreamErrorBeforePersistence(t *testing.T) {
	const (
		echoedPrompt = "truncated prompt-like user text"
		bearerToken  = "Bearer truncated-provider-bearer-token-1234567890"
		providerKey  = "sk-truncated-provider-key-1234567890"
		tokenHash    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	bodyPrefix := `{"error":{"message":"provider echoed request context","prompt":"` + echoedPrompt + `","authorization":"` + bearerToken + `","token_hash":"` + tokenHash + `","provider_api_key":"` + providerKey + `","tail":"`
	upstreamBody := bodyPrefix + strings.Repeat("x", 2048)
	maxErrorBytes := len(bodyPrefix) + 32
	if json.Valid([]byte(upstreamBody[:maxErrorBytes])) {
		t.Fatal("truncated upstream body unexpectedly valid JSON")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(upstreamBody))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.Diagnostics.StoreSanitizedUpstreamError = true
	cfg.Server.Diagnostics.MaxErrorBytes = maxErrorBytes
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"do not persist caller prompt"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var attempts []requestAttemptRecord
	if err := svc.usage.db.Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	var traces []requestTraceEventRecord
	if err := svc.usage.db.Find(&traces).Error; err != nil {
		t.Fatal(err)
	}
	var errors []requestErrorRecord
	if err := svc.usage.db.Find(&errors).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || len(errors) != 1 {
		t.Fatalf("diagnostic rows attempts=%d errors=%d", len(attempts), len(errors))
	}

	raw, err := os.ReadFile(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	diagnosticsText := string(raw)
	for _, attempt := range attempts {
		diagnosticsText += "\n" + attempt.ErrorMessage
	}
	for _, trace := range traces {
		diagnosticsText += "\n" + trace.Message
	}
	for _, rec := range errors {
		diagnosticsText += "\n" + rec.ErrorMessage
	}
	for _, forbidden := range []string{echoedPrompt, bearerToken, providerKey, tokenHash, "do not persist caller prompt"} {
		if strings.Contains(diagnosticsText, forbidden) {
			t.Fatalf("diagnostics leaked %q in: %s", forbidden, diagnosticsText)
		}
	}
	for _, want := range []string{`"prompt":[REDACTED]`, `"authorization":[REDACTED]`, `"token_hash":[REDACTED]`, `"provider_api_key":[REDACTED]`} {
		if !strings.Contains(diagnosticsText, want) {
			t.Fatalf("sanitized diagnostics missing %q in: %s", want, diagnosticsText)
		}
	}
}

func TestConfiguredDefaultModelGroupHandlesOmittedModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_default",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "configured default ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.DefaultModelGroup = "example-basic"
	cfg.Models = map[string]ModelGroup{
		"example-basic": {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "mock-model"}}},
	}
	cfg.Callers[0].Allow = []string{"example-basic"}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "configured default ok") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestOmittedModelWithoutConfiguredDefaultReturnsMissingModel(t *testing.T) {
	svc := newTestService(t, "http://127.0.0.1:1", "provider-key")
	defer svc.Close()
	svc.cfg.Server.DefaultModelGroup = ""

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "missing-model") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestContentCaptureDisabledByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_default_capture_disabled",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "metadata only"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"do not capture this by default"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := svc.usage.db.Model(&contentCaptureRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("content capture rows=%d, want 0", count)
	}
}

func TestContentCaptureRequestPayloadDropsRawForScopeOptOuts(t *testing.T) {
	req := &IRRequest{
		Model: "default",
		Messages: []IRMessage{{
			Role: "user",
			Parts: []IRContentPart{{
				Type:     "image",
				ImageURL: "data:image/png;base64,RAW_NORMALIZED_IMAGE_DATA",
			}},
		}},
		Tools: []map[string]any{{"type": "function", "function": map[string]any{"name": "raw_tool"}}},
		Raw: map[string]any{
			"tools": []any{map[string]any{"function": map[string]any{"name": "raw_tool"}}},
			"messages": []any{map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,RAW_OPENAI_IMAGE_DATA"}},
					map[string]any{"type": "image", "source": map[string]any{"type": "base64", "data": "RAW_ANTHROPIC_IMAGE_DATA"}},
				},
			}},
		},
	}
	payload := contentCaptureRequestPayload(req, ContentCaptureConfig{CaptureImages: false, CaptureToolCalls: false})
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"\"raw\"", "\"tools\"", "raw_tool", "RAW_NORMALIZED_IMAGE_DATA", "RAW_OPENAI_IMAGE_DATA", "RAW_ANTHROPIC_IMAGE_DATA"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("request capture payload leaked %q in %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "[IMAGE_REDACTED]") {
		t.Fatalf("request capture payload did not retain image redaction marker: %s", text)
	}
}

func TestContentCaptureStoresRedactedRequestResponseAndAllowedHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_capture_enabled",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "send result to bob@example.com with sk-test-secret"},
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
		})
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.ContentCapture = ContentCaptureConfig{
		Enabled:                 true,
		RetentionDays:           7,
		CaptureRequest:          true,
		CaptureResponse:         true,
		CaptureHeadersAllowlist: []string{"User-Agent", "X-Trace-Id"},
		RedactionPatterns:       []ContentCaptureRedactionRule{{Name: "email", Expression: `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`}},
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"email alice@example.com and use Bearer rtr_should_not_store_secret"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("User-Agent", "capture-test")
	req.Header.Set("X-Trace-Id", "trace-123")
	req.Header.Set("X-API-Key", "do-not-store")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var rows []contentCaptureRecord
	if err := svc.usage.db.Order("scope").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("capture rows=%d, want 2: %#v", len(rows), rows)
	}
	joined := rows[0].ContentText + "\n" + rows[1].ContentText
	for _, forbidden := range []string{testToken, cfg.Callers[0].TokenSHA256, "alice@example.com", "bob@example.com", "sk-test-secret", "rtr_should_not_store_secret", "do-not-store"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("captured content leaked %q in %s", forbidden, joined)
		}
	}
	for _, want := range []string{"[REDACTED_EMAIL]", "[REDACTED_SECRET]"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("captured content missing redaction marker %q in %s", want, joined)
		}
	}
	for _, row := range rows {
		if row.RequestID == "" || row.CallerID != "alice" || row.TokenID != "rtr_alice_test" {
			t.Fatalf("capture row not joinable/safe: %#v", row)
		}
		if row.RetentionUntil == "" {
			t.Fatalf("capture row missing retention_until: %#v", row)
		}
	}
	var headers []contentCaptureHeaderRecord
	if err := svc.usage.db.Order("name").Find(&headers).Error; err != nil {
		t.Fatal(err)
	}
	if len(headers) != 2 {
		t.Fatalf("header rows=%d, want 2: %#v", len(headers), headers)
	}
	headerText := ""
	for _, h := range headers {
		headerText += h.Name + "=" + h.Value + "\n"
	}
	if !strings.Contains(headerText, "User-Agent=capture-test") || !strings.Contains(headerText, "X-Trace-Id=trace-123") {
		t.Fatalf("allowed headers not captured: %s", headerText)
	}
	if strings.Contains(headerText, "Authorization") || strings.Contains(headerText, "X-Api-Key") {
		t.Fatalf("forbidden header captured: %s", headerText)
	}
}

func TestContentCaptureStoresSanitizedUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"prompt: secret body Authorization: Bearer sk-leaky-secret"}}`))
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.ContentCapture = ContentCaptureConfig{
		Enabled:               true,
		RetentionDays:         7,
		CaptureUpstreamErrors: true,
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"trigger upstream failure"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var row contentCaptureRecord
	if err := svc.usage.db.Where("scope = ?", contentCaptureScopeUpstreamError).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.SourceStatus != http.StatusBadGateway {
		t.Fatalf("source status=%d", row.SourceStatus)
	}
	for _, forbidden := range []string{"sk-leaky-secret", "secret body"} {
		if strings.Contains(row.ContentText, forbidden) {
			t.Fatalf("upstream error capture leaked %q in %s", forbidden, row.ContentText)
		}
	}
	if !strings.Contains(row.ContentText, "upstream error body redacted") {
		t.Fatalf("upstream error capture missing sanitized marker: %s", row.ContentText)
	}
}

func TestContentCaptureAdminDeleteRequiresContentAdminAndAudits(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_capture_delete",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "captured"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	adminToken := "rtr_content_admin_test_token"
	adminHash := sha256.Sum256([]byte(adminToken))
	dir := t.TempDir()
	cfg := testConfig(t, upstream.URL, "provider-key", dir)
	cfg.Server.ContentCapture = ContentCaptureConfig{Enabled: true, RetentionDays: 7, CaptureRequest: true}
	cfg.Callers = append(cfg.Callers, CallerConfig{
		ID:           "content-admin",
		User:         "content-admin",
		Project:      "platform",
		Environment:  "test",
		TokenSHA256:  hex.EncodeToString(adminHash[:]),
		TokenID:      "rtr_content_admin_test",
		Allow:        []string{"default"},
		ContentAdmin: true,
		Rate:         RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
		Quota:        QuotaConfig{Day: BudgetConfig{Requests: 100, Tokens: 100000}, Month: BudgetConfig{Tokens: 1000000}, SoftPct: 80},
		Key:          KeyConfig{LifetimeTokens: 1000000, SoftPct: 90, OnExhaust: "disable"},
	})
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"capture me"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	requestID := rr.Header().Get("X-Request-Id")
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/content-captures/"+requestID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+testToken)
	deleteRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusForbidden {
		t.Fatalf("non-admin delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodDelete, "/v1/content-captures/"+requestID, nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(adminRR, adminReq)
	if adminRR.Code != http.StatusOK {
		t.Fatalf("admin delete status=%d body=%s", adminRR.Code, adminRR.Body.String())
	}
	var remaining int64
	if err := svc.usage.db.Model(&contentCaptureRecord{}).Where("request_id = ?", requestID).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("remaining captures=%d, want 0", remaining)
	}
	var audit contentCaptureAuditRecord
	if err := svc.usage.db.Where("action = ? AND request_id = ?", "request_delete", requestID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.ActorCallerID != "content-admin" || audit.ActorTokenID != "rtr_content_admin_test" || audit.RowsAffected != 1 {
		t.Fatalf("unexpected audit row: %#v", audit)
	}
}

func TestContentCaptureMaintenanceUsesCasbinAuthorization(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", dir)
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{
		Enabled: true,
		Policy: []string{
			"g, caller:content-policy, content_admin, platform/prod",
			"g, caller:reports-only, reports_admin, platform/prod",
			"g, caller:content-wrong-domain, content_admin, platform/prod",
			"p, content_admin, platform/prod, content:capture, delete|purge",
			"p, reports_admin, platform/prod, admin:reports, read|export|drilldown",
		},
	}
	contentToken := "rtr_content_policy_test_token"
	reportsToken := "rtr_reports_only_test_token"
	wrongDomainToken := "rtr_content_wrong_domain_test_token"
	for _, caller := range []struct {
		id, token, project, environment string
	}{
		{id: "content-policy", token: contentToken, project: "platform", environment: "prod"},
		{id: "reports-only", token: reportsToken, project: "platform", environment: "prod"},
		{id: "content-wrong-domain", token: wrongDomainToken, project: "platform", environment: "test"},
	} {
		sum := sha256.Sum256([]byte(caller.token))
		cfg.Callers = append(cfg.Callers, CallerConfig{
			ID:          caller.id,
			User:        caller.id,
			Project:     caller.project,
			Environment: caller.environment,
			TokenSHA256: hex.EncodeToString(sum[:]),
			TokenID:     caller.id,
			Allow:       []string{"default"},
			Rate:        RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
			Quota:       QuotaConfig{Day: BudgetConfig{Requests: 100, Tokens: 100000}, Month: BudgetConfig{Tokens: 1000000}, SoftPct: 80},
			Key:         KeyConfig{LifetimeTokens: 1000000, SoftPct: 90, OnExhaust: "disable"},
		})
	}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	unauth := httptest.NewRequest(http.MethodDelete, "/v1/content-captures/req_unauth", nil)
	unauthRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(unauthRR, unauth)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("unauth content delete status=%d body=%s", unauthRR.Code, unauthRR.Body.String())
	}

	for _, tc := range []struct {
		name  string
		token string
	}{
		{name: "ordinary caller", token: testToken},
		{name: "reports only", token: reportsToken},
		{name: "wrong domain", token: wrongDomainToken},
	} {
		req := httptest.NewRequest(http.MethodDelete, "/v1/content-captures/req_denied", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		rr := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "content-forbidden") {
			t.Fatalf("%s content delete status=%d body=%s", tc.name, rr.Code, rr.Body.String())
		}
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/content-captures/req_allowed", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+contentToken)
	deleteRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("policy content delete status=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	purgeReq := httptest.NewRequest(http.MethodPost, "/v1/content-captures/purge-expired", nil)
	purgeReq.Header.Set("Authorization", "Bearer "+contentToken)
	purgeRR := httptest.NewRecorder()
	svc.Handler().ServeHTTP(purgeRR, purgeReq)
	if purgeRR.Code != http.StatusOK {
		t.Fatalf("policy content purge status=%d body=%s", purgeRR.Code, purgeRR.Body.String())
	}
}

func TestMalformedCasbinPolicyFailsStartup(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.AdminAuth.Authorization = AdminAuthorizationConfig{
		Enabled: true,
		Policy:  []string{"p, content_admin, platform/prod, content:capture"},
	}
	svc, err := New(cfg)
	if err == nil {
		svc.Close()
		t.Fatal("New succeeded with malformed authorization policy")
	}
	if !strings.Contains(err.Error(), "p lines require subject, domain, object, action") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpstreamAttemptTimeoutReturnsGatewayTimeoutAndDiagnostics(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "late",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "late"},
			}},
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	group := cfg.Models["default"]
	group.AttemptTimeoutMS = 10
	cfg.Models["default"] = group
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"upstream-timeout"`) ||
		!strings.Contains(rr.Body.String(), `"request_id"`) {
		t.Fatalf("unexpected body=%s", rr.Body.String())
	}
	var attempts []requestAttemptRecord
	if err := svc.usage.db.Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt rows=%d", len(attempts))
	}
	if !attempts[0].TimedOut || attempts[0].ErrorClass != "upstream_timeout" || attempts[0].AttemptTimeoutMS != 10 {
		t.Fatalf("unexpected attempt row: %#v", attempts[0])
	}
	var errors []requestErrorRecord
	if err := svc.usage.db.Find(&errors).Error; err != nil {
		t.Fatal(err)
	}
	if len(errors) != 1 || errors[0].ErrorType != "upstream-timeout" || !errors[0].Retryable {
		t.Fatalf("unexpected error rows: %#v", errors)
	}
}

func TestDecisionTelemetryDisabledByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_decision_default",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	svc := newTestService(t, upstream.URL, "provider-key")
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertDecisionTelemetryCounts(t, svc, 0, 0, 0, 0, 0)
}

func TestDecisionTelemetryEnabledRecordsTextCandidateAndDecision(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_decision_enabled",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.DecisionTelemetry.Enabled = true
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertDecisionTelemetryCounts(t, svc, 10, 1, 0, 1, 1)
	var selected int64
	if err := svc.usage.db.Model(&decisionTargetCandidateRecord{}).Where("selected = ?", true).Count(&selected).Error; err != nil {
		t.Fatal(err)
	}
	if selected != 1 {
		t.Fatalf("selected candidate rows=%d, want 1", selected)
	}
	var decision routingDecisionRecord
	if err := svc.usage.db.First(&decision).Error; err != nil {
		t.Fatal(err)
	}
	if decision.Strategy != "static" || decision.Provider != "mock" || decision.Model != "mock-model" || decision.SelectedCandidateIndex != 0 {
		t.Fatalf("unexpected routing decision: %#v", decision)
	}
}

func TestDecisionTelemetryRecordsNoEligibleFilterReason(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.DecisionTelemetry.Enabled = true
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "tool-only", ToolOnly: true}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertDecisionTelemetryCounts(t, svc, 10, 1, 1, 0, 0)
	assertDecisionFilterReason(t, svc, "tool-only-target")
}

func TestDecisionTelemetryRecordsToolSupportFilterReason(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "provider-key", t.TempDir())
	cfg.Server.DecisionTelemetry.Enabled = true
	cfg.Models["default"] = ModelGroup{Strategy: "static", Targets: []Target{{Provider: "mock", Model: "plain-chat"}}}
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	body := `{"model":"default","messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), `"type":"no-eligible-target"`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertDecisionTelemetryCounts(t, svc, 10, 1, 1, 0, 0)
	assertDecisionFilterReason(t, svc, "tool-support")
}

func TestDecisionTelemetryRecordsCacheBypassReason(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "up_cache_bypass",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer upstream.Close()
	cfg := testConfig(t, upstream.URL, "provider-key", t.TempDir())
	cfg.Server.DecisionTelemetry.Enabled = true
	svc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"default","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Cache-Control", "no-cache")
	rr := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var reason decisionCacheReasonRecord
	if err := svc.usage.db.First(&reason).Error; err != nil {
		t.Fatal(err)
	}
	if reason.Status != "bypass" || reason.Reason != "cache-request-no-cache" {
		t.Fatalf("unexpected cache reason: %#v", reason)
	}
}

func assertDecisionTelemetryCounts(t *testing.T, svc *Service, shape, candidates, filters, decisions, cacheReasons int64) {
	t.Helper()
	got := []struct {
		name  string
		want  int64
		model any
	}{
		{name: "shape", want: shape, model: &decisionShapeFeatureRecord{}},
		{name: "candidates", want: candidates, model: &decisionTargetCandidateRecord{}},
		{name: "filters", want: filters, model: &decisionTargetFilterReasonRecord{}},
		{name: "decisions", want: decisions, model: &routingDecisionRecord{}},
		{name: "cache reasons", want: cacheReasons, model: &decisionCacheReasonRecord{}},
	}
	for _, item := range got {
		var count int64
		if err := svc.usage.db.Model(item.model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != item.want {
			t.Fatalf("%s rows=%d, want %d", item.name, count, item.want)
		}
	}
}

func assertDecisionFilterReason(t *testing.T, svc *Service, want string) {
	t.Helper()
	var count int64
	if err := svc.usage.db.Model(&decisionTargetFilterReasonRecord{}).Where("reason = ?", want).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("filter reason %q count=%d, want 1", want, count)
	}
}

const testToken = "rtr_test_token"

func newTestService(t *testing.T, upstreamURL, providerKey string) *Service {
	t.Helper()
	svc, err := New(testConfig(t, upstreamURL, providerKey, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func testConfig(t *testing.T, upstreamURL, providerKey, dir string) *Config {
	t.Helper()
	sum := sha256.Sum256([]byte(testToken))
	return &Config{
		Server: ServerConfig{
			Listen:            ":0",
			DefaultModelGroup: "default",
			Cache:             CacheConfig{Enabled: true, MaxBytes: 1 << 20, DefaultTTL: 0},
			Logging: LoggingConfig{
				Path: filepath.Join(dir, "requests.jsonl"),
			},
		},
		StatePath: filepath.Join(dir, "state.json"),
		Provider: map[string]ProviderConfig{
			"mock": {BaseURL: upstreamURL + "/v1", Dialect: "openai", APIKey: providerKey},
		},
		Models: map[string]ModelGroup{
			"default": {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "mock-model"}}},
			"other":   {Strategy: "static", Targets: []Target{{Provider: "mock", Model: "other-model"}}},
		},
		Callers: []CallerConfig{{
			ID:          "alice",
			User:        "alice",
			Project:     "metrum-insights",
			Environment: "test",
			TokenSHA256: hex.EncodeToString(sum[:]),
			TokenID:     "rtr_alice_test",
			Allow:       []string{"default", "other"},
			Rate:        RateConfig{RPM: 100, TPM: 100000, Concurrent: 4},
			Quota:       QuotaConfig{Day: BudgetConfig{Requests: 100, Tokens: 100000}, Month: BudgetConfig{Tokens: 1000000}, SoftPct: 80},
			Key:         KeyConfig{LifetimeTokens: 1000000, SoftPct: 90, OnExhaust: "disable"},
		}},
	}
}

func testPIIFilterConfig(mode string) PIIFilterConfig {
	restore := mode == "redact_and_restore"
	return PIIFilterConfig{
		Enabled:         true,
		Mode:            mode,
		RestoreResponse: &restore,
		Rules: []PIIFilterRule{
			{
				Name:              "email",
				Expression:        `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
				PlaceholderPrefix: "EMAIL",
			},
			{
				Name:              "phone",
				Expression:        `\b(?:\+1[-. ]?)?\(?[2-9]\d{2}\)?[-. ]?[2-9]\d{2}[-. ]?\d{4}\b`,
				PlaceholderPrefix: "PHONE",
			},
			{
				Name:              "ssn",
				Expression:        `\b\d{3}-\d{2}-\d{4}\b`,
				PlaceholderPrefix: "US_SSN",
			},
		},
	}
}

func mustJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertJSONEquivalent(t *testing.T, name string, got, want any) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s got value is not JSON-serializable: %v", name, err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("%s want value is not JSON-serializable: %v", name, err)
	}
	if string(gotRaw) != string(wantRaw) {
		t.Fatalf("%s mismatch\ngot:  %s\nwant: %s", name, gotRaw, wantRaw)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
