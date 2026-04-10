# Block: `auth-jwt`

JWT issue/validate + claims, login flow, AuthMiddleware. The token/claims/
password logic is imported from `runtime/auth` + `runtime/middleware`; this
block vendors the project glue.

Status: **available**. `ix add auth-jwt` vendors:

- `internal/auth/wire.go` — `NewTokenService()` (reads `JWT_SECRET`),
  `AuthMiddleware`, `GetClaims`, and re-exports of the runtime `Claims`,
  `TokenService`, and `Role` types.
- `internal/auth/login.go` — a user-lookup-decoupled `Login` flow that verifies
  a password and issues an access token, so it compiles regardless of your
  users-table shape.
- `internal/api/schema/auth.graphqls` — a `login` mutation extending the base
  `Mutation` type, with an `AuthPayload` result.

It patches `cmd/api/main.go` at the `ix:wire-services` anchor to construct the
token service.

## Wiring it in (manual next steps)

The graphql-server router is private to `server.New`, so the block can't
auto-attach the middleware. After adding it:

1. Attach `auth.AuthMiddleware(tokenService)` to the routes you want
   authenticated in `internal/api/server`.
2. Build an `auth.UserLookup` over your users query, construct
   `auth.NewService(tokenService, lookup)`, and thread it into `resolver.New`.
3. Implement the generated `login` resolver by calling `Service.Login`.
