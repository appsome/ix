// Package authz is the Casbin RBAC-with-domains + ABAC authorization engine.
//
// Two design choices keep it runtime-safe:
//   - The Casbin model is a parameter (WithModel), defaulting to an embedded
//     RBAC-with-domains model, rather than a hard-coded embed. The authz block
//     vendors an editable model.conf the project can pass in.
//   - The Auditor interface returns only error, so authz carries no dependency
//     on the project's datastore.
package authz

import "context"

// Subject, Domain, Resource, and Action are typed string aliases used by the
// enforcer's request and policy entries, so callers cannot accidentally swap an
// action for a resource path without a compile error. The matcher treats them
// as opaque strings.
type (
	// Subject is the entity making the request — a user id, or a role name
	// inside grouping policies.
	Subject string
	// Domain is the tenant scope. The wildcard "*" is reserved for
	// cross-tenant (ADMIN) policies.
	Domain string
	// Resource is the object acted upon — a REST path or GraphQL operation.
	// keyMatch2 placeholders like "/sessions/:id" are supported.
	Resource string
	// Action is the verb. Regex matchers are supported, e.g. "(GET|LIST)".
	Action string
)

// Effect mirrors the Casbin `eft` column. Deny wins over allow.
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// CrossTenantDomain is the wildcard domain for policies spanning every tenant.
const CrossTenantDomain Domain = "*"

// DefaultCondition is the condition used by policies with no ABAC constraint.
// The matcher calls eval(p.cond) on every check, so an empty cond would be a
// syntax error — callers default to the literal "true".
const DefaultCondition = "true"

// TenantHeader is the HTTP header that scopes a request to a specific tenant.
// Its value is the full Casbin domain string, used verbatim.
const TenantHeader = "X-Tenant"

// PolicyRow is the shape of a `p` row, a convenience for ListPolicies callers.
type PolicyRow struct {
	Subject   Subject
	Domain    Domain
	Resource  Resource
	Action    Action
	Condition string
	Effect    Effect
}

// GroupingRow is a `g` row — a user-to-role assignment scoped to a domain.
type GroupingRow struct {
	User   Subject
	Role   Subject
	Domain Domain
}

// Auditor records policy mutations. It is satisfied by the project's audit
// service. Returning only error keeps authz free of any datastore type; a nil
// Auditor disables auditing.
type Auditor interface {
	CreateAuditLog(ctx context.Context, entry AuditLogEntry) error
}

// AuditLogEntry is the structured record passed to an Auditor.
type AuditLogEntry struct {
	UserID     *int64
	Username   string
	Action     string
	EntityType string
	EntityID   string
	IPAddress  string
	UserAgent  string
	Changes    map[string]any
	Metadata   map[string]any
}

const (
	EntityTypePolicy   = "authz.policy"
	EntityTypeGrouping = "authz.grouping"
)

const (
	AuditActionPolicyAdded     = "authz.policy.added"
	AuditActionPolicyRemoved   = "authz.policy.removed"
	AuditActionGroupingAdded   = "authz.grouping.added"
	AuditActionGroupingRemoved = "authz.grouping.removed"
)
