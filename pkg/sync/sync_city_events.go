package sync

import (
	"context"
	"fmt"
	"time"

	"meeting/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// syncCityEvents 同步 City Event 表
// 字段名含英文括号注释，必须完全匹配（钉钉返回的 key 是带括号的全名）
//   - 城市活动名称（CityEvent_Name）
//   - 城市活动类型（CityEvent_Type）
//   - 场馆具体名称（Venue_Name）
//   - 城市活动起始时间（CityEvent_Start Date）
//   - 城市活动截止时间（CityEvent_End Date）
func (e *Engine) syncCityEvents(ctx context.Context) error {
	sheetId := e.cfg.DingTalk.Sheet.Sheets.CityEvents
	if sheetId == "" {
		logx.Info("[syncCityEvents] 未配置 CityEvents sheetId，跳过")
		return nil
	}

	rows, err := e.sheet.WithWorksheet(sheetId).GetAllRows()
	if err != nil {
		return fmt.Errorf("读取 City Event 表失败: %w", err)
	}

	defaultCity := e.cfg.DingTalk.Sheet.DefaultCity
	if defaultCity == "" {
		defaultCity = "苏州"
	}

	// 先把 AI 表里的数据拆成 (日期 × 活动) 拟落库列表，再决定是否清空
	type insertItem struct {
		VenueName string
		EventName string
		EventType string
		EventDate time.Time
	}
	var toInsert []insertItem

	count, skipped := 0, 0
	for _, row := range rows {
		eventName := textField(row, "城市活动名称（CityEvent_Name）")
		if eventName == "" {
			skipped++
			continue
		}
		eventType := singleSelectName(row, "城市活动类型（CityEvent_Type）")
		venueName := textField(row, "场馆具体名称（Venue_Name）")
		startAt := dateField(row, "城市活动起始时间（CityEvent_Start Date）")
		endAt := dateField(row, "城市活动截止时间（CityEvent_End Date）")

		if startAt == nil {
			skipped++
			continue
		}

		endDate := *startAt
		if endAt != nil {
			endDate = *endAt
		}
		// 活动跨多天 → 按每天一条（方便 dashboard 按日查询）
		for d := truncateDay(*startAt); !d.After(truncateDay(endDate)); d = d.AddDate(0, 0, 1) {
			toInsert = append(toInsert, insertItem{venueName, eventName, eventType, d})
		}
	}

	// 只有在 AI 表实际有有效数据时才覆盖 DB。
	// 这样如果 AI 表还没填数据、或出现空表，已有的手动/历史 city_events 不会被清掉。
	if len(toInsert) == 0 {
		e.logSync("city_events", "success", 0, "AI 表无有效记录，保留现有 city_events")
		logx.Infof("[syncCityEvents] AI 表无有效记录，保留现有 DB 数据（跳过 %d 行）", skipped)
		return nil
	}

	e.db.Where("city = ?", defaultCity).Delete(&model.CityEvent{})
	for _, it := range toInsert {
		e.db.Create(&model.CityEvent{
			City:      defaultCity,
			VenueName: it.VenueName,
			EventName: it.EventName,
			EventType: it.EventType,
			EventDate: it.EventDate,
		})
		count++
	}

	msg := fmt.Sprintf("同步 %d 条城市活动（跳过 %d 行）", count, skipped)
	e.logSync("city_events", "success", count, msg)
	logx.Infof("[syncCityEvents] %s", msg)
	return nil
}

func truncateDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
