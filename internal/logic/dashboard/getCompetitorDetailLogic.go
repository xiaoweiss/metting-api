package dashboard

import (
	"context"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCompetitorDetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCompetitorDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCompetitorDetailLogic {
	return &GetCompetitorDetailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetCompetitorDetailLogic) GetCompetitorDetail(req *types.CompetitorDetailReq) (resp *types.CompetitorDetailResp, err error) {
	groupId := req.GroupId
	if groupId == 0 && req.HotelId > 0 {
		var cg struct{ Id int64 }
		l.svcCtx.DB.Raw("SELECT id FROM competitor_groups WHERE base_hotel_id = ? LIMIT 1", req.HotelId).Scan(&cg)
		groupId = cg.Id
	}

	// 数据源：hotel_events 表（从 Hotel Event AI 表同步来）
	// 比 meeting_records 富：有 event_name / event_type / booking_status
	var rows []struct {
		HotelName     string
		EventName     string
		EventType     string
		BookingStatus string
		VenueName     string
		Period        string
		RecordDate    string
	}

	l.svcCtx.DB.Raw(`
		SELECT h.name AS hotel_name,
		       COALESCE(he.event_name, '') AS event_name,
		       COALESCE(he.event_type, '') AS event_type,
		       COALESCE(he.booking_status, '') AS booking_status,
		       COALESCE(v.name, '') AS venue_name,
		       COALESCE(he.period, '') AS period,
		       DATE_FORMAT(he.event_date,'%Y-%m-%d') AS record_date
		FROM hotel_events he
		JOIN hotels h ON h.id = he.hotel_id
		LEFT JOIN venues v ON v.id = he.venue_id
		JOIN competitor_group_hotels cgh ON cgh.hotel_id = he.hotel_id AND cgh.group_id = ?
		WHERE DATE_FORMAT(he.event_date,'%Y-%m-%d') = ?
		ORDER BY h.name, he.event_type, he.event_date`,
		groupId, req.Date,
	).Scan(&rows)

	// 按酒店 + 活动类型分组
	type groupKey struct {
		HotelName    string
		ActivityType string
	}
	grouped := map[groupKey]*types.CompetitorHotelDetail{}
	var order []groupKey

	for _, r := range rows {
		k := groupKey{r.HotelName, r.EventType}
		if _, exists := grouped[k]; !exists {
			grouped[k] = &types.CompetitorHotelDetail{
				HotelName:    r.HotelName,
				ActivityType: r.EventType,
			}
			order = append(order, k)
		}
		g := grouped[k]
		g.Count++
		g.Activities = append(g.Activities, types.CompetitorActivity{
			VenueName:     r.VenueName,
			Period:        r.Period,
			Date:          r.RecordDate,
			EventName:     r.EventName,
			EventType:     r.EventType,
			BookingStatus: r.BookingStatus,
		})
	}

	resp = &types.CompetitorDetailResp{}
	for _, k := range order {
		resp.List = append(resp.List, *grouped[k])
	}
	return resp, nil
}
