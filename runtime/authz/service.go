package authz

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	sqladapter "github.com/Blank-Xu/sql-adapter"
	psqlwatcher "github.com/IguteChung/casbin-psql-watcher"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
)

// defaultModel is the embedded RBAC-with-domains + ABAC model used when no
// WithModel option is supplied. Projects override it by vendoring their own
// model.conf (the authz block) and passing it via WithModel.
//
//go:embed model.conf
var defaultModel string

// DefaultModel returns the embedded default Casbin model text.
func DefaultModel() string { return defaultModel }

// DefaultWatcherChannel is the Postgres LISTEN/NOTIFY channel used by the policy
// watcher when no explicit channel is configured.
const DefaultWatcherChannel = "casbin_policy"

// PolicyWatcher is the Casbin Watcher contract, re-exported so callers can
// attach their own watcher without importing the persist package.
type PolicyWatcher = persist.Watcher

const (
	casbinDriverName = "postgres"
	casbinTableName  = "casbin_rule"
)

// ErrNilDB is returned when NewService is called without a *sql.DB.
var ErrNilDB = errors.New("authz: db is nil")

// Service is the single authorization entry point. It wraps a Casbin
// SyncedEnforcer loading its policy from the casbin_rule table via
// Blank-Xu/sql-adapter, and exposes only purpose-built methods so future
// maintenance does not touch every caller.
type Service struct {
	enforcer *casbin.SyncedEnforcer
	auditor  Auditor
	model    string
	watcher  PolicyWatcher
}

// Option configures a Service at construction time.
type Option func(*Service)

// WithModel overrides the embedded default Casbin model.
func WithModel(modelText string) Option {
	return func(s *Service) {
		if modelText != "" {
			s.model = modelText
		}
	}
}

// WithAuditor injects an Auditor that records policy mutations. Unset disables
// auditing.
func WithAuditor(a Auditor) Option {
	return func(s *Service) { s.auditor = a }
}

// WithWatcher attaches a Casbin watcher so policy mutations on one replica
// propagate to every enforcer listening on the same channel. Its Close() is
// invoked from Service.Close().
func WithWatcher(w PolicyWatcher) Option {
	return func(s *Service) { s.watcher = w }
}

// NewService creates an enforcer backed by db. Sharing the same *sql.DB as the
// datastore keeps policy reads on the same connection limits. Construction
// fails early if the model is malformed or the adapter cannot be built.
func NewService(db *sql.DB, opts ...Option) (*Service, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	adapter, err := sqladapter.NewAdapter(db, casbinDriverName, casbinTableName)
	if err != nil {
		return nil, fmt.Errorf("authz: failed to construct sql adapter: %w", err)
	}

	svc, err := newServiceFromAdapter(adapter, opts...)
	if err != nil {
		return nil, err
	}

	if svc.watcher != nil {
		if err := svc.attachWatcher(svc.watcher); err != nil {
			return nil, err
		}
	}
	return svc, nil
}

// NewServiceWithWatcher builds an enforcer plus a Postgres-backed watcher
// listening on channel (blank ⇒ DefaultWatcherChannel). The watcher's lifetime
// is owned by Service.Close().
func NewServiceWithWatcher(ctx context.Context, db *sql.DB, connString, channel string, opts ...Option) (*Service, error) {
	if channel == "" {
		channel = DefaultWatcherChannel
	}

	w, err := psqlwatcher.NewWatcherWithConnString(ctx, connString, psqlwatcher.Option{Channel: channel})
	if err != nil {
		return nil, fmt.Errorf("authz: failed to construct policy watcher: %w", err)
	}

	svc, err := NewService(db, append(opts, WithWatcher(w))...)
	if err != nil {
		w.Close()
		return nil, err
	}
	return svc, nil
}

// NewInMemoryServiceForTesting constructs a Service whose enforcer keeps
// policies only in memory. Named "...ForTesting" so review flags accidental
// production use — in-memory state is lost on Close.
func NewInMemoryServiceForTesting(opts ...Option) (*Service, error) {
	return newServiceFromAdapter(nil, opts...)
}

func newServiceFromAdapter(adapter persist.Adapter, opts ...Option) (*Service, error) {
	svc := &Service{model: defaultModel}
	for _, opt := range opts {
		opt(svc)
	}

	m, err := model.NewModelFromString(svc.model)
	if err != nil {
		return nil, fmt.Errorf("authz: failed to parse model: %w", err)
	}

	var enforcer *casbin.SyncedEnforcer
	if adapter == nil {
		enforcer, err = casbin.NewSyncedEnforcer(m)
	} else {
		enforcer, err = casbin.NewSyncedEnforcer(m, adapter)
	}
	if err != nil {
		return nil, fmt.Errorf("authz: failed to construct enforcer: %w", err)
	}

	svc.enforcer = enforcer
	return svc, nil
}

func (s *Service) attachWatcher(w PolicyWatcher) error {
	if s == nil || s.enforcer == nil {
		return errors.New("authz: enforcer is not initialised")
	}
	if w == nil {
		return errors.New("authz: watcher is nil")
	}
	if err := s.enforcer.SetWatcher(w); err != nil {
		return fmt.Errorf("authz: SetWatcher failed: %w", err)
	}
	if err := w.SetUpdateCallback(func(string) { _ = s.enforcer.LoadPolicy() }); err != nil {
		return fmt.Errorf("authz: SetUpdateCallback failed: %w", err)
	}
	s.watcher = w
	return nil
}

// SetWatcher attaches a watcher after construction. Production callers should
// prefer NewServiceWithWatcher.
func (s *Service) SetWatcher(w PolicyWatcher) error { return s.attachWatcher(w) }

// Close releases the attached watcher. The underlying *sql.DB is owned by the
// caller and is intentionally not closed here.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	if s.watcher != nil {
		s.watcher.Close()
		s.watcher = nil
	}
	return nil
}

// Enforce is the single check used by all callers. (false, nil) means deny;
// (_, err) means the check could not be evaluated.
func (s *Service) Enforce(ctx context.Context, sub Subject, dom Domain, obj Resource, act Action) (bool, error) {
	_ = ctx // reserved for future tracing hooks; the v2 enforcer is not context-aware.
	if s == nil || s.enforcer == nil {
		return false, errors.New("authz: enforcer is not initialised")
	}

	// An empty policy set makes the matcher's eval(p.cond) error rather than
	// return false; translate that into the natural default-deny.
	policies, err := s.enforcer.GetPolicy()
	if err != nil {
		return false, fmt.Errorf("authz: enforce failed: %w", err)
	}
	if len(policies) == 0 {
		return false, nil
	}

	allowed, err := s.enforcer.Enforce(string(sub), string(dom), string(obj), string(act), DefaultCondition)
	if err != nil {
		return false, fmt.Errorf("authz: enforce failed: %w", err)
	}
	return allowed, nil
}

// AddPolicy inserts a `p` row. Condition defaults to DefaultCondition and
// Effect to EffectAllow when empty.
func (s *Service) AddPolicy(ctx context.Context, row PolicyRow) (bool, error) {
	if s == nil || s.enforcer == nil {
		return false, errors.New("authz: enforcer is not initialised")
	}
	cond, eft := normaliseCondEffect(row.Condition, row.Effect)

	added, err := s.enforcer.AddPolicy(string(row.Subject), string(row.Domain), string(row.Resource), string(row.Action), cond, string(eft))
	if err != nil {
		return false, fmt.Errorf("authz: add policy failed: %w", err)
	}
	if added {
		s.audit(ctx, AuditActionPolicyAdded, EntityTypePolicy, policyEntityID(row), policyChanges(row))
	}
	return added, nil
}

// RemovePolicy deletes a `p` row matching the supplied fields.
func (s *Service) RemovePolicy(ctx context.Context, row PolicyRow) (bool, error) {
	if s == nil || s.enforcer == nil {
		return false, errors.New("authz: enforcer is not initialised")
	}
	cond, eft := normaliseCondEffect(row.Condition, row.Effect)

	removed, err := s.enforcer.RemovePolicy(string(row.Subject), string(row.Domain), string(row.Resource), string(row.Action), cond, string(eft))
	if err != nil {
		return false, fmt.Errorf("authz: remove policy failed: %w", err)
	}
	if removed {
		s.audit(ctx, AuditActionPolicyRemoved, EntityTypePolicy, policyEntityID(row), policyChanges(row))
	}
	return removed, nil
}

// AddGroupingPolicy assigns a user to a role within a domain.
func (s *Service) AddGroupingPolicy(ctx context.Context, row GroupingRow) (bool, error) {
	if s == nil || s.enforcer == nil {
		return false, errors.New("authz: enforcer is not initialised")
	}
	added, err := s.enforcer.AddGroupingPolicy(string(row.User), string(row.Role), string(row.Domain))
	if err != nil {
		return false, fmt.Errorf("authz: add grouping policy failed: %w", err)
	}
	if added {
		s.audit(ctx, AuditActionGroupingAdded, EntityTypeGrouping, groupingEntityID(row), groupingChanges(row))
	}
	return added, nil
}

// RemoveGroupingPolicy revokes a user-to-role assignment.
func (s *Service) RemoveGroupingPolicy(ctx context.Context, row GroupingRow) (bool, error) {
	if s == nil || s.enforcer == nil {
		return false, errors.New("authz: enforcer is not initialised")
	}
	removed, err := s.enforcer.RemoveGroupingPolicy(string(row.User), string(row.Role), string(row.Domain))
	if err != nil {
		return false, fmt.Errorf("authz: remove grouping policy failed: %w", err)
	}
	if removed {
		s.audit(ctx, AuditActionGroupingRemoved, EntityTypeGrouping, groupingEntityID(row), groupingChanges(row))
	}
	return removed, nil
}

// ListPolicies returns the current `p` rows as typed PolicyRow structs.
func (s *Service) ListPolicies() ([]PolicyRow, error) {
	if s == nil || s.enforcer == nil {
		return nil, errors.New("authz: enforcer is not initialised")
	}
	raw, err := s.enforcer.GetPolicy()
	if err != nil {
		return nil, fmt.Errorf("authz: list policies failed: %w", err)
	}
	rows := make([]PolicyRow, 0, len(raw))
	for _, r := range raw {
		row := PolicyRow{}
		if len(r) > 0 {
			row.Subject = Subject(r[0])
		}
		if len(r) > 1 {
			row.Domain = Domain(r[1])
		}
		if len(r) > 2 {
			row.Resource = Resource(r[2])
		}
		if len(r) > 3 {
			row.Action = Action(r[3])
		}
		if len(r) > 4 {
			row.Condition = r[4]
		}
		if len(r) > 5 {
			row.Effect = Effect(r[5])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// GetRolesForUser returns the roles assigned to user within domain. An empty
// domain falls back to Casbin's no-domain lookup (every role for the user).
func (s *Service) GetRolesForUser(user Subject, domain Domain) ([]Subject, error) {
	if s == nil || s.enforcer == nil {
		return nil, errors.New("authz: enforcer is not initialised")
	}
	var (
		roles []string
		err   error
	)
	if domain == "" {
		roles, err = s.enforcer.GetRolesForUser(string(user))
	} else {
		roles, err = s.enforcer.GetRolesForUser(string(user), string(domain))
	}
	if err != nil {
		return nil, fmt.Errorf("authz: get roles for user failed: %w", err)
	}
	out := make([]Subject, 0, len(roles))
	for _, r := range roles {
		out = append(out, Subject(r))
	}
	return out, nil
}

// ListGroupingPolicies returns the current `g` rows as typed GroupingRow structs.
func (s *Service) ListGroupingPolicies() ([]GroupingRow, error) {
	if s == nil || s.enforcer == nil {
		return nil, errors.New("authz: enforcer is not initialised")
	}
	raw, err := s.enforcer.GetGroupingPolicy()
	if err != nil {
		return nil, fmt.Errorf("authz: list grouping policies failed: %w", err)
	}
	rows := make([]GroupingRow, 0, len(raw))
	for _, r := range raw {
		row := GroupingRow{}
		if len(r) > 0 {
			row.User = Subject(r[0])
		}
		if len(r) > 1 {
			row.Role = Subject(r[1])
		}
		if len(r) > 2 {
			row.Domain = Domain(r[2])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// audit records a policy mutation through the injected Auditor. No-op when no
// auditor is wired. Audit errors are swallowed: a failed audit write must not
// break a successful policy mutation.
func (s *Service) audit(ctx context.Context, action, entityType, entityID string, changes map[string]any) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.CreateAuditLog(ctx, AuditLogEntry{
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Changes:    changes,
	})
}

func normaliseCondEffect(cond string, eft Effect) (string, Effect) {
	if cond == "" {
		cond = DefaultCondition
	}
	if eft == "" {
		eft = EffectAllow
	}
	return cond, eft
}

func policyEntityID(row PolicyRow) string {
	return fmt.Sprintf("%s|%s|%s|%s", row.Subject, row.Domain, row.Resource, row.Action)
}

func groupingEntityID(row GroupingRow) string {
	return fmt.Sprintf("%s|%s|%s", row.User, row.Role, row.Domain)
}

func policyChanges(row PolicyRow) map[string]any {
	cond, eft := normaliseCondEffect(row.Condition, row.Effect)
	return map[string]any{
		"sub":  string(row.Subject),
		"dom":  string(row.Domain),
		"obj":  string(row.Resource),
		"act":  string(row.Action),
		"cond": cond,
		"eft":  string(eft),
	}
}

func groupingChanges(row GroupingRow) map[string]any {
	return map[string]any{
		"user": string(row.User),
		"role": string(row.Role),
		"dom":  string(row.Domain),
	}
}
