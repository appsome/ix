import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    // SPA build: every page is CSR-rendered (ssr=false in the root layout, the
    // JWT lives in localStorage). The Go api serves build/ with an index.html
    // fallback for client-side routes.
    adapter: adapter({ fallback: 'index.html' }),
    alias: {
      $houdini: './$houdini'
    }
  }
};

export default config;
