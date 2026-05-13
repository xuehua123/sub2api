//go:build unit

package handler

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

func TestSupportIssuePublicDTOExcludesPrivateFields(t *testing.T) {
	issue := supportIssueHandlerFixture()

	payload, err := json.Marshal(dto.PublicSupportIssueFromService(issue))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.NotContains(t, got, "account_email")
	require.NotContains(t, got, "account_email_normalized")
	require.NotContains(t, got, "api_key_suffix")
	require.NotContains(t, got, "created_by_user_id")
	require.NotContains(t, got, "resolved_by_user_id")
	require.NotContains(t, got, "hidden_comment_count")
	require.NotContains(t, got, "events")

	attachments := got["attachments"].([]any)
	attachment := attachments[0].(map[string]any)
	require.NotContains(t, attachment, "file_path")
	require.NotContains(t, attachment, "uploaded_by_user_id")
	require.NotContains(t, attachment, "hidden_by_user_id")

	comments := got["comments"].([]any)
	comment := comments[0].(map[string]any)
	require.NotContains(t, comment, "author_user_id")
	require.NotContains(t, comment, "hidden_by_user_id")
	require.NotContains(t, comment, "hide_reason")
}

func TestSupportIssueHandler_ListUsesSearchWhenQueryPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues?q=rate&status=open", "/issues", h.List, false, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.searchPublicCalled)
	require.False(t, fake.listPublicCalled)
	require.Equal(t, "rate", fake.lastRawQuery)
	require.Equal(t, service.SupportIssueStatusOpen, fake.lastFilters.Status)
}

func TestSupportIssueHandler_ListUsesListWhenQueryEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues?category=payment", "/issues", h.List, false, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.listPublicCalled)
	require.False(t, fake.searchPublicCalled)
	require.Equal(t, service.SupportIssueCategoryPayment, fake.lastFilters.Category)
}

func TestSupportIssueHandler_CreateRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SupportIssueHandler{supportIssueService: &fakeSupportIssueUserService{}}

	w := performSupportIssueHandlerRequest(http.MethodPost, "/issues", "/issues", h.Create, false, `{}`)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportIssueHandler_AddCommentRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SupportIssueHandler{supportIssueService: &fakeSupportIssueUserService{}}

	w := performSupportIssueHandlerRequest(http.MethodPost, "/issues/123/comments", "/issues/:id/comments", h.AddComment, false, `{"content":"hello"}`)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportIssueHandler_ResolveRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SupportIssueHandler{supportIssueService: &fakeSupportIssueUserService{}}

	w := performSupportIssueHandlerRequest(http.MethodPatch, "/issues/123/resolve", "/issues/:id/resolve", h.Resolve, false, "")

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

type fakeSupportIssueUserService struct {
	listPublicCalled   bool
	searchPublicCalled bool
	lastRawQuery       string
	lastParams         pagination.PaginationParams
	lastFilters        service.ListSupportIssueFilters

	createCalled bool
	createdActor service.SupportIssueActor
	createdInput service.CreateSupportIssueInput

	addCommentCalled bool
	resolveCalled    bool
}

func (f *fakeSupportIssueUserService) Create(ctx context.Context, actor service.SupportIssueActor, input service.CreateSupportIssueInput) (*service.SupportIssue, error) {
	f.createCalled = true
	f.createdActor = actor
	f.createdInput = input
	return supportIssueHandlerFixture(), nil
}

func (f *fakeSupportIssueUserService) ListPublic(ctx context.Context, params pagination.PaginationParams, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error) {
	f.listPublicCalled = true
	f.lastParams = params
	f.lastFilters = filters
	return []service.SupportIssue{*supportIssueHandlerFixture()}, supportIssueHandlerPage(), nil
}

func (f *fakeSupportIssueUserService) SearchPublic(ctx context.Context, params pagination.PaginationParams, rawQuery string, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error) {
	f.searchPublicCalled = true
	f.lastParams = params
	f.lastRawQuery = rawQuery
	f.lastFilters = filters
	return []service.SupportIssue{*supportIssueHandlerFixture()}, supportIssueHandlerPage(), nil
}

func (f *fakeSupportIssueUserService) GetPublic(ctx context.Context, issueID int64) (*service.SupportIssue, error) {
	return supportIssueHandlerFixture(), nil
}

func (f *fakeSupportIssueUserService) AddComment(ctx context.Context, actor service.SupportIssueActor, issueID int64, content string) (*service.SupportIssueComment, error) {
	f.addCommentCalled = true
	return &service.SupportIssueComment{ID: 20, IssueID: issueID, AuthorRole: service.RoleUser, Content: content, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (f *fakeSupportIssueUserService) Resolve(ctx context.Context, actor service.SupportIssueActor, issueID int64) (*service.SupportIssue, error) {
	f.resolveCalled = true
	issue := supportIssueHandlerFixture()
	issue.ID = issueID
	issue.Status = service.SupportIssueStatusResolved
	return issue, nil
}

func (f *fakeSupportIssueUserService) SuggestSimilar(ctx context.Context, actor service.SupportIssueActor, input service.CreateSupportIssueInput) ([]service.SupportIssue, error) {
	return []service.SupportIssue{*supportIssueHandlerFixture()}, nil
}

func performSupportIssueHandlerRequest(method, target, routePath string, handler gin.HandlerFunc, authenticated bool, body string) *httptest.ResponseRecorder {
	r := gin.New()
	r.Handle(method, routePath, func(c *gin.Context) {
		if authenticated {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
			c.Set(string(middleware2.ContextKeyUserRole), service.RoleUser)
		}
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

func supportIssueHandlerFixture() *service.SupportIssue {
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

func supportIssueHandlerPage() *pagination.PaginationResult {
	return &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 20, Pages: 1}
}
