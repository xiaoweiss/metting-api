package admin

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAdminThresholdsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAdminThresholdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAdminThresholdsLogic {
	return &GetAdminThresholdsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetAdminThresholdsLogic) GetAdminThresholds(req *types.AdminThresholdReq) (resp *types.ThresholdResp, err error) {
	var rows []struct {
		MetricType string
		Level      string
		MinValue   float64
		MaxValue   float64
		Color      string
	}
	// 优先取酒店级，不存在则取全局（hotel_id IS NULL）
	if err = l.svcCtx.DB.Raw(`
		SELECT metric_type, level, min_value, max_value, color
		FROM color_thresholds
		WHERE hotel_id = ? OR hotel_id IS NULL
		ORDER BY hotel_id DESC, metric_type, FIELD(level,'low','medium','high')`,
		req.HotelId,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 去重：同一 (metric_type, level) 优先用酒店级（排序已保证在前）
	seen := map[string]bool{}
	resp = &types.ThresholdResp{}
	for _, r := range rows {
		key := r.MetricType + ":" + r.Level
		if seen[key] {
			continue
		}
		seen[key] = true
		resp.List = append(resp.List, types.ThresholdItem{
			MetricType: r.MetricType,
			Level:      r.Level,
			MinValue:   r.MinValue,
			MaxValue:   r.MaxValue,
			Color:      r.Color,
		})
	}
	return resp, nil
}
