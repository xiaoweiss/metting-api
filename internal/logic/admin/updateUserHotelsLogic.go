package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserHotelsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserHotelsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserHotelsLogic {
	return &UpdateUserHotelsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateUserHotelsLogic) UpdateUserHotels(req *types.UpdateUserHotelsReq) (resp *types.BaseResp, err error) {
	tx := l.svcCtx.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	// 清除旧权限
	if err = tx.Exec("DELETE FROM user_hotel_perms WHERE user_id = ?", req.Id).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	// 插入新权限
	for _, hotelId := range req.HotelIds {
		if err = tx.Exec(
			"INSERT INTO user_hotel_perms (user_id, hotel_id) VALUES (?, ?)", req.Id, hotelId,
		).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err = tx.Commit().Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Code: 0, Message: "ok"}, nil
}
