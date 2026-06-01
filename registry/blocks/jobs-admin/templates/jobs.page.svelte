<!--
  Background jobs dashboard for the admin shell (jobs-admin block).
  Plain SvelteKit you own. Reads the jobs admin JSON API mounted by the Go side
  at /admin/jobs (queues + tasks-by-state, with run/archive/delete actions),
  authed with the same Bearer/X-Tenant headers as the rest of the admin.

  The API base is relative; in dev, vite proxies /admin to the API on :8080
  (see vite.config.ts). Adjust JOBS_API if you mount it elsewhere.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getToken, getTenant } from '$lib/ix/auth';

  const JOBS_API = '/admin/jobs';
  const STATES = ['active', 'pending', 'scheduled', 'retry', 'archived', 'completed'] as const;
  type State = (typeof STATES)[number];

  type QueueStats = {
    queue: string;
    size: number;
    active: number;
    pending: number;
    scheduled: number;
    retry: number;
    archived: number;
    completed: number;
    processed: number;
    failed: number;
    paused: boolean;
  };
  type Task = {
    id: string;
    queue: string;
    type: string;
    state: State;
    maxRetry: number;
    retried: number;
    lastErr: string;
    nextProcessAt?: string;
  };

  let queues: QueueStats[] = [];
  let selectedQueue = '';
  let state: State = 'pending';
  let tasks: Task[] = [];
  let error = '';
  let loading = true;

  function headers(): Record<string, string> {
    const h: Record<string, string> = { 'content-type': 'application/json' };
    const t = getToken();
    if (t) h['Authorization'] = `Bearer ${t}`;
    const tenant = getTenant();
    if (tenant) h['X-Tenant'] = tenant;
    return h;
  }

  async function api(path: string, method = 'GET'): Promise<Response> {
    return fetch(`${JOBS_API}${path}`, { method, headers: headers() });
  }

  async function loadQueues() {
    error = '';
    try {
      const res = await api('/queues');
      if (!res.ok) throw new Error(`queues: ${res.status}`);
      queues = await res.json();
      if (!selectedQueue && queues.length) selectedQueue = queues[0].queue;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function loadTasks() {
    if (!selectedQueue) return;
    error = '';
    try {
      const res = await api(`/queues/${encodeURIComponent(selectedQueue)}/tasks?state=${state}`);
      if (!res.ok) throw new Error(`tasks: ${res.status}`);
      tasks = (await res.json()) ?? [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function act(task: Task, verb: 'run' | 'archive' | 'delete') {
    const base = `/queues/${encodeURIComponent(task.queue)}/tasks/${encodeURIComponent(task.id)}`;
    const [path, method] = verb === 'delete' ? [base, 'DELETE'] : [`${base}/${verb}`, 'POST'];
    const res = await api(path, method);
    if (!res.ok && res.status !== 204) {
      error = `${verb} failed: ${res.status}`;
      return;
    }
    await Promise.all([loadQueues(), loadTasks()]);
  }

  async function selectQueue(q: string) {
    selectedQueue = q;
    await loadTasks();
  }

  async function selectState(s: State) {
    state = s;
    await loadTasks();
  }

  onMount(async () => {
    await loadQueues();
    await loadTasks();
    loading = false;
  });
</script>

<h1 class="mb-4 text-xl font-semibold">Background jobs</h1>

{#if error}
  <p class="mb-4 rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p>
{/if}

{#if loading}
  <p>Loading…</p>
{:else}
  <section class="mb-6">
    <h2 class="mb-2 text-sm font-medium text-slate-500">Queues</h2>
    <div class="overflow-x-auto rounded border">
      <table class="w-full text-sm">
        <thead class="bg-slate-50 text-left">
          <tr>
            <th class="px-3 py-2">Queue</th>
            <th class="px-3 py-2">Active</th>
            <th class="px-3 py-2">Pending</th>
            <th class="px-3 py-2">Scheduled</th>
            <th class="px-3 py-2">Retry</th>
            <th class="px-3 py-2">Archived</th>
            <th class="px-3 py-2">Failed</th>
            <th class="px-3 py-2"></th>
          </tr>
        </thead>
        <tbody>
          {#each queues as q}
            <tr class="border-t {q.queue === selectedQueue ? 'bg-slate-100' : ''}">
              <td class="px-3 py-2 font-medium">
                {q.queue}{#if q.paused}<span class="ml-1 text-xs text-amber-600">(paused)</span>{/if}
              </td>
              <td class="px-3 py-2">{q.active}</td>
              <td class="px-3 py-2">{q.pending}</td>
              <td class="px-3 py-2">{q.scheduled}</td>
              <td class="px-3 py-2">{q.retry}</td>
              <td class="px-3 py-2">{q.archived}</td>
              <td class="px-3 py-2">{q.failed}</td>
              <td class="px-3 py-2">
                <button class="text-blue-600 hover:underline" on:click={() => selectQueue(q.queue)}>view</button>
              </td>
            </tr>
          {/each}
          {#if !queues.length}
            <tr><td class="px-3 py-2 text-slate-500" colspan="8">No queues yet — enqueue a task.</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  </section>

  {#if selectedQueue}
    <section>
      <div class="mb-2 flex items-center gap-2">
        <h2 class="text-sm font-medium text-slate-500">Tasks in <span class="font-mono">{selectedQueue}</span></h2>
        <div class="ml-auto flex gap-1">
          {#each STATES as s}
            <button
              class="rounded px-2 py-1 text-xs {s === state ? 'bg-slate-800 text-white' : 'bg-slate-100'}"
              on:click={() => selectState(s)}>{s}</button>
          {/each}
        </div>
      </div>

      <div class="overflow-x-auto rounded border">
        <table class="w-full text-sm">
          <thead class="bg-slate-50 text-left">
            <tr>
              <th class="px-3 py-2">ID</th>
              <th class="px-3 py-2">Type</th>
              <th class="px-3 py-2">Retries</th>
              <th class="px-3 py-2">Last error</th>
              <th class="px-3 py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each tasks as task}
              <tr class="border-t">
                <td class="px-3 py-2 font-mono text-xs">{task.id}</td>
                <td class="px-3 py-2">{task.type}</td>
                <td class="px-3 py-2">{task.retried}/{task.maxRetry}</td>
                <td class="px-3 py-2 text-red-600">{task.lastErr}</td>
                <td class="px-3 py-2">
                  <div class="flex gap-2">
                    {#if state === 'scheduled' || state === 'retry' || state === 'archived'}
                      <button class="text-blue-600 hover:underline" on:click={() => act(task, 'run')}>run</button>
                    {/if}
                    {#if state !== 'archived'}
                      <button class="text-amber-600 hover:underline" on:click={() => act(task, 'archive')}>archive</button>
                    {/if}
                    <button class="text-red-600 hover:underline" on:click={() => act(task, 'delete')}>delete</button>
                  </div>
                </td>
              </tr>
            {/each}
            {#if !tasks.length}
              <tr><td class="px-3 py-2 text-slate-500" colspan="5">No {state} tasks.</td></tr>
            {/if}
          </tbody>
        </table>
      </div>
    </section>
  {/if}
{/if}
