package blast

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/logx"

	"meeting/internal/config"
	"meeting/internal/model"
	"meeting/pkg/cronx"

	"gorm.io/gorm"
)

// CleanupScheduler 每天凌晨 03:00 删除 30 天前的看板截图(数据库行 + 物理文件)。
type CleanupScheduler struct {
	cron    *cron.Cron
	db      *gorm.DB
	cfg     config.Config
	entryID cron.EntryID
}

func NewCleanupScheduler(db *gorm.DB, cfg config.Config) *CleanupScheduler {
	return &CleanupScheduler{cron: cronx.New(), db: db, cfg: cfg}
}

func (s *CleanupScheduler) Start() error {
	expr := "0 0 3 * * *" // 每天 03:00
	id, err := s.cron.AddFunc(expr, func() {
		s.runOnce(context.Background())
	})
	if err != nil {
		return err
	}
	s.entryID = id
	s.cron.Start()
	logx.Infof("[SnapshotCleanup] 调度器已启动, cron: %s", expr)
	return nil
}

func (s *CleanupScheduler) Stop() { s.cron.Stop() }

func (s *CleanupScheduler) runOnce(_ context.Context) {
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	var rows []model.DashboardSnapshot
	if err := s.db.Where("uploaded_at < ?", cutoff).Find(&rows).Error; err != nil {
		logx.Errorf("[SnapshotCleanup] 查询过期记录失败: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	for _, r := range rows {
		abs := filepath.Join(s.cfg.Mail.SnapshotDir, r.FilePath)
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			logx.Errorf("[SnapshotCleanup] 删文件失败 %s: %v", abs, err)
		}
	}
	if err := s.db.Where("uploaded_at < ?", cutoff).Delete(&model.DashboardSnapshot{}).Error; err != nil {
		logx.Errorf("[SnapshotCleanup] 删 DB 行失败: %v", err)
		return
	}
	logx.Infof("[SnapshotCleanup] 删除 %d 条过期截图", len(rows))
}
