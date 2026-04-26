//go:build unit

package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newPaymentWebhookEntClient(t *testing.T) *dbent.Client {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		require.NoError(t, db.Close())
	})
	return client
}

func TestStripeWebhook_AmbiguousProviderReturnsRetryableFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := t.Context()
	client := newPaymentWebhookEntClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-a").
		SetConfig("{}").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-b").
		SetConfig("{}").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	svc := service.NewPaymentService(client, registry, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewPaymentWebhookHandler(svc, registry)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment/webhook/stripe",
		strings.NewReader(`{"type":"charge.refunded","data":{"object":{"id":"ch_test"}}}`),
	)

	h.StripeWebhook(c)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotEqual(t, "success", strings.TrimSpace(rec.Body.String()))
}
