package email

import (
	"context"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteScheduleLogic {
	return &DeleteScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteScheduleLogic) DeleteSchedule(req *types.ScheduleIdReq) (resp *types.BaseResp, err error) {
	if err := l.svcCtx.DB.Where("id = ?", req.Id).Delete(&model.EmailSchedule{}).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
