package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/totp"
)

// resetTokenTTL 一次性重置令牌有效期。
const resetTokenTTL = 15 * time.Minute

// emailRe 简单邮箱格式校验（非严格 RFC，够用即可）。
var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)

// isDefaultAdmin 判断是否为内置默认 admin 账号（bootstrap 创建，username='admin'）。
// 该账号的角色与密码受保护：不可由管理员在用户管理里更改/重置（恢复走离线 CLI）。
func isDefaultAdmin(u *model.User) bool {
	return u != nil && u.Username == "admin"
}

type UserService struct {
	users *repository.UserRepo
}

func NewUserService(db *sql.DB) *UserService {
	return &UserService{users: repository.NewUserRepo(db)}
}

// Username 返回用户 ID 对应的用户名（不存在/已删除返回空串）。用于审计详情可读性。
// 删除类操作需在删除之前调用，否则软删除后查询不到。
func (s *UserService) Username(id int64) string {
	u, err := s.users.GetByID(id)
	if err != nil {
		return ""
	}
	return u.Username
}

// GenerateResetToken 生成一次性重置令牌（15 分钟有效），返回给管理员线下转交用户。
// 内置 admin 账号的密码不可由管理员重置（防止把默认管理员锁死），请走离线 CLI。
func (s *UserService) GenerateResetToken(userID int64) (string, time.Time, error) {
	target, err := s.users.GetByID(userID)
	if err != nil {
		return "", time.Time{}, err
	}
	if isDefaultAdmin(target) {
		return "", time.Time{}, errors.New("不能重置内置 admin 账号的密码")
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

// Create 创建用户：校验用户名/显示名/邮箱/密码强度/角色，bcrypt 加密后入库。
func (s *UserService) Create(username, displayName, email, password, role string) (*model.User, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	email = strings.TrimSpace(email)
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	if role != "admin" && role != "user" {
		return nil, errors.New("角色必须是 admin 或 user")
	}
	if email != "" && !emailRe.MatchString(email) {
		return nil, errors.New("邮箱格式不正确")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{Username: username, DisplayName: displayName, Email: email, PasswordHash: string(hash), Role: role}
	if err := s.users.Create(u); err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return nil, errors.New("用户名已存在")
		}
		return nil, err
	}
	return u, nil
}

// Delete 删除用户。规则：仅管理员可操作；不能删除自己；内置 admin 账号
// （username=admin）不可被任何人删除；普通管理员只能删除普通用户；
// 内置 admin 可删除其它管理员账号。
func (s *UserService) Delete(operator *model.User, targetID int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	if targetID == operator.ID {
		return errors.New("不能删除当前登录账号")
	}
	target, err := s.users.GetByID(targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}
	if isDefaultAdmin(target) {
		return errors.New("不能删除内置 admin 账号")
	}
	if target.Role == "admin" && !isDefaultAdmin(operator) {
		return errors.New("普通管理员不能删除管理员账号")
	}
	return s.users.Delete(targetID)
}

// BatchDelete 批量删除用户。规则同 Delete：仅管理员；任一目标为自己、内置
// admin、或「普通管理员删除管理员」时整体拒绝。
func (s *UserService) BatchDelete(operator *model.User, ids []int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	for _, id := range ids {
		if id == operator.ID {
			return errors.New("不能删除当前登录账号")
		}
		target, err := s.users.GetByID(id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return errors.New("用户不存在")
			}
			return err
		}
		if isDefaultAdmin(target) {
			return errors.New("不能删除内置 admin 账号")
		}
		if target.Role == "admin" && !isDefaultAdmin(operator) {
			return errors.New("普通管理员不能删除管理员账号")
		}
	}
	return s.users.BatchDelete(ids) // 校验通过后单条 SQL 批量软删除
}

// DisableUser 禁用用户：登录与已签发令牌立即失效（数据保留，可重新启用）。
// 规则同删除：仅管理员；不能禁用自己；内置 admin 账号不可禁用；
// 普通管理员只能禁用普通用户；内置 admin 可禁用其它管理员。
func (s *UserService) DisableUser(operator *model.User, targetID int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	if targetID == operator.ID {
		return errors.New("不能禁用当前登录账号")
	}
	target, err := s.users.GetByID(targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}
	if isDefaultAdmin(target) {
		return errors.New("不能禁用内置 admin 账号")
	}
	if target.Role == "admin" && !isDefaultAdmin(operator) {
		return errors.New("普通管理员不能禁用管理员账号")
	}
	return s.users.SetEnabled(targetID, false)
}

// EnableUser 重新启用用户。规则：仅管理员；内置 admin 账号无需启用；
// 普通管理员不能操作管理员账号（仅内置 admin 可启用其它管理员）。
func (s *UserService) EnableUser(operator *model.User, targetID int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	target, err := s.users.GetByID(targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}
	if isDefaultAdmin(target) {
		return errors.New("内置 admin 账号无需启用")
	}
	if target.Role == "admin" && !isDefaultAdmin(operator) {
		return errors.New("普通管理员不能操作管理员账号")
	}
	return s.users.SetEnabled(targetID, true)
}

// Update 修改用户角色/密码/显示名/邮箱。operatorRole 为操作者角色；仅 admin 可操作。
// 规则：管理员角色可降级（含提升后再降级），但至少保留一个管理员；不能修改当前登录
// 账号（个人密码请走个人设置）；内置 admin 账号（username='admin'）的角色不可更改、
// 密码不可由管理员重置。
func (s *UserService) Update(operatorID int64, operatorRole string, targetID int64, role, newPass *string, displayName, email *string) error {
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
		if isDefaultAdmin(target) && *role != "admin" {
			return errors.New("不能修改内置 admin 账号的角色")
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
		if isDefaultAdmin(target) {
			return errors.New("不能重置内置 admin 账号的密码")
		}
		if err := validatePassword(*newPass); err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*newPass), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		nextHash = string(hash)
	}
	nextDisplayName := target.DisplayName
	if displayName != nil {
		nextDisplayName = strings.TrimSpace(*displayName)
	}
	nextEmail := target.Email
	if email != nil {
		nextEmail = strings.TrimSpace(*email)
	}
	if nextEmail != "" && !emailRe.MatchString(nextEmail) {
		return errors.New("邮箱格式不正确")
	}
	u := &model.User{ID: targetID, Role: nextRole, PasswordHash: nextHash,
		DisplayName: nextDisplayName, Email: nextEmail}
	return s.users.Update(u)
}

/* ── 管理员强制 2FA ────────────────────────────────────────────────── */

// ForceEnable2FA 管理员为用户强制开启双因子认证：重新生成 TOTP 密钥与
// 一次性备用码并直接启用（覆盖该用户此前的 2FA 配置）。返回明文密钥/
// otpauth URL/备用码，由管理员线下转交用户完成绑定。
func (s *UserService) ForceEnable2FA(userID int64) (secret, otpauthURL string, codes []string, err error) {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return "", "", nil, errors.New("用户不存在")
	}
	secret, err = totp.GenerateSecret()
	if err != nil {
		return "", "", nil, err
	}
	codes, err = totp.GenerateRecoveryCodes(8)
	if err != nil {
		return "", "", nil, err
	}
	hashed := totp.HashRecoveryCodes(codes)
	b, _ := json.Marshal(hashed)
	if err := s.users.SetTOTP(userID, secret, string(b)); err != nil {
		return "", "", nil, err
	}
	if err := s.users.EnableTOTP(userID); err != nil {
		return "", "", nil, err
	}
	return secret, totp.OTPAuthURI("Notice Service", u.Username, secret), codes, nil
}

// ForceDisable2FA 管理员为用户强制关闭双因子认证（用户丢失手机/备用码时的恢复路径）。
func (s *UserService) ForceDisable2FA(userID int64) error {
	if _, err := s.users.GetByID(userID); err != nil {
		return errors.New("用户不存在")
	}
	return s.users.DisableTOTP(userID)
}

/* ── 批量操作（仅管理员） ───────────────────────────────────────────── */

// BatchDisable 批量禁用用户：先逐个校验（规则同 DisableUser），校验通过后
// 单条 SQL 批量置 enabled=false。
func (s *UserService) BatchDisable(operator *model.User, ids []int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	if err := s.validateBatchTargets(operator, ids, "禁用"); err != nil {
		return err
	}
	return s.users.SetEnabledBatch(ids, false)
}

// BatchEnable 批量启用用户：先逐个校验（规则同 EnableUser），校验通过后
// 单条 SQL 批量置 enabled=true。
func (s *UserService) BatchEnable(operator *model.User, ids []int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	if err := s.validateBatchTargets(operator, ids, "启用"); err != nil {
		return err
	}
	return s.users.SetEnabledBatch(ids, true)
}

// BatchResetPassword 批量重置用户密码为统一新密码：逐个校验（不能对自己、
// 不能是内置 admin、普通管理员不能操作管理员），bcrypt 后逐个更新。
func (s *UserService) BatchResetPassword(operator *model.User, ids []int64, newPassword string) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	if err := s.validateBatchTargets(operator, ids, "重置密码"); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.users.UpdatePassword(id, string(hash)); err != nil {
			return err
		}
	}
	return nil
}

// BatchUser2FAResult 批量强制开启 2FA 时，单个用户的密钥与备用码结果。
type BatchUser2FAResult struct {
	ID            int64    `json:"id"`
	Username      string   `json:"username"`
	Secret        string   `json:"secret"`
	OtpauthURL    string   `json:"otpauth_url"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// BatchForceEnable2FA 批量强制开启双因子认证：逐个生成密钥与备用码，
// 返回每个用户的凭据供管理员线下转交。内置 admin 账号不可强制开启。
func (s *UserService) BatchForceEnable2FA(operator *model.User, ids []int64) ([]BatchUser2FAResult, error) {
	if operator.Role != "admin" {
		return nil, errors.New("无权操作")
	}
	// 先校验全部目标（含是否内置 admin），再逐个生成，避免部分生成后报错。
	targets := make([]*model.User, 0, len(ids))
	for _, id := range ids {
		t, err := s.users.GetByID(id)
		if err != nil {
			return nil, errors.New("用户不存在")
		}
		if isDefaultAdmin(t) {
			return nil, errors.New("不能对内置 admin 账号强制开启双因子认证")
		}
		targets = append(targets, t)
	}
	out := make([]BatchUser2FAResult, 0, len(targets))
	for _, t := range targets {
		secret, uri, codes, err := s.ForceEnable2FA(t.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, BatchUser2FAResult{
			ID: t.ID, Username: t.Username, Secret: secret, OtpauthURL: uri, RecoveryCodes: codes,
		})
	}
	return out, nil
}

// BatchForceDisable2FA 批量强制关闭双因子认证。
func (s *UserService) BatchForceDisable2FA(operator *model.User, ids []int64) error {
	if operator.Role != "admin" {
		return errors.New("无权操作")
	}
	for _, id := range ids {
		if err := s.ForceDisable2FA(id); err != nil {
			return err
		}
	}
	return nil
}

// validateBatchTargets 批量操作的公共校验：每个目标必须存在；不能是自己；
// 不能是内置 admin；普通管理员不能操作管理员账号。
func (s *UserService) validateBatchTargets(operator *model.User, ids []int64, action string) error {
	for _, id := range ids {
		if id == operator.ID {
			return errors.New("不能对当前登录账号执行「" + action + "」")
		}
		target, err := s.users.GetByID(id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return errors.New("用户不存在")
			}
			return err
		}
		if isDefaultAdmin(target) {
			return errors.New("不能对内置 admin 账号执行「" + action + "」")
		}
		if target.Role == "admin" && !isDefaultAdmin(operator) {
			return errors.New("普通管理员不能操作管理员账号")
		}
	}
	return nil
}
