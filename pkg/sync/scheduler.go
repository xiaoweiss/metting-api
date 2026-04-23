package sync

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/logx"
)

type Scheduler struct {
	cron    *cron.Cron
	engine  *Engine
	entryID cron.EntryID
}

func NewScheduler(engine *Engine) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		engine: engine,
	}
}

// Start 启动定时同步
func (s *Scheduler) Start(cronExpr string) error {
	id, err := s.cron.AddFunc(cronExpr, func() {
		ctx := context.Background()
		if err := s.engine.RunFullSync(ctx); err != nil {
			logx.Errorf("[DataSync] 定时同步失败: %v", err)
		}
	})
	if err != nil {
		return err
	}
	s.entryID = id
	s.cron.Start()
	logx.Infof("[DataSync] 调度器已启动，cron: %s", cronExpr)
	return nil
}

// UpdateSchedule 运行时更新 cron 表达式
func (s *Scheduler) UpdateSchedule(cronExpr string) error {
	s.cron.Remove(s.entryID)
	id, err := s.cron.AddFunc(cronExpr, func() {
		ctx := context.Background()
		if err := s.engine.RunFullSync(ctx); err != nil {
			logx.Errorf("[DataSync] 定时同步失败: %v", err)
		}
	})
	if err != nil {
		return err
	}
	s.entryID = id
	logx.Infof("[DataSync] 调度已更新，cron: %s", cronExpr)
	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// NextRun 返回下次执行时间
func (s *Scheduler) NextRun() time.Time {
	entry := s.cron.Entry(s.entryID)
	return entry.Next
}
