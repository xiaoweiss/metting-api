package dashboard

import (
	"context"
	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCityEventsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCityEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCityEventsLogic {
	return &GetCityEventsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCityEventsLogic) GetCityEvents(req *types.CityEventReq) (resp *types.CityEventResp, err error) {
	var rows []model.CityEvent
	l.svcCtx.DB.Where("city = ? AND DATE_FORMAT(event_date,'%Y-%m-%d') = ?", req.City, req.Date).
		Find(&rows)

	resp = &types.CityEventResp{}
	for _, r := range rows {
		resp.List = append(resp.List, types.CityEventItem{
			VenueName: r.VenueName,
			EventName: r.EventName,
			EventType: r.EventType,
		})
	}
	return resp, nil
}
