# ix — Design

> A full-stack, schema-first scaffolding framework. You own the code; `ix` keeps
> it upgradeable.

This document is the shape the implementation follows: the contracts (manifest
format, block layout, CLI commands, upgrade algorithm) and the reasoning behind
them.

---

## 1. Vision

`ix` scaffolds new projects with a consistent spine:

```
desired SQL schema  ──▶  Atlas (versioned up/down migrations)
        │
        └──▶ sqlc (Go db bindings)  ─┐
                                     ├──▶ gqlgen (GraphQL server)  ──▶ chi HTTP
        schema-derived types  ───────┘                                  │
                                                                        ▼
                              Houdini codegen ──▶ SvelteKit + shadcn-svelte admin
```

The developer edits one source of truth (`schema.sql` plus `queries/*.sql` and
`*.graphqls`) and the toolchain regenerates everything downstream. `ix` owns the
*wiring* of that pipeline (configs, scripts, Makefile targets, CI, Docker, Helm)
and a library of *building blocks* (auth, authz, pubsub, metrics, admin CRUD)
you can drop in.

### Non-goals

- `ix` is **not** a runtime web framework you hand control to. Generated apps
  are plain Go (chi + gqlgen) and plain SvelteKit. `ix` assembles and upgrades
  them; it does not sit in the request path.
- `ix` does **not** carry any business domain. Your domain code is yours; `ix`
  owns only the reusable spine underneath it.

---

## 2. The hybrid distribution model

The single most important decision: **what is imported vs. what is vendored.**

| Layer | Distribution | Upgrade path | Why |
|-------|-------------|--------------|-----|
| **Runtime libraries** (`ix/runtime/...`: authz, auth/jwt, middleware, pubsub, metric, util) | Versioned Go **module** you `import` | `go get -u github.com/appsome/ix/runtime@latest` | Stable, well-tested logic with a clean API. Idiomatic Go upgrade. No merge pain. |
| **Codegen wiring** (`schema.sql`, `sqlc.yaml`, `atlas.hcl`, `gqlgen.yml`, `scripts/`) | **Vendored** into your repo | `ix upgrade` (3-way merge) | You constantly edit these; they must live in your tree. |
| **Generated/owned glue** (chi server bootstrap, resolver helpers, entity CRUD) | **Vendored** | `ix upgrade` | You customize per project; ownership matters (shadcn philosophy). |
| **Infra** (Makefile, Dockerfiles, docker-compose, Helm charts, GitHub/GitLab CI) | **Vendored** | `ix upgrade` | Project-specific values; you tune them. |
| **Frontend** (SvelteKit app, shadcn-svelte components, Houdini config, generated pages) | **Vendored** | `ix upgrade` | shadcn-svelte's own model — components live in your repo. |

This mirrors why shadcn vendors UI (you restyle it) but you'd never vendor
`react` itself. Our `runtime` is "react"; everything else is "your components."

```
          imported, versioned                vendored, owned
        ┌─────────────────────┐         ┌──────────────────────────┐
your    │  github.com/appsome/ │  go get │  internal/authz/*.conf   │ ix upgrade
project │  ix/runtime/authz    │◀────────│  sqlc.yaml, atlas.hcl    │◀───────────
        │  ix/runtime/pubsub   │         │  charts/, .github/, ...  │
        └─────────────────────┘         │  web/ (SvelteKit)        │
                                         └──────────────────────────┘
```

---

## 3. Repository layout (this repo)

```
ix/
├── cmd/ix/                     # the `ix` CLI entrypoint
├── internal/
│   ├── cli/                    # command implementations (init/add/upgrade/generate/...)
│   └── registry/               # block discovery, manifest/lock parsing, merge engine
├── runtime/                    # SEPARATE Go module: github.com/appsome/ix/runtime
│   ├── go.mod                  #   imported by generated projects
│   ├── authz/                  #   Casbin RBAC-with-domains + psql watcher
│   ├── auth/                   #   JWT issue/validate, claims
│   ├── middleware/             #   chi auth/authz/audit middleware
│   ├── pubsub/                 #   in-proc broker + Postgres LISTEN/NOTIFY driver
│   ├── metric/                 #   Prometheus helpers + error registry
│   └── util/                   #   env, sql null helpers, parse, rand, password
├── registry/                   # vendorable blocks (shadcn-style registry)
│   ├── registry.json           #   index of all blocks + versions
│   └── blocks/<name>/
│       ├── block.yaml          #     block metadata (files, deps, vars, hooks)
│       └── templates/          #     Go-template'd files materialized into projects
├── templates/project/          # base scaffold materialized by `ix init`
├── examples/                   # end-to-end reference app (examples/reference)
└── docs/                       # this design + per-area docs
```

The `runtime/` submodule has its own `go.mod` so it versions and releases
independently of the CLI. The CLI embeds the `registry/` tree at build time
(`go:embed`) so a given `ix` binary is a self-contained snapshot of all blocks.

---

## 4. The `ix` CLI

```
ix init [dir]            Scaffold a new project (interactive or --config ix.yaml)
ix add <block>...        Vendor one or more blocks into the project
ix list [--installed]    List available / installed blocks
ix generate [pipeline]   Run the codegen pipeline (atlas|sqlc|gqlgen|houdini|all)
ix migrate new <name>    atlas migrate diff <name> --env local
ix upgrade [block...]    Re-render blocks at the new version, 3-way merge into tree
ix diff [block...]       Show what an upgrade would change, without writing
ix doctor                Verify toolchain (atlas, sqlc, gqlgen, node, ...) + drift
ix version               CLI + embedded registry version
```

`generate`, `migrate`, and `doctor` are thin, opinionated wrappers around the
tools the project already vendors — they read `ix.yaml` for paths so they work
regardless of where the user put things.

---

## 5. Project manifest & lockfile

Two committed files at the project root define the `ix`-managed surface.

### `ix.yaml` — project configuration (human-edited)

```yaml
version: 1
module: gitlab.com/acme/widgets        # Go module path of the generated project
database:
  engine: postgresql
  postgis: true                        # toggles the PostGIS extension + geom override
paths:
  schema: internal/datastore/schema.sql
  queries: internal/datastore/queries
  migrations: migrations
  graphql_schema: internal/api/schema
  web: web                             # SvelteKit app root
frontend:
  framework: sveltekit
  ui: shadcn-svelte
  graphql: houdini
blocks:                                # installed blocks (managed by `ix add`)
  - name: core-schema
  - name: graphql-server
  - name: authz
  - name: docker
  - name: helm
  - name: ci-github
  - name: admin-svelte
runtime: github.com/appsome/ix/runtime  # import path of the runtime module
```

### `ix.lock` — resolved state (tool-managed, committed)

```yaml
version: 1
cli_version: 0.3.1
registry_version: 0.3.1
blocks:
  authz:
    version: 0.2.0
    runtime: { module: github.com/appsome/ix/runtime, version: v0.2.0 }
    files:
      internal/authz/model.conf:    { hash: sha256:ab12…, managed: true }
      internal/authz/seed_policy.csv:{ hash: sha256:cd34…, managed: false } # user-owned after first render
```

`hash` is the SHA-256 of the **last pristine render** of that file. It is the
baseline for 3-way merge (see §7). `managed: false` files are rendered once on
`add` and never touched again by `upgrade` (e.g. seed data).

---

## 6. What a block is

A block is a unit you `ix add`. Its `block.yaml`:

```yaml
name: authz
version: 0.2.0
category: runtime-glue            # runtime-glue | codegen | infra | frontend | generator
summary: Casbin RBAC-with-domains authorization, wired into chi + gqlgen.
requires:                         # other blocks that must be present
  - core-schema
runtime:                          # versioned module dependency added to go.mod
  module: github.com/appsome/ix/runtime
  version: v0.2.0
  packages: [authz, middleware]
vars:                             # resolved from ix.yaml or prompted; usable in templates
  - { name: Module, from: module }
files:                            # rendered with text/template; dest is project-relative
  - { src: model.conf.tmpl,        dest: internal/authz/model.conf }
  - { src: seed_policy.csv.tmpl,   dest: internal/authz/seed_policy.csv, managed: false }
  - { src: wire.go.tmpl,           dest: internal/api/authz_wire.go }
  - { src: migration.sql.tmpl,     dest: "migrations/{{.Timestamp}}_casbin_rule.sql", once: true }
patches:                          # structured edits to files owned by other blocks
  - file: cmd/api/main.go
    anchor: "// ix:wire-services"
    insert: "authzService, _ := authz.NewServiceWithWatcher(ctx, db, connStr, channel)"
hooks:
  post_add:   ["go mod tidy"]
  post_upgrade: ["ix generate sqlc"]
```

Key ideas:

- **`files`** are Go-templated and copied to `dest`. `managed: false` ⇒ render
  once, then leave alone. `once: true` ⇒ skip if `dest` already exists.
- **`patches`** let a block inject into files another block owns, via named
  **anchors** (`// ix:wire-services` comments the base scaffold ships). Anchors
  are stable insertion points — no fragile line numbers. An upgrade re-applies
  patches idempotently (skip if the insert text is already present).
- **`runtime`** declares the versioned module dep, keeping the hybrid split
  explicit and machine-checkable.

---

## 7. The upgrade algorithm (3-way merge)

The hard problem in any vendoring tool: the user edited the vendored file, and
upstream changed it too. `ix` solves it with a stored baseline + diff3.

For each `managed: true` file in a block being upgraded:

```
BASE    = last pristine render (recoverable: re-render the OLD block version)
THEIRS  = current file on disk (user's edits)
OURS    = new pristine render (NEW block version)

merged, conflicts = diff3(OURS, BASE, THEIRS)
write merged → dest
record sha256(OURS) → ix.lock
```

How `ix` gets `BASE` without storing it: each `ix` binary embeds the registry
at its build version, and `ix.lock` records the block version that produced the
current file. On `ix upgrade`, the CLI:

1. Reads the old block version from `ix.lock`.
2. Fetches that old version's templates from the embedded **history** (the
   registry ships prior template versions under `registry/blocks/<name>/.history/<ver>/`)
   *or*, if absent, falls back to a `.ix/baseline/` directory committed in the
   project (pristine copies written on every `add`/`upgrade`).
3. Re-renders BASE with the project's current vars, computes the 3-way merge.

**Decision: ship `.ix/baseline/` as the source of truth.** It is simplest and
fully self-contained — no need to embed template history in every binary. The
directory is committed, gitignored from editors, and never hand-edited. Trade-
off: a little repo weight; in exchange, `ix upgrade` works offline and is
deterministic regardless of which CLI version you upgrade from.

Conflicts are written with standard `<<<<<<< / ======= / >>>>>>>` markers and
listed in the `ix upgrade` summary so they're resolved like any merge.

Runtime-module upgrades are orthogonal and trivial:
`go get -u github.com/appsome/ix/runtime@<ver>` — `ix upgrade` runs it when a
block's `runtime.version` advances.

---

## 8. Block catalogue

| Block | Category | What it gives you |
|-------|----------|-------------------|
| `core-schema` | codegen | The pipeline spine: `schema.sql`, `sqlc.yaml`, `atlas.hcl`, generate/migrate scripts. PostGIS optional. |
| `graphql-server` | codegen | chi server + gqlgen scaffold + connection/filter/pagination helpers. |
| `entity` (generator) | generator | `ix add entity --name foo` scaffolds a full vertical slice (queries + repository + graphqls + resolver, optionally frontend). |
| `authz` | runtime-glue | Casbin model + seed + watcher wiring; logic in `runtime/authz`. |
| `auth-jwt` | runtime-glue | JWT issue/validate + claims; logic in `runtime/auth`. |
| `pubsub` | runtime-glue | in-proc + Postgres NOTIFY broker; logic in `runtime/pubsub`. |
| `metrics` | runtime-glue | Prometheus endpoint + error registry; logic in `runtime/metric`. |
| `jobs` | runtime-glue | Asynq (Redis) background task queue + worker binary; logic in `runtime/jobs`. |
| `jobs-admin` | frontend | Optional dashboard to monitor/manage Asynq jobs. |
| `admin-cli` | infra | user/role/policy management CLI. |
| `docker` | infra | multi-service Dockerfiles + air hot-reload. |
| `compose` | infra | docker-compose: postgres(+postgis), redis, services, migrations. |
| `helm` | infra | Helm chart: Deployment/Service/Ingress/HPA + migrations hook. |
| `ci-github` | infra | GitHub Actions: lint/test/build. |
| `ci-gitlab` | infra | GitLab CI: lint/test/build + pre-commit. |
| `admin-svelte` | frontend | SvelteKit + shadcn-svelte + Houdini admin shell with generated CRUD. |

Runtime-glue blocks split in two: the reusable logic is a package under
`runtime/` (imported); the *wiring* (embeds, config, registration in `main.go`)
is vendored by the block.

---

## 9. Frontend architecture (`admin-svelte`)

The admin frontend is SvelteKit + shadcn-svelte.

```
web/
├── src/client.ts                # Houdini client + auth link (Bearer/X-Tenant)
├── src/lib/ix/                  # thin shared lib (vendored, owned)
│   ├── DataTable.svelte         #   sortable table primitive
│   ├── auth.ts                  #   login, token store, tenant switcher state
│   └── utils.ts                 #   class helpers (shadcn-svelte style)
├── src/routes/(admin)/<entity>/ # generated per-entity: +page (list) + +page.gql,
│                                #   [id]/ (show/edit + mutations), new/ (create)
└── houdini.config.js            # points at the gqlgen /query endpoint
```

- **Houdini** chosen for GraphQL: native SvelteKit `load` integration, reactive
  stores, normalized cache, subscriptions (maps onto the existing gqlgen
  `Subscription` topics from `runtime/pubsub`).
- **Generation**: `ix add entity --name foo --frontend` emits list/show-edit/
  create routes + Houdini `+page.gql` documents matching the entity's generated
  GraphQL schema; `ix generate houdini` then produces the typed client.
  Generated pages are plain Svelte you own; the shared `lib/ix` keeps the
  boilerplate down without putting a runtime abstraction in the request path.
- Auth flow: JWT in an `Authorization: Bearer` header, optional `X-Tenant` for
  admins scoping to a Casbin domain.

---

## 10. Versioning & release

- `runtime/` is tagged `runtime/vMAJOR.MINOR.PATCH` (Go submodule tag convention).
- The CLI + registry release together as `vMAJOR.MINOR.PATCH`; `ix version`
  reports both and the embedded registry version.
- Blocks carry independent `version:` in `block.yaml`; `registry.json` is the
  index. `ix upgrade` is per-block, so a project upgrades `helm` without touching
  `authz`.
- Compatibility: a block declares the minimum `runtime` version it needs; `ix
  doctor` flags a project whose imported `runtime` is older than an installed
  block requires.

### Release checklist when `runtime/` changes

The runtime is a published Go module that generated projects `import` and pin
by version (block manifests set `runtime: version: vX.Y.Z`, which lands in the
project's `go.mod`). **A runtime change that isn't tagged is invisible to every
downstream project**: they keep resolving the old tag, and a local checkout
silently drifts from what CI fetches. So whenever you touch `runtime/`:

1. **Tag it.** Cut `runtime/vX.Y.Z` for the new runtime tree (semver: patch for
   fixes, minor for additive API, major for breaking). This is separate from
   the CLI/registry version; the runtime versions on its own cadence.
2. **Bump the block pins.** Update `runtime: version:` in every `block.yaml`
   that imports the changed package(s) (e.g. `auth-jwt`, `authz`, `metrics`,
   `pubsub`) to the new tag, and bump those blocks' own `version:`.
3. **Bump + tag the registry/CLI** (`registry.json` `version`, then the repo's
   `vX.Y.Z` tag) so a rebuilt `ix` embeds the updated manifests.
4. **Rebuild `ix`** (`go build ./cmd/ix`) so the embedded registry reflects the
   new pins before you `ix add`/`ix upgrade` anywhere.
5. In existing projects, `go get -u github.com/appsome/ix/runtime@latest` (or the
   pinned tag) adopts the new runtime; `ix upgrade <block>` re-renders glue.

Skipping step 1 is the common trap: the registry can advance (new blocks) while
`runtime/` stays on an old tag, which is fine *only* if the runtime tree truly
did not change. If it did, downstream projects can never `go get -u` to the fix.

---

## 11. Status

The framework is built and verified end-to-end. The runtime module
(`util`, `metric`, `pubsub`, `auth`, `authz`, `middleware`, `jobs`) builds and
passes hermetic tests. The CLI does `init`, `add`, `list`, `generate`, `migrate`,
`diff`, `upgrade`, and `doctor`, with the 3-way merge engine driving in-place
block upgrades against the committed `.ix/baseline/`. The block catalogue in §8 is
fully addable, and a runnable [reference example](../examples/reference) ties the
pipeline together (`schema.sql` → sqlc → a chi server consuming the runtime,
behind JWT + Casbin-gated routes).

A few design choices worth keeping in mind:

- The runtime's API takes a few things as parameters rather than hard-coding
  them: the Casbin model (`authz.WithModel`, with an embedded default),
  `authz.Auditor` returns `error` so it carries no datastore type, and domain
  resolution lives in `runtime/middleware` so `authz` has no `auth` dependency.
- `.ix/baseline/` keeps committed pristine copies of rendered files. That costs
  a little repo weight, but it makes `ix upgrade` deterministic and offline.
- `entity` scaffolds a first-draft `.graphqls` from the table for you to edit
  rather than inferring GraphQL types from the SQL schema automatically.
