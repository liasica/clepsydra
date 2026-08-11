package handler

import (
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/ent"
	"clepsydra/internal/service"
)

// Tag 标签管理接口
type Tag struct {
	svc *service.Tag
}

// NewTag 构建标签 handler
func NewTag(svc *service.Tag) *Tag {
	return &Tag{svc: svc}
}

// tagRequest 创建 / 更新请求体，仅名称：颜色由服务端按名称生成并固化，不接受外部传入
type tagRequest struct {
	Name string `json:"name"`
}

// tagDTO 标签响应结构；demand_count 为关联需求数（不含已软删需求），
// 仅列表接口的查询预加载了关联，创建 / 更新响应中恒为 0，前端以列表为准
type tagDTO struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	DemandCount int       `json:"demand_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// newTagDTO 将 ent.Tag 映射为响应结构
func newTagDTO(t *ent.Tag) tagDTO {
	return tagDTO{
		ID:          t.ID,
		Name:        t.Name,
		Color:       t.Color,
		DemandCount: len(t.Edges.Demands),
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// List GET /api/tags
func (h *Tag) List(c echo.Context) error {
	rows, err := h.svc.List(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	dtos := make([]tagDTO, 0, len(rows))
	for _, t := range rows {
		dtos = append(dtos, newTagDTO(t))
	}

	return api.OK(c, dtos)
}

// Create POST /api/tags
func (h *Tag) Create(c echo.Context) error {
	var req tagRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	t, err := h.svc.Create(c.Request().Context(), actor(c), req.Name)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newTagDTO(t))
}

// Update PUT /api/tags/:id
func (h *Tag) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	var req tagRequest
	if err = c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	t, err := h.svc.Update(c.Request().Context(), actor(c), id, req.Name)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, newTagDTO(t))
}

// Delete DELETE /api/tags/:id
func (h *Tag) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return api.Fail(c, err)
	}

	if err = h.svc.Delete(c.Request().Context(), actor(c), id); err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
