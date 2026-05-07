package biz

import (
	"context"
	"errors"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"

	"go.uber.org/zap"
)

var (
	ErrUserAlreadyExists = errors.New("[user] user already exists")
	ErrUserNotFound      = errors.New("[user] user not found")
	ErrAuthFailed        = errors.New("[user] authentication failed")
)

// UserInfo 业务层用户模型
type UserInfo struct {
}

type (
	SignInRequest struct {
		Code  string
		State string
	}

	SignInResponse struct {
		State string
		Data  string
	}
)

// UserRepo 用户接口
type UserRepo interface {
	SignIn(ctx context.Context, req SignInRequest) (*SignInResponse, error)
}

type UserUseCase struct {
	repo UserRepo
	cfg  *conf.Auth
	l    *zap.Logger
}

func NewUserUseCase(repo UserRepo, cfg *conf.Bootstrap, logger *zap.Logger) *UserUseCase {
	return &UserUseCase{
		repo: repo,
		cfg:  cfg.Auth,
		l:    logger.Named("UserUseCase"),
	}
}

func (uc *UserUseCase) SignIn(ctx context.Context, req SignInRequest) (*SignInResponse, error) {
	return uc.repo.SignIn(ctx, req)
}
