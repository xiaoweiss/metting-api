package dashboard

import (
	"net/http"

	"meeting/internal/logic/dashboard"
	"meeting/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetMeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIdStr := r.Header.Get("X-User-Id")
		resp, err := dashboard.GetMeByUserId(r.Context(), svcCtx, userIdStr)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
