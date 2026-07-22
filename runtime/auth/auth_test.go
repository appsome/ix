package auth

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// TestTokenCarriesItsActor: a token issued to one user on another's behalf
// names the actor; one without an actor leaves the claim out, and a token
// issued before the claim existed reads as no actor.
func TestTokenCarriesItsActor(t *testing.T) {
	t.Parallel()
	svc := NewTokenService("test-secret")

	signed, _, err := svc.IssueAccessToken(Claims{UserID: 42, Email: "dana@example.com", Role: RoleViewer, ActorID: 7})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	claims, err := svc.ValidateToken(signed)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != 42 || claims.ActorID != 7 {
		t.Fatalf("the actor did not round-trip: %+v", claims)
	}

	plain, _, err := svc.IssueAccessToken(Claims{UserID: 42, Email: "dana@example.com", Role: RoleViewer})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(plain, ".")[1])
	if err != nil {
		t.Fatalf("decode the payload: %v", err)
	}
	if strings.Contains(string(payload), "actor_id") {
		t.Fatalf("a token without an actor names one: %s", payload)
	}

	type before struct {
		UserID int64 `json:"user_id"`
		jwt.RegisteredClaims
	}
	old := jwt.NewWithClaims(jwt.SigningMethodHS256, &before{UserID: 42, RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}})
	oldSigned, err := old.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err = svc.ValidateToken(oldSigned)
	if err != nil || claims.ActorID != 0 || claims.UserID != 42 {
		t.Fatalf("a token from before the claim: %+v, %v", claims, err)
	}
}
