<script lang="ts">
	import { tick } from 'svelte';
	import {
		AudioLines,
		CloudUpload,
		Download,
		Film,
		FileText,
		FolderOpen,
		FolderPlus,
		Image,
		Info,
		Trash2,
	} from '@lucide/svelte';
	import { isImageFile } from '#lib/utils/image.ts';
	import { isAudioMimeType, isMediaMimeType } from '#lib/utils/media.ts';
	import { isPdfFile, isTextFile, type FileManagerEntry } from './types';

	type Props = {
		open: boolean;
		entry: FileManagerEntry | null;
		x: number;
		y: number;
		onClose: () => void;
		onOpenFolder: (id: string) => void;
		onDownload: (entry: FileManagerEntry) => void;
		onPreviewImage: (entry: FileManagerEntry) => void;
		onPreviewMedia: (entry: FileManagerEntry) => void;
		onPreviewPdf: (entry: FileManagerEntry) => void;
		onPreviewText: (entry: FileManagerEntry) => void;
		onProperties: (entry: FileManagerEntry) => void;
		onDelete: (entry: FileManagerEntry) => void;
		onCreateFolder: (entry: FileManagerEntry | null) => void;
		onCreateTextFile: () => void;
		onUploadFiles: () => void;
		onUploadFolder: () => void;
	};

	let {
		open,
		entry,
		x,
		y,
		onClose,
		onOpenFolder,
		onDownload,
		onPreviewImage,
		onPreviewMedia,
		onPreviewPdf,
		onPreviewText,
		onProperties,
		onDelete,
		onCreateFolder,
		onCreateTextFile,
		onUploadFiles,
		onUploadFolder,
	}: Props = $props();
	let menu = $state<HTMLDivElement>();
	let position = $state({ left: 8, top: 8 });

	$effect(() => {
		if (!open) return;
		const targetX = x;
		const targetY = y;
		void tick().then(() => {
			if (!menu) return;
			const rect = menu.getBoundingClientRect();
			position = {
				left: Math.max(8, Math.min(targetX, window.innerWidth - rect.width - 8)),
				top: Math.max(8, Math.min(targetY, window.innerHeight - rect.height - 8)),
			};
			menu.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus();
		});
	});

	function closeOutside(event: PointerEvent): void {
		if (open && event.target instanceof Node && !menu?.contains(event.target)) onClose();
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (!open) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			onClose();

			return;
		}
		if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
		const items = Array.from(menu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? []);

		if (!items.length) return;
		event.preventDefault();
		const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement);
		const nextIndex =
			event.key === 'Home'
				? 0
				: event.key === 'End'
					? items.length - 1
					: (currentIndex + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
		items[nextIndex]?.focus();
	}

	function openFolder(): void {
		if (!entry || entry.kind !== 'folder') return;
		const id = entry.id;
		onClose();
		onOpenFolder(id);
	}

	function showProperties(): void {
		if (!entry) return;
		const selected = entry;
		onClose();
		onProperties(selected);
	}

	function downloadFile(): void {
		if (!entry || entry.kind !== 'file') return;
		const selected = entry;
		onClose();
		onDownload(selected);
	}

	function previewPdf(): void {
		if (!entry || entry.kind !== 'file' || !isPdfFile(entry)) return;
		const selected = entry;
		onClose();
		onPreviewPdf(selected);
	}

	function previewImage(): void {
		if (!entry || entry.kind !== 'file' || !isImageFile(entry.name, entry.type)) return;
		const selected = entry;
		onClose();
		onPreviewImage(selected);
	}

	function previewMedia(): void {
		if (!entry || entry.kind !== 'file' || !isMediaMimeType(entry.type)) return;
		const selected = entry;
		onClose();
		onPreviewMedia(selected);
	}

	function previewText(): void {
		if (!entry || entry.kind !== 'file' || !isTextFile(entry)) return;
		const selected = entry;
		onClose();
		onPreviewText(selected);
	}

	function confirmDelete(): void {
		if (!entry) return;
		const selected = entry;
		onClose();
		onDelete(selected);
	}

	function createFolder(): void {
		onClose();
		onCreateFolder(entry);
	}

	function createTextFile(): void {
		onClose();
		onCreateTextFile();
	}

	function uploadFiles(): void {
		onClose();
		onUploadFiles();
	}

	function uploadFolder(): void {
		onClose();
		onUploadFolder();
	}
</script>

<svelte:window onpointerdown={closeOutside} onkeydown={handleKeydown} />

{#if open}
	<div
		bind:this={menu}
		class="fixed z-50 grid w-52 gap-1 rounded-md border border-(--border) bg-(--surface) p-1.5 shadow-xl"
		style={`left: ${position.left}px; top: ${position.top}px`}
		role="menu"
		aria-label={entry ? `${entry.name} actions` : 'Files actions'}
	>
		{#if !entry}
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={uploadFiles}
			>
				<CloudUpload class="size-4 text-(--muted)" aria-hidden="true" />
				<span>Upload files</span>
			</button>
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={uploadFolder}
			>
				<FolderOpen class="size-4 text-(--muted)" aria-hidden="true" />
				<span>Upload folder</span>
			</button>
		{/if}
		{#if entry?.kind === 'folder'}
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={openFolder}
			>
				<FolderOpen class="size-4 text-(--muted)" aria-hidden="true" />
				<span>Open folder</span>
			</button>
		{/if}
		{#if entry?.kind === 'file'}
			{#if isImageFile(entry.name, entry.type)}
				<button
					class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
					type="button"
					role="menuitem"
					onclick={previewImage}
				>
					<Image class="size-4 text-(--muted)" aria-hidden="true" />
					<span>Preview</span>
				</button>
			{:else if isMediaMimeType(entry.type)}
				<button
					class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
					type="button"
					role="menuitem"
					onclick={previewMedia}
				>
					{#if isAudioMimeType(entry.type)}<AudioLines
							class="size-4 text-(--muted)"
							aria-hidden="true"
						/>{:else}<Film class="size-4 text-(--muted)" aria-hidden="true" />{/if}
					<span>Preview</span>
				</button>
			{:else if isPdfFile(entry)}
				<button
					class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
					type="button"
					role="menuitem"
					onclick={previewPdf}
				>
					<FileText class="size-4 text-(--muted)" aria-hidden="true" />
					<span>Preview</span>
				</button>
			{:else if isTextFile(entry)}
				<button
					class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
					type="button"
					role="menuitem"
					onclick={previewText}
				>
					<FileText class="size-4 text-(--muted)" aria-hidden="true" />
					<span>Open</span>
				</button>
			{/if}
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={downloadFile}
			>
				<Download class="size-4 text-(--muted)" aria-hidden="true" />
				<span>Download</span>
			</button>
		{/if}
		{#if entry?.kind !== 'folder'}
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={createTextFile}
			>
				<FileText class="size-4 text-(--muted)" aria-hidden="true" />
				<span>New text file</span>
			</button>
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={createFolder}
			>
				<FolderPlus class="size-4 text-(--muted)" aria-hidden="true" />
				<span>New folder here</span>
			</button>
		{/if}
		{#if entry}
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)"
				type="button"
				role="menuitem"
				onclick={showProperties}
			>
				<Info class="size-4 text-(--muted)" aria-hidden="true" />
				<span>Properties</span>
			</button>
			<button
				class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm text-red-400 hover:bg-red-500/10"
				type="button"
				role="menuitem"
				onclick={confirmDelete}
			>
				<Trash2 class="size-4" aria-hidden="true" />
				<span>Delete</span>
			</button>
		{/if}
	</div>
{/if}
