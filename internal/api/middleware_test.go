package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

func TestRequireAuthAndAdmin(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:mw?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	hash, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.DefaultCost)
	client.User.Create().SetUsername("c").SetPasswordHash(string(hash)).
		SetName("需求方").SetRole("client").SaveX(context.Background())

	auth := service.NewAuth(client, config.JWT{Secret: "s", Expire: time.Hour})
	token, _, _ := auth.Login(context.Background(), "c", "p")

	e := echo.New()
	handler := func(c echo.Context) error { return OK(c, "pass") }

	// 无 token 返回 401
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	err := RequireAuth(auth)(handler)(e.NewContext(req, rec))
	if err == nil {
		t.Error("无 token 应返回错误")
	}

	// 有 token 通过 RequireAuth
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	if err = RequireAuth(auth)(handler)(e.NewContext(req, rec)); err != nil {
		t.Errorf("有效 token 应通过: %v", err)
	}

	// client 角色被 RequireAdmin 拒绝
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	if err = RequireAuth(auth)(RequireAdmin(handler))(e.NewContext(req, rec)); err == nil {
		t.Error("client 访问 admin 接口应被拒绝")
	}
}
