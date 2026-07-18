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
		Bindings: []service.UpstreamAccountBinding{{KeyFingerprint: "key-fingerprint"}},
	})

	payload, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "cipher-secret")
	require.NotContains(t, string(payload), "credential-fingerprint")
	require.NotContains(t, string(payload), "key-fingerprint")
	require.Contains(t, string(payload), "abcd...wxyz")
}
