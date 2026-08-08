package handler

import (
	"os"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// Upload 图片上传接口
type Upload struct {
	uploadSvc *service.Upload
}

// NewUpload 构建上传 handler
func NewUpload(uploadSvc *service.Upload) *Upload {
	return &Upload{uploadSvc: uploadSvc}
}

// Image POST /api/uploads
// 表单字段名固定为 file，返回 { url } 供编辑器写进 markdown
func (h *Upload) Image(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return api.Fail(c, service.ErrBadRequest("未收到上传文件"))
	}

	src, err := file.Open()
	if err != nil {
		return api.Fail(c, service.ErrBadRequest("上传文件无法读取"))
	}
	defer func() {
		_ = src.Close()
	}()

	url, err := h.uploadSvc.SaveImage(src, file.Header.Get("Content-Type"), file.Size)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]string{"url": url})
}

// Serve GET /api/uploads/:name
// 图片要能直接放进 <img src>，带不了 Authorization 头，因此这里不做鉴权，
// 访问控制依赖随机文件名不可猜
func (h *Upload) Serve(c echo.Context) error {
	path, err := h.uploadSvc.FilePath(c.Param("name"))
	if err != nil {
		return api.Fail(c, err)
	}

	if _, err = os.Stat(path); err != nil {
		return api.Fail(c, service.ErrNotFound)
	}

	// 文件名随机且内容不可变，允许长期缓存
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	return c.File(path)
}
