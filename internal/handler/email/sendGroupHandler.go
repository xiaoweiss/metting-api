package email

import (
	"context"
	"net/http"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"

	"meeting/internal/svc"
)

// SendGroupHandler POST /api/email/groups/:id/send
// body: { "templateId": 1 }
// 立即异步触发：goroutine 里跑 BlastEngine.SendGroup，handler 直接返回。
// 发送结果落 email_logs，前端去日志 tab 查看。
func SendGroupHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Id         int64 `path:"id"`
			TemplateId int64 `json:"templateId"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.TemplateId == 0 {
			http.Error(w, "请选择邮件模板", http.StatusBadRequest)
			return
		}

		go func() {
			// 重新拿一个 ctx，因为 r.Context() 在 handler 返回后会被 cancel
			ctx := context.Background()
			if _, err := svcCtx.BlastEngine.SendGroup(ctx, req.Id, req.TemplateId); err != nil {
				logx.Errorf("[GroupSend] group=%d template=%d 失败: %v", req.Id, req.TemplateId, err)
			}
		}()

		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "已开始异步发送，请稍后到「发送日志」查看结果"})
	}
}
