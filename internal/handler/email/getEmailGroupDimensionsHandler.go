package email

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"meeting/internal/logic/email"
	"meeting/internal/svc"
)

func GetEmailGroupDimensionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := email.NewGetEmailGroupDimensionsLogic(r.Context(), svcCtx)
		resp, err := l.GetEmailGroupDimensions()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
