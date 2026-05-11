package main

import (
	"context"
	"flag"
	"fmt"

	"meeting/internal/config"
	"meeting/internal/handler"
	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/pkg/blast"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/meeting-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// 启动数据同步调度器
	if c.DingTalk.Sheet.BaseId != "" {
		// 从 DB 读取持久化的调度配置
		var schedule model.SyncSchedule
		if err := ctx.DB.First(&schedule).Error; err == nil && schedule.Enabled {
			if err := ctx.SyncScheduler.Start(schedule.CronExpr); err != nil {
				logx.Errorf("[DataSync] 启动调度器失败: %v", err)
			}
		} else if c.Sync.AutoStart {
			if err := ctx.SyncScheduler.Start(c.Sync.CronExpr); err != nil {
				logx.Errorf("[DataSync] 启动调度器失败: %v", err)
			}
		}

		// 启动时执行一次初始同步（不阻塞服务器启动）
		go func() {
			if err := ctx.SyncEngine.RunFullSync(context.Background()); err != nil {
				logx.Errorf("[DataSync] 初始同步失败: %v", err)
			}
		}()
	}

	// 启动每日更新检测调度器：DB 配置优先，回退 yaml
	{
		var ucSchedule model.UpdateCheckSchedule
		if err := ctx.DB.First(&ucSchedule).Error; err == nil && ucSchedule.Enabled {
			if err := ctx.CheckScheduler.Start(ucSchedule.CronExpr); err != nil {
				logx.Errorf("[UpdateCheck] 启动调度器失败: %v", err)
			}
		} else if c.UpdateCheck.AutoStart && c.UpdateCheck.CronExpr != "" {
			if err := ctx.CheckScheduler.Start(c.UpdateCheck.CronExpr); err != nil {
				logx.Errorf("[UpdateCheck] 启动调度器失败: %v", err)
			}
		}
	}

	// 启动全员群发邮件调度器:载入 DB 里所有 enabled=true 的 schedule,各自注册 cron entry
	{
		var rows []model.MailBlastSchedule
		ctx.DB.Where("enabled = ? AND template_id > 0", true).Find(&rows)
		for _, bs := range rows {
			if err := ctx.BlastScheduler.Add(bs.Id, bs.CronExpr); err != nil {
				logx.Errorf("[Blast] 加载调度 id=%d (%s) 失败: %v", bs.Id, bs.Name, err)
			}
		}
		logx.Infof("[Blast] 启动加载 %d 条群发调度", len(rows))
	}

	// 启动看板截图清理调度器(每天 03:00 删 30 天前的截图)
	{
		cs := blast.NewCleanupScheduler(ctx.DB, c)
		if err := cs.Start(); err != nil {
			logx.Errorf("[SnapshotCleanup] 启动失败: %v", err)
		}
	}

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
