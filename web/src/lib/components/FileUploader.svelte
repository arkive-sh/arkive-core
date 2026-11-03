<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { UploadItem } from '#lib/transfers/manager.ts';

	let {
		unlocked,
		onFilesDropped,
		children,
	}: { unlocked: boolean; onFilesDropped: (files: UploadItem[]) => void; children: Snippet } =
		$props();
	let dragOver = $state(false);
	let status = $state('');

	function leaveDropArea(event: DragEvent): void {
		if (
			event.relatedTarget instanceof Node &&
			event.currentTarget instanceof Node &&
			event.currentTarget.contains(event.relatedTarget)
		)
			return;
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
			const entries = await new Promise<FileSystemEntry[]>((resolve, reject) =>
				reader.readEntries(resolve, reject),
			);

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
			onFilesDropped(
				Array.from(dataTransfer.files, (file) => ({ file, relativePath: file.webkitRelativePath })),
			);

			return;
		}

		try {
			const files: UploadItem[] = [];

			for (const entry of entries) files.push(...(await readDroppedEntry(entry)));
			onFilesDropped(files);
		} catch (error) {
			status = error instanceof Error ? error.message : 'Could not read dropped files.';
		}
	}
</script>

<div
	class="relative"
	role="region"
	aria-label="Files and folders upload area. Drop files or folders here."
	ondragover={(event) => {
		if (unlocked) {
			event.preventDefault();
			dragOver = true;
		}
	}}
	ondragleave={leaveDropArea}
	ondrop={dropFiles}
>
	{@render children()}
	{#if dragOver}
		<div
			class="pointer-events-none absolute inset-2 z-20 grid place-items-center rounded-md border-2 border-dashed border-(--accent) bg-(--surface)/90 text-center text-base font-semibold text-(--text)"
			role="status"
		>
			Drop files or folders to upload here
		</div>
	{/if}
	<p class="sr-only" role="status" aria-live="polite">{status}</p>
</div>
