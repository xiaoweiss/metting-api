// Package check 每日酒店数据更新检测 —— 针对每家酒店判断今天是否录入过 meeting_records，
// 若未录入则通过已启用的通知渠道发送提醒
package check

import (
	"context"
	"fmt"
	"strings"
	"time"

	"meeting/internal/config"
	"meeting/internal/model"
	"meeting/pkg/dingtalk"
	"meeting/pkg/notify"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type Engine struct {
	DB     *gorm.DB
	Cfg    config.Config
	Client *dingtalk.Client
}

func NewEngine(db *gorm.DB, cfg config.Config, client *dingtalk.Client) *Engine {
	return &Engine{DB: db, Cfg: cfg, Client: client}
}

// HotelStatus 单个酒店的检测结果
type HotelStatus struct {
	HotelId     int64
	HotelName   string
	IsUpdated   bool
	RecordCount int
}

// RunCheck 对 checkDate 做一次全量检测 + 发送通知
// checkDate 为零值时用今天
func (e *Engine) RunCheck(ctx context.Context, checkDate time.Time) ([]HotelStatus, error) {
	if checkDate.IsZero() {
		checkDate = time.Now()
	}
	dateStr := checkDate.Format("2006-01-02")

	// 查所有酒店
	var hotels []model.Hotel
	if err := e.DB.Order("id").Find(&hotels).Error; err != nil {
		return nil, err
	}

	// 统计每家酒店当天的 entry_date 录入行数（以录入日期为准）
	type countRow struct {
		HotelId int64
		Cnt     int
	}
	var counts []countRow
	e.DB.Raw(`
		SELECT hotel_id, COUNT(*) AS cnt
		FROM meeting_records
		WHERE DATE_FORMAT(entry_date, '%Y-%m-%d') = ?
		GROUP BY hotel_id
	`, dateStr).Scan(&counts)

	countMap := map[int64]int{}
	for _, c := range counts {
		countMap[c.HotelId] = c.Cnt
	}

	statuses := make([]HotelStatus, 0, len(hotels))
	var missing []HotelStatus
	for _, h := range hotels {
		cnt := countMap[h.Id]
		st := HotelStatus{HotelId: h.Id, HotelName: h.Name, RecordCount: cnt, IsUpdated: cnt > 0}
		statuses = append(statuses, st)
		if !st.IsUpdated {
			missing = append(missing, st)
		}
	}

	// 落库检测结果
	for _, s := range statuses {
		e.DB.Exec(`
			INSERT INTO hotel_update_checks (check_date, hotel_id, is_updated, record_count, created_at)
			VALUES (?, ?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE
				is_updated = VALUES(is_updated),
				record_count = VALUES(record_count)
		`, dateStr, s.HotelId, s.IsUpdated, s.RecordCount)
	}

	// 有缺失 → 通知
	if len(missing) > 0 {
		if err := e.Notify(ctx, dateStr, missing); err != nil {
			logx.Errorf("[UpdateCheck] 通知发送失败: %v", err)
		}
	}

	logx.Infof("[UpdateCheck] %s 检测完成，共 %d 家，未更新 %d 家", dateStr, len(statuses), len(missing))
	return statuses, nil
}

// Notify 遍历已启用的通知渠道发送
func (e *Engine) Notify(ctx context.Context, dateStr string, missing []HotelStatus) error {
	// 组 markdown
	var bullets []string
	for _, m := range missing {
		bullets = append(bullets, fmt.Sprintf("• %s", m.HotelName))
	}
	md := fmt.Sprintf("**⚠️ 会议室数据未录入提醒**\n\n业务日期：**%s**\n\n以下 **%d** 家酒店今日尚未录入数据：\n\n%s\n\n请相关对接人员尽快登录钉钉 AI 表完成录入。",
		dateStr, len(missing), strings.Join(bullets, "\n"))
	text := fmt.Sprintf("【会议室数据未录入提醒】%s 以下 %d 家酒店未录入：%s",
		dateStr, len(missing), strings.Join(nameList(missing), "，"))

	// 收集收件人
	phones := e.collectPhones(missing)
	unionids := e.collectUnionIds(missing)

	msg := notify.Message{
		Title:    "会议室数据未录入提醒",
		Text:     text,
		Markdown: md,
		Phones:   phones,
		Unionids: unionids,
	}

	// 找所有启用的渠道
	var settings []model.NotificationSetting
	e.DB.Where("enabled = 1").Find(&settings)

	var sentChannels []string
	for _, s := range settings {
		sender := e.buildSender(notify.Channel(s.Channel))
		if sender == nil {
			continue
		}
		if err := sender.Send(ctx, msg); err != nil {
			logx.Errorf("[UpdateCheck] 渠道 %s 发送失败: %v", s.Channel, err)
			continue
		}
		sentChannels = append(sentChannels, s.Channel)
	}

	if len(sentChannels) > 0 {
		// 记录已通知渠道
		now := time.Now()
		for _, m := range missing {
			e.DB.Model(&model.HotelUpdateCheck{}).
				Where("check_date = ? AND hotel_id = ?", dateStr, m.HotelId).
				Updates(map[string]interface{}{
					"notified_channels": strings.Join(sentChannels, ","),
					"notified_at":       &now,
				})
		}
	}
	return nil
}

func (e *Engine) buildSender(ch notify.Channel) notify.Sender {
	switch ch {
	case notify.ChannelDingTalkRobot:
		return &notify.DingTalkRobotSender{DB: e.DB}
	case notify.ChannelSMS:
		return &notify.AliyunSMSSender{DB: e.DB}
	case notify.ChannelDingTalkDing:
		return &notify.DingTalkDingSender{DB: e.DB, Cfg: e.Cfg, Client: e.Client}
	}
	return nil
}

func (e *Engine) collectPhones(missing []HotelStatus) []string {
	if len(missing) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(missing))
	for _, m := range missing {
		ids = append(ids, m.HotelId)
	}
	var phones []string
	e.DB.Raw(`
		SELECT DISTINCT u.phone
		FROM users u
		JOIN user_hotel_perms p ON p.user_id = u.id
		WHERE p.hotel_id IN ? AND u.phone <> '' AND u.status = 'active'
	`, ids).Scan(&phones)
	return phones
}

func (e *Engine) collectUnionIds(missing []HotelStatus) []string {
	if len(missing) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(missing))
	for _, m := range missing {
		ids = append(ids, m.HotelId)
	}
	var unions []string
	e.DB.Raw(`
		SELECT DISTINCT u.dingtalk_union_id
		FROM users u
		JOIN user_hotel_perms p ON p.user_id = u.id
		WHERE p.hotel_id IN ? AND u.dingtalk_union_id <> '' AND u.status = 'active'
	`, ids).Scan(&unions)
	return unions
}

func nameList(hs []HotelStatus) []string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.HotelName)
	}
	return out
}
