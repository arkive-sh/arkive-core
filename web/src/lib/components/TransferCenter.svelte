<script lang="ts">
	import { onMount } from 'svelte';
	import {
		ArrowDownUp,
		ChevronDown,
		ChevronUp,
		Download,
		GripVertical,
		RotateCcw,
		Trash2,
		Upload,
		X,
	} from '@lucide/svelte';
	import { createVirtualizer } from '@tanstack/svelte-virtual';
	import { get } from 'svelte/store';
	import Tooltip from '#lib/components/Tooltip.svelte';
	import type { TransferManager, TransferState } from '#lib/transfers/manager.ts';
	import { formatBytes } from '#lib/utils/format.ts';

	let { manager }: { manager: TransferManager } = $props();
	let transfers = $state<TransferState>({ jobs: [], open: false });
	let transferListViewport = $state<HTMLDivElement>();
	let selectAllCheckbox = $state<HTMLInputElement>();
	let selectedIds = $state<Set<string>>(new Set());
	const counts = $derived.by(() => {
		let active = 0;
		let completed = 0;
		let selected = 0;
		let selectedRetryable = 0;
		let selectedRemovable = 0;

		for (const job of transfers.jobs) {
			const isActive = job.status === 'queued' || job.status === 'running';
			const isRetryable = job.status === 'failed' || job.status === 'canceled';

			if (isActive) active++;
			if (job.status === 'completed') completed++;
			if (!selectedIds.has(job.id)) continue;
			selected++;
			if (isRetryable) selectedRetryable++;
			if (job.status !== 'queued' && job.status !== 'running') selectedRemovable++;
		}

		return { active, completed, selected, selectedRetryable, selectedRemovable };
	});
	const allSelected = $derived(
		transfers.jobs.length > 0 && counts.selected === transfers.jobs.length,
	);
	const rowVirtualizer = createVirtualizer<HTMLDivElement, HTMLLIElement>({
		count: 0,
		getScrollElement: () => transferListViewport ?? null,
		estimateSize: () => 96,
		overscan: 6,
		getItemKey: (index) => transfers.jobs[index]?.id ?? index,
	});
	const virtualRows = $derived($rowVirtualizer.getVirtualItems());
	const totalListSize = $derived($rowVirtualizer.getTotalSize());

	$effect(() => {
		if (selectAllCheckbox) selectAllCheckbox.indeterminate = counts.selected > 0 && !allSelected;
	});

	$effect(() => {
		const count = transfers.jobs.length;
		const scrollElement = transferListViewport;
		const virtualizer = get(rowVirtualizer);

		if (
			virtualizer.options.count !== count ||
			virtualizer.options.getScrollElement() !== scrollElement
		) {
			virtualizer.setOptions({ count, getScrollElement: () => scrollElement ?? null });
		}
	});

	function measureTransferRow(element: HTMLLIElement, index: number) {
		const measure = () => {
			element.dataset.index = String(index);
			get(rowVirtualizer).measureElement(element);
		};
		measure();

		return {
			update(nextIndex: number) {
				index = nextIndex;
				measure();
			},
		};
	}

	function toggleSelection(id: string): void {
		const next = new Set(selectedIds);

		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function toggleAll(): void {
		selectedIds = allSelected ? new Set() : new Set(transfers.jobs.map((job) => job.id));
	}

	function clearCompleted(): void {
		const completed = new Set(
			transfers.jobs.filter((job) => job.status === 'completed').map((job) => job.id),
		);
		manager.clearCompleted();
		selectedIds = new Set([...selectedIds].filter((id) => !completed.has(id)));
	}

	function removeSelected(): void {
		const removable = new Set(
			transfers.jobs
				.filter(
					(job) => selectedIds.has(job.id) && job.status !== 'queued' && job.status !== 'running',
				)
				.map((job) => job.id),
		);
		manager.removeMany([...removable]);
		selectedIds = new Set([...selectedIds].filter((id) => !removable.has(id)));
	}

	onMount(() => manager.state.subscribe((next) => (transfers = next)));
</script>

<div class="fixed right-0 bottom-4 z-50" style="width: min(420px, calc(100vw - 1rem))">
	{#if transfers.open}
		<section
			id="transfer-center-panel"
			class="mr-2 overflow-hidden rounded-lg border border-(--border) bg-(--surface) shadow-xl"
			aria-label="Transfers"
		>
			<header class="flex items-center justify-between gap-3 border-b border-(--border) px-4 py-3">
				<div class="min-w-0">
					<h2 class="m-0 text-sm leading-5 font-semibold">Transfers</h2>
					<p class="m-0 text-xs text-(--muted)">{counts.active} active</p>
				</div>
				<div class="flex items-center gap-1">
					{#if counts.active}
						<button
							class="cursor-pointer rounded-md px-2 py-1 text-xs text-(--muted) hover:bg-(--surface-highest) hover:text-(--danger)"
							type="button"
							onclick={() => void manager.cancelAll()}>Cancel all</button
						>
					{/if}
					<Tooltip label="Minimize transfers" side="left">
						<button
							class="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
							type="button"
							aria-label="Minimize transfers"
							onclick={() => manager.setOpen(false)}
						>
							<ChevronDown class="size-4" aria-hidden="true" />
						</button>
					</Tooltip>
				</div>
			</header>
			{#if transfers.jobs.length}
				<div
					class="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b border-(--border) px-4 py-2"
				>
					<label class="inline-flex cursor-pointer items-center gap-2 text-xs text-(--muted)">
						<input
							bind:this={selectAllCheckbox}
							class="size-4 cursor-pointer accent-(--accent)"
							type="checkbox"
							checked={allSelected}
							aria-label="Select all transfers"
							onchange={toggleAll}
						/>
						{#if counts.selected}<span>{counts.selected} selected</span>{/if}
					</label>
					{#if counts.selected}
						<div class="flex flex-wrap items-center justify-end gap-1">
							{#if counts.selectedRetryable}
								<button
									class="inline-flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-xs text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
									type="button"
									onclick={() => manager.retryMany([...selectedIds])}
									><RotateCcw class="size-3.5" aria-hidden="true" />Retry</button
								>
							{/if}
							{#if counts.selectedRemovable}
								<button
									class="inline-flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-xs text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
									type="button"
									onclick={removeSelected}
									><Trash2 class="size-3.5" aria-hidden="true" />Remove</button
								>
							{/if}
							<Tooltip label="Clear selection" side="left">
								<button
									class="inline-flex size-7 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-highest) hover:text-(--text)"
									type="button"
									aria-label="Clear selection"
									onclick={() => (selectedIds = new Set())}
									><X class="size-4" aria-hidden="true" /></button
								>
							</Tooltip>
						</div>
					{/if}
				</div>
			{/if}
			{#if transfers.jobs.length}
				<div bind:this={transferListViewport} class="max-h-[min(60vh,28rem)] overflow-y-auto">
					<ul class="relative m-0 list-none p-0" style={`height: ${totalListSize}px`}>
						{#each virtualRows as virtualRow (virtualRow.key)}
							{@const job = transfers.jobs[virtualRow.index]}
							<li
								use:measureTransferRow={virtualRow.index}
								class={`absolute top-0 left-0 grid w-full gap-2 border-b border-(--border) px-4 py-3 ${selectedIds.has(job.id) ? 'bg-(--surface-highest)' : 'bg-(--surface)'}`}
								style={`transform: translateY(${virtualRow.start}px)`}
							>
								<div class="flex min-w-0 items-center gap-2">
									<input
										class="size-4 shrink-0 cursor-pointer accent-(--accent)"
										type="checkbox"
										checked={selectedIds.has(job.id)}
										aria-label={`Select ${job.fileName}`}
										onchange={() => toggleSelection(job.id)}
									/>
									{#if job.kind === 'upload'}
										<Upload class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
									{:else}
										<Download class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
									{/if}
									<span class="min-w-0 flex-1 truncate text-sm font-medium" title={job.fileName}
										>{job.fileName}</span
									>
									<span class="shrink-0 text-xs capitalize text-(--muted)">{job.status}</span>
								</div>
								<div
									class="h-1 overflow-hidden rounded-full bg-(--surface-highest)"
									role="progressbar"
									aria-label={`${job.kind} progress for ${job.fileName}`}
									aria-valuemin="0"
									aria-valuemax={job.totalBytes}
									aria-valuenow={job.transferredBytes}
								>
									<span
										class="block h-full rounded-full bg-(--accent) transition-[width] duration-200"
										style={`width: ${job.totalBytes ? Math.min(100, (job.transferredBytes / job.totalBytes) * 100) : 0}%`}
									></span>
								</div>
								<div class="flex items-center justify-between gap-2 text-xs text-(--muted)">
									<span class="font-mono"
										>{formatBytes(job.transferredBytes)} / {formatBytes(job.totalBytes)}</span
									>
								</div>
								{#if job.error}<p class="m-0 text-xs text-(--danger)" role="alert">
										{job.error}
									</p>{/if}
							</li>
						{/each}
					</ul>
				</div>
			{:else}
				<p class="m-0 px-4 py-8 text-center text-sm text-(--muted)" role="status">
					Your uploads and downloads will appear here.
				</p>
			{/if}
			{#if counts.completed}
				<footer class="flex justify-end border-t border-(--border) px-4 py-2">
					<button
						class="cursor-pointer text-xs text-(--muted) hover:text-(--text)"
						type="button"
						onclick={clearCompleted}>Clear completed</button
					>
				</footer>
			{/if}
		</section>
	{:else}
		<button
			class="group relative ml-auto flex h-12 w-6 cursor-pointer items-center overflow-hidden rounded-l-full border border-r-0 border-l-2 border-(--border) bg-(--surface-raised) p-0 text-sm font-semibold shadow-lg transition-[width,background-color,border-color] duration-200 hover:w-40 hover:border-(--accent) hover:bg-(--surface-highest) focus-visible:w-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent)"
			style="border-left-color: var(--accent)"
			type="button"
			aria-label="Open transfers"
			aria-controls="transfer-center-panel"
			aria-expanded={transfers.open}
			onclick={() => manager.setOpen(true)}
		>
			<GripVertical
				class="absolute left-1 size-3 text-(--muted) transition-opacity duration-150 group-hover:opacity-0 group-focus-visible:opacity-0"
				aria-hidden="true"
			/>
			<span
				class="flex h-full w-40 shrink-0 items-center gap-2 px-3 opacity-0 transition-opacity duration-150 group-hover:opacity-100 group-focus-visible:opacity-100"
			>
				<ArrowDownUp class="size-4 shrink-0" aria-hidden="true" />
				<span>{counts.active ? `${counts.active} active` : 'Transfers'}</span>
				<ChevronUp class="size-4 shrink-0 text-(--muted)" aria-hidden="true" />
			</span>
		</button>
	{/if}
</div>
