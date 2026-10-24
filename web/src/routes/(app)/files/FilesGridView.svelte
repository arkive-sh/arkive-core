<script lang="ts">
	import { onDestroy } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { File as FileIcon, Folder } from '@lucide/svelte';
	import { api } from '#lib/api.ts';
	import { getCachedThumbnail, cacheThumbnail } from '#lib/thumbnail-cache.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import { decryptSearchThumbnail } from '#lib/vault/search.ts';
	import type { FileManagerEntry } from './types';

	const THUMBNAIL_CONCURRENCY = 4;
	let { rows, onOpenFolder }: { rows: FileManagerEntry[]; onOpenFolder: (id: string) => void } = $props();
	const thumbnails = new SvelteMap<string, string>();
	const objectUrls = new Map<string, string>();
	const pendingIds = new Set<string>();
	const attemptedIds = new Set<string>();
	const controller = new AbortController();
	let currentIds = new Set<string>();
	let queue: FileManagerEntry[] = [];
	let activeLoads = 0;

	async function loadThumbnail(entry: FileManagerEntry): Promise<void> {
		if (!entry.thumbnail) return;
		try {
			const preview = entry.thumbnail;
			let encryptedBytes = await getCachedThumbnail(entry.id, preview.version, preview.size);
			if (controller.signal.aborted || !currentIds.has(entry.id)) return;

			if (!encryptedBytes) {
				const { data } = await api.get<ArrayBuffer>(`/api/files/${encodeURIComponent(entry.id)}/thumbnail`, {
					responseType: 'arraybuffer',
					signal: controller.signal
				});
				if (controller.signal.aborted || !currentIds.has(entry.id)) return;
				encryptedBytes = new Uint8Array(data);
				void cacheThumbnail(entry.id, preview.version, preview.size, encryptedBytes).catch(() => {});
			}

			const { thumbnailBytes } = await decryptSearchThumbnail({
				id: entry.id,
				vaultId: preview.vaultId,
				encryptedMetadata: preview.encryptedMetadata,
				encryptedFileKey: preview.encryptedFileKey,
				score: 0,
				url: `/files/${entry.id}/view`
			}, encryptedBytes);
			if (controller.signal.aborted || !currentIds.has(entry.id) || !thumbnailBytes.length) {
				thumbnailBytes.fill(0);
				return;
			}

			const url = URL.createObjectURL(new Blob([Uint8Array.from(thumbnailBytes)], { type: preview.mime }));
			objectUrls.set(entry.id, url);
			thumbnails.set(entry.id, url);
			thumbnailBytes.fill(0);
		} catch {
			// Missing previews leave the file card's icon in place.
		}
	}

	function loadQueuedThumbnails(): void {
		while (activeLoads < THUMBNAIL_CONCURRENCY && queue.length) {
			const entry = queue.shift();
			if (!entry) return;
			if (!currentIds.has(entry.id)) {
				pendingIds.delete(entry.id);
				continue;
			}
			activeLoads += 1;
			void loadThumbnail(entry).finally(() => {
				activeLoads -= 1;
				pendingIds.delete(entry.id);
				loadQueuedThumbnails();
			});
		}
	}

	$effect(() => {
		currentIds = new Set(rows.map((entry) => entry.id));
		for (const id of attemptedIds) {
			if (!currentIds.has(id)) attemptedIds.delete(id);
		}
		for (const [id, url] of objectUrls) {
			if (currentIds.has(id)) continue;
			URL.revokeObjectURL(url);
			objectUrls.delete(id);
			thumbnails.delete(id);
		}
		for (const entry of queue) {
			if (!currentIds.has(entry.id)) pendingIds.delete(entry.id);
		}
		queue = queue.filter((entry) => currentIds.has(entry.id));
		for (const entry of rows) {
			if (entry.kind === 'file' && entry.thumbnail && !objectUrls.has(entry.id) && !pendingIds.has(entry.id) && !attemptedIds.has(entry.id)) {
				attemptedIds.add(entry.id);
				pendingIds.add(entry.id);
				queue.push(entry);
			}
		}
		loadQueuedThumbnails();
	});

	onDestroy(() => {
		controller.abort();
		for (const url of objectUrls.values()) URL.revokeObjectURL(url);
		objectUrls.clear();
		thumbnails.clear();
	});
</script>

<div class="grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-3 p-4 max-[640px]:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]">
	{#each rows as entry (`${entry.kind}:${entry.id}`)}
		<article class="min-w-0 overflow-hidden rounded-md border border-(--border) bg-(--surface) transition-colors hover:border-(--accent)">
			<div class="grid aspect-[4/3] place-items-center overflow-hidden bg-(--surface-highest)">
				{#if entry.kind === 'folder'}
					<Folder class="size-10 text-(--accent)" aria-hidden="true" />
				{:else if thumbnails.get(entry.id)}
					<img class="size-full object-cover" src={thumbnails.get(entry.id)} alt={entry.name} loading="lazy" decoding="async" />
				{:else}
					<FileIcon class="size-10 text-(--muted)" aria-hidden="true" />
				{/if}
			</div>
			<div class="grid min-w-0 gap-1 p-3">
				{#if entry.kind === 'folder'}
					<button class="min-w-0 cursor-pointer truncate text-left font-medium hover:text-(--accent)" type="button" onclick={() => onOpenFolder(entry.id)}>{entry.name}</button>
				{:else}
					<span class="truncate font-medium" title={entry.name}>{entry.name}</span>
				{/if}
				<span class="truncate font-mono text-xs text-(--muted)" title={entry.type}>{entry.type}</span>
				<span class="text-xs text-(--muted)">{formatBytes(entry.size)}</span>
			</div>
		</article>
	{/each}
	{#if rows.length === 0}
		<p class="col-span-full m-0 px-2 py-8 text-center text-(--muted)">This folder is empty.</p>
	{/if}
</div>
