package logger

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"

	"clepsydra/internal/config"
)

// New 构建应用日志器
// debug 为 true 时输出彩色控制台日志且不写文件
// 否则写入 <dir>/clepsydra.log，按大小轮转、保留 MaxAge 天、gzip 归档
// 返回的 lumberjack.Logger 供定时任务每日零点调用 Rotate() 实现每日切割
func New(cfg config.Log, debug bool) (zerolog.Logger, *lumberjack.Logger) {
	if debug {
		writer := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.TimeFormat = time.RFC3339
		})
		return zerolog.New(writer).With().Timestamp().Caller().Logger(), nil
	}

	rotator := &lumberjack.Logger{
		Filename: filepath.Join(cfg.Dir, "clepsydra.log"),
		MaxSize:  cfg.MaxSize,
		MaxAge:   cfg.MaxAge,
		Compress: true,
	}

	_ = os.MkdirAll(cfg.Dir, 0o755)

	return zerolog.New(rotator).With().Timestamp().Logger(), rotator
}
