package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/lens077/go-connect-template/internal/biz"

	v1 "github.com/lens077/go-connect-template/api/user/v1"
	"github.com/lens077/go-connect-template/api/user/v1/userv1connect"

	"connectrpc.com/connect"
)

type UserService struct {
	uc *biz.UserUseCase
}

var _ userv1connect.UserServiceHandler = (*UserService)(nil)
var _ Service = (*UserService)(nil)

func NewUserService(uc *biz.UserUseCase) *UserService {
	return &UserService{
		uc: uc,
	}
}

func (s *UserService) RegisterHandlers(mux *http.ServeMux, options ...connect.HandlerOption) {
	path, handler := userv1connect.NewUserServiceHandler(s, options...)
	mux.Handle(path, handler)
}

func (s *UserService) SignIn(ctx context.Context, c *connect.Request[v1.SignInRequest]) (*connect.Response[v1.SignInResponse], error) {
	res, err := s.uc.SignIn(
		ctx,
		biz.SignInRequest{
			Code:  c.Msg.Code,
			State: c.Msg.State,
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrUserAlreadyExists):
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		case errors.Is(err, biz.ErrAuthFailed):
			return nil, connect.NewError(connect.CodeInternal, err)
		case errors.Is(err, biz.ErrUserNotFound):
			return nil, connect.NewError(connect.CodeNotFound, err)
		default:
			return nil, connect.NewError(connect.CodeUnknown, err)
		}
	}

	response := &v1.SignInResponse{
		State: res.State,
		Data:  res.Data,
	}

	return connect.NewResponse(response), nil
}
