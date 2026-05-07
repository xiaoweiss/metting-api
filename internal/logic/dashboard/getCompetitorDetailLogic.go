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

	// 数据源：meeting_records（Daily Data Input 同步来）
	// 字段较少（无 event_name / booking_status），但是日报数据完整
	var rows []struct {
		HotelName    string
		ActivityType string
		VenueName    string
		Period       string
		RecordDate   string
	}

	l.svcCtx.DB.Raw(`
		SELECT h.name AS hotel_name,
		       COALESCE(mr.activity_type, '') AS activity_type,
		       v.name AS venue_name,
		       mr.period,
		       DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date
		FROM meeting_records mr
		JOIN hotels h ON h.id = mr.hotel_id
		JOIN venues v ON v.id = mr.venue_id
		JOIN competitor_group_hotels cgh ON cgh.hotel_id = mr.hotel_id AND cgh.group_id = ?
		WHERE mr.is_booked = 1
		  AND DATE_FORMAT(mr.record_date,'%Y-%m-%d') = ?
		ORDER BY h.name, mr.activity_type, mr.period`,
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
		k := groupKey{r.HotelName, r.ActivityType}
		if _, exists := grouped[k]; !exists {
			grouped[k] = &types.CompetitorHotelDetail{
				HotelName:    r.HotelName,
				ActivityType: r.ActivityType,
			}
			order = append(order, k)
		}
		g := grouped[k]
		g.Count++
		g.Activities = append(g.Activities, types.CompetitorActivity{
			VenueName:     r.VenueName,
			Period:        r.Period,
			Date:          r.RecordDate,
			EventName:     "",
			EventType:     r.ActivityType,
			BookingStatus: "已出租",
		})
	}

	resp = &types.CompetitorDetailResp{}
	for _, k := range order {
		resp.List = append(resp.List, *grouped[k])
	}
	return resp, nil
}
