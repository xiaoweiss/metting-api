package dashboard

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"meeting/internal/logic/dashboard"
	"meeting/internal/svc"
	"meeting/internal/types"
)

func GetCompetitorDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CompetitorDetailReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := dashboard.NewGetCompetitorDetailLogic(r.Context(), svcCtx)
		resp, err := l.GetCompetitorDetail(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
