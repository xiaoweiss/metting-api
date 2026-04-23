package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncHotelEvents 同步 Hotel Event 表（zJSnWZm）
// 记录具体活动（活动名称 + 活动类型 + 预订状态）
// 用 DingTalk recordId 做 upsert 键
func (e *Engine) syncHotelEvents(ctx context.Context) error {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.HotelEvents
	if sheetId == "" {
		logx.Info("[syncHotelEvents] 未配置 HotelEvents sheetId，跳过")
		return nil
	}
	records, err := e.sheet.WithWorksheet(sheetId).GetAllRecords()
	if err != nil {
		return fmt.Errorf("读取 Hotel Event 失败: %w", err)
	}

	hotelNameToId := make(map[string]int64)
	var hotels []model.Hotel
	e.db.Find(&hotels)
	for _, h := range hotels {
		hotelNameToId[h.Name] = h.Id
	}

	venueKeyToId := make(map[string]int64)
	var venues []model.Venue
	e.db.Find(&venues)
	for _, v := range venues {
		venueKeyToId[fmt.Sprintf("%d:%s", v.HotelId, v.Name)] = v.Id
	}

	count, skipped := 0, 0
	for _, rec := range records {
		row := rec.Fields

		hotelName := textField(row, "酒店名称")
		eventDate := dateField(row, "活动日期（Event_Date）")
		if hotelName == "" || eventDate == nil {
			skipped++
			continue
		}

		hotelId, ok := hotelNameToId[hotelName]
		if !ok {
			logx.Infof("[syncHotelEvents] 酒店 '%s' 未找到，跳过", hotelName)
			skipped++
			continue
		}

		venueName := textField(row, "会议室名称（Room_Name）")
		var venueId int64
		if venueName != "" {
			venueId = venueKeyToId[fmt.Sprintf("%d:%s", hotelId, venueName)]
		}

		eventName := textField(row, "活动名称（Event_Name）")
		eventType := singleSelectName(row, "活动类型（Event_Type）")
		bookingStatus := singleSelectName(row, "预订状态（Booking_Status）")
		targetDate := dateField(row, "预定日期（Target_Date）")
		endDate := dateField(row, "结束日期（End_Date）")

		periodNames := multipleSelectNames(row, "时间段")
		var periods []string
		for _, p := range periodNames {
			periods = append(periods, mapPeriod(p))
		}
		periodStr := strings.Join(periods, ",")

		ev := model.HotelEvent{
			HotelId:        hotelId,
			VenueId:        venueId,
			EventName:      eventName,
			EventType:      eventType,
			EventDate:      *eventDate,
			TargetDate:     targetDate,
			EndDate:        endDate,
			Period:         periodStr,
			BookingStatus:  bookingStatus,
			DingtalkRecord: rec.RecordId,
			CreatedAt:      time.Now(),
		}

		// upsert by dingtalk_record_id
		e.db.Exec(`
			INSERT INTO hotel_events
			(hotel_id, venue_id, event_name, event_type, event_date, target_date, end_date, period, booking_status, dingtalk_record_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE
				hotel_id = VALUES(hotel_id),
				venue_id = VALUES(venue_id),
				event_name = VALUES(event_name),
				event_type = VALUES(event_type),
				event_date = VALUES(event_date),
				target_date = VALUES(target_date),
				end_date = VALUES(end_date),
				period = VALUES(period),
				booking_status = VALUES(booking_status)
		`,
			ev.HotelId, ev.VenueId, ev.EventName, ev.EventType, ev.EventDate,
			ev.TargetDate, ev.EndDate, ev.Period, ev.BookingStatus, ev.DingtalkRecord,
		)
		count++
	}

	msg := fmt.Sprintf("同步 %d 条活动（跳过 %d 条）", count, skipped)
	e.logSync("hotel_events", "success", count, msg)
	logx.Infof("[syncHotelEvents] %s", msg)
	return nil
}
