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
