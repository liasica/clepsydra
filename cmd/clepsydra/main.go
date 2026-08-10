package main

import (
	"context"
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

	// 注册 schema 上声明的 hook 与 interceptor，需求软删除依赖它们生效；
	// 缺了这个空导入，查询会直接报 uninitialized interceptor
	_ "clepsydra/internal/ent/runtime"

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

	// JWT secret 是签发与校验登录令牌的唯一密钥，为空将导致任意伪造 token 通过校验
	if cfg.JWT.Secret == "" {
		log.Fatal().Msg("jwt.secret 不能为空")
	}

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

	// 加载 holiday-cn 格式的节假日数据并种子，读取或解析失败时跳过导入并告警
	var entries []workday.Entry
	var data []byte
	data, err = os.ReadFile(cfg.Holiday.File)
	if err != nil {
		log.Warn().Err(err).Str("file", cfg.Holiday.File).Msg("节假日数据文件读取失败，跳过导入")
	} else if entries, err = workday.ParseHolidayCN(data); err != nil {
		log.Warn().Err(err).Str("file", cfg.Holiday.File).Msg("节假日数据文件解析失败，跳过导入")
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
	projectSvc := service.NewProject(client, audit)
	billSvc := service.NewBill(client, settingSvc, demandSvc, audit)
	dashboardSvc := service.NewDashboard(client, settingSvc)

	uploadSvc, err := service.NewUpload(cfg.Upload)
	if err != nil {
		log.Fatal().Err(err).Str("dir", cfg.Upload.Dir).Msg("初始化上传目录失败")
	}

	handlers := api.Handlers{
		Auth:      handler.NewAuth(authSvc),
		User:      handler.NewUser(userSvc),
		Setting:   handler.NewSetting(settingSvc, holidaySvc),
		Demand:    handler.NewDemand(demandSvc),
		Project:   handler.NewProject(projectSvc),
		Bill:      handler.NewBill(billSvc),
		Dashboard: handler.NewDashboard(dashboardSvc),
		AuditLog:  handler.NewAuditLog(audit),
		Upload:    handler.NewUpload(uploadSvc),
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
