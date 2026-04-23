package sync

import (
	"net/http"

	"meeting/internal/logic/sync"
	"meeting/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetSyncScheduleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := sync.NewGetSyncScheduleLogic(r.Context(), svcCtx)
		resp, err := l.GetSyncSchedule()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
