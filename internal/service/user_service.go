package service

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

type UserService struct {
	users *repository.UserRepo
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{users: repository.NewUserRepo(db)}
}

func (s *UserService) List() ([]*model.User, error) {
	return s.users.List()
}

// Create 创建用户：校验用户名/密码（密码至少 6 位）/角色，bcrypt 加密后入库。
func (s *UserService) Create(username, password, role string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if len(password) < 6 {
		return nil, errors.New("密码至少 6 位")
	}
	if role != "admin" && role != "user" {
		return nil, errors.New("角色必须是 admin 或 user")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{Username: username, PasswordHash: string(hash), Role: role}
	if err := s.users.Create(u); err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return nil, errors.New("用户名已存在")
		}
		return nil, err
	}
	return u, nil
}

// Delete 删除用户。规则：非 admin 无权操作；不能删除管理员；不能删除自己。
func (s *UserService) Delete(operatorRole string, operatorID, targetID int64) error {
	if operatorRole != "admin" {
		return errors.New("无权操作")
	}
	target, err := s.users.GetByID(targetID)
	if err != nil {
		return err
	}
	if target.ID == operatorID {
		return errors.New("不能删除当前登录账号")
	}
	if target.Role == "admin" {
		return errors.New("不能删除管理员账号")
	}
	return s.users.Delete(targetID)
}

// Update 修改用户角色或重置密码。operatorRole 为操作者角色；仅 admin 可操作。
// 规则：不能修改管理员角色；不能修改当前登录账号（个人密码请走个人设置）。
func (s *UserService) Update(operatorID int64, operatorRole string, targetID int64, role, newPass *string) error {
	if operatorRole != "admin" {
		return errors.New("无权操作")
	}
	if targetID == operatorID {
		return errors.New("不能修改当前登录账号的角色/密码（请用个人设置修改密码）")
	}
	target, err := s.users.GetByID(targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}
	if role != nil {
		if *role != "admin" && *role != "user" {
			return errors.New("角色必须是 admin 或 user")
		}
		if target.Role == "admin" && *role != "admin" {
			return errors.New("不能修改管理员角色")
		}
	}
	nextRole := target.Role
	if role != nil {
		nextRole = *role
	}
	nextHash := target.PasswordHash
	if newPass != nil && *newPass != "" {
		if len(*newPass) < 6 {
			return errors.New("密码至少 6 位")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*newPass), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		nextHash = string(hash)
	}
	u := &model.User{ID: targetID, Role: nextRole, PasswordHash: nextHash}
	return s.users.Update(u)
}
