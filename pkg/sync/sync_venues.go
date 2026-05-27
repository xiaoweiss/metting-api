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
		// 钉钉源「选择酒店」已从 linkedRecord 改为多选 option,格式 [{id,name},...]
		// 用 multipleSelectNames 拿名字,按 name 匹配 hotels.name 找 hotel_id
		var hotelId int64
		var exists bool
		hotelOptionNames := multipleSelectNames(row, "选择酒店")
		if len(hotelOptionNames) > 0 {
			var h model.Hotel
			if err := e.db.Where("name = ?", hotelOptionNames[0]).First(&h).Error; err == nil && h.Id > 0 {
				hotelId = h.Id
				exists = true
			}
		}
		// 兜底:如果「选择酒店」option 没匹配上,用会议室行里的「酒店名称 Hotel Name」
		// (lookup 字段,name 部分)匹配酒店 name
		if !exists {
			hotelName := linkedRecordName(row, "酒店名称 Hotel Name")
			if hotelName != "" {
				var fallback model.Hotel
				if err := e.db.Where("name = ?", hotelName).First(&fallback).Error; err == nil && fallback.Id > 0 {
					hotelId = fallback.Id
					exists = true
					logx.Infof("[syncVenues] %s 走 lookup 字段兜底匹配酒店 '%s'", venueName, hotelName)
				}
			}
		}
		if !exists {
			logx.Infof("[syncVenues] %s 酒店「%v」未找到 + 兜底也失败，跳过", venueName, hotelOptionNames)
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
