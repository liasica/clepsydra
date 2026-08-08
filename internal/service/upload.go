package service

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"clepsydra/internal/config"
)

// 允许上传的图片类型，键为 MIME，值为落盘扩展名
// 只放开位图格式，svg 可携带脚本，作为富文本附件风险过高
var allowedImageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// Upload 图片上传服务，落地到本地磁盘
type Upload struct {
	dir     string
	maxSize int64 // 单文件字节上限
}

// NewUpload 构建上传服务，同时确保存储目录存在
func NewUpload(cfg config.Upload) (*Upload, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = "uploads"
	}

	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = 10
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}

	return &Upload{dir: dir, maxSize: maxSize << 20}, nil
}

// MaxSize 单文件字节上限，供 handler 做前置校验
func (s *Upload) MaxSize() int64 {
	return s.maxSize
}

// SaveImage 校验并保存图片，返回可直接写进 markdown 的访问路径
// contentType 取自上传表单，实际扩展名以白名单为准，不信任客户端文件名
func (s *Upload) SaveImage(src io.Reader, contentType string, size int64) (string, error) {
	if size > s.maxSize {
		return "", ErrBadRequest("图片超过大小限制")
	}

	ext, ok := allowedImageTypes[normalizeContentType(contentType)]
	if !ok {
		return "", ErrBadRequest("仅支持 png、jpg、gif、webp 格式的图片")
	}

	name, err := randomName(ext)
	if err != nil {
		return "", err
	}

	// 文件名由随机串生成，不含用户输入，不存在路径穿越
	dst, err := os.Create(filepath.Join(s.dir, name))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = dst.Close()
	}()

	// 即便前面校验过 size，这里仍按上限截断，防止客户端谎报 Content-Length
	written, err := io.Copy(dst, io.LimitReader(src, s.maxSize+1))
	if err != nil {
		return "", err
	}
	if written > s.maxSize {
		_ = os.Remove(dst.Name())

		return "", ErrBadRequest("图片超过大小限制")
	}

	return "/api/uploads/" + name, nil
}

// FilePath 把访问用的文件名还原成磁盘路径
// 文件名只允许随机串加扩展名，借此挡掉 ../ 之类的穿越尝试
func (s *Upload) FilePath(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return "", ErrNotFound
	}

	return filepath.Join(s.dir, name), nil
}

// normalizeContentType 去掉 charset 之类的参数并统一小写
func normalizeContentType(contentType string) string {
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}

	return strings.ToLower(strings.TrimSpace(contentType))
}

// randomName 生成 32 位十六进制随机文件名
// 图片接口不鉴权，靠这个不可猜的名字兜住访问控制
func randomName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf) + ext, nil
}
