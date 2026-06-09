# Adopting ix in an existing app

ix slots into any existing Go + schema-first project. You don't need a big-bang
rewrite. Work through the steps below in order. Each one ships on its own, so you
can stop after any step and still have a working app.

## 1. Adopt the runtime module (lowest risk, most payoff)

Replace your hand-rolled infrastructure with the imported, versioned runtime:

```sh
go get github.com/appsome/ix/runtime@latest
```

Map your existing packages to their runtime equivalents, then delete the
originals:

| Your hand-rolled package (typical location) | runtime package |
|---------------------------------------------|-----------------|
| `internal/<x>/util` (env, null helpers) | `runtime/util` |
| `internal/.../metric` | `runtime/metric` |
| `internal/.../pubsub` | `runtime/pubsub` |
| JWT / claims handling | `runtime/auth` |
| Casbin service | `runtime/authz` |
| auth/authz middleware | `runtime/middleware` |

The runtime's API differs from a typical hand-rolled version in a few deliberate
ways (see DESIGN §11 phase 2). The Casbin model is passed via `authz.WithModel`,
`authz.Auditor` returns `error`, and domain resolution lives in
`runtime/middleware`. Keep your own `model.conf` / `seed_policy.csv` and pass
them in.

## 2. Bring the codegen pipeline under ix

Adopt the manifest so `ix generate` / `ix migrate` drive your existing
`schema.sql → atlas → sqlc → gqlgen` flow. Your configs then become upgradeable:

```sh
ix init --module github.com/acme/widgets .   # writes ix.yaml + ix.lock in place
```

Then reconcile: point `ix.yaml` `paths:` at your existing locations, and let
`ix add core-schema graphql-server` reconcile the configs. Existing files are
left untouched, so review the diffs and adopt selectively. From here `ix diff` /
`ix upgrade` keep your sqlc/gqlgen/atlas wiring current.

## 3. Move per-entity code to the generator's shape

New entities go through `ix add entity --name <thing>`. Existing ones can stay
as they are. The generator's layout (queries + repository + graphqls, resolver
stubs from gqlgen) matches the conventional schema-first layout, so generated and
hand-written entities coexist.

## 4. Vendor infra + frontend

Run `ix add docker compose helm ci-github ci-gitlab` and, for a fresh admin,
`ix add admin-svelte`. Your bespoke charts and CI can be retired once the
vendored equivalents are tuned to your setup.

## Domain code stays put

Your domain code, whatever it is, is not part of ix. ix owns only the reusable
spine: runtime, codegen wiring, infra, and frontend scaffolding. Adopting ix
means swapping that spine for the shared one. It does not mean moving your domain
logic anywhere.
