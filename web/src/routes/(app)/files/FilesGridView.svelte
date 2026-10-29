<script lang="ts">
	import { onDestroy } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { Folder } from '@lucide/svelte';
	import { createWindowVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { api } from '#lib/api.ts';
	import { getCachedThumbnail, cacheThumbnail } from '#lib/thumbnail-cache.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import { decryptSearchThumbnail } from '#lib/vault/search.ts';
	import { fileManagerEntryKey, type FileManagerEntry } from './types';

	const THUMBNAIL_CONCURRENCY = 4;
	let {
		rows,
		selectedIds,
		tileSize,
		showType,
		showSize,
		onToggleSelection,
		onOpenFolder,
		onShowContextMenu
	}: {
		rows: FileManagerEntry[];
		selectedIds: ReadonlySet<string>;
		tileSize: number;
		showType: boolean;
		showSize: boolean;
		onToggleSelection: (entry: FileManagerEntry) => void;
		onOpenFolder: (id: string) => void;
		onShowContextMenu: (entry: FileManagerEntry, x: number, y: number) => void;
	} = $props();
	let gridLayout = $state<HTMLDivElement>();
	let gridContent = $state<HTMLDivElement>();
	let columnCount = $state(1);
	const cardGap = $derived(Math.min(12, Math.max(4, Math.round(tileSize * 0.08))));
	const gridPadding = $derived(Math.min(16, Math.max(4, Math.round(tileSize * 0.1))));
	const contentGap = $derived(Math.max(2, Math.round(tileSize * 0.025)));
	const nameFontSize = $derived(Math.min(16, Math.max(12, Math.round(tileSize * 0.1))));
	const detailFontSize = $derived(Math.min(12, Math.max(10, Math.round(tileSize * 0.08))));
	const iconSize = $derived(Math.min(40, Math.max(16, Math.round(tileSize * 0.27))));
	const checkboxSize = $derived(Math.min(16, Math.max(12, Math.round(tileSize * 0.1))));
	const checkboxInset = $derived(Math.min(8, Math.max(3, Math.round(tileSize * 0.05))));
	const thumbnails = new SvelteMap<string, string>();
	const objectUrls = new Map<string, string>();
	const pendingIds = new Set<string>();
	const attemptedIds = new Set<string>();
	const controller = new AbortController();
	let measuredLayout = '';
	let visibleIds = new Set<string>();
	let queue: FileManagerEntry[] = [];
	let activeLoads = 0;
	const rowVirtualizer = createWindowVirtualizer({
		count: 0,
		estimateSize: () => {
			const contentWidth = Math.max(tileSize, (gridLayout?.clientWidth ?? 0) - gridPadding * 2);
			const cardWidth = (contentWidth - cardGap * (columnCount - 1)) / columnCount;
			const detailLines = Number(showType) + Number(showSize);
			return cardWidth * 0.75 + cardGap * 2 + nameFontSize * 1.25 + detailLines * (detailFontSize * 1.25 + contentGap) + 2;
		},
		gap: cardGap,
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
			const columns = Math.max(1, Math.floor((layout.clientWidth - gridPadding * 2 + cardGap) / (tileSize + cardGap)));
			columnCount = columns;
			get(rowVirtualizer).setOptions({
				count: Math.ceil(count / columns),
				gap: cardGap,
				scrollMargin: content.getBoundingClientRect().top + window.scrollY
			});
			const layoutKey = `${tileSize}:${columns}:${showType}:${showSize}`;
			if (layoutKey !== measuredLayout) {
				get(rowVirtualizer).measure();
				measuredLayout = layoutKey;
			}
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
				const { data } = await api.get<{ url: string }>(`/api/files/${encodeURIComponent(entry.id)}/thumbnail/presign`, {
					signal: controller.signal
				});
				const response = await fetch(data.url, { credentials: 'omit', signal: controller.signal });
				if (!response.ok) throw new Error(`Thumbnail request failed (${response.status}).`);
				if (controller.signal.aborted || !visibleIds.has(entry.id)) return;
				encryptedBytes = new Uint8Array(await response.arrayBuffer());
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
		} catch (error) {
			if (typeof window !== 'undefined' && window.location.hostname === 'localhost') {
				console.warn('Thumbnail preview failed:', error instanceof Error ? error.message : 'Unknown error');
			}
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
	class="grid"
	style={`grid-template-columns: repeat(auto-fill, minmax(${tileSize}px, 1fr)); gap: ${cardGap}px; padding: ${gridPadding}px`}
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
					class="absolute top-0 left-0 grid w-full"
					style={`grid-template-columns: repeat(${columnCount}, minmax(0, 1fr)); gap: ${cardGap}px; transform: translateY(${virtualRow.start - rowScrollMargin}px)`}
				>
					{#each rows.slice(start, start + columnCount) as entry (`${entry.kind}:${entry.id}`)}
						<article
							class={`relative min-w-0 overflow-hidden rounded-md border bg-(--surface) transition-colors hover:border-(--accent) ${selectedIds.has(fileManagerEntryKey(entry)) ? 'border-(--accent)' : 'border-(--border)'}`}
							oncontextmenu={(event) => {
								event.preventDefault();
								onShowContextMenu(entry, event.clientX, event.clientY);
							}}
						>
							<input
								class="absolute z-1 cursor-pointer"
								style={`top: ${checkboxInset}px; right: ${checkboxInset}px; width: ${checkboxSize}px; height: ${checkboxSize}px`}
								type="checkbox"
								checked={selectedIds.has(fileManagerEntryKey(entry))}
								aria-label={`Select ${entry.name}`}
								onchange={() => onToggleSelection(entry)}
							/>
							<div class="grid aspect-[4/3] place-items-center overflow-hidden bg-(--surface-highest)">
								{#if entry.kind === 'folder'}
									<Folder class="text-(--accent)" style={`width: ${iconSize}px; height: ${iconSize}px`} aria-hidden="true" />
								{:else if thumbnails.get(entry.id)}
									<img class="size-full object-cover" src={thumbnails.get(entry.id)} alt={entry.name} loading="lazy" decoding="async" />
								{:else}
									<FileTypeIcon name={entry.name} mime={entry.type} style={`width: ${iconSize}px; height: ${iconSize}px`} />
								{/if}
							</div>
							<div class="grid min-w-0" style={`gap: ${contentGap}px; padding: ${cardGap}px`}>
								{#if entry.kind === 'folder'}
									<button class="min-w-0 cursor-pointer truncate text-left font-medium leading-tight hover:text-(--accent)" style={`font-size: ${nameFontSize}px`} type="button" onclick={() => onOpenFolder(entry.id)}>{entry.name}</button>
								{:else}
									<span class="truncate font-medium leading-tight" style={`font-size: ${nameFontSize}px`} title={entry.name}>{entry.name}</span>
								{/if}
								{#if showType}<span class="truncate font-mono leading-tight text-(--muted)" style={`font-size: ${detailFontSize}px`} title={entry.type}>{entry.type}</span>{/if}
								{#if showSize}<span class="leading-tight text-(--muted)" style={`font-size: ${detailFontSize}px`}>{formatBytes(entry.size)}</span>{/if}
							</div>
						</article>
					{/each}
				</div>
			{/each}
		{/if}
	</div>
</div>
