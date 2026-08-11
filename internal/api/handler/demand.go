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

// demandRequest 更新请求体，仅标题与描述
type demandRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// demandCreateRequest 创建请求体；预估相关三个字段是超管专属的可选快捷路径
type demandCreateRequest struct {
	Title             string `json:"title"`
	Description       string `json:"description"`
	EstimatedHalfDays int    `json:"estimated_half_days"`
	PlannedStartDate  string `json:"planned_start_date"`
	Confirmed         bool   `json:"confirmed"`
	ProjectIDs        []int  `json:"project_ids"`
	TagIDs            []int  `json:"tag_ids"`
}

// demandProjectsRequest 项目标签全量覆盖请求体
type demandProjectsRequest struct {
	ProjectIDs []int `json:"project_ids"`
}

// demandTagsRequest 性质标签全量覆盖请求体
type demandTagsRequest struct {
	TagIDs []int `json:"tag_ids"`
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

// List GET /api/demands?status=&project_id=&tag_id=
func (h *Demand) List(c echo.Context) error {
	status := c.QueryParam("status")
	projectID, _ := strconv.Atoi(c.QueryParam("project_id")) // 非法或缺省按 0 处理，即不筛选
	tagID, _ := strconv.Atoi(c.QueryParam("tag_id"))         // 同上

	demands, err := h.svc.List(c.Request().Context(), status, projectID, tagID)
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
	var req demandCreateRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	// 预估相关字段是超管专属快捷路径，需求方创建仍只允许标题与描述
	hasEstimate := req.EstimatedHalfDays != 0 || req.PlannedStartDate != "" || req.Confirmed
	if hasEstimate && api.Claims(c).Role != "admin" {
		return api.Fail(c, service.ErrForbidden)
	}
	// 日期与已确认都是预估的附属信息，必须依附正人天
	if (req.Confirmed || req.PlannedStartDate != "") && req.EstimatedHalfDays <= 0 {
		return api.Fail(c, service.ErrBadRequest("预估人天必须为正"))
	}

	planned, err := parseDate(req.PlannedStartDate)
	if err != nil {
		return api.Fail(c, err)
	}

	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description,
		req.EstimatedHalfDays, planned, req.Confirmed, req.ProjectIDs, req.TagIDs)
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

// UpdateProjects PUT /api/demands/:id/projects
// 任何状态可用：标签是归类元数据，不影响人天与账单金额
func (h *Demand) UpdateProjects(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandProjectsRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.UpdateProjects(c.Request().Context(), actor(c), id, req.ProjectIDs)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}

// UpdateTags PUT /api/demands/:id/tags
// 任何状态可用：标签是归类元数据，不影响人天与账单金额
func (h *Demand) UpdateTags(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req demandTagsRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	d, err := h.svc.UpdateTags(c.Request().Context(), actor(c), id, req.TagIDs)
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
