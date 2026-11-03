<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { api } from '#lib/api.ts';
	import { toast } from '#lib/toast.ts';
	import { isImageFile } from '#lib/utils/image.ts';
	import { fileManagerEntryKey, NEW_FOLDER_DRAFT_ID, type FileManagerEntry, type FilesView } from './types';
	import { createFolderEntries } from './folder-entries.svelte';
	import FilesBrowser from './FilesBrowser.svelte';
	import FileManagerOverlays from './FileManagerOverlays.svelte';
	import { useTransferManager } from '#lib/transfers/manager.ts';
	import { waitUntilReady } from '#lib/vault/session.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { PageProps } from './$types';

	// shortcut: browsers without File System Access cannot download; add another streaming file sink when supporting them.
	type FilePickerWindow = Window & {
		showSaveFilePicker?: (options?: { suggestedName?: string }) => Promise<FileSystemFileHandle>;
		showDirectoryPicker?: (options?: { mode?: 'read' | 'readwrite' }) => Promise<FileSystemDirectoryHandle>;
	};

	let { data }: PageProps = $props();
	let deleteDialogOpen = $state(false);
	let deleting = $state(false);
	let deleteTargets = $state<FileManagerEntry[]>([]);
	let newFolderActive = $state(false);
	let newFolderParentId = $state<string | null>(null);
	let newFolderName = $state('');
	let creatingFolder = $state(false);
	let propertiesEntry = $state<FileManagerEntry | null>(null);
	let imagePreviewRequest = $state<{ fileId: string; trigger?: HTMLElement } | null>(null);
	let videoPreviewEntry = $state<FileManagerEntry | null>(null);
	let VideoViewer = $state<typeof import('./VideoViewer.svelte').default>();
	let pdfPreviewEntry = $state<FileManagerEntry | null>(null);
	let PdfViewer = $state<typeof import('./PdfViewer.svelte').default>();
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

	const folderId = $derived(page.url.searchParams.get('folder_id') || null);
	const focusFileId = $derived(page.url.searchParams.get('focus_file'));
	const viewMode = $derived(page.url.searchParams.get('view') === 'grid' ? 'grid' : 'list');
	const folderEntries = createFolderEntries(() => data.user.id, () => folderId, () => focusFileId, () => $vaultUnlocked);
	const entriesQuery = folderEntries.query;
	const rows = $derived(folderEntries.rows);
	const breadcrumbs = $derived(folderEntries.breadcrumbs);
	const entriesReady = $derived(folderEntries.entriesReady);
	const loadedEntries = $derived(folderEntries.loadedEntries);
	const selectedEntries = $derived(loadedEntries.filter((entry) => selectedIds.has(fileManagerEntryKey(entry))));
	const selectedFiles = $derived(selectedEntries.filter((entry) => entry.kind === 'file'));
	const showNewFolder = $derived(newFolderActive && entriesReady && newFolderParentId === folderId);
	const displayedRows = $derived(showNewFolder ? [{ id: NEW_FOLDER_DRAFT_ID, kind: 'folder' as const, name: '', type: 'Folder', size: null, updatedAt: '' }, ...rows] : rows);
	// shortcut: PhotoSwipe navigates loaded rows; add paged image lookup if full-folder navigation is needed.
	const imageEntries = $derived(rows.filter((entry) => entry.kind === 'file' && isImageFile(entry.name, entry.type)));
	const allRowsSelected = $derived(loadedEntries.length > 0 && selectedEntries.length === loadedEntries.length);
	const currentLocation = $derived(breadcrumbs.map((crumb) => crumb.name).join(' / '));
	const breadcrumbItems = $derived(breadcrumbs.map((crumb, index) => ({
		id: crumb.id ?? 'root',
		label: crumb.name,
		href: index < breadcrumbs.length - 1 ? urlFor(crumb.id) : undefined
	})));
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
	});

	$effect(() => {
		if ($vaultUnlocked) return;
		contextMenuEntry = null;
		contextMenuOpen = false;
		propertiesEntry = null;
		cancelNewFolder(true);
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

	function beginNewFolder(parentFolderId: string | null = folderId): void {
		if (!$vaultUnlocked || creatingFolder) return;
		newFolderParentId = parentFolderId;
		newFolderName = '';
		newFolderActive = true;
		contextMenuEntry = null;
		contextMenuOpen = false;
		if (parentFolderId !== folderId) void goto(urlFor(parentFolderId));
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
		if (target.isContentEditable || target.closest('input:not([type="checkbox"]):not([type="radio"]), textarea, select, [contenteditable="true"]')) return;
		if (contextMenuOpen || imagePreviewRequest || videoPreviewEntry || pdfPreviewEntry || document.querySelector('[data-modal-open="true"]')) return;
		if ((event.ctrlKey || event.metaKey) && event.altKey && !event.shiftKey && event.key.toLowerCase() === 'n') {
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
		if (!(event.target instanceof Element) || event.target.closest('[data-file-entry], button, input, a, [role="menu"]')) return;
		event.preventDefault();
		showContextMenu(null, event.clientX, event.clientY);
	}

	function previewImage(entry: FileManagerEntry, trigger?: HTMLElement): void {
		imagePreviewRequest = { fileId: entry.id, trigger };
	}

	function previewVideo(entry: FileManagerEntry): void {
		videoPreviewEntry = entry;
		if (!VideoViewer) void import('./VideoViewer.svelte').then(({ default: component }) => VideoViewer = component);
	}

	function previewPdf(entry: FileManagerEntry): void {
		pdfPreviewEntry = entry;
		if (!PdfViewer) void import('./PdfViewer.svelte').then(({ default: component }) => PdfViewer = component);
	}

	function enqueueDownload(entry: FileManagerEntry, fileName: string, getTarget: () => Promise<FileSystemFileHandle>): void {
		transfers.enqueueDownload(fileName, entry.size ?? 0, async (signal, onProgress, concurrency) => {
			const { downloadEncryptedFile } = await import('#lib/downloads/download-file.ts');
			await downloadEncryptedFile(entry.id, getTarget, signal, onProgress, concurrency);
		});
	}

	function showDownloadError(error: unknown): void {
		if (error instanceof DOMException && error.name === 'AbortError') return;
		toast.error(error instanceof Error ? error.message : 'Could not start download.');
	}

	function safeDownloadName(name: string): string {
		return name.replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_').replace(/[. ]+$/g, '') || 'download';
	}

	function downloadEntry(entry: FileManagerEntry): void {
		if (entry.kind !== 'file') return;
		const picker = window as FilePickerWindow;
		if (!picker.showSaveFilePicker) {
			toast.error('This browser cannot save streamed files directly to disk.');
			return;
		}
		try {
			void picker.showSaveFilePicker({ suggestedName: safeDownloadName(entry.name || 'download') })
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
			void picker.showDirectoryPicker({ mode: 'readwrite' }).then(async (directory) => {
				const reserved = new Set<string>();
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
			}).catch(showDownloadError);
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
	folderName={breadcrumbs.at(-1)?.name ?? 'Files'}
	{breadcrumbItems}
	{focusFileId}
	{viewMode}
	query={entriesQuery}
	{rows}
	{displayedRows}
	{entriesReady}
	{newFolderActive}
	{showNewFolder}
	{selectedIds}
	{selectedEntries}
	{selectedFiles}
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
	onDeleteSelected={() => requestDelete()}
	onOpenFolder={openFolder}
	onShowContextMenu={showContextMenu}
	onShowBackgroundContextMenu={showBackgroundContextMenu}
	onPreviewImage={previewImage}
	onPreviewVideo={previewVideo}
	onPreviewPdf={previewPdf}
	onCreateFolder={saveNewFolder}
	onCancelNewFolder={cancelNewFolder}
	onViewChange={changeView}
	onTileSizeChange={(size) => tileSize = size}
	onShowTypeChange={(show) => showType = show}
	onShowSizeChange={(show) => showSize = show}
	onShowDimensionsChange={(show) => showDimensions = show}
/>

<svelte:window onkeydown={handleFileManagerKeydown} />

<FileManagerOverlays
	{imageEntries}
	{imagePreviewRequest}
	{VideoViewer}
	{videoPreviewEntry}
	{PdfViewer}
	{pdfPreviewEntry}
	{contextMenuOpen}
	{contextMenuEntry}
	{contextMenuPosition}
	{propertiesEntry}
	{currentLocation}
	{deleteDialogOpen}
	{deleting}
	{deleteTargets}
	onCloseImage={() => imagePreviewRequest = null}
	onError={(message) => toast.error(message)}
	onCloseVideo={() => videoPreviewEntry = null}
	onClosePdf={() => pdfPreviewEntry = null}
	onCloseContextMenu={() => { contextMenuEntry = null; contextMenuOpen = false; }}
	onOpenFolder={openFolder}
	onDownload={downloadEntry}
	onPreviewImage={previewImage}
	onPreviewVideo={previewVideo}
	onPreviewPdf={previewPdf}
	onProperties={(entry) => propertiesEntry = entry}
	onDeleteEntry={requestDelete}
	onCreateFolder={createFolderFromContext}
	onCloseProperties={() => propertiesEntry = null}
	onCloseDelete={() => { if (!deleting) deleteDialogOpen = false; }}
	onCancelDelete={() => deleteDialogOpen = false}
	onConfirmDelete={() => void deleteSelected()}
/>
