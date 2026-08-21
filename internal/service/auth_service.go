package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/model"
	"notice-service/internal/repository"
	"notice-service/internal/totp"
)

type AuthClaims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	TwoFA  bool   `json:"2fa,omitempty"` // true = 2FA 待验证令牌
	jwt.RegisteredClaims
}

type AuthService struct {
	users     *repository.UserRepo
	jwtSecret []byte
	adminUser string
	adminPass string
	tokenTTL  time.Duration
	limiter   *loginLimiter
}

func NewAuthService(db *sql.DB, jwtSecret, adminUser, adminPass string) *AuthService {
	return &AuthService{
		users:     repository.NewUserRepo(db),
		jwtSecret: []byte(jwtSecret),
		adminUser: adminUser,
		adminPass: adminPass,
		tokenTTL:  24 * time.Hour,
		limiter:   newLoginLimiter(5, 15*time.Minute),
	}
}

func (s *AuthService) IssueToken(userID int64, role string) (string, error) {
	return s.IssueTokenWithTTL(userID, role, s.tokenTTL)
}

func (s *AuthService) IssueTokenWithTTL(userID int64, role string, ttl time.Duration) (string, error) {
	claims := AuthClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "notice-service",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

// IssuePending2FAToken 签发 2FA 待验证令牌（短时效，仅用于登录第二步）。
func (s *AuthService) IssuePending2FAToken(userID int64) (string, error) {
	claims := AuthClaims{
		UserID: userID,
		TwoFA:  true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(twoFATokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "notice-service",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

// VerifyPending2FAToken 校验 2FA 待验证令牌，返回用户 ID（仅接受 TwoFA 标记的令牌）。
func (s *AuthService) VerifyPending2FAToken(token string) (int64, error) {
	claims := &AuthClaims{}
	if _, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	}); err != nil {
		return 0, errors.New("验证会话已过期，请重新登录")
	}
	if !claims.TwoFA || claims.UserID <= 0 {
		return 0, errors.New("无效的验证会话")
	}
	return claims.UserID, nil
}

const twoFATokenTTL = 5 * time.Minute

func (s *AuthService) VerifyToken(token string) (*AuthClaims, error) {
	claims := &AuthClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// UserActive 返回用户是否仍有效（未删除/未禁用）。被禁用（软删除）的用户
// 其已签发 JWT 应立即失效，而不是等到令牌自然过期。
// UserActive 判断用户是否可用（未删除且未禁用）。被禁用/删除的用户其已签发
// 令牌立即失效（Auth 中间件每次请求回查）。
func (s *AuthService) UserActive(userID int64) bool {
	u, err := s.users.GetByID(userID)
	return err == nil && u.Enabled
}

// GetUsername 返回用户名（用户不存在/已删除时返回空串）。用于审计与发送日志
// 记录「谁触发」。
func (s *AuthService) GetUsername(userID int64) string {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return ""
	}
	return u.Username
}

// User 返回用户完整信息（含 2FA 启用状态，供 /auth/me 使用）。
func (s *AuthService) User(userID int64) (*model.User, error) {
	return s.users.GetByID(userID)
}

// UpdateProfile 自助更新当前用户资料（显示名/邮箱）。角色与密码不允许自助修改。
// 显示名可选（可清空），邮箱非空时校验格式；长度与表结构列宽一致。
func (s *AuthService) UpdateProfile(userID int64, displayName, email string) error {
	displayName = strings.TrimSpace(displayName)
	email = strings.TrimSpace(email)
	if utf8.RuneCountInString(displayName) > 100 {
		return errors.New("显示名不能超过 100 个字符")
	}
	if email != "" && !emailRe.MatchString(email) {
		return errors.New("邮箱格式不正确")
	}
	if utf8.RuneCountInString(email) > 190 {
		return errors.New("邮箱不能超过 190 个字符")
	}
	if _, err := s.users.GetByID(userID); err != nil {
		return errors.New("用户不存在")
	}
	return s.users.UpdateProfile(userID, displayName, email)
}

func (s *AuthService) BootstrapAdmin() error {
	if _, err := s.users.GetByUsername(s.adminUser); err == nil {
		return nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.adminPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := &model.User{Username: s.adminUser, PasswordHash: string(hash), Role: "admin"}
	if err := s.users.Create(u); err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return nil // 另一个实例已创建管理员，视为成功
		}
		return err
	}
	return nil
}

// LoginResult 登录结果：未启用 2FA 时直接返回完整 Token；已启用 2FA 时
// 返回 Requires2FA=true + 短时效 PendingToken，由前端走第二步验证。
type LoginResult struct {
	User         *model.User
	Token        string
	Requires2FA  bool
	PendingToken string
}

func (s *AuthService) Login(username, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username) // 忽略首尾空格
	password = strings.TrimSpace(password)
	if err := s.limiter.checkLocked(username); err != nil {
		return nil, err
	}
	u, err := s.users.GetByUsername(username)
	if errors.Is(err, repository.ErrNotFound) {
		s.limiter.recordFailure(username)
		return nil, errors.New("用户名或密码错误")
	}
	if err != nil {
		return nil, err
	}
	// 禁用账号：明确拒绝（不纳入登录失败限流，也无需提示具体密码）
	if !u.Enabled {
		return nil, errors.New("账号已被禁用")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		s.limiter.recordFailure(username)
		return nil, errors.New("用户名或密码错误")
	}
	s.limiter.reset(username)
	if u.TOTPEnabled {
		pending, err := s.IssuePending2FAToken(u.ID)
		if err != nil {
			return nil, err
		}
		return &LoginResult{User: u, Requires2FA: true, PendingToken: pending}, nil
	}
	tok, err := s.IssueToken(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	return &LoginResult{User: u, Token: tok}, nil
}

/* ── 双因子认证（TOTP + 备用码） ────────────────────────────────────── */

// Setup2FA 生成 TOTP 密钥与一次性备用码并落库（启用标记置 0，验证后启用）。
// 返回明文密钥、otpauth URL 与明文备用码（仅此一次展示）。
func (s *AuthService) Setup2FA(userID int64) (secret, otpauthURL string, recoveryCodes []string, err error) {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return "", "", nil, errors.New("用户不存在")
	}
	secret, err = totp.GenerateSecret()
	if err != nil {
		return "", "", nil, err
	}
	codes, err := totp.GenerateRecoveryCodes(8)
	if err != nil {
		return "", "", nil, err
	}
	hashed := totp.HashRecoveryCodes(codes)
	b, _ := json.Marshal(hashed)
	if err := s.users.SetTOTP(userID, secret, string(b)); err != nil {
		return "", "", nil, err
	}
	return secret, totp.OTPAuthURI("Notice Service", u.Username, secret), codes, nil
}

// Enable2FA 用动态码验证密钥后启用双因子认证。
func (s *AuthService) Enable2FA(userID int64, code string) error {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return errors.New("用户不存在")
	}
	if u.TOTPSecret == "" {
		return errors.New("请先完成双因子认证设置")
	}
	if !totp.Validate(code, u.TOTPSecret) {
		return errors.New("验证码不正确，请检查认证器中的 6 位动态码")
	}
	return s.users.EnableTOTP(userID)
}

// Disable2FA 校验当前动态码或备用码后关闭双因子认证（防止他人恶意关闭）。
func (s *AuthService) Disable2FA(userID int64, code string) error {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return errors.New("用户不存在")
	}
	if !u.TOTPEnabled || u.TOTPSecret == "" {
		return errors.New("当前未启用双因子认证")
	}
	if !totp.Validate(code, u.TOTPSecret) {
		if idx := s.matchRecovery(u, code); idx < 0 {
			return errors.New("验证码不正确，无法关闭双因子认证")
		}
	}
	return s.users.DisableTOTP(userID)
}

// Verify2FA 登录第二步：校验动态码或备用码，成功返回完整 JWT。
// 使用备用码登录时该备用码被消费（从列表中移除）。
func (s *AuthService) Verify2FA(pendingToken, code string) (string, *model.User, error) {
	uid, err := s.VerifyPending2FAToken(pendingToken)
	if err != nil {
		return "", nil, err
	}
	u, err := s.users.GetByID(uid)
	if err != nil || !u.TOTPEnabled {
		return "", nil, errors.New("用户不存在或未启用双因子认证")
	}
	if !totp.Validate(code, u.TOTPSecret) {
		// 备用码：命中则消费并从列表移除
		idx := s.matchRecovery(u, code)
		if idx < 0 {
			return "", nil, errors.New("验证码不正确")
		}
		if err := s.consumeRecovery(uid, idx); err != nil {
			return "", nil, errors.New("备用码校验失败，请重试")
		}
	}
	tok, err := s.IssueToken(u.ID, u.Role)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}

// matchRecovery 校验 code 是否为该用户的备用码，命中返回下标，否则 -1。
func (s *AuthService) matchRecovery(u *model.User, code string) int {
	var hashed []string
	if u.TOTPRecoveryJSON != "" {
		_ = json.Unmarshal([]byte(u.TOTPRecoveryJSON), &hashed)
	}
	return totp.MatchRecoveryCode(code, hashed)
}

// consumeRecovery 消费（删除）第 idx 个备用码。
func (s *AuthService) consumeRecovery(userID int64, idx int) error {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return err
	}
	var hashed []string
	if u.TOTPRecoveryJSON != "" {
		_ = json.Unmarshal([]byte(u.TOTPRecoveryJSON), &hashed)
	}
	if idx < 0 || idx >= len(hashed) {
		return errors.New("备用码无效")
	}
	hashed = append(hashed[:idx], hashed[idx+1:]...)
	b, _ := json.Marshal(hashed)
	if len(hashed) == 0 {
		return s.users.SetTOTPRecoveryCodes(userID, "")
	}
	return s.users.SetTOTPRecoveryCodes(userID, string(b))
}

// ChangePassword 校验旧密码并更新为新密码。
func (s *AuthService) ChangePassword(userID int64, oldPass, newPass string) error {
	if err := validatePassword(newPass); err != nil {
		return err
	}
	u, err := s.users.GetByID(userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPass)) != nil {
		return errors.New("原密码不正确")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(u.ID, string(hash))
}

// ResetPassword 忘记密码：用管理员生成的一次性令牌重置密码（公开接口）。
// 令牌一次性且带过期时间，重置成功后即失效。
func (s *AuthService) ResetPassword(username, token, newPass string) error {
	username = strings.TrimSpace(username)
	if err := validatePassword(newPass); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ok, err := s.users.ResetPasswordByToken(username, token, string(hash))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("重置令牌无效或已过期，请向管理员重新申请")
	}
	return nil
}
