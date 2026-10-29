<script lang="ts">
	import { Folder } from '@lucide/svelte';
	import { createTable, FlexRender, tableFeatures, type ColumnDef } from '@tanstack/svelte-table';
	import { createWindowVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { formatBytes, formatDate } from '#lib/utils/format.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import { fileManagerEntryKey, type FileManagerEntry } from './types';

	const GRID_COLUMNS = '3rem minmax(220px, 2fr) minmax(160px, 1.2fr) minmax(90px, 0.75fr) minmax(150px, 1fr)';

	let {
		rows,
		selectedIds,
		selectedCount,
		loadedCount,
		onToggleSelection,
		onToggleAll,
		onOpenFolder,
		onShowContextMenu,
		onPreviewImage
	}: {
		rows: FileManagerEntry[];
		selectedIds: ReadonlySet<string>;
		selectedCount: number;
		loadedCount: number;
		onToggleSelection: (entry: FileManagerEntry) => void;
		onToggleAll: () => void;
		onOpenFolder: (id: string) => void;
		onShowContextMenu: (entry: FileManagerEntry, x: number, y: number) => void;
		onPreviewImage: (entry: FileManagerEntry, trigger: HTMLElement) => void;
	} = $props();
	let selectAllCheckbox = $state<HTMLInputElement>();
	let tableBody = $state<HTMLTableSectionElement>();
	const allSelected = $derived(loadedCount > 0 && selectedCount === loadedCount);

	$effect(() => {
		if (selectAllCheckbox) selectAllCheckbox.indeterminate = selectedCount > 0 && !allSelected;
	});

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
	const rowVirtualizer = createWindowVirtualizer({
		count: 0,
		estimateSize: () => 45,
		overscan: 8,
		getItemKey: (index) => tableRows[index]?.id ?? index
	});
	const virtualRows = $derived($rowVirtualizer.getVirtualItems());
	const rowScrollMargin = $derived($rowVirtualizer.options.scrollMargin);

	$effect(() => {
		const body = tableBody;
		const count = tableRows.length;
		if (!body || typeof window === 'undefined') return;

		const updateOptions = () => {
			get(rowVirtualizer).setOptions({
				count,
				scrollMargin: body.getBoundingClientRect().top + window.scrollY
			});
		};
		updateOptions();

		const resizeObserver = new ResizeObserver(updateOptions);
		const content = document.getElementById('main-content');
		if (content) resizeObserver.observe(content);
		window.addEventListener('resize', updateOptions);
		return () => {
			resizeObserver.disconnect();
			window.removeEventListener('resize', updateOptions);
		};
	});
</script>

<div class="overflow-x-auto">
	<table class="grid w-full min-w-[760px] border-collapse text-left">
		<thead class="grid bg-(--surface-highest) text-xs font-semibold tracking-wide text-(--muted)">
			{#each table.getHeaderGroups() as headerGroup (headerGroup.id)}
				<tr class="grid w-full" style={`grid-template-columns: ${GRID_COLUMNS}`}>
					<th class="flex items-center px-4 py-3 first:pl-5" scope="col">
						<input
							bind:this={selectAllCheckbox}
							class="size-4 cursor-pointer accent-(--accent)"
							type="checkbox"
							disabled={!rows.length}
							checked={allSelected}
							aria-label="Select all loaded entries"
							onchange={onToggleAll}
						/>
					</th>
					{#each headerGroup.headers as header (header.id)}
						<th class="flex items-center whitespace-nowrap px-4 py-3 first:pl-5 last:pr-5">
							{#if !header.isPlaceholder}<FlexRender {header} />{/if}
						</th>
					{/each}
				</tr>
			{/each}
		</thead>
		<tbody bind:this={tableBody} class="relative block divide-y divide-(--border-soft)" style={tableRows.length ? `height: ${$rowVirtualizer.getTotalSize()}px` : 'height: auto'}>
			{#if !tableRows.length}
				<tr class="grid w-full" style={`grid-template-columns: ${GRID_COLUMNS}`}><td class="col-span-5 px-5 py-10 text-center text-(--muted)">This folder is empty.</td></tr>
			{:else}
				{#each virtualRows as virtualRow (virtualRow.key)}
					{@const row = tableRows[virtualRow.index]}
					<tr
						data-index={virtualRow.index}
						class={`absolute top-0 left-0 grid h-[45px] w-full transition-colors hover:bg-(--surface-highest) ${selectedIds.has(fileManagerEntryKey(row.original)) ? 'bg-(--surface-highest)' : 'bg-(--surface-raised)'}`}
						style={`grid-template-columns: ${GRID_COLUMNS}; transform: translateY(${virtualRow.start - rowScrollMargin}px)`}
						oncontextmenu={(event) => {
							event.preventDefault();
							onShowContextMenu(row.original, event.clientX, event.clientY);
						}}
					>
						<td class="flex items-center px-4 py-3 first:pl-5">
							<input
								class="size-4 cursor-pointer accent-(--accent)"
								type="checkbox"
								checked={selectedIds.has(fileManagerEntryKey(row.original))}
								aria-label={`Select ${row.original.name}`}
								onchange={() => onToggleSelection(row.original)}
							/>
						</td>
						{#each row.getAllCells() as cell (cell.id)}
							<td class="flex min-w-0 items-center whitespace-nowrap px-4 py-3 first:pl-5 last:pr-5">
								{#if cell.column.id === 'name' && row.original.kind === 'folder'}
									<button class="inline-flex min-w-0 max-w-full cursor-pointer items-center gap-2 text-left font-medium text-(--text) hover:text-(--accent)" type="button" onclick={() => onOpenFolder(row.original.id)}>
										<Folder class="size-4 shrink-0 text-(--accent)" aria-hidden="true" />
										<span class="min-w-0 truncate" title={row.original.name}><FlexRender {cell} /></span>
									</button>
								{:else if cell.column.id === 'name' && row.original.type.startsWith('image/')}
									<button class="inline-flex min-w-0 max-w-full cursor-zoom-in items-center gap-2 text-left font-medium" type="button" onclick={(event) => onPreviewImage(row.original, event.currentTarget)}>
										<FileTypeIcon name={row.original.name} mime={row.original.type} className="size-4 shrink-0" />
										<span class="min-w-0 truncate" title={row.original.name}><FlexRender {cell} /></span>
									</button>
								{:else if cell.column.id === 'name'}
									<span class="inline-flex min-w-0 max-w-full items-center gap-2 font-medium">
										<FileTypeIcon name={row.original.name} mime={row.original.type} className="size-4 shrink-0" />
										<span class="min-w-0 truncate" title={row.original.name}><FlexRender {cell} /></span>
									</span>
								{:else if cell.column.id === 'size'}
									<span class="text-(--muted)">{formatBytes(cell.getValue() as number | null)}</span>
								{:else if cell.column.id === 'updatedAt'}
									<span class="text-(--muted)">{formatDate(cell.getValue() as string)}</span>
								{:else}
									<span class="block min-w-0 truncate font-mono text-xs text-(--muted)" title={cell.getValue() as string}>{cell.getValue() as string}</span>
								{/if}
							</td>
						{/each}
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>
</div>
