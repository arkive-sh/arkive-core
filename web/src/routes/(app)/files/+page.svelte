<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { createQuery } from '@tanstack/svelte-query';
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';
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
		Page: number;
		PageSize: number;
		TotalEntries: number;
		TotalPages: number;
		Path: ApiFolder[];
		Folders: ApiFolder[];
		Files: ApiFile[];
	};

	let { data }: PageProps = $props();
	let rows = $state<FileManagerEntry[]>([]);
	let breadcrumbs = $state<Array<{ id: string | null; name: string }>>([{ id: null, name: 'Files' }]);
	let decryptGeneration = 0;

	const folderId = $derived(page.url.searchParams.get('folder_id'));
	const pageNumber = $derived(Math.max(1, Number(page.url.searchParams.get('page')) || 1));
	const viewMode = $derived(page.url.searchParams.get('view') === 'grid' ? 'grid' : 'list');
	const entriesQuery = createQuery(() => ({
		queryKey: ['folder-entries', folderId, pageNumber],
		enabled: $vaultUnlocked,
		queryFn: async () => {
			const endpoint = folderId
				? `/api/folders/${encodeURIComponent(folderId)}/entries`
				: '/api/folders/root/entries';
			const { data: result } = await api.get<FolderEntriesResponse>(endpoint, {
				params: { page: pageNumber, pageSize: PAGE_SIZE }
			});
			return result;
		}
	}));

	const result = $derived(entriesQuery.data);

	async function decryptEntries(response: FolderEntriesResponse): Promise<void> {
		const generation = ++decryptGeneration;
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

		const path = await Promise.all(response.Path.map(async (folder) => {
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
		}));

		if (generation !== decryptGeneration || !$vaultUnlocked) return;
		rows = [...folderRows, ...fileRows];
		breadcrumbs = [{ id: null, name: 'Files' }, ...path];
	}

	$effect(() => {
		const response = result;
		if (!$vaultUnlocked || !response) {
			decryptGeneration += 1;
			rows = [];
			breadcrumbs = [{ id: null, name: 'Files' }];
			return;
		}
		void decryptEntries(response);
	});

	function urlFor(folder: string | null, nextPage = 1, nextView: FilesView = viewMode): string {
		const params = new URLSearchParams();
		if (folder) params.set('folder_id', folder);
		if (nextPage > 1) params.set('page', String(nextPage));
		if (nextView === 'grid') params.set('view', 'grid');
		const query = params.toString();
		return query ? `/files?${query}` : '/files';
	}

	function openFolder(id: string): void {
		void goto(urlFor(id));
	}

	function changeView(view: FilesView): void {
		void goto(urlFor(folderId, pageNumber, view));
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
		{:else if entriesQuery.isError}
			<section class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-red-400" role="alert">
				{entriesQuery.error instanceof Error ? entriesQuery.error.message : 'Could not load files.'}
			</section>
		{:else if result}
			<section class="overflow-hidden rounded-md border border-(--border) bg-(--surface-raised)" aria-label="Files and folders">
				{#if viewMode === 'grid'}
					<FilesGridView {rows} onOpenFolder={openFolder} />
				{:else}
					<FilesListView {rows} onOpenFolder={openFolder} />
				{/if}

				<footer class="flex flex-wrap items-center justify-between gap-3 border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)">
					<span>{result.TotalEntries} {result.TotalEntries === 1 ? 'item' : 'items'}</span>
					<div class="flex items-center gap-2">
						<span>Page {result.Page} of {Math.max(1, result.TotalPages)}</span>
						<button class="inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) disabled:cursor-not-allowed disabled:opacity-40" type="button" aria-label="Previous page" disabled={result.Page <= 1} onclick={() => void goto(urlFor(folderId, result.Page - 1))}>
							<ChevronLeft class="size-4" aria-hidden="true" />
						</button>
						<button class="inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) disabled:cursor-not-allowed disabled:opacity-40" type="button" aria-label="Next page" disabled={result.Page >= result.TotalPages} onclick={() => void goto(urlFor(folderId, result.Page + 1))}>
							<ChevronRight class="size-4" aria-hidden="true" />
						</button>
					</div>
				</footer>
			</section>
		{/if}
	</div>
</div>
