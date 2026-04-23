package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListEmailLogsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmailLogsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmailLogsLogic {
	return &ListEmailLogsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListEmailLogsLogic) ListEmailLogs(req *types.EmailLogListReq) (resp *types.EmailLogListResp, err error) {
	// todo: add your logic here and delete this line

	return
}
