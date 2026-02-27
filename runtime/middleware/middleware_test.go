package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/appsome/ix/runtime/auth"
	"github.com/appsome/ix/runtime/authz"
)

// stubEnforcer lets RequireAuthz be tested without Casbin or Postgres.
type stubEnforcer struct {
	allow bool
	err   error
}

func (s stubEnforcer) Enforce(_ context.Context, _ authz.Subject, _ authz.Domain, _ authz.Resource, _ authz.Action) (bool, error) {
	return s.allow, s.err
}

func withClaims(r *http.Request, c *auth.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ClaimsContextKey, c))
}

func TestRequireAuthz(t *testing.T) {
	t.Parallel()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	cases := []struct {
		name     string
		claims   *auth.Claims
		enforcer authzEnforcer
		want     int
	}{
		{"no claims ⇒ 401", nil, stubEnforcer{allow: true}, http.StatusUnauthorized},
		{"nil enforcer ⇒ 403", &auth.Claims{Email: "a@b.c"}, nil, http.StatusForbidden},
		{"deny ⇒ 403", &auth.Claims{Email: "a@b.c"}, stubEnforcer{allow: false}, http.StatusForbidden},
		{"error ⇒ 403", &auth.Claims{Email: "a@b.c"}, stubEnforcer{err: context.DeadlineExceeded}, http.StatusForbidden},
		{"allow ⇒ 200", &auth.Claims{Email: "a@b.c"}, stubEnforcer{allow: true}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := RequireAuthz(tc.enforcer, "/x", "GET")(ok)
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.claims != nil {
				r = withClaims(r, tc.claims)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestResolveDomain(t *testing.T) {
	t.Parallel()

	t.Run("header wins", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(authz.TenantHeader, "TENANT_X")
		if got := ResolveDomain(r, &auth.Claims{PartyID: "ACME", Role: auth.RoleAdmin}); got != "TENANT_X" {
			t.Errorf("got %q, want TENANT_X", got)
		}
	})
	t.Run("party id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := ResolveDomain(r, &auth.Claims{PartyID: "ACME"}); got != "ACME" {
			t.Errorf("got %q, want ACME", got)
		}
	})
	t.Run("admin wildcard", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := ResolveDomain(r, &auth.Claims{Role: auth.RoleAdmin}); got != "*" {
			t.Errorf("got %q, want *", got)
		}
	})
	t.Run("non-admin no party ⇒ empty deny", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := ResolveDomain(r, &auth.Claims{Role: auth.RoleViewer}); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()
	t.Run("query param", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?token=abc", nil)
		if got, err := ExtractBearerToken(r); err != nil || got != "abc" {
			t.Errorf("got %q, %v", got, err)
		}
	})
	t.Run("header", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer xyz")
		if got, err := ExtractBearerToken(r); err != nil || got != "xyz" {
			t.Errorf("got %q, %v", got, err)
		}
	})
	t.Run("missing", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if _, err := ExtractBearerToken(r); err == nil {
			t.Error("expected error for missing token")
		}
	})
}
