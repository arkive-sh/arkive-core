<script lang="ts">
	import Modal from '#lib/components/Modal.svelte';
	import FileEntryContextMenu from './FileEntryContextMenu.svelte';
	import FilePropertiesModal from './FilePropertiesModal.svelte';
	import ImageViewer from './ImageViewer.svelte';
	import type { FileManagerEntry } from './types';

	let {
		imageEntries,
		imagePreviewRequest,
		VideoViewer,
		videoPreviewEntry,
		PdfViewer,
		pdfPreviewEntry,
		contextMenuOpen,
		contextMenuEntry,
		contextMenuPosition,
		propertiesEntry,
		currentLocation,
		deleteDialogOpen,
		deleting,
		deleteTargets,
		onCloseImage,
		onError,
		onCloseVideo,
		onClosePdf,
		onCloseContextMenu,
		onOpenFolder,
		onDownload,
		onPreviewImage,
		onPreviewVideo,
		onPreviewPdf,
		onProperties,
		onDeleteEntry,
		onCreateFolder,
		onCloseProperties,
		onCloseDelete,
		onCancelDelete,
		onConfirmDelete
	}: {
		imageEntries: FileManagerEntry[];
		imagePreviewRequest: { fileId: string; trigger?: HTMLElement } | null;
		VideoViewer?: typeof import('./VideoViewer.svelte').default;
		videoPreviewEntry: FileManagerEntry | null;
		PdfViewer?: typeof import('./PdfViewer.svelte').default;
		pdfPreviewEntry: FileManagerEntry | null;
		contextMenuOpen: boolean;
		contextMenuEntry: FileManagerEntry | null;
		contextMenuPosition: { x: number; y: number };
		propertiesEntry: FileManagerEntry | null;
		currentLocation: string;
		deleteDialogOpen: boolean;
		deleting: boolean;
		deleteTargets: FileManagerEntry[];
		onCloseImage: () => void;
		onError: (message: string) => void;
		onCloseVideo: () => void;
		onClosePdf: () => void;
		onCloseContextMenu: () => void;
		onOpenFolder: (id: string) => void;
		onDownload: (entry: FileManagerEntry) => void;
		onPreviewImage: (entry: FileManagerEntry) => void;
		onPreviewVideo: (entry: FileManagerEntry) => void;
		onPreviewPdf: (entry: FileManagerEntry) => void;
		onProperties: (entry: FileManagerEntry) => void;
		onDeleteEntry: (entry: FileManagerEntry) => void;
		onCreateFolder: (entry: FileManagerEntry | null) => void;
		onCloseProperties: () => void;
		onCloseDelete: () => void;
		onCancelDelete: () => void;
		onConfirmDelete: () => void;
	} = $props();
</script>

<ImageViewer entries={imageEntries} request={imagePreviewRequest} onClose={onCloseImage} onError={onError} />

{#if VideoViewer}
	<VideoViewer entry={videoPreviewEntry} onClose={onCloseVideo} />
{/if}

{#if PdfViewer && pdfPreviewEntry}
	<PdfViewer entry={pdfPreviewEntry} onClose={onClosePdf} />
{/if}

<FileEntryContextMenu
	open={contextMenuOpen}
	entry={contextMenuEntry}
	x={contextMenuPosition.x}
	y={contextMenuPosition.y}
	onClose={onCloseContextMenu}
	{onOpenFolder}
	{onDownload}
	{onPreviewImage}
	{onPreviewVideo}
	{onPreviewPdf}
	{onProperties}
	onDelete={onDeleteEntry}
	{onCreateFolder}
/>

<FilePropertiesModal entry={propertiesEntry} location={currentLocation} onClose={onCloseProperties} />

<Modal
	open={deleteDialogOpen}
	label="Confirm delete"
	title={`Delete ${deleteTargets.length} ${deleteTargets.length === 1 ? 'item' : 'items'}?`}
	onClose={onCloseDelete}
>
	<p class="m-0 text-sm text-(--muted)">This action cannot be undone.</p>
	{#snippet actions()}
		<button class="cursor-pointer rounded border border-(--border) px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={deleting} onclick={onCancelDelete}>Cancel</button>
		<button class="cursor-pointer rounded border border-red-500 bg-red-500 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50" type="button" disabled={deleting || !deleteTargets.length} onclick={onConfirmDelete}>{deleting ? 'Deleting…' : 'Delete'}</button>
	{/snippet}
</Modal>
