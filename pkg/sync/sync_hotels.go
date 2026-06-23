package sync

import (
	"context"
	"fmt"
	"strings"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncHotels 同步酒店基础信息表，返回 DingTalk recordId → MySQL hotel.Id 映射
func (e *Engine) syncHotels(ctx context.Context) (map[string]int64, error) {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.Hotels
	records, err := e.sheet.WithWorksheet(sheetId).GetAllRecords()
	if err != nil {
		return nil, fmt.Errorf("读取酒店表失败: %w", err)
	}

	recordIdToHotelId := make(map[string]int64)
	hotelNameToId := make(map[string]int64) // 给第二遍竞对解析用,按 name 反查
	defaultCity := e.cfg.DingTalk.Sheet.DefaultCity

	// 累积跳过样本,避免每条 record 都打 log(钉钉源 300 行全空时会刷爆 CPU/log)
	var skippedNoName int
	var skippedSamples []string

	// 第一遍：upsert 商圈 + 酒店
	for _, rec := range records {
		row := rec.Fields
		hotelName := textField(row, "酒店名称 Hotel Name")
		if hotelName == "" {
			skippedNoName++
			if len(skippedSamples) < 3 {
				skippedSamples = append(skippedSamples, rec.RecordId)
			}
			continue
		}

		// upsert 商圈
		marketAreaName := singleSelectName(row, "所在商圈")
		var marketAreaId int64
		if marketAreaName != "" {
			var ma model.MarketArea
			e.db.Where("name = ?", marketAreaName).Attrs(model.MarketArea{
				Name: marketAreaName,
				City: defaultCity,
			}).FirstOrCreate(&ma)
			marketAreaId = ma.Id
		}

		// upsert 酒店
		var hotel model.Hotel
		e.db.Where("name = ?", hotelName).Attrs(model.Hotel{
			Name: hotelName,
			City: defaultCity,
		}).FirstOrCreate(&hotel)

		// 更新商圈关联
		if marketAreaId > 0 {
			e.db.Model(&hotel).Update("market_area_id", marketAreaId)
		}

		// 同步定位级别(超高端/高端/中高端/...);钉钉表头可能是「定位级别 Level」「定位级别Level」「定位级别」等多种写法
		// 兜底:遍历 row 找含「定位级别」或精确「Level」的 key,任一拿到非空就用
		for _, candidate := range []string{"定位级别 Level", "定位级别Level", "定位级别", "Level"} {
			if level := singleSelectName(row, candidate); level != "" {
				e.db.Model(&hotel).Update("level", level)
				break
			}
		}
		// 还没拿到 → 模糊匹配 row keys,临时日志,定位完字段名后可移除
		var picked string
		var matchedKey string
		for k := range row {
			if (strings.Contains(k, "定位级别") || strings.Contains(k, "Level")) && !strings.Contains(k, "星级") {
				if v := singleSelectName(row, k); v != "" {
					picked = v
					matchedKey = k
					break
				}
			}
		}
		if picked != "" {
			e.db.Model(&hotel).Update("level", picked)
			logx.Infof("[syncHotels] hotel=%s 定位级别 via fuzzy key '%s' = %s", hotelName, matchedKey, picked)
		}

		recordIdToHotelId[rec.RecordId] = hotel.Id
		hotelNameToId[hotelName] = hotel.Id
	}

	// 第二遍：构建竞对关系（需要所有酒店都已入库）
	// 钉钉源「选择竞对酒店」已从 linkedRecord 改为多选 option,格式 [{id,name},...]
	// 用 multipleSelectNames 拿名字,再按 name 匹配 hotels.name 找 hotel_id
	for _, rec := range records {
		row := rec.Fields
		hotelId, ok := recordIdToHotelId[rec.RecordId]
		if !ok {
			continue
		}

		competitorNames := multipleSelectNames(row, "选择竞对酒店")
		if len(competitorNames) == 0 {
			continue
		}

		// upsert 竞对组
		var group model.CompetitorGroup
		e.db.Where("base_hotel_id = ?", hotelId).Attrs(model.CompetitorGroup{
			BaseHotelId: hotelId,
			Name:        textField(row, "酒店名称 Hotel Name") + " 竞对组",
		}).FirstOrCreate(&group)

		// 清空旧关系，重建
		e.db.Where("group_id = ?", group.Id).Delete(&model.CompetitorGroupHotel{})

		for _, compName := range competitorNames {
			compHotelId, exists := hotelNameToId[compName]
			if !exists {
				logx.Infof("[syncHotels] 竞对酒店「%s」在 hotels 表里找不到,跳过", compName)
				continue
			}
			e.db.Create(&model.CompetitorGroupHotel{
				GroupId: group.Id,
				HotelId: compHotelId,
			})
		}
	}

	count := len(recordIdToHotelId)
	if skippedNoName > 0 {
		logx.Infof("[syncHotels] 跳过 %d 个空酒店名(样本 recordId: %v) — 钉钉源「酒店名称 Hotel Name」字段为空", skippedNoName, skippedSamples)
	}
	e.logSync("hotels", "success", count, fmt.Sprintf("同步 %d 家酒店", count))
	logx.Infof("[syncHotels] 完成，%d 家酒店", count)
	return recordIdToHotelId, nil
}
