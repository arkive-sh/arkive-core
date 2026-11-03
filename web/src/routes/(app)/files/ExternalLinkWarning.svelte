<script lang="ts">
	import Button from '#lib/components/Button.svelte';

	let { url, onClose, onContinue }: { url: string; onClose: () => void; onContinue: () => void } = $props();
	const id = $props.id();
	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		if (!dialog) return;
		if (url && !dialog.open) dialog.showModal();
		else if (!url && dialog.open) dialog.close();
	});

	function cancel(event: Event): void {
		event.preventDefault();
		onClose();
	}
</script>

<dialog
	bind:this={dialog}
	class="m-auto w-[min(460px,calc(100vw-2rem))] max-w-full overflow-hidden rounded-lg border border-(--border) bg-(--surface) p-0 text-(--text) shadow-[0_24px_64px_rgba(0,0,0,0.48)]"
	aria-labelledby="{id}-title"
	aria-describedby="{id}-description"
	oncancel={cancel}
>
	<div class="flex flex-col">
		<header class="border-b border-(--border) px-5 py-4">
			<h2 id="{id}-title" class="m-0 text-xl leading-7 font-semibold">Open external link?</h2>
		</header>
		<div class="grid gap-4 px-5 py-5">
			<p id="{id}-description" class="m-0 text-sm text-(--muted)">This link will take you outside Arkive. External websites may be unsafe. Continue only if you trust the destination.</p>
			<p class="m-0 break-all rounded border border-(--border) bg-(--bg) p-3 text-sm">{url}</p>
		</div>
		<footer class="flex justify-end gap-2.5 border-t border-(--border) bg-white/2 px-5 pt-4 pb-5">
			<Button text="Cancel" variant="secondary" onclick={onClose} />
			<Button text="Continue" variant="primary" onclick={onContinue} />
		</footer>
	</div>
</dialog>

<style>
	dialog { color: var(--text); }
	dialog::backdrop { background: rgb(0 0 0 / 78%); }
</style>
