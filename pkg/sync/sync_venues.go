package sync

import (
	"context"
	"fmt"
	"strings"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncVenues 同步酒店会议室信息表
func (e *Engine) syncVenues(ctx context.Context, recordIdToHotelId map[string]int64) error {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.Venues
	rows, err := e.sheet.WithWorksheet(sheetId).GetAllRecords()
	if err != nil {
		return fmt.Errorf("读取会议室表失败: %w", err)
	}

	count := 0
	for _, rec := range rows {
		row := rec.Fields
		venueName := textField(row, "会议室名称 Meeting Room")
		if venueName == "" {
			continue
		}

		venueType := singleSelectName(row, "会议室类型 （Meeting Room Category)")

		// 解析可出租时段
		periodNames := multipleSelectNames(row, "可出租时间段")
		var periods []string
		for _, p := range periodNames {
			periods = append(periods, mapPeriod(p))
		}
		availablePeriods := strings.Join(periods, ",")

		// 解析酒店关联
		hotelRecordIds := linkedRecordIds(row, "选择酒店")
		if len(hotelRecordIds) == 0 {
			logx.Infof("[syncVenues] %s 无酒店关联，跳过", venueName)
			continue
		}

		hotelId, exists := recordIdToHotelId[hotelRecordIds[0]]
		if !exists {
			logx.Infof("[syncVenues] %s 酒店 recordId=%s 未找到，跳过", venueName, hotelRecordIds[0])
			continue
		}

		// upsert: 按 hotel_id + name 唯一
		var venue model.Venue
		e.db.Where("hotel_id = ? AND name = ?", hotelId, venueName).
			Attrs(model.Venue{HotelId: hotelId, Name: venueName}).
			FirstOrCreate(&venue)

		// 更新类型和时段
		e.db.Model(&venue).Updates(map[string]interface{}{
			"type":              venueType,
			"available_periods": availablePeriods,
		})
		count++
	}

	e.logSync("venues", "success", count, fmt.Sprintf("同步 %d 个会议室", count))
	logx.Infof("[syncVenues] 完成，%d 个会议室", count)
	return nil
}
