// Package middleware provides chi-compatible authentication and authorization
// middleware.
//
// It is the one place that depends on both runtime/auth (JWT claims) and
// runtime/authz (the Casbin enforcer). Domain resolution lives here too — moved
// out of authz so that package carries no dependency on auth.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/appsome/ix/runtime/auth"
	"github.com/appsome/ix/runtime/authz"
)

type contextKey string

// ClaimsContextKey is where AuthMiddleware stores the validated *auth.Claims.
const ClaimsContextKey contextKey = "claims"

var (
	ErrMissingAuthToken    = errors.New("missing authentication token")
	ErrInvalidAuthHeader   = errors.New("invalid authorization header format")
	ErrEmptyBearerToken    = errors.New("empty bearer token")
	ErrUnsupportedAuthType = errors.New("unsupported authorization scheme")
)

// tokenValidator is the narrow contract AuthMiddleware needs; *auth.TokenService
// satisfies it. The interface keeps the middleware testable without a real
// signing key.
type tokenValidator interface {
	ValidateToken(token string) (*auth.Claims, error)
}

// authzEnforcer is the narrow contract RequireAuthz needs; *authz.Service
// satisfies it.
type authzEnforcer interface {
	Enforce(ctx context.Context, sub authz.Subject, dom authz.Domain, obj authz.Resource, act authz.Action) (bool, error)
}

// AuthMiddleware validates the Bearer JWT and attaches the claims to the
// request context. Missing or invalid tokens return 401.
func AuthMiddleware(v tokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := ExtractBearerToken(r)
			if err != nil {
				http.Error(w, "Authorization required", http.StatusUnauthorized)
				return
			}
			claims, err := v.ValidateToken(token)
			if err != nil {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthz gates a route through the Casbin enforcer. Error contract:
//   - 401 when no claims are present (AuthMiddleware did not run).
//   - 403 when the enforcer denies, errors, or is nil (fail-closed).
//
// resource and action are forwarded verbatim to Enforce; routes pass the same
// shape the seed policy uses (e.g. "/sessions/:id", "GET").
func RequireAuthz(enforcer authzEnforcer, resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ClaimsContextKey).(*auth.Claims)
			if !ok || claims == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			// A nil enforcer is a misconfiguration; deny with the same shape
			// as a real deny so clients can't distinguish the two.
			if enforcer == nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			allowed, err := enforcer.Enforce(
				r.Context(),
				resolveSubject(claims),
				authz.Domain(ResolveDomain(r, claims)),
				authz.Resource(resource),
				authz.Action(action),
			)
			if err != nil || !allowed {
				// Fail-closed: storage failures and explicit denies look the
				// same to the client.
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetClaims extracts the claims attached by AuthMiddleware.
func GetClaims(r *http.Request) (*auth.Claims, bool) {
	claims, ok := r.Context().Value(ClaimsContextKey).(*auth.Claims)
	return claims, ok
}

// ResolveDomain returns the Casbin domain scoping a request. Order:
//  1. X-Tenant header (verbatim) — lets an admin scope without re-issuing a JWT.
//  2. claims.PartyID if non-empty.
//  3. the cross-tenant wildcard "*" for ADMIN callers.
//  4. "" otherwise — a guaranteed deny at the enforcer.
func ResolveDomain(r *http.Request, claims *auth.Claims) string {
	if r != nil {
		if header := strings.TrimSpace(r.Header.Get(authz.TenantHeader)); header != "" {
			return header
		}
	}
	if claims != nil && strings.TrimSpace(claims.PartyID) != "" {
		return claims.PartyID
	}
	if claims != nil && claims.Role == auth.RoleAdmin {
		return string(authz.CrossTenantDomain)
	}
	return ""
}

// resolveSubject prefers the email claim (seed policies key subjects by email),
// falling back to the user id so machine tokens still produce a non-empty
// subject — an empty subject would match wildcard rows by accident.
func resolveSubject(claims *auth.Claims) authz.Subject {
	if claims == nil {
		return ""
	}
	if claims.Email != "" {
		return authz.Subject(claims.Email)
	}
	if claims.UserID != 0 {
		return authz.Subject(strconv.FormatInt(claims.UserID, 10))
	}
	return ""
}

// ExtractBearerToken pulls a bearer token from the ?token= query parameter or
// the Authorization header.
func ExtractBearerToken(r *http.Request) (string, error) {
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token, nil
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return "", ErrMissingAuthToken
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", ErrInvalidAuthHeader
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", ErrUnsupportedAuthType
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", ErrEmptyBearerToken
	}
	return token, nil
}
