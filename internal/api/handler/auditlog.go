package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// AuditLog 审计日志接口
type AuditLog struct {
	audit *service.Audit
}

// NewAuditLog 构建审计日志 handler
func NewAuditLog(audit *service.Audit) *AuditLog {
	return &AuditLog{audit: audit}
}

// List GET /api/audit-logs?target_type=&target_id=&page=&size=
func (h *AuditLog) List(c echo.Context) error {
	targetType := c.QueryParam("target_type")
	targetID, _ := strconv.Atoi(c.QueryParam("target_id"))
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("size"))

	total, rows, err := h.audit.List(c.Request().Context(), targetType, targetID, page, size)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]any{"total": total, "rows": rows})
}
