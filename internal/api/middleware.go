package api

import (
	"strings"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/service"
)

const claimsKey = "claims"

// RequireAuth 解析 Bearer token 并注入 claims
func RequireAuth(auth *service.Auth) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				// Fail 仅负责写响应体（成功写入时返回 nil），中间件链路需要非 nil 错误来终止后续处理，因此显式返回原始业务错误
				_ = Fail(c, service.ErrUnauthorized)
				return service.ErrUnauthorized
			}

			claims, err := auth.ParseToken(token)
			if err != nil {
				_ = Fail(c, err)
				return err
			}

			c.Set(claimsKey, claims)

			return next(c)
		}
	}
}

// RequireAdmin 仅允许超级管理员
func RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if Claims(c).Role != "admin" {
			_ = Fail(c, service.ErrForbidden)
			return service.ErrForbidden
		}

		return next(c)
	}
}

// Claims 从 context 取出登录信息
func Claims(c echo.Context) *service.Claims {
	claims, _ := c.Get(claimsKey).(*service.Claims)
	return claims
}
