package sync

import (
	"context"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetSyncStatusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSyncStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSyncStatusLogic {
	return &GetSyncStatusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetSyncStatusLogic) GetSyncStatus() (*types.SyncStatusResp, error) {
	var log model.SyncLog
	result := l.svcCtx.DB.Where("source = ?", "full_sync").Order("synced_at DESC").First(&log)

	resp := &types.SyncStatusResp{
		Status:  "never",
		Message: "尚未执行过同步",
	}

	if result.Error == nil {
		resp.LastSyncAt = log.SyncedAt.Format(time.RFC3339)
		resp.Status = log.Status
		resp.Message = log.Message
	}

	nextRun := l.svcCtx.SyncScheduler.NextRun()
	if !nextRun.IsZero() {
		resp.NextRunAt = nextRun.Format(time.RFC3339)
	}

	return resp, nil
}
