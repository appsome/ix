# ix

A schema-first, full-stack scaffolding framework. New projects start with the
same pipeline wired up end to end:

```
schema.sql ─▶ Atlas migrations ─▶ sqlc (Go) ─▶ gqlgen (GraphQL) ─▶ chi
                                                      └─▶ Houdini ─▶ SvelteKit + shadcn-svelte
```

You **own** the generated code (shadcn-style vendoring), but `ix` keeps it
upgradeable.

## How it's distributed (hybrid model)

- **Runtime libraries** (`runtime/`: authz, auth, middleware, pubsub, metric,
  util) ship as a versioned Go module you `import` and upgrade with
  `go get -u github.com/appsome/ix/runtime@latest`.
- **Everything else** (codegen configs, server glue, infra, Helm, CI, the
  SvelteKit admin) is **vendored** into your repo by the `ix` CLI and upgraded
  in place via 3-way merge (`ix upgrade`).

See [docs/DESIGN.md](docs/DESIGN.md) for the full design.

## Layout

```
cmd/ix/        the CLI
internal/      CLI implementation + registry/manifest types
runtime/       imported Go module (own go.mod)
registry/      vendorable blocks (registry.json + blocks/<name>/)
templates/     base project scaffold for `ix init`
docs/          design + per-area docs
```

## CLI

```
ix init [dir]        scaffold a new project
ix add <block>...    vendor blocks (e.g. ix add authz, ix add entity --name product)
ix list              list available / installed blocks
ix generate          run the codegen pipeline
ix migrate new <n>   create an Atlas migration
ix upgrade [block]   re-render + 3-way merge a block at its new version
ix diff [block]      preview an upgrade
ix doctor            verify toolchain + detect drift
```

## Status

The framework works end-to-end:

- The runtime module (`util`, `metric`, `pubsub`, `auth`, `authz`, `middleware`,
  `jobs`) builds, vets clean, and passes hermetic tests.
- `ix init` scaffolds a runnable chi + gqlgen server, and `ix add` vendors blocks
  (rendering templates, applying anchor patches, recording `.ix/baseline/` copies
  for future merges).
- `ix add entity --name product` generates the sqlc queries, repository, GraphQL
  schema, and SvelteKit pages for an entity; `ix generate` produces the db
  bindings and resolver stubs.
- `ix diff` / `ix upgrade` apply a line-based 3-way merge of upstream block
  changes against your edits, with the usual `<<<<<<<` conflict markers.
- Infra blocks (`docker`, `compose`, `helm`, `ci-github`, `ci-gitlab`) and the
  `admin-svelte` frontend drop in with names derived from your module.

A runnable, offline-building [reference example](examples/reference) ties it
together, and [docs/porting.md](docs/porting.md) covers adopting ix in an
existing app.
