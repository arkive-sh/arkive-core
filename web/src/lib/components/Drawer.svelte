<script lang="ts">
	import { X } from '@lucide/svelte';
	import { fade, fly } from 'svelte/transition';
	import type { Snippet } from 'svelte';

	type Props = {
		open: boolean;
		label: string;
		onClose: () => void;
		title: string;
		panelClass?: string;
		bodyClass?: string;
		actionsClass?: string;
		children?: Snippet;
		actions?: Snippet;
	};

	const id = $props.id();
	const titleId = `${id}-title`;
	let {
		open,
		label,
		title,
		onClose,
		panelClass = '',
		bodyClass = '',
		actionsClass = '',
		children,
		actions,
	}: Props = $props();
	let backdrop = $state<HTMLDivElement>();

	function closeTopmost(event: KeyboardEvent): void {
		if (event.key !== 'Escape' || !open) return;
		const drawers = document.querySelectorAll('[data-drawer-open="true"]');

		if (drawers.item(drawers.length - 1) === backdrop) onClose();
	}
</script>

<svelte:window onkeydown={closeTopmost} />

{#if open}
	<div
		class="fixed inset-0 z-50 bg-black/60"
		data-drawer-open="true"
		bind:this={backdrop}
		transition:fade|global={{ duration: 140 }}
	>
		<button
			class="absolute inset-0 cursor-pointer border-0 bg-transparent"
			type="button"
			aria-label={`Close ${label}`}
			tabindex="-1"
			onclick={onClose}
		></button>
		<div
			class={`relative z-10 ml-auto flex h-dvh w-full max-w-[480px] flex-col overflow-hidden border-l border-(--border) bg-(--bg) shadow-[-24px_0_64px_rgba(0,0,0,0.32)] ${panelClass}`}
			role="dialog"
			aria-modal="true"
			aria-labelledby={titleId}
			tabindex="-1"
			transition:fly|global={{ x: 32, duration: 180 }}
		>
			<header
				class="flex shrink-0 items-center justify-between gap-4 border-b border-(--border) px-5 py-4"
			>
				<h2 id={titleId} class="m-0 min-w-0 truncate text-lg font-semibold">{title}</h2>
				<button
					class="grid size-9 shrink-0 cursor-pointer place-items-center rounded border border-(--border) text-(--muted) hover:bg-(--surface-raised) hover:text-(--text)"
					type="button"
					aria-label={`Close ${label}`}
					onclick={onClose}
				>
					<X class="size-4" aria-hidden="true" />
				</button>
			</header>
			{#if children}
				<div class={`min-h-0 flex-1 overflow-y-auto px-5 py-5 ${bodyClass}`}>
					{@render children()}
				</div>
			{/if}
			{#if actions}
				<footer
					class={`flex shrink-0 justify-end gap-2.5 border-t border-(--border) px-5 py-4 ${actionsClass}`}
				>
					{@render actions()}
				</footer>
			{/if}
		</div>
	</div>
{/if}
