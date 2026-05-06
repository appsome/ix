<!--
  Generic, dependency-light table for list pages. Pass columns + rows; swap for
  shadcn-svelte's <Table> components once you `npx shadcn-svelte add table`.
-->
<script lang="ts">
  type Column = { key: string; label: string; href?: (row: any) => string };
  export let columns: Column[] = [];
  export let rows: any[] = [];
  export let loading = false;
</script>

{#if loading}
  <p class="p-4 text-slate-500">Loading…</p>
{:else if rows.length === 0}
  <p class="p-4 text-slate-500">No records.</p>
{:else}
  <table class="w-full border-collapse text-sm">
    <thead>
      <tr class="border-b text-left">
        {#each columns as col}
          <th class="px-3 py-2 font-medium text-slate-600">{col.label}</th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each rows as row}
        <tr class="border-b hover:bg-slate-50">
          {#each columns as col}
            <td class="px-3 py-2">
              {#if col.href}
                <a class="text-blue-600 hover:underline" href={col.href(row)}>{row[col.key]}</a>
              {:else}
                {row[col.key]}
              {/if}
            </td>
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>
{/if}
