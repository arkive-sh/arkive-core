<script lang="ts">
	import { onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { Folder, LayoutGrid, LayoutList } from '@lucide/svelte';
	import { api } from '#lib/api.ts';
	import FolderTree from '#lib/components/FolderTree.svelte';
	import InfiniteScrollSentinel from '#lib/components/InfiniteScrollSentinel.svelte';
	import SidebarResizeHandle from '#lib/components/SidebarResizeHandle.svelte';
	import SidebarToggle from '#lib/components/SidebarToggle.svelte';
	import {
		createPublicShareFolderTreeSource,
		type PublicFolderEntries,
	} from '#lib/components/folder-tree.svelte.ts';
	import { decodeJSON } from '#lib/vault/bytes.ts';
	import { openPublicShareFileMetadata, openPublicShareFolder } from '#lib/vault/shares.ts';
	import { isImageFile } from '#lib/utils/image.ts';
	import type { VaultMetadata } from '#lib/vault/types.ts';
	import FileEntryContextMenu from '../../(app)/files/FileEntryContextMenu.svelte';
	import FilesGridView from '../../(app)/files/FilesGridView.svelte';
	import FilesListView from '../../(app)/files/FilesListView.svelte';
	import type { FileManagerEntry, FilesView } from '../../(app)/files/types';

	type Props = {
		share: {
			token: string;
			folderId: string;
			encryptedFolderName: string;
			encryptedFolderMetadata: string;
		};
		shareKey: Uint8Array;
		allowPreview: boolean;
		allowDownload: boolean;
		error?: string;
		onOpenFile: (entry: FileManagerEntry, imageEntries: FileManagerEntry[]) => void | Promise<void>;
		onDownloadFile: (entry: FileManagerEntry) => void;
	};

	let {
		share,
		shareKey,
		allowPreview,
		allowDownload,
		error = $bindable(''),
		onOpenFile,
		onDownloadFile,
	}: Props = $props();

	let folderPath = $state<Array<{ id: string; name: string }>>([]);
	let folderEntries = $state<FileManagerEntry[]>([]);
	const noSelection = new SvelteSet<string>();
	let folderView = $state<FilesView>('list');
	let folderCursor = $state<string | null>(null);
	let folderLoading = $state(true);
	let folderTreeOpen = $state(true);
	let folderTreeWidth = $state(200);
	let contextMenuEntry = $state<FileManagerEntry | null>(null);
	let contextMenuPosition = $state({ x: 0, y: 0 });
	let contextMenuOpen = $derived(contextMenuEntry !== null);
	const treeSource = createPublicShareFolderTreeSource(
		() => share.token,
		() => shareKey,
		decryptFolderName,
	);

	async function decryptFolderName(
		folderId: string,
		encryptedName: string,
		encryptedMetadata: string,
	): Promise<string> {
		const decrypted = await openPublicShareFolder(
			folderId,
			share.token,
			encryptedName,
			encryptedMetadata,
			shareKey.slice(),
		);
		const nameMetadata = decodeJSON<VaultMetadata>(decrypted.nameBytes);
		decodeJSON<VaultMetadata>(decrypted.metadataBytes);

		return typeof nameMetadata.name === 'string' ? nameMetadata.name : 'Shared folder';
	}

	async function loadFolderEntries(folderId: string, append = false): Promise<void> {
		folderLoading = true;
		error = '';
		if (!append) {
			folderEntries = [];
			folderCursor = null;
		}
		try {
			const { data } = await api.get<PublicFolderEntries>(
				`/api/public/shares/${encodeURIComponent(share.token)}/folders/${encodeURIComponent(folderId)}/entries`,
				{
					params: { pageSize: 50, cursor: append ? (folderCursor ?? undefined) : undefined },
				},
			);
			const folders = await Promise.all(
				data.folders.map(async (folder): Promise<FileManagerEntry> => ({
					id: folder.id,
					kind: 'folder',
					name: await decryptFolderName(folder.id, folder.encryptedName, folder.encryptedMetadata),
					type: 'Folder',
					size: null,
					updatedAt: folder.updatedAt,
				})),
			);
			const files = await Promise.all(
				data.files.map(async (file): Promise<FileManagerEntry> => {
					const decrypted = await openPublicShareFileMetadata(
						{
							token: share.token,
							fileId: file.id,
							vaultId: file.vaultId,
							encryptedFileKeyForShare: file.encryptedFileKeyForShare,
							encryptedMetadata: file.encryptedMetadata,
						},
						shareKey.slice(),
					);
					const metadata = decodeJSON<VaultMetadata>(decrypted.metadataBytes);

					return {
						id: file.id,
						kind: 'file',
						name: typeof metadata.name === 'string' ? metadata.name : 'Shared file',
						type: typeof metadata.mime === 'string' ? metadata.mime : 'application/octet-stream',
						size: typeof metadata.size === 'number' ? metadata.size : null,
						updatedAt: file.updatedAt,
					};
				}),
			);
			const nextEntries = [...folders, ...files].sort((a, b) => a.name.localeCompare(b.name));
			folderEntries = append ? [...folderEntries, ...nextEntries] : nextEntries;
			folderCursor = data.nextCursor || null;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load this folder.';

			throw cause;
		} finally {
			folderLoading = false;
		}
	}

	async function openFolder(entry: FileManagerEntry): Promise<void> {
		try {
			await loadFolderEntries(entry.id);
			folderPath = [...folderPath, { id: entry.id, name: entry.name }];
		} catch {
			// The error is shown by the folder view.
		}
	}

	function openFolderById(folderId: string): void {
		const entry = folderEntries.find(
			(candidate) => candidate.kind === 'folder' && candidate.id === folderId,
		);

		if (entry) void openFolder(entry);
	}

	async function openTreeFolder(folderId: string, path: FileManagerEntry[]): Promise<void> {
		const root = folderPath[0];

		try {
			await loadFolderEntries(folderId);
			if (root) folderPath = [root, ...path];
		} catch {
			// The error is shown by the folder view.
		}
	}

	async function openTreeFile(entry: FileManagerEntry, path: FileManagerEntry[]): Promise<void> {
		const folderId = entry.parentFolderId ?? path.at(-1)?.id ?? share.folderId;
		const root = folderPath[0];

		try {
			await loadFolderEntries(folderId);
			if (root) folderPath = [root, ...path];
			const selected = folderEntries.find(
				(candidate) => candidate.kind === 'file' && candidate.id === entry.id,
			);

			if (!selected) return;
			if (allowPreview) openFile(selected);
			else if (allowDownload) onDownloadFile(selected);
		} catch {
			// The error is shown by the folder view.
		}
	}

	async function navigateToBreadcrumb(index: number): Promise<void> {
		const destination = folderPath[index];

		if (!destination || index === folderPath.length - 1) return;
		try {
			await loadFolderEntries(destination.id);
			folderPath = folderPath.slice(0, index + 1);
		} catch {
			// The error is shown by the folder view.
		}
	}

	function showContextMenu(entry: FileManagerEntry | null, x: number, y: number): void {
		contextMenuEntry = entry;
		contextMenuPosition = { x, y };
	}

	function openFile(entry: FileManagerEntry): void {
		const imageEntries = folderEntries.filter(
			(candidate) => candidate.kind === 'file' && isImageFile(candidate.name, candidate.type),
		);
		void onOpenFile(entry, imageEntries);
	}

	function loadMore(): void {
		const currentFolder = folderPath.at(-1);

		if (currentFolder) void loadFolderEntries(currentFolder.id, true).catch(() => {});
	}

	function persistTreeWidth(width: number): void {
		folderTreeWidth = width;
		localStorage.setItem('arkive.sidebar.width', String(width));
	}

	onMount(() => {
		const savedWidth = Number(localStorage.getItem('arkive.sidebar.width'));

		if (savedWidth >= 160 && savedWidth <= 320) folderTreeWidth = savedWidth;

		void (async () => {
			try {
				const name = await decryptFolderName(
					share.folderId,
					share.encryptedFolderName,
					share.encryptedFolderMetadata,
				);
				folderPath = [{ id: share.folderId, name }];
				await loadFolderEntries(share.folderId);
			} catch (cause) {
				error = cause instanceof Error ? cause.message : 'Could not open this folder.';
				folderLoading = false;
			}
		})();
	});
</script>

<section class="flex min-h-0 flex-1 flex-col">
	<nav
		class="flex min-w-0 items-center gap-3 border-b border-[#30363d] py-3 text-sm"
		aria-label="Folder path"
	>
		<SidebarToggle
			open={folderTreeOpen}
			controls="public-shared-folder-tree"
			label={folderTreeOpen ? 'Hide folder tree' : 'Show folder tree'}
			onToggle={() => (folderTreeOpen = !folderTreeOpen)}
		/>
		<div class="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
			<Folder class="size-5 shrink-0 text-sky-400" aria-hidden="true" />
			{#each folderPath as crumb, index (crumb.id)}
				{#if index > 0}<span class="text-[#6f7983]" aria-hidden="true">/</span>{/if}
				{#if index < folderPath.length - 1}
					<button
						class="max-w-52 shrink-0 cursor-pointer truncate border-0 bg-transparent p-0 text-[#aeb8c2] hover:text-white"
						type="button"
						onclick={() => navigateToBreadcrumb(index)}>{crumb.name}</button
					>
				{:else}
					<span class="max-w-52 shrink-0 truncate text-[#e5eaef]" aria-current="page"
						>{crumb.name}</span
					>
				{/if}
			{/each}
		</div>
		<span class="shrink-0 text-[#87929a] max-[520px]:hidden">Read-only</span>
		<div
			class="inline-flex shrink-0 items-center rounded-md border border-[#30363d] bg-[#161b22] p-1"
			role="group"
			aria-label="Folder view"
		>
			<button
				class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${folderView === 'list' ? 'bg-[#30363d] text-[#e5eaef]' : 'text-[#87929a] hover:text-white'}`}
				type="button"
				aria-label="List view"
				aria-pressed={folderView === 'list'}
				onclick={() => (folderView = 'list')}
			>
				<LayoutList class="size-4.5" aria-hidden="true" />
			</button>
			<button
				class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${folderView === 'grid' ? 'bg-[#30363d] text-[#e5eaef]' : 'text-[#87929a] hover:text-white'}`}
				type="button"
				aria-label="Grid view"
				aria-pressed={folderView === 'grid'}
				onclick={() => (folderView = 'grid')}
			>
				<LayoutGrid class="size-4.5" aria-hidden="true" />
			</button>
		</div>
	</nav>
	{#if error}<p class="m-0 border-b border-[#30363d] py-3 text-red-400" role="alert">
			{error}
		</p>{/if}
	<div
		class={`grid min-h-0 flex-1 transition-[grid-template-columns,gap] duration-200 ease-in-out motion-reduce:transition-none ${folderTreeOpen ? 'gap-4 grid-cols-[var(--public-tree-width)_minmax(0,1fr)] max-[700px]:grid-cols-1' : 'gap-0 grid-cols-[0px_minmax(0,1fr)] max-[700px]:grid-cols-1'}`}
		style={`--public-tree-width:${folderTreeWidth}px`}
	>
		<aside
			id="public-shared-folder-tree"
			class={`relative min-h-0 min-w-0 border-(--border) transition-[max-height,padding] duration-200 ease-in-out motion-reduce:transition-none ${folderTreeOpen ? 'overflow-y-auto border-r px-3 py-3 max-[700px]:max-h-48 max-[700px]:border-r-0 max-[700px]:border-b' : 'overflow-hidden border-r-0 px-0 py-0 max-[700px]:max-h-0 max-[700px]:border-b-0'}`}
			aria-label="Shared folder tree"
			aria-hidden={!folderTreeOpen}
			inert={!folderTreeOpen}
			data-folder-tree-scroll-root
		>
			<FolderTree
				source={treeSource}
				enabled={Boolean(folderPath.length)}
				rootFolderId={share.folderId}
				rootName={folderPath[0]?.name ?? 'Shared folder'}
				showRoot
				currentFolderId={folderPath.at(-1)?.id ?? share.folderId}
				currentFileId={null}
				onOpenFolder={openTreeFolder}
				onOpenFile={openTreeFile}
			/>
			{#if folderTreeOpen}
				<SidebarResizeHandle
					width={folderTreeWidth}
					onResize={(width) => (folderTreeWidth = width)}
					onResizeCommit={persistTreeWidth}
				/>
			{/if}
		</aside>
		<div class="flex min-h-0 min-w-0 flex-col">
			{#if folderEntries.length === 0 && folderLoading}
				<p class="py-8 text-center text-[#87929a]" role="status">Loading folder…</p>
			{:else if folderView === 'grid'}
				<FilesGridView
					rows={folderEntries}
					focusFileId={null}
					selectedIds={noSelection}
					selectionEnabled={false}
					previewEnabled={allowPreview}
					tileSize={200}
					showType={false}
					showSize={true}
					showDimensions={false}
					creatingFolder={false}
					onOpenFolder={openFolderById}
					onShowContextMenu={showContextMenu}
					onPreviewImage={openFile}
					onPreviewMedia={openFile}
					onPreviewPdf={openFile}
					onPreviewText={openFile}
				/>
			{:else}
				<FilesListView
					rows={folderEntries}
					focusFileId={null}
					selectedIds={noSelection}
					selectionEnabled={false}
					previewEnabled={allowPreview}
					selectedCount={0}
					loadedCount={folderEntries.length}
					showDimensions={false}
					creatingFolder={false}
					onOpenFolder={openFolderById}
					onShowContextMenu={showContextMenu}
					onPreviewImage={openFile}
					onPreviewMedia={openFile}
					onPreviewPdf={openFile}
					onPreviewText={openFile}
				/>
			{/if}
			{#if folderCursor}
				<div class="flex justify-center border-t border-[#30363d] py-4">
					{#if error}
						<button
							class="cursor-pointer rounded border border-[#30363d] bg-transparent px-4 py-2 text-[#dee3e8]"
							type="button"
							onclick={loadMore}>Retry loading items</button
						>
					{:else if folderLoading}
						<p class="m-0 py-2 text-sm text-[#87929a]" role="status">Loading more…</p>
					{:else}
						<InfiniteScrollSentinel
							enabled={Boolean(folderCursor) && !folderLoading && !error}
							onLoad={loadMore}
						/>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</section>

<FileEntryContextMenu
	mode="public"
	open={contextMenuOpen}
	entry={contextMenuEntry}
	x={contextMenuPosition.x}
	y={contextMenuPosition.y}
	{allowPreview}
	{allowDownload}
	onClose={() => (contextMenuEntry = null)}
	onOpenFolder={openFolderById}
	onDownload={onDownloadFile}
	onPreviewImage={openFile}
	onPreviewMedia={openFile}
	onPreviewPdf={openFile}
	onPreviewText={openFile}
/>
