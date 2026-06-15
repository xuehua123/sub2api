//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type subscriptionEntitlementLinkRepoStub struct {
	UserSubscriptionRepository
	subs []UserSubscription
}

func (r *subscriptionEntitlementLinkRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	out := make([]UserSubscription, len(r.subs))
	copy(out, r.subs)
	return out, nil
}

func TestSubscriptionServiceListUserSubscriptionsPreservesEntitlementLink(t *testing.T) {
	now := time.Now().UTC()
	planID := int64(88)
	planName := "Backfilled Plan"
	primaryGroupID := int64(77)
	repo := &subscriptionEntitlementLinkRepoStub{
		subs: []UserSubscription{
			{
				ID:        1,
				UserID:    10,
				GroupID:   20,
				StartsAt:  now.Add(-time.Hour),
				ExpiresAt: now.Add(time.Hour),
				Status:    SubscriptionStatusActive,
			},
			{
				ID:        2,
				UserID:    10,
				GroupID:   21,
				StartsAt:  now.Add(-time.Hour),
				ExpiresAt: now.Add(time.Hour),
				Status:    SubscriptionStatusActive,
				EntitlementLink: &UserSubscriptionEntitlementLink{
					EntitlementID:  99,
					PlanID:         &planID,
					PlanName:       &planName,
					Status:         SubscriptionStatusActive,
					ExpiresAt:      now.Add(2 * time.Hour),
					PrimaryGroupID: &primaryGroupID,
					OveragePolicy:  SubscriptionEntitlementOverageBlock,
				},
			},
		},
	}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	got, err := svc.ListUserSubscriptions(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Nil(t, got[0].EntitlementLink)
	require.NotNil(t, got[1].EntitlementLink)
	require.Equal(t, int64(99), got[1].EntitlementLink.EntitlementID)
	require.Equal(t, planID, *got[1].EntitlementLink.PlanID)
	require.Equal(t, planName, *got[1].EntitlementLink.PlanName)
}

var _ UserSubscriptionRepository = (*subscriptionEntitlementLinkRepoStub)(nil)
