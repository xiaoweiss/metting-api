package sync

import (
	"context"
	"fmt"

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

	// 第一遍：upsert 商圈 + 酒店
	for _, rec := range records {
		row := rec.Fields
		hotelName := textField(row, "酒店名称 Hotel Name")
		if hotelName == "" {
			logx.Infof("[syncHotels] 跳过空酒店名，recordId=%s", rec.RecordId)
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
	e.logSync("hotels", "success", count, fmt.Sprintf("同步 %d 家酒店", count))
	logx.Infof("[syncHotels] 完成，%d 家酒店", count)
	return recordIdToHotelId, nil
}
