<script lang="ts">
	import { onDestroy } from 'svelte';
	import { CloudUpload, File as FileIcon } from '@lucide/svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { uploadFile } from '#lib/uploads/upload-file.ts';

	type Props = { onUploaded?: () => void | Promise<void> };
	type JobStatus = 'queued' | 'running' | 'completed' | 'failed' | 'canceled';
	type UploadJob = { id: string; file: File; status: JobStatus; uploadedBytes: number; error?: string };
	let { onUploaded }: Props = $props();

	let selectedFiles = $state<File[]>([]);
	let queue = $state<UploadJob[]>([]);
	let uploading = $state(false);
	let dragOver = $state(false);
	let confirmOpen = $state(false);
	let status = $state('Select files manually to start a secure upload.');
	let errorMessage = $state('');
	let fileInput: HTMLInputElement | undefined;
	let controller: AbortController | undefined;

	function formatBytes(bytes: number): string {
		if (bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB'];
		const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
		return `${(bytes / 1024 ** unit).toFixed(unit === 0 ? 0 : unit === 3 ? 2 : 1)} ${units[unit]}`;
	}

	function stageFiles(files: FileList | File[]): void {
		if (uploading || !files.length) return;
		selectedFiles = Array.from(files);
		status = '';
		errorMessage = '';
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

	function updateJob(index: number, update: Partial<UploadJob>): void {
		queue = queue.map((job, jobIndex) => jobIndex === index ? { ...job, ...update } : job);
	}

	async function startUpload(): Promise<void> {
		if (uploading || !selectedFiles.length) return;
		const files = selectedFiles.slice();
		selectedFiles = [];
		confirmOpen = false;
		queue = files.map((file) => ({ id: crypto.randomUUID(), file, status: 'queued', uploadedBytes: 0 }));
		const uploadController = new AbortController();
		controller = uploadController;
		errorMessage = '';
		status = '';
		uploading = true;
		let completed = 0;

		try {
			for (const [index, file] of files.entries()) {
				if (uploadController.signal.aborted) {
					for (let rest = index; rest < files.length; rest += 1) updateJob(rest, { status: 'canceled' });
					break;
				}
				updateJob(index, { status: 'running' });
				status = `Encrypting and uploading ${file.name} (${index + 1} of ${files.length})`;
				try {
					await uploadFile(file, uploadController.signal, (uploadedBytes) => updateJob(index, { uploadedBytes }));
					updateJob(index, { status: 'completed', uploadedBytes: file.size });
					completed += 1;
				} catch (error) {
					if (uploadController.signal.aborted || (error instanceof Error && error.name === 'AbortError')) {
						updateJob(index, { status: 'canceled' });
						for (let rest = index + 1; rest < files.length; rest += 1) updateJob(rest, { status: 'canceled' });
					} else {
						const message = error instanceof Error ? error.message : 'Upload failed.';
						updateJob(index, { status: 'failed', error: message });
						for (let rest = index + 1; rest < files.length; rest += 1) updateJob(rest, { status: 'canceled' });
						errorMessage = message;
					}
					break;
				}
			}

			if (uploadController.signal.aborted) status = `Upload canceled after ${completed} ${completed === 1 ? 'file' : 'files'}.`;
			else if (errorMessage) status = `Uploaded ${completed} of ${files.length} files.`;
			else status = `Uploaded ${completed} ${completed === 1 ? 'file' : 'files'}.`;
		} finally {
			uploading = false;
			controller = undefined;
			if (completed > 0) void Promise.resolve(onUploaded?.()).catch(() => {});
		}
	}

	function cancelUpload(): void {
		controller?.abort();
	}

	onDestroy(cancelUpload);
</script>

<section class="grid gap-3" id="upload-panel" aria-label="Secure file upload">
	<input bind:this={fileInput} id="upload-files-input" class="sr-only" type="file" multiple disabled={uploading} onchange={selectFiles} />
	<div
		class={`grid min-h-[360px] cursor-pointer justify-items-center content-center gap-4 rounded-[14px] border border-dashed border-(--border) bg-(--surface) px-6 py-10 text-center transition-[border-color,background-color] duration-200 hover:border-(--accent) hover:bg-(--surface-raised) max-[760px]:min-h-[280px] max-[760px]:px-4 max-[760px]:py-8 ${dragOver ? 'border-(--accent)! bg-(--surface-raised)!' : ''} ${uploading ? 'pointer-events-none opacity-60' : ''}`}
		role="button"
		tabindex={uploading ? -1 : 0}
		aria-label="Secure Upload. Drop files here or select files manually."
		aria-describedby="upload-helper"
		onclick={() => !uploading && fileInput?.click()}
		onkeydown={dropzoneKeydown}
		ondragover={(event) => { event.preventDefault(); if (!uploading) dragOver = true; }}
		ondragleave={() => dragOver = false}
		ondrop={dropFiles}
	>
		<span class="grid size-[84px] place-items-center rounded-md border border-(--border) bg-(--surface-highest)">
			<CloudUpload class="size-[34px] stroke-2 text-(--accent-strong)" aria-hidden="true" />
		</span>
		<div class="grid max-w-[620px] justify-items-center gap-3">
			<h1 class="m-0 text-2xl leading-8 font-semibold">Secure Upload</h1>
			<p id="upload-helper" class="m-0 text-lg leading-7 text-(--muted)">
				Drag and drop files here to begin encrypted upload. Data is encrypted in browser before transfer.
			</p>
			<div class="mt-2 flex justify-center">
				<button
					class="inline-flex min-h-10 min-w-[280px] cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-transparent px-4 text-sm leading-5 font-semibold text-(--text) shadow-none transition-colors hover:bg-(--surface-highest) disabled:cursor-not-allowed disabled:opacity-60 max-[760px]:w-full max-[760px]:min-w-0"
					type="button"
					disabled={uploading}
					onclick={(event) => { event.stopPropagation(); fileInput?.click(); }}
				>Select Files Manually</button>
			</div>
		</div>
	</div>

	<p class="m-0 text-sm leading-5 text-(--muted)" role="status" aria-live="polite">{status}</p>
	{#if errorMessage}<p class="m-0 text-sm text-(--danger)" role="alert">{errorMessage}</p>{/if}

	<section class="overflow-hidden rounded-md border border-(--border) bg-(--surface-raised)" aria-label="Upload queue">
		<header class="flex items-center justify-between gap-3 border-b border-(--border) px-4 py-3">
			<h2 class="m-0 text-[11px] leading-4 font-semibold tracking-wider text-(--muted) uppercase">Queue</h2>
			<span class="text-sm text-(--muted)">{queue.filter((job) => job.status === 'running').length} items active</span>
		</header>
		{#if uploading}
			<div class="flex justify-end border-b border-(--border) px-4 py-2">
				<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-[rgba(147,0,10,0.18)] px-4 text-sm font-semibold text-(--danger) shadow-none transition-colors hover:bg-[rgba(147,0,10,0.26)]" type="button" onclick={cancelUpload}>Cancel all</button>
			</div>
		{/if}
		{#if queue.length}
			<ul class="m-0 grid list-none p-0">
				{#each queue as job (job.id)}
					<li class={`grid gap-2.5 border-b border-(--border) px-4 py-3 last:border-0 ${job.status === 'completed' ? 'bg-[rgba(56,189,248,0.03)]' : ''} ${job.status === 'failed' || job.status === 'canceled' ? 'bg-[rgba(147,0,10,0.08)]' : ''}`}>
						<div class="flex min-w-0 items-center justify-between gap-2">
							<div class="flex min-w-0 items-center gap-2">
								<FileIcon class="size-5 shrink-0 text-(--muted-strong)" aria-hidden="true" />
								<span class="truncate text-sm font-semibold">{job.file.name}</span>
							</div>
							<span class={`shrink-0 rounded-full border border-(--border) px-2 py-0.5 text-[10px] leading-[1.4] tracking-wider text-(--muted) uppercase ${job.status === 'running' || job.status === 'completed' ? 'border-[rgba(56,189,248,0.24)] bg-[rgba(56,189,248,0.08)] text-(--accent)' : ''} ${job.status === 'failed' || job.status === 'canceled' ? 'border-[rgba(147,0,10,0.3)] bg-[rgba(147,0,10,0.14)] text-(--danger)' : ''}`}>{job.status}</span>
						</div>
						<div class="h-1 overflow-hidden rounded-sm bg-(--surface-highest)">
							<span class="block h-full rounded-sm bg-(--accent) transition-[width] duration-200" style={`width: ${job.file.size ? Math.round(job.uploadedBytes / job.file.size * 100) : 0}%`}></span>
						</div>
						<div class="flex items-center justify-between gap-2 text-xs text-(--muted)">
							<span class="font-mono">{formatBytes(job.uploadedBytes)} / {formatBytes(job.file.size)}</span>
							<span>{job.error || (job.status === 'running' ? 'Encrypting and uploading' : job.status)}</span>
						</div>
						{#if job.error}<p class="m-0 text-sm text-(--danger)" role="alert">{job.error}</p>{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<p class="m-0 px-4 py-5 text-sm text-(--muted)">No active transfers in queue.</p>
		{/if}
	</section>
</section>

<Modal open={confirmOpen} label="Confirm upload" title="Start upload?" onClose={cancelSelection}>
	<p class="m-0 text-sm text-(--muted)">{selectedFiles.length} file(s) selected.</p>
	{#snippet actions()}
		<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) bg-transparent px-4 text-sm font-semibold text-(--text) shadow-none transition-colors hover:bg-(--surface-highest)" type="button" onclick={cancelSelection}>Cancel</button>
		<button class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--accent) bg-(--accent) px-4 text-sm font-semibold text-(--accent-ink) transition-colors hover:border-(--accent-strong) hover:bg-(--accent-strong)" type="button" onclick={() => void startUpload()}>Start upload</button>
	{/snippet}
</Modal>
