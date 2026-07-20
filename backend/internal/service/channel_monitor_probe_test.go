//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorProbeGatewayFailoverPolicy(t *testing.T) {
	ctx := WithChannelMonitorProbe(context.Background())

	gatewaySvc := &GatewayService{}
	require.False(t, gatewaySvc.shouldFailoverUpstreamError(http.StatusBadRequest))
	require.True(t, gatewaySvc.shouldFailoverUpstreamErrorForContext(ctx, http.StatusBadRequest))

	antigravitySvc := &AntigravityGatewayService{}
	require.False(t, antigravitySvc.shouldFailoverUpstreamError(http.StatusBadRequest))
	require.True(t, antigravitySvc.shouldFailoverUpstreamErrorForContext(ctx, http.StatusBadRequest))
}

func TestAccountSwitchLimitForContext_ChannelMonitorProbe(t *testing.T) {
	require.Equal(t, 3, AccountSwitchLimitForContext(context.Background(), 3))
	require.Equal(t, ChannelMonitorProbeMaxAccountSwitches, AccountSwitchLimitForContext(WithChannelMonitorProbe(context.Background()), 3))
	require.Equal(t, ChannelMonitorProbeMaxAccountSwitches, AccountSwitchLimitForContext(WithChannelMonitorProbe(context.Background()), ChannelMonitorProbeMaxAccountSwitches+1))
}

func TestChannelMonitorProbeSignatureValidation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	headers := map[string]string{}

	AddChannelMonitorProbeHeaders(headers, http.MethodPost, "/v1/responses", now)

	require.Equal(t, ChannelMonitorProbeHeaderValue(), headers[ChannelMonitorProbeHeaderName])
	require.NotEmpty(t, headers[ChannelMonitorProbeTSHeaderName])
	require.NotEmpty(t, headers[ChannelMonitorProbeSigHeaderName])
	require.True(t, IsValidChannelMonitorProbe(
		http.MethodPost,
		"/v1/responses",
		headers[ChannelMonitorProbeHeaderName],
		headers[ChannelMonitorProbeTSHeaderName],
		headers[ChannelMonitorProbeSigHeaderName],
		now,
	))
	require.False(t, IsValidChannelMonitorProbe(
		http.MethodGet,
		"/v1/responses",
		headers[ChannelMonitorProbeHeaderName],
		headers[ChannelMonitorProbeTSHeaderName],
		headers[ChannelMonitorProbeSigHeaderName],
		now,
	))
	require.False(t, IsValidChannelMonitorProbe(
		http.MethodPost,
		"/v1/embeddings",
		headers[ChannelMonitorProbeHeaderName],
		headers[ChannelMonitorProbeTSHeaderName],
		headers[ChannelMonitorProbeSigHeaderName],
		now,
	))
	require.False(t, IsValidChannelMonitorProbe(
		http.MethodPost,
		"/v1/responses",
		headers[ChannelMonitorProbeHeaderName],
		headers[ChannelMonitorProbeTSHeaderName],
		headers[ChannelMonitorProbeSigHeaderName],
		now.Add(channelMonitorProbeSignatureTTL+time.Second),
	))
	require.False(t, IsValidChannelMonitorProbe(
		http.MethodPost,
		"/v1/responses",
		ChannelMonitorProbeHeaderValue(),
		"",
		"",
		now,
	))
}

func TestChannelMonitorProbeConfiguredSecretValidatesAcrossInstances(t *testing.T) {
	restore := snapshotChannelMonitorProbeSigningKey(t)
	defer restore()

	now := time.Unix(1_700_000_000, 0)
	ConfigureChannelMonitorProbeSecret("shared-secret-value-with-at-least-32-bytes")
	headers := map[string]string{}
	AddChannelMonitorProbeHeaders(headers, http.MethodPost, "/v1/responses", now)

	channelMonitorProbeSigningKeyMu.Lock()
	channelMonitorProbeSigningKey = nil
	channelMonitorProbeSigningKeyMu.Unlock()
	ConfigureChannelMonitorProbeSecret("shared-secret-value-with-at-least-32-bytes")

	require.True(t, IsValidChannelMonitorProbe(
		http.MethodPost,
		"/v1/responses",
		headers[ChannelMonitorProbeHeaderName],
		headers[ChannelMonitorProbeTSHeaderName],
		headers[ChannelMonitorProbeSigHeaderName],
		now,
	))

	channelMonitorProbeSigningKeyMu.Lock()
	channelMonitorProbeSigningKey = nil
	channelMonitorProbeSigningKeyMu.Unlock()
	ConfigureChannelMonitorProbeSecret("different-secret-value-with-at-least-32-bytes")

	require.False(t, IsValidChannelMonitorProbe(
		http.MethodPost,
		"/v1/responses",
		headers[ChannelMonitorProbeHeaderName],
		headers[ChannelMonitorProbeTSHeaderName],
		headers[ChannelMonitorProbeSigHeaderName],
		now,
	))
}

func TestChannelMonitorProbeExcludedAccountsRoundTripAndContextCopy(t *testing.T) {
	headers := map[string]string{}
	input := map[int64]struct{}{8: {}, 3: {}, -1: {}, 11: {}}

	AddChannelMonitorProbeExcludedAccounts(headers, input)
	require.Equal(t, "3,8,11", headers[ChannelMonitorProbeExcludedAccountsHeaderName])

	parsed := ParseChannelMonitorProbeExcludedAccounts("3, invalid, 8, 0, 11")
	require.Equal(t, map[int64]struct{}{3: {}, 8: {}, 11: {}}, parsed)

	ctx := WithChannelMonitorProbeExcludedAccounts(context.Background(), parsed)
	delete(parsed, 3)
	fromContext := ChannelMonitorProbeExcludedAccounts(ctx)
	require.Contains(t, fromContext, int64(3))
	delete(fromContext, 8)
	require.Contains(t, ChannelMonitorProbeExcludedAccounts(ctx), int64(8))
}

func snapshotChannelMonitorProbeSigningKey(t *testing.T) func() {
	t.Helper()
	channelMonitorProbeSigningKeyMu.RLock()
	original := append([]byte(nil), channelMonitorProbeSigningKey...)
	channelMonitorProbeSigningKeyMu.RUnlock()
	return func() {
		channelMonitorProbeSigningKeyMu.Lock()
		defer channelMonitorProbeSigningKeyMu.Unlock()
		channelMonitorProbeSigningKey = original
	}
}
