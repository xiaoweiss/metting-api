package email

import (
	"context"
	"errors"
	"strings"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateScheduleLogic {
	return &CreateScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateScheduleLogic) CreateSchedule(req *types.CreateScheduleReq) (resp *types.BaseResp, err error) {
	if req.GroupId <= 0 {
		return nil, errors.New("groupId 不能为空")
	}
	cron := strings.TrimSpace(req.CronExpr)
	if cron == "" {
		return nil, errors.New("cron 表达式不能为空")
	}
	s := model.EmailSchedule{
		GroupId:  req.GroupId,
		CronExpr: cron,
		Enabled:  req.Enabled,
	}
	if err := l.svcCtx.DB.Create(&s).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
