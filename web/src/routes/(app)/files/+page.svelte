<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { Trash2, X } from '@lucide/svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
	import { api } from '#lib/api.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import FileEntryContextMenu from './FileEntryContextMenu.svelte';
	import FilePropertiesModal from './FilePropertiesModal.svelte';
	import { toast } from '#lib/toast.ts';
	import FilesGridView from './FilesGridView.svelte';
	import FilesListView from './FilesListView.svelte';
	import FilesViewOptions from './FilesViewOptions.svelte';
	import ImageViewer from './ImageViewer.svelte';
	import { fileManagerEntryKey, type FileManagerEntry, type FilesView } from './types';
	import { decryptSearchFile, decryptSearchFolder } from '#lib/vault/search.ts';
	import { waitUntilReady } from '#lib/vault/session.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { PageProps } from './$types';

	const PAGE_SIZE = 50;

	type ApiFolder = {
		ID: string;
		VaultID: string;
		ParentFolderID: string | null;
		EncryptedName: string;
		EncryptedMetadata: string;
		SizeBytes?: number;
		UpdatedAt: string;
	};

	type ApiFile = {
		ID: string;
		FolderID: string | null;
		EncryptedMetadata: string;
		EncryptedFileKey: string;
		UpdatedAt: string;
	};

	type FolderEntriesResponse = {
		FolderID: string | null;
		Path: ApiFolder[];
		Folders: ApiFolder[];
		Files: ApiFile[];
		NextCursor: string | null;
		HasMore: boolean;
	};

	let { data }: PageProps = $props();
	let rows = $state<FileManagerEntry[]>([]);
	let breadcrumbs = $state<Array<{ id: string | null; name: string }>>([{ id: null, name: 'Root' }]);
	let entriesReady = $state(false);
	let sentinel: HTMLDivElement | undefined = $state();
	let deleteDialogOpen = $state(false);
	let deleting = $state(false);
	let deleteTargets = $state<FileManagerEntry[]>([]);
	let propertiesEntry = $state<FileManagerEntry | null>(null);
	let imagePreviewRequest = $state<{ fileId: string; trigger: HTMLElement } | null>(null);
	let videoPreviewEntry = $state<FileManagerEntry | null>(null);
	let VideoViewer = $state<typeof import('./VideoViewer.svelte').default>();
	let contextMenuEntry = $state<FileManagerEntry | null>(null);
	let contextMenuPosition = $state({ x: 0, y: 0 });
	let tileSize = $state(150);
	let showType = $state(false);
	let showSize = $state(true);
	let viewOptionsReady = $state(false);
	const selectedIds = new SvelteSet<string>();
	const queryClient = useQueryClient();
	let decryptGeneration = 0;
	let hydratedPageCount = 0;
	let hydratedFolderId: string | null | undefined;
	let hydratedFirstPage: FolderEntriesResponse | undefined;
	let hydrationQueue = Promise.resolve();

	const folderId = $derived(page.url.searchParams.get('folder_id') || null);
	const viewMode = $derived(page.url.searchParams.get('view') === 'grid' ? 'grid' : 'list');
	const entriesQuery = createInfiniteQuery(() => ({
		queryKey: ['folder-entries', folderId],
		enabled: $vaultUnlocked,
		initialPageParam: null as string | null,
		queryFn: async ({ pageParam, signal }) => {
			const endpoint = folderId
				? `/api/folders/${encodeURIComponent(folderId)}/entries`
				: '/api/folders/root/entries';
			const { data: result } = await api.get<FolderEntriesResponse>(endpoint, {
				params: { cursor: pageParam ?? undefined, pageSize: PAGE_SIZE },
				signal
			});
			return result;
		},
		getNextPageParam: (lastPage) => lastPage.HasMore ? lastPage.NextCursor || undefined : undefined
	}));
	const loadedEntries = $derived([...new Map([
		...(entriesQuery.data?.pages ?? []).flatMap((response) => [
			...response.Folders.map((folder) => ({ id: folder.ID, kind: 'folder' as const })),
			...response.Files.map((file) => ({ id: file.ID, kind: 'file' as const }))
		]),
		...rows
	].map((entry) => [fileManagerEntryKey(entry), entry] as const)).values()]);
	const selectedEntries = $derived(loadedEntries.filter((entry) => selectedIds.has(fileManagerEntryKey(entry))));
	// shortcut: PhotoSwipe navigates loaded rows; add paged image lookup if full-folder navigation is needed.
	const imageEntries = $derived(rows.filter((entry) => entry.kind === 'file' && entry.type.startsWith('image/')));
	const allRowsSelected = $derived(loadedEntries.length > 0 && selectedEntries.length === loadedEntries.length);
	const currentLocation = $derived(breadcrumbs.map((crumb) => crumb.name).join(' / '));
	const breadcrumbItems = $derived(breadcrumbs.map((crumb, index) => ({
		id: crumb.id ?? 'root',
		label: crumb.name,
		href: index < breadcrumbs.length - 1 ? urlFor(crumb.id) : undefined
	})));

	async function decryptEntries(
		response: FolderEntriesResponse,
		generation: number,
		setPath: boolean
	): Promise<{ rows: FileManagerEntry[]; breadcrumbs?: Array<{ id: string | null; name: string }> } | undefined> {
		const vaultId = data.user.id;
		const [folderRows, fileRows] = await Promise.all([
			Promise.all(response.Folders.map(async (folder): Promise<FileManagerEntry> => {
				try {
					const decrypted = await decryptSearchFolder({
						id: folder.ID,
						vaultId,
						encryptedName: folder.EncryptedName,
						encryptedMetadata: folder.EncryptedMetadata,
						score: 0,
						url: `/folders/${folder.ID}`
					});
					return { id: folder.ID, kind: 'folder', name: decrypted.name.name || 'Encrypted folder', type: 'Folder', size: folder.SizeBytes ?? null, updatedAt: folder.UpdatedAt };
				} catch {
					return { id: folder.ID, kind: 'folder', name: 'Encrypted folder', type: 'Folder', size: folder.SizeBytes ?? null, updatedAt: folder.UpdatedAt };
				}
			})),
			Promise.all(response.Files.map(async (file): Promise<FileManagerEntry> => {
				try {
					const decrypted = await decryptSearchFile({
						id: file.ID,
						vaultId,
						encryptedMetadata: file.EncryptedMetadata,
						encryptedFileKey: file.EncryptedFileKey,
						score: 0,
						url: `/files/${file.ID}/view`
					});
					const preview = decrypted.metadata.preview;
					const thumbnailVersion = Number(preview?.thumbnail_version || 0);
					const thumbnailSize = Number(preview?.thumbnail_size || 0);
					const mime = typeof decrypted.metadata.mime === 'string' ? decrypted.metadata.mime : 'application/octet-stream';
					return {
						id: file.ID,
						kind: 'file',
						name: decrypted.metadata.name || 'Encrypted file',
						type: mime,
						thumbnail: preview?.has_thumbnail && thumbnailVersion > 0 && thumbnailSize > 0
							? {
								version: thumbnailVersion,
								size: thumbnailSize,
								mime: preview.thumbnail_mime || 'image/webp',
								width: Number(preview.thumbnail_width || 0),
								height: Number(preview.thumbnail_height || 0),
								vaultId,
								encryptedMetadata: file.EncryptedMetadata,
								encryptedFileKey: file.EncryptedFileKey
							}
							: undefined,
						size: typeof decrypted.metadata.size === 'number' ? decrypted.metadata.size : null,
						updatedAt: file.UpdatedAt
					};
				} catch {
					return { id: file.ID, kind: 'file', name: 'Encrypted file', type: 'application/octet-stream', size: null, updatedAt: file.UpdatedAt };
				}
			}))
		]);

		const path = setPath ? await Promise.all(response.Path.map(async (folder) => {
			try {
				const decrypted = await decryptSearchFolder({
					id: folder.ID,
					vaultId,
					encryptedName: folder.EncryptedName,
					encryptedMetadata: folder.EncryptedMetadata,
					score: 0,
					url: `/folders/${folder.ID}`
				});
				return { id: folder.ID, name: decrypted.name.name || 'Encrypted folder' };
			} catch {
				return { id: folder.ID, name: 'Encrypted folder' };
			}
		})) : undefined;

		if (generation !== decryptGeneration || !$vaultUnlocked) return;
		return {
			rows: [...folderRows, ...fileRows],
			breadcrumbs: path ? [{ id: null, name: 'Root' }, ...path] : undefined
		};
	}

	$effect(() => {
		const activeFolderId = folderId;
		const pages = entriesQuery.data?.pages ?? [];
		if (activeFolderId !== hydratedFolderId) {
			hydratedFolderId = activeFolderId;
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydratedFirstPage = undefined;
			hydrationQueue = Promise.resolve();
			rows = [];
			breadcrumbs = [{ id: null, name: 'Root' }];
			contextMenuEntry = null;
			propertiesEntry = null;
			entriesReady = false;
			selectedIds.clear();
		}
		if (pages[0] && (pages[0].FolderID?.toLowerCase() ?? null) !== (activeFolderId?.toLowerCase() ?? null)) return;
		if (!$vaultUnlocked) {
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydrationQueue = Promise.resolve();
			rows = [];
			breadcrumbs = [{ id: null, name: 'Root' }];
			contextMenuEntry = null;
			propertiesEntry = null;
			entriesReady = false;
			return;
		}
		if (pages.length < hydratedPageCount || pages[0] !== hydratedFirstPage) {
			const keepCurrentRows = Boolean(pages[0]) && entriesReady;
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydratedFirstPage = pages[0];
			hydrationQueue = Promise.resolve();
			if (keepCurrentRows) {
				const generation = decryptGeneration;
				void (async () => {
					const refreshedRows: FileManagerEntry[] = [];
					let refreshedBreadcrumbs: Array<{ id: string | null; name: string }> | undefined;
					for (const [index, response] of pages.entries()) {
						const decrypted = await decryptEntries(response, generation, index === 0);
						if (!decrypted) return;
						refreshedRows.push(...decrypted.rows);
						if (decrypted.breadcrumbs) refreshedBreadcrumbs = decrypted.breadcrumbs;
					}
					if (generation !== decryptGeneration || !$vaultUnlocked) return;
					rows = refreshedRows;
					if (refreshedBreadcrumbs) breadcrumbs = refreshedBreadcrumbs;
					hydratedPageCount = pages.length;
				})();
				return;
			}
			rows = [];
			breadcrumbs = [{ id: null, name: 'Root' }];
			entriesReady = false;
		}

		const generation = decryptGeneration;
		const start = hydratedPageCount;
		const newPages = pages.slice(start);
		hydratedPageCount = pages.length;
		for (const [index, response] of newPages.entries()) {
			hydrationQueue = hydrationQueue.then(async () => {
				const decrypted = await decryptEntries(response, generation, start === 0 && index === 0);
				if (!decrypted) return;
				rows = [...rows, ...decrypted.rows];
				if (decrypted.breadcrumbs) breadcrumbs = decrypted.breadcrumbs;
				entriesReady = true;
			});
		}
	});

	$effect(() => {
		const target = sentinel;
		if (!target || !entriesQuery.hasNextPage || entriesQuery.isFetching) return;

		const observer = new IntersectionObserver(([entry]) => {
			if (entry?.isIntersecting && entriesQuery.hasNextPage && !entriesQuery.isFetching) {
				void entriesQuery.fetchNextPage();
			}
		}, { rootMargin: '400px' });
		observer.observe(target);
		return () => observer.disconnect();
	});

	function urlFor(folder: string | null, nextView: FilesView = viewMode): string {
		const params = new URLSearchParams();
		if (folder) params.set('folder_id', folder);
		if (nextView === 'grid') params.set('view', 'grid');
		const query = params.toString();
		return query ? `/files?${query}` : '/files';
	}

	function openFolder(id: string): void {
		void goto(urlFor(id));
	}

	function toggleSelection(entry: FileManagerEntry): void {
		const key = fileManagerEntryKey(entry);
		if (selectedIds.has(key)) selectedIds.delete(key);
		else selectedIds.add(key);
	}

	function toggleAllLoaded(): void {
		const keys = loadedEntries.map(fileManagerEntryKey);
		const deselect = keys.length > 0 && keys.every((key) => selectedIds.has(key));
		for (const key of keys) {
			if (deselect) selectedIds.delete(key);
			else selectedIds.add(key);
		}
	}

	function clearSelection(): void {
		selectedIds.clear();
	}

	function showContextMenu(entry: FileManagerEntry, x: number, y: number): void {
		contextMenuEntry = entry;
		contextMenuPosition = { x, y };
	}

	function previewImage(entry: FileManagerEntry, trigger: HTMLElement): void {
		imagePreviewRequest = { fileId: entry.id, trigger };
	}

	function previewVideo(entry: FileManagerEntry): void {
		videoPreviewEntry = entry;
		if (!VideoViewer) void import('./VideoViewer.svelte').then(({ default: component }) => VideoViewer = component);
	}

	function requestDelete(entry?: FileManagerEntry): void {
		if (entry) {
			const entryIsSelected = selectedIds.has(fileManagerEntryKey(entry));
			deleteTargets = entryIsSelected && selectedEntries.length ? selectedEntries : [entry];
		} else {
			deleteTargets = selectedEntries;
		}
		deleteDialogOpen = deleteTargets.length > 0;
	}

	async function deleteSelected(): Promise<void> {
		if (deleting || !deleteTargets.length) return;
		deleting = true;
		try {
			await api.post('/api/entries/delete', {
				fileIds: deleteTargets.filter((entry) => entry.kind === 'file').map((entry) => entry.id),
				folderIds: deleteTargets.filter((entry) => entry.kind === 'folder').map((entry) => entry.id)
			});
			selectedIds.clear();
			deleteTargets = [];
			deleteDialogOpen = false;
			toast.success('Items deleted.');
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['folder-entries'] }),
				queryClient.invalidateQueries({ queryKey: ['search'] })
			]);
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not delete selected items.');
		} finally {
			deleting = false;
		}
	}

	function changeView(view: FilesView): void {
		void goto(urlFor(folderId, view));
	}

	onMount(() => {
		try {
			const savedTileSize = localStorage.getItem('arkive.files.tile-size');
			if (savedTileSize !== null) {
				const parsedTileSize = Number(savedTileSize);
				if (Number.isFinite(parsedTileSize)) {
					tileSize = Math.min(260, Math.max(60, Math.round(parsedTileSize / 10) * 10));
				}
			}
			showType = localStorage.getItem('arkive.files.show-type') === 'true';
			showSize = localStorage.getItem('arkive.files.show-size') !== 'false';
		} catch {
			// Keep the default view options when local storage is unavailable.
		}
		viewOptionsReady = true;
		void waitUntilReady();
	});

	$effect(() => {
		if (!viewOptionsReady) return;
		try {
			localStorage.setItem('arkive.files.tile-size', String(tileSize));
			localStorage.setItem('arkive.files.show-type', String(showType));
			localStorage.setItem('arkive.files.show-size', String(showSize));
		} catch {
			// Preferences still work for the current page when local storage is unavailable.
		}
	});
</script>

<svelte:head>
	<title>Arkive · Files</title>
</svelte:head>

<div class="min-h-full bg-(--bg) py-6 pb-8">
	<div class="mx-auto grid w-[min(1440px,calc(100%-40px))] gap-5 max-[960px]:w-[calc(100%-24px)]">
		<header class="flex items-start justify-between gap-4 max-[640px]:flex-col">
			<h1 class="sr-only">{breadcrumbs.at(-1)?.name ?? 'Files'}</h1>
			<div class="flex min-w-0 flex-1 items-center gap-4 max-[640px]:w-full max-[640px]:flex-col max-[640px]:items-stretch">
				<div class="min-w-0 flex-1"><Breadcrumbs items={breadcrumbItems} label="Folder path" /></div>
				<div class="flex shrink-0 items-center justify-between gap-3">
					{#if (rows.length && viewMode === 'grid') || selectedEntries.length}
						<div class="inline-flex flex-wrap items-center gap-1 rounded-md border border-(--border) bg-(--surface-raised) p-1">
							{#if rows.length && viewMode === 'grid' && !allRowsSelected}
								<button class="inline-flex h-8 cursor-pointer items-center rounded px-2 text-sm text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" onclick={toggleAllLoaded}>
									Select all
								</button>
							{/if}
							{#if selectedEntries.length}
								<span class="px-2 text-sm text-(--muted)">{selectedEntries.length} selected</span>
								<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-red-400 hover:bg-red-500/10 hover:text-red-300" type="button" aria-label="Delete selected items" onclick={() => requestDelete()}>
									<Trash2 class="size-4" aria-hidden="true" />
								</button>
								<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" aria-label="Clear selection" onclick={clearSelection}>
									<X class="size-4" aria-hidden="true" />
								</button>
							{/if}
						</div>
					{/if}
					<FilesViewOptions
						view={viewMode}
						{tileSize}
						{showType}
						{showSize}
						onViewChange={changeView}
						onTileSizeChange={(size) => tileSize = size}
						onShowTypeChange={(show) => showType = show}
						onShowSizeChange={(show) => showSize = show}
					/>
				</div>
			</div>
		</header>

		{#if !$vaultUnlocked}
			<section class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)" role="status">
				Unlock the vault to view your files.
			</section>
		{:else if entriesQuery.isPending}
			<section class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)" role="status">
				Loading files…
			</section>
		{:else if entriesQuery.isError && !entriesQuery.data}
			<section class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-red-400" role="alert">
				{entriesQuery.error instanceof Error ? entriesQuery.error.message : 'Could not load files.'}
			</section>
		{:else if entriesQuery.data}
			<section class="overflow-hidden rounded-md border border-(--border) bg-(--surface-raised)" aria-label="Files and folders">
				{#if !entriesReady}
					<div class="px-5 py-8 text-(--muted)" role="status">Decrypting file details…</div>
				{:else if viewMode === 'grid'}
					<FilesGridView
						{rows}
						{selectedIds}
						{tileSize}
						{showType}
						{showSize}
						onToggleSelection={toggleSelection}
						onOpenFolder={openFolder}
						onShowContextMenu={showContextMenu}
						onPreviewImage={previewImage}
						onPreviewVideo={previewVideo}
					/>
				{:else}
					<FilesListView
						{rows}
						{selectedIds}
						selectedCount={selectedEntries.length}
						loadedCount={loadedEntries.length}
						onToggleSelection={toggleSelection}
						onToggleAll={toggleAllLoaded}
						onOpenFolder={openFolder}
						onShowContextMenu={showContextMenu}
						onPreviewImage={previewImage}
						onPreviewVideo={previewVideo}
					/>
				{/if}

				{#if entriesReady && entriesQuery.hasNextPage}
					<div bind:this={sentinel} class="flex flex-col items-center gap-2 border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)">
						{#if entriesQuery.isFetchNextPageError}
							<p class="m-0 text-red-400" role="alert">{entriesQuery.error instanceof Error ? entriesQuery.error.message : 'Could not load more files.'}</p>
						{/if}
						<button class="cursor-pointer rounded border border-(--border) px-4 py-2 disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={entriesQuery.isFetchingNextPage} onclick={() => void entriesQuery.fetchNextPage()}>
							{entriesQuery.isFetchingNextPage ? 'Loading more…' : 'Load more'}
						</button>
					</div>
				{:else if entriesReady}
					<footer class="border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)">
						{rows.length} {rows.length === 1 ? 'item' : 'items'}
					</footer>
				{/if}
			</section>
		{/if}
	</div>
</div>

<ImageViewer
	entries={imageEntries}
	request={imagePreviewRequest}
	onClose={() => imagePreviewRequest = null}
	onError={(message) => toast.error(message)}
/>

{#if VideoViewer}
	<VideoViewer entry={videoPreviewEntry} onClose={() => videoPreviewEntry = null} />
{/if}

<FileEntryContextMenu
	entry={contextMenuEntry}
	x={contextMenuPosition.x}
	y={contextMenuPosition.y}
	onClose={() => contextMenuEntry = null}
	onOpenFolder={openFolder}
	onProperties={(entry) => propertiesEntry = entry}
	onDelete={requestDelete}
/>

<FilePropertiesModal entry={propertiesEntry} location={currentLocation} onClose={() => propertiesEntry = null} />

<Modal
	open={deleteDialogOpen}
	label="Confirm delete"
	title={`Delete ${deleteTargets.length} ${deleteTargets.length === 1 ? 'item' : 'items'}?`}
	onClose={() => { if (!deleting) deleteDialogOpen = false; }}
>
	<p class="m-0 text-sm text-(--muted)">This action cannot be undone.</p>
	{#snippet actions()}
		<button class="cursor-pointer rounded border border-(--border) px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={deleting} onclick={() => deleteDialogOpen = false}>Cancel</button>
		<button class="cursor-pointer rounded border border-red-500 bg-red-500 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={deleting || !deleteTargets.length} onclick={() => void deleteSelected()}>
			{deleting ? 'Deleting…' : 'Delete'}
		</button>
	{/snippet}
</Modal>
