package handler

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/ent"
	"clepsydra/internal/service"
)

// Demand 需求接口
type Demand struct {
	svc *service.Demand
}

// NewDemand 构建需求 handler
func NewDemand(svc *service.Demand) *Demand {
	return &Demand{svc: svc}
}

// demandRequest 创建与更新共用的请求体
type demandRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// estimateRequest 提交人天确认请求体
type estimateRequest struct {
	EstimatedHalfDays int    `json:"estimated_half_days"`
	PlannedStartDate  string `json:"planned_start_date"`
}

// startRequest 开工请求体
type startRequest struct {
	ActualStartDate string `json:"actual_start_date"`
}

// finishRequest 完成请求体
type finishRequest struct {
	ActualStartDate string `json:"actual_start_date"`
	ActualEndDate   string `json:"actual_end_date"`
	ActualHalfDays  int    `json:"actual_half_days"`
}

// parseID 解析路径中的需求 ID
func parseID(c echo.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, service.ErrBadRequest("ID 不合法")
	}

	return id, nil
}

// parseDate 解析 YYYY-MM-DD 日期，空串返回 nil
func parseDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}

	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, service.ErrBadRequest("日期格式必须为 YYYY-MM-DD")
	}

	return &d, nil
}

// actor 从登录态组装操作者
func actor(c echo.Context) service.Actor {
	claims := api.Claims(c)
	return service.Actor{ID: claims.UserID, Name: claims.Name}
}

// List GET /api/demands?status=
func (h *Demand) List(c echo.Context) error {
	status := c.QueryParam("status")

	demands, err := h.svc.List(c.Request().Context(), status)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, demands)
}

// Get GET /api/demands/:id
func (h *Demand) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var d *ent.Demand
	d, err = h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// Create POST /api/demands
func (h *Demand) Create(c echo.Context) error {
	var req demandRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// Update PUT /api/demands/:id
func (h *Demand) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	var d *ent.Demand
	d, err = h.svc.Update(c.Request().Context(), actor(c), id, req.Title, req.Description)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// Delete DELETE /api/demands/:id
func (h *Demand) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Delete(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// SubmitEstimate POST /api/demands/:id/submit-estimate
func (h *Demand) SubmitEstimate(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req estimateRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	var planned *time.Time
	planned, err = parseDate(req.PlannedStartDate)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.SubmitEstimate(c.Request().Context(), actor(c), id, req.EstimatedHalfDays, planned); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// ConfirmEstimate POST /api/demands/:id/confirm-estimate
func (h *Demand) ConfirmEstimate(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.ConfirmEstimate(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Start POST /api/demands/:id/start
func (h *Demand) Start(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req startRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	var actualStart *time.Time
	actualStart, err = parseDate(req.ActualStartDate)
	if err != nil {
		return api.Fail(c, err)
	}
	if actualStart == nil {
		return api.Fail(c, service.ErrBadRequest("开工日期不能为空"))
	}

	err = h.svc.Start(
		c.Request().Context(),
		actor(c),
		id,
		*actualStart,
	)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Finish POST /api/demands/:id/finish
func (h *Demand) Finish(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req finishRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	var actualStart *time.Time
	actualStart, err = parseDate(req.ActualStartDate)
	if err != nil {
		return api.Fail(c, err)
	}
	if actualStart == nil {
		return api.Fail(c, service.ErrBadRequest("日期不能为空"))
	}

	var actualEnd *time.Time
	actualEnd, err = parseDate(req.ActualEndDate)
	if err != nil {
		return api.Fail(c, err)
	}
	if actualEnd == nil {
		return api.Fail(c, service.ErrBadRequest("日期不能为空"))
	}

	err = h.svc.Finish(
		c.Request().Context(),
		actor(c),
		id,
		*actualStart,
		*actualEnd,
		req.ActualHalfDays,
	)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}

// Accept POST /api/demands/:id/accept
// 路由层仅要求登录态，人工确认固定传 auto=false, locked=false
func (h *Demand) Accept(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	err = h.svc.Accept(
		c.Request().Context(),
		actor(c),
		id,
		false,
		false,
	)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
