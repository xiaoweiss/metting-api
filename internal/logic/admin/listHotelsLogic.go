package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListHotelsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListHotelsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListHotelsLogic {
	return &ListHotelsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListHotelsLogic) ListHotels() (resp *types.HotelListResp, err error) {
	var hotels []struct {
		Id           int64
		Name         string
		City         string
		MarketAreaId int64
	}
	if err = l.svcCtx.DB.Raw(
		"SELECT id, name, city, market_area_id FROM hotels ORDER BY id",
	).Scan(&hotels).Error; err != nil {
		return nil, err
	}
	resp = &types.HotelListResp{}
	for _, h := range hotels {
		resp.List = append(resp.List, types.HotelItem{
			Id:           h.Id,
			Name:         h.Name,
			City:         h.City,
			MarketAreaId: h.MarketAreaId,
		})
	}
	return resp, nil
}
