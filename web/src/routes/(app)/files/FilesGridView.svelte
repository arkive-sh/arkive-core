<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { Folder } from '@lucide/svelte';
	import { createWindowVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { formatBytes } from '#lib/utils/format.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import { isImageFile } from '#lib/utils/image.ts';
	import { readDecryptedThumbnail } from '#lib/vault/thumbnail.ts';
	import { fileManagerEntryKey, type FileManagerEntry } from './types';

	const THUMBNAIL_CONCURRENCY = 4;
	let {
		rows,
		focusFileId,
		selectedIds,
		tileSize,
		showType,
		showSize,
		showDimensions,
		onToggleSelection,
		onOpenFolder,
		onShowContextMenu,
		onPreviewImage,
		onPreviewVideo
	}: {
		rows: FileManagerEntry[];
		focusFileId: string | null;
		selectedIds: ReadonlySet<string>;
		tileSize: number;
		showType: boolean;
		showSize: boolean;
		showDimensions: boolean;
		onToggleSelection: (entry: FileManagerEntry) => void;
		onOpenFolder: (id: string) => void;
		onShowContextMenu: (entry: FileManagerEntry, x: number, y: number) => void;
		onPreviewImage: (entry: FileManagerEntry, trigger: HTMLElement) => void;
		onPreviewVideo: (entry: FileManagerEntry) => void;
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
			const detailLines = Number(showType) + Number(showSize) + Number(showDimensions);
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
			const layoutKey = `${tileSize}:${columns}:${showType}:${showSize}:${showDimensions}`;
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

	$effect(() => {
		const targetId = focusFileId;
		const index = rows.findIndex((entry) => entry.kind === 'file' && entry.id === targetId);
		if (!targetId || index < 0) return;

		let canceled = false;
		void tick().then(() => {
			if (!canceled) get(rowVirtualizer).scrollToIndex(Math.floor(index / columnCount), { align: 'center' });
		});
		return () => { canceled = true; };
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
			const blob = await readDecryptedThumbnail(
				{ id: entry.id, ...entry.thumbnail },
				controller.signal,
				() => visibleIds.has(entry.id)
			);
			if (controller.signal.aborted || !visibleIds.has(entry.id) || !blob) return;
			const url = URL.createObjectURL(blob);
			objectUrls.set(entry.id, url);
			thumbnails.set(entry.id, url);
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
							class={`relative min-w-0 overflow-hidden rounded-md border bg-(--surface) transition-colors hover:border-(--accent) ${selectedIds.has(fileManagerEntryKey(entry)) || focusFileId === entry.id ? 'border-(--accent)' : 'border-(--border)'} ${focusFileId === entry.id ? 'ring-2 ring-(--accent)' : ''}`}
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
							{#if entry.kind === 'file' && isImageFile(entry.name, entry.type)}
								<button class="grid aspect-4/3 w-full cursor-zoom-in place-items-center overflow-hidden bg-(--surface-highest)" type="button" aria-label={`Preview ${entry.name}`} onclick={(event) => onPreviewImage(entry, event.currentTarget)}>
									{#if thumbnails.get(entry.id)}
										<img class="size-full object-cover" src={thumbnails.get(entry.id)} alt={entry.name} loading="lazy" decoding="async" />
									{:else}
										<FileTypeIcon name={entry.name} mime={entry.type} style={`width: ${iconSize}px; height: ${iconSize}px`} />
									{/if}
								</button>
							{:else if entry.kind === 'file' && entry.type.startsWith('video/')}
								<button class="grid aspect-4/3 w-full cursor-pointer place-items-center overflow-hidden bg-(--surface-highest)" type="button" aria-label={`Play ${entry.name}`} onclick={() => onPreviewVideo(entry)}>
									{#if thumbnails.get(entry.id)}
										<img class="size-full object-cover" src={thumbnails.get(entry.id)} alt={entry.name} loading="lazy" decoding="async" />
									{:else}
										<FileTypeIcon name={entry.name} mime={entry.type} style={`width: ${iconSize}px; height: ${iconSize}px`} />
									{/if}
								</button>
							{:else}
								<div class="grid aspect-4/3 place-items-center overflow-hidden bg-(--surface-highest)">
									{#if entry.kind === 'folder'}
										<Folder class="text-(--accent)" style={`width: ${iconSize}px; height: ${iconSize}px`} aria-hidden="true" />
									{:else if thumbnails.get(entry.id)}
										<img class="size-full object-cover" src={thumbnails.get(entry.id)} alt={entry.name} loading="lazy" decoding="async" />
									{:else}
										<FileTypeIcon name={entry.name} mime={entry.type} style={`width: ${iconSize}px; height: ${iconSize}px`} />
									{/if}
								</div>
							{/if}
							<div class="grid min-w-0" style={`gap: ${contentGap}px; padding: ${cardGap}px`}>
								{#if entry.kind === 'folder'}
									<button class="min-w-0 cursor-pointer truncate text-left font-medium leading-tight hover:text-(--accent)" style={`font-size: ${nameFontSize}px`} type="button" onclick={() => onOpenFolder(entry.id)}>{entry.name}</button>
								{:else if isImageFile(entry.name, entry.type)}
									<button class="min-w-0 cursor-zoom-in truncate text-left font-medium leading-tight hover:text-(--accent)" style={`font-size: ${nameFontSize}px`} type="button" onclick={(event) => onPreviewImage(entry, event.currentTarget)}>{entry.name}</button>
								{:else if entry.type.startsWith('video/')}
									<button class="min-w-0 cursor-pointer truncate text-left font-medium leading-tight hover:text-(--accent)" style={`font-size: ${nameFontSize}px`} type="button" onclick={() => onPreviewVideo(entry)}>{entry.name}</button>
								{:else}
									<span class="truncate font-medium leading-tight" style={`font-size: ${nameFontSize}px`} title={entry.name}>{entry.name}</span>
								{/if}
								{#if showType}<span class="truncate font-mono leading-tight text-(--muted)" style={`font-size: ${detailFontSize}px`} title={entry.type}>{entry.type}</span>{/if}
								{#if showSize}<span class="leading-tight text-(--muted)" style={`font-size: ${detailFontSize}px`}>{formatBytes(entry.size)}</span>{/if}
								{#if showDimensions && entry.dimensions}<span class="leading-tight text-(--muted)" style={`font-size: ${detailFontSize}px`}>{entry.dimensions.width} × {entry.dimensions.height}</span>{/if}
							</div>
						</article>
					{/each}
				</div>
			{/each}
		{/if}
	</div>
</div>
