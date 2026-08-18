package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"

	"github.com/google/uuid"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
	EncryptKey string
	Port       string
	InstanceID string
	AdminUser  string
	AdminPass  string
}

func Load() *Config {
	return &Config{
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "notice"),
		DBPassword: getEnv("DB_PASSWORD", "notice123"),
		DBName:     getEnv("DB_NAME", "notice_service"),
		JWTSecret:  getEnv("JWT_SECRET", "change-me"),
		EncryptKey: getEnv("ENCRYPT_KEY", randomHex(32)),
		Port:       getEnv("PORT", "8080"),
		InstanceID: getEnv("INSTANCE_ID", uuid.NewString()),
		AdminUser:  getEnv("ADMIN_USER", "admin"),
		AdminPass:  getEnv("ADMIN_PASS", "admin123"),
	}
}

func (c *Config) DSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" +
		c.DBName + "?parseTime=true&charset=utf8mb4&loc=Local"
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0123456789abcdef0123456789abcdef"
	}
	return hex.EncodeToString(b)
}
