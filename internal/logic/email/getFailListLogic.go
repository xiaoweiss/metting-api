package email

import (
	"context"

	"meeting/internal/svc"
	"meeting/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFailListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFailListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFailListLogic {
	return &GetFailListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetFailListLogic) GetFailList(req *types.EmailLogIdReq) (resp *types.FailListResp, err error) {
	// todo: add your logic here and delete this line

	return
}
