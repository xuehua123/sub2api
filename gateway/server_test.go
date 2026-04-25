package gateway

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerStartsSignInWhenLobeHubSessionIsMissing(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/workspace":
			http.Redirect(w, r, "/signin", http.StatusFound)
		case "/api/auth/sign-in/oauth2":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.Host != "chat.example.com" {
				t.Fatalf("expected public host chat.example.com, got %s", r.Host)
			}
			if got := r.Header.Get("X-Forwarded-Proto"); got != "https" {
				t.Fatalf("expected x-forwarded-proto https, got %s", got)
			}
			if got := r.Header.Get("X-Forwarded-For"); got != "203.0.113.10" {
				t.Fatalf("expected x-forwarded-for 203.0.113.10, got %s", got)
			}
			if got := r.Header.Get("User-Agent"); got != "LobeHubGatewayTest/1.0" {
				t.Fatalf("expected browser user-agent, got %s", got)
			}
			if got := r.Header.Get("Origin"); got != "https://chat.example.com" {
				t.Fatalf("expected origin https://chat.example.com, got %s", got)
			}
			if got := r.Header.Get("Referer"); got != "https://chat.example.com/workspace" {
				t.Fatalf("expected referer https://chat.example.com/workspace, got %s", got)
			}
			body, _ := io.ReadAll(r.Body)
			var payload signInOAuth2Request
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("unmarshal request body: %v", err)
			}
			if payload.ProviderID != "generic-oidc" {
				t.Fatalf("expected provider id generic-oidc, got %s", payload.ProviderID)
			}
			if payload.CallbackURL != "https://chat.example.com/__lobehub_bootstrap?mode=login&return_url=https%3A%2F%2Fchat.example.com%2Fworkspace" {
				t.Fatalf("unexpected callback URL body: %s", payload.CallbackURL)
			}
			w.Header().Add("Set-Cookie", "__Secure-better-auth.state=state-123; Path=/; HttpOnly; Secure; SameSite=Lax")
			writeJSONResponse(t, w, map[string]any{
				"url":      "https://api.example.com/api/v1/lobehub/oidc/authorize?state=state-123",
				"redirect": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("User-Agent", "LobeHubGatewayTest/1.0")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://api.example.com/api/v1/lobehub/oidc/authorize?state=state-123" {
		t.Fatalf("unexpected redirect location: %s", resp.Header.Get("Location"))
	}

	foundStateCookie := false
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "__Secure-better-auth.state" {
			foundStateCookie = true
			if cookie.Value != "state-123" {
				t.Fatalf("expected state cookie value state-123, got %s", cookie.Value)
			}
		}
	}
	if !foundStateCookie {
		t.Fatalf("expected state cookie to be forwarded")
	}
}

func TestServerRedirectsToRefreshTargetWhenTargetCookieIsMissing(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasCookie(r, "lobehub_session", "ok") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, "/signin", http.StatusFound)
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "lobehub_session", Value: "ok"})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	location := resp.Header.Get("Location")
	expected := "https://app.example.com/auth/lobehub-sso?mode=refresh-target&return_url=https%3A%2F%2Fchat.example.com%2Fworkspace"
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if location != expected {
		t.Fatalf("expected redirect %s, got %s", expected, location)
	}
}

func TestServerProxiesWhenSessionAndSyncStateAlreadyMatch(t *testing.T) {
	t.Helper()

	var bootstrapExchangeCalls atomic.Int32
	var compareCalls atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/trpc/lambda/aiProvider.getAiProviderRuntimeState":
			writeTRPCResponse(t, w, map[string]any{
				"runtimeConfig": map[string]any{
					"openai": map[string]any{
						"keyVaults": map[string]any{
							"apiKey":  "sk-user-1",
							"baseURL": "https://api.example.com/v1",
						},
					},
				},
			})
			return
		default:
		}
		if hasCookie(r, "lobehub_session", "ok") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("upstream-page"))
			return
		}
		http.Redirect(w, r, "/signin", http.StatusFound)
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		case "/api/v1/lobehub/config-probe/compare":
			compareCalls.Add(1)
			writeAPIResponse(t, w, map[string]any{
				"matched":                    true,
				"desired_config_fingerprint": "fp-1",
				"current_config_fingerprint": "fp-1",
			})
		case "/api/v1/lobehub/bootstrap-exchange":
			bootstrapExchangeCalls.Add(1)
			writeAPIResponse(t, w, map[string]any{
				"bootstrap_ticket_id": "ticket-should-not-be-used",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"desired_config_fingerprint": "fp-1",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "lobehub_session", Value: "ok"})
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	req.AddCookie(&http.Cookie{Name: SyncCookieName, Value: hashSyncState(syncState{
		DesiredConfigFingerprint: "fp-1",
		RuntimeConfigVersion:     "runtime-v1",
	})})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "upstream-page" {
		t.Fatalf("expected proxied upstream body, got %s", string(body))
	}
	if compareCalls.Load() != 0 {
		t.Fatalf("expected matching sync cookie to skip config probe, got %d compare calls", compareCalls.Load())
	}
	if bootstrapExchangeCalls.Load() != 0 {
		t.Fatalf("expected no bootstrap exchange call, got %d", bootstrapExchangeCalls.Load())
	}
}

func TestServerProxiesStaticAssetsWithoutBootstrap(t *testing.T) {
	t.Helper()

	var signInCalls atomic.Int32
	var refreshCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.webmanifest":
			w.Header().Set("Content-Type", "application/manifest+json")
			_, _ = w.Write([]byte(`{"name":"LobeHub"}`))
		case "/api/auth/sign-in/oauth2":
			signInCalls.Add(1)
			http.Error(w, "should not start sign-in for static assets", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalls.Add(1)
		http.Error(w, "should not call sub2api for static assets", http.StatusInternalServerError)
	}))
	defer backend.Close()

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/manifest.webmanifest", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != `{"name":"LobeHub"}` {
		t.Fatalf("expected manifest body, got %s", string(body))
	}
	if signInCalls.Load() != 0 {
		t.Fatalf("expected no sign-in calls, got %d", signInCalls.Load())
	}
	if refreshCalls.Load() != 0 {
		t.Fatalf("expected no refresh calls, got %d", refreshCalls.Load())
	}
}

func TestServerProxiesWhenConfigProbeMatches(t *testing.T) {
	t.Helper()

	var bootstrapExchangeCalls atomic.Int32
	var compareCalls atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/trpc/lambda/aiProvider.getAiProviderRuntimeState":
			writeTRPCResponse(t, w, map[string]any{
				"runtimeConfig": map[string]any{
					"openai": map[string]any{
						"keyVaults": map[string]any{
							"apiKey":  "sk-user-1",
							"baseURL": "https://api.example.com/v1",
						},
					},
				},
			})
			return
		default:
			if hasCookie(r, "lobehub_session", "ok") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("upstream-page"))
				return
			}
			http.Redirect(w, r, "/signin", http.StatusFound)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		case "/api/v1/lobehub/config-probe/compare":
			compareCalls.Add(1)
			writeAPIResponse(t, w, map[string]any{
				"matched":                    true,
				"desired_config_fingerprint": "fp-1",
				"current_config_fingerprint": "fp-1",
			})
		case "/api/v1/lobehub/bootstrap-exchange":
			bootstrapExchangeCalls.Add(1)
			writeAPIResponse(t, w, map[string]any{
				"bootstrap_ticket_id": "ticket-should-not-be-used",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"desired_config_fingerprint": "fp-1",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "lobehub_session", Value: "ok"})
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "upstream-page" {
		t.Fatalf("expected proxied upstream body, got %s", string(body))
	}
	if compareCalls.Load() != 1 {
		t.Fatalf("expected one config probe compare call, got %d", compareCalls.Load())
	}
	if bootstrapExchangeCalls.Load() != 0 {
		t.Fatalf("expected no bootstrap exchange call, got %d", bootstrapExchangeCalls.Load())
	}
}

func TestServerBootstrapsWhenConfigProbeReportsMismatch(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/trpc/lambda/aiProvider.getAiProviderRuntimeState":
			writeTRPCResponse(t, w, map[string]any{
				"runtimeConfig": map[string]any{
					"openai": map[string]any{
						"keyVaults": map[string]any{
							"apiKey":  "sk-user-1",
							"baseURL": "https://api.example.com/v1",
						},
					},
				},
			})
			return
		default:
			if hasCookie(r, "lobehub_session", "ok") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("upstream-page"))
				return
			}
			http.Redirect(w, r, "/signin", http.StatusFound)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		case "/api/v1/lobehub/config-probe/compare":
			writeAPIResponse(t, w, map[string]any{
				"matched":                    false,
				"desired_config_fingerprint": "fp-2",
			})
		case "/api/v1/lobehub/bootstrap-exchange":
			writeAPIResponse(t, w, map[string]any{
				"bootstrap_ticket_id": "ticket-123",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"desired_config_fingerprint": "fp-2",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "lobehub_session", Value: "ok"})
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	location := resp.Header.Get("Location")
	expected := "https://chat.example.com/__lobehub_bootstrap?mode=sync&ticket=ticket-123"
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if location != expected {
		t.Fatalf("expected redirect %s, got %s", expected, location)
	}
}

func TestServerExchangesBootstrapWhenSyncCookieIsOutdated(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasCookie(r, "lobehub_session", "ok") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("upstream-page"))
			return
		}
		http.Redirect(w, r, "/signin", http.StatusFound)
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		case "/api/v1/lobehub/bootstrap-exchange":
			writeAPIResponse(t, w, map[string]any{
				"bootstrap_ticket_id": "ticket-123",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"desired_config_fingerprint": "fp-2",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "lobehub_session", Value: "ok"})
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	location := resp.Header.Get("Location")
	expected := "https://chat.example.com/__lobehub_bootstrap?mode=sync&ticket=ticket-123"
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if location != expected {
		t.Fatalf("expected redirect %s, got %s", expected, location)
	}
}

func TestBootstrapConsumesTicketRedirectsAndSetsSyncCookie(t *testing.T) {
	t.Helper()

	var applyCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/list-accounts":
			writeJSONResponse(t, w, []any{map[string]any{
				"id":         "account-101",
				"providerId": "generic-oidc",
				"accountId":  "101",
				"userId":     "user_internal_101",
			}})
			return
		case "/api/auth/get-session":
			writeJSONResponse(t, w, map[string]any{
				"user": map[string]any{"id": "user_internal_101"},
			})
			return
		case "/trpc/lambda/aiProvider.updateAiProviderConfig":
			applyCalls.Add(1)
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			requireEqualJSON(t, `{"json":{"id":"openai","value":{"fetchOnClient":false,"keyVaults":{"apiKey":"sk-user-1","baseURL":"https://api.example.com/v1"}}}}`, string(body))
			writeTRPCResponse(t, w, map[string]any{"ok": true})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/lobehub/bootstrap/consume":
			writeAPIResponse(t, w, map[string]any{
				"redirect_url":    "https://chat.example.com/workspace?settings=%7B%7D",
				"provider_id":     "openai",
				"fetch_on_client": false,
				"key_vaults": map[string]any{
					"apiKey":  "sk-user-1",
					"baseURL": "https://api.example.com/v1",
				},
			})
		case "/api/v1/settings/public":
			writeAPIResponse(t, w, map[string]any{
				"lobehub_enabled":                true,
				"lobehub_runtime_config_version": "runtime-v1",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"user_id":                    101,
		"desired_config_fingerprint": "fp-3",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/__lobehub_bootstrap?ticket=ticket-123", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://chat.example.com/workspace?settings=%7B%7D" {
		t.Fatalf("unexpected redirect: %s", resp.Header.Get("Location"))
	}
	if applyCalls.Load() != 1 {
		t.Fatalf("expected provider config to be applied exactly once, got %d", applyCalls.Load())
	}
	foundSync := false
	for _, cookie := range resp.Cookies() {
		if cookie.Name == SyncCookieName {
			foundSync = true
			expected := hashSyncState(syncState{
				DesiredConfigFingerprint: "fp-3",
				RuntimeConfigVersion:     "runtime-v1",
			})
			if cookie.Value != expected {
				t.Fatalf("expected sync cookie %s, got %s", expected, cookie.Value)
			}
		}
	}
	if !foundSync {
		t.Fatalf("expected sync cookie to be set")
	}
}

func TestBootstrapRedirectsToRefreshTargetWhenSessionIsMissing(t *testing.T) {
	t.Helper()

	var signInCalls atomic.Int32
	var consumeCalls atomic.Int32
	var applyCalls atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/list-accounts":
			http.Redirect(w, r, "/signin", http.StatusFound)
		case "/api/auth/sign-in/oauth2":
			signInCalls.Add(1)
			writeJSONResponse(t, w, map[string]any{
				"url":      "https://api.example.com/api/v1/lobehub/oidc/authorize?state=state-456",
				"redirect": true,
			})
		case "/trpc/lambda/aiProvider.updateAiProviderConfig":
			applyCalls.Add(1)
			http.Error(w, "should not apply config without a readable session", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/lobehub/bootstrap/consume":
			consumeCalls.Add(1)
			http.Error(w, "should not consume ticket before session is readable", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"user_id":                    101,
		"desired_config_fingerprint": "fp-3",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/__lobehub_bootstrap?mode=sync&ticket=ticket-123", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	expected := "https://app.example.com/auth/lobehub-sso?mode=refresh-target&return_url=https%3A%2F%2Fchat.example.com%2F__lobehub_bootstrap%3Fmode%3Dsync%26ticket%3Dticket-123"
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != expected {
		t.Fatalf("expected redirect %s, got %s", expected, resp.Header.Get("Location"))
	}
	if signInCalls.Load() != 0 {
		t.Fatalf("expected bootstrap sync not to start sign-in, got %d sign-in calls", signInCalls.Load())
	}
	if consumeCalls.Load() != 0 {
		t.Fatalf("expected bootstrap ticket to remain unused, got %d consume calls", consumeCalls.Load())
	}
	if applyCalls.Load() != 0 {
		t.Fatalf("expected provider config not to be applied, got %d apply calls", applyCalls.Load())
	}
}

func TestBootstrapRedirectsToRefreshTargetWhenSessionAccountLookupIsRateLimited(t *testing.T) {
	t.Helper()

	var signInCalls atomic.Int32
	var consumeCalls atomic.Int32
	var applyCalls atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/list-accounts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"Too many requests. Please try again later."}`))
		case "/api/auth/sign-in/oauth2":
			signInCalls.Add(1)
			writeJSONResponse(t, w, map[string]any{
				"url":      "https://api.example.com/api/v1/lobehub/oidc/authorize?state=state-456",
				"redirect": true,
			})
		case "/trpc/lambda/aiProvider.updateAiProviderConfig":
			applyCalls.Add(1)
			http.Error(w, "should not apply config while auth is rate limited", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/lobehub/bootstrap/consume":
			consumeCalls.Add(1)
			http.Error(w, "should not consume ticket while auth is rate limited", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"user_id":                    101,
		"desired_config_fingerprint": "fp-3",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/__lobehub_bootstrap?mode=sync&ticket=ticket-123", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	expected := "https://app.example.com/auth/lobehub-sso?mode=refresh-target&return_url=https%3A%2F%2Fchat.example.com%2F__lobehub_bootstrap%3Fmode%3Dsync%26ticket%3Dticket-123"
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != expected {
		t.Fatalf("expected redirect %s, got %s", expected, resp.Header.Get("Location"))
	}
	if signInCalls.Load() != 0 {
		t.Fatalf("expected bootstrap sync not to start sign-in, got %d sign-in calls", signInCalls.Load())
	}
	if consumeCalls.Load() != 0 {
		t.Fatalf("expected bootstrap ticket to remain unused, got %d consume calls", consumeCalls.Load())
	}
	if applyCalls.Load() != 0 {
		t.Fatalf("expected provider config not to be applied, got %d apply calls", applyCalls.Load())
	}
}

func TestBootstrapRedirectsToRefreshTargetWhenSessionUserDoesNotMatchTargetToken(t *testing.T) {
	t.Helper()

	var signInCalls atomic.Int32
	var consumeCalls atomic.Int32
	var applyCalls atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/list-accounts":
			writeJSONResponse(t, w, []any{map[string]any{
				"id":         "account-202",
				"providerId": "generic-oidc",
				"accountId":  "202",
				"userId":     "user_internal_202",
			}})
		case "/api/auth/sign-in/oauth2":
			signInCalls.Add(1)
			writeJSONResponse(t, w, map[string]any{
				"url":      "https://api.example.com/api/v1/lobehub/oidc/authorize?state=state-456",
				"redirect": true,
			})
		case "/trpc/lambda/aiProvider.updateAiProviderConfig":
			applyCalls.Add(1)
			http.Error(w, "should not apply config", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/lobehub/bootstrap/consume":
			consumeCalls.Add(1)
			http.Error(w, "should not consume ticket before session match", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	targetToken := newUnsignedJWT(t, map[string]any{
		"user_id":                    101,
		"desired_config_fingerprint": "fp-3",
		"runtime_config_version":     "runtime-v1",
		"exp":                        time.Now().Add(10 * time.Minute).Unix(),
	})

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  backend.URL + "/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/__lobehub_bootstrap?ticket=ticket-123", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: TargetCookieName, Value: targetToken})
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	expected := "https://app.example.com/auth/lobehub-sso?mode=refresh-target&return_url=https%3A%2F%2Fchat.example.com%2F__lobehub_bootstrap%3Fticket%3Dticket-123"
	if resp.Header.Get("Location") != expected {
		t.Fatalf("expected redirect %s, got %s", expected, resp.Header.Get("Location"))
	}
	if signInCalls.Load() != 0 {
		t.Fatalf("expected bootstrap sync not to start sign-in, got %d sign-in calls", signInCalls.Load())
	}
	if consumeCalls.Load() != 0 {
		t.Fatalf("expected bootstrap ticket to remain unused, got %d consume calls", consumeCalls.Load())
	}
	if applyCalls.Load() != 0 {
		t.Fatalf("expected provider config not to be applied, got %d apply calls", applyCalls.Load())
	}
}

func TestBootstrapLoginModeRedirectsBackToOriginalURLWithoutTicket(t *testing.T) {
	t.Helper()

	server := mustNewTestServer(t, Config{
		UpstreamURL:        "https://chat.example.com",
		Sub2APIAPIBaseURL:  "https://api.example.com/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"http://chat.example.com/__lobehub_bootstrap?mode=login&return_url=https%3A%2F%2Fchat.example.com%2Fworkspace%3Ffoo%3Dbar",
		nil,
	)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://chat.example.com/workspace?foo=bar" {
		t.Fatalf("unexpected redirect: %s", resp.Header.Get("Location"))
	}
}

func TestFetchObservedSettingsUsesConfiguredUserStatePath(t *testing.T) {
	t.Helper()

	var requestedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		if r.URL.Path != "/trpc/custom/userState" {
			http.NotFound(w, r)
			return
		}
		writeTRPCResponse(t, w, map[string]any{
			"runtimeConfig": map[string]any{
				"openai": map[string]any{
					"keyVaults": map[string]any{
						"apiKey":  "sk-user-1",
						"baseURL": "https://api.example.com/v1",
					},
				},
			},
		})
	}))
	defer upstream.Close()

	server := mustNewTestServer(t, Config{
		UpstreamURL:        upstream.URL,
		Sub2APIAPIBaseURL:  "https://api.example.com/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
		UserStatePath:      "/trpc/custom/userState",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace", nil)
	req.Host = "chat.example.com"
	req.AddCookie(&http.Cookie{Name: "lobehub_session", Value: "ok"})

	observed, err := server.fetchObservedSettings(req.Context(), req)
	if err != nil {
		t.Fatalf("fetchObservedSettings returned error: %v", err)
	}
	if requestedPath != "/trpc/custom/userState" {
		t.Fatalf("expected configured user state path, got %s", requestedPath)
	}
	if observed.KeyVaults["openai"].APIKey != "sk-user-1" {
		t.Fatalf("expected parsed api key, got %+v", observed.KeyVaults["openai"])
	}
}

func mustNewTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	return server
}

func writeAPIResponse(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}

func writeTRPCResponse(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{
			"data": data,
		},
	})
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func hasCookie(r *http.Request, name string, want string) bool {
	cookie, err := r.Cookie(name)
	if err != nil {
		return false
	}
	return cookie.Value == want
}

func newUnsignedJWT(t *testing.T, claims map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{
		"alg": "HS256",
		"typ": "JWT",
	})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString(headerBytes),
		base64.RawURLEncoding.EncodeToString(claimBytes),
		"signature",
	}, ".")
}

func TestCurrentRequestURL(t *testing.T) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/path?q=1", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := currentRequestURL(req)
	want := "https://chat.example.com/path?q=1"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestHealthzReturnsOK(t *testing.T) {
	t.Helper()

	server := mustNewTestServer(t, Config{
		UpstreamURL:        "http://localhost:3210",
		Sub2APIAPIBaseURL:  "https://api.example.com/api/v1",
		Sub2APIFrontendURL: "https://app.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/healthz", nil)
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if strings.TrimSpace(string(body)) != "ok" {
		t.Fatalf("expected ok body, got %s", string(body))
	}
}

func TestRequestOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/workspace?foo=bar", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := requestOrigin(req)
	want := "https://chat.example.com"
	if got != want {
		t.Fatalf("requestOrigin() = %s, want %s", got, want)
	}
}

func requireEqualJSON(t *testing.T, expected string, actual string) {
	t.Helper()

	var expectedValue any
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("failed to unmarshal expected json: %v", err)
	}
	var actualValue any
	if err := json.Unmarshal([]byte(actual), &actualValue); err != nil {
		t.Fatalf("failed to unmarshal actual json: %v", err)
	}
	if !jsonEqual(expectedValue, actualValue) {
		t.Fatalf("expected json %s, got %s", expected, actual)
	}
}

func jsonEqual(expected any, actual any) bool {
	expectedBytes, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	actualBytes, err := json.Marshal(actual)
	if err != nil {
		return false
	}
	return string(expectedBytes) == string(actualBytes)
}

func TestBuildRefreshTargetURL(t *testing.T) {
	t.Helper()

	got, err := buildRefreshTargetURL("https://app.example.com", "https://chat.example.com/workspace")
	if err != nil {
		t.Fatalf("buildRefreshTargetURL returned error: %v", err)
	}

	expected, _ := url.Parse("https://app.example.com/auth/lobehub-sso")
	query := expected.Query()
	query.Set("mode", "refresh-target")
	query.Set("return_url", "https://chat.example.com/workspace")
	expected.RawQuery = query.Encode()

	if got != expected.String() {
		t.Fatalf("expected %s, got %s", expected.String(), got)
	}
}

func TestSharedCookieDomain_UsesRegistrableDomainOnlyForSubdomains(t *testing.T) {
	if got := sharedCookieDomain("chat.example.com"); got != ".example.com" {
		t.Fatalf("sharedCookieDomain(chat.example.com) = %s", got)
	}
	if got := sharedCookieDomain("chat.example.co.uk"); got != ".example.co.uk" {
		t.Fatalf("sharedCookieDomain(chat.example.co.uk) = %s", got)
	}
	if got := sharedCookieDomain("example.com"); got != "" {
		t.Fatalf("sharedCookieDomain(example.com) = %s, want empty", got)
	}
	if got := sharedCookieDomain("127.0.0.1:3210"); got != "" {
		t.Fatalf("sharedCookieDomain(127.0.0.1:3210) = %s, want empty", got)
	}
}

func TestParseObservedSettings_PreservesLanguageModelWhenRuntimeConfigPresent(t *testing.T) {
	payload := []byte(`{
		"runtimeConfig": {
			"openai": {
				"keyVaults": {
					"apiKey": "sk-user-1",
					"baseURL": "https://api.example.com/v1"
				}
			}
		},
		"settings": {
			"languageModel": {
				"openai": {
					"enabled": true,
					"enabledModels": ["gpt-4.1"]
				}
			}
		}
	}`)

	observed, err := parseObservedSettings(payload)
	if err != nil {
		t.Fatalf("parseObservedSettings returned error: %v", err)
	}
	if observed == nil {
		t.Fatal("parseObservedSettings returned nil observed settings")
	}
	if vault := observed.KeyVaults["openai"]; vault.APIKey != "sk-user-1" || vault.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("unexpected key vault parsed: %+v", vault)
	}
	model, ok := observed.LanguageModel["openai"]
	if !ok {
		t.Fatal("expected language model config for openai")
	}
	if !model.Enabled {
		t.Fatal("expected openai language model to be enabled")
	}
	if len(model.EnabledModels) != 1 || model.EnabledModels[0] != "gpt-4.1" {
		t.Fatalf("unexpected enabled models: %+v", model.EnabledModels)
	}
}

func TestSanitizeBootstrapReturnURL_FallsBackForCrossOriginRedirect(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodGet,
		"http://chat.example.com/__lobehub_bootstrap?mode=login&return_url=http%3A%2F%2Fchat.example.com%2Fworkspace",
		nil,
	)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := sanitizeBootstrapReturnURL(req)
	if got != "https://chat.example.com/" {
		t.Fatalf("sanitizeBootstrapReturnURL() = %s", got)
	}
}
