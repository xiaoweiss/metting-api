package sync

import (
	"net/http"

	"meeting/internal/logic/sync"
	"meeting/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetSyncStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := sync.NewGetSyncStatusLogic(r.Context(), svcCtx)
		resp, err := l.GetSyncStatus()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
