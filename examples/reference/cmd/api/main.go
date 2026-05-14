// Command api is the ix reference application: the smallest end-to-end app that
// exercises the schema-first pipeline (schema.sql → sqlc, see ../../sqlc.yaml)
// and consumes every package of the imported ix runtime module.
//
// It is intentionally gqlgen-free so it builds offline: a plain chi server with
// a JWT-authenticated, Casbin-authorized /widgets route backed by the sqlc
// Queries. A full ix project adds the gqlgen layer via the graphql-server block.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"

	"github.com/appsome/ix/examples/reference/internal/datastore/pkg/db"
	"github.com/appsome/ix/runtime/auth"
	"github.com/appsome/ix/runtime/authz"
	"github.com/appsome/ix/runtime/metric"
	"github.com/appsome/ix/runtime/middleware"
	"github.com/appsome/ix/runtime/util"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- runtime/auth: issue + validate JWTs -------------------------------
	tokens := auth.NewTokenService(util.GetEnv("JWT_SECRET", "dev-secret"))

	// --- runtime/authz: an in-memory Casbin enforcer for the demo ----------
	// (A real app uses authz.NewServiceWithWatcher against Postgres.)
	enforcer, err := authz.NewInMemoryServiceForTesting()
	if err != nil {
		log.Fatalf("authz: %v", err)
	}
	defer func() { _ = enforcer.Close() }()
	// Grant the admin role read access to widgets cross-tenant, and map our
	// demo user onto it.
	_, _ = enforcer.AddPolicy(ctx, authz.PolicyRow{Subject: "admin", Domain: "*", Resource: "/widgets", Action: "GET"})
	_, _ = enforcer.AddGroupingPolicy(ctx, authz.GroupingRow{User: "demo@example.com", Role: "admin", Domain: "*"})

	// --- runtime/metric: Prometheus /metrics on its own port --------------
	metrics := metric.New()
	wg := &sync.WaitGroup{}
	metrics.Start(ctx, wg)

	// --- the schema-first datastore: open Postgres, build sqlc Queries -----
	var queries *db.Queries
	if database := openDB(); database != nil {
		defer func() { _ = database.Close() }()
		queries = db.New(database)
	}

	// --- chi server with runtime/middleware auth + authz -------------------
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	// Demo helper: mint a token for the seeded user (a real app has a login flow).
	r.Get("/token", func(w http.ResponseWriter, _ *http.Request) {
		t, _, err := tokens.IssueAccessToken(auth.Claims{UserID: 1, Email: "demo@example.com", Role: auth.RoleAdmin})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"access_token": t})
	})

	// /widgets is gated: AuthMiddleware validates the JWT, RequireAuthz runs
	// the Casbin check, then the handler reads through the sqlc Queries.
	r.Group(func(pr chi.Router) {
		pr.Use(middleware.AuthMiddleware(tokens))
		pr.Use(middleware.RequireAuthz(enforcer, "/widgets", "GET"))
		pr.Get("/widgets", func(w http.ResponseWriter, req *http.Request) {
			if queries == nil {
				http.Error(w, "database not configured (set DB_*)", http.StatusServiceUnavailable)
				return
			}
			rows, err := queries.ListWidgets(req.Context(), db.ListWidgetsParams{Limit: 50, Offset: 0})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, rows)
		})
	})

	port := util.GetEnv("REST_PORT", "8080")
	log.Printf("reference api listening on :%s", port)
	_ = http.ListenAndServe(":"+port, r)
}

func openDB() *sql.DB {
	host := util.GetEnv("DB_HOST", "")
	if host == "" {
		log.Printf("DB_HOST unset — /widgets will report 503 until configured")
		return nil
	}
	dsn := "postgres://" + util.GetEnv("DB_USER", "app") + ":" + util.GetEnv("DB_PASS", "app") +
		"@" + host + "/" + util.GetEnv("DB_NAME", "app") + "?sslmode=" + util.GetEnv("SSL_MODE", "disable")
	d, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Printf("db open: %v", err)
		return nil
	}
	d.SetConnMaxLifetime(time.Minute)
	return d
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
