<script lang="ts">
	import { SvelteSet } from 'svelte/reactivity';
	import { createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
	import { RotateCcw, Trash2, X } from '@lucide/svelte';
	import { api } from '#lib/api.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import FileEntryContextMenu from '../files/FileEntryContextMenu.svelte';
	import FilesGridView from '../files/FilesGridView.svelte';
	import FilesListView from '../files/FilesListView.svelte';
	import FilesViewOptions from '../files/FilesViewOptions.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { toast } from '#lib/toast.ts';
	import { decryptSearchFile, decryptSearchFolder } from '#lib/vault/search.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import { fileManagerEntryKey, type FileManagerEntry, type FilesView } from '../files/types';
	import type { PageProps } from './$types';

	const PAGE_SIZE = 50;
	type ApiTrashEntry = {
		type: 'file' | 'folder';
		id: string;
		parentFolderId?: string | null;
		encryptedName?: string;
		encryptedMetadata?: string;
		encryptedFileKey?: string;
		deletedAt: string;
		isContainer?: boolean;
	};
	type TrashPage = { entries: ApiTrashEntry[]; nextCursor: string | null; hasMore: boolean };
	type LoadedTrashPage = TrashPage & { folderId: string | null };
	type SelectionResult = { fileIds: string[]; folderIds: string[] };
	type BreadcrumbItem = { id: string; label: string; onSelect?: () => void };

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();
	const selectedIds = new SvelteSet<string>();
	let folderPath = $state<FileManagerEntry[]>([]);
	const currentFolderId = $derived(folderPath.at(-1)?.id ?? null);
	const query = createInfiniteQuery(() => ({
		queryKey: ['trash', currentFolderId],
		enabled: $vaultUnlocked,
		initialPageParam: null as string | null,
		queryFn: async ({ pageParam, signal }): Promise<LoadedTrashPage> => {
			const { data: result } = await api.get<TrashPage>('/api/trash/entries', {
				params: {
					limit: PAGE_SIZE,
					cursor: pageParam ?? undefined,
					parentFolderId: currentFolderId ?? undefined,
				},
				signal,
			});

			return { ...result, folderId: currentFolderId };
		},
		getNextPageParam: (lastPage) =>
			lastPage.hasMore ? lastPage.nextCursor || undefined : undefined,
	}));

	let entries = $state<FileManagerEntry[]>([]);
	let entriesReady = $state(false);
	let hydratedPageCount = 0;
	let hydratedFolderId: string | null | undefined;
	let firstPage: LoadedTrashPage | undefined;
	let hydrationGeneration = 0;
	let hydrationQueue = Promise.resolve();
	let selectedTargets = $state<FileManagerEntry[]>([]);
	let permanentDeleteContainer = $state<FileManagerEntry | null>(null);
	let permanentDeleteOpen = $state(false);
	let mutating = $state(false);
	let contextMenuOpen = $state(false);
	let contextMenuEntry = $state<FileManagerEntry | null>(null);
	let contextMenuPosition = $state({ x: 0, y: 0 });
	let viewMode = $state<FilesView>('list');
	let tileSize = $state(200);
	let showType = $state(false);
	let showSize = $state(true);
	let showDimensions = $state(false);
	let viewOptionsReady = $state(false);

	const selectedEntries = $derived(
		entries.filter(
			(entry) => entry.selectable !== false && selectedIds.has(fileManagerEntryKey(entry)),
		),
	);
	const selectableCount = $derived(entries.filter((entry) => entry.selectable !== false).length);
	const allSelected = $derived(selectableCount > 0 && selectedEntries.length === selectableCount);
	const breadcrumbItems = $derived<BreadcrumbItem[]>([
		{ id: 'trash', label: 'Trash', onSelect: () => (folderPath = []) },
		...folderPath.map((folder, index) => ({
			id: folder.id,
			label: folder.name,
			onSelect: () => (folderPath = folderPath.slice(0, index + 1)),
		})),
	]);

	async function decryptEntry(entry: ApiTrashEntry): Promise<FileManagerEntry> {
		try {
			if (entry.type === 'folder') {
				if (!entry.encryptedName || !entry.encryptedMetadata)
					throw new Error('Encrypted folder details are missing.');
				const decrypted = await decryptSearchFolder({
					id: entry.id,
					parentFolderId: entry.parentFolderId ?? null,
					vaultId: data.user.id,
					encryptedName: entry.encryptedName,
					encryptedMetadata: entry.encryptedMetadata,
					score: 0,
					url: `/folders/${entry.id}`,
				});

				return {
					id: entry.id,
					kind: 'folder',
					name: decrypted.name.name || 'Encrypted folder',
					type: 'Folder',
					size: null,
					updatedAt: entry.deletedAt,
					selectable: !entry.isContainer,
					isTrashContainer: entry.isContainer,
				};
			}

			if (!entry.encryptedMetadata || !entry.encryptedFileKey)
				throw new Error('Encrypted file details are missing.');
			const decrypted = await decryptSearchFile({
				id: entry.id,
				folderId: entry.parentFolderId ?? null,
				vaultId: data.user.id,
				encryptedMetadata: entry.encryptedMetadata,
				encryptedFileKey: entry.encryptedFileKey,
				score: 0,
				url: `/files/${entry.id}/view`,
			});

			return {
				id: entry.id,
				kind: 'file',
				name: decrypted.metadata.name || 'Encrypted file',
				type:
					typeof decrypted.metadata.mime === 'string'
						? decrypted.metadata.mime
						: 'application/octet-stream',
				size: typeof decrypted.metadata.size === 'number' ? decrypted.metadata.size : null,
				updatedAt: entry.deletedAt,
			};
		} catch {
			return {
				id: entry.id,
				kind: entry.type,
				name: entry.type === 'folder' ? 'Encrypted folder' : 'Encrypted file',
				type: entry.type === 'folder' ? 'Folder' : 'application/octet-stream',
				size: null,
				updatedAt: entry.deletedAt,
				selectable: entry.type !== 'folder' || !entry.isContainer,
				isTrashContainer: entry.type === 'folder' && entry.isContainer,
			};
		}
	}

	$effect(() => {
		const pages = query.data?.pages ?? [];
		const activeFolderId = currentFolderId;

		if (hydratedFolderId !== activeFolderId) {
			hydratedFolderId = activeFolderId;
			hydrationGeneration += 1;
			hydratedPageCount = 0;
			firstPage = undefined;
			hydrationQueue = Promise.resolve();
			entries = [];
			entriesReady = false;
			selectedIds.clear();
		}

		if (!$vaultUnlocked) {
			hydrationGeneration += 1;
			hydratedPageCount = 0;
			firstPage = undefined;
			hydrationQueue = Promise.resolve();
			entries = [];
			entriesReady = false;
			selectedIds.clear();
			selectedTargets = [];
			permanentDeleteContainer = null;
			permanentDeleteOpen = false;
			closeContextMenu();

			return;
		}

		if (!pages.length || pages[0].folderId !== activeFolderId) return;
		if (pages.length < hydratedPageCount || pages[0] !== firstPage) {
			hydrationGeneration += 1;
			hydratedPageCount = 0;
			firstPage = pages[0];
			hydrationQueue = Promise.resolve();
			entries = [];
			entriesReady = false;
			selectedIds.clear();
		}

		const generation = hydrationGeneration;
		const newPages = pages.slice(hydratedPageCount);
		hydratedPageCount = pages.length;
		for (const page of newPages) {
			hydrationQueue = hydrationQueue.then(async () => {
				const decrypted = await Promise.all(page.entries.map(decryptEntry));

				if (generation !== hydrationGeneration || !$vaultUnlocked) return;
				entries = [...entries, ...decrypted];
				entriesReady = true;
			});
		}
	});

	$effect(() => {
		if (viewOptionsReady) return;
		try {
			viewMode = localStorage.getItem('arkive.files.view') === 'grid' ? 'grid' : 'list';
			tileSize = Number(localStorage.getItem('arkive.files.tile-size')) || 200;
			showType = localStorage.getItem('arkive.files.show-type') === 'true';
			showSize = localStorage.getItem('arkive.files.show-size') !== 'false';
			showDimensions = localStorage.getItem('arkive.files.show-dimensions') === 'true';
		} catch {
			// Keep defaults if view preferences are unavailable.
		}
		viewOptionsReady = true;
	});

	$effect(() => {
		if (!viewOptionsReady) return;
		try {
			localStorage.setItem('arkive.files.view', viewMode);
			localStorage.setItem('arkive.files.tile-size', String(tileSize));
			localStorage.setItem('arkive.files.show-type', String(showType));
			localStorage.setItem('arkive.files.show-size', String(showSize));
			localStorage.setItem('arkive.files.show-dimensions', String(showDimensions));
		} catch {
			// Preferences still work for the current Trash page if storage is unavailable.
		}
	});

	function selectionPayload(targets: FileManagerEntry[]): SelectionResult {
		return {
			fileIds: targets.filter((entry) => entry.kind === 'file').map((entry) => entry.id),
			folderIds: targets.filter((entry) => entry.kind === 'folder').map((entry) => entry.id),
		};
	}

	function toggleEntry(entry: FileManagerEntry): void {
		if (entry.selectable === false) return;
		const key = fileManagerEntryKey(entry);

		if (selectedIds.has(key)) selectedIds.delete(key);
		else selectedIds.add(key);
	}

	function toggleAll(): void {
		if (allSelected) selectedIds.clear();
		else {
			for (const entry of entries) {
				if (entry.selectable !== false) selectedIds.add(fileManagerEntryKey(entry));
			}
		}
	}

	function openFolder(id: string): void {
		const folder = entries.find((entry) => entry.kind === 'folder' && entry.id === id);

		if (!folder) return;
		folderPath = [...folderPath, folder];
		closeContextMenu();
	}

	function openContextMenu(entry: FileManagerEntry | null, x: number, y: number): void {
		if (!entry) return;
		contextMenuEntry = entry;
		contextMenuPosition = { x, y };
		contextMenuOpen = true;
	}

	function closeContextMenu(): void {
		contextMenuOpen = false;
		contextMenuEntry = null;
	}

	function requestPermanentDelete(targets: FileManagerEntry[]): void {
		if (mutating || !targets.length) return;

		selectedTargets = targets;
		permanentDeleteContainer = null;
		permanentDeleteOpen = true;
		closeContextMenu();
	}

	function requestPermanentDeleteContents(entry: FileManagerEntry): void {
		if (mutating || !entry.isTrashContainer) return;

		selectedTargets = [];
		permanentDeleteContainer = entry;
		permanentDeleteOpen = true;
		closeContextMenu();
	}

	async function restore(targets: FileManagerEntry[]): Promise<void> {
		if (mutating || !targets.length) return;
		mutating = true;
		try {
			const { data: result } = await api.post<{ restoredFiles: number; restoredFolders: number }>(
				'/api/trash/restore',
				selectionPayload(targets),
			);
			selectedIds.clear();
			toast.success(`Restored ${result.restoredFiles + result.restoredFolders} items.`);
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['trash'] }),
				queryClient.invalidateQueries({ queryKey: ['folder-entries'] }),
				queryClient.invalidateQueries({ queryKey: ['search'] }),
			]);
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not restore selected items.');
		} finally {
			mutating = false;
		}
	}

	async function restoreContainerContents(entry: FileManagerEntry): Promise<void> {
		if (mutating || !entry.isTrashContainer) return;
		mutating = true;
		try {
			const { data: result } = await api.post<{ restoredFiles: number; restoredFolders: number }>(
				'/api/trash/restore',
				{ containerFolderIds: [entry.id] },
			);
			toast.success(`Restored ${result.restoredFiles + result.restoredFolders} items.`);
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['trash'] }),
				queryClient.invalidateQueries({ queryKey: ['folder-entries'] }),
				queryClient.invalidateQueries({ queryKey: ['search'] }),
			]);
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not restore Trash contents.');
		} finally {
			mutating = false;
		}
	}

	function contextTargets(entry: FileManagerEntry): FileManagerEntry[] {
		return selectedIds.has(fileManagerEntryKey(entry)) && selectedEntries.length
			? selectedEntries
			: [entry];
	}

	async function permanentlyDelete(): Promise<void> {
		if (mutating || (!selectedTargets.length && !permanentDeleteContainer)) return;
		const container = permanentDeleteContainer;
		mutating = true;
		try {
			await api.post(
				'/api/trash/permanent-delete',
				container ? { containerFolderIds: [container.id] } : selectionPayload(selectedTargets),
			);
			selectedIds.clear();
			selectedTargets = [];
			permanentDeleteContainer = null;
			permanentDeleteOpen = false;
			toast.success(
				container
					? 'Remaining Trash items permanently deleted. Storage cleanup is queued.'
					: 'Permanently deleted. Storage cleanup is queued.',
			);
			await queryClient.invalidateQueries({ queryKey: ['trash'] });
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not permanently delete items.');
		} finally {
			mutating = false;
		}
	}

	function closePermanentDelete(): void {
		if (mutating) return;
		permanentDeleteOpen = false;
		selectedTargets = [];
		permanentDeleteContainer = null;
	}
</script>

<svelte:head>
	<title>Arkive · Trash</title>
</svelte:head>

<div class="min-h-full bg-(--bg) py-6 pb-8">
	<div class="mx-auto grid w-[min(1440px,calc(100%-40px))] gap-5 max-[960px]:w-[calc(100%-24px)]">
		<header
			class="flex items-center justify-between gap-4 rounded-md border border-(--border) bg-(--surface-raised) px-3 py-2.5 max-[640px]:flex-col max-[640px]:items-stretch"
		>
			<div class="flex min-w-0 flex-1 items-center gap-2 max-[640px]:w-full">
				<Trash2 class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
				<div class="min-w-0 flex-1">
					<Breadcrumbs items={breadcrumbItems} label="Trash path" />
				</div>
			</div>
			<div
				class="flex shrink-0 flex-wrap items-center justify-end gap-2 max-[640px]:w-full max-[640px]:justify-between"
			>
				{#if selectedEntries.length > 0 || (viewMode === 'grid' && selectableCount > 0)}
					<div
						class="inline-flex max-w-full flex-wrap items-center gap-1 rounded-md border border-(--border) bg-(--surface) p-1"
					>
						{#if viewMode === 'grid' && !allSelected}
							<button
								class="inline-flex h-8 cursor-pointer items-center rounded px-2 text-sm text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
								type="button"
								onclick={toggleAll}>Select loaded</button
							>
						{/if}
						{#if selectedEntries.length}
							<span class="px-2 text-sm text-(--muted)">{selectedEntries.length} selected</span>
							<button
								class="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded px-2 text-sm text-(--text) hover:bg-(--surface-highest) disabled:cursor-not-allowed disabled:opacity-50"
								type="button"
								disabled={mutating}
								onclick={() => void restore(selectedEntries)}
								><RotateCcw class="size-4" aria-hidden="true" />Restore</button
							>
							<button
								class="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded px-2 text-sm text-red-400 hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-50"
								type="button"
								disabled={mutating}
								onclick={() => requestPermanentDelete(selectedEntries)}
								><Trash2 class="size-4" aria-hidden="true" />Delete permanently</button
							>
							<button
								class="inline-flex size-8 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
								type="button"
								aria-label="Clear selection"
								onclick={() => selectedIds.clear()}><X class="size-4" aria-hidden="true" /></button
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
					onViewChange={(view) => (viewMode = view)}
					onTileSizeChange={(size) => (tileSize = size)}
					onShowTypeChange={(show) => (showType = show)}
					onShowSizeChange={(show) => (showSize = show)}
					onShowDimensionsChange={(show) => (showDimensions = show)}
				/>
			</div>
		</header>

		{#if !$vaultUnlocked}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)"
				role="status"
			>
				Unlock the vault from the header to view Trash.
			</section>
		{:else if query.isPending}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)"
				role="status"
			>
				Loading Trash…
			</section>
		{:else if query.isError && !query.data}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-red-400"
				role="alert"
			>
				{query.error instanceof Error ? query.error.message : 'Could not load Trash.'}
			</section>
		{:else if !entriesReady}
			<section
				class="rounded-md border border-(--border) bg-(--surface-raised) px-5 py-8 text-(--muted)"
				role="status"
			>
				Decrypting Trash entries…
			</section>
		{:else}
			<section
				class="overflow-hidden rounded-md border border-(--border) bg-(--surface-raised)"
				aria-label="Trash entries"
			>
				{#if entries.length === 0}
					<div class="px-5 py-8 text-center text-sm text-(--muted)" role="status">
						{currentFolderId ? 'This folder is empty.' : 'Trash is empty.'}
					</div>
				{:else if viewMode === 'grid'}
					<FilesGridView
						rows={entries}
						focusFileId={null}
						{selectedIds}
						selectionEnabled={true}
						previewEnabled={false}
						{tileSize}
						{showType}
						{showSize}
						{showDimensions}
						creatingFolder={false}
						onCreateFolder={() => undefined}
						onCancelNewFolder={() => undefined}
						onToggleSelection={toggleEntry}
						onOpenFolder={openFolder}
						onShowContextMenu={openContextMenu}
						onPreviewImage={() => undefined}
						onPreviewMedia={() => undefined}
						onPreviewPdf={() => undefined}
						onPreviewText={() => undefined}
					/>
				{:else}
					<FilesListView
						rows={entries}
						focusFileId={null}
						{selectedIds}
						selectionEnabled={true}
						previewEnabled={false}
						dateColumnLabel="Deleted"
						selectedCount={selectedEntries.length}
						loadedCount={selectableCount}
						showDimensions={false}
						creatingFolder={false}
						onCreateFolder={() => undefined}
						onCancelNewFolder={() => undefined}
						onToggleSelection={toggleEntry}
						onToggleAll={toggleAll}
						onOpenFolder={openFolder}
						onShowContextMenu={openContextMenu}
						onPreviewImage={() => undefined}
						onPreviewMedia={() => undefined}
						onPreviewPdf={() => undefined}
						onPreviewText={() => undefined}
					/>
				{/if}
				{#if query.data?.pages.at(-1)?.hasMore}
					<div
						class="flex flex-col items-center gap-2 border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)"
					>
						{#if query.isFetchNextPageError}<p class="m-0 text-red-400" role="alert">
								Could not load more Trash entries.
							</p>{/if}
						<button
							class="cursor-pointer rounded border border-(--border) px-4 py-2 disabled:cursor-not-allowed disabled:opacity-50"
							type="button"
							disabled={query.isFetchingNextPage}
							onclick={() => void query.fetchNextPage()}
							>{query.isFetchingNextPage ? 'Loading more…' : 'Load more'}</button
						>
					</div>
				{:else}
					<footer class="border-t border-(--border-soft) px-5 py-3 text-sm text-(--muted)">
						{entries.length}
						{entries.length === 1 ? 'item' : 'items'}
					</footer>
				{/if}
			</section>
		{/if}
	</div>
</div>

<FileEntryContextMenu
	mode="trash"
	canMutateEntry={contextMenuEntry?.selectable !== false}
	open={contextMenuOpen}
	entry={contextMenuEntry}
	x={contextMenuPosition.x}
	y={contextMenuPosition.y}
	onClose={closeContextMenu}
	onOpenFolder={openFolder}
	onDownload={() => undefined}
	onPreviewImage={() => undefined}
	onPreviewMedia={() => undefined}
	onPreviewPdf={() => undefined}
	onPreviewText={() => undefined}
	onProperties={() => undefined}
	onDelete={() => undefined}
	onCreateFolder={() => undefined}
	onCreateTextFile={() => undefined}
	onUploadFiles={() => undefined}
	onUploadFolder={() => undefined}
	onRestore={(entry) => void restore(contextTargets(entry))}
	onRestoreContents={(entry) => void restoreContainerContents(entry)}
	onPermanentDelete={(entry) => requestPermanentDelete(contextTargets(entry))}
	onPermanentDeleteContents={requestPermanentDeleteContents}
/>

<Modal
	open={permanentDeleteOpen}
	label="Confirm permanent deletion"
	title={permanentDeleteContainer
		? `Permanently delete remaining items in ${permanentDeleteContainer.name}?`
		: `Permanently delete ${selectedTargets.length} ${selectedTargets.length === 1 ? 'item' : 'items'}?`}
	onClose={closePermanentDelete}
>
	<p class="m-0 text-sm text-(--muted)">
		{#if permanentDeleteContainer}
			This cannot be undone. All remaining deleted files and folders inside this folder will be
			removed from Trash and permanently deleted.
		{:else}
			This cannot be undone. The items will be removed from Trash and the encrypted objects will be
			permanently deleted.
		{/if}
	</p>
	{#snippet actions()}
		<button
			class="cursor-pointer rounded border border-(--border) px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
			type="button"
			disabled={mutating}
			onclick={closePermanentDelete}>Cancel</button
		>
		<button
			class="cursor-pointer rounded border border-red-500 bg-red-500 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
			type="button"
			disabled={mutating || (!selectedTargets.length && !permanentDeleteContainer)}
			onclick={() => void permanentlyDelete()}
			>{mutating ? 'Deleting…' : 'Delete permanently'}</button
		>
	{/snippet}
</Modal>
