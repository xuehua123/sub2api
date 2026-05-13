//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
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

func TestSupportIssueAdminDTOStillIncludesFilePath(t *testing.T) {
	issue := supportIssueHandlerFixture()

	payload, err := json.Marshal(dto.AdminSupportIssueFromService(issue))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	attachments := got["attachments"].([]any)
	attachment := attachments[0].(map[string]any)
	require.Equal(t, "data/private.png", attachment["file_path"])
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

func TestSupportIssueHandler_MineRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SupportIssueHandler{supportIssueService: &fakeSupportIssueUserService{}}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues/mine", "/issues/mine", h.Mine, false, "")

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportIssueHandler_MineFiltersByAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues/mine?q=rate&status=open", "/issues/mine", h.Mine, true, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.searchPublicCalled)
	require.NotNil(t, fake.lastFilters.CreatedByUserID)
	require.Equal(t, int64(42), *fake.lastFilters.CreatedByUserID)
	require.Equal(t, service.SupportIssueStatusOpen, fake.lastFilters.Status)
}

func TestSupportIssueHandler_NotificationsRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SupportIssueHandler{supportIssueService: &fakeSupportIssueUserService{}}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues/notifications", "/issues/notifications", h.Notifications, false, "")

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportIssueHandler_NotificationsReturnsSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues/notifications", "/issues/notifications", h.Notifications, true, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.notificationCalled)
	require.Contains(t, w.Body.String(), `"unread_count":2`)
	require.Contains(t, w.Body.String(), `"needs_info_count":1`)
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

func TestSupportIssueHandler_UploadAttachmentRequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SupportIssueHandler{supportIssueService: &fakeSupportIssueUserService{}}

	w := performSupportIssueHandlerRequest(http.MethodPost, "/issues/attachments", "/issues/attachments", h.UploadAttachment, false, "")

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSupportIssueHandler_UploadAttachmentRejectsInvalidMIME(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{uploadErr: service.ErrSupportIssueAttachmentInvalidType}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueUploadRequest(t, h.UploadAttachment, true, "note.txt", "text/plain", []byte("plain text"))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.True(t, fake.uploadAttachmentCalled)
}

func TestSupportIssueHandler_UploadAttachmentHidesFilePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueUploadRequest(t, h.UploadAttachment, true, "../screenshot.png", "image/png", supportIssueHandlerPNG())

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.uploadAttachmentCalled)
	require.Equal(t, "screenshot.png", fake.lastUploadInput.FileName)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	data := got["data"].(map[string]any)
	require.NotContains(t, data, "file_path")
	require.NotContains(t, data, "uploaded_by_user_id")
	require.Equal(t, "safe.png", data["file_name"])
}

func TestSupportIssueHandler_CreateRejectsInlineAttachments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	body := `{
		"title":"Payment issue",
		"description":"The payment balance did not arrive after checkout.",
		"account_email":"user@example.com",
		"occurred_at":"2026-05-13T10:00:00Z",
		"screenshot_text":"rate limit error",
		"screenshot_language":"en",
		"category":"payment",
		"severity":"blocked",
		"attachments":[{"file_url":"/unsafe","file_name":"unsafe.png","mime_type":"image/png","size_bytes":1}]
	}`

	w := performSupportIssueHandlerRequest(http.MethodPost, "/issues", "/issues", h.Create, true, body)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, fake.createCalled)
}

func TestSupportIssueHandler_CreatePassesAttachmentIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{}
	h := &SupportIssueHandler{supportIssueService: fake}

	body := `{
		"title":"Payment issue",
		"description":"The payment balance did not arrive after checkout.",
		"account_email":"user@example.com",
		"occurred_at":"2026-05-13T10:00:00Z",
		"screenshot_text":"rate limit error",
		"screenshot_language":"en",
		"category":"payment",
		"severity":"blocked",
		"attachment_ids":[777]
	}`

	w := performSupportIssueHandlerRequest(http.MethodPost, "/issues", "/issues", h.Create, true, body)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, fake.createCalled)
	require.Equal(t, []int64{777}, fake.createdInput.AttachmentIDs)
}

func TestSupportIssueHandler_ServeAttachmentFileReturnsPublicFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	filePath := filepath.Join(t.TempDir(), "safe.png")
	require.NoError(t, os.WriteFile(filePath, []byte("png"), 0o600))
	fake := &fakeSupportIssueUserService{
		publicAttachment: &service.SupportIssueAttachment{
			ID:         777,
			IssueID:    123,
			FilePath:   filePath,
			FileName:   "safe.png",
			MimeType:   "image/png",
			Visibility: service.SupportIssueAttachmentVisibilityPublic,
		},
	}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues/attachments/777/file", "/issues/attachments/:id/file", h.ServeAttachmentFile, false, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png", w.Body.String())
	require.Equal(t, int64(777), fake.lastOpenAttachmentID)
}

func TestSupportIssueHandler_ServeAttachmentFileRejectsHiddenOrUnbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeSupportIssueUserService{openAttachmentErr: service.ErrSupportIssueAttachmentNotFound}
	h := &SupportIssueHandler{supportIssueService: fake}

	w := performSupportIssueHandlerRequest(http.MethodGet, "/issues/attachments/777/file", "/issues/attachments/:id/file", h.ServeAttachmentFile, false, "")

	require.Equal(t, http.StatusNotFound, w.Code)
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

	addCommentCalled   bool
	lastRelatedIssueID *int64
	resolveCalled      bool
	lastViewer         service.SupportIssueViewer
	notificationCalled bool

	uploadAttachmentCalled bool
	lastUploadInput        service.UploadSupportIssueAttachmentInput
	uploadErr              error

	publicAttachment     *service.SupportIssueAttachment
	lastOpenAttachmentID int64
	openAttachmentErr    error
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

func (f *fakeSupportIssueUserService) GetPublic(ctx context.Context, issueID int64, viewer service.SupportIssueViewer) (*service.SupportIssue, error) {
	f.lastViewer = viewer
	return supportIssueHandlerFixture(), nil
}

func (f *fakeSupportIssueUserService) NotificationSummary(ctx context.Context, actor service.SupportIssueActor) (*service.SupportIssueNotificationSummary, error) {
	f.notificationCalled = true
	return &service.SupportIssueNotificationSummary{UnreadCount: 2, NeedsInfoCount: 1}, nil
}

func (f *fakeSupportIssueUserService) AddComment(ctx context.Context, actor service.SupportIssueActor, issueID int64, content string, relatedIssueID *int64) (*service.SupportIssueComment, error) {
	f.addCommentCalled = true
	f.lastRelatedIssueID = relatedIssueID
	return &service.SupportIssueComment{ID: 20, IssueID: issueID, AuthorRole: service.RoleUser, Content: content, RelatedIssueID: relatedIssueID, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
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

func (f *fakeSupportIssueUserService) UploadAttachment(ctx context.Context, actor service.SupportIssueActor, input service.UploadSupportIssueAttachmentInput) (*service.SupportIssueAttachment, error) {
	f.uploadAttachmentCalled = true
	f.lastUploadInput = input
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	now := time.Now()
	return &service.SupportIssueAttachment{
		ID:               777,
		UploadedByUserID: &actor.UserID,
		FilePath:         "data/support-issue-attachments/private.png",
		FileURL:          "/api/v1/issues/attachments/777/file",
		FileName:         "safe.png",
		MimeType:         "image/png",
		SizeBytes:        int64(len(input.Content)),
		Visibility:       service.SupportIssueAttachmentVisibilityPublic,
		CreatedAt:        now,
	}, nil
}

func (f *fakeSupportIssueUserService) OpenAttachmentForPublic(ctx context.Context, attachmentID int64) (*service.SupportIssueAttachment, error) {
	f.lastOpenAttachmentID = attachmentID
	if f.openAttachmentErr != nil {
		return nil, f.openAttachmentErr
	}
	if f.publicAttachment == nil {
		return nil, service.ErrSupportIssueAttachmentNotFound
	}
	out := *f.publicAttachment
	return &out, nil
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

func performSupportIssueUploadRequest(t *testing.T, handler gin.HandlerFunc, authenticated bool, fileName string, contentType string, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textprotoMIMEHeader(map[string]string{
		`Content-Disposition`: `form-data; name="file"; filename="` + fileName + `"`,
		`Content-Type`:        contentType,
	}))
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	r := gin.New()
	r.POST("/issues/attachments", func(c *gin.Context) {
		if authenticated {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
			c.Set(string(middleware2.ContextKeyUserRole), service.RoleUser)
		}
		handler(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/issues/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func textprotoMIMEHeader(values map[string]string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader, len(values))
	for key, value := range values {
		header.Set(key, value)
	}
	return header
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

func supportIssueHandlerPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89,
	}
}
