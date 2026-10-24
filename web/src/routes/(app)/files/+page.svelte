<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { createInfiniteQuery } from '@tanstack/svelte-query';
	import { api } from '#lib/api.ts';
	import FilesGridView from './FilesGridView.svelte';
	import FilesListView from './FilesListView.svelte';
	import FilesViewToggle from './FilesViewToggle.svelte';
	import type { FileManagerEntry, FilesView } from './types';
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
			<FilesViewToggle view={viewMode} onChange={changeView} />
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
					<FilesGridView {rows} onOpenFolder={openFolder} />
				{:else}
					<FilesListView {rows} onOpenFolder={openFolder} />
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
