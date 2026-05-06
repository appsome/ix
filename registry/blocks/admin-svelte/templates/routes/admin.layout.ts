import { redirect } from '@sveltejs/kit';
import { getToken } from '$lib/ix/auth';

// Guard the (admin) group: no token ⇒ bounce to /login.
export function load() {
  if (!getToken()) {
    throw redirect(302, '/login');
  }
}
