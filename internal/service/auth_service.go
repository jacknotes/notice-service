package service

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/crypto"
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

// dummyBcryptHash 用于登录「用户不存在」分支的等价工作量比对，
// 内容为公开已知口令的哈希即可——用途是消耗相同 CPU 时间，非机密。
var dummyBcryptHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

type AuthService struct {
	users      *repository.UserRepo
	rateLimit  *repository.RateLimitRepo
	jwtSecret  []byte
	adminUser  string
	adminPass  string
	tokenTTL   time.Duration
	maxFails   int
	lockWindow time.Duration
	// cipher 用于 TOTP 密钥加密落库（可为 nil：测试场景降级明文）。
	cipher *crypto.Cipher
}

func NewAuthService(db *sql.DB, jwtSecret, adminUser, adminPass string) *AuthService {
	return &AuthService{
		users:      repository.NewUserRepo(db),
		rateLimit:  repository.NewRateLimitRepo(db),
		jwtSecret:  []byte(jwtSecret),
		adminUser:  adminUser,
		adminPass:  adminPass,
		tokenTTL:   24 * time.Hour,
		maxFails:   5,
		lockWindow: 15 * time.Minute,
	}
}

// SetCipher 注入渠道配置同源的 AES-GCM cipher，启用 TOTP 密钥加密存储。
func (s *AuthService) SetCipher(c *crypto.Cipher) { s.cipher = c }

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
		// 固定仅接受签发端使用的 HS256：HMAC 家族其余变体（HS384/512）虽无
		// alg-confusion 实害，但与签发端不一致即应拒绝，避免实现漂移。
		if t.Method != jwt.SigningMethodHS256 {
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
		// 固定仅接受签发端使用的 HS256：HMAC 家族其余变体（HS384/512）虽无
		// alg-confusion 实害，但与签发端不一致即应拒绝，避免实现漂移。
		if t.Method != jwt.SigningMethodHS256 {
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

// loginBucket 登录限流复合桶：username + client IP 双维度。
// 仅按 username 计数时，任何知道用户名的人都能把该账号锁到无法登录
// （拒绝服务：5 次错误即锁 15 分钟且可无限续锁）；叠加 IP 后攻击者只能
// 锁住「自己 IP → 该账号」这一条路径，受害者本人不受影响。单一 IP 的
// 爆破面仍由 maxFails/lockWindow 完整约束。
func (s *AuthService) loginBucket(username, ip string) string {
	return "login:" + username + "|" + ip
}

func (s *AuthService) Login(username, password, ip string) (*LoginResult, error) {
	username = strings.TrimSpace(username) // 忽略首尾空格
	password = strings.TrimSpace(password)
	bucket := s.loginBucket(username, ip)
	// 锁定判定：DB 集中式（多实例共享）。DB 故障时 fail-open（登录本身依赖 DB）。
	locked, err := s.rateLimit.LoginLocked(bucket)
	if err != nil {
		log.Printf("auth: rate limit check failed: %v", err)
	} else if locked {
		return nil, errors.New("登录失败次数过多，请稍后再试")
	}
	u, err := s.users.GetByUsername(username)
	if errors.Is(err, repository.ErrNotFound) {
		// 账号不存在也跑一次等价 bcrypt 比对：抹平与「存在但密码错」路径的
		// 响应时序差，阻断按耗时区分用户名是否存在的枚举探测。
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(password))
		_ = s.rateLimit.RecordLoginFailure(bucket, s.maxFails, s.lockWindow)
		return nil, errors.New("用户名或密码错误")
	}
	if err != nil {
		return nil, err
	}
	// 禁用账号：消耗等价 bcrypt 工作量后返回与「密码错误」一致的提示，
	// 抹平时序差并避免确认用户名存在（枚举防护）。
	if !u.Enabled {
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(password))
		return nil, errors.New("用户名或密码错误")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		_ = s.rateLimit.RecordLoginFailure(bucket, s.maxFails, s.lockWindow)
		return nil, errors.New("用户名或密码错误")
	}
	_ = s.rateLimit.Reset(bucket)
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
// 返回明文密钥、otpauth URL 与明文备用码（仅此一次展示）。密钥加密落库
// （cipher 未注入时降级明文），DB 泄漏不再直接等于 2FA 失守。
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
	if err := s.storeTOTPSecret(userID, secret, string(b)); err != nil {
		return "", "", nil, err
	}
	return secret, totp.OTPAuthURI("Notice Service", u.Username, secret), codes, nil
}

// storeTOTPSecret 加密 TOTP 密钥后落库；加密失败时保留明文回退（可用性优先，
// 且比明文落库不会更差），历史明文列仅在降级场景写入。
func (s *AuthService) storeTOTPSecret(userID int64, secret, recoveryJSON string) error {
	if s.cipher != nil {
		if enc, encErr := s.cipher.EncryptString(secret); encErr == nil {
			return s.users.SetTOTP(userID, enc, "", recoveryJSON)
		} else {
			log.Printf("auth: encrypt totp secret failed, fallback to plaintext column: %v", encErr)
		}
	}
	return s.users.SetTOTP(userID, "", secret, recoveryJSON)
}

// totpSecret 取用户的 TOTP 密钥明文：优先解密密文列；密文列空（历史数据）
// 回退明文列。同时返回密钥来源，供验证成功后升级加密存储。
func (s *AuthService) totpSecret(u *model.User) (secret string, encrypted bool, err error) {
	if u.TOTPSecretEnc != "" && s.cipher != nil {
		plain, decErr := s.cipher.DecryptString(u.TOTPSecretEnc)
		if decErr != nil {
			return "", true, errors.New("TOTP 密钥解密失败，请重新设置双因子认证")
		}
		return plain, true, nil
	}
	return u.TOTPSecret, false, nil
}

// Enable2FA 用动态码验证密钥后启用双因子认证。
func (s *AuthService) Enable2FA(userID int64, code string) error {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return errors.New("用户不存在")
	}
	secret, encrypted, err := s.totpSecret(u)
	if err != nil {
		return err
	}
	if secret == "" {
		return errors.New("请先完成双因子认证设置")
	}
	// 启用是会话内的确认操作，不消耗防重放计数器：否则用户启用后 30s 内
	// 立即登录会用同一时间步验证码被防重放误拒。
	if !totp.Validate(code, secret) {
		return errors.New("验证码不正确，请检查认证器中的 6 位动态码")
	}
	// 启用时升级为加密存储（历史明文数据首次启用即升级）
	if !encrypted && s.cipher != nil {
		if enc, encErr := s.cipher.EncryptString(secret); encErr == nil {
			if err := s.users.SetTOTPSecretEnc(userID, enc); err != nil {
				return err
			}
		}
	}
	return s.users.EnableTOTP(userID)
}

// Disable2FA 校验当前动态码或备用码后关闭双因子认证（防止他人恶意关闭）。
func (s *AuthService) Disable2FA(userID int64, code string) error {
	u, err := s.users.GetByID(userID)
	if err != nil {
		return errors.New("用户不存在")
	}
	if !u.TOTPEnabled {
		return errors.New("当前未启用双因子认证")
	}
	secret, _, err := s.totpSecret(u)
	if err != nil {
		return err
	}
	// 同 Enable2FA：确认性操作不消耗防重放计数器
	if !totp.Validate(code, secret) {
		// 动态码不匹配时尝试备用码（备用码一次性，天然防重放）
		if idx := s.matchRecovery(u, code); idx < 0 {
			return errors.New("验证码不正确，无法关闭双因子认证")
		}
	}
	return s.users.DisableTOTP(userID)
}

// Verify2FA 登录第二步：校验动态码或备用码，成功返回完整 JWT。
// 使用备用码登录时该备用码被消费（从列表中移除）。
// 失败计入与密码登录相同的复合限流桶（username+IP）：公开接口 + 无 bcrypt
// 类慢函数兜底，不限流则持有 pending token（5 分钟 TTL）者可高速爆破 TOTP。
func (s *AuthService) Verify2FA(pendingToken, code, ip string) (string, *model.User, error) {
	uid, err := s.VerifyPending2FAToken(pendingToken)
	if err != nil {
		return "", nil, err
	}
	u, err := s.users.GetByID(uid)
	if err != nil || !u.TOTPEnabled || !u.Enabled {
		// Enabled 一并校验：被禁用用户若在禁用前已拿到 pending token，
		// 5 分钟窗口内不得再完成登录。
		return "", nil, errors.New("用户不存在或未启用双因子认证")
	}
	bucket := s.loginBucket(u.Username, ip)
	if locked, err := s.rateLimit.LoginLocked(bucket); err != nil {
		log.Printf("auth: rate limit check failed: %v", err)
	} else if locked {
		return "", nil, errors.New("登录失败次数过多，请稍后再试")
	}
	secret, encrypted, err := s.totpSecret(u)
	if err != nil {
		return "", nil, err
	}
	used, verr := s.validateTOTPWithReplay(u.ID, code, secret, u.TOTPLastCounter)
	if verr != nil {
		// 备用码：命中则消费并从列表移除（备用码本身一次性，天然防重放）
		idx := s.matchRecovery(u, code)
		if idx < 0 {
			_ = s.rateLimit.RecordLoginFailure(bucket, s.maxFails, s.lockWindow)
			return "", nil, errors.New("验证码不正确")
		}
		if err := s.consumeRecovery(uid, idx); err != nil {
			return "", nil, errors.New("备用码校验失败，请重试")
		}
	} else {
		_ = used
		// 首次用明文历史密钥验证成功：升级为加密存储
		if !encrypted && s.cipher != nil {
			if enc, encErr := s.cipher.EncryptString(secret); encErr == nil {
				if err := s.users.SetTOTPSecretEnc(uid, enc); err != nil {
					log.Printf("auth: upgrade totp secret enc for user %d: %v", uid, err)
				}
			}
		}
	}
	_ = s.rateLimit.Reset(bucket)
	tok, err := s.IssueToken(u.ID, u.Role)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}

// validateTOTPWithReplay 防重放的 TOTP 校验：同一时间步的验证码只能消费一次。
// 返回是否校验通过；通过时已把该用户 last_counter 推进到本次 counter。
// counter 为 0（首次）视为无历史，仅要求大于 0 的 counter 未被用过。
func (s *AuthService) validateTOTPWithReplay(userID int64, code, secret string, lastCounter uint64) (bool, error) {
	if secret == "" {
		return false, errors.New("验证码不正确")
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false, errors.New("验证码不正确，请检查认证器中的 6 位动态码")
	}
	key, err := totp.DecodeSecret(secret)
	if err != nil {
		return false, errors.New("验证码不正确")
	}
	counter, ok := totp.MatchingCounter(key, code)
	if !ok {
		return false, errors.New("验证码不正确，请检查认证器中的 6 位动态码")
	}
	if lastCounter > 0 && counter <= lastCounter {
		// 该时间步的验证码已被消费（重放）
		return false, errors.New("验证码已被使用，请等待下一个动态码")
	}
	if err := s.users.SetTOTPLastCounter(userID, counter); err != nil {
		// 记录失败时宁可拒绝本次登录：不记 counter 则同一验证码窗口内可重放
		return false, errors.New("验证码校验失败，请重试")
	}
	return true, nil
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

// RevokeAllSessions 吊销该用户当前全部已签发 JWT（登出全部设备）。
func (s *AuthService) RevokeAllSessions(userID int64) error {
	return s.users.RevokeAllSessions(userID)
}

// ResetPassword 忘记密码：用管理员生成的一次性令牌重置密码（公开接口）。
// 令牌一次性且带过期时间，重置成功后即失效。落库为 SHA-256 哈希，校验前
// 先把明文令牌哈希化再比对。
// 顺序：先做 bcrypt（与调用方无关的固定工作量，抹平时序差），校验通过后
// 由 DB 原子完成「令牌匹配 → 换密码 → 吊销会话 → 清令牌」。
func (s *AuthService) ResetPassword(username, token, newPass string) error {
	username = strings.TrimSpace(username)
	if err := validatePassword(newPass); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ok, err := s.users.ResetPasswordByToken(username, hashCode(token), string(hash))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("重置令牌无效或已过期，请向管理员重新申请")
	}
	return nil
}

// hashCode 重置令牌哈希（SHA-256 hex），与 user_service.GenerateResetToken 落库口径一致。
func hashCode(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
