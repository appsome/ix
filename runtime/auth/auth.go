// Package auth provides JWT token issue/validation, claims, roles, and password
// hashing: the core authentication primitives.
//
// It is deliberately decoupled from any database or user-repository: the
// project-side login flow (look up a user, check a password, mint a token)
// lives in the vendored auth-jwt block and composes these primitives. That
// keeps the runtime free of the project's user-table shape.
package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken = errors.New("auth: invalid token")
	ErrTokenExpired = errors.New("auth: token expired")
)

// Default token lifetimes. Override per call via TokenService fields.
const (
	DefaultAccessTokenDuration  = 15 * time.Minute
	DefaultRefreshTokenDuration = 7 * 24 * time.Hour
)

// Role is a coarse user role. Projects may define additional roles; these three
// are the baseline the middleware and seed policies assume.
type Role string

const (
	RoleAdmin    Role = "ADMIN"
	RoleOperator Role = "OPERATOR"
	RoleViewer   Role = "VIEWER"
)

// Claims are the JWT claims carried by an access token. PartyID and CountryCode
// are optional tenant-scoping claims consumed by the authz layer; they are
// omitted from the token when empty (omitempty) and the authz tenant resolver
// treats their absence as "fall back to the admin-only '*' domain".
type Claims struct {
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
	Role        Role   `json:"role"`
	PartyID     string `json:"party_id,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	jwt.RegisteredClaims
}

// TokenService issues and validates HS256 JWTs signed with Secret.
type TokenService struct {
	Secret          string
	AccessDuration  time.Duration
	RefreshDuration time.Duration
}

// NewTokenService returns a TokenService with the default durations.
func NewTokenService(secret string) *TokenService {
	return &TokenService{
		Secret:          secret,
		AccessDuration:  DefaultAccessTokenDuration,
		RefreshDuration: DefaultRefreshTokenDuration,
	}
}

// IssueAccessToken signs an access token for claims and returns the signed
// string and its expiry. The registered time claims (exp/iat/nbf) are set here;
// callers only populate the application claims.
func (s *TokenService) IssueAccessToken(claims Claims) (string, time.Time, error) {
	dur := s.AccessDuration
	if dur == 0 {
		dur = DefaultAccessTokenDuration
	}
	expiresAt := time.Now().Add(dur)

	claims.RegisteredClaims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	signed, err := token.SignedString([]byte(s.Secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// IssueRefreshToken signs an opaque refresh token whose subject is the user id.
// Storage/rotation of refresh tokens is the project's concern (auth-jwt block).
func (s *TokenService) IssueRefreshToken(userID int64) (string, error) {
	dur := s.RefreshDuration
	if dur == 0 {
		dur = DefaultRefreshTokenDuration
	}
	claims := &jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(dur)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.Secret))
}

// ValidateToken parses and validates an access token, returning its claims.
// It rejects non-HMAC signing methods and expired tokens.
func (s *TokenService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
			return nil, ErrTokenExpired
		}
		return claims, nil
	}
	return nil, ErrInvalidToken
}

// HashPassword creates a salted SHA-256 hash of password (the salt is typically
// the JWT secret). ComparePasswordBcrypt is provided for projects that store
// bcrypt hashes instead.
func HashPassword(password, salt string) (string, error) {
	h := sha256.New()
	h.Write([]byte(password + salt))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComparePasswordBcrypt compares a bcrypt hash with a plaintext password.
func ComparePasswordBcrypt(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
