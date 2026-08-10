package handler

import (
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/ent"
	"clepsydra/internal/service"
)

// Project 项目管理接口
type Project struct {
	svc *service.Project
}

// NewProject 构建项目 handler
func NewProject(svc *service.Project) *Project {
	return &Project{svc: svc}
}

// projectRequest 创建 / 更新请求体
type projectRequest struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Remark string `json:"remark"`
}

// projectDTO 项目响应结构；demand_count 为关联需求数（不含已软删需求），
// 仅列表接口的查询预加载了关联，创建 / 更新响应中恒为 0，前端以列表为准
type projectDTO struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Remark      string    `json:"remark"`
	DemandCount int       `json:"demand_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// newProjectDTO 将 ent.Project 映射为响应结构
func newProjectDTO(p *ent.Project) projectDTO {
	return projectDTO{
		ID:          p.ID,
		Name:        p.Name,
		Color:       p.Color,
		Remark:      p.Remark,
		DemandCount: len(p.Edges.Demands),
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// List GET /api/projects
func (h *Project) List(c echo.Context) error {
	rows, err := h.svc.List(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	dtos := make([]projectDTO, 0, len(rows))
	for _, p := range rows {
		dtos = append(dtos, newProjectDTO(p))
	}

	return api.OK(c, dtos)
}

// Create POST /api/projects
func (h *Project) Create(c echo.Context) error {
	var req projectRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	p, err := h.svc.Create(c.Request().Context(), actor(c), req.Name, req.Color, req.Remark)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newProjectDTO(p))
}

// Update PUT /api/projects/:id
func (h *Project) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req projectRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	p, err := h.svc.Update(c.Request().Context(), actor(c), id, req.Name, req.Color, req.Remark)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newProjectDTO(p))
}

// Delete DELETE /api/projects/:id
func (h *Project) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Delete(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
