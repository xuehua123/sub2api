package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotImageCountBillingRoundtrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			groupID := int64(50)
			apiKey := &APIKey{
				ID: 82, UserID: 40, GroupID: &groupID, Key: "sk-image-count-roundtrip", Status: StatusActive,
				User: &User{ID: 40, Status: StatusActive},
				Group: &Group{
					ID: groupID, Platform: PlatformOpenAI, Status: StatusActive,
					AllowImageGeneration: true, ImageCountBilling: enabled,
				},
			}
			svc := &APIKeyService{}
			snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
			require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
			payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
			require.NoError(t, err)
			var cached APIKeyAuthCacheEntry
			require.NoError(t, json.Unmarshal(payload, &cached))

			materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
			require.NoError(t, err)
			require.True(t, used)
			require.NotNil(t, materialized.Group)
			require.Equal(t, enabled, materialized.Group.ImageCountBilling)
			require.Equal(t, enabled, imageCountBillingApplies(materialized, true))
		})
	}
}

func TestAPIKeyServiceRejectsV24AuthSnapshotWithoutImageCountBilling(t *testing.T) {
	svc := &APIKeyService{}
	apiKey, used, err := svc.applyAuthCacheEntry("sk-legacy-image-count", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 24},
	})
	require.NoError(t, err)
	require.False(t, used, "不含按张计费配置的旧快照必须回源重建")
	require.Nil(t, apiKey)
}
