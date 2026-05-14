# ix reference example

The smallest end-to-end app that exercises both halves of ix's hybrid model:

- **the schema-first pipeline** — [`internal/datastore/schema.sql`](internal/datastore/schema.sql)
  + [`queries/widget.sql`](internal/datastore/queries/widget.sql) → `sqlc generate`
  → the committed [`internal/datastore/pkg/db`](internal/datastore/pkg/db) bindings;
- **the imported runtime** — [`cmd/api/main.go`](cmd/api/main.go) wires every
  `github.com/appsome/ix/runtime` package: `auth` (JWT), `authz` (Casbin),
  `middleware` (chi auth + authz gate), `metric` (Prometheus), `util`.

It is intentionally gqlgen-free so it builds offline; a full ix project adds the
GraphQL layer via the `graphql-server` block. Until `runtime/v0.1.0` is
published, the module uses a `replace` to the in-repo runtime (see `go.mod`).

## Run

```sh
go run ./cmd/api          # :8080, plus Prometheus metrics on $METRIC_PORT

curl localhost:8080/health                                  # ok
TOKEN=$(curl -s localhost:8080/token | jq -r .access_token) # demo admin JWT
curl localhost:8080/widgets                                 # 401 (no token)
curl -H "Authorization: Bearer $TOKEN" localhost:8080/widgets
# 503 until DB_* are set; with Postgres reachable it lists widgets via sqlc.
```

Set `DB_HOST/DB_NAME/DB_USER/DB_PASS` (and apply the schema) to make `/widgets`
return rows. The `/token` route mints a token for a seeded admin who the
in-memory Casbin policy authorizes for `GET /widgets`; any other subject is
denied (403) and an invalid token is rejected (401).
