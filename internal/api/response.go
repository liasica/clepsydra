package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/service"
)

// body 统一响应结构
type body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 成功响应
func OK(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, body{Code: 0, Message: "ok", Data: data})
}

// Fail 失败响应，业务错误映射 HTTP 状态，其余按 500 处理
func Fail(c echo.Context, err error) error {
	var svcErr *service.Error
	if errors.As(err, &svcErr) {
		return c.JSON(svcErr.Code/100, body{Code: svcErr.Code, Message: svcErr.Message})
	}

	return c.JSON(http.StatusInternalServerError, body{Code: 50000, Message: "服务器内部错误"})
}
