//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type rateMultiplierPriorityRepoStub struct {
	accounts []Account
	updates  map[int64]int
}

type rateMultiplierPrioritySettingsStub struct {
	enabled  bool
	settings *RateMultiplierPrioritySettings
	err      error
}

func (s rateMultiplierPrioritySettingsStub) GetRateMultiplierPrioritySettings(context.Context) (*RateMultiplierPrioritySettings, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.settings != nil {
		copy := *s.settings
		return &copy, nil
	}
	return &RateMultiplierPrioritySettings{Enabled: s.enabled}, nil
}

func (r *rateMultiplierPriorityRepoStub) ListActiveSchedulableForRateMultiplierPriority(context.Context) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *rateMultiplierPriorityRepoStub) UpdateRateMultiplierPriorities(_ context.Context, priorities map[int64]int) (int64, error) {
	r.updates = make(map[int64]int, len(priorities))
	for id, priority := range priorities {
		r.updates[id] = priority
	}
	return int64(len(priorities)), nil
}

func TestRateMultiplierPriorityReconcileUsesDistinctMultiplierBands(t *testing.T) {
	low := 0.5
	second := 0.8
	high := 1.0
	higher := 1.5
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &low},
		{ID: 2, Status: StatusActive, Schedulable: true, Priority: 9, RateMultiplier: &low},
		{ID: 3, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &second},
		{ID: 4, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &high},
		{ID: 5, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &higher},
	}}

	updated, err := NewRateMultiplierPriorityService(repo, nil, 0).Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 5 {
		t.Fatalf("updated = %d, want 5", updated)
	}
	want := map[int64]int{1: 1, 2: 1, 3: 2, 4: 3, 5: 4}
	if len(repo.updates) != len(want) {
		t.Fatalf("updates = %#v, want %#v", repo.updates, want)
	}
	for id, priority := range want {
		if repo.updates[id] != priority {
			t.Fatalf("account %d priority = %d, want %d", id, repo.updates[id], priority)
		}
	}
}

func TestRateMultiplierPriorityReconcileLeavesClosedAccountsUntouched(t *testing.T) {
	low := 0.5
	high := 1.0
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Priority: 1, RateMultiplier: &low},
		{ID: 2, Status: StatusDisabled, Schedulable: true, Priority: 99, RateMultiplier: &low},
		{ID: 3, Status: StatusActive, Schedulable: false, Priority: 99, RateMultiplier: &high},
	}}

	updated, err := NewRateMultiplierPriorityService(repo, nil, 0).Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("ineligible accounts were changed: %#v", repo.updates)
	}
}

func TestRateMultiplierPriorityReconcileRepairsBoundGroupPriorities(t *testing.T) {
	low := 0.5
	high := 1.0
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{
		{
			ID: 1, Status: StatusActive, Schedulable: true, Priority: 1, RateMultiplier: &low,
			AccountGroups: []AccountGroup{{GroupID: 10, Priority: 50}, {GroupID: 20, Priority: 50}},
		},
		{
			ID: 2, Status: StatusActive, Schedulable: true, Priority: 2, RateMultiplier: &high,
			AccountGroups: []AccountGroup{{GroupID: 10, Priority: 1}},
		},
	}}

	updated, err := NewRateMultiplierPriorityService(repo, nil, 0).Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	want := map[int64]int{1: 1, 2: 2}
	if len(repo.updates) != len(want) {
		t.Fatalf("updates = %#v, want %#v", repo.updates, want)
	}
	for id, priority := range want {
		if repo.updates[id] != priority {
			t.Fatalf("account %d priority = %d, want %d", id, repo.updates[id], priority)
		}
	}
}

func TestRateMultiplierPriorityReconcileUsesConfiguredStep(t *testing.T) {
	low := 0.05
	middle := 0.5
	high := 1.0
	premium := 1.8
	settings := &RateMultiplierPrioritySettings{
		Enabled:         true,
		IntervalMinutes: 5,
		PriorityStep:    20,
	}
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Priority: 99, RateMultiplier: &low, AccountGroups: []AccountGroup{{GroupID: 10, Priority: 99}}},
		{ID: 2, Status: StatusActive, Schedulable: true, Priority: 99, RateMultiplier: &middle},
		{ID: 3, Status: StatusActive, Schedulable: true, Priority: 99, RateMultiplier: &high},
		{ID: 4, Status: StatusActive, Schedulable: true, Priority: 99, RateMultiplier: &premium},
	}}

	updated, err := NewRateMultiplierPriorityService(repo, rateMultiplierPrioritySettingsStub{settings: settings}, 0).ReconcileIfEnabled(context.Background())
	if err != nil {
		t.Fatalf("ReconcileIfEnabled() error = %v", err)
	}
	if updated != 4 {
		t.Fatalf("updated = %d, want 4", updated)
	}
	want := map[int64]int{1: 1, 2: 21, 3: 41, 4: 61}
	for id, priority := range want {
		if repo.updates[id] != priority {
			t.Fatalf("account %d priority = %d, want %d", id, repo.updates[id], priority)
		}
	}
}

func TestRateMultiplierPriorityPeriodicReconcileHonorsConfiguredInterval(t *testing.T) {
	multiplier := 0.5
	now := time.Date(2026, time.July, 17, 2, 0, 0, 0, time.UTC)
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{{
		ID: 1, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &multiplier,
	}}}
	settings := &RateMultiplierPrioritySettings{
		Enabled:         true,
		IntervalMinutes: 5,
		PriorityStep:    1,
	}
	service := NewRateMultiplierPriorityService(repo, rateMultiplierPrioritySettingsStub{settings: settings}, time.Hour)
	service.lastPeriodicAttempt = now

	updated, err := service.reconcilePeriodically(context.Background(), now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("reconcilePeriodically() before interval error = %v", err)
	}
	if updated != 0 || len(repo.updates) != 0 {
		t.Fatalf("reconciled before configured interval: updated=%d updates=%#v", updated, repo.updates)
	}

	updated, err = service.reconcilePeriodically(context.Background(), now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("reconcilePeriodically() at interval error = %v", err)
	}
	if updated != 1 || repo.updates[1] != 1 {
		t.Fatalf("configured interval did not reconcile: updated=%d updates=%#v", updated, repo.updates)
	}
}

func TestRateMultiplierPriorityReconcileIfEnabledRespectsPersistedSwitch(t *testing.T) {
	multiplier := 0.5
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{{
		ID: 1, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &multiplier,
	}}}

	service := NewRateMultiplierPriorityService(repo, rateMultiplierPrioritySettingsStub{enabled: false}, 0)
	updated, err := service.ReconcileIfEnabled(context.Background())
	if err != nil {
		t.Fatalf("ReconcileIfEnabled() error = %v", err)
	}
	if updated != 0 || len(repo.updates) != 0 {
		t.Fatalf("disabled switch changed priorities: updated=%d updates=%#v", updated, repo.updates)
	}

	service = NewRateMultiplierPriorityService(repo, rateMultiplierPrioritySettingsStub{enabled: true}, 0)
	updated, err = service.ReconcileIfEnabled(context.Background())
	if err != nil {
		t.Fatalf("ReconcileIfEnabled() error = %v", err)
	}
	if updated != 1 || repo.updates[1] != 1 {
		t.Fatalf("enabled switch did not reconcile: updated=%d updates=%#v", updated, repo.updates)
	}
}

func TestRateMultiplierPriorityStartDoesNotReconcileImmediately(t *testing.T) {
	multiplier := 0.5
	repo := &rateMultiplierPriorityRepoStub{accounts: []Account{{
		ID: 1, Status: StatusActive, Schedulable: true, Priority: 50, RateMultiplier: &multiplier,
	}}}
	service := NewRateMultiplierPriorityService(repo, rateMultiplierPrioritySettingsStub{enabled: true}, time.Hour)
	service.Start()
	service.Stop()

	if len(repo.updates) != 0 {
		t.Fatalf("Start() reconciled before its first interval: %#v", repo.updates)
	}
}

func TestRateMultiplierPrioritySettingsDefaultDisabledAndRoundTrip(t *testing.T) {
	repo := newMockSettingRepo()
	settingsService := NewSettingService(repo, &config.Config{})

	settings, err := settingsService.GetRateMultiplierPrioritySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRateMultiplierPrioritySettings() error = %v", err)
	}
	if settings.Enabled {
		t.Fatal("default rate multiplier priority setting must be disabled")
	}
	if settings.IntervalMinutes != DefaultRateMultiplierPriorityIntervalMinutes || settings.PriorityStep != DefaultRateMultiplierPriorityStep {
		t.Fatalf("default settings = %#v, want interval=%d step=%d", settings, DefaultRateMultiplierPriorityIntervalMinutes, DefaultRateMultiplierPriorityStep)
	}

	if err := settingsService.SetRateMultiplierPrioritySettings(context.Background(), &RateMultiplierPrioritySettings{Enabled: true}); err != nil {
		t.Fatalf("SetRateMultiplierPrioritySettings() error = %v", err)
	}
	settings, err = settingsService.GetRateMultiplierPrioritySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRateMultiplierPrioritySettings() after set error = %v", err)
	}
	if !settings.Enabled {
		t.Fatal("persisted enabled setting was not returned")
	}
	if settings.IntervalMinutes != DefaultRateMultiplierPriorityIntervalMinutes || settings.PriorityStep != DefaultRateMultiplierPriorityStep {
		t.Fatalf("legacy settings did not receive defaults: %#v", settings)
	}
}

func TestRateMultiplierPrioritySettingsIntervalAndStepRoundTripAndValidate(t *testing.T) {
	repo := newMockSettingRepo()
	settingsService := NewSettingService(repo, &config.Config{})
	want := &RateMultiplierPrioritySettings{
		Enabled:         true,
		IntervalMinutes: 10,
		PriorityStep:    20,
	}
	if err := settingsService.SetRateMultiplierPrioritySettings(context.Background(), want); err != nil {
		t.Fatalf("SetRateMultiplierPrioritySettings() error = %v", err)
	}
	got, err := settingsService.GetRateMultiplierPrioritySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRateMultiplierPrioritySettings() error = %v", err)
	}
	if got.IntervalMinutes != 10 || got.PriorityStep != 20 {
		t.Fatalf("stored settings = %#v", got)
	}

	if err := settingsService.SetRateMultiplierPrioritySettings(context.Background(), &RateMultiplierPrioritySettings{IntervalMinutes: 61}); err == nil {
		t.Fatal("an interval greater than 60 must be rejected")
	}

	if err := settingsService.SetRateMultiplierPrioritySettings(context.Background(), &RateMultiplierPrioritySettings{PriorityStep: 0}); err != nil {
		t.Fatalf("zero priority step should use the default: %v", err)
	}
	if err := settingsService.SetRateMultiplierPrioritySettings(context.Background(), &RateMultiplierPrioritySettings{PriorityStep: 1001}); err == nil {
		t.Fatal("a priority step greater than 1000 must be rejected")
	}
}
