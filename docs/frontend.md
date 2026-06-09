# Frontend (`admin-svelte`)

The admin frontend is **SvelteKit + shadcn-svelte + Houdini**. See
[DESIGN.md](DESIGN.md) §9.

## Stack choices

- **SvelteKit** — routing, SSR, file-based pages.
- **shadcn-svelte** — vendored UI components you own and restyle (same
  philosophy as ix itself).
- **Houdini** — GraphQL client with native SvelteKit `load` integration,
  reactive document stores, normalized cache, and subscriptions that map onto
  the gqlgen `Subscription` topics backed by `runtime/pubsub`.

## Shape

```
web/
├── src/lib/ix/                  # thin shared lib (vendored, owned)
│   ├── gql/                     #   Houdini client + Bearer/X-Tenant auth link
│   ├── data-table.svelte        #   sortable/filterable/paginated table
│   ├── crud/                    #   list/show/form primitives over Houdini stores
│   ├── auth/                    #   login, token store, route guard, tenant switcher
│   └── components/ui/           #   shadcn-svelte components
├── src/routes/(admin)/<entity>/ # generated per-entity: list / [id] / new
└── houdini.config.js            # points at the gqlgen schema (SDL or introspection)
```

## Generation

`ix add entity --name product --frontend` reads the GraphQL schema and emits
list/show/edit/create routes + Houdini `.gql` documents for `product`. The
pages are plain Svelte you own; `lib/ix` removes the boilerplate without a
runtime abstraction in the request path.

## Auth

JWT in `Authorization: Bearer`, optional `X-Tenant` header for admins scoping a
request to a Casbin domain (tenant). The token store and route guard live in
`lib/ix/auth`.
