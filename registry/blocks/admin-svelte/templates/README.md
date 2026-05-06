# Admin (SvelteKit + shadcn-svelte + Houdini)

Vendored by `ix add admin-svelte`. You own every file here.

```sh
npm install
npm run dev          # http://localhost:5173, proxying /query → API on :8080
```

- **GraphQL** via [Houdini](https://houdinigraphql.com) — `houdini.config.js`
  points at the gqlgen endpoint; run `npm run dev` to (re)generate the typed
  document stores under `$houdini`.
- **UI** via [shadcn-svelte](https://shadcn-svelte.com) — `components.json` is
  configured; add components with `npx shadcn-svelte@latest add button table`.
- **Shared lib** in `src/lib/ix/`: `auth.ts` (token + tenant stores, login),
  `utils.ts` (`cn`), `DataTable.svelte`. The auth flow sends
  `Authorization: Bearer <jwt>` and an optional `X-Tenant` header (see
  `src/client.ts`).
- **Per-entity pages** come from `ix add entity --name <x> --frontend`, landing
  under `src/routes/(admin)/<plural>/`.
