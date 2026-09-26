package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func catalogConnectionIDs(c *gin.Context) ([]int64, bool) {
	raw := c.Query("connection_ids")
	ids := []int64{}
	if raw == "" {
		return ids, true
	}
	for _, v := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 || len(ids) >= 200 {
			response.BadRequest(c, "Invalid connection IDs")
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}
func (h *UpstreamConnectionHandler) ListGroupCatalog(c *gin.Context) {
	ids, ok := catalogConnectionIDs(c)
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	p := service.UpstreamGroupCatalogParams{Page: page, PageSize: size, ConnectionIDs: ids, Search: c.Query("search"), Provider: c.Query("provider"), Tag: c.Query("tag"), Binding: c.Query("binding"), Freshness: c.Query("freshness"), Sort: c.Query("sort"), Favorites: c.Query("favorites") == "true", GroupByConnection: c.Query("group_by_connection") == "true"}
	for key, dest := range map[string]**float64{"min_rate": &p.MinRate, "max_rate": &p.MaxRate} {
		if raw := c.Query(key); raw != "" {
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				response.BadRequest(c, "Invalid multiplier")
				return
			}
			*dest = &v
		}
	}
	result, err := h.service.ListGroupCatalog(c.Request.Context(), p)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *UpstreamConnectionHandler) UpdateGroupAnnotations(c *gin.Context) {
	var request service.UpstreamGroupAnnotationUpdate
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid annotation request")
		return
	}
	if err := h.service.UpdateGroupAnnotations(c.Request.Context(), request); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}
func (h *UpstreamConnectionHandler) GetCostHistory(c *gin.Context) {
	ids, ok := catalogConnectionIDs(c)
	if !ok {
		return
	}
	result, err := h.service.GetCostHistory(c.Request.Context(), ids, c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UpstreamConnectionHandler) GetGroupModels(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid connection ID")
		return
	}
	result, err := h.service.GetGroupModels(c.Request.Context(), service.UpstreamGroupReference{ConnectionID: id, RemoteKey: c.Query("remote_key")})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *UpstreamConnectionHandler) SyncGroupModels(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid connection ID")
		return
	}
	var request struct {
		RemoteKey string `json:"remote_key"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid model sync request")
		return
	}
	result, err := h.service.SyncGroupModels(c.Request.Context(), service.UpstreamGroupReference{ConnectionID: id, RemoteKey: request.RemoteKey}, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
