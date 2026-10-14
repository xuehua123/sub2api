//go:build unit

package handler

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type cachedBillingEntitlementRepo struct {
	service.SubscriptionEntitlementRepository
	ent *service.SubscriptionEntitlement
}

func (r *cachedBillingEntitlementRepo) GetByID(_ context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	if id != r.ent.ID {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	copy := *r.ent
	return &copy, nil
}

type cachedBillingCommandRepo struct {
	service.UsageBillingRepository
	mu   sync.Mutex
	cmds []*service.UsageBillingCommand
}

func (r *cachedBillingCommandRepo) Apply(_ context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *cmd
	r.cmds = append(r.cmds, &copy)
	return &service.UsageBillingApplyResult{Applied: true, BillingSource: service.BillingSourceEntitlementQuota}, nil
}

func (r *cachedBillingCommandRepo) first(t *testing.T) *service.UsageBillingCommand {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.cmds)
	return r.cmds[0]
}

func newCachedBillingEntitlementEnv(t *testing.T, limit float64) (*cachedBillingEnv, *cachedBillingCommandRepo) {
	t.Helper()
	billingRepo := &cachedBillingCommandRepo{}
	env := newCachedBillingEnvWithBillingRepo(t, 0, billingRepo, 900)
	now := time.Now()
	ent := &service.SubscriptionEntitlement{
		ID: 91, UserID: env.apiKey.UserID, Status: service.SubscriptionStatusActive,
		StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(24 * time.Hour),
		DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now,
		MonthlyLimitUSD: &limit, OveragePolicy: service.SubscriptionEntitlementOverageBlock,
		Groups: []service.Group{*env.group},
	}
	apiKeySvc := service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	apiKeySvc.SetSubscriptionEntitlementDependencies(nil, service.NewSubscriptionEntitlementService(&cachedBillingEntitlementRepo{ent: ent}, nil))
	env.h.apiKeyService = apiKeySvc
	env.entitlement = ent
	env.apiKey.SubscriptionEntitlementID = &ent.ID
	env.apiKey.AccessSource = service.APIKeyAccessSourceEntitlement
	return env, billingRepo
}

func TestGeminiCachedContentsCreate_ZeroBalanceUsesEntitlement(t *testing.T) {
	env, billingRepo := newCachedBillingEntitlementEnv(t, 500)
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))
	rec := env.create()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	env.waitForUsage(t, 1)
	cmd := billingRepo.first(t)
	require.Equal(t, env.entitlement.ID, *cmd.EntitlementID)
	require.Zero(t, cmd.BalanceCost)
	require.InDelta(t, cachedBillingTokens*cachedBillingInputPrice+cachedBillingTokens*cachedBillingStoragePrice*24, cmd.SubscriptionCost, 1e-9)
	require.Equal(t, env.entitlement.ID, *env.usage.first().EntitlementID)
}

func TestGeminiCachedContentsPatch_ZeroBalanceUsesEntitlement(t *testing.T) {
	env, billingRepo := newCachedBillingEntitlementEnv(t, 500)
	env.seed(time.Now().Add(time.Hour))
	rec := env.patch()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	env.waitForUsage(t, 1)
	cmd := billingRepo.first(t)
	require.Equal(t, env.entitlement.ID, *cmd.EntitlementID)
	require.Zero(t, cmd.BalanceCost)
	require.InDelta(t, cachedBillingTokens*cachedBillingStoragePrice*23, cmd.SubscriptionCost, 0.01)
	require.False(t, cmd.AllowEntitlementOverage, "storage extension must enforce the entitlement quota")
}

func TestGeminiCachedContentsPatch_InsufficientEntitlementRejectsBeforeUpstream(t *testing.T) {
	env, _ := newCachedBillingEntitlementEnv(t, 1)
	env.seed(time.Now().Add(time.Hour))
	rec := env.patch()
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	require.Zero(t, env.upstream.methodCalls(http.MethodPatch))
	require.False(t, env.repo.pendingMutation(100))
	require.Empty(t, env.usage.totals())
}

func TestGeminiCachedContentsCreate_InsufficientStorageCleansUpAndBillsInputToEntitlement(t *testing.T) {
	env, billingRepo := newCachedBillingEntitlementEnv(t, 1)
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))
	rec := env.create()
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete))
	require.Empty(t, env.repo.records)
	env.waitForUsage(t, 1)
	cmd := billingRepo.first(t)
	require.Equal(t, env.entitlement.ID, *cmd.EntitlementID)
	require.Zero(t, cmd.BalanceCost)
	require.InDelta(t, cachedBillingTokens*cachedBillingInputPrice, cmd.SubscriptionCost, 1e-9)
	require.True(t, cmd.AllowEntitlementOverage, "input already consumed upstream must still be recorded")
}

func TestGeminiCachedContentsCreate_SaveFailureBillsInputToEntitlement(t *testing.T) {
	env, billingRepo := newCachedBillingEntitlementEnv(t, 500)
	env.repo.failCreate = true
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))
	rec := env.create()
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete))
	require.Empty(t, env.repo.records)
	env.waitForUsage(t, 1)
	cmd := billingRepo.first(t)
	require.Equal(t, env.entitlement.ID, *cmd.EntitlementID)
	require.Zero(t, cmd.BalanceCost)
	require.InDelta(t, cachedBillingTokens*cachedBillingInputPrice, cmd.SubscriptionCost, 1e-9)
	require.True(t, cmd.AllowEntitlementOverage)
}

func TestGeminiCachedContentsPatch_APIKeyQuotaStillAppliesToEntitlement(t *testing.T) {
	env, _ := newCachedBillingEntitlementEnv(t, 500)
	env.apiKey.Quota = 1
	env.seed(time.Now().Add(time.Hour))
	rec := env.patch()
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Zero(t, env.upstream.methodCalls(http.MethodPatch))
	require.Empty(t, env.usage.totals())
}

func TestGeminiCachedContentsPatch_MutationConflictDoesNotAdvanceEntitlementCycle(t *testing.T) {
	env, _ := newCachedBillingEntitlementEnv(t, 500)
	env.entitlement.MonthlyUsageUSD = 500
	env.entitlement.AutoAdvanceMonthly = true
	env.seed(time.Now().Add(time.Hour))
	lease := cachedBillingMutation{token: "other", leaseUntil: time.Now().Add(time.Minute)}
	env.repo.mutations[100] = lease

	// The entitlement repository deliberately cannot advance a monthly cycle.
	// Admission before checking the lease would return a maintenance error.
	rec := env.patch()
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "ABORTED")
	require.Zero(t, env.upstream.methodCalls(http.MethodPatch))
	require.Equal(t, lease, env.repo.mutations[100])
	require.Empty(t, env.usage.totals())
}

func TestGeminiCachedContents_ModelAllowlistBlocksCreateAndBoundPatch(t *testing.T) {
	for _, create := range []bool{true, false} {
		name := "bound patch"
		if create {
			name = "create"
		}
		t.Run(name, func(t *testing.T) {
			env := newCachedBillingEnv(t, 500, 900)
			env.group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-allowed"}}
			env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))
			env.seed(time.Now().Add(time.Hour))
			var status int
			if create {
				status = env.create().Code
			} else {
				status = env.patch().Code
			}
			require.Equal(t, http.StatusNotFound, status)
			require.Zero(t, env.upstream.methodCalls(http.MethodPost))
			require.Zero(t, env.upstream.methodCalls(http.MethodPatch))
			require.False(t, env.repo.pendingMutation(100))
			require.Empty(t, env.usage.totals())
		})
	}
}

func TestGeminiCachedContents_ModelAllowlistAcceptsGeminiResourceNames(t *testing.T) {
	env := newCachedBillingEnv(t, 500, 900)
	env.group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-2.5-pro"}}
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))
	rec := env.create()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPost))
}

func TestGeminiCachedContentsPatch_ModelAllowlistUsesOriginalRequestedModel(t *testing.T) {
	for _, allowedModel := range []string{"gemini-alias", "gemini-2.5-pro"} {
		t.Run(allowedModel, func(t *testing.T) {
			env := newCachedBillingEnv(t, 500, 900)
			env.seed(time.Now().Add(time.Hour))
			env.repo.records["abc"].ChannelUsage.OriginalModel = "gemini-alias"
			env.group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{allowedModel}}

			rec := env.patch()
			if allowedModel == "gemini-alias" {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch))
				return
			}
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Zero(t, env.upstream.methodCalls(http.MethodPatch))
			require.False(t, env.repo.pendingMutation(100))
			require.Empty(t, env.usage.totals())
		})
	}
}
