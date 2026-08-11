package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticateNewAPIManagementSessionAcceptsResponseAccessToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/user/login" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		writer.Header().Add("Set-Cookie", "new_api_refresh=opaque-refresh; Path=/; HttpOnly")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{"id":42,"access_token":"response-access-token"}}`))
	}))
	defer server.Close()

	client := &upstreamManagementClient{client: server.Client()}
	session, err := client.authenticateNewAPIManagementSession(
		context.Background(),
		server.Client(),
		server.URL,
		upstreamManagementConfig{AuthMode: UpstreamManagementAuthModePassword},
		upstreamManagementAuthSecret{Username: "operator", Password: "password"},
	)
	if err != nil {
		t.Fatalf("authenticate session: %v", err)
	}
	if session.accessToken != "response-access-token" {
		t.Fatalf("access token = %q, want response token", session.accessToken)
	}
	if session.remoteUserID != 42 {
		t.Fatalf("remote user id = %d, want 42", session.remoteUserID)
	}
	if session.cookie != nil {
		t.Fatalf("session cookie = %q, want nil when only refresh cookie is returned", session.cookie.Name)
	}
}
