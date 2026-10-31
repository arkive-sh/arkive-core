<script lang="ts">
	import { X } from '@lucide/svelte';
	import type { FileManagerEntry } from './types';
	import EncryptedVideoPlayer from './EncryptedVideoPlayer.svelte';

	let { entry, onClose }: { entry: FileManagerEntry | null; onClose: () => void } = $props();
	let overlay = $state<HTMLDivElement>();

	function closeTopmost(event: KeyboardEvent): void {
		if (event.key !== 'Escape' || !entry) return;
		const overlays = document.querySelectorAll('[data-modal-open="true"]');
		if (overlays.item(overlays.length - 1) === overlay) onClose();
	}
</script>

<svelte:window onkeydown={closeTopmost} />

{#if entry}
	<div class="fixed inset-0 z-50 grid place-items-center bg-black/78 p-4" data-modal-open="true" bind:this={overlay}>
		<button class="absolute inset-0 cursor-pointer border-0 bg-transparent" type="button" aria-label="Close video preview" tabindex="-1" onclick={onClose}></button>
		<div class="relative z-1 w-[min(1200px,92vw,149vh)]" role="dialog" aria-modal="true" aria-label={`Video preview: ${entry.name}`} tabindex="-1">
			<button class="absolute -top-12 right-0 z-10 inline-flex size-10 cursor-pointer items-center justify-center rounded-full bg-black/45 text-white transition-colors hover:bg-black/70" type="button" aria-label="Close video preview" onclick={onClose}>
				<X class="size-5" aria-hidden="true" />
			</button>
			<EncryptedVideoPlayer fileId={entry.id} mimeType={entry.type} />
		</div>
	</div>
{/if}
