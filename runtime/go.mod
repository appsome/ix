// Module ix/runtime is the versioned runtime library imported by projects
// scaffolded with ix. It is the "imported, not vendored" half of the hybrid
// model (see ../docs/DESIGN.md §2): stable logic with a clean API that
// generated projects upgrade with `go get -u`, never a 3-way merge.
//
// It versions and releases independently of the ix CLI, using the Go submodule
// tag convention `runtime/vMAJOR.MINOR.PATCH`.
module github.com/appsome/ix/runtime

go 1.25.0

require (
	github.com/Blank-Xu/sql-adapter v1.1.2
	github.com/IguteChung/casbin-psql-watcher v1.0.0
	github.com/casbin/casbin/v2 v2.135.0
	github.com/go-chi/chi/v5 v5.3.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/hibiken/asynq v0.26.0
	github.com/lib/pq v1.12.3
	github.com/prometheus/client_golang v1.23.2
	golang.org/x/crypto v0.52.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.9.1 // indirect
	github.com/casbin/govaluate v1.10.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgx/v5 v5.1.1 // indirect
	github.com/jackc/puddle/v2 v2.1.2 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/redis/go-redis/v9 v9.14.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)
