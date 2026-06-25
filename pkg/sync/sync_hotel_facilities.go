package sync

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// normalizeStarRating 统一星级写法,避免同档级被算作不同组
// 钉钉里常见脏数据:"5 stars" / "5-Star" / "5星" / "5 Star" → 一律归 "X-Star"
// 不识别的值原样返回(向后兼容)
var starRatingDigitRe = regexp.MustCompile(`(\d+)`)

func normalizeStarRating(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	m := starRatingDigitRe.FindString(s)
	if m == "" {
		return s // 无数字 → 不动
	}
	return m + "-Star"
}

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
		hotelName := textField(row, "酒店中文名Hotel Name")
		if hotelName == "" {
			skipped++
			continue
		}

		hotelId, hotelExists := hotelNameToId[hotelName]

		// 酒店级元数据(2026-06 钉钉重构:字段名统一去空格 + 去全角括号;星级字段被业务方去掉)
		if hotelExists {
			hu := hotelUpdates[hotelId]
			if hu == nil {
				hu = make(map[string]interface{})
				hotelUpdates[hotelId] = hu
				matchedHotel++
			}
			if s := singleSelectName(row, "品牌中文名Brand"); s != "" {
				hu["brand"] = s
			}
			if s := singleSelectName(row, "集团名称Group"); s != "" {
				hu["hotel_group"] = s
			}
			if s := singleSelectName(row, "酒店地址-区域Hotel_Address_Region"); s != "" {
				hu["region"] = s
			}
			if s := singleSelectName(row, "酒店地址-省份Hotel_Address_Province"); s != "" {
				hu["province"] = s
			}
			if s := singleSelectName(row, "酒店地址-城市群Hotel_Address_Metropolitan_Area"); s != "" {
				hu["metropolitan_area"] = s
			}
			if s := singleSelectName(row, "城市群核心城市Metropolitan_Area_Core_City"); s != "" {
				hu["core_city"] = s
			}
			if s := singleSelectName(row, "酒店类型-经典Hotel Type - Classic"); s != "" {
				hu["hotel_type"] = s
			}
			// 业务方把「定位级别」重命名为「酒店档次Category」
			if s := singleSelectName(row, "酒店档次Category"); s != "" {
				hu["level"] = s
			}
			if s := singleSelectName(row, "所属商圈Hotel_Business_District"); s != "" {
				hu["business_district"] = s
			}
		}

		// 会议室级元数据
		venueName := textField(row, "会议室名称Meeting Room")
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
