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

	if rotator.MaxSize != 100 {
		t.Errorf("rotator.MaxSize 应为 100，实际 %d", rotator.MaxSize)
	}
	if rotator.MaxAge != 30 {
		t.Errorf("rotator.MaxAge 应为 30，实际 %d", rotator.MaxAge)
	}
	if !rotator.Compress {
		t.Error("rotator.Compress 应为 true")
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
	dir := t.TempDir()
	log, rotator := New(config.Log{Dir: dir, MaxSize: 100, MaxAge: 30}, true)
	if rotator != nil {
		t.Error("debug 模式不写文件，rotator 应为 nil")
	}

	log.Info().Msg("console only")

	_, err := os.Stat(filepath.Join(dir, "clepsydra.log"))
	if !os.IsNotExist(err) {
		t.Error("debug 模式不应生成日志文件")
	}
}
