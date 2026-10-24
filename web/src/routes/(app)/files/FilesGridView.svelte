<script lang="ts">
	import { onDestroy } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { File as FileIcon, Folder } from '@lucide/svelte';
	import { createWindowVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { api } from '#lib/api.ts';
	import { getCachedThumbnail, cacheThumbnail } from '#lib/thumbnail-cache.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import { decryptSearchThumbnail } from '#lib/vault/search.ts';
	import { fileManagerEntryKey, type FileManagerEntry } from './types';

	const THUMBNAIL_CONCURRENCY = 4;
	let {
		rows,
		selectedIds,
		onToggleSelection,
		onOpenFolder
	}: {
		rows: FileManagerEntry[];
		selectedIds: ReadonlySet<string>;
		onToggleSelection: (entry: FileManagerEntry) => void;
		onOpenFolder: (id: string) => void;
	} = $props();
	let gridLayout = $state<HTMLDivElement>();
	let gridContent = $state<HTMLDivElement>();
	let columnCount = $state(1);
	const thumbnails = new SvelteMap<string, string>();
	const objectUrls = new Map<string, string>();
	const pendingIds = new Set<string>();
	const attemptedIds = new Set<string>();
	const controller = new AbortController();
	let visibleIds = new Set<string>();
	let queue: FileManagerEntry[] = [];
	let activeLoads = 0;
	const rowVirtualizer = createWindowVirtualizer({
		count: 0,
		estimateSize: () => {
			const minCardWidth = typeof window !== 'undefined' && window.innerWidth <= 640 ? 145 : 190;
			const contentWidth = Math.max(minCardWidth, (gridLayout?.clientWidth ?? 0) - 32);
			const cardWidth = (contentWidth - 12 * (columnCount - 1)) / columnCount;
			return cardWidth * 0.75 + 88;
		},
		gap: 12,
		overscan: 2,
		getItemKey: (index) => {
			const entry = rows[index * columnCount];
			return entry ? fileManagerEntryKey(entry) : index;
		}
	});
	const virtualRows = $derived($rowVirtualizer.getVirtualItems());
	const totalGridSize = $derived($rowVirtualizer.getTotalSize());
	const rowScrollMargin = $derived($rowVirtualizer.options.scrollMargin);
	const renderedEntries = $derived(virtualRows.flatMap(({ index }) => rows.slice(index * columnCount, (index + 1) * columnCount)));

	$effect(() => {
		const layout = gridLayout;
		const content = gridContent;
		const count = rows.length;
		if (!layout || !content || typeof window === 'undefined') return;

		const updateOptions = () => {
			const minCardWidth = window.innerWidth <= 640 ? 145 : 190;
			const columns = Math.max(1, Math.floor((layout.clientWidth - 32 + 12) / (minCardWidth + 12)));
			columnCount = columns;
			get(rowVirtualizer).setOptions({
				count: Math.ceil(count / columns),
				scrollMargin: content.getBoundingClientRect().top + window.scrollY
			});
		};
		updateOptions();

		const resizeObserver = new ResizeObserver(updateOptions);
		resizeObserver.observe(layout);
		const mainContent = document.getElementById('main-content');
		if (mainContent) resizeObserver.observe(mainContent);
		window.addEventListener('resize', updateOptions);
		return () => {
			resizeObserver.disconnect();
			window.removeEventListener('resize', updateOptions);
		};
	});

	function measureGridRow(element: HTMLDivElement, index: number) {
		const measure = () => {
			element.dataset.index = String(index);
			get(rowVirtualizer).measureElement(element);
		};
		measure();
		return {
			update(nextIndex: number) {
				index = nextIndex;
				measure();
			}
		};
	}

	async function loadThumbnail(entry: FileManagerEntry): Promise<void> {
		if (!entry.thumbnail) return;
		try {
			const preview = entry.thumbnail;
			let encryptedBytes = await getCachedThumbnail(entry.id, preview.version, preview.size);
			if (controller.signal.aborted || !visibleIds.has(entry.id)) return;

			if (!encryptedBytes) {
				const { data } = await api.get<ArrayBuffer>(`/api/files/${encodeURIComponent(entry.id)}/thumbnail`, {
					responseType: 'arraybuffer',
					signal: controller.signal
				});
				if (controller.signal.aborted || !visibleIds.has(entry.id)) return;
				encryptedBytes = new Uint8Array(data);
				void cacheThumbnail(entry.id, preview.version, preview.size, encryptedBytes).catch(() => {});
			}

			const { thumbnailBytes } = await decryptSearchThumbnail({
				id: entry.id,
				vaultId: preview.vaultId,
				encryptedMetadata: preview.encryptedMetadata,
				encryptedFileKey: preview.encryptedFileKey,
				score: 0,
				url: `/files/${entry.id}/view`
			}, encryptedBytes);
			if (controller.signal.aborted || !visibleIds.has(entry.id) || !thumbnailBytes.length) {
				thumbnailBytes.fill(0);
				return;
			}

			const url = URL.createObjectURL(new Blob([Uint8Array.from(thumbnailBytes)], { type: preview.mime }));
			objectUrls.set(entry.id, url);
			thumbnails.set(entry.id, url);
			thumbnailBytes.fill(0);
		} catch {
			// Missing previews leave the file card's icon in place.
		}
	}

	function loadQueuedThumbnails(): void {
		while (activeLoads < THUMBNAIL_CONCURRENCY && queue.length) {
			const entry = queue.shift();
			if (!entry) return;
			if (!visibleIds.has(entry.id)) {
				pendingIds.delete(entry.id);
				continue;
			}
			activeLoads += 1;
			void loadThumbnail(entry).finally(() => {
				activeLoads -= 1;
				pendingIds.delete(entry.id);
				loadQueuedThumbnails();
			});
		}
	}

	$effect(() => {
		visibleIds = new Set(renderedEntries.map((entry) => entry.id));
		for (const id of attemptedIds) {
			if (!visibleIds.has(id)) attemptedIds.delete(id);
		}
		for (const [id, url] of objectUrls) {
			if (visibleIds.has(id)) continue;
			URL.revokeObjectURL(url);
			objectUrls.delete(id);
			thumbnails.delete(id);
		}
		for (const entry of queue) {
			if (!visibleIds.has(entry.id)) pendingIds.delete(entry.id);
		}
		queue = queue.filter((entry) => visibleIds.has(entry.id));
		for (const entry of renderedEntries) {
			if (entry.kind === 'file' && entry.thumbnail && !objectUrls.has(entry.id) && !pendingIds.has(entry.id) && !attemptedIds.has(entry.id)) {
				attemptedIds.add(entry.id);
				pendingIds.add(entry.id);
				queue.push(entry);
			}
		}
		loadQueuedThumbnails();
	});

	onDestroy(() => {
		controller.abort();
		for (const url of objectUrls.values()) URL.revokeObjectURL(url);
		objectUrls.clear();
		thumbnails.clear();
	});
</script>

<div
	bind:this={gridLayout}
	class="grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-3 p-4 max-[640px]:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]"
>
	<div
		bind:this={gridContent}
		class={`relative col-span-full ${rows.length ? '' : 'px-2 py-8 text-center text-(--muted)'}`}
		style={rows.length ? `height: ${totalGridSize}px` : undefined}
	>
		{#if rows.length === 0}
			This folder is empty.
		{:else}
			{#each virtualRows as virtualRow (virtualRow.key)}
				{@const start = virtualRow.index * columnCount}
				<div
					use:measureGridRow={virtualRow.index}
					class="absolute top-0 left-0 grid w-full gap-3"
					style={`grid-template-columns: repeat(${columnCount}, minmax(0, 1fr)); transform: translateY(${virtualRow.start - rowScrollMargin}px)`}
				>
					{#each rows.slice(start, start + columnCount) as entry (`${entry.kind}:${entry.id}`)}
						<article class={`relative min-w-0 overflow-hidden rounded-md border bg-(--surface) transition-colors hover:border-(--accent) ${selectedIds.has(fileManagerEntryKey(entry)) ? 'border-(--accent)' : 'border-(--border)'}`}>
							<input
								class="absolute top-2 right-2 z-1 size-4 cursor-pointer"
								type="checkbox"
								checked={selectedIds.has(fileManagerEntryKey(entry))}
								aria-label={`Select ${entry.name}`}
								onchange={() => onToggleSelection(entry)}
							/>
							<div class="grid aspect-[4/3] place-items-center overflow-hidden bg-(--surface-highest)">
								{#if entry.kind === 'folder'}
									<Folder class="size-10 text-(--accent)" aria-hidden="true" />
								{:else if thumbnails.get(entry.id)}
									<img class="size-full object-cover" src={thumbnails.get(entry.id)} alt={entry.name} loading="lazy" decoding="async" />
								{:else}
									<FileIcon class="size-10 text-(--muted)" aria-hidden="true" />
								{/if}
							</div>
							<div class="grid min-w-0 gap-1 p-3">
								{#if entry.kind === 'folder'}
									<button class="min-w-0 cursor-pointer truncate text-left font-medium hover:text-(--accent)" type="button" onclick={() => onOpenFolder(entry.id)}>{entry.name}</button>
								{:else}
									<span class="truncate font-medium" title={entry.name}>{entry.name}</span>
								{/if}
								<span class="truncate font-mono text-xs text-(--muted)" title={entry.type}>{entry.type}</span>
								<span class="text-xs text-(--muted)">{formatBytes(entry.size)}</span>
							</div>
						</article>
					{/each}
				</div>
			{/each}
		{/if}
	</div>
</div>
