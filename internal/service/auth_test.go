package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
)

func TestLoginAndParseToken(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:auth?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()

	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	client.User.Create().SetUsername("admin").SetPasswordHash(string(hash)).
		SetName("管理员").SetRole("admin").SaveX(ctx)

	auth := NewAuth(client, config.JWT{Secret: "test-secret", Expire: time.Hour})

	// 正确密码登录成功
	token, user, err := auth.Login(ctx, "admin", "secret123")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if token == "" || user.Username != "admin" {
		t.Error("登录返回不完整")
	}

	// token 可解析出用户与角色
	claims, err := auth.ParseToken(token)
	if err != nil {
		t.Fatalf("解析 token 失败: %v", err)
	}
	if claims.UserID != user.ID || claims.Role != "admin" {
		t.Errorf("claims = %+v", claims)
	}

	// 错误密码拒绝
	if _, _, err = auth.Login(ctx, "admin", "wrong"); err == nil {
		t.Error("错误密码应拒绝登录")
	}

	// 禁用用户拒绝
	client.User.Create().SetUsername("closed").SetPasswordHash(string(hash)).
		SetName("停用").SetRole("client").SetEnabled(false).SaveX(ctx)
	if _, _, err = auth.Login(ctx, "closed", "secret123"); err == nil {
		t.Error("禁用用户应拒绝登录")
	}
}
