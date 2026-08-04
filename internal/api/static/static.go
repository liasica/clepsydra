// Package static 托管前端构建产物并提供 SPA 回退
package static

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// distFS 前端构建产物，仓库仅含 .gitkeep 占位，make dashboard 时同步真实产物
//
//go:embed all:dist
var distFS embed.FS

// Register 以内嵌产物注册静态托管
func Register(e *echo.Echo) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	RegisterFS(e, sub)
}

// RegisterFS 以给定文件系统注册静态托管，供测试注入
func RegisterFS(e *echo.Echo, files fs.FS) {
	server := http.FileServer(http.FS(files))

	e.GET("/*", func(c echo.Context) error {
		path := strings.TrimPrefix(c.Request().URL.Path, "/")

		// api 前缀不属于页面路由，未命中时保持 404 语义
		if path == "api" || strings.HasPrefix(path, "api/") {
			return echo.ErrNotFound
		}

		if path != "" {
			if _, err := fs.Stat(files, path); err == nil {
				server.ServeHTTP(c.Response(), c.Request())
				return nil
			}
		}

		// 未命中静态文件时回退 index.html，支持 history 路由刷新
		if _, err := fs.Stat(files, "index.html"); err != nil {
			return c.String(http.StatusServiceUnavailable, "前端未构建，请先执行 make dashboard")
		}

		c.Request().URL.Path = "/"
		server.ServeHTTP(c.Response(), c.Request())
		return nil
	})
}
