package sync

import (
	"context"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetSyncScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSyncScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSyncScheduleLogic {
	return &GetSyncScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetSyncScheduleLogic) GetSyncSchedule() (*types.SyncScheduleResp, error) {
	var schedule model.SyncSchedule
	l.svcCtx.DB.First(&schedule)

	resp := &types.SyncScheduleResp{
		CronExpr: schedule.CronExpr,
		Enabled:  schedule.Enabled,
	}

	nextRun := l.svcCtx.SyncScheduler.NextRun()
	if !nextRun.IsZero() {
		resp.NextRun = nextRun.Format(time.RFC3339)
	}

	return resp, nil
}
