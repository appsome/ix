import { sveltekit } from '@sveltejs/kit/vite';
import houdini from 'houdini/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [houdini(), sveltekit()],
  server: {
    proxy: {
      // Proxy GraphQL and the admin JSON APIs (e.g. jobs-admin's /admin/jobs)
      // to the Go API during dev so the browser hits same-origin.
      '/query': 'http://localhost:8080',
      '/admin': 'http://localhost:8080'
    }
  }
});
