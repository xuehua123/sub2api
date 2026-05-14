//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupportIssueAdminDTOIncludesPrivateFields(t *testing.T) {
	issue := adminSupportIssueHandlerFixture()

	payload, err := json.Marshal(dto.AdminSupportIssueFromService(issue))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, "user@example.com", got["account_email"])
	require.Equal(t, "user@example.com", got["account_email_normalized"])
	require.Equal(t, "ab12cd", got["api_key_suffix"])
	require.Contains(t, got, "created_by_user_id")
	require.Contains(t, got, "resolved_by_user_id")
	require.Equal(t, float64(1), got["hidden_comment_count"])
	require.Contains(t, got, "events")

	attachments := got["attachments"].([]any)
	attachment := attachments[0].(map[string]any)
	require.Equal(t, "data/private.png", attachment["file_path"])
	require.Contains(t, attachment, "uploaded_by_user_id")
	require.Contains(t, attachment, "hidden_by_user_id")

	comments := got["comments"].([]any)
	comment := comments[0].(map[string]any)
	require.Contains(t, comment, "author_user_id")
	require.Contains(t, comment, "hidden_by_user_id")
	require.Equal(t, "private", comment["hide_reason"])
}

func TestSupportIssueAdminHandler_UpdateStatusCallsService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueAdminService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performAdminSupportIssueHandlerRequest(http.MethodPatch, "/admin/issues/10/status", "/admin/issues/:id/status", h.UpdateStatus, `{"status":"in_progress","reason":"investigating"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.updateStatusCalled)
	require.Equal(t, int64(10), fake.lastIssueID)
	require.Equal(t, service.SupportIssueStatusInProgress, fake.lastStatus)
	require.Equal(t, "investigating", fake.lastReason)
	require.True(t, fake.lastActor.IsAdmin)
}

func TestSupportIssueAdminHandler_ReopenCallsService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueAdminService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performAdminSupportIssueHandlerRequest(http.MethodPost, "/admin/issues/10/reopen", "/admin/issues/:id/reopen", h.Reopen, `{"reason":"retry"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.reopenCalled)
	require.Equal(t, int64(10), fake.lastIssueID)
	require.Equal(t, "retry", fake.lastReason)
}

func TestSupportIssueAdminHandler_HideCommentCallsService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueAdminService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performAdminSupportIssueHandlerRequest(http.MethodPost, "/admin/issues/10/comments/20/hide", "/admin/issues/:id/comments/:comment_id/hide", h.HideComment, `{"reason":"secret"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.hideCommentCalled)
	require.Equal(t, int64(10), fake.lastIssueID)
	require.Equal(t, int64(20), fake.lastCommentID)
	require.Equal(t, "secret", fake.lastReason)
}

func TestSupportIssueAdminHandler_HideAttachmentCallsService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueAdminService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performAdminSupportIssueHandlerRequest(http.MethodPost, "/admin/issues/10/attachments/30/hide", "/admin/issues/:id/attachments/:attachment_id/hide", h.HideAttachment, `{"reason":"secret"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.hideAttachmentCalled)
	require.Equal(t, int64(10), fake.lastIssueID)
	require.Equal(t, int64(30), fake.lastAttachmentID)
	require.Equal(t, "secret", fake.lastReason)
}

func TestSupportIssueAdminHandler_EventsCallsService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueAdminService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performAdminSupportIssueHandlerRequest(http.MethodGet, "/admin/issues/10/events", "/admin/issues/:id/events", h.Events, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.eventsCalled)
	require.Equal(t, int64(10), fake.lastIssueID)
}

func TestSupportIssueAdminHandler_ListPendingStatusMapsToActionableStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueAdminService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performAdminSupportIssueHandlerRequest(http.MethodGet, "/admin/issues?status=pending", "/admin/issues", h.List, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, fake.lastFilters.Status)
	require.Equal(t, []string{
		service.SupportIssueStatusOpen,
		service.SupportIssueStatusNeedsInfo,
		service.SupportIssueStatusInProgress,
	}, fake.lastFilters.Statuses)
}

type fakeSupportIssueAdminService struct {
	updateStatusCalled   bool
	reopenCalled         bool
	hideIssueCalled      bool
	restoreIssueCalled   bool
	pinIssueCalled       bool
	unpinIssueCalled     bool
	markSolutionCalled   bool
	clearSolutionCalled  bool
	setRelatedCalled     bool
	clearRelatedCalled   bool
	hideCommentCalled    bool
	hideAttachmentCalled bool
	eventsCalled         bool

	lastActor        service.SupportIssueActor
	lastIssueID      int64
	lastCommentID    int64
	lastAttachmentID int64
	lastRelatedID    int64
	lastStatus       string
	lastReason       string
	lastFilters      service.ListSupportIssueFilters
}

func (f *fakeSupportIssueAdminService) AdminList(ctx context.Context, params pagination.PaginationParams, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error) {
	f.lastFilters = filters
	return []service.SupportIssue{*adminSupportIssueHandlerFixture()}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 20, Pages: 1}, nil
}

func (f *fakeSupportIssueAdminService) AdminSearch(ctx context.Context, params pagination.PaginationParams, rawQuery string, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error) {
	f.lastFilters = filters
	return []service.SupportIssue{*adminSupportIssueHandlerFixture()}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 20, Pages: 1}, nil
}

func (f *fakeSupportIssueAdminService) AdminGet(ctx context.Context, issueID int64) (*service.SupportIssue, error) {
	return adminSupportIssueHandlerFixture(), nil
}

func (f *fakeSupportIssueAdminService) AdminUpdateStatus(ctx context.Context, actor service.SupportIssueActor, issueID int64, nextStatus string, reason string) (*service.SupportIssue, error) {
	f.updateStatusCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastStatus = nextStatus
	f.lastReason = reason
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	issue.Status = nextStatus
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminReopen(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error) {
	f.reopenCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastReason = reason
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	issue.Status = service.SupportIssueStatusOpen
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminHideIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error) {
	f.hideIssueCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastReason = reason
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	now := time.Now()
	issue.HiddenAt = &now
	issue.HideReason = reason
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminRestoreIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error) {
	f.restoreIssueCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastReason = reason
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminPinIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error) {
	f.pinIssueCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastReason = reason
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	now := time.Now()
	issue.PinnedAt = &now
	issue.PinnedReason = reason
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminUnpinIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64) (*service.SupportIssue, error) {
	f.unpinIssueCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminMarkSolution(ctx context.Context, actor service.SupportIssueActor, issueID int64, commentID int64) (*service.SupportIssue, error) {
	f.markSolutionCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastCommentID = commentID
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	issue.SolutionCommentID = &commentID
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminClearSolution(ctx context.Context, actor service.SupportIssueActor, issueID int64) (*service.SupportIssue, error) {
	f.clearSolutionCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminSetRelatedIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64, relatedIssueID int64, reason string) (*service.SupportIssue, error) {
	f.setRelatedCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastRelatedID = relatedIssueID
	f.lastReason = reason
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	issue.RelatedIssueID = &relatedIssueID
	issue.RelatedIssueReason = reason
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminClearRelatedIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64) (*service.SupportIssue, error) {
	f.clearRelatedCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	issue := adminSupportIssueHandlerFixture()
	issue.ID = issueID
	return issue, nil
}

func (f *fakeSupportIssueAdminService) AdminHideComment(ctx context.Context, actor service.SupportIssueActor, issueID int64, commentID int64, reason string) error {
	f.hideCommentCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastCommentID = commentID
	f.lastReason = reason
	return nil
}

func (f *fakeSupportIssueAdminService) AdminHideAttachment(ctx context.Context, actor service.SupportIssueActor, issueID int64, attachmentID int64, reason string) error {
	f.hideAttachmentCalled = true
	f.lastActor = actor
	f.lastIssueID = issueID
	f.lastAttachmentID = attachmentID
	f.lastReason = reason
	return nil
}

func (f *fakeSupportIssueAdminService) AdminListEvents(ctx context.Context, issueID int64) ([]service.SupportIssueEvent, error) {
	f.eventsCalled = true
	f.lastIssueID = issueID
	return adminSupportIssueHandlerFixture().Events, nil
}

func performAdminSupportIssueHandlerRequest(method, target, routePath string, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	r := gin.New()
	r.Handle(method, routePath, func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})
		c.Set(string(middleware2.ContextKeyUserRole), service.RoleAdmin)
		handler(c)
	})
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func adminSupportIssueHandlerFixture() *service.SupportIssue {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	userID := int64(42)
	adminID := int64(1)
	httpStatus := 429
	from := service.SupportIssueStatusOpen
	to := service.SupportIssueStatusResolved
	return &service.SupportIssue{
		ID:                     10,
		PublicID:               "ISS-000010",
		Title:                  "Payment issue",
		Description:            "Balance did not arrive.",
		AccountEmail:           "user@example.com",
		AccountEmailNormalized: "user@example.com",
		AccountEmailMasked:     "u***@example.com",
		OccurredAt:             now,
		ScreenshotText:         "rate limit",
		ScreenshotLanguage:     service.SupportIssueScreenshotLanguageEN,
		Category:               service.SupportIssueCategoryPayment,
		Severity:               service.SupportIssueSeverityBlocked,
		Status:                 service.SupportIssueStatusOpen,
		ModelName:              "claude",
		ClientName:             "claude-code",
		HTTPStatus:             &httpStatus,
		ErrorCode:              "rate_limit",
		APIKeySuffix:           "ab12cd",
		CreatedByUserID:        &userID,
		ResolvedByUserID:       &adminID,
		CommentCount:           1,
		HiddenCommentCount:     1,
		AttachmentCount:        1,
		CreatedAt:              now,
		UpdatedAt:              now,
		Comments: []service.SupportIssueComment{{
			ID:             30,
			IssueID:        10,
			AuthorUserID:   &userID,
			AuthorRole:     service.RoleUser,
			Content:        "comment",
			HiddenByUserID: &adminID,
			HideReason:     "private",
			CreatedAt:      now,
			UpdatedAt:      now,
		}},
		Attachments: []service.SupportIssueAttachment{{
			ID:               40,
			IssueID:          10,
			UploadedByUserID: &userID,
			FilePath:         "data/private.png",
			FileURL:          "/uploads/public.png",
			FileName:         "public.png",
			MimeType:         "image/png",
			SizeBytes:        100,
			Visibility:       service.SupportIssueAttachmentVisibilityPublic,
			HiddenByUserID:   &adminID,
			CreatedAt:        now,
		}},
		Events: []service.SupportIssueEvent{{
			ID:          50,
			IssueID:     10,
			ActorUserID: &adminID,
			EventType:   service.SupportIssueEventStatusChanged,
			FromStatus:  &from,
			ToStatus:    &to,
			CreatedAt:   now,
		}},
	}
}
