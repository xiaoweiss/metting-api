package sync

import (
	"context"
	"fmt"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncHotelFacilities 同步酒店设施表（hAi1ytw）
// 用途：用"酒店名 + 会议室名"匹配，丰富 hotels 和 venues 的元数据
// 不创建新酒店/会议室（基础数据由 Hotels / Venues 同步提供）
func (e *Engine) syncHotelFacilities(ctx context.Context) error {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.HotelFacilities
	if sheetId == "" {
		logx.Info("[syncHotelFacilities] 未配置 HotelFacilities sheetId，跳过")
		return nil
	}
	rows, err := e.sheet.WithWorksheet(sheetId).GetAllRows()
	if err != nil {
		return fmt.Errorf("读取酒店设施表失败: %w", err)
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

	hotelUpdates := make(map[int64]map[string]interface{}) // hotelId → updates
	venueUpdates := make(map[int64]map[string]interface{}) // venueId → updates
	matchedHotel, matchedVenue, skipped := 0, 0, 0

	for _, row := range rows {
		hotelName := textField(row, "酒店名称 Hotel Name")
		if hotelName == "" {
			skipped++
			continue
		}

		hotelId, hotelExists := hotelNameToId[hotelName]

		// 酒店级元数据（每个 hotel 的多行会重复写，最后一行生效 —— 值都一样没问题）
		if hotelExists {
			hu := hotelUpdates[hotelId]
			if hu == nil {
				hu = make(map[string]interface{})
				hotelUpdates[hotelId] = hu
				matchedHotel++
			}
			if s := singleSelectName(row, "酒店星级等级 Hotel Star Rating"); s != "" {
				hu["star_rating"] = s
			}
			if s := singleSelectName(row, "品牌"); s != "" {
				hu["brand"] = s
			}
			if s := singleSelectName(row, "集团名称"); s != "" {
				hu["hotel_group"] = s
			}
			if s := singleSelectName(row, "酒店地址-区域（Hotel_Address_Region）"); s != "" {
				hu["region"] = s
			}
			if s := singleSelectName(row, "酒店地址-省份（Hotel_Address_Province）"); s != "" {
				hu["province"] = s
			}
			if s := singleSelectName(row, "酒店地址-城市群（Hotel_Address_Metropolitan_Area）"); s != "" {
				hu["metropolitan_area"] = s
			}
			if s := singleSelectName(row, "城市群核心城市（Metropolitan_Area_Core_City）"); s != "" {
				hu["core_city"] = s
			}
			if s := singleSelectName(row, "酒店类型-经典 Hotel Type - Classic"); s != "" {
				hu["hotel_type"] = s
			}
			if s := singleSelectName(row, "所属商圈（Hotel_Business_District）"); s != "" {
				hu["business_district"] = s
			}
		}

		// 会议室级元数据
		venueName := textField(row, "会议室名称 Meeting Room")
		if venueName != "" && hotelExists {
			venueId, ok := venueKeyToId[fmt.Sprintf("%d:%s", hotelId, venueName)]
			if ok {
				vu := venueUpdates[venueId]
				if vu == nil {
					vu = make(map[string]interface{})
					venueUpdates[venueId] = vu
					matchedVenue++
				}
				if n := numberField(row, "面积(单位:平米) Area(Sqm)"); n > 0 {
					vu["area_sqm"] = n
				}
				if n := numberField(row, "剧院式 Theater"); n > 0 {
					vu["theater_capacity"] = int(n)
				}
				if s := singleSelectName(row, "是否有柱 (If any Pillars?)"); s != "" {
					hasPillar := s != "无柱No" && s != "无柱" && s != "No"
					vu["has_pillar"] = hasPillar
				}
			} else {
				logx.Infof("[syncHotelFacilities] 未匹配会议室 %s@%s", venueName, hotelName)
			}
		}
	}

	// 落库
	for hotelId, u := range hotelUpdates {
		if len(u) > 0 {
			e.db.Model(&model.Hotel{}).Where("id = ?", hotelId).Updates(u)
		}
	}
	for venueId, u := range venueUpdates {
		if len(u) > 0 {
			e.db.Model(&model.Venue{}).Where("id = ?", venueId).Updates(u)
		}
	}

	msg := fmt.Sprintf("匹配 %d 家酒店 / %d 个会议室（跳过 %d 行）", matchedHotel, matchedVenue, skipped)
	e.logSync("hotel_facilities", "success", matchedHotel+matchedVenue, msg)
	logx.Infof("[syncHotelFacilities] %s", msg)
	return nil
}
