package dashboard

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"meeting/internal/logic/dashboard"
	"meeting/internal/svc"
	"meeting/internal/types"
)

func GetActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := dashboard.NewGetActivityLogic(r.Context(), svcCtx)
		resp, err := l.GetActivity(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
