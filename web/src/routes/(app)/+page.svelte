<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { SvelteSet, SvelteURLSearchParams } from 'svelte/reactivity';
	import { useQueryClient } from '@tanstack/svelte-query';
	import Modal from '#lib/components/Modal.svelte';
	import { api } from '#lib/api.ts';
	import { toast } from '#lib/toast.ts';
	import { isImageFile } from '#lib/utils/image.ts';
	import { rewrapFileKeyForParent, rewrapFolderMetadataForParent } from '#lib/vault/files.ts';
	import {
		fileManagerEntryKey,
		NEW_FOLDER_DRAFT_ID,
		type FileManagerEntry,
		type FilesView,
	} from './files/types';
	import { createFolderEntries } from './files/folder-entries.svelte';
	import FilesBrowser from './files/FilesBrowser.svelte';
	import FileManagerOverlays from './files/FileManagerOverlays.svelte';
	import { useTransferManager, type UploadItem } from '#lib/transfers/manager.ts';
	import { waitUntilReady } from '#lib/vault/session.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { PageProps } from './$types';

	// shortcut: browsers without File System Access cannot download; add another streaming file sink when supporting them.
	type FilePickerWindow = Window & {
		showSaveFilePicker?: (options?: { suggestedName?: string }) => Promise<FileSystemFileHandle>;
		showDirectoryPicker?: (options?: {
			mode?: 'read' | 'readwrite';
		}) => Promise<FileSystemDirectoryHandle>;
	};

	let { data }: PageProps = $props();
	let filePicker: HTMLInputElement | undefined;
	let folderPicker: HTMLInputElement | undefined;
	let selectedUploadFiles = $state<UploadItem[]>([]);
	let uploadConfirmOpen = $state(false);
	let uploadStatus = $state('');
	let deleteDialogOpen = $state(false);
	let deleting = $state(false);
	let deleteTargets = $state<FileManagerEntry[]>([]);
	let cutEntries = $state<FileManagerEntry[]>([]);
	let movingEntries = $state(false);
	let newFolderActive = $state(false);
	let newFolderParentId = $state<string | null>(null);
	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let propertiesEntry = $state<FileManagerEntry | null>(null);
	let shareEntry = $state<FileManagerEntry | null>(null);
	let imagePreviewRequest = $state<{ fileId: string; trigger?: HTMLElement } | null>(null);
	let mediaPreviewEntry = $state<FileManagerEntry | null>(null);
	let MediaViewer = $state<typeof import('./files/MediaViewer.svelte').default>();
	let pdfPreviewEntry = $state<FileManagerEntry | null>(null);
	let PdfViewer = $state<typeof import('./files/PdfViewer.svelte').default>();
	let textEditorOpen = $state(false);
	let textEditorEntry = $state<FileManagerEntry | null>(null);
	let contextMenuEntry = $state<FileManagerEntry | null>(null);
	let contextMenuOpen = $state(false);
	let contextMenuPosition = $state({ x: 0, y: 0 });
	let tileSize = $state(200);
	let showType = $state(false);
	let showSize = $state(true);
	let showDimensions = $state(false);
	let viewOptionsReady = $state(false);
	const selectedIds = new SvelteSet<string>();
	const transfers = useTransferManager();
	const queryClient = useQueryClient();
	// Keep this aligned with the move API's per-request root limit.
	const MAX_MOVE_ENTRIES = 1000;

	const folderId = $derived(page.url.searchParams.get('folder_id') || null);
	const focusFileId = $derived(page.url.searchParams.get('focus_file'));
	const viewMode = $derived(page.url.searchParams.get('view') === 'grid' ? 'grid' : 'list');
	const folderEntries = createFolderEntries(
		() => data.user.id,
		() => folderId,
		() => focusFileId,
		() => $vaultUnlocked,
	);
	const entriesQuery = folderEntries.query;
	const rows = $derived(folderEntries.rows);
	const breadcrumbs = $derived(folderEntries.breadcrumbs);
	const entriesReady = $derived(folderEntries.entriesReady);
	const loadedEntries = $derived(folderEntries.loadedEntries);
	const selectedEntries = $derived(
		loadedEntries.filter((entry) => selectedIds.has(fileManagerEntryKey(entry))),
	);
	const selectedFiles = $derived(selectedEntries.filter((entry) => entry.kind === 'file'));
	const showNewFolder = $derived(newFolderActive && entriesReady && newFolderParentId === folderId);
	const displayedRows = $derived(
		showNewFolder
			? [
					{
						id: NEW_FOLDER_DRAFT_ID,
						kind: 'folder' as const,
						name: '',
						type: 'Folder',
						size: null,
						updatedAt: '',
					},
					...rows,
				]
			: rows,
	);
	// shortcut: PhotoSwipe navigates loaded rows; add paged image lookup if full-folder navigation is needed.
	const imageEntries = $derived(
		rows.filter((entry) => entry.kind === 'file' && isImageFile(entry.name, entry.type)),
	);
	const allRowsSelected = $derived(
		loadedEntries.length > 0 && selectedEntries.length === loadedEntries.length,
	);
	const currentLocation = $derived(breadcrumbs.map((crumb) => crumb.name).join(' / '));
	const breadcrumbItems = $derived(
		breadcrumbs.map((crumb, index) => ({
			id: crumb.id ?? 'root',
			label: crumb.name,
			href: index < breadcrumbs.length - 1 ? urlFor(crumb.id) : undefined,
		})),
	);
	let lastFolderId: string | null | undefined;

	$effect(() => {
		const activeFolderId = folderId;

		if (lastFolderId === undefined) {
			lastFolderId = activeFolderId;

			return;
		}
		if (activeFolderId === lastFolderId) return;
		lastFolderId = activeFolderId;
		selectedIds.clear();
		contextMenuEntry = null;
		contextMenuOpen = false;
		propertiesEntry = null;
		shareEntry = null;
	});

	$effect(() => {
		if ($vaultUnlocked) return;
		cutEntries = [];
		contextMenuEntry = null;
		contextMenuOpen = false;
		propertiesEntry = null;
		shareEntry = null;
		cancelNewFolder(true);
		cancelUploadSelection();
	});

	function urlFor(folder: string | null, nextView: FilesView = viewMode): '' | `?${string}` {
		const params = new SvelteURLSearchParams();

		if (folder) params.set('folder_id', folder);
		if (nextView === 'grid') params.set('view', 'grid');
		const query = params.toString();

		return query ? `?${query}` : '';
	}

	function openFolder(id: string): void {
		// The route is fixed; folder and view state live in its query string.
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto(resolve('/(app)') + urlFor(id));
	}

	function openShare(fileId: string): void {
		void goto(`${resolve('/(app)/shares')}?focus_file=${encodeURIComponent(fileId)}`);
	}

	function stageUploadFiles(files: UploadItem[]): void {
		if (!$vaultUnlocked || !files.length) return;
		selectedUploadFiles = files;
		uploadStatus = '';
		uploadConfirmOpen = true;
	}

	function handleUploadPicker(event: Event): void {
		const input = event.currentTarget;

		if (!(input instanceof HTMLInputElement)) return;
		stageUploadFiles(
			Array.from(input.files ?? [], (file) => ({ file, relativePath: file.webkitRelativePath })),
		);
		input.value = '';
	}

	function cancelUploadSelection(): void {
		uploadConfirmOpen = false;
		selectedUploadFiles = [];
	}

	function startUpload(): void {
		if (!$vaultUnlocked || !selectedUploadFiles.length) return;
		const count = transfers.enqueueUploads(selectedUploadFiles, data.user.id, folderId);
		selectedUploadFiles = [];
		uploadConfirmOpen = false;
		uploadStatus = `Added ${count} ${count === 1 ? 'file' : 'files'} to transfers.`;
	}

	function chooseUploadFiles(): void {
		if ($vaultUnlocked) filePicker?.click();
	}

	function chooseUploadFolder(): void {
		if ($vaultUnlocked) folderPicker?.click();
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

	function stageCut(entries: FileManagerEntry[]): void {
		if (!$vaultUnlocked || movingEntries || !entries.length) return;
		if (entries.length > MAX_MOVE_ENTRIES) {
			toast.error(`Move up to ${MAX_MOVE_ENTRIES} items at a time.`);

			return;
		}
		if (
			entries.some(
				(entry) =>
					(entry.kind === 'file' && !entry.encryptedFileKey) ||
					(entry.kind === 'folder' && (!entry.encryptedName || !entry.encryptedMetadata)),
			)
		) {
			toast.error('Wait for the selected items to finish loading, then try again.');

			return;
		}

		cutEntries = [...entries];
		selectedIds.clear();
		contextMenuEntry = null;
		contextMenuOpen = false;
	}

	function cutEntry(entry: FileManagerEntry): void {
		const entries =
			selectedIds.has(fileManagerEntryKey(entry)) && selectedEntries.length
				? selectedEntries
				: [entry];
		stageCut(entries);
	}

	async function pasteCutEntries(targetFolderId: string | null): Promise<void> {
		if (movingEntries || !cutEntries.length || !$vaultUnlocked) return;
		const entries = cutEntries.filter((entry) => (entry.parentFolderId ?? null) !== targetFolderId);

		if (!entries.length) {
			cutEntries = [];
			toast.success('These items are already in this folder.');

			return;
		}
		movingEntries = true;
		try {
			// Parent IDs are part of entry AAD, so rewrap moved roots before updating their parent IDs.
			const files = await Promise.all(
				entries
					.filter((entry) => entry.kind === 'file')
					.map(async (entry) => ({
						id: entry.id,
						...(await rewrapFileKeyForParent(
							{
								vaultId: data.user.id,
								fileId: entry.id,
								folderId: entry.parentFolderId ?? null,
								encryptedFileKey: entry.encryptedFileKey!,
							},
							targetFolderId,
						)),
					})),
			);
			const folders = await Promise.all(
				entries
					.filter((entry) => entry.kind === 'folder')
					.map(async (entry) => ({
						id: entry.id,
						...(await rewrapFolderMetadataForParent(
							{
								vaultId: data.user.id,
								folderId: entry.id,
								parentFolderId: entry.parentFolderId ?? null,
								encryptedName: entry.encryptedName!,
								encryptedMetadata: entry.encryptedMetadata!,
							},
							targetFolderId,
						)),
					})),
			);
			await api.post('/api/entries/move', { targetFolderId, files, folders });
			const alreadyThere = cutEntries.length - entries.length;
			cutEntries = [];
			selectedIds.clear();
			toast.success(
				alreadyThere
					? `Moved ${entries.length} items; ${alreadyThere} already in this folder.`
					: `Moved ${entries.length} ${entries.length === 1 ? 'item' : 'items'}.`,
			);
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['folder-entries'] }),
				queryClient.invalidateQueries({ queryKey: ['search'] }),
			]);
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not move selected items.');
		} finally {
			movingEntries = false;
		}
	}

	function beginNewFolder(parentFolderId: string | null = folderId): void {
		if (!$vaultUnlocked || creatingFolder) return;
		newFolderParentId = parentFolderId;
		newFolderName = '';
		newFolderActive = true;
		contextMenuEntry = null;
		contextMenuOpen = false;
		if (parentFolderId !== folderId) {
			const href = urlFor(parentFolderId);
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			void goto(resolve('/(app)') + href);
		}
	}

	function createFolderFromContext(entry: FileManagerEntry | null): void {
		beginNewFolder(entry?.kind === 'folder' ? entry.id : folderId);
	}

	function cancelNewFolder(force = false): void {
		if (creatingFolder && !force) return;
		newFolderActive = false;
		newFolderName = '';
		newFolderParentId = null;
	}

	async function saveNewFolder(): Promise<void> {
		const name = newFolderName.trim();

		if (creatingFolder || !name || !showNewFolder || !$vaultUnlocked) return;
		const parentFolderId = newFolderParentId;
		const vaultId = data.user.id;
		creatingFolder = true;
		try {
			const { createUploadFolder } = await import('#lib/uploads/upload-file.ts');
			await createUploadFolder(name, parentFolderId, vaultId);
			newFolderActive = false;
			newFolderName = '';
			newFolderParentId = null;
			toast.success('Folder created.');
			await queryClient.invalidateQueries({ queryKey: ['folder-entries', parentFolderId] });
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not create folder.');
		} finally {
			creatingFolder = false;
		}
	}

	function handleFileManagerKeydown(event: KeyboardEvent): void {
		const target = event.target;

		if (event.defaultPrevented || event.repeat || !(target instanceof HTMLElement)) return;
		if (
			target.isContentEditable ||
			target.closest(
				'input:not([type="checkbox"]):not([type="radio"]), textarea, select, [contenteditable="true"]',
			)
		)
			return;
		if (
			contextMenuOpen ||
			imagePreviewRequest ||
			mediaPreviewEntry ||
			pdfPreviewEntry ||
			document.querySelector('[data-modal-open="true"]')
		)
			return;
		const command = event.ctrlKey || event.metaKey;

		if (command && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'x') {
			if (selectedEntries.length) {
				event.preventDefault();
				stageCut(selectedEntries);
			}

			return;
		}
		if (command && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'v') {
			if (cutEntries.length) {
				event.preventDefault();
				void pasteCutEntries(folderId);
			}

			return;
		}
		if (
			(event.ctrlKey || event.metaKey) &&
			event.altKey &&
			!event.shiftKey &&
			event.key.toLowerCase() === 'n'
		) {
			event.preventDefault();
			beginNewFolder(folderId);
		}
	}

	function showContextMenu(entry: FileManagerEntry | null, x: number, y: number): void {
		contextMenuEntry = entry;
		contextMenuPosition = { x, y };
		contextMenuOpen = true;
	}

	function showBackgroundContextMenu(event: MouseEvent): void {
		if (
			!(event.target instanceof Element) ||
			event.target.closest('[data-file-entry], button, input, a, [role="menu"]')
		)
			return;
		event.preventDefault();
		showContextMenu(null, event.clientX, event.clientY);
	}

	function previewImage(entry: FileManagerEntry, trigger?: HTMLElement): void {
		imagePreviewRequest = { fileId: entry.id, trigger };
	}

	function previewMedia(entry: FileManagerEntry): void {
		mediaPreviewEntry = entry;
		if (!MediaViewer)
			void import('./files/MediaViewer.svelte').then(
				({ default: component }) => (MediaViewer = component),
			);
	}

	function previewPdf(entry: FileManagerEntry): void {
		pdfPreviewEntry = entry;
		if (!PdfViewer)
			void import('./files/PdfViewer.svelte').then(
				({ default: component }) => (PdfViewer = component),
			);
	}

	function previewText(entry: FileManagerEntry): void {
		textEditorEntry = entry;
		textEditorOpen = true;
	}

	function createTextFile(): void {
		textEditorEntry = null;
		textEditorOpen = true;
	}

	function enqueueDownload(
		entry: FileManagerEntry,
		fileName: string,
		getTarget: () => Promise<FileSystemFileHandle>,
	): void {
		transfers.enqueueDownload(
			fileName,
			entry.size ?? 0,
			async (signal, onProgress, concurrency) => {
				const { downloadEncryptedFile } = await import('#lib/downloads/download-file.ts');
				await downloadEncryptedFile(entry.id, getTarget, signal, onProgress, concurrency);
			},
		);
	}

	function showDownloadError(error: unknown): void {
		if (error instanceof DOMException && error.name === 'AbortError') return;
		toast.error(error instanceof Error ? error.message : 'Could not start download.');
	}

	function safeDownloadName(name: string): string {
		return name.replace(/[<>:"/\\|?*\p{Cc}]/gu, '_').replace(/[. ]+$/g, '') || 'download';
	}

	function downloadEntry(entry: FileManagerEntry): void {
		if (entry.kind !== 'file') return;
		const picker = window as FilePickerWindow;

		if (!picker.showSaveFilePicker) {
			toast.error('This browser cannot save streamed files directly to disk.');

			return;
		}
		try {
			void picker
				.showSaveFilePicker({ suggestedName: safeDownloadName(entry.name || 'download') })
				.then((target) => enqueueDownload(entry, target.name, async () => target))
				.catch(showDownloadError);
		} catch (error) {
			showDownloadError(error);
		}
	}

	function downloadSelected(): void {
		const entries = [...selectedFiles];

		if (entries.length === 1) {
			downloadEntry(entries[0]!);

			return;
		}
		if (!entries.length) return;

		const picker = window as FilePickerWindow;

		if (!picker.showDirectoryPicker) {
			toast.error('This browser cannot save multiple streamed files to a folder.');

			return;
		}
		try {
			void picker
				.showDirectoryPicker({ mode: 'readwrite' })
				.then(async (directory) => {
					const reserved = new SvelteSet<string>();
					const targets: Array<{ entry: FileManagerEntry; name: string }> = [];

					for (const entry of entries) {
						const safeName = safeDownloadName(entry.name || 'download');
						const extensionAt = safeName.lastIndexOf('.');
						const stem = extensionAt > 0 ? safeName.slice(0, extensionAt) : safeName;
						const extension = extensionAt > 0 ? safeName.slice(extensionAt) : '';

						for (let suffix = 0; ; suffix++) {
							const name = suffix ? `${stem} (${suffix})${extension}` : safeName;
							const key = name.toLowerCase();

							if (reserved.has(key)) continue;
							try {
								await directory.getFileHandle(name);
								reserved.add(key);
							} catch (error) {
								if (!(error instanceof DOMException)) throw error;
								if (error.name === 'TypeMismatchError') {
									reserved.add(key);
									continue;
								}
								if (error.name !== 'NotFoundError') throw error;
								reserved.add(key);
								targets.push({ entry, name });
								break;
							}
						}
					}
					for (const { entry, name } of targets) {
						enqueueDownload(entry, name, () => directory.getFileHandle(name, { create: true }));
					}
				})
				.catch(showDownloadError);
		} catch (error) {
			showDownloadError(error);
		}
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
				folderIds: deleteTargets
					.filter((entry) => entry.kind === 'folder')
					.map((entry) => entry.id),
			});
			selectedIds.clear();
			deleteTargets = [];
			deleteDialogOpen = false;
			toast.success('Items moved to Trash.');
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['trash'] }),
				queryClient.invalidateQueries({ queryKey: ['folder-entries'] }),
				queryClient.invalidateQueries({ queryKey: ['search'] }),
			]);
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Could not delete selected items.');
		} finally {
			deleting = false;
		}
	}

	function changeView(view: FilesView): void {
		const href = urlFor(folderId, view);
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto(resolve('/(app)') + href);
	}

	onMount(() => {
		try {
			const savedTileSize = localStorage.getItem('arkive.files.tile-size');

			if (savedTileSize !== null) {
				const parsedTileSize = Number(savedTileSize);

				if (Number.isFinite(parsedTileSize)) {
					const normalized = Math.min(260, Math.max(60, Math.round(parsedTileSize / 10) * 10));
					// The old 150px default was persisted automatically; move that default to the roomier grid.
					tileSize = normalized === 150 ? 200 : normalized;
				}
			}
			showType = localStorage.getItem('arkive.files.show-type') === 'true';
			showSize = localStorage.getItem('arkive.files.show-size') !== 'false';
			showDimensions = localStorage.getItem('arkive.files.show-dimensions') === 'true';
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
			localStorage.setItem('arkive.files.show-dimensions', String(showDimensions));
		} catch {
			// Preferences still work for the current page when local storage is unavailable.
		}
	});
</script>

<svelte:head>
	<title>Arkive · Files</title>
</svelte:head>

<FilesBrowser
	unlocked={$vaultUnlocked}
	onFilesDropped={stageUploadFiles}
	folderName={breadcrumbs.at(-1)?.name ?? 'Files'}
	{breadcrumbItems}
	{focusFileId}
	{viewMode}
	query={entriesQuery}
	{rows}
	{displayedRows}
	{entriesReady}
	{newFolderActive}
	{selectedIds}
	{selectedEntries}
	{selectedFiles}
	clipboardCount={cutEntries.length}
	moving={movingEntries}
	loadedCount={loadedEntries.length}
	{allRowsSelected}
	{tileSize}
	{showType}
	{showSize}
	{showDimensions}
	bind:newFolderName
	{creatingFolder}
	onBeginNewFolder={() => beginNewFolder(folderId)}
	onToggleSelection={toggleSelection}
	onToggleAll={toggleAllLoaded}
	onClearSelection={clearSelection}
	onDownloadSelected={downloadSelected}
	onCutSelected={() => stageCut(selectedEntries)}
	onPaste={() => void pasteCutEntries(folderId)}
	onClearClipboard={() => (cutEntries = [])}
	onDeleteSelected={() => requestDelete()}
	onOpenFolder={openFolder}
	onOpenShares={openShare}
	onShowContextMenu={showContextMenu}
	onShowBackgroundContextMenu={showBackgroundContextMenu}
	onPreviewImage={previewImage}
	onPreviewMedia={previewMedia}
	onPreviewPdf={previewPdf}
	onPreviewText={previewText}
	onCreateFolder={saveNewFolder}
	onCancelNewFolder={cancelNewFolder}
	onViewChange={changeView}
	onTileSizeChange={(size) => (tileSize = size)}
	onShowTypeChange={(show) => (showType = show)}
	onShowSizeChange={(show) => (showSize = show)}
	onShowDimensionsChange={(show) => (showDimensions = show)}
/>

<svelte:window onkeydown={handleFileManagerKeydown} />

<FileManagerOverlays
	{imageEntries}
	{imagePreviewRequest}
	{MediaViewer}
	{mediaPreviewEntry}
	{PdfViewer}
	{pdfPreviewEntry}
	{textEditorOpen}
	{textEditorEntry}
	vaultId={data.user.id}
	parentFolderId={folderId}
	{contextMenuOpen}
	{contextMenuEntry}
	{contextMenuPosition}
	clipboardCount={movingEntries ? 0 : cutEntries.length}
	{propertiesEntry}
	{shareEntry}
	{currentLocation}
	{deleteDialogOpen}
	{deleting}
	{deleteTargets}
	onCloseImage={() => (imagePreviewRequest = null)}
	onError={(message) => toast.error(message)}
	onCloseMedia={() => (mediaPreviewEntry = null)}
	onClosePdf={() => (pdfPreviewEntry = null)}
	onCloseText={() => (textEditorOpen = false)}
	onCloseContextMenu={() => {
		contextMenuEntry = null;
		contextMenuOpen = false;
	}}
	onOpenFolder={openFolder}
	onDownload={downloadEntry}
	onPreviewImage={previewImage}
	onPreviewMedia={previewMedia}
	onPreviewPdf={previewPdf}
	onPreviewText={previewText}
	onProperties={(entry) => (propertiesEntry = entry)}
	onShareEntry={(entry) => (shareEntry = entry.kind === 'file' ? entry : null)}
	onCloseShare={() => (shareEntry = null)}
	onDeleteEntry={requestDelete}
	onCutEntry={cutEntry}
	onPaste={(entry) => void pasteCutEntries(entry?.kind === 'folder' ? entry.id : folderId)}
	onCreateFolder={createFolderFromContext}
	onCreateTextFile={createTextFile}
	onUploadFiles={chooseUploadFiles}
	onUploadFolder={chooseUploadFolder}
	onCloseProperties={() => (propertiesEntry = null)}
	onCloseDelete={() => {
		if (!deleting) deleteDialogOpen = false;
	}}
	onCancelDelete={() => (deleteDialogOpen = false)}
	onConfirmDelete={() => void deleteSelected()}
/>

<input bind:this={filePicker} class="hidden" type="file" multiple onchange={handleUploadPicker} />
<input
	bind:this={folderPicker}
	class="hidden"
	type="file"
	multiple
	webkitdirectory
	onchange={handleUploadPicker}
/>
<p class="sr-only" role="status" aria-live="polite">{uploadStatus}</p>

<Modal
	open={uploadConfirmOpen}
	label="Confirm upload"
	title="Start upload?"
	onClose={cancelUploadSelection}
>
	<p class="m-0 text-sm text-(--muted)">{selectedUploadFiles.length} file(s) selected.</p>
	{#snippet actions()}
		<button
			class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-transparent px-4 text-sm font-semibold text-(--text) shadow-none transition-colors hover:bg-(--surface-highest)"
			type="button"
			onclick={cancelUploadSelection}>Cancel</button
		>
		<button
			class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--accent) bg-(--accent) px-4 text-sm font-semibold text-(--accent-ink) transition-colors hover:border-(--accent-strong) hover:bg-(--accent-strong)"
			type="button"
			onclick={startUpload}>Start upload</button
		>
	{/snippet}
</Modal>
