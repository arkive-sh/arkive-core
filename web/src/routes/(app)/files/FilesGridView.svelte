<script lang="ts">
	import { File as FileIcon, Folder } from '@lucide/svelte';
	import { api } from '#lib/api.ts';
	import { getCachedThumbnail, cacheThumbnail } from '#lib/thumbnail-cache.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import { decryptSearchThumbnail } from '#lib/vault/search.ts';
	import type { FileManagerEntry } from './types';

	const THUMBNAIL_CONCURRENCY = 4;
	let { rows, onOpenFolder }: { rows: FileManagerEntry[]; onOpenFolder: (id: string) => void } = $props();
	let thumbnails = $state<Record<string, string>>({});

	$effect(() => {
		const pending = rows.filter((entry) => entry.kind === 'file' && entry.thumbnail);
		const queue = [...pending];
		const controller = new AbortController();
		const objectUrls: string[] = [];
		thumbnails = {};

		async function loadNextThumbnail(): Promise<void> {
			while (!controller.signal.aborted) {
				const entry = queue.shift();
				if (!entry?.thumbnail) return;

				try {
					const preview = entry.thumbnail;
					let encryptedBytes = await getCachedThumbnail(entry.id, preview.version, preview.size);
					if (controller.signal.aborted) return;

					if (!encryptedBytes) {
						const { data } = await api.get<ArrayBuffer>(`/api/files/${encodeURIComponent(entry.id)}/thumbnail`, {
							responseType: 'arraybuffer',
							signal: controller.signal
						});
						if (controller.signal.aborted) return;
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
					if (controller.signal.aborted || !thumbnailBytes.length) continue;

					const url = URL.createObjectURL(new Blob([Uint8Array.from(thumbnailBytes)], { type: preview.mime }));
					objectUrls.push(url);
					thumbnails = { ...thumbnails, [entry.id]: url };
					thumbnailBytes.fill(0);
				} catch {
					// Missing previews leave the file card's icon in place.
				}
			}
		}

		void Promise.all(Array.from({ length: Math.min(THUMBNAIL_CONCURRENCY, queue.length) }, () => loadNextThumbnail()));

		return () => {
			controller.abort();
			for (const url of objectUrls) URL.revokeObjectURL(url);
			thumbnails = {};
		};
	});
</script>

<div class="grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-3 p-4 max-[640px]:grid-cols-[repeat(auto-fill,minmax(145px,1fr))]">
	{#each rows as entry (`${entry.kind}:${entry.id}`)}
		<article class="min-w-0 overflow-hidden rounded-md border border-(--border) bg-(--surface) transition-colors hover:border-(--accent)">
			<div class="grid aspect-[4/3] place-items-center overflow-hidden bg-(--surface-highest)">
				{#if entry.kind === 'folder'}
					<Folder class="size-10 text-(--accent)" aria-hidden="true" />
				{:else if thumbnails[entry.id]}
					<img class="size-full object-cover" src={thumbnails[entry.id]} alt={entry.name} loading="lazy" decoding="async" />
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
