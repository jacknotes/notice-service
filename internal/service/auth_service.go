package service

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"notice-service/internal/model"
	"notice-service/internal/repository"
)

type AuthClaims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type AuthService struct {
	users     *repository.UserRepo
	jwtSecret []byte
	adminUser string
	adminPass string
	tokenTTL  time.Duration
}

func NewAuthService(db *sql.DB, jwtSecret, adminUser, adminPass string) *AuthService {
	return &AuthService{
		users:     repository.NewUserRepo(db),
		jwtSecret: []byte(jwtSecret),
		adminUser: adminUser,
		adminPass: adminPass,
		tokenTTL:  24 * time.Hour,
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

func (s *AuthService) Login(username, password string) (string, *model.User, error) {
	u, err := s.users.GetByUsername(username)
	if errors.Is(err, repository.ErrNotFound) {
		return "", nil, errors.New("用户名或密码错误")
	}
	if err != nil {
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", nil, errors.New("用户名或密码错误")
	}
	tok, err := s.IssueToken(u.ID, u.Role)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}

// ChangePassword 校验旧密码并更新为新密码。
func (s *AuthService) ChangePassword(userID int64, oldPass, newPass string) error {
	if len(newPass) < 6 {
		return errors.New("新密码至少 6 位")
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
