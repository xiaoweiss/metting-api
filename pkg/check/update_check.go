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

// userTarget 单个用户的通知目标(用于按用户分发各类通道)
type userTarget struct {
	UserId  int64
	Name    string
	Email   string
	Phone   string
	UnionId string
	Hotels  []HotelStatus
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

// Notify 全部 1-on-1 通道(email / sms / dingtalk_ding)都按 user 分发:
//   每个负责人只收自己负责的酒店列表,内容隔离 + 信息精准
// dingtalk_robot 群发只送"无对接人员"酒店清单 + 简要"已通知 N 位"摘要
func (e *Engine) Notify(ctx context.Context, dateStr string, missing []HotelStatus) error {
	userTargets, orphanHotels := e.groupByUser(missing)

	// 加载启用渠道
	var settings []model.NotificationSetting
	e.DB.Where("enabled = 1").Find(&settings)
	enabled := map[string]bool{}
	for _, s := range settings {
		enabled[s.Channel] = true
	}

	emailSender := &notify.EmailSender{DB: e.DB, Cfg: e.Cfg}
	smsSender := &notify.AliyunSMSSender{DB: e.DB}
	dingSender := &notify.DingTalkDingSender{DB: e.DB, Cfg: e.Cfg, Client: e.Client}

	emailSent, smsSent, dingSent := 0, 0, 0
	for _, u := range userTargets {
		userMsg := buildUserMessage(dateStr, u)

		if enabled["email"] && u.Email != "" {
			m := userMsg
			m.Emails = []string{u.Email}
			if err := emailSender.Send(ctx, m); err != nil {
				logx.Errorf("[UpdateCheck] email → %s(%s)失败: %v", u.Name, u.Email, err)
			} else {
				emailSent++
			}
		}
		if enabled["sms"] && u.Phone != "" {
			m := userMsg
			m.Phones = []string{u.Phone}
			if err := smsSender.Send(ctx, m); err != nil {
				logx.Errorf("[UpdateCheck] sms → %s(%s)失败: %v", u.Name, u.Phone, err)
			} else {
				smsSent++
			}
		}
		if enabled["dingtalk_ding"] && u.UnionId != "" {
			m := userMsg
			m.Unionids = []string{u.UnionId}
			if err := dingSender.Send(ctx, m); err != nil {
				logx.Errorf("[UpdateCheck] dingtalk_ding → %s(%s)失败: %v", u.Name, u.UnionId, err)
			} else {
				dingSent++
			}
		}
	}

	var sentChannels []string
	if emailSent > 0 {
		sentChannels = append(sentChannels, "email")
	}
	if smsSent > 0 {
		sentChannels = append(sentChannels, "sms")
	}
	if dingSent > 0 {
		sentChannels = append(sentChannels, "dingtalk_ding")
	}

	if enabled["dingtalk_robot"] {
		if e.sendDingTalkRobot(ctx, dateStr, orphanHotels, userTargets) {
			sentChannels = append(sentChannels, "dingtalk_robot")
		}
	}

	logx.Infof("[UpdateCheck] %s 通知分发: email=%d sms=%d ding=%d (共 %d 位负责人, %d 家孤儿)",
		dateStr, emailSent, smsSent, dingSent, len(userTargets), len(orphanHotels))

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

// buildUserMessage 拼这位用户专属的消息体,只列他负责的酒店
func buildUserMessage(dateStr string, u userTarget) notify.Message {
	var bullets []string
	for _, h := range u.Hotels {
		bullets = append(bullets, fmt.Sprintf("• %s", h.HotelName))
	}
	md := fmt.Sprintf("**⚠️ 你负责的酒店今日尚未录入数据**\n\n业务日期：**%s**\n\n以下 **%d** 家(你负责)今日尚未录入:\n\n%s\n\n请尽快登录钉钉 AI 表完成录入。",
		dateStr, len(u.Hotels), strings.Join(bullets, "\n"))
	text := fmt.Sprintf("【会议室未录提醒】%s 你负责的 %d 家未录:%s",
		dateStr, len(u.Hotels), strings.Join(nameList(u.Hotels), "、"))
	return notify.Message{
		Title:    "会议室数据未录入提醒",
		Text:     text,
		Markdown: md,
	}
}

// sendDingTalkRobot 钉钉群机器人:
// - 优先发"无负责人酒店"清单(admin 关注)
// - 没有孤儿但有 userTargets 时发简要摘要
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
		text := fmt.Sprintf("【会议室未录已通知】%s 已分别通知 %d 位负责人,无孤儿酒店",
			dateStr, len(userTargets))
		md := fmt.Sprintf("**ℹ️ 会议室未录提醒(已通知负责人)**\n\n业务日期：**%s**\n\n已分别按 user 分发到 **%d** 位负责人(邮件/短信/钉钉工作通知各按启用情况),无孤儿酒店。",
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

// groupByUser 按 user 分组缺录酒店,并算出"无负责人"酒店
// 同时返回 user 的 phone + unionId,供 sms / dingtalk_ding 1-on-1 投递使用
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
		Phone   string
		UnionId string
		HotelId int64
	}
	var rows []joinRow
	e.DB.Raw(`
		SELECT u.id AS user_id, u.name, u.email, u.phone, u.dingtalk_union_id AS union_id, p.hotel_id
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
			u = &userTarget{
				UserId:  r.UserId,
				Name:    r.Name,
				Email:   r.Email,
				Phone:   r.Phone,
				UnionId: r.UnionId,
			}
			userMap[r.UserId] = u
		}
		if hs, found := missingMap[r.HotelId]; found {
			u.Hotels = append(u.Hotels, hs)
		}
	}

	userTargets := make([]userTarget, 0, len(userMap))
	for _, u := range userMap {
		if len(u.Hotels) > 0 {
			userTargets = append(userTargets, *u)
		}
	}

	var orphan []HotelStatus
	for _, m := range missing {
		if !coveredHotels[m.HotelId] {
			orphan = append(orphan, m)
		}
	}
	return userTargets, orphan
}

func nameList(hs []HotelStatus) []string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.HotelName)
	}
	return out
}
