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

func TestServerReturnsAutoSubmitWhenLobeHubSessionIsMissing(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, req)

	resp := recorder.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "fetch('/api/auth/sign-in/oauth2'") {
		t.Fatalf("expected auto-submit fetch, body=%s", string(body))
	}
	if !strings.Contains(string(body), "'Content-Type': 'application/json'") {
		t.Fatalf("expected JSON content type, body=%s", string(body))
	}
	if !strings.Contains(string(body), "additionalData") {
		t.Fatalf("expected additionalData payload, body=%s", string(body))
	}
	if !strings.Contains(string(body), "generic-oidc") {
		t.Fatalf("expected providerId in payload, body=%s", string(body))
	}
	if !strings.Contains(string(body), "__lobehub_bootstrap") {
		t.Fatalf("expected bootstrap callback URL, body=%s", string(body))
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
		case "/trpc/lambda/user.getUserState":
			writeTRPCResponse(t, w, map[string]any{
				"settings": map[string]any{
					"keyVaults": map[string]any{
						"openai": map[string]any{
							"apiKey":  "sk-user-1",
							"baseURL": "https://api.example.com/v1",
						},
					},
					"languageModel": map[string]any{
						"openai": map[string]any{
							"enabled":       true,
							"enabledModels": []string{"gpt-4.1"},
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
	if compareCalls.Load() != 1 {
		t.Fatalf("expected one config probe compare call, got %d", compareCalls.Load())
	}
	if bootstrapExchangeCalls.Load() != 0 {
		t.Fatalf("expected no bootstrap exchange call, got %d", bootstrapExchangeCalls.Load())
	}
}

func TestServerProxiesWhenConfigProbeMatches(t *testing.T) {
	t.Helper()

	var bootstrapExchangeCalls atomic.Int32
	var compareCalls atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/trpc/lambda/user.getUserState":
			writeTRPCResponse(t, w, map[string]any{
				"settings": map[string]any{
					"keyVaults": map[string]any{
						"openai": map[string]any{
							"apiKey":  "sk-user-1",
							"baseURL": "https://api.example.com/v1",
						},
					},
					"languageModel": map[string]any{
						"openai": map[string]any{
							"enabled":       true,
							"enabledModels": []string{"gpt-4.1"},
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
		case "/trpc/lambda/user.getUserState":
			writeTRPCResponse(t, w, map[string]any{
				"settings": map[string]any{
					"keyVaults": map[string]any{
						"openai": map[string]any{
							"apiKey":  "sk-user-1",
							"baseURL": "https://api.example.com/v1",
						},
					},
					"languageModel": map[string]any{
						"openai": map[string]any{
							"enabled":       true,
							"enabledModels": []string{"gpt-4.1"},
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

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/lobehub/bootstrap/consume":
			writeAPIResponse(t, w, map[string]any{
				"redirect_url": "https://chat.example.com/workspace?settings=%7B%7D",
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
			"settings": map[string]any{
				"keyVaults": map[string]any{
					"openai": map[string]any{
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
