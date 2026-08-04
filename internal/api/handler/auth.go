package handler

import (
	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// Auth 认证相关接口
type Auth struct {
	svc *service.Auth
}

// NewAuth 构建认证 handler
func NewAuth(svc *service.Auth) *Auth {
	return &Auth{svc: svc}
}

// Login POST /api/auth/login
func (h *Auth) Login(c echo.Context) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	token, user, err := h.svc.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":   user.ID,
			"name": user.Name,
			"role": user.Role,
		},
	})
}

// Me GET /api/auth/me
func (h *Auth) Me(c echo.Context) error {
	u, err := h.svc.Me(c.Request().Context(), api.Claims(c).UserID)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]any{
		"id":   u.ID,
		"name": u.Name,
		"role": u.Role,
	})
}
