package svc

import (
	"context"
	"fmt"
	"meeting/internal/config"
	"meeting/internal/middleware"
	"meeting/pkg/blast"
	"meeting/pkg/check"
	"meeting/pkg/dingtalk"
	pkgsync "meeting/pkg/sync"
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ServiceContext struct {
	Config         config.Config
	DB             *gorm.DB
	Redis          *redis.Client
	Auth           func(http.HandlerFunc) http.HandlerFunc
	AdminOnly      func(http.HandlerFunc) http.HandlerFunc
	SyncEngine     *pkgsync.Engine
	SyncScheduler  *pkgsync.Scheduler
	CheckEngine    *check.Engine
	CheckScheduler *check.Scheduler
	BlastEngine    *blast.Engine
	BlastScheduler *blast.Scheduler
	DTClient       *dingtalk.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := gorm.Open(mysql.Open(c.DB.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic(fmt.Sprintf("连接数据库失败: %v", err))
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(c.DB.MaxOpenConn)
	sqlDB.SetMaxIdleConns(c.DB.MaxIdleConn)

	rdb := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Addr,
		Password: c.Redis.Password,
		DB:       c.Redis.DB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic(fmt.Sprintf("连接 Redis 失败: %v", err))
	}

	// 初始化钉钉表格客户端
	dtClient := &dingtalk.Client{
		AppKey:    c.DingTalk.AppKey,
		AppSecret: c.DingTalk.AppSecret,
	}
	sheetClient := &dingtalk.SheetClient{
		Client:     dtClient,
		BaseId:     c.DingTalk.Sheet.BaseId,
		OperatorId: c.DingTalk.Sheet.OperatorId,
	}

	// 初始化同步引擎和调度器
	syncEngine := pkgsync.NewEngine(db, rdb, sheetClient, c)
	syncScheduler := pkgsync.NewScheduler(syncEngine)

	// 初始化检测引擎 + 调度器（每日未更新提醒）
	checkEngine := check.NewEngine(db, c, dtClient)
	checkScheduler := check.NewScheduler(checkEngine)

	// 初始化全员邮件群发引擎 + 调度器
	blastEngine := blast.NewEngine(db, c)
	blastScheduler := blast.NewScheduler(blastEngine)

	return &ServiceContext{
		Config:         c,
		DB:             db,
		Redis:          rdb,
		Auth:           middleware.NewAuthMiddleware(c.JWT.Secret),
		AdminOnly:      middleware.NewAdminOnlyMiddleware(db),
		SyncEngine:     syncEngine,
		SyncScheduler:  syncScheduler,
		CheckEngine:    checkEngine,
		CheckScheduler: checkScheduler,
		BlastEngine:    blastEngine,
		BlastScheduler: blastScheduler,
		DTClient:       dtClient,
	}
}
