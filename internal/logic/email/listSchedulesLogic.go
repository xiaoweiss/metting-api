package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListSchedulesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSchedulesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSchedulesLogic {
	return &ListSchedulesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListSchedulesLogic) ListSchedules() (resp *types.EmailScheduleListResp, err error) {
	var rows []struct {
		Id        int64
		GroupId   int64
		GroupName string
		CronExpr  string
		Enabled   bool
	}
	if err = l.svcCtx.DB.Raw(`
		SELECT s.id        AS id,
		       s.group_id  AS group_id,
		       IFNULL(g.name,'') AS group_name,
		       s.cron_expr AS cron_expr,
		       s.enabled   AS enabled
		FROM email_schedules s
		LEFT JOIN email_groups g ON g.id = s.group_id
		ORDER BY s.id DESC
	`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	resp = &types.EmailScheduleListResp{List: []types.EmailScheduleItem{}}
	for _, r := range rows {
		resp.List = append(resp.List, types.EmailScheduleItem{
			Id:        r.Id,
			GroupId:   r.GroupId,
			GroupName: r.GroupName,
			CronExpr:  r.CronExpr,
			Enabled:   r.Enabled,
		})
	}
	return resp, nil
}
