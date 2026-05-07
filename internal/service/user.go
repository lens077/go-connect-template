package service

import (
	"context"
	"errors"

	"github.com/lens077/go-connect-template/internal/biz"

	v1 "github.com/lens077/go-connect-template/api/user/v1"
	"github.com/lens077/go-connect-template/api/user/v1/userv1connect"

	"connectrpc.com/connect"
)

// UserService 实现 Connect 服务
type UserService struct {
	uc *biz.UserUseCase
}

// 显式接口检查
var _ userv1connect.UserServiceHandler = (*UserService)(nil)

func NewUserService(uc *biz.UserUseCase) userv1connect.UserServiceHandler {
	return &UserService{
		uc: uc,
	}
}

func (s *UserService) SignIn(ctx context.Context, c *connect.Request[v1.SignInRequest]) (*connect.Response[v1.SignInResponse], error) {
	res, err := s.uc.SignIn(
		ctx,
		biz.SignInRequest{
			Code:  c.Msg.Code,
			State: c.Msg.State,
		},
	)
	if err != nil { // 根据业务错误类型映射状态码
		switch {
		case errors.Is(err, biz.ErrUserAlreadyExists):
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		case errors.Is(err, biz.ErrAuthFailed):
			return nil, connect.NewError(connect.CodeInternal, err)
		case errors.Is(err, biz.ErrUserNotFound):
			return nil, connect.NewError(connect.CodeNotFound, err)
		default:
			// 可以在这里包装一个具体的 Unknown 描述，或者直接返回
			return nil, connect.NewError(connect.CodeUnknown, err)
		}
	}

	response := &v1.SignInResponse{
		State: res.State,
		Data:  res.Data,
	}

	return connect.NewResponse(response), nil
}
