package dashboard

import (
	"context"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/cache"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetActivityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetActivityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetActivityLogic {
	return &GetActivityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

type activityRow struct {
	RecordDate string
	Period     string
	Count      int
}

func (l *GetActivityLogic) GetActivity(req *types.ActivityReq) (resp *types.ActivityResp, err error) {
	cacheKey := cache.ActivityKey(req.HotelId, req.Year, req.Month, req.VenueType)
	resp = &types.ActivityResp{}
	if cache.Get(l.ctx, l.svcCtx.Redis, cacheKey, resp) {
		return resp, nil
	}

	venueType := req.VenueType
	if venueType == "" {
		venueType = "all"
	}

	// 本酒店：新增活动数（is_booked=1 的数量）
	var hotelRows []activityRow
	l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date,
		       mr.period, COUNT(*) AS count
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		WHERE mr.hotel_id = ? AND mr.is_booked = 1
		  AND YEAR(mr.record_date) = ? AND MONTH(mr.record_date) = ?
		  AND (? = 'all' OR v.type = ?)
		GROUP BY mr.record_date, mr.period`,
		req.HotelId, req.Year, req.Month, venueType, venueType,
	).Scan(&hotelRows)

	// 竞对群：总值（SUM，不是平均值）
	var compRows []activityRow
	l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date,
		       mr.period, COUNT(*) AS count
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		JOIN competitor_group_hotels cgh ON cgh.hotel_id = mr.hotel_id
		JOIN competitor_groups cg ON cg.id = cgh.group_id AND cg.base_hotel_id = ?
		WHERE mr.is_booked = 1
		  AND YEAR(mr.record_date) = ? AND MONTH(mr.record_date) = ?
		  AND (? = 'all' OR v.type = ?)
		GROUP BY mr.record_date, mr.period`,
		req.HotelId, req.Year, req.Month, venueType, venueType,
	).Scan(&compRows)

	// 商圈：总值（SUM，不是平均值）
	var mktRows []activityRow
	l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date,
		       mr.period, COUNT(*) AS count
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		JOIN hotels h ON h.id = mr.hotel_id
		WHERE h.market_area_id = (SELECT market_area_id FROM hotels WHERE id = ?)
		  AND mr.is_booked = 1
		  AND YEAR(mr.record_date) = ? AND MONTH(mr.record_date) = ?
		  AND (? = 'all' OR v.type = ?)
		GROUP BY mr.record_date, mr.period`,
		req.HotelId, req.Year, req.Month, venueType, venueType,
	).Scan(&mktRows)

	// 城市活动
	var cityEventCounts []struct {
		EventDate string
		Count     int
	}
	var hotel struct{ City string }
	l.svcCtx.DB.Raw("SELECT city FROM hotels WHERE id = ?", req.HotelId).Scan(&hotel)
	l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(event_date,'%Y-%m-%d') AS event_date, COUNT(*) AS count
		FROM city_events
		WHERE city = ? AND YEAR(event_date) = ? AND MONTH(event_date) = ?
		GROUP BY event_date`,
		hotel.City, req.Year, req.Month,
	).Scan(&cityEventCounts)

	resp.List = buildActivityList(hotelRows, compRows, mktRows, cityEventCounts)
	cache.Set(l.ctx, l.svcCtx.Redis, cacheKey, resp)
	return resp, nil
}

func buildActivityList(
	hotel, comp, mkt []activityRow,
	events []struct{ EventDate string; Count int },
) []types.DailyActivity {
	toActMap := func(rows []activityRow) map[string]map[string]int {
		m := map[string]map[string]int{}
		for _, r := range rows {
			if m[r.RecordDate] == nil {
				m[r.RecordDate] = map[string]int{}
			}
			m[r.RecordDate][r.Period] = r.Count
		}
		return m
	}

	hotelMap := toActMap(hotel)
	compMap := toActMap(comp)
	mktMap := toActMap(mkt)
	eventMap := map[string]int{}
	for _, e := range events {
		eventMap[e.EventDate] = e.Count
	}

	dateSet := map[string]bool{}
	for _, r := range hotel {
		dateSet[r.RecordDate] = true
	}

	var result []types.DailyActivity
	for date := range dateSet {
		result = append(result, types.DailyActivity{
			Date: date,
			Hotel: types.PeriodData{
				M: float64(hotelMap[date]["AM"]),
				A: float64(hotelMap[date]["PM"]),
				E: float64(hotelMap[date]["EV"]),
			},
			CompetitorTotal: types.PeriodData{
				M: float64(compMap[date]["AM"]),
				A: float64(compMap[date]["PM"]),
				E: float64(compMap[date]["EV"]),
			},
			MarketTotal: types.PeriodData{
				M: float64(mktMap[date]["AM"]),
				A: float64(mktMap[date]["PM"]),
				E: float64(mktMap[date]["EV"]),
			},
			CityEventCount: eventMap[date],
		})
	}
	return result
}
