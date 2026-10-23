<script lang="ts">
	import { File as FileIcon, Folder } from '@lucide/svelte';
	import { createTable, FlexRender, tableFeatures, type ColumnDef } from '@tanstack/svelte-table';
	import { formatBytes, formatDate } from '#lib/utils/format.ts';
	import type { FileManagerEntry } from './types';

	let { rows, onOpenFolder }: { rows: FileManagerEntry[]; onOpenFolder: (id: string) => void } = $props();

	const features = tableFeatures({});
	const columns: ColumnDef<typeof features, FileManagerEntry>[] = [
		{ accessorKey: 'name', header: 'Name' },
		{ accessorKey: 'type', header: 'Type' },
		{ accessorKey: 'size', header: 'Size' },
		{ accessorKey: 'updatedAt', header: 'Modified' }
	];
	const table = createTable({
		features,
		columns,
		get data() {
			return rows;
		},
		getRowId: (row) => `${row.kind}:${row.id}`
	});
	const tableRows = $derived(table.getRowModel().rows);
</script>

<div class="overflow-x-auto">
	<table class="w-full border-collapse text-left">
		<thead class="bg-(--surface-highest) text-xs font-semibold tracking-wide text-(--muted)">
			{#each table.getHeaderGroups() as headerGroup (headerGroup.id)}
				<tr>
					{#each headerGroup.headers as header (header.id)}
						<th class="whitespace-nowrap px-4 py-3 first:pl-5 last:pr-5">
							{#if !header.isPlaceholder}<FlexRender {header} />{/if}
						</th>
					{/each}
				</tr>
			{/each}
		</thead>
		<tbody class="divide-y divide-(--border-soft)">
			{#each tableRows as row (row.id)}
				<tr class="transition-colors hover:bg-(--surface-highest)">
					{#each row.getAllCells() as cell (cell.id)}
						<td class="whitespace-nowrap px-4 py-3 first:pl-5 last:pr-5">
							{#if cell.column.id === 'name' && row.original.kind === 'folder'}
								<button class="inline-flex cursor-pointer items-center gap-2 text-left font-medium text-(--text) hover:text-(--accent)" type="button" onclick={() => onOpenFolder(row.original.id)}>
									<Folder class="size-4 shrink-0 text-(--accent)" aria-hidden="true" />
									<FlexRender {cell} />
								</button>
							{:else if cell.column.id === 'name'}
								<span class="inline-flex items-center gap-2 font-medium">
									<FileIcon class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
									<FlexRender {cell} />
								</span>
							{:else if cell.column.id === 'size'}
								<span class="text-(--muted)">{formatBytes(cell.getValue() as number | null)}</span>
							{:else if cell.column.id === 'updatedAt'}
								<span class="text-(--muted)">{formatDate(cell.getValue() as string)}</span>
							{:else}
								<span class="font-mono text-xs text-(--muted)">{cell.getValue() as string}</span>
							{/if}
						</td>
					{/each}
				</tr>
			{:else}
				<tr><td class="px-5 py-10 text-center text-(--muted)" colspan="4">This folder is empty.</td></tr>
			{/each}
		</tbody>
	</table>
</div>
