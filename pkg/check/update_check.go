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

// userTarget 单个用户的通知目标(用于按用户分发邮件)
type userTarget struct {
	UserId int64
	Name   string
	Email  string
	Hotels []HotelStatus
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

	// 统计每家酒店"今天是否有录入动作"
	// 用 entry_date(投手在钉钉「录入日期」字段填的日期),而不是 record_date(会议室出租日期)
	// 业务语义:今天投手登录钉钉录了任何一条数据 → 这家酒店今天 OK,不预警
	//          (不管录的是哪一天的会议,只要今天有动作即可)
	type countRow struct {
		HotelId int64
		Cnt     int
	}
	var counts []countRow
	e.DB.Raw(`
		SELECT hotel_id, COUNT(*) AS cnt
		FROM meeting_records
		WHERE entry_date IS NOT NULL
		  AND DATE_FORMAT(entry_date, '%Y-%m-%d') = ?
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

// Notify 发送通知:
// - email channel: 按 user 分发(每个负责人只收到他自己负责的缺录酒店列表,To 单收件人,无邮箱互相暴露)
// - dingtalk_robot: 群里发"无负责人酒店"清单 + "已通知 N 位负责人"摘要
// - sms / dingtalk_ding: 维持旧逻辑(汇总所有 phones/unionids 一次发,SMS 是模板触达,工作通知 API 本身就是 1-on-1)
func (e *Engine) Notify(ctx context.Context, dateStr string, missing []HotelStatus) error {
	// 按 user 分组缺录酒店,并算出"无负责人"酒店
	userTargets, orphanHotels := e.groupByUser(missing)

	// 旧版聚合 msg(给 sms / dingtalk_ding / 摘要文本 用)
	var bullets []string
	for _, m := range missing {
		bullets = append(bullets, fmt.Sprintf("• %s", m.HotelName))
	}
	md := fmt.Sprintf("**⚠️ 会议室数据未录入提醒**\n\n业务日期：**%s**\n\n以下 **%d** 家酒店今日尚未录入数据：\n\n%s\n\n请相关对接人员尽快登录钉钉 AI 表完成录入。",
		dateStr, len(missing), strings.Join(bullets, "\n"))
	text := fmt.Sprintf("【会议室数据未录入提醒】%s 以下 %d 家酒店未录入：%s",
		dateStr, len(missing), strings.Join(nameList(missing), "，"))

	phones := e.collectPhones(missing)
	unionids := e.collectUnionIds(missing)

	aggregateMsg := notify.Message{
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
		switch notify.Channel(s.Channel) {
		case notify.ChannelEmail:
			if e.sendEmailPerUser(ctx, dateStr, userTargets) {
				sentChannels = append(sentChannels, s.Channel)
			}

		case notify.ChannelDingTalkRobot:
			if e.sendDingTalkRobot(ctx, dateStr, orphanHotels, userTargets) {
				sentChannels = append(sentChannels, s.Channel)
			}

		default:
			// sms / dingtalk_ding 维持原状
			sender := e.buildSender(notify.Channel(s.Channel))
			if sender == nil {
				continue
			}
			if err := sender.Send(ctx, aggregateMsg); err != nil {
				logx.Errorf("[UpdateCheck] 渠道 %s 发送失败: %v", s.Channel, err)
				continue
			}
			sentChannels = append(sentChannels, s.Channel)
		}
	}

	if len(sentChannels) > 0 {
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

// sendEmailPerUser 按用户分发邮件,每人 To 只他自己,正文只列他负责的缺录酒店
// 返回 true 表示至少发出 1 封
func (e *Engine) sendEmailPerUser(ctx context.Context, dateStr string, userTargets []userTarget) bool {
	if len(userTargets) == 0 {
		return false
	}
	sender := &notify.EmailSender{DB: e.DB, Cfg: e.Cfg}
	sent := 0
	for _, u := range userTargets {
		if u.Email == "" {
			continue
		}
		var bullets []string
		for _, h := range u.Hotels {
			bullets = append(bullets, fmt.Sprintf("• %s", h.HotelName))
		}
		userMd := fmt.Sprintf("**⚠️ 你负责的酒店今日尚未录入数据**\n\n业务日期：**%s**\n\n以下 **%d** 家(你负责)今日尚未录入：\n\n%s\n\n请尽快登录钉钉 AI 表完成录入。",
			dateStr, len(u.Hotels), strings.Join(bullets, "\n"))
		userText := fmt.Sprintf("【会议室未录提醒】%s 你负责的 %d 家未录:%s",
			dateStr, len(u.Hotels), strings.Join(nameList(u.Hotels), "、"))

		userMsg := notify.Message{
			Title:    "会议室数据未录入提醒",
			Text:     userText,
			Markdown: userMd,
			Emails:   []string{u.Email},
		}
		if err := sender.Send(ctx, userMsg); err != nil {
			logx.Errorf("[UpdateCheck] 邮件 → %s(%s)发送失败: %v", u.Name, u.Email, err)
			continue
		}
		sent++
	}
	logx.Infof("[UpdateCheck] 邮件按用户分发,成功 %d/%d", sent, len(userTargets))
	return sent > 0
}

// sendDingTalkRobot 钉钉群机器人:
// - 优先发"无负责人酒店"清单(admin 关注)
// - 如果没有孤儿,仅当有 userTargets 时发一条简要摘要(已邮件通知 N 位)
func (e *Engine) sendDingTalkRobot(ctx context.Context, dateStr string, orphan []HotelStatus, userTargets []userTarget) bool {
	sender := &notify.DingTalkRobotSender{DB: e.DB}
	var msg notify.Message

	if len(orphan) > 0 {
		var bullets []string
		for _, h := range orphan {
			bullets = append(bullets, fmt.Sprintf("• %s", h.HotelName))
		}
		text := fmt.Sprintf("【无对接人员且未录】%s 共 %d 家:%s",
			dateStr, len(orphan), strings.Join(nameList(orphan), "、"))
		md := fmt.Sprintf("**⚠️ 以下酒店今日未录入,且钉钉「酒店对接人员」字段为空**\n\n业务日期：**%s**\n\n共 **%d** 家:\n\n%s\n\n请管理员到「用户管理」分配负责人,或到钉钉酒店基础信息表补充「酒店对接人员」。",
			dateStr, len(orphan), strings.Join(bullets, "\n"))
		msg = notify.Message{
			Title:    "会议室未录(无对接人员)",
			Text:     text,
			Markdown: md,
		}
	} else if len(userTargets) > 0 {
		text := fmt.Sprintf("【会议室未录已通知】%s 已分别邮件通知 %d 位负责人,无孤儿酒店",
			dateStr, len(userTargets))
		md := fmt.Sprintf("**ℹ️ 会议室未录提醒(已通知负责人)**\n\n业务日期：**%s**\n\n已分别邮件通知 **%d** 位负责人,无孤儿酒店。",
			dateStr, len(userTargets))
		msg = notify.Message{
			Title:    "会议室未录已通知",
			Text:     text,
			Markdown: md,
		}
	} else {
		return false
	}

	if err := sender.Send(ctx, msg); err != nil {
		logx.Errorf("[UpdateCheck] 钉钉群机器人发送失败: %v", err)
		return false
	}
	return true
}

// groupByUser 按 user 分组缺录酒店,并算出"无负责人"酒店(missing 中没被任何 active user 通过 user_hotel_perms 覆盖的)
func (e *Engine) groupByUser(missing []HotelStatus) ([]userTarget, []HotelStatus) {
	if len(missing) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(missing))
	missingMap := map[int64]HotelStatus{}
	for _, m := range missing {
		ids = append(ids, m.HotelId)
		missingMap[m.HotelId] = m
	}

	type joinRow struct {
		UserId  int64
		Name    string
		Email   string
		HotelId int64
	}
	var rows []joinRow
	e.DB.Raw(`
		SELECT u.id AS user_id, u.name, u.email, p.hotel_id
		FROM user_hotel_perms p
		JOIN users u ON u.id = p.user_id
		WHERE p.hotel_id IN ? AND u.status = 'active'
		ORDER BY u.id
	`, ids).Scan(&rows)

	userMap := map[int64]*userTarget{}
	coveredHotels := map[int64]bool{}
	for _, r := range rows {
		coveredHotels[r.HotelId] = true
		u, ok := userMap[r.UserId]
		if !ok {
			u = &userTarget{UserId: r.UserId, Name: r.Name, Email: r.Email}
			userMap[r.UserId] = u
		}
		if hs, found := missingMap[r.HotelId]; found {
			u.Hotels = append(u.Hotels, hs)
		}
	}

	// 去掉没缺录酒店的 user(防御性 — SQL WHERE 已过滤,这里再保险一次)
	userTargets := make([]userTarget, 0, len(userMap))
	for _, u := range userMap {
		if len(u.Hotels) > 0 {
			userTargets = append(userTargets, *u)
		}
	}

	// 孤儿酒店
	var orphan []HotelStatus
	for _, m := range missing {
		if !coveredHotels[m.HotelId] {
			orphan = append(orphan, m)
		}
	}
	return userTargets, orphan
}

func (e *Engine) buildSender(ch notify.Channel) notify.Sender {
	switch ch {
	case notify.ChannelDingTalkRobot:
		return &notify.DingTalkRobotSender{DB: e.DB}
	case notify.ChannelSMS:
		return &notify.AliyunSMSSender{DB: e.DB}
	case notify.ChannelDingTalkDing:
		return &notify.DingTalkDingSender{DB: e.DB, Cfg: e.Cfg, Client: e.Client}
	case notify.ChannelEmail:
		return &notify.EmailSender{DB: e.DB, Cfg: e.Cfg}
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
