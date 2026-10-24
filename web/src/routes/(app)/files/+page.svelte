<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { Trash2, X } from '@lucide/svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
	import { api } from '#lib/api.ts';
	import Modal from '#lib/components/Modal.svelte';
	import { toast } from '#lib/toast.ts';
	import FilesGridView from './FilesGridView.svelte';
	import FilesListView from './FilesListView.svelte';
	import FilesViewToggle from './FilesViewToggle.svelte';
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
	let breadcrumbs = $state<Array<{ id: string | null; name: string }>>([{ id: null, name: 'Files' }]);
	let entriesReady = $state(false);
	let sentinel: HTMLDivElement | undefined = $state();
	let deleteDialogOpen = $state(false);
	let deleting = $state(false);
	const selectedIds = new SvelteSet<string>();
	const queryClient = useQueryClient();
	let decryptGeneration = 0;
	let hydratedPageCount = 0;
	let hydratedFolderId: string | null | undefined;
	let hydratedFirstPage: FolderEntriesResponse | undefined;
	let hydrationQueue = Promise.resolve();

	const folderId = $derived(page.url.searchParams.get('folder_id') || null);
	const viewMode = $derived(page.url.searchParams.get('view') === 'grid' ? 'grid' : 'list');
	const selectedEntries = $derived(rows.filter((entry) => selectedIds.has(fileManagerEntryKey(entry))));
	const allRowsSelected = $derived(rows.length > 0 && selectedEntries.length === rows.length);
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
					return { id: folder.ID, kind: 'folder', name: decrypted.name.name || 'Encrypted folder', type: 'Folder', size: null, updatedAt: folder.UpdatedAt };
				} catch {
					return { id: folder.ID, kind: 'folder', name: 'Encrypted folder', type: 'Folder', size: null, updatedAt: folder.UpdatedAt };
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
			breadcrumbs: path ? [{ id: null, name: 'Files' }, ...path] : undefined
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
			breadcrumbs = [{ id: null, name: 'Files' }];
			entriesReady = false;
			selectedIds.clear();
		}
		if (pages[0] && (pages[0].FolderID?.toLowerCase() ?? null) !== (activeFolderId?.toLowerCase() ?? null)) return;
		if (!$vaultUnlocked) {
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydrationQueue = Promise.resolve();
			rows = [];
			breadcrumbs = [{ id: null, name: 'Files' }];
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
			breadcrumbs = [{ id: null, name: 'Files' }];
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
		if (allRowsSelected) {
			for (const entry of rows) selectedIds.delete(fileManagerEntryKey(entry));
			return;
		}
		for (const entry of rows) selectedIds.add(fileManagerEntryKey(entry));
	}

	function clearSelection(): void {
		selectedIds.clear();
	}

	async function deleteSelected(): Promise<void> {
		if (deleting || !selectedEntries.length) return;
		deleting = true;
		try {
			await api.post('/api/entries/delete', {
				fileIds: selectedEntries.filter((entry) => entry.kind === 'file').map((entry) => entry.id),
				folderIds: selectedEntries.filter((entry) => entry.kind === 'folder').map((entry) => entry.id)
			});
			selectedIds.clear();
			deleteDialogOpen = false;
			toast.success('Selected items deleted.');
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
		void waitUntilReady();
	});
</script>

<svelte:head>
	<title>Arkive · Files</title>
</svelte:head>

<div class="min-h-full bg-(--bg) py-6 pb-8">
	<div class="mx-auto grid w-[min(1440px,calc(100%-40px))] gap-5 max-[960px]:w-[calc(100%-24px)]">
		<header class="flex items-start justify-between gap-4 max-[640px]:flex-col">
			<div class="grid gap-3">
				<h1 class="m-0 text-2xl leading-8 font-semibold">Files</h1>
				<nav class="flex flex-wrap items-center gap-2 text-sm text-(--muted)" aria-label="Folder path">
					{#each breadcrumbs as crumb, index (crumb.id ?? 'root')}
						{#if index > 0}<span aria-hidden="true">/</span>{/if}
						{#if index === breadcrumbs.length - 1}
							<span aria-current="page" class="text-(--text)">{crumb.name}</span>
						{:else}
							<a class="cursor-pointer hover:text-(--text)" href={urlFor(crumb.id)}>{crumb.name}</a>
						{/if}
					{/each}
				</nav>
			</div>
			<div class="flex flex-wrap items-center gap-3">
				{#if rows.length && viewMode === 'grid'}
					<button class="inline-flex h-9 cursor-pointer items-center rounded-md border border-(--border) px-3 text-sm text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" onclick={toggleAllLoaded}>
						{allRowsSelected ? 'Deselect all' : 'Select all'}
					</button>
				{/if}
				{#if selectedEntries.length}
					<div class="inline-flex items-center gap-1 rounded-md border border-(--border) bg-(--surface-raised) p-1">
						<span class="px-2 text-sm text-(--muted)">{selectedEntries.length} selected</span>
						<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-red-400 hover:bg-red-500/10 hover:text-red-300" type="button" aria-label="Delete selected items" title="Delete selected items" onclick={() => deleteDialogOpen = true}>
							<Trash2 class="size-4" aria-hidden="true" />
						</button>
						<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" aria-label="Clear selection" title="Clear selection" onclick={clearSelection}>
							<X class="size-4" aria-hidden="true" />
						</button>
					</div>
				{/if}
				<FilesViewToggle view={viewMode} onChange={changeView} />
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
					<FilesGridView {rows} {selectedIds} onToggleSelection={toggleSelection} onOpenFolder={openFolder} />
				{:else}
					<FilesListView {rows} {selectedIds} onToggleSelection={toggleSelection} onToggleAll={toggleAllLoaded} onOpenFolder={openFolder} />
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

<Modal
	open={deleteDialogOpen}
	label="Confirm delete"
	title={`Delete ${selectedEntries.length} selected ${selectedEntries.length === 1 ? 'item' : 'items'}?`}
	onClose={() => { if (!deleting) deleteDialogOpen = false; }}
>
	<p class="m-0 text-sm text-(--muted)">This action cannot be undone.</p>
	{#snippet actions()}
		<button class="cursor-pointer rounded border border-(--border) px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={deleting} onclick={() => deleteDialogOpen = false}>Cancel</button>
		<button class="cursor-pointer rounded border border-red-500 bg-red-500 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={deleting || !selectedEntries.length} onclick={() => void deleteSelected()}>
			{deleting ? 'Deleting…' : 'Delete'}
		</button>
	{/snippet}
</Modal>
