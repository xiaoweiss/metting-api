package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateHotelLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateHotelLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateHotelLogic {
	return &CreateHotelLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateHotelLogic) CreateHotel(req *types.CreateHotelReq) (resp *types.BaseResp, err error) {
	if req.Name == "" {
		return &types.BaseResp{Code: 400, Message: "酒店名称不能为空"}, nil
	}
	if err = l.svcCtx.DB.Exec(
		"INSERT INTO hotels (name, city, market_area_id) VALUES (?, ?, ?)",
		req.Name, req.City, req.MarketAreaId,
	).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Code: 0, Message: "ok"}, nil
}
