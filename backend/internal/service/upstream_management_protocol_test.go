//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRefreshSub2APIManagementTokenReusesCredentialUserAgent(t *testing.T) {
	const expectedUserAgent = "Mozilla/5.0 exact-login-agent"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, expectedUserAgent, request.Header.Get("User-Agent"))
		require.Equal(t, "/api/v1/auth/refresh", request.URL.Path)
		_, _ = writer.Write([]byte(`{"data":{"access_token":"next-access","refresh_token":"next-refresh","expires_in":3600}}`))
	}))
	defer server.Close()

	client := &upstreamManagementClient{}
	secret, err := client.refreshSub2APIManagementToken(
		context.Background(), server.Client(), server.URL,
		upstreamManagementAuthSecret{RefreshToken: "current-refresh", UserAgent: expectedUserAgent},
	)
	require.NoError(t, err)
	require.Equal(t, "next-access", secret.AccessToken)
	require.Equal(t, "next-refresh", secret.RefreshToken)
	require.Equal(t, expectedUserAgent, secret.UserAgent)
	require.Greater(t, secret.ExpiresAt, time.Now().Unix())
}

func TestRefreshSub2APIManagementTokenUsesJWTExpiryWhenExpiresInIsOmitted(t *testing.T) {
	const accessToken = "e30.eyJleHAiOjQxMDAwMDAwMDB9.signature"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/v1/auth/refresh", request.URL.Path)
		_, _ = writer.Write([]byte(`{"data":{"access_token":"` + accessToken + `","refresh_token":"next-refresh"}}`))
	}))
	defer server.Close()

	secret, err := (&upstreamManagementClient{}).refreshSub2APIManagementToken(
		context.Background(), server.Client(), server.URL, upstreamManagementAuthSecret{RefreshToken: "current-refresh"},
	)

	require.NoError(t, err)
	require.Equal(t, int64(4100000000), secret.ExpiresAt)
}

func TestUpstreamAuthenticationRejectionMessageMatchesOfficialNewAPIPasswordError(t *testing.T) {
	t.Parallel()

	require.True(t, isUpstreamAuthenticationRejectionMessage(
		"Username or password is incorrect, or user has been banned",
	))
	require.False(t, isUpstreamAuthenticationRejectionMessage("temporary upstream failure"))
}
