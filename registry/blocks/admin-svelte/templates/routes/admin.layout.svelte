<script lang="ts">
  import { goto } from '$app/navigation';
  import { tenant, setTenant, logout } from '$lib/ix/auth';

  let tenantInput = $tenant ?? '';

  function applyTenant() {
    setTenant(tenantInput.trim() || null);
  }

  function signOut() {
    logout();
    goto('/login');
  }
</script>

<div class="flex min-h-screen">
  <aside class="w-56 shrink-0 border-r bg-slate-50 p-4">
    <h1 class="mb-4 text-lg font-semibold">Admin</h1>
    <nav class="flex flex-col gap-1 text-sm">
      <a class="rounded px-2 py-1 hover:bg-slate-200" href="/">Dashboard</a>
      <!-- Per-entity links land here as you run `ix add entity --frontend`. -->
    </nav>

    <div class="mt-6 border-t pt-4">
      <label class="block text-xs text-slate-500" for="tenant">Tenant (X-Tenant)</label>
      <input
        id="tenant"
        class="mt-1 w-full rounded border px-2 py-1 text-sm"
        bind:value={tenantInput}
        on:change={applyTenant}
        placeholder="(all)"
      />
      <button class="mt-3 text-sm text-red-600 hover:underline" on:click={signOut}>Sign out</button>
    </div>
  </aside>

  <main class="flex-1 p-6">
    <slot />
  </main>
</div>
