package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"clepsydra/internal/api/docs"
	"clepsydra/internal/service"
)

// handler 包下的具体 handler 均反向依赖本包的 OK/Fail/Claims，
// 因此这里用接口而非具体类型承接，避免 api 包与 handler 包相互导入

// AuthHandler 认证接口方法集
type AuthHandler interface {
	Login(c echo.Context) error
}

// UserHandler 用户管理接口方法集
type UserHandler interface {
	List(c echo.Context) error
	Create(c echo.Context) error
	Update(c echo.Context) error
	ResetPassword(c echo.Context) error
}

// SettingHandler 设置与节假日接口方法集
type SettingHandler interface {
	All(c echo.Context) error
	Update(c echo.Context) error
	Holidays(c echo.Context) error
	SaveHolidays(c echo.Context) error
	DeleteHoliday(c echo.Context) error
}

// DemandHandler 需求接口方法集
type DemandHandler interface {
	List(c echo.Context) error
	Get(c echo.Context) error
	Create(c echo.Context) error
	Update(c echo.Context) error
	SubmitEstimate(c echo.Context) error
	ConfirmEstimate(c echo.Context) error
	Start(c echo.Context) error
	Finish(c echo.Context) error
	Accept(c echo.Context) error
}

// BillHandler 账单接口方法集
type BillHandler interface {
	List(c echo.Context) error
	Get(c echo.Context) error
	Generate(c echo.Context) error
	ToggleWaive(c echo.Context) error
	Share(c echo.Context) error
	Revoke(c echo.Context) error
	Confirm(c echo.Context) error
}

// DashboardHandler 工作台接口方法集
type DashboardHandler interface {
	Todos(c echo.Context) error
}

// AuditLogHandler 审计日志接口方法集
type AuditLogHandler interface {
	List(c echo.Context) error
}

// Handlers 全部 handler 集合
type Handlers struct {
	Auth      AuthHandler
	User      UserHandler
	Setting   SettingHandler
	Demand    DemandHandler
	Bill      BillHandler
	Dashboard DashboardHandler
	AuditLog  AuditLogHandler
}

// Register 注册全部路由
func Register(e *echo.Echo, auth *service.Auth, h Handlers) {
	e.Use(middleware.Recover(), middleware.CORS())

	// 接口文档，挂在认证中间件之外，无需登录即可访问
	docs.RegisterDocs(e)

	root := e.Group("/api")
	root.POST("/auth/login", h.Auth.Login)

	// 登录可访问
	authed := root.Group("", RequireAuth(auth))
	authed.GET("/dashboard/todos", h.Dashboard.Todos)
	authed.GET("/demands", h.Demand.List)
	authed.GET("/demands/:id", h.Demand.Get)
	authed.POST("/demands/:id/confirm-estimate", h.Demand.ConfirmEstimate)
	authed.POST("/demands/:id/accept", h.Demand.Accept)
	authed.GET("/bills", h.Bill.List)
	authed.GET("/bills/:id", h.Bill.Get)
	authed.POST("/bills/:id/confirm", h.Bill.Confirm)

	// 仅超级管理员
	adminGroup := authed.Group("", RequireAdmin)
	adminGroup.GET("/users", h.User.List)
	adminGroup.POST("/users", h.User.Create)
	adminGroup.PUT("/users/:id", h.User.Update)
	adminGroup.PUT("/users/:id/password", h.User.ResetPassword)
	adminGroup.GET("/settings", h.Setting.All)
	adminGroup.PUT("/settings", h.Setting.Update)
	adminGroup.GET("/holidays", h.Setting.Holidays)
	adminGroup.PUT("/holidays", h.Setting.SaveHolidays)
	adminGroup.DELETE("/holidays/:date", h.Setting.DeleteHoliday)
	adminGroup.POST("/demands", h.Demand.Create)
	adminGroup.PUT("/demands/:id", h.Demand.Update)
	adminGroup.POST("/demands/:id/submit-estimate", h.Demand.SubmitEstimate)
	adminGroup.POST("/demands/:id/start", h.Demand.Start)
	adminGroup.POST("/demands/:id/finish", h.Demand.Finish)
	adminGroup.POST("/bills/generate", h.Bill.Generate)
	adminGroup.POST("/bills/:id/items/:itemId/waive", h.Bill.ToggleWaive)
	adminGroup.POST("/bills/:id/share", h.Bill.Share)
	adminGroup.POST("/bills/:id/revoke", h.Bill.Revoke)
	adminGroup.GET("/audit-logs", h.AuditLog.List)
}
