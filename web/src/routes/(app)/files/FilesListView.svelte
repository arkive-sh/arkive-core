<script lang="ts">
	import { tick } from 'svelte';
	import { Folder, Share2 } from '@lucide/svelte';
	import { createTable, FlexRender, tableFeatures, type ColumnDef } from '@tanstack/svelte-table';
	import { createWindowVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { formatBytes, formatDate } from '#lib/utils/format.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import InlineNewFolder from './InlineNewFolder.svelte';
	import { isImageFile } from '#lib/utils/image.ts';
	import { isMediaMimeType } from '#lib/utils/media.ts';
	import {
		fileManagerEntryKey,
		isPdfFile,
		isTextFile,
		NEW_FOLDER_DRAFT_ID,
		type FileManagerEntry,
	} from './types';

	const ENTRY_COLUMNS =
		'minmax(220px, 2fr) minmax(160px, 1.2fr) minmax(90px, 0.75fr) minmax(150px, 1fr)';

	let {
		rows,
		focusFileId,
		selectedIds,
		selectionEnabled = true,
		previewEnabled = true,
		dateColumnLabel = 'Modified',
		selectedCount,
		loadedCount,
		showDimensions,
		newFolderName = $bindable(''),
		creatingFolder,
		onCreateFolder,
		onCancelNewFolder,
		onToggleSelection,
		onSelectEntry,
		onToggleAll,
		onOpenFolder,
		onOpenShares,
		onShowContextMenu,
		onPreviewImage,
		onPreviewMedia,
		onPreviewPdf,
		onPreviewText,
	}: {
		rows: FileManagerEntry[];
		focusFileId: string | null;
		selectedIds: ReadonlySet<string>;
		selectionEnabled?: boolean;
		previewEnabled?: boolean;
		dateColumnLabel?: string;
		selectedCount: number;
		loadedCount: number;
		showDimensions: boolean;
		newFolderName?: string;
		creatingFolder: boolean;
		onCreateFolder?: () => void | Promise<void>;
		onCancelNewFolder?: () => void;
		onToggleSelection?: (entry: FileManagerEntry) => void;
		onSelectEntry?: (entry: FileManagerEntry, additive: boolean) => void;
		onToggleAll?: () => void;
		onOpenFolder: (id: string) => void;
		onOpenShares?: (id: string) => void;
		onShowContextMenu: (entry: FileManagerEntry | null, x: number, y: number) => void;
		onPreviewImage: (entry: FileManagerEntry, trigger: HTMLElement) => void;
		onPreviewMedia: (entry: FileManagerEntry) => void;
		onPreviewPdf: (entry: FileManagerEntry) => void;
		onPreviewText: (entry: FileManagerEntry) => void;
	} = $props();
	const canSelect = $derived(selectionEnabled && Boolean(onToggleSelection));
	const activeGridColumns = $derived(
		`${canSelect ? '3rem' : '0px'} ${ENTRY_COLUMNS}${showDimensions ? ' minmax(120px, 0.8fr)' : ''}`,
	);
	let selectAllCheckbox = $state<HTMLInputElement>();
	let tableBody = $state<HTMLTableSectionElement>();
	let pendingFocusIndex = $state<number | null>(null);
	const allSelected = $derived(loadedCount > 0 && selectedCount === loadedCount);

	$effect(() => {
		if (selectAllCheckbox) selectAllCheckbox.indeterminate = selectedCount > 0 && !allSelected;
	});

	const features = tableFeatures({});
	const columns: ColumnDef<typeof features, FileManagerEntry>[] = [
		{ accessorKey: 'name', header: 'Name' },
		{ accessorKey: 'type', header: 'Type' },
		{ accessorKey: 'size', header: 'Size' },
		{ accessorKey: 'updatedAt', header: () => dateColumnLabel },
		{
			id: 'dimensions',
			accessorFn: (entry) =>
				entry.dimensions ? `${entry.dimensions.width} × ${entry.dimensions.height}` : '—',
			header: 'Resolution',
		},
	];
	const table = createTable({
		features,
		columns,
		get data() {
			return rows;
		},
		getRowId: (row) => `${row.kind}:${row.id}`,
	});
	const tableRows = $derived(table.getRowModel().rows);
	const rowVirtualizer = createWindowVirtualizer({
		count: 0,
		estimateSize: () => 45,
		overscan: 8,
		getItemKey: (index) => tableRows[index]?.id ?? index,
	});
	const virtualRows = $derived($rowVirtualizer.getVirtualItems());
	const rowScrollMargin = $derived($rowVirtualizer.options.scrollMargin);

	$effect(() => {
		const index = pendingFocusIndex;

		if (index === null || !virtualRows.some((row) => row.index === index)) return;
		const button = tableBody?.querySelector<HTMLButtonElement>(
			`[data-entry-action][data-entry-index="${index}"]`,
		);

		if (!button) return;
		pendingFocusIndex = null;
		button.focus({ preventScroll: true });
	});

	$effect(() => {
		const body = tableBody;
		const count = tableRows.length;

		if (!body || typeof window === 'undefined') return;

		const updateOptions = () => {
			get(rowVirtualizer).setOptions({
				count,
				scrollMargin: body.getBoundingClientRect().top + window.scrollY,
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

	$effect(() => {
		const targetId = focusFileId;
		const index = tableRows.findIndex(
			(row) => row.original.kind === 'file' && row.original.id === targetId,
		);

		if (!targetId || index < 0) return;

		let canceled = false;
		void tick().then(() => {
			if (!canceled) get(rowVirtualizer).scrollToIndex(index, { align: 'center' });
		});

		return () => {
			canceled = true;
		};
	});

	function entryActionLabel(entry: FileManagerEntry): string | undefined {
		if (entry.kind === 'folder') return `Open folder ${entry.name}`;
		if (!previewEnabled) return;
		if (isImageFile(entry.name, entry.type) || isPdfFile(entry)) return `Preview ${entry.name}`;
		if (isMediaMimeType(entry.type)) return `Play ${entry.name}`;
		if (isTextFile(entry)) return `Open ${entry.name}`;
	}

	function openEntry(entry: FileManagerEntry, trigger: HTMLElement): void {
		if (entry.kind === 'folder') onOpenFolder(entry.id);
		else if (!previewEnabled) return;
		else if (isImageFile(entry.name, entry.type)) onPreviewImage(entry, trigger);
		else if (isMediaMimeType(entry.type)) onPreviewMedia(entry);
		else if (isPdfFile(entry)) onPreviewPdf(entry);
		else if (isTextFile(entry)) onPreviewText(entry);
	}

	function focusEntry(index: number): void {
		if (index < 0 || index >= tableRows.length) return;
		pendingFocusIndex = index;
		get(rowVirtualizer).scrollToIndex(index, { align: 'auto' });
	}

	function handleEntryKeydown(event: KeyboardEvent): void {
		if (event.defaultPrevented || !(event.target instanceof HTMLElement)) return;
		if (event.target.closest('[aria-modal="true"], [role="menu"]')) return;
		if (
			event.target.closest(
				'input:not([type="checkbox"]), textarea, select, [contenteditable="true"]',
			)
		)
			return;
		const row = event.target.closest<HTMLElement>('[data-entry-index]');
		const navigableIndexes = tableRows.flatMap((item, rowIndex) =>
			item.original.id === NEW_FOLDER_DRAFT_ID ? [] : [rowIndex],
		);

		if (!row) {
			if (!['ArrowDown', 'ArrowUp'].includes(event.key) || event.altKey) return;
			const selected = navigableIndexes.find((index) =>
				selectedIds.has(fileManagerEntryKey(tableRows[index].original)),
			);
			const first = selected ?? navigableIndexes[0];

			if (first !== undefined) {
				event.preventDefault();
				focusEntry(first);
			}

			return;
		}
		const index = Number(row.dataset.entryIndex);

		if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
			if (event.altKey) return;
			event.preventDefault();
			const direction = event.key === 'ArrowDown' ? 1 : -1;
			const current = navigableIndexes.indexOf(index);
			const next =
				navigableIndexes[
					current < 0 ? (direction > 0 ? 0 : navigableIndexes.length - 1) : current + direction
				];

			if (next !== undefined) focusEntry(next);
			else if (direction < 0) window.scrollTo({ top: 0, behavior: 'smooth' });
		} else if (
			(event.key === 'Enter' || event.code === 'Space') &&
			event.target.closest('[data-entry-action]')
		) {
			const entry = tableRows[index]?.original;

			if (!entry) return;
			event.preventDefault();
			if (event.code === 'Space') {
				onSelectEntry?.(entry, event.ctrlKey || event.metaKey);

				return;
			}
			const action = event.target.closest<HTMLElement>('[data-entry-action]')!;

			if (entryActionLabel(entry)) openEntry(entry, action);
			else onSelectEntry?.(entry, event.ctrlKey || event.metaKey);
		}
	}
</script>

<svelte:window onkeydown={handleEntryKeydown} />

<div class="overflow-x-auto">
	<table
		class="grid w-full border-collapse text-left"
		style={`min-width: ${showDimensions ? '940px' : '800px'}`}
	>
		<thead
			class="grid border-b border-(--border) bg-transparent text-xs font-semibold tracking-wide text-(--muted)"
		>
			{#each table.getHeaderGroups() as headerGroup (headerGroup.id)}
				<tr class="grid w-full" style={`grid-template-columns: ${activeGridColumns}`}>
					<th class={canSelect ? 'flex items-center px-4 py-3 first:pl-5' : 'w-0 p-0'} scope="col">
						{#if canSelect && onToggleAll}<input
								bind:this={selectAllCheckbox}
								class="size-4 cursor-pointer accent-(--accent)"
								type="checkbox"
								disabled={!loadedCount}
								checked={allSelected}
								aria-label="Select all loaded entries"
								onchange={() => onToggleAll?.()}
							/>{/if}
					</th>
					{#each headerGroup.headers as header (header.id)}
						{#if header.id !== 'dimensions' || showDimensions}
							<th class="flex items-center whitespace-nowrap px-4 py-3 first:pl-5 last:pr-5">
								{#if !header.isPlaceholder}<FlexRender {header} />{/if}
							</th>
						{/if}
					{/each}
				</tr>
			{/each}
		</thead>
		<tbody
			bind:this={tableBody}
			class="relative block divide-y divide-(--border-soft)"
			style={tableRows.length ? `height: ${$rowVirtualizer.getTotalSize()}px` : 'height: auto'}
		>
			{#if !tableRows.length}
				<tr class="grid w-full" style={`grid-template-columns: ${activeGridColumns}`}
					><td
						class="px-5 py-10 text-center text-(--muted)"
						style={`grid-column: 1 / ${showDimensions ? 7 : 6}`}>This folder is empty.</td
					></tr
				>
			{:else}
				{#each virtualRows as virtualRow (virtualRow.key)}
					{@const row = tableRows[virtualRow.index]}
					{@const isDraftFolder = row.original.id === NEW_FOLDER_DRAFT_ID}
					{@const actionLabel = isDraftFolder ? undefined : entryActionLabel(row.original)}
					<tr
						data-index={virtualRow.index}
						data-entry-index={virtualRow.index}
						data-file-entry
						class={`absolute top-0 left-0 grid h-11.25 w-full cursor-pointer transition-colors hover:bg-(--surface-highest) ${selectedIds.has(fileManagerEntryKey(row.original)) ? 'bg-(--surface-highest)' : 'bg-transparent'} ${focusFileId === row.original.id ? 'ring-2 ring-inset ring-(--accent)' : ''}`}
						style={`grid-template-columns: ${activeGridColumns}; transform: translateY(${virtualRow.start - rowScrollMargin}px)`}
						oncontextmenu={isDraftFolder
							? undefined
							: (event) => {
									event.preventDefault();
									onShowContextMenu(row.original, event.clientX, event.clientY);
								}}
					>
						<td class={canSelect ? 'flex items-center px-4 py-1.5 first:pl-5' : 'w-0 p-0'}>
							{#if !isDraftFolder}<button
									class="absolute inset-0 z-0 m-0 cursor-pointer appearance-none border-0 bg-transparent p-0 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-(--accent)"
									type="button"
									data-entry-action
									data-entry-index={virtualRow.index}
									aria-label={canSelect && row.original.selectable !== false
										? `Select ${row.original.name}${actionLabel ? '; press Enter or double-click to open' : ''}`
										: (actionLabel ?? row.original.name)}
									title={actionLabel ? 'Click to select; double-click to open' : undefined}
									onclick={(event) => onSelectEntry?.(row.original, event.ctrlKey || event.metaKey)}
									ondblclick={(event) => openEntry(row.original, event.currentTarget)}
								></button>{/if}
							{#if canSelect && !isDraftFolder && row.original.selectable !== false}<input
									class="relative z-10 size-4 cursor-pointer accent-(--accent)"
									type="checkbox"
									checked={selectedIds.has(fileManagerEntryKey(row.original))}
									aria-label={`Select ${row.original.name}`}
									onchange={() => onToggleSelection?.(row.original)}
								/>{/if}
						</td>
						{#each row.getAllCells() as cell (cell.id)}
							{#if cell.column.id !== 'dimensions' || showDimensions}
								<td
									class="flex min-w-0 items-center whitespace-nowrap px-4 py-1.5 first:pl-5 last:pr-5"
								>
									{#if isDraftFolder && cell.column.id === 'name' && onCreateFolder && onCancelNewFolder}
										<InlineNewFolder
											variant="list"
											bind:name={newFolderName}
											busy={creatingFolder}
											onCreate={onCreateFolder}
											onCancel={onCancelNewFolder}
										/>
									{:else if isDraftFolder}
										<span></span>
									{:else if cell.column.id === 'name' && row.original.kind === 'folder'}
										<span
											class="inline-flex min-w-0 max-w-full items-center gap-2 text-left font-medium text-(--text)"
										>
											<Folder class="size-4 shrink-0 text-(--accent)" aria-hidden="true" />
											<span class="min-w-0 truncate" title={row.original.name}
												><FlexRender {cell} /></span
											>
										</span>
									{:else if cell.column.id === 'name'}
										<span class="inline-flex min-w-0 max-w-full items-center gap-2 font-medium">
											<FileTypeIcon
												name={row.original.name}
												mime={row.original.type}
												className="size-4 shrink-0"
											/>
											<span class="min-w-0 truncate" title={row.original.name}
												><FlexRender {cell} /></span
											>
										</span>
									{:else if cell.column.id === 'size'}
										<span class="text-(--muted)"
											>{formatBytes(cell.getValue() as number | null)}</span
										>
									{:else if cell.column.id === 'updatedAt'}
										<span class="text-(--muted)"
											>{row.original.isTrashContainer
												? 'Contains deleted items'
												: formatDate(cell.getValue() as string)}</span
										>
									{:else if cell.column.id === 'dimensions'}
										<span class="text-(--muted)">{cell.getValue() as string}</span>
									{:else}
										<span
											class="block min-w-0 truncate font-mono text-xs text-(--muted)"
											title={cell.getValue() as string}>{cell.getValue() as string}</span
										>
									{/if}
									{#if cell.column.id === 'name' && row.original.isShared}
										<button
											class="relative z-10 ml-2 grid size-7 shrink-0 cursor-pointer place-items-center rounded text-(--muted) hover:bg-(--surface) hover:text-(--text)"
											type="button"
											title="Open this file’s share"
											aria-label={`Open share for ${row.original.name}`}
											onclick={() => onOpenShares?.(row.original.id)}
										>
											<Share2 class="size-4" aria-hidden="true" />
										</button>
									{/if}
								</td>
							{/if}
						{/each}
					</tr>
				{/each}
			{/if}
		</tbody>
	</table>
</div>
