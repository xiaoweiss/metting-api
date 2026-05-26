package sync

import (
	"context"
	"fmt"
	"time"

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
		// 兼容钉钉表格列名变化:酒店名 / 会议室名 是 linked field,优先 name 部分;
		// 日期列钉钉历史上叫过「会议室出租日期」,现在叫「出租日期」,两个都尝试
		hotelName := linkedRecordName(row, "酒店名称 Hotel Name")
		if hotelName == "" {
			hotelName = textField(row, "酒店名称 Hotel Name")
		}
		venueName := linkedRecordName(row, "会议室名称 Room Name")
		if venueName == "" {
			venueName = textField(row, "会议室名称 Room Name")
		}
		period := mapPeriod(singleSelectName(row, "场次 Session"))
		recordDate := dateField(row, "出租日期")
		if recordDate == nil {
			recordDate = dateField(row, "会议室出租日期")
		}

		if hotelName == "" || venueName == "" || period == "" || recordDate == nil {
			logx.Infof("[syncRecords] 行字段不全, skip: hotel=%q venue=%q period=%q date=%v", hotelName, venueName, period, recordDate)
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

	// 检测源数据重复:同一 (hotel, venue, date, period) 出现多次。
	// 不直接静默跳过,先 log 出所有重复行的差异字段,让人工评估业务侧是不是误录(同时段同会议室双订)
	type dupKey struct {
		HotelId    int64
		VenueId    int64
		RecordDate string
		Period     string
	}
	dupMap := make(map[dupKey][]recordRow)
	for _, r := range parsed {
		ds := ""
		// dateField 返回 *time.Time, 而 parsed.recordDate 是 interface{}
		switch v := r.recordDate.(type) {
		case *time.Time:
			if v != nil {
				ds = v.Format("2006-01-02")
			}
		case time.Time:
			ds = v.Format("2006-01-02")
		case string:
			ds = v
		}
		k := dupKey{HotelId: r.hotelId, VenueId: r.venueId, RecordDate: ds, Period: r.period}
		dupMap[k] = append(dupMap[k], r)
	}
	for k, rs := range dupMap {
		if len(rs) <= 1 {
			continue
		}
		for i, r := range rs {
			logx.Infof("[syncRecords] 源数据重复 #%d hotel=%d venue=%d date=%s period=%s isBooked=%v activityType=%q entryDate=%v",
				i+1, k.HotelId, k.VenueId, k.RecordDate, k.Period, r.isBooked, r.activityType, r.entryDate)
		}
	}

	// 全量替换:事务内 DELETE 全表 + INSERT 钉钉所有有效记录
	// 钉钉里删除/未录的记录,系统会跟着清掉
	count := 0
	dupSkipped := 0
	tx := e.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Exec("DELETE FROM meeting_records").Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("清空 meeting_records 失败: %w", err)
	}
	for _, r := range parsed {
		// INSERT IGNORE:对源数据重复行,只保留首条(后续的因 uk_record 冲突被丢弃)。
		// 数据一致性:这里 is_booked 取首条;如果业务侧出现"首条 false 但第二条 true",
		// 出租率统计会偏低,但这是源数据本身的问题,需要业务在钉钉里清理重复行。
		res := tx.Exec(`
			INSERT IGNORE INTO meeting_records (hotel_id, venue_id, record_date, period, is_booked, activity_type, banquet_food_revenue, banquet_venue_revenue, entry_date, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, NOW())
		`, r.hotelId, r.venueId, r.recordDate, r.period, r.isBooked, r.activityType, r.entryDate)
		if res.Error != nil {
			tx.Rollback()
			return fmt.Errorf("插入 meeting_records 失败: %w", res.Error)
		}
		if res.RowsAffected > 0 {
			count++
		} else {
			dupSkipped++
		}
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	msg := fmt.Sprintf("全量同步 %d 条,字段不全跳过 %d 条,源数据重复跳过 %d 条", count, skipped, dupSkipped)
	e.logSync("meeting_records", "success", count, msg)
	logx.Infof("[syncRecords] 完成,%d 条记录,字段不全跳过 %d 条,源数据重复跳过 %d 条", count, skipped, dupSkipped)
	return nil
}
