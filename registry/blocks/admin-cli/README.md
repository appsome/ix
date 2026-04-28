# Block: `admin-cli`

CLI for user/role/policy management. Builds on the authz block: it constructs
the project's vendored `internal/authz` service and drives its policy/role
methods.

Status: **available**. `ix add admin-cli` vendors:

- `cmd/admin-cli/main.go` — a standard-library (no cobra) CLI with `policy
  add|remove|list` and `role assign|remove|list` subcommands. Written once;
  yours to extend.

Requires `core-schema` (DB + `casbin_rule` table) and `authz` (the service +
model).

## Use

```
go build -o bin/admin-cli ./cmd/admin-cli

admin-cli policy list
admin-cli policy add    --sub alice --dom '*' --obj /widgets --act GET
admin-cli role assign   --user alice --role ADMIN --dom '*'
```

It connects with the same `DB_USER`/`DB_PASS`/`DB_HOST`/`DB_NAME` environment
variables as the API server.
