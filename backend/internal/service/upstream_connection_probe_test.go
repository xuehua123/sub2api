//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestUpstreamConnectionInspectorAutoReportsMissingRemoteUserID(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderAuto, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.ErrorIs(t, err, errUpstreamConnectionRemoteUserIDRequired)
}

func TestUpstreamConnectionInspectorAutoStopsAtLocationConfirmation(t *testing.T) {
	var fallbackLoginCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"system_name": "Sub2API"}})
		case "/api/v1/auth/login":
			writer.WriteHeader(http.StatusBadRequest)
			writeProbeJSON(t, writer, map[string]any{
				"code": 400, "message": "must confirm you are not located in mainland China",
				"reason": "NOT_IN_CN_CONFIRMATION_REQUIRED",
			})
		case "/api/user/login":
			fallbackLoginCalls.Add(1)
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "fallback-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderAuto, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.ErrorIs(t, err, ErrUpstreamManagementLocationConfirmationRequired)
	require.Zero(t, fallbackLoginCalls.Load())
}

func TestUpstreamConnectionProviderHintFromStatus(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		payload  map[string]any
		expected string
	}{
		{name: "newapi name", payload: map[string]any{"system_name": "New API"}, expected: UpstreamConnectionProviderNewAPI},
		{name: "oneapi name", payload: map[string]any{"system_name": "One API"}, expected: UpstreamConnectionProviderOneAPI},
		{name: "onehub name", payload: map[string]any{"system_name": "One Hub"}, expected: UpstreamConnectionProviderOneHub},
		{name: "donehub renamed", payload: map[string]any{"system_name": "Private Relay", "ClaudeAPIEnabled": true}, expected: UpstreamConnectionProviderDoneHub},
		{name: "veloera renamed", payload: map[string]any{"system_name": "Private Relay", "aff_enabled": true}, expected: UpstreamConnectionProviderVeloera},
		{name: "unknown", payload: map[string]any{"system_name": "Private Relay"}, expected: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.expected, upstreamConnectionProviderHintFromStatus(test.payload))
		})
	}
}

func TestUpstreamConnectionInspectorAutoUsesVeloeraStatusHint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"system_name": "Veloera", "quota_per_unit": 500_000,
			}})
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "veloera-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7}})
		case "/api/user/self":
			require.Equal(t, "7", request.Header.Get("Veloera-User"))
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 7, "group": "vip", "quota": 1_000_000,
			}})
		case "/api/user/self/groups":
			require.Equal(t, "7", request.Header.Get("Veloera-User"))
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"group_ratio": map[string]any{"default": 1.0, "vip": 0.3},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderAuto, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Equal(t, UpstreamConnectionProviderVeloera, snapshot.DetectedProvider)
	require.Empty(t, snapshot.Warnings)
	require.Len(t, snapshot.Groups, 2)
}

func TestUpstreamConnectionInspectorAutoSkipsDegradedGenericDialect(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"system_name": "Private Relay", "quota_per_unit": 500_000,
			}})
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "oneapi-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 3}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 3, "group": "default", "quota": 250_000,
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderAuto, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Equal(t, UpstreamConnectionProviderOneAPI, snapshot.DetectedProvider)
	require.Empty(t, snapshot.Warnings)
	require.Len(t, snapshot.Groups, 1)
	require.Nil(t, snapshot.Groups[0].RateMultiplier)
}

func TestUpstreamConnectionInspectorUsesValidatedDirectClientWhenAllowlistEnabled(t *testing.T) {
	plainClient := &http.Client{}
	inspector := newUpstreamConnectionInspector(&config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{Enabled: true},
	}}, nil, plainClient)

	client, err := inspector.clientForConnection(context.Background(), &UpstreamConnection{})
	require.NoError(t, err)
	require.NotSame(t, plainClient, client)
}

func TestSanitizeUpstreamKeyLookupErrorRemovesForwardingKey(t *testing.T) {
	const apiKey = "sk-sensitive-forwarding-key"
	err := &url.Error{
		Op: "Get", URL: "https://upstream.example/api/token/search?token=" + apiKey,
		Err: errors.New("dial failed for " + apiKey),
	}

	sanitized := sanitizeUpstreamKeyLookupError(err, apiKey)
	require.Error(t, sanitized)
	require.NotContains(t, sanitized.Error(), apiKey)
	require.Contains(t, sanitized.Error(), "[redacted]")
}

func TestUpstreamConnectionInspectorNewAPIPasswordLoadsWalletAndGroups(t *testing.T) {
	var loginCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			loginCalls.Add(1)
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "session-value"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7}})
		case "/api/user/self":
			require.Contains(t, request.Header.Get("Cookie"), "session=session-value")
			require.Equal(t, "7", request.Header.Get("New-API-User"))
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 7, "group": "default", "quota": 1_000_000, "used_quota": 250_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"quota_per_unit": 500_000, "quota_display_type": "USD",
			}})
		case "/api/user/self/groups":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"group_ratio": map[string]any{"default": 1.0, "vip": 0.5},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	inspector.now = func() time.Time { return time.Unix(1_000, 0) }
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Equal(t, int32(1), loginCalls.Load())
	require.Equal(t, UpstreamConnectionProviderNewAPI, snapshot.DetectedProvider)
	require.Equal(t, "7", snapshot.RemoteUserID)
	require.NotNil(t, snapshot.Wallet)
	require.Equal(t, 2.0, *snapshot.Wallet.Amount)
	require.Equal(t, 2.0, *snapshot.Wallet.USD)
	require.Equal(t, "USD", snapshot.Wallet.Currency)
	require.Equal(t, "exact", snapshot.Wallet.Reliability)
	require.Len(t, snapshot.Groups, 2)
	require.Equal(t, "vip", snapshot.Groups[0].Name)
	require.Equal(t, 0.5, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, true, snapshot.Capabilities["wallet"])
	require.Equal(t, true, snapshot.Capabilities["groups"])
}

func TestUpstreamConnectionInspectorOneHubLoadsSymbolBasedGroupRatios(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "onehub-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 9}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 9, "group": "vip", "quota": 500_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500_000}})
		case "/api/user_group_map":
			require.Contains(t, request.Header.Get("Cookie"), "session=onehub-session")
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"vip": map[string]any{"id": 2, "symbol": "vip", "name": "VIP Customers", "ratio": 0.4},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneHub, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, "vip", snapshot.Groups[0].Name)
	require.Equal(t, "2", snapshot.Groups[0].RemoteID)
	require.Equal(t, 0.4, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "onehub:user_group_map", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceDefault, snapshot.Groups[0].Confidence)
}

func TestUpstreamConnectionInspectorOneAPIObservesInheritedUserGroupWithoutInventingRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "oneapi-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 3}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 3, "group": "default", "quota": 250_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500_000}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, "default", snapshot.Groups[0].Name)
	require.Nil(t, snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "unknown", snapshot.Groups[0].Confidence)
	require.Equal(t, "oneapi:user_self", snapshot.Groups[0].Source)
}

func TestUpstreamConnectionInspectorNewAPIWalletUsesDisplayCurrencyExchangeRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "session-value"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 7, "quota": 1_000_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"quota_per_unit": 500_000, "quota_display_type": "CNY", "usd_exchange_rate": 7.2,
			}})
		case "/api/user/self/groups":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"group_ratio": map[string]any{"default": 1.0},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.NotNil(t, snapshot.Wallet)
	require.Equal(t, 14.4, *snapshot.Wallet.Amount)
	require.Equal(t, 2.0, *snapshot.Wallet.USD)
	require.Equal(t, "CNY", snapshot.Wallet.Currency)
	require.Equal(t, "converted", snapshot.Wallet.Reliability)
}

func TestUpstreamConnectionInspectorNewAPIRejectsGenericSuccessfulJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{}})
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "7",
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected the authenticated session")
}

func TestUpstreamConnectionInspectorNewAPIRejectsInvalidAccessTokenWhenPublicPricingSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self", "/api/user/self/groups":
			writer.WriteHeader(http.StatusUnauthorized)
			writeProbeJSON(t, writer, map[string]any{"success": false, "message": "unauthorized"})
		case "/api/pricing":
			writeProbeJSON(t, writer, map[string]any{"success": true, "group_ratio": map[string]any{"default": 1.0}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "7",
	}, upstreamConnectionCredential{Version: 1, AccessToken: "invalid-management-token"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected the authenticated session")
}

func TestUpstreamConnectionInspectorNewAPIPricingFallbackReadsOnlyGroupRatios(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "session-value"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7, "quota": 100}})
		case "/api/user/self/groups":
			http.NotFound(writer, request)
		case "/api/pricing":
			writeProbeJSON(t, writer, map[string]any{
				"success": true,
				"data": []any{
					map[string]any{"model_name": "gpt-example", "model_ratio": 12.5, "completion_ratio": 4},
				},
				"group_ratio": map[string]any{"default": 1.0, "vip": 0.35},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 2)
	require.Equal(t, "vip", snapshot.Groups[0].Name)
	require.Equal(t, 0.35, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "default", snapshot.Groups[1].Name)
	require.Equal(t, 1.0, *snapshot.Groups[1].RateMultiplier)
	for _, group := range snapshot.Groups {
		require.NotEqual(t, "model_ratio", group.Name)
		require.NotEqual(t, "completion_ratio", group.Name)
		require.Equal(t, "newapi:pricing", group.Source)
		require.Equal(t, upstreamGroupRateConfidenceUnavailable, group.Confidence)
	}
}

func TestUpstreamConnectionInspectorNewAPIKeepsDynamicGroupWithoutInventingRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "session-value"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 7, "quota": 100}})
		case "/api/user/self/groups":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"default": map[string]any{"ratio": 1.0},
				"auto":    map[string]any{"ratio": "automatic"},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 2)
	require.Equal(t, "default", snapshot.Groups[0].Name)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "newapi:self_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceDefault, snapshot.Groups[0].Confidence)
	require.Equal(t, "auto", snapshot.Groups[1].Name)
	require.Nil(t, snapshot.Groups[1].RateMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnknown, snapshot.Groups[1].Confidence)
}

func TestUpstreamConnectionInspectorSub2APIKeepsMissingRateUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "Bearer management-token", request.Header.Get("Authorization"))
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 42.75}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 1, "name": "cheap"},
				map[string]any{"id": 2, "name": "unknown"},
			}}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"1": 0.2}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Equal(t, "11", snapshot.RemoteUserID)
	require.Equal(t, 42.75, *snapshot.Wallet.USD)
	require.Len(t, snapshot.Groups, 2)
	require.Equal(t, "cheap", snapshot.Groups[0].Name)
	require.Equal(t, 0.2, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceOverride, snapshot.Groups[0].Confidence)
	require.Equal(t, "unknown", snapshot.Groups[1].Name)
	require.Nil(t, snapshot.Groups[1].RateMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnknown, snapshot.Groups[1].Confidence)
}

func TestUpstreamConnectionInspectorSub2APIMarksAvailableRatesUnavailableWhenRatesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 42.75}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 1, "name": "default", "rate_multiplier": 0.2},
			}})
		case "/api/v1/groups/rates":
			http.NotFound(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 0.2, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates unavailable; showing available-group default rates for reference only")
}

func TestUpstreamConnectionInspectorSub2APIFallsBackToRootManagementPaths(t *testing.T) {
	var legacyPathCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login", "/api/v1/auth/me", "/api/v1/groups/available", "/api/v1/groups/rates", "/api/v1/keys":
			legacyPathCalls.Add(1)
			http.NotFound(writer, request)
		case "/auth/login":
			require.Equal(t, http.MethodPost, request.Method)
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"access_token": "root-token"}})
		case "/auth/me":
			require.Equal(t, "Bearer root-token", request.Header.Get("Authorization"))
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 42.75}})
		case "/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "plus", "rate_multiplier": 0.5},
			}})
		case "/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"2": 0.25}})
		case "/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 17, "name": "plus-key", "key": "sub2-secret-key", "group_id": 2},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	connection := &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}
	credential := upstreamConnectionCredential{Version: 1, Username: "alice@example.com", Password: "secret"}
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())

	snapshot, err := inspector.Inspect(context.Background(), connection, credential)
	require.NoError(t, err)
	require.Equal(t, "11", snapshot.RemoteUserID)
	require.Equal(t, 42.75, *snapshot.Wallet.USD)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 0.25, *snapshot.Groups[0].RateMultiplier)

	connection.Groups = snapshot.Groups
	binding, err := inspector.ResolveKey(context.Background(), connection, credential, "sub2-secret-key")
	require.NoError(t, err)
	require.Equal(t, "plus", binding.RemoteGroupName)
	require.Equal(t, 0.25, *binding.ObservedMultiplier)
	require.Greater(t, legacyPathCalls.Load(), int32(0))
}

func TestUpstreamConnectionInspectorSub2APIUsesV1ResourcesAfterRootLogin(t *testing.T) {
	var rootResourceCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			http.NotFound(writer, request)
		case "/auth/login":
			writeProbeJSON(t, writer, map[string]any{
				"access_token": "hybrid-token", "refresh_token": "refresh-token",
			})
		case "/auth/me":
			writer.WriteHeader(http.StatusUnauthorized)
			writeProbeJSON(t, writer, map[string]any{"message": "route requires a different profile endpoint"})
		case "/user/profile":
			require.Equal(t, "Bearer hybrid-token", request.Header.Get("Authorization"))
			writeProbeJSON(t, writer, map[string]any{"id": 58, "credit_balance": 34.5})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 3, "name": "vip", "rate_multiplier": 0.5},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"3": 0.25}})
		case "/api/v1/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 18, "name": "vip-key", "key": "hybrid-secret-key", "group_id": 3},
			}}})
		case "/groups/available", "/keys":
			rootResourceCalls.Add(1)
			writer.WriteHeader(http.StatusUnauthorized)
			writeProbeJSON(t, writer, map[string]any{"message": "v1 resource endpoint required"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	connection := &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}
	credential := upstreamConnectionCredential{Version: 1, Username: "alice@example.com", Password: "secret"}
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())

	snapshot, err := inspector.Inspect(context.Background(), connection, credential)
	require.NoError(t, err)
	require.Equal(t, "58", snapshot.RemoteUserID)
	require.Equal(t, 34.5, *snapshot.Wallet.USD)
	require.Equal(t, "sub2api:user_profile", snapshot.Wallet.Source)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 0.25, *snapshot.Groups[0].RateMultiplier)
	require.Empty(t, snapshot.Warnings)

	connection.Groups = snapshot.Groups
	binding, err := inspector.ResolveKey(context.Background(), connection, credential, "hybrid-secret-key")
	require.NoError(t, err)
	require.Equal(t, "vip", binding.RemoteGroupName)
	require.Equal(t, 0.25, *binding.ObservedMultiplier)
	require.Zero(t, rootResourceCalls.Load())
}

func TestUpstreamConnectionInspectorSub2APIRootPathFallbackDoesNotRetryAuthenticationFailure(t *testing.T) {
	var rootLoginCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/login":
			writer.WriteHeader(http.StatusUnauthorized)
			writeProbeJSON(t, writer, map[string]any{"message": "invalid credentials"})
		case "/auth/login":
			rootLoginCalls.Add(1)
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"access_token": "must-not-be-used"}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice@example.com", Password: "wrong"})

	require.ErrorIs(t, err, ErrUpstreamConnectionAuthentication)
	require.Zero(t, rootLoginCalls.Load())
}

func TestUpstreamConnectionInspectorSub2APIPartialRatesMarkMissingGroupsAsDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 1, "name": "covered", "rate_multiplier": 0.5},
				map[string]any{"id": 2, "name": "missing", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			// Only one group is present. The other keeps the available-group
			// default multiplier and is still auto-syncable as "default".
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"1": 0.25}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 2)
	require.Equal(t, "covered", snapshot.Groups[0].Name)
	require.Equal(t, 0.25, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:group_rates", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceOverride, snapshot.Groups[0].Confidence)
	require.Equal(t, "missing", snapshot.Groups[1].Name)
	require.Equal(t, 1.0, *snapshot.Groups[1].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[1].Source)
	require.Equal(t, upstreamGroupRateConfidenceDefault, snapshot.Groups[1].Confidence)
}

func TestUpstreamConnectionInspectorSub2APIEmptyRatesObjectIsValidDefault(t *testing.T) {
	// Empty data:{} means "no user-specific overrides"; available defaults remain auto-syncable.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 0.25},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 0.25, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceDefault, snapshot.Groups[0].Confidence)
}

func TestUpstreamConnectionInspectorSub2APISuccessEnvelopeWithoutDataIsUnavailable(t *testing.T) {
	// {"success":true} without data must not be treated as an empty rates map.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"success": true})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates response was invalid; showing available-group default rates for reference only")
}

func TestUpstreamConnectionInspectorSub2APIBareItemsObjectIsUnavailable(t *testing.T) {
	// Bare {"items":[]} is not an empty rates map and must not enable default auto-sync.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"items": []any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates response was invalid; showing available-group default rates for reference only")
}

func TestUpstreamConnectionInspectorSub2APIRatesIgnoresUnknownGroupIDs(t *testing.T) {
	// /rates often includes more group IDs than /available. Extra numeric IDs
	// must not invalidate the whole rates table.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{
				"2":  0.25,
				"99": 0.5, // not in available snapshot
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 0.25, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:group_rates", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceOverride, snapshot.Groups[0].Confidence)
}

func TestUpstreamConnectionInspectorSub2APIRatesNonNumericUnknownKeyIsUnavailable(t *testing.T) {
	// Non-numeric garbage keys must not validate rates and promote available defaults.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"foo": 0.5}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates response was invalid; showing available-group default rates for reference only")
}

func TestUpstreamConnectionInspectorSub2APIRatesUnknownGroupIDIllegalValueIsUnavailable(t *testing.T) {
	// Unknown numeric IDs may be ignored only when their multiplier is valid.
	// {"99": null} must not validate the rates table.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"99": nil}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates response was invalid; showing available-group default rates for reference only")
}

func TestIsSub2APIGroupRateIDKeyRejectsAmbiguousOrOverflowKeys(t *testing.T) {
	require.True(t, isSub2APIGroupRateIDKey("12"))
	require.True(t, isSub2APIGroupRateIDKey("99"))

	require.False(t, isSub2APIGroupRateIDKey(""))
	require.False(t, isSub2APIGroupRateIDKey("0"))
	require.False(t, isSub2APIGroupRateIDKey("01"))
	require.False(t, isSub2APIGroupRateIDKey("+99"))
	require.False(t, isSub2APIGroupRateIDKey("-99"))
	require.False(t, isSub2APIGroupRateIDKey("foo"))
	// One digit past math.MaxInt64.
	require.False(t, isSub2APIGroupRateIDKey("9223372036854775808"))
}

func TestUpstreamConnectionInspectorSub2APIRatesAmbiguousUnknownIDKeyIsUnavailable(t *testing.T) {
	// Leading-zero / signed keys must not be treated as ignorable group IDs.
	for _, garbageKey := range []string{"01", "+99"} {
		garbageKey := garbageKey
		t.Run(garbageKey, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/api/v1/auth/me":
					writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
				case "/api/v1/groups/available":
					writeProbeJSON(t, writer, map[string]any{"data": []any{
						map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
					}})
				case "/api/v1/groups/rates":
					writeProbeJSON(t, writer, map[string]any{"data": map[string]any{garbageKey: 0.5}})
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
			snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
				Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
				ManagementBaseURL: server.URL,
			}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

			require.NoError(t, err)
			require.Len(t, snapshot.Groups, 1)
			require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
			require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
			require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
		})
	}
}

func TestUpstreamConnectionInspectorSub2APIRatesArrayIsUnavailable(t *testing.T) {
	// HTTP success with data:[] is not a valid rates map and must not promote defaults.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": []any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates response was invalid; showing available-group default rates for reference only")
}

func TestUpstreamConnectionInspectorSub2APIInvalidKnownGroupRateIsUnavailable(t *testing.T) {
	// Known group present in rates with null/illegal value fails the whole rates map.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 10}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": []any{
				map[string]any{"id": 2, "name": "default", "rate_multiplier": 1.0},
			}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"2": nil}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, 1.0, *snapshot.Groups[0].RateMultiplier)
	require.Equal(t, "sub2api:available_groups", snapshot.Groups[0].Source)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, snapshot.Groups[0].Confidence)
	require.Contains(t, snapshot.Warnings, "groups: user-specific rates response was invalid; showing available-group default rates for reference only")
}

func TestUpstreamConnectionInspectorAutoRejectsGenericSub2APIJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/auth/me", "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderAuto, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.ErrorIs(t, err, errUpstreamConnectionRemoteUserIDRequired)
}

func TestUpstreamConnectionInspectorNewAPIAccessTokenRequiresRemoteUserID(t *testing.T) {
	inspector := newUpstreamConnectionInspector(nil, nil, &http.Client{})
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: "https://example.com",
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})
	require.ErrorIs(t, err, errUpstreamConnectionRemoteUserIDRequired)
}

func TestUpstreamConnectionInspectorOneAPIAccessTokenDiscoversRemoteUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "Bearer management-token", request.Header.Get("Authorization"))
		switch request.URL.Path {
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 3, "group": "default", "quota": 250_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500_000}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.NoError(t, err)
	require.Equal(t, "3", snapshot.RemoteUserID)
	require.Len(t, snapshot.Groups, 1)
}

func TestUpstreamConnectionInspectorOneAPIInvalidAccessTokenIsAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/user/self", request.URL.Path)
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "invalid-management-token"})

	require.ErrorIs(t, err, ErrUpstreamConnectionAuthentication)
}

func TestUpstreamConnectionInspectorPasswordLoginDiscoversMissingRemoteUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "oneapi-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{}})
		case "/api/user/self":
			require.Contains(t, request.Header.Get("Cookie"), "session=oneapi-session")
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 3, "group": "default", "quota": 250_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500_000}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})

	require.NoError(t, err)
	require.Equal(t, "3", snapshot.RemoteUserID)
	require.Len(t, snapshot.Groups, 1)
}

func TestUpstreamConnectionInspectorResolveNewAPIKeyUsesExactGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "Bearer management-token", request.Header.Get("Authorization"))
		require.Equal(t, "23", request.Header.Get("New-API-User"))
		require.Equal(t, "/api/token/search", request.URL.Path)
		require.Equal(t, "sk-local-forwarding-key", request.URL.Query().Get("token"))
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
			map[string]any{"id": 91, "name": "production key", "key": "sk-l**********-key", "group": "vip"},
		}}})
	}))
	defer server.Close()
	multiplier := 0.3
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "23",
		Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier, Confidence: upstreamGroupRateConfidenceDefault}},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sk-local-forwarding-key")

	require.NoError(t, err)
	require.Equal(t, "91", binding.RemoteTokenID)
	require.Equal(t, "production key", binding.RemoteTokenName)
	require.Equal(t, UpstreamBindingResolutionFixed, binding.ResolutionKind)
	require.Equal(t, "vip", binding.RemoteGroupName)
	require.Equal(t, 0.3, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceDefault, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Equal(t, UpstreamBindingApplyAuto, binding.ApplyPolicy)
}

func TestUpstreamConnectionInspectorInheritedKeyWithoutReportedGroupIsUnresolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/token/search":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
				map[string]any{"id": 91, "name": "inherited key"},
			}}})
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 23}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "23",
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sk-local-forwarding-key")

	require.NoError(t, err)
	require.Equal(t, UpstreamBindingResolutionInherited, binding.ResolutionKind)
	require.Equal(t, UpstreamBindingStatusUnresolved, binding.Status)
	require.Empty(t, binding.RemoteGroupName)
	require.Contains(t, binding.LastError, "did not expose a group")
}

func TestUpstreamConnectionInspectorResolveNewAPIAutoGroupDoesNotFlattenMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
			map[string]any{"id": 92, "name": "auto key", "group": "auto", "cross_group_retry": true},
		}}})
	}))
	defer server.Close()
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "23",
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sk-auto-key")

	require.NoError(t, err)
	require.Equal(t, UpstreamBindingResolutionFallbackChain, binding.ResolutionKind)
	require.Nil(t, binding.ObservedMultiplier)
	require.Equal(t, "auto", binding.RemoteGroupName)
}

func TestUpstreamConnectionInspectorResolveOneHubKeyFromPaginatedTokenList(t *testing.T) {
	var listCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/", request.URL.Path)
		require.Equal(t, "1", request.URL.Query().Get("page"))
		require.Equal(t, "100", request.URL.Query().Get("size"))
		listCalls.Add(1)
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
			"data": []any{map[string]any{
				"id": 44, "name": "production", "key": "onehub-secret", "group": "vip", "backup_group": "standard",
			}},
			"page": 1, "size": 100, "total_count": 1,
		}})
	}))
	defer server.Close()

	vipRate := 0.4
	standardRate := 1.0
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneHub, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "9",
		Groups: []UpstreamGroup{
			{Name: "vip", RateMultiplier: &vipRate},
			{Name: "standard", RateMultiplier: &standardRate},
		},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sk-onehub-secret")

	require.NoError(t, err)
	require.Equal(t, int32(1), listCalls.Load())
	require.Equal(t, "44", binding.RemoteTokenID)
	require.Equal(t, "production", binding.RemoteTokenName)
	require.Equal(t, "vip", binding.RemoteGroupName)
	require.Equal(t, UpstreamBindingResolutionFallbackChain, binding.ResolutionKind)
	require.Equal(t, []string{"standard"}, binding.FallbackGroups)
	require.Equal(t, 0.4, *binding.ObservedMultiplier)
	require.Equal(t, "onehub:token_list", binding.Source)
}

func TestUpstreamConnectionInspectorResolveOneAPIKeyFromPaginatedTokenList(t *testing.T) {
	var listCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/token/":
			listCalls.Add(1)
			switch request.URL.Query().Get("p") {
			case "0":
				writeProbeJSON(t, writer, map[string]any{"success": true, "data": []any{
					map[string]any{"id": 7, "name": "default key", "key": "oneapi-secret"},
				}})
			case "1":
				writeProbeJSON(t, writer, map[string]any{"success": true, "data": []any{}})
			default:
				t.Fatalf("unexpected OneAPI page %q", request.URL.Query().Get("p"))
			}
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 3, "group": "default"}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderOneAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "3",
		Groups: []UpstreamGroup{{Name: "default"}},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sk-oneapi-secret")

	require.NoError(t, err)
	require.Equal(t, int32(2), listCalls.Load())
	require.Equal(t, "7", binding.RemoteTokenID)
	require.Equal(t, "default key", binding.RemoteTokenName)
	require.Equal(t, "default", binding.RemoteGroupName)
	require.Equal(t, UpstreamBindingResolutionInherited, binding.ResolutionKind)
	require.Equal(t, UpstreamBindingStatusReady, binding.Status)
	require.Nil(t, binding.ObservedMultiplier)
	require.Equal(t, "oneapi:token_list", binding.Source)
}

func TestUpstreamConnectionInspectorDoneHubUsesGroupMapAndPaginatedTokenList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 6, "group": "grok", "quota": 500_000,
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500_000}})
		case "/api/user_group_map":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"grok": map[string]any{"id": 8, "symbol": "grok", "name": "Grok", "ratio": 0.2},
			}})
		case "/api/token/":
			require.Equal(t, "1", request.URL.Query().Get("page"))
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"data": []any{map[string]any{"id": 18, "name": "grok key", "key": "donehub-secret", "group": "grok"}},
				"page": 1, "size": 100, "total_count": 1,
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	connection := &UpstreamConnection{
		Provider: UpstreamConnectionProviderDoneHub, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "6",
	}
	credential := upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}
	snapshot, err := inspector.Inspect(context.Background(), connection, credential)
	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, "donehub:user_group_map", snapshot.Groups[0].Source)
	connection.Groups = snapshot.Groups

	binding, err := inspector.ResolveKey(context.Background(), connection, credential, "sk-donehub-secret")
	require.NoError(t, err)
	require.Equal(t, "grok", binding.RemoteGroupName)
	require.Equal(t, 0.2, *binding.ObservedMultiplier)
	require.Equal(t, "donehub:token_list", binding.Source)
}

func TestUpstreamConnectionInspectorVeloeraKeepsExactTokenSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/token/search", request.URL.Path)
		require.Equal(t, "4", request.Header.Get("Veloera-User"))
		require.Equal(t, "sk-veloera-secret", request.URL.Query().Get("token"))
		writeProbeJSON(t, writer, map[string]any{"success": true, "data": []any{
			map[string]any{"id": 21, "name": "veloera key", "group": "vip"},
		}})
	}))
	defer server.Close()

	rate := 0.5
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderVeloera, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "4", Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &rate}},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sk-veloera-secret")

	require.NoError(t, err)
	require.Equal(t, "vip", binding.RemoteGroupName)
	require.Equal(t, "veloera:token_search", binding.Source)
}

func TestUpstreamConnectionInspectorResolveSub2APIKeyByExactValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/v1/keys", request.URL.Path)
		writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
			map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
		}}})
	}))
	defer server.Close()
	multiplier := 0.18
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
		Groups: []UpstreamGroup{{
			RemoteID: "2", Name: "grok", RateMultiplier: &multiplier,
			Confidence: upstreamGroupRateConfidenceOverride,
		}},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sub2-secret-key")

	require.NoError(t, err)
	require.Equal(t, "17", binding.RemoteTokenID)
	require.Equal(t, "2", binding.RemoteGroupID)
	require.Equal(t, "grok", binding.RemoteGroupName)
	require.Equal(t, 0.18, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceOverride, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
}

func TestUpstreamConnectionInspectorResolveSub2APIKeyRecordsUnavailableRateConfidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/v1/keys", request.URL.Path)
		writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
			map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
		}}})
	}))
	defer server.Close()
	multiplier := 1.0
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	binding, err := inspector.ResolveKey(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
		Groups: []UpstreamGroup{{
			RemoteID: "2", Name: "grok", RateMultiplier: &multiplier,
			Confidence: upstreamGroupRateConfidenceUnavailable,
		}},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"}, "sub2-secret-key")

	require.NoError(t, err)
	require.Equal(t, UpstreamBindingStatusReady, binding.Status)
	require.Equal(t, 1.0, *binding.ObservedMultiplier)
	require.Equal(t, upstreamGroupRateConfidenceUnavailable, binding.ResolutionDetails[upstreamBindingRateConfidenceDetailKey])
	require.Nil(t, observedAccountRateMultiplier(&binding))
}

func TestUpstreamConnectionInspectorPreparedNewAPIResolverAuthenticatesOnce(t *testing.T) {
	var loginCalls atomic.Int32
	var searchCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/user/login":
			loginCalls.Add(1)
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "shared-session"})
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"id": 8}})
		case "/api/token/search":
			searchCalls.Add(1)
			require.Contains(t, request.Header.Get("Cookie"), "session=shared-session")
			key := request.URL.Query().Get("token")
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{"items": []any{
				map[string]any{"id": searchCalls.Load(), "name": key, "group": "vip"},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	multiplier := 0.5
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	resolver, err := inspector.PrepareKeyResolver(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModePassword),
		ManagementBaseURL: server.URL, Groups: []UpstreamGroup{{Name: "vip", RateMultiplier: &multiplier}},
	}, upstreamConnectionCredential{Version: 1, Username: "alice", Password: "secret"})
	require.NoError(t, err)

	_, err = resolver(context.Background(), "sk-one")
	require.NoError(t, err)
	_, err = resolver(context.Background(), "sk-two")
	require.NoError(t, err)
	require.Equal(t, int32(1), loginCalls.Load())
	require.Equal(t, int32(2), searchCalls.Load())
}

func TestUpstreamConnectionInspectorPreparedSub2APIResolverListsKeysOnce(t *testing.T) {
	var listCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/v1/keys", request.URL.Path)
		listCalls.Add(1)
		writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
			map[string]any{"id": 1, "name": "one", "key": "sub2-one", "group_id": 2},
			map[string]any{"id": 2, "name": "two", "key": "sub2-two", "group_id": 2},
		}}})
	}))
	defer server.Close()
	multiplier := 0.2
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	resolver, err := inspector.PrepareKeyResolver(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
		Groups:            []UpstreamGroup{{RemoteID: "2", Name: "default", RateMultiplier: &multiplier}},
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})
	require.NoError(t, err)

	_, err = resolver(context.Background(), "sub2-one")
	require.NoError(t, err)
	_, err = resolver(context.Background(), "sub2-two")
	require.NoError(t, err)
	require.Equal(t, int32(1), listCalls.Load())
}

func TestUpstreamConnectionInspectorSub2APIRejectsTruncatedKeyListing(t *testing.T) {
	var listCalls atomic.Int32
	items := make([]any, 100)
	for index := range items {
		items[index] = map[string]any{
			"id":       index + 1,
			"key":      fmt.Sprintf("sub2-key-%d", index+1),
			"group_id": 2,
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, "/api/v1/keys", request.URL.Path)
		listCalls.Add(1)
		writeProbeJSON(t, writer, map[string]any{"data": map[string]any{
			"items": items,
			"pages": 101,
		}})
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	_, err := inspector.PrepareKeyResolver(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}, upstreamConnectionCredential{Version: 1, AccessToken: "management-token"})

	require.EqualError(t, err, "Sub2API key listing exceeds the 100-page safety limit")
	require.Equal(t, int32(1), listCalls.Load())
}

func TestUpstreamConnectionInspectorSub2APIReusesCredentialUserAgent(t *testing.T) {
	const expectedUserAgent = "Mozilla/5.0 exact-login-agent"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, expectedUserAgent, request.Header.Get("User-Agent"))
		require.Equal(t, "Bearer management-token", request.Header.Get("Authorization"))
		switch request.URL.Path {
		case "/api/v1/auth/me":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"id": 11, "balance": 42.75}})
		case "/api/v1/groups/available":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 2, "name": "grok"},
			}}})
		case "/api/v1/groups/rates":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"2": 0.18}})
		case "/api/v1/keys":
			writeProbeJSON(t, writer, map[string]any{"data": map[string]any{"items": []any{
				map[string]any{"id": 17, "name": "codex", "key": "sub2-secret-key", "group_id": 2},
			}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	credential := upstreamConnectionCredential{
		Version: 1, AccessToken: "management-token", UserAgent: expectedUserAgent,
	}
	connection := &UpstreamConnection{
		Provider: UpstreamConnectionProviderSub2API, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL,
	}
	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), connection, credential)
	require.NoError(t, err)
	require.Equal(t, 0.18, *snapshot.Groups[0].RateMultiplier)
	connection.Groups = snapshot.Groups

	binding, err := inspector.ResolveKey(context.Background(), connection, credential, "sub2-secret-key")
	require.NoError(t, err)
	require.Equal(t, "grok", binding.RemoteGroupName)
	require.Equal(t, 0.18, *binding.ObservedMultiplier)
}

func TestUpstreamConnectionInspectorNewAPIReusesCredentialUserAgent(t *testing.T) {
	const expectedUserAgent = "Mozilla/5.0 exact-login-agent"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		require.Equal(t, expectedUserAgent, request.Header.Get("User-Agent"))
		require.Equal(t, "Bearer management-token", request.Header.Get("Authorization"))
		switch request.URL.Path {
		case "/api/user/self":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"id": 7, "group": "vip", "quota": 500_000,
			}})
		case "/api/user/self/groups":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"group_ratio": map[string]any{"vip": 0.3},
			}})
		case "/api/status":
			writeProbeJSON(t, writer, map[string]any{"success": true, "data": map[string]any{
				"quota_per_unit": 500_000, "quota_display_type": "USD",
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	inspector := newUpstreamConnectionInspector(nil, nil, server.Client())
	snapshot, err := inspector.Inspect(context.Background(), &UpstreamConnection{
		Provider: UpstreamConnectionProviderNewAPI, AuthMode: string(UpstreamManagementAuthModeAccessToken),
		ManagementBaseURL: server.URL, RemoteUserID: "7",
	}, upstreamConnectionCredential{
		Version: 1, AccessToken: "management-token", UserAgent: expectedUserAgent,
	})

	require.NoError(t, err)
	require.Equal(t, "7", snapshot.RemoteUserID)
	require.Equal(t, 1.0, *snapshot.Wallet.USD)
	require.Equal(t, 0.3, *snapshot.Groups[0].RateMultiplier)
}

func writeProbeJSON(t *testing.T, writer http.ResponseWriter, payload any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(writer).Encode(payload))
}
