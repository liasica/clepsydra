package config

import "testing"

func TestLoad(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if cfg.Server.Address != ":8080" {
		t.Errorf("Server.Address = %q, want %q", cfg.Server.Address, ":8080")
	}
	if cfg.Database.DSN == "" {
		t.Error("Database.DSN 不能为空")
	}
	if cfg.JWT.Expire.Hours() != 72 {
		t.Errorf("JWT.Expire = %v, want 72h", cfg.JWT.Expire)
	}
	if cfg.Log.MaxSize != 100 || cfg.Log.MaxAge != 30 {
		t.Errorf("Log 轮转参数错误: %+v", cfg.Log)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("CLEPSYDRA_SERVER_ADDRESS", ":9999")
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if cfg.Server.Address != ":9999" {
		t.Errorf("Server.Address = %q, want %q", cfg.Server.Address, ":9999")
	}
}
