// Package docs 提供 OpenAPI 规范文件与基于 scalar 的接口文档渲染页面
package docs

import (
	_ "embed"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed openapi.yaml
var openAPISpec []byte

// page scalar 官方标准集成方式：内联 api-reference 脚本标签指向 spec 地址，再引入 scalar 渲染脚本
const page = `<!doctype html>
<html>
<head>
  <title>Clepsydra API</title>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
  <script id="api-reference" data-url="/docs/openapi.yaml"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>
`

// RegisterDocs 注册接口文档路由，挂在认证中间件之外，无需登录即可访问
func RegisterDocs(e *echo.Echo) {
	e.GET("/docs", func(c echo.Context) error {
		return c.HTML(http.StatusOK, page)
	})

	e.GET("/docs/openapi.yaml", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/yaml", openAPISpec)
	})
}
