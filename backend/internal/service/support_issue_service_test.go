//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestSupportIssueService_CreateNormalizesEmailAndBuildsIssue(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)
	code := 429

	issue, err := svc.Create(context.Background(), supportIssueTestActor(42, false), validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.HTTPStatus = &code
		input.ModelName = "claude-sonnet"
		input.ClientName = "Claude Code"
		input.ErrorCode = "insufficient_quota"
	}))

	require.NoError(t, err)
	require.Same(t, repo.createdIssue, issue)
	require.Equal(t, "user@example.com", issue.AccountEmail)
	require.Equal(t, "user@example.com", issue.AccountEmailNormalized)
	require.Equal(t, "u***@example.com", issue.AccountEmailMasked)
	require.NotNil(t, issue.CreatedByUserID)
	require.Equal(t, int64(42), *issue.CreatedByUserID)
	require.Equal(t, SupportIssueStatusOpen, issue.Status)
	require.Contains(t, issue.SearchText, issue.Title)
	require.Contains(t, issue.SearchText, issue.Description)
	require.Contains(t, issue.SearchText, issue.AccountEmailMasked)
	require.Contains(t, issue.SearchText, issue.AccountEmailNormalized)
	require.Contains(t, issue.SearchText, issue.ScreenshotText)
	require.Contains(t, issue.SearchText, issue.Category)
	require.Contains(t, issue.SearchText, issue.Severity)
	require.Contains(t, issue.SearchText, issue.ModelName)
	require.Contains(t, issue.SearchText, issue.ClientName)
	require.Contains(t, issue.SearchText, issue.ErrorCode)
	require.Contains(t, issue.SearchText, "429")
	require.Equal(t, SupportIssueEventCreated, repo.createdEvent.EventType)
	require.NotNil(t, repo.createdEvent.ActorUserID)
	require.Equal(t, int64(42), *repo.createdEvent.ActorUserID)
}

func TestSupportIssueService_CreateMissingRequiredFieldsFails(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)
	input := validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.Title = ""
	})

	_, err := svc.Create(context.Background(), supportIssueTestActor(42, false), input)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueTitleInvalid))
	require.Nil(t, repo.createdIssue)
}

func TestSupportIssueService_CreateDoesNotTrustCallerDerivedFields(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)
	input := validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.AccountEmail = "User@Example.com"
		input.AccountEmailNormalized = "attacker@example.net"
		input.AccountEmailMasked = "a***@example.net"
		input.Status = SupportIssueStatusClosed
		input.SearchText = "caller supplied search text"
	})

	issue, err := svc.Create(context.Background(), supportIssueTestActor(42, false), input)

	require.NoError(t, err)
	require.Equal(t, "user@example.com", issue.AccountEmailNormalized)
	require.Equal(t, "u***@example.com", issue.AccountEmailMasked)
	require.Equal(t, SupportIssueStatusOpen, issue.Status)
	require.NotEqual(t, "caller supplied search text", issue.SearchText)
}

func TestSupportIssueService_UploadAttachmentRejectsNonImageMIME(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.UploadAttachment(context.Background(), supportIssueTestActor(42, false), UploadSupportIssueAttachmentInput{
		FileName:    "note.txt",
		ContentType: "text/plain",
		Content:     []byte("not an image"),
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueAttachmentInvalidType))
	require.False(t, repo.createUnboundAttachmentCalled)
}

func TestSupportIssueService_UploadAttachmentRejectsTooLargeFile(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.UploadAttachment(context.Background(), supportIssueTestActor(42, false), UploadSupportIssueAttachmentInput{
		FileName:    "large.png",
		ContentType: "image/png",
		Content:     append(supportIssueTestPNG(), make([]byte, SupportIssueMaxAttachmentBytes)...),
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueAttachmentTooLarge))
	require.False(t, repo.createUnboundAttachmentCalled)
}

func TestSupportIssueService_UploadAttachmentSanitizesFileNameAndHidesPath(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)
	oldRoot := supportIssueAttachmentStorageRoot
	supportIssueAttachmentStorageRoot = t.TempDir()
	t.Cleanup(func() {
		supportIssueAttachmentStorageRoot = oldRoot
	})

	attachment, err := svc.UploadAttachment(context.Background(), supportIssueTestActor(42, false), UploadSupportIssueAttachmentInput{
		FileName:    "../bad\\name\u0000.png",
		ContentType: "image/png",
		Content:     supportIssueTestPNG(),
	})

	require.NoError(t, err)
	require.True(t, repo.createUnboundAttachmentCalled)
	require.NotContains(t, repo.createdUnboundAttachment.FileName, "/")
	require.NotContains(t, repo.createdUnboundAttachment.FileName, "\\")
	require.NotContains(t, repo.createdUnboundAttachment.FileName, "\u0000")
	require.NoFileExists(t, repo.createdUnboundAttachment.FileName)
	require.FileExists(t, repo.createdUnboundAttachment.FilePath)
	require.Empty(t, attachment.FilePath)
	require.Nil(t, attachment.UploadedByUserID)
	require.Equal(t, int64(777), attachment.ID)
	require.Equal(t, "/api/v1/issues/attachments/777/file", attachment.FileURL)
}

func TestSupportIssueService_CreateBindsAttachmentIDs(t *testing.T) {
	userID := int64(42)
	repo := &fakeSupportIssueRepository{
		unboundAttachments: []SupportIssueAttachment{supportIssueTestUnboundAttachment(777, userID)},
	}
	svc := NewSupportIssueService(repo)

	issue, err := svc.Create(context.Background(), supportIssueTestActor(userID, false), validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.AttachmentIDs = []int64{777}
	}))

	require.NoError(t, err)
	require.True(t, repo.listUnboundAttachmentsCalled)
	require.Equal(t, []int64{777}, repo.lastUnboundAttachmentIDs)
	require.Len(t, repo.createdAttachments, 1)
	require.Equal(t, int64(777), repo.createdAttachments[0].ID)
	require.Equal(t, 1, issue.AttachmentCount)
	require.Len(t, issue.Attachments, 1)
}

func TestSupportIssueService_CreateRejectsForeignAttachmentID(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.Create(context.Background(), supportIssueTestActor(42, false), validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.AttachmentIDs = []int64{777}
	}))

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueAttachmentNotFound))
	require.Nil(t, repo.createdIssue)
}

func TestSupportIssueService_CreateRejectsAlreadyBoundAttachmentID(t *testing.T) {
	attachment := supportIssueTestUnboundAttachment(777, 42)
	attachment.IssueID = 99
	repo := &fakeSupportIssueRepository{unboundAttachments: []SupportIssueAttachment{attachment}}
	svc := NewSupportIssueService(repo)

	_, err := svc.Create(context.Background(), supportIssueTestActor(42, false), validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.AttachmentIDs = []int64{777}
	}))

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueAttachmentNotFound))
	require.Nil(t, repo.createdIssue)
}

func TestSupportIssueService_CreateRejectsTooManyAttachmentIDs(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.Create(context.Background(), supportIssueTestActor(42, false), validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.AttachmentIDs = []int64{1, 2, 3, 4, 5, 6}
	}))

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueInvalidInput))
	require.False(t, repo.listUnboundAttachmentsCalled)
	require.Nil(t, repo.createdIssue)
}

func TestSupportIssueService_OpenAttachmentForPublicRequiresPublicBoundAttachment(t *testing.T) {
	oldRoot := supportIssueAttachmentStorageRoot
	supportIssueAttachmentStorageRoot = t.TempDir()
	t.Cleanup(func() {
		supportIssueAttachmentStorageRoot = oldRoot
	})
	filePath := filepath.Join(supportIssueAttachmentStorageRoot, "public.png")
	require.NoError(t, os.WriteFile(filePath, supportIssueTestPNG(), 0o600))

	repo := &fakeSupportIssueRepository{
		publicAttachment: func() *SupportIssueAttachment {
			attachment := supportIssueTestUnboundAttachment(777, 42)
			attachment.IssueID = 123
			attachment.FilePath = filePath
			return &attachment
		}(),
	}
	svc := NewSupportIssueService(repo)

	attachment, err := svc.OpenAttachmentForPublic(context.Background(), 777)

	require.NoError(t, err)
	require.Equal(t, int64(123), attachment.IssueID)
	require.Equal(t, filepath.Clean(filePath), filepath.Clean(attachment.FilePath))
}

func TestSupportIssueService_OpenAttachmentForPublicRejectsHiddenAttachment(t *testing.T) {
	attachment := supportIssueTestUnboundAttachment(777, 42)
	attachment.IssueID = 123
	attachment.Visibility = SupportIssueAttachmentVisibilityHidden
	repo := &fakeSupportIssueRepository{publicAttachment: &attachment}
	svc := NewSupportIssueService(repo)

	_, err := svc.OpenAttachmentForPublic(context.Background(), 777)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueAttachmentNotFound))
}

func TestSupportIssueService_OpenAttachmentForPublicRejectsUnboundAttachment(t *testing.T) {
	attachment := supportIssueTestUnboundAttachment(777, 42)
	repo := &fakeSupportIssueRepository{publicAttachment: &attachment}
	svc := NewSupportIssueService(repo)

	_, err := svc.OpenAttachmentForPublic(context.Background(), 777)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueAttachmentNotFound))
}

func TestSupportIssueService_SearchPublicRejectsKeyFilter(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, _, err := svc.SearchPublic(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, "key:ab12cd", ListSupportIssueFilters{})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueSearchInvalid))
	require.False(t, repo.searchCalled)
}

func TestSupportIssueService_AdminSearchAllowsKeyFilter(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, _, err := svc.AdminSearch(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, "key:ab12cd", ListSupportIssueFilters{})

	require.NoError(t, err)
	require.True(t, repo.searchCalled)
	require.Equal(t, "ab12cd", repo.lastSearchQuery.Parsed.APIKeySuffix)
	require.True(t, repo.lastSearchQuery.IncludeHidden)
}

func TestSupportIssueService_AddCommentRejectsLockedIssue(t *testing.T) {
	lockedAt := time.Now()
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:              12,
		Status:          SupportIssueStatusOpen,
		LockedAt:        &lockedAt,
		CreatedByUserID: supportIssueTestInt64(42),
	}}
	svc := NewSupportIssueService(repo)

	_, err := svc.AddComment(context.Background(), supportIssueTestActor(42, false), 12, "I can reproduce this.", nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueLocked))
	require.False(t, repo.addCommentCalled)
}

func TestSupportIssueService_AddCommentRejectsResolvedIssue(t *testing.T) {
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:              12,
		Status:          SupportIssueStatusResolved,
		CreatedByUserID: supportIssueTestInt64(42),
	}}
	svc := NewSupportIssueService(repo)

	_, err := svc.AddComment(context.Background(), supportIssueTestActor(42, false), 12, "I can reproduce this.", nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueLocked))
	require.False(t, repo.addCommentCalled)
}

func TestSupportIssueService_AddCommentOnOpenIssueCreatesCommentedEvent(t *testing.T) {
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:              12,
		Status:          SupportIssueStatusOpen,
		CreatedByUserID: supportIssueTestInt64(42),
	}}
	svc := NewSupportIssueService(repo)

	comment, err := svc.AddComment(context.Background(), supportIssueTestActor(42, false), 12, "  I can reproduce this.  ", nil)

	require.NoError(t, err)
	require.True(t, repo.addCommentCalled)
	require.Equal(t, int64(12), comment.IssueID)
	require.Equal(t, "I can reproduce this.", comment.Content)
	require.Equal(t, RoleUser, comment.AuthorRole)
	require.Equal(t, SupportIssueEventCommented, repo.addCommentEvent.EventType)
	require.NotNil(t, repo.addCommentEvent.ActorUserID)
	require.Equal(t, int64(42), *repo.addCommentEvent.ActorUserID)
}

func TestSupportIssueService_ResolveReporterOwnIssueSucceeds(t *testing.T) {
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:              12,
		Status:          SupportIssueStatusOpen,
		CreatedByUserID: supportIssueTestInt64(42),
	}}
	svc := NewSupportIssueService(repo)

	_, err := svc.Resolve(context.Background(), supportIssueTestActor(42, false), 12)

	require.NoError(t, err)
	require.True(t, repo.updateStatusCalled)
	require.Equal(t, SupportIssueStatusResolved, repo.lastUpdateStatus)
	require.False(t, repo.lastUpdateActorIsAdmin)
}

func TestSupportIssueService_ResolveAdminAnyIssueSucceeds(t *testing.T) {
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:              12,
		Status:          SupportIssueStatusOpen,
		CreatedByUserID: supportIssueTestInt64(7),
	}}
	svc := NewSupportIssueService(repo)

	_, err := svc.Resolve(context.Background(), supportIssueTestActor(42, true), 12)

	require.NoError(t, err)
	require.True(t, repo.updateStatusCalled)
	require.Equal(t, SupportIssueStatusResolved, repo.lastUpdateStatus)
	require.True(t, repo.lastUpdateActorIsAdmin)
}

func TestSupportIssueService_ResolveOtherUserIssueFails(t *testing.T) {
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:              12,
		Status:          SupportIssueStatusOpen,
		CreatedByUserID: supportIssueTestInt64(7),
	}}
	svc := NewSupportIssueService(repo)

	_, err := svc.Resolve(context.Background(), supportIssueTestActor(42, false), 12)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssuePermissionDenied))
	require.False(t, repo.updateStatusCalled)
}

func TestSupportIssueService_AdminUpdateStatusRejectsNonAdmin(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.AdminUpdateStatus(context.Background(), supportIssueTestActor(42, false), 12, SupportIssueStatusInProgress, "")

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssuePermissionDenied))
}

func TestSupportIssueService_AdminReopenRejectsNonAdmin(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.AdminReopen(context.Background(), supportIssueTestActor(42, false), 12, "")

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssuePermissionDenied))
}

func TestSupportIssueService_AdminHideCommentRejectsNonAdmin(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	err := svc.AdminHideComment(context.Background(), supportIssueTestActor(42, false), 12, 99, "")

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssuePermissionDenied))
	require.False(t, repo.hideCommentCalled)
}

func TestSupportIssueService_AdminHideAttachmentRejectsNonAdmin(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	err := svc.AdminHideAttachment(context.Background(), supportIssueTestActor(42, false), 12, 99, "")

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssuePermissionDenied))
	require.False(t, repo.hideAttachmentCalled)
}

func TestSupportIssueService_AdminHideIssueRejectsNonAdmin(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.AdminHideIssue(context.Background(), supportIssueTestActor(42, false), 12, "")

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssuePermissionDenied))
	require.False(t, repo.hideIssueCalled)
}

func TestSupportIssueService_AdminHideCommentUsesDefaultReason(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	err := svc.AdminHideComment(context.Background(), supportIssueTestActor(42, true), 12, 99, "")

	require.NoError(t, err)
	require.True(t, repo.hideCommentCalled)
	require.Equal(t, SupportIssueDefaultHideReason, repo.lastHideCommentInput.HideReason)
	require.Equal(t, SupportIssueEventCommentHidden, repo.hideCommentEvent.EventType)
}

func TestSupportIssueService_AdminHideIssueUsesDefaultReason(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.AdminHideIssue(context.Background(), supportIssueTestActor(42, true), 12, "")

	require.NoError(t, err)
	require.True(t, repo.hideIssueCalled)
	require.Equal(t, SupportIssueDefaultHideReason, repo.lastHideIssueInput.HideReason)
	require.Equal(t, SupportIssueEventIssueHidden, repo.hideIssueEvent.EventType)
}

func TestSupportIssueService_AdminRestoreIssueCallsRepository(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.AdminRestoreIssue(context.Background(), supportIssueTestActor(42, true), 12, "visible again")

	require.NoError(t, err)
	require.True(t, repo.restoreIssueCalled)
	require.Equal(t, int64(12), repo.lastRestoreIssueID)
	require.Equal(t, "visible again", repo.restoreIssueEvent.Metadata["reason"])
}

func TestSupportIssueService_SuggestSimilarReturnsAtMostFive(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	for i := 0; i < 8; i++ {
		repo.searchIssuesResult = append(repo.searchIssuesResult, SupportIssue{
			ID:                 int64(i + 1),
			Title:              fmt.Sprintf("rate limit %d", i),
			AccountEmail:       "user@example.com",
			AccountEmailMasked: "u***@example.com",
			APIKeySuffix:       "ab12cd",
		})
	}
	svc := NewSupportIssueService(repo)
	code := 429

	items, err := svc.SuggestSimilar(context.Background(), supportIssueTestActor(42, false), validCreateSupportIssueInput(func(input *CreateSupportIssueInput) {
		input.Title = "rate limit on claude"
		input.ScreenshotText = "rate limit error"
		input.ErrorCode = "rate_limit"
		input.HTTPStatus = &code
	}))

	require.NoError(t, err)
	require.Len(t, items, 5)
	require.True(t, repo.searchCalled)
	require.Equal(t, 5, repo.lastSearchParams.PageSize)
	require.Equal(t, "rate_limit", repo.lastSearchQuery.Parsed.ErrorCode)
	require.NotNil(t, repo.lastSearchQuery.Parsed.HTTPStatus)
	require.Equal(t, 429, *repo.lastSearchQuery.Parsed.HTTPStatus)
}

func TestSupportIssueService_GetPublicUsesVisibleQuery(t *testing.T) {
	uploadedByUserID := int64(42)
	hiddenByUserID := int64(99)
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:     12,
		Status: SupportIssueStatusOpen,
		Comments: []SupportIssueComment{{
			ID:             31,
			IssueID:        12,
			AuthorUserID:   &uploadedByUserID,
			AuthorRole:     RoleUser,
			Content:        "visible public comment",
			HiddenByUserID: &hiddenByUserID,
			HideReason:     "admin moderation detail",
		}},
		Attachments: []SupportIssueAttachment{{
			ID:               44,
			IssueID:          12,
			UploadedByUserID: &uploadedByUserID,
			FilePath:         "data/support-issue-attachments/private.png",
			FileURL:          "/uploads/support-issues/public.png",
			FileName:         "public.png",
			MimeType:         "image/png",
			SizeBytes:        1024,
			OCRText:          "visible ocr text",
			Visibility:       SupportIssueAttachmentVisibilityPublic,
			HiddenByUserID:   &hiddenByUserID,
		}},
	}}
	svc := NewSupportIssueService(repo)

	issue, err := svc.GetPublic(context.Background(), 12, SupportIssueViewer{IP: "127.0.0.1", UserAgent: "test"})

	require.NoError(t, err)
	require.Equal(t, []bool{false}, repo.getIssueIncludeHidden)
	require.True(t, repo.recordViewCalled)
	require.Len(t, issue.Attachments, 1)
	require.Empty(t, issue.Attachments[0].FilePath)
	require.Nil(t, issue.Attachments[0].UploadedByUserID)
	require.Nil(t, issue.Attachments[0].HiddenByUserID)
	require.Equal(t, "/uploads/support-issues/public.png", issue.Attachments[0].FileURL)
	require.Equal(t, "public.png", issue.Attachments[0].FileName)
	require.Equal(t, "image/png", issue.Attachments[0].MimeType)
	require.Equal(t, int64(1024), issue.Attachments[0].SizeBytes)
	require.Equal(t, "visible ocr text", issue.Attachments[0].OCRText)
	require.Len(t, issue.Comments, 1)
	require.Nil(t, issue.Comments[0].AuthorUserID)
	require.Nil(t, issue.Comments[0].HiddenByUserID)
	require.Empty(t, issue.Comments[0].HideReason)
}

func TestSupportIssueService_GetPublicRejectsHiddenIssue(t *testing.T) {
	now := time.Now()
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:       12,
		Status:   SupportIssueStatusOpen,
		HiddenAt: &now,
	}}
	svc := NewSupportIssueService(repo)

	_, err := svc.GetPublic(context.Background(), 12, SupportIssueViewer{IP: "127.0.0.1"})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSupportIssueNotFound))
	require.False(t, repo.recordViewCalled)
}

func TestSupportIssueService_NotificationSummaryRequiresActor(t *testing.T) {
	repo := &fakeSupportIssueRepository{}
	svc := NewSupportIssueService(repo)

	_, err := svc.NotificationSummary(context.Background(), SupportIssueActor{})

	require.Error(t, err)
}

func TestSupportIssueService_NotificationSummaryReturnsRepoSummary(t *testing.T) {
	repo := &fakeSupportIssueRepository{notificationSummary: SupportIssueNotificationSummary{
		UnreadCount:    2,
		NeedsInfoCount: 1,
	}}
	svc := NewSupportIssueService(repo)

	got, err := svc.NotificationSummary(context.Background(), SupportIssueActor{UserID: 42})

	require.NoError(t, err)
	require.Equal(t, 2, got.UnreadCount)
	require.Equal(t, 1, got.NeedsInfoCount)
}

func TestSupportIssueService_AdminGetUsesHiddenQuery(t *testing.T) {
	uploadedByUserID := int64(42)
	hiddenByUserID := int64(99)
	repo := &fakeSupportIssueRepository{issue: &SupportIssue{
		ID:     12,
		Status: SupportIssueStatusOpen,
		Comments: []SupportIssueComment{{
			ID:             31,
			IssueID:        12,
			AuthorUserID:   &uploadedByUserID,
			AuthorRole:     RoleUser,
			Content:        "visible admin comment",
			HiddenByUserID: &hiddenByUserID,
			HideReason:     "admin moderation detail",
		}},
		Attachments: []SupportIssueAttachment{{
			ID:               44,
			IssueID:          12,
			UploadedByUserID: &uploadedByUserID,
			FilePath:         "data/support-issue-attachments/private.png",
			FileURL:          "/uploads/support-issues/public.png",
			FileName:         "public.png",
			MimeType:         "image/png",
			SizeBytes:        1024,
			Visibility:       SupportIssueAttachmentVisibilityPublic,
			HiddenByUserID:   &hiddenByUserID,
		}},
	}}
	svc := NewSupportIssueService(repo)

	issue, err := svc.AdminGet(context.Background(), 12)

	require.NoError(t, err)
	require.Equal(t, []bool{true}, repo.getIssueIncludeHidden)
	require.Len(t, issue.Attachments, 1)
	require.Equal(t, "data/support-issue-attachments/private.png", issue.Attachments[0].FilePath)
	require.NotNil(t, issue.Attachments[0].UploadedByUserID)
	require.NotNil(t, issue.Attachments[0].HiddenByUserID)
	require.Len(t, issue.Comments, 1)
	require.Equal(t, "admin moderation detail", issue.Comments[0].HideReason)
	require.NotNil(t, issue.Comments[0].HiddenByUserID)
}

type fakeSupportIssueRepository struct {
	createdIssue       *SupportIssue
	createdAttachments []SupportIssueAttachment
	createdEvent       SupportIssueEvent
	createErr          error

	createUnboundAttachmentCalled bool
	createdUnboundAttachment      SupportIssueAttachment
	createUnboundAttachmentErr    error

	listUnboundAttachmentsCalled bool
	lastUnboundAttachmentUserID  int64
	lastUnboundAttachmentIDs     []int64
	unboundAttachments           []SupportIssueAttachment
	listUnboundAttachmentsErr    error

	publicAttachment        *SupportIssueAttachment
	openPublicAttachmentID  int64
	openPublicAttachmentErr error

	issue                 *SupportIssue
	getIssueErr           error
	getIssueIncludeHidden []bool
	recordViewCalled      bool
	lastViewIssueID       int64
	lastViewer            SupportIssueViewer
	notificationSummary   SupportIssueNotificationSummary
	notificationErr       error

	listIssuesResult []SupportIssue
	listPage         *pagination.PaginationResult
	listErr          error

	searchCalled       bool
	searchIssuesResult []SupportIssue
	searchPage         *pagination.PaginationResult
	searchErr          error
	lastSearchParams   pagination.PaginationParams
	lastSearchQuery    SearchSupportIssueQuery

	addCommentCalled bool
	lastComment      *SupportIssueComment
	addCommentEvent  SupportIssueEvent
	addCommentErr    error

	updateStatusCalled     bool
	lastUpdateIssueID      int64
	lastUpdateStatus       string
	lastUpdateActorID      int64
	lastUpdateActorIsAdmin bool
	lastUpdateEvent        SupportIssueEvent
	updateStatusErr        error

	hideIssueCalled     bool
	lastHideIssueInput  HideSupportIssueInput
	hideIssueEvent      SupportIssueEvent
	hideIssueErr        error
	restoreIssueCalled  bool
	lastRestoreIssueID  int64
	restoreIssueActorID int64
	restoreIssueEvent   SupportIssueEvent
	restoreIssueErr     error

	pinIssueCalled          bool
	lastPinIssueInput       PinSupportIssueInput
	pinIssueEvent           SupportIssueEvent
	pinIssueErr             error
	unpinIssueCalled        bool
	lastUnpinIssueID        int64
	unpinIssueEvent         SupportIssueEvent
	unpinIssueErr           error
	setSolutionCalled       bool
	lastSolutionIssueID     int64
	lastSolutionCommentID   int64
	setSolutionEvent        SupportIssueEvent
	setSolutionErr          error
	clearSolutionCalled     bool
	lastClearSolutionID     int64
	clearSolutionEvent      SupportIssueEvent
	clearSolutionErr        error
	setRelatedIssueCalled   bool
	lastSetRelatedInput     SetRelatedSupportIssueInput
	setRelatedIssueEvent    SupportIssueEvent
	setRelatedIssueErr      error
	clearRelatedIssueCalled bool
	lastClearRelatedID      int64
	clearRelatedIssueEvent  SupportIssueEvent
	clearRelatedIssueErr    error

	hideCommentCalled    bool
	lastHideCommentInput HideSupportIssueCommentInput
	hideCommentEvent     SupportIssueEvent
	hideCommentErr       error
	hideAttachmentCalled bool
	lastHideAttachment   HideSupportIssueAttachmentInput
	hideAttachmentEvent  SupportIssueEvent
	hideAttachmentErr    error
	listEventsCalled     bool
	listEventsResult     []SupportIssueEvent
	listEventsErr        error
}

func (r *fakeSupportIssueRepository) CreateIssue(ctx context.Context, issue *SupportIssue, attachments []SupportIssueAttachment, event SupportIssueEvent) error {
	r.createdIssue = issue
	r.createdAttachments = append([]SupportIssueAttachment(nil), attachments...)
	r.createdEvent = event
	if r.createErr != nil {
		return r.createErr
	}
	if issue.ID == 0 {
		issue.ID = 123
	}
	if issue.PublicID == "" {
		issue.PublicID = "ISS-000123"
	}
	issue.AttachmentCount = len(attachments)
	issue.Attachments = append([]SupportIssueAttachment(nil), attachments...)
	for i := range issue.Attachments {
		issue.Attachments[i].IssueID = issue.ID
	}
	return nil
}

func (r *fakeSupportIssueRepository) CreateUnboundAttachment(ctx context.Context, attachment *SupportIssueAttachment) error {
	r.createUnboundAttachmentCalled = true
	if r.createUnboundAttachmentErr != nil {
		return r.createUnboundAttachmentErr
	}
	if attachment.ID == 0 {
		attachment.ID = 777
	}
	if attachment.FileURL == "" {
		attachment.FileURL = fmt.Sprintf("/api/v1/issues/attachments/%d/file", attachment.ID)
	}
	r.createdUnboundAttachment = *attachment
	return nil
}

func (r *fakeSupportIssueRepository) ListUnboundAttachmentsForUser(ctx context.Context, userID int64, ids []int64) ([]SupportIssueAttachment, error) {
	r.listUnboundAttachmentsCalled = true
	r.lastUnboundAttachmentUserID = userID
	r.lastUnboundAttachmentIDs = append([]int64(nil), ids...)
	if r.listUnboundAttachmentsErr != nil {
		return nil, r.listUnboundAttachmentsErr
	}
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	out := make([]SupportIssueAttachment, 0, len(r.unboundAttachments))
	for _, attachment := range r.unboundAttachments {
		if _, ok := idSet[attachment.ID]; ok {
			out = append(out, attachment)
		}
	}
	return out, nil
}

func (r *fakeSupportIssueRepository) OpenAttachmentForPublic(ctx context.Context, attachmentID int64) (*SupportIssueAttachment, error) {
	r.openPublicAttachmentID = attachmentID
	if r.openPublicAttachmentErr != nil {
		return nil, r.openPublicAttachmentErr
	}
	if r.publicAttachment == nil {
		return nil, ErrSupportIssueAttachmentNotFound
	}
	out := *r.publicAttachment
	return &out, nil
}

func (r *fakeSupportIssueRepository) GetIssue(ctx context.Context, id int64, includeHidden bool) (*SupportIssue, error) {
	r.getIssueIncludeHidden = append(r.getIssueIncludeHidden, includeHidden)
	if r.getIssueErr != nil {
		return nil, r.getIssueErr
	}
	if r.issue == nil {
		return nil, ErrSupportIssueNotFound
	}
	out := *r.issue
	return &out, nil
}

func (r *fakeSupportIssueRepository) RecordView(ctx context.Context, issueID int64, viewer SupportIssueViewer, throttleWindow time.Duration) error {
	r.recordViewCalled = true
	r.lastViewIssueID = issueID
	r.lastViewer = viewer
	return nil
}

func (r *fakeSupportIssueRepository) GetUserNotificationSummary(ctx context.Context, userID int64) (SupportIssueNotificationSummary, error) {
	if r.notificationErr != nil {
		return SupportIssueNotificationSummary{}, r.notificationErr
	}
	return r.notificationSummary, nil
}

func (r *fakeSupportIssueRepository) ListIssues(ctx context.Context, params pagination.PaginationParams, filters ListSupportIssueFilters) ([]SupportIssue, *pagination.PaginationResult, error) {
	if r.listErr != nil {
		return nil, nil, r.listErr
	}
	return append([]SupportIssue(nil), r.listIssuesResult...), r.listPage, nil
}

func (r *fakeSupportIssueRepository) SearchIssues(ctx context.Context, params pagination.PaginationParams, query SearchSupportIssueQuery) ([]SupportIssue, *pagination.PaginationResult, error) {
	r.searchCalled = true
	r.lastSearchParams = params
	r.lastSearchQuery = query
	if r.searchErr != nil {
		return nil, nil, r.searchErr
	}
	return append([]SupportIssue(nil), r.searchIssuesResult...), r.searchPage, nil
}

func (r *fakeSupportIssueRepository) AddComment(ctx context.Context, comment *SupportIssueComment, event SupportIssueEvent) error {
	r.addCommentCalled = true
	r.lastComment = comment
	r.addCommentEvent = event
	if r.addCommentErr != nil {
		return r.addCommentErr
	}
	if comment.ID == 0 {
		comment.ID = 321
	}
	return nil
}

func (r *fakeSupportIssueRepository) UpdateStatus(
	ctx context.Context,
	issueID int64,
	nextStatus string,
	actorUserID int64,
	actorIsAdmin bool,
	event SupportIssueEvent,
) (*SupportIssue, error) {
	r.updateStatusCalled = true
	r.lastUpdateIssueID = issueID
	r.lastUpdateStatus = nextStatus
	r.lastUpdateActorID = actorUserID
	r.lastUpdateActorIsAdmin = actorIsAdmin
	r.lastUpdateEvent = event
	if r.updateStatusErr != nil {
		return nil, r.updateStatusErr
	}
	updated := &SupportIssue{ID: issueID, Status: nextStatus}
	if r.issue != nil {
		copy := *r.issue
		copy.Status = nextStatus
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) HideIssue(ctx context.Context, input HideSupportIssueInput, event SupportIssueEvent) (*SupportIssue, error) {
	r.hideIssueCalled = true
	r.lastHideIssueInput = input
	r.hideIssueEvent = event
	if r.hideIssueErr != nil {
		return nil, r.hideIssueErr
	}
	updated := &SupportIssue{ID: input.IssueID, HiddenByUserID: &input.HiddenByUserID, HideReason: input.HideReason}
	now := time.Now()
	updated.HiddenAt = &now
	if r.issue != nil {
		copy := *r.issue
		copy.HiddenAt = &now
		copy.HiddenByUserID = &input.HiddenByUserID
		copy.HideReason = input.HideReason
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) RestoreIssue(ctx context.Context, issueID int64, actorUserID int64, event SupportIssueEvent) (*SupportIssue, error) {
	r.restoreIssueCalled = true
	r.lastRestoreIssueID = issueID
	r.restoreIssueActorID = actorUserID
	r.restoreIssueEvent = event
	if r.restoreIssueErr != nil {
		return nil, r.restoreIssueErr
	}
	updated := &SupportIssue{ID: issueID}
	if r.issue != nil {
		copy := *r.issue
		copy.HiddenAt = nil
		copy.HiddenByUserID = nil
		copy.HideReason = ""
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) PinIssue(ctx context.Context, input PinSupportIssueInput, event SupportIssueEvent) (*SupportIssue, error) {
	r.pinIssueCalled = true
	r.lastPinIssueInput = input
	r.pinIssueEvent = event
	if r.pinIssueErr != nil {
		return nil, r.pinIssueErr
	}
	now := time.Now()
	updated := &SupportIssue{ID: input.IssueID, PinnedAt: &now, PinnedByUserID: &input.PinnedByUserID, PinnedReason: input.Reason}
	if r.issue != nil {
		copy := *r.issue
		copy.PinnedAt = &now
		copy.PinnedByUserID = &input.PinnedByUserID
		copy.PinnedReason = input.Reason
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) UnpinIssue(ctx context.Context, issueID int64, actorUserID int64, event SupportIssueEvent) (*SupportIssue, error) {
	r.unpinIssueCalled = true
	r.lastUnpinIssueID = issueID
	r.unpinIssueEvent = event
	if r.unpinIssueErr != nil {
		return nil, r.unpinIssueErr
	}
	updated := &SupportIssue{ID: issueID}
	if r.issue != nil {
		copy := *r.issue
		copy.PinnedAt = nil
		copy.PinnedByUserID = nil
		copy.PinnedReason = ""
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) SetSolutionComment(ctx context.Context, issueID int64, commentID int64, actorUserID int64, event SupportIssueEvent) (*SupportIssue, error) {
	r.setSolutionCalled = true
	r.lastSolutionIssueID = issueID
	r.lastSolutionCommentID = commentID
	r.setSolutionEvent = event
	if r.setSolutionErr != nil {
		return nil, r.setSolutionErr
	}
	updated := &SupportIssue{ID: issueID, SolutionCommentID: &commentID}
	if r.issue != nil {
		copy := *r.issue
		copy.SolutionCommentID = &commentID
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) ClearSolutionComment(ctx context.Context, issueID int64, actorUserID int64, event SupportIssueEvent) (*SupportIssue, error) {
	r.clearSolutionCalled = true
	r.lastClearSolutionID = issueID
	r.clearSolutionEvent = event
	if r.clearSolutionErr != nil {
		return nil, r.clearSolutionErr
	}
	updated := &SupportIssue{ID: issueID}
	if r.issue != nil {
		copy := *r.issue
		copy.SolutionCommentID = nil
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) SetRelatedIssue(ctx context.Context, input SetRelatedSupportIssueInput, event SupportIssueEvent) (*SupportIssue, error) {
	r.setRelatedIssueCalled = true
	r.lastSetRelatedInput = input
	r.setRelatedIssueEvent = event
	if r.setRelatedIssueErr != nil {
		return nil, r.setRelatedIssueErr
	}
	updated := &SupportIssue{ID: input.IssueID, RelatedIssueID: &input.RelatedIssueID, RelatedIssueReason: input.Reason}
	if r.issue != nil {
		copy := *r.issue
		copy.RelatedIssueID = &input.RelatedIssueID
		copy.RelatedIssueReason = input.Reason
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) ClearRelatedIssue(ctx context.Context, issueID int64, actorUserID int64, event SupportIssueEvent) (*SupportIssue, error) {
	r.clearRelatedIssueCalled = true
	r.lastClearRelatedID = issueID
	r.clearRelatedIssueEvent = event
	if r.clearRelatedIssueErr != nil {
		return nil, r.clearRelatedIssueErr
	}
	updated := &SupportIssue{ID: issueID}
	if r.issue != nil {
		copy := *r.issue
		copy.RelatedIssueID = nil
		copy.RelatedIssueReason = ""
		updated = &copy
	}
	return updated, nil
}

func (r *fakeSupportIssueRepository) HideComment(ctx context.Context, input HideSupportIssueCommentInput, event SupportIssueEvent) error {
	r.hideCommentCalled = true
	r.lastHideCommentInput = input
	r.hideCommentEvent = event
	return r.hideCommentErr
}

func (r *fakeSupportIssueRepository) HideAttachment(ctx context.Context, input HideSupportIssueAttachmentInput, event SupportIssueEvent) error {
	r.hideAttachmentCalled = true
	r.lastHideAttachment = input
	r.hideAttachmentEvent = event
	return r.hideAttachmentErr
}

func (r *fakeSupportIssueRepository) ListEvents(ctx context.Context, issueID int64) ([]SupportIssueEvent, error) {
	r.listEventsCalled = true
	if r.listEventsErr != nil {
		return nil, r.listEventsErr
	}
	return append([]SupportIssueEvent(nil), r.listEventsResult...), nil
}

func validCreateSupportIssueInput(mutate func(*CreateSupportIssueInput)) CreateSupportIssueInput {
	input := CreateSupportIssueInput{
		Title:              "Payment issue",
		Description:        "The payment balance did not arrive after checkout.",
		AccountEmail:       "User@Example.com",
		OccurredAt:         time.Now().UTC().Add(-time.Hour),
		ScreenshotText:     "Your account is temporarily unavailable",
		ScreenshotLanguage: SupportIssueScreenshotLanguageEN,
		Category:           SupportIssueCategoryPayment,
		Severity:           SupportIssueSeverityBlocked,
		ModelName:          "claude-sonnet",
		ClientName:         "Claude Code",
		ErrorCode:          "temporarily_unavailable",
	}
	if mutate != nil {
		mutate(&input)
	}
	return input
}

func supportIssueTestActor(userID int64, admin bool) SupportIssueActor {
	role := RoleUser
	if admin {
		role = RoleAdmin
	}
	return SupportIssueActor{
		UserID:  userID,
		Email:   fmt.Sprintf("user-%d@example.com", userID),
		Role:    role,
		IsAdmin: admin,
	}
}

func supportIssueTestInt64(v int64) *int64 {
	return &v
}

func TestSupportIssueService_SearchTextKeepsRequiredFieldsReadable(t *testing.T) {
	text := buildSupportIssueSearchText(&SupportIssue{
		Title:                  "余额未到账",
		Description:            "Payment balance is missing",
		AccountEmailMasked:     "u***@example.com",
		AccountEmailNormalized: "user@example.com",
		ScreenshotText:         "insufficient_quota",
		Category:               SupportIssueCategoryBalance,
		Severity:               SupportIssueSeverityPartial,
		ModelName:              "claude",
		ClientName:             "claude-code",
		ErrorCode:              "insufficient_quota",
		HTTPStatus:             supportIssueTestInt(429),
	})

	for _, want := range []string{
		"余额未到账",
		"Payment balance is missing",
		"u***@example.com",
		"user@example.com",
		"insufficient_quota",
		SupportIssueCategoryBalance,
		SupportIssueSeverityPartial,
		"claude",
		"claude-code",
		"429",
	} {
		require.True(t, strings.Contains(text, want), "search_text missing %q in %q", want, text)
	}
}

func supportIssueTestInt(v int) *int {
	return &v
}

func supportIssueTestPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89,
	}
}

func supportIssueTestUnboundAttachment(id int64, userID int64) SupportIssueAttachment {
	return SupportIssueAttachment{
		ID:               id,
		UploadedByUserID: &userID,
		FilePath:         "data/support-issue-attachments/test.png",
		FileURL:          fmt.Sprintf("/api/v1/issues/attachments/%d/file", id),
		FileName:         "test.png",
		MimeType:         "image/png",
		SizeBytes:        int64(len(supportIssueTestPNG())),
		Visibility:       SupportIssueAttachmentVisibilityPublic,
	}
}
