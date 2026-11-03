<script lang="ts">
	import { Download, FolderOpen, FolderPlus, Trash2, X } from '@lucide/svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import FilesGridView from './FilesGridView.svelte';
	import FilesListView from './FilesListView.svelte';
	import FilesViewOptions from './FilesViewOptions.svelte';
	import FileUploader from '#lib/components/FileUploader.svelte';
	import type { UploadItem } from '#lib/transfers/manager.ts';
	import type { FileManagerEntry, FilesView } from './types';
	import type { createFolderEntries } from './folder-entries.svelte';

	type Query = ReturnType<typeof createFolderEntries>['query'];
	type BreadcrumbItem = { id: string; label: string; href?: string };

	let {
		unlocked,
		onFilesDropped,
		folderName,
		breadcrumbItems,
		focusFileId,
		viewMode,
		query,
		rows,
		displayedRows,
		entriesReady,
		newFolderActive,
		showNewFolder,
		selectedIds,
		selectedEntries,
		selectedFiles,
		loadedCount,
		allRowsSelected,
		tileSize,
		showType,
		showSize,
		showDimensions,
		newFolderName = $bindable(''),
		creatingFolder,
		onBeginNewFolder,
		onToggleSelection,
		onToggleAll,
		onClearSelection,
		onDownloadSelected,
		onDeleteSelected,
		onOpenFolder,
		onShowContextMenu,
		onShowBackgroundContextMenu,
		onPreviewImage,
		onPreviewMedia,
		onPreviewPdf,
		onPreviewText,
		onCreateFolder,
		onCancelNewFolder,
		onViewChange,
		onTileSizeChange,
		onShowTypeChange,
		onShowSizeChange,
		onShowDimensionsChange,
	}: {
		unlocked: boolean;
		onFilesDropped: (files: UploadItem[]) => void;
		folderName: string;
		breadcrumbItems: BreadcrumbItem[];
		focusFileId: string | null;
		viewMode: FilesView;
		query: Query;
		rows: FileManagerEntry[];
		displayedRows: FileManagerEntry[];
		entriesReady: boolean;
		newFolderActive: boolean;
		showNewFolder: boolean;
		selectedIds: ReadonlySet<string>;
		selectedEntries: FileManagerEntry[];
		selectedFiles: FileManagerEntry[];
		loadedCount: number;
		allRowsSelected: boolean;
		tileSize: number;
		showType: boolean;
		showSize: boolean;
		showDimensions: boolean;
		newFolderName?: string;
		creatingFolder: boolean;
		onBeginNewFolder: () => void;
		onToggleSelection: (entry: FileManagerEntry) => void;
		onToggleAll: () => void;
		onClearSelection: () => void;
		onDownloadSelected: () => void;
		onDeleteSelected: () => void;
		onOpenFolder: (id: string) => void;
		onShowContextMenu: (entry: FileManagerEntry | null, x: number, y: number) => void;
		onShowBackgroundContextMenu: (event: MouseEvent) => void;
		onPreviewImage: (entry: FileManagerEntry, trigger: HTMLElement) => void;
		onPreviewMedia: (entry: FileManagerEntry) => void;
		onPreviewPdf: (entry: FileManagerEntry) => void;
		onPreviewText: (entry: FileManagerEntry) => void;
		onCreateFolder: () => void | Promise<void>;
		onCancelNewFolder: () => void;
		onViewChange: (view: FilesView) => void;
		onTileSizeChange: (size: number) => void;
		onShowTypeChange: (show: boolean) => void;
		onShowSizeChange: (show: boolean) => void;
		onShowDimensionsChange: (show: boolean) => void;
	} = $props();

	let sentinel = $state<HTMLDivElement>();
	const selectedCount = $derived(selectedEntries.length);
	const showSelectionToolbar = $derived(
		(rows.length > 0 && viewMode === 'grid') || selectedCount > 0,
	);

	$effect(() => {
		const target = sentinel;

		if (
			!target ||
			!query.hasNextPage ||
			query.isFetching ||
			typeof IntersectionObserver === 'undefined'
		)
			return;
		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry?.isIntersecting && query.hasNextPage && !query.isFetching)
					void query.fetchNextPage();
			},
			{ rootMargin: '400px' },
		);
		observer.observe(target);

		return () => observer.disconnect();
	});
</script>

<div class="min-h-full bg-(--bg) py-6 pb-8">
	<div class="mx-auto grid w-[min(1440px,calc(100%-40px))] gap-5 max-[960px]:w-[calc(100%-24px)]">
		<header
			class="flex items-center justify-between gap-4 rounded-md border border-(--border) bg-(--surface-raised) px-3 py-2.5 max-[640px]:flex-col max-[640px]:items-stretch"
		>
			<h1 class="sr-only">{folderName || 'Files'}</h1>
			<div class="flex min-w-0 flex-1 items-center gap-2 max-[640px]:w-full">
				<FolderOpen class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
				<div class="min-w-0 flex-1">
					<Breadcrumbs items={breadcrumbItems} label="Folder path" />
				</div>
			</div>
			<div
				class="flex min-w-0 shrink-0 flex-wrap items-center justify-end gap-2 max-[640px]:w-full max-[640px]:justify-between max-[640px]:pt-2"
			>
				{#if unlocked}
					<button
						class="inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 rounded-md border border-(--border) bg-(--surface) px-3 text-sm text-(--text) hover:bg-(--surface-highest) disabled:cursor-not-allowed disabled:opacity-50"
						type="button"
						disabled={newFolderActive || creatingFolder}
						aria-keyshortcuts="Control+Alt+n Meta+Alt+n"
						onclick={onBeginNewFolder}
					>
						<FolderPlus class="size-4" aria-hidden="true" />
						<span>New folder</span>
					</button>
				{/if}
				{#if showSelectionToolbar}
					<div
						class="inline-flex max-w-full flex-wrap items-center gap-1 rounded-md border border-(--border) bg-(--surface) p-1"
					>
						{#if rows.length && viewMode === 'grid' && !allRowsSelected}
							<button
								class="inline-flex h-8 cursor-pointer items-center rounded px-2 text-sm text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
								type="button"
								onclick={onToggleAll}>Select all</button
							>
						{/if}
						{#if selectedCount}
							<span class="px-2 text-sm text-(--muted)">{selectedCount} selected</span>
							{#if selectedFiles.length}
								<button
									class="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded px-2 text-sm text-(--text) hover:bg-(--surface-highest)"
									type="button"
									aria-label={`Download ${selectedFiles.length} selected ${selectedFiles.length === 1 ? 'file' : 'files'}`}
									onclick={onDownloadSelected}
								>
									<Download class="size-4" aria-hidden="true" />
									<span>Download</span>
								</button>
							{/if}
							<button
								class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-red-400 hover:bg-red-500/10 hover:text-red-300"
								type="button"
								aria-label="Delete selected items"
								onclick={onDeleteSelected}><Trash2 class="size-4" aria-hidden="true" /></button
							>
							<button
								class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
								type="button"
								aria-label="Clear selection"
								onclick={onClearSelection}><X class="size-4" aria-hidden="true" /></button
							>
						{/if}
					</div>
				{/if}
				<FilesViewOptions
					view={viewMode}
					{tileSize}
					{showType}
					{showSize}
					{showDimensions}
					{onViewChange}
					{onTileSizeChange}
					{onShowTypeChange}
					{onShowSizeChange}
					{onShowDimensionsChange}
				/>
			</div>
		</header>
		{#if !unlocked}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)"
				role="status"
			>
				Unlock the vault from the header to view your files.
			</section>
		{:else if query.isPending}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)"
				role="status"
			>
				Loading files…
			</section>
		{:else if query.isError && !query.data}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-red-400"
				role="alert"
			>
				{query.error instanceof Error ? query.error.message : 'Could not load files.'}
			</section>
		{:else if query.data}
			<FileUploader {unlocked} {onFilesDropped}>
				<section
					class="overflow-hidden rounded-md border border-(--border) bg-(--surface-raised)"
					aria-label="Files and folders"
					oncontextmenu={onShowBackgroundContextMenu}
				>
					{#if !entriesReady}
						<div class="px-5 py-8 text-(--muted)" role="status">Decrypting file details…</div>
					{:else if viewMode === 'grid'}
						<FilesGridView
							rows={displayedRows}
							{focusFileId}
							{selectedIds}
							{tileSize}
							{showType}
							{showSize}
							{showDimensions}
							bind:newFolderName
							{creatingFolder}
							{onCreateFolder}
							{onCancelNewFolder}
							{onToggleSelection}
							{onOpenFolder}
							{onShowContextMenu}
							{onPreviewImage}
							{onPreviewMedia}
							{onPreviewPdf}
							{onPreviewText}
						/>
					{:else}
						<FilesListView
							rows={displayedRows}
							{focusFileId}
							{selectedIds}
							{showDimensions}
							bind:newFolderName
							{creatingFolder}
							{onCreateFolder}
							{onCancelNewFolder}
							{selectedCount}
							{loadedCount}
							{onToggleSelection}
							{onToggleAll}
							{onOpenFolder}
							{onShowContextMenu}
							{onPreviewImage}
							{onPreviewMedia}
							{onPreviewPdf}
							{onPreviewText}
						/>
					{/if}

					{#if entriesReady && query.hasNextPage}
						<div
							bind:this={sentinel}
							data-upload-click-ignore
							class="flex flex-col items-center gap-2 border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)"
						>
							{#if query.isFetchNextPageError}<p class="m-0 text-red-400" role="alert">
									{query.error instanceof Error
										? query.error.message
										: 'Could not load more files.'}
								</p>{/if}
							<button
								class="cursor-pointer rounded border border-(--border) px-4 py-2 disabled:cursor-not-allowed disabled:opacity-50"
								type="button"
								disabled={query.isFetchingNextPage}
								onclick={() => void query.fetchNextPage()}
								>{query.isFetchingNextPage ? 'Loading more…' : 'Load more'}</button
							>
						</div>
					{:else if entriesReady}
						<footer class="border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)">
							{rows.length}
							{rows.length === 1 ? 'item' : 'items'}
						</footer>
					{/if}
				</section>
			</FileUploader>
		{/if}
	</div>
</div>
