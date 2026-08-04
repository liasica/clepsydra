package service

import "fmt"

// Error 业务错误，Code 用于前端分支处理
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

var (
	ErrNotFound          = &Error{Code: 40400, Message: "资源不存在"}
	ErrForbidden         = &Error{Code: 40300, Message: "无权限操作"}
	ErrUnauthorized      = &Error{Code: 40100, Message: "未登录或凭证失效"}
	ErrInvalidTransition = &Error{Code: 42200, Message: "当前状态不允许该操作"}
)

// ErrBadRequest 构造参数错误
func ErrBadRequest(msg string) *Error {
	return &Error{Code: 40000, Message: msg}
}
