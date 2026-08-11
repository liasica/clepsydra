package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// TestDemandCreateHandler 覆盖创建接口的正常路径与参数校验
func TestDemandCreateHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdemand?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)

	e := echo.New()
	reqBody := `{"title":"新功能","description":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(reqBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 1, Role: "admin", Name: "管理员"})

	if err := h.Create(c); err != nil {
		t.Fatalf("创建接口错误: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("HTTP 状态 = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Errorf("响应异常: %s", rec.Body.String())
	}

	rows, err := svc.List(ctx, "", 0, 0, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("创建后查询列表失败: %v, len=%d", err, len(rows))
	}
	idStr := strconv.Itoa(rows[0].ID)

	// submit-estimate 时预估人天为 0 应拒绝
	req = httptest.NewRequest(http.MethodPost, "/api/demands/"+idStr+"/submit-estimate", strings.NewReader(`{"estimated_half_days":0}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	c.Set("claims", &service.Claims{UserID: 1, Role: "admin", Name: "管理员"})
	_ = h.SubmitEstimate(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法参数应返回 400, got %d", rec.Code)
	}
}

// newDemandTestContext 构造带登录态的测试请求上下文，body 为空串时不携带请求体
func newDemandTestContext(e *echo.Echo, method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 1, Role: "admin", Name: "管理员"})

	return c, rec
}

// TestDemandLifecycleHandlers 覆盖需求从创建到验收的完整状态流转链路
func TestDemandLifecycleHandlers(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdemandlifecycle?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 创建需求
	c, rec := newDemandTestContext(e, http.MethodPost, "/api/demands",
		`{"title":"周期联调","description":"跨系统联调"}`)
	if err := h.Create(c); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("创建 HTTP 状态 = %d, body = %s", rec.Code, rec.Body.String())
	}

	rows, err := svc.List(ctx, "", 0, 0, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("创建后查询列表失败: %v, len=%d", err, len(rows))
	}
	id := rows[0].ID
	idStr := strconv.Itoa(id)

	// List：带状态过滤应能查到刚创建的 draft 需求
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/demands?status=draft", "")
	if err = h.List(c); err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "周期联调") {
		t.Errorf("List 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Get：按 ID 查询
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/demands/"+idStr, "")
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.Get(c); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "周期联调") {
		t.Errorf("Get 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Get：非法 ID 返回 400
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/demands/abc", "")
	c.SetParamNames("id")
	c.SetParamValues("abc")
	_ = h.Get(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 ID 应返回 400, got %d", rec.Code)
	}

	// Update：仅修改标题与描述
	c, rec = newDemandTestContext(e, http.MethodPut, "/api/demands/"+idStr,
		`{"title":"周期联调-改","description":"跨系统联调"}`)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.Update(c); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "周期联调-改") {
		t.Errorf("Update 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// SubmitEstimate：draft → pending_estimate，携带预估人天与预计开工日期
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/"+idStr+"/submit-estimate",
		`{"estimated_half_days":6,"planned_start_date":"2026-08-05"}`)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.SubmitEstimate(c); err != nil {
		t.Fatalf("SubmitEstimate 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("SubmitEstimate 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// ConfirmEstimate：pending_estimate → confirmed
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/"+idStr+"/confirm-estimate", "")
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.ConfirmEstimate(c); err != nil {
		t.Fatalf("ConfirmEstimate 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("ConfirmEstimate 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Start：日期为空应返回 400，且不能影响后续正常流转
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/"+idStr+"/start", `{"actual_start_date":""}`)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	_ = h.Start(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("开工日期为空应返回 400, got %d", rec.Code)
	}

	// Start：confirmed → in_progress，实际开工日期须落在当前时间之前
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/"+idStr+"/start", `{"actual_start_date":"2026-07-20"}`)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.Start(c); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Start 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Finish：in_progress → pending_acceptance，支持预登记未来完成日期
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/"+idStr+"/finish",
		`{"actual_start_date":"2026-07-20","actual_end_date":"2026-07-25","actual_half_days":9}`)
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.Finish(c); err != nil {
		t.Fatalf("Finish 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Finish 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Accept：pending_acceptance → accepted，人工确认固定传 false, false
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/"+idStr+"/accept", "")
	c.SetParamNames("id")
	c.SetParamValues(idStr)
	if err = h.Accept(c); err != nil {
		t.Fatalf("Accept 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Accept 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// 验证最终状态与人工确认标记
	final, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("查询最终状态失败: %v", err)
	}
	if final.Status.String() != "accepted" {
		t.Errorf("最终状态 = %s, want accepted", final.Status)
	}
	if final.AcceptAuto {
		t.Error("人工确认 AcceptAuto 应为 false")
	}
	if final.AcceptLocked {
		t.Error("人工确认 AcceptLocked 应为 false")
	}
}

// TestDemandFinishRequiresBothDates 校验 Finish 接口两个日期均必填
func TestDemandFinishRequiresBothDates(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdemandfinish?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 缺少 actual_end_date
	c, rec := newDemandTestContext(e, http.MethodPost, "/api/demands/1/finish",
		`{"actual_start_date":"2026-08-05","actual_half_days":4}`)
	c.SetParamNames("id")
	c.SetParamValues("1")
	_ = h.Finish(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺少完成日期应返回 400, got %d", rec.Code)
	}

	// 缺少 actual_start_date
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands/1/finish",
		`{"actual_end_date":"2026-08-12","actual_half_days":4}`)
	c.SetParamNames("id")
	c.SetParamValues("1")
	_ = h.Finish(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺少开工日期应返回 400, got %d", rec.Code)
	}
}

// TestDemandCreateWithEstimateHandler 创建接口携带预估字段的权限与校验
func TestDemandCreateWithEstimateHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdemandcreateest?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 超管带人天 + 日期 + 已确认 → 创建即 confirmed
	c, rec := newDemandTestContext(e, http.MethodPost, "/api/demands",
		`{"title":"快捷创建","estimated_half_days":4,"planned_start_date":"2026-09-01","confirmed":true}`)
	if err := h.Create(c); err != nil {
		t.Fatalf("快捷创建失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP 状态 = %d, body = %s", rec.Code, rec.Body.String())
	}
	rows, err := svc.List(ctx, "confirmed", 0, 0, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("confirmed 需求数 = %d, err = %v, want 1", len(rows), err)
	}
	if rows[0].EstimatedHalfDays != 4 || rows[0].EstimateConfirmedBy == nil || *rows[0].EstimateConfirmedBy != 1 {
		t.Errorf("人天 = %d, 确认人 = %v, want 4 / 1", rows[0].EstimatedHalfDays, rows[0].EstimateConfirmedBy)
	}

	// 勾选已确认但未填人天 → 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands", `{"title":"缺人天","confirmed":true}`)
	_ = h.Create(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("已确认缺人天应返回 400, got %d", rec.Code)
	}

	// 只填日期未填人天 → 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/demands",
		`{"title":"缺人天带日期","planned_start_date":"2026-09-01"}`)
	_ = h.Create(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("只填日期应返回 400, got %d", rec.Code)
	}

	// 需求方携带预估字段 → 403
	req := httptest.NewRequest(http.MethodPost, "/api/demands",
		strings.NewReader(`{"title":"越权预估","estimated_half_days":4}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	_ = h.Create(c)
	if rec.Code != http.StatusForbidden {
		t.Errorf("需求方携带预估字段应返回 403, got %d", rec.Code)
	}

	// 需求方不带预估字段 → 正常创建 draft
	req = httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(`{"title":"需求方创建"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err = h.Create(c); err != nil {
		t.Fatalf("需求方创建失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("需求方普通创建应成功, got %d, body = %s", rec.Code, rec.Body.String())
	}
	drafts, _ := svc.List(ctx, "draft", 0, 0, "")
	if len(drafts) != 1 {
		t.Errorf("draft 需求数 = %d, want 1", len(drafts))
	}
}

// TestDemandProjectsHandler 覆盖创建带项目与独立改标签接口
func TestDemandProjectsHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdproj?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	p := client.Project.Create().SetName("官网").SaveX(ctx)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 创建带项目
	body := `{"title":"需求一","project_ids":[` + strconv.Itoa(p.ID) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.Create(c); err != nil {
		t.Fatalf("创建错误: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("创建响应异常: %s", rec.Body.String())
	}

	rows, _ := svc.List(ctx, "", 0, 0, "")
	id := strconv.Itoa(rows[0].ID)

	// 独立接口清空标签（需求方也可操作）
	req = httptest.NewRequest(http.MethodPut, "/api/demands/"+id+"/projects", strings.NewReader(`{"project_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.UpdateProjects(c); err != nil {
		t.Fatalf("改标签错误: %v", err)
	}
	got, _ := svc.Get(ctx, rows[0].ID)
	if len(got.Edges.Projects) != 0 {
		t.Errorf("标签应已清空: %+v", got.Edges.Projects)
	}

	// 列表按项目筛选参数透传
	req = httptest.NewRequest(http.MethodGet, "/api/demands?project_id="+strconv.Itoa(p.ID), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatalf("列表错误: %v", err)
	}
	if strings.Contains(rec.Body.String(), "需求一") {
		t.Errorf("标签已清空，按该项目筛选不应包含需求一: %s", rec.Body.String())
	}
}

// TestDemandTagsHandler 覆盖创建带性质标签与独立改标签接口
func TestDemandTagsHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdtag?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	tg := client.Tag.Create().SetName("优化").SetColor("#112233").SaveX(ctx)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)
	e := echo.New()

	// 创建带标签
	body := `{"title":"需求一","tag_ids":[` + strconv.Itoa(tg.ID) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.Create(c); err != nil {
		t.Fatalf("创建错误: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"code":0`) {
		t.Fatalf("创建响应异常: %s", rec.Body.String())
	}

	rows, _ := svc.List(ctx, "", 0, 0, "")
	id := strconv.Itoa(rows[0].ID)

	// 独立接口清空标签（需求方也可操作）
	req = httptest.NewRequest(http.MethodPut, "/api/demands/"+id+"/tags", strings.NewReader(`{"tag_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set("claims", &service.Claims{UserID: 2, Role: "client", Name: "需求方"})
	if err := h.UpdateTags(c); err != nil {
		t.Fatalf("改标签错误: %v", err)
	}
	got, _ := svc.Get(ctx, rows[0].ID)
	if len(got.Edges.Tags) != 0 {
		t.Errorf("标签应已清空: %+v", got.Edges.Tags)
	}

	// 列表按标签筛选参数透传
	req = httptest.NewRequest(http.MethodGet, "/api/demands?tag_id="+strconv.Itoa(tg.ID), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatalf("列表错误: %v", err)
	}
	if strings.Contains(rec.Body.String(), "需求一") {
		t.Errorf("标签已清空，按该标签筛选不应包含需求一: %s", rec.Body.String())
	}
}
