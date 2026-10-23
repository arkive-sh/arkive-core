<script lang="ts">
	import { onMount } from 'svelte';
	import { ArrowDownUp, ChevronDown, ChevronUp, Download, RotateCcw, Trash2, Upload, X } from '@lucide/svelte';
	import type { TransferManager, TransferState } from '#lib/transfers/manager.ts';

	let { manager }: { manager: TransferManager } = $props();
	let transfers = $state<TransferState>({ jobs: [], open: false });
	const activeCount = $derived(transfers.jobs.filter((job) => job.status === 'queued' || job.status === 'running').length);
	const completedCount = $derived(transfers.jobs.filter((job) => job.status === 'completed').length);

	onMount(() => manager.state.subscribe((next) => transfers = next));

	function formatBytes(bytes: number): string {
		if (bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
		return `${(bytes / 1024 ** unit).toFixed(unit === 0 ? 0 : unit >= 3 ? 2 : 1)} ${units[unit]}`;
	}
</script>

{#if transfers.jobs.length}
	<div class="fixed right-4 bottom-4 z-50 w-[min(420px,calc(100vw-2rem))]">
		{#if transfers.open}
			<section class="overflow-hidden rounded-lg border border-(--border) bg-(--surface) shadow-xl" aria-label="Transfers">
				<header class="flex items-center justify-between gap-3 border-b border-(--border) px-4 py-3">
					<div class="min-w-0">
						<h2 class="m-0 text-sm leading-5 font-semibold">Transfers</h2>
						<p class="m-0 text-xs text-(--muted)">{activeCount} active</p>
					</div>
					<div class="flex items-center gap-1">
						{#if activeCount}
							<button class="cursor-pointer rounded-md px-2 py-1 text-xs text-(--muted) hover:bg-(--surface-highest) hover:text-(--danger)" type="button" onclick={() => void manager.cancelAll()}>Cancel all</button>
						{/if}
						<button class="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" aria-label="Minimize transfers" onclick={() => manager.setOpen(false)}>
							<ChevronDown class="size-4" aria-hidden="true" />
						</button>
					</div>
				</header>
				<ul class="m-0 max-h-[min(60vh,28rem)] list-none overflow-y-auto p-0">
					{#each transfers.jobs as job (job.id)}
						<li class="grid gap-2 border-b border-(--border) px-4 py-3 last:border-0">
							<div class="flex min-w-0 items-center gap-2">
								{#if job.kind === 'upload'}
									<Upload class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
								{:else}
									<Download class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
								{/if}
								<span class="min-w-0 flex-1 truncate text-sm font-medium" title={job.fileName}>{job.fileName}</span>
								<span class="shrink-0 text-xs capitalize text-(--muted)">{job.status}</span>
							</div>
							<div class="h-1 overflow-hidden rounded-full bg-(--surface-highest)" role="progressbar" aria-label={`${job.kind} progress for ${job.fileName}`} aria-valuemin="0" aria-valuemax={job.totalBytes} aria-valuenow={job.transferredBytes}>
								<span class="block h-full rounded-full bg-(--accent) transition-[width] duration-200" style={`width: ${job.totalBytes ? Math.min(100, job.transferredBytes / job.totalBytes * 100) : 0}%`}></span>
							</div>
							<div class="flex items-center justify-between gap-2 text-xs text-(--muted)">
								<span class="font-mono">{formatBytes(job.transferredBytes)} / {formatBytes(job.totalBytes)}</span>
								<div class="flex items-center gap-1">
									{#if job.status === 'queued' || job.status === 'running'}
										<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-(--muted) hover:bg-(--surface-highest) hover:text-(--danger)" type="button" aria-label={`Cancel ${job.fileName}`} onclick={() => manager.cancel(job.id)}><X class="size-4" aria-hidden="true" /></button>
									{:else if job.status === 'failed' || job.status === 'canceled'}
										<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" aria-label={`Retry ${job.fileName}`} onclick={() => manager.retry(job.id)}><RotateCcw class="size-4" aria-hidden="true" /></button>
										<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" aria-label={`Remove ${job.fileName}`} onclick={() => manager.remove(job.id)}><Trash2 class="size-4" aria-hidden="true" /></button>
									{:else}
										<button class="inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)" type="button" aria-label={`Remove ${job.fileName}`} onclick={() => manager.remove(job.id)}><Trash2 class="size-4" aria-hidden="true" /></button>
									{/if}
								</div>
							</div>
							{#if job.error}<p class="m-0 text-xs text-(--danger)" role="alert">{job.error}</p>{/if}
						</li>
					{/each}
				</ul>
				{#if completedCount}
					<footer class="flex justify-end border-t border-(--border) px-4 py-2">
						<button class="cursor-pointer text-xs text-(--muted) hover:text-(--text)" type="button" onclick={() => manager.clearCompleted()}>Clear completed</button>
					</footer>
				{/if}
			</section>
		{:else}
			<button class="ml-auto flex min-h-11 cursor-pointer items-center gap-2 rounded-full border border-(--border) bg-(--surface) px-4 text-sm font-semibold shadow-lg hover:bg-(--surface-raised)" type="button" aria-label="Open transfers" onclick={() => manager.setOpen(true)}>
				<ArrowDownUp class="size-4" aria-hidden="true" />
				<span>{activeCount ? `${activeCount} active` : 'Transfers'}</span>
				<ChevronUp class="size-4 text-(--muted)" aria-hidden="true" />
			</button>
		{/if}
	</div>
{/if}
