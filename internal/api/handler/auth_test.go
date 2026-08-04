package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// TestAuthMeHandler 覆盖当前用户接口：正常返回本人信息，用户被停用后视为凭证失效
func TestAuthMeHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hauthme?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	svc := service.NewAuth(client, config.JWT{Secret: "s", Expire: time.Hour})
	h := NewAuth(svc)
	e := echo.New()

	c, rec := newDemandTestContext(e, http.MethodGet, "/api/auth/me", "")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Me 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, field := range []string{`"id":1`, `"name":`, `"role":"admin"`} {
		if !strings.Contains(body, field) {
			t.Errorf("响应缺少字段 %s: %s", field, body)
		}
	}

	// 停用用户后，凭证虽有效也应返回 401，保证停用即时生效
	client.User.UpdateOneID(1).SetEnabled(false).ExecX(ctx)
	c2, rec2 := newDemandTestContext(e, http.MethodGet, "/api/auth/me", "")
	_ = h.Me(c2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("停用用户应返回 401, 实际: %d, %s", rec2.Code, rec2.Body.String())
	}
}
