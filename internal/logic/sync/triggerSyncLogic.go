package sync

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type TriggerSyncLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTriggerSyncLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TriggerSyncLogic {
	return &TriggerSyncLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *TriggerSyncLogic) TriggerSync() (*types.SyncTriggerResp, error) {
	go func() {
		ctx := context.Background()
		if err := l.svcCtx.SyncEngine.RunFullSync(ctx); err != nil {
			logx.Errorf("[DataSync] 手动同步失败: %v", err)
		}
	}()
	return &types.SyncTriggerResp{Code: 0, Message: "同步已触发"}, nil
}
