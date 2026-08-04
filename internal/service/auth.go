package service

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/config"
	"clepsydra/internal/ent"
	"clepsydra/internal/ent/user"
)

// dummyHash 哑元密码哈希，用户不存在时也执行一次 bcrypt 比较，避免计时侧信道泄露用户是否存在
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)

// Claims JWT 载荷，Name 供审计记录操作者姓名，避免每次查库
type Claims struct {
	UserID int    `json:"uid"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

// Auth 认证服务
type Auth struct {
	client *ent.Client
	cfg    config.JWT
}

// NewAuth 构建认证服务
func NewAuth(client *ent.Client, cfg config.JWT) *Auth {
	return &Auth{client: client, cfg: cfg}
}

// Login 校验用户名密码，通过后签发 JWT
func (a *Auth) Login(ctx context.Context, username, password string) (string, *ent.User, error) {
	u, err := a.client.User.Query().Where(user.Username(username), user.Enabled(true)).Only(ctx)
	if err != nil {
		// 哑元比较抹平耗时差，防止通过响应时间枚举用户名
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", nil, ErrBadRequest("用户名或密码错误")
	}

	if err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrBadRequest("用户名或密码错误")
	}

	claims := Claims{
		UserID: u.ID,
		Role:   u.Role.String(),
		Name:   u.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.cfg.Expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	var token string
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.Secret))
	if err != nil {
		return "", nil, err
	}

	return token, u, nil
}

// ParseToken 解析并校验 JWT
func (a *Auth) ParseToken(token string) (*Claims, error) {
	claims := new(Claims)

	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return []byte(a.cfg.Secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !parsed.Valid {
		return nil, ErrUnauthorized
	}

	return claims, nil
}

// Me 按 ID 返回启用中的用户，用户不存在或已停用一律视为凭证失效
func (a *Auth) Me(ctx context.Context, userID int) (*ent.User, error) {
	u, err := a.client.User.Query().Where(user.ID(userID), user.Enabled(true)).Only(ctx)
	if err != nil {
		return nil, ErrUnauthorized
	}

	return u, nil
}
