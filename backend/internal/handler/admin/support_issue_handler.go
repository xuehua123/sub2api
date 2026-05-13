package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type supportIssueAdminService interface {
	AdminList(ctx context.Context, params pagination.PaginationParams, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error)
	AdminSearch(ctx context.Context, params pagination.PaginationParams, rawQuery string, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error)
	AdminGet(ctx context.Context, issueID int64) (*service.SupportIssue, error)
	AdminUpdateStatus(ctx context.Context, actor service.SupportIssueActor, issueID int64, nextStatus string, reason string) (*service.SupportIssue, error)
	AdminReopen(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error)
	AdminHideIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error)
	AdminRestoreIssue(ctx context.Context, actor service.SupportIssueActor, issueID int64, reason string) (*service.SupportIssue, error)
	AdminHideComment(ctx context.Context, actor service.SupportIssueActor, issueID int64, commentID int64, reason string) error
	AdminHideAttachment(ctx context.Context, actor service.SupportIssueActor, issueID int64, attachmentID int64, reason string) error
	AdminListEvents(ctx context.Context, issueID int64) ([]service.SupportIssueEvent, error)
}

type SupportIssueHandler struct {
	supportIssueService supportIssueAdminService
}

func NewSupportIssueHandler(supportIssueService *service.SupportIssueService) *SupportIssueHandler {
	return &SupportIssueHandler{supportIssueService: supportIssueService}
}

func (h *SupportIssueHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := adminSupportIssuePaginationParams(c, page, pageSize)
	filters, ok := adminSupportIssueListFiltersFromQuery(c)
	if !ok {
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	var (
		items      []service.SupportIssue
		pageResult *pagination.PaginationResult
		err        error
	)
	if q != "" {
		items, pageResult, err = h.supportIssueService.AdminSearch(c.Request.Context(), params, q, filters)
	} else {
		items, pageResult, err = h.supportIssueService.AdminList(c.Request.Context(), params, filters)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, dto.AdminSupportIssuesFromService(items), adminSupportIssuePageTotal(pageResult), page, pageSize)
}

func (h *SupportIssueHandler) Get(c *gin.Context) {
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}

	issue, err := h.supportIssueService.AdminGet(c.Request.Context(), issueID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) UpdateStatus(c *gin.Context) {
	actor, ok := adminSupportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}

	var req dto.UpdateSupportIssueStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	issue, err := h.supportIssueService.AdminUpdateStatus(c.Request.Context(), actor, issueID, req.Status, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) Reopen(c *gin.Context) {
	actor, ok := adminSupportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}

	var req dto.SupportIssueReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	issue, err := h.supportIssueService.AdminReopen(c.Request.Context(), actor, issueID, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) HideIssue(c *gin.Context) {
	actor, ok := adminSupportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}

	var req dto.SupportIssueReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	issue, err := h.supportIssueService.AdminHideIssue(c.Request.Context(), actor, issueID, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) RestoreIssue(c *gin.Context) {
	actor, ok := adminSupportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}

	var req dto.SupportIssueReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	issue, err := h.supportIssueService.AdminRestoreIssue(c.Request.Context(), actor, issueID, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AdminSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) HideComment(c *gin.Context) {
	actor, ok := adminSupportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}
	commentID, ok := adminSupportIssueIDParam(c, "comment_id")
	if !ok {
		return
	}

	var req dto.SupportIssueReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.supportIssueService.AdminHideComment(c.Request.Context(), actor, issueID, commentID, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func (h *SupportIssueHandler) HideAttachment(c *gin.Context) {
	actor, ok := adminSupportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := adminSupportIssueIDParam(c, "attachment_id")
	if !ok {
		return
	}

	var req dto.SupportIssueReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.supportIssueService.AdminHideAttachment(c.Request.Context(), actor, issueID, attachmentID, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func (h *SupportIssueHandler) Events(c *gin.Context) {
	issueID, ok := adminSupportIssueIDParam(c, "id")
	if !ok {
		return
	}

	events, err := h.supportIssueService.AdminListEvents(c.Request.Context(), issueID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.SupportIssueEventsFromService(events))
}

func adminSupportIssueActorFromContext(c *gin.Context) (service.SupportIssueActor, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return service.SupportIssueActor{}, false
	}
	return service.SupportIssueActor{
		UserID:  subject.UserID,
		Role:    service.RoleAdmin,
		IsAdmin: true,
	}, true
}

func adminSupportIssueIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid issue ID")
		return 0, false
	}
	return id, true
}

func adminSupportIssueListFiltersFromQuery(c *gin.Context) (service.ListSupportIssueFilters, bool) {
	hasImage, ok := adminSupportIssueOptionalBoolQuery(c, "has_image")
	if !ok {
		return service.ListSupportIssueFilters{}, false
	}
	hidden, ok := adminSupportIssueOptionalBoolQuery(c, "hidden")
	if !ok {
		return service.ListSupportIssueFilters{}, false
	}
	return service.ListSupportIssueFilters{
		Status:        strings.TrimSpace(c.Query("status")),
		Category:      strings.TrimSpace(c.Query("category")),
		Severity:      strings.TrimSpace(c.Query("severity")),
		HasImage:      hasImage,
		Hidden:        hidden,
		IncludeHidden: true,
	}, true
}

func adminSupportIssuePaginationParams(c *gin.Context, page int, pageSize int) pagination.PaginationParams {
	return pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    strings.TrimSpace(c.Query("sort_by")),
		SortOrder: strings.TrimSpace(c.Query("sort_order")),
	}
}

func adminSupportIssueOptionalBoolQuery(c *gin.Context, key string) (*bool, bool) {
	raw := strings.TrimSpace(strings.ToLower(c.Query(key)))
	if raw == "" {
		return nil, true
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		v := true
		return &v, true
	case "0", "false", "no", "n", "off":
		v := false
		return &v, true
	default:
		response.BadRequest(c, "Invalid "+key)
		return nil, false
	}
}

func adminSupportIssuePageTotal(result *pagination.PaginationResult) int64 {
	if result == nil {
		return 0
	}
	return result.Total
}
