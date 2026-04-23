package sync

import (
	"context"
	"fmt"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncMeetingRecords 同步 Daily Data Input
func (e *Engine) syncMeetingRecords(ctx context.Context) error {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.DailyData
	rows, err := e.sheet.WithWorksheet(sheetId).GetAllRows()
	if err != nil {
		return fmt.Errorf("读取 Daily Data Input 失败: %w", err)
	}

	// 构建 name→id 查找表
	hotelNameToId := make(map[string]int64)
	var hotels []model.Hotel
	e.db.Find(&hotels)
	for _, h := range hotels {
		hotelNameToId[h.Name] = h.Id
	}

	// venue key = "hotelId:venueName" → venueId
	venueKeyToId := make(map[string]int64)
	var venues []model.Venue
	e.db.Find(&venues)
	for _, v := range venues {
		key := fmt.Sprintf("%d:%s", v.HotelId, v.Name)
		venueKeyToId[key] = v.Id
	}

	count := 0
	skipped := 0
	for _, row := range rows {
		hotelName := textField(row, "酒店名称 Hotel Name")
		venueName := textField(row, "会议室名称 Room Name")
		period := mapPeriod(singleSelectName(row, "场次 Session"))
		recordDate := dateField(row, "会议室出租日期")

		if hotelName == "" || venueName == "" || period == "" || recordDate == nil {
			skipped++
			continue
		}

		hotelId, ok := hotelNameToId[hotelName]
		if !ok {
			logx.Infof("[syncRecords] 酒店 '%s' 未找到，跳过", hotelName)
			skipped++
			continue
		}

		venueKey := fmt.Sprintf("%d:%s", hotelId, venueName)
		venueId, ok := venueKeyToId[venueKey]
		if !ok {
			logx.Infof("[syncRecords] 会议室 '%s@%s' 未找到，跳过", venueName, hotelName)
			skipped++
			continue
		}

		isBooked := checkboxField(row, "请在已出租的场次打√")
		activityType := singleSelectName(row, "活动类型 Event Type")
		entryDate := dateField(row, "录入日期 Date")
		// Revenue 字段当前不在 Daily Data Input 里（实际表只有 7 列）
		// 如果后续加到 Daily Data Input (Revenue) 表（2GiUIZq），需要单独 sync 函数
		foodRevenue := 0.0
		venueRevenue := 0.0

		// upsert: ON DUPLICATE KEY UPDATE (uk: hotel_id, venue_id, record_date, period)
		e.db.Exec(`
			INSERT INTO meeting_records (hotel_id, venue_id, record_date, period, is_booked, activity_type, banquet_food_revenue, banquet_venue_revenue, entry_date, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE
				is_booked = VALUES(is_booked),
				activity_type = VALUES(activity_type),
				banquet_food_revenue = VALUES(banquet_food_revenue),
				banquet_venue_revenue = VALUES(banquet_venue_revenue),
				entry_date = VALUES(entry_date)
		`, hotelId, venueId, recordDate, period, isBooked, activityType, foodRevenue, venueRevenue, entryDate)
		count++
	}

	e.logSync("meeting_records", "success", count, fmt.Sprintf("同步 %d 条记录，跳过 %d 条", count, skipped))
	logx.Infof("[syncRecords] 完成，%d 条记录，跳过 %d 条", count, skipped)
	return nil
}
