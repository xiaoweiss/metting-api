package email

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"meeting/internal/logic/email"
	"meeting/internal/svc"
	"meeting/internal/types"
)

func PreviewEmailGroupMembersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PreviewEmailGroupMembersReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := email.NewPreviewEmailGroupMembersLogic(r.Context(), svcCtx)
		resp, err := l.PreviewEmailGroupMembers(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
