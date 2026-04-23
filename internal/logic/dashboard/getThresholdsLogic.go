package dashboard

import (
	"context"
	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/cache"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetThresholdsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetThresholdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetThresholdsLogic {
	return &GetThresholdsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetThresholdsLogic) GetThresholds(req *types.ThresholdReq) (resp *types.ThresholdResp, err error) {
	cacheKey := cache.ThresholdKey(req.HotelId)
	resp = &types.ThresholdResp{}
	if cache.Get(l.ctx, l.svcCtx.Redis, cacheKey, resp) {
		return resp, nil
	}

	// 优先取酒店专属阈值，无则取全局默认（hotel_id IS NULL）
	var rows []model.ColorThreshold
	l.svcCtx.DB.Raw(`
		SELECT * FROM color_thresholds
		WHERE hotel_id = ? OR hotel_id IS NULL
		ORDER BY hotel_id DESC, min_value ASC`,
		req.HotelId,
	).Scan(&rows)

	// 去重：同 metric_type+level 以酒店专属为准
	seen := map[string]bool{}
	for _, r := range rows {
		key := r.MetricType + "_" + r.Level
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

	cache.Set(l.ctx, l.svcCtx.Redis, cacheKey, resp)
	return resp, nil
}
