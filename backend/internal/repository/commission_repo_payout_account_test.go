package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCommissionRepositoryUpsertPayoutAccountAcceptsUploadedQRDataURL(t *testing.T) {
	userRepo, client := newUserRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateDefaultChatRepoUser(t, ctx, userRepo, "payout-qr@test.com", nil)
	repo := NewCommissionRepository(client)
	qrDataURL := "data:image/png;base64," + strings.Repeat("A", 1024)
	maskedAccountNo := "alice@example.com"
	encryptedAccountNo := "encrypted"

	account := &service.CommissionPayoutAccount{
		UserID:             user.ID,
		Method:             service.CommissionPayoutMethodAlipay,
		AccountName:        "Alice",
		AccountNoMasked:    &maskedAccountNo,
		AccountNoEncrypted: &encryptedAccountNo,
		QRImageURL:         &qrDataURL,
		IsDefault:          true,
		Status:             service.StatusActive,
	}

	require.NoError(t, repo.UpsertPayoutAccount(ctx, account))
	require.NotZero(t, account.ID)

	accounts, err := repo.ListPayoutAccountsByUser(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.NotNil(t, accounts[0].QRImageURL)
	require.Equal(t, qrDataURL, *accounts[0].QRImageURL)
}
