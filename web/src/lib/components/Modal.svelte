<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		open: boolean;
		label: string;
		onClose: () => void;
		title?: string;
		backdropClass?: string;
		dialogClass?: string;
		bodyClass?: string;
		actionsClass?: string;
		header?: Snippet;
		children?: Snippet;
		actions?: Snippet;
	};

	const id = $props.id();
	const titleId = `${id}-title`;
	let {
		open,
		label,
		onClose,
		title,
		backdropClass = '',
		dialogClass = '',
		bodyClass = '',
		actionsClass = '',
		header,
		children,
		actions,
	}: Props = $props();
	let backdrop = $state<HTMLDivElement>();

	function closeTopmost(event: KeyboardEvent) {
		if (event.key !== 'Escape' || !open) return;
		const openModals = document.querySelectorAll('[data-modal-open="true"]');

		if (openModals.item(openModals.length - 1) === backdrop) onClose();
	}
</script>

<svelte:window onkeydown={closeTopmost} />

{#if open}
	<div
		{id}
		class={`fixed inset-0 z-40 grid place-items-center overflow-y-auto bg-black/78 p-4 max-[640px]:items-end max-[640px]:p-3 ${backdropClass}`}
		data-modal-open="true"
		bind:this={backdrop}
	>
		<button
			class="absolute inset-0 cursor-pointer border-0 bg-transparent"
			type="button"
			aria-label="Close dialog"
			tabindex="-1"
			onclick={onClose}
		></button>
		<div
			class={`relative z-1 flex max-h-[90vh] w-[min(460px,100%)] flex-col overflow-x-hidden overflow-y-auto rounded-lg border border-(--border) bg-(--surface) shadow-[0_24px_64px_rgba(0,0,0,0.48)] max-[640px]:max-h-[min(90vh,760px)] max-[640px]:w-full ${dialogClass}`}
			role="dialog"
			aria-modal="true"
			aria-labelledby={title && !header ? titleId : undefined}
			aria-label={title && !header ? undefined : label}
			tabindex="-1"
		>
			{#if header}
				{@render header()}
			{:else if title}
				<header
					class="flex items-center justify-between gap-3 border-b border-(--border) px-5.5 pt-5 pb-4.5 max-[640px]:px-4"
				>
					<h2 id={titleId} class="m-0 text-xl leading-7 font-semibold">{title}</h2>
				</header>
			{/if}
			{#if children}
				<div class={`grid gap-4 px-5.5 py-5 max-[640px]:px-4 ${bodyClass}`}>
					{@render children()}
				</div>
			{/if}
			{#if actions}
				<footer
					class={`flex justify-end gap-2.5 border-t border-(--border) bg-white/2 px-5.5 pt-4 pb-5 max-[640px]:flex-wrap max-[640px]:px-4 ${actionsClass}`}
				>
					{@render actions()}
				</footer>
			{/if}
		</div>
	</div>
{/if}
