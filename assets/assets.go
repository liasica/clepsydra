// Package assets 集中管理仓库内嵌资源
package assets

import (
	"embed"
	"io/fs"
)

// dashboardFS 前端构建产物，仓库仅含 .gitkeep 占位，make dashboard 时同步真实产物
//
//go:embed all:dashboard
var dashboardFS embed.FS

// Dashboard 返回前端构建产物文件系统
func Dashboard() fs.FS {
	sub, err := fs.Sub(dashboardFS, "dashboard")
	if err != nil {
		panic(err)
	}

	return sub
}
