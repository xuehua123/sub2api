package admin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func parsePositiveInt64Query(c *gin.Context, name string) (int64, bool, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false, fmt.Errorf("invalid %s", name)
	}
	return value, true, nil
}
