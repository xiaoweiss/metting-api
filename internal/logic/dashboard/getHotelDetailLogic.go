package dashboard

import (
	"context"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetHotelDetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetHotelDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetHotelDetailLogic {
	return &GetHotelDetailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetHotelDetailLogic) GetHotelDetail(req *types.HotelDetailReq) (resp *types.HotelDetailResp, err error) {
	venueType := req.VenueType
	if venueType == "" {
		venueType = "all"
	}

	var rows []struct {
		VenueName    string
		Period       string
		ActivityType string
		IsBooked     bool
	}

	l.svcCtx.DB.Raw(`
		SELECT v.name AS venue_name, mr.period, mr.activity_type, mr.is_booked
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		WHERE mr.hotel_id = ?
		  AND DATE_FORMAT(mr.record_date,'%Y-%m-%d') = ?
		  AND (? = 'all' OR v.type = ?)
		ORDER BY v.name, FIELD(mr.period,'AM','PM','EV')`,
		req.HotelId, req.Date, venueType, venueType,
	).Scan(&rows)

	resp = &types.HotelDetailResp{}
	for _, r := range rows {
		resp.List = append(resp.List, types.HotelDetailItem{
			VenueName:    r.VenueName,
			Period:       r.Period,
			ActivityType: r.ActivityType,
			IsBooked:     r.IsBooked,
		})
	}
	return resp, nil
}
