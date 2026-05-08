// recipientsHandler 邮件日志的收件人列表 + 单封重发
package email

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"

	"meeting/internal/model"
	"meeting/internal/svc"
)

type recipientItem struct {
	Id         int64  `json:"id"`
	Email      string `json:"email"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	RetryCount int    `json:"retryCount"`
	SentAt     string `json:"sentAt"`
}

// ListLogRecipientsHandler GET /api/email/logs/:id/recipients?page=&pageSize=&status=
// status 可选：sent / failed；不传则全部
func ListLogRecipientsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Id       int64  `path:"id"`
			Page     int    `form:"page,optional,default=1"`
			PageSize int    `form:"pageSize,optional,default=20"`
			Status   string `form:"status,optional"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Page <= 0 {
			req.Page = 1
		}
		if req.PageSize <= 0 || req.PageSize > 200 {
			req.PageSize = 20
		}

		q := svcCtx.DB.Model(&model.EmailLogRecipient{}).Where("log_id = ?", req.Id)
		if req.Status != "" {
			q = q.Where("status = ?", req.Status)
		}
		var total int64
		q.Count(&total)

		var rows []model.EmailLogRecipient
		q.Order("status ASC, id ASC"). // failed 在前，方便用户先看失败
			Limit(req.PageSize).Offset((req.Page - 1) * req.PageSize).
			Find(&rows)

		// 没有 recipients 行（历史日志、迁移前的数据）→ 用 fail_list 兜底，标 failed
		var fallback []recipientItem
		if total == 0 && req.Page == 1 && req.Status != "sent" {
			var src model.EmailLog
			if err := svcCtx.DB.First(&src, req.Id).Error; err == nil && src.FailList != "" {
				var arr []map[string]interface{}
				if json.Unmarshal([]byte(src.FailList), &arr) == nil {
					for _, m := range arr {
						email, _ := m["email"].(string)
						errStr, _ := m["error"].(string)
						if email == "" {
							continue
						}
						fallback = append(fallback, recipientItem{
							Email:  email,
							Status: "failed",
							Error:  errStr,
							SentAt: src.SentAt.Format(time.RFC3339),
						})
					}
				} else {
					var emails []string
					_ = json.Unmarshal([]byte(src.FailList), &emails)
					for _, e := range emails {
						fallback = append(fallback, recipientItem{
							Email:  e,
							Status: "failed",
							SentAt: src.SentAt.Format(time.RFC3339),
						})
					}
				}
			}
		}

		out := make([]recipientItem, 0, len(rows)+len(fallback))
		for _, r := range rows {
			out = append(out, recipientItem{
				Id:         r.Id,
				Email:      r.Email,
				Status:     r.Status,
				Error:      r.Error,
				RetryCount: r.RetryCount,
				SentAt:     r.SentAt.Format(time.RFC3339),
			})
		}
		if len(out) == 0 && len(fallback) > 0 {
			out = fallback
			total = int64(len(fallback))
		}

		httpx.OkJsonCtx(r.Context(), w, map[string]interface{}{
			"list":  out,
			"total": total,
		})
	}
}

// RetryLogRecipientHandler POST /api/email/logs/:logId/recipients/:rid/retry
// 重发单个收件人；handler 直接 goroutine 异步触发，避免阻塞
func RetryLogRecipientHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			LogId       int64 `path:"logId"`
			RecipientId int64 `path:"rid"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		go func() {
			ctx := context.Background()
			if _, err := svcCtx.BlastEngine.RetryRecipient(ctx, req.RecipientId); err != nil {
				logx.Errorf("[RetryRecipient] log=%d rid=%d 失败: %v", req.LogId, req.RecipientId, err)
			}
		}()
		httpx.OkJsonCtx(r.Context(), w, map[string]string{
			"message": "已开始异步重发 #" + strconv.FormatInt(req.RecipientId, 10),
		})
	}
}
