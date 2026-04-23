package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SaveAdminThresholdsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSaveAdminThresholdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SaveAdminThresholdsLogic {
	return &SaveAdminThresholdsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SaveAdminThresholdsLogic) SaveAdminThresholds(req *types.SaveThresholdReq) (resp *types.BaseResp, err error) {
	tx := l.svcCtx.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	// 删除该酒店（或全局）旧阈值
	var delSQL string
	var delArgs []interface{}
	if req.HotelId == 0 {
		delSQL = "DELETE FROM color_thresholds WHERE hotel_id IS NULL"
	} else {
		delSQL = "DELETE FROM color_thresholds WHERE hotel_id = ?"
		delArgs = []interface{}{req.HotelId}
	}
	if err = tx.Exec(delSQL, delArgs...).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 批量插入新阈值
	for _, item := range req.List {
		var insertSQL string
		var args []interface{}
		if req.HotelId == 0 {
			insertSQL = "INSERT INTO color_thresholds (hotel_id, metric_type, level, min_value, max_value, color) VALUES (NULL,?,?,?,?,?)"
			args = []interface{}{item.MetricType, item.Level, item.MinValue, item.MaxValue, item.Color}
		} else {
			insertSQL = "INSERT INTO color_thresholds (hotel_id, metric_type, level, min_value, max_value, color) VALUES (?,?,?,?,?,?)"
			args = []interface{}{req.HotelId, item.MetricType, item.Level, item.MinValue, item.MaxValue, item.Color}
		}
		if err = tx.Exec(insertSQL, args...).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err = tx.Commit().Error; err != nil {
		return nil, err
	}
	return &types.BaseResp{Code: 0, Message: "ok"}, nil
}
