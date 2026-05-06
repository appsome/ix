import type { Config } from 'tailwindcss';

// Minimal config compatible with shadcn-svelte. Running
// `npx shadcn-svelte@latest init` will extend this with the design tokens.
export default {
  darkMode: ['class'],
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {}
  },
  plugins: []
} satisfies Config;
