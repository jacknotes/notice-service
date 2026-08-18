package service

import (
	"testing"
	"time"
)

func TestJWTIssueAndVerify(t *testing.T) {
	svc := NewAuthService(nil, "secret-secret-secret", "admin", "admin123")
	tok, err := svc.IssueToken(1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.VerifyToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 1 || claims.Role != "admin" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestJWTTamperedRejected(t *testing.T) {
	svc := NewAuthService(nil, "secret-secret-secret", "admin", "admin123")
	tok, _ := svc.IssueToken(1, "admin")
	if _, err := svc.VerifyToken(tok + "x"); err == nil {
		t.Error("tampered token should be rejected")
	}
}

func TestJWTExpired(t *testing.T) {
	svc := NewAuthService(nil, "secret-secret-secret", "admin", "admin123")
	tok, err := svc.IssueTokenWithTTL(1, "user", -1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyToken(tok); err == nil {
		t.Error("expired token should be rejected")
	}
}
