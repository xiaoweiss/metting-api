package blast

import (
	"context"
	"sync"
	"time"

	"meeting/pkg/cronx"

	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/logx"
)

// Scheduler 管理 N 条独立的全员群发调度。每条 schedule(DB row)对应一个 cron entry。
type Scheduler struct {
	cron    *cron.Cron
	engine  *Engine
	mu      sync.Mutex
	entries map[int64]cron.EntryID // schedule_id → cron entry id
	exprs   map[int64]string       // schedule_id → 规范化后的 cron expr
}

func NewScheduler(engine *Engine) *Scheduler {
	s := &Scheduler{
		cron:    cronx.New(),
		engine:  engine,
		entries: make(map[int64]cron.EntryID),
		exprs:   make(map[int64]string),
	}
	s.cron.Start()
	return s
}

// Add 给某条 schedule 注册一个 cron entry。如果已存在,先 Remove 再加。
func (s *Scheduler) Add(scheduleId int64, cronExpr string) error {
	expr, err := cronx.Normalize(cronExpr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.entries[scheduleId]; ok {
		s.cron.Remove(old)
	}
	id, err := s.cron.AddFunc(expr, func() {
		if _, err := s.engine.RunBlastById(context.Background(), scheduleId); err != nil {
			logx.Errorf("[Blast] 定时群发 schedule=%d 失败: %v", scheduleId, err)
		}
	})
	if err != nil {
		return err
	}
	s.entries[scheduleId] = id
	s.exprs[scheduleId] = expr
	logx.Infof("[Blast] 注册调度 schedule=%d cron=%s", scheduleId, expr)
	return nil
}

// Remove 取消某条 schedule 的 cron entry。
func (s *Scheduler) Remove(scheduleId int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[scheduleId]; ok {
		s.cron.Remove(id)
		delete(s.entries, scheduleId)
		delete(s.exprs, scheduleId)
		logx.Infof("[Blast] 注销调度 schedule=%d", scheduleId)
	}
}

// NextRun 返回某条 schedule 的下次触发时间(没注册返回零值)
func (s *Scheduler) NextRun(scheduleId int64) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[scheduleId]; ok {
		return s.cron.Entry(id).Next
	}
	return time.Time{}
}

// Stop 停所有 cron entries(进程退出用)
func (s *Scheduler) Stop() { s.cron.Stop() }
