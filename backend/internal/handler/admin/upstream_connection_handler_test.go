//go:build unit

package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamConnectionResponseNeverSerializesSecretMaterial(t *testing.T) {
	response := upstreamConnectionToResponse(&service.UpstreamConnection{
		ID: 1, Name: "upstream", CredentialEncrypted: "cipher-secret",
		CredentialFingerprint: "credential-fingerprint", CredentialHint: "abcd...wxyz",
		BoundAccountIDs: []int64{17, 23},
		Bindings:        []service.UpstreamAccountBinding{{KeyFingerprint: "key-fingerprint"}},
	})

	payload, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "cipher-secret")
	require.NotContains(t, string(payload), "credential-fingerprint")
	require.NotContains(t, string(payload), "key-fingerprint")
	require.Contains(t, string(payload), "abcd...wxyz")
	require.Contains(t, string(payload), `"bound_account_ids":[17,23]`)
}

func TestUpstreamCredentialRequestToServiceCapturesRequestUserAgent(t *testing.T) {
	credential := upstreamCredentialRequestToService(upstreamConnectionCredentialRequest{
		AccessToken: "management-token",
	}, "Mozilla/5.0 exact-login-agent")

	require.Equal(t, "management-token", credential.AccessToken)
	require.Equal(t, "Mozilla/5.0 exact-login-agent", credential.UserAgent)
}
