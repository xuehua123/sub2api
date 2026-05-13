//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSupportIssueRepository_CreateIssueCreatesPublicIDAttachmentAndEvent(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := newSupportIssueFixture(t, user.ID, func(issue *service.SupportIssue) {
		issue.HTTPStatus = ptrInt(429)
	})
	attachments := []service.SupportIssueAttachment{newSupportIssueAttachmentFixture(user.ID)}

	require.NoError(t, repo.CreateIssue(ctx, issue, attachments, newSupportIssueEvent(user.ID, service.SupportIssueEventCreated)))

	require.NotZero(t, issue.ID)
	require.Equal(t, fmt.Sprintf("ISS-%06d", issue.ID), issue.PublicID)
	require.Equal(t, 1, issue.AttachmentCount)
	require.Len(t, issue.Attachments, 1)
	require.NotZero(t, issue.Attachments[0].ID)
	require.Len(t, issue.Events, 1)
	require.Equal(t, service.SupportIssueEventCreated, issue.Events[0].EventType)
}

func TestSupportIssueRepository_CreateUnboundAttachmentThenBindToIssue(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	attachment := newSupportIssueAttachmentFixture(user.ID)

	require.NoError(t, repo.CreateUnboundAttachment(ctx, &attachment))
	require.NotZero(t, attachment.ID)
	require.Zero(t, attachment.IssueID)
	require.Equal(t, fmt.Sprintf("/api/v1/issues/attachments/%d/file", attachment.ID), attachment.FileURL)

	unbound, err := repo.ListUnboundAttachmentsForUser(ctx, user.ID, []int64{attachment.ID})
	require.NoError(t, err)
	require.Len(t, unbound, 1)

	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil, unbound[0])
	require.Len(t, issue.Attachments, 1)
	require.Equal(t, attachment.ID, issue.Attachments[0].ID)
	require.Equal(t, issue.ID, issue.Attachments[0].IssueID)
	require.Equal(t, 1, issue.AttachmentCount)

	unbound, err = repo.ListUnboundAttachmentsForUser(ctx, user.ID, []int64{attachment.ID})
	require.NoError(t, err)
	require.Empty(t, unbound)
}

func TestSupportIssueRepository_ListUnboundAttachmentsRequiresOwner(t *testing.T) {
	ctx, client, repo, user := newSupportIssueRepoTest(t)
	other := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("%s@example.com", uniqueTestValue(t, "support-issue-other-user")),
	})
	attachment := newSupportIssueAttachmentFixture(user.ID)
	require.NoError(t, repo.CreateUnboundAttachment(ctx, &attachment))

	items, err := repo.ListUnboundAttachmentsForUser(ctx, other.ID, []int64{attachment.ID})

	require.NoError(t, err)
	require.Empty(t, items)
}

func TestSupportIssueRepository_OpenAttachmentForPublicRequiresPublicBoundAttachment(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil, newSupportIssueAttachmentFixture(user.ID))

	attachment, err := repo.OpenAttachmentForPublic(ctx, issue.Attachments[0].ID)

	require.NoError(t, err)
	require.Equal(t, issue.ID, attachment.IssueID)
	require.Equal(t, service.SupportIssueAttachmentVisibilityPublic, attachment.Visibility)
	require.NotEmpty(t, attachment.FilePath)
}

func TestSupportIssueRepository_OpenAttachmentForPublicRejectsHiddenAttachment(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil, newSupportIssueAttachmentFixture(user.ID))
	require.NoError(t, repo.HideAttachment(ctx, service.HideSupportIssueAttachmentInput{
		IssueID:        issue.ID,
		AttachmentID:   issue.Attachments[0].ID,
		HiddenByUserID: user.ID,
	}, newSupportIssueEvent(user.ID, service.SupportIssueEventAttachmentHidden)))

	_, err := repo.OpenAttachmentForPublic(ctx, issue.Attachments[0].ID)

	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrSupportIssueAttachmentNotFound)
}

func TestSupportIssueRepository_OpenAttachmentForPublicRejectsUnboundAttachment(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	attachment := newSupportIssueAttachmentFixture(user.ID)
	require.NoError(t, repo.CreateUnboundAttachment(ctx, &attachment))

	_, err := repo.OpenAttachmentForPublic(ctx, attachment.ID)

	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrSupportIssueAttachmentNotFound)
}

func TestSupportIssueRepository_HideAndRestoreIssue(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)

	hidden, err := repo.HideIssue(ctx, service.HideSupportIssueInput{
		IssueID:        issue.ID,
		HiddenByUserID: user.ID,
		HideReason:     "contains private data",
	}, newSupportIssueEvent(user.ID, service.SupportIssueEventIssueHidden))
	require.NoError(t, err)
	require.NotNil(t, hidden.HiddenAt)
	require.Equal(t, "contains private data", hidden.HideReason)

	visible := false
	items, _, err := repo.ListIssues(ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, service.ListSupportIssueFilters{Hidden: &visible})
	require.NoError(t, err)
	requireSupportIssueIDsNotContain(t, items, issue.ID)

	restored, err := repo.RestoreIssue(ctx, issue.ID, user.ID, newSupportIssueEvent(user.ID, service.SupportIssueEventIssueRestored))
	require.NoError(t, err)
	require.Nil(t, restored.HiddenAt)
	require.Empty(t, restored.HideReason)
}

func TestSupportIssueRepository_RecordViewThrottlesByViewer(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)
	viewer := service.SupportIssueViewer{IP: "127.0.0.1", UserAgent: "unit-test"}

	require.NoError(t, repo.RecordView(ctx, issue.ID, viewer, time.Hour))
	require.NoError(t, repo.RecordView(ctx, issue.ID, viewer, time.Hour))

	loaded, err := repo.GetIssue(ctx, issue.ID, true)
	require.NoError(t, err)
	require.Equal(t, 1, loaded.ViewCount)
	require.NotNil(t, loaded.LastViewedAt)
}

func TestSupportIssueRepository_GetIssueExcludesHiddenContent(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil, newSupportIssueAttachmentFixture(user.ID))

	comment := &service.SupportIssueComment{
		IssueID:      issue.ID,
		AuthorUserID: &user.ID,
		AuthorRole:   service.RoleUser,
		Content:      "contains sensitive key suffix",
	}
	require.NoError(t, repo.AddComment(ctx, comment, newSupportIssueEvent(user.ID, service.SupportIssueEventCommented)))

	require.NoError(t, repo.HideComment(ctx, service.HideSupportIssueCommentInput{
		IssueID:        issue.ID,
		CommentID:      comment.ID,
		HiddenByUserID: user.ID,
		HideReason:     "sensitive info",
	}, newSupportIssueEvent(user.ID, service.SupportIssueEventCommentHidden)))
	require.NoError(t, repo.HideAttachment(ctx, service.HideSupportIssueAttachmentInput{
		IssueID:        issue.ID,
		AttachmentID:   issue.Attachments[0].ID,
		HiddenByUserID: user.ID,
	}, newSupportIssueEvent(user.ID, service.SupportIssueEventAttachmentHidden)))

	publicIssue, err := repo.GetIssue(ctx, issue.ID, false)
	require.NoError(t, err)
	require.Empty(t, publicIssue.Comments)
	require.Empty(t, publicIssue.Attachments)

	adminIssue, err := repo.GetIssue(ctx, issue.ID, true)
	require.NoError(t, err)
	require.Len(t, adminIssue.Comments, 1)
	require.Len(t, adminIssue.Attachments, 1)
}

func TestSupportIssueRepository_ListIssuesFilters(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	matching := mustCreateSupportIssue(t, ctx, repo, user.ID, func(issue *service.SupportIssue) {
		issue.Status = service.SupportIssueStatusOpen
		issue.Category = service.SupportIssueCategoryPayment
		issue.Severity = service.SupportIssueSeverityBlocked
	}, newSupportIssueAttachmentFixture(user.ID))
	mustCreateSupportIssue(t, ctx, repo, user.ID, func(issue *service.SupportIssue) {
		issue.Status = service.SupportIssueStatusResolved
		issue.Category = service.SupportIssueCategoryLogin
		issue.Severity = service.SupportIssueSeverityQuestion
	})

	hasImage := true
	items, result, err := repo.ListIssues(ctx, pagination.PaginationParams{
		Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: pagination.SortOrderDesc,
	}, service.ListSupportIssueFilters{
		Status: service.SupportIssueStatusOpen, Category: service.SupportIssueCategoryPayment,
		Severity: service.SupportIssueSeverityBlocked, HasImage: &hasImage,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.GreaterOrEqual(t, result.Total, int64(1))
	requireSupportIssueIDsContain(t, items, matching.ID)
}

func TestSupportIssueRepository_SearchIssuesByPublicID(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)

	items := searchSupportIssues(t, ctx, repo, service.ParsedSupportIssueSearch{PublicID: issue.PublicID})
	require.Len(t, items, 1)
	require.Equal(t, issue.ID, items[0].ID)
}

func TestSupportIssueRepository_SearchIssuesByEmail(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)

	items := searchSupportIssues(t, ctx, repo, service.ParsedSupportIssueSearch{Email: issue.AccountEmailNormalized})
	require.Len(t, items, 1)
	require.Equal(t, issue.ID, items[0].ID)
}

func TestSupportIssueRepository_SearchIssuesByHTTPStatus(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	code := 429
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, func(issue *service.SupportIssue) {
		issue.HTTPStatus = &code
	})

	items := searchSupportIssues(t, ctx, repo, service.ParsedSupportIssueSearch{HTTPStatus: &code})
	requireSupportIssueIDsContain(t, items, issue.ID)
}

func TestSupportIssueRepository_SearchIssuesByPhraseAndTerm(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, func(issue *service.SupportIssue) {
		issue.Title = "rate limit on balance payment"
		issue.Description = "The balance payment flow returns a rate limit message."
		issue.ScreenshotText = "rate limit exceeded for balance operation"
		issue.SearchText = strings.Join([]string{issue.Title, issue.Description, issue.ScreenshotText}, " ")
	})

	items := searchSupportIssues(t, ctx, repo, service.ParsedSupportIssueSearch{
		Phrases: []string{"rate limit"},
		Terms:   []string{"balance"},
	})
	requireSupportIssueIDsContain(t, items, issue.ID)
}

func TestSupportIssueRepository_AddCommentUpdatesCounters(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)
	comment := &service.SupportIssueComment{
		IssueID:      issue.ID,
		AuthorUserID: &user.ID,
		AuthorRole:   service.RoleUser,
		Content:      "I can reproduce this with Claude Code.",
	}

	require.NoError(t, repo.AddComment(ctx, comment, newSupportIssueEvent(user.ID, service.SupportIssueEventCommented)))
	require.NotZero(t, comment.ID)

	got, err := repo.GetIssue(ctx, issue.ID, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.CommentCount)
	require.NotNil(t, got.LastCommentAt)
	require.Len(t, got.Comments, 1)
}

func TestSupportIssueRepository_UpdateStatusResolvedSetsResolvedAndLockedAt(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)

	updated, err := repo.UpdateStatus(
		ctx,
		issue.ID,
		service.SupportIssueStatusResolved,
		user.ID,
		false,
		newSupportIssueEvent(user.ID, service.SupportIssueEventStatusChanged),
	)
	require.NoError(t, err)
	require.Equal(t, service.SupportIssueStatusResolved, updated.Status)
	require.NotNil(t, updated.ResolvedAt)
	require.NotNil(t, updated.LockedAt)
	require.NotNil(t, updated.ResolvedByUserID)
	require.Equal(t, user.ID, *updated.ResolvedByUserID)
}

func TestSupportIssueRepository_HideCommentUpdatesCounters(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)
	comment := &service.SupportIssueComment{
		IssueID:      issue.ID,
		AuthorUserID: &user.ID,
		AuthorRole:   service.RoleUser,
		Content:      "hide this sensitive comment",
	}
	require.NoError(t, repo.AddComment(ctx, comment, newSupportIssueEvent(user.ID, service.SupportIssueEventCommented)))

	require.NoError(t, repo.HideComment(ctx, service.HideSupportIssueCommentInput{
		IssueID:        issue.ID,
		CommentID:      comment.ID,
		HiddenByUserID: user.ID,
		HideReason:     "contains sensitive info",
	}, newSupportIssueEvent(user.ID, service.SupportIssueEventCommentHidden)))

	got, err := repo.GetIssue(ctx, issue.ID, false)
	require.NoError(t, err)
	require.Equal(t, 0, got.CommentCount)
	require.Equal(t, 1, got.HiddenCommentCount)
	require.Empty(t, got.Comments)
}

func TestSupportIssueRepository_HideAttachmentUpdatesVisibleCount(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil, newSupportIssueAttachmentFixture(user.ID))

	require.NoError(t, repo.HideAttachment(ctx, service.HideSupportIssueAttachmentInput{
		IssueID:        issue.ID,
		AttachmentID:   issue.Attachments[0].ID,
		HiddenByUserID: user.ID,
	}, newSupportIssueEvent(user.ID, service.SupportIssueEventAttachmentHidden)))

	got, err := repo.GetIssue(ctx, issue.ID, false)
	require.NoError(t, err)
	require.Equal(t, 0, got.AttachmentCount)
	require.Empty(t, got.Attachments)
}

func TestSupportIssueRepository_ListEventsReturnsLatestFirst(t *testing.T) {
	ctx, _, repo, user := newSupportIssueRepoTest(t)
	issue := mustCreateSupportIssue(t, ctx, repo, user.ID, nil)
	_, err := repo.UpdateStatus(
		ctx,
		issue.ID,
		service.SupportIssueStatusResolved,
		user.ID,
		false,
		newSupportIssueEvent(user.ID, service.SupportIssueEventStatusChanged),
	)
	require.NoError(t, err)

	events, err := repo.ListEvents(ctx, issue.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 2)
	require.Equal(t, service.SupportIssueEventStatusChanged, events[0].EventType)
}

func newSupportIssueRepoTest(t *testing.T) (context.Context, *dbent.Client, service.SupportIssueRepository, *service.User) {
	t.Helper()

	baseCtx := context.Background()
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(baseCtx, tx)
	client := tx.Client()
	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("%s@example.com", uniqueTestValue(t, "support-issue-user")),
	})
	return ctx, client, NewSupportIssueRepository(client), user
}

func mustCreateSupportIssue(
	t *testing.T,
	ctx context.Context,
	repo service.SupportIssueRepository,
	userID int64,
	mutate func(*service.SupportIssue),
	attachments ...service.SupportIssueAttachment,
) *service.SupportIssue {
	t.Helper()

	issue := newSupportIssueFixture(t, userID, mutate)
	require.NoError(t, repo.CreateIssue(ctx, issue, attachments, newSupportIssueEvent(userID, service.SupportIssueEventCreated)))
	return issue
}

func newSupportIssueFixture(
	t *testing.T,
	userID int64,
	mutate func(*service.SupportIssue),
) *service.SupportIssue {
	t.Helper()

	email := fmt.Sprintf("%s@example.com", uniqueTestValue(t, "support-issue"))
	normalizedEmail, err := domain.NormalizeSupportIssueEmail(email)
	require.NoError(t, err)
	maskedEmail, err := domain.MaskSupportIssueEmail(normalizedEmail)
	require.NoError(t, err)

	title := "Issue " + uniqueTestValue(t, "title")
	description := "Detailed support issue description for " + t.Name()
	screenshotText := "upstream error insufficient_quota for test " + t.Name()
	userIDPtr := userID
	issue := &service.SupportIssue{
		Title:                  title,
		Description:            description,
		AccountEmail:           normalizedEmail,
		AccountEmailNormalized: normalizedEmail,
		AccountEmailMasked:     maskedEmail,
		OccurredAt:             time.Now().UTC().Add(-1 * time.Hour),
		ScreenshotText:         screenshotText,
		ScreenshotLanguage:     service.SupportIssueScreenshotLanguageEN,
		Category:               service.SupportIssueCategoryPayment,
		Severity:               service.SupportIssueSeverityBlocked,
		Status:                 service.SupportIssueStatusOpen,
		ModelName:              "claude-sonnet-test",
		ClientName:             "claude-code",
		ErrorCode:              "insufficient_quota",
		APIKeySuffix:           "ab12cd",
		CreatedByUserID:        &userIDPtr,
		SearchText:             strings.Join([]string{title, description, screenshotText, "insufficient_quota claude-code"}, " "),
	}
	if mutate != nil {
		mutate(issue)
	}
	return issue
}

func newSupportIssueAttachmentFixture(userID int64) service.SupportIssueAttachment {
	return service.SupportIssueAttachment{
		UploadedByUserID: &userID,
		FilePath:         "data/support-issue-attachments/test.png",
		FileURL:          "/uploads/support-issues/test.png",
		FileName:         "test.png",
		MimeType:         "image/png",
		SizeBytes:        1024,
		Visibility:       service.SupportIssueAttachmentVisibilityPublic,
	}
}

func newSupportIssueEvent(userID int64, eventType string) service.SupportIssueEvent {
	return service.SupportIssueEvent{
		ActorUserID: &userID,
		EventType:   eventType,
		Metadata:    map[string]any{"test": true},
	}
}

func searchSupportIssues(
	t *testing.T,
	ctx context.Context,
	repo service.SupportIssueRepository,
	parsed service.ParsedSupportIssueSearch,
) []service.SupportIssue {
	t.Helper()

	items, _, err := repo.SearchIssues(ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, service.SearchSupportIssueQuery{
		Parsed: parsed,
	})
	require.NoError(t, err)
	return items
}

func requireSupportIssueIDsContain(t *testing.T, items []service.SupportIssue, id int64) {
	t.Helper()

	for _, item := range items {
		if item.ID == id {
			return
		}
	}
	t.Fatalf("support issue id %d not found in %#v", id, items)
}

func requireSupportIssueIDsNotContain(t *testing.T, items []service.SupportIssue, id int64) {
	t.Helper()

	for _, item := range items {
		if item.ID == id {
			t.Fatalf("support issue id %d unexpectedly found in %#v", id, items)
		}
	}
}

func ptrInt(v int) *int {
	return &v
}
