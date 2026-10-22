<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { cubicOut } from 'svelte/easing';

	type Props = {
		label: string;
		trigger: Snippet;
		children: Snippet;
		align?: 'left' | 'right';
		id?: string;
		rootClass?: string;
		triggerClass?: string;
		menuClass?: string;
	};

	let {
		label,
		trigger,
		children,
		align = 'right',
		id,
		rootClass = '',
		triggerClass = '',
		menuClass = ''
	}: Props = $props();
	let open = $state(false);
	let root: HTMLDivElement | undefined;
	let triggerButton: HTMLButtonElement | undefined;

	function dropdown(_node: HTMLElement) {
		return {
			duration: 160,
			easing: cubicOut,
			css: (progress: number) =>
				`opacity: ${progress}; transform: translateY(${(1 - progress) * -6}px) scale(${0.98 + progress * 0.02});`
		};
	}

	function close() {
		open = false;
	}

	onMount(() => {
		const closeOutside = (event: MouseEvent) => {
			if (!(event.target instanceof Node)) return;
			if (!root?.contains(event.target)) close();
			else if (!triggerButton?.contains(event.target) && event.target instanceof Element && event.target.closest('a, button')) close();
		};
		const closeOnEscape = (event: KeyboardEvent) => {
			if (event.key === 'Escape' && open) {
				close();
				triggerButton?.focus();
			}
		};

		document.addEventListener('click', closeOutside);
		window.addEventListener('keydown', closeOnEscape);
		return () => {
			document.removeEventListener('click', closeOutside);
			window.removeEventListener('keydown', closeOnEscape);
		};
	});
</script>

<div class={`relative inline-flex items-center ${open ? 'z-50' : ''} ${rootClass}`} bind:this={root}>
	<button
		class={`inline-flex cursor-pointer items-center justify-center border-0 bg-transparent p-0 ${triggerClass}`}
		type="button"
		aria-label={label}
		aria-haspopup="menu"
		aria-expanded={open}
		aria-controls={id ? `${id}-menu` : undefined}
		bind:this={triggerButton}
		onclick={() => (open = !open)}
	>
		{@render trigger()}
	</button>
	{#if open}
		<div
			id={id ? `${id}-menu` : undefined}
			class={`absolute top-[calc(100%+10px)] z-50 min-w-55 rounded-2xl border border-(--border) bg-(--surface) p-2.5 shadow-(--shadow) ${align === 'right' ? 'right-0' : 'left-0'} ${menuClass}`}
			role="menu"
			tabindex="-1"
			transition:dropdown
		>
			{@render children()}
		</div>
	{/if}
</div>
