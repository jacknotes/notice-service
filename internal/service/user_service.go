package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

// resetTokenTTL 一次性重置令牌有效期。
const resetTokenTTL = 15 * time.Minute

type UserService struct {
	users *repository.UserRepo
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{users: repository.NewUserRepo(db)}
}

// GenerateResetToken 生成一次性重置令牌（15 分钟有效），返回给管理员线下转交用户。
func (s *UserService) GenerateResetToken(userID int64) (string, time.Time, error) {
	if _, err := s.users.GetByID(userID); err != nil {
		return "", time.Time{}, err
	}
	token := randomToken(24)
	expires := time.Now().Add(resetTokenTTL)
	if err := s.users.SetResetToken(userID, token, expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// randomToken 生成 n 字节的十六进制随机令牌（长度 = n*2 字符）。
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *UserService) List() ([]*model.User, error) {
	return s.users.List()
}

// Create 创建用户：校验用户名/密码强度/角色，bcrypt 加密后入库。
func (s *UserService) Create(username, password, role string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if err := validatePassword(password); err != nil {
		return nil, err
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

// BatchDelete 批量删除用户。规则同 Delete：非 admin 无权操作；
// 若任一目标为管理员或当前登录账号则整体拒绝。
func (s *UserService) BatchDelete(operatorID int64, operatorRole string, ids []int64) error {
	if operatorRole != "admin" {
		return errors.New("无权操作")
	}
	for _, id := range ids {
		if id == operatorID {
			return errors.New("不能删除当前登录账号")
		}
		target, err := s.users.GetByID(id)
		if err != nil {
			return err
		}
		if target.Role == "admin" {
			return errors.New("不能删除管理员账号")
		}
	}
	return s.users.BatchDelete(ids) // 校验通过后单条 SQL 批量软删除
}

// Update 修改用户角色或重置密码。operatorRole 为操作者角色；仅 admin 可操作。
// 规则：管理员角色可降级，但至少保留一个管理员；不能修改当前登录账号（个人密码请走个人设置）。
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
			// 允许降级管理员，但至少要保留一个管理员（防止系统锁死）
			admins, err := s.users.CountAdmins()
			if err != nil {
				return err
			}
			if admins <= 1 {
				return errors.New("至少需要保留一个管理员")
			}
		}
	}
	nextRole := target.Role
	if role != nil {
		nextRole = *role
	}
	nextHash := target.PasswordHash
	if newPass != nil && *newPass != "" {
		if err := validatePassword(*newPass); err != nil {
			return err
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
