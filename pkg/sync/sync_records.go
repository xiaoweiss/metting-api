package sync

import (
	"context"
	"fmt"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncMeetingRecords 同步 Daily Data Input
// 全量替换模式：钉钉里删除的记录，系统也跟着删，避免历史残留导致出租率算错
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

	// 主匹配：钉钉行 record id → venueId（同名不同 type 也能精准定位）
	// 兜底：(hotelId, name) → venueId（老数据 dingtalk_record_id 还没回填时用）
	venueByRecordId := make(map[string]int64)
	venueByHotelName := make(map[string]int64)
	var venues []model.Venue
	e.db.Find(&venues)
	for _, v := range venues {
		if v.DingtalkRecordId != "" {
			venueByRecordId[v.DingtalkRecordId] = v.Id
		}
		venueByHotelName[fmt.Sprintf("%d:%s", v.HotelId, v.Name)] = v.Id
	}

	// 先解析所有 row 到内存（避免事务里跑钉钉 IO）
	type recordRow struct {
		hotelId      int64
		venueId      int64
		recordDate   interface{}
		period       string
		isBooked     bool
		activityType string
		entryDate    interface{}
	}
	var parsed []recordRow
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

		// 优先按钉钉行 record id 匹配（同名不同 type 也能区分）
		var venueId int64
		var found bool
		if rid := linkedRecordId(row, "会议室名称 Room Name"); rid != "" {
			venueId, found = venueByRecordId[rid]
		}
		if !found {
			venueId, found = venueByHotelName[fmt.Sprintf("%d:%s", hotelId, venueName)]
		}
		if !found {
			logx.Infof("[syncRecords] 会议室 '%s@%s' 未找到，跳过", venueName, hotelName)
			skipped++
			continue
		}

		parsed = append(parsed, recordRow{
			hotelId:      hotelId,
			venueId:      venueId,
			recordDate:   recordDate,
			period:       period,
			isBooked:     checkboxField(row, "请在已出租的场次打√"),
			activityType: singleSelectName(row, "活动类型 Event Type"),
			entryDate:    dateField(row, "录入日期 Date"),
		})
	}

	// 全量替换：事务内 DELETE 全表 + INSERT 钉钉所有有效记录
	// 钉钉里删除/未录的记录，系统会跟着清掉
	count := 0
	tx := e.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Exec("DELETE FROM meeting_records").Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("清空 meeting_records 失败: %w", err)
	}
	for _, r := range parsed {
		if err := tx.Exec(`
			INSERT INTO meeting_records (hotel_id, venue_id, record_date, period, is_booked, activity_type, banquet_food_revenue, banquet_venue_revenue, entry_date, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, NOW())
		`, r.hotelId, r.venueId, r.recordDate, r.period, r.isBooked, r.activityType, r.entryDate).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("插入 meeting_records 失败: %w", err)
		}
		count++
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	e.logSync("meeting_records", "success", count, fmt.Sprintf("全量同步 %d 条，跳过 %d 条", count, skipped))
	logx.Infof("[syncRecords] 完成，%d 条记录，跳过 %d 条", count, skipped)
	return nil
}
