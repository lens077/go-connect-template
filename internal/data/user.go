package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/lens077/go-connect-template/internal/biz"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ biz.UserRepo = (*userRepo)(nil)

type userRepo struct {
	// queries *models.Queries
	rdb  *redis.Client
	auth *casdoorsdk.Client
	l    *zap.SugaredLogger
}

func NewUserRepo(data *Data, logger *zap.Logger) biz.UserRepo {
	return &userRepo{
		// queries: models.New(data.db),
		rdb:  data.rdb,
		auth: data.auth,
		l:    logger.Sugar(),
	}
}

func (u userRepo) SignIn(_ context.Context, req biz.SignInRequest) (*biz.SignInResponse, error) {
	if u.auth == nil {
		return nil, fmt.Errorf("auth client is nil:%w", errors.New("config error"))
	}
	token, err := u.auth.GetOAuthToken(req.Code, req.State)
	if err != nil {
		return nil, fmt.Errorf("%w: casdoor get oauth token err: %w", biz.ErrAuthFailed, err)
	}
	u.l.Debug(token.AccessToken)
	return &biz.SignInResponse{
		State: "ok",
		Data:  token.AccessToken,
	}, nil
}
