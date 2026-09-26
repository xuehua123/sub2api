//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

type groupModelFixture struct {
	repo *upstreamConnectionRepository
	ref  service.UpstreamGroupReference
	now  time.Time
}

func newGroupModelFixture(t *testing.T) *groupModelFixture {
	t.Helper()
	ctx := context.Background()
	repo := &upstreamConnectionRepository{client: integrationEntClient}
	c := &service.UpstreamConnection{Name: "models", Provider: "sub2api", AuthMode: "access_token", ManagementBaseURL: "https://models.invalid", CredentialEncrypted: "fixture", CredentialFingerprint: fmt.Sprint(time.Now().UnixNano()), Status: "ready", SyncEnabled: true, SyncIntervalSeconds: 60, Version: 1}
	require.NoError(t, repo.Create(ctx, c))
	t.Cleanup(func() { _ = integrationEntClient.UpstreamConnection.DeleteOneID(c.ID).Exec(ctx) })
	_, err := integrationEntClient.UpstreamGroup.Create().SetConnectionID(c.ID).SetRemoteID("12").SetName("Group").Save(ctx)
	require.NoError(t, err)
	return &groupModelFixture{repo: repo, ref: service.UpstreamGroupReference{ConnectionID: c.ID, RemoteKey: "id:12"}, now: time.Now().UTC()}
}

func (f *groupModelFixture) account(t *testing.T, n int) int64 {
	t.Helper()
	ctx := context.Background()
	key := fmt.Sprintf("fixture-key-%d", n)
	a, err := integrationEntClient.Account.Create().SetName("model-key").SetPlatform("openai").SetType(service.AccountTypeAPIKey).
		SetCredentials(map[string]any{"api_key": key, "base_url": "https://models.invalid"}).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = integrationEntClient.Account.DeleteOneID(a.ID).Exec(ctx) })
	_, err = integrationEntClient.UpstreamAccountBinding.Create().SetConnectionID(f.ref.ConnectionID).SetAccountID(a.ID).SetRemoteGroupID("12").SetStatus("ready").SetConfidence("exact").SetResolutionKind("fixed").SetKeyFingerprint(fmt.Sprintf("sha256:v1:%x", sha256.Sum256([]byte("upstream-api-key\x00"+key)))).SetFreshUntil(time.Now().Add(24 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	return a.ID
}

func (f *groupModelFixture) claim(t *testing.T, token string) {
	t.Helper()
	f.now = f.now.Add(time.Minute)
	ok, manual, err := f.repo.ClaimGroupModels(context.Background(), f.ref, 1, token, f.now, true)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, manual)
}

func (f *groupModelFixture) sources(t *testing.T, force bool) []service.UpstreamGroupModelAccountSource {
	t.Helper()
	sources, err := f.repo.ListGroupModelAccountSources(context.Background(), f.ref, 1, f.now, 8, force)
	require.NoError(t, err)
	return sources
}

func (f *groupModelFixture) saveAccounts(t *testing.T, token string, results ...service.UpstreamGroupModelAccountResult) *service.UpstreamGroupModels {
	t.Helper()
	ok, err := f.repo.SaveGroupModels(context.Background(), f.ref, 1, token, service.UpstreamGroupModels{Source: "bound_keys", Coverage: "bound_keys", Status: "pending", AccountResults: results}, f.now)
	require.NoError(t, err)
	require.True(t, ok)
	return f.read(t)
}

func (f *groupModelFixture) read(t *testing.T) *service.UpstreamGroupModels {
	t.Helper()
	result, err := f.repo.GetGroupModels(context.Background(), f.ref)
	require.NoError(t, err)
	return result
}

func modelAccountResult(source service.UpstreamGroupModelAccountSource, success bool, model, tag string) service.UpstreamGroupModelAccountResult {
	r := service.UpstreamGroupModelAccountResult{AccountID: source.Account.ID, Fingerprint: source.Fingerprint, Success: success}
	if success {
		r.Models = []string{model}
		r.AutoTags = []string{tag}
	}
	return r
}

func TestUpstreamGroupModelSnapshotsAndOverrides(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	require.Equal(t, "unknown", f.read(t).Status)
	f.claim(t, "published")
	ok, _, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "duplicate", f.now, true)
	require.NoError(t, err)
	require.False(t, ok)
	snap := service.UpstreamGroupModels{Models: []string{"claude-sonnet-4-6", "gpt-5"}, AutoTags: []string{"Claude", "GPT", "Text"}, Source: "published", Coverage: "published", Status: "ready"}
	ok, err = f.repo.SaveGroupModels(ctx, f.ref, 1, "published", snap, f.now)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, f.repo.UpdateGroupAnnotations(ctx, service.UpstreamGroupAnnotationUpdate{Groups: []service.UpstreamGroupReference{f.ref}, RemoveTags: []string{"GPT"}, AddTags: []string{"primary"}}))
	ok, err = f.repo.ApplyProbeSuccess(ctx, f.ref.ConnectionID, 1, service.UpstreamConnectionProbePersistence{GroupsObserved: true, Groups: []service.UpstreamGroup{{RemoteID: "12", Name: "Renamed"}}, Status: "ready"})
	require.NoError(t, err)
	require.True(t, ok)
	params := service.UpstreamGroupCatalogParams{Page: 1, PageSize: 20, ConnectionIDs: []int64{f.ref.ConnectionID}, Search: "gpt-5"}
	catalog, err := f.repo.ListGroupCatalog(ctx, params, f.now)
	require.NoError(t, err)
	require.Len(t, catalog.Items, 1)
	require.Equal(t, 2, catalog.Items[0].ModelCount)
	require.NotContains(t, catalog.Items[0].Tags, "GPT")
	require.Contains(t, catalog.Items[0].Tags, "primary")
	f.claim(t, "failure")
	ok, err = f.repo.SaveGroupModels(ctx, f.ref, 1, "failure", service.UpstreamGroupModels{Status: "error", ErrorCode: "unavailable"}, f.now)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, snap.Models, f.read(t).Models)
	require.Equal(t, "error", f.read(t).Status)
	require.NoError(t, f.repo.UpdateGroupAnnotations(ctx, service.UpstreamGroupAnnotationUpdate{Groups: []service.UpstreamGroupReference{f.ref}, ResetAutoTags: true}))
	params.Tag = "GPT"
	catalog, err = f.repo.ListGroupCatalog(ctx, params, f.now)
	require.NoError(t, err)
	require.Equal(t, int64(1), catalog.Total)
	_, err = integrationEntClient.UpstreamConnection.UpdateOneID(f.ref.ConnectionID).SetVersion(2).Save(ctx)
	require.NoError(t, err)
	require.Empty(t, f.read(t).Models)
}

func TestUpstreamGroupModelsIgnoreUsageUpdatesAndOrdinaryBindingRefresh(t *testing.T) {
	f := newGroupModelFixture(t)
	id := f.account(t, 1)
	f.claim(t, "usage")
	source := f.sources(t, true)[0]
	_, err := integrationEntClient.ExecContext(context.Background(), `UPDATE accounts SET last_used_at=NOW(),updated_at=NOW(),rate_multiplier=2,extra=jsonb_build_object('quota_daily_used',20) WHERE id=$1`, id)
	require.NoError(t, err)
	_, err = integrationEntClient.ExecContext(context.Background(), `UPDATE upstream_account_bindings SET observed_at=NOW(),updated_at=NOW(),fresh_until=NOW()+INTERVAL '24 hours',observed_multiplier=2 WHERE account_id=$1`, id)
	require.NoError(t, err)
	got := f.saveAccounts(t, "usage", modelAccountResult(source, true, "gpt-5", "GPT"))
	require.Equal(t, "ready", got.Status)
	require.Equal(t, []string{"gpt-5"}, got.Models)
	require.Equal(t, source.Fingerprint, f.sources(t, true)[0].Fingerprint)
}

func TestUpstreamGroupModelsRetainUnchangedIdentityOnProbeFailureOrExpiry(t *testing.T) {
	for _, condition := range []string{"probe_error", "expired", "missing_expiry"} {
		t.Run(condition, func(t *testing.T) {
			f := newGroupModelFixture(t)
			ctx := context.Background()
			id := f.account(t, 1)
			f.claim(t, "first")
			source := f.sources(t, true)[0]
			ready := f.saveAccounts(t, "first", modelAccountResult(source, true, "gpt-old", "GPT"))
			query := `UPDATE upstream_account_bindings SET status='error',last_error='temporary timeout' WHERE account_id=$1`
			if condition == "expired" {
				query = `UPDATE upstream_account_bindings SET fresh_until=NOW()-INTERVAL '1 second' WHERE account_id=$1`
			}
			if condition == "missing_expiry" {
				query = `UPDATE upstream_account_bindings SET fresh_until=NULL WHERE account_id=$1`
			}
			_, err := integrationEntClient.ExecContext(ctx, query, id)
			require.NoError(t, err)
			got := f.read(t)
			require.Equal(t, ready.Models, got.Models)
			require.Equal(t, ready.AutoTags, got.AutoTags)
			require.Equal(t, ready.ObservedAt, got.ObservedAt)
			require.Zero(t, got.ReadySourceCount)
			if condition == "probe_error" {
				require.Equal(t, "partial", got.Status)
				require.Equal(t, 1, got.FailedSourceCount)
			} else {
				require.Equal(t, "stale", got.Status)
				require.Equal(t, 1, got.StaleSourceCount)
			}
			require.Empty(t, f.sources(t, true), "new fetches still require a fresh confirmed binding")
			page, err := f.repo.ListGroupCatalog(ctx, service.UpstreamGroupCatalogParams{Page: 1, PageSize: 20, ConnectionIDs: []int64{f.ref.ConnectionID}, Search: "gpt-old", Tag: "GPT"}, f.now)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			require.Equal(t, got.Status, page.Items[0].ModelStatus)
			// Rejected in-flight writes cannot turn a stale observation into fresh.
			f.claim(t, "inflight")
			got = f.saveAccounts(t, "inflight", modelAccountResult(source, true, "must-not-publish", "Image"))
			require.Equal(t, ready.Models, got.Models)
			_, err = integrationEntClient.ExecContext(ctx, `UPDATE upstream_account_bindings SET status='ready',last_error='',fresh_until=NOW()+INTERVAL '1 hour' WHERE account_id=$1`, id)
			require.NoError(t, err)
			require.Equal(t, "ready", f.read(t).Status)
			// A subsequent real identity change must hide data even during failure.
			_, err = integrationEntClient.ExecContext(ctx, `UPDATE accounts SET credentials=jsonb_set(credentials,'{base_url}','"https://changed.invalid"') WHERE id=$1`, id)
			require.NoError(t, err)
			_, err = integrationEntClient.ExecContext(ctx, `UPDATE upstream_account_bindings SET status='error' WHERE account_id=$1`, id)
			require.NoError(t, err)
			require.Empty(t, f.read(t).Models)
			require.Empty(t, f.read(t).AutoTags)
		})
	}
}

func TestUpstreamGroupModelsInvalidateChangedSourcesOnBothReadPaths(t *testing.T) {
	for _, change := range []string{"rebind", "credentials", "base_url", "delete", "unbind", "fallback", "headers", "platform", "custom_base"} {
		t.Run(change, func(t *testing.T) {
			f := newGroupModelFixture(t)
			id := f.account(t, 1)
			f.claim(t, "before")
			source := f.sources(t, true)[0]
			f.saveAccounts(t, "before", modelAccountResult(source, true, "obsolete-model", "OldTag"))
			query := map[string]string{
				"rebind":      `UPDATE upstream_account_bindings SET remote_group_id='99' WHERE account_id=$1`,
				"credentials": `UPDATE accounts SET credentials=jsonb_set(credentials,'{api_key}','"changed-key"') WHERE id=$1`,
				"base_url":    `UPDATE accounts SET credentials=jsonb_set(credentials,'{base_url}','"https://changed.invalid"') WHERE id=$1`,
				"delete":      `UPDATE accounts SET deleted_at=NOW() WHERE id=$1`,
				"unbind":      `DELETE FROM upstream_account_bindings WHERE account_id=$1`,
				"fallback":    `UPDATE upstream_account_bindings SET resolution_kind='fallback_chain',fallback_groups='["other"]' WHERE account_id=$1`,
				"headers":     `UPDATE accounts SET credentials=jsonb_set(credentials,'{header_overrides}','{"X-Upstream":"changed"}') WHERE id=$1`,
				"platform":    `UPDATE accounts SET platform='gemini' WHERE id=$1`,
				"custom_base": `UPDATE accounts SET extra=jsonb_set(extra,'{custom_base_url_enabled}','true') WHERE id=$1`,
			}[change]
			_, err := integrationEntClient.ExecContext(context.Background(), query, id)
			require.NoError(t, err)
			got := f.read(t)
			require.Empty(t, got.Models)
			require.Empty(t, got.AutoTags)
			require.NotEqual(t, "ready", got.Status)
			page, err := f.repo.ListGroupCatalog(context.Background(), service.UpstreamGroupCatalogParams{Page: 1, PageSize: 20, ConnectionIDs: []int64{f.ref.ConnectionID}, Search: "obsolete-model"}, f.now)
			require.NoError(t, err)
			require.Zero(t, page.Total)
			f.claim(t, "stale-worker")
			got = f.saveAccounts(t, "stale-worker", modelAccountResult(source, true, "must-not-publish", "OldTag"))
			require.Empty(t, got.Models)
		})
	}
}

func TestUpstreamGroupModelsMergeSuccessfulSourcesAndRetainOnlyFailedSource(t *testing.T) {
	f := newGroupModelFixture(t)
	f.account(t, 1)
	f.account(t, 2)
	f.claim(t, "first")
	sources := f.sources(t, true)
	require.Len(t, sources, 2)
	f.saveAccounts(t, "first", modelAccountResult(sources[0], true, "old-a", "GPT"), modelAccountResult(sources[1], true, "old-b", "Claude"))
	f.claim(t, "mixed")
	got := f.saveAccounts(t, "mixed", modelAccountResult(sources[0], true, "new-a", "Gemini"), modelAccountResult(sources[1], false, "", ""))
	require.Equal(t, "partial", got.Status)
	require.Equal(t, []string{"new-a", "old-b"}, got.Models)
	require.ElementsMatch(t, []string{"Gemini", "Claude"}, got.AutoTags)
	require.Equal(t, 1, got.FailedSourceCount)
	f.claim(t, "partial-again")
	got = f.saveAccounts(t, "partial-again", modelAccountResult(sources[0], true, "newer-a", "Image"), modelAccountResult(sources[1], false, "", ""))
	require.Equal(t, []string{"newer-a", "old-b"}, got.Models)
	// A successful empty source replaces its old models; another source survives.
	f.claim(t, "empty")
	got = f.saveAccounts(t, "empty", service.UpstreamGroupModelAccountResult{AccountID: sources[0].Account.ID, Fingerprint: sources[0].Fingerprint, Success: true}, modelAccountResult(sources[1], true, "new-b", "Claude"))
	require.Equal(t, "ready", got.Status)
	require.Equal(t, []string{"new-b"}, got.Models)
	require.Equal(t, []string{"Claude"}, got.AutoTags)
}

func TestUpstreamGroupModelsRotatePastEightAndBackoffFailures(t *testing.T) {
	f := newGroupModelFixture(t)
	for i := 0; i < 19; i++ {
		f.account(t, i)
	}
	seen := map[int64]bool{}
	for round := 0; round < 3; round++ {
		token := fmt.Sprint("round-", round)
		f.claim(t, token)
		sources := f.sources(t, false)
		results := []service.UpstreamGroupModelAccountResult{}
		for _, s := range sources {
			require.False(t, seen[s.Account.ID])
			seen[s.Account.ID] = true
			results = append(results, modelAccountResult(s, true, fmt.Sprint("model-", s.Account.ID), "GPT"))
		}
		require.LessOrEqual(t, len(results), 8)
		got := f.saveAccounts(t, token, results...)
		if round < 2 {
			require.Equal(t, "pending", got.Status)
			require.Positive(t, got.PendingSourceCount)
		} else {
			require.Equal(t, "ready", got.Status)
			require.Len(t, got.Models, 19)
		}
	}
	require.Len(t, seen, 19)
	require.Empty(t, f.sources(t, false))
	// Manual refresh starts with oldest attempted sources, rather than account 1 forever.
	f.claim(t, "manual")
	sources := f.sources(t, true)
	require.Len(t, sources, 8)
	f.saveAccounts(t, "manual", modelAccountResult(sources[0], false, "", ""))
	due, err := f.repo.ListGroupModelAccountSources(context.Background(), f.ref, 1, f.now.Add(time.Minute), 8, false)
	require.NoError(t, err)
	require.Len(t, due, 8, "remaining manual-batch sources must continue despite their fresh cached results")
	for _, source := range due {
		require.NotEqual(t, sources[0].Account.ID, source.Account.ID, "failed source is attempted once per batch before normal backoff")
	}
}

func completeInitialModelBatch(t *testing.T, f *groupModelFixture, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		f.account(t, i)
	}
	for round := 0; round < (count+7)/8; round++ {
		token := fmt.Sprintf("initial-%d", round)
		f.claim(t, token)
		results := []service.UpstreamGroupModelAccountResult{}
		for _, source := range f.sources(t, false) {
			results = append(results, modelAccountResult(source, true, fmt.Sprintf("old-%d", source.Account.ID), "GPT"))
		}
		f.saveAccounts(t, token, results...)
	}
	require.Equal(t, "ready", f.read(t).Status)
	require.False(t, f.read(t).RefreshInProgress)
}

func TestUpstreamGroupModelsManualRefreshTracksAllFreshSources(t *testing.T) {
	for _, automaticEnabled := range []bool{true, false} {
		t.Run(fmt.Sprint("automatic-", automaticEnabled), func(t *testing.T) {
			f := newGroupModelFixture(t)
			ctx := context.Background()
			completeInitialModelBatch(t, f, 19)
			if !automaticEnabled {
				_, err := integrationEntClient.UpstreamConnection.UpdateOneID(f.ref.ConnectionID).SetSyncEnabled(false).SetStatus("disabled").Save(ctx)
				require.NoError(t, err)
			}
			seen := map[int64]bool{}
			for round := 0; round < 3; round++ {
				token := fmt.Sprintf("manual-%d", round)
				f.now = f.now.Add(time.Minute)
				if round > 0 {
					due, err := f.repo.ListDueGroupModels(ctx, f.now, 100)
					require.NoError(t, err)
					require.Contains(t, due, f.ref)
				}
				ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, token, f.now, round == 0)
				require.NoError(t, err)
				require.True(t, ok)
				require.True(t, manual)
				sources := f.sources(t, round == 0)
				results := []service.UpstreamGroupModelAccountResult{}
				for _, source := range sources {
					require.False(t, seen[source.Account.ID])
					seen[source.Account.ID] = true
					results = append(results, modelAccountResult(source, true, fmt.Sprintf("new-%d", source.Account.ID), "GPT"))
				}
				got := f.saveAccounts(t, token, results...)
				require.Equal(t, len(seen), got.ReadySourceCount)
				require.Equal(t, 19-len(seen), got.PendingSourceCount)
				if round < 2 {
					require.Equal(t, "pending", got.Status)
					require.True(t, got.RefreshInProgress)
				} else {
					require.Equal(t, "ready", got.Status)
					require.False(t, got.RefreshInProgress)
				}
			}
			require.Len(t, seen, 19)
			for _, model := range f.read(t).Models {
				require.Contains(t, model, "new-")
			}
			require.Empty(t, f.sources(t, false))
			due, err := f.repo.ListDueGroupModels(ctx, f.now.Add(time.Minute), 100)
			require.NoError(t, err)
			require.NotContains(t, due, f.ref)
		})
	}
}

func TestUpstreamGroupModelManualBatchDoesNotRestartOnRepeatedClick(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	completeInitialModelBatch(t, f, 12)
	f.claim(t, "first")
	ok, _, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "duplicate", f.now, true)
	require.NoError(t, err)
	require.False(t, ok)
	var generation int64
	rows, err := integrationEntClient.QueryContext(ctx, "SELECT refresh_generation FROM upstream_group_model_snapshots WHERE connection_id=$1", f.ref.ConnectionID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&generation))
	require.NoError(t, rows.Close())
	results := []service.UpstreamGroupModelAccountResult{}
	for _, source := range f.sources(t, true) {
		results = append(results, modelAccountResult(source, true, fmt.Sprint("new-", source.Account.ID), "GPT"))
	}
	f.saveAccounts(t, "first", results...)
	f.claim(t, "clicked-again")
	remaining := f.sources(t, true)
	require.Len(t, remaining, 4)
	rows, err = integrationEntClient.QueryContext(ctx, "SELECT refresh_generation FROM upstream_group_model_snapshots WHERE connection_id=$1", f.ref.ConnectionID)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var continued int64
	require.NoError(t, rows.Scan(&continued))
	require.NoError(t, rows.Close())
	require.Equal(t, generation, continued)
	results = nil
	for _, source := range remaining {
		results = append(results, modelAccountResult(source, true, "new-last", "GPT"))
	}
	got := f.saveAccounts(t, "clicked-again", results...)
	require.Equal(t, "ready", got.Status)
	require.False(t, got.RefreshInProgress)
}

func TestUpstreamGroupModelBatchFailuresDoNotBlockRemainingSources(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	completeInitialModelBatch(t, f, 10)
	f.claim(t, "first")
	sources := f.sources(t, true)
	results := []service.UpstreamGroupModelAccountResult{}
	failedID := sources[0].Account.ID
	for i, source := range sources {
		results = append(results, modelAccountResult(source, i != 0, fmt.Sprint("new-", source.Account.ID), "GPT"))
	}
	got := f.saveAccounts(t, "first", results...)
	require.Equal(t, "pending", got.Status)
	require.Equal(t, 1, got.FailedSourceCount)
	require.Equal(t, 2, got.PendingSourceCount)
	require.True(t, got.RefreshInProgress)
	f.now = f.now.Add(time.Minute)
	ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "continue", f.now, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, manual)
	remaining := f.sources(t, false)
	require.Len(t, remaining, 2)
	results = nil
	for _, source := range remaining {
		require.NotEqual(t, failedID, source.Account.ID)
		results = append(results, modelAccountResult(source, true, "last", "GPT"))
	}
	got = f.saveAccounts(t, "continue", results...)
	require.Equal(t, "partial", got.Status)
	require.Equal(t, 9, got.ReadySourceCount)
	require.Zero(t, got.PendingSourceCount)
	require.False(t, got.RefreshInProgress)
	require.Contains(t, got.Models, fmt.Sprint("old-", failedID), "failed source retains its old data but is not counted as successful")
	require.Empty(t, f.sources(t, false), "failed source respects backoff after batch completion")
}

func TestUpstreamGroupModelBatchChangedSourcesAndWorkerRestart(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	completeInitialModelBatch(t, f, 10)
	f.claim(t, "first")
	results := []service.UpstreamGroupModelAccountResult{}
	for _, source := range f.sources(t, true) {
		results = append(results, modelAccountResult(source, true, "first-new", "GPT"))
	}
	f.saveAccounts(t, "first", results...)
	remaining := f.sources(t, false)
	require.Len(t, remaining, 2)
	_, err := integrationEntClient.ExecContext(ctx, "UPDATE upstream_account_bindings SET remote_group_id='other' WHERE account_id=$1", remaining[0].Account.ID)
	require.NoError(t, err)
	// A new service process has no in-memory queue; the persisted batch still runs.
	f.repo = &upstreamConnectionRepository{client: integrationEntClient}
	f.now = f.now.Add(time.Minute)
	ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "new-worker", f.now, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, manual)
	remaining = f.sources(t, false)
	require.Len(t, remaining, 1)
	got := f.saveAccounts(t, "new-worker", modelAccountResult(remaining[0], true, "last-new", "GPT"))
	require.Equal(t, 9, got.SourceCount)
	require.Equal(t, "ready", got.Status)
	require.False(t, got.RefreshInProgress)
}

func TestUpstreamGroupModelPublishedBatchRecoversAfterWorkerCrash(t *testing.T) {
	for _, syncEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("automatic-%t", syncEnabled), func(t *testing.T) {
			f := newGroupModelFixture(t)
			ctx := context.Background()
			f.account(t, 1)
			f.claim(t, "published")
			ok, err := f.repo.SaveGroupModels(ctx, f.ref, 1, "published", service.UpstreamGroupModels{
				Coverage: "published", Source: "newapi:published_models", Status: "ready", Models: []string{"cached-model"},
			}, f.now)
			require.NoError(t, err)
			require.True(t, ok)
			if !syncEnabled {
				_, err = integrationEntClient.UpstreamConnection.UpdateOneID(f.ref.ConnectionID).SetSyncEnabled(false).SetStatus("disabled").Save(ctx)
				require.NoError(t, err)
			}
			f.claim(t, "crashed-worker")
			// Simulate process loss after claiming but before the first batch save.
			_, err = integrationEntClient.ExecContext(ctx, `UPDATE upstream_group_model_snapshots SET lease_until=NOW()-INTERVAL '1 second' WHERE connection_id=$1`, f.ref.ConnectionID)
			require.NoError(t, err)
			next := f.now.Add(2 * time.Minute)
			due, err := f.repo.ListDueGroupModels(ctx, next, 100)
			require.NoError(t, err)
			require.Contains(t, due, f.ref)
			ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "replacement-worker", next, false)
			require.NoError(t, err)
			require.True(t, ok)
			require.True(t, manual)
			sources, err := f.repo.ListGroupModelAccountSources(ctx, f.ref, 1, next, 8, false)
			require.NoError(t, err)
			require.Len(t, sources, 1)
			f.now = next
			got := f.saveAccounts(t, "replacement-worker", modelAccountResult(sources[0], true, "recovered-model", "GPT"))
			require.Equal(t, []string{"recovered-model"}, got.Models)
			require.Equal(t, "ready", got.Status)
			require.False(t, got.RefreshInProgress)
			ok, err = f.repo.SaveGroupModels(ctx, f.ref, 1, "crashed-worker", service.UpstreamGroupModels{Coverage: "published", Status: "ready", Models: []string{"obsolete"}}, next)
			require.NoError(t, err)
			require.False(t, ok)
		})
	}
}

func TestUpstreamGroupModelPublishedBatchWithoutKeysShowsPendingAfterCrash(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	f.claim(t, "published")
	ok, err := f.repo.SaveGroupModels(ctx, f.ref, 1, "published", service.UpstreamGroupModels{
		Coverage: "published", Source: "newapi:published_models", Status: "ready", Models: []string{"old-model"},
	}, f.now)
	require.NoError(t, err)
	require.True(t, ok)
	f.claim(t, "crashed-worker")
	_, err = integrationEntClient.ExecContext(ctx, `UPDATE upstream_group_model_snapshots SET lease_until=NOW()-INTERVAL '1 second' WHERE connection_id=$1`, f.ref.ConnectionID)
	require.NoError(t, err)
	got := f.read(t)
	require.Equal(t, "pending", got.Status)
	require.True(t, got.RefreshInProgress)
	require.Equal(t, []string{"old-model"}, got.Models)
	next := f.now.Add(2 * time.Minute)
	due, err := f.repo.ListDueGroupModels(ctx, next, 100)
	require.NoError(t, err)
	require.Contains(t, due, f.ref)
	ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "replacement-worker", next, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, manual)
	ok, err = f.repo.SaveGroupModels(ctx, f.ref, 1, "replacement-worker", service.UpstreamGroupModels{
		Coverage: "published", Source: "newapi:published_models", Status: "ready", Models: []string{"new-model"},
	}, next)
	require.NoError(t, err)
	require.True(t, ok)
	got = f.read(t)
	require.Equal(t, "ready", got.Status)
	require.False(t, got.RefreshInProgress)
	require.Equal(t, []string{"new-model"}, got.Models)
}

func TestUpstreamGroupModelBatchContinuesWhenPublishedFallbackFirstChunkFails(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		f.account(t, i)
	}
	f.claim(t, "published")
	ok, err := f.repo.SaveGroupModels(ctx, f.ref, 1, "published", service.UpstreamGroupModels{Coverage: "published", Source: "published", Status: "ready", Models: []string{"published-model"}, AutoTags: []string{"GPT"}}, f.now)
	require.NoError(t, err)
	require.True(t, ok)
	f.claim(t, "fallback")
	results := []service.UpstreamGroupModelAccountResult{}
	for _, source := range f.sources(t, true) {
		results = append(results, modelAccountResult(source, false, "", ""))
	}
	got := f.saveAccounts(t, "fallback", results...)
	require.Equal(t, []string{"published-model"}, got.Models)
	require.Equal(t, "pending", got.Status)
	require.True(t, got.RefreshInProgress)
	f.now = f.now.Add(time.Minute)
	due, err := f.repo.ListDueGroupModels(ctx, f.now, 100)
	require.NoError(t, err)
	require.Contains(t, due, f.ref)
	ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "continue", f.now, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, manual)
	remaining := f.sources(t, false)
	require.Len(t, remaining, 2)
	results = nil
	for _, source := range remaining {
		results = append(results, modelAccountResult(source, true, "live-model", "Claude"))
	}
	got = f.saveAccounts(t, "continue", results...)
	require.Equal(t, "partial", got.Status)
	require.False(t, got.RefreshInProgress)
	require.Equal(t, 2, got.ReadySourceCount)
	require.Equal(t, 8, got.FailedSourceCount)
}

func TestUpstreamGroupModelsLostLeaseCannotOverwriteAccountResults(t *testing.T) {
	f := newGroupModelFixture(t)
	f.account(t, 1)
	f.claim(t, "old")
	source := f.sources(t, true)[0]
	f.now = f.now.Add(2 * time.Minute)
	ok, _, err := f.repo.ClaimGroupModels(context.Background(), f.ref, 1, "new", f.now, true)
	require.NoError(t, err)
	require.True(t, ok)
	f.saveAccounts(t, "new", modelAccountResult(source, true, "current-model", "GPT"))
	ok, err = f.repo.SaveGroupModels(context.Background(), f.ref, 1, "old", service.UpstreamGroupModels{Coverage: "bound_keys", AccountResults: []service.UpstreamGroupModelAccountResult{modelAccountResult(source, true, "stale-model", "GPT")}}, f.now)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, []string{"current-model"}, f.read(t).Models)
}

func TestUpstreamGroupModelsProxyChangesInvalidateWithoutTimestampDependency(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	id := f.account(t, 1)
	p, err := integrationEntClient.Proxy.Create().SetName("model-fixture-proxy").SetProtocol("http").SetHost("proxy.invalid").SetPort(8080).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationEntClient.ExecContext(ctx, "UPDATE accounts SET proxy_id=NULL WHERE id=$1", id)
		_ = integrationEntClient.Proxy.DeleteOneID(p.ID).Exec(ctx)
	})
	_, err = integrationEntClient.Account.UpdateOneID(id).SetProxyID(p.ID).Save(ctx)
	require.NoError(t, err)
	f.claim(t, "proxy")
	source := f.sources(t, true)[0]
	require.NotNil(t, source.Account.Proxy)
	require.Equal(t, "http://proxy.invalid:8080", source.Account.Proxy.URL())
	f.saveAccounts(t, "proxy", modelAccountResult(source, true, "gpt-5", "GPT"))
	_, err = integrationEntClient.Proxy.UpdateOneID(p.ID).SetHost("other-proxy.invalid").Save(ctx)
	require.NoError(t, err)
	require.Empty(t, f.read(t).Models)
	// Changes become eligible for automatic refresh before the group's six-hour TTL.
	due, err := f.repo.ListDueGroupModels(ctx, f.now.Add(time.Minute), 100)
	require.NoError(t, err)
	require.Contains(t, due, f.ref)
	ok, manual, err := f.repo.ClaimGroupModels(ctx, f.ref, 1, "auto-changed", f.now.Add(time.Minute), false)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, manual)
}

func TestUpstreamGroupCatalogBoundSourceScale(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	_, err := integrationEntClient.ExecContext(ctx, `INSERT INTO upstream_groups(connection_id,remote_id,name) SELECT $1,(1000+n)::text,'Group '||n FROM generate_series(1,200) n`, f.ref.ConnectionID)
	require.NoError(t, err)
	name := fmt.Sprint("model-scale-", f.ref.ConnectionID)
	t.Cleanup(func() { _, _ = integrationEntClient.ExecContext(ctx, `DELETE FROM accounts WHERE name=$1`, name) })
	_, err = integrationEntClient.ExecContext(ctx, `WITH inserted AS (
 INSERT INTO accounts(name,platform,type,credentials)
 SELECT $2,'openai','apikey',jsonb_build_object('api_key','fixture-'||g.remote_id||'-'||n,'fixture_group',g.remote_id)
 FROM upstream_groups g CROSS JOIN generate_series(1,3) n WHERE g.connection_id=$1 AND g.remote_id<>'12' RETURNING id,credentials
 ) INSERT INTO upstream_account_bindings(connection_id,account_id,remote_group_id,status,confidence,resolution_kind,fresh_until,key_fingerprint)
 SELECT $1,id,credentials->>'fixture_group','ready','exact','fixed',NOW()+INTERVAL '24 hours',
 'sha256:v1:'||encode(sha256(convert_to('upstream-api-key','UTF8')||decode('00','hex')||convert_to(credentials->>'api_key','UTF8')),'hex') FROM inserted`, f.ref.ConnectionID, name)
	require.NoError(t, err)
	_, err = integrationEntClient.ExecContext(ctx, `INSERT INTO upstream_group_model_account_snapshots(connection_id,remote_key,account_id,connection_version,source_fingerprint,models,auto_tags,status,observed_at,fresh_until,last_attempt_at,next_sync_at)
 SELECT connection_id,remote_key,account_id,connection_version,source_fingerprint,ARRAY(SELECT 'model-'||n FROM generate_series(1,300) n),ARRAY['GPT','Text'],'ready',NOW(),NOW()+INTERVAL '6 hours',NOW(),NOW()+INTERVAL '6 hours'
 FROM upstream_group_model_current_sources WHERE connection_id=$1`, f.ref.ConnectionID)
	require.NoError(t, err)
	_, err = integrationEntClient.ExecContext(ctx, `INSERT INTO upstream_group_model_snapshots(connection_id,remote_key,connection_version,coverage,source,status)
 SELECT $1,'id:'||remote_id,1,'bound_keys','bound_keys','ready' FROM upstream_groups WHERE connection_id=$1 AND remote_id<>'12'`, f.ref.ConnectionID)
	require.NoError(t, err)
	start := time.Now()
	result, err := f.repo.ListGroupCatalog(ctx, service.UpstreamGroupCatalogParams{Page: 1, PageSize: 20, ConnectionIDs: []int64{f.ref.ConnectionID}, Search: "model-299", Tag: "GPT"}, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(200), result.Total)
	require.Len(t, result.Items, 20)
	for _, item := range result.Items {
		require.Equal(t, 300, item.ModelCount)
		require.Len(t, item.ModelPreview, 3)
		require.Equal(t, "ready", item.ModelStatus)
	}
	t.Logf("200 groups / 600 bound accounts / 180,000 source model IDs: catalog query %s", time.Since(start))
	rows, err := integrationEntClient.QueryContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+upstreamGroupCatalogSQL+"SELECT count(*) FROM filtered", time.Now(), "model-299", pq.Array([]int64{f.ref.ConnectionID}), "", "GPT", "", "", false, nil, nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var raw []byte
	require.NoError(t, rows.Scan(&raw))
	require.NoError(t, rows.Close())
	var plan []map[string]any
	require.NoError(t, json.Unmarshal(raw, &plan))
	t.Logf("Model catalog explain execution=%v planning=%v jit=%v", plan[0]["Execution Time"], plan[0]["Planning Time"], plan[0]["JIT"])
}

func TestUpstreamGroupCatalogModelPayloadIsBounded(t *testing.T) {
	f := newGroupModelFixture(t)
	ctx := context.Background()
	_, err := integrationEntClient.ExecContext(ctx, `INSERT INTO upstream_groups(connection_id,remote_id,name) SELECT $1,(1000+n)::text,'Group '||n FROM generate_series(1,200) n`, f.ref.ConnectionID)
	require.NoError(t, err)
	_, err = integrationEntClient.ExecContext(ctx, `INSERT INTO upstream_group_model_snapshots(connection_id,remote_key,connection_version,models,auto_tags,status,coverage,observed_at)
 SELECT $1,'id:'||(1000+n),1,ARRAY(SELECT 'model-'||m FROM generate_series(1,300) m),ARRAY['GPT','Text'],'ready','published',NOW() FROM generate_series(1,200) n`, f.ref.ConnectionID)
	require.NoError(t, err)
	start := time.Now()
	result, err := f.repo.ListGroupCatalog(ctx, service.UpstreamGroupCatalogParams{ConnectionIDs: []int64{f.ref.ConnectionID}, Page: 1, PageSize: 20, Search: "model-299", Tag: "GPT"}, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(200), result.Total)
	require.Len(t, result.Items, 20)
	for _, item := range result.Items {
		require.Equal(t, 300, item.ModelCount)
		require.Len(t, item.ModelPreview, 3)
	}
	t.Logf("200 groups / 60,000 model IDs: filter and page %s; 60 preview IDs returned", time.Since(start))
}
