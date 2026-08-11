package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

// TestBillLifecycleHandlers 覆盖账单从生成到确认的完整链路
func TestBillLifecycleHandlers(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hbill?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	h := NewBill(billSvc)
	e := echo.New()

	// 准备一个账期内已完成并验收的需求
	act := service.Actor{ID: 1, Name: "管理员"}
	d, err := demandSvc.Create(ctx, act, "联调需求", "", 0, nil, false, nil, "")
	if err != nil {
		t.Fatalf("创建需求失败: %v", err)
	}
	_ = demandSvc.SubmitEstimate(ctx, act, d.ID, 4, nil)
	_ = demandSvc.ConfirmEstimate(ctx, act, d.ID)
	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
	_ = demandSvc.Start(ctx, act, d.ID, start)
	if err = demandSvc.Finish(ctx, act, d.ID, start, end, 4); err != nil {
		t.Fatalf("完成需求失败: %v", err)
	}
	if err = demandSvc.Accept(ctx, act, d.ID, false, false); err != nil {
		t.Fatalf("验收需求失败: %v", err)
	}

	// Generate：生成 2026-07 账单
	c, rec := newDemandTestContext(e, http.MethodPost, "/api/bills/generate", `{"period":"2026-07"}`)
	if err = h.Generate(c); err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Generate 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	bills, err := billSvc.List(ctx)
	if err != nil || len(bills) != 1 {
		t.Fatalf("生成后查询列表失败: %v, len=%d", err, len(bills))
	}
	billID := bills[0].ID
	billIDStr := strconv.Itoa(billID)

	// Generate：账期格式非法应返回 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/generate", `{"period":"202607"}`)
	_ = h.Generate(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法账期应返回 400, got %d", rec.Code)
	}

	// List：应能查到刚生成的账单
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/bills", "")
	if err = h.List(c); err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "2026-07") {
		t.Errorf("List 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "自动生成：2026-07") {
		t.Errorf("List 响应应包含自动账单名称, got %s", rec.Body.String())
	}

	// List：契约上明细仅详情接口返回，列表项不应残留 items/edges 字段
	var listResp struct {
		Data []map[string]any `json:"data"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("List 响应解析失败: %v", err)
	}
	if len(listResp.Data) != 1 {
		t.Fatalf("List 应返回 1 条账单, got %d", len(listResp.Data))
	}
	if _, ok := listResp.Data[0]["items"]; ok {
		t.Errorf("List 响应不应包含 items 字段, got %s", rec.Body.String())
	}
	if _, ok := listResp.Data[0]["edges"]; ok {
		t.Errorf("List 响应不应包含 edges 字段, got %s", rec.Body.String())
	}

	// Get：应在顶层含 items 明细，不应再嵌套于 ent 的 edges 结构下
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/bills/"+billIDStr, "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err = h.Get(c); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[`) {
		t.Errorf("Get 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"edges"`) {
		t.Errorf("Get 响应不应包含 edges 嵌套结构, got %s", rec.Body.String())
	}

	// Get：非法 ID 返回 400
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/bills/abc", "")
	c.SetParamNames("id")
	c.SetParamValues("abc")
	_ = h.Get(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 ID 应返回 400, got %d", rec.Code)
	}

	// 取出计费明细 ID 用于 ToggleWaive
	full, err := billSvc.Get(ctx, billID)
	if err != nil || len(full.Edges.Items) == 0 {
		t.Fatalf("查询账单明细失败: %v", err)
	}
	itemID := full.Edges.Items[0].ID
	itemIDStr := strconv.Itoa(itemID)

	// ToggleWaive：非法 itemId 返回 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/items/abc/waive", "")
	c.SetParamNames("id", "itemId")
	c.SetParamValues(billIDStr, "abc")
	_ = h.ToggleWaive(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 itemId 应返回 400, got %d", rec.Code)
	}

	// ToggleWaive：切换明细减免
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/items/"+itemIDStr+"/waive", "")
	c.SetParamNames("id", "itemId")
	c.SetParamValues(billIDStr, itemIDStr)
	if err = h.ToggleWaive(c); err != nil {
		t.Fatalf("ToggleWaive 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("ToggleWaive 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Get：减免后明细金额归零，零值字段不应被 ent 的 omitempty 省略
	c, rec = newDemandTestContext(e, http.MethodGet, "/api/bills/"+billIDStr, "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err = h.Get(c); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"amount":0`) {
		t.Errorf("减免后应保留零值 amount 字段, got %s", rec.Body.String())
	}

	// Confirm：确认账单，人工确认固定 auto=false
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/confirm", "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err = h.Confirm(c); err != nil {
		t.Fatalf("Confirm 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Confirm 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	final, err := billSvc.Get(ctx, billID)
	if err != nil {
		t.Fatalf("查询最终状态失败: %v", err)
	}
	if final.Status.String() != "unpaid" {
		t.Errorf("最终状态 = %s, want unpaid", final.Status)
	}
	if final.ConfirmAuto {
		t.Error("人工确认 ConfirmAuto 应为 false")
	}
}

// TestBillManualAndItemsHandlers 覆盖手动生成、可选需求、加/移项、标记支付接口
func TestBillManualAndItemsHandlers(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hbillmanual?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	h := NewBill(billSvc)
	e := echo.New()

	// 准备两个已验收需求
	act := service.Actor{ID: 1, Name: "管理员"}
	mk := func(title string, halfDays int) int {
		d, _ := demandSvc.Create(ctx, act, title, "", 0, nil, false, nil, "")
		_ = demandSvc.SubmitEstimate(ctx, act, d.ID, halfDays, nil)
		_ = demandSvc.ConfirmEstimate(ctx, act, d.ID)
		start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
		end := time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)
		_ = demandSvc.Start(ctx, act, d.ID, start)
		_ = demandSvc.Finish(ctx, act, d.ID, start, end, halfDays)
		_ = demandSvc.Accept(ctx, act, d.ID, false, false)
		return d.ID
	}
	id1 := mk("结算需求一", 2)
	id2 := mk("结算需求二", 4)

	// SelectableDemands：两个需求都可计费
	c, rec := newDemandTestContext(e, http.MethodGet, "/api/bills/selectable-demands", "")
	if err := h.SelectableDemands(c); err != nil {
		t.Fatalf("SelectableDemands 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"billable"`) {
		t.Fatalf("SelectableDemands 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// CreateManual：空名称 400
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/manual", `{"name":"  ","demand_ids":[`+strconv.Itoa(id1)+`]}`)
	_ = h.CreateManual(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空名称应返回 400, got %d", rec.Code)
	}

	// CreateManual：正常创建
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/manual", `{"name":"七月专项结算","demand_ids":[`+strconv.Itoa(id1)+`]}`)
	if err := h.CreateManual(c); err != nil {
		t.Fatalf("CreateManual 失败: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "七月专项结算") {
		t.Fatalf("CreateManual 响应异常: %d, %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			ID     int     `json:"id"`
			Period *string `json:"period"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("CreateManual 响应解析失败: %v", err)
	}
	if created.Data.Period != nil {
		t.Errorf("手动账单 period 应为 null, got %v", *created.Data.Period)
	}
	billIDStr := strconv.Itoa(created.Data.ID)

	// AddItem：加入第二个需求
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/items", `{"demand_id":`+strconv.Itoa(id2)+`}`)
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err := h.AddItem(c); err != nil {
		t.Fatalf("AddItem 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("AddItem 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// RemoveItem：移除刚加入的明细
	full, _ := billSvc.Get(ctx, created.Data.ID)
	var itemID int
	for _, it := range full.Edges.Items {
		if it.DemandID == id2 {
			itemID = it.ID
		}
	}
	itemIDStr := strconv.Itoa(itemID)
	c, rec = newDemandTestContext(e, http.MethodDelete, "/api/bills/"+billIDStr+"/items/"+itemIDStr, "")
	c.SetParamNames("id", "itemId")
	c.SetParamValues(billIDStr, itemIDStr)
	if err := h.RemoveItem(c); err != nil {
		t.Fatalf("RemoveItem 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("RemoveItem 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	// Pay：未确认时 409/422 语义（非 200），确认后成功
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/pay", "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	_ = h.Pay(c)
	if rec.Code == http.StatusOK {
		t.Error("待确认账单标记支付不应成功")
	}

	_ = billSvc.Confirm(ctx, act, created.Data.ID, false)
	c, rec = newDemandTestContext(e, http.MethodPost, "/api/bills/"+billIDStr+"/pay", "")
	c.SetParamNames("id")
	c.SetParamValues(billIDStr)
	if err := h.Pay(c); err != nil {
		t.Fatalf("Pay 失败: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Pay 响应异常: %d, %s", rec.Code, rec.Body.String())
	}

	final, _ := billSvc.Get(ctx, created.Data.ID)
	if final.Status.String() != "paid" {
		t.Errorf("最终状态 = %s, want paid", final.Status)
	}
}
