<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		label,
		children,
		disabled = false,
		className = ''
	}: {
		label: string;
		children: Snippet;
		disabled?: boolean;
		className?: string;
	} = $props();
</script>

<span class={`group/tooltip relative inline-flex ${className}`}>
	{@render children()}
	{#if !disabled}
		<span
			class="pointer-events-none invisible absolute top-1/2 left-full z-50 ml-2.5 -translate-y-1/2 whitespace-nowrap rounded-md border border-(--border) bg-[color-mix(in_srgb,var(--surface-highest)_88%,black)] px-2.5 py-1.5 text-xs font-medium text-(--text) opacity-0 shadow-lg transition-[opacity,visibility] duration-200 ease-out group-hover/tooltip:visible group-hover/tooltip:opacity-100 group-focus-within/tooltip:visible group-focus-within/tooltip:opacity-100"
			role="tooltip"
		>
			{label}
		</span>
	{/if}
</span>
