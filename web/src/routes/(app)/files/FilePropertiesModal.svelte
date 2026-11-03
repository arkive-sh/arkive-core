<script lang="ts">
	import Modal from '#lib/components/Modal.svelte';
	import { formatBytes, formatDate } from '#lib/utils/format.ts';
	import type { FileManagerEntry } from './types';

	let {
		entry,
		location,
		onClose,
	}: { entry: FileManagerEntry | null; location: string; onClose: () => void } = $props();
</script>

<Modal open={entry !== null} label="Item properties" title={entry?.name ?? 'Properties'} {onClose}>
	{#if entry}
		<dl class="m-0 grid gap-3 text-sm">
			<div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3">
				<dt class="text-(--muted)">Name</dt>
				<dd class="m-0 min-w-0 wrap-break-word">{entry.name}</dd>
			</div>
			<div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3">
				<dt class="text-(--muted)">Type</dt>
				<dd class="m-0 min-w-0 wrap-break-word">{entry.type}</dd>
			</div>
			{#if entry.dimensions}
				<div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3">
					<dt class="text-(--muted)">Resolution</dt>
					<dd class="m-0">{entry.dimensions.width} × {entry.dimensions.height}</dd>
				</div>
			{/if}
			<div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3">
				<dt class="text-(--muted)">Size</dt>
				<dd class="m-0">{formatBytes(entry.size)}</dd>
			</div>
			<div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3">
				<dt class="text-(--muted)">Modified</dt>
				<dd class="m-0">{formatDate(entry.updatedAt)}</dd>
			</div>
			<div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-3">
				<dt class="text-(--muted)">Location</dt>
				<dd class="m-0 min-w-0 wrap-break-word">{location}</dd>
			</div>
		</dl>
	{/if}
</Modal>
