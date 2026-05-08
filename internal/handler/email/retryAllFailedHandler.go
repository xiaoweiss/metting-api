package email

import (
	"context"
	"net/http"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"

	"meeting/internal/svc"
)

// RetryAllFailedHandler POST /api/email/logs/retry-all-failed
// 异步重发所有 status in (partial, failed) 的日志。handler 立刻返回。
func RetryAllFailedHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		go func() {
			ctx := context.Background()
			if _, err := svcCtx.BlastEngine.RetryAllFailed(ctx); err != nil {
				logx.Errorf("[RetryAllFailed] %v", err)
			}
		}()
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "已开始异步重发，请稍后到「发送日志」查看结果"})
	}
}
