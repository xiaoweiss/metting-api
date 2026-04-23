package check

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
	expr    string
}

func NewScheduler(engine *Engine) *Scheduler {
	return &Scheduler{cron: cron.New(), engine: engine}
}

func (s *Scheduler) Start(cronExpr string) error {
	id, err := s.cron.AddFunc(cronExpr, func() {
		_, err := s.engine.RunCheck(context.Background(), time.Time{})
		if err != nil {
			logx.Errorf("[UpdateCheck] 定时检测失败: %v", err)
		}
	})
	if err != nil {
		return err
	}
	s.entryID = id
	s.expr = cronExpr
	s.cron.Start()
	logx.Infof("[UpdateCheck] 调度器已启动，cron: %s", cronExpr)
	return nil
}

func (s *Scheduler) UpdateSchedule(cronExpr string) error {
	s.cron.Remove(s.entryID)
	return s.Start(cronExpr)
}

func (s *Scheduler) Stop() { s.cron.Stop() }

func (s *Scheduler) NextRun() time.Time {
	return s.cron.Entry(s.entryID).Next
}

func (s *Scheduler) CronExpr() string { return s.expr }
