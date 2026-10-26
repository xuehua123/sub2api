package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type notificationReceiptContextKey struct{}

type notificationReceiptSettingRepo struct {
	*notificationEmailMemorySettingRepo
	persist func(context.Context, string, string) error
}

func (r *notificationReceiptSettingRepo) Set(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.persist != nil && strings.HasPrefix(key, notificationEmailDeliveryKeyPrefix) {
		return r.persist(ctx, key, value)
	}
	return r.notificationEmailMemorySettingRepo.Set(ctx, key, value)
}

func notificationReceiptReminderInput() NotificationEmailSendInput {
	return NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionExpiryReminder,
		RecipientEmail: "receipt-fixture@example.com", UserID: 42,
		SourceType: "user_subscription", SourceID: "-123", ReminderKey: "7d",
		Variables: map[string]string{"subscription_group": "Fixture", "expiry_time": "2030-01-01 12:00", "days_remaining": "7"},
	}
}

func TestNotificationEmailReceiptSurvivesCallerCancellationAfterSMTP(t *testing.T) {
	for _, expiredDeadline := range []bool{false, true} {
		name := "canceled"
		if expiredDeadline {
			name = "deadline_exceeded"
		}
		t.Run(name, func(t *testing.T) {
			smtpServer := startNotificationEmailTestSMTPServer(t)
			repo := &notificationReceiptSettingRepo{notificationEmailMemorySettingRepo: newNotificationEmailMemorySettingRepo()}
			require.NoError(t, repo.SetMultiple(context.Background(), smtpServer.settings()))
			input := notificationReceiptReminderInput()
			svc := NewNotificationEmailService(repo, NewEmailService(repo, nil))
			parent := context.WithValue(context.Background(), notificationReceiptContextKey{}, "receipt-context")
			// Trigger deadline expiry only after DATA arrives, not during setup.
			var ctx context.Context
			var cancel context.CancelFunc
			if expiredDeadline {
				ctx, cancel = context.WithTimeout(parent, 250*time.Millisecond)
				smtpServer.afterAccepted = func() { <-ctx.Done() }
			} else {
				ctx, cancel = context.WithCancel(parent)
				smtpServer.afterAccepted = cancel
			}
			defer cancel()
			repo.persist = func(persistCtx context.Context, key, value string) error {
				require.Error(t, ctx.Err())
				require.NoError(t, persistCtx.Err())
				require.Equal(t, "receipt-context", persistCtx.Value(notificationReceiptContextKey{}))
				deadline, ok := persistCtx.Deadline()
				require.True(t, ok)
				require.Positive(t, time.Until(deadline))
				require.LessOrEqual(t, time.Until(deadline), notificationEmailReceiptPersistTimeout)
				return repo.notificationEmailMemorySettingRepo.Set(persistCtx, key, value)
			}
			require.NoError(t, svc.Send(ctx, input))
			require.Equal(t, int64(1), smtpServer.messageCount())
			key := notificationEmailDeliveryKey(input.Event, input.SourceType, input.SourceID, input.RecipientEmail, input.ReminderKey)
			_, err := repo.GetValue(context.Background(), key)
			require.NoError(t, err)
			for i := 0; i < 1000; i++ {
				require.NoError(t, svc.Send(context.Background(), input))
			}
			require.Equal(t, int64(1), smtpServer.messageCount(), "a persisted receipt suppresses all subsequent scan attempts")
		})
	}
}

func TestNotificationEmailReceiptPersistenceFailureDoesNotFallback(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)
	cause := errors.New("receipt database unavailable")
	repo := &notificationReceiptSettingRepo{notificationEmailMemorySettingRepo: newNotificationEmailMemorySettingRepo(), persist: func(context.Context, string, string) error { return cause }}
	require.NoError(t, repo.SetMultiple(context.Background(), smtpServer.settings()))
	svc := NewNotificationEmailService(repo, NewEmailService(repo, nil))
	err := svc.Send(context.Background(), notificationReceiptReminderInput())
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), "after SMTP acceptance")
	require.False(t, shouldFallbackNotificationEmail(err), "SMTP already accepted the email; a legacy body would be a duplicate")
	require.Equal(t, int64(1), smtpServer.messageCount())
}

func TestNotificationEmailReceiptCanceledBeforeSendHasNoSMTPAcceptance(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)
	repo := newNotificationEmailMemorySettingRepo()
	require.NoError(t, repo.SetMultiple(context.Background(), smtpServer.settings()))
	// Use a context-aware settings reader to represent the production repository.
	reader := &notificationReceiptReadRepo{notificationEmailMemorySettingRepo: repo}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := NewNotificationEmailService(reader, NewEmailService(reader, nil))
	require.Error(t, svc.Send(ctx, notificationReceiptReminderInput()))
	require.Equal(t, int64(0), smtpServer.messageCount())
	values, err := repo.GetAll(context.Background())
	require.NoError(t, err)
	for key := range values {
		require.False(t, strings.HasPrefix(key, notificationEmailDeliveryKeyPrefix))
	}
}

type notificationReceiptReadRepo struct {
	*notificationEmailMemorySettingRepo
}

func (r *notificationReceiptReadRepo) GetValue(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return r.notificationEmailMemorySettingRepo.GetValue(ctx, key)
}

// Synchronize hook changes with server callbacks so cancellation tests stay
// race-free when exercising many independent logical reminder days.
func TestNotificationEmailReceiptIndependentReminderDays(t *testing.T) {
	smtpServer := startNotificationEmailTestSMTPServer(t)
	repo := &notificationReceiptSettingRepo{notificationEmailMemorySettingRepo: newNotificationEmailMemorySettingRepo()}
	require.NoError(t, repo.SetMultiple(context.Background(), smtpServer.settings()))
	svc := NewNotificationEmailService(repo, NewEmailService(repo, nil))
	var mu sync.Mutex
	var currentCancel context.CancelFunc
	smtpServer.afterAccepted = func() {
		mu.Lock()
		cancel := currentCancel
		mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	for _, day := range []string{"7d", "3d", "1d"} {
		input := notificationReceiptReminderInput()
		input.ReminderKey = day
		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		currentCancel = cancel
		mu.Unlock()
		require.NoError(t, svc.Send(ctx, input))
		cancel()
		require.NoError(t, svc.Send(context.Background(), input))
	}
	require.Equal(t, int64(3), smtpServer.messageCount(), "each due date still receives its own single reminder")
}
