# Clepsydra 后端实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建 Clepsydra 后端：人天统计、需求状态机、月度账单与自动确认的完整 REST API 服务。

**Architecture:** 单体 Go 服务，分层 `api / task → service → dao → ent`，`workday`、`config`、`logger` 为底层公共包；`main.go` 手写构造函数装配（禁止 DI 框架）；定时任务全部为幂等扫描式。

**Tech Stack:** Go 1.26、echo v4、ent（PostgreSQL，测试用 SQLite 内存）、viper、zerolog + lumberjack、robfig/cron v3、golang-jwt v5、bcrypt。

**Spec:** `.superpowers/specs/2026-08-04-clepsydra-design.md`

## Global Constraints

- 遵循 `/Users/liasica/Golang规范.md`：注释中文且结尾不加句号、逻辑分块加空行与说明注释、`err` 变量复用不重命名、JSON key 禁止中文、低层包禁止引用高层包
- 人天一律用整数「半天数」存储与传输（字段名 `*_half_days`，1 人天 = 2）
- 金额单位为整数「元」；`daily_rate` 必须为偶数（保证 0.5 人天金额为整数），行金额 = `half_days × daily_rate / 2`
- 所有确认/流转/减免操作必须写 AuditLog（只增不改不删）
- 定时任务与扫描必须幂等：判断条件只基于数据状态
- Git 提交遵循 Conventional Commits，禁止 AI 署名
- 提交前 `gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m` 无 issue
- 时区：所有业务时间按服务器本地时区（Asia/Shanghai），日期字段用 `time.Time` 截断到日

---

### Task 1: 配置包 config

**Files:**
- Create: `internal/config/config.go`
- Create: `config.yaml`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load(path string) (*Config, error)`；结构体 `Config{Server, Database, JWT, Admin, Log, Holiday}`（字段见实现）

- [ ] **Step 1: 写失败测试**

```go
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
```

`internal/config/testdata/config.yaml`：

```yaml
server:
  address: ":8080"
  mode: debug
database:
  dsn: "postgres://postgres:postgres@localhost:5432/clepsydra?sslmode=disable"
jwt:
  secret: "test-secret"
  expire: 72h
admin:
  username: admin
  password: admin123
log:
  dir: logs
  max_size: 100
  max_age: 30
holiday:
  file: assets/holidays.json
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config/ -v`
Expected: FAIL（`Load` 未定义）

- [ ] **Step 3: 最小实现**

```go
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
```

同时创建项目根 `config.yaml`（内容同 testdata，DSN 与 secret 按本地环境）。

依赖安装：

```bash
go get github.com/spf13/viper@latest
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: 添加配置加载包"
```

注：根目录 `config.yaml` 含本地凭据，已在 `.gitignore` 追加 `config.yaml`（提交 `../../configs/config.example.yaml` 替代，内容同 testdata）。

---

### Task 2: 日志包 logger

**Files:**
- Create: `internal/logger/logger.go`
- Test: `internal/logger/logger_test.go`

**Interfaces:**
- Consumes: `config.Log`
- Produces: `logger.New(cfg config.Log, debug bool) (zerolog.Logger, *lumberjack.Logger)`；debug 模式输出彩色控制台，release 模式写 `<dir>/clepsydra.log`；返回的 `*lumberjack.Logger` 供 cron 每日调用 `Rotate()`

- [ ] **Step 1: 写失败测试**

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/logger/ -v`
Expected: FAIL（`New` 未定义）

- [ ] **Step 3: 最小实现**

```go
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

	// lumberjack 首次写入时会自动创建日志目录
	rotator := &lumberjack.Logger{
		Filename: filepath.Join(cfg.Dir, "clepsydra.log"),
		MaxSize:  cfg.MaxSize,
		MaxAge:   cfg.MaxAge,
		Compress: true,
	}

	return zerolog.New(rotator).With().Timestamp().Logger(), rotator
}
```

依赖安装：

```bash
go get github.com/rs/zerolog@latest gopkg.in/natefinch/lumberjack.v2@latest
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/logger/ go.mod go.sum
git commit -m "feat: 添加 zerolog 日志包（lumberjack 轮转）"
```

---

### Task 3: ent schema 与代码生成

**Files:**
- Create: `internal/ent/generate.go`
- Create: `internal/ent/schema/user.go`
- Create: `internal/ent/schema/demand.go`
- Create: `internal/ent/schema/bill.go`
- Create: `internal/ent/schema/billitem.go`
- Create: `internal/ent/schema/setting.go`
- Create: `internal/ent/schema/auditlog.go`
- Create: `internal/ent/schema/holiday.go`
- Test: `internal/ent/smoke_test.go`

**Interfaces:**
- Produces: 生成的 `clepsydra/internal/ent` 客户端；状态枚举值（Demand: `draft/pending_estimate/confirmed/in_progress/pending_acceptance/accepted`；Bill: `draft/pending/confirmed`；User.role: `admin/client`；Holiday.type: `holiday/workday`）。后续所有任务经由 `ent.Client` 访问数据。

- [ ] **Step 1: 写 schema（核心即约束，schema 先行无独立失败测试，以冒烟测试收口）**

`internal/ent/generate.go`：

```go
package ent

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema
```

`internal/ent/schema/user.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// User 系统用户，admin 为超级管理员，client 为需求方
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").Unique(),
		field.String("password_hash").Sensitive(),
		field.String("name"),
		field.Enum("role").Values("admin", "client"),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
```

`internal/ent/schema/demand.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Demand 开发需求（项目），人天以半天数存储
type Demand struct {
	ent.Schema
}

func (Demand) Fields() []ent.Field {
	return []ent.Field{
		field.String("title"),
		field.Text("description").Optional(),
		field.Int("estimated_half_days").NonNegative(),
		field.Time("estimate_confirmed_at").Optional().Nillable(),
		field.Int("estimate_confirmed_by").Optional().Nillable(),
		field.Time("planned_start_date").Optional().Nillable(),
		field.Time("actual_start_date").Optional().Nillable(),
		field.Time("actual_end_date").Optional().Nillable(),
		field.Int("actual_half_days").Optional().Nillable(),
		field.Enum("status").
			Values("draft", "pending_estimate", "confirmed", "in_progress", "pending_acceptance", "accepted").
			Default("draft"),
		field.Time("accept_deadline").Optional().Nillable(),
		field.Time("accepted_at").Optional().Nillable(),
		field.Int("accepted_by").Optional().Nillable(),
		field.Bool("accept_auto").Default(false),
		field.Bool("accept_locked").Default(false), // 出账前锁定产生的自动确认
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Demand) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("actual_end_date"),
	}
}
```

`internal/ent/schema/bill.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Bill 月度账单，period 形如 2026-07，全局唯一
type Bill struct {
	ent.Schema
}

func (Bill) Fields() []ent.Field {
	return []ent.Field{
		field.String("period").Unique(),
		field.Enum("status").Values("draft", "pending", "confirmed").Default("draft"),
		field.Int("daily_rate"), // 生成时快照，单位元
		field.Int("base_fee"),   // 生成时快照，单位元
		field.Int("total_half_days"),
		field.Int("total_amount"), // 单位元
		field.Time("shared_at").Optional().Nillable(),
		field.Time("confirm_deadline").Optional().Nillable(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.Int("confirmed_by").Optional().Nillable(),
		field.Bool("confirm_auto").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Bill) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", BillItem.Type),
	}
}
```

`internal/ent/schema/billitem.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// BillItem 账单明细行，快照生成时的需求信息
type BillItem struct {
	ent.Schema
}

func (BillItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int("demand_id"),
		field.String("demand_title"),   // 快照
		field.String("demand_status"),  // 快照
		field.Int("half_days"),         // 计费行为实际人天，展示行为预估人天
		field.Int("amount"),            // 单位元，展示行与减免行为 0
		field.Bool("billable"),         // true 计费行，false 展示行
		field.Bool("waived").Default(false),
		field.Time("planned_start_date").Optional().Nillable(), // 快照
		field.String("note").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (BillItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("bill", Bill.Type).Ref("items").Unique().Required(),
	}
}
```

`internal/ent/schema/setting.go`：

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Setting 键值设置
type Setting struct {
	ent.Schema
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique(),
		field.String("value"),
	}
}
```

`internal/ent/schema/auditlog.go`：

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog 审计日志，只增不改不删，是合同效力的操作依据
type AuditLog struct {
	ent.Schema
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("actor_id"),      // 0 表示系统自动操作
		field.String("actor_name"), // 快照，系统操作为 system
		field.String("action"),     // 如 demand.accept、bill.share
		field.String("target_type"),
		field.Int("target_id"),
		field.JSON("detail", map[string]any{}).Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_type", "target_id"),
	}
}
```

`internal/ent/schema/holiday.go`：

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Holiday 节假日与调休补班日
type Holiday struct {
	ent.Schema
}

func (Holiday) Fields() []ent.Field {
	return []ent.Field{
		field.String("date").Unique(), // 格式 2026-01-01
		field.Enum("type").Values("holiday", "workday"),
		field.String("name").Optional(),
	}
}
```

- [ ] **Step 2: 生成代码**

```bash
go get entgo.io/ent@latest
go generate ./internal/ent/...
```

Expected: 生成 `internal/ent/` 下客户端代码，无报错

- [ ] **Step 3: 写冒烟测试**

`internal/ent/smoke_test.go`：

```go
package ent_test

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/enttest"
)

func TestSchemaSmoke(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()

	// 创建需求并流转状态，验证枚举与默认值
	d := client.Demand.Create().
		SetTitle("测试需求").
		SetEstimatedHalfDays(4).
		SaveX(ctx)
	if d.Status.String() != "draft" {
		t.Errorf("默认状态 = %s, want draft", d.Status)
	}

	// 账单账期唯一约束
	client.Bill.Create().SetPeriod("2026-07").SetDailyRate(1200).SetBaseFee(12000).
		SetTotalHalfDays(0).SetTotalAmount(12000).SaveX(ctx)
	_, err := client.Bill.Create().SetPeriod("2026-07").SetDailyRate(1200).SetBaseFee(12000).
		SetTotalHalfDays(0).SetTotalAmount(12000).Save(ctx)
	if err == nil {
		t.Error("重复账期应违反唯一约束")
	}

	_ = ent.Asc
}
```

依赖安装：

```bash
go get github.com/mattn/go-sqlite3@latest
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/ent/ -run TestSchemaSmoke -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/ent/ go.mod go.sum
git commit -m "feat: 添加 ent schema 与生成代码"
```

---

### Task 4: workday 包（工作日/出账日/窗口计算）

**Files:**
- Create: `internal/workday/workday.go`
- Create: `assets/holidays.json`
- Test: `internal/workday/workday_test.go`

**Interfaces:**
- Produces:
  - `type Entry struct { Date string \`json:"date"\`; Type string \`json:"type"\`; Name string \`json:"name"\` }`（Type 取值 `holiday`/`workday`）
  - `type Unit string`（`UnitNatural Unit = "natural"`、`UnitWorkday Unit = "workday"`）
  - `func New(entries []Entry, saturdayAsWorkday bool) *Calendar`
  - `func (c *Calendar) IsWorkday(d time.Time) bool`
  - `func (c *Calendar) BillingDueDate(year int, month time.Month) time.Time` — 返回该月出账截止日
  - `func (c *Calendar) Deadline(start time.Time, days int, unit Unit) time.Time` — 返回确认截止时间

- [ ] **Step 1: 写失败测试（表驱动，覆盖 spec 要求的边界）**

```go
package workday

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	d, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return d
}

// 测试用日历：假设 10-01 至 10-03 为节假日，10-11（周日）为调休补班日
func testCalendar(saturdayAsWorkday bool) *Calendar {
	entries := []Entry{
		{Date: "2026-10-01", Type: "holiday", Name: "国庆节"},
		{Date: "2026-10-02", Type: "holiday", Name: "国庆节"},
		{Date: "2026-10-03", Type: "holiday", Name: "国庆节"},
		{Date: "2026-10-11", Type: "workday", Name: "国庆调休"},
	}
	return New(entries, saturdayAsWorkday)
}

func TestIsWorkday(t *testing.T) {
	c := testCalendar(true)

	cases := []struct {
		name string
		day  string
		want bool
	}{
		{"普通周一", "2026-10-05", true},
		{"节假日", "2026-10-01", false},
		{"周六算工作日", "2026-10-10", true},
		{"普通周日", "2026-10-04", false},
		{"调休补班周日", "2026-10-11", true},
	}
	for _, tc := range cases {
		if got := c.IsWorkday(date(tc.day)); got != tc.want {
			t.Errorf("%s IsWorkday(%s) = %v, want %v", tc.name, tc.day, got, tc.want)
		}
	}

	// 周六不算工作日的口径
	c2 := testCalendar(false)
	if c2.IsWorkday(date("2026-10-10")) {
		t.Error("saturdayAsWorkday=false 时周六不应算工作日")
	}
}

func TestBillingDueDate(t *testing.T) {
	// 10 号是周六且为工作日 → 直接取 10 号
	c := testCalendar(true)
	if got := c.BillingDueDate(2026, time.October); !got.Equal(date("2026-10-10")) {
		t.Errorf("BillingDueDate = %s, want 2026-10-10", got.Format("2006-01-02"))
	}

	// 构造 10 号为节假日的场景：9 号也是节假日 → 应取 8 号
	entries := []Entry{
		{Date: "2026-10-09", Type: "holiday"},
		{Date: "2026-10-10", Type: "holiday"},
	}
	c3 := New(entries, true)
	if got := c3.BillingDueDate(2026, time.October); !got.Equal(date("2026-10-08")) {
		t.Errorf("BillingDueDate = %s, want 2026-10-08", got.Format("2006-01-02"))
	}
}

func TestDeadline(t *testing.T) {
	c := testCalendar(true)
	start := date("2026-09-28")

	// 自然日：直接 +5 天
	if got := c.Deadline(start, 5, UnitNatural); !got.Equal(date("2026-10-03")) {
		t.Errorf("自然日 Deadline = %s, want 2026-10-03", got.Format("2006-01-02"))
	}

	// 工作日：跳过 10-01 至 10-03 节假日与 10-04 周日
	// 09-29(二) 09-30(三) 10-05(一) 10-06(二) 10-07(三) → 第 5 个工作日为 10-07
	if got := c.Deadline(start, 5, UnitWorkday); !got.Equal(date("2026-10-07")) {
		t.Errorf("工作日 Deadline = %s, want 2026-10-07", got.Format("2006-01-02"))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/workday/ -v`
Expected: FAIL（类型与函数未定义）

- [ ] **Step 3: 最小实现**

```go
package workday

import "time"

// Entry 节假日数据条目，type 为 holiday 表示放假、workday 表示调休补班
type Entry struct {
	Date string `json:"date"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// Unit 时限窗口口径
type Unit string

const (
	UnitNatural Unit = "natural" // 自然日
	UnitWorkday Unit = "workday" // 工作日
)

const dateLayout = "2006-01-02"

// Calendar 工作日日历，基于节假日数据与周六口径判定
type Calendar struct {
	holidays          map[string]bool
	makeupDays        map[string]bool
	saturdayAsWorkday bool
}

// New 构建日历
func New(entries []Entry, saturdayAsWorkday bool) *Calendar {
	c := &Calendar{
		holidays:          make(map[string]bool),
		makeupDays:        make(map[string]bool),
		saturdayAsWorkday: saturdayAsWorkday,
	}

	for _, e := range entries {
		switch e.Type {
		case "holiday":
			c.holidays[e.Date] = true
		case "workday":
			c.makeupDays[e.Date] = true
		}
	}

	return c
}

// IsWorkday 判定某天是否为工作日
// 规则：节假日一律休息；调休补班日一律上班；否则周一至周五上班，周六按口径，周日休息
// 入参先规范化到本地时区，避免数据库读出的 UTC 时间导致跨日误判
func (c *Calendar) IsWorkday(d time.Time) bool {
	d = d.In(time.Local)
	key := d.Format(dateLayout)

	if c.holidays[key] {
		return false
	}
	if c.makeupDays[key] {
		return true
	}

	switch d.Weekday() {
	case time.Sunday:
		return false
	case time.Saturday:
		return c.saturdayAsWorkday
	default:
		return true
	}
}

// BillingDueDate 计算某月的出账截止日
// 默认 10 号，若非工作日则从 10 号起逐日向前取第一个工作日
// 业务规则限定在 1-10 号范围内，极端配置下最多回溯到 1 号
func (c *Calendar) BillingDueDate(year int, month time.Month) time.Time {
	d := time.Date(year, month, 10, 0, 0, 0, 0, time.Local)

	for d.Day() > 1 && !c.IsWorkday(d) {
		d = d.AddDate(0, 0, -1)
	}

	return d
}

// Deadline 计算确认截止日期
// 自然日口径直接加 days 天；工作日口径逐日累计 days 个工作日
// 入参先规范化到本地时区，保证与 IsWorkday 的日界一致
func (c *Calendar) Deadline(start time.Time, days int, unit Unit) time.Time {
	start = start.In(time.Local)
	if unit == UnitNatural {
		return start.AddDate(0, 0, days)
	}

	d := start
	remain := days
	for remain > 0 {
		d = d.AddDate(0, 0, 1)
		if c.IsWorkday(d) {
			remain--
		}
	}

	return d
}
```

`assets/holidays.json`（初始数据，**须按国务院办公厅 2026 年放假安排核对补全**，系统运行后也可在设置中心维护）：

```json
[
  {"date": "2026-01-01", "type": "holiday", "name": "元旦"}
]
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/workday/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/workday/ assets/
git commit -m "feat: 添加工作日与出账日计算包"
```

---

### Task 5: API 基座与登录

**Files:**
- Create: `internal/api/response.go`
- Create: `internal/api/middleware.go`
- Create: `internal/service/errors.go`
- Create: `internal/service/auth.go`
- Create: `internal/api/handler/auth.go`
- Test: `internal/service/auth_test.go`
- Test: `internal/api/middleware_test.go`

**Interfaces:**
- Consumes: `ent.Client`、`config.JWT`
- Produces:
  - `service.ErrNotFound`、`service.ErrForbidden`、`service.ErrInvalidTransition`、`service.ErrBadRequest(msg)`（`*service.Error{Code, Message}` 实现 `error`）
  - `service.NewAuth(client *ent.Client, cfg config.JWT) *Auth`；`(*Auth).Login(ctx, username, password) (token string, user *ent.User, err error)`；`(*Auth).ParseToken(token string) (*Claims, error)`，`Claims{UserID int, Role string}`
  - `api.OK(c echo.Context, data any)` / `api.Fail(c, err)` 统一响应：`{"code": 0, "message": "ok", "data": ...}`，业务错误 code 非 0
  - `api.RequireAuth(auth *service.Auth) echo.MiddlewareFunc`（注入 `claims` 到 context）、`api.RequireAdmin`（校验 `claims.Role == "admin"`）

- [ ] **Step 1: 写失败测试**

`internal/service/auth_test.go`：

```go
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
```

`internal/api/middleware_test.go`：

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ ./internal/api/ -v`
Expected: FAIL（类型与函数未定义）

- [ ] **Step 3: 实现**

`internal/service/errors.go`：

```go
package service

import "fmt"

// Error 业务错误，Code 用于前端分支处理
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

var (
	ErrNotFound          = &Error{Code: 40400, Message: "资源不存在"}
	ErrForbidden         = &Error{Code: 40300, Message: "无权限操作"}
	ErrUnauthorized      = &Error{Code: 40100, Message: "未登录或凭证失效"}
	ErrInvalidTransition = &Error{Code: 42200, Message: "当前状态不允许该操作"}
)

// ErrBadRequest 构造参数错误
func ErrBadRequest(msg string) *Error {
	return &Error{Code: 40000, Message: msg}
}
```

`internal/service/auth.go`：

```go
package service

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/user"
)

// Claims JWT 载荷，Name 供审计记录操作者姓名，避免每次查库
type Claims struct {
	UserID int    `json:"uid"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

// Auth 认证服务
type Auth struct {
	client *ent.Client
	cfg    config.JWT
}

// NewAuth 构建认证服务
func NewAuth(client *ent.Client, cfg config.JWT) *Auth {
	return &Auth{client: client, cfg: cfg}
}

// Login 校验用户名密码，通过后签发 JWT
func (a *Auth) Login(ctx context.Context, username, password string) (string, *ent.User, error) {
	u, err := a.client.User.Query().Where(user.Username(username), user.Enabled(true)).Only(ctx)
	if err != nil {
		return "", nil, ErrBadRequest("用户名或密码错误")
	}

	if err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrBadRequest("用户名或密码错误")
	}

	claims := Claims{
		UserID: u.ID,
		Role:   u.Role.String(),
		Name:   u.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.cfg.Expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.Secret))
	if err != nil {
		return "", nil, err
	}

	return token, u, nil
}

// ParseToken 解析并校验 JWT
func (a *Auth) ParseToken(token string) (*Claims, error) {
	claims := new(Claims)

	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return []byte(a.cfg.Secret), nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrUnauthorized
	}

	return claims, nil
}
```

`internal/api/response.go`：

```go
package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/service"
)

// body 统一响应结构
type body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 成功响应
func OK(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, body{Code: 0, Message: "ok", Data: data})
}

// Fail 失败响应，业务错误映射 HTTP 状态，其余按 500 处理
func Fail(c echo.Context, err error) error {
	var svcErr *service.Error
	if errors.As(err, &svcErr) {
		return c.JSON(svcErr.Code/100, body{Code: svcErr.Code, Message: svcErr.Message})
	}

	return c.JSON(http.StatusInternalServerError, body{Code: 50000, Message: "服务器内部错误"})
}
```

`internal/api/middleware.go`：

```go
package api

import (
	"strings"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/service"
)

const claimsKey = "claims"

// RequireAuth 解析 Bearer token 并注入 claims
func RequireAuth(auth *service.Auth) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				return Fail(c, service.ErrUnauthorized)
			}

			claims, err := auth.ParseToken(token)
			if err != nil {
				return Fail(c, err)
			}

			c.Set(claimsKey, claims)

			return next(c)
		}
	}
}

// RequireAdmin 仅允许超级管理员
func RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if Claims(c).Role != "admin" {
			return Fail(c, service.ErrForbidden)
		}

		return next(c)
	}
}

// Claims 从 context 取出登录信息
func Claims(c echo.Context) *service.Claims {
	claims, _ := c.Get(claimsKey).(*service.Claims)
	return claims
}
```

`internal/api/handler/auth.go`：

```go
package handler

import (
	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// Auth 认证相关接口
type Auth struct {
	svc *service.Auth
}

// NewAuth 构建认证 handler
func NewAuth(svc *service.Auth) *Auth {
	return &Auth{svc: svc}
}

// Login POST /api/auth/login
func (h *Auth) Login(c echo.Context) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	token, user, err := h.svc.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, map[string]any{
		"token": token,
		"user": map[string]any{
			"id":   user.ID,
			"name": user.Name,
			"role": user.Role,
		},
	})
}
```

依赖安装：

```bash
go get github.com/labstack/echo/v4@latest github.com/golang-jwt/jwt/v5@latest golang.org/x/crypto@latest
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ ./internal/api/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/ internal/api/ go.mod go.sum
git commit -m "feat: 添加 API 基座、认证服务与登录接口"
```

---

### Task 6: 数据种子与用户管理

**Files:**
- Create: `internal/service/seed.go`
- Create: `internal/service/user.go`
- Create: `internal/api/handler/user.go`
- Test: `internal/service/seed_test.go`
- Test: `internal/service/user_test.go`

**Interfaces:**
- Consumes: `ent.Client`、`config.Admin`、`workday.Entry`
- Produces:
  - `service.Seed(ctx, client, adminCfg, holidayEntries) error` — 幂等：admin 用户不存在则创建；默认设置项缺失则补齐；节假日数据按 date upsert
  - 默认设置常量：`SettingBillIncludeStatuses = "bill_include_statuses"`（默认 `"accepted,in_progress,confirmed"`）、`SettingDemandConfirmWindow = "demand_confirm_window"`（`"5"`）、`SettingBillConfirmWindow = "bill_confirm_window"`（`"3"`）、`SettingWindowUnit = "window_unit"`（`"natural"`）、`SettingDailyRate = "daily_rate"`（`"1200"`）、`SettingBaseFee = "base_fee"`（`"12000"`）、`SettingSaturdayAsWorkday = "saturday_as_workday"`（`"true"`）
  - `service.NewUser(client) *User`；方法 `List(ctx) ([]*ent.User, error)`、`Create(ctx, username, password, name, role string) (*ent.User, error)`、`Update(ctx, id int, name string, enabled bool) (*ent.User, error)`、`ResetPassword(ctx, id int, password string) error`

- [ ] **Step 1: 写失败测试**

`internal/service/seed_test.go`：

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/ent/setting"
	"clepsydra/internal/ent/user"
	"clepsydra/internal/workday"
)

func TestSeedIdempotent(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:seed?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	adminCfg := config.Admin{Username: "admin", Password: "admin123"}
	entries := []workday.Entry{{Date: "2026-01-01", Type: "holiday", Name: "元旦"}}

	// 执行两次验证幂等
	if err := Seed(ctx, client, adminCfg, entries); err != nil {
		t.Fatalf("首次种子失败: %v", err)
	}
	if err := Seed(ctx, client, adminCfg, entries); err != nil {
		t.Fatalf("二次种子失败: %v", err)
	}

	// admin 只有一个
	if n := client.User.Query().Where(user.Username("admin")).CountX(ctx); n != 1 {
		t.Errorf("admin 数量 = %d, want 1", n)
	}

	// 默认设置齐全
	if n := client.Setting.Query().CountX(ctx); n != 7 {
		t.Errorf("设置项数量 = %d, want 7", n)
	}
	rate := client.Setting.Query().Where(setting.Key(SettingDailyRate)).OnlyX(ctx)
	if rate.Value != "1200" {
		t.Errorf("默认单价 = %s, want 1200", rate.Value)
	}

	// 节假日已导入
	if n := client.Holiday.Query().CountX(ctx); n != 1 {
		t.Errorf("节假日数量 = %d, want 1", n)
	}
}
```

`internal/service/user_test.go`：

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/ent/enttest"
)

func TestUserCRUD(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:usercrud?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	svc := NewUser(client)

	// 创建需求方用户
	u, err := svc.Create(ctx, "jiafang", "pass1234", "甲方对接人", "client")
	if err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	// 用户名重复拒绝
	if _, err = svc.Create(ctx, "jiafang", "x", "重复", "client"); err == nil {
		t.Error("重复用户名应拒绝")
	}

	// 非法角色拒绝
	if _, err = svc.Create(ctx, "bad", "x", "非法角色", "root"); err == nil {
		t.Error("非法角色应拒绝")
	}

	// 更新与禁用
	u, err = svc.Update(ctx, u.ID, "新名字", false)
	if err != nil || u.Name != "新名字" || u.Enabled {
		t.Errorf("更新失败: %v, %+v", err, u)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run 'TestSeed|TestUserCRUD' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/service/seed.go`：

```go
package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/holiday"
	"clepsydra/internal/ent/setting"
	"clepsydra/internal/ent/user"
	"clepsydra/internal/workday"
)

// 设置项 key 常量，全项目唯一入口
const (
	SettingBillIncludeStatuses = "bill_include_statuses"
	SettingDemandConfirmWindow = "demand_confirm_window"
	SettingBillConfirmWindow   = "bill_confirm_window"
	SettingWindowUnit          = "window_unit"
	SettingDailyRate           = "daily_rate"
	SettingBaseFee             = "base_fee"
	SettingSaturdayAsWorkday   = "saturday_as_workday"
)

// defaultSettings 默认设置值
var defaultSettings = map[string]string{
	SettingBillIncludeStatuses: "accepted,in_progress,confirmed",
	SettingDemandConfirmWindow: "5",
	SettingBillConfirmWindow:   "3",
	SettingWindowUnit:          "natural",
	SettingDailyRate:           "1200",
	SettingBaseFee:             "12000",
	SettingSaturdayAsWorkday:   "true",
}

// Seed 初始化基础数据，幂等可重复执行
func Seed(ctx context.Context, client *ent.Client, adminCfg config.Admin, entries []workday.Entry) error {
	// 初始管理员：不存在才创建
	exists, err := client.User.Query().Where(user.Username(adminCfg.Username)).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminCfg.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err = client.User.Create().
			SetUsername(adminCfg.Username).
			SetPasswordHash(string(hash)).
			SetName("超级管理员").
			SetRole("admin").
			Save(ctx); err != nil {
			return err
		}
	}

	// 默认设置：缺失才补齐
	for key, value := range defaultSettings {
		exists, err = client.Setting.Query().Where(setting.Key(key)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			if _, err = client.Setting.Create().SetKey(key).SetValue(value).Save(ctx); err != nil {
				return err
			}
		}
	}

	// 节假日：按日期 upsert
	for _, e := range entries {
		exists, err = client.Holiday.Query().Where(holiday.Date(e.Date)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err = client.Holiday.Create().
			SetDate(e.Date).
			SetType(holiday.Type(e.Type)).
			SetName(e.Name).
			Save(ctx); err != nil {
			return err
		}
	}

	return nil
}
```

`internal/service/user.go`：

```go
package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/user"
)

// User 用户管理服务
type User struct {
	client *ent.Client
}

// NewUser 构建用户服务
func NewUser(client *ent.Client) *User {
	return &User{client: client}
}

// List 查询全部用户
func (s *User) List(ctx context.Context) ([]*ent.User, error) {
	return s.client.User.Query().Order(ent.Asc(user.FieldID)).All(ctx)
}

// Create 创建用户，角色仅允许 admin 或 client
func (s *User) Create(ctx context.Context, username, password, name, role string) (*ent.User, error) {
	if role != "admin" && role != "client" {
		return nil, ErrBadRequest("角色不合法")
	}
	if username == "" || len(password) < 6 {
		return nil, ErrBadRequest("用户名不能为空且密码至少 6 位")
	}

	exists, err := s.client.User.Query().Where(user.Username(username)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("用户名已存在")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return s.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetName(name).
		SetRole(user.Role(role)).
		Save(ctx)
}

// Update 更新用户姓名与启用状态
func (s *User) Update(ctx context.Context, id int, name string, enabled bool) (*ent.User, error) {
	u, err := s.client.User.UpdateOneID(id).SetName(name).SetEnabled(enabled).Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return u, err
}

// ResetPassword 重置用户密码
func (s *User) ResetPassword(ctx context.Context, id int, password string) error {
	if len(password) < 6 {
		return ErrBadRequest("密码至少 6 位")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	err = s.client.User.UpdateOneID(id).SetPasswordHash(string(hash)).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}

	return err
}
```

`internal/api/handler/user.go` — 按以下精确规格实现（模式与 Task 5 的 `handler/auth.go` 一致：Bind 请求 → 调 service → `api.OK`/`api.Fail`）：

| 方法 | 路由 | 请求体 | 调用 |
|---|---|---|---|
| `List` | GET `/api/users` | — | `svc.List` |
| `Create` | POST `/api/users` | `{"username","password","name","role"}` | `svc.Create` |
| `Update` | PUT `/api/users/:id` | `{"name","enabled"}` | `svc.Update`（`:id` 用 `strconv.Atoi`，失败返回 `ErrBadRequest("ID 不合法")`） |
| `ResetPassword` | PUT `/api/users/:id/password` | `{"password"}` | `svc.ResetPassword` |

响应中禁止返回 `password_hash`（ent Sensitive 已保证序列化排除，handler 直接返回 ent 实体即可）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/ internal/api/handler/
git commit -m "feat: 添加数据种子与用户管理"
```

---

### Task 7: 设置与节假日服务

**Files:**
- Create: `internal/service/setting.go`
- Create: `internal/service/holidaysvc.go`
- Create: `internal/api/handler/setting.go`
- Test: `internal/service/setting_test.go`

**Interfaces:**
- Consumes: 设置 key 常量（Task 6）、`workday.Entry`
- Produces:
  - `service.NewSetting(client) *Setting`；方法：`All(ctx) (map[string]string, error)`、`Update(ctx, values map[string]string) error`（仅允许已知 key；`daily_rate` 必须为正偶数；`window_unit` 仅 natural/workday；`demand_confirm_window`/`bill_confirm_window` 为正整数；`base_fee` 非负整数；`saturday_as_workday` 为 true/false；`bill_include_statuses` 为合法状态逗号集合）
  - 类型化读取：`(*Setting).Int(ctx, key) (int, error)`、`(*Setting).Bool(ctx, key) (bool, error)`、`(*Setting).Str(ctx, key) (string, error)`
  - `(*Setting).Calendar(ctx) (*workday.Calendar, error)` — 从 Holiday 表 + `saturday_as_workday` 构建日历（后续 demand/bill/task 统一从这里拿日历）
  - `service.NewHolidaySvc(client) *HolidaySvc`；方法：`List(ctx, year string) ([]*ent.Holiday, error)`、`Save(ctx, entries []workday.Entry) error`（按 date upsert，type 可覆盖更新）、`Delete(ctx, date string) error`

- [ ] **Step 1: 写失败测试**

```go
package service

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/workday"
)

func TestSettingValidation(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:settingv?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	svc := NewSetting(client)

	// 未知 key 拒绝
	if err := svc.Update(ctx, map[string]string{"unknown": "1"}); err == nil {
		t.Error("未知设置 key 应拒绝")
	}

	// 奇数单价拒绝（0.5 人天金额必须为整数）
	if err := svc.Update(ctx, map[string]string{SettingDailyRate: "1201"}); err == nil {
		t.Error("奇数单价应拒绝")
	}

	// 合法更新生效
	if err := svc.Update(ctx, map[string]string{SettingDailyRate: "1400"}); err != nil {
		t.Fatalf("合法更新失败: %v", err)
	}
	rate, err := svc.Int(ctx, SettingDailyRate)
	if err != nil || rate != 1400 {
		t.Errorf("单价 = %d, %v", rate, err)
	}

	// 非法窗口口径拒绝
	if err = svc.Update(ctx, map[string]string{SettingWindowUnit: "lunar"}); err == nil {
		t.Error("非法口径应拒绝")
	}
}

func TestSettingCalendar(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:settingc?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	entries := []workday.Entry{{Date: "2026-10-01", Type: "holiday", Name: "国庆节"}}
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, entries); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	cal, err := NewSetting(client).Calendar(ctx)
	if err != nil {
		t.Fatalf("构建日历失败: %v", err)
	}

	d, _ := timeParse("2026-10-01")
	if cal.IsWorkday(d) {
		t.Error("节假日不应为工作日")
	}
}
```

测试辅助（加在同文件）：

```go
func timeParse(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.Local)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestSetting -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/service/setting.go`：

```go
package service

import (
	"context"
	"strconv"
	"strings"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/holiday"
	"clepsydra/internal/ent/setting"
	"clepsydra/internal/workday"
)

// Setting 设置服务，负责设置读写校验与工作日日历构建
type Setting struct {
	client *ent.Client
}

// NewSetting 构建设置服务
func NewSetting(client *ent.Client) *Setting {
	return &Setting{client: client}
}

// validDemandStatuses 账单可包含的需求状态合法值
var validDemandStatuses = map[string]bool{
	"draft": true, "pending_estimate": true, "confirmed": true,
	"in_progress": true, "pending_acceptance": true, "accepted": true,
}

// validate 校验单个设置值
func validate(key, value string) error {
	switch key {
	case SettingDailyRate:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 || n%2 != 0 {
			return ErrBadRequest("单价必须为正偶数")
		}
	case SettingBaseFee:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return ErrBadRequest("基础维护费必须为非负整数")
		}
	case SettingDemandConfirmWindow, SettingBillConfirmWindow:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return ErrBadRequest("确认窗口必须为正整数")
		}
	case SettingWindowUnit:
		if value != string(workday.UnitNatural) && value != string(workday.UnitWorkday) {
			return ErrBadRequest("窗口口径仅支持 natural 或 workday")
		}
	case SettingSaturdayAsWorkday:
		if value != "true" && value != "false" {
			return ErrBadRequest("周六口径仅支持 true 或 false")
		}
	case SettingBillIncludeStatuses:
		for _, s := range strings.Split(value, ",") {
			if !validDemandStatuses[strings.TrimSpace(s)] {
				return ErrBadRequest("包含非法的需求状态: " + s)
			}
		}
	default:
		return ErrBadRequest("未知设置项: " + key)
	}

	return nil
}

// All 读取全部设置
func (s *Setting) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.client.Setting.Query().All(ctx)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}

	return values, nil
}

// Update 批量更新设置，全部校验通过后逐项写入
func (s *Setting) Update(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if err := validate(key, value); err != nil {
			return err
		}
	}

	for key, value := range values {
		err := s.client.Setting.Update().Where(setting.Key(key)).SetValue(value).Exec(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Str 读取字符串设置
func (s *Setting) Str(ctx context.Context, key string) (string, error) {
	row, err := s.client.Setting.Query().Where(setting.Key(key)).Only(ctx)
	if err != nil {
		return "", err
	}

	return row.Value, nil
}

// Int 读取整数设置
func (s *Setting) Int(ctx context.Context, key string) (int, error) {
	value, err := s.Str(ctx, key)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(value)
}

// Bool 读取布尔设置
func (s *Setting) Bool(ctx context.Context, key string) (bool, error) {
	value, err := s.Str(ctx, key)
	if err != nil {
		return false, err
	}

	return value == "true", nil
}

// Calendar 从节假日表与周六口径构建工作日日历
func (s *Setting) Calendar(ctx context.Context) (*workday.Calendar, error) {
	rows, err := s.client.Holiday.Query().Order(ent.Asc(holiday.FieldDate)).All(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]workday.Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, workday.Entry{Date: row.Date, Type: row.Type.String(), Name: row.Name})
	}

	saturday, err := s.Bool(ctx, SettingSaturdayAsWorkday)
	if err != nil {
		return nil, err
	}

	return workday.New(entries, saturday), nil
}
```

`internal/service/holidaysvc.go`：

```go
package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/holiday"
	"clepsydra/internal/workday"
)

// HolidaySvc 节假日维护服务
type HolidaySvc struct {
	client *ent.Client
}

// NewHolidaySvc 构建节假日服务
func NewHolidaySvc(client *ent.Client) *HolidaySvc {
	return &HolidaySvc{client: client}
}

// List 按年份查询节假日，year 为空返回全部
func (s *HolidaySvc) List(ctx context.Context, year string) ([]*ent.Holiday, error) {
	q := s.client.Holiday.Query().Order(ent.Asc(holiday.FieldDate))
	if year != "" {
		q = q.Where(holiday.DateHasPrefix(year + "-"))
	}

	return q.All(ctx)
}

// Save 批量保存节假日，先整体校验再写入，已存在的日期覆盖更新类型与名称
// 日期必须为零填充的 YYYY-MM-DD，否则与日历 map key 格式不一致导致节假日静默失效
func (s *HolidaySvc) Save(ctx context.Context, entries []workday.Entry) error {
	// 先整体校验，任一条目非法则全部拒绝，与 Setting.Update 的原子语义一致
	for _, e := range entries {
		if e.Type != "holiday" && e.Type != "workday" {
			return ErrBadRequest("类型仅支持 holiday 或 workday")
		}
		if _, err := time.ParseInLocation("2006-01-02", e.Date, time.Local); err != nil {
			return ErrBadRequest("日期格式必须为 YYYY-MM-DD: " + e.Date)
		}
	}

	for _, e := range entries {
		existing, err := s.client.Holiday.Query().Where(holiday.Date(e.Date)).Only(ctx)
		if ent.IsNotFound(err) {
			if _, err = s.client.Holiday.Create().
				SetDate(e.Date).SetType(holiday.Type(e.Type)).SetName(e.Name).Save(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}

		if _, err = existing.Update().SetType(holiday.Type(e.Type)).SetName(e.Name).Save(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Delete 删除某个节假日条目
func (s *HolidaySvc) Delete(ctx context.Context, date string) error {
	n, err := s.client.Holiday.Delete().Where(holiday.Date(date)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}

	return nil
}
```

`internal/api/handler/setting.go` — 构造函数 `handler.NewSetting(settingSvc *service.Setting, holidaySvc *service.HolidaySvc) *Setting`，按规格实现（模式同前）：

| 方法 | 路由 | 请求体 | 调用 |
|---|---|---|---|
| `All` | GET `/api/settings` | — | `settingSvc.All` |
| `Update` | PUT `/api/settings` | `{"values": {"key": "value"}}` | `settingSvc.Update` |
| `Holidays` | GET `/api/holidays?year=2026` | — | `holidaySvc.List` |
| `SaveHolidays` | PUT `/api/holidays` | `{"entries": [{"date","type","name"}]}` | `holidaySvc.Save` |
| `DeleteHoliday` | DELETE `/api/holidays/:date` | — | `holidaySvc.Delete` |

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/ internal/api/handler/
git commit -m "feat: 添加设置与节假日服务"
```

---

### Task 8: 审计服务与 Demand 状态机

**Files:**
- Create: `internal/service/audit.go`
- Create: `internal/service/demand.go`
- Test: `internal/service/demand_test.go`

**Interfaces:**
- Consumes: `Setting.Calendar/Int/Str`（Task 7）、`workday.Calendar.Deadline`
- Produces:
  - `service.NewAudit(client) *Audit`；`(*Audit).Record(ctx, actorID int, actorName, action, targetType string, targetID int, detail map[string]any)` — 记录失败仅打日志不阻断业务
  - `service.Actor{ID int, Name string}`（操作者；系统自动操作用 `service.SystemActor` = `Actor{ID: 0, Name: "system"}`）
  - `service.NewDemand(client, settingSvc, audit) *Demand`；方法（除查询外全部写审计）：
    - `List(ctx, status string) ([]*ent.Demand, error)`、`Get(ctx, id) (*ent.Demand, error)`
    - `Create(ctx, actor, title, description string, estimatedHalfDays int, plannedStart *time.Time) (*ent.Demand, error)`（初始 draft）
    - `Update(ctx, actor, id, ...同 Create 字段) (*ent.Demand, error)`（仅 draft/pending_estimate 可改）
    - `SubmitEstimate(ctx, actor, id)`（draft → pending_estimate）
    - `ConfirmEstimate(ctx, actor, id)`（pending_estimate → confirmed，记录确认人）
    - `Start(ctx, actor, id, actualStart time.Time)`（confirmed → in_progress）
    - `Finish(ctx, actor, id, actualStart, actualEnd time.Time, actualHalfDays int)`（in_progress → pending_acceptance；校验 actualEnd ≥ actualStart、actualHalfDays > 0；按设置计算 `accept_deadline = Calendar.Deadline(now, demand_confirm_window, window_unit)`）
    - `Accept(ctx, actor, id, auto, locked bool)`（pending_acceptance → accepted；`auto` 自动确认标记，`locked` 出账锁定标记；条件更新防并发重复确认）

- [ ] **Step 1: 写失败测试**

```go
package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
	"clepsydra/internal/ent/enttest"
)

// newDemandEnv 构建 Demand 测试环境
func newDemandEnv(t *testing.T, name string) (*ent.Client, *Demand) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	settingSvc := NewSetting(client)
	return client, NewDemand(client, settingSvc, NewAudit(client))
}

var admin = Actor{ID: 1, Name: "超级管理员"}
var clientActor = Actor{ID: 2, Name: "甲方"}

func TestDemandLifecycle(t *testing.T) {
	client, svc := newDemandEnv(t, "dlife")
	ctx := context.Background()

	// 创建 → 提交预估 → 确认预估 → 开工 → 完成 → 验收
	d, err := svc.Create(ctx, admin, "新功能", "描述", 4, nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err = svc.SubmitEstimate(ctx, admin, d.ID); err != nil {
		t.Fatalf("提交预估失败: %v", err)
	}
	if err = svc.ConfirmEstimate(ctx, clientActor, d.ID); err != nil {
		t.Fatalf("确认预估失败: %v", err)
	}

	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local)
	if err = svc.Start(ctx, admin, d.ID, start); err != nil {
		t.Fatalf("开工失败: %v", err)
	}
	if err = svc.Finish(ctx, admin, d.ID, start, end, 6); err != nil {
		t.Fatalf("完成失败: %v", err)
	}

	// 完成后应有确认截止时间（默认 5 自然日）
	d = svc.mustGet(ctx, t, d.ID)
	if d.Status.String() != "pending_acceptance" || d.AcceptDeadline == nil {
		t.Fatalf("完成后状态 = %s, deadline = %v", d.Status, d.AcceptDeadline)
	}

	if err = svc.Accept(ctx, clientActor, d.ID, false, false); err != nil {
		t.Fatalf("验收失败: %v", err)
	}
	d = svc.mustGet(ctx, t, d.ID)
	if d.Status.String() != "accepted" || d.AcceptAuto {
		t.Errorf("验收后状态 = %s, auto = %v", d.Status, d.AcceptAuto)
	}

	// 全流程审计日志已记录（create/submit/confirm/start/finish/accept 共 6 条）
	if n := client.AuditLog.Query().Where(auditlog.TargetType("demand")).CountX(ctx); n != 6 {
		t.Errorf("审计日志数 = %d, want 6", n)
	}
}

func TestDemandInvalidTransition(t *testing.T) {
	_, svc := newDemandEnv(t, "dinvalid")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 2, nil)

	// draft 不能直接验收
	if err := svc.Accept(ctx, clientActor, d.ID, false, false); err == nil {
		t.Error("draft 直接验收应拒绝")
	}

	// draft 不能直接开工
	if err := svc.Start(ctx, admin, d.ID, time.Now()); err == nil {
		t.Error("draft 直接开工应拒绝")
	}
}

func TestDemandFinishValidation(t *testing.T) {
	_, svc := newDemandEnv(t, "dfinish")
	ctx := context.Background()

	d, _ := svc.Create(ctx, admin, "需求", "", 2, nil)
	_ = svc.SubmitEstimate(ctx, admin, d.ID)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now())

	// 完成日期早于开工日期拒绝
	end := time.Now().AddDate(0, 0, -3)
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), end, 2); err == nil {
		t.Error("完成日期早于开工日期应拒绝")
	}

	// 实际人天必须为正
	if err := svc.Finish(ctx, admin, d.ID, time.Now(), time.Now(), 0); err == nil {
		t.Error("实际人天为 0 应拒绝")
	}
}
```

测试辅助方法（`demand.go` 内仅测试使用的简化取值，放测试文件）：

```go
// mustGet 测试辅助：按 ID 取需求
func (s *Demand) mustGet(ctx context.Context, t *testing.T, id int) *ent.Demand {
	t.Helper()

	d, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("查询需求失败: %v", err)
	}

	return d
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestDemand -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/service/audit.go`：

```go
package service

import (
	"context"

	"github.com/rs/zerolog/log"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/auditlog"
)

// Actor 操作者信息
type Actor struct {
	ID   int
	Name string
}

// SystemActor 系统自动操作者
var SystemActor = Actor{ID: 0, Name: "system"}

// Audit 审计服务，记录所有关键操作
type Audit struct {
	client *ent.Client
}

// NewAudit 构建审计服务
func NewAudit(client *ent.Client) *Audit {
	return &Audit{client: client}
}

// Record 写入审计日志，失败仅记录错误日志不阻断业务
func (a *Audit) Record(ctx context.Context, actor Actor, action, targetType string, targetID int, detail map[string]any) {
	builder := a.client.AuditLog.Create().
		SetActorID(actor.ID).
		SetActorName(actor.Name).
		SetAction(action).
		SetTargetType(targetType).
		SetTargetID(targetID)

	if detail != nil {
		builder.SetDetail(detail)
	}

	if _, err := builder.Save(ctx); err != nil {
		log.Error().Err(err).Str("action", action).Msg("写入审计日志失败")
	}
}

// List 分页查询审计日志，targetType/targetID 为空或 0 时不过滤
// page 从 1 起，size 上限 100，按 id 倒序
func (a *Audit) List(ctx context.Context, targetType string, targetID, page, size int) (int, []*ent.AuditLog, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	q := a.client.AuditLog.Query()
	if targetType != "" {
		q = q.Where(auditlog.TargetType(targetType))
	}
	if targetID > 0 {
		q = q.Where(auditlog.TargetID(targetID))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return 0, nil, err
	}

	rows, err := q.Order(ent.Desc(auditlog.FieldID)).Offset((page - 1) * size).Limit(size).All(ctx)
	if err != nil {
		return 0, nil, err
	}

	return total, rows, nil
}
```

`internal/service/demand.go`：

```go
package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/workday"
)

// Demand 需求服务，管理需求全生命周期状态机
type Demand struct {
	client  *ent.Client
	setting *Setting
	audit   *Audit
}

// NewDemand 构建需求服务
func NewDemand(client *ent.Client, setting *Setting, audit *Audit) *Demand {
	return &Demand{client: client, setting: setting, audit: audit}
}

// transitions 状态机白名单：当前状态 → 允许进入的下一状态
var transitions = map[demand.Status][]demand.Status{
	demand.StatusDraft:             {demand.StatusPendingEstimate},
	demand.StatusPendingEstimate:   {demand.StatusConfirmed},
	demand.StatusConfirmed:         {demand.StatusInProgress},
	demand.StatusInProgress:        {demand.StatusPendingAcceptance},
	demand.StatusPendingAcceptance: {demand.StatusAccepted},
}

// canTransit 判定状态流转是否合法
func canTransit(from, to demand.Status) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}

	return false
}

// List 按状态筛选需求，status 为空返回全部
func (s *Demand) List(ctx context.Context, status string) ([]*ent.Demand, error) {
	q := s.client.Demand.Query().Order(ent.Desc(demand.FieldID))
	if status != "" {
		q = q.Where(demand.StatusEQ(demand.Status(status)))
	}

	return q.All(ctx)
}

// Get 按 ID 查询需求
func (s *Demand) Get(ctx context.Context, id int) (*ent.Demand, error) {
	d, err := s.client.Demand.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return d, err
}

// Create 创建需求，初始状态 draft
func (s *Demand) Create(ctx context.Context, actor Actor, title, description string, estimatedHalfDays int, plannedStart *time.Time) (*ent.Demand, error) {
	if title == "" {
		return nil, ErrBadRequest("标题不能为空")
	}
	if estimatedHalfDays <= 0 {
		return nil, ErrBadRequest("预估人天必须为正")
	}

	builder := s.client.Demand.Create().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays)
	if plannedStart != nil {
		builder.SetPlannedStartDate(*plannedStart)
	}

	d, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.create", "demand", d.ID, map[string]any{
		"title": title, "estimated_half_days": estimatedHalfDays,
	})

	return d, nil
}

// Update 更新需求基本信息，仅 draft 与 pending_estimate 状态允许
func (s *Demand) Update(ctx context.Context, actor Actor, id int, title, description string, estimatedHalfDays int, plannedStart *time.Time) (*ent.Demand, error) {
	d, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.Status != demand.StatusDraft && d.Status != demand.StatusPendingEstimate {
		return nil, ErrInvalidTransition
	}
	if title == "" || estimatedHalfDays <= 0 {
		return nil, ErrBadRequest("标题不能为空且预估人天必须为正")
	}

	builder := d.Update().
		SetTitle(title).
		SetDescription(description).
		SetEstimatedHalfDays(estimatedHalfDays)
	if plannedStart != nil {
		builder.SetPlannedStartDate(*plannedStart)
	} else {
		builder.ClearPlannedStartDate()
	}

	d, err = builder.Save(ctx)
	if err != nil {
		return nil, err
	}

	s.audit.Record(ctx, actor, "demand.update", "demand", d.ID, map[string]any{
		"title": title, "estimated_half_days": estimatedHalfDays,
	})

	return d, nil
}

// transit 通用状态流转：条件更新防止并发下重复流转
func (s *Demand) transit(ctx context.Context, id int, from, to demand.Status, apply func(*ent.DemandUpdate)) error {
	if !canTransit(from, to) {
		return ErrInvalidTransition
	}

	update := s.client.Demand.Update().
		Where(demand.ID(id), demand.StatusEQ(from)).
		SetStatus(to)
	apply(update)

	n, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	return nil
}

// SubmitEstimate 提交预估，进入待需求方确认人天
func (s *Demand) SubmitEstimate(ctx context.Context, actor Actor, id int) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	err = s.transit(ctx, id, d.Status, demand.StatusPendingEstimate, func(u *ent.DemandUpdate) {})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.submit_estimate", "demand", id, nil)

	return nil
}

// ConfirmEstimate 需求方确认预估人天
func (s *Demand) ConfirmEstimate(ctx context.Context, actor Actor, id int) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	err = s.transit(ctx, id, d.Status, demand.StatusConfirmed, func(u *ent.DemandUpdate) {
		u.SetEstimateConfirmedAt(now).SetEstimateConfirmedBy(actor.ID)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.confirm_estimate", "demand", id, nil)

	return nil
}

// Start 标记开工
func (s *Demand) Start(ctx context.Context, actor Actor, id int, actualStart time.Time) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	err = s.transit(ctx, id, d.Status, demand.StatusInProgress, func(u *ent.DemandUpdate) {
		u.SetActualStartDate(actualStart)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.start", "demand", id, map[string]any{
		"actual_start_date": actualStart.Format("2006-01-02"),
	})

	return nil
}

// Finish 标记完成：写入实际日期与人天，计算需求方确认截止时间
func (s *Demand) Finish(ctx context.Context, actor Actor, id int, actualStart, actualEnd time.Time, actualHalfDays int) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if actualHalfDays <= 0 {
		return ErrBadRequest("实际人天必须为正")
	}
	if actualEnd.Before(actualStart) {
		return ErrBadRequest("完成日期不能早于开工日期")
	}

	// 按设置计算确认截止时间
	window, err := s.setting.Int(ctx, SettingDemandConfirmWindow)
	if err != nil {
		return err
	}
	unit, err := s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return err
	}
	cal, err := s.setting.Calendar(ctx)
	if err != nil {
		return err
	}
	deadline := cal.Deadline(time.Now(), window, workday.Unit(unit))

	err = s.transit(ctx, id, d.Status, demand.StatusPendingAcceptance, func(u *ent.DemandUpdate) {
		u.SetActualStartDate(actualStart).
			SetActualEndDate(actualEnd).
			SetActualHalfDays(actualHalfDays).
			SetAcceptDeadline(deadline)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.finish", "demand", id, map[string]any{
		"actual_end_date": actualEnd.Format("2006-01-02"), "actual_half_days": actualHalfDays,
	})

	return nil
}

// Accept 确认完成：auto 表示逾期自动确认，locked 表示出账前锁定
func (s *Demand) Accept(ctx context.Context, actor Actor, id int, auto, locked bool) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	err = s.transit(ctx, id, d.Status, demand.StatusAccepted, func(u *ent.DemandUpdate) {
		u.SetAcceptedAt(now).SetAcceptedBy(actor.ID).SetAcceptAuto(auto).SetAcceptLocked(locked)
	})
	if err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "demand.accept", "demand", id, map[string]any{
		"auto": auto, "locked": locked,
	})

	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/
git commit -m "feat: 添加审计服务与需求状态机"
```

---

### Task 9: Demand API

**Files:**
- Create: `internal/api/handler/demand.go`
- Test: `internal/api/handler/demand_test.go`

**Interfaces:**
- Consumes: `service.Demand`（Task 8）、`api.Claims/OK/Fail`、`api.RequireAuth/RequireAdmin`
- Produces: 路由规格（注册在 Task 12 的 router 中）

| 方法 | 路由 | 权限 | 请求体 | 调用 |
|---|---|---|---|---|
| `List` | GET `/api/demands?status=` | 登录 | — | `svc.List` |
| `Get` | GET `/api/demands/:id` | 登录 | — | `svc.Get` |
| `Create` | POST `/api/demands` | admin | `{"title","description","estimated_half_days","planned_start_date"}`（日期 `2026-08-01` 或 null） | `svc.Create` |
| `Update` | PUT `/api/demands/:id` | admin | 同 Create | `svc.Update` |
| `SubmitEstimate` | POST `/api/demands/:id/submit-estimate` | admin | — | `svc.SubmitEstimate` |
| `ConfirmEstimate` | POST `/api/demands/:id/confirm-estimate` | 登录（client 或 admin） | — | `svc.ConfirmEstimate` |
| `Start` | POST `/api/demands/:id/start` | admin | `{"actual_start_date"}` | `svc.Start` |
| `Finish` | POST `/api/demands/:id/finish` | admin | `{"actual_start_date","actual_end_date","actual_half_days"}` | `svc.Finish` |
| `Accept` | POST `/api/demands/:id/accept` | 登录 | — | `svc.Accept(ctx, actor, id, false, false)` |

操作者统一从 `api.Claims(c)` 组装：`service.Actor{ID: claims.UserID, Name: claims.Name}`（`Claims.Name` 已在 Task 5 定义并随登录签发）。

- [ ] **Step 1: 写失败测试（handler 级 HTTP 测试，覆盖创建与验收路径）**

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

func TestDemandCreateHandler(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:hdemand?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := service.NewSetting(client)
	svc := service.NewDemand(client, settingSvc, service.NewAudit(client))
	h := NewDemand(svc)

	e := echo.New()
	reqBody := `{"title":"新功能","description":"","estimated_half_days":4,"planned_start_date":"2026-08-10"}`
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

	// 预估人天为 0 拒绝
	req = httptest.NewRequest(http.MethodPost, "/api/demands", strings.NewReader(`{"title":"x","estimated_half_days":0}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.Set("claims", &service.Claims{UserID: 1, Role: "admin", Name: "管理员"})
	_ = h.Create(c)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法参数应返回 400, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/api/handler/ -v`
Expected: FAIL（`NewDemand` 未定义）

- [ ] **Step 3: 实现**

`internal/api/handler/demand.go` 核心代码：

```go
package handler

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"clepsydra/internal/api"
	"clepsydra/internal/service"
)

// Demand 需求接口
type Demand struct {
	svc *service.Demand
}

// NewDemand 构建需求 handler
func NewDemand(svc *service.Demand) *Demand {
	return &Demand{svc: svc}
}

// demandRequest 创建与更新共用的请求体
type demandRequest struct {
	Title             string `json:"title"`
	Description       string `json:"description"`
	EstimatedHalfDays int    `json:"estimated_half_days"`
	PlannedStartDate  string `json:"planned_start_date"`
}

// parseID 解析路径中的需求 ID
func parseID(c echo.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, service.ErrBadRequest("ID 不合法")
	}

	return id, nil
}

// parseDate 解析 YYYY-MM-DD 日期，空串返回 nil
func parseDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}

	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, service.ErrBadRequest("日期格式必须为 YYYY-MM-DD")
	}

	return &d, nil
}

// actor 从登录态组装操作者
func actor(c echo.Context) service.Actor {
	claims := api.Claims(c)
	return service.Actor{ID: claims.UserID, Name: claims.Name}
}

// Create POST /api/demands
func (h *Demand) Create(c echo.Context) error {
	var req demandRequest
	if err := c.Bind(&req); err != nil {
		return api.Fail(c, service.ErrBadRequest("参数错误"))
	}

	planned, err := parseDate(req.PlannedStartDate)
	if err != nil {
		return api.Fail(c, err)
	}

	d, err := h.svc.Create(c.Request().Context(), actor(c), req.Title, req.Description, req.EstimatedHalfDays, planned)
	if err != nil {
		return api.Fail(c, err)
	}

	return api.OK(c, d)
}
```

其余方法按接口表实现，模式与 `Create` 一致：解析 ID/请求体 → 调 service → `api.OK`/`api.Fail`。`Finish` 请求体 `{"actual_start_date","actual_end_date","actual_half_days"}` 两个日期均必填（`parseDate` 后判 nil 则 `ErrBadRequest("日期不能为空")`）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/api/handler/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/api/ internal/service/auth.go
git commit -m "feat: 添加需求管理接口"
```

---

### Task 10: Bill service（生成/锁定/减免/分享/确认）

**Files:**
- Create: `internal/service/bill.go`
- Test: `internal/service/bill_test.go`

**Interfaces:**
- Consumes: `Setting`（各设置项与 Calendar）、`Demand.Accept`（出账锁定复用）、`Audit.Record`
- Produces: `service.NewBill(client, settingSvc, demandSvc, audit) *Bill`；方法：
  - `List(ctx) ([]*ent.Bill, error)`、`Get(ctx, id) (*ent.Bill, error)`（携带 items）
  - `Generate(ctx, actor, period string) (*ent.Bill, error)` — period 形如 `2026-07`；核心流程：
    1. 已存在同账期账单：状态非 draft 返回 `ErrBadRequest("账单已分享或已确认，不可重新生成")`；draft 则删除旧明细与账单后重建
    2. **出账前锁定**：`actual_end_date` 在该账期内且状态仍为 `pending_acceptance` 的需求，逐个调 `demandSvc.Accept(ctx, SystemActor, id, true, true)`
    3. 读设置快照 `daily_rate`/`base_fee`/`bill_include_statuses`
    4. 组装明细：`accepted` 且完成日在账期内 → 计费行（`half_days` = 实际人天、`amount = half_days × rate / 2`、`billable = true`）；`in_progress`/`confirmed`（在包含列表中且非计费行归属）→ 展示行（`half_days` = 预估、`amount = 0`、`billable = false`、快照 `planned_start_date`）
    5. 汇总 `total_half_days` = Σ 计费行 half_days；`total_amount` = Σ 计费行 amount + base_fee
  - `ToggleWaive(ctx, actor, billID, itemID int) error` — 仅 draft 账单；翻转明细 `waived`，减免后 `amount = 0`、恢复则重算，并同步账单 `total_amount`
  - `Share(ctx, actor, id) error` — draft → pending；`shared_at = now`，`confirm_deadline = Calendar.Deadline(now, bill_confirm_window, window_unit)`
  - `Revoke(ctx, actor, id) error` — pending → draft，清空分享与截止字段
  - `Confirm(ctx, actor, id, auto bool) error` — pending → confirmed；条件更新防重复
  - `PrevPeriod(now time.Time) string` — 工具函数：返回上月账期字符串

- [ ] **Step 1: 写失败测试**

```go
package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/enttest"
)

// newBillEnv 构建账单测试环境
func newBillEnv(t *testing.T, name string) (*ent.Client, *Demand, *Bill) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	settingSvc := NewSetting(client)
	audit := NewAudit(client)
	demandSvc := NewDemand(client, settingSvc, audit)
	return client, demandSvc, NewBill(client, settingSvc, demandSvc, audit)
}

// prepareDemand 造一个已完成待确认的需求，完成日期在 2026-07
func prepareDemand(t *testing.T, svc *Demand, title string, halfDays int) int {
	t.Helper()

	ctx := context.Background()
	d, _ := svc.Create(ctx, admin, title, "", halfDays, nil)
	_ = svc.SubmitEstimate(ctx, admin, d.ID)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)

	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	_ = svc.Start(ctx, admin, d.ID, start)
	if err := svc.Finish(ctx, admin, d.ID, start, end, halfDays); err != nil {
		t.Fatalf("完成需求失败: %v", err)
	}

	return d.ID
}

func TestBillGenerate(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bgen")
	ctx := context.Background()

	// 需求 1：已完成已验收（3 人天 = 6 半天）
	id1 := prepareDemand(t, demandSvc, "已验收需求", 6)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	// 需求 2：已完成未验收 → 出账时应被锁定自动确认
	id2 := prepareDemand(t, demandSvc, "未验收需求", 4)

	// 需求 3：进行中 → 展示行
	d3, _ := demandSvc.Create(ctx, admin, "进行中需求", "", 8, nil)
	_ = demandSvc.SubmitEstimate(ctx, admin, d3.ID)
	_ = demandSvc.ConfirmEstimate(ctx, clientActor, d3.ID)
	_ = demandSvc.Start(ctx, admin, d3.ID, time.Date(2026, 7, 25, 0, 0, 0, 0, time.Local))

	bill, err := billSvc.Generate(ctx, admin, "2026-07")
	if err != nil {
		t.Fatalf("生成账单失败: %v", err)
	}

	// 出账前锁定：需求 2 已被自动确认且带锁定标记
	d2 := demandSvc.mustGet(ctx, t, id2)
	if d2.Status.String() != "accepted" || !d2.AcceptAuto || !d2.AcceptLocked {
		t.Errorf("需求 2 应被出账锁定: status=%s auto=%v locked=%v", d2.Status, d2.AcceptAuto, d2.AcceptLocked)
	}

	// 金额：计费 6+4=10 半天 × 1200/2 = 6000，加基础维护费 12000 = 18000
	if bill.TotalHalfDays != 10 || bill.TotalAmount != 18000 {
		t.Errorf("账单合计 = %d 半天 / %d 元, want 10 / 18000", bill.TotalHalfDays, bill.TotalAmount)
	}

	// 明细：2 计费行 + 1 展示行
	billable := client.BillItem.Query().Where(billitem.Billable(true)).CountX(ctx)
	display := client.BillItem.Query().Where(billitem.Billable(false)).CountX(ctx)
	if billable != 2 || display != 1 {
		t.Errorf("明细行 = %d 计费 / %d 展示, want 2 / 1", billable, display)
	}

	// draft 状态可重新生成
	if _, err = billSvc.Generate(ctx, admin, "2026-07"); err != nil {
		t.Errorf("draft 账单应可重新生成: %v", err)
	}
}

func TestBillWaiveAndShareConfirm(t *testing.T) {
	client, demandSvc, billSvc := newBillEnv(t, "bshare")
	ctx := context.Background()

	id1 := prepareDemand(t, demandSvc, "小缺陷修复", 2)
	_ = demandSvc.Accept(ctx, clientActor, id1, false, false)

	bill, _ := billSvc.Generate(ctx, admin, "2026-07")

	// 减免：1 人天 × 1200 = 1200 → 减免后总额只剩基础维护费
	item := client.BillItem.Query().Where(billitem.Billable(true)).OnlyX(ctx)
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err != nil {
		t.Fatalf("减免失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.TotalAmount != 12000 {
		t.Errorf("减免后总额 = %d, want 12000", bill.TotalAmount)
	}

	// 分享 → 确认
	if err := billSvc.Share(ctx, admin, bill.ID); err != nil {
		t.Fatalf("分享失败: %v", err)
	}
	bill, _ = billSvc.Get(ctx, bill.ID)
	if bill.Status.String() != "pending" || bill.ConfirmDeadline == nil {
		t.Fatalf("分享后状态 = %s", bill.Status)
	}

	// 分享后不可重新生成
	if _, err := billSvc.Generate(ctx, admin, "2026-07"); err == nil {
		t.Error("已分享账单不应可重新生成")
	}

	if err := billSvc.Confirm(ctx, clientActor, bill.ID, false); err != nil {
		t.Fatalf("确认失败: %v", err)
	}

	// 已确认后减免与撤回均拒绝
	if err := billSvc.ToggleWaive(ctx, admin, bill.ID, item.ID); err == nil {
		t.Error("已确认账单不应可减免")
	}
	if err := billSvc.Revoke(ctx, admin, bill.ID); err == nil {
		t.Error("已确认账单不应可撤回")
	}
}

func TestPrevPeriod(t *testing.T) {
	if got := PrevPeriod(time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)); got != "2026-07" {
		t.Errorf("PrevPeriod = %s, want 2026-07", got)
	}
	if got := PrevPeriod(time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local)); got != "2025-12" {
		t.Errorf("PrevPeriod = %s, want 2025-12", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run 'TestBill|TestPrevPeriod' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/service/bill.go`：

```go
package service

import (
	"context"
	"strings"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/billitem"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/workday"
)

// Bill 账单服务
type Bill struct {
	client  *ent.Client
	setting *Setting
	demand  *Demand
	audit   *Audit
}

// NewBill 构建账单服务
func NewBill(client *ent.Client, setting *Setting, demandSvc *Demand, audit *Audit) *Bill {
	return &Bill{client: client, setting: setting, demand: demandSvc, audit: audit}
}

// PrevPeriod 返回 now 所在月的上一个账期，格式 YYYY-MM
func PrevPeriod(now time.Time) string {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	prev := first.AddDate(0, -1, 0)

	return prev.Format("2006-01")
}

// periodRange 解析账期为 [起, 止) 时间区间
func periodRange(period string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", period, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, ErrBadRequest("账期格式必须为 YYYY-MM")
	}

	return start, start.AddDate(0, 1, 0), nil
}

// List 查询全部账单
func (s *Bill) List(ctx context.Context) ([]*ent.Bill, error) {
	return s.client.Bill.Query().Order(ent.Desc(bill.FieldPeriod)).All(ctx)
}

// Get 查询账单及明细
func (s *Bill) Get(ctx context.Context, id int) (*ent.Bill, error) {
	b, err := s.client.Bill.Query().Where(bill.ID(id)).WithItems().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return b, err
}

// Generate 生成指定账期的账单草稿，可对 draft 状态重复执行
func (s *Bill) Generate(ctx context.Context, actor Actor, period string) (*ent.Bill, error) {
	start, end, err := periodRange(period)
	if err != nil {
		return nil, err
	}

	// 同账期已有账单：非草稿拒绝，草稿删除重建
	existing, err := s.client.Bill.Query().Where(bill.Period(period)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if existing != nil {
		if existing.Status != bill.StatusDraft {
			return nil, ErrBadRequest("账单已分享或已确认，不可重新生成")
		}
		if _, err = s.client.BillItem.Delete().Where(billitem.HasBillWith(bill.ID(existing.ID))).Exec(ctx); err != nil {
			return nil, err
		}
		if err = s.client.Bill.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			return nil, err
		}
	}

	// 出账前锁定：账期内完成且仍待确认的需求全部自动确认
	pending, err := s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusPendingAcceptance),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range pending {
		if err = s.demand.Accept(ctx, SystemActor, d.ID, true, true); err != nil {
			return nil, err
		}
	}

	// 读取设置快照
	rate, err := s.setting.Int(ctx, SettingDailyRate)
	if err != nil {
		return nil, err
	}
	baseFee, err := s.setting.Int(ctx, SettingBaseFee)
	if err != nil {
		return nil, err
	}
	include, err := s.setting.Str(ctx, SettingBillIncludeStatuses)
	if err != nil {
		return nil, err
	}
	includeSet := make(map[string]bool)
	for _, st := range strings.Split(include, ",") {
		includeSet[strings.TrimSpace(st)] = true
	}

	// 计费行：账期内完成且已确认的需求
	accepted, err := s.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusAccepted),
		demand.ActualEndDateGTE(start),
		demand.ActualEndDateLT(end),
	).Order(ent.Asc(demand.FieldActualEndDate)).All(ctx)
	if err != nil {
		return nil, err
	}

	// 展示行：设置包含的未完结状态需求
	var display []*ent.Demand
	for _, st := range []demand.Status{demand.StatusInProgress, demand.StatusConfirmed} {
		if !includeSet[st.String()] {
			continue
		}
		rows, err := s.client.Demand.Query().Where(demand.StatusEQ(st)).Order(ent.Asc(demand.FieldID)).All(ctx)
		if err != nil {
			return nil, err
		}
		display = append(display, rows...)
	}

	// 汇总并落库
	totalHalfDays, totalAmount := 0, baseFee
	for _, d := range accepted {
		if d.ActualHalfDays != nil {
			totalHalfDays += *d.ActualHalfDays
			totalAmount += *d.ActualHalfDays * rate / 2
		}
	}

	b, err := s.client.Bill.Create().
		SetPeriod(period).
		SetDailyRate(rate).
		SetBaseFee(baseFee).
		SetTotalHalfDays(totalHalfDays).
		SetTotalAmount(totalAmount).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	for _, d := range accepted {
		halfDays := 0
		if d.ActualHalfDays != nil {
			halfDays = *d.ActualHalfDays
		}
		if err = s.createItem(ctx, b, d, halfDays, halfDays*rate/2, true); err != nil {
			return nil, err
		}
	}
	for _, d := range display {
		if err = s.createItem(ctx, b, d, d.EstimatedHalfDays, 0, false); err != nil {
			return nil, err
		}
	}

	s.audit.Record(ctx, actor, "bill.generate", "bill", b.ID, map[string]any{
		"period": period, "total_amount": totalAmount,
	})

	return b, nil
}

// createItem 写入一条账单明细
func (s *Bill) createItem(ctx context.Context, b *ent.Bill, d *ent.Demand, halfDays, amount int, billable bool) error {
	builder := s.client.BillItem.Create().
		SetBill(b).
		SetDemandID(d.ID).
		SetDemandTitle(d.Title).
		SetDemandStatus(d.Status.String()).
		SetHalfDays(halfDays).
		SetAmount(amount).
		SetBillable(billable)
	if d.PlannedStartDate != nil {
		builder.SetPlannedStartDate(*d.PlannedStartDate)
	}

	_, err := builder.Save(ctx)

	return err
}

// ToggleWaive 翻转明细减免状态并重算账单总额，仅草稿账单允许
func (s *Bill) ToggleWaive(ctx context.Context, actor Actor, billID, itemID int) error {
	b, err := s.Get(ctx, billID)
	if err != nil {
		return err
	}
	if b.Status != bill.StatusDraft {
		return ErrBadRequest("仅草稿账单可调整减免")
	}

	item, err := s.client.BillItem.Query().Where(billitem.ID(itemID), billitem.HasBillWith(bill.ID(billID))).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !item.Billable {
		return ErrBadRequest("展示行不可减免")
	}

	// 翻转减免：减免后金额归零，恢复按快照单价重算
	waived := !item.Waived
	amount := 0
	if !waived {
		amount = item.HalfDays * b.DailyRate / 2
	}
	if _, err = item.Update().SetWaived(waived).SetAmount(amount).Save(ctx); err != nil {
		return err
	}

	// 重算账单合计
	items, err := s.client.BillItem.Query().Where(billitem.HasBillWith(bill.ID(billID))).All(ctx)
	if err != nil {
		return err
	}
	total := b.BaseFee
	for _, it := range items {
		if it.ID == item.ID {
			total += amount
			continue
		}
		total += it.Amount
	}
	if _, err = b.Update().SetTotalAmount(total).Save(ctx); err != nil {
		return err
	}

	s.audit.Record(ctx, actor, "bill.toggle_waive", "bill", billID, map[string]any{
		"item_id": itemID, "waived": waived,
	})

	return nil
}

// Share 分享账单进入待确认状态，计算确认截止时间
func (s *Bill) Share(ctx context.Context, actor Actor, id int) error {
	window, err := s.setting.Int(ctx, SettingBillConfirmWindow)
	if err != nil {
		return err
	}
	unit, err := s.setting.Str(ctx, SettingWindowUnit)
	if err != nil {
		return err
	}
	cal, err := s.setting.Calendar(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	deadline := cal.Deadline(now, window, workday.Unit(unit))

	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusDraft)).
		SetStatus(bill.StatusPending).
		SetSharedAt(now).
		SetConfirmDeadline(deadline).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.share", "bill", id, map[string]any{
		"confirm_deadline": deadline.Format(time.RFC3339),
	})

	return nil
}

// Revoke 撤回已分享未确认的账单回到草稿
func (s *Bill) Revoke(ctx context.Context, actor Actor, id int) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusPending)).
		SetStatus(bill.StatusDraft).
		ClearSharedAt().
		ClearConfirmDeadline().
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.revoke", "bill", id, nil)

	return nil
}

// Confirm 确认账单，auto 表示逾期自动确认
func (s *Bill) Confirm(ctx context.Context, actor Actor, id int, auto bool) error {
	n, err := s.client.Bill.Update().
		Where(bill.ID(id), bill.StatusEQ(bill.StatusPending)).
		SetStatus(bill.StatusConfirmed).
		SetConfirmedAt(time.Now()).
		SetConfirmedBy(actor.ID).
		SetConfirmAuto(auto).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidTransition
	}

	s.audit.Record(ctx, actor, "bill.confirm", "bill", id, map[string]any{"auto": auto})

	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/
git commit -m "feat: 添加账单服务（生成、锁定、减免、分享、确认）"
```

---

### Task 11: Bill API 与 Dashboard 待办

**Files:**
- Create: `internal/api/handler/bill.go`
- Create: `internal/service/dashboard.go`
- Create: `internal/api/handler/dashboard.go`
- Test: `internal/service/dashboard_test.go`

**Interfaces:**
- Consumes: `service.Bill`、`service.Setting.Calendar`、`workday.Calendar.BillingDueDate`、`service.PrevPeriod`
- Produces:
  - Bill 路由规格：

| 方法 | 路由 | 权限 | 请求体 | 调用 |
|---|---|---|---|---|
| `List` | GET `/api/bills` | 登录 | — | `svc.List` |
| `Get` | GET `/api/bills/:id` | 登录 | — | `svc.Get`（返回含 `items`） |
| `Generate` | POST `/api/bills/generate` | admin | `{"period":"2026-07"}` | `svc.Generate` |
| `ToggleWaive` | POST `/api/bills/:id/items/:itemId/waive` | admin | — | `svc.ToggleWaive` |
| `Share` | POST `/api/bills/:id/share` | admin | — | `svc.Share` |
| `Revoke` | POST `/api/bills/:id/revoke` | admin | — | `svc.Revoke` |
| `Confirm` | POST `/api/bills/:id/confirm` | 登录 | — | `svc.Confirm(ctx, actor, id, false)` |

  - `service.NewDashboard(client, settingSvc) *Dashboard`；`(*Dashboard).Todos(ctx, role string, now time.Time) (*Todos, error)`：

```go
// Todos 工作台待办汇总
type Todos struct {
	PendingEstimateCount   int    `json:"pending_estimate_count"`   // 待确认人天的需求数
	PendingAcceptanceCount int    `json:"pending_acceptance_count"` // 完成待确认的需求数
	PendingBillCount       int    `json:"pending_bill_count"`       // 待确认的账单数
	BillingDueDate         string `json:"billing_due_date"`         // 本月出账截止日
	BillingDueToday        bool   `json:"billing_due_today"`        // 今天是否出账截止日
	PrevBillShared         bool   `json:"prev_bill_shared"`         // 上月账单是否已分享（含已确认）
}
```

`Todos` 逻辑：各计数直接按状态 count；`BillingDueDate = Calendar.BillingDueDate(now.Year(), now.Month())`；`PrevBillShared` = 上月账期账单存在且状态非 draft。

- [ ] **Step 1: 写失败测试**

```go
package service

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"clepsydra/internal/config"
	"clepsydra/internal/ent/enttest"
)

func TestDashboardTodos(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:dash?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()
	_ = Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil)

	settingSvc := NewSetting(client)
	audit := NewAudit(client)
	demandSvc := NewDemand(client, settingSvc, audit)

	// 一个待确认人天的需求
	d, _ := demandSvc.Create(ctx, admin, "待确认", "", 2, nil)
	_ = demandSvc.SubmitEstimate(ctx, admin, d.ID)

	svc := NewDashboard(client, settingSvc)
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.Local)

	todos, err := svc.Todos(ctx, "admin", now)
	if err != nil {
		t.Fatalf("查询待办失败: %v", err)
	}

	if todos.PendingEstimateCount != 1 {
		t.Errorf("待确认人天数 = %d, want 1", todos.PendingEstimateCount)
	}
	if todos.PrevBillShared {
		t.Error("上月账单未生成，PrevBillShared 应为 false")
	}
	if todos.BillingDueDate == "" {
		t.Error("出账截止日不应为空")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestDashboard -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/service/dashboard.go`：

```go
package service

import (
	"context"
	"time"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
)

// Todos 工作台待办汇总
type Todos struct {
	PendingEstimateCount   int    `json:"pending_estimate_count"`
	PendingAcceptanceCount int    `json:"pending_acceptance_count"`
	PendingBillCount       int    `json:"pending_bill_count"`
	BillingDueDate         string `json:"billing_due_date"`
	BillingDueToday        bool   `json:"billing_due_today"`
	PrevBillShared         bool   `json:"prev_bill_shared"`
}

// Dashboard 工作台服务
type Dashboard struct {
	client  *ent.Client
	setting *Setting
}

// NewDashboard 构建工作台服务
func NewDashboard(client *ent.Client, setting *Setting) *Dashboard {
	return &Dashboard{client: client, setting: setting}
}

// Todos 汇总待办信息
func (s *Dashboard) Todos(ctx context.Context, role string, now time.Time) (*Todos, error) {
	todos := new(Todos)

	// 各状态计数
	var err error
	if todos.PendingEstimateCount, err = s.client.Demand.Query().
		Where(demand.StatusEQ(demand.StatusPendingEstimate)).Count(ctx); err != nil {
		return nil, err
	}
	if todos.PendingAcceptanceCount, err = s.client.Demand.Query().
		Where(demand.StatusEQ(demand.StatusPendingAcceptance)).Count(ctx); err != nil {
		return nil, err
	}
	if todos.PendingBillCount, err = s.client.Bill.Query().
		Where(bill.StatusEQ(bill.StatusPending)).Count(ctx); err != nil {
		return nil, err
	}

	// 出账截止日与上月账单状态
	cal, err := s.setting.Calendar(ctx)
	if err != nil {
		return nil, err
	}
	due := cal.BillingDueDate(now.Year(), now.Month())
	todos.BillingDueDate = due.Format("2006-01-02")
	todos.BillingDueToday = due.Format("2006-01-02") == now.Format("2006-01-02")

	prev, err := s.client.Bill.Query().Where(bill.Period(PrevPeriod(now))).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	todos.PrevBillShared = prev != nil && prev.Status != bill.StatusDraft

	return todos, nil
}
```

`internal/api/handler/bill.go` 与 `internal/api/handler/dashboard.go` 按路由规格实现，模式同 Task 9（解析参数 → service → OK/Fail）。Dashboard 路由：GET `/api/dashboard/todos`（登录），`now` 取 `time.Now()`，`role` 取 `api.Claims(c).Role`。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ ./internal/api/... -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/ internal/api/
git commit -m "feat: 添加账单接口与工作台待办"
```

---

### Task 12: 定时任务、路由装配与入口

> 变更整合（来自用户 TODO.md）：运行时配置移到 `configs/config.yaml`（目录已 gitignore）；节假日数据采用 holiday-cn（github.com/NateScarlet/holiday-cn）年度 JSON 格式，新增 `workday.ParseHolidayCN` 适配。

**Files:**
- Create: `internal/task/task.go`
- Create: `internal/api/router.go`
- Create: `internal/api/handler/auditlog.go`
- Create: `internal/workday/holidaycn.go`
- Create: `assets/holidays/2026.json`（holiday-cn 原始格式，下载自 `https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/2026.json`；下载失败则报 BLOCKED）
- Delete: `assets/holidays.json`（旧自定义格式，由 holiday-cn 数据替代）
- Modify: `../../configs/config.example.yaml`（holiday.file 改为 `assets/holidays/2026.json`）
- Create: `cmd/clepsydra/main.go`
- Create: `Makefile`
- Create: `.golangci.yml`
- Test: `internal/task/task_test.go`
- Test: `internal/workday/holidaycn_test.go`

**Interfaces:**
- Consumes: 前述全部 service；`lumberjack.Logger.Rotate`
- Produces:
  - `task.New(client, settingSvc, demandSvc, billSvc, rotator, log) *Runner`
  - `(*Runner).Start()` 注册 cron 并启动；`(*Runner).Stop()`
  - 可独立测试的纯逻辑方法：
    - `(*Runner).ScanExpired(ctx, now time.Time) error` — 自动确认过期需求（`status = pending_acceptance AND accept_deadline < now` → `demandSvc.Accept(SystemActor, id, true, false)`）与过期账单（`status = pending AND confirm_deadline < now` → `billSvc.Confirm(SystemActor, id, true)`）
    - `(*Runner).EnsurePrevBill(ctx, now time.Time) error` — 上月账单不存在则 `billSvc.Generate(SystemActor, PrevPeriod(now))`；已存在（任意状态）跳过
  - cron 注册：`0 0 * * *` 日志轮转（rotator 非 nil 时）；`5 0 * * *` ScanExpired；`10 0 1 * *` EnsurePrevBill；启动时立即执行一次 `EnsurePrevBill`（宕机补生成）
  - 审计日志接口：GET `/api/audit-logs?target_type=&target_id=&page=&size=`（admin；`page` 从 1 起，`size` 默认 20 上限 100，按 id 倒序，返回 `{"total": n, "rows": [...]}`）

- [ ] **Step 1: 写失败测试**

```go
package task

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/enttest"
	"clepsydra/internal/service"
)

var admin = service.Actor{ID: 1, Name: "超级管理员"}
var clientActor = service.Actor{ID: 2, Name: "甲方"}

// newEnv 构建定时任务测试环境
func newEnv(t *testing.T, name string) (*ent.Client, *service.Demand, *service.Bill, *Runner) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := service.Seed(ctx, client, config.Admin{Username: "a", Password: "admin123"}, nil); err != nil {
		t.Fatalf("种子失败: %v", err)
	}

	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	runner := New(client, settingSvc, demandSvc, billSvc, nil, zerolog.Nop())

	return client, demandSvc, billSvc, runner
}

// finishDemand 造一个完成待确认的需求
func finishDemand(t *testing.T, svc *service.Demand, title string) int {
	t.Helper()

	ctx := context.Background()
	d, _ := svc.Create(ctx, admin, title, "", 4, nil)
	_ = svc.SubmitEstimate(ctx, admin, d.ID)
	_ = svc.ConfirmEstimate(ctx, clientActor, d.ID)
	_ = svc.Start(ctx, admin, d.ID, time.Now().AddDate(0, 0, -10))
	if err := svc.Finish(ctx, admin, d.ID, time.Now().AddDate(0, 0, -10), time.Now().AddDate(0, 0, -8), 4); err != nil {
		t.Fatalf("完成需求失败: %v", err)
	}

	return d.ID
}

func TestScanExpired(t *testing.T) {
	_, demandSvc, _, runner := newEnv(t, "scan")
	ctx := context.Background()

	id := finishDemand(t, demandSvc, "过期需求")

	// 未过期：扫描不动它（deadline 是 now + 5 天）
	if err := runner.ScanExpired(ctx, time.Now()); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	d, _ := demandSvc.Get(ctx, id)
	if d.Status.String() != "pending_acceptance" {
		t.Fatalf("未过期需求不应被确认, status = %s", d.Status)
	}

	// 模拟 6 天后：应被自动确认
	if err := runner.ScanExpired(ctx, time.Now().AddDate(0, 0, 6)); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	d, _ = demandSvc.Get(ctx, id)
	if d.Status.String() != "accepted" || !d.AcceptAuto || d.AcceptLocked {
		t.Errorf("过期需求应被自动确认: status=%s auto=%v locked=%v", d.Status, d.AcceptAuto, d.AcceptLocked)
	}

	// 幂等：重复扫描无副作用
	if err := runner.ScanExpired(ctx, time.Now().AddDate(0, 0, 7)); err != nil {
		t.Errorf("重复扫描应无副作用: %v", err)
	}
}

func TestEnsurePrevBill(t *testing.T) {
	client, _, _, runner := newEnv(t, "ensure")
	ctx := context.Background()

	now := time.Date(2026, 8, 4, 0, 15, 0, 0, time.Local)

	// 首次执行生成上月账单
	if err := runner.EnsurePrevBill(ctx, now); err != nil {
		t.Fatalf("补生成失败: %v", err)
	}
	b := client.Bill.Query().Where(bill.Period("2026-07")).OnlyX(ctx)
	if b.Status.String() != "draft" {
		t.Errorf("生成的账单应为草稿, got %s", b.Status)
	}

	// 幂等：已存在则跳过，不重建
	if err := runner.EnsurePrevBill(ctx, now); err != nil {
		t.Fatalf("重复执行失败: %v", err)
	}
	if n := client.Bill.Query().CountX(ctx); n != 1 {
		t.Errorf("账单数量 = %d, want 1", n)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/task/ -v`
Expected: FAIL

- [ ] **Step 3: 实现**

`internal/task/task.go`：

```go
package task

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/bill"
	"clepsydra/internal/ent/demand"
	"clepsydra/internal/service"
)

// Runner 定时任务运行器，所有任务均为幂等扫描式
type Runner struct {
	client  *ent.Client
	setting *service.Setting
	demand  *service.Demand
	bill    *service.Bill
	rotator *lumberjack.Logger
	log     zerolog.Logger
	cron    *cron.Cron
}

// New 构建定时任务运行器，rotator 为 nil 时跳过日志轮转任务
func New(client *ent.Client, setting *service.Setting, demandSvc *service.Demand, billSvc *service.Bill, rotator *lumberjack.Logger, log zerolog.Logger) *Runner {
	return &Runner{
		client:  client,
		setting: setting,
		demand:  demandSvc,
		bill:    billSvc,
		rotator: rotator,
		log:     log,
		cron:    cron.New(),
	}
}

// Start 注册并启动全部定时任务，启动时先补生成上月账单
func (r *Runner) Start() {
	ctx := context.Background()

	// 启动自检：宕机漏跑的上月账单补生成
	if err := r.EnsurePrevBill(ctx, time.Now()); err != nil {
		r.log.Error().Err(err).Msg("启动补生成上月账单失败")
	}

	// 每日零点切割日志文件
	if r.rotator != nil {
		_, _ = r.cron.AddFunc("0 0 * * *", func() {
			if err := r.rotator.Rotate(); err != nil {
				r.log.Error().Err(err).Msg("日志轮转失败")
			}
		})
	}

	// 每日 00:05 扫描过期未确认
	_, _ = r.cron.AddFunc("5 0 * * *", func() {
		if err := r.ScanExpired(context.Background(), time.Now()); err != nil {
			r.log.Error().Err(err).Msg("自动确认扫描失败")
		}
	})

	// 每月 1 日 00:10 生成上月账单（内含出账前锁定）
	_, _ = r.cron.AddFunc("10 0 1 * *", func() {
		if err := r.EnsurePrevBill(context.Background(), time.Now()); err != nil {
			r.log.Error().Err(err).Msg("生成上月账单失败")
		}
	})

	r.cron.Start()
}

// Stop 停止定时任务
func (r *Runner) Stop() {
	r.cron.Stop()
}

// ScanExpired 自动确认所有过期未确认的需求与账单
func (r *Runner) ScanExpired(ctx context.Context, now time.Time) error {
	// 过期需求自动确认
	demands, err := r.client.Demand.Query().Where(
		demand.StatusEQ(demand.StatusPendingAcceptance),
		demand.AcceptDeadlineLT(now),
	).All(ctx)
	if err != nil {
		return err
	}
	for _, d := range demands {
		if err = r.demand.Accept(ctx, service.SystemActor, d.ID, true, false); err != nil {
			return err
		}
		r.log.Info().Int("demand_id", d.ID).Msg("需求逾期自动确认")
	}

	// 过期账单自动确认
	bills, err := r.client.Bill.Query().Where(
		bill.StatusEQ(bill.StatusPending),
		bill.ConfirmDeadlineLT(now),
	).All(ctx)
	if err != nil {
		return err
	}
	for _, b := range bills {
		if err = r.bill.Confirm(ctx, service.SystemActor, b.ID, true); err != nil {
			return err
		}
		r.log.Info().Int("bill_id", b.ID).Str("period", b.Period).Msg("账单逾期自动确认")
	}

	return nil
}

// EnsurePrevBill 确保上月账单已生成，不存在则生成（内含出账前锁定）
func (r *Runner) EnsurePrevBill(ctx context.Context, now time.Time) error {
	period := service.PrevPeriod(now)

	exists, err := r.client.Bill.Query().Where(bill.Period(period)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = r.bill.Generate(ctx, service.SystemActor, period)
	if err == nil {
		r.log.Info().Str("period", period).Msg("已生成上月账单草稿")
	}

	return err
}
```

`internal/api/router.go`：

```go
package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"clepsydra/internal/api/handler"
	"clepsydra/internal/service"
)

// Handlers 全部 handler 集合
type Handlers struct {
	Auth      *handler.Auth
	User      *handler.User
	Setting   *handler.Setting
	Demand    *handler.Demand
	Bill      *handler.Bill
	Dashboard *handler.Dashboard
	AuditLog  *handler.AuditLog
}

// Register 注册全部路由
func Register(e *echo.Echo, auth *service.Auth, h Handlers) {
	e.Use(middleware.Recover(), middleware.CORS())

	root := e.Group("/api")
	root.POST("/auth/login", h.Auth.Login)

	// 登录可访问
	authed := root.Group("", RequireAuth(auth))
	authed.GET("/dashboard/todos", h.Dashboard.Todos)
	authed.GET("/demands", h.Demand.List)
	authed.GET("/demands/:id", h.Demand.Get)
	authed.POST("/demands/:id/confirm-estimate", h.Demand.ConfirmEstimate)
	authed.POST("/demands/:id/accept", h.Demand.Accept)
	authed.GET("/bills", h.Bill.List)
	authed.GET("/bills/:id", h.Bill.Get)
	authed.POST("/bills/:id/confirm", h.Bill.Confirm)

	// 仅超级管理员
	adminGroup := authed.Group("", RequireAdmin)
	adminGroup.GET("/users", h.User.List)
	adminGroup.POST("/users", h.User.Create)
	adminGroup.PUT("/users/:id", h.User.Update)
	adminGroup.PUT("/users/:id/password", h.User.ResetPassword)
	adminGroup.GET("/settings", h.Setting.All)
	adminGroup.PUT("/settings", h.Setting.Update)
	adminGroup.GET("/holidays", h.Setting.Holidays)
	adminGroup.PUT("/holidays", h.Setting.SaveHolidays)
	adminGroup.DELETE("/holidays/:date", h.Setting.DeleteHoliday)
	adminGroup.POST("/demands", h.Demand.Create)
	adminGroup.PUT("/demands/:id", h.Demand.Update)
	adminGroup.POST("/demands/:id/submit-estimate", h.Demand.SubmitEstimate)
	adminGroup.POST("/demands/:id/start", h.Demand.Start)
	adminGroup.POST("/demands/:id/finish", h.Demand.Finish)
	adminGroup.POST("/bills/generate", h.Bill.Generate)
	adminGroup.POST("/bills/:id/items/:itemId/waive", h.Bill.ToggleWaive)
	adminGroup.POST("/bills/:id/share", h.Bill.Share)
	adminGroup.POST("/bills/:id/revoke", h.Bill.Revoke)
	adminGroup.GET("/audit-logs", h.AuditLog.List)
}
```

`internal/api/handler/auditlog.go` — 构造函数 `handler.NewAuditLog(audit *service.Audit) *AuditLog`，`List` 方法解析 query 参数（`strconv.Atoi` 容错取默认值）后调用 `audit.List`（Task 8 已定义），响应 `api.OK(c, map[string]any{"total": total, "rows": rows})`。

`internal/service/notifier.go` — 预留通知接口（spec 要求，本期只有空实现）：

```go
package service

import "context"

// Notifier 通知接口，本期无外部通知渠道，预留给后续邮件或微信实现
// 接入点：需求自动确认、账单分享与自动确认后各调用一次对应方法
type Notifier interface {
	// DemandAccepted 需求被确认（含自动确认）后通知
	DemandAccepted(ctx context.Context, demandID int, auto bool)
	// BillShared 账单分享后通知需求方
	BillShared(ctx context.Context, billID int)
	// BillConfirmed 账单被确认（含自动确认）后通知
	BillConfirmed(ctx context.Context, billID int, auto bool)
}

// NopNotifier 空通知实现
type NopNotifier struct{}

func (NopNotifier) DemandAccepted(ctx context.Context, demandID int, auto bool) {}
func (NopNotifier) BillShared(ctx context.Context, billID int)                  {}
func (NopNotifier) BillConfirmed(ctx context.Context, billID int, auto bool)    {}
```

本期不在业务代码中注入 Notifier 调用（避免无意义的 no-op 调用点），接口文件仅声明契约；后续接入通知时在 `Demand.Accept`、`Bill.Share`、`Bill.Confirm` 尾部各加一次调用即可。

`internal/workday/holidaycn.go`：

```go
package workday

import "encoding/json"

// holidayCNFile holiday-cn（github.com/NateScarlet/holiday-cn）年度数据文件结构
type holidayCNFile struct {
	Year int `json:"year"`
	Days []struct {
		Name     string `json:"name"`
		Date     string `json:"date"`
		IsOffDay bool   `json:"isOffDay"`
	} `json:"days"`
}

// ParseHolidayCN 解析 holiday-cn 年度 JSON 为节假日条目
// isOffDay 为 true 表示放假，false 表示调休补班
func ParseHolidayCN(data []byte) ([]Entry, error) {
	var file holidayCNFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(file.Days))
	for _, d := range file.Days {
		entryType := "workday"
		if d.IsOffDay {
			entryType = "holiday"
		}
		entries = append(entries, Entry{Date: d.Date, Type: entryType, Name: d.Name})
	}

	return entries, nil
}
```

`internal/workday/holidaycn_test.go`：

```go
package workday

import "testing"

func TestParseHolidayCN(t *testing.T) {
	data := []byte(`{
		"year": 2026,
		"days": [
			{"name": "元旦", "date": "2026-01-01", "isOffDay": true},
			{"name": "春节调休", "date": "2026-02-15", "isOffDay": false}
		]
	}`)

	entries, err := ParseHolidayCN(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("条目数 = %d, want 2", len(entries))
	}
	if entries[0].Type != "holiday" || entries[0].Date != "2026-01-01" {
		t.Errorf("放假日解析错误: %+v", entries[0])
	}
	if entries[1].Type != "workday" || entries[1].Name != "春节调休" {
		t.Errorf("调休日解析错误: %+v", entries[1])
	}

	// 非法 JSON 返回错误
	if _, err = ParseHolidayCN([]byte("not json")); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}
```

`cmd/clepsydra/main.go`：

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/labstack/echo/v4"
	zlog "github.com/rs/zerolog/log"

	"clepsydra/internal/api"
	"clepsydra/internal/api/handler"
	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/logger"
	"clepsydra/internal/service"
	"clepsydra/internal/task"
	"clepsydra/internal/workday"
)

func main() {
	configPath := flag.String("c", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置与日志
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}
	log, rotator := logger.New(cfg.Log, cfg.Server.Mode == "debug")
	zlog.Logger = log // 同步全局 logger，audit 等包的 zerolog/log 输出与主日志一致

	// 连接数据库并迁移
	client, err := ent.Open("postgres", cfg.Database.DSN)
	if err != nil {
		log.Fatal().Err(err).Msg("连接数据库失败")
	}
	defer client.Close()

	ctx := context.Background()
	if err = client.Schema.Create(ctx); err != nil {
		log.Fatal().Err(err).Msg("数据库迁移失败")
	}

	// 加载 holiday-cn 格式的节假日数据并种子，文件缺失或格式错误时跳过导入
	var entries []workday.Entry
	if data, err := os.ReadFile(cfg.Holiday.File); err == nil {
		entries, _ = workday.ParseHolidayCN(data)
	}
	if err = service.Seed(ctx, client, cfg.Admin, entries); err != nil {
		log.Fatal().Err(err).Msg("初始化基础数据失败")
	}

	// 手动装配服务与 handler
	settingSvc := service.NewSetting(client)
	audit := service.NewAudit(client)
	authSvc := service.NewAuth(client, cfg.JWT)
	userSvc := service.NewUser(client)
	holidaySvc := service.NewHolidaySvc(client)
	demandSvc := service.NewDemand(client, settingSvc, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	dashboardSvc := service.NewDashboard(client, settingSvc)

	handlers := api.Handlers{
		Auth:      handler.NewAuth(authSvc),
		User:      handler.NewUser(userSvc),
		Setting:   handler.NewSetting(settingSvc, holidaySvc),
		Demand:    handler.NewDemand(demandSvc),
		Bill:      handler.NewBill(billSvc),
		Dashboard: handler.NewDashboard(dashboardSvc),
		AuditLog:  handler.NewAuditLog(audit),
	}

	// 启动定时任务
	runner := task.New(client, settingSvc, demandSvc, billSvc, rotator, log)
	runner.Start()
	defer runner.Stop()

	// 启动 HTTP 服务并优雅退出
	e := echo.New()
	e.HideBanner = true
	api.Register(e, authSvc, handlers)

	go func() {
		if err := e.Start(cfg.Server.Address); err != nil {
			log.Info().Err(err).Msg("HTTP 服务退出")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = e.Shutdown(shutdownCtx)
}
```

依赖安装：

```bash
go get github.com/robfig/cron/v3@latest github.com/lib/pq@latest
```

`Makefile`：

```makefile
.PHONY: build run test lint generate

build:
	go build -o bin/clepsydra ./cmd/clepsydra

run:
	go run ./cmd/clepsydra -c configs/config.yaml

test:
	go test ./... -count=1

lint:
	gclint run --config .golangci.yml --timeout=10m

generate:
	go generate ./internal/ent/...
```

`.golangci.yml`：

```yaml
run:
  timeout: 10m

linters:
  enable:
    - govet
    - staticcheck
    - errcheck
    - ineffassign
    - unused
    - misspell

issues:
  exclude-dirs:
    - internal/ent
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
```

- [ ] **Step 4: 运行全部测试与构建确认通过**

Run: `go test ./... -count=1 && go build -o bin/clepsydra ./cmd/clepsydra`
Expected: 全部 PASS，构建成功

- [ ] **Step 5: 提交**

```bash
git add internal/ cmd/ Makefile .golangci.yml
git commit -m "feat: 添加定时任务、路由装配与服务入口"
```

---

## 收尾验证（计划完成检查）

- [ ] `go test ./... -count=1` 全部通过
- [ ] `gclint run --config .golangci.yml --new-from-rev=HEAD~1 --timeout=10m` 无 issue
- [ ] 本地起 PostgreSQL 后 `make run`，用 curl 走通：登录 → 创建需求 → 提交/确认预估 → 开工 → 完成 → 验收 → 生成账单 → 减免 → 分享 → 确认
- [ ] 核对 `assets/holidays/2026.json` 与 holiday-cn 上游一致（该数据集跟随国务院办公厅公告维护）
