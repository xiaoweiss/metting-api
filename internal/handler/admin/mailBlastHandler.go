package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/rest/httpx"
	"gorm.io/gorm"

	"meeting/internal/model"
	"meeting/internal/svc"
	"meeting/pkg/audit"
)

// ListMailBlastSchedulesHandler GET /api/admin/mail-blast/schedules
// 返回所有全员群发调度。
func ListMailBlastSchedulesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var rows []model.MailBlastSchedule
		svcCtx.DB.Order("id ASC").Find(&rows)
		list := make([]map[string]interface{}, 0, len(rows))
		for _, s := range rows {
			item := map[string]interface{}{
				"id":         s.Id,
				"name":       s.Name,
				"cronExpr":   s.CronExpr,
				"templateId": s.TemplateId,
				"enabled":    s.Enabled,
			}
			if s.LastRunAt != nil {
				item["lastRunAt"] = s.LastRunAt.Format(time.RFC3339)
			} else {
				item["lastRunAt"] = ""
			}
			if next := svcCtx.BlastScheduler.NextRun(s.Id); !next.IsZero() {
				item["nextRun"] = next.Format(time.RFC3339)
			} else {
				item["nextRun"] = ""
			}
			list = append(list, item)
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"list": list})
	}
}

type mailBlastScheduleReq struct {
	Name       string `json:"name"`
	CronExpr   string `json:"cronExpr"`
	TemplateId int64  `json:"templateId"`
	Enabled    bool   `json:"enabled"`
}

func validateBlastReq(svcCtx *svc.ServiceContext, req *mailBlastScheduleReq) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return errors.New("任务名不能为空")
	}
	if req.CronExpr == "" {
		return errors.New("cron 表达式不能为空")
	}
	if req.Enabled && req.TemplateId == 0 {
		return errors.New("启用前请先选择邮件模板")
	}
	if req.TemplateId > 0 {
		var tpl model.MailTemplate
		if err := svcCtx.DB.First(&tpl, req.TemplateId).Error; err != nil {
			return errors.New("邮件模板不存在")
		}
	}
	return nil
}

// CreateMailBlastScheduleHandler POST /api/admin/mail-blast/schedules
func CreateMailBlastScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mailBlastScheduleReq
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := validateBlastReq(svcCtx, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		row := model.MailBlastSchedule{
			Name:       req.Name,
			CronExpr:   req.CronExpr,
			TemplateId: req.TemplateId,
			Enabled:    req.Enabled,
		}
		if err := svcCtx.DB.Create(&row).Error; err != nil {
			http.Error(w, "新建失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if row.Enabled {
			if err := svcCtx.BlastScheduler.Add(row.Id, row.CronExpr); err != nil {
				http.Error(w, "cron 表达式无效: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		audit.Log(r.Context(), svcCtx.DB, audit.ActionCreate, audit.TargetMailBlastSchedules,
			row.Id, row.Name, nil, row)
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"id": row.Id})
	}
}

// UpdateMailBlastScheduleHandler PUT /api/admin/mail-blast/schedules/:id
func UpdateMailBlastScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 单次 Parse 把 path + json body 一起解,避免 body 被消费两次
		var req struct {
			Id         int64  `path:"id"`
			Name       string `json:"name"`
			CronExpr   string `json:"cronExpr"`
			TemplateId int64  `json:"templateId"`
			Enabled    bool   `json:"enabled"`
		}
		if err := httpx.Parse(r, &req); err != nil || req.Id == 0 {
			http.Error(w, "请求非法: "+errOrEmpty(err), http.StatusBadRequest)
			return
		}
		body := mailBlastScheduleReq{Name: req.Name, CronExpr: req.CronExpr, TemplateId: req.TemplateId, Enabled: req.Enabled}
		if err := validateBlastReq(svcCtx, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// 同步调度器:启用 → Add(覆盖旧 entry);禁用 → Remove
		if body.Enabled {
			if err := svcCtx.BlastScheduler.Add(req.Id, body.CronExpr); err != nil {
				http.Error(w, "cron 表达式无效: "+err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			svcCtx.BlastScheduler.Remove(req.Id)
		}

		// audit before
		var before model.MailBlastSchedule
		svcCtx.DB.First(&before, req.Id)

		updates := map[string]interface{}{
			"name":        body.Name,
			"cron_expr":   body.CronExpr,
			"template_id": body.TemplateId,
			"enabled":     body.Enabled,
		}
		if err := svcCtx.DB.Model(&model.MailBlastSchedule{}).Where("id = ?", req.Id).Updates(updates).Error; err != nil {
			http.Error(w, "更新失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var after model.MailBlastSchedule
		svcCtx.DB.First(&after, req.Id)
		audit.Log(r.Context(), svcCtx.DB, audit.ActionUpdate, audit.TargetMailBlastSchedules,
			after.Id, after.Name, before, after)
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"id": req.Id})
	}
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// DeleteMailBlastScheduleHandler DELETE /api/admin/mail-blast/schedules/:id
func DeleteMailBlastScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var path struct {
			Id int64 `path:"id"`
		}
		if err := httpx.Parse(r, &path); err != nil || path.Id == 0 {
			http.Error(w, "id 非法", http.StatusBadRequest)
			return
		}
		var row model.MailBlastSchedule
		if err := svcCtx.DB.First(&row, path.Id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "调度不存在", http.StatusNotFound)
				return
			}
			http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		svcCtx.BlastScheduler.Remove(path.Id)
		if err := svcCtx.DB.Delete(&row).Error; err != nil {
			http.Error(w, "删除失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		audit.Log(r.Context(), svcCtx.DB, audit.ActionDelete, audit.TargetMailBlastSchedules,
			row.Id, row.Name, row, nil)
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{"deleted": path.Id})
	}
}

// TriggerMailBlastHandler POST /api/admin/mail-blast/schedules/:id/trigger
// 手动立刻触发某条 schedule
func TriggerMailBlastHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var path struct {
			Id int64 `path:"id"`
		}
		if err := httpx.Parse(r, &path); err != nil || path.Id == 0 {
			http.Error(w, "id 非法", http.StatusBadRequest)
			return
		}
		// audit: 取触发前 schedule 快照
		var before model.MailBlastSchedule
		svcCtx.DB.First(&before, path.Id)
		log, err := svcCtx.BlastEngine.RunBlastById(r.Context(), path.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var after model.MailBlastSchedule
		svcCtx.DB.First(&after, path.Id)
		audit.Log(r.Context(), svcCtx.DB, audit.ActionTrigger, audit.TargetMailBlastSchedules,
			after.Id, after.Name, before, after)
		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"status":    log.Status,
			"total":     log.Total,
			"failCount": log.FailCount,
			"sentAt":    log.SentAt.Format(time.RFC3339),
		})
	}
}
