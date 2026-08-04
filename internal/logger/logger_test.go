package logger

import (
	"os"
	"path/filepath"
	"testing"

	"clepsydra/internal/config"
)

func TestNewWritesToFile(t *testing.T) {
	dir := t.TempDir()

	log, rotator := New(config.Log{Dir: dir, MaxSize: 100, MaxAge: 30}, false)
	log.Info().Str("k", "v").Msg("hello")

	if rotator == nil {
		t.Fatal("release 模式必须返回可轮转的 rotator")
	}

	data, err := os.ReadFile(filepath.Join(dir, "clepsydra.log"))
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("日志文件不应为空")
	}
}

func TestNewDebugMode(t *testing.T) {
	log, rotator := New(config.Log{Dir: t.TempDir(), MaxSize: 100, MaxAge: 30}, true)
	if rotator != nil {
		t.Error("debug 模式不写文件，rotator 应为 nil")
	}

	log.Info().Msg("console only")
}
