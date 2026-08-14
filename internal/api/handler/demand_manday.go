package handler

import (
	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// demandHalfDaysRequest 人天调整请求体，指针字段区分「未提供」与显式 0
type demandHalfDaysRequest struct {
	EstimatedHalfDays *int `json:"estimated_half_days"`
	ActualHalfDays    *int `json:"actual_half_days"`
}

// UpdateHalfDays PUT /api/demands/:id/half-days
// 超管任意状态修正人天：预估任意状态可改，实际人天仅完成后可改，联动未确认账单
func (h *Demand) UpdateHalfDays(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandHalfDaysRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.UpdateHalfDays(c.Request().Context(), actor(c), id, service.DemandHalfDaysPatch{
		EstimatedHalfDays: req.EstimatedHalfDays,
		ActualHalfDays:    req.ActualHalfDays,
	})
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// MandayHistory GET /api/demands/:id/manday-history
// 人天调整历史，登录即可查看，需求方以此追溯超管修正记录
func (h *Demand) MandayHistory(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	rows, err := h.svc.MandayHistory(c.Request().Context(), id)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, rows)
}
