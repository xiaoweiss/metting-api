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
		if rec.RecordId == "" {
			logx.Infof("[syncVenues] %s 没有 recordId，跳过", venueName)
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
		var hotelId int64
		var exists bool
		if len(hotelRecordIds) > 0 {
			hotelId, exists = recordIdToHotelId[hotelRecordIds[0]]
		}
		// 兜底:如果 recordId 找不到(钉钉重建过酒店行,recordId 变了但会议室还指着旧 id),
		// 用会议室行里的「酒店名称 Hotel Name」(linked field, name 部分)匹配酒店 name
		if !exists {
			hotelName := linkedRecordName(row, "酒店名称 Hotel Name")
			if hotelName != "" {
				var fallback model.Hotel
				if err := e.db.Where("name = ?", hotelName).First(&fallback).Error; err == nil && fallback.Id > 0 {
					hotelId = fallback.Id
					exists = true
					logx.Infof("[syncVenues] %s 走名字兜底匹配酒店 '%s' (recordId=%v 未在 hotels 里)", venueName, hotelName, hotelRecordIds)
				}
			}
		}
		if !exists {
			logx.Infof("[syncVenues] %s 酒店 recordId=%v 未找到 + 名字兜底也失败，跳过", venueName, hotelRecordIds)
			continue
		}

		// upsert: 用钉钉 recordId 当唯一键
		// 这样同名不同 type 会落到独立行，type 也不会被后续行覆盖。
		var venue model.Venue
		e.db.Where("dingtalk_record_id = ?", rec.RecordId).
			Attrs(model.Venue{DingtalkRecordId: rec.RecordId}).
			FirstOrCreate(&venue)

		// 全量更新元数据（hotel / name / type 都可能在钉钉里被改过）
		e.db.Model(&venue).Updates(map[string]interface{}{
			"hotel_id":          hotelId,
			"name":              venueName,
			"type":              venueType,
			"available_periods": availablePeriods,
		})
		count++
	}

	e.logSync("venues", "success", count, fmt.Sprintf("同步 %d 个会议室", count))
	logx.Infof("[syncVenues] 完成，%d 个会议室", count)
	return nil
}
