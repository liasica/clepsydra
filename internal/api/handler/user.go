package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// User 用户管理接口
type User struct {
	svc *service.User
}

// NewUser 构建用户管理 handler
func NewUser(svc *service.User) *User {
	return &User{svc: svc}
}

// List GET /api/users
func (h *User) List(c echo.Context) error {
	users, err := h.svc.List(c.Request().Context())
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, users)
}

// Create POST /api/users
func (h *User) Create(c echo.Context) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Role     string `json:"role"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	user, err := h.svc.Create(c.Request().Context(), req.Username, req.Password, req.Name, req.Role)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, user)
}

// Update PUT /api/users/:id
func (h *User) Update(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return api.Fail(c, service.ErrBadRequest("ID 不合法"))
	}

	var req struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	user, err := h.svc.Update(c.Request().Context(), id, req.Name, req.Enabled)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, user)
}

// ResetPassword PUT /api/users/:id/password
func (h *User) ResetPassword(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return api.Fail(c, service.ErrBadRequest("ID 不合法"))
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	err = h.svc.ResetPassword(c.Request().Context(), id, req.Password)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, nil)
}
