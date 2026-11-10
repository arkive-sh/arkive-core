<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { Folder, Share2 } from '@lucide/svelte';
	import { createWindowVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import { formatBytes } from '#lib/utils/format.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import InlineNewFolder from './InlineNewFolder.svelte';
	import { isImageFile } from '#lib/utils/image.ts';
	import { isMediaMimeType } from '#lib/utils/media.ts';
	import { readDecryptedThumbnail } from '#lib/vault/thumbnail.ts';
	import {
		fileManagerEntryKey,
		isPdfFile,
		isTextFile,
		NEW_FOLDER_DRAFT_ID,
		type FileManagerEntry,
	} from './types';

	const THUMBNAIL_CONCURRENCY = 4;
	let {
		rows,
		focusFileId,
		selectedIds,
		selectionEnabled = true,
		previewEnabled = true,
		tileSize,
		showType,
		showSize,
		showDimensions,
		newFolderName = $bindable(''),
		creatingFolder,
		onCreateFolder,
		onCancelNewFolder,
		onToggleSelection,
		onSelectEntry,
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
		tileSize: number;
		showType: boolean;
		showSize: boolean;
		showDimensions: boolean;
		newFolderName?: string;
		creatingFolder: boolean;
		onCreateFolder?: () => void | Promise<void>;
		onCancelNewFolder?: () => void;
		onToggleSelection?: (entry: FileManagerEntry) => void;
		onSelectEntry?: (entry: FileManagerEntry, additive: boolean) => void;
		onOpenFolder: (id: string) => void;
		onOpenShares?: (id: string) => void;
		onShowContextMenu: (entry: FileManagerEntry | null, x: number, y: number) => void;
		onPreviewImage: (entry: FileManagerEntry, trigger: HTMLElement) => void;
		onPreviewMedia: (entry: FileManagerEntry) => void;
		onPreviewPdf: (entry: FileManagerEntry) => void;
		onPreviewText: (entry: FileManagerEntry) => void;
	} = $props();
	let gridLayout = $state<HTMLDivElement>();
	let gridContent = $state<HTMLDivElement>();
	let pendingFocusIndex = $state<number | null>(null);
	let columnCount = $state(1);
	const canSelect = $derived(selectionEnabled && Boolean(onToggleSelection));
	const cardGap = $derived(Math.min(8, Math.max(4, Math.round(tileSize * 0.04))));
	const gridPadding = $derived(Math.min(8, Math.max(4, Math.round(tileSize * 0.05))));
	const contentGap = $derived(Math.max(2, Math.round(tileSize * 0.025)));
	const nameFontSize = $derived(Math.min(16, Math.max(12, Math.round(tileSize * 0.1))));
	const detailFontSize = $derived(Math.min(12, Math.max(10, Math.round(tileSize * 0.08))));
	const entryIconSize = $derived(Math.min(88, Math.max(48, Math.round(tileSize * 0.4))));
	const checkboxSize = $derived(Math.min(16, Math.max(12, Math.round(tileSize * 0.1))));
	const checkboxInset = $derived(Math.min(8, Math.max(3, Math.round(tileSize * 0.05))));
	const thumbnails = new SvelteMap<string, string>();
	const objectUrls = new SvelteMap<string, string>();
	const pendingIds = new SvelteSet<string>();
	const attemptedIds = new SvelteSet<string>();
	const controller = new AbortController();
	let measuredLayout = '';
	let visibleIds = new SvelteSet<string>();
	let queue: FileManagerEntry[] = [];
	let activeLoads = 0;
	const rowVirtualizer = createWindowVirtualizer({
		count: 0,
		estimateSize: () => {
			const detailLines = Number(showType) + Number(showSize) + Number(showDimensions);

			return (
				entryIconSize +
				cardGap * 2 +
				nameFontSize * 1.25 +
				detailLines * (detailFontSize * 1.25 + contentGap)
			);
		},
		gap: 0,
		overscan: 2,
		getItemKey: (index) => {
			const entry = rows[index * columnCount];

			return entry ? fileManagerEntryKey(entry) : index;
		},
	});
	const virtualRows = $derived($rowVirtualizer.getVirtualItems());
	const totalGridSize = $derived($rowVirtualizer.getTotalSize());
	const rowScrollMargin = $derived($rowVirtualizer.options.scrollMargin);
	const renderedEntries = $derived(
		virtualRows.flatMap(({ index }) => rows.slice(index * columnCount, (index + 1) * columnCount)),
	);

	$effect(() => {
		const index = pendingFocusIndex;

		if (index === null || !virtualRows.some((row) => row.index === Math.floor(index / columnCount)))
			return;
		const button = gridContent?.querySelector<HTMLButtonElement>(
			`[data-entry-action][data-entry-index="${index}"]`,
		);

		if (!button) return;
		pendingFocusIndex = null;
		button.focus({ preventScroll: true });
	});

	$effect(() => {
		const layout = gridLayout;
		const content = gridContent;
		const count = rows.length;

		if (!layout || !content || typeof window === 'undefined') return;

		const updateOptions = () => {
			const columns = Math.max(
				1,
				Math.floor((layout.clientWidth - gridPadding * 2 + cardGap) / (tileSize + cardGap)),
			);
			columnCount = columns;
			get(rowVirtualizer).setOptions({
				count: Math.ceil(count / columns),
				gap: cardGap,
				scrollMargin: content.getBoundingClientRect().top + window.scrollY,
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
			if (!canceled)
				get(rowVirtualizer).scrollToIndex(Math.floor(index / columnCount), { align: 'center' });
		});

		return () => {
			canceled = true;
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
			},
		};
	}

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
		if (index < 0 || index >= rows.length || rows[index].id === NEW_FOLDER_DRAFT_ID) return;
		pendingFocusIndex = index;
		get(rowVirtualizer).scrollToIndex(Math.floor(index / columnCount), { align: 'auto' });
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
		const article = event.target.closest<HTMLElement>('[data-entry-index]');

		if (!article) {
			if (!['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(event.key) || event.altKey)
				return;
			const selected = rows.findIndex(
				(entry) => entry.id !== NEW_FOLDER_DRAFT_ID && selectedIds.has(fileManagerEntryKey(entry)),
			);
			const first =
				selected >= 0 ? selected : rows.findIndex((entry) => entry.id !== NEW_FOLDER_DRAFT_ID);

			if (first >= 0) {
				event.preventDefault();
				focusEntry(first);
			}

			return;
		}
		const index = Number(article.dataset.entryIndex);

		if (['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(event.key)) {
			if (event.altKey) return;
			event.preventDefault();
			const step =
				event.key === 'ArrowDown'
					? columnCount
					: event.key === 'ArrowUp'
						? -columnCount
						: event.key === 'ArrowRight'
							? 1
							: -1;
			let next = index + step;

			while (rows[next]?.id === NEW_FOLDER_DRAFT_ID) next += Math.sign(step);
			if (event.key === 'ArrowDown' && next >= rows.length) next = rows.length - 1;
			if (next < 0 || next >= rows.length || next === index) {
				if (event.key === 'ArrowUp') window.scrollTo({ top: 0, behavior: 'smooth' });

				return;
			}
			focusEntry(next);
		} else if (
			(event.key === 'Enter' || event.code === 'Space') &&
			event.target.closest('[data-entry-action]')
		) {
			const entry = rows[index];

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

	async function loadThumbnail(entry: FileManagerEntry): Promise<void> {
		if (!entry.thumbnail) return;
		try {
			const blob = await readDecryptedThumbnail(
				{ id: entry.id, ...entry.thumbnail },
				controller.signal,
				() => visibleIds.has(entry.id),
			);

			if (controller.signal.aborted || !visibleIds.has(entry.id) || !blob) return;
			const url = URL.createObjectURL(blob);
			objectUrls.set(entry.id, url);
			thumbnails.set(entry.id, url);
		} catch (error) {
			if (typeof window !== 'undefined' && window.location.hostname === 'localhost') {
				console.warn(
					'Thumbnail preview failed:',
					error instanceof Error ? error.message : 'Unknown error',
				);
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
		visibleIds = new SvelteSet(renderedEntries.map((entry) => entry.id));
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
			if (
				entry.kind === 'file' &&
				entry.thumbnail &&
				!objectUrls.has(entry.id) &&
				!pendingIds.has(entry.id) &&
				!attemptedIds.has(entry.id)
			) {
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

<svelte:window onkeydown={handleEntryKeydown} />

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
					{#each rows.slice(start, start + columnCount) as entry, columnIndex (`${entry.kind}:${entry.id}`)}
						{@const entryIndex = start + columnIndex}
						{#if entry.id === NEW_FOLDER_DRAFT_ID}
							{#if onCreateFolder && onCancelNewFolder}
								<article data-file-entry class="min-w-0">
									<div
										class="grid place-items-center overflow-hidden"
										style={`height: ${entryIconSize + 32}px`}
									>
										<Folder
											class="fill-(--accent) text-(--accent)"
											style={`width: ${entryIconSize}px; height: ${entryIconSize}px`}
											aria-hidden="true"
										/>
									</div>
									<div style={`padding: ${cardGap}px`}>
										<InlineNewFolder
											variant="grid"
											bind:name={newFolderName}
											busy={creatingFolder}
											onCreate={onCreateFolder}
											onCancel={onCancelNewFolder}
										/>
									</div>
								</article>
							{/if}
						{:else}
							{@const actionLabel = entryActionLabel(entry)}
							<article
								data-file-entry
								data-entry-index={entryIndex}
								class={`group relative min-w-0 cursor-pointer rounded-md transition-colors ${selectedIds.has(fileManagerEntryKey(entry)) ? 'bg-(--surface-highest)' : ''} ${focusFileId === entry.id ? 'ring-2 ring-(--accent)' : ''}`}
								oncontextmenu={(event) => {
									event.preventDefault();
									onShowContextMenu(entry, event.clientX, event.clientY);
								}}
							>
								<button
									class="absolute inset-0 z-1 m-0 cursor-pointer appearance-none border-0 bg-transparent p-0 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-(--accent)"
									type="button"
									data-entry-action
									data-entry-index={entryIndex}
									aria-label={canSelect && entry.selectable !== false
										? `Select ${entry.name}${actionLabel ? '; press Enter or double-click to open' : ''}`
										: (actionLabel ?? entry.name)}
									title={actionLabel ? 'Click to select; double-click to open' : undefined}
									onclick={(event) => onSelectEntry?.(entry, event.ctrlKey || event.metaKey)}
									ondblclick={(event) => openEntry(entry, event.currentTarget)}
								></button>
								{#if canSelect && entry.selectable !== false}<input
										class="absolute z-10 cursor-pointer"
										style={`top: ${checkboxInset}px; right: ${checkboxInset}px; width: ${checkboxSize}px; height: ${checkboxSize}px`}
										type="checkbox"
										checked={selectedIds.has(fileManagerEntryKey(entry))}
										aria-label={`Select ${entry.name}`}
										onchange={() => onToggleSelection?.(entry)}
									/>{/if}
								{#if entry.kind === 'folder'}
									<div
										class="grid w-full place-items-center overflow-hidden"
										style={`height: ${entryIconSize + 32}px`}
										aria-hidden="true"
									>
										<Folder
											class="fill-(--accent) text-(--accent) transition-[filter] group-hover:brightness-110"
											style={`width: ${entryIconSize}px; height: ${entryIconSize}px`}
											aria-hidden="true"
										/>
									</div>
								{:else if thumbnails.get(entry.id)}
									<div
										class="grid w-full place-items-center overflow-hidden"
										style={`height: ${entryIconSize + 32}px`}
										aria-hidden="true"
									>
										<img
											class="max-h-full max-w-full object-contain transition-[filter] group-hover:brightness-105"
											src={thumbnails.get(entry.id)}
											alt=""
											loading="lazy"
											decoding="async"
										/>
									</div>
								{:else}
									<div
										class="grid w-full place-items-center overflow-hidden"
										style={`height: ${entryIconSize + 32}px`}
										aria-hidden="true"
									>
										<FileTypeIcon
											name={entry.name}
											mime={entry.type}
											className="transition-[filter] group-hover:brightness-110"
											style={`width: ${entryIconSize}px; height: ${entryIconSize}px`}
										/>
									</div>
								{/if}
								<div
									class="grid min-w-0 text-center"
									style={`gap: ${contentGap}px; padding: ${cardGap}px`}
								>
									<div class="flex min-w-0 items-center justify-center gap-1">
										<div class="min-w-0 flex-1" title={entry.name}>
											<span
												class="block truncate font-medium leading-tight"
												style={`font-size: ${nameFontSize}px`}>{entry.name}</span
											>
										</div>
										{#if entry.isShared}
											<button
												class="relative z-10 ml-2 grid size-7 shrink-0 cursor-pointer place-items-center rounded text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
												type="button"
												title="Open this file’s share"
												aria-label={`Open share for ${entry.name}`}
												onclick={() => onOpenShares?.(entry.id)}
											>
												<Share2 class="size-4" aria-hidden="true" />
											</button>
										{/if}
									</div>
									{#if showType}<span
											class="truncate font-mono leading-tight text-(--muted)"
											style={`font-size: ${detailFontSize}px`}
											title={entry.type}>{entry.type}</span
										>{/if}
									{#if showSize}<span
											class="leading-tight text-(--muted)"
											style={`font-size: ${detailFontSize}px`}>{formatBytes(entry.size)}</span
										>{/if}
									{#if showDimensions && entry.dimensions}<span
											class="leading-tight text-(--muted)"
											style={`font-size: ${detailFontSize}px`}
											>{entry.dimensions.width} × {entry.dimensions.height}</span
										>{/if}
								</div>
							</article>
						{/if}
					{/each}
				</div>
			{/each}
		{/if}
	</div>
</div>
