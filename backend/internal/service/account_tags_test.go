//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type accountRepoStubForTags struct {
	accountRepoStub
	updateExtraID      int64
	updateExtraPayload map[string]any
	updateExtraErr     error
}

func (s *accountRepoStubForTags) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	s.updateExtraID = id
	s.updateExtraPayload = updates
	return s.updateExtraErr
}

func TestNormalizeAccountTags(t *testing.T) {
	t.Parallel()

	longTag := strings.Repeat("长", 40)
	input := []string{
		"  pro  ",
		"",
		"PLUS",
		"plus",
		"生图",
		longTag,
		"ccmax",
		"Pro",
		"  ",
	}
	for i := 0; i < 30; i++ {
		input = append(input, "tag"+string(rune('a'+i)))
	}

	got := NormalizeAccountTags(input)

	require.Len(t, got, accountTagsMaxCount)
	require.Equal(t, "pro", got[0])
	require.Equal(t, "PLUS", got[1])
	require.Equal(t, "生图", got[2])
	require.Equal(t, strings.Repeat("长", accountTagMaxLength), got[3])
	require.Equal(t, "ccmax", got[4])
	require.NotContains(t, got, "")
}

func TestAccountTagsFromExtra(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"pro", "生图"}, AccountTagsFromExtra(map[string]any{
		AccountExtraTagsKey: []any{" pro ", "生图", 123, "PRO"},
	}))
	require.Equal(t, []string{"plus", "ccmax"}, AccountTagsFromExtra(map[string]any{
		AccountExtraTagsKey: "plus，ccmax;plus",
	}))
	require.Empty(t, AccountTagsFromExtra(map[string]any{AccountExtraTagsKey: []any{123, true}}))
	require.Empty(t, AccountTagsFromExtra(nil))
}

func TestAdminServiceUpdateAccountTagsStoresNormalizedExtra(t *testing.T) {
	repo := &accountRepoStubForTags{}
	svc := &adminServiceImpl{accountRepo: repo}

	tags, err := svc.UpdateAccountTags(context.Background(), 42, []string{" pro ", "PRO", "生图"})

	require.NoError(t, err)
	require.Equal(t, int64(42), repo.updateExtraID)
	require.Equal(t, []string{"pro", "生图"}, tags)
	require.Equal(t, []string{"pro", "生图"}, repo.updateExtraPayload[AccountExtraTagsKey])
}

func TestOpsAccountHealthIncludesTags(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData: []Account{
			{
				ID:          42,
				Name:        "plus account",
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					AccountExtraTagsKey: []any{"plus", "生图"},
				},
			},
		},
	}
	svc := &OpsService{
		opsRepo:     &opsRepoMock{},
		accountRepo: repo,
	}

	resp, err := svc.GetAccountHealth(context.Background(), &OpsAccountHealthFilter{})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, []string{"plus", "生图"}, resp.Items[0].Tags)
}
