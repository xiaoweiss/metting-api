package sync

import (
	"context"
	"fmt"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateSyncScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateSyncScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateSyncScheduleLogic {
	return &UpdateSyncScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateSyncScheduleLogic) UpdateSyncSchedule(req *types.UpdateSyncScheduleReq) (*types.SyncScheduleResp, error) {
	if req.CronExpr == "" {
		return nil, fmt.Errorf("cron 表达式不能为空")
	}

	// 更新调度器
	if req.Enabled {
		if err := l.svcCtx.SyncScheduler.UpdateSchedule(req.CronExpr); err != nil {
			return nil, fmt.Errorf("无效的 cron 表达式: %w", err)
		}
	} else {
		l.svcCtx.SyncScheduler.Stop()
	}

	// 持久化到 DB
	var schedule model.SyncSchedule
	l.svcCtx.DB.First(&schedule)
	l.svcCtx.DB.Model(&schedule).Updates(map[string]interface{}{
		"cron_expr": req.CronExpr,
		"enabled":   req.Enabled,
	})

	return &types.SyncScheduleResp{
		CronExpr: req.CronExpr,
		Enabled:  req.Enabled,
	}, nil
}
