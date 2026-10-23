<script lang="ts">
	import { CloudUpload } from '@lucide/svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { useTransferManager } from '#lib/transfers/manager.ts';

	const transfers = useTransferManager();
	let selectedFiles = $state<File[]>([]);
	let dragOver = $state(false);
	let confirmOpen = $state(false);
	let status = $state('Select files manually to start a secure upload.');
	let fileInput: HTMLInputElement | undefined;

	function stageFiles(files: FileList | File[]): void {
		if (!files.length) return;
		selectedFiles = Array.from(files);
		status = '';
		confirmOpen = true;
	}

	function selectFiles(event: Event): void {
		const input = event.currentTarget;
		if (!(input instanceof HTMLInputElement)) return;
		stageFiles(input.files ?? []);
		input.value = '';
	}

	function dropFiles(event: DragEvent): void {
		event.preventDefault();
		dragOver = false;
		if (event.dataTransfer?.files) stageFiles(event.dataTransfer.files);
	}

	function dropzoneKeydown(event: KeyboardEvent): void {
		if (event.target !== event.currentTarget || (event.key !== 'Enter' && event.key !== ' ')) return;
		event.preventDefault();
		fileInput?.click();
	}

	function cancelSelection(): void {
		confirmOpen = false;
		selectedFiles = [];
	}

	function startUpload(): void {
		if (!selectedFiles.length) return;
		const count = transfers.enqueueUploads(selectedFiles);
		selectedFiles = [];
		confirmOpen = false;
		status = `Added ${count} ${count === 1 ? 'file' : 'files'} to transfers.`;
	}
</script>

<section class="grid gap-3" id="upload-panel" aria-label="Secure file upload">
	<input bind:this={fileInput} id="upload-files-input" class="sr-only" type="file" multiple onchange={selectFiles} />
	<div
		class={`grid min-h-90 cursor-pointer justify-items-center content-center gap-4 rounded-[14px] border border-dashed border-(--border) bg-(--surface) px-6 py-10 text-center transition-[border-color,background-color] duration-200 hover:border-(--accent) hover:bg-(--surface-raised) max-[760px]:min-h-70 max-[760px]:px-4 max-[760px]:py-8 ${dragOver ? 'border-(--accent)! bg-(--surface-raised)!' : ''}`}
		role="button"
		tabindex="0"
		aria-label="Secure Upload. Drop files here or select files manually."
		aria-describedby="upload-helper"
		onclick={() => fileInput?.click()}
		onkeydown={dropzoneKeydown}
		ondragover={(event) => { event.preventDefault(); dragOver = true; }}
		ondragleave={() => dragOver = false}
		ondrop={dropFiles}
	>
		<span class="grid size-21 place-items-center rounded-md border border-(--border) bg-(--surface-highest)">
			<CloudUpload class="size-8.5 stroke-2 text-(--accent-strong)" aria-hidden="true" />
		</span>
		<div class="grid max-w-155 justify-items-center gap-3">
			<h1 class="m-0 text-2xl leading-8 font-semibold">Secure Upload</h1>
			<p id="upload-helper" class="m-0 text-lg leading-7 text-(--muted)">
				Drag and drop files here to begin encrypted upload. Data is encrypted in browser before transfer.
			</p>
			<div class="mt-2 flex justify-center">
				<button
					class="inline-flex min-h-10 min-w-70 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-transparent px-4 text-sm leading-5 font-semibold text-(--text) shadow-none transition-colors hover:bg-(--surface-highest) max-[760px]:w-full max-[760px]:min-w-0"
					type="button"
					onclick={(event) => { event.stopPropagation(); fileInput?.click(); }}
				>Select Files Manually</button>
			</div>
		</div>
	</div>

	<p class="m-0 text-sm leading-5 text-(--muted)" role="status" aria-live="polite">{status}</p>
</section>

<Modal open={confirmOpen} label="Confirm upload" title="Start upload?" onClose={cancelSelection}>
	<p class="m-0 text-sm text-(--muted)">{selectedFiles.length} file(s) selected.</p>
	{#snippet actions()}
		<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-transparent px-4 text-sm font-semibold text-(--text) shadow-none transition-colors hover:bg-(--surface-highest)" type="button" onclick={cancelSelection}>Cancel</button>
		<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--accent) bg-(--accent) px-4 text-sm font-semibold text-(--accent-ink) transition-colors hover:border-(--accent-strong) hover:bg-(--accent-strong)" type="button" onclick={startUpload}>Start upload</button>
	{/snippet}
</Modal>
