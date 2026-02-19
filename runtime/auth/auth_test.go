package auth

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	t.Parallel()
	svc := NewTokenService("test-secret")

	signed, expiresAt, err := svc.IssueAccessToken(Claims{
		UserID:  42,
		Email:   "admin@example.com",
		Role:    RoleAdmin,
		PartyID: "ACME",
	})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt %v not in the future", expiresAt)
	}

	claims, err := svc.ValidateToken(signed)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != 42 || claims.Email != "admin@example.com" || claims.Role != RoleAdmin || claims.PartyID != "ACME" {
		t.Fatalf("claims round-trip mismatch: %+v", claims)
	}
}

func TestValidateRejectsWrongSecret(t *testing.T) {
	t.Parallel()
	signed, _, err := NewTokenService("secret-a").IssueAccessToken(Claims{UserID: 1})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if _, err := NewTokenService("secret-b").ValidateToken(signed); err == nil {
		t.Fatal("expected validation to fail with a different secret")
	}
}

func TestValidateRejectsExpired(t *testing.T) {
	t.Parallel()
	svc := &TokenService{Secret: "s", AccessDuration: -time.Minute}
	signed, _, err := svc.IssueAccessToken(Claims{UserID: 1})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	// jwt validation rejects the expired exp claim before our own check.
	if _, err := svc.ValidateToken(signed); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestHashPasswordDeterministic(t *testing.T) {
	t.Parallel()
	a, _ := HashPassword("pw", "salt")
	b, _ := HashPassword("pw", "salt")
	if a != b {
		t.Fatal("HashPassword not deterministic for same input")
	}
	if c, _ := HashPassword("pw", "other"); c == a {
		t.Fatal("HashPassword ignored the salt")
	}
}
