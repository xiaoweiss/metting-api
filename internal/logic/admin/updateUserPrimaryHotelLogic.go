package admin

import (
	"context"
	"errors"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserPrimaryHotelLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserPrimaryHotelLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserPrimaryHotelLogic {
	return &UpdateUserPrimaryHotelLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateUserPrimaryHotelLogic) UpdateUserPrimaryHotel(req *types.UpdateUserPrimaryHotelReq) (*types.BaseResp, error) {
	// 校验酒店存在（如果非 0）
	if req.PrimaryHotelId > 0 {
		var h model.Hotel
		if err := l.svcCtx.DB.Select("id").First(&h, req.PrimaryHotelId).Error; err != nil {
			return nil, errors.New("酒店不存在")
		}
	}

	// 0 → 清除（写 NULL）。
	// 用 column 写法避免 *int64 指针字段被 GORM 当作零值跳过更新。
	updateVal := interface{}(nil)
	if req.PrimaryHotelId > 0 {
		updateVal = req.PrimaryHotelId
	}
	if err := l.svcCtx.DB.Table("users").Where("id = ?", req.Id).
		Update("primary_hotel_id", updateVal).Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Message: "ok"}, nil
}
