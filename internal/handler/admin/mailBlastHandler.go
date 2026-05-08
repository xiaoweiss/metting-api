package admin

import (
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/rest/httpx"

	"meeting/internal/model"
	"meeting/internal/svc"
)

// GetMailBlastScheduleHandler GET /api/admin/mail-blast/schedule
// 返回当前全员群发的 cron / template_id / enabled / nextRun / lastRunAt
func GetMailBlastScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s model.MailBlastSchedule
		svcCtx.DB.Where("lock_key = ?", "singleton").First(&s)
		resp := map[string]interface{}{
			"cronExpr":   s.CronExpr,
			"templateId": s.TemplateId,
			"enabled":    s.Enabled,
		}
		if s.LastRunAt != nil {
			resp["lastRunAt"] = s.LastRunAt.Format(time.RFC3339)
		} else {
			resp["lastRunAt"] = ""
		}
		if next := svcCtx.BlastScheduler.NextRun(); !next.IsZero() {
			resp["nextRun"] = next.Format(time.RFC3339)
		} else {
			resp["nextRun"] = ""
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// UpdateMailBlastScheduleHandler PUT /api/admin/mail-blast/schedule
func UpdateMailBlastScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CronExpr   string `json:"cronExpr"`
			TemplateId int64  `json:"templateId"`
			Enabled    bool   `json:"enabled"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.CronExpr == "" {
			http.Error(w, "cron 表达式不能为空", http.StatusBadRequest)
			return
		}
		if req.Enabled && req.TemplateId == 0 {
			http.Error(w, "请选择邮件模板", http.StatusBadRequest)
			return
		}
		// 校验模板存在
		if req.TemplateId > 0 {
			var tpl model.MailTemplate
			if err := svcCtx.DB.First(&tpl, req.TemplateId).Error; err != nil {
				http.Error(w, "邮件模板不存在", http.StatusBadRequest)
				return
			}
		}
		// 更新调度器
		if req.Enabled {
			if err := svcCtx.BlastScheduler.UpdateSchedule(req.CronExpr); err != nil {
				http.Error(w, "无效的 cron 表达式: "+err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			svcCtx.BlastScheduler.Stop()
		}
		// 落库
		updates := map[string]interface{}{
			"cron_expr":   req.CronExpr,
			"template_id": req.TemplateId,
			"enabled":     req.Enabled,
		}
		svcCtx.DB.Model(&model.MailBlastSchedule{}).
			Where("lock_key = ?", "singleton").
			Updates(updates)

		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"cronExpr":   req.CronExpr,
			"templateId": req.TemplateId,
			"enabled":    req.Enabled,
		})
	}
}

// TriggerMailBlastHandler POST /api/admin/mail-blast/trigger
// 立刻触发一次全员群发（拿当前 schedule 里配置的模板）
func TriggerMailBlastHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log, err := svcCtx.BlastEngine.RunBlast(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"status":    log.Status,
			"total":     log.Total,
			"failCount": log.FailCount,
			"sentAt":    log.SentAt.Format(time.RFC3339),
		})
	}
}
