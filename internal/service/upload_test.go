package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clepsydra/internal/config"
)

func newTestUpload(t *testing.T, maxSizeMB int64) *Upload {
	t.Helper()

	svc, err := NewUpload(config.Upload{Dir: t.TempDir(), MaxSize: maxSizeMB})
	if err != nil {
		t.Fatalf("构建上传服务失败: %v", err)
	}

	return svc
}

func TestSaveImageRejectsDisallowedType(t *testing.T) {
	svc := newTestUpload(t, 10)

	content := bytes.NewReader([]byte("<svg/>"))
	if _, err := svc.SaveImage(content, "image/svg+xml", 6); err == nil {
		t.Error("svg 应被拒绝")
	}

	content = bytes.NewReader([]byte("plain"))
	if _, err := svc.SaveImage(content, "text/plain", 5); err == nil {
		t.Error("非图片类型应被拒绝")
	}
}

func TestSaveImageAcceptsAllowedTypeAndRandomizesName(t *testing.T) {
	svc := newTestUpload(t, 10)

	// 带 charset 参数的 Content-Type 也应正常识别
	first, err := svc.SaveImage(bytes.NewReader([]byte("png-data")), "image/png; charset=binary", 8)
	if err != nil {
		t.Fatalf("保存 png 失败: %v", err)
	}
	if !strings.HasPrefix(first, "/api/uploads/") || !strings.HasSuffix(first, ".png") {
		t.Fatalf("返回路径不符合预期: %s", first)
	}

	second, err := svc.SaveImage(bytes.NewReader([]byte("png-data")), "image/png", 8)
	if err != nil {
		t.Fatalf("保存第二张图失败: %v", err)
	}
	if first == second {
		t.Error("同名内容应生成不同的随机文件名")
	}

	// 落盘内容与上传一致
	path, err := svc.FilePath(strings.TrimPrefix(first, "/api/uploads/"))
	if err != nil {
		t.Fatalf("解析文件路径失败: %v", err)
	}
	saved, err := os.ReadFile(path) //nolint:gosec // 路径由随机文件名拼出，测试内可控
	if err != nil {
		t.Fatalf("读取落盘文件失败: %v", err)
	}
	if string(saved) != "png-data" {
		t.Errorf("落盘内容不一致: %s", saved)
	}
}

func TestSaveImageRejectsOversizeAndLeavesNoFile(t *testing.T) {
	svc := newTestUpload(t, 1)

	// 谎报 size 也要被实际写入长度拦住，且不能留下半截文件
	oversize := bytes.Repeat([]byte("x"), int(svc.MaxSize())+1024)
	if _, err := svc.SaveImage(bytes.NewReader(oversize), "image/png", 1); err == nil {
		t.Fatal("超限图片应被拒绝")
	}

	entries, err := os.ReadDir(filepath.Dir(mustFilePath(t, svc, "probe.png")))
	if err != nil {
		t.Fatalf("读取上传目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("超限上传不应残留文件，实际残留 %d 个", len(entries))
	}
}

func TestFilePathRejectsTraversal(t *testing.T) {
	svc := newTestUpload(t, 10)

	for _, name := range []string{"", "..", "../etc/passwd", "sub/dir.png"} {
		if _, err := svc.FilePath(name); err == nil {
			t.Errorf("非法文件名应被拒绝: %q", name)
		}
	}
}

func mustFilePath(t *testing.T, svc *Upload, name string) string {
	t.Helper()

	path, err := svc.FilePath(name)
	if err != nil {
		t.Fatalf("解析文件路径失败: %v", err)
	}

	return path
}
