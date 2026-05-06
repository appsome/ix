/// <references types="houdini-svelte">

/** @type {import('houdini').ConfigFile} */
const config = {
  // Point Houdini at the gqlgen server's GraphQL endpoint. During dev the Vite
  // proxy forwards /query to the Go API on :8080; override via HOUDINI_API_URL.
  watchSchema: {
    url: process.env.HOUDINI_API_URL || 'http://localhost:8080/query'
  },
  plugins: {
    'houdini-svelte': {}
  },
  scalars: {
    // gqlgen maps GraphQL ID to int64; treat it as a string client-side.
    ID: { type: 'string' }
  }
};

export default config;
