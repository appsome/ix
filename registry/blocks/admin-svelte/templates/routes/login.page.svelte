<script lang="ts">
  import { goto } from '$app/navigation';
  import { login } from '$lib/ix/auth';

  let email = '';
  let password = '';
  let error = '';
  let busy = false;

  async function submit() {
    error = '';
    busy = true;
    try {
      await login(email, password);
      await goto('/');
    } catch (e) {
      error = e instanceof Error ? e.message : 'login failed';
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex min-h-screen items-center justify-center bg-slate-50">
  <form class="w-80 rounded-lg border bg-white p-6 shadow-sm" on:submit|preventDefault={submit}>
    <h1 class="mb-4 text-lg font-semibold">Sign in</h1>
    {#if error}
      <p class="mb-3 rounded bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>
    {/if}
    <label class="block text-sm">Email
      <input class="mt-1 w-full rounded border px-2 py-1" type="email" bind:value={email} required />
    </label>
    <label class="mt-3 block text-sm">Password
      <input class="mt-1 w-full rounded border px-2 py-1" type="password" bind:value={password} required />
    </label>
    <button
      class="mt-4 w-full rounded bg-slate-900 px-3 py-2 text-sm text-white disabled:opacity-50"
      type="submit"
      disabled={busy}
    >
      {busy ? 'Signing in…' : 'Sign in'}
    </button>
  </form>
</div>
