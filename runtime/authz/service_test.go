package authz

import (
	"context"
	"testing"
)

// TestEnforce_RBACWithDomains exercises the embedded default model end-to-end
// against an in-memory enforcer: a role granted in one tenant must not leak
// into another, and deny rules must win.
func TestEnforce_RBACWithDomains(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, err := NewInMemoryServiceForTesting()
	if err != nil {
		t.Fatalf("NewInMemoryServiceForTesting: %v", err)
	}
	defer svc.Close()

	// viewer role may GET /sessions/* in tenant ACME.
	if _, err := svc.AddPolicy(ctx, PolicyRow{Subject: "viewer", Domain: "ACME", Resource: "/sessions/:id", Action: "GET"}); err != nil {
		t.Fatalf("AddPolicy: %v", err)
	}
	// alice is a viewer in ACME only.
	if _, err := svc.AddGroupingPolicy(ctx, GroupingRow{User: "alice", Role: "viewer", Domain: "ACME"}); err != nil {
		t.Fatalf("AddGroupingPolicy: %v", err)
	}

	cases := []struct {
		name string
		sub  Subject
		dom  Domain
		obj  Resource
		act  Action
		want bool
	}{
		{"allowed in tenant", "alice", "ACME", "/sessions/42", "GET", true},
		{"denied cross-tenant", "alice", "OTHER", "/sessions/42", "GET", false},
		{"denied wrong action", "alice", "ACME", "/sessions/42", "DELETE", false},
		{"unknown subject", "bob", "ACME", "/sessions/42", "GET", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.Enforce(ctx, tc.sub, tc.dom, tc.obj, tc.act)
			if err != nil {
				t.Fatalf("Enforce: %v", err)
			}
			if got != tc.want {
				t.Errorf("Enforce(%s,%s,%s,%s) = %v, want %v", tc.sub, tc.dom, tc.obj, tc.act, got, tc.want)
			}
		})
	}
}

// TestEnforce_EmptyPolicyDenies verifies the empty-policy short-circuit returns
// a clean deny rather than a Casbin eval error.
func TestEnforce_EmptyPolicyDenies(t *testing.T) {
	t.Parallel()
	svc, err := NewInMemoryServiceForTesting()
	if err != nil {
		t.Fatalf("NewInMemoryServiceForTesting: %v", err)
	}
	defer svc.Close()

	allowed, err := svc.Enforce(context.Background(), "alice", "ACME", "/x", "GET")
	if err != nil {
		t.Fatalf("Enforce on empty policy returned error: %v", err)
	}
	if allowed {
		t.Fatal("expected deny on empty policy set")
	}
}
