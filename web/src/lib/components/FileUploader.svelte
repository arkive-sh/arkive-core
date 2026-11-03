<script lang="ts">
	import type { Snippet } from 'svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { useTransferManager, type UploadItem } from '#lib/transfers/manager.ts';

	let { vaultId, folderId, unlocked, children }: { vaultId: string; folderId: string | null; unlocked: boolean; children: Snippet } = $props();
	const transfers = useTransferManager();
	let selectedFiles = $state<UploadItem[]>([]);
	let dragOver = $state(false);
	let confirmOpen = $state(false);
	let status = $state('');
	let fileInput: HTMLInputElement | undefined;

	function stageFiles(files: UploadItem[]): void {
		if (!files.length) return;
		selectedFiles = files;
		status = '';
		confirmOpen = true;
	}

	function selectFiles(event: Event): void {
		const input = event.currentTarget;
		if (!(input instanceof HTMLInputElement)) return;
		stageFiles(Array.from(input.files ?? [], (file) => ({ file, relativePath: file.webkitRelativePath })));
		input.value = '';
	}

	function openPickerFromBackground(event: PointerEvent): void {
		if (!unlocked || event.button !== 0 || !(event.target instanceof Element)) return;
		if (event.target.closest('button, input, a, [data-file-entry], thead, footer, [role="menu"], [data-upload-click-ignore]')) return;
		fileInput?.click();
	}

	function leaveDropArea(event: DragEvent): void {
		if (event.relatedTarget instanceof Node && event.currentTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
		dragOver = false;
	}

	async function readDroppedEntry(entry: FileSystemEntry, parentPath = ''): Promise<UploadItem[]> {
		const relativePath = parentPath ? `${parentPath}/${entry.name}` : entry.name;
		if (entry.isFile) {
			return new Promise((resolve, reject) => {
				(entry as FileSystemFileEntry).file((file) => resolve([{ file, relativePath }]), reject);
			});
		}
		if (!entry.isDirectory) return [];

		const reader = (entry as FileSystemDirectoryEntry).createReader();
		const files: UploadItem[] = [];
		while (true) {
			const entries = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject));
			if (!entries.length) return files;
			for (const child of entries) files.push(...(await readDroppedEntry(child, relativePath)));
		}
	}

	async function dropFiles(event: DragEvent): Promise<void> {
		event.preventDefault();
		if (!unlocked) return;
		dragOver = false;
		const dataTransfer = event.dataTransfer;
		if (!dataTransfer) return;

		const entries = Array.from(dataTransfer.items)
			.map((item) => item.webkitGetAsEntry())
			.filter((entry): entry is FileSystemEntry => entry !== null);
		if (!entries.length) {
			stageFiles(Array.from(dataTransfer.files, (file) => ({ file, relativePath: file.webkitRelativePath })));
			return;
		}

		try {
			const files: UploadItem[] = [];
			for (const entry of entries) files.push(...(await readDroppedEntry(entry)));
			stageFiles(files);
		} catch (error) {
			status = error instanceof Error ? error.message : 'Could not read dropped files.';
		}
	}

	function cancelSelection(): void {
		confirmOpen = false;
		selectedFiles = [];
	}

	function startUpload(): void {
		if (!selectedFiles.length) return;
		const count = transfers.enqueueUploads(selectedFiles, vaultId, folderId);
		selectedFiles = [];
		confirmOpen = false;
		status = `Added ${count} ${count === 1 ? 'file' : 'files'} to transfers.`;
	}
</script>

<div
	class="relative"
	role="region"
	aria-label="Files and folders upload area. Drop files or folders here."
	onpointerup={openPickerFromBackground}
	ondragover={(event) => { if (unlocked) { event.preventDefault(); dragOver = true; } }}
	ondragleave={leaveDropArea}
	ondrop={dropFiles}
>
	{@render children()}
	{#if dragOver}
		<div class="pointer-events-none absolute inset-2 z-20 grid place-items-center rounded-md border-2 border-dashed border-(--accent) bg-(--surface)/90 text-center text-base font-semibold text-(--text)" role="status">
			Drop files or folders to upload here
		</div>
	{/if}
	<input bind:this={fileInput} class="sr-only" type="file" multiple onchange={selectFiles} />
	<button class="sr-only focus:not-sr-only focus:fixed focus:bottom-4 focus:left-4 focus:z-50 focus:rounded focus:bg-(--surface) focus:p-3" type="button" aria-label="Choose files to upload" onclick={() => fileInput?.click()}>
		Choose files to upload
	</button>
	<p class="sr-only" role="status" aria-live="polite">{status}</p>
</div>

<Modal open={confirmOpen} label="Confirm upload" title="Start upload?" onClose={cancelSelection}>
	<p class="m-0 text-sm text-(--muted)">{selectedFiles.length} file(s) selected.</p>
	{#snippet actions()}
		<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-transparent px-4 text-sm font-semibold text-(--text) shadow-none transition-colors hover:bg-(--surface-highest)" type="button" onclick={cancelSelection}>Cancel</button>
		<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--accent) bg-(--accent) px-4 text-sm font-semibold text-(--accent-ink) transition-colors hover:border-(--accent-strong) hover:bg-(--accent-strong)" type="button" onclick={startUpload}>Start upload</button>
	{/snippet}
</Modal>
