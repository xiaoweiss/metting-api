package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/pkg/notify"

	"github.com/zeromicro/go-zero/rest/httpx"
)

type NotificationSettingItem struct {
	Channel   string                 `json:"channel"`
	Enabled   bool                   `json:"enabled"`
	Config    map[string]interface{} `json:"config"`
	UpdatedAt string                 `json:"updatedAt"`
}

// ListNotificationSettingsHandler GET /api/admin/notifications
func ListNotificationSettingsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var rows []model.NotificationSetting
		svcCtx.DB.Order("channel").Find(&rows)
		list := make([]NotificationSettingItem, 0, len(rows))
		for _, n := range rows {
			cfg := map[string]interface{}{}
			if n.Config != "" {
				_ = json.Unmarshal([]byte(n.Config), &cfg)
			}
			// 敏感字段脱敏
			if v, ok := cfg["accessKeySecret"].(string); ok && v != "" {
				cfg["accessKeySecret"] = maskMiddle(v)
			}
			if v, ok := cfg["secret"].(string); ok && v != "" {
				cfg["secret"] = maskMiddle(v)
			}
			list = append(list, NotificationSettingItem{
				Channel:   n.Channel,
				Enabled:   n.Enabled,
				Config:    cfg,
				UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
			})
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"list": list})
	}
}

type updateNotificationReq struct {
	Channel string                 `path:"channel"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

// UpdateNotificationSettingHandler PUT /api/admin/notifications/:channel
func UpdateNotificationSettingHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateNotificationReq
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		channel := req.Channel
		if channel == "" {
			http.Error(w, "channel 不能为空", http.StatusBadRequest)
			return
		}

		// 合并现有 config（避免脱敏的 secret 被空值覆盖）
		var existing model.NotificationSetting
		_ = svcCtx.DB.Where("channel = ?", channel).First(&existing).Error
		merged := map[string]interface{}{}
		if existing.Config != "" {
			_ = json.Unmarshal([]byte(existing.Config), &merged)
		}
		for k, v := range req.Config {
			// 如果值是被脱敏的占位（含 *），跳过
			if s, ok := v.(string); ok && isMasked(s) {
				continue
			}
			merged[k] = v
		}
		cfgJSON, _ := json.Marshal(merged)

		if existing.Id == 0 {
			svcCtx.DB.Create(&model.NotificationSetting{
				Channel: channel,
				Config:  string(cfgJSON),
				Enabled: req.Enabled,
			})
		} else {
			svcCtx.DB.Model(&existing).Updates(map[string]interface{}{
				"config":  string(cfgJSON),
				"enabled": req.Enabled,
			})
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
	}
}

// TestNotificationHandler POST /api/admin/notifications/:channel/test
// 触发一次测试发送（用当前配置）。
// 请求体可选：
//
//	{ "phone": "13800000000",  // 指定短信测试的手机号；SMS 渠道强烈建议传
//	  "email": "x@y.com",       // 指定邮件渠道收件人；不传则取第一个有邮箱的 admin
//	  "unionId": "xxx" }        // 指定钉钉工作通知收件人的 unionId；不传则取第一个 admin
func TestNotificationHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Channel string `path:"channel"`
			Phone   string `json:"phone,optional"`
			Email   string `json:"email,optional"`
			UnionId string `json:"unionId,optional"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		channel := req.Channel
		var sender notify.Sender
		switch notify.Channel(channel) {
		case notify.ChannelDingTalkRobot:
			sender = &notify.DingTalkRobotSender{DB: svcCtx.DB}
		case notify.ChannelSMS:
			sender = &notify.AliyunSMSSender{DB: svcCtx.DB}
		case notify.ChannelDingTalkDing:
			sender = &notify.DingTalkDingSender{DB: svcCtx.DB, Cfg: svcCtx.Config, Client: svcCtx.DTClient}
		case notify.ChannelEmail:
			sender = &notify.EmailSender{DB: svcCtx.DB, Cfg: svcCtx.Config}
		default:
			http.Error(w, "未知渠道", http.StatusBadRequest)
			return
		}

		msg := notify.Message{
			Title:    "【测试】通知渠道联调",
			Text:     "这是一条来自会议室运营平台的测试消息，用于验证通知渠道配置是否正确。",
			Markdown: "**【测试】通知渠道联调**\n\n这是一条来自会议室运营平台的测试消息，用于验证通知渠道配置是否正确。",
		}

		// 收件人：请求体优先，fallback 到第一个 active admin
		switch notify.Channel(channel) {
		case notify.ChannelSMS:
			if req.Phone != "" {
				msg.Phones = []string{req.Phone}
			} else {
				var user model.User
				if err := svcCtx.DB.Where("is_admin = 1 AND status = 'active' AND phone <> ''").First(&user).Error; err == nil {
					msg.Phones = []string{user.Phone}
				}
			}
		case notify.ChannelEmail:
			if req.Email != "" {
				msg.Emails = []string{req.Email}
			} else {
				var user model.User
				if err := svcCtx.DB.Where("is_admin = 1 AND status = 'active' AND email <> ''").First(&user).Error; err == nil {
					msg.Emails = []string{user.Email}
				}
			}
		case notify.ChannelDingTalkDing:
			if req.UnionId != "" {
				msg.Unionids = []string{req.UnionId}
			} else {
				var user model.User
				if err := svcCtx.DB.Where("is_admin = 1 AND status = 'active' AND dingtalk_union_id <> ''").First(&user).Error; err == nil {
					msg.Unionids = []string{user.DingTalkUnionId}
				}
			}
		}

		if err := sender.Send(context.Background(), msg); err != nil {
			errMsg := err.Error()
			if errors.Is(err, context.Canceled) {
				errMsg = "已取消"
			}
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "测试消息已发送"})
	}
}

// ========== 更新检测 ==========

type updateCheckItem struct {
	HotelId          int64  `json:"hotelId"`
	HotelName        string `json:"hotelName"`
	CheckDate        string `json:"checkDate"`
	IsUpdated        bool   `json:"isUpdated"`
	RecordCount      int    `json:"recordCount"`
	NotifiedChannels string `json:"notifiedChannels"`
	NotifiedAt       string `json:"notifiedAt"`
}

// ListUpdateChecksHandler GET /api/admin/update-checks?date=YYYY-MM-DD
func ListUpdateChecksHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}

		var rows []struct {
			HotelId          int64
			HotelName        string
			CheckDate        string
			IsUpdated        bool
			RecordCount      int
			NotifiedChannels string
			NotifiedAt       *time.Time
		}
		svcCtx.DB.Raw(`
			SELECT h.id AS hotel_id, h.name AS hotel_name,
			       DATE_FORMAT(c.check_date, '%Y-%m-%d') AS check_date,
			       COALESCE(c.is_updated, 0) AS is_updated,
			       COALESCE(c.record_count, 0) AS record_count,
			       COALESCE(c.notified_channels, '') AS notified_channels,
			       c.notified_at
			FROM hotels h
			LEFT JOIN hotel_update_checks c
			  ON c.hotel_id = h.id AND DATE_FORMAT(c.check_date, '%Y-%m-%d') = ?
			ORDER BY h.id
		`, date).Scan(&rows)

		list := make([]updateCheckItem, 0, len(rows))
		updated, missing := 0, 0
		for _, r := range rows {
			item := updateCheckItem{
				HotelId:          r.HotelId,
				HotelName:        r.HotelName,
				CheckDate:        firstNonEmpty(r.CheckDate, date),
				IsUpdated:        r.IsUpdated,
				RecordCount:      r.RecordCount,
				NotifiedChannels: r.NotifiedChannels,
			}
			if r.NotifiedAt != nil {
				item.NotifiedAt = r.NotifiedAt.Format(time.RFC3339)
			}
			list = append(list, item)
			if r.IsUpdated {
				updated++
			} else {
				missing++
			}
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"date":    date,
			"list":    list,
			"total":   len(list),
			"updated": updated,
			"missing": missing,
		})
	}
}

// TriggerUpdateCheckHandler POST /api/admin/update-checks/trigger
func TriggerUpdateCheckHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := svcCtx.CheckEngine.RunCheck(r.Context(), time.Time{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "检测已完成"})
	}
}

// GetUpdateCheckScheduleHandler GET /api/admin/update-checks/schedule
func GetUpdateCheckScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s model.UpdateCheckSchedule
		svcCtx.DB.First(&s)
		resp := map[string]interface{}{
			"cronExpr": s.CronExpr,
			"enabled":  s.Enabled,
		}
		if next := svcCtx.CheckScheduler.NextRun(); !next.IsZero() {
			resp["nextRun"] = next.Format(time.RFC3339)
		} else {
			resp["nextRun"] = ""
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// UpdateUpdateCheckScheduleHandler PUT /api/admin/update-checks/schedule
func UpdateUpdateCheckScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CronExpr string `json:"cronExpr"`
			Enabled  bool   `json:"enabled"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.CronExpr == "" {
			http.Error(w, "cron 表达式不能为空", http.StatusBadRequest)
			return
		}
		// 更新调度器
		if req.Enabled {
			if err := svcCtx.CheckScheduler.UpdateSchedule(req.CronExpr); err != nil {
				http.Error(w, "无效的 cron 表达式: "+err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			svcCtx.CheckScheduler.Stop()
		}
		// 落库（确保只有一行）
		var s model.UpdateCheckSchedule
		if err := svcCtx.DB.First(&s).Error; err != nil {
			svcCtx.DB.Create(&model.UpdateCheckSchedule{CronExpr: req.CronExpr, Enabled: req.Enabled})
		} else {
			svcCtx.DB.Model(&s).Updates(map[string]interface{}{
				"cron_expr": req.CronExpr,
				"enabled":   req.Enabled,
			})
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"cronExpr": req.CronExpr,
			"enabled":  req.Enabled,
		})
	}
}

// ---------- 辅助 ----------

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func maskMiddle(s string) string {
	if len(s) <= 6 {
		return "****"
	}
	return s[:3] + "****" + s[len(s)-3:]
}

func isMasked(s string) bool {
	for _, c := range s {
		if c == '*' {
			return true
		}
	}
	return false
}
