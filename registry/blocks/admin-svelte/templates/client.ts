import { HoudiniClient } from '$houdini';
import { getToken, getTenant } from '$lib/ix/auth';

// The Houdini client for the gqlgen API. Every request carries the JWT as a
// Bearer token and, when an admin has scoped to a tenant, the X-Tenant header
// the runtime/middleware authz layer reads.
export default new HoudiniClient({
  url: '/query',
  fetchParams() {
    const headers: Record<string, string> = {};
    const token = getToken();
    if (token) headers['Authorization'] = `Bearer ${token}`;
    const tenant = getTenant();
    if (tenant) headers['X-Tenant'] = tenant;
    return { headers };
  }
});
