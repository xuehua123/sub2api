package repository

import "github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

func paginationResultFrom(total int, params pagination.PaginationParams) *pagination.PaginationResult {
	limit := params.Limit()
	pages := 1
	if total > 0 && limit > 0 {
		pages = (total + limit - 1) / limit
	}
	return &pagination.PaginationResult{
		Total:    int64(total),
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    pages,
	}
}

func uniqueInt64(items []int64) []int64 {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(items))
	result := make([]int64, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
