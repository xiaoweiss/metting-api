package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RetryEmailLogLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRetryEmailLogLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RetryEmailLogLogic {
	return &RetryEmailLogLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RetryEmailLogLogic) RetryEmailLog(req *types.EmailLogIdReq) (resp *types.BaseResp, err error) {
	// todo: add your logic here and delete this line

	return
}
