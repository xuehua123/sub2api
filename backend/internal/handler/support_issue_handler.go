package handler

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

type supportIssueUserService interface {
	Create(ctx context.Context, actor service.SupportIssueActor, input service.CreateSupportIssueInput) (*service.SupportIssue, error)
	ListPublic(ctx context.Context, params pagination.PaginationParams, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error)
	SearchPublic(ctx context.Context, params pagination.PaginationParams, rawQuery string, filters service.ListSupportIssueFilters) ([]service.SupportIssue, *pagination.PaginationResult, error)
	GetPublic(ctx context.Context, issueID int64) (*service.SupportIssue, error)
	AddComment(ctx context.Context, actor service.SupportIssueActor, issueID int64, content string) (*service.SupportIssueComment, error)
	Resolve(ctx context.Context, actor service.SupportIssueActor, issueID int64) (*service.SupportIssue, error)
	SuggestSimilar(ctx context.Context, actor service.SupportIssueActor, input service.CreateSupportIssueInput) ([]service.SupportIssue, error)
}

type SupportIssueHandler struct {
	supportIssueService supportIssueUserService
}

func NewSupportIssueHandler(supportIssueService *service.SupportIssueService) *SupportIssueHandler {
	return &SupportIssueHandler{supportIssueService: supportIssueService}
}

func (h *SupportIssueHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := supportIssuePaginationParams(c, page, pageSize)
	filters, ok := supportIssueListFiltersFromQuery(c)
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
		items, pageResult, err = h.supportIssueService.SearchPublic(c.Request.Context(), params, q, filters)
	} else {
		items, pageResult, err = h.supportIssueService.ListPublic(c.Request.Context(), params, filters)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Paginated(c, dto.PublicSupportIssuesFromService(items), supportIssuePageTotal(pageResult), page, pageSize)
}

func (h *SupportIssueHandler) Get(c *gin.Context) {
	issueID, ok := supportIssueIDParam(c, "id")
	if !ok {
		return
	}

	issue, err := h.supportIssueService.GetPublic(c.Request.Context(), issueID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PublicSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) Create(c *gin.Context) {
	actor, ok := supportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req dto.CreateSupportIssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	issue, err := h.supportIssueService.Create(c.Request.Context(), actor, req.ToServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PublicSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) AddComment(c *gin.Context) {
	actor, ok := supportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := supportIssueIDParam(c, "id")
	if !ok {
		return
	}

	var req dto.AddSupportIssueCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	comment, err := h.supportIssueService.AddComment(c.Request.Context(), actor, issueID, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PublicSupportIssueCommentFromService(comment))
}

func (h *SupportIssueHandler) Resolve(c *gin.Context) {
	actor, ok := supportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	issueID, ok := supportIssueIDParam(c, "id")
	if !ok {
		return
	}

	issue, err := h.supportIssueService.Resolve(c.Request.Context(), actor, issueID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PublicSupportIssueFromService(issue))
}

func (h *SupportIssueHandler) SearchSuggestions(c *gin.Context) {
	actor, ok := supportIssueActorFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req dto.CreateSupportIssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	items, err := h.supportIssueService.SuggestSimilar(c.Request.Context(), actor, req.ToServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PublicSupportIssuesFromService(items))
}

func supportIssueActorFromContext(c *gin.Context) (service.SupportIssueActor, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return service.SupportIssueActor{}, false
	}
	role, _ := middleware2.GetUserRoleFromContext(c)
	return service.SupportIssueActor{
		UserID:  subject.UserID,
		Role:    role,
		IsAdmin: role == service.RoleAdmin,
	}, true
}

func supportIssueIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid issue ID")
		return 0, false
	}
	return id, true
}

func supportIssueListFiltersFromQuery(c *gin.Context) (service.ListSupportIssueFilters, bool) {
	hasImage, ok := supportIssueOptionalBoolQuery(c, "has_image")
	if !ok {
		return service.ListSupportIssueFilters{}, false
	}
	return service.ListSupportIssueFilters{
		Status:   strings.TrimSpace(c.Query("status")),
		Category: strings.TrimSpace(c.Query("category")),
		Severity: strings.TrimSpace(c.Query("severity")),
		HasImage: hasImage,
	}, true
}

func supportIssuePaginationParams(c *gin.Context, page int, pageSize int) pagination.PaginationParams {
	return pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    strings.TrimSpace(c.Query("sort_by")),
		SortOrder: strings.TrimSpace(c.Query("sort_order")),
	}
}

func supportIssueOptionalBoolQuery(c *gin.Context, key string) (*bool, bool) {
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

func supportIssuePageTotal(result *pagination.PaginationResult) int64 {
	if result == nil {
		return 0
	}
	return result.Total
}
