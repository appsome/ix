/// <references types="houdini-svelte">

/** @type {import('houdini').ConfigFile} */
const config = {
  // Point Houdini at the gqlgen server's GraphQL endpoint. During dev the Vite
  // proxy forwards /query to the Go API on :8080; override via HOUDINI_API_URL.
  watchSchema: {
    url: process.env.HOUDINI_API_URL || 'http://localhost:8080/query'
  },
  plugins: {
    // static: the admin is an adapter-static SPA with no SvelteKit server, so
    // Houdini must not inject its root +layout.server.js (it would make every
    // page fetch /__data.json, which nothing answers in production — the Go
    // server's index.html fallback comes back and the app dies on JSON.parse).
    'houdini-svelte': { static: true }
  },
  scalars: {
    // gqlgen maps GraphQL ID to int64; treat it as a string client-side.
    ID: { type: 'string' }
  }
};

export default config;
