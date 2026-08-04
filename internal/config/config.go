package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用全量配置
type Config struct {
	Server   Server   `mapstructure:"server"`
	Database Database `mapstructure:"database"`
	JWT      JWT      `mapstructure:"jwt"`
	Admin    Admin    `mapstructure:"admin"`
	Log      Log      `mapstructure:"log"`
	Holiday  Holiday  `mapstructure:"holiday"`
}

// Server HTTP 服务配置
type Server struct {
	Address string `mapstructure:"address"`
	Mode    string `mapstructure:"mode"` // debug 输出彩色控制台日志，release 输出 JSON 文件
}

// Database 数据库配置
type Database struct {
	DSN string `mapstructure:"dsn"`
}

// JWT 认证配置
type JWT struct {
	Secret string        `mapstructure:"secret"`
	Expire time.Duration `mapstructure:"expire"`
}

// Admin 初始管理员配置，仅首次种子时生效
type Admin struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// Log 日志轮转配置
type Log struct {
	Dir     string `mapstructure:"dir"`
	MaxSize int    `mapstructure:"max_size"` // 单文件上限，单位 MB
	MaxAge  int    `mapstructure:"max_age"`  // 保留天数
}

// Holiday 节假日数据文件配置
type Holiday struct {
	File string `mapstructure:"file"`
}

// Load 从指定路径加载 YAML 配置，环境变量以 CLEPSYDRA_ 前缀覆盖
// 嵌套字段的分隔点替换为下划线，如 CLEPSYDRA_SERVER_ADDRESS 覆盖 server.address
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("CLEPSYDRA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := new(Config)
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
