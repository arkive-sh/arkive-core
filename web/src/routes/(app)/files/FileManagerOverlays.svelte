<script lang="ts">
	import Modal from '#lib/components/Modal.svelte';
	import FileEntryContextMenu from './FileEntryContextMenu.svelte';
	import FilePropertiesModal from './FilePropertiesModal.svelte';
	import ImageViewer from './ImageViewer.svelte';
	import TextFileEditor from './TextFileEditor.svelte';
	import type { FileManagerEntry } from './types';

	let {
		imageEntries,
		imagePreviewRequest,
		MediaViewer,
		mediaPreviewEntry,
		PdfViewer,
		pdfPreviewEntry,
		textEditorOpen,
		textEditorEntry,
		vaultId,
		parentFolderId,
		contextMenuOpen,
		contextMenuEntry,
		contextMenuPosition,
		clipboardCount,
		propertiesEntry,
		currentLocation,
		deleteDialogOpen,
		deleting,
		deleteTargets,
		onCloseImage,
		onError,
		onCloseMedia,
		onClosePdf,
		onCloseText,
		onCloseContextMenu,
		onOpenFolder,
		onDownload,
		onPreviewImage,
		onPreviewMedia,
		onPreviewPdf,
		onPreviewText,
		onProperties,
		onDeleteEntry,
		onCutEntry,
		onPaste,
		onCreateFolder,
		onCreateTextFile,
		onUploadFiles,
		onUploadFolder,
		onCloseProperties,
		onCloseDelete,
		onCancelDelete,
		onConfirmDelete,
	}: {
		imageEntries: FileManagerEntry[];
		imagePreviewRequest: { fileId: string; trigger?: HTMLElement } | null;
		MediaViewer?: typeof import('./MediaViewer.svelte').default;
		mediaPreviewEntry: FileManagerEntry | null;
		PdfViewer?: typeof import('./PdfViewer.svelte').default;
		pdfPreviewEntry: FileManagerEntry | null;
		textEditorOpen: boolean;
		textEditorEntry: FileManagerEntry | null;
		vaultId: string;
		parentFolderId: string | null;
		contextMenuOpen: boolean;
		contextMenuEntry: FileManagerEntry | null;
		contextMenuPosition: { x: number; y: number };
		clipboardCount: number;
		propertiesEntry: FileManagerEntry | null;
		currentLocation: string;
		deleteDialogOpen: boolean;
		deleting: boolean;
		deleteTargets: FileManagerEntry[];
		onCloseImage: () => void;
		onError: (message: string) => void;
		onCloseMedia: () => void;
		onClosePdf: () => void;
		onCloseText: () => void;
		onCloseContextMenu: () => void;
		onOpenFolder: (id: string) => void;
		onDownload: (entry: FileManagerEntry) => void;
		onPreviewImage: (entry: FileManagerEntry) => void;
		onPreviewMedia: (entry: FileManagerEntry) => void;
		onPreviewPdf: (entry: FileManagerEntry) => void;
		onPreviewText: (entry: FileManagerEntry) => void;
		onProperties: (entry: FileManagerEntry) => void;
		onDeleteEntry: (entry: FileManagerEntry) => void;
		onCutEntry: (entry: FileManagerEntry) => void;
		onPaste: (entry: FileManagerEntry | null) => void;
		onCreateFolder: (entry: FileManagerEntry | null) => void;
		onCreateTextFile: () => void;
		onUploadFiles: () => void;
		onUploadFolder: () => void;
		onCloseProperties: () => void;
		onCloseDelete: () => void;
		onCancelDelete: () => void;
		onConfirmDelete: () => void;
	} = $props();
</script>

<ImageViewer
	entries={imageEntries}
	request={imagePreviewRequest}
	onClose={onCloseImage}
	{onError}
/>

{#if MediaViewer}
	<MediaViewer entry={mediaPreviewEntry} onClose={onCloseMedia} />
{/if}

{#if textEditorOpen}
	<TextFileEditor entry={textEditorEntry} {vaultId} {parentFolderId} onClose={onCloseText} />
{/if}

{#if PdfViewer && pdfPreviewEntry}
	<PdfViewer entry={pdfPreviewEntry} onClose={onClosePdf} />
{/if}

<FileEntryContextMenu
	open={contextMenuOpen}
	entry={contextMenuEntry}
	{clipboardCount}
	x={contextMenuPosition.x}
	y={contextMenuPosition.y}
	onClose={onCloseContextMenu}
	{onOpenFolder}
	{onDownload}
	{onPreviewImage}
	{onPreviewMedia}
	{onPreviewPdf}
	{onPreviewText}
	{onProperties}
	onDelete={onDeleteEntry}
	onCut={onCutEntry}
	{onPaste}
	{onCreateFolder}
	{onCreateTextFile}
	{onUploadFiles}
	{onUploadFolder}
/>

<FilePropertiesModal
	entry={propertiesEntry}
	location={currentLocation}
	onClose={onCloseProperties}
/>

<Modal
	open={deleteDialogOpen}
	label="Confirm move to Trash"
	title={`Move ${deleteTargets.length} ${deleteTargets.length === 1 ? 'item' : 'items'} to Trash?`}
	onClose={onCloseDelete}
>
	<p class="m-0 text-sm text-(--muted)">
		Items stay in Trash for 30 days and can be restored before then.
	</p>
	{#snippet actions()}
		<button
			class="cursor-pointer rounded border border-(--border) px-4 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
			type="button"
			disabled={deleting}
			onclick={onCancelDelete}>Cancel</button
		>
		<button
			class="cursor-pointer rounded border border-red-500 bg-red-500 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
			type="button"
			disabled={deleting || !deleteTargets.length}
			onclick={onConfirmDelete}>{deleting ? 'Moving…' : 'Move to Trash'}</button
		>
	{/snippet}
</Modal>
