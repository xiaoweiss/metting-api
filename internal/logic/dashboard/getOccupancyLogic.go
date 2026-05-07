package dashboard

import (
	"context"
	"meeting/internal/svc"
	"meeting/internal/types"
	"meeting/pkg/cache"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOccupancyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOccupancyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOccupancyLogic {
	return &GetOccupancyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

type periodRow struct {
	RecordDate string
	Period     string
	Booked     int
	Total      int
}

func (l *GetOccupancyLogic) GetOccupancy(req *types.OccupancyReq) (resp *types.OccupancyResp, err error) {
	cacheKey := cache.OccupancyKey(req.HotelId, req.Year, req.Month, req.VenueType)
	resp = &types.OccupancyResp{}
	if cache.Get(l.ctx, l.svcCtx.Redis, cacheKey, resp) {
		return resp, nil
	}

	venueType := req.VenueType
	if venueType == "" {
		venueType = "all"
	}

	// 关键：分母应该是「该范围内符合 venue type 的 venue 总数」，
	// 而不是 meeting_records 的 COUNT(*)。
	// 业务在钉钉表里只录已预订的场次，未预订的不录，
	// 用 COUNT(*) 当分母会让出租率永远 100%。

	// 本酒店该 venue type 的 venue 数
	var hotelTotalVenues int
	l.svcCtx.DB.Raw(`
		SELECT COUNT(*) FROM venues
		WHERE hotel_id = ? AND (? = 'all' OR type = ?)`,
		req.HotelId, venueType, venueType,
	).Scan(&hotelTotalVenues)

	// 竞对群所有酒店的 venue 数
	var compTotalVenues int
	l.svcCtx.DB.Raw(`
		SELECT COUNT(*) FROM venues v
		JOIN competitor_group_hotels cgh ON cgh.hotel_id = v.hotel_id
		JOIN competitor_groups cg ON cg.id = cgh.group_id AND cg.base_hotel_id = ?
		WHERE (? = 'all' OR v.type = ?)`,
		req.HotelId, venueType, venueType,
	).Scan(&compTotalVenues)

	// 同商圈所有酒店的 venue 数
	var marketTotalVenues int
	l.svcCtx.DB.Raw(`
		SELECT COUNT(*) FROM venues v
		JOIN hotels h ON h.id = v.hotel_id
		WHERE h.market_area_id = (SELECT market_area_id FROM hotels WHERE id = ?)
		  AND (? = 'all' OR v.type = ?)`,
		req.HotelId, venueType, venueType,
	).Scan(&marketTotalVenues)

	// 查本酒店每日每时段已预订数（total 用 venue 数填充）
	var hotelRows []periodRow
	query := l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date,
		       mr.period,
		       SUM(mr.is_booked) AS booked,
		       ? AS total
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		WHERE mr.hotel_id = ?
		  AND YEAR(mr.record_date) = ?
		  AND MONTH(mr.record_date) = ?
		  AND (? = 'all' OR v.type = ?)
		GROUP BY mr.record_date, mr.period
		ORDER BY mr.record_date, FIELD(mr.period,'AM','PM','EV')`,
		hotelTotalVenues, req.HotelId, req.Year, req.Month, venueType, venueType,
	).Scan(&hotelRows)
	if query.Error != nil {
		return nil, query.Error
	}

	// 竞对群每日每时段
	var competitorRows []periodRow
	l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date,
		       mr.period,
		       SUM(mr.is_booked) AS booked,
		       ? AS total
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		JOIN competitor_group_hotels cgh ON cgh.hotel_id = mr.hotel_id
		JOIN competitor_groups cg ON cg.id = cgh.group_id AND cg.base_hotel_id = ?
		WHERE YEAR(mr.record_date) = ?
		  AND MONTH(mr.record_date) = ?
		  AND (? = 'all' OR v.type = ?)
		GROUP BY mr.record_date, mr.period`,
		compTotalVenues, req.HotelId, req.Year, req.Month, venueType, venueType,
	).Scan(&competitorRows)

	// 商圈每日每时段
	var marketRows []periodRow
	l.svcCtx.DB.Raw(`
		SELECT DATE_FORMAT(mr.record_date,'%Y-%m-%d') AS record_date,
		       mr.period,
		       SUM(mr.is_booked) AS booked,
		       ? AS total
		FROM meeting_records mr
		JOIN venues v ON v.id = mr.venue_id
		JOIN hotels h ON h.id = mr.hotel_id
		WHERE h.market_area_id = (SELECT market_area_id FROM hotels WHERE id = ?)
		  AND YEAR(mr.record_date) = ?
		  AND MONTH(mr.record_date) = ?
		  AND (? = 'all' OR v.type = ?)
		GROUP BY mr.record_date, mr.period`,
		marketTotalVenues, req.HotelId, req.Year, req.Month, venueType, venueType,
	).Scan(&marketRows)

	// 查城市活动数量（按日期）
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

	// 组装结果：按日期分组
	resp.List = buildOccupancyList(hotelRows, competitorRows, marketRows, cityEventCounts)

	// 月度汇总：按 venue 加权（∑booked / ∑total），仅统计已录入的日期/时段
	// 不让 5 月底未录入的日子拉低月均
	resp.Summary = types.OccupancySummary{
		HotelRate:      sumRate(hotelRows),
		CompetitorRate: sumRate(competitorRows),
		MarketRate:     sumRate(marketRows),
	}

	cache.Set(l.ctx, l.svcCtx.Redis, cacheKey, resp)
	return resp, nil
}

func buildOccupancyList(
	hotel, competitor, market []periodRow,
	events []struct{ EventDate string; Count int },
) []types.DailyOccupancy {
	// 建索引 map
	hotelMap := toMap(hotel)
	compMap := toMap(competitor)
	mktMap := toMap(market)
	eventMap := map[string]int{}
	for _, e := range events {
		eventMap[e.EventDate] = e.Count
	}

	// 收集所有日期
	dateSet := map[string]bool{}
	for _, r := range hotel {
		dateSet[r.RecordDate] = true
	}

	var result []types.DailyOccupancy
	for date := range dateSet {
		result = append(result, types.DailyOccupancy{
			Date: date,
			Hotel: types.PeriodData{
				M: rate(hotelMap[date]["AM"]),
				A: rate(hotelMap[date]["PM"]),
				E: rate(hotelMap[date]["EV"]),
			},
			CompetitorAvg: types.PeriodData{
				M: rate(compMap[date]["AM"]),
				A: rate(compMap[date]["PM"]),
				E: rate(compMap[date]["EV"]),
			},
			MarketAvg: types.PeriodData{
				M: rate(mktMap[date]["AM"]),
				A: rate(mktMap[date]["PM"]),
				E: rate(mktMap[date]["EV"]),
			},
			CityEventCount: eventMap[date],
		})
	}
	return result
}

func toMap(rows []periodRow) map[string]map[string]periodRow {
	m := map[string]map[string]periodRow{}
	for _, r := range rows {
		if m[r.RecordDate] == nil {
			m[r.RecordDate] = map[string]periodRow{}
		}
		m[r.RecordDate][r.Period] = r
	}
	return m
}

// sumRate 按 venue 加权计算所有行的合并出租率：∑booked / ∑total × 100
func sumRate(rows []periodRow) float64 {
	var booked, total int
	for _, r := range rows {
		booked += r.Booked
		total += r.Total
	}
	if total == 0 {
		return 0
	}
	return float64(booked) / float64(total) * 100
}

func rate(r periodRow) float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Booked) / float64(r.Total) * 100
}
