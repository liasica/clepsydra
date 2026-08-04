package handler

import (
	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
	"clepsydra/internal/workday"
)

// Setting 设置相关接口
type Setting struct {
	settingSvc  *service.Setting
	holidaySvc  *service.HolidaySvc
}

// NewSetting 构建设置 handler
func NewSetting(settingSvc *service.Setting, holidaySvc *service.HolidaySvc) *Setting {
	return &Setting{
		settingSvc:  settingSvc,
		holidaySvc:  holidaySvc,
	}
}

// All GET /api/settings
func (h *Setting) All(c echo.Context) error {
	values, err := h.settingSvc.All(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, values)
}

// Update PUT /api/settings
func (h *Setting) Update(c echo.Context) error {
	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	err := h.settingSvc.Update(c.Request().Context(), req.Values)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Holidays GET /api/holidays?year=2026
func (h *Setting) Holidays(c echo.Context) error {
	year := c.QueryParam("year")

	holidays, err := h.holidaySvc.List(c.Request().Context(), year)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, holidays)
}

// SaveHolidays PUT /api/holidays
func (h *Setting) SaveHolidays(c echo.Context) error {
	var req struct {
		Entries []workday.Entry `json:"entries"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	err := h.holidaySvc.Save(c.Request().Context(), req.Entries)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// DeleteHoliday DELETE /api/holidays/:date
func (h *Setting) DeleteHoliday(c echo.Context) error {
	date := c.Param("date")

	err := h.holidaySvc.Delete(c.Request().Context(), date)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
