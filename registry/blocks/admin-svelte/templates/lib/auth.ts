// Minimal client-side auth for the admin: a JWT + an optional tenant scope,
// both persisted in localStorage. The admin runs CSR-only (see +layout.ts), so
// reading localStorage at module scope is safe.
import { writable } from 'svelte/store';
import { browser } from '$app/environment';

const TOKEN_KEY = 'ix.token';
const TENANT_KEY = 'ix.tenant';

function read(key: string): string | null {
  return browser ? localStorage.getItem(key) : null;
}

export const token = writable<string | null>(read(TOKEN_KEY));
export const tenant = writable<string | null>(read(TENANT_KEY));

export function getToken(): string | null {
  return read(TOKEN_KEY);
}
export function getTenant(): string | null {
  return read(TENANT_KEY);
}

export function setToken(value: string | null) {
  token.set(value);
  if (!browser) return;
  if (value) localStorage.setItem(TOKEN_KEY, value);
  else localStorage.removeItem(TOKEN_KEY);
}

export function setTenant(value: string | null) {
  tenant.set(value);
  if (!browser) return;
  if (value) localStorage.setItem(TENANT_KEY, value);
  else localStorage.removeItem(TENANT_KEY);
}

export function logout() {
  setToken(null);
  setTenant(null);
}

// login posts to the gqlgen `login` mutation (provided by the auth-jwt block)
// and stores the returned access token. Adjust the field names to your schema.
export async function login(email: string, password: string): Promise<void> {
  const res = await fetch('/query', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      query: `mutation($e: String!, $p: String!) { login(email: $e, password: $p) { accessToken } }`,
      variables: { e: email, p: password }
    })
  });
  const json = await res.json();
  const t = json?.data?.login?.accessToken;
  if (!t) throw new Error(json?.errors?.[0]?.message ?? 'login failed');
  setToken(t);
}
