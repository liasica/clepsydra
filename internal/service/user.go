package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"clepsydra/internal/ent"
	"clepsydra/internal/ent/user"
)

// User 用户管理服务
type User struct {
	client *ent.Client
}

// NewUser 构建用户服务
func NewUser(client *ent.Client) *User {
	return &User{client: client}
}

// List 查询全部用户
func (s *User) List(ctx context.Context) ([]*ent.User, error) {
	return s.client.User.Query().Order(ent.Asc(user.FieldID)).All(ctx)
}

// Create 创建用户，角色仅允许 admin 或 client
func (s *User) Create(ctx context.Context, username, password, name, role string) (*ent.User, error) {
	if role != "admin" && role != "client" {
		return nil, ErrBadRequest("角色不合法")
	}
	if username == "" || len(password) < 6 {
		return nil, ErrBadRequest("用户名不能为空且密码至少 6 位")
	}

	exists, err := s.client.User.Query().Where(user.Username(username)).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrBadRequest("用户名已存在")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return s.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetName(name).
		SetRole(user.Role(role)).
		Save(ctx)
}

// Update 更新用户姓名与启用状态
func (s *User) Update(ctx context.Context, id int, name string, enabled bool) (*ent.User, error) {
	u, err := s.client.User.UpdateOneID(id).SetName(name).SetEnabled(enabled).Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}

	return u, err
}

// ResetPassword 重置用户密码
func (s *User) ResetPassword(ctx context.Context, id int, password string) error {
	if len(password) < 6 {
		return ErrBadRequest("密码至少 6 位")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	err = s.client.User.UpdateOneID(id).SetPasswordHash(string(hash)).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}

	return err
}
