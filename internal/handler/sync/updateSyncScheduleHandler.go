package sync

import (
	"net/http"

	"meeting/internal/logic/sync"
	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func UpdateSyncScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateSyncScheduleReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := sync.NewUpdateSyncScheduleLogic(r.Context(), svcCtx)
		resp, err := l.UpdateSyncSchedule(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
