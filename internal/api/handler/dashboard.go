package handler

import (
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// Dashboard 工作台接口
type Dashboard struct {
	svc *service.Dashboard
}

// NewDashboard 构建工作台 handler
func NewDashboard(svc *service.Dashboard) *Dashboard {
	return &Dashboard{svc: svc}
}

// Todos GET /api/dashboard/todos
func (h *Dashboard) Todos(c echo.Context) error {
	claims := api.Claims(c)

	todos, err := h.svc.Todos(c.Request().Context(), claims.Role, time.Now())
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, todos)
}
