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

type UpdateScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateScheduleLogic {
	return &UpdateScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateScheduleLogic) UpdateSchedule(req *types.UpdateScheduleReq) (resp *types.BaseResp, err error) {
	cron := strings.TrimSpace(req.CronExpr)
	if cron == "" {
		return nil, errors.New("cron 表达式不能为空")
	}
	updates := map[string]interface{}{
		"cron_expr": cron,
		"enabled":   req.Enabled,
	}
	if req.GroupId > 0 {
		updates["group_id"] = req.GroupId
	}
	if err := l.svcCtx.DB.Model(&model.EmailSchedule{}).
		Where("id = ?", req.Id).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
